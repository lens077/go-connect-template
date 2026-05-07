package server

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"connectrpc.com/connect"
	connectcors "connectrpc.com/cors"
	"connectrpc.com/validate"
	"github.com/lens077/go-connect-template/api/user/v1/userv1connect"
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
	userv1Service userv1connect.UserServiceHandler,
	logger *zap.Logger,
	connectOptions []connect.HandlerOption,
	deps *data.Data, // 基础设施依赖
) *http.Server {

	mux := http.NewServeMux()

	// validate 拦截器
	validateInterceptor := validate.NewInterceptor()

	// 将 validate 拦截器本身也作为一个 connect.HandlerOption
	combinedOptions := append(
		connectOptions,
		connect.WithInterceptors(validateInterceptor),
	)

	// 注册 Connect 业务处理器
	// 直接展开 (Variadic) 传入所有的拦截器
	userv1connectPath, userv1connectHandler := userv1connect.NewUserServiceHandler(
		userv1Service,
		combinedOptions...,
	)
	mux.Handle(userv1connectPath, userv1connectHandler)

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

	// 配置 HTTP/2 (h2c) 允许非加密传输
	p := new(http.Protocols)
	p.SetHTTP1(true)
	p.SetUnencryptedHTTP2(true)

	server := &http.Server{
		Addr:         cfg.Server.Addr,
		Handler:      h2c.NewHandler(handlerChain, &http2.Server{}),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
		Protocols:    p,
	}

	// 注册 Fx 生命周期
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			logger.Info("http server starting",
				zap.String("addr", cfg.Server.Addr),
				zap.String("mode", "h2c"),
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
