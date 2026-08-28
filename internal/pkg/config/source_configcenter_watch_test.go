package config

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	configv1 "github.com/lens077/control-tower/api/config/v1"
	"github.com/lens077/control-tower/api/config/v1/configv1connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeWatchService 起一个真实的 ConnectRPC 服务端流,由测试逐条推事件。
// 用真服务端(而不是 mock 客户端)才能覆盖建流、分帧、断流这些容易出错的地方。
type fakeWatchService struct {
	configv1connect.UnimplementedConfigServiceHandler

	// streams 每建一条流投递一次,测试据此断言重连行为
	streams chan chan *configv1.WatchKeysResponse

	mu      sync.Mutex
	lastReq *configv1.WatchKeysRequest
}

func (f *fakeWatchService) LastReq() *configv1.WatchKeysRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastReq
}

func (f *fakeWatchService) WatchKeys(
	ctx context.Context,
	req *connect.Request[configv1.WatchKeysRequest],
	stream *connect.ServerStream[configv1.WatchKeysResponse],
) error {
	f.mu.Lock()
	f.lastReq = req.Msg
	f.mu.Unlock()

	// 把这条流的「投递口」交给测试;关闭它 = 服务端正常结束这条流
	out := make(chan *configv1.WatchKeysResponse)
	select {
	case f.streams <- out:
	case <-ctx.Done():
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-out:
			if !ok {
				return nil
			}
			if err := stream.Send(msg); err != nil {
				return err
			}
		}
	}
}

func startFakeWatchService(t *testing.T) (*fakeWatchService, string) {
	t.Helper()

	svc := &fakeWatchService{streams: make(chan chan *configv1.WatchKeysResponse, 4)}
	mux := http.NewServeMux()
	mux.Handle(configv1connect.NewConfigServiceHandler(svc))

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return svc, server.URL
}

func (f *fakeWatchService) nextStream(t *testing.T, within time.Duration) chan *configv1.WatchKeysResponse {
	t.Helper()
	select {
	case out := <-f.streams:
		return out
	case <-time.After(within):
		t.Fatal("客户端没有建立 watch 流")
		return nil
	}
}

// runWatch 在后台跑 Watch,返回收到的事件通道;测试结束时取消。
func runWatch(t *testing.T, src Source) <-chan WatchEvent {
	t.Helper()

	w, ok := src.(Watcher)
	require.True(t, ok, "配置中心数据源必须支持变更推送")

	ctx, cancel := context.WithCancel(context.Background())
	events := make(chan WatchEvent, 16)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = w.Watch(ctx, func(ev WatchEvent) { events <- ev })
	}()

	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("Watch 在 ctx 取消后没有退出")
		}
	})
	return events
}

func nextEvent(t *testing.T, events <-chan WatchEvent, within time.Duration) WatchEvent {
	t.Helper()
	select {
	case ev := <-events:
		return ev
	case <-time.After(within):
		t.Fatal("没有等到配置推送事件")
		return WatchEvent{}
	}
}

func TestConfigCenterSource_WatchDeliversSnapshotAndPut(t *testing.T) {
	svc, addr := startFakeWatchService(t)
	src := useConfigCenterSource(t, addr, "cart", "dev", "bootstrap.yaml")

	events := runWatch(t, src)
	out := svc.nextStream(t, 2*time.Second)

	// 只订阅自己那一个 key:别人的配置与本进程无关
	last := svc.LastReq()
	require.NotNil(t, last)
	assert.Equal(t, "cart", last.GetNamespace())
	assert.Equal(t, "dev", last.GetEnvironment())
	assert.Equal(t, []string{"bootstrap.yaml"}, last.GetKeys())

	out <- &configv1.WatchKeysResponse{
		Type:  configv1.WatchEventType_WATCH_EVENT_TYPE_SNAPSHOT,
		Entry: &configv1.ConfigEntry{Value: testBootstrapYAML},
	}
	ev := nextEvent(t, events, 2*time.Second)
	require.NoError(t, ev.Err)
	require.NotNil(t, ev.Raw)
	assert.Equal(t, "0.0.0.0:30006", ev.Raw["server"].(map[string]any)["addr"])

	// 心跳不该产生任何事件,否则会被当成一次配置变更白白重建连接池
	out <- &configv1.WatchKeysResponse{Type: configv1.WatchEventType_WATCH_EVENT_TYPE_HEARTBEAT}

	out <- &configv1.WatchKeysResponse{
		Type: configv1.WatchEventType_WATCH_EVENT_TYPE_PUT,
		Entry: &configv1.ConfigEntry{
			Value: "server:\n  addr: \"0.0.0.0:40006\"\n",
		},
	}
	ev = nextEvent(t, events, 2*time.Second)
	require.NoError(t, ev.Err)
	require.NotNil(t, ev.Raw)
	assert.Equal(t, "0.0.0.0:40006", ev.Raw["server"].(map[string]any)["addr"])
}

