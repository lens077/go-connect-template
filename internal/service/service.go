package service

import (
	"go.uber.org/fx"
)

var Module = fx.Module("service",
	fx.Provide(
		NewSearchService, // +co:example
		// +co:anchor service-providers
	),
)
