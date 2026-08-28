package otel

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"regexp"
	"runtime"
	"strings"
	"sync" // +co:redis
	"time"

	confv1 "github.com/lens077/go-connect-template/internal/conf/v1"
	"github.com/lens077/go-connect-template/internal/pkg/meta"

	redisotel "github.com/redis/go-redis/extra/redisotel-native/v9" // +co:redis
	otelruntime "go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

const (
	// defaultSampleRatio 是 observability.trace.sample_ratio 缺省时的采样率。
	// 取 1.0 而不是某个「合理的」小数,是为了让没配这个字段的存量环境
	// 升级前后行为完全一致(升级前是 AlwaysSample)。要降采样得显式去配。
	defaultSampleRatio = 1.0

	// defaultMetricInterval 是 observability.metric.export_interval 缺省时的导出间隔。
	// OTel SDK 自己的默认值是 60s,这里取 30s 作为折中。
	// (历史值是硬编码的 3s —— 20 倍于默认频率,乘以服务数量后 collector 侧不划算)
	defaultMetricInterval = 30 * time.Second
)

// Module 提供 Fx 模块
var Module = fx.Module("otel",
	fx.Provide(
		// 提供 OpenTelemetry 设置函数
		func(info meta.AppInfo, cfg *confv1.Observability, logger *zap.Logger) (func(context.Context) error, error) {
			return SetupOTelSDK(context.Background(), info, cfg, logger)
		},
	),
)

// SetupOTelSDK 初始化 trace / metric / log 三条导出管道,返回统一的关闭函数。
func SetupOTelSDK(ctx context.Context, info meta.AppInfo, cfg *confv1.Observability, logger *zap.Logger) (func(context.Context) error, error) {
	if cfg == nil || !cfg.Enable {
		logger.Info("observability is disabled, skipping OpenTelemetry setup")
		return func(context.Context) error { return nil }, nil
	}

	// 存储所有子组件停止方法的切片
	var shutdownFuncs []func(context.Context) error

	// 返回给外部的关闭 otel,合并所有组件的错误
	shutdown := func(ctx context.Context) error {
		var err error
		for _, fn := range shutdownFuncs {
			err = errors.Join(err, fn(ctx))
		}
		shutdownFuncs = nil
		return err
	}

	// SDK 自己的内部错误(导出失败、批队列溢出、endpoint 不通)默认是被丢弃的。
	// 不装这个 handler,collector 挂掉时应用侧一行日志都不会有 —— 表现就是
	// 「服务一切正常但 Jaeger 里什么都没有」,只能去翻 collector 才定位得到。
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		logger.Error("opentelemetry sdk error", zap.Error(err))
	}))

	otel.SetTextMapPropagator(newPropagator())

	res, err := newResource(info)
	if err != nil {
		return shutdown, errors.Join(err, shutdown(ctx))
	}

	tracerProvider, err := newTracerProvider(ctx, res, cfg.Trace, logger)
	if err != nil {
		return shutdown, errors.Join(err, shutdown(ctx))
	}
	shutdownFuncs = append(shutdownFuncs, tracerProvider.Shutdown)
	otel.SetTracerProvider(tracerProvider)

	meterProvider, err := newMeterProvider(ctx, res, cfg.Metric, logger)
	if err != nil {
		return shutdown, errors.Join(err, shutdown(ctx))
	}
	shutdownFuncs = append(shutdownFuncs, meterProvider.Shutdown)
	otel.SetMeterProvider(meterProvider)

	// Go runtime 指标(goroutine 数、堆内存、GC 目标)。放在 SetMeterProvider
	// 之后,observable 回调直接注册到真实 provider,不经过全局代理的延迟绑定。
	if err := otelruntime.Start(); err != nil {
		return shutdown, errors.Join(err, shutdown(ctx))
	}

	loggerProvider, err := newLoggerProvider(ctx, res, cfg.Log, logger)
	if err != nil {
		return shutdown, errors.Join(err, shutdown(ctx))
	}
	shutdownFuncs = append(shutdownFuncs, loggerProvider.Shutdown)
	global.SetLoggerProvider(loggerProvider)

	return shutdown, nil
}

// tlsClientConfig 把配置里的 TLS 段翻译成 *tls.Config。
// 返回 nil 表示这条管道不启用 TLS,调用方据此改用 WithInsecure。
func tlsClientConfig(cfg *confv1.Observability_Tls, logger *zap.Logger) *tls.Config {
	if cfg == nil || !cfg.Enable {
		return nil
	}

	conf := &tls.Config{InsecureSkipVerify: cfg.InsecureSkipVerify}
	if !cfg.InsecureSkipVerify && len(cfg.CaPem) > 0 {
		pool := x509.NewCertPool()
		if pool.AppendCertsFromPEM([]byte(cfg.CaPem)) {
			conf.RootCAs = pool
		} else {
			// 不 fatal:退回系统根证书仍有可能握手成功,而且可观测性不该拖垮主流程。
			logger.Error("failed to append ca cert, falling back to system roots")
		}
	}
	return conf
}

