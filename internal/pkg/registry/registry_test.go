package registry

import (
	"testing"
	"time"

	confv1 "github.com/lens077/go-connect-template/internal/conf/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/durationpb"
)

func TestOptionsFromBootstrapKeepsGRPCReadiness(t *testing.T) {
	options := optionsFromBootstrap(&confv1.Bootstrap{
		Server: &confv1.Server{Addr: "0.0.0.0:30006"},
		Discovery: &confv1.Discovery{Consul: &confv1.Discovery_Consul{
			Addr: "consul:8500",
			Check: &confv1.Discovery_Consul_Check{
				Ttl: &confv1.Discovery_Consul_Check_TTL{
					Duration:     "30s",
					PingInterval: durationpb.New(10 * time.Second),
				},
				DeregisterCriticalServiceAfter: "1m",
			},
		}},
	})

	require.True(t, options.Enabled)
	require.Equal(t, "consul:8500", options.Address)
	require.Equal(t, "0.0.0.0:30006", options.ServerAddress)
	require.NotNil(t, options.Check.GRPC)
	require.Equal(t, 10*time.Second, options.Check.GRPC.Interval)
}
