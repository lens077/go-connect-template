package data

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"

	"github.com/lens077/go-connect-template/constants"
	conf "github.com/lens077/go-connect-template/internal/conf/v1"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

// CatalogProduct is the project-owned document shape shared by search adapters.
type CatalogProduct struct {
	ID           uint32  `json:"id"`
	Name         string  `json:"name"`
	SpuCode      string  `json:"spu_code"`
	Price        float64 `json:"price"`
	Status       string  `json:"status"`
	MainMediaURL string  `json:"main_media_url"`
	Quantity     uint32  `json:"quantity"`
}

// SearchCatalog is the seam used by repository code. co selects exactly one
// adapter while generating a project; callers never import a vendor SDK.
type SearchCatalog interface {
	EnsureIndex(context.Context, string) error
	IndexProduct(context.Context, string, CatalogProduct) error
	SearchProducts(context.Context, string, string) ([]CatalogProduct, error)
	Health(context.Context) error
	Close()
}

type searchCatalogFactory func(*conf.Bootstrap, *zap.Logger) (SearchCatalog, error)

// NewSearchCatalog constructs the adapter selected by co. The source template
// contains every adapter so CI can compile them; generated projects retain one.
func NewSearchCatalog(lc fx.Lifecycle, cfg *conf.Bootstrap, logger *zap.Logger) (SearchCatalog, error) {
	catalogCfg := cfg.GetSearch().GetCatalog()
	if catalogCfg == nil {
		return nil, fmt.Errorf("search.catalog is not configured")
	}
	if catalogCfg.GetEndpoint() == "" {
		return nil, fmt.Errorf("search.catalog.endpoint is empty")
	}
	if catalogCfg.GetIndex() == "" {
		return nil, fmt.Errorf("search.catalog.index is empty")
	}

	factories := []searchCatalogFactory{
		newElasticsearchCatalog, // Raw template default. +co:elasticsearch
		newMeilisearchCatalog,   // +co:meilisearch
	}
	if len(factories) == 0 {
		return nil, fmt.Errorf("no search catalog adapter was generated")
	}

	catalog, err := factories[0](cfg, logger)
	if err != nil {
		return nil, err
	}
	lc.Append(fx.Hook{OnStop: func(context.Context) error {
		catalog.Close()
		return nil
	}})
	return catalog, nil
}

func newSearchTransport(cfg *conf.Search_Catalog) (*http.Transport, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConnsPerHost = 20

	tlsCfg := cfg.GetTls()
	if tlsCfg == nil || !tlsCfg.GetEnable() {
		return transport, nil
	}

	transport.TLSClientConfig = &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: tlsCfg.GetInsecureSkipVerify(),
	}
	if tlsCfg.GetCaPem() == "" {
		return transport, nil
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(tlsCfg.GetCaPem())) {
		return nil, fmt.Errorf("failed to parse search CA certificate: invalid PEM format")
	}
	transport.TLSClientConfig.RootCAs = pool
	return transport, nil
}

// CheckSearch reports whether the selected catalog and its configured index are ready.
func (d *Data) CheckSearch(ctx context.Context) error {
	if d.catalog == nil {
		return fmt.Errorf("search catalog not initialized")
	}
	ctx, cancel := context.WithTimeout(ctx, constants.DefaultHealthCheckTimeout)
	defer cancel()
	if err := d.catalog.Health(ctx); err != nil {
		return fmt.Errorf("search catalog health check failed: %w", err)
	}
	return nil
}
