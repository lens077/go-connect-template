package otel

import (
	"context"
	"testing"
	"time"

	confv1 "github.com/lens077/go-connect-template/internal/conf/v1"
	"github.com/lens077/go-connect-template/internal/pkg/meta"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// OtelTestSuite 是 Otel 的测试套件
type OtelTestSuite struct {
	suite.Suite
	testLogger  *zap.Logger
	testAppInfo meta.AppInfo
}

func (suite *OtelTestSuite) SetupTest() {
	// 创建测试用的 logger
	var err error
	suite.testLogger, err = zap.NewDevelopment()
	assert.NoError(suite.T(), err)

	// 设置测试用的应用信息
	suite.testAppInfo = meta.AppInfo{
		ID:          "test-service-id",
		Name:        "test-service",
		Host:        "127.0.0.1",
		Environment: "dev",
	}
}

func (suite *OtelTestSuite) TestModuleCreation() {
	// 测试模块创建
	module := Module
	assert.NotNil(suite.T(), module)
	assert.Contains(suite.T(), module.String(), "otel")
}

func (suite *OtelTestSuite) TestNewResource() {
	res, err := newResource(suite.testAppInfo)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), res)

	// Merge 只有在两边 schema URL 一致时才会保留 schema URL。
	// 断言它非空,等于守住「本文件 semconv 版本必须跟 sdk 内部版本对齐」这条约束 ——
	// 哪天升 sdk 升出了版本差,这里会先红,而不是等到线上 resource 属性莫名其妙地少。
	assert.NotEmpty(suite.T(), res.SchemaURL(), "schema URL 为空说明 semconv 版本与 sdk 不一致")

	attrs := map[string]string{}
	for _, kv := range res.Attributes() {
		attrs[string(kv.Key)] = kv.Value.Emit()
	}
	assert.Equal(suite.T(), "test-service", attrs["service.name"])
	assert.Equal(suite.T(), "test-service-id", attrs["service.instance.id"])
	assert.Equal(suite.T(), "dev", attrs["deployment.environment.name"])
	assert.Equal(suite.T(), "go", attrs["process.runtime.name"])
	assert.NotEmpty(suite.T(), attrs["process.runtime.version"])
	// 来自 resource.Default(),证明 Merge 确实生效了
	assert.NotEmpty(suite.T(), attrs["telemetry.sdk.version"])
}

func (suite *OtelTestSuite) TestNewPropagator() {
	prop := newPropagator()
	assert.NotNil(suite.T(), prop)
	// 跨服务链路依赖 traceparent,少了它 trace 会断在服务边界上
	assert.Contains(suite.T(), prop.Fields(), "traceparent")
	assert.Contains(suite.T(), prop.Fields(), "baggage")
}

// TestSampleRatio_UnsetFallsBackToOne 是这组测试里最重要的一条。
// 存量配置里没有 sample_ratio 字段,如果它被当成 0.0,升级后一条 trace 都不会采,
// 而且不报任何错 —— 静默失明。必须回落到 1.0。
func (suite *OtelTestSuite) TestSampleRatio_UnsetFallsBackToOne() {
	assert.Equal(suite.T(), 1.0, sampleRatio(&confv1.Observability_Trace{}, suite.testLogger))
	assert.Equal(suite.T(), 1.0, sampleRatio(nil, suite.testLogger))
}

// 显式配 0.0 必须真的是「不采」,不能和「没配」混为一谈。
func (suite *OtelTestSuite) TestSampleRatio_ExplicitZeroIsHonoured() {
	cfg := &confv1.Observability_Trace{SampleRatio: wrapperspb.Double(0)}
	assert.Equal(suite.T(), 0.0, sampleRatio(cfg, suite.testLogger))
}

func (suite *OtelTestSuite) TestSampleRatio_Clamped() {
	cases := []struct {
		configured float64
		want       float64
	}{
		{-0.5, 0},
		{0.25, 0.25},
		{1, 1},
		{7, 1},
	}
	for _, c := range cases {
		cfg := &confv1.Observability_Trace{SampleRatio: wrapperspb.Double(c.configured)}
		assert.Equal(suite.T(), c.want, sampleRatio(cfg, suite.testLogger), "configured=%v", c.configured)
	}
}

func (suite *OtelTestSuite) TestTLSClientConfig_DisabledReturnsNil() {
	assert.Nil(suite.T(), tlsClientConfig(nil, suite.testLogger))
	assert.Nil(suite.T(), tlsClientConfig(&confv1.Observability_Tls{Enable: false}, suite.testLogger))
}

