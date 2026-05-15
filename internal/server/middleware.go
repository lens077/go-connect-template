package server

import (
	"connectrpc.com/connect"
	"connectrpc.com/otelconnect"
	confv1 "github.com/lens077/go-connect-template/internal/conf/v1"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

var MiddlewareModule = fx.Module("server.middleware",
	fx.Provide(
		// 提供拦截器实例
		NewLoggingInterceptor,

		// 组装成一个拦截器切片，或者直接返回 Connect Option
		NewConnectOptions,
	),
)

func NewConnectOptions(
	logger *zap.Logger,
	logging *LoggingInterceptor,
	observability *confv1.Observability,
) []connect.HandlerOption {
	var interceptors []connect.Interceptor

	// 只有当 observability 启用时才添加 otel 拦截器
	if observability != nil && observability.Enable {
		otelInterceptor, err := otelconnect.NewInterceptor()
		if err != nil {
			logger.Fatal("failed to init otel interceptor", zap.Error(err))
		}
		interceptors = append(interceptors, otelInterceptor)
	}

	// 添加日志拦截器
	interceptors = append(interceptors, logging)

	return []connect.HandlerOption{
		connect.WithInterceptors(interceptors...),
	}
}
