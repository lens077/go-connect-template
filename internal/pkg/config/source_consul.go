package config

import (
	"context"
	"fmt"

	"github.com/hashicorp/consul/api"
	"github.com/lens077/go-connect-template/constants"
	"github.com/lens077/go-connect-template/internal/pkg/env"
)

var _ Source = (*consulSource)(nil)

// consulSource 从 Consul KV 读整份 Bootstrap:CONSUL_PATH 指向的那一个 key 存着完整 YAML。
//
// 这里的 Consul 客户端只用于读配置,与 internal/pkg/registry 里做服务注册/发现的
// 客户端是两个独立实例 —— 换用别的配置数据源时,服务发现那套完全不受影响。
type consulSource struct {
	client *api.Client
	addr   string
	path   string
}

// NewConsulSource 由 CONSUL_* 环境变量构造:
// CONSUL_ADDR / CONSUL_PATH / CONSUL_SCHEME / CONSUL_TOKEN,
// scheme=https 时另看 CONSUL_INSECURE_SKIP_VERIFY 与 CONSUL_CA_FILE/CERT_FILE/KEY_FILE。
func NewConsulSource() (Source, error) {
	path := env.GetEnvString(constants.EnvConsulPath, constants.ConsulPath)
	if path == "" {
		return nil, fmt.Errorf("required env %s is missing", constants.EnvConsulPath)
	}

	addr := env.GetEnvString(constants.EnvConsulAddr, constants.ConsulAddr)

	cfg := api.DefaultConfig()
	cfg.Address = addr
	cfg.Token = env.GetEnvString(constants.EnvConsulToken, constants.ConsulToken)
	cfg.Scheme = env.GetEnvString(constants.EnvConsulScheme, constants.ConsulScheme)

	if cfg.Scheme == constants.ConsulTlsScheme {
		if env.GetEnvBool(constants.EnvConsulInsecureSkipVerify, constants.ConsulInsecureSkipVerify) {
			cfg.TLSConfig.InsecureSkipVerify = true
		} else {
			cfg.TLSConfig = api.TLSConfig{
				CAFile:   env.GetEnvString(constants.EnvConsulCaFile, ""),
				CertFile: env.GetEnvString(constants.EnvConsulCertFile, ""),
				KeyFile:  env.GetEnvString(constants.EnvConsulKeyFile, ""),
			}
		}
	}

	client, err := api.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("initialize consul client failed: %w", err)
	}

	return &consulSource{client: client, addr: addr, path: path}, nil
}

func (s *consulSource) Name() string { return constants.ConfigSourceConsul }

func (s *consulSource) Load(ctx context.Context) (map[string]any, error) {
	pair, _, err := s.client.KV().Get(s.path, (&api.QueryOptions{}).WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("consul kv get failed (%s @ %s): %w", s.path, s.addr, err)
	}
	if pair == nil || len(pair.Value) == 0 {
		return nil, fmt.Errorf("consul kv path is empty: %s @ %s", s.path, s.addr)
	}
	return parseYAMLToMap(pair.Value)
}
