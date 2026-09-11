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

// fakeMeili 是最小的 Meilisearch HTTP 假服务。Meilisearch 的写操作都是异步任务,
// 所以假服务要模拟「返回 taskUid → 客户端轮询 /tasks/{uid} → succeeded」这条链。
type fakeMeili struct {
	mu       sync.Mutex
	indices  map[string]bool
	docs     map[string][]json.RawMessage
	tasks    map[int64]string // uid -> status
	nextTask int64
	auth     string
	failNext bool // 下一个任务以 failed 结束,验 adapter 不会把失败当成功
}

func newFakeMeili() *fakeMeili {
	return &fakeMeili{indices: map[string]bool{}, docs: map[string][]json.RawMessage{}, tasks: map[int64]string{}}
}

func (f *fakeMeili) enqueue(status string) int64 {
	f.nextTask++
	if f.failNext {
		status = "failed"
		f.failNext = false
	}
	f.tasks[f.nextTask] = status
	return f.nextTask
}

func (f *fakeMeili) handler(t *testing.T) http.Handler {
	writeJSON := func(w http.ResponseWriter, code int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(v)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if a := r.Header.Get("Authorization"); a != "" {
			f.auth = a
		}
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/health":
			writeJSON(w, 200, map[string]string{"status": "available"})

		case r.Method == http.MethodGet && len(parts) == 2 && parts[0] == "indexes": // GET /indexes/{uid}
			if !f.indices[parts[1]] {
				writeJSON(w, 404, map[string]string{"message": "Index `" + parts[1] + "` not found.", "code": "index_not_found", "type": "invalid_request", "link": "https://docs.meilisearch.com/errors#index_not_found"})
				return
			}
			writeJSON(w, 200, map[string]any{"uid": parts[1], "primaryKey": "id"})

		case r.Method == http.MethodPost && r.URL.Path == "/indexes": // POST /indexes
			var req struct {
				Uid        string `json:"uid"`
				PrimaryKey string `json:"primaryKey"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req.PrimaryKey != "id" {
				t.Errorf("index must be created with primaryKey=id, got %q", req.PrimaryKey)
			}
			uid := f.enqueue("succeeded")
			if f.tasks[uid] == "succeeded" {
				f.indices[req.Uid] = true
			}
			writeJSON(w, 202, map[string]any{"taskUid": uid, "indexUid": req.Uid, "status": "enqueued", "type": "indexCreation", "enqueuedAt": "2026-01-01T00:00:00Z"})

		case r.Method == http.MethodPost && len(parts) == 3 && parts[2] == "documents": // POST /indexes/{uid}/documents
			var batch []json.RawMessage
			_ = json.NewDecoder(r.Body).Decode(&batch)
			uid := f.enqueue("succeeded")
			if f.tasks[uid] == "succeeded" {
				f.docs[parts[1]] = append(f.docs[parts[1]], batch...)
			}
			writeJSON(w, 202, map[string]any{"taskUid": uid, "indexUid": parts[1], "status": "enqueued", "type": "documentAdditionOrUpdate", "enqueuedAt": "2026-01-01T00:00:00Z"})

		case r.Method == http.MethodGet && len(parts) == 2 && parts[0] == "tasks": // GET /tasks/{uid}
			uid, _ := strconv.ParseInt(parts[1], 10, 64)
			status, ok := f.tasks[uid]
			if !ok {
				writeJSON(w, 404, map[string]string{"message": "task not found", "code": "task_not_found", "type": "invalid_request", "link": ""})
				return
			}
			resp := map[string]any{"uid": uid, "status": status, "type": "indexCreation", "enqueuedAt": "2026-01-01T00:00:00Z"}
			if status == "failed" {
				resp["error"] = map[string]string{"message": "simulated failure", "code": "internal", "type": "internal", "link": ""}
			}
			writeJSON(w, 200, resp)

		case r.Method == http.MethodPost && len(parts) == 3 && parts[2] == "search": // POST /indexes/{uid}/search
			var req struct {
				Q string `json:"q"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			hits := []json.RawMessage{}
			for _, d := range f.docs[parts[1]] {
				if strings.Contains(strings.ToLower(string(d)), strings.ToLower(req.Q)) {
					hits = append(hits, d)
				}
			}
			writeJSON(w, 200, map[string]any{"hits": hits, "query": req.Q, "processingTimeMs": 1, "limit": 20, "offset": 0, "estimatedTotalHits": len(hits)})

		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	})
}

