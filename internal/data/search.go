package data

import (
	"context"
	"fmt"

	"github.com/lens077/go-connect-template/internal/biz"
	"go.uber.org/zap"
)

var _ biz.SearchRepo = (*searchRepo)(nil)

type searchRepo struct {
	data *Data
	log  *zap.Logger
}

func NewSearchRepo(data *Data, logger *zap.Logger) biz.SearchRepo {
	return &searchRepo{data: data, log: logger}
}

func (r searchRepo) EnsureIndex(ctx context.Context, index string) error {
	return r.data.catalog.EnsureIndex(ctx, index)
}

func (r searchRepo) IndexProduct(ctx context.Context, index string, product biz.Product) error {
	return r.data.catalog.IndexProduct(ctx, index, CatalogProduct{
		ID:           product.ID,
		Name:         product.Name,
		SpuCode:      product.SpuCode,
		Price:        product.Price,
		Status:       product.Status,
		MainMediaURL: product.MainMediaUrl,
		Quantity:     product.Quantity,
	})
}

func (r searchRepo) Search(ctx context.Context, req biz.SearchRequest) (*biz.SearchResponse, error) {
	documents, err := r.data.catalog.SearchProducts(ctx, req.Index, req.Name)
	if err != nil {
		r.log.Error("search catalog query failed", zap.Error(err))
		return nil, fmt.Errorf("search products: %w", err)
	}

	products := make([]biz.Product, 0, len(documents))
	for _, document := range documents {
		products = append(products, biz.Product{
			ID:           document.ID,
			Name:         document.Name,
			SpuCode:      document.SpuCode,
			Price:        document.Price,
			Status:       document.Status,
			MainMediaUrl: document.MainMediaURL,
			Quantity:     document.Quantity,
		})
	}
	return &biz.SearchResponse{Products: products}, nil
}
