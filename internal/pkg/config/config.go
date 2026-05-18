package config

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/hashicorp/consul/api"
	"github.com/lens077/go-connect-template/constants"
	confv1 "github.com/lens077/go-connect-template/internal/conf/v1"
	"github.com/lens077/go-connect-template/internal/pkg/env"
	"github.com/mitchellh/mapstructure"
	"github.com/spf13/viper"
	"go.uber.org/fx"
	"google.golang.org/protobuf/types/known/durationpb"
)

var (
	confMu sync.RWMutex
	conf   = &confv1.Bootstrap{}

	Module = fx.Module("config",
		fx.Provide(
			func(lc fx.Lifecycle) (*confv1.Bootstrap, error) {
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

func decodeConfig(data map[string]any, target any) error {
	v := viper.New()
	v.SetConfigType(constants.ConsulFileFormat)
	for k, val := range data {
		v.Set(k, val)
	}

	stringToProtoDurationHook := func(f reflect.Type, t reflect.Type, data any) (any, error) {
		if f.Kind() != reflect.String {
			return data, nil
		}
		if t != reflect.TypeOf(&durationpb.Duration{}) {
			return data, nil
		}

		d, err := time.ParseDuration(data.(string))
		if err != nil {
			return nil, err
		}
		return durationpb.New(d), nil
	}

	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		TagName: "json",
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			stringToProtoDurationHook,
		),
		Result: target,
	})
	if err != nil {
		return err
	}
	return decoder.Decode(v.AllSettings())
}

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

	rawConfig, err := GetConfigFromConsul(consulClient, path)
	if err != nil {
		return nil, err
	}

	localConf := &confv1.Bootstrap{}
	if err := decodeConfig(rawConfig, localConf); err != nil {
		return nil, err
	}

	conf = localConf
	return localConf, nil
}

func GetConfig() *confv1.Bootstrap {
	confMu.RLock()
	defer confMu.RUnlock()
	return conf
}
