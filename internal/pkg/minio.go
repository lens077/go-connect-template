package pkg

import (
	"net/url"
	"strings"

	confv1 "github.com/lens077/go-connect-template/internal/conf/v1"
)

// FormatObjectURL 将相对 Key 转换为带有正确域名的绝对 URL
func FormatObjectURL(bucket, relativeKey string, cfg *confv1.Store) string {
	if relativeKey == "" {
		return ""
	}

	// 1. 绝对路径或 schema 开头直接返回
	if strings.HasPrefix(relativeKey, "http://") || strings.HasPrefix(relativeKey, "https://") || strings.HasPrefix(relativeKey, "//") {
		return relativeKey
	}

	if cfg == nil || cfg.Minio == nil {
		return relativeKey
	}

	// 2. 核心修复：带斜杠精准匹配切除桶前缀，避免误伤（如 ecommerce_banner）
	bucketPrefix := bucket + "/"
	imgURL := relativeKey
	if after, ok := strings.CutPrefix(relativeKey, bucketPrefix); ok {
		imgURL = after
	}

	var baseURL string

	// 3. 查找该 bucket 是否有专属域名
	if cfg.Minio.Buckets != nil {
		baseURL = cfg.Minio.Buckets[bucket]
	}

	// 4. 降级逻辑：若没有专属域名，使用 DefaultDomain + bucket 拼接
	if baseURL == "" {
		if cfg.Minio.DefaultDomain == "" {
			return relativeKey
		}

		var err error
		baseURL, err = url.JoinPath(cfg.Minio.DefaultDomain, bucket)
		if err != nil {
			// 如果 DefaultDomain 配置不规范导致 JoinPath 报错，采用安全的手动拼接降级
			baseURL = strings.TrimSuffix(cfg.Minio.DefaultDomain, "/") + "/" + bucket
		}
	}

	// 5. 拼接完整 URL
	fullURL, err := url.JoinPath(baseURL, imgURL)
	if err != nil {
		// 容错降级：如果 baseURL 依然解析失败，手动兜底拼接，而不是直接丢弃域名
		return strings.TrimSuffix(baseURL, "/") + "/" + strings.TrimPrefix(imgURL, "/")
	}

	return fullURL
}
