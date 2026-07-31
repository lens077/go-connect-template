package log

import (
	"net/http"
	"time"

	conf "github.com/lens077/go-connect-template/internal/conf/v1"
	"go.uber.org/zap"
)

// ZapESLogger 把 go-elasticsearch 的请求日志接到 zap 上。
// 它实现的是 elastic-transport 的 Logger 接口,由
// elasticsearch.WithLogger(...) 装配进客户端。
type ZapESLogger struct {
	Logger *zap.Logger
	Conf   *conf.Log
}

// Conf 可能没配 log.elasticsearch 段(整段是 optional),这时按「不记录」处理,
// 而不是在每次请求的路径上 panic。
func (z *ZapESLogger) RequestBodyEnabled() bool {
	return z.Conf.GetElasticsearch().GetEnableRequest()
}

func (z *ZapESLogger) ResponseBodyEnabled() bool {
	return z.Conf.GetElasticsearch().GetEnableResponse()
}

func (z *ZapESLogger) LogRoundTrip(req *http.Request, res *http.Response, err error, start time.Time, dur time.Duration) error {
	fields := []zap.Field{
		zap.String("method", req.Method),
		zap.String("url", req.URL.String()),
		zap.Duration("duration", dur),
	}
	// 连接失败时 res 是 nil,err 非 nil。原来直接取 res.StatusCode ——
	// 也就是说 ES 一挂,日志钩子自己先 panic,真正的错误反而丢了。
	if res != nil {
		fields = append(fields, zap.Int("status", res.StatusCode))
	}
	if err != nil {
		fields = append(fields, zap.Error(err))
		z.Logger.Warn("elasticsearch request failed", fields...)
		return nil
	}
	z.Logger.Info("elasticsearch request", fields...)
	return nil
}
