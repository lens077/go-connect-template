package pkg

import (
	"net/url"
	"strings"

	confv1 "github.com/lens077/go-connect-template/internal/conf/v1"
)

// FormatObjectURL 把库里存的相对 Key 拼成可直接访问的绝对 URL。
//
// 库里只存 Key(如 "avatar/2024/x.png"),域名放配置:换 CDN、换 bucket 域名
// 时改一处配置即可,不用刷全表。任何一步拼不出来都返回原始 relativeKey,
// 而不是空串 —— 宁可前端拿到一个相对路径自己接域名,也不要整个字段消失。
func FormatObjectURL(bucket, relativeKey string, cfg *confv1.Store) string {
	if relativeKey == "" {
		return ""
	}

	// 已经是绝对地址(含协议相对的 //host/path)的直接放行,便于存量数据混用
	if strings.HasPrefix(relativeKey, "http://") ||
		strings.HasPrefix(relativeKey, "https://") ||
		strings.HasPrefix(relativeKey, "//") {
		return relativeKey
	}

	if cfg == nil || cfg.Minio == nil {
		return relativeKey
	}

	// 带斜杠精准切前缀:直接 TrimPrefix(key, bucket) 会把 "banner_v2/x.png"
	// 在 bucket="banner" 时误切成 "_v2/x.png"
	imgURL := relativeKey
	if after, ok := strings.CutPrefix(relativeKey, bucket+"/"); ok {
		imgURL = after
	}

	var baseURL string

	// 优先用该 bucket 的专属域名(独立 CDN 的场景)
	if cfg.Minio.Buckets != nil {
		baseURL = cfg.Minio.Buckets[bucket]
	}

	// 没配专属域名就退回 default_domain + bucket
	if baseURL == "" {
		if cfg.Minio.DefaultDomain == "" {
			return relativeKey
		}

		var err error
		baseURL, err = url.JoinPath(cfg.Minio.DefaultDomain, bucket)
		if err != nil {
			// default_domain 写得不规范导致 JoinPath 失败时手工拼,别丢域名
			baseURL = strings.TrimSuffix(cfg.Minio.DefaultDomain, "/") + "/" + bucket
		}
	}

	fullURL, err := url.JoinPath(baseURL, imgURL)
	if err != nil {
		return strings.TrimSuffix(baseURL, "/") + "/" + strings.TrimPrefix(imgURL, "/")
	}

	return fullURL
}
