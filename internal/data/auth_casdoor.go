package data

import (
	"fmt"

	"github.com/casdoor/casdoor-go-sdk/casdoorsdk"
	conf "github.com/lens077/go-connect-template/internal/conf/v1"
	"go.uber.org/zap"
)

// NewCasdoorAuthClient 构造 Casdoor 客户端。
//
// 这里不做任何网络探活:casdoorsdk.NewClient 只是装配结构体,
// 而 Casdoor 的接口都要带凭据调,启动阶段去 ping 反而会让服务
// 在 IAM 短暂不可用时起不来。凭据是否正确留给第一次鉴权时暴露。
func NewCasdoorAuthClient(cfg *conf.Bootstrap, logger *zap.Logger) (*casdoorsdk.Client, error) {
	casdoorCfg := cfg.GetAuth().GetCasdoor()
	if casdoorCfg == nil {
		return nil, fmt.Errorf("auth.casdoor is not configured")
	}

	client := casdoorsdk.NewClient(
		casdoorCfg.Endpoint,         // endpoint
		casdoorCfg.ClientId,         // clientId
		casdoorCfg.ClientSecret,     // clientSecret
		casdoorCfg.Certificate,      // certificate (x509 format)
		casdoorCfg.OrganizationName, // organizationName
		casdoorCfg.ApplicationName,  // applicationName
	)

	logger.Info("casdoor client initialized", zap.String("endpoint", casdoorCfg.Endpoint))

	return client, nil
}

// Auth 返回底层客户端,供仓储层直接使用
func (d *Data) Auth() *casdoorsdk.Client {
	return d.auth
}
