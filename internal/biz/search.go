package biz

import (
	"context"
)

type Product struct {
	ID           uint32  `json:"id"`
	Name         string  `json:"name"`
	SpuCode      string  `json:"spu_code"`
	Price        float64 `json:"price"`
	Status       string  `json:"status"`
	MainMediaUrl string  `json:"main_media_url"`
	Quantity     uint32  `json:"quantity"`
}

type (
	SearchRequest struct {
		Index string
		Name  string
	}

	SearchResponse struct {
		Products []Product
	}
)

// SearchRepo 用户接口
type SearchRepo interface {
	Search(ctx context.Context, req SearchRequest) (*SearchResponse, error)
}

type SearchUseCase struct {
	repo SearchRepo
}

func NewSearchUseCase(repo SearchRepo) *SearchUseCase {
	return &SearchUseCase{
		repo: repo,
	}
}

func (uc *SearchUseCase) Search(ctx context.Context, req SearchRequest) (*SearchResponse, error) {
	return uc.repo.Search(ctx, req)
}
