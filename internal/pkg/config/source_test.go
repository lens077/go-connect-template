package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lens077/go-connect-template/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testBootstrapYAML 是配置加载测试共用的样例，含 duration 字段以覆盖解码钩子。
const testBootstrapYAML = `
server:
  addr: "0.0.0.0:30006"
  http:
    read_timeout: 10s
    write_timeout: 20s
    idle_timeout: 1m30s
data:
  database:
    postgres:
      host: localhost
      port: 5432
      user: postgres
      db_name: ecommerce
discovery:
  consul:
    addr: 127.0.0.1:8500
    scheme: http
    health_check: true
`

// clearSourceEnv 把所有影响选源的环境变量置空。
// env.GetEnvString 把空串当作「未设置」,故置空等价于 unsetenv,而 t.Setenv 会在
// 用例结束时自动还原 —— 无需 os.Clearenv 那种会污染整个进程的做法。
func clearSourceEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		constants.EnvConfigSource, constants.EnvConfigFile, constants.EnvConfigSourceFile,
	} {
		t.Setenv(k, "")
	}
}

func TestNewSource_Dispatch(t *testing.T) {
	cases := []struct {
		name     string
		env      string
		wantName string
		wantErr  bool
		errSub   string
	}{
		{name: "默认走 file 源", env: "", wantName: constants.DefaultConfigSource},
		{name: "显式 file", env: constants.ConfigSourceFile, wantName: constants.ConfigSourceFile},
		{name: "consul KV 已退役", env: "consul", wantErr: true, errSub: "consul"},
		{name: "未知取值直接报错,不静默降级", env: "etcd", wantErr: true, errSub: "etcd"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			clearSourceEnv(t)
			if c.env != "" {
				t.Setenv(constants.EnvConfigSource, c.env)
			}

			src, err := NewSource()
			if c.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), constants.EnvConfigSource)
				if c.errSub != "" {
					assert.Contains(t, err.Error(), c.errSub)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, c.wantName, src.Name())
		})
	}
}

func TestFileSource_Load(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dev.yml")
	require.NoError(t, os.WriteFile(path, []byte("server:\n  addr: \"0.0.0.0:30000\"\n"), 0o600))

	clearSourceEnv(t)
	t.Setenv(constants.EnvConfigSource, constants.ConfigSourceFile)
	t.Setenv(constants.EnvConfigFile, path)

	src, err := NewSource()
	require.NoError(t, err)

	got, err := src.Load(t.Context())
	require.NoError(t, err)

	// viper 会把 YAML 压成小写 key 的嵌套 map
	server, ok := got["server"].(map[string]any)
	require.True(t, ok, "expected server section, got %#v", got)
	assert.Equal(t, "0.0.0.0:30000", server["addr"])
}

func TestFileSource_Load_Errors(t *testing.T) {
	clearSourceEnv(t)
	t.Setenv(constants.EnvConfigSource, constants.ConfigSourceFile)

	t.Run("文件不存在时报绝对路径", func(t *testing.T) {
		t.Setenv(constants.EnvConfigFile, filepath.Join(t.TempDir(), "missing.yml"))
		src, err := NewSource()
		require.NoError(t, err)

		_, err = src.Load(t.Context())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing.yml")
	})

	t.Run("空文件视为错误而非空配置", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "empty.yml")
		require.NoError(t, os.WriteFile(path, nil, 0o600))
		t.Setenv(constants.EnvConfigFile, path)

		src, err := NewSource()
		require.NoError(t, err)

		_, err = src.Load(t.Context())
		require.ErrorContains(t, err, "empty")
	})
}

func TestParseYAMLToMap(t *testing.T) {
	got, err := parseYAMLToMap([]byte(testBootstrapYAML))
	require.NoError(t, err)

	server, ok := got["server"].(map[string]any)
	require.True(t, ok, "server 应被解析为嵌套 map")
	assert.Equal(t, "0.0.0.0:30006", server["addr"])

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
	got, err := parseYAMLToMap(nil)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestFileSourceNameMatchesConfigSourceValue(t *testing.T) {
	assert.Equal(t, constants.ConfigSourceFile, (&fileSource{}).Name())
}

func TestSourceName_EmptyBeforeInit(t *testing.T) {
	srcMu.Lock()
	activeSource = nil
	srcMu.Unlock()

	assert.Empty(t, SourceName())
}
