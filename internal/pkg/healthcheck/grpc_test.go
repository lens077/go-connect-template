package healthcheck

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"connectrpc.com/connect"
	"connectrpc.com/grpchealth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

func TestGRPCHandlerReportsDeepReadiness(t *testing.T) {
	var ready atomic.Bool
	path, handler := NewGRPCHandler(func(context.Context) bool {
		return ready.Load()
	}, "example.v1.ExampleService")
	assert.Equal(t, "/grpc.health.v1.Health/", path)

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client := grpchealth.NewClient(server.Client(), server.URL)

	response, err := client.Check(t.Context(), &grpchealth.CheckRequest{})
	require.NoError(t, err)
	assert.Equal(t, grpchealth.StatusNotServing, response.Status)

	ready.Store(true)
	response, err = client.Check(t.Context(), &grpchealth.CheckRequest{Service: "example.v1.ExampleService"})
	require.NoError(t, err)
	assert.Equal(t, grpchealth.StatusServing, response.Status)
}

func TestGRPCHandlerSupportsPlaintextGRPC(t *testing.T) {
	_, handler := NewGRPCHandler(func(context.Context) bool { return true }, "example.v1.ExampleService")
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	server := &http.Server{Handler: h2c.NewHandler(handler, &http2.Server{})}
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(func() {
		require.NoError(t, server.Shutdown(context.Background()))
	})

	transport := &http2.Transport{
		AllowHTTP: true,
		DialTLSContext: func(ctx context.Context, network, address string, _ *tls.Config) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, address)
		},
	}
	client := grpchealth.NewClient(
		&http.Client{Transport: transport},
		"http://"+listener.Addr().String(),
		connect.WithGRPC(),
	)
	response, err := client.Check(t.Context(), &grpchealth.CheckRequest{})
	require.NoError(t, err)
	assert.Equal(t, grpchealth.StatusServing, response.Status)
}

func TestGRPCHandlerRejectsUnknownService(t *testing.T) {
	_, handler := NewGRPCHandler(func(context.Context) bool { return true }, "example.v1.ExampleService")
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	client := grpchealth.NewClient(server.Client(), server.URL)
	_, err := client.Check(t.Context(), &grpchealth.CheckRequest{Service: "missing.v1.Service"})
	require.Error(t, err)
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

func TestGRPCHandlerRequiresReadinessFunction(t *testing.T) {
	assert.PanicsWithValue(t, "healthcheck: nil readiness function", func() {
		NewGRPCHandler(nil)
	})
}
