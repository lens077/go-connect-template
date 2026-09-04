package otel

import (
	"testing"
	"time"

	confv1 "github.com/lens077/go-connect-template/internal/conf/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestOptionsFromBootstrap(t *testing.T) {
	options := optionsFromBootstrap(&confv1.Bootstrap{Observability: &confv1.Observability{
		Enable: true,
		Trace: &confv1.Observability_Trace{
			Endpoint:    "trace:4318",
			SampleRatio: wrapperspb.Double(0.25),
		},
		Metric: &confv1.Observability_Metric{
			Endpoint:       "metric:4318",
			ExportInterval: durationpb.New(45 * time.Second),
		},
		Log: &confv1.Observability_Logging{Endpoint: "log:4318"},
	}})

	require.NotNil(t, options.Trace)
	require.NotNil(t, options.Metric)
	require.NotNil(t, options.Logging)
	require.Equal(t, 0.25, *options.Trace.SampleRatio)
	require.Equal(t, 45*time.Second, options.Metric.ExportInterval)
	require.True(t, options.RuntimeMetrics)
}

func TestOptionsFromBootstrapDisablesAllSignals(t *testing.T) {
	options := optionsFromBootstrap(&confv1.Bootstrap{})
	require.Nil(t, options.Trace)
	require.Nil(t, options.Metric)
	require.Nil(t, options.Logging)
}
