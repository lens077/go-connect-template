package pkg

import (
	"testing"

	confv1 "github.com/lens077/go-connect-template/internal/conf/v1"
	"github.com/stretchr/testify/assert"
)

func storeWith(defaultDomain string, buckets map[string]string) *confv1.Store {
	return &confv1.Store{
		Minio: &confv1.Store_Minio{
			DefaultDomain: defaultDomain,
			Buckets:       buckets,
		},
	}
}

func TestFormatObjectURL(t *testing.T) {
	cases := []struct {
		name   string
		bucket string
		key    string
		cfg    *confv1.Store
		want   string
	}{
		{
			name: "空 key 返回空",
			want: "",
		},
		{
			name:   "已是绝对 URL 时原样返回",
			bucket: "avatar",
			key:    "https://cdn.example.com/avatar/x.png",
			cfg:    storeWith("https://minio.example.com", nil),
			want:   "https://cdn.example.com/avatar/x.png",
		},
		{
			name:   "协议相对 URL 原样返回",
			bucket: "avatar",
			key:    "//cdn.example.com/avatar/x.png",
			cfg:    storeWith("https://minio.example.com", nil),
			want:   "//cdn.example.com/avatar/x.png",
		},
		{
			name:   "cfg 为 nil 时退回原 key",
			bucket: "avatar",
			key:    "avatar/x.png",
			cfg:    nil,
			want:   "avatar/x.png",
		},
		{
			name:   "default_domain 未配置时退回原 key",
			bucket: "avatar",
			key:    "avatar/x.png",
			cfg:    storeWith("", nil),
			want:   "avatar/x.png",
		},
		{
			name:   "用 default_domain 拼 bucket",
			bucket: "avatar",
			key:    "2024/x.png",
			cfg:    storeWith("https://minio.example.com", nil),
			want:   "https://minio.example.com/avatar/2024/x.png",
		},
		{
			name:   "key 里已含 bucket 前缀时不重复拼",
			bucket: "avatar",
			key:    "avatar/2024/x.png",
			cfg:    storeWith("https://minio.example.com", nil),
			want:   "https://minio.example.com/avatar/2024/x.png",
		},
		{
			name:   "bucket 专属域名优先于 default_domain",
			bucket: "avatar",
			key:    "avatar/x.png",
			cfg:    storeWith("https://minio.example.com", map[string]string{"avatar": "https://cdn.example.com/img"}),
			want:   "https://cdn.example.com/img/x.png",
		},
		{
			name:   "default_domain 带尾斜杠不产生双斜杠",
			bucket: "avatar",
			key:    "x.png",
			cfg:    storeWith("https://minio.example.com/", nil),
			want:   "https://minio.example.com/avatar/x.png",
		},
		{
			// 这条是 CutPrefix 带斜杠匹配的意义所在:裸 TrimPrefix 会切成 "_v2/x.png"
			name:   "同前缀的相邻 bucket 名不被误切",
			bucket: "banner",
			key:    "banner_v2/x.png",
			cfg:    storeWith("https://minio.example.com", nil),
			want:   "https://minio.example.com/banner/banner_v2/x.png",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, FormatObjectURL(c.bucket, c.key, c.cfg))
		})
	}
}
