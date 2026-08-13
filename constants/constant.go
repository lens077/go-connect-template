package constants

import "time"

const (
	Host = "localhost"
	Port = "8080"
)

// 基础设施超时兜底值。配置里没写对应字段时用这些,
// 不能留零值:context.WithTimeout(0) 会立刻超时,连接本身没问题也起不来。
const (
	// DefaultDBPingTimeout 建池后的首次探活超时
	DefaultDBPingTimeout = 5 * time.Second

	// DefaultHealthCheckTimeout /healthz 里每个依赖的单项检查超时。
	// 要明显小于 Consul 的 check.ttl,否则健康检查还没返回就被判 critical。
	DefaultHealthCheckTimeout = 2 * time.Second
)

// RPC metadata
const (
	UserOwnerMetadataKey = "x-md-global-owner"
	UserNameMetadataKey  = "x-md-global-name"
	UserRoleMetadataKey  = "x-md-global-role"
	UserIdMetadataKey    = "x-md-global-user-id"
)

// Log options
const (
	FormatConsole = "console"
	FormatJson    = "json"
)

// Postgres ssl mode options
const (
	SslModeDisable    = "disable"
	SslModeAllow      = "allow"
	SslModePrefer     = "prefer"
	SslModeVerifyCa   = "verify-ca"
	SslModeVerifyFull = "verify-full"
)

// Consul configs default values
const (
	ConsulAddr               = "127.0.0.1:8500"
	ConsulScheme             = "http"
	ConsulTlsScheme          = "https"
	ConsulInsecureSkipVerify = false
	ConsulToken              = ""
)

// Consul service tags
const (
	ConsulTagFx  = "fx"
	ConsulTagTtl = "ttl"
)

// 配置数据源:整份 Bootstrap 从哪儿拉。
// 生产路径走 CONFIG_SOURCE_FILE(selector);CONFIG_SOURCE 只保留本地 file 与已废弃的 configcenter。
const (
	ConfigSourceFile = "file" // 从本地 YAML 文件读,零外部依赖
	// ConfigSourceConfigCenter 已废弃:改用 CONFIG_SOURCE_FILE 指向 type: config_center 的 selector。
	ConfigSourceConfigCenter = "configcenter"

	// DefaultConfigSource 默认读本地文件:克隆下来不配任何环境变量就能起服务。
	// 生产部署用 CONFIG_SOURCE_FILE,不依赖这个默认值。
	DefaultConfigSource = ConfigSourceFile

	// ConfigFileFormat 各数据源存的都是 YAML 文本,解析时统一按此格式。
	ConfigFileFormat = "yaml"

	// ConfigFilePath ConfigSourceFile 的默认路径,相对进程工作目录。
	ConfigFilePath = "configs/dev.yml"
)
