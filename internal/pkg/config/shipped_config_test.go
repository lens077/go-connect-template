package config

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/lens077/go-connect-template/constants"
	confv1 "github.com/lens077/go-connect-template/internal/conf/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestShippedConfigsDecode 保证仓库里随附的 configs/*.yml 能被当前 conf.proto 完整解出。
//
// 这个测试是「克隆下来就能 make dev-file 起服务」这条性质的唯一保障。mapstructure
// 对多余的 key 是静默忽略的:proto 改了字段名而 YAML 没跟着改时,配置照样「解析成功」,
// 只是值全部落空 —— 服务起来了但读的是零值。所以下面除了 require.NoError,
// 还必须逐段断言关键字段非零,光看不报错等于没测。
func TestShippedConfigsDecode(t *testing.T) {
	for _, name := range []string{"dev.yml", "pre.yml"} {
		t.Run(name, func(t *testing.T) {
			clearSourceEnv(t)
			t.Setenv(constants.EnvConfigSource, constants.ConfigSourceFile)
			t.Setenv(constants.EnvConfigFile, filepath.Join("..", "..", "..", "configs", name))

			src, err := NewSource()
			require.NoError(t, err)

			raw, err := src.Load(context.Background())
			require.NoError(t, err)

			var b confv1.Bootstrap
			require.NoError(t, decodeConfig(raw, &b))

			assertBootstrapPopulated(t, &b)
		})
	}
}

func assertBootstrapPopulated(t *testing.T, b *confv1.Bootstrap) {
	t.Helper()

	assert.NotEmpty(t, b.GetServer().GetAddr(), "server.addr")
	assert.NotZero(t, b.GetServer().GetHttp().GetReadTimeout().AsDuration(), "server.http.read_timeout")

	pg := b.GetData().GetDatabase().GetPostgres()
	assert.NotEmpty(t, pg.GetHost(), "postgres.host")
	assert.NotZero(t, pg.GetPort(), "postgres.port")
	// Duration 写成裸数字时这里会是 0:这是最容易漏的一类漂移
	assert.NotZero(t, pg.GetPool().GetMaxConnLifetime().AsDuration(), "postgres.pool.max_conn_lifetime")
	assert.NotZero(t, pg.GetPool().GetMaxConnIdleTime().AsDuration(), "postgres.pool.max_conn_idle_time")

	redis := b.GetData().GetCache().GetRedis()
	assert.NotEmpty(t, redis.GetHost(), "redis.host")
	assert.NotZero(t, redis.GetDialTimeout().AsDuration(), "redis.dial_timeout")

	// log 分 application / framework 两段,早期版本是平铺的 level/format,
	// 平铺写法解出来是空值,服务会以 info 级别静默启动
	assert.NotEmpty(t, b.GetLog().GetApplication().GetLevel(), "log.application.level")
	assert.NotEmpty(t, b.GetLog().GetFramework().GetLogLevel(), "log.framework.log_level")

	// consul 的 ttl 挂在 check 下面,直接写 consul.ttl 会被静默丢弃,
	// 心跳间隔取到 0 会让 ticker 直接 panic
	check := b.GetDiscovery().GetConsul().GetCheck()
	assert.NotEmpty(t, check.GetTtl().GetDuration(), "consul.check.ttl.duration")
	assert.NotZero(t, check.GetTtl().GetPingInterval().AsDuration(), "consul.check.ttl.ping_interval")
	assert.NotEmpty(t, check.GetDeregisterCriticalServiceAfter(), "consul.check.deregister_critical_service_after")

	assert.NotEmpty(t, b.GetSearch().GetElasticSearch().GetAddresses(), "search.elastic_search.addresses")
	assert.NotEmpty(t, b.GetAuth().GetCasdoor().GetEndpoint(), "auth.casdoor.endpoint")
}
