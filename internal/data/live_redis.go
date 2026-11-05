package data

import (
	"sync/atomic"

	"github.com/redis/go-redis/v9"
)

// LiveRedis is the stable handle used by repositories that need Redis. The
// client is always loaded at call time so a configuration swap is visible.
type LiveRedis struct{ c atomic.Pointer[redis.Client] }

func NewLiveRedis(client *redis.Client) *LiveRedis {
	l := &LiveRedis{}
	l.c.Store(client)
	return l
}

func (l *LiveRedis) Client() *redis.Client                   { return l.c.Load() }
func (l *LiveRedis) Swap(client *redis.Client) *redis.Client { return l.c.Swap(client) }
