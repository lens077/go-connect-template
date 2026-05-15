package data

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"time"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	"github.com/elastic/go-elasticsearch/v9"
	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lens077/go-connect-template/constants"
	conf "github.com/lens077/go-connect-template/internal/conf/v1"
	"github.com/lens077/go-connect-template/internal/pkg/log"
	"github.com/redis/go-redis/v9"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// Module 导出给 FX 的 Provider
var Module = fx.Module("data",
	fx.Provide(
		NewData,
		NewPostgresPool,
		NewRedisClient,
		NewCasdoorAuthClient,
		NewElasticSearchClient,
		NewSearchRepo,
	),
)

// Data 包含所有数据源的客户端
type Data struct {
	db   *pgxpool.Pool
	rdb  *redis.Client
	auth *casdoorsdk.Client
	es   *elasticsearch.TypedClient
	log  *zap.Logger
}

// NewData 是 Data 的构造函数
func NewData(db *pgxpool.Pool, rdb *redis.Client, auth *casdoorsdk.Client, es *elasticsearch.TypedClient, logger *zap.Logger) *Data {
	return &Data{
		db:   db,
		rdb:  rdb,
		auth: auth,
		es:   es,
		log:  logger,
	}
}

// NewPostgresPool 创建pg数据库连接池
func NewPostgresPool(lc fx.Lifecycle, cfg *conf.Bootstrap, logger *zap.Logger) (*pgxpool.Pool, error) {
	dbCfg := cfg.Data.Database.Postgres // 从 Config 中获取 Data 配置

	connString := fmt.Sprintf("postgresql://%s:%s@%s:%d/%s?sslmode=%s&timezone=%s",
		dbCfg.User,
		dbCfg.Password,
		dbCfg.Host,
		dbCfg.Port,
		dbCfg.DbName,
		dbCfg.SslMode,
		dbCfg.Timezone,
	)

	poolCfg, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("parse database config failed: %v", err)
	}

	switch dbCfg.SslMode {
	case constants.SslModeVerifyCa, constants.SslModeVerifyFull:
		if dbCfg.Tls.CaPem != "" {
			caCertPool := x509.NewCertPool()
			if ok := caCertPool.AppendCertsFromPEM([]byte(dbCfg.Tls.CaPem)); !ok {
				return nil, fmt.Errorf("failed to parse CA PEM")
			}

			// TODO tls config
			if poolCfg.ConnConfig.TLSConfig == nil {
				poolCfg.ConnConfig.TLSConfig = &tls.Config{}
			}

			poolCfg.ConnConfig.TLSConfig.RootCAs = caCertPool
			// 关键点：如果你的证书域名是 server.dc1.consul，而连接地址是 IP
			// 那么需要显式指定 ServerName 否则 verify-full 会报错
			poolCfg.ConnConfig.TLSConfig.ServerName = dbCfg.Host
		}
	}

	// 链路追踪配置
	poolCfg.ConnConfig.Tracer = otelpgx.NewTracer()

	// 创建连接池
	pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		return nil, fmt.Errorf("connect to database failed: %v", err)
	}

	// 记录数据库统计信息
	if err := otelpgx.RecordStats(pool); err != nil {
		return nil, fmt.Errorf("unable to record database stats: %w", err)
	}

	// 测试连接
	if err := pool.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("database ping failed: %v", err)
	}

	logger.Info(fmt.Sprintf("database connected successfully to %s", dbCfg.Host))

	lc.Append(fx.Hook{
		// 应用停止时释放资源
		OnStop: func(ctx context.Context) error {
			logger.Info("closing database connection...")
			pool.Close()
			return nil
		},
	})

	return pool, nil
}

