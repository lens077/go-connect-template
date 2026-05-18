package log

import (
	"net/http"
	"time"

	conf "github.com/lens077/go-connect-template/internal/conf/v1"
	"go.uber.org/zap"
)

type ZapESLogger struct {
	Logger *zap.Logger
	Conf   *conf.Log
}

func (z *ZapESLogger) RequestBodyEnabled() bool {
	return z.Conf.Elasticsearch.EnableRequest
}

func (z *ZapESLogger) ResponseBodyEnabled() bool {
	return z.Conf.Elasticsearch.EnableResponse
}

func (z *ZapESLogger) LogRoundTrip(req *http.Request, res *http.Response, err error, start time.Time, dur time.Duration) error {
	z.Logger.Info("elasticsearch request",
		zap.String("method", req.Method),
		zap.String("url", req.URL.String()),
		zap.Duration("duration", dur),
		zap.Int("status", res.StatusCode),
	)
	return nil
}
