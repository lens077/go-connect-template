package service

import (
	"context"

	"connectrpc.com/connect"
	v1 "github.com/lens077/go-connect-template/api/search/v1"
	"github.com/lens077/go-connect-template/api/search/v1/searchv1connect"
	"github.com/lens077/go-connect-template/internal/biz"
)

type SearchService struct {
	uc *biz.SearchUseCase
}

var _ searchv1connect.SearchServiceHandler = (*SearchService)(nil)

func NewSearchService(uc *biz.SearchUseCase) searchv1connect.SearchServiceHandler {
	return &SearchService{uc: uc}
}

func (s *SearchService) Search(ctx context.Context, c *connect.Request[v1.SearchRequest]) (*connect.Response[v1.SearchResponse], error) {
	// 1. 调用业务逻辑层
	result, err := s.uc.Search(ctx, biz.SearchRequest{
		Index: c.Msg.Index,
		Name:  c.Msg.Name,
	})
	if err != nil {
		return nil, err
	}

	// 2. 转换结果集
	v1Products := make([]*v1.Product, 0, len(result.Products))
	for _, p := range result.Products {
		v1Products = append(v1Products, bizToV1Product(&p))
	}

	// 3. 返回响应
	return connect.NewResponse(&v1.SearchResponse{
		Products: v1Products,
	}), nil
}

// 转换逻辑封装
func bizToV1Product(bp *biz.Product) *v1.Product {
	if bp == nil {
		return nil
	}

	return &v1.Product{
		Id:           bp.ID,
		Name:         bp.Name,
		SpuCode:      bp.SpuCode,
		Price:        bp.Price,
		Status:       bp.Status,
		MainMediaUrl: bp.MainMediaUrl,
		Quantity:     bp.Quantity,
	}
}
