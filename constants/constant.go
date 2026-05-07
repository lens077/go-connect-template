package constants

const (
	Host = "localhost"
	Port = "8080"
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
	ConsulPath               = "/consul/"
	ConsulFileFormat         = "yaml"
	ConsulScheme             = "http"
	ConsulTlsScheme          = "https"
	ConsulInsecureSkipVerify = false
	ConsulToken              = ""
)
