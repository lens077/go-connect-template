package main

import (
	"context"
	"errors"
	"flag"
	"net/http"
	"os"

	"github.com/lens077/go-connect-template/constants"
	"github.com/lens077/go-connect-template/internal/biz"
	"github.com/lens077/go-connect-template/internal/pkg/env"
	"github.com/lens077/go-connect-template/internal/pkg/meta"
	"github.com/lens077/go-connect-template/internal/pkg/otel"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap/zapcore"

	confv1 "github.com/lens077/go-connect-template/internal/conf/v1"
	"github.com/lens077/go-connect-template/internal/data"
	"github.com/lens077/go-connect-template/internal/pkg/config"
	logger "github.com/lens077/go-connect-template/internal/pkg/log"
	"github.com/lens077/go-connect-template/internal/pkg/registry"
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

	ctx := context.Background()

	// 启动应用
	if err := fxApp.Start(ctx); err != nil {
		zap.Error(err)
		os.Exit(1)
	}

	// 等待中断信号
	<-fxApp.Done()

	// 优雅关闭
	if err := fxApp.Stop(ctx); err != nil {
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
		logger.Module, // 日志
		config.Module, // 配置
		// 注入 FX 事件日志适配器
		fx.WithLogger(func(log *zap.Logger) fxevent.Logger {
			zlog := &fxevent.ZapLogger{Logger: log}
			// 按需调整日志级别（可选）
			zlog.UseLogLevel(zapcore.InfoLevel)    // 普通事件用 Info 级别
			zlog.UseErrorLevel(zapcore.ErrorLevel) // 错误事件用 Error 级别
			return zlog
		}),

		registry.Module, // 服务注册/发现

		// 可观测性 - 根据配置决定是否启用
		fx.Provide(func(conf *confv1.Bootstrap) *confv1.Observability {
			if conf.Observability == nil {
				return &confv1.Observability{Enable: false}
			}
			return conf.Observability
		}),
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
			// 注册应用到注册中心
			func(_ *registry.ConsulRegistry) {},

			// 初始化并启动核心应用逻辑
			func(lc fx.Lifecycle, conf *confv1.Bootstrap, d *data.Data, logger *zap.Logger, srv *http.Server, otelShutdown func(context.Context) error) {
				lc.Append(fx.Hook{
					// 启动服务时的操作
					OnStart: func(ctx context.Context) error {
						logger.Info("performing startup health checks...")

						// 检查数据库
						if err := d.CheckDatabase(ctx); err != nil {
							return err
						}
						// 检查缓存
						if err := d.CheckCache(ctx); err != nil {
							return err
						}
						// 检查 Elasticsearch
						if err := d.CheckElasticSearch(ctx); err != nil {
							return err
						}

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
