package server

import (
	"connectrpc.com/connect/v2"
	"connectrpc.com/otelconnect"
	"connectrpc.com/validate"
	confv1 "github.com/lens077/go-connect-template/internal/conf/v1"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

var MiddlewareModule = fx.Module("server.middleware",
	fx.Provide(
		// 提供拦截器实例
		NewLoggingInterceptor,

		// 组装成 connect v2 服务端拦截器切片
		NewConnectOptions,
	),
)

// NewConnectOptions 与 otel、config 一样直接消费 *confv1.Bootstrap:
// fx 图里只有 Bootstrap 这一个配置 provider,子 message 由各消费方自己取。
// 曾经这里要 *confv1.Observability,而没有任何地方 provide 它,服务在 fx 构建阶段就起不来;
// go build / go vet 抓不到这种错,只有真正启动才会暴露。
//
// v2 里拦截器按「最先注册的最先运行」排序;validate 放最外层,坏消息在
// 记日志/开 span 之前就被打回,otel 与日志只服务真正进入业务链的请求。
func NewConnectOptions(
	logger *zap.Logger,
	logging connect.ServerInterceptor,
	conf *confv1.Bootstrap,
) []connect.ServerInterceptor {
	var interceptors []connect.ServerInterceptor

	// 请求体校验始终启用;v2 的 validate 包装 ServerStream,在消息进入
	// 业务 handler 之前完成校验。
	interceptors = append(interceptors, validate.NewServerInterceptor())

	// 只有当 observability 启用时才添加 otel 拦截器(GetObservability 对 nil 安全)
	if observability := conf.GetObservability(); observability.GetEnable() {
		// WithTrustRemote:采信上游传来的 traceparent,把本 span 挂成它的子 span。
		// 不加这个选项时 otelconnect 会强制 WithNewRoot(),把上游 context 降级成
		// 一条 link —— 结果是网关和本服务在 Jaeger 里是两条独立的 trace,点不进去。
		// 本服务只从网关入站(网关已做 JWT 鉴权),这个信任边界成立。
		otelInterceptor, err := otelconnect.NewServerInterceptor(
			otelconnect.WithTrustRemote(),
			// v1 的 WithoutServerPeerAttributes 在 v2 已成默认(v2 改为
			// WithServerPeerAttributes 选择开启),不再显式传。
		)
		if err != nil {
			logger.Fatal("failed to init otel interceptor", zap.Error(err))
		}
		interceptors = append(interceptors, otelInterceptor)
	}

	// 添加日志拦截器
	interceptors = append(interceptors, logging)

	return interceptors
}
