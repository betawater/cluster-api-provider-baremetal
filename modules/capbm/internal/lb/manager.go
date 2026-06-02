/*
Copyright 2024 The CAPBM Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package lb

import (
	"context"
	"fmt"

	capbmv1 "github.com/BetaWater/cluster-api-provider-baremetal/modules/capbm/api/v1beta1"
)

// Backend represents a load balancer backend server.
type Backend struct {
	Name string
	IP   string
	Port int
}

// Provider defines the interface for load balancer providers.
type Provider interface {
	// RegisterBackend adds a backend server to the load balancer pool.
	RegisterBackend(ctx context.Context, backend Backend) error

	// UnregisterBackend removes a backend server from the load balancer pool.
	UnregisterBackend(ctx context.Context, backend Backend) error

	// GetBackends returns the current list of backend servers.
	GetBackends(ctx context.Context) ([]Backend, error)

	// HealthCheck checks if a backend is healthy.
	HealthCheck(ctx context.Context, backend Backend) (bool, error)
}

// NewProvider creates a new load balancer provider based on the configuration.
func NewProvider(config *capbmv1.LoadBalancerConfig) (Provider, error) {
	if config == nil {
		return nil, nil
	}

	switch config.Provider {
	case "haproxy":
		return NewHAProxyProvider(config.HAProxy)
	case "f5":
		return NewF5Provider(config.F5)
	case "keepalived":
		return NewKeepalivedProvider(config.Keepalived)
	case "metal-lb":
		return NewMetalLBProvider(config.MetalLB)
	case "":
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported load balancer provider: %s", config.Provider)
	}
}
