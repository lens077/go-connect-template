package registry

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/lens077/go-connect-template/constants"
	"github.com/lens077/go-connect-template/internal/pkg/meta"

	confv1 "github.com/lens077/go-connect-template/internal/conf/v1"

	"github.com/hashicorp/consul/api"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

type ConsulRegistry struct {
	Addr   string
	ID     string
	Name   string
	client *api.Client
	logger *zap.Logger
	// cancelPing 停止 TTL 心跳 goroutine;仅由 OnStart 写入、OnStop 读取,不并发访问。
	cancelPing context.CancelFunc
}

type Option func(*options)
type options struct {
	logger  *zap.Logger
	tlsConf *api.TLSConfig
	scheme  string
}

// WithLogger 注入日志器
func WithLogger(logger *zap.Logger) Option {
	return func(o *options) {
		o.logger = logger
	}
}

// WithTLS Consul TLS配置
func WithTLS(insecureSkipVerify bool, caPem string) Option {
	return func(o *options) {
		o.tlsConf = &api.TLSConfig{
			CAPem:              []byte(caPem),
			InsecureSkipVerify: insecureSkipVerify,
		}
	}
}

// Module 提供 Fx 模块
var Module = fx.Module("registry",
	fx.Provide(
		// 提供 Consul 注册中心（支持优雅降级）
		//
		// 返回 (nil, nil) 表示「没有注册中心也照常跑」,因此所有下游消费者都必须自己判空。
		func(lc fx.Lifecycle, logger *zap.Logger, conf *confv1.Bootstrap, appInfo meta.AppInfo) (*ConsulRegistry, error) {
			if os.Getenv(constants.EnvConsulEnabled) == "false" ||
				conf.Discovery == nil || conf.Discovery.Consul == nil || conf.Discovery.Consul.Addr == "" {
				logger.Info("Consul disabled or not configured, service discovery disabled")
				return nil, nil
			}
			consulCfg := conf.Discovery.Consul

			opts := []Option{
				WithLogger(logger),
			}
			// 先判空再取 Enable：反过来写会在未配置 tls 段时直接空指针 panic
			if consulCfg.Tls != nil && consulCfg.Tls.Enable {
				opts = append(opts, WithTLS(consulCfg.Tls.InsecureSkipVerify, consulCfg.Tls.CaPem))
			}

			reg, err := NewConsulRegistry(consulCfg.Addr, appInfo.ID, appInfo.Name, opts...)
			if err != nil {
				logger.Warn("failed to initialize Consul client, service discovery disabled", zap.Error(err))
				return nil, nil // 降级运行
			}

			// 使用生命周期钩子自动注册、启动心跳和注销
			lc.Append(fx.Hook{
				OnStart: func(ctx context.Context) error {
					if err := reg.Register(conf, appInfo); err != nil {
						logger.Warn("failed to register with Consul, service discovery disabled", zap.Error(err))
						return nil // 允许应用继续运行
					}

					// OnStart 的 ctx 只是启动阶段的超时控制,启动一结束就会被取消。
					// 心跳是常驻的,必须另起一个与应用同寿命的 context,否则 pinger
					// 会在启动窗口结束时静默退出,服务随后被 Consul 判为 critical。
					pingCtx, cancel := context.WithCancel(context.Background())
					reg.cancelPing = cancel

					// 启动 TTL 心跳 Pinger
					go reg.TtlCheckPinger(pingCtx, conf)
					return nil
				},
				OnStop: func(ctx context.Context) error {
					// Register 失败时 cancelPing 为 nil；client 为 nil 时下面会空指针
					if reg == nil || reg.client == nil {
						return nil
					}
					if reg.cancelPing != nil {
						reg.cancelPing()
					}
					if err := reg.Deregister(); err != nil {
						logger.Warn("failed to deregister from Consul", zap.Error(err))
					}
					return nil
				},
			})
			return reg, nil
		},
	),
)

