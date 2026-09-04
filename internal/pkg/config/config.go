package config

import (
	controlsource "github.com/lens077/control-tower/sdk/configsource" // +co:config-configcenter
	kitconfig "github.com/lens077/go-connect-kit/config"
	confv1 "github.com/lens077/go-connect-template/internal/conf/v1"
	"google.golang.org/protobuf/proto"
)

// Live is this generated service's concrete go-connect-kit configuration value.
type Live = kitconfig.Live[*confv1.Bootstrap]

// Module keeps only the service-owned protobuf projection in the generated tree.
var Module = kitconfig.Module[*confv1.Bootstrap](
	newSource,
	kitconfig.LoadOptions{},
	restartRequiredSections,
)

func newSource() (kitconfig.Source, error) {
	var selector kitconfig.SelectorFactory
	// +co:begin config-configcenter
	selector = controlsource.NewKitSource
	// +co:end
	return kitconfig.FromEnvironment(selector)
}

func restartRequiredSections(conf *confv1.Bootstrap) []kitconfig.RestartRequiredSection {
	return []kitconfig.RestartRequiredSection{
		{Name: "server", Message: proto.Message(conf.GetServer())},
		{Name: "discovery", Message: proto.Message(conf.GetDiscovery())},
		{Name: "observability", Message: proto.Message(conf.GetObservability())},
		{Name: "search", Message: proto.Message(conf.GetSearch())},
	}
}