func newResource(info meta.AppInfo) (*resource.Resource, error) {
	// 与 resource.Default() 合并,才能拿到 SDK 自己填的 telemetry.sdk.* 等属性。
	// 前提是两边 semconv 版本一致 —— 本文件的 semconv 必须跟 sdk 内部用的版本对齐,
	// 否则 Merge 会直接返回 ErrSchemaURLConflict —— 而本文件把 newResource 的错误当致命
	// 处理,结果是服务起不来。sdk v1.45 内部用的是 semconv v1.43.0。
	// (2026-08-06 实测:otel v1.44→v1.45 把内部 semconv 从 v1.41 换到 v1.43,
	//  本文件没跟着改就会炸在启动阶段;TestNewResource 的 SchemaURL 断言就是防这个。)
	return resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(info.Name),       // 应用名称
			semconv.ServiceVersion(info.Version), // 应用版本
			// 实例 ID。缺了它,同一服务的多个副本在指标上会被聚成一条曲线,
			// 「到底是哪个 pod 在抖」这种问题就没法查。
			semconv.ServiceInstanceID(info.ID),
			semconv.DeploymentEnvironmentNameKey.String(info.Environment), // 部署环境
			semconv.HostIP(info.Host),
			// 运行时信息用 semconv 标准字段,不要自造 key ——
			// 自造的 key 后端无法按标准维度聚合。
			semconv.ProcessRuntimeName("go"),
			semconv.ProcessRuntimeVersion(runtime.Version()),
		),
	)
}

func newPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}

// sqlcQueryNameRe 匹配 sqlc 生成的查询头:`-- name: GetCartItems :many`
var sqlcQueryNameRe = regexp.MustCompile(`--\s*name:\s*(\S+)`)

// SQLSpanName 从 SQL 语句推导 pgx span 名,给 otelpgx.WithSpanNameFunc 用。
//
// 为什么不用 otelpgx 自带的 WithTrimSQLInSpanName:它取的是「SQL 的第一个词」,
// 而 sqlc 生成的语句第一个词是注释符 `--`,结果所有查询的 span 名都变成
// "query --" —— 基数是降到 1 了,但也彻底分不出哪条是哪条,比不改更糟。
// (这一点是实测发现的,不是照文档推断的。)
//
// 这里改成取 sqlc 的查询名,span 名形如 "query GetCartItems":既是有界的低基数,
// 又能一眼看出跑的是哪条查询。完整 SQL 仍然在 db.statement attribute 上。
func SQLSpanName(stmt string) string {
	if m := sqlcQueryNameRe.FindStringSubmatch(stmt); m != nil {
		return m[1]
	}

	// 退路:手写 SQL 没有 sqlc 头,取第一个非注释非空行的首个词(SELECT / INSERT ...)
	for line := range strings.SplitSeq(stmt, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "--") {
			continue
		}
		if i := strings.IndexAny(line, " \t("); i > 0 {
			return line[:i]
		}
		return line
	}
	return "unknown"
}

// sampleRatio 读取配置里的采样率并夹到 [0,1]。
// 用指针判空:配置里没写这个字段时是 nil,回落到 defaultSampleRatio;
// 显式写 0.0 则是「一条都不采」—— 这两者必须能区分开。
func sampleRatio(cfg *confv1.Observability_Trace, logger *zap.Logger) float64 {
	if cfg.GetSampleRatio() == nil {
		return defaultSampleRatio
	}

	ratio := cfg.GetSampleRatio().GetValue()
	switch {
	case ratio < 0:
		logger.Warn("trace.sample_ratio below 0, clamped to 0", zap.Float64("configured", ratio))
		return 0
	case ratio > 1:
		logger.Warn("trace.sample_ratio above 1, clamped to 1", zap.Float64("configured", ratio))
		return 1
	}
	return ratio
}

