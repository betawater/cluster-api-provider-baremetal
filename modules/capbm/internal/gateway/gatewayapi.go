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

	"github.com/BetaWater/cluster-api-provider-baremetal/modules/cvo/pkg/ssh"
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
	sshConn *ssh.SSHConnection
}

// NewGatewayAPICRDsInstaller creates a new Gateway API CRDs installer.
func NewGatewayAPICRDsInstaller(version string) *GatewayAPICRDsInstaller {
	return &GatewayAPICRDsInstaller{version: version}
}

// WithSSHConnection sets the SSH connection for executing install scripts.
func (i *GatewayAPICRDsInstaller) WithSSHConnection(conn *ssh.SSHConnection) *GatewayAPICRDsInstaller {
	i.sshConn = conn
	return i
}

// Install installs Gateway API CRDs.
func (i *GatewayAPICRDsInstaller) Install(ctx context.Context) (*InstallResult, error) {
	// Gateway API CRDs are installed via kubectl apply from standard manifests
	manifestURL := fmt.Sprintf("https://github.com/kubernetes-sigs/gateway-api/releases/download/%s/standard-install.yaml", i.version)

	script := fmt.Sprintf(`#!/bin/bash
set -euo pipefail

GATEWAY_API_VERSION="%s"
MANIFEST_URL="%s"
INSTALL_SOURCE="${INSTALL_SOURCE:-online}"
RELEASE_SERVER="${RELEASE_SERVER:-}"
LOCAL_PATH="${LOCAL_PATH:-}"

echo "=== Installing Gateway API CRDs (version=$GATEWAY_API_VERSION) ==="

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

manifest=$(mktemp)
case "$INSTALL_SOURCE" in
    online)
        kubectl apply -f "$MANIFEST_URL"
        ;;
    http|local)
        fetch_resource "gateway/gateway-api/v${GATEWAY_API_VERSION}/gateway-api-crds.yaml" "$manifest"
        kubectl apply -f "$manifest"
        rm -f "$manifest"
        ;;
esac

echo "Waiting for CRDs to be established..."
kubectl wait --for=condition=Established crd/gatewayclasses.gateway.networking.k8s.io --timeout=60s
kubectl wait --for=condition=Established crd/gateways.gateway.networking.k8s.io --timeout=60s
kubectl wait --for=condition=Established crd/httproutes.gateway.networking.k8s.io --timeout=60s

echo "=== Gateway API CRDs installation completed ==="
`, i.version, manifestURL)

	// Execute script via SSH if connection is available
	if i.sshConn != nil {
		res, err := i.sshConn.ExecuteScript(ctx, script)
		if err != nil {
			return &InstallResult{
				Completed: true,
				Success:   false,
				Version:   i.version,
				Error:     fmt.Sprintf("failed to execute install script: %v", err),
			}, nil
		}
		if res.ExitCode != 0 {
			return &InstallResult{
				Completed: true,
				Success:   false,
				Version:   i.version,
				Error:     fmt.Sprintf("install script failed: %s", res.Stderr),
			}, nil
		}
	}

	return &InstallResult{
		Completed: true,
		Success:   true,
		Version:   i.version,
	}, nil
}
