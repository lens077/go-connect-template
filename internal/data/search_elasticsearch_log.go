package data

import (
	"net/http"
	"time"

	confv1 "github.com/lens077/go-connect-template/internal/conf/v1"
	"go.uber.org/zap"
)

type zapESLogger struct {
	logger *zap.Logger
	conf   *confv1.Log_Search
}

func (logger *zapESLogger) RequestBodyEnabled() bool {
	return logger.conf.GetEnableRequest()
}

func (logger *zapESLogger) ResponseBodyEnabled() bool {
	return logger.conf.GetEnableResponse()
}

func (logger *zapESLogger) LogRoundTrip(req *http.Request, res *http.Response, err error, _ time.Time, duration time.Duration) error {
	fields := []zap.Field{
		zap.String("method", req.Method),
		zap.String("url", req.URL.String()),
		zap.Duration("duration", duration),
	}
	if res != nil {
		fields = append(fields, zap.Int("status", res.StatusCode))
	}
	if err != nil {
		logger.logger.Warn("elasticsearch request failed", append(fields, zap.Error(err))...)
		return nil
	}
	logger.logger.Info("elasticsearch request", fields...)
	return nil
}
