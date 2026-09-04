package data

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"time"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	kitotel "github.com/lens077/go-connect-kit/otel"
	"github.com/lens077/go-connect-template/constants"
	conf "github.com/lens077/go-connect-template/internal/conf/v1"
	"github.com/lens077/go-connect-template/internal/data/models"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// 本文件收敛所有 postgres 相关的东西:连接池构造、TLS 装配、事务边界。
// 数据库驱动的替换面就是这一个文件 —— data.go 只依赖 Data.db 这个字段,
// 换驱动时不需要动业务侧的仓储代码。

// contextTxKey 事务在 context 中的键。用私有空结构体做键,避免与其它包相撞。
type contextTxKey struct{}

// WithTx 将事务存入上下文
func (d *Data) WithTx(ctx context.Context, tx pgx.Tx) context.Context {
	return context.WithValue(ctx, contextTxKey{}, tx)
}

// Tx 返回 context 中的事务句柄;不在事务中时返回 nil, false。
// 仓储层据此决定把 sqlc 的 Queries 绑到事务还是连接池上:
//
//	q := models.New(d.DB(ctx))
func (d *Data) Tx(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(contextTxKey{}).(pgx.Tx)
	return tx, ok
}

// DB 返回当前该用来跑查询的句柄:事务中返回事务,否则返回连接池。
// pgx.Tx 与 *pgxpool.Pool 都满足 sqlc 生成的 models.DBTX,所以仓储层
// 一律写成下面这样,同一份代码在事务内外都成立,不必分两条路径:
//
//	models.New(d.DB(ctx)).GetProduct(ctx, id)
func (d *Data) DB(ctx context.Context) models.DBTX {
	if tx, ok := d.Tx(ctx); ok {
		return tx
	}
	return d.db
}

// ExecTx 在事务中执行 fn。
//
// 已在事务中时复用外层事务而不是嵌套开启:pgx 的嵌套事务要走 SAVEPOINT,
// 语义与外层不一致,复用是这里唯一想要的行为。
// fn 返回错误则回滚;fn panic 则回滚后原样抛出,不吞 panic。
func (d *Data) ExecTx(ctx context.Context, fn func(context.Context) error) error {
	if _, ok := d.Tx(ctx); ok {
		d.log.Debug("reuse existing transaction")
		return fn(ctx)
	}

	d.log.Debug("begin transaction")
	tx, err := d.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin tx failed: %w", err)
	}

	txCtx := d.WithTx(ctx, tx)

	defer func() {
		if p := recover(); p != nil {
			// 回滚用 context.WithoutCancel:panic 现场的 ctx 可能已被取消,
			// 拿已取消的 ctx 去回滚会直接失败,连接就一直挂在事务里直到超时
			_ = tx.Rollback(context.WithoutCancel(ctx))
			panic(p)
		}
	}()

	if err := fn(txCtx); err != nil {
		if rbErr := tx.Rollback(context.WithoutCancel(ctx)); rbErr != nil {
			return fmt.Errorf("%w (rollback err: %v)", err, rbErr)
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit failed: %w", err)
	}
	d.log.Debug("transaction committed")
	return nil
}

