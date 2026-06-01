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

func NewFromReleaseImage(sshConn *sshclient.SSHConnection, releaseImage *infrav1.ReleaseImage, config infrav1.CNIConfig, podCIDR string) *Installer {
	if config.Type == "" {
		if addon := findAddon(releaseImage, "calico"); addon != nil {
			config.Type = "calico"
			config.Version = addon.Version
		} else if addon := findAddon(releaseImage, "cilium"); addon != nil {
			config.Type = "cilium"
			config.Version = addon.Version
		}
	}
	if config.Version == "" && config.Type != "" {
		if addon := findAddon(releaseImage, config.Type); addon != nil {
			config.Version = addon.Version
		}
	}
	return &Installer{
		sshConn: sshConn,
		config:  config,
		podCIDR: podCIDR,
	}
}

// findAddon finds an addon by name in the ReleaseImage.
func findAddon(releaseImage *infrav1.ReleaseImage, name string) *infrav1.AddonDefinition {
	for i := range releaseImage.Spec.Addons {
		if releaseImage.Spec.Addons[i].Name == name {
			return &releaseImage.Spec.Addons[i]
		}
	}
	return nil
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

	script := i.generateInstallScript()

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

func (i *Installer) generateInstallScript() string {
	cniPluginsVersion := "1.3.0"

	installSource := "online"
	releaseServer := ""
	localPath := ""
	if i.config.AirGap != nil && i.config.AirGap.Enabled {
		switch i.config.AirGap.ManifestSource {
		case "HTTPServer":
			installSource = "http"
			if i.config.AirGap.HTTPServerConfig != nil {
				releaseServer = i.config.AirGap.HTTPServerConfig.BaseURL
			}
		case "LocalPath":
			installSource = "local"
			localPath = i.config.AirGap.LocalPath
		default:
			installSource = "http"
		}
	}

	script := `#!/bin/bash
set -euo pipefail

CNI_TYPE="${CNI_TYPE:-calico}"
CNI_VERSION="${CNI_VERSION}"
POD_CIDR="${POD_CIDR:-10.244.0.0/16}"
CNI_PLUGINS_VERSION="${CNI_PLUGINS_VERSION:-1.3.0}"
INSTALL_MODE="${INSTALL_MODE:-Manifest}"
INSTALL_SOURCE="${INSTALL_SOURCE:-online}"
RELEASE_SERVER="${RELEASE_SERVER:-}"
LOCAL_PATH="${LOCAL_PATH:-}"

echo "=== CNI 安装开始 (type=$CNI_TYPE, version=$CNI_VERSION, source=$INSTALL_SOURCE) ==="

fetch_resource() {
    local resource="$1"
    local dest="$2"
    case "$INSTALL_SOURCE" in
        online) curl -fsSL "$resource" -o "$dest" ;;
        http)   curl -fsSL "${RELEASE_SERVER}/${resource}" -o "$dest" ;;
        local)  cp "${LOCAL_PATH}/${resource}" "$dest" ;;
        *)      echo "ERROR: 不支持的安装源: $INSTALL_SOURCE"; exit 1 ;;
    esac
}

install_cni_plugins() {
    if [ -d "/opt/cni/bin" ] && [ "$(ls -A /opt/cni/bin 2>/dev/null)" ]; then
        echo "CNI 二进制插件已安装"
        return 0
    fi
    mkdir -p /opt/cni/bin

    local archive=$(mktemp)
    case "$INSTALL_SOURCE" in
        online)
            fetch_resource "https://github.com/containernetworking/plugins/releases/download/v${CNI_PLUGINS_VERSION}/cni-plugins-linux-amd64-v${CNI_PLUGINS_VERSION}.tgz" "$archive"
            ;;
        http|local)
            fetch_resource "cni/plugins/v${CNI_PLUGINS_VERSION}/linux-${ARCH}/cni-plugins-linux-${ARCH}-v${CNI_PLUGINS_VERSION}.tgz" "$archive"
            ;;
    esac
    tar -C /opt/cni/bin -xzf "$archive"
    rm -f "$archive"
    echo "CNI 二进制插件安装完成"
}

install_calico() {
    # 加载 Calico 镜像 (离线模式)
    if [ "$INSTALL_SOURCE" != "online" ]; then
        echo "=== 加载 Calico 镜像 ==="
        for image in calico-node.tar calico-kube-controllers.tar calico-cni.tar; do
            echo "下载: $image"
            fetch_resource "images/calico/v${CNI_VERSION}/$image" "/tmp/$image"
            echo "导入: $image"
            ctr -n k8s.io images import "/tmp/$image"
            rm -f "/tmp/$image"
        done
        echo "=== Calico 镜像加载完成 ==="
    fi

    local manifest=$(mktemp)
    case "$INSTALL_SOURCE" in
        online)
            fetch_resource "https://raw.githubusercontent.com/projectcalico/calico/v${CNI_VERSION}/manifests/calico.yaml" "$manifest"
            ;;
        http|local)
            fetch_resource "cni/calico/v${CNI_VERSION}/calico.yaml" "$manifest"
            ;;
    esac
    sed -i "s|\"192.168.0.0/16\"|\"${POD_CIDR}\"|g" "$manifest"
    kubectl apply -f "$manifest"
    rm -f "$manifest"
    kubectl rollout status daemonset/calico-node -n kube-system --timeout=300s
    echo "Calico 部署完成"
}

install_cilium_helm() {
    # 加载 Cilium 镜像 (离线模式)
    if [ "$INSTALL_SOURCE" != "online" ]; then
        echo "=== 加载 Cilium 镜像 ==="
        for image in cilium.tar cilium-operator.tar hubble-relay.tar; do
            echo "下载: $image"
            fetch_resource "images/cilium/v${CNI_VERSION}/$image" "/tmp/$image"
            echo "导入: $image"
            ctr -n k8s.io images import "/tmp/$image"
            rm -f "/tmp/$image"
        done
        echo "=== Cilium 镜像加载完成 ==="
    fi

    case "$INSTALL_SOURCE" in
        online)
            helm repo add cilium https://helm.cilium.io/ || true
            helm repo update
            helm upgrade --install cilium cilium/cilium \
                --namespace kube-system --version "v${CNI_VERSION}" \
                --set ipam.mode=kubernetes \
                --set kubeProxyReplacement="${CILIUM_KUBE_PROXY_REPLACEMENT:-partial}" \
                --wait --timeout=300s
            ;;
        http|local)
            local chart=$(mktemp)
            fetch_resource "cni/cilium/v${CNI_VERSION}/cilium.tgz" "$chart"
            helm upgrade --install cilium "$chart" --namespace kube-system \
                --set ipam.mode=kubernetes \
                --set kubeProxyReplacement="${CILIUM_KUBE_PROXY_REPLACEMENT:-partial}" \
                --wait --timeout=300s
            rm -f "$chart"
            ;;
    esac
    echo "Cilium 部署完成"
}

install_flannel() {
    # 加载 Flannel 镜像 (离线模式)
    if [ "$INSTALL_SOURCE" != "online" ]; then
        echo "=== 加载 Flannel 镜像 ==="
        for image in flannel.tar; do
            echo "下载: $image"
            fetch_resource "images/flannel/v${CNI_VERSION}/$image" "/tmp/$image"
            echo "导入: $image"
            ctr -n k8s.io images import "/tmp/$image"
            rm -f "/tmp/$image"
        done
        echo "=== Flannel 镜像加载完成 ==="
    fi

    local manifest=$(mktemp)
    case "$INSTALL_SOURCE" in
        online)
            fetch_resource "https://github.com/flannel-io/flannel/releases/download/v${CNI_VERSION}/kube-flannel.yml" "$manifest"
            ;;
        http|local)
            fetch_resource "cni/flannel/v${CNI_VERSION}/flannel.yaml" "$manifest"
            ;;
    esac
    sed -i "s|\"10.244.0.0/16\"|\"${POD_CIDR}\"|g" "$manifest"
    kubectl apply -f "$manifest"
    rm -f "$manifest"
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
		"INSTALL_MODE":        i.config.InstallMode,
		"INSTALL_SOURCE":      installSource,
	}

	if releaseServer != "" {
		envVars["RELEASE_SERVER"] = releaseServer
	}
	if localPath != "" {
		envVars["LOCAL_PATH"] = localPath
	}

	if i.config.Config != nil && i.config.Config.Cilium != nil {
		if i.config.Config.Cilium.KubeProxyReplacement != "" {
			envVars["CILIUM_KUBE_PROXY_REPLACEMENT"] = i.config.Config.Cilium.KubeProxyReplacement
		}
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
