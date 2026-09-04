package data

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"

	kitotel "github.com/lens077/go-connect-kit/otel"
	"github.com/lens077/go-connect-template/constants"
	conf "github.com/lens077/go-connect-template/internal/conf/v1"
	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// NewRedisClient 创建 Redis 客户端
func NewRedisClient(lc fx.Lifecycle, cfg *conf.Bootstrap, logger *zap.Logger) (*redis.Client, error) {
	redisCfg := cfg.GetData().GetCache().GetRedis()
	if redisCfg == nil {
		return nil, fmt.Errorf("data.cache.redis is not configured")
	}

	opts := &redis.Options{
		Addr:         fmt.Sprintf("%s:%d", redisCfg.Host, redisCfg.Port),
		Username:     redisCfg.Username,
		Password:     redisCfg.Password,
		DB:           int(redisCfg.Db),
		DialTimeout:  redisCfg.DialTimeout.AsDuration(),
		ReadTimeout:  redisCfg.ReadTimeout.AsDuration(),
		WriteTimeout: redisCfg.WriteTimeout.AsDuration(),
		PoolSize:     int(redisCfg.PoolSize),
		MinIdleConns: int(redisCfg.MinIdleConns),
	}

	if tlsCfg := redisCfg.GetTls(); tlsCfg != nil && tlsCfg.Enable {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: tlsCfg.InsecureSkipVerify,
		}

		// CA 直接以 PEM 字符串放在配置里,不落盘:配置来自 Consul 时本就没有文件可读
		if tlsCfg.CaPem != "" {
			caCertPool := x509.NewCertPool()
			if ok := caCertPool.AppendCertsFromPEM([]byte(tlsCfg.CaPem)); !ok {
				return nil, fmt.Errorf("failed to parse redis CA certificate: invalid PEM format")
			}
			tlsConfig.RootCAs = caCertPool

			// 证书 SAN 与 Addr 里的 host 不一致时(比如走 IP 直连),
			// 需要显式指定 ServerName,否则握手会因主机名不匹配失败:
			// tlsConfig.ServerName = "redis.example.com"
		}

		opts.TLSConfig = tlsConfig
		logger.Info("redis tls enabled")
	}

	kitotel.EnsureRedisInstrumentation(logger)
	rdb := redis.NewClient(opts)

	dialTimeout := redisCfg.DialTimeout.AsDuration()
	if dialTimeout <= 0 {
		dialTimeout = constants.DefaultDBPingTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		logger.Error("redis ping failed",
			zap.String("addr", redisCfg.Host),
			zap.Error(err),
		)

		// 探活失败也要把客户端关掉:fx 拿不到对象就不会注册 OnStop,
		// 直接 return 会把连接池和它的后台 goroutine 一起漏掉
		if errClose := rdb.Close(); errClose != nil {
			logger.Error("failed to close redis connection after ping failure",
				zap.String("addr", redisCfg.Host),
				zap.Error(errClose),
			)
		}

		return nil, fmt.Errorf("redis ping failed: %w", err)
	}
	logger.Info("redis connected successfully",
		zap.String("addr", redisCfg.Host),
	)

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			logger.Info("closing redis connection...")
			return rdb.Close()
		},
	})

	return rdb, nil
}

// Redis 返回底层客户端,供仓储层直接使用
func (d *Data) Redis() *redis.Client {
	return d.rdb
}

// CheckCache 检查缓存连通性
func (d *Data) CheckCache(ctx context.Context) error {
	if d.rdb == nil {
		return fmt.Errorf("redis client not initialized")
	}
	ctx, cancel := context.WithTimeout(ctx, constants.DefaultHealthCheckTimeout)
	defer cancel()
	if err := d.rdb.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("cache ping failed: %w", err)
	}
	return nil
}
