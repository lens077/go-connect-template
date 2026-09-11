package data

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	conf "github.com/lens077/go-connect-template/internal/conf/v1"
	"go.uber.org/zap"
)

// fakeES 是最小的 Elasticsearch HTTP 假服务,只实现 adapter 用到的几个端点。
// 验的是 adapter 与 SDK 之间的契约(路径、方法、请求体、响应解码),不验 ES 本身。
type fakeES struct {
	mu      sync.Mutex
	indices map[string]bool
	docs    map[string]map[string]json.RawMessage // index -> id -> source
	auth    string                                // 记录收到的 Authorization 头
}

func newFakeES() *fakeES {
	return &fakeES{indices: map[string]bool{}, docs: map[string]map[string]json.RawMessage{}}
}

func (f *fakeES) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		// v9 客户端会校验这个头,缺了就拒绝把对方当 Elasticsearch
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		w.Header().Set("Content-Type", "application/json")
		if a := r.Header.Get("Authorization"); a != "" {
			f.auth = a
		}

		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/":
			_, _ = w.Write([]byte(`{"version":{"number":"9.0.0"},"tagline":"You Know, for Search"}`))

		case r.Method == http.MethodHead && len(parts) == 1: // HEAD /{index}
			if f.indices[parts[0]] {
				w.WriteHeader(http.StatusOK)
			} else {
				w.WriteHeader(http.StatusNotFound)
			}

		case r.Method == http.MethodPut && len(parts) == 1: // PUT /{index}
			f.indices[parts[0]] = true
			f.docs[parts[0]] = map[string]json.RawMessage{}
			_, _ = w.Write([]byte(`{"acknowledged":true,"shards_acknowledged":true,"index":"` + parts[0] + `"}`))

		case r.Method == http.MethodPut && len(parts) == 3 && parts[1] == "_doc": // PUT /{index}/_doc/{id}
			var src json.RawMessage
			if err := json.NewDecoder(r.Body).Decode(&src); err != nil {
				t.Errorf("bad index body: %v", err)
			}
			if f.docs[parts[0]] == nil {
				f.docs[parts[0]] = map[string]json.RawMessage{}
			}
			f.docs[parts[0]][parts[2]] = src
			_, _ = w.Write([]byte(`{"_index":"` + parts[0] + `","_id":"` + parts[2] + `","_version":1,"result":"created","_shards":{"total":1,"successful":1,"failed":0},"_seq_no":0,"_primary_term":1}`))

		case r.Method == http.MethodPost && len(parts) == 2 && parts[1] == "_search": // POST /{index}/_search
			var req struct {
				Query struct {
					MultiMatch struct {
						Query  string   `json:"query"`
						Fields []string `json:"fields"`
					} `json:"multi_match"`
				} `json:"query"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("bad search body: %v", err)
			}
			hits := []string{}
			for id, src := range f.docs[parts[0]] {
				if strings.Contains(strings.ToLower(string(src)), strings.ToLower(req.Query.MultiMatch.Query)) {
					hits = append(hits, `{"_index":"`+parts[0]+`","_id":"`+id+`","_score":1.0,"_source":`+string(src)+`}`)
				}
			}
			_, _ = w.Write([]byte(`{"took":1,"timed_out":false,"_shards":{"total":1,"successful":1,"skipped":0,"failed":0},"hits":{"total":{"value":` +
				strconv.Itoa(len(hits)) + `,"relation":"eq"},"max_score":1.0,"hits":[` + strings.Join(hits, ",") + `]}}`))

		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

func esBootstrap(endpoint string, mutate func(*conf.Search_Catalog)) *conf.Bootstrap {
	c := &conf.Search_Catalog{Endpoint: endpoint, Index: "products"}
	if mutate != nil {
		mutate(c)
	}
	return &conf.Bootstrap{Search: &conf.Search{Catalog: c}}
}

func TestElasticsearchCatalog_RejectsHalfCredentials(t *testing.T) {
	_, err := newElasticsearchCatalog(esBootstrap("http://127.0.0.1:1", func(c *conf.Search_Catalog) {
		c.Username = "elastic" // 只有用户名没有密码
	}), zap.NewNop())
	if err == nil || !strings.Contains(err.Error(), "username and password must be configured together") {
		t.Fatalf("want half-credential rejection, got %v", err)
	}
}

func TestElasticsearchCatalog_BasicAuthWinsOverAPIKey(t *testing.T) {
	f := newFakeES()
	srv := httptest.NewServer(f.handler(t))
	defer srv.Close()

	cat, err := newElasticsearchCatalog(esBootstrap(srv.URL, func(c *conf.Search_Catalog) {
		c.Username, c.Password, c.ApiKey = "elastic", "pw", "should-be-ignored"
	}), zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	_ = cat.EnsureIndex(context.Background(), "")

	if !strings.HasPrefix(f.auth, "Basic ") {
		t.Fatalf("both credential forms present: Basic Auth must win, got Authorization=%q", f.auth)
	}
}

func TestElasticsearchCatalog_Contract(t *testing.T) {
	f := newFakeES()
	srv := httptest.NewServer(f.handler(t))
	defer srv.Close()

	cat, err := newElasticsearchCatalog(esBootstrap(srv.URL, func(c *conf.Search_Catalog) {
		c.ApiKey = "k"
	}), zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	ctx := context.Background()

	// Health 契约:服务可达 且 默认索引就绪。空集群上必须报 unhealthy。
	if err := cat.Health(ctx); err == nil {
		t.Fatal("Health on empty cluster must fail: default index absent")
	}
	if !strings.HasPrefix(strings.ToLower(f.auth), "apikey ") {
		t.Fatalf("api_key alone must be sent as ApiKey auth, got %q", f.auth)
	}

	// EnsureIndex:404 -> 创建;再调一次 200 -> 跳过(幂等)
	if err := cat.EnsureIndex(ctx, ""); err != nil {
		t.Fatalf("EnsureIndex #1: %v", err)
	}
	if !f.indices["products"] {
		t.Fatal("EnsureIndex must PUT /products when HEAD returns 404")
	}
	if err := cat.EnsureIndex(ctx, ""); err != nil {
		t.Fatalf("EnsureIndex #2 (idempotent): %v", err)
	}
	if err := cat.Health(ctx); err != nil {
		t.Fatalf("Health after index exists: %v", err)
	}

	// IndexProduct:文档 _id 必须是 product.ID,否则同 id 重写会变成两条
	p := CatalogProduct{ID: 42, Name: "running shoe", SpuCode: "SHOE-42", Price: 99.5, Status: "online", Quantity: 3}
	if err := cat.IndexProduct(ctx, "", p); err != nil {
		t.Fatalf("IndexProduct: %v", err)
	}
	if _, ok := f.docs["products"]["42"]; !ok {
		t.Fatalf("document must be stored under _id=42, got ids %v", keys(f.docs["products"]))
	}

	// SearchProducts:_source 解码回项目自有 DTO,字段完整
	got, err := cat.SearchProducts(ctx, "", "shoe")
	if err != nil {
		t.Fatalf("SearchProducts: %v", err)
	}
	if len(got) != 1 || got[0] != p {
		t.Fatalf("round-trip mismatch:\n want %+v\n got  %+v", p, got)
	}

	// 显式 index 参数优先于默认索引
	if err := cat.EnsureIndex(ctx, "other"); err != nil {
		t.Fatal(err)
	}
	if !f.indices["other"] {
		t.Fatal("explicit index name must override default")
	}
}

func keys(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
