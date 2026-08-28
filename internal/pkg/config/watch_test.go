package config

import (
	"context"
	"strings"
	"testing"
	"time"

	confv1 "github.com/lens077/go-connect-template/internal/conf/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx/fxtest"
	"go.uber.org/zap"
)

// stubWatchSource 一个可由测试逐条投喂事件的数据源。
// startWatch 关心的只是「收到事件之后怎么做」,不必真起一个配置中心。
type stubWatchSource struct {
	events  chan WatchEvent
	started chan struct{}
}

func newStubWatchSource() *stubWatchSource {
	return &stubWatchSource{
		events:  make(chan WatchEvent),
		started: make(chan struct{}, 4),
	}
}

func (s *stubWatchSource) Name() string { return "stub" }

func (s *stubWatchSource) Load(context.Context) (map[string]any, error) { return nil, nil }

func (s *stubWatchSource) Watch(ctx context.Context, onEvent func(WatchEvent)) error {
	s.started <- struct{}{}
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-s.events:
			if !ok {
				return nil
			}
			onEvent(ev)
		}
	}
}

// useSource 临时把包级 activeSource 换成给定数据源(Init 平时负责这件事)。
func useSource(t *testing.T, src Source) {
	t.Helper()
	srcMu.Lock()
	prev := activeSource
	activeSource = src
	srcMu.Unlock()

	t.Cleanup(func() {
		srcMu.Lock()
		activeSource = prev
		srcMu.Unlock()
	})
}

// runStartWatch 起一个跑着 startWatch 的 fx 生命周期,返回喂事件用的桩数据源。
func runStartWatch(t *testing.T, live *Live) *stubWatchSource {
	t.Helper()

	src := newStubWatchSource()
	useSource(t, src)

	lc := fxtest.NewLifecycle(t)
	startWatch(lc, zap.NewNop(), live)
	lc.RequireStart()
	t.Cleanup(lc.RequireStop)

	select {
	case <-src.started:
	case <-time.After(2 * time.Second):
		t.Fatal("Watch 没有被启动")
	}
	return src
}

// send 投一条事件并等它被处理完(stub 是无缓冲 channel,收下即表示上一条已回调完)。
func (s *stubWatchSource) send(t *testing.T, ev WatchEvent) {
	t.Helper()
	select {
	case s.events <- ev:
	case <-time.After(2 * time.Second):
		t.Fatal("投递事件超时")
	}
}

func TestStartWatch_AppliesPushedConfig(t *testing.T) {
	live := NewLive(&confv1.Bootstrap{Server: &confv1.Server{Addr: "old"}})
	src := runStartWatch(t, live)

	applied := make(chan struct{})
	live.Subscribe(func(_, _ *confv1.Bootstrap) { close(applied) })

	raw, err := parseYAMLToMap([]byte(testBootstrapYAML))
	require.NoError(t, err)
	src.send(t, WatchEvent{Raw: raw})

	select {
	case <-applied:
	case <-time.After(2 * time.Second):
		t.Fatal("推送的配置没有生效")
	}
	assert.Equal(t, "0.0.0.0:30006", live.Get().GetServer().GetAddr())
	assert.Equal(t, "localhost", live.Get().GetData().GetDatabase().GetPostgres().GetHost())
}

// 一份写错的配置不能把在跑的服务带塌:解码失败只记日志,当前配置原样保留。
func TestStartWatch_KeepsCurrentConfigOnDecodeFailure(t *testing.T) {
	live := NewLive(&confv1.Bootstrap{Server: &confv1.Server{Addr: "keep-me"}})
	src := runStartWatch(t, live)

	// read_timeout 不是合法的 duration,decodeConfig 的钩子会报错
	raw, err := parseYAMLToMap([]byte("server:\n  http:\n    read_timeout: not-a-duration\n"))
	require.NoError(t, err)
	src.send(t, WatchEvent{Raw: raw})

	// 再投一条无害事件,确保上一条已被处理完(stub 无缓冲,收下即代表前一条回调结束)
	src.send(t, WatchEvent{Err: assertErr})

	assert.Equal(t, "keep-me", live.Get().GetServer().GetAddr())
}

// 解码得过但不满足 conf.proto 约束的配置同样不能生效:校验失败只记日志,当前配置原样保留。
func TestStartWatch_KeepsCurrentConfigOnValidationFailure(t *testing.T) {
	live := NewLive(&confv1.Bootstrap{Server: &confv1.Server{Addr: "keep-me"}})
	src := runStartWatch(t, live)

	// framework.format 只允许 console/json
	bad := strings.Replace(testBootstrapYAML, "format: console", "format: xml", 1)
	raw, err := parseYAMLToMap([]byte(bad))
	require.NoError(t, err)
	src.send(t, WatchEvent{Raw: raw})

	src.send(t, WatchEvent{Err: assertErr})

	assert.Equal(t, "keep-me", live.Get().GetServer().GetAddr())
}

// 配置项被删不该让服务失能:继续用最后一份已知配置。
func TestStartWatch_KeepsCurrentConfigOnDelete(t *testing.T) {
	live := NewLive(&confv1.Bootstrap{Server: &confv1.Server{Addr: "keep-me"}})
	src := runStartWatch(t, live)

	src.send(t, WatchEvent{Deleted: true})
	src.send(t, WatchEvent{Err: assertErr})

	assert.Equal(t, "keep-me", live.Get().GetServer().GetAddr())
}

// 数据源不支持推送时(consul)不能起 watch,也不能报错 —— 保持「启动读一次」的语义。
func TestStartWatch_NoopWhenSourceCannotWatch(t *testing.T) {
	useSource(t, &plainSource{})

	lc := fxtest.NewLifecycle(t)
	assert.NotPanics(t, func() { startWatch(lc, zap.NewNop(), NewLive(nil)) })
	lc.RequireStart()
	lc.RequireStop()
}

// plainSource 只实现 Source,不实现 Watcher
type plainSource struct{}

func (plainSource) Name() string                                 { return "plain" }
func (plainSource) Load(context.Context) (map[string]any, error) { return nil, nil }

// assertErr 用作「无副作用的同步屏障事件」
var assertErr = errSentinel{}

type errSentinel struct{}

func (errSentinel) Error() string { return "sentinel" }
