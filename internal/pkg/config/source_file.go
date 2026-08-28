package config

import (
	"context"
	"fmt"
	"os"

	"github.com/lens077/go-connect-template/constants"
	"github.com/lens077/go-connect-template/internal/pkg/env"
)

var _ Source = (*fileSource)(nil)

// fileSource is deliberately startup-only. A local file is the emergency and
// developer source; unlike Config Center it does not imply hot reload.
type fileSource struct {
	path string
}

func NewFileSource() (Source, error) {
	path := env.GetEnvString(constants.EnvConfigFile, "")
	if path == "" {
		return nil, fmt.Errorf("required env %s is missing when %s=%s",
			constants.EnvConfigFile, constants.EnvConfigSource, constants.ConfigSourceFile)
	}
	return &fileSource{path: path}, nil
}

func (s *fileSource) Name() string { return constants.ConfigSourceFile }

func (s *fileSource) Load(context.Context) (map[string]any, error) {
	contents, err := os.ReadFile(s.path)
	if err != nil {
		return nil, fmt.Errorf("read local config %q: %w", s.path, err)
	}
	if len(contents) == 0 {
		return nil, fmt.Errorf("local config %q is empty", s.path)
	}
	return parseYAMLToMap(contents)
}
