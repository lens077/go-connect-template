package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/lens077/go-connect-template/constants"
	"github.com/lens077/go-connect-template/internal/pkg/env"
)

var _ Source = (*fileSource)(nil)

// fileSource 从本地 YAML 文件读整份 Bootstrap。
//
// 存在的意义是「零依赖启动」:克隆下来 go run 就能跑,不需要先架一套 Consul。
// 本地开发和单机部署用它;多实例部署应换成 consul 等集中式数据源,
// 否则每台机器上的配置各改各的,漂移了也无从发现。
type fileSource struct {
	path string
}

// NewFileSource 由 CONFIG_FILE 指定路径,默认 constants.ConfigFilePath。
// 相对路径按进程工作目录解析,启动失败时报的是绝对路径,省去猜「到底读的哪个文件」。
func NewFileSource() (Source, error) {
	path := env.GetEnvString(constants.EnvConfigFile, constants.ConfigFilePath)

	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve %s=%q failed: %w", constants.EnvConfigFile, path, err)
	}

	return &fileSource{path: abs}, nil
}

func (s *fileSource) Name() string { return constants.ConfigSourceFile }

func (s *fileSource) Load(_ context.Context) (map[string]any, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return nil, fmt.Errorf("read config file %s failed: %w", s.path, err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("config file is empty: %s", s.path)
	}
	return parseYAMLToMap(data)
}
