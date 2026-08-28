package healthcheck

import (
	"context"
	"fmt"
	"net/http"

	"connectrpc.com/connect"
	"connectrpc.com/grpchealth"
)

// ReadyFunc reports whether the process can currently serve application traffic.
type ReadyFunc func(context.Context) bool

type checker struct {
	ready    ReadyFunc
	services map[string]struct{}
}

// NewGRPCHandler exposes deep readiness through the standard gRPC health protocol.
// An empty service name represents the whole process; named services must be listed.
func NewGRPCHandler(ready ReadyFunc, services ...string) (string, http.Handler) {
	if ready == nil {
		panic("healthcheck: nil readiness function")
	}
	known := make(map[string]struct{}, len(services))
	for _, service := range services {
		if service != "" {
			known[service] = struct{}{}
		}
	}
	return grpchealth.NewHandler(&checker{ready: ready, services: known})
}

func (c *checker) Check(ctx context.Context, request *grpchealth.CheckRequest) (*grpchealth.CheckResponse, error) {
	service := ""
	if request != nil {
		service = request.Service
	}
	if service != "" {
		if _, ok := c.services[service]; !ok {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("unknown service %q", service))
		}
	}
	status := grpchealth.StatusNotServing
	if c.ready(ctx) {
		status = grpchealth.StatusServing
	}
	return &grpchealth.CheckResponse{Status: status}, nil
}