func NewConsulRegistry(addr, ID, Name string, opts ...Option) (*ConsulRegistry, error) {
	o := &options{
		scheme: constants.ConsulScheme,
	}
	for _, opt := range opts {
		opt(o)
	}

	config := api.Config{
		Address: addr,
		Scheme:  o.scheme,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: 5 * time.Second, // 建立连接超时
			}).DialContext,
			TLSHandshakeTimeout: 5 * time.Second, // TLS 握手超时
		},
		WaitTime: 10 * time.Second,
	}

	if o.tlsConf != nil {
		config.Scheme = constants.ConsulTlsScheme
		config.TLSConfig = *o.tlsConf
	}

	client, err := api.NewClient(&config)
	if err != nil {
		return nil, err
	}

	return &ConsulRegistry{
		ID:     ID,
		Name:   Name,
		Addr:   addr,
		client: client,
		logger: o.logger,
	}, nil
}

// Register 使用 TTL 健康检查注册服务
func (r *ConsulRegistry) Register(conf *confv1.Bootstrap, info meta.AppInfo) error {
	r.logger.Debug("registering service to Consul", zap.String("id", r.ID))
	// 使用服务本身的地址和端口，而不是 Consul 的地址
	host := info.Host
	// 从服务配置中获取端口
	_, portStr, err := net.SplitHostPort(conf.Server.Addr)
	if err != nil {
		return err
	}
	portNum, err := strconv.Atoi(portStr)
	if err != nil {
		return err
	}
	reg := &api.AgentServiceRegistration{
		ID:      r.ID,
		Name:    r.Name,
		Address: host,
		Port:    portNum,
		// 服务名已经是 Name 字段，再塞进 Tags 只会让按 tag 过滤时永远命中
		Tags: []string{
			info.Version,
			constants.ConsulTagFx,
			constants.ConsulTagTtl,
		},
		Check: &api.AgentServiceCheck{
			// 使用 TTL 替换 HTTP/TCP 检查
			TTL: conf.Discovery.Consul.Check.Ttl.Duration,
			// 配置在检查失败后自动注销
			DeregisterCriticalServiceAfter: conf.Discovery.Consul.Check.DeregisterCriticalServiceAfter,
		},
	}
	r.logger.Debug("service registration completed", zap.String("id", r.ID))

	if err := r.client.Agent().ServiceRegister(reg); err != nil {
		r.logger.Error("failed to register service with Consul", zap.Error(err))
		return err
	}

	r.logger.Info("Service registered with Consul using TTL check", zap.String("id", r.ID), zap.String("ttl", conf.Discovery.Consul.Check.Ttl.Duration))
	return nil
}

// TtlCheckPinger 负责定期向 Consul Agent 发送心跳信号
func (r *ConsulRegistry) TtlCheckPinger(ctx context.Context, conf *confv1.Bootstrap) {
	TtlPingInterval := conf.Discovery.Consul.Check.Ttl.PingInterval.AsDuration()
	ticker := time.NewTicker(TtlPingInterval)
	defer ticker.Stop()

	// Consul Agent 要求 CheckID 必须是 "service:<ID>" 的格式
	checkID := fmt.Sprintf("service:%s", r.ID)

	r.logger.Info("starting ttl pinger", zap.Duration("interval", TtlPingInterval), zap.String("checkID", checkID))

	for {
		select {
		case <-ctx.Done():
			r.logger.Info("ttl pinger stopped gracefully")
			return
		case <-ticker.C:
			// 发送 'pass' 状态的心跳
			err := r.client.Agent().UpdateTTL(checkID, "ttl check passing", api.HealthPassing)
			if err != nil {
				// 记录错误，但不退出 Pinger，因为这可能是暂时的网络问题
				// 如果长时间失败，Consul Agent 会将服务标记为 Critical
				r.logger.Error("failed to update Consul TTL", zap.Error(err), zap.String("ID", r.ID))
			}
		}
	}
}

func (r *ConsulRegistry) Deregister() error {
	r.logger.Info("deregistering service from consul", zap.String("id", r.ID))
	return r.client.Agent().ServiceDeregister(r.ID)
}
