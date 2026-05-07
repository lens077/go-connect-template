package log

import (
	"testing"

	"github.com/lens077/go-connect-template/constants"
	"github.com/lens077/go-connect-template/internal/pkg/meta"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

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

func (suite *LogTestSuite) TestNewLogger_DebugLevel() {
	// 测试 debug 日志级别
	logger := NewLogger("debug", constants.FormatJson, suite.testAppInfo)
	assert.NotNil(suite.T(), logger)

	// 验证日志级别
	assert.True(suite.T(), logger.Core().Enabled(zapcore.DebugLevel))
}

func (suite *LogTestSuite) TestNewLogger_InfoLevel() {
	// 测试 info 日志级别
	logger := NewLogger("info", constants.FormatJson, suite.testAppInfo)
	assert.NotNil(suite.T(), logger)

	assert.True(suite.T(), logger.Core().Enabled(zapcore.InfoLevel))
	assert.False(suite.T(), logger.Core().Enabled(zapcore.DebugLevel))
}

func (suite *LogTestSuite) TestNewLogger_WarnLevel() {
	// 测试 warn 日志级别
	logger := NewLogger("warn", constants.FormatJson, suite.testAppInfo)
	assert.NotNil(suite.T(), logger)

	assert.True(suite.T(), logger.Core().Enabled(zapcore.WarnLevel))
	assert.False(suite.T(), logger.Core().Enabled(zapcore.InfoLevel))
}

func (suite *LogTestSuite) TestNewLogger_ErrorLevel() {
	// 测试 error 日志级别
	logger := NewLogger("error", constants.FormatJson, suite.testAppInfo)
	assert.NotNil(suite.T(), logger)

	assert.True(suite.T(), logger.Core().Enabled(zapcore.ErrorLevel))
	assert.False(suite.T(), logger.Core().Enabled(zapcore.WarnLevel))
}

func (suite *LogTestSuite) TestNewLogger_InvalidLevel() {
	// 测试无效日志级别
	logger := NewLogger("invalid-level", constants.FormatJson, suite.testAppInfo)
	assert.NotNil(suite.T(), logger)

	// 无效级别应该默认使用 info 级别
	assert.True(suite.T(), logger.Core().Enabled(zapcore.InfoLevel))
}

func (suite *LogTestSuite) TestNewLogger_EmptyLevel() {
	// 测试空日志级别
	logger := NewLogger("", constants.FormatJson, suite.testAppInfo)
	assert.NotNil(suite.T(), logger)

	assert.True(suite.T(), logger.Core().Enabled(zapcore.InfoLevel))
}

func (suite *LogTestSuite) TestNewLogger_ConsoleFormat() {
	// 测试 console 日志格式
	logger := NewLogger("info", constants.FormatConsole, suite.testAppInfo)
	assert.NotNil(suite.T(), logger)
}

func (suite *LogTestSuite) TestNewLogger_JsonFormat() {
	// 测试 json 日志格式
	logger := NewLogger("info", constants.FormatJson, suite.testAppInfo)
	assert.NotNil(suite.T(), logger)
}

func (suite *LogTestSuite) TestNewLogger_InvalidFormat() {
	// 测试无效日志格式
	logger := NewLogger("info", "invalid-format", suite.testAppInfo)
	assert.NotNil(suite.T(), logger)
	// 无效格式应该默认使用 json 格式
}

func (suite *LogTestSuite) TestModuleCreation() {
	// 测试模块创建
	module := Module
	assert.NotNil(suite.T(), module)
	assert.Contains(suite.T(), module.String(), "log")
}

func (suite *LogTestSuite) TestLoggerInterface() {
	// 测试日志接口实现
	logger := NewLogger("info", constants.FormatJson, suite.testAppInfo)
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
	logger := NewLogger("info", constants.FormatJson, suite.testAppInfo)
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
	logger := NewLogger("info", constants.FormatJson, suite.testAppInfo)
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
		_ = NewLogger("info", constants.FormatJson, testAppInfo)
	})
}
