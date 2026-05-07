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

	// 注册独立的检查项
	checks := map[string]func(context.Context) error{
		"database": deps.CheckDatabase, // 数据库
		"cache":    deps.CheckCache,    // 缓存
		// 如果有需要，可以继续添加其它基础设施的健康检查
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
