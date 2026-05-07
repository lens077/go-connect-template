package server

import (
	"context"

	"connectrpc.com/connect"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type LoggingInterceptor struct {
	logger *zap.Logger
}

func NewLoggingInterceptor(logger *zap.Logger) *LoggingInterceptor {
	return &LoggingInterceptor{logger: logger.Named("LoggingInterceptor")}
}

func (l *LoggingInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		resp, err := next(ctx, req)

		// 从 otelconnect 已经注入好的 ctx 中获取 SpanContext
		span := trace.SpanFromContext(ctx)
		traceID := span.SpanContext().TraceID().String()

		// 只记录一条统一的调用完成日志
		fields := []zap.Field{
			zap.String("rpc.procedure", req.Spec().Procedure),
			zap.String("rpc.code", connect.CodeOf(err).String()),
			zap.String("trace_id", traceID),
		}

		switch connect.CodeOf(err) {
		// 需要立即关注的系统错误
		case connect.CodeInternal, connect.CodeUnknown, connect.CodeDataLoss:
			l.logger.Error("rpc system error", append(fields, zap.Error(err))...)

		// 可能代表性能瓶颈或不稳定的环境
		case connect.CodeDeadlineExceeded, connect.CodeUnavailable, connect.CodeAborted:
			l.logger.Warn("rpc infrastructure warning", append(fields, zap.Error(err))...)

		// 通常是噪音，不需要在生产环境报警
		case connect.CodeCanceled:
			l.logger.Debug("rpc request canceled by client", append(fields, zap.Error(err))...)

		// 正常的业务阻断（非法参数、权限不足等）
		default:
			l.logger.Info("rpc business exception", fields...)
		}

		return resp, err
	}
}

func (l *LoggingInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (l *LoggingInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next
}