// NewPostgresPool 创建 pg 连接池
func NewPostgresPool(lc fx.Lifecycle, cfg *conf.Bootstrap, logger *zap.Logger) (*pgxpool.Pool, error) {
	dbCfg := cfg.GetData().GetDatabase().GetPostgres()
	if dbCfg == nil {
		return nil, fmt.Errorf("data.database.postgres is not configured")
	}

	// 用 ParseConfig("") 生成一套带默认值的模板再灌配置,
	// 比手工构造 pgxpool.Config 少踩很多「默认值为零」的坑
	pgConf, err := pgxpool.ParseConfig("")
	if err != nil {
		return nil, fmt.Errorf("parse base pool config failed: %w", err)
	}

	if pgConf.ConnConfig.RuntimeParams == nil {
		pgConf.ConnConfig.RuntimeParams = make(map[string]string)
	}
	pgConf.ConnConfig.RuntimeParams["timezone"] = dbCfg.Timezone

	pgConf.ConnConfig.Host = dbCfg.Host
	pgConf.ConnConfig.Port = uint16(dbCfg.Port)
	pgConf.ConnConfig.Database = dbCfg.DbName
	pgConf.ConnConfig.User = dbCfg.User
	pgConf.ConnConfig.Password = dbCfg.Password

	// 先判空:pool 段缺失时直接取字段会空指针,而这一段本就是可选的
	if pool := dbCfg.GetPool(); pool != nil {
		pgConf.MaxConnLifetime = pool.MaxConnLifetime.AsDuration()
		pgConf.MaxConnIdleTime = pool.MaxConnIdleTime.AsDuration()
		pgConf.MaxConns = int32(pool.MaxConns)
		pgConf.MinConns = int32(pool.MinConns)
		if t := pool.PingTimeout.AsDuration(); t > 0 {
			pgConf.PingTimeout = t
		}
	}

	if err := applyPostgresTLS(pgConf, dbCfg, logger); err != nil {
		return nil, err
	}

	// 链路追踪:每条 SQL 都会带上 span,与 otel.Module 共用同一个 TracerProvider
	pgConf.ConnConfig.Tracer = otelpgx.NewTracer(
		otelpgx.WithTrimSQLInSpanName(),
		otelpgx.WithSpanNameFunc(kitotel.SQLSpanName),
	)

	pool, err := pgxpool.NewWithConfig(context.Background(), pgConf)
	if err != nil {
		return nil, fmt.Errorf("connect to database failed: %w", err)
	}

	if err := otelpgx.RecordStats(pool); err != nil {
		pool.Close()
		return nil, fmt.Errorf("unable to record database stats: %w", err)
	}

	// 探活失败要把池关掉再返回:直接 return 会把这个池连同它的后台
	// 健康检查 goroutine 一起漏掉,fx 因为没拿到对象也不会注册 OnStop
	ctx, cancel := context.WithTimeout(context.Background(), pingTimeoutOr(pgConf.PingTimeout))
	defer cancel()
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database ping failed: %w", err)
	}

	logger.Info("postgres connected successfully", zap.String("host", dbCfg.Host))

	lc.Append(fx.Hook{
		OnStop: func(ctx context.Context) error {
			logger.Info("closing postgres connection...")
			pool.Close()
			return nil
		},
	})

	return pool, nil
}

// applyPostgresTLS 按 ssl_mode 装配 TLS。只有 verify-ca / verify-full 需要特殊处理,
// 其余模式交给 pgx 的默认行为。
func applyPostgresTLS(pgConf *pgxpool.Config, dbCfg *conf.Data_Database_Postgres, logger *zap.Logger) error {
	tlsCfg := dbCfg.GetTls()
	if tlsCfg == nil || tlsCfg.CaPem == "" {
		return nil
	}

	caCertPool := x509.NewCertPool()
	if ok := caCertPool.AppendCertsFromPEM([]byte(tlsCfg.CaPem)); !ok {
		return fmt.Errorf("failed to parse CA PEM")
	}

	switch tlsCfg.SslMode {
	case constants.SslModeVerifyCa:
		// verify-ca 要「验证书链但不验主机名」。Go 的 tls 没有这个开关组合,
		// 只能关掉内置校验、在 VerifyPeerCertificate 里自己验链 ——
		// 这里的 InsecureSkipVerify 不是「不校验」,是「换我来校验」。
		logger.Info("setting up ssl mode: verify-ca config")
		pgConf.ConnConfig.TLSConfig = &tls.Config{
			RootCAs:            caCertPool,
			InsecureSkipVerify: true,
			VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
				if len(rawCerts) == 0 {
					return fmt.Errorf("no certificate presented by server")
				}
				opts := x509.VerifyOptions{
					Roots:         caCertPool,
					CurrentTime:   time.Now(),
					Intermediates: x509.NewCertPool(),
				}
				cert, err := x509.ParseCertificate(rawCerts[0])
				if err != nil {
					return err
				}
				for _, rawCert := range rawCerts[1:] {
					if c, err := x509.ParseCertificate(rawCert); err == nil {
						opts.Intermediates.AddCert(c)
					}
				}
				_, err = cert.Verify(opts)
				return err
			},
		}
	case constants.SslModeVerifyFull:
		logger.Info("setting up ssl mode: verify-full config")
		pgConf.ConnConfig.TLSConfig = &tls.Config{
			RootCAs:            caCertPool,
			InsecureSkipVerify: false,
			ServerName:         dbCfg.Host,
		}
	}
	return nil
}

// pingTimeoutOr 兜底:配置没写 ping_timeout 时给个有限值。
// context.WithTimeout(0) 会立刻超时,连接池明明是好的也起不来。
func pingTimeoutOr(d time.Duration) time.Duration {
	if d <= 0 {
		return constants.DefaultDBPingTimeout
	}
	return d
}
