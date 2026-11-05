package server

import (
	"context"
	"encoding/json"
	"net/http"

	"connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connecthttp"
	connectcors "connectrpc.com/cors"
	"github.com/lens077/go-connect-template/api/search/v1/searchv1connect" // +co:example
	// +co:anchor server-imports
	conf "github.com/lens077/go-connect-template/internal/conf/v1"
	"github.com/lens077/go-connect-template/internal/data"
	"github.com/rs/cors"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

var Module = fx.Module("server",
	fx.Provide(
		NewHTTPServer,
	),
)

// NewHTTPServer 构造函数已重构
func NewHTTPServer(
	lc fx.Lifecycle,
	cfg *conf.Bootstrap,
	searchv1Service searchv1connect.SearchServiceHandler, // +co:example
	// +co:anchor server-handler-params
	logger *zap.Logger,
	interceptors []connect.ServerInterceptor,
	deps *data.Data, // 基础设施依赖
) *http.Server {

	mux := http.NewServeMux()

	// connect v2:拦截器挂在 Server 上,对 Server 上全部服务生效,不再有
	// per-handler 选项(v1 逐 handler 复制选项切片,漏一步就「只有最后一个
	// 服务带拦截器」);HTTP 绑定经 connecthttp.Mount,默认按 content-type
	// 在同一端口协商 Connect/gRPC/gRPC-Web 三种协议。
	// NewServer 的拦截器对 Server 上全部服务生效。注意顺序:所有 Register
	// 都必须在 Mount 之前 —— Mount 按 Server 当前的过程表装配 HTTP 路由,
	// 后注册的过程不再被路由。co resource add / proto gen 插的注册行走
	// server-handler-register 锚点,正好落在 Mount 之前。
	server := connect.NewServer(interceptors...)
	// +co:begin example
	searchv1connect.RegisterSearchServiceHandler(server, searchv1Service)
	// +co:end
	// +co:anchor server-handler-register
	connecthttp.Mount(mux, server)

	// 应用本身的健康检查
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		status := healthStatus(r.Context(), deps)
		w.Header().Set("Content-Type", "application/json")
		if !status.Healthy {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		json.NewEncoder(w).Encode(status)
	})

	// 构建处理器链
	handlerChain := withCORS(mux, cfg.Server.Cors.AllowedOrigins)

	// 配置 HTTP/2 (H2C - 明文 HTTP/2)
	h2s := &http2.Server{}
	// 使用 h2c 包装处理器，支持同时处理 HTTP/1.1 和 HTTP/2
	handlerChain = h2c.NewHandler(handlerChain, h2s)

	httpServer := &http.Server{
		Addr:         cfg.Server.Addr,
		Handler:      handlerChain,
		ReadTimeout:  cfg.Server.Http.ReadTimeout.AsDuration(),
		WriteTimeout: cfg.Server.Http.WriteTimeout.AsDuration(),
		IdleTimeout:  cfg.Server.Http.IdleTimeout.AsDuration(),
	}

	// 注册 Fx 生命周期
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			logger.Info("http server starting",
				zap.String("addr", cfg.Server.Addr),
			)
			return nil
		},
		OnStop: func(ctx context.Context) error {
			logger.Info("http server shutting down...")
			return httpServer.Shutdown(ctx)
		},
	})

	return httpServer
}

// withCORS 为处理器添加跨域支持
func withCORS(h http.Handler, allowedOrigins []string) http.Handler {
	middleware := cors.New(cors.Options{
		AllowedOrigins:   allowedOrigins,
		AllowedMethods:   connectcors.AllowedMethods(),
		AllowedHeaders:   connectcors.AllowedHeaders(),
		ExposedHeaders:   connectcors.ExposedHeaders(),
		AllowCredentials: true,
	})
	return middleware.Handler(h)
}
