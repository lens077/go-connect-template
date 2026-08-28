package healthcheck

import (
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/hashicorp/consul/api"
)

const (
	defaultConsulCheckInterval = 10 * time.Second
	// The slowest current readiness path is serial: 2s Postgres + 2s Redis + 5s Gorse.
	consulCheckTimeout = 12 * time.Second
)

// ConsulTTLCheckID returns the stable ID shared by registration and the TTL pinger.
func ConsulTTLCheckID(serviceID string) string {
	return fmt.Sprintf("service:%s", serviceID)
}

// NewConsulTTLCheck builds the process-liveness check that owns auto-deregistration.
func NewConsulTTLCheck(serviceID, ttl, deregisterAfter string) *api.AgentServiceCheck {
	return &api.AgentServiceCheck{
		CheckID:                        ConsulTTLCheckID(serviceID),
		Name:                           "TTL process liveness",
		TTL:                            ttl,
		DeregisterCriticalServiceAfter: deregisterAfter,
	}
}

// NewConsulGRPCCheck builds a recoverable deep-readiness check. It deliberately
// omits DeregisterCriticalServiceAfter so a dependency outage cannot permanently
// remove a live process from Consul; the TTL check owns deregistration.
func NewConsulGRPCCheck(serviceID, host string, port int, interval time.Duration) *api.AgentServiceCheck {
	if interval <= 0 {
		interval = defaultConsulCheckInterval
	}
	return &api.AgentServiceCheck{
		CheckID:                fmt.Sprintf("service:%s:grpc-readiness", serviceID),
		Name:                   "gRPC deep readiness",
		GRPC:                   net.JoinHostPort(host, strconv.Itoa(port)),
		Interval:               interval.String(),
		Timeout:                consulCheckTimeout.String(),
		SuccessBeforePassing:   1,
		FailuresBeforeCritical: 3,
	}
}
