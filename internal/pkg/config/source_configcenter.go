package config

import (
	"context"
	"fmt"
	"net/http"

	"connectrpc.com/connect"
	configv1 "github.com/lens077/go-connect-template/api/config/v1"
	"github.com/lens077/go-connect-template/api/config/v1/configv1connect"
	"github.com/lens077/go-connect-template/constants"
	"github.com/lens077/go-connect-template/internal/pkg/env"
)

var _ Source = (*configCenterSource)(nil)

// configCenterSource 经 ConnectRPC 从 config-service 拉整份 Bootstrap。
// 配置以「一个 key 一份 YAML」的粒度存储,由 namespace/environment/key 三元组定位。
//
// 客户端代码来自 api/config/v1(config-service 的契约副本)。这份 proto 只描述
// 配置中心的读写接口,与本服务的业务无关,所以它跟着本数据源一起启用/一起裁掉。
type configCenterSource struct {
	client      configv1connect.ConfigServiceClient
	addr        string
	namespace   string
	environment string
	key         string
}

// NewConfigCenterSource 由 CONFIG_CENTER_* 环境变量构造:
// CONFIG_CENTER_ADDR / CONFIG_CENTER_NAMESPACE / CONFIG_CENTER_ENV / CONFIG_CENTER_KEY。
//
// namespace 与 environment 强制必填,不给默认值:猜错了不会报错,只会静默读到
// 另一个环境的配置或空配置,比启动失败难查得多。
// 服务对服务是集群内直连 config-service(不过网关),因此不需要 JWT。
func NewConfigCenterSource() (Source, error) {
	addr := env.GetEnvString(constants.EnvConfigCenterAddr, constants.ConfigCenterAddr)
	namespace := env.GetEnvString(constants.EnvConfigCenterNamespace, "")
	environment := env.GetEnvString(constants.EnvConfigCenterEnv, "")
	key := env.GetEnvString(constants.EnvConfigCenterKey, constants.ConfigCenterKey)

	if namespace == "" || environment == "" {
		return nil, fmt.Errorf("required env %s and %s must be set when %s=%s",
			constants.EnvConfigCenterNamespace, constants.EnvConfigCenterEnv,
			constants.EnvConfigSource, constants.ConfigSourceConfigCenter)
	}

	return &configCenterSource{
		client:      configv1connect.NewConfigServiceClient(http.DefaultClient, addr),
		addr:        addr,
		namespace:   namespace,
		environment: environment,
		key:         key,
	}, nil
}

func (s *configCenterSource) Name() string { return constants.ConfigSourceConfigCenter }

func (s *configCenterSource) Load(ctx context.Context) (map[string]any, error) {
	resp, err := s.client.GetKey(ctx, connect.NewRequest(&configv1.GetKeyRequest{
		Namespace:   s.namespace,
		Environment: s.environment,
		Key:         s.key,
	}))
	if err != nil {
		return nil, fmt.Errorf("config center get key failed (%s/%s/%s @ %s): %w",
			s.namespace, s.environment, s.key, s.addr, err)
	}

	entry := resp.Msg.GetEntry()
	if entry == nil || entry.GetValue() == "" {
		// 空值当失败处理:让服务带着一份空配置起来,等于把问题推迟到第一次
		// 读某个字段时才炸,那时候已经离现场很远了
		return nil, fmt.Errorf("config center key is empty: %s/%s/%s @ %s",
			s.namespace, s.environment, s.key, s.addr)
	}

	return parseYAMLToMap([]byte(entry.GetValue()))
}
