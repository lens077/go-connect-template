package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/lens077/go-connect-template/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSource_Dispatch(t *testing.T) {
	cases := []struct {
		name     string
		env      string
		wantName string
		wantErr  bool
	}{
		{name: "默认走 file 源", env: "", wantName: constants.DefaultConfigSource},
		{name: "显式 file", env: constants.ConfigSourceFile, wantName: constants.ConfigSourceFile},
		{name: "显式 consul", env: constants.ConfigSourceConsul, wantName: constants.ConfigSourceConsul},
		{name: "未知取值直接报错,不静默降级", env: "etcd", wantErr: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.env == "" {
				t.Setenv(constants.EnvConfigSource, "")
			} else {
				t.Setenv(constants.EnvConfigSource, c.env)
			}

			src, err := NewSource()
			if c.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), constants.EnvConfigSource)
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

	t.Setenv(constants.EnvConfigSource, constants.ConfigSourceFile)
	t.Setenv(constants.EnvConfigFile, path)

	src, err := NewSource()
	require.NoError(t, err)

	got, err := src.Load(context.Background())
	require.NoError(t, err)

	// viper 会把 YAML 压成小写 key 的嵌套 map
	server, ok := got["server"].(map[string]any)
	require.True(t, ok, "expected server section, got %#v", got)
	assert.Equal(t, "0.0.0.0:30000", server["addr"])
}

func TestFileSource_Load_Errors(t *testing.T) {
	t.Setenv(constants.EnvConfigSource, constants.ConfigSourceFile)

	t.Run("文件不存在时报绝对路径", func(t *testing.T) {
		t.Setenv(constants.EnvConfigFile, filepath.Join(t.TempDir(), "missing.yml"))
		src, err := NewSource()
		require.NoError(t, err)

		_, err = src.Load(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing.yml")
	})

	t.Run("空文件视为错误而非空配置", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "empty.yml")
		require.NoError(t, os.WriteFile(path, nil, 0o600))
		t.Setenv(constants.EnvConfigFile, path)

		src, err := NewSource()
		require.NoError(t, err)

		_, err = src.Load(context.Background())
		require.ErrorContains(t, err, "empty")
	})
}

func TestSourceName_EmptyBeforeInit(t *testing.T) {
	// Init 之前 SourceName 必须是空串,而不是某个看似合理的默认值
	confMu.Lock()
	srcName = ""
	confMu.Unlock()

	assert.Empty(t, SourceName())
}
