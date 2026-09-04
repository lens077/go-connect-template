package data

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/elastic/elastic-transport-go/v8/elastictransport"
	"github.com/elastic/go-elasticsearch/v9"
	"github.com/elastic/go-elasticsearch/v9/typedapi/core/search"
	"github.com/elastic/go-elasticsearch/v9/typedapi/indices/create"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types/enums/refresh"
	conf "github.com/lens077/go-connect-template/internal/conf/v1"
	"go.uber.org/zap"
)

var _ SearchCatalog = (*elasticsearchCatalog)(nil)

type elasticsearchCatalog struct {
	client       *elasticsearch.TypedClient
	transport    *http.Transport
	defaultIndex string
	log          *zap.Logger
}

func newElasticsearchCatalog(bootstrap *conf.Bootstrap, logger *zap.Logger) (SearchCatalog, error) {
	cfg := bootstrap.GetSearch().GetCatalog()
	if (cfg.GetUsername() == "") != (cfg.GetPassword() == "") {
		return nil, fmt.Errorf("search.catalog username and password must be configured together")
	}

	transport, err := newSearchTransport(cfg)
	if err != nil {
		return nil, err
	}
	opts := []elasticsearch.Option{
		elasticsearch.WithAddresses(cfg.GetEndpoint()),
		elasticsearch.WithLogger(&zapESLogger{logger: logger, conf: bootstrap.GetLog().GetSearch()}),
		elasticsearch.WithTransportOptions(elastictransport.WithTransport(transport)),
	}
	// Basic Auth wins when both credential forms are present. This lets the
	// provider-neutral example config also carry a key for the other adapter.
	if cfg.GetUsername() != "" {
		opts = append(opts, elasticsearch.WithBasicAuth(cfg.GetUsername(), cfg.GetPassword()))
	} else if cfg.GetApiKey() != "" {
		opts = append(opts, elasticsearch.WithAPIKey(cfg.GetApiKey()))
	}

	client, err := elasticsearch.NewTyped(opts...)
	if err != nil {
		return nil, fmt.Errorf("create elasticsearch catalog: %w", err)
	}
	logger.Info("elasticsearch search catalog initialized",
		zap.String("endpoint", cfg.GetEndpoint()),
		zap.String("index", cfg.GetIndex()),
	)
	return &elasticsearchCatalog{
		client:       client,
		transport:    transport,
		defaultIndex: cfg.GetIndex(),
		log:          logger,
	}, nil
}

func (c *elasticsearchCatalog) EnsureIndex(ctx context.Context, index string) error {
	index = c.indexName(index)
	exists, err := c.client.Indices.Exists(index).Do(ctx)
	if err != nil {
		return fmt.Errorf("check elasticsearch index %q: %w", index, err)
	}
	if exists {
		return nil
	}

	if _, err := c.client.Indices.Create(index).Request(&create.Request{
		Mappings: &types.TypeMapping{Properties: map[string]types.Property{
			"name":           types.NewTextProperty(),
			"spu_code":       types.NewKeywordProperty(),
			"status":         types.NewKeywordProperty(),
			"main_media_url": types.NewKeywordProperty(),
			"price":          types.NewDoubleNumberProperty(),
			"quantity":       types.NewIntegerNumberProperty(),
		}},
	}).Do(ctx); err != nil {
		return fmt.Errorf("create elasticsearch index %q: %w", index, err)
	}
	c.log.Info("elasticsearch index created", zap.String("index", index))
	return nil
}

func (c *elasticsearchCatalog) IndexProduct(ctx context.Context, index string, product CatalogProduct) error {
	index = c.indexName(index)
	if _, err := c.client.Index(index).
		Id(strconv.FormatUint(uint64(product.ID), 10)).
		Request(product).
		Refresh(refresh.Waitfor).
		Do(ctx); err != nil {
		return fmt.Errorf("index product %d into elasticsearch index %q: %w", product.ID, index, err)
	}
	return nil
}

func (c *elasticsearchCatalog) SearchProducts(ctx context.Context, index, query string) ([]CatalogProduct, error) {
	index = c.indexName(index)
	res, err := c.client.Search().Index(index).Request(&search.Request{
		Query: &types.Query{MultiMatch: &types.MultiMatchQuery{
			Query:  query,
			Fields: []string{"name", "spu_code"},
		}},
	}).Do(ctx)
	if err != nil {
		return nil, fmt.Errorf("search elasticsearch index %q: %w", index, err)
	}

	products := make([]CatalogProduct, 0, len(res.Hits.Hits))
	for _, hit := range res.Hits.Hits {
		var product CatalogProduct
		if err := json.Unmarshal(hit.Source_, &product); err != nil {
			return nil, fmt.Errorf("decode elasticsearch document %v: %w", hit.Id_, err)
		}
		products = append(products, product)
	}
	return products, nil
}

func (c *elasticsearchCatalog) Health(ctx context.Context) error {
	if _, err := c.client.Ping().Do(ctx); err != nil {
		return fmt.Errorf("elasticsearch ping failed: %w", err)
	}
	exists, err := c.client.Indices.Exists(c.defaultIndex).Do(ctx)
	if err != nil {
		return fmt.Errorf("check elasticsearch index %q readiness: %w", c.defaultIndex, err)
	}
	if !exists {
		return fmt.Errorf("elasticsearch index %q is not ready", c.defaultIndex)
	}
	return nil
}

func (c *elasticsearchCatalog) Close() {
	c.transport.CloseIdleConnections()
}

func (c *elasticsearchCatalog) indexName(index string) string {
	if index != "" {
		return index
	}
	return c.defaultIndex
}
