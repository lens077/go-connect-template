package config

import (
	"bytes"
	"context"
	"fmt"

	"github.com/lens077/go-connect-template/constants"
	"github.com/lens077/go-connect-template/internal/pkg/env"
	"github.com/spf13/viper"
)

// Source 负责取回整份 Bootstrap 配置（一份 YAML 文本）。
type Source interface {
	// Name 返回配置源标识。
	Name() string
	// Load 拉取整份 Bootstrap 配置,返回 viper 扁平化后的 map。
	Load(ctx context.Context) (map[string]any, error)
}

// WatchEvent 一次配置推送。
//
// 三种情况互斥:Err 非空表示这一条推送处理失败(如内容不是合法 YAML),
// Deleted 表示配置项在配置中心被删除,其余情况 Raw 为解析好的新配置。
type WatchEvent struct {
	Raw     map[string]any // Deleted 或 Err 非空时为 nil
	Deleted bool
	Err     error
}

// Watcher 是 Config Center 的变更推送能力。直接文件源仅供显式本地测试，
// 不实现 Watcher。
//
// 实现方负责断线重连,Watch 只在 ctx 取消或遇到不可恢复的错误时返回。
// 单条事件的错误经 WatchEvent.Err 上报,不中断整个订阅
// —— 别人写坏一次配置不该让本服务从此收不到后续的修正。
type Watcher interface {
	Watch(ctx context.Context, onEvent func(WatchEvent)) error
}

// NewSource 正常启动必须通过 CONFIG_SOURCE_FILE 的 selector 读取 Config Center。
// CONFIG_SOURCE=file 仅保留为显式本地测试入口；不存在任何远端回退路径。
func NewSource() (Source, error) {
	// +co:begin config-configcenter
	if sourceConfigFile := env.GetEnvString(constants.EnvConfigSourceFile, ""); sourceConfigFile != "" {
		return NewSDKSource(sourceConfigFile)
	}
	// +co:end

	name := env.GetEnvString(constants.EnvConfigSource, "")
	switch name {
	// +co:begin config-file
	case constants.ConfigSourceFile:
		return NewFileSource()
	// +co:end
	// +co:begin config-configcenter
	case constants.ConfigSourceConfigCenter:
		return nil, fmt.Errorf("%s=%s is deprecated; set %s to a local SourceConfig file instead",
			constants.EnvConfigSource, constants.ConfigSourceConfigCenter, constants.EnvConfigSourceFile)
	// +co:end
	// +co:anchor config-source-cases
	case "":
		return nil, fmt.Errorf("required env %s is missing", constants.EnvConfigSourceFile)
	default:
		return nil, fmt.Errorf("unknown %s=%q, expect %q or set %s",
			constants.EnvConfigSource, name,
			constants.ConfigSourceFile, constants.EnvConfigSourceFile)
	}
}

// parseYAMLToMap 将 YAML 文档解析为 viper 扁平 map,供 decodeConfig 填充 Bootstrap。
// SDK 与显式本地文件入口都返回 YAML 文本，解析逻辑共用。
func parseYAMLToMap(data []byte) (map[string]any, error) {
	v := viper.New()
	v.SetConfigType(constants.ConfigFileFormat)
	if err := v.ReadConfig(bytes.NewBuffer(data)); err != nil {
		return nil, err
	}
	return v.AllSettings(), nil
}
