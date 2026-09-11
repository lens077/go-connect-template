package data

import (
	"strings"
	"testing"

	conf "github.com/lens077/go-connect-template/internal/conf/v1"
	"go.uber.org/zap"
)

func TestNewCasdoorAuthClient_RejectsMissingConfig(t *testing.T) {
	_, err := NewCasdoorAuthClient(&conf.Bootstrap{}, zap.NewNop())
	if err == nil || !strings.Contains(err.Error(), "auth.casdoor is not configured") {
		t.Fatalf("want missing-config error, got %v", err)
	}
}

// casdoorsdk.NewClient 的参数是六个连续的 string,顺序错了编译照样通过。
// 这里按字段名逐个核对,守住「endpoint 和 client_id 被调换」这类静默错误。
func TestNewCasdoorAuthClient_MapsEveryField(t *testing.T) {
	cfg := &conf.Bootstrap{Auth: &conf.Auth{Casdoor: &conf.Auth_Casdoor{
		Endpoint:         "https://iam.example",
		ClientId:         "cid",
		ClientSecret:     "csecret",
		Certificate:      "-----BEGIN CERTIFICATE-----\nx\n-----END CERTIFICATE-----",
		OrganizationName: "org",
		ApplicationName:  "app",
	}}}
	c, err := NewCasdoorAuthClient(cfg, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	src := cfg.Auth.Casdoor
	if c.Endpoint != src.Endpoint || c.ClientId != src.ClientId || c.ClientSecret != src.ClientSecret ||
		c.Certificate != src.Certificate || c.OrganizationName != src.OrganizationName || c.ApplicationName != src.ApplicationName {
		t.Fatalf("field mapping mismatch:\n want %v\n got  %+v", src, c.AuthConfig)
	}
}
