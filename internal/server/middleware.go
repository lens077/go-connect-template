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

// NewConnectOptions 与 otel、config 一样直接消费 *confv1.Bootstrap:
// fx 图里只有 Bootstrap 这一个配置 provider,子 message 由各消费方自己取。
// 曾经这里要 *confv1.Observability,而没有任何地方 provide 它,服务在 fx 构建阶段就起不来;
// go build / go vet 抓不到这种错,只有真正启动才会暴露。
func NewConnectOptions(
	logger *zap.Logger,
	logging *LoggingInterceptor,
	conf *confv1.Bootstrap,
) []connect.HandlerOption {
	var interceptors []connect.Interceptor

	// 只有当 observability 启用时才添加 otel 拦截器(GetObservability 对 nil 安全)
	if observability := conf.GetObservability(); observability.GetEnable() {
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
