package config

import (
	"sync"
	"testing"

	confv1 "github.com/lens077/go-connect-template/internal/conf/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLive_GetReturnsLatest(t *testing.T) {
	live := NewLive(&confv1.Bootstrap{Server: &confv1.Server{Addr: "old"}})
	assert.Equal(t, "old", live.Get().GetServer().GetAddr())

	live.Set(&confv1.Bootstrap{Server: &confv1.Server{Addr: "new"}})
	assert.Equal(t, "new", live.Get().GetServer().GetAddr())
}

func TestLive_NilIsNormalized(t *testing.T) {
	live := NewLive(nil)
	require.NotNil(t, live.Get())

	live.Set(&confv1.Bootstrap{Server: &confv1.Server{Addr: "kept"}})
	live.Set(nil)
	assert.Equal(t, "kept", live.Get().GetServer().GetAddr())
}

func TestLive_SubscriberSeesSwappedValue(t *testing.T) {
	live := NewLive(&confv1.Bootstrap{Server: &confv1.Server{Addr: "old"}})

	var gotOld, gotCur, gotDuringCallback string
	live.Subscribe(func(old, cur *confv1.Bootstrap) {
		gotOld = old.GetServer().GetAddr()
		gotCur = cur.GetServer().GetAddr()
		gotDuringCallback = live.Get().GetServer().GetAddr()
	})

	live.Set(&confv1.Bootstrap{Server: &confv1.Server{Addr: "new"}})

	assert.Equal(t, "old", gotOld)
	assert.Equal(t, "new", gotCur)
	assert.Equal(t, "new", gotDuringCallback)
}

func TestLive_SubscribeCancelIsIdempotent(t *testing.T) {
	live := NewLive(nil)

	var calls int
	cancel := live.Subscribe(func(_, _ *confv1.Bootstrap) { calls++ })

	live.Set(&confv1.Bootstrap{})
	require.Equal(t, 1, calls)

	cancel()
	assert.NotPanics(t, cancel)

	live.Set(&confv1.Bootstrap{})
	assert.Equal(t, 1, calls, "注销后不该再被回调")
}

func TestLive_CallbacksAreSerialized(t *testing.T) {
	live := NewLive(nil)

	var mu sync.Mutex
	var inFlight, maxInFlight int
	live.Subscribe(func(_, _ *confv1.Bootstrap) {
		mu.Lock()
		inFlight++
		if inFlight > maxInFlight {
			maxInFlight = inFlight
		}
		mu.Unlock()
		defer func() {
			mu.Lock()
			inFlight--
			mu.Unlock()
		}()
	})

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			live.Set(&confv1.Bootstrap{})
		}()
	}
	wg.Wait()

	assert.Equal(t, 1, maxInFlight, "同一个订阅者的回调不应并发执行")
}