func TestConfigCenterSource_WatchReportsDelete(t *testing.T) {
	svc, addr := startFakeWatchService(t)
	src := useConfigCenterSource(t, addr, "cart", "dev", "bootstrap.yaml")

	events := runWatch(t, src)
	out := svc.nextStream(t, 2*time.Second)

	out <- &configv1.WatchKeysResponse{
		Type:  configv1.WatchEventType_WATCH_EVENT_TYPE_DELETE,
		Entry: &configv1.ConfigEntry{Namespace: "cart", Environment: "dev", Key: "bootstrap.yaml"},
	}

	ev := nextEvent(t, events, 2*time.Second)
	assert.True(t, ev.Deleted)
	assert.Nil(t, ev.Raw)
}

// 别人写坏一次配置,不该让本服务从此收不到后续的修正:坏内容单条报错,流继续。
func TestConfigCenterSource_WatchSurvivesBadPayload(t *testing.T) {
	svc, addr := startFakeWatchService(t)
	src := useConfigCenterSource(t, addr, "cart", "dev", "bootstrap.yaml")

	events := runWatch(t, src)
	out := svc.nextStream(t, 2*time.Second)

	// 不是合法 YAML
	out <- &configv1.WatchKeysResponse{
		Type:  configv1.WatchEventType_WATCH_EVENT_TYPE_PUT,
		Entry: &configv1.ConfigEntry{Value: "server: [unclosed\n"},
	}
	ev := nextEvent(t, events, 2*time.Second)
	require.Error(t, ev.Err)
	assert.Nil(t, ev.Raw)

	// 空值同样是坏内容:让服务带着空 Bootstrap 跑比报错更难查
	out <- &configv1.WatchKeysResponse{
		Type:  configv1.WatchEventType_WATCH_EVENT_TYPE_PUT,
		Entry: &configv1.ConfigEntry{Value: ""},
	}
	ev = nextEvent(t, events, 2*time.Second)
	require.Error(t, ev.Err)

	// 同一条流上后续的正确推送照常送达
	out <- &configv1.WatchKeysResponse{
		Type:  configv1.WatchEventType_WATCH_EVENT_TYPE_PUT,
		Entry: &configv1.ConfigEntry{Value: testBootstrapYAML},
	}
	ev = nextEvent(t, events, 2*time.Second)
	require.NoError(t, ev.Err)
	require.NotNil(t, ev.Raw)
}

// 服务端断流后必须自己重连并重新收到 SNAPSHOT —— 断连期间漏掉的变更由此自愈。
func TestConfigCenterSource_WatchReconnects(t *testing.T) {
	svc, addr := startFakeWatchService(t)
	src := useConfigCenterSource(t, addr, "cart", "dev", "bootstrap.yaml")

	events := runWatch(t, src)
	out := svc.nextStream(t, 2*time.Second)

	// 先收一条,证明这条流是好的(否则重连会一直保持最小退避)
	out <- &configv1.WatchKeysResponse{
		Type:  configv1.WatchEventType_WATCH_EVENT_TYPE_SNAPSHOT,
		Entry: &configv1.ConfigEntry{Value: testBootstrapYAML},
	}
	require.NoError(t, nextEvent(t, events, 2*time.Second).Err)

	close(out) // 服务端结束这条流

	// 断流本身要作为一条错误事件上报,不能静默
	ev := nextEvent(t, events, 2*time.Second)
	require.Error(t, ev.Err)

	// 退避后重建流,并重新推一遍当前值
	out2 := svc.nextStream(t, watchMinBackoff+3*time.Second)
	out2 <- &configv1.WatchKeysResponse{
		Type:  configv1.WatchEventType_WATCH_EVENT_TYPE_SNAPSHOT,
		Entry: &configv1.ConfigEntry{Value: testBootstrapYAML},
	}
	ev = nextEvent(t, events, 2*time.Second)
	require.NoError(t, ev.Err)
	require.NotNil(t, ev.Raw)
}