// NewRedisClient 创建 Redis 客户端
func NewRedisClient(lc fx.Lifecycle, cfg *conf.Bootstrap, logger *zap.Logger) (*redis.Client, error) {
	redisCfg := cfg.Data.Cache.Redis

	// 基础配置
	opts := &redis.Options{
		Addr:         fmt.Sprintf("%s:%d", redisCfg.Host, redisCfg.Port),
		Username:     redisCfg.Username,
		Password:     redisCfg.Password,
		DB:           int(redisCfg.Db),
		DialTimeout:  time.Duration(redisCfg.DialTimeout) * time.Second,
		ReadTimeout:  time.Duration(redisCfg.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(redisCfg.WriteTimeout) * time.Second,
		PoolSize:     int(redisCfg.PoolSize),
		MinIdleConns: int(redisCfg.MinIdleConns),
	}

	// TLS 适配
	if redisCfg.Tls != nil && redisCfg.Tls.Enable {
		tlsConfig := &tls.Config{
			InsecureSkipVerify: redisCfg.Tls.InsecureSkipVerify,
		}

		// 处理 CA 证书字符串
		if redisCfg.Tls.CaPem != "" {
			caCertPool := x509.NewCertPool()
			// 注意：这里直接使用字符串解析，不需要 os.ReadFile
			if ok := caCertPool.AppendCertsFromPEM([]byte(redisCfg.Tls.CaPem)); !ok {
				return nil, fmt.Errorf("failed to parse redis CA certificate: invalid PEM format")
			}
			tlsConfig.RootCAs = caCertPool

			// 如果你的证书中限制了访问域名（SANs），需要匹配 Addr 中的 Host
			// 你的证书里包含：dragonfly.sumery.com
			// tlsConfig.ServerName = "dragonfly.sumery.com"
		}

		opts.TLSConfig = tlsConfig
		logger.Info("tls connection initialized with CA string")
	}

	rdb := redis.NewClient(opts)

	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(redisCfg.DialTimeout)*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		// 记录带上下文的错误日志
		logger.Error("redis ping failed",
			zap.String("addr", redisCfg.Host),
			zap.Error(err),
		)

		// 关闭连接
		if errClose := rdb.Close(); errClose != nil {
			logger.Error("failed to close redis connection after ping failure",
				zap.String("addr", redisCfg.Host),
				zap.Error(errClose),
			)
		}

		// 返回错误给调用方（让 Fx 知道初始化失败）
		return nil, fmt.Errorf("redis ping failed: %w", err)
	}
	logger.Info("redis connected successfully",
		zap.String("addr", redisCfg.Host),
	)

	lc.Append(fx.Hook{
		// 应用停止时释放资源
		OnStop: func(ctx context.Context) error {
			logger.Info("closing redis connection...")
			return rdb.Close()
		},
	})

	return rdb, nil
}

func NewCasdoorAuthClient(conf *conf.Bootstrap, logger *zap.Logger) *casdoorsdk.Client {
	casdoorCfg := conf.Auth.Casdoor
	client := casdoorsdk.NewClient(
		casdoorCfg.Endpoint,         // endpoint
		casdoorCfg.ClientId,         // clientId
		casdoorCfg.ClientSecret,     // clientSecret
		casdoorCfg.Certificate,      // certificate (x509 format)
		casdoorCfg.OrganizationName, // organizationName
		casdoorCfg.ApplicationName,  // applicationName
	)

	logger.Info(fmt.Sprintf("casdoor connected successfully to %s", casdoorCfg.Endpoint))

	return client
}

// NewElasticSearchClient https://www.elastic.co/docs/reference/elasticsearch/clients/go/examples
func NewElasticSearchClient(lc fx.Lifecycle, conf *conf.Bootstrap, logger *zap.Logger) (*elasticsearch.TypedClient, error) {
	cfg := conf.Search.ElasticSearch
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// Elasticsearch 通常是高频内部调用，默认的 MaxIdleConnsPerHost（默认为 2）可能太小了
	// 如果并发请求很多，这会导致连接频繁创建和销毁，造成大量 TIME_WAIT
	// transport.MaxIdleConnsPerHost = 20

	if cfg.Tls.Enable {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: cfg.Tls.InsecureSkipVerify}
		if cfg.Tls.CaPem != "" {
			pool := x509.NewCertPool()
			if pool.AppendCertsFromPEM([]byte(cfg.Tls.CaPem)) {
				transport.TLSClientConfig.RootCAs = pool
			}
		}
	}

	esCfg := elasticsearch.Config{
		Addresses: cfg.Addresses,
		Username:  cfg.Username,
		Password:  cfg.Password,
		Logger:    &log.ZapESLogger{Logger: logger, Conf: conf.Log},
		Transport: transport,
	}

	es, err := elasticsearch.NewTypedClient(esCfg)
	if err != nil {
		logger.Error("failed to initialize elasticsearch client", zap.Error(err))
		return nil, err
	}

	logger.Info("elasticsearch client initialized", zap.Strings("addresses", cfg.Addresses))

	return es, nil
}

// CheckDatabase 检查数据库连通性
func (d *Data) CheckDatabase(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := d.db.Ping(ctx); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}
	return nil
}

// CheckCache 检查缓存连通性
func (d *Data) CheckCache(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := d.rdb.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("cache ping failed: %w", err)
	}
	return nil
}

// CheckElasticSearch 检查ES连通性
func (d *Data) CheckElasticSearch(ctx context.Context) error {
	if d.es == nil {
		return fmt.Errorf("elasticsearch client not initialized")
	}
	// 调用 Ping 方法
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := d.es.Ping().Do(ctx)
	if err != nil {
		return fmt.Errorf("elasticsearch ping failed: %w", err)
	}
	return nil
}
