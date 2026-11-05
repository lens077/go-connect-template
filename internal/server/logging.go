package server

import (
	"connectrpc.com/connect/v2"
	"github.com/lens077/go-connect-kit/meta"
	"github.com/lens077/go-connect-kit/rpcobs"
	"go.uber.org/zap"
)

// NewLoggingInterceptor 把 RPC 日志与错误规范化下沉到 kit/rpcobs,服务不再各存一份。
// 服务名作为返回给客户端的 google.rpc.ErrorInfo.Domain;reason 只在 domain 内唯一,
// 前端据 (domain, reason) 判断错误。rpcobs 的实现是 connect v2 的
// ServerInterceptor,unary 与 streaming 一并覆盖。
func NewLoggingInterceptor(logger *zap.Logger, info meta.AppInfo) connect.ServerInterceptor {
	return rpcobs.New(logger, rpcobs.WithDomain(info.Name))
}
