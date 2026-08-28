package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lens077/go-connect-template/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// clearSourceEnv 把所有影响选源的环境变量置空。
// env.GetEnvString 把空串当作「未设置」,故置空等价于 unsetenv,而 t.Setenv 会在
// 用例结束时自动还原 —— 无需 os.Clearenv 那种会污染整个进程的做法。
func clearSourceEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		constants.EnvConfigSource, constants.EnvConfigFile, constants.EnvConfigSourceFile,
		constants.EnvConsulAddr, constants.EnvConsulScheme, constants.EnvConsulToken,
	} {
		t.Setenv(k, "")
	}
}

func TestNewSource_MissingSelectorFails(t *testing.T) {
	clearSourceEnv(t)

	src, err := NewSource()
	assert.Nil(t, src)
	require.Error(t, err)
	assert.Contains(t, err.Error(), constants.EnvConfigSourceFile)
}

func TestNewSource_ConsulIsRejected(t *testing.T) {
	clearSourceEnv(t)
	t.Setenv(constants.EnvConfigSource, "consul")

	src, err := NewSource()
	assert.Nil(t, src)
	require.Error(t, err)
	assert.Contains(t, err.Error(), constants.EnvConfigSourceFile)
}

func TestNewSource_SDKSelector(t *testing.T) {
	clearSourceEnv(t)
	selector := filepath.Join(t.TempDir(), "source.yaml")
	require.NoError(t, os.WriteFile(selector, []byte("type: config_center\nconfig_center:\n  address: http://config-center:30010\n  namespace: cart\n  environment: pre\n  key: bootstrap.yaml\n"), 0o600))
	t.Setenv(constants.EnvConfigSourceFile, selector)

	src, err := NewSource()
	require.NoError(t, err)
	assert.Equal(t, "config_center", src.Name())
}

func TestNewSource_SDKSelectorRejectsFile(t *testing.T) {
	clearSourceEnv(t)
	selector := filepath.Join(t.TempDir(), "source.yaml")
	require.NoError(t, os.WriteFile(selector, []byte("type: file\nfile:\n  path: bootstrap.yaml\n"), 0o600))
	t.Setenv(constants.EnvConfigSourceFile, selector)

	src, err := NewSource()
	assert.Nil(t, src)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must select config_center")
}

func TestNewSource_File(t *testing.T) {
	clearSourceEnv(t)
	t.Setenv(constants.EnvConfigSource, constants.ConfigSourceFile)
	t.Setenv(constants.EnvConfigFile, "configs/dev.yaml")

	src, err := NewSource()
	require.NoError(t, err)
	assert.Equal(t, constants.ConfigSourceFile, src.Name())
}

func TestNewSource_DeprecatedConfigCenterEnvFailsFast(t *testing.T) {
	clearSourceEnv(t)
	t.Setenv(constants.EnvConfigSource, constants.ConfigSourceConfigCenter)

	src, err := NewSource()
	assert.Nil(t, src)
	require.Error(t, err)
	assert.Contains(t, err.Error(), constants.EnvConfigSourceFile)
}

func TestNewSource_UnknownValue(t *testing.T) {
	clearSourceEnv(t)
	t.Setenv(constants.EnvConfigSource, "etcd")

	src, err := NewSource()
	assert.Nil(t, src)
	require.Error(t, err)
	// 拼错值时不能静默回落到默认源 —— 那会让服务读到一份你以为早已换掉的配置
	assert.Contains(t, err.Error(), "etcd")
	assert.Contains(t, err.Error(), constants.ConfigSourceFile)
	assert.Contains(t, err.Error(), constants.EnvConfigSourceFile)
}

func TestParseYAMLToMap(t *testing.T) {
	got, err := parseYAMLToMap([]byte(testBootstrapYAML))
	require.NoError(t, err)

	server, ok := got["server"].(map[string]any)
	require.True(t, ok, "server 应被解析为嵌套 map")
	assert.Equal(t, "0.0.0.0:30006", server["addr"])

	// viper 会把 key 统一小写,后续 mapstructure 按 json tag 匹配的正是小写下划线名
	data, ok := got["data"].(map[string]any)
	require.True(t, ok)
	pg := data["database"].(map[string]any)["postgres"].(map[string]any)
	assert.Equal(t, "localhost", pg["host"])
	assert.Equal(t, 5432, pg["port"])
}

func TestParseYAMLToMap_Invalid(t *testing.T) {
	_, err := parseYAMLToMap([]byte("server:\n\taddr: bad-tab-indent"))
	require.Error(t, err)
}

func TestParseYAMLToMap_Empty(t *testing.T) {
	// 空文档不是语法错误,但会解析出空 map;调用方(各 source)负责在取值时判空
	got, err := parseYAMLToMap(nil)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestFileSourceNameMatchesConfigSourceValue(t *testing.T) {
	assert.Equal(t, constants.ConfigSourceFile, (&fileSource{}).Name())
}
