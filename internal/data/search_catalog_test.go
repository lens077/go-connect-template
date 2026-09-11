package data

import (
	"strings"
	"testing"

	conf "github.com/lens077/go-connect-template/internal/conf/v1"
	"go.uber.org/fx/fxtest"
	"go.uber.org/zap"
)

// 共享 seam 的入口校验:不管选了哪个 adapter,这三条都要在 adapter 构造之前拦住。
func TestNewSearchCatalog_RejectsIncompleteConfig(t *testing.T) {
	cases := map[string]struct {
		cfg  *conf.Bootstrap
		want string
	}{
		"missing catalog": {&conf.Bootstrap{}, "search.catalog is not configured"},
		"empty endpoint":  {&conf.Bootstrap{Search: &conf.Search{Catalog: &conf.Search_Catalog{Index: "p"}}}, "endpoint is empty"},
		"empty index":     {&conf.Bootstrap{Search: &conf.Search{Catalog: &conf.Search_Catalog{Endpoint: "http://x"}}}, "index is empty"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := NewSearchCatalog(fxtest.NewLifecycle(t), tc.cfg, zap.NewNop())
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestNewSearchTransport_TLS(t *testing.T) {
	t.Run("disabled: default tls untouched", func(t *testing.T) {
		// DefaultTransport.Clone() 自带一份 TLSClientConfig(h2 ALPN 等),
		// 「禁用」的语义是不动它:不放松校验、不换 CA。
		tr, err := newSearchTransport(&conf.Search_Catalog{})
		if err != nil {
			t.Fatal(err)
		}
		if c := tr.TLSClientConfig; c != nil && (c.InsecureSkipVerify || c.RootCAs != nil) {
			t.Fatalf("tls disabled must not alter verification, got InsecureSkipVerify=%v RootCAs=%v", c.InsecureSkipVerify, c.RootCAs != nil)
		}
	})
	t.Run("enabled with insecure_skip_verify", func(t *testing.T) {
		tr, err := newSearchTransport(&conf.Search_Catalog{Tls: &conf.Search_Catalog_Tls{Enable: true, InsecureSkipVerify: true}})
		if err != nil || tr.TLSClientConfig == nil || !tr.TLSClientConfig.InsecureSkipVerify {
			t.Fatalf("insecure_skip_verify must propagate, got %+v err=%v", tr.TLSClientConfig, err)
		}
	})
	t.Run("invalid ca_pem is an error, not silently ignored", func(t *testing.T) {
		_, err := newSearchTransport(&conf.Search_Catalog{Tls: &conf.Search_Catalog_Tls{Enable: true, CaPem: "not a pem"}})
		if err == nil || !strings.Contains(err.Error(), "invalid PEM") {
			t.Fatalf("garbage ca_pem must fail loudly, got %v", err)
		}
	})
}
