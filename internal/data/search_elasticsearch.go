package data

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"

	"github.com/elastic/elastic-transport-go/v8/elastictransport"
	"github.com/elastic/go-elasticsearch/v9"
	"github.com/lens077/go-connect-template/constants"
	conf "github.com/lens077/go-connect-template/internal/conf/v1"
	"github.com/lens077/go-connect-template/internal/pkg/log"
	"go.uber.org/zap"
)

// NewElasticSearchClient 构造 ES 客户端。
// 参考 https://www.elastic.co/docs/reference/elasticsearch/clients/go/examples
//
// 用 go-elasticsearch v9.4 起的函数式选项 API(NewTyped + With*)。
// 老的 NewTypedClient(elasticsearch.Config{}) 整套已标记 Deprecated:
// Config 是个大结构体,加字段就是改公开 API,而选项函数各自独立演进。
// 它仍然可用,但新代码不该再写。
//
// 同样不在启动阶段探活:ES 属于可降级依赖,挂了不该拖住整个服务启动。
// cmd/server/main.go 的启动检查对它只打警告,连通性由 /healthz 的
// CheckElasticSearch 持续暴露。
//
// 服务端大版本要跟 go.mod 里的客户端对齐(现在是 go-elasticsearch/v9 → ES 9.x):
// v9 会发 `Accept: application/vnd.elasticsearch+json; compatible-with=9`,
// 8.x 服务端不认,连 Ping 都返回 400,而错误信息里只有 "status code: 400"。
// 本地开发直接用 infrastructure/elasticsearch/compose.yaml,版本已经对好。
func NewElasticSearchClient(cfg *conf.Bootstrap, logger *zap.Logger) (*elasticsearch.TypedClient, error) {
	esCfg := cfg.GetSearch().GetElasticSearch()
	if esCfg == nil {
		return nil, fmt.Errorf("search.elastic_search is not configured")
	}
	// 地址留空时 NewTyped 会静默退回 ELASTICSEARCH_URL 或 http://localhost:9200。
	// 那是个能连上、但几乎肯定不是你想连的地方,所以这里直接拦下来。
	if len(esCfg.Addresses) == 0 {
		return nil, fmt.Errorf("search.elastic_search.addresses is empty")
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	// ES 通常是高频内部调用,默认 MaxIdleConnsPerHost 只有 2,并发一高
	// 就会不断新建/销毁连接堆出大量 TIME_WAIT。按实际并发放开:
	// transport.MaxIdleConnsPerHost = 20

	if tlsCfg := esCfg.GetTls(); tlsCfg != nil && tlsCfg.Enable {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: tlsCfg.InsecureSkipVerify}
		if tlsCfg.CaPem != "" {
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM([]byte(tlsCfg.CaPem)) {
				return nil, fmt.Errorf("failed to parse elasticsearch CA certificate: invalid PEM format")
			}
			transport.TLSClientConfig.RootCAs = pool
		}
	}

	opts := []elasticsearch.Option{
		elasticsearch.WithAddresses(esCfg.Addresses...),
		elasticsearch.WithLogger(&log.ZapESLogger{Logger: logger, Conf: cfg.Log}),
		// TLS 由上面那个 transport 定,所以走 WithTransportOptions 直接换掉
		// RoundTripper,而不是 elasticsearch.WithCACert —— 后者会自建一个
		// 只含该 CA 的池,和 InsecureSkipVerify 这类开关拼不到一起。
		elasticsearch.WithTransportOptions(elastictransport.WithTransport(transport)),
	}
	// 用户名为空时不要挂 basic auth:挂了会发一个空 Authorization 头,
	// 在开了匿名访问的集群上反而会被判成鉴权失败。
	if esCfg.Username != "" {
		opts = append(opts, elasticsearch.WithBasicAuth(esCfg.Username, esCfg.Password))
	}

	es, err := elasticsearch.NewTyped(opts...)
	if err != nil {
		logger.Error("failed to initialize elasticsearch client", zap.Error(err))
		return nil, err
	}

	logger.Info("elasticsearch client initialized", zap.Strings("addresses", esCfg.Addresses))

	return es, nil
}

// ES 返回底层客户端,供仓储层直接使用
func (d *Data) ES() *elasticsearch.TypedClient {
	return d.es
}

// CheckElasticSearch 检查 ES 连通性
func (d *Data) CheckElasticSearch(ctx context.Context) error {
	if d.es == nil {
		return fmt.Errorf("elasticsearch client not initialized")
	}
	ctx, cancel := context.WithTimeout(ctx, constants.DefaultHealthCheckTimeout)
	defer cancel()
	if _, err := d.es.Ping().Do(ctx); err != nil {
		return fmt.Errorf("elasticsearch ping failed: %w", err)
	}
	return nil
}
