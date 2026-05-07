package server

import (
	"connectrpc.com/connect"
	"connectrpc.com/otelconnect"
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
) []connect.HandlerOption {

	otelInterceptor, err := otelconnect.NewInterceptor()
	if err != nil {
		logger.Fatal("failed to init otel interceptor", zap.Error(err))
	}

	return []connect.HandlerOption{
		connect.WithInterceptors(
			otelInterceptor,
			logging,
		),
	}
}
