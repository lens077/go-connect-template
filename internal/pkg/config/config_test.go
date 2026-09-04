package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lens077/go-connect-template/constants"
	confv1 "github.com/lens077/go-connect-template/internal/conf/v1"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

func TestModuleKeepsStrictKitDefaults(t *testing.T) {
	valid, err := os.ReadFile(filepath.Join("..", "..", "..", "configs", "dev.yml"))
	require.NoError(t, err)
	example, err := os.ReadFile(filepath.Join("..", "..", "..", "configs", "config.yaml.example"))
	require.NoError(t, err)

	for _, test := range []struct {
		name        string
		contents    []byte
		wantProblem string
	}{
		{name: "valid baseline", contents: valid},
		{name: "valid documented example", contents: example},
		{name: "unknown field", contents: append(append([]byte(nil), valid...), []byte("\nunexpected_key: true\n")...), wantProblem: "unexpected_key"},
		{name: "missing required sections", contents: []byte("server:\n  addr: ''\n"), wantProblem: "validate config"},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yml")
			require.NoError(t, os.WriteFile(path, test.contents, 0o600))
			t.Setenv(constants.EnvConfigSourceFile, "")
			t.Setenv(constants.EnvConfigSource, constants.ConfigSourceFile)
			t.Setenv(constants.EnvConfigFile, path)

			var live *Live
			app := fx.New(fx.NopLogger, fx.Supply(zap.NewNop()), Module, fx.Populate(&live))
			if test.wantProblem == "" {
				require.NoError(t, app.Err())
				require.NotNil(t, live)
				return
			}
			require.ErrorContains(t, app.Err(), test.wantProblem)
		})
	}
}

func TestRestartRequiredSections(t *testing.T) {
	sections := restartRequiredSections(&confv1.Bootstrap{})
	require.Len(t, sections, 4)
	require.Equal(t, []string{"server", "discovery", "observability", "search"}, []string{
		sections[0].Name,
		sections[1].Name,
		sections[2].Name,
		sections[3].Name,
	})
}
