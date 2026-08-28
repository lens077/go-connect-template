package healthcheck

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewConsulTTLCheckUsesStableID(t *testing.T) {
	check := NewConsulTTLCheck("cart-abc", "30s", "1m")

	assert.Equal(t, "service:cart-abc", check.CheckID)
	assert.Equal(t, "TTL process liveness", check.Name)
	assert.Equal(t, "30s", check.TTL)
	assert.Equal(t, "1m", check.DeregisterCriticalServiceAfter)
}

func TestNewConsulGRPCCheck(t *testing.T) {
	check := NewConsulGRPCCheck("cart-abc", "10.0.0.7", 30006, 7*time.Second)

	assert.Equal(t, "service:cart-abc:grpc-readiness", check.CheckID)
	assert.Equal(t, "gRPC deep readiness", check.Name)
	assert.Equal(t, "10.0.0.7:30006", check.GRPC)
	assert.Equal(t, "7s", check.Interval)
	assert.Equal(t, "12s", check.Timeout)
	assert.Equal(t, 1, check.SuccessBeforePassing)
	assert.Equal(t, 3, check.FailuresBeforeCritical)
	assert.Empty(t, check.DeregisterCriticalServiceAfter)
}

func TestNewConsulGRPCCheckDefaultsIntervalAndFormatsIPv6(t *testing.T) {
	check := NewConsulGRPCCheck("user-abc", "fd00::7", 30001, 0)

	assert.Equal(t, "[fd00::7]:30001", check.GRPC)
	assert.Equal(t, "10s", check.Interval)
}
