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

package cni

import (
	"context"
	"fmt"
	"strings"

	infrav1 "github.com/BetaWater/cluster-api-provider-baremetal/api/v1beta1"
	sshclient "github.com/BetaWater/cluster-api-provider-baremetal/internal/ssh"
)

type Installer struct {
	sshConn *sshclient.SSHConnection
	config  infrav1.CNIConfig
	podCIDR string
}

type InstallResult struct {
	Completed bool   `json:"completed"`
	Success   bool   `json:"success"`
	Version   string `json:"version,omitempty"`
	Error     string `json:"error,omitempty"`
}

func New(sshConn *sshclient.SSHConnection, config infrav1.CNIConfig, podCIDR string) *Installer {
	return &Installer{
		sshConn: sshConn,
		config:  config,
		podCIDR: podCIDR,
	}
}

// NewFromReleaseImage creates a CNI Installer with versions sourced from a ReleaseImage.
func NewFromReleaseImage(sshConn *sshclient.SSHConnection, releaseImage *infrav1.ReleaseImage, config infrav1.CNIConfig, podCIDR string) *Installer {
	// Derive CNI type and version from ReleaseImage components
	if config.Type == "" {
		if releaseImage.Spec.Components.Calico != "" {
			config.Type = "calico"
			config.Version = releaseImage.Spec.Components.Calico
		} else if releaseImage.Spec.Components.Cilium != "" {
			config.Type = "cilium"
			config.Version = releaseImage.Spec.Components.Cilium
		}
	}
	if config.Version == "" && config.Type != "" {
		switch config.Type {
		case "calico":
			config.Version = releaseImage.Spec.Components.Calico
		case "cilium":
			config.Version = releaseImage.Spec.Components.Cilium
		case "flannel":
			config.Version = releaseImage.Spec.Components.Kubernetes["flannel"]
		}
	}
	return &Installer{
		sshConn: sshConn,
		config:  config,
		podCIDR: podCIDR,
	}
}

func (i *Installer) Install(ctx context.Context) (*InstallResult, error) {
	if !i.config.Enabled {
		return &InstallResult{Completed: true, Success: true, Error: "CNI installation disabled"}, nil
	}

	if i.config.Type == "" {
		i.config.Type = "calico"
	}
	if i.config.InstallMode == "" {
		i.config.InstallMode = "Manifest"
	}

	existing, _ := i.checkExisting(ctx)
	if existing.installed && existing.version == i.config.Version {
		return &InstallResult{Completed: true, Success: true, Version: existing.version}, nil
	}

	var script string
	if i.config.AirGap != nil && i.config.AirGap.Enabled {
		script = i.generateOfflineInstallScript()
	} else {
		script = i.generateOnlineInstallScript()
	}

	result, err := i.sshConn.ExecuteScript(ctx, script)
	if err != nil {
		return &InstallResult{Completed: false, Success: false, Error: fmt.Sprintf("CNI installation failed: %v, stderr: %s", err, result.Stderr)}, err
	}

	if result.ExitCode != 0 {
		return &InstallResult{Completed: false, Success: false, Error: fmt.Sprintf("CNI installation failed with exit code %d: %s", result.ExitCode, result.Stderr)}, fmt.Errorf("CNI installation failed with exit code %d", result.ExitCode)
	}

	return &InstallResult{Completed: true, Success: true, Version: i.config.Version}, nil
}

type existingCNI struct {
	installed bool
	version   string
}

func (i *Installer) checkExisting(ctx context.Context) (*existingCNI, error) {
	ec := &existingCNI{}

	result, _ := i.sshConn.ExecuteCommand(ctx, "ls /opt/cni/bin/ 2>/dev/null | head -1 || echo ''")
	if strings.TrimSpace(result.Stdout) != "" {
		ec.installed = true
	}

	switch i.config.Type {
	case "calico":
		result, _ := i.sshConn.ExecuteCommand(ctx, "kubectl get daemonset calico-node -n kube-system -o jsonpath='{.spec.template.spec.containers[0].image}' 2>/dev/null || echo ''")
		if strings.Contains(result.Stdout, "v") {
			parts := strings.Split(result.Stdout, "v")
			if len(parts) > 1 {
				ec.version = strings.Split(parts[1], ":")[0]
			}
		}
	case "cilium":
		result, _ := i.sshConn.ExecuteCommand(ctx, "kubectl get deployment cilium-operator -n kube-system -o jsonpath='{.spec.template.spec.containers[0].image}' 2>/dev/null || echo ''")
		if strings.Contains(result.Stdout, "v") {
			parts := strings.Split(result.Stdout, "v")
			if len(parts) > 1 {
				ec.version = strings.Split(parts[1], ":")[0]
			}
		}
	case "flannel":
		result, _ := i.sshConn.ExecuteCommand(ctx, "kubectl get daemonset kube-flannel-ds -n kube-flannel -o jsonpath='{.spec.template.spec.containers[0].image}' 2>/dev/null || echo ''")
		if strings.Contains(result.Stdout, "v") {
			parts := strings.Split(result.Stdout, "v")
			if len(parts) > 1 {
				ec.version = strings.Split(parts[1], ":")[0]
			}
		}
	}

	return ec, nil
}

