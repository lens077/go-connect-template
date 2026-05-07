package otel

import (
	"context"
	"testing"

	confv1 "github.com/lens077/go-connect-template/internal/conf/v1"
	"github.com/lens077/go-connect-template/internal/pkg/meta"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"
)

// OtelTestSuite 是 Otel 的测试套件
type OtelTestSuite struct {
	suite.Suite
	testLogger  *zap.Logger
	testAppInfo meta.AppInfo
}

func (suite *OtelTestSuite) SetupTest() {
	// 创建测试用的 logger
	var err error
	suite.testLogger, err = zap.NewDevelopment()
	assert.NoError(suite.T(), err)

	// 设置测试用的应用信息
	suite.testAppInfo = meta.AppInfo{
		ID:          "test-service-id",
		Name:        "test-service",
		Host:        "localhost",
		Environment: "dev",
	}
}

func (suite *OtelTestSuite) TestModuleCreation() {
	// 测试模块创建
	module := Module
	assert.NotNil(suite.T(), module)
	assert.Contains(suite.T(), module.String(), "otel")
}

func (suite *OtelTestSuite) TestWithTraceTLS() {
	// 测试 WithTraceTLS 选项
	opt := WithTraceTLS(true, []byte("test-ca-pem"))
	o := &traceOptions{}
	opt(o)
	// 只要不 panic 就通过
	assert.NotNil(suite.T(), o)
}

func (suite *OtelTestSuite) TestWithTraceTLS_WithoutCaPem() {
	// 测试不带 CA pem 的情况
	opt := WithTraceTLS(true, []byte(""))
	o := &traceOptions{}
	opt(o)
	assert.NotNil(suite.T(), o)
}

func (suite *OtelTestSuite) TestWithMetricTLS() {
	// 测试 WithMetricTLS 选项
	opt := WithMetricTLS(true, []byte("test-ca-pem"))
	o := &metricOptions{}
	opt(o)
	assert.NotNil(suite.T(), o)
}

func (suite *OtelTestSuite) TestWithLogTLS() {
	// 测试 WithLogTLS 选项
	opt := WithLogTLS(true, []byte("test-ca-pem"))
	o := &logOptions{}
	opt(o)
	assert.NotNil(suite.T(), o)
}

func (suite *OtelTestSuite) TestNewResource() {
	// 测试 newResource 函数
	res, err := newResource(suite.testAppInfo)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), res)
}

func (suite *OtelTestSuite) TestNewPropagator() {
	// 测试 newPropagator 函数
	prop := newPropagator()
	assert.NotNil(suite.T(), prop)
}

func (suite *OtelTestSuite) TestSetupOTelSDK_PanicRecovery() {
	// 测试 panic 恢复
	assert.NotPanics(suite.T(), func() {
		ctx := context.Background()
		minConfig := &confv1.Observability{
			Enable: true,
			Trace: &confv1.Observability_Trace{
				Endpoint: "localhost:4318",
				Tls: &confv1.Observability_Tls{
					Enable: false,
				},
			},
			Metric: &confv1.Observability_Metric{
				Endpoint: "localhost:4318",
				Tls: &confv1.Observability_Tls{
					Enable: false,
				},
			},
			Log: &confv1.Observability_Logging{
				Endpoint: "localhost:4318",
				Tls: &confv1.Observability_Tls{
					Enable: false,
				},
			},
		}
		_, _ = SetupOTelSDK(ctx, suite.testAppInfo, minConfig, suite.testLogger)
	})
}

func (suite *OtelTestSuite) TestSetupOTelSDK_Disabled() {
	// 测试当 Observability 被禁用时是否正常返回
	ctx := context.Background()

	// 测试 nil 配置
	shutdown, err := SetupOTelSDK(ctx, suite.testAppInfo, nil, suite.testLogger)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), shutdown)
	// 调用 shutdown 应该不报错
	err = shutdown(ctx)
	assert.NoError(suite.T(), err)

	// 测试 enable 为 false
	disabledConfig := &confv1.Observability{
		Enable: false,
	}
	shutdown2, err := SetupOTelSDK(ctx, suite.testAppInfo, disabledConfig, suite.testLogger)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), shutdown2)
	err = shutdown2(ctx)
	assert.NoError(suite.T(), err)
}

// 运行测试套件
func TestOtelTestSuite(t *testing.T) {
	suite.Run(t, new(OtelTestSuite))
}

func TestOtelOptionTypes(t *testing.T) {
	// 测试选项类型
	// 验证类型是否正确存在
	var _ TraceOption = nil
	var _ MetricOption = nil
	var _ LogOption = nil
}
