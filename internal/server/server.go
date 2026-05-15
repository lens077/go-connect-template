package server

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"connectrpc.com/connect"
	connectcors "connectrpc.com/cors"
	"connectrpc.com/validate"
	"github.com/lens077/go-connect-template/api/search/v1/searchv1connect"
	conf "github.com/lens077/go-connect-template/internal/conf/v1"
	"github.com/lens077/go-connect-template/internal/data"
	"github.com/rs/cors"
	"go.uber.org/fx"
	"go.uber.org/zap"
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
	searchv1Service searchv1connect.SearchServiceHandler,
	logger *zap.Logger,
	connectOptions []connect.HandlerOption,
	deps *data.Data, // 基础设施依赖
) *http.Server {

	mux := http.NewServeMux()

	// 将 validate 拦截器添加到选项中
	combinedOptions := append(connectOptions, connect.WithInterceptors(validate.NewInterceptor()))

	// 注册 Connect 业务处理器
	searchv1connectPath, searchv1connectHandler := searchv1connect.NewSearchServiceHandler(
		searchv1Service,
		combinedOptions...,
	)
	mux.Handle(searchv1connectPath, searchv1connectHandler)

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

	server := &http.Server{
		Addr:         cfg.Server.Addr,
		Handler:      handlerChain,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
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
