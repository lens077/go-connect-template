package server

import (
	"context"
	"encoding/json"
	"net/http"

	"connectrpc.com/connect"
	connectcors "connectrpc.com/cors"
	"connectrpc.com/validate"
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
	connectOptions []connect.HandlerOption,
	deps *data.Data, // 基础设施依赖
) *http.Server {

	mux := http.NewServeMux()

	// 注册 Connect 业务处理器
	// +co:begin example
	searchv1connectPath, searchv1connectHandler := searchv1connect.NewSearchServiceHandler(
		searchv1Service,
		handlerOptions(connectOptions)...,
	)
	mux.Handle(searchv1connectPath, searchv1connectHandler)
	// +co:end
	// +co:anchor server-handler-register

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

	server := &http.Server{
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
			return server.Shutdown(ctx)
		},
	})

	return server
}

// handlerOptions 返回注册 Connect handler 用的选项:全局选项 + validate 拦截器。
//
// 写成函数而不是 NewHTTPServer 里的一个局部变量,是为了让「一个 handler 都不注册」
// 的骨架也编译得过 —— 未使用的局部变量是编译错误,未使用的函数不是。
// co new --no-resource 生成的正是这种骨架。
//
// 每次调用复制一份,不直接 append 调用方那个切片:多个 handler 各 append 一次
// 同一个底层数组时会互相覆盖最后一格,表现为「只有最后注册的那个服务带上了
// validate 拦截器」,而且不报错。
func handlerOptions(base []connect.HandlerOption) []connect.HandlerOption {
	out := make([]connect.HandlerOption, 0, len(base)+1)
	out = append(out, base...)
	return append(out, connect.WithInterceptors(validate.NewInterceptor()))
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
