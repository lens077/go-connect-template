package config

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/lens077/go-connect-template/constants"
	confv1 "github.com/lens077/go-connect-template/internal/conf/v1"
	"github.com/mitchellh/mapstructure"
	"github.com/spf13/viper"
	"go.uber.org/fx"
	"google.golang.org/protobuf/types/known/durationpb"
)

var (
	confMu sync.RWMutex
	conf   = &confv1.Bootstrap{}
	// srcName 本次启动实际生效的数据源名,供启动日志打印(排查「读的到底是哪份配置」)
	srcName string

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
	v.SetConfigType(constants.ConfigFileFormat)
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

// Init 拉取并解析整份 Bootstrap 配置。
//
// 「从哪儿拉」由 CONFIG_SOURCE 决定(见 source.go),本函数只负责选源、解码、落盘,
// 不感知任何数据源细节 —— 引导参数各自由对应 source 从环境变量读取,其余全部配置
// (含 Consul 服务发现地址、DB、Redis 等)一律由选中的数据源下发。
func Init(ctx context.Context) (*confv1.Bootstrap, error) {
	src, err := NewSource()
	if err != nil {
		return nil, err
	}

	rawConfig, err := src.Load(ctx)
	if err != nil {
		return nil, err
	}

	localConf := &confv1.Bootstrap{}
	if err := decodeConfig(rawConfig, localConf); err != nil {
		return nil, fmt.Errorf("decode bootstrap from %s: %w", src.Name(), err)
	}

	confMu.Lock()
	conf = localConf
	srcName = src.Name()
	confMu.Unlock()

	return localConf, nil
}

func GetConfig() *confv1.Bootstrap {
	confMu.RLock()
	defer confMu.RUnlock()
	return conf
}

// SourceName 返回本次启动实际生效的配置数据源名;Init 之前为空。
func SourceName() string {
	confMu.RLock()
	defer confMu.RUnlock()
	return srcName
}
