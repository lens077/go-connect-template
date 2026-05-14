package service

import (
	"net/http"

	"connectrpc.com/connect"
	"go.uber.org/fx"
)

var Module = fx.Module("service",
	fx.Provide(NewUserService),
)

// Service 定义服务接口
type Service interface {
	RegisterHandlers(mux *http.ServeMux, options ...connect.HandlerOption)
}
