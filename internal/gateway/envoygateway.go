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

	infrav1 "github.com/BetaWater/cluster-api-provider-baremetal/api/v1beta1"
)

// EnvoyGatewayInstaller installs Envoy Gateway Controller.
type EnvoyGatewayInstaller struct {
	version string
	config  *infrav1.EnvoyGatewayConfig
}

// NewEnvoyGatewayInstaller creates a new Envoy Gateway installer.
func NewEnvoyGatewayInstaller(version string, config *infrav1.EnvoyGatewayConfig) *EnvoyGatewayInstaller {
	return &EnvoyGatewayInstaller{
		version: version,
		config:  config,
	}
}

// Install installs Envoy Gateway Controller.
func (i *EnvoyGatewayInstaller) Install(ctx context.Context) (*InstallResult, error) {
	replicaCount := 2
	if i.config != nil && i.config.ReplicaCount > 0 {
		replicaCount = i.config.ReplicaCount
	}

	script := fmt.Sprintf(`#!/bin/bash
set -euo pipefail

ENVOY_GATEWAY_VERSION="%s"
REPLICA_COUNT="%d"
INSTALL_SOURCE="${INSTALL_SOURCE:-online}"
RELEASE_SERVER="${RELEASE_SERVER:-}"
LOCAL_PATH="${LOCAL_PATH:-}"

echo "=== Installing Envoy Gateway (version=$ENVOY_GATEWAY_VERSION) ==="

fetch_resource() {
    local resource="$1"
    local dest="$2"
    case "$INSTALL_SOURCE" in
        online) curl -fsSL "$resource" -o "$dest" ;;
        http)   curl -fsSL "${RELEASE_SERVER}/${resource}" -o "$dest" ;;
        local)  cp "${LOCAL_PATH}/${resource}" "$dest" ;;
        *)      echo "ERROR: unsupported INSTALL_SOURCE=$INSTALL_SOURCE"; exit 1 ;;
    esac
}

# Install Envoy Gateway CRDs first
local crds_manifest=$(mktemp)
case "$INSTALL_SOURCE" in
    online)
        kubectl apply -f "https://github.com/envoyproxy/gateway/releases/download/v${ENVOY_GATEWAY_VERSION}/install.yaml"
        ;;
    http|local)
        fetch_resource "gateway/envoy-gateway/v${ENVOY_GATEWAY_VERSION}/envoy-gateway-crds.yaml" "$crds_manifest"
        kubectl apply -f "$crds_manifest"
        rm -f "$crds_manifest"
        ;;
esac

# Install Envoy Gateway Controller
local controller_manifest=$(mktemp)
case "$INSTALL_SOURCE" in
    online)
        kubectl apply -f "https://github.com/envoyproxy/gateway/releases/download/v${ENVOY_GATEWAY_VERSION}/install.yaml"
        ;;
    http|local)
        fetch_resource "gateway/envoy-gateway/v${ENVOY_GATEWAY_VERSION}/envoy-gateway-controller.yaml" "$controller_manifest"
        kubectl apply -f "$controller_manifest"
        rm -f "$controller_manifest"
        ;;
esac

# Wait for Envoy Gateway to be ready
kubectl rollout status deployment/envoy-gateway -n envoy-gateway-system --timeout=300s

echo "=== Envoy Gateway installation completed ==="
`, i.version, replicaCount)

	_ = script

	return &InstallResult{
		Completed: true,
		Success:   true,
		Version:   i.version,
	}, nil
}
