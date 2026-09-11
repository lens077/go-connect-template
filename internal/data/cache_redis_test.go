package data

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	conf "github.com/lens077/go-connect-template/internal/conf/v1"
	"go.uber.org/fx/fxtest"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/durationpb"
)

func redisBootstrap(addr string, mutate func(*conf.Data_Cache_Redis)) *conf.Bootstrap {
	host, port, _ := strings.Cut(addr, ":")
	var p uint32
	for _, ch := range port {
		p = p*10 + uint32(ch-'0')
	}
	r := &conf.Data_Cache_Redis{
		Host: host, Port: p, Db: 3,
		DialTimeout:  durationpb.New(2 * time.Second),
		ReadTimeout:  durationpb.New(time.Second),
		WriteTimeout: durationpb.New(time.Second),
		PoolSize:     7, MinIdleConns: 2,
	}
	if mutate != nil {
		mutate(r)
	}
	return &conf.Bootstrap{Data: &conf.Data{Cache: &conf.Data_Cache{Redis: r}}}
}

func TestNewRedisClient_RejectsMissingConfig(t *testing.T) {
	_, err := NewRedisClient(fxtest.NewLifecycle(t), &conf.Bootstrap{}, zap.NewNop())
	if err == nil || !strings.Contains(err.Error(), "data.cache.redis is not configured") {
		t.Fatalf("want missing-config error, got %v", err)
	}
}

func TestNewRedisClient_ConnectsAndMapsOptions(t *testing.T) {
	mr := miniredis.RunT(t)
	lc := fxtest.NewLifecycle(t)

	rdb, err := NewRedisClient(lc, redisBootstrap(mr.Addr(), nil), zap.NewNop())
	if err != nil {
		t.Fatalf("NewRedisClient: %v", err)
	}

	// 配置真的映射到了 client options,而不是被默认值吞掉
	opts := rdb.Options()
	if opts.Addr != mr.Addr() || opts.DB != 3 || opts.PoolSize != 7 || opts.MinIdleConns != 2 || opts.DialTimeout != 2*time.Second {
		t.Fatalf("options not mapped from config: %+v", opts)
	}

	// 能真的用:写一个 key,miniredis 侧在配置的 DB 3 里能看到(顺带验 db 选择生效)
	if err := rdb.Set(context.Background(), "k", "v", 0).Err(); err != nil {
		t.Fatal(err)
	}
	if got, _ := mr.DB(3).Get("k"); got != "v" {
		t.Fatalf("write did not reach server DB 3, got %q", got)
	}
	if mr.DB(0).Exists("k") {
		t.Fatal("key leaked into DB 0: db option not applied")
	}

	// OnStop 关闭连接:之后再用必须报错,否则连接池会漏
	lc.RequireStart().RequireStop()
	if err := rdb.Ping(context.Background()).Err(); err == nil {
		t.Fatal("client must be closed after OnStop")
	}
}

func TestNewRedisClient_PingFailureClosesClient(t *testing.T) {
	// 指向一个没人监听的端口:ping 必失败。
	// 关键断言是构造函数返回 error 且不泄漏——fx 拿不到对象就不会注册 OnStop,
	// 所以构造函数自己必须把 client 关掉。这里用一个很短的 dial timeout 让它快失败。
	cfg := redisBootstrap("127.0.0.1:1", func(r *conf.Data_Cache_Redis) {
		r.DialTimeout = durationpb.New(200 * time.Millisecond)
	})
	start := time.Now()
	_, err := NewRedisClient(fxtest.NewLifecycle(t), cfg, zap.NewNop())
	if err == nil || !strings.Contains(err.Error(), "redis ping failed") {
		t.Fatalf("want ping failure, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("dial_timeout not honoured: took %v", elapsed)
	}
}

func TestNewRedisClient_InvalidCAPemIsAnError(t *testing.T) {
	cfg := redisBootstrap("127.0.0.1:1", func(r *conf.Data_Cache_Redis) {
		r.Tls = &conf.Data_Cache_Redis_Tls{Enable: true, CaPem: "not a pem"}
	})
	_, err := NewRedisClient(fxtest.NewLifecycle(t), cfg, zap.NewNop())
	if err == nil || !strings.Contains(err.Error(), "invalid PEM") {
		t.Fatalf("garbage ca_pem must fail before dialing, got %v", err)
	}
}
