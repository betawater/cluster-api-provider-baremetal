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
	"net"
	"time"

	capbmv1 "github.com/BetaWater/cluster-api-provider-baremetal/modules/capbm/api/v1beta1"
)

// KeepalivedProvider implements the Provider interface for Keepalived.
type KeepalivedProvider struct {
	config *capbmv1.KeepalivedConfig
}

// NewKeepalivedProvider creates a new Keepalived provider.
func NewKeepalivedProvider(config *capbmv1.KeepalivedConfig) (Provider, error) {
	if config == nil {
		return nil, fmt.Errorf("keepalived configuration is required")
	}
	if config.VirtualIP == "" {
		return nil, fmt.Errorf("keepalived virtualIP is required")
	}
	if config.Interface == "" {
		config.Interface = "eth0"
	}
	if config.VirtualRouterID == 0 {
		config.VirtualRouterID = 51
	}
	if config.Priority == 0 {
		config.Priority = 100
	}
	if config.AdvertInterval == 0 {
		config.AdvertInterval = 1
	}
	return &KeepalivedProvider{config: config}, nil
}

// RegisterBackend adds a backend server. For Keepalived, this is a no-op since
// Keepalived manages VIP failover, not backend registration.
func (p *KeepalivedProvider) RegisterBackend(ctx context.Context, backend Backend) error {
	return nil
}

// UnregisterBackend removes a backend server. For Keepalived, this is a no-op.
func (p *KeepalivedProvider) UnregisterBackend(ctx context.Context, backend Backend) error {
	return nil
}

// GetBackends returns the current list of backend servers.
func (p *KeepalivedProvider) GetBackends(ctx context.Context) ([]Backend, error) {
	return nil, nil
}

// HealthCheck checks if a backend is healthy.
func (p *KeepalivedProvider) HealthCheck(ctx context.Context, backend Backend) (bool, error) {
	addr := net.JoinHostPort(backend.IP, fmt.Sprintf("%d", backend.Port))
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return false, nil
	}
	conn.Close()
	return true, nil
}
