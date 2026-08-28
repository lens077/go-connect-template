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
		// WithTrustRemote:采信上游传来的 traceparent,把本 span 挂成它的子 span。
		// 不加这个选项时 otelconnect 会强制 WithNewRoot(),把上游 context 降级成
		// 一条 link —— 结果是网关和本服务在 Jaeger 里是两条独立的 trace,点不进去。
		// 本服务只从网关入站(网关已做 JWT 鉴权),这个信任边界成立。
		otelInterceptor, err := otelconnect.NewInterceptor(
			otelconnect.WithTrustRemote(),
			otelconnect.WithoutServerPeerAttributes(),
		)
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
