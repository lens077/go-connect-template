package server

import (
	"context"

	"github.com/lens077/go-connect-template/internal/data"
)

type HealthStatus struct {
	Healthy bool              `json:"healthy"`
	Details map[string]string `json:"details,omitempty"`
}

func healthStatus(ctx context.Context, deps *data.Data) HealthStatus {
	details := make(map[string]string)
	healthy := true

	// 每个依赖一个独立检查项:整体 healthy 为 false 时,details 里能直接看出是哪一个挂了,
	// 不用再翻日志。带 +co: 标记的行会被 co-cli 按所选 feature 裁掉。
	checks := map[string]func(context.Context) error{
		"postgres":      deps.CheckDatabase,
		"redis":         deps.CheckCache,         // +co:redis
		"elasticSearch": deps.CheckElasticSearch, // +co:elasticsearch
	}

	for name, check := range checks {
		state := "ok"
		if err := check(ctx); err != nil {
			state = err.Error()
			healthy = false
		}
		details[name] = state
	}

	return HealthStatus{Healthy: healthy, Details: details}
}