func (i *Installer) generateOnlineInstallScript() string {
	cniPluginsVersion := "1.3.0"

	script := `#!/bin/bash
set -euo pipefail

CNI_TYPE="${CNI_TYPE:-calico}"
CNI_VERSION="${CNI_VERSION}"
POD_CIDR="${POD_CIDR:-10.244.0.0/16}"
CNI_PLUGINS_VERSION="${CNI_PLUGINS_VERSION:-1.3.0}"

echo "=== CNI 安装开始 (type=$CNI_TYPE, version=$CNI_VERSION) ==="

install_cni_plugins() {
    if [ -d "/opt/cni/bin" ] && [ "$(ls -A /opt/cni/bin 2>/dev/null)" ]; then
        echo "CNI 二进制插件已安装"
        return 0
    fi
    mkdir -p /opt/cni/bin
    curl -fsSL "https://github.com/containernetworking/plugins/releases/download/v${CNI_PLUGINS_VERSION}/cni-plugins-linux-amd64-v${CNI_PLUGINS_VERSION}.tgz" | tar -C /opt/cni/bin -xz
    echo "CNI 二进制插件安装完成"
}

install_calico() {
    local manifest_url="https://raw.githubusercontent.com/projectcalico/calico/v${CNI_VERSION}/manifests/calico.yaml"
    local temp_manifest=$(mktemp)
    curl -fsSL "$manifest_url" -o "$temp_manifest"
    sed -i "s|\"192.168.0.0/16\"|\"${POD_CIDR}\"|g" "$temp_manifest"
    kubectl apply -f "$temp_manifest"
    rm -f "$temp_manifest"
    kubectl rollout status daemonset/calico-node -n kube-system --timeout=300s
    echo "Calico 部署完成"
}

install_cilium_helm() {
    helm repo add cilium https://helm.cilium.io/ || true
    helm repo update
    helm upgrade --install cilium cilium/cilium \
        --namespace kube-system --version "v${CNI_VERSION}" \
        --set ipam.mode=kubernetes \
        --set kubeProxyReplacement="${CILIUM_KUBE_PROXY_REPLACEMENT:-partial}" \
        --wait --timeout=300s
    echo "Cilium 部署完成"
}

install_flannel() {
    local manifest_url="https://github.com/flannel-io/flannel/releases/download/v${CNI_VERSION}/kube-flannel.yml"
    local temp_manifest=$(mktemp)
    curl -fsSL "$manifest_url" -o "$temp_manifest"
    sed -i "s|\"10.244.0.0/16\"|\"${POD_CIDR}\"|g" "$temp_manifest"
    kubectl apply -f "$temp_manifest"
    rm -f "$temp_manifest"
    kubectl rollout status daemonset/kube-flannel-ds -n kube-flannel --timeout=300s
    echo "Flannel 部署完成"
}

verify_cni() {
    [ -d "/opt/cni/bin" ] && [ -n "$(ls -A /opt/cni/bin 2>/dev/null)" ] && echo "CNI 二进制: OK" || { echo "ERROR: /opt/cni/bin 为空"; return 1; }
    [ -d "/etc/cni/net.d" ] && [ -n "$(ls -A /etc/cni/net.d 2>/dev/null)" ] && echo "CNI 配置: OK" || { echo "ERROR: /etc/cni/net.d 为空"; return 1; }
    local status=$(kubectl get node $(hostname) -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || echo "Unknown")
    [ "$status" = "True" ] && echo "Node Ready: OK" || echo "WARNING: Node 尚未 Ready"
    echo "CNI 验证完成"
}

install_cni_plugins
case "$CNI_TYPE" in
    calico)  install_calico ;;
    cilium)  install_cilium_helm ;;
    flannel) install_flannel ;;
    *) echo "ERROR: 不支持的 CNI 类型: $CNI_TYPE"; exit 1 ;;
esac
verify_cni
echo "=== CNI 安装完成 ==="
`

	envVars := map[string]string{
		"CNI_TYPE":            i.config.Type,
		"CNI_VERSION":         i.config.Version,
		"POD_CIDR":            i.podCIDR,
		"CNI_PLUGINS_VERSION": cniPluginsVersion,
	}

	if i.config.Config != nil && i.config.Config.Cilium != nil {
		if i.config.Config.Cilium.KubeProxyReplacement != "" {
			envVars["CILIUM_KUBE_PROXY_REPLACEMENT"] = i.config.Config.Cilium.KubeProxyReplacement
		}
	}

	return prependEnvVars(script, envVars)
}

