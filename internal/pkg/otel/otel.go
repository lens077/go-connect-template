package otel

import (
	"time"

	kitotel "github.com/lens077/go-connect-kit/otel"
	confv1 "github.com/lens077/go-connect-template/internal/conf/v1"
	"go.uber.org/fx"
)

// Module projects the generated service's protobuf into go-connect-kit/otel.
var Module = fx.Module("service-otel-adapter",
	fx.Provide(optionsFromBootstrap),
	kitotel.Module,
)

func optionsFromBootstrap(conf *confv1.Bootstrap) kitotel.Options {
	observability := conf.GetObservability()
	if !observability.GetEnable() {
		return kitotel.Options{}
	}

	trace := observability.GetTrace()
	metric := observability.GetMetric()
	logging := observability.GetLog()
	var sampleRatio *float64
	if configured := trace.GetSampleRatio(); configured != nil {
		value := configured.GetValue()
		sampleRatio = &value
	}
	var exportInterval time.Duration
	if configured := metric.GetExportInterval(); configured != nil {
		exportInterval = configured.AsDuration()
	}

	return kitotel.Options{
		Trace: &kitotel.TraceOptions{
			Endpoint:    trace.GetEndpoint(),
			SampleRatio: sampleRatio,
			TLS:         tlsOptions(trace.GetTls()),
		},
		Metric: &kitotel.MetricOptions{
			Endpoint:       metric.GetEndpoint(),
			ExportInterval: exportInterval,
			TLS:            tlsOptions(metric.GetTls()),
		},
		Logging: &kitotel.LoggingOptions{
			Endpoint: logging.GetEndpoint(),
			TLS:      tlsOptions(logging.GetTls()),
		},
		RuntimeMetrics: true,
	}
}

func tlsOptions(conf *confv1.Observability_Tls) kitotel.TLSOptions {
	return kitotel.TLSOptions{
		Enabled:            conf.GetEnable(),
		InsecureSkipVerify: conf.GetInsecureSkipVerify(),
		CAPEM:              conf.GetCaPem(),
	}
}
