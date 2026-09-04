package registry

import (
	kitregistry "github.com/lens077/go-connect-kit/registry"
	confv1 "github.com/lens077/go-connect-template/internal/conf/v1"
	"go.uber.org/fx"
)

// Module projects the generated service's protobuf into go-connect-kit/registry.
var Module = fx.Module("service-registry-adapter",
	fx.Provide(optionsFromBootstrap),
	kitregistry.Module,
)

func optionsFromBootstrap(conf *confv1.Bootstrap) kitregistry.Options {
	consul := conf.GetDiscovery().GetConsul()
	check := consul.GetCheck()
	ttl := check.GetTtl()
	return kitregistry.Options{
		Enabled:       consul != nil && consul.GetAddr() != "",
		Address:       consul.GetAddr(),
		ServerAddress: conf.GetServer().GetAddr(),
		TLS: kitregistry.TLSOptions{
			Enabled:            consul.GetTls().GetEnable(),
			InsecureSkipVerify: consul.GetTls().GetInsecureSkipVerify(),
			CAPEM:              consul.GetTls().GetCaPem(),
		},
		Check: kitregistry.CheckOptions{
			TTL: kitregistry.TTLCheckOptions{
				Enabled:      ttl != nil,
				Duration:     ttl.GetDuration(),
				PingInterval: ttl.GetPingInterval().AsDuration(),
			},
			GRPC: &kitregistry.GRPCCheckOptions{
				Interval: ttl.GetPingInterval().AsDuration(),
			},
			DeregisterCriticalServiceAfter: check.GetDeregisterCriticalServiceAfter(),
		},
	}
}
