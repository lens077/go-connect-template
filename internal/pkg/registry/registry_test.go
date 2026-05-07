package registry

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"
)

// RegistryTestSuite 是 Registry 的测试套件
type RegistryTestSuite struct {
	suite.Suite
	testLogger *zap.Logger
}

func (suite *RegistryTestSuite) SetupTest() {
	// 创建测试用的 logger
	var err error
	suite.testLogger, err = zap.NewDevelopment()
	assert.NoError(suite.T(), err)

	// 清理环境变量
	os.Clearenv()
}

func (suite *RegistryTestSuite) TestNewConsulRegistry_WithValidAddr() {
	// 测试 NewConsulRegistry 函数
	reg, err := NewConsulRegistry("localhost:8500", "test-id", "test-service", WithLogger(suite.testLogger))
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), reg)
	assert.Equal(suite.T(), "test-id", reg.ID)
	assert.Equal(suite.T(), "test-service", reg.Name)
	assert.Equal(suite.T(), "localhost:8500", reg.Addr)
}

func (suite *RegistryTestSuite) TestNewConsulRegistry_WithInvalidAddr() {
	// 测试无效地址的情况
	reg, err := NewConsulRegistry("invalid-addr", "test-id", "test-service", WithLogger(suite.testLogger))
	// 这里应该不会在创建时就出错，而是在实际使用时出错
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), reg)
}

func (suite *RegistryTestSuite) TestNewConsulRegistry_WithTLS() {
	// 测试带 TLS 配置的情况
	reg, err := NewConsulRegistry("localhost:8500", "test-id", "test-service", WithLogger(suite.testLogger), WithTLS(true, ""))
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), reg)
}

func (suite *RegistryTestSuite) TestWithLogger() {
	// 测试 WithLogger 选项
	opt := WithLogger(suite.testLogger)
	o := &options{}
	opt(o)
	assert.Equal(suite.T(), suite.testLogger, o.logger)
}

func (suite *RegistryTestSuite) TestWithTLS() {
	// 测试 WithTLS 选项
	opt := WithTLS(true, "test-ca-pem")
	o := &options{}
	opt(o)
	assert.NotNil(suite.T(), o.tlsConf)
	assert.True(suite.T(), o.tlsConf.InsecureSkipVerify)
}

func (suite *RegistryTestSuite) TestModuleCreation() {
	// 测试模块创建
	module := Module
	assert.NotNil(suite.T(), module)
	assert.Contains(suite.T(), module.String(), "registry")
}

// 运行测试套件
func TestRegistryTestSuite(t *testing.T) {
	suite.Run(t, new(RegistryTestSuite))
}

func TestNewConsulRegistry_PanicRecovery(t *testing.T) {
	// 测试 panic 恢复
	assert.NotPanics(t, func() {
		_, _ = NewConsulRegistry("localhost:8500", "test-id", "test-name")
	})
}
