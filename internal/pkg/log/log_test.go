package log

import (
	"testing"

	confv1 "github.com/lens077/go-connect-template/internal/conf/v1"
	"github.com/stretchr/testify/require"
)

func TestOptionsFromBootstrap(t *testing.T) {
	options := optionsFromBootstrap(&confv1.Bootstrap{Log: &confv1.Log{
		Application: &confv1.Log_Application{Level: "info", Format: "json"},
		Framework:   &confv1.Log_Framework{LogLevel: "debug", ErrorLevel: "error"},
	}})

	require.Equal(t, "info", options.Level)
	require.Equal(t, "json", options.Format)
	require.Equal(t, "debug", options.FrameworkLogLevel)
	require.Equal(t, "error", options.FrameworkErrorLevel)
}
