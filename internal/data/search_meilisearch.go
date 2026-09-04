package data

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	conf "github.com/lens077/go-connect-template/internal/conf/v1"
	"github.com/meilisearch/meilisearch-go"
	"go.uber.org/zap"
)

var _ SearchCatalog = (*meilisearchCatalog)(nil)

type meilisearchCatalog struct {
	client       meilisearch.ServiceManager
	transport    *http.Transport
	defaultIndex string
}

func newMeilisearchCatalog(bootstrap *conf.Bootstrap, logger *zap.Logger) (SearchCatalog, error) {
	cfg := bootstrap.GetSearch().GetCatalog()
	transport, err := newSearchTransport(cfg)
	if err != nil {
		return nil, err
	}
	client := meilisearch.New(
		cfg.GetEndpoint(),
		meilisearch.WithAPIKey(cfg.GetApiKey()),
		meilisearch.WithCustomClient(&http.Client{
			Transport: transport,
			Timeout:   10 * time.Second,
		}),
	)
	logger.Info("meilisearch search catalog initialized",
		zap.String("endpoint", cfg.GetEndpoint()),
		zap.String("index", cfg.GetIndex()),
	)
	return &meilisearchCatalog{
		client:       client,
		transport:    transport,
		defaultIndex: cfg.GetIndex(),
	}, nil
}

func (c *meilisearchCatalog) EnsureIndex(ctx context.Context, index string) error {
	index = c.indexName(index)
	if _, err := c.client.Index(index).FetchInfoWithContext(ctx); err == nil {
		return nil
	} else if !isMeilisearchNotFound(err) {
		return fmt.Errorf("check meilisearch index %q: %w", index, err)
	}

	task, err := c.client.CreateIndexWithContext(ctx, &meilisearch.IndexConfig{
		Uid:        index,
		PrimaryKey: "id",
	})
	if err != nil {
		return fmt.Errorf("create meilisearch index %q: %w", index, err)
	}
	if err := c.waitForTask(ctx, task.TaskUID); err != nil {
		return fmt.Errorf("create meilisearch index %q: %w", index, err)
	}
	return nil
}

func (c *meilisearchCatalog) IndexProduct(ctx context.Context, index string, product CatalogProduct) error {
	index = c.indexName(index)
	task, err := c.client.Index(index).AddDocumentsWithContext(
		ctx,
		[]CatalogProduct{product},
		&meilisearch.DocumentOptions{PrimaryKey: meilisearch.StringPtr("id")},
	)
	if err != nil {
		return fmt.Errorf("index product %d into meilisearch index %q: %w", product.ID, index, err)
	}
	if err := c.waitForTask(ctx, task.TaskUID); err != nil {
		return fmt.Errorf("index product %d into meilisearch index %q: %w", product.ID, index, err)
	}
	return nil
}

func (c *meilisearchCatalog) SearchProducts(ctx context.Context, index, query string) ([]CatalogProduct, error) {
	index = c.indexName(index)
	res, err := c.client.Index(index).SearchWithContext(ctx, query, &meilisearch.SearchRequest{
		AttributesToRetrieve: []string{
			"id", "name", "spu_code", "price", "status", "main_media_url", "quantity",
		},
	})
	if err != nil {
		return nil, fmt.Errorf("search meilisearch index %q: %w", index, err)
	}

	products := make([]CatalogProduct, 0, len(res.Hits))
	for _, hit := range res.Hits {
		var product CatalogProduct
		if err := hit.DecodeInto(&product); err != nil {
			return nil, fmt.Errorf("decode meilisearch document: %w", err)
		}
		products = append(products, product)
	}
	return products, nil
}

func (c *meilisearchCatalog) Health(ctx context.Context) error {
	health, err := c.client.HealthWithContext(ctx)
	if err != nil {
		return fmt.Errorf("meilisearch health request failed: %w", err)
	}
	if health.Status != "available" {
		return fmt.Errorf("meilisearch returned health status %q", health.Status)
	}
	if _, err := c.client.Index(c.defaultIndex).FetchInfoWithContext(ctx); err != nil {
		return fmt.Errorf("meilisearch index %q is not ready: %w", c.defaultIndex, err)
	}
	return nil
}

func (c *meilisearchCatalog) Close() {
	c.client.Close()
	c.transport.CloseIdleConnections()
}

func (c *meilisearchCatalog) waitForTask(ctx context.Context, uid int64) error {
	task, err := c.client.WaitForTaskWithContext(ctx, uid, 50*time.Millisecond)
	if err != nil {
		return err
	}
	if task.Status != meilisearch.TaskStatusSucceeded {
		return fmt.Errorf("task %d finished with status %q", uid, task.Status)
	}
	return nil
}

func (c *meilisearchCatalog) indexName(index string) string {
	if index != "" {
		return index
	}
	return c.defaultIndex
}

func isMeilisearchNotFound(err error) bool {
	var apiErr *meilisearch.Error
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}