func meiliBootstrap(endpoint, apiKey string) *conf.Bootstrap {
	return &conf.Bootstrap{Search: &conf.Search{Catalog: &conf.Search_Catalog{Endpoint: endpoint, Index: "products", ApiKey: apiKey}}}
}

func TestMeilisearchCatalog_Contract(t *testing.T) {
	f := newFakeMeili()
	srv := httptest.NewServer(f.handler(t))
	defer srv.Close()

	cat, err := newMeilisearchCatalog(meiliBootstrap(srv.URL, ""), zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	ctx := context.Background()

	// Health 契约与 ES adapter 一致:服务可达 且 默认索引就绪
	if err := cat.Health(ctx); err == nil {
		t.Fatal("Health on empty instance must fail: default index absent")
	}
	if f.auth != "" {
		t.Fatalf("development mode without api_key must not send Authorization, got %q", f.auth)
	}

	// EnsureIndex:404 -> 创建(primaryKey=id)并等任务;再调一次 200 -> 跳过
	if err := cat.EnsureIndex(ctx, ""); err != nil {
		t.Fatalf("EnsureIndex #1: %v", err)
	}
	if !f.indices["products"] {
		t.Fatal("EnsureIndex must POST /indexes when GET returns 404")
	}
	if err := cat.EnsureIndex(ctx, ""); err != nil {
		t.Fatalf("EnsureIndex #2 (idempotent): %v", err)
	}
	if err := cat.Health(ctx); err != nil {
		t.Fatalf("Health after index exists: %v", err)
	}

	// IndexProduct 等异步任务 succeeded 后返回;文档进入 index
	p := CatalogProduct{ID: 7, Name: "running shoe", SpuCode: "SHOE-7", Price: 99.5, Status: "online", Quantity: 3}
	if err := cat.IndexProduct(ctx, "", p); err != nil {
		t.Fatalf("IndexProduct: %v", err)
	}
	if len(f.docs["products"]) != 1 {
		t.Fatalf("want 1 doc stored, got %d", len(f.docs["products"]))
	}

	// SearchProducts:hits 解码回项目自有 DTO,字段完整
	got, err := cat.SearchProducts(ctx, "", "shoe")
	if err != nil {
		t.Fatalf("SearchProducts: %v", err)
	}
	if len(got) != 1 || got[0] != p {
		t.Fatalf("round-trip mismatch:\n want %+v\n got  %+v", p, got)
	}
}

func TestMeilisearchCatalog_APIKeySentAsBearer(t *testing.T) {
	f := newFakeMeili()
	srv := httptest.NewServer(f.handler(t))
	defer srv.Close()

	cat, err := newMeilisearchCatalog(meiliBootstrap(srv.URL, "master-key"), zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	_ = cat.Health(context.Background())

	if f.auth != "Bearer master-key" {
		t.Fatalf("api_key must be sent as Bearer token, got %q", f.auth)
	}
}

func TestMeilisearchCatalog_FailedTaskIsAnError(t *testing.T) {
	f := newFakeMeili()
	srv := httptest.NewServer(f.handler(t))
	defer srv.Close()

	cat, err := newMeilisearchCatalog(meiliBootstrap(srv.URL, ""), zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()

	// 异步任务以 failed 结束时,adapter 必须报错——静默返回 nil 会让调用方以为写入成功
	f.mu.Lock()
	f.failNext = true
	f.mu.Unlock()
	err = cat.EnsureIndex(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), `status "failed"`) {
		t.Fatalf("failed task must surface as error, got %v", err)
	}
}
