package log

import (
	"os"

	"github.com/lens077/go-connect-template/constants"
	confv1 "github.com/lens077/go-connect-template/internal/conf/v1"
	"github.com/lens077/go-connect-template/internal/pkg/meta"
	"go.opentelemetry.io/contrib/bridges/otelzap"
	"go.opentelemetry.io/otel/log/global"
	"go.uber.org/zap/zapcore"

	"go.uber.org/fx"
	"go.uber.org/zap"
)

// Module 提供 Fx 模块
var Module = fx.Module("log",
	fx.Provide(
		// 提供日志创建函数
		func(conf *confv1.Bootstrap, info meta.AppInfo) *zap.Logger {
			return NewLogger(conf.Log.Level, conf.Log.Format, info)
		},
	),
)

// NewLogger 创建一个新的 Zap Logger.
// levelStr 可选的参数: debug / info / warn / error / dpanic / panic / fatal.
// format 可选的参数: 参考constants/env.go的Log注释部分.
func NewLogger(levelStr string, format string, info meta.AppInfo) *zap.Logger {
	var level zapcore.Level
	if err := level.UnmarshalText([]byte(levelStr)); err != nil {
		level = zapcore.InfoLevel
	}

	// 定义基础的 Encoder (编码器)
	var encoder zapcore.Encoder
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	if format == constants.FormatConsole {
		encoderConfig = zap.NewDevelopmentEncoderConfig()
		encoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	} else {
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	}

	// 创建标准输出 Core (Stdout)
	stdCore := zapcore.NewCore(encoder, zapcore.AddSync(os.Stdout), level)

	// 创建 OTel Core (发送到 OTLP)
	// 这里使用 global.GetLoggerProvider()
	otelCore := otelzap.NewCore(
		info.Name, // 你的 Instrumentation Name
		otelzap.WithLoggerProvider(global.GetLoggerProvider()),
	)

	// 4. 使用 Tee 组合两个 Core
	// 这样 logger.Info 就会同时发往：
	// 1. 控制台/JSON文件
	// 2. OTel Collector
	core := zapcore.NewTee(stdCore, otelCore)

	return zap.New(core, zap.AddCaller())
}
