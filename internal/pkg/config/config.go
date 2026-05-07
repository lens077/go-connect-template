package config

import (
	"context"
	"fmt"
	"sync"

	"github.com/hashicorp/consul/api"
	"github.com/lens077/go-connect-template/constants"
	confv1 "github.com/lens077/go-connect-template/internal/conf/v1"
	"github.com/lens077/go-connect-template/internal/pkg/env"
	"github.com/mitchellh/mapstructure"
	"github.com/spf13/viper"
	"go.uber.org/fx"
)

var (
	confMu sync.RWMutex
	conf   = &confv1.Bootstrap{}

	Module = fx.Module("config",
		fx.Provide(
			func(lc fx.Lifecycle) (*confv1.Bootstrap, error) {
				// 创建一个可以取消的上下文，用于优雅关闭 Watch 协程
				ctx, cancel := context.WithCancel(context.Background())

				lc.Append(fx.Hook{
					OnStop: func(ctx context.Context) error {
						cancel()
						return nil
					},
				})

				bootstrap, err := Init(ctx)
				if err != nil {
					return nil, err
				}

				return bootstrap, nil
			},
		),
	)
)

// decodeConfig 将 Map 解析为结构体
func decodeConfig(data map[string]any, target any) error {
	v := viper.New()
	v.SetConfigType(constants.ConsulFileFormat)
	for k, val := range data {
		v.Set(k, val)
	}

	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		TagName: "json", // Protobuf 生成的结构体使用 json tag
		Result:  target,
	})
	if err != nil {
		return err
	}
	return decoder.Decode(v.AllSettings())
}

// updateConfig 线程安全地更新全局配置
func updateConfig(newConfig map[string]any) {
	newBootstrap := &confv1.Bootstrap{}
	if err := decodeConfig(newConfig, newBootstrap); err != nil {
		return
	}

	confMu.Lock()
	conf = newBootstrap
	confMu.Unlock()
}

// Init 初始化配置加载
func Init(ctx context.Context) (*confv1.Bootstrap, error) {
	addr := env.GetEnvString(constants.EnvConsulAddr, constants.ConsulAddr)
	path := env.GetEnvString(constants.EnvConsulPath, constants.ConsulPath)
	if path == "" {
		return nil, fmt.Errorf("required env %s is missing", constants.EnvConsulPath)
	}

	consulCfg := api.DefaultConfig()
	consulCfg.Address = addr
	consulCfg.Token = env.GetEnvString(constants.EnvConsulToken, constants.ConsulToken)
	consulCfg.Scheme = env.GetEnvString(constants.EnvConsulScheme, constants.ConsulScheme)

	if consulCfg.Scheme == "https" {
		if env.GetEnvBool(constants.EnvConsulInsecureSkipVerify, constants.ConsulInsecureSkipVerify) {
			consulCfg.TLSConfig.InsecureSkipVerify = true
		} else {
			consulCfg.TLSConfig = api.TLSConfig{
				CAFile:   env.GetEnvString(constants.EnvConsulCaFile, ""),
				CertFile: env.GetEnvString(constants.EnvConsulCertFile, ""),
				KeyFile:  env.GetEnvString(constants.EnvConsulKeyFile, ""),
			}
		}
	}

	consulClient, err := api.NewClient(consulCfg)
	if err != nil {
		return nil, fmt.Errorf("initialize consul client failed: %v", err)
	}

	// 2. 首次同步拉取配置
	rawConfig, err := GetConfigFromConsul(consulClient, path)
	if err != nil {
		return nil, err
	}

	localConf := &confv1.Bootstrap{}
	if err := decodeConfig(rawConfig, localConf); err != nil {
		return nil, err
	}

	// 初始化全局变量
	conf = localConf

	// 启动后台监听 (集成重试与 Context)
	WatchConsulConfig(ctx, consulClient, path, updateConfig)

	return localConf, nil
}

// GetConfig 线程安全地获取当前配置
func GetConfig() *confv1.Bootstrap {
	confMu.RLock()
	defer confMu.RUnlock()
	return conf
}