func newTracerProvider(ctx context.Context, res *resource.Resource, cfg *confv1.Observability_Trace, logger *zap.Logger) (*trace.TracerProvider, error) {
	opts := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(cfg.GetEndpoint()),
		otlptracehttp.WithCompression(otlptracehttp.GzipCompression),
	}
	if t := tlsClientConfig(cfg.GetTls(), logger); t != nil {
		opts = append(opts, otlptracehttp.WithTLSClientConfig(t))
	} else {
		// 没有 TLS 配置时必须显式声明 Insecure,否则 exporter 会按 https 去连。
		opts = append(opts, otlptracehttp.WithInsecure())
	}

	traceExporter, err := otlptracehttp.New(ctx, opts...)
	if err != nil {
		return nil, err
	}

	ratio := sampleRatio(cfg, logger)
	logger.Info("trace sampler configured", zap.Float64("ratio", ratio))

	return trace.NewTracerProvider(
		// ParentBased 是关键:采样决策必须整条链一致。
		// 只用 TraceIDRatioBased 的话每个服务各自掷骰子,ratio=0.1 时一条 5 跳的链
		// 能完整留下来的概率是 0.1^5 —— 拿到的是满屏残缺的半截 trace,比不采样更难用。
		// 包一层 ParentBased 之后,下游无条件跟随上游已经做出的决策:采就整条采、
		// 不采就整条丢,只有在没有上游(链路入口)时才按 ratio 掷骰子。
		trace.WithSampler(trace.ParentBased(trace.TraceIDRatioBased(ratio))),
		trace.WithResource(res),
		trace.WithSpanProcessor(trace.NewBatchSpanProcessor(traceExporter)),
	), nil
}

// newMeterProvider 用 Push 模式主动把指标推给 OTLP Collector。
func newMeterProvider(ctx context.Context, res *resource.Resource, cfg *confv1.Observability_Metric, logger *zap.Logger) (*metric.MeterProvider, error) {
	opts := []otlpmetrichttp.Option{
		otlpmetrichttp.WithEndpoint(cfg.GetEndpoint()),
		otlpmetrichttp.WithCompression(otlpmetrichttp.GzipCompression),
	}
	if t := tlsClientConfig(cfg.GetTls(), logger); t != nil {
		opts = append(opts, otlpmetrichttp.WithTLSClientConfig(t))
	} else {
		opts = append(opts, otlpmetrichttp.WithInsecure())
	}

	metricExporter, err := otlpmetrichttp.New(ctx, opts...)
	if err != nil {
		return nil, err
	}

	interval := defaultMetricInterval
	if d := cfg.GetExportInterval(); d != nil && d.AsDuration() > 0 {
		interval = d.AsDuration()
	}
	logger.Info("metric exporter configured", zap.Duration("interval", interval))

	return metric.NewMeterProvider(
		metric.WithResource(res),
		metric.WithReader(metric.NewPeriodicReader(metricExporter,
			metric.WithInterval(interval),
			// go.schedule.duration 直方图:goroutine 就绪后等到实际运行的时长。
			// 新版 runtime 指标集没有 GC 暂停项,调度延迟是它的替代信号 ——
			// GC STW 和 CPU 饱和都会先在这里冒头。
			metric.WithProducer(otelruntime.NewProducer()),
		)),
	), nil
}

// redisOTelOnce 见 EnsureRedisInstrumentation。
// +co:begin redis
var redisOTelOnce sync.Once

// EnsureRedisInstrumentation 给 go-redis 装配 OTel 指标(命令时延、连接池、错误)。
// 幂等;必须在第一次 redis.NewClient 之前调用 —— 连接池 gauge 的注册发生在
// NewClient 时刻,建完 client 再 Init 会漏掉池级指标。
// 与 SetupOTelSDK 的先后无所谓:Init 从全局 MeterProvider 的延迟代理拿 meter,
// SetMeterProvider 之后会自动接上。
// 注意指标名与 otelpgx 共用 semconv 的 db.client.* 前缀,查询端必须按
// db.system.name(redis / postgresql)区分,不能按指标名裸聚合。
func EnsureRedisInstrumentation(logger *zap.Logger) {
	redisOTelOnce.Do(func() {
		cfg := redisotel.NewConfig().WithEnabled(true)
		if err := redisotel.GetObservabilityInstance().Init(cfg); err != nil {
			// 装配失败不拖垮主流程:记日志,redis 照常可用,只是没有指标。
			logger.Error("failed to init redis otel instrumentation", zap.Error(err))
		}
	})
}

// +co:end

func newLoggerProvider(ctx context.Context, res *resource.Resource, cfg *confv1.Observability_Logging, logger *zap.Logger) (*log.LoggerProvider, error) {
	opts := []otlploghttp.Option{
		otlploghttp.WithEndpoint(cfg.GetEndpoint()),
		otlploghttp.WithCompression(otlploghttp.GzipCompression),
	}
	if t := tlsClientConfig(cfg.GetTls(), logger); t != nil {
		opts = append(opts, otlploghttp.WithTLSClientConfig(t))
	} else {
		opts = append(opts, otlploghttp.WithInsecure())
	}

	logExporter, err := otlploghttp.New(ctx, opts...)
	if err != nil {
		return nil, err
	}

	return log.NewLoggerProvider(
		log.WithResource(res),
		log.WithProcessor(log.NewBatchProcessor(logExporter)),
	), nil
}
