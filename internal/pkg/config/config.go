package config

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/lens077/go-connect-template/constants"
	confv1 "github.com/lens077/go-connect-template/internal/conf/v1"
	"github.com/mitchellh/mapstructure"
	"github.com/spf13/viper"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
)

var (
	// defaultLive 本进程「当前」这一份配置。GetConfig 与 fx 注入的 *Live 都指向它,
	// 保证「配置只有一份」—— 两份各自更新的快照迟早会对不上。
	defaultLive = NewLive(&confv1.Bootstrap{})

	srcMu sync.RWMutex
	// activeSource 本次启动实际生效的数据源,供启动日志与变更订阅使用
	// (排查「读的到底是哪份配置」)
	activeSource Source

	Module = fx.Module("config",
		fx.Provide(
			func(lc fx.Lifecycle) (*confv1.Bootstrap, error) {
				ctx, cancel := context.WithCancel(context.Background())

				lc.Append(fx.Hook{
					OnStop: func(ctx context.Context) error {
						cancel()
						return nil
					},
				})

				bootstrap, err := Init(ctx)
				if err != nil {
					return nil, err
				}

				return bootstrap, nil
			},
			// *Bootstrap 是启动那一刻的快照,给只能在构造期读一次的消费者;
			// 需要「随时读到最新值」的消费者注入 *Live。
			func() *Live { return defaultLive },
		),
		fx.Invoke(startWatch),
	)
)

func decodeConfig(data map[string]any, target any) error {
	v := viper.New()
	v.SetConfigType(constants.ConfigFileFormat)
	for k, val := range data {
		v.Set(k, val)
	}

	stringToProtoDurationHook := func(f reflect.Type, t reflect.Type, data any) (any, error) {
		if f.Kind() != reflect.String {
			return data, nil
		}
		if t != reflect.TypeOf(&durationpb.Duration{}) {
			return data, nil
		}

		d, err := time.ParseDuration(data.(string))
		if err != nil {
			return nil, err
		}
		return durationpb.New(d), nil
	}

	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		TagName: "json",
		DecodeHook: mapstructure.ComposeDecodeHookFunc(
			stringToProtoDurationHook,
		),
		Result: target,
	})
	if err != nil {
		return err
	}
	return decoder.Decode(v.AllSettings())
}

// Init 拉取并解析整份 Bootstrap 配置。
//
// 「从哪儿拉」由 NewSource 决定(见 source.go),本函数只负责加载、解码和发布,
// 不感知远端细节;其余全部配置(含 Consul 服务发现地址、DB、Redis 等)
// 一律由选中的数据源下发。
func Init(ctx context.Context) (*confv1.Bootstrap, error) {
	src, err := NewSource()
	if err != nil {
		return nil, err
	}

	rawConfig, err := src.Load(ctx)
	if err != nil {
		return nil, err
	}

	localConf := &confv1.Bootstrap{}
	if err := decodeConfig(rawConfig, localConf); err != nil {
		return nil, fmt.Errorf("decode bootstrap from %s: %w", src.Name(), err)
	}

	srcMu.Lock()
	activeSource = src
	srcMu.Unlock()
	defaultLive.Set(localConf)

	return localConf, nil
}

// GetConfig 返回当前配置(热更新后即为新值)。
func GetConfig() *confv1.Bootstrap {
	return defaultLive.Get()
}

// SourceName 返回本次启动实际生效的配置数据源名;Init 之前为空。
func SourceName() string {
	if src := currentSource(); src != nil {
		return src.Name()
	}
	return ""
}

func currentSource() Source {
	srcMu.RLock()
	defer srcMu.RUnlock()
	return activeSource
}

// startWatch 在数据源支持变更推送时订阅热更新。
//
// 不支持推送的数据源(本地文件)保持「启动读一次」的语义,只打一行日志说明,
// 免得运维以为改了配置就会生效。
func startWatch(lc fx.Lifecycle, logger *zap.Logger, live *Live) {
	log := logger.Named("configWatch")

	w, ok := currentSource().(Watcher)
	if !ok {
		log.Info("当前配置数据源不支持变更推送,配置仅在启动时加载一次",
			zap.String("source", SourceName()))
		return
	}

	live.Subscribe(func(old, cur *confv1.Bootstrap) { warnNotHotReloadable(log, old, cur) })

	ctx, cancel := context.WithCancel(context.Background())
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go func() {
				err := w.Watch(ctx, func(ev WatchEvent) {
					switch {
					case ev.Err != nil:
						log.Error("收到一条无法处理的配置推送,保留当前配置", zap.Error(ev.Err))
					case ev.Deleted:
						// 配置项被删不该让服务失能:继续用最后一份已知配置
						log.Error("配置项已在配置中心被删除,继续沿用当前配置",
							zap.String("source", SourceName()))
					default:
						cur := &confv1.Bootstrap{}
						if err := decodeConfig(ev.Raw, cur); err != nil {
							// 一份写错的配置不能把在跑的服务带塌
							log.Error("热更新配置解码失败,保留当前配置", zap.Error(err))
							return
						}
						live.Set(cur)
						log.Info("配置已热更新", zap.String("source", SourceName()))
					}
				})
				if err != nil && ctx.Err() == nil {
					log.Error("配置变更订阅已退出,本服务将停留在最后一份已知配置", zap.Error(err))
				}
			}()
			return nil
		},
		OnStop: func(context.Context) error {
			cancel()
			return nil
		},
	})
}

// warnNotHotReloadable 对「改了也不会立刻生效」的配置段发出警告。
//
// 这几段不做热生效是权衡后的取舍(重新绑定端口会切断在途连接、重注册服务
// 需要先摘节点、重建 tracer 会丢掉未导出的 span),滚动重启是更可控的做法。
// 但沉默是不可接受的:改完没反应、也没人说一声,只会让人以为热更新坏了。
func warnNotHotReloadable(log *zap.Logger, old, cur *confv1.Bootstrap) {
	for _, s := range []struct {
		name     string
		old, cur proto.Message
	}{
		{"server", old.GetServer(), cur.GetServer()},
		{"discovery", old.GetDiscovery(), cur.GetDiscovery()},
		{"observability", old.GetObservability(), cur.GetObservability()},
	} {
		if !proto.Equal(s.old, s.cur) {
			log.Warn("该配置段已变更,但需要重启服务才会生效", zap.String("section", s.name))
		}
	}
}
