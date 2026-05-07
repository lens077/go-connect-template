package meta

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

// MetaTestSuite 是 Meta 的测试套件
type MetaTestSuite struct {
	suite.Suite
}

func (suite *MetaTestSuite) TestAppInfoStruct() {
	// 测试 AppInfo 结构体
	appInfo := AppInfo{
		ID:          "test-id",
		Name:        "test-service",
		Host:        "localhost",
		Environment: "dev",
	}

	assert.Equal(suite.T(), "test-id", appInfo.ID)
	assert.Equal(suite.T(), "test-service", appInfo.Name)
	assert.Equal(suite.T(), "localhost", appInfo.Host)
	assert.Equal(suite.T(), "dev", appInfo.Environment)
}

func (suite *MetaTestSuite) TestGetOutboundIP() {
	// 测试 GetOutboundIP 函数
	ip, err := GetOutboundIP()

	// 这个测试可能因为网络原因失败，所以我们接受两种情况
	if err == nil {
		assert.NotEmpty(suite.T(), ip)
		// 验证 IP 格式
		assert.NotContains(suite.T(), ip, ":") // 不应该包含端口
	} else {
		// 如果出错也接受，因为可能在无网络环境中
		suite.T().Logf("GetOutboundIP failed (expected in offline environment): %v", err)
	}
}

func (suite *MetaTestSuite) TestGetOutboundIP_PanicRecovery() {
	// 测试 panic 恢复
	assert.NotPanics(suite.T(), func() {
		_, _ = GetOutboundIP()
	})
}

// 运行测试套件
func TestMetaTestSuite(t *testing.T) {
	suite.Run(t, new(MetaTestSuite))
}

// 单元测试函数
func TestAppInfoZeroValue(t *testing.T) {
	// 测试零值
	var appInfo AppInfo
	assert.Empty(t, appInfo.ID)
	assert.Empty(t, appInfo.Name)
	assert.Empty(t, appInfo.Host)
	assert.Empty(t, appInfo.Environment)
}

func TestAppInfoInitialization(t *testing.T) {
	// 测试结构体初始化
	testCases := []struct {
		name        string
		input       AppInfo
		expectedID  string
		expectedName string
	}{
		{
			name: "Full info",
			input: AppInfo{
				ID:          "service-1",
				Name:        "user-service",
				Host:        "192.168.1.1",
				Environment: "production",
			},
			expectedID:   "service-1",
			expectedName: "user-service",
		},
		{
			name: "Partial info",
			input: AppInfo{
				ID:   "service-2",
				Name: "payment-service",
			},
			expectedID:   "service-2",
			expectedName: "payment-service",
		},
		{
			name:        "Empty info",
			input:       AppInfo{},
			expectedID:  "",
			expectedName: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expectedID, tc.input.ID)
			assert.Equal(t, tc.expectedName, tc.input.Name)
		})
	}
}
