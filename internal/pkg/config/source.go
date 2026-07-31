package config

import (
	"bytes"
	"context"
	"fmt"

	"github.com/lens077/go-connect-template/constants"
	"github.com/lens077/go-connect-template/internal/pkg/env"
	"github.com/spf13/viper"
)

// Source 配置数据源:负责把整份 Bootstrap 配置(一份 YAML 文本)取回来。
//
// 每个实现放在自己的 source_*.go 里,自带环境变量解析与客户端构造,
// 彼此不共享任何状态 —— 删掉其中一个文件,另一个仍能独立编译运行。
// 新增数据源(如 etcd、Nacos、配置中心)只需实现本接口,再到 NewSource 加一个分支。
type Source interface {
	// Name 数据源标识,与 CONFIG_SOURCE 的取值一致。
	Name() string
	// Load 拉取整份 Bootstrap 配置,返回 viper 扁平化后的 map。
	Load(ctx context.Context) (map[string]any, error)
}

// NewSource 按 CONFIG_SOURCE 选择数据源,未设置时用 constants.DefaultConfigSource。
//
// 刻意不做「主源失败自动降级到备源」:配置来源必须是确定的。静默降级会让服务
// 拿着一份你以为早已废弃的配置正常跑起来,比直接启动失败难排查得多。
func NewSource() (Source, error) {
	name := env.GetEnvString(constants.EnvConfigSource, constants.DefaultConfigSource)
	switch name {
	// +co:begin config-file
	case constants.ConfigSourceFile:
		return NewFileSource()
	// +co:end
	// +co:begin config-consul
	case constants.ConfigSourceConsul:
		return NewConsulSource()
	// +co:end
	// +co:begin config-configcenter
	case constants.ConfigSourceConfigCenter:
		return NewConfigCenterSource()
	// +co:end
	// +co:anchor config-source-cases
	default:
		return nil, fmt.Errorf("unknown %s=%q, expect one of %q",
			constants.EnvConfigSource, name, availableSources())
	}
}

// availableSources 供报错时列出本次构建实际编译进来的数据源。
// 裁掉某个 source_*.go 后,这里的列表会随之变短,错误信息不会指向不存在的选项。
func availableSources() []string {
	return []string{
		constants.ConfigSourceFile,         // +co:config-file
		constants.ConfigSourceConsul,       // +co:config-consul
		constants.ConfigSourceConfigCenter, // +co:config-configcenter
		// +co:anchor config-source-names
	}
}

// parseYAMLToMap 将 YAML 文档解析为 viper 扁平 map,供 decodeConfig 填充 Bootstrap。
// 各数据源取回的都是同一份 YAML 文本,差别只在从哪儿取,故解析逻辑共用。
func parseYAMLToMap(data []byte) (map[string]any, error) {
	v := viper.New()
	v.SetConfigType(constants.ConfigFileFormat)
	if err := v.ReadConfig(bytes.NewBuffer(data)); err != nil {
		return nil, err
	}
	return v.AllSettings(), nil
}