func (i *Installer) generateOfflineInstallScript() string {
	airGap := i.config.AirGap
	pluginsArchive := "/opt/capbm/cni/cni-plugins-linux-amd64-v1.3.0.tgz"
	manifestPath := "/opt/capbm/cni/calico.yaml"
	chartArchive := "/opt/capbm/cni/calico-" + i.config.Version + ".tgz"

	if airGap != nil {
		if airGap.CNIPluginsArchive != "" {
			pluginsArchive = airGap.CNIPluginsArchive
		}
		if airGap.LocalPath != "" {
			manifestPath = airGap.LocalPath + "/calico.yaml"
			chartArchive = airGap.LocalPath + "/calico-" + i.config.Version + ".tgz"
		}
		if airGap.ChartArchive != "" {
			chartArchive = airGap.ChartArchive
		}
	}

	script := `#!/bin/bash
set -euo pipefail

CNI_TYPE="${CNI_TYPE:-calico}"
CNI_VERSION="${CNI_VERSION}"
CNI_PLUGINS_ARCHIVE="${CNI_PLUGINS_ARCHIVE}"
CNI_MANIFEST_PATH="${CNI_MANIFEST_PATH}"
CNI_CHART_ARCHIVE="${CNI_CHART_ARCHIVE}"
INSTALL_MODE="${INSTALL_MODE:-Manifest}"

echo "=== CNI 离线安装开始 ==="

install_cni_plugins_offline() {
    [ -d "/opt/cni/bin" ] && [ -n "$(ls -A /opt/cni/bin 2>/dev/null)" ] && { echo "CNI 二进制已安装"; return 0; }
    [ ! -f "$CNI_PLUGINS_ARCHIVE" ] && { echo "ERROR: CNI 二进制插件包不存在: $CNI_PLUGINS_ARCHIVE"; return 1; }
    mkdir -p /opt/cni/bin && tar -C /opt/cni/bin -xzf "$CNI_PLUGINS_ARCHIVE"
    echo "CNI 二进制离线安装完成"
}

install_cni_manifest_offline() {
    [ ! -f "$CNI_MANIFEST_PATH" ] && { echo "ERROR: Manifest 不存在: $CNI_MANIFEST_PATH"; return 1; }
    kubectl apply -f "$CNI_MANIFEST_PATH"
    case "$CNI_TYPE" in
        calico)  kubectl rollout status daemonset/calico-node -n kube-system --timeout=300s ;;
        flannel) kubectl rollout status daemonset/kube-flannel-ds -n kube-flannel --timeout=300s ;;
    esac
    echo "CNI 离线部署完成 (Manifest)"
}

install_cni_helm_offline() {
    [ ! -f "$CNI_CHART_ARCHIVE" ] && { echo "ERROR: Helm Chart 不存在: $CNI_CHART_ARCHIVE"; return 1; }
    helm upgrade --install "$CNI_TYPE" "$CNI_CHART_ARCHIVE" --namespace kube-system --wait --timeout=300s
    echo "CNI 离线部署完成 (Helm)"
}

install_cni_plugins_offline
case "$INSTALL_MODE" in
    Manifest) install_cni_manifest_offline ;;
    Helm)     install_cni_helm_offline ;;
esac
echo "=== CNI 离线安装完成 ==="
`

	envVars := map[string]string{
		"CNI_TYPE":            i.config.Type,
		"CNI_VERSION":         i.config.Version,
		"CNI_PLUGINS_ARCHIVE": pluginsArchive,
		"CNI_MANIFEST_PATH":   manifestPath,
		"CNI_CHART_ARCHIVE":   chartArchive,
		"INSTALL_MODE":        i.config.InstallMode,
	}

	return prependEnvVars(script, envVars)
}

func prependEnvVars(script string, envVars map[string]string) string {
	for key, val := range envVars {
		if val != "" {
			script = fmt.Sprintf("%s=%q\n%s", key, val, script)
		}
	}
	return script
}
