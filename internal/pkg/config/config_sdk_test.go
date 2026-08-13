package config

import (
	"context"
	"sync"
	"testing"
	"time"

	configv1 "github.com/lens077/config-center/api/config/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInit_FromConfigCenter(t *testing.T) {
	_, addr := startFakeConfigService(t, map[string]*configv1.ConfigEntry{
		"cart/dev/bootstrap.yaml": {Value: testBootstrapYAML},
	})
	useConfigCenterSource(t, addr, "cart", "dev", "bootstrap.yaml")

	got, err := Init(context.Background())
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Equal(t, "0.0.0.0:30006", got.Server.Addr)
	assert.Equal(t, 10*time.Second, got.Server.Http.ReadTimeout.AsDuration())
	assert.Equal(t, "config_center", SourceName())
	assert.Same(t, got, GetConfig())
}

func TestInit_SourceUnreachable(t *testing.T) {
	useConfigCenterSource(t, "http://127.0.0.1:1", "cart", "pre", "bootstrap.yaml")

	got, err := Init(context.Background())
	assert.Nil(t, got)
	require.Error(t, err)
}

func TestInit_DecodeErrorMentionsSource(t *testing.T) {
	_, addr := startFakeConfigService(t, map[string]*configv1.ConfigEntry{
		"cart/pre/bootstrap.yaml": {Value: "server:\n  http:\n    read_timeout: not-a-duration\n"},
	})
	useConfigCenterSource(t, addr, "cart", "pre", "bootstrap.yaml")

	got, err := Init(context.Background())
	assert.Nil(t, got)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config_center")
}

func TestGetConfig_ConcurrentWithInit(t *testing.T) {
	_, addr := startFakeConfigService(t, map[string]*configv1.ConfigEntry{
		"cart/pre/bootstrap.yaml": {Value: testBootstrapYAML},
	})
	useConfigCenterSource(t, addr, "cart", "pre", "bootstrap.yaml")

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 50 {
				assert.NotNil(t, GetConfig())
				_ = SourceName()
			}
		})
	}

	for range 4 {
		wg.Go(func() {
			_, _ = Init(context.Background())
		})
	}

	wg.Wait()
	assert.Equal(t, "config_center", SourceName())
}
