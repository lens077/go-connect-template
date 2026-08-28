package config

import (
	"context"
	"fmt"
	"time"

	"github.com/lens077/control-tower/sdk/configsource"
)

var _ Source = (*sdkSource)(nil)
var _ Watcher = (*sdkSource)(nil)

const (
	watchMinBackoff = time.Second
	watchMaxBackoff = 30 * time.Second
)

// sdkSource delegates source selection to the standalone config-center module.
// The selector is a local file so a service can always choose its bootstrap
// source before it has loaded a remote Bootstrap document.
type sdkSource struct {
	config configsource.Config
}

func NewSDKSource(sourceConfigFile string) (Source, error) {
	cfg, err := configsource.LoadSourceConfig(sourceConfigFile)
	if err != nil {
		return nil, err
	}
	if cfg.Type != configsource.TypeConfigCenter {
		return nil, fmt.Errorf("%s must select config_center, got %q", sourceConfigFile, cfg.Type)
	}
	return &sdkSource{config: cfg}, nil
}

func (s *sdkSource) Name() string { return string(s.config.Type) }

func (s *sdkSource) Load(ctx context.Context) (map[string]any, error) {
	contents, err := configsource.Load(ctx, s.config)
	if err != nil {
		return nil, err
	}
	return parseYAMLToMap(contents)
}

// Watch preserves Cart's reconnect semantics around the SDK's single stream.
func (s *sdkSource) Watch(ctx context.Context, onEvent func(WatchEvent)) error {
	if s.config.Type != configsource.TypeConfigCenter {
		return configsource.ErrUnsupportedWatch
	}

	backoff := watchMinBackoff
	for {
		gotEvent := false
		err := configsource.Watch(ctx, s.config, func(event configsource.Event) {
			gotEvent = true
			if event.Err != nil {
				onEvent(WatchEvent{Err: event.Err})
				return
			}
			if event.Deleted {
				onEvent(WatchEvent{Deleted: true})
				return
			}
			raw, parseErr := parseYAMLToMap(event.Value)
			if parseErr != nil {
				onEvent(WatchEvent{Err: fmt.Errorf("parse config-center update: %w", parseErr)})
				return
			}
			onEvent(WatchEvent{Raw: raw})
		})
		if ctx.Err() != nil {
			return nil
		}
		if gotEvent {
			backoff = watchMinBackoff
		}
		onEvent(WatchEvent{Err: fmt.Errorf("config-center watch stream ended, retry in %s: %w", backoff, err)})

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		if backoff *= 2; backoff > watchMaxBackoff {
			backoff = watchMaxBackoff
		}
	}
}
