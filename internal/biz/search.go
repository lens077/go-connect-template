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
	// EnsureIndex 幂等地建索引。ES 会在写入时自动建索引,但字段类型是猜的,
	// name 猜成 text 还好,spu_code 猜成 text 就没法精确匹配了 —— 所以显式建。
	EnsureIndex(ctx context.Context, index string) error
	// IndexProduct 写入(或覆盖)一篇文档
	IndexProduct(ctx context.Context, index string, p Product) error
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

// Reindex 是示例用法:建索引 + 灌一批文档。
//
// 没有对应的 RPC —— 它存在的意义是让 search 这个示例资源能被真正跑通:
// 只有 Search 的话,空集群上第一次调用必然是 index_not_found_exception,
// 看起来像客户端配错了。真实项目里这一步通常由 DB 变更事件驱动,
// 而不是像这里一样整批重灌。
func (uc *SearchUseCase) Reindex(ctx context.Context, index string, products []Product) error {
	if err := uc.repo.EnsureIndex(ctx, index); err != nil {
		return err
	}
	for _, p := range products {
		if err := uc.repo.IndexProduct(ctx, index, p); err != nil {
			return err
		}
	}
	return nil
}
