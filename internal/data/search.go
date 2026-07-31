package data

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/elastic/go-elasticsearch/v9/typedapi/core/search"
	"github.com/elastic/go-elasticsearch/v9/typedapi/indices/create"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types/enums/refresh"
	"github.com/lens077/go-connect-template/internal/biz"
	// "github.com/lens077/go-connect-template/internal/data/models"
	"go.uber.org/zap"
)

var _ biz.SearchRepo = (*searchRepo)(nil)

type searchRepo struct {
	// queries *models.Queries
	data *Data
	log  *zap.Logger
}

func NewSearchRepo(data *Data, logger *zap.Logger) biz.SearchRepo {
	return &searchRepo{
		// queries: models.New(data.db),
		data: data,
		log:  logger,
	}
}

// EnsureIndex 幂等地建索引,字段类型显式声明。
//
// 关键是 spu_code 这类标识符必须是 keyword 而不是 text:text 会被分词器切碎,
// "SPU-001" 变成 ["spu", "001"],精确查/聚合/排序全部失效。而 ES 的动态映射
// 会把它猜成 text —— 等发现时索引里已经有数据了,只能重建。
func (u searchRepo) EnsureIndex(ctx context.Context, index string) error {
	exists, err := u.data.es.Indices.Exists(index).Do(ctx)
	if err != nil {
		return fmt.Errorf("check index %q: %w", index, err)
	}
	if exists {
		return nil
	}

	if _, err := u.data.es.Indices.Create(index).Request(&create.Request{
		Mappings: &types.TypeMapping{
			Properties: map[string]types.Property{
				"name":           types.NewTextProperty(),
				"spu_code":       types.NewKeywordProperty(),
				"status":         types.NewKeywordProperty(),
				"main_media_url": types.NewKeywordProperty(),
				"price":          types.NewDoubleNumberProperty(),
				"quantity":       types.NewIntegerNumberProperty(),
			},
		},
	}).Do(ctx); err != nil {
		return fmt.Errorf("create index %q: %w", index, err)
	}
	u.log.Info("elasticsearch index created", zap.String("index", index))
	return nil
}

// IndexProduct 按 ID 写入/覆盖一篇文档。
//
// Refresh(wait_for) 让调用返回时文档已经可被搜到。默认是异步刷新(最长 1s),
// 「写完立刻搜」会搜不到,在示例和测试里这个行为很难自证。生产的批量导入
// 不要这么用 —— 每篇都等一次刷新会把吞吐拖垮,那种场景该走 Bulk。
func (u searchRepo) IndexProduct(ctx context.Context, index string, p biz.Product) error {
	if _, err := u.data.es.Index(index).
		Id(strconv.FormatUint(uint64(p.ID), 10)).
		Request(p).
		Refresh(refresh.Waitfor).
		Do(ctx); err != nil {
		return fmt.Errorf("index product %d into %q: %w", p.ID, index, err)
	}
	return nil
}

func (u searchRepo) Search(ctx context.Context, req biz.SearchRequest) (*biz.SearchResponse, error) {
	// 与 EnsureIndex 的 mapping 保持一致。多字段检索按需扩展,
	// 嵌套对象写成 "skus.attributes.*" 这样的通配也可以 —— 匹配不到字段
	// 不会报错,只是不参与打分。
	searchFields := []string{
		"name",
		"spu_code",
	}

	res, err := u.data.es.Search().Index(req.Index).Request(&search.Request{
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
		// Score_ 是指针,且确实会是 nil:带 sort 的查询、或 track_scores=false 时
		// ES 根本不算分。裸解引用在那些查询下会 panic。
		var score types.Float64
		if hit.Score_ != nil {
			score = *hit.Score_
		}
		u.log.Info(fmt.Sprintf("文档ID: %v, 评分: %f", hit.Id_, score))
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
