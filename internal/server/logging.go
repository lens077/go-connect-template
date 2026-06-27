package server

import (
	"context"
	"time"

	"connectrpc.com/connect"
	"go.opentelemetry.io/otel/attribute"
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
		start := time.Now()
		resp, err := next(ctx, req)
		duration := time.Since(start)

		span := trace.SpanFromContext(ctx)
		traceID := span.SpanContext().TraceID().String()
		spanID := span.SpanContext().SpanID().String()

		span.SetAttributes(
			attribute.String("rpc.method", req.Spec().Procedure),
			attribute.String("rpc.code", connect.CodeOf(err).String()),
			attribute.Int64("rpc.duration_ms", duration.Milliseconds()),
		)

		fields := []zap.Field{
			zap.String("rpc.procedure", req.Spec().Procedure),
			zap.String("rpc.code", connect.CodeOf(err).String()),
			zap.String("span_id", spanID),
			zap.String("trace_id", traceID),
			zap.Int64("duration_ms", duration.Milliseconds()),
		}

		if err != nil {
			span.RecordError(err)
			switch connect.CodeOf(err) {
			case connect.CodeInternal, connect.CodeUnknown, connect.CodeDataLoss:
				span.AddEvent("rpc_system_error", trace.WithAttributes(
					attribute.String("message", err.Error()),
				))
				l.logger.Error("rpc system error", append(fields, zap.Error(err))...)
			case connect.CodeDeadlineExceeded, connect.CodeUnavailable, connect.CodeAborted:
				span.AddEvent("rpc_infrastructure_warning", trace.WithAttributes(
					attribute.String("message", err.Error()),
				))
				l.logger.Warn("rpc infrastructure warning", append(fields, zap.Error(err))...)
			case connect.CodeCanceled:
				span.AddEvent("rpc_request_canceled", trace.WithAttributes(
					attribute.String("message", err.Error()),
				))
				l.logger.Debug("rpc request canceled by client", append(fields, zap.Error(err))...)
			default:
				span.AddEvent("rpc_business_exception", trace.WithAttributes(
					attribute.String("message", err.Error()),
				))
				l.logger.Info("rpc business exception", fields...)
			}
		} else {
			span.AddEvent("rpc_completed", trace.WithAttributes(
				attribute.Int64("duration_ms", duration.Milliseconds()),
			))
			l.logger.Info("rpc completed", fields...)
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
