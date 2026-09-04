package main

import (
	"context"
	"errors"
	"flag"
	"net/http"
	"os"
	"time"

	"github.com/lens077/go-connect-kit/env"
	"github.com/lens077/go-connect-kit/meta"
	kitregistry "github.com/lens077/go-connect-kit/registry" // +co:consul
	"github.com/lens077/go-connect-template/constants"
	"github.com/lens077/go-connect-template/internal/biz"
	confv1 "github.com/lens077/go-connect-template/internal/conf/v1"
	"github.com/lens077/go-connect-template/internal/data"
	"github.com/lens077/go-connect-template/internal/pkg/config"
	logger "github.com/lens077/go-connect-template/internal/pkg/log"
	"github.com/lens077/go-connect-template/internal/pkg/otel"
	"github.com/lens077/go-connect-template/internal/pkg/registry" // +co:consul
	"github.com/lens077/go-connect-template/internal/server"
	"github.com/lens077/go-connect-template/internal/service"

	"github.com/google/uuid"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

var (
	serviceName    = flag.String("serviceName", env.GetEnvString(constants.EnvServiceName, "org-service"), "应用名称, e.g.,org-service")
	serviceVersion = flag.String("serviceVersion", env.GetEnvString(constants.EnvServiceVersion, "v1"), "应用版本,e.g.,v1")
	deploymentMode = flag.String("deploymentMode", env.GetEnvString(constants.EnvDeploymentMode, "dev"), "标记应用部署的环境,e.g.,dev/prod/pre/uat")
)

func main() {
	flag.Parse()

	fxApp := NewApp(
		*serviceName,
		*deploymentMode,
		*serviceVersion,
	)

	// 启动应用
	if err := fxApp.Start(context.Background()); err != nil {
		zap.Error(err)
		os.Exit(1)
	}

	// 等待中断信号
	<-fxApp.Done()

	// 优雅关闭
	// 定制一个超时的 Context
	// 确保所有微服务的 OnStop 钩子（包括 Consul 注销、HTTP 关闭、OTel 刷盘）必须在定义的值内收尾
	stopCtx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
	defer cancel()
	if err := fxApp.Stop(stopCtx); err != nil {
		zap.Error(err)
		os.Exit(1)
	}
}

// NewApp 创建并配置 FX 应用
func NewApp(serviceName, deploymentMode, serviceVersion string) *fx.App {
	host, err := meta.GetOutboundIP()
	if err != nil {
		zap.Error(err)
	}
	appInfo := meta.AppInfo{
		ID:          uuid.New().String(),
		Name:        serviceName,
		Version:     serviceVersion,
		Host:        host,
		Environment: deploymentMode,
	}

	return fx.New(
		// 基础模块
		logger.Module,     // 日志
		config.Module,     // 配置
		logger.FxLogger(), // Fx框架本身的日志控制器

		registry.Module, // 服务注册/发现 +co:consul

		// 可观测性 - 根据配置决定是否启用
		otel.Module,

		// 注入业务模块（按依赖顺序）
		data.Module,
		biz.Module,
		service.Module,
		server.MiddlewareModule, // 中间件需要在服务模块之前
		server.Module,

		// 传递全局变量
		fx.Supply(appInfo),

		// 配置验证和初始化
		fx.Invoke(
			// 启动之前初始化 Consul 注册中心
			// +co:begin consul
			func(reg *kitregistry.ConsulRegistry, logger *zap.Logger) {
				if reg != nil {
					logger.Info("consul service discovery component lifecycle successfully initialized")
				}
			},
			// +co:end

			// 初始化并启动核心应用逻辑
			func(lc fx.Lifecycle, conf *confv1.Bootstrap, live *config.Live, d *data.Data, logger *zap.Logger, srv *http.Server, otelShutdown func(context.Context) error) {
				lc.Append(fx.Hook{
					// 启动服务时的操作
					OnStart: func(ctx context.Context) error {
						// 打出配置源:线上排查「为什么读到的是旧配置」时,
						// 第一件事就是确认这份配置到底来自文件还是 Consul
						logger.Info("performing startup health checks...",
							zap.String("configSource", live.SourceName()),
						)

						// 检查数据库
						if err := d.CheckDatabase(ctx); err != nil {
							return err
						}
						// 检查缓存
						// +co:begin redis
						if err := d.CheckCache(ctx); err != nil {
							return err
						}
						// +co:end
						// 检索后端是可降级依赖:连不上只让搜索不可用,不阻塞其它业务启动。
						// +co:begin elasticsearch|meilisearch
						if err := d.CheckSearch(ctx); err != nil {
							logger.Warn("search catalog unreachable, search will degrade", zap.Error(err))
						}
						// +co:end

						logger.Info("starting server",
							zap.String("addr", srv.Addr),
							zap.String("environment", deploymentMode),
						)
						go func() {
							if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
								logger.Fatal("failed to start server", zap.Error(err))
							}
						}()
						return nil
					},
					// 停止服务前的操作
					OnStop: func(ctx context.Context) error {
						logger.Info("stopping server...")
						// 关闭服务器
						if err := srv.Shutdown(ctx); err != nil {
							logger.Error("failed to shutdown server gracefully", zap.Error(err))
						}

						// 关闭transport 维护的空闲 TCP 连接
						if t, ok := http.DefaultTransport.(*http.Transport); ok {
							t.CloseIdleConnections()
						}

						// 关闭otel
						// 1. trace: 强制将内存中还没发出的 Span（链路数据）通过 HTTP 刷给 Collector
						// 2. metric: 它会触发最后一次指标收集，并确保数据推送到后端
						// 3. logging: 确保内存中的日志数据全部持久化
						if otelShutdown != nil {
							return otelShutdown(ctx) // 执行聚合后的停止逻辑
						}
						return nil
					},
				})
			},
		),
	)
}
