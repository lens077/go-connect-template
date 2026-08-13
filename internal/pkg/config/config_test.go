package config

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/lens077/go-connect-template/constants"
	confv1 "github.com/lens077/go-connect-template/internal/conf/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeConfig(t *testing.T) {
	raw, err := parseYAMLToMap([]byte(testBootstrapYAML))
	require.NoError(t, err)

	got := &confv1.Bootstrap{}
	require.NoError(t, decodeConfig(raw, got))

	require.NotNil(t, got.Server)
	assert.Equal(t, "0.0.0.0:30006", got.Server.Addr)

	require.NotNil(t, got.Data)
	require.NotNil(t, got.Data.Database)
	require.NotNil(t, got.Data.Database.Postgres)
	assert.Equal(t, "localhost", got.Data.Database.Postgres.Host)
	assert.Equal(t, uint32(5432), got.Data.Database.Postgres.Port)
	assert.Equal(t, "ecommerce", got.Data.Database.Postgres.DbName)

	require.NotNil(t, got.Discovery)
	assert.Equal(t, "127.0.0.1:8500", got.Discovery.Consul.Addr)
	assert.True(t, got.Discovery.Consul.HealthCheck)
}

func TestDecodeConfig_DurationHook(t *testing.T) {
	raw, err := parseYAMLToMap([]byte(testBootstrapYAML))
	require.NoError(t, err)

	got := &confv1.Bootstrap{}
	require.NoError(t, decodeConfig(raw, got))

	require.NotNil(t, got.Server.Http)
	assert.Equal(t, 10*time.Second, got.Server.Http.ReadTimeout.AsDuration())
	assert.Equal(t, 20*time.Second, got.Server.Http.WriteTimeout.AsDuration())
	assert.Equal(t, 90*time.Second, got.Server.Http.IdleTimeout.AsDuration())
}

func TestDecodeConfig_InvalidDuration(t *testing.T) {
	raw, err := parseYAMLToMap([]byte("server:\n  http:\n    read_timeout: 10 seconds\n"))
	require.NoError(t, err)

	err = decodeConfig(raw, &confv1.Bootstrap{})
	require.Error(t, err)
}

func TestDecodeConfig_IgnoresUnknownFields(t *testing.T) {
	raw, err := parseYAMLToMap([]byte("server:\n  addr: \":1\"\nnot_a_real_section:\n  foo: bar\n"))
	require.NoError(t, err)

	got := &confv1.Bootstrap{}
	require.NoError(t, decodeConfig(raw, got))
	assert.Equal(t, ":1", got.Server.Addr)
}

func TestInit_FromFile(t *testing.T) {
	path := t.TempDir() + "/bootstrap.yaml"
	require.NoError(t, os.WriteFile(path, []byte(testBootstrapYAML), 0o600))
	clearSourceEnv(t)
	t.Setenv(constants.EnvConfigSource, constants.ConfigSourceFile)
	t.Setenv(constants.EnvConfigFile, path)

	got, err := Init(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "0.0.0.0:30006", got.Server.Addr)
	assert.Equal(t, constants.ConfigSourceFile, SourceName())
}

func TestInit_UnknownSourceFailsFast(t *testing.T) {
	clearSourceEnv(t)
	t.Setenv(constants.EnvConfigSource, "nacos")

	got, err := Init(context.Background())
	assert.Nil(t, got)
	require.Error(t, err)
}

func TestModule(t *testing.T) {
	require.NotNil(t, Module)
	assert.Contains(t, Module.String(), "config")
}