func (suite *OtelTestSuite) TestTLSClientConfig_SkipVerify() {
	conf := tlsClientConfig(&confv1.Observability_Tls{
		Enable:             true,
		InsecureSkipVerify: true,
	}, suite.testLogger)
	assert.NotNil(suite.T(), conf)
	assert.True(suite.T(), conf.InsecureSkipVerify)
	assert.Nil(suite.T(), conf.RootCAs)
}

// CA 解析失败不能让进程挂掉:可观测性坏了不该拖垮主流程。
func (suite *OtelTestSuite) TestTLSClientConfig_BadCaPemFallsBack() {
	conf := tlsClientConfig(&confv1.Observability_Tls{
		Enable: true,
		CaPem:  "not a pem",
	}, suite.testLogger)
	assert.NotNil(suite.T(), conf)
	assert.Nil(suite.T(), conf.RootCAs, "解析失败应退回系统根证书")
}

// sqlc 生成的语句长这样,第一个词是注释符 `--`。
// otelpgx 自带的 WithTrimSQLInSpanName 取「第一个词」,于是所有查询的 span 名
// 都会变成 "query --" —— 实测踩到过。这组用例守的就是别再退回那个行为。
func (suite *OtelTestSuite) TestSQLSpanName_SqlcHeader() {
	stmt := `-- name: GetCartItems :many
SELECT id,
       merchant_id
FROM cart.cart_item
WHERE user_id = $1`
	assert.Equal(suite.T(), "GetCartItems", SQLSpanName(stmt))
}

func (suite *OtelTestSuite) TestSQLSpanName_DistinctQueriesStayDistinct() {
	a := SQLSpanName("-- name: GetCartItems :many\nSELECT 1")
	b := SQLSpanName("-- name: InsertCartItem :one\nINSERT INTO x VALUES (1)")
	assert.NotEqual(suite.T(), a, b, "不同查询必须得到不同 span 名,否则等于没法区分")
	assert.Equal(suite.T(), "GetCartItems", a)
	assert.Equal(suite.T(), "InsertCartItem", b)
}

// 手写 SQL(没有 sqlc 头)退回取首个关键字,而不是返回注释符或整段语句。
func (suite *OtelTestSuite) TestSQLSpanName_FallbackToKeyword() {
	assert.Equal(suite.T(), "SELECT", SQLSpanName("SELECT 1"))
	assert.Equal(suite.T(), "SELECT", SQLSpanName("\n\n  SELECT id FROM t\n"))
	assert.Equal(suite.T(), "BEGIN", SQLSpanName("BEGIN"))
	// 纯注释 / 空语句不能 panic,也不能返回空串
	assert.Equal(suite.T(), "unknown", SQLSpanName(""))
	assert.Equal(suite.T(), "unknown", SQLSpanName("-- just a comment\n"))
}

// span 名必须是有界的低基数值:不能把整段 SQL 带进去。
func (suite *OtelTestSuite) TestSQLSpanName_IsBounded() {
	stmt := "-- name: GetCartItems :many\n" + "SELECT " + string(make([]byte, 4096))
	name := SQLSpanName(stmt)
	assert.Equal(suite.T(), "GetCartItems", name)
	assert.NotContains(suite.T(), name, "\n")
}

func (suite *OtelTestSuite) TestSetupOTelSDK_DisabledIsNoop() {
	shutdown, err := SetupOTelSDK(context.Background(), suite.testAppInfo,
		&confv1.Observability{Enable: false}, suite.testLogger)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), shutdown)
	assert.NoError(suite.T(), shutdown(context.Background()))
}

func (suite *OtelTestSuite) TestSetupOTelSDK_PanicRecovery() {
	// 测试 panic 恢复
	assert.NotPanics(suite.T(), func() {
		ctx := context.Background()
		minConfig := &confv1.Observability{
			Enable: true,
			Trace: &confv1.Observability_Trace{
				Endpoint:    "localhost:4318",
				Tls:         &confv1.Observability_Tls{Enable: false},
				SampleRatio: wrapperspb.Double(0.5),
			},
			Metric: &confv1.Observability_Metric{
				Endpoint:       "localhost:4318",
				Tls:            &confv1.Observability_Tls{Enable: false},
				ExportInterval: durationpb.New(30 * time.Second),
			},
			Log: &confv1.Observability_Logging{
				Endpoint: "localhost:4318",
				Tls:      &confv1.Observability_Tls{Enable: false},
			},
		}
		shutdown, _ := SetupOTelSDK(ctx, suite.testAppInfo, minConfig, suite.testLogger)
		if shutdown != nil {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_ = shutdown(shutdownCtx)
		}
	})
}

// 运行测试套件
func TestOtelTestSuite(t *testing.T) {
	suite.Run(t, new(OtelTestSuite))
}
