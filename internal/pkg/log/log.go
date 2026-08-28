package log

import (
	"os"

	"github.com/lens077/go-connect-template/constants"
	confv1 "github.com/lens077/go-connect-template/internal/conf/v1"
	"github.com/lens077/go-connect-template/internal/pkg/config"
	"github.com/lens077/go-connect-template/internal/pkg/meta"
	"go.opentelemetry.io/contrib/bridges/otelzap"
	"go.opentelemetry.io/otel/log/global"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap/zapcore"

	"go.uber.org/fx"
	"go.uber.org/zap"
)

var Module = fx.Module("log",
	fx.Provide(
		func(conf *confv1.Bootstrap, info meta.AppInfo, live *config.Live) *zap.Logger {
			logger, level := newLogger(conf, info)

			// 日志级别热生效:线上出问题时把 level 调成 debug 看细节,
			// 是最常见也最不该需要重启的一类配置改动。
			live.Subscribe(func(_, cur *confv1.Bootstrap) {
				want := parseLevel(cur.GetLog().GetApplication().GetLevel())
				if want == level.Level() {
					return
				}
				level.SetLevel(want)
				logger.Info("log level changed", zap.String("level", want.String()))
			})

			return logger
		},
	),
)

func FxLogger() fx.Option {
	return fx.WithLogger(func(log *zap.Logger, conf *confv1.Bootstrap) fxevent.Logger {
		zlog := &fxevent.ZapLogger{Logger: log}

		var fxLogLevel zapcore.Level
		if err := fxLogLevel.UnmarshalText([]byte(conf.Log.Framework.LogLevel)); err != nil {
			fxLogLevel = zapcore.DebugLevel
		}
		var fxErrLevel zapcore.Level
		if err := fxErrLevel.UnmarshalText([]byte(conf.Log.Framework.ErrorLevel)); err != nil {
			fxErrLevel = zapcore.ErrorLevel
		}

		zlog.UseLogLevel(fxLogLevel)
		zlog.UseErrorLevel(fxErrLevel)

		return zlog
	})
}

// NewLogger 构造应用 logger。级别在启动后固定;需要热调级别的走 Module。
func NewLogger(conf *confv1.Bootstrap, info meta.AppInfo) *zap.Logger {
	logger, _ := newLogger(conf, info)
	return logger
}

// parseLevel 解析日志级别,无法识别时退回 debug(与原有行为一致:
// 宁可日志多一点,也不要因为写错一个字符而丢掉排查现场)。
func parseLevel(s string) zapcore.Level {
	var level zapcore.Level
	if err := level.UnmarshalText([]byte(s)); err != nil {
		return zapcore.DebugLevel
	}
	return level
}

// newLogger 额外返回可动态调整的级别开关,供配置热更新使用。
func newLogger(conf *confv1.Bootstrap, info meta.AppInfo) (*zap.Logger, zap.AtomicLevel) {
	logConfig := conf.Log.Application
	// AtomicLevel 而不是固定的 Level:core 一旦建好就无法替换级别,
	// 只有把这个开关留在外面,后续才改得动。
	level := zap.NewAtomicLevelAt(parseLevel(logConfig.Level))

	var encoder zapcore.Encoder
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	if logConfig.Format == constants.FormatConsole {
		encoderConfig = zap.NewDevelopmentEncoderConfig()
		encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	} else {
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	}

	stdCore := zapcore.NewCore(encoder, zapcore.AddSync(os.Stdout), level)

	otelCore := otelzap.NewCore(
		info.Name,
		otelzap.WithLoggerProvider(global.GetLoggerProvider()),
	)

	core := zapcore.NewTee(stdCore, otelCore)

	return zap.New(core, zap.AddCaller()), level
}
