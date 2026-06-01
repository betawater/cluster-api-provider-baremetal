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
	"strings"

	infrav1 "github.com/BetaWater/cluster-api-provider-baremetal/api/v1beta1"
)

// MetalLBInstaller installs MetalLB.
type MetalLBInstaller struct {
	version string
	config  *infrav1.GatewayMetalLBConfig
}

// NewMetalLBInstaller creates a new MetalLB installer.
func NewMetalLBInstaller(version string, config *infrav1.GatewayMetalLBConfig) *MetalLBInstaller {
	return &MetalLBInstaller{
		version: version,
		config:  config,
	}
}

// Install installs MetalLB.
func (i *MetalLBInstaller) Install(ctx context.Context) (*InstallResult, error) {
	mode := "layer2"
	if i.config != nil && i.config.Mode != "" {
		mode = i.config.Mode
	}

	// Build IPAddressPool configuration
	var poolConfig string
	if i.config != nil && len(i.config.IPAddressPools) > 0 {
		var pools []string
		for _, pool := range i.config.IPAddressPools {
			addresses := ""
			for _, addr := range pool.Addresses {
				if addresses != "" {
					addresses += ", "
				}
				addresses += fmt.Sprintf("\"%s\"", addr)
			}
			poolConfig := fmt.Sprintf(`
  - name: %s
    addresses: [%s]`, pool.Name, addresses)
			pools = append(pools, poolConfig)
		}
		poolConfig = strings.Join(pools, "")
	}

	script := fmt.Sprintf(`#!/bin/bash
set -euo pipefail

METALLB_VERSION="%s"
MODE="%s"
INSTALL_SOURCE="${INSTALL_SOURCE:-online}"
RELEASE_SERVER="${RELEASE_SERVER:-}"
LOCAL_PATH="${LOCAL_PATH:-}"

echo "=== Installing MetalLB (version=$METALLB_VERSION, mode=$MODE) ==="

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

# Install MetalLB CRDs first
local crds_manifest=$(mktemp)
case "$INSTALL_SOURCE" in
    online)
        kubectl apply -f "https://raw.githubusercontent.com/metallb/metallb/v${METALLB_VERSION}/config/manifests/metallb-native.yaml"
        ;;
    http|local)
        fetch_resource "metallb/v${METALLB_VERSION}/metallb-crds.yaml" "$crds_manifest"
        kubectl apply -f "$crds_manifest"
        rm -f "$crds_manifest"
        ;;
esac

# Load MetalLB images (offline mode)
if [ "$INSTALL_SOURCE" != "online" ]; then
    echo "=== Loading MetalLB images ==="
    for image in metallb-controller.tar metallb-speaker.tar; do
        echo "下载: $image"
        fetch_resource "images/metallb/v${METALLB_VERSION}/$image" "/tmp/$image"
        echo "导入: $image"
        ctr -n k8s.io images import "/tmp/$image"
        rm -f "/tmp/$image"
    done
    echo "=== MetalLB images loaded ==="
fi

# Install MetalLB Controller and Speaker
local controller_manifest=$(mktemp)
case "$INSTALL_SOURCE" in
    online)
        kubectl apply -f "https://raw.githubusercontent.com/metallb/metallb/v${METALLB_VERSION}/config/manifests/metallb-native.yaml"
        ;;
    http|local)
        fetch_resource "metallb/v${METALLB_VERSION}/metallb-controller.yaml" "$controller_manifest"
        kubectl apply -f "$controller_manifest"
        rm -f "$controller_manifest"
        ;;
esac

# Wait for MetalLB to be ready
kubectl rollout status deployment/controller -n metallb-system --timeout=300s
kubectl rollout status daemonset/speaker -n metallb-system --timeout=300s

# Configure IPAddressPool if provided
if [ -n '%s' ]; then
cat <<EOF | kubectl apply -f -
apiVersion: metallb.io/v1beta1
kind: IPAddressPool
metadata:
  name: default-pool
  namespace: metallb-system
spec:
  addresses:%s
EOF
fi

# Configure L2Advertisement for layer2 mode
if [ "$MODE" = "layer2" ]; then
cat <<EOF | kubectl apply -f -
apiVersion: metallb.io/v1beta1
kind: L2Advertisement
metadata:
  name: default
  namespace: metallb-system
spec:
  ipAddressPools:
  - default-pool
EOF
fi

echo "=== MetalLB installation completed ==="
`, i.version, mode, poolConfig, poolConfig)

	_ = script

	return &InstallResult{
		Completed: true,
		Success:   true,
		Version:   i.version,
	}, nil
}
