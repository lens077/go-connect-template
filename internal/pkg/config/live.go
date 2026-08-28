package config

import (
	"sync"
	"sync/atomic"

	confv1 "github.com/lens077/go-connect-template/internal/conf/v1"
)

// Live 持有「当前」这一份配置,支持整体原子替换。
//
// 用 atomic.Pointer 而不是 RWMutex:Get 落在每个请求的热路径上
// (见 internal/data/cart.go 拼缩略图 URL 的地方),而写只在配置变更时发生。
//
// 整体替换而不是逐字段改:配置是一个整体,半新半旧的中间态没有任何调用方能正确处理。
type Live struct {
	ptr atomic.Pointer[confv1.Bootstrap]

	// applyMu 串行化整个 Set(换指针 + 回调)。与 mu 分开是为了让回调里
	// 调用 Subscribe/取消订阅不至于自锁。
	applyMu sync.Mutex

	mu   sync.Mutex
	next int
	subs map[int]func(old, cur *confv1.Bootstrap)
}

func NewLive(b *confv1.Bootstrap) *Live {
	l := &Live{subs: make(map[int]func(old, cur *confv1.Bootstrap))}
	if b == nil {
		b = &confv1.Bootstrap{}
	}
	l.ptr.Store(b)
	return l
}

// Get 返回当前配置。返回的指针是只读的 —— 调用方不得就地修改。
func (l *Live) Get() *confv1.Bootstrap { return l.ptr.Load() }

// Set 替换当前配置,并**同步**依次回调订阅者。
//
// 同步且全程串行是有意的:订阅者要做的是重建连接池这类有副作用的事,
// 串行才能保证「上一次重建做完才开始下一次」,不会两次变更并发重建同一个池,
// 也不会出现后到的配置先建完、先到的后建完从而把旧配置留在最后的情况。
//
// 代价是回调里不能再调 Set(会自锁),但「处理配置变更时又去改配置」本身就是个环。
func (l *Live) Set(cur *confv1.Bootstrap) {
	if cur == nil {
		return
	}

	l.applyMu.Lock()
	defer l.applyMu.Unlock()

	old := l.ptr.Swap(cur)

	l.mu.Lock()
	fns := make([]func(old, cur *confv1.Bootstrap), 0, len(l.subs))
	for _, fn := range l.subs {
		fns = append(fns, fn)
	}
	l.mu.Unlock()

	for _, fn := range fns {
		fn(old, cur)
	}
}

// Subscribe 注册配置变更回调,返回的函数用于注销(幂等)。
func (l *Live) Subscribe(fn func(old, cur *confv1.Bootstrap)) func() {
	l.mu.Lock()
	id := l.next
	l.next++
	l.subs[id] = fn
	l.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			l.mu.Lock()
			delete(l.subs, id)
			l.mu.Unlock()
		})
	}
}
