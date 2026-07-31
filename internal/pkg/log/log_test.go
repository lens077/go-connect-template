package log

import (
	"testing"

	"github.com/lens077/go-connect-template/constants"
	confv1 "github.com/lens077/go-connect-template/internal/conf/v1"
	"github.com/lens077/go-connect-template/internal/pkg/meta"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// bootstrapWith 构造一份只填了 Log 段的 Bootstrap。
// NewLogger 只读 conf.Log,其余字段留空不影响测试。
func bootstrapWith(level, format string) *confv1.Bootstrap {
	return &confv1.Bootstrap{
		Log: &confv1.Log{
			Application: &confv1.Log_Application{
				Level:  level,
				Format: format,
			},
			Framework: &confv1.Log_Framework{
				LogLevel:   "debug",
				ErrorLevel: "error",
			},
		},
	}
}

// LogTestSuite 是 Log 的测试套件
type LogTestSuite struct {
	suite.Suite
	testAppInfo meta.AppInfo
}

func (suite *LogTestSuite) SetupTest() {
	// 设置测试用的应用信息
	suite.testAppInfo = meta.AppInfo{
		ID:          "test-service-id",
		Name:        "test-service",
		Host:        "localhost",
		Environment: "dev",
	}
}

func (suite *LogTestSuite) newLogger(level, format string) *zap.Logger {
	return NewLogger(bootstrapWith(level, format), suite.testAppInfo)
}

func (suite *LogTestSuite) TestNewLogger_Levels() {
	// 每个级别都应放行自身,并挡住更低一级
	cases := []struct {
		level   string
		enabled zapcore.Level
		blocked zapcore.Level
	}{
		{"debug", zapcore.DebugLevel, 0},
		{"info", zapcore.InfoLevel, zapcore.DebugLevel},
		{"warn", zapcore.WarnLevel, zapcore.InfoLevel},
		{"error", zapcore.ErrorLevel, zapcore.WarnLevel},
	}

	for _, c := range cases {
		suite.Run(c.level, func() {
			logger := suite.newLogger(c.level, constants.FormatJson)
			assert.NotNil(suite.T(), logger)
			assert.True(suite.T(), logger.Core().Enabled(c.enabled))
			if c.level != "debug" {
				assert.False(suite.T(), logger.Core().Enabled(c.blocked))
			}
		})
	}
}

func (suite *LogTestSuite) TestNewLogger_UnparsableLevelFallsBackToDebug() {
	// 级别解析失败时 NewLogger 退到 debug:宁可多打,也不要因为配置写错而丢日志
	logger := suite.newLogger("invalid-level", constants.FormatJson)
	assert.NotNil(suite.T(), logger)
	assert.True(suite.T(), logger.Core().Enabled(zapcore.DebugLevel))
}

func (suite *LogTestSuite) TestNewLogger_EmptyLevelIsInfo() {
	// 空级别不算解析失败:zapcore 把 "" 映射为 info,不会走到 debug 兜底
	logger := suite.newLogger("", constants.FormatJson)
	assert.NotNil(suite.T(), logger)
	assert.True(suite.T(), logger.Core().Enabled(zapcore.InfoLevel))
	assert.False(suite.T(), logger.Core().Enabled(zapcore.DebugLevel))
}

func (suite *LogTestSuite) TestNewLogger_Formats() {
	// console 走带颜色的开发编码器,其余一律按 json 处理
	for _, format := range []string{constants.FormatConsole, constants.FormatJson, "invalid-format"} {
		logger := suite.newLogger("info", format)
		assert.NotNil(suite.T(), logger, "format=%q", format)
	}
}

func (suite *LogTestSuite) TestModuleCreation() {
	// 测试模块创建
	module := Module
	assert.NotNil(suite.T(), module)
	assert.Contains(suite.T(), module.String(), "log")
}

func (suite *LogTestSuite) TestLoggerInterface() {
	// 测试日志接口实现
	logger := suite.newLogger("info", constants.FormatJson)
	assert.NotNil(suite.T(), logger)

	// 测试各种日志级别
	assert.NotPanics(suite.T(), func() {
		logger.Debug("debug message")
		logger.Info("info message")
		logger.Warn("warn message")
		logger.Error("error message")
	})
}

func (suite *LogTestSuite) TestLoggerWithFields() {
	// 测试带字段的日志
	logger := suite.newLogger("info", constants.FormatJson)
	assert.NotNil(suite.T(), logger)

	assert.NotPanics(suite.T(), func() {
		logger.With(
			zap.String("key", "value"),
			zap.Int("number", 42),
		).Info("message with fields")
	})
}

func (suite *LogTestSuite) TestLoggerSugar() {
	// 测试 Sugar 日志
	logger := suite.newLogger("info", constants.FormatJson)
	assert.NotNil(suite.T(), logger)
	sugar := logger.Sugar()

	assert.NotPanics(suite.T(), func() {
		sugar.Debugw("debug message", "key", "value")
		sugar.Infow("info message", "key", "value")
		sugar.Warnw("warn message", "key", "value")
		sugar.Errorw("error message", "key", "value")
	})
}

// 运行测试套件
func TestLogTestSuite(t *testing.T) {
	suite.Run(t, new(LogTestSuite))
}

// 单元测试函数
func TestNewLogger_PanicRecovery(t *testing.T) {
	// 测试日志创建时的 panic 恢复
	assert.NotPanics(t, func() {
		testAppInfo := meta.AppInfo{
			ID:   "test-id",
			Name: "test-name",
		}
		_ = NewLogger(bootstrapWith("info", constants.FormatJson), testAppInfo)
	})
}
