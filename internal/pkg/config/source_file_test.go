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

func TestFileSource_Load(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bootstrap.yaml")
	require.NoError(t, os.WriteFile(path, []byte(testBootstrapYAML), 0o600))
	clearSourceEnv(t)
	t.Setenv(constants.EnvConfigSource, constants.ConfigSourceFile)
	t.Setenv(constants.EnvConfigFile, path)

	src, err := NewSource()
	require.NoError(t, err)
	assert.Equal(t, constants.ConfigSourceFile, src.Name())

	config, err := src.Load(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "0.0.0.0:30006", config["server"].(map[string]any)["addr"])
}

func TestFileSource_RequiresPath(t *testing.T) {
	clearSourceEnv(t)
	t.Setenv(constants.EnvConfigSource, constants.ConfigSourceFile)

	_, err := NewSource()
	require.Error(t, err)
	assert.Contains(t, err.Error(), constants.EnvConfigFile)
}
