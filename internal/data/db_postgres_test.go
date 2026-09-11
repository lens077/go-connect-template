package data

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lens077/go-connect-template/constants"
	conf "github.com/lens077/go-connect-template/internal/conf/v1"
	"go.uber.org/fx/fxtest"
	"go.uber.org/zap"
)

// 不连库:NewPostgresPool 会真的 Ping,成功路径由启动健康检查和 CI 的 compose 覆盖。
// 这里守的是「连之前」就能判定的逻辑:缺配置、TLS 装配。

func TestNewPostgresPool_RejectsMissingConfig(t *testing.T) {
	_, err := NewPostgresPool(fxtest.NewLifecycle(t), &conf.Bootstrap{}, zap.NewNop())
	if err == nil || !strings.Contains(err.Error(), "data.database.postgres is not configured") {
		t.Fatalf("want missing-config error, got %v", err)
	}
}

// selfSignedCA 生成一份测试用 CA PEM。
func selfSignedCA(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func TestApplyPostgresTLS(t *testing.T) {
	ca := selfSignedCA(t)
	base := func() *pgxpool.Config {
		c, _ := pgxpool.ParseConfig("")
		return c
	}

	t.Run("no tls / no ca: leaves pgx defaults", func(t *testing.T) {
		c := base()
		if err := applyPostgresTLS(c, &conf.Data_Database_Postgres{}, zap.NewNop()); err != nil || c.ConnConfig.TLSConfig != nil {
			t.Fatalf("expected untouched, got tls=%v err=%v", c.ConnConfig.TLSConfig, err)
		}
	})

	t.Run("garbage ca_pem fails loudly", func(t *testing.T) {
		err := applyPostgresTLS(base(), &conf.Data_Database_Postgres{Tls: &conf.Data_Database_Postgres_Tls{CaPem: "nope", SslMode: constants.SslModeVerifyFull}}, zap.NewNop())
		if err == nil || !strings.Contains(err.Error(), "parse CA PEM") {
			t.Fatalf("want PEM error, got %v", err)
		}
	})

	t.Run("verify-full: RootCAs + ServerName, builtin verification ON", func(t *testing.T) {
		c := base()
		err := applyPostgresTLS(c, &conf.Data_Database_Postgres{Host: "db.example", Tls: &conf.Data_Database_Postgres_Tls{CaPem: ca, SslMode: constants.SslModeVerifyFull}}, zap.NewNop())
		if err != nil {
			t.Fatal(err)
		}
		tc := c.ConnConfig.TLSConfig
		if tc == nil || tc.RootCAs == nil || tc.InsecureSkipVerify || tc.ServerName != "db.example" {
			t.Fatalf("verify-full misconfigured: %+v", tc)
		}
	})

	t.Run("verify-ca: custom chain verification, hostname NOT checked", func(t *testing.T) {
		// 这里 InsecureSkipVerify=true 是刻意的:Go 没有「验链不验主机名」的开关,
		// 必须关掉内置校验、在 VerifyPeerCertificate 里自己验链。
		// 把它「修正」成 false 会让 verify-ca 退化成 verify-full,走 IP 直连的库全部握手失败。
		c := base()
		err := applyPostgresTLS(c, &conf.Data_Database_Postgres{Tls: &conf.Data_Database_Postgres_Tls{CaPem: ca, SslMode: constants.SslModeVerifyCa}}, zap.NewNop())
		if err != nil {
			t.Fatal(err)
		}
		tc := c.ConnConfig.TLSConfig
		if tc == nil || !tc.InsecureSkipVerify || tc.VerifyPeerCertificate == nil || tc.RootCAs == nil {
			t.Fatalf("verify-ca must skip builtin verification and install VerifyPeerCertificate: %+v", tc)
		}
		// 自定义校验函数对「服务端没给证书」要报错,不能静默放行
		if err := tc.VerifyPeerCertificate(nil, nil); err == nil {
			t.Fatal("VerifyPeerCertificate must reject empty chain")
		}
	})
}

func TestPingTimeoutOr(t *testing.T) {
	if got := pingTimeoutOr(0); got != constants.DefaultDBPingTimeout {
		t.Fatalf("zero must fall back to default, got %v", got)
	}
	if got := pingTimeoutOr(3 * time.Second); got != 3*time.Second {
		t.Fatalf("explicit value must be kept, got %v", got)
	}
}
