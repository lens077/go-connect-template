package data

import (
	"encoding/json"
	"fmt"

	"github.com/elastic/go-elasticsearch/v9"
	"github.com/elastic/go-elasticsearch/v9/typedapi/core/search"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
	"github.com/lens077/go-connect-template/internal/biz"
	"github.com/redis/go-redis/v9"

	// "github.com/lens077/go-connect-template/internal/data/models"
	"context"

	"go.uber.org/zap"
)

var _ biz.SearchRepo = (*searchRepo)(nil)

type searchRepo struct {
	// queries *models.Queries
	es  *elasticsearch.TypedClient
	rdb *redis.Client
	log *zap.Logger
}

func NewSearchRepo(data *Data, logger *zap.Logger, es *elasticsearch.TypedClient) biz.SearchRepo {
	return &searchRepo{
		// queries: models.New(data.db),
		es:  es,
		rdb: data.rdb,
		log: logger,
	}
}

func (u searchRepo) Search(ctx context.Context, req biz.SearchRequest) (*biz.SearchResponse, error) {
	// 调整搜索字段以匹配新的数据表结构
	searchFields := []string{
		"name",              // 对应spus.name
		"spu_code",          // 对应spus.spu_code
		"description",       // 对应spus.description
		"specs.*",           // 对应spus.specs
		"skus.attributes.*", // 对应skus.attributes
	}

	res, err := u.es.Search().Index(req.Index).Request(&search.Request{
		Query: &types.Query{
			MultiMatch: &types.MultiMatchQuery{
				Query:  req.Name,
				Fields: searchFields,
			},
		},
	}).Do(ctx)
	if err != nil {
		return nil, err
	}

	bizProducts := make([]biz.Product, 0)
	for _, hit := range res.Hits.Hits {
		var productMap map[string]any
		if err := json.Unmarshal(hit.Source_, &productMap); err != nil {
			u.log.Error("解析文档失败:%v" + err.Error())
			continue
		}

		// 获取基本字段
		id := getInt64Field(productMap, "id")
		name := getStringField(productMap, "name")
		spuCode := getStringField(productMap, "spu_code")
		status := getStringField(productMap, "status")
		mainMediaUrl := getStringField(productMap, "main_media_url")

		// 计算最低价格
		minPrice := 0.0
		if skus, ok := productMap["skus"].([]any); ok && len(skus) > 0 {
			firstSku := true
			for _, sku := range skus {
				if skuMap, ok := sku.(map[string]any); ok {
					price := getFloat64Field(skuMap, "price")
					if firstSku || price < minPrice {
						minPrice = price
						firstSku = false
					}
				}
			}
		}

		// 计算总销量
		totalSales := 0
		if saleDetail, ok := productMap["sale_detail"].([]any); ok && len(saleDetail) > 0 {
			for _, sale := range saleDetail {
				if saleMap, ok := sale.(map[string]any); ok {
					quantity := getIntField(saleMap, "quantity")
					if quantity > 0 {
						totalSales += quantity
					}
				}
			}
		}

		// 构建biz.Product对象
		bizProduct := biz.Product{
			ID:           uint32(id),
			Name:         name,
			SpuCode:      spuCode,
			Price:        minPrice,
			Status:       status,
			MainMediaUrl: mainMediaUrl,
			Quantity:     uint32(totalSales),
		}

		bizProducts = append(bizProducts, bizProduct)
		u.log.Info(fmt.Sprintf("文档ID: %v, 评分: %f", hit.Id_, *hit.Score_))
		u.log.Info(fmt.Sprintf("商品: %+v,", bizProduct))
	}
	u.log.Info(fmt.Sprintf("成功解析 %d 个商品", len(bizProducts)))

	return &biz.SearchResponse{
		Products: bizProducts,
	}, nil
}

// 辅助函数：获取字符串字段
func getStringField(m map[string]any, key string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return ""
}

// 辅助函数：获取整数字段
func getIntField(m map[string]any, key string) int {
	if val, ok := m[key].(float64); ok {
		return int(val)
	}
	if val, ok := m[key].(int); ok {
		return val
	}
	return 0
}

// 辅助函数：获取int64字段
func getInt64Field(m map[string]any, key string) int64 {
	if val, ok := m[key].(float64); ok {
		return int64(val)
	}
	if val, ok := m[key].(int64); ok {
		return val
	}
	if val, ok := m[key].(int); ok {
		return int64(val)
	}
	return 0
}

// 辅助函数：获取float64字段
func getFloat64Field(m map[string]any, key string) float64 {
	if val, ok := m[key].(float64); ok {
		return val
	}
	if val, ok := m[key].(int); ok {
		return float64(val)
	}
	return 0
}
