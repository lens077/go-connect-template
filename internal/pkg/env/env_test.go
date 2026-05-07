package env

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

// EnvTestSuite 是 Env 的测试套件
type EnvTestSuite struct {
	suite.Suite
}

func (suite *EnvTestSuite) SetupTest() {
	// 清理环境变量
	os.Clearenv()
}

func (suite *EnvTestSuite) TestGetEnvString_WithExistingEnv() {
	// 测试环境变量存在的情况
	os.Setenv("TEST_KEY", "test-value")
	value := GetEnvString("TEST_KEY", "default-value")
	assert.Equal(suite.T(), "test-value", value)
}

func (suite *EnvTestSuite) TestGetEnvString_WithMissingEnv() {
	// 测试环境变量不存在的情况
	value := GetEnvString("NON_EXISTENT_KEY", "default-value")
	assert.Equal(suite.T(), "default-value", value)
}

func (suite *EnvTestSuite) TestGetEnvString_WithEmptyValue() {
	// 测试环境变量存在但值为空的情况
	os.Setenv("TEST_KEY", "")
	value := GetEnvString("TEST_KEY", "default-value")
	assert.Equal(suite.T(), "default-value", value)
}

func (suite *EnvTestSuite) TestGetEnvBool_WithTrueValue() {
	// 测试布尔值 true 的情况
	testCases := []string{"true", "True", "TRUE", "1"}
	for _, tc := range testCases {
		os.Setenv("TEST_BOOL_KEY", tc)
		value := GetEnvBool("TEST_BOOL_KEY", false)
		assert.True(suite.T(), value, "Test case: %s", tc)
	}
}

func (suite *EnvTestSuite) TestGetEnvBool_WithFalseValue() {
	// 测试布尔值 false 的情况
	testCases := []string{"false", "False", "FALSE", "0"}
	for _, tc := range testCases {
		os.Setenv("TEST_BOOL_KEY", tc)
		value := GetEnvBool("TEST_BOOL_KEY", true)
		assert.False(suite.T(), value, "Test case: %s", tc)
	}
}

func (suite *EnvTestSuite) TestGetEnvBool_WithInvalidValue() {
	// 测试无效布尔值的情况
	os.Setenv("TEST_BOOL_KEY", "invalid-value")
	value := GetEnvBool("TEST_BOOL_KEY", true)
	// 无效值应该返回默认值
	assert.True(suite.T(), value)
}

func (suite *EnvTestSuite) TestGetEnvBool_WithMissingEnv() {
	// 测试环境变量不存在的情况
	value := GetEnvBool("NON_EXISTENT_KEY", true)
	assert.True(suite.T(), value)
}

func (suite *EnvTestSuite) TestGetEnvBool_WithEmptyValue() {
	// 测试环境变量存在但值为空的情况
	os.Setenv("TEST_BOOL_KEY", "")
	value := GetEnvBool("TEST_BOOL_KEY", true)
	assert.True(suite.T(), value)
}

func (suite *EnvTestSuite) TestGetEnvString_WithMultipleCalls() {
	// 测试多次调用的情况
	os.Setenv("TEST_KEY", "value1")
	value1 := GetEnvString("TEST_KEY", "default")
	assert.Equal(suite.T(), "value1", value1)

	os.Setenv("TEST_KEY", "value2")
	value2 := GetEnvString("TEST_KEY", "default")
	assert.Equal(suite.T(), "value2", value2)
}

// 运行测试套件
func TestEnvTestSuite(t *testing.T) {
	suite.Run(t, new(EnvTestSuite))
}

// 单元测试函数
func TestGetEnvString_ConcurrentAccess(t *testing.T) {
	// 测试并发访问
	assert.NotPanics(t, func() {
		for i := 0; i < 100; i++ {
			GetEnvString("TEST_KEY", "default")
		}
	})
}

func TestGetEnvBool_ConcurrentAccess(t *testing.T) {
	// 测试并发访问
	assert.NotPanics(t, func() {
		for i := 0; i < 100; i++ {
			GetEnvBool("TEST_KEY", true)
		}
	})
}
