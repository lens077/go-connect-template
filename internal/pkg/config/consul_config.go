package config

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/hashicorp/consul/api"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

// ConfigStore 定义了配置存储的内部状态，防止并发读写冲突
type ConfigStore struct {
	mu       sync.RWMutex
	settings map[string]interface{}
}

// parseYAMLToMap 将字节流解析为配置字典
func parseYAMLToMap(data []byte) (map[string]interface{}, error) {
	v := viper.New()
	v.SetConfigType("yaml")
	if err := v.ReadConfig(bytes.NewBuffer(data)); err != nil {
		return nil, err
	}
	return v.AllSettings(), nil
}

// GetConfigFromConsul 显式拉取配置
func GetConfigFromConsul(client *api.Client, path string) (map[string]interface{}, error) {
	kv := client.KV()
	pair, _, err := kv.Get(path, nil)
	if err != nil {
		return nil, fmt.Errorf("consul kv get failed: %w", err)
	}

	if pair == nil || len(pair.Value) == 0 {
		return nil, fmt.Errorf("config path is empty: %s", path)
	}

	return parseYAMLToMap(pair.Value)
}

// WatchConsulConfig 监听配置变化
// 增加了 context 支持，以便在应用关闭时能优雅退出协程
func WatchConsulConfig(ctx context.Context, client *api.Client, path string, onChange func(map[string]interface{})) {
	go func() {
		kv := client.KV()
		var lastIndex uint64

		// 指数退避重试策略，防止 Consul 宕机时日志刷屏
		backoff := time.Second

		for {
			select {
			case <-ctx.Done():
				return
			default:
				pair, meta, err := kv.Get(path, &api.QueryOptions{
					WaitIndex: lastIndex,
					WaitTime:  10 * time.Minute, // 使用较长的等待时间，减少无效轮询
				})

				if err != nil {
					zap.L().Error("Consul watch error, retrying...", zap.String("path", path), zap.Error(err))
					time.Sleep(backoff)
					// 最大等待间隔 30s
					if backoff < 30*time.Second {
						backoff *= 2
					}
					continue
				}

				// 重置重试间隔
				backoff = time.Second

				// 只有当 Index 发生变化时才处理逻辑
				if meta.LastIndex <= lastIndex {
					continue
				}
				lastIndex = meta.LastIndex

				if pair == nil {
					zap.L().Warn("Config deleted in consul", zap.String("path", path))
					continue
				}

				// 解析新配置
				newSettings, err := parseYAMLToMap(pair.Value)
				if err != nil {
					zap.L().Error("Failed to parse watched config", zap.Error(err))
					continue
				}

				// 触发外部业务逻辑
				onChange(newSettings)
			}
		}
	}()
}
