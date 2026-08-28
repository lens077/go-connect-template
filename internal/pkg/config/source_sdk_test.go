package config

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"connectrpc.com/connect"
	configv1 "github.com/lens077/control-tower/api/config/v1"
	"github.com/lens077/control-tower/api/config/v1/configv1connect"
	"github.com/lens077/go-connect-template/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeConfigService 只实现 GetKey,其余 RPC 继承 Unimplemented。
// 起一个真实的 ConnectRPC 服务端(而非 mock 客户端),顺带覆盖了序列化与错误码映射。
type fakeConfigService struct {
	configv1connect.UnimplementedConfigServiceHandler

	// entries 以 "namespace/environment/key" 为索引,构造后只读
	entries map[string]*configv1.ConfigEntry

	// httptest 每个连接一个 goroutine,并发用例下 handler 会同时写断言字段,故加锁
	mu sync.Mutex
	// lastReq 记录最后一次请求,用于断言三元组被正确传递
	lastReq    *configv1.GetKeyRequest
	lastHeader http.Header
}

// LastReq 返回最后一次收到的 GetKey 请求
func (f *fakeConfigService) LastReq() *configv1.GetKeyRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastReq
}

func (f *fakeConfigService) LastHeader() http.Header {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastHeader.Clone()
}

func (f *fakeConfigService) GetKey(
	_ context.Context, req *connect.Request[configv1.GetKeyRequest],
) (*connect.Response[configv1.GetKeyResponse], error) {
	f.mu.Lock()
	f.lastReq = req.Msg
	f.lastHeader = req.Header().Clone()
	f.mu.Unlock()

	id := req.Msg.GetNamespace() + "/" + req.Msg.GetEnvironment() + "/" + req.Msg.GetKey()
	entry, ok := f.entries[id]
	if !ok {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("config not found: "+id))
	}
	return connect.NewResponse(&configv1.GetKeyResponse{Entry: entry}), nil
}

func startFakeConfigService(t *testing.T, entries map[string]*configv1.ConfigEntry) (*fakeConfigService, string) {
	t.Helper()

	svc := &fakeConfigService{entries: entries}
	mux := http.NewServeMux()
	mux.Handle(configv1connect.NewConfigServiceHandler(svc))

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return svc, server.URL
}

// useConfigCenterSource 把 SDK selector 指向桩服务,返回构造好的数据源。
func useConfigCenterSource(t *testing.T, addr, namespace, environment, key string) Source {
	t.Helper()
	selector := filepath.Join(t.TempDir(), "source.yaml")
	contents := []byte("type: config_center\nconfig_center:\n" +
		"  address: " + addr + "\n" +
		"  namespace: " + namespace + "\n" +
		"  environment: " + environment + "\n" +
		"  key: " + key + "\n" +
		"  service_token: test-service-token\n")
	require.NoError(t, os.WriteFile(selector, contents, 0o600))
	clearSourceEnv(t)
	t.Setenv(constants.EnvConfigSourceFile, selector)

	src, err := NewSource()
	require.NoError(t, err)
	return src
}

func TestConfigCenterSource_Load(t *testing.T) {
	svc, addr := startFakeConfigService(t, map[string]*configv1.ConfigEntry{
		"cart/dev/bootstrap.yaml": {
			Namespace:   "cart",
			Environment: "dev",
			Key:         "bootstrap.yaml",
			Format:      configv1.ConfigFormat_CONFIG_FORMAT_YAML,
			Value:       testBootstrapYAML,
			Version:     3,
		},
	})

	src := useConfigCenterSource(t, addr, "cart", "dev", "bootstrap.yaml")
	got, err := src.Load(context.Background())
	require.NoError(t, err)

	server := got["server"].(map[string]any)
	assert.Equal(t, "0.0.0.0:30006", server["addr"])

	// 三元组必须原样传给服务端,否则会静默读到别的环境的配置
	last := svc.LastReq()
	require.NotNil(t, last)
	assert.Equal(t, "cart", last.GetNamespace())
	assert.Equal(t, "dev", last.GetEnvironment())
	assert.Equal(t, "bootstrap.yaml", last.GetKey())
	assert.Equal(t, "test-service-token", svc.LastHeader().Get("x-config-center-service-token"))
}

func TestConfigCenterSource_LoadNotFound(t *testing.T) {
	_, addr := startFakeConfigService(t, map[string]*configv1.ConfigEntry{})

	src := useConfigCenterSource(t, addr, "cart", "prod", "bootstrap.yaml")
	_, err := src.Load(context.Background())
	require.Error(t, err)

	// 报错要带齐 namespace/environment/key,便于一眼看出取的是哪一份
	assert.Contains(t, err.Error(), "cart/prod/bootstrap.yaml")
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

// 配置项存在但值为空,和不存在一样是致命的:让服务带着空 Bootstrap 起来更难查
func TestConfigCenterSource_LoadEmptyValue(t *testing.T) {
	_, addr := startFakeConfigService(t, map[string]*configv1.ConfigEntry{
		"cart/dev/bootstrap.yaml": {Value: ""},
	})

	src := useConfigCenterSource(t, addr, "cart", "dev", "bootstrap.yaml")
	_, err := src.Load(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

func TestConfigCenterSource_LoadInvalidYAML(t *testing.T) {
	_, addr := startFakeConfigService(t, map[string]*configv1.ConfigEntry{
		"cart/dev/bootstrap.yaml": {Value: "server:\n\taddr: tab"},
	})

	src := useConfigCenterSource(t, addr, "cart", "dev", "bootstrap.yaml")
	_, err := src.Load(context.Background())
	require.Error(t, err)
}

func TestConfigCenterSource_LoadUnreachable(t *testing.T) {
	src := useConfigCenterSource(t, "http://127.0.0.1:1", "cart", "dev", "bootstrap.yaml")

	_, err := src.Load(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read config center key")
}

func TestConfigCenterSource_LoadRespectsContext(t *testing.T) {
	_, addr := startFakeConfigService(t, map[string]*configv1.ConfigEntry{
		"cart/dev/bootstrap.yaml": {Value: testBootstrapYAML},
	})
	src := useConfigCenterSource(t, addr, "cart", "dev", "bootstrap.yaml")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := src.Load(ctx)
	require.Error(t, err)
	assert.Equal(t, connect.CodeCanceled, connect.CodeOf(err))
}
