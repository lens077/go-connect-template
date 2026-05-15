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

// RequestBodyEnabled 是否需要日志中看到发送给 ES 的 JSON 内容
func (z *ZapESLogger) RequestBodyEnabled() bool {
	return z.Conf.EsLog.EnableRequestLog
}

// ResponseBodyEnabled 是否需要日志中看到 ES 返回的完整结果
func (z *ZapESLogger) ResponseBodyEnabled() bool {
	return z.Conf.EsLog.EnableResponseLog
}

func (z *ZapESLogger) LogRoundTrip(req *http.Request, res *http.Response, err error, start time.Time, dur time.Duration) error {
	z.Logger.Info("Elasticsearch Request",
		zap.String("method", req.Method),
		zap.String("url", req.URL.String()),
		zap.Duration("duration", dur),
		zap.Int("status", res.StatusCode),
	)
	return nil
}
