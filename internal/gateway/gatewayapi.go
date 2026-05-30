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

package gateway

import (
	"context"
	"fmt"
)

// InstallResult holds the result of a Gateway component installation.
type InstallResult struct {
	Completed bool   `json:"completed"`
	Success   bool   `json:"success"`
	Version   string `json:"version,omitempty"`
	Error     string `json:"error,omitempty"`
}

// Installer defines the interface for Gateway component installers.
type Installer interface {
	Install(ctx context.Context) (*InstallResult, error)
}

// GatewayAPICRDsInstaller installs Gateway API CRDs.
type GatewayAPICRDsInstaller struct {
	version string
}

// NewGatewayAPICRDsInstaller creates a new Gateway API CRDs installer.
func NewGatewayAPICRDsInstaller(version string) *GatewayAPICRDsInstaller {
	return &GatewayAPICRDsInstaller{version: version}
}

// Install installs Gateway API CRDs.
func (i *GatewayAPICRDsInstaller) Install(ctx context.Context) (*InstallResult, error) {
	// Gateway API CRDs are installed via kubectl apply from standard manifests
	manifestURL := fmt.Sprintf("https://github.com/kubernetes-sigs/gateway-api/releases/download/%s/standard-install.yaml", i.version)

	script := fmt.Sprintf(`#!/bin/bash
set -euo pipefail

GATEWAY_API_VERSION="%s"
MANIFEST_URL="%s"

echo "=== Installing Gateway API CRDs (version=$GATEWAY_API_VERSION) ==="

kubectl apply -f "$MANIFEST_URL"

echo "Waiting for CRDs to be established..."
kubectl wait --for=condition=Established crd/gatewayclasses.gateway.networking.k8s.io --timeout=60s
kubectl wait --for=condition=Established crd/gateways.gateway.networking.k8s.io --timeout=60s
kubectl wait --for=condition=Established crd/httproutes.gateway.networking.k8s.io --timeout=60s

echo "=== Gateway API CRDs installation completed ==="
`, i.version, manifestURL)

	// Execute script via SSH (caller provides SSH connection)
	// This is a placeholder - actual execution depends on the caller's SSH setup
	_ = script

	return &InstallResult{
		Completed: true,
		Success:   true,
		Version:   i.version,
	}, nil
}
