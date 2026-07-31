package data

import (
	"context"
	"fmt"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk" // +co:casdoor
	"github.com/elastic/go-elasticsearch/v9"       // +co:elasticsearch
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lens077/go-connect-template/constants"
	"github.com/lens077/go-connect-template/internal/pkg/dbutil"
	"github.com/redis/go-redis/v9" // +co:redis
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// 本文件只负责把各数据源客户端聚成一个 Data 并交给 fx。
// 每种数据源的构造逻辑各自成文件(db_postgres.go / cache_redis.go /
// auth_casdoor.go / search_elasticsearch.go),这样 co 裁剪某个 feature 时
// 是整文件删除加上这里的几行标记,不需要改写任何代码。
//
// 行尾 / 成对的 +co: 注释是给 co-cli 看的裁剪标记,对 Go 编译器只是普通注释。

// Module 导出给 FX 的 Provider
var Module = fx.Module("data",
	fx.Provide(
		NewData,
		NewPostgresPool,
		NewRedisClient,         // +co:redis
		NewCasdoorAuthClient,   // +co:casdoor
		NewElasticSearchClient, // +co:elasticsearch
		// 逗号是「与」:两个 feature 都开着这一行才保留。
		// 示例资源本身就是拿 ES 实现的,单独留下它会编译不过。
		NewSearchRepo, // +co:example,elasticsearch
		// +co:anchor data-providers
	),
)

// Data 持有所有数据源的客户端
type Data struct {
	db           *pgxpool.Pool
	rdb          *redis.Client              // +co:redis
	auth         *casdoorsdk.Client         // +co:casdoor
	es           *elasticsearch.TypedClient // +co:elasticsearch
	dbErrHandler *dbutil.Handler
	log          *zap.Logger
}

// NewData 是 Data 的构造函数
func NewData(
	db *pgxpool.Pool,
	rdb *redis.Client, // +co:redis
	auth *casdoorsdk.Client, // +co:casdoor
	es *elasticsearch.TypedClient, // +co:elasticsearch
	logger *zap.Logger,
) *Data {
	return &Data{
		db:   db,
		rdb:  rdb,  // +co:redis
		auth: auth, // +co:casdoor
		es:   es,   // +co:elasticsearch
		log:  logger,
		dbErrHandler: dbutil.NewHandler(
			// 把 pg 的错误码映射成业务错误,在这里集中登记:
			// dbutil.WithErrorMapping("23505", biz.ErrAlreadyExists),
			// dbutil.WithErrorMapping("23503", biz.ErrNotFound),
			dbutil.WithLogging(true),
			dbutil.WithLogger(func(err error, pgErr *pgconn.PgError) {
				if pgErr != nil {
					logger.Warn("database error",
						zap.String("code", pgErr.Code),
						zap.String("message", pgErr.Message),
						zap.String("detail", pgErr.Detail),
					)
				}
			}),
		),
	}
}

// CheckDatabase 检查数据库连通性
func (d *Data) CheckDatabase(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, constants.DefaultHealthCheckTimeout)
	defer cancel()
	if err := d.db.Ping(ctx); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}
	return nil
}
