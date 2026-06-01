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

package csi

import (
	"context"
	"fmt"
	"strings"

	capbmv1 "github.com/BetaWater/cluster-api-provider-baremetal/api/capbm/v1beta1"
	commonv1 "github.com/BetaWater/cluster-api-provider-baremetal/api/common/v1beta1"
	cfov1 "github.com/BetaWater/cluster-api-provider-baremetal/api/cvo/v1beta1"
	sshclient "github.com/BetaWater/cluster-api-provider-baremetal/internal/capbm/ssh"
)

type Installer struct {
	sshConn *sshclient.SSHConnection
	config  capbmv1.CSIConfig
}

type InstallResult struct {
	Completed bool   `json:"completed"`
	Success   bool   `json:"success"`
	Version   string `json:"version,omitempty"`
	Error     string `json:"error,omitempty"`
}

func New(sshConn *sshclient.SSHConnection, config capbmv1.CSIConfig) *Installer {
	return &Installer{
		sshConn: sshConn,
		config:  config,
	}
}

func NewFromReleaseImage(sshConn *sshclient.SSHConnection, releaseImage *cfov1.ReleaseImage, config capbmv1.CSIConfig) *Installer {
	if config.Driver == "" {
		if addon := findCSIAddon(releaseImage, "ceph-csi"); addon != nil {
			config.Driver = "ceph-csi"
			config.Version = addon.Version
		}
	}
	if config.Version == "" && config.Driver != "" {
		if addon := findCSIAddon(releaseImage, config.Driver); addon != nil {
			config.Version = addon.Version
		}
	}
	return &Installer{
		sshConn: sshConn,
		config:  config,
	}
}

// findCSIAddon finds a CSI addon by name in the ReleaseImage.
func findCSIAddon(releaseImage *cfov1.ReleaseImage, name string) *commonv1.AddonDefinition {
	for i := range releaseImage.Spec.Addons {
		if releaseImage.Spec.Addons[i].Name == name {
			return &releaseImage.Spec.Addons[i]
		}
	}
	return nil
}

func (i *Installer) Install(ctx context.Context) (*InstallResult, error) {
	if !i.config.Enabled {
		return &InstallResult{Completed: true, Success: true, Error: "CSI installation disabled"}, nil
	}

	if i.config.Driver == "" {
		i.config.Driver = "local-csi"
	}
	if i.config.InstallMode == "" {
		i.config.InstallMode = "Helm"
	}

	existing, _ := i.checkExisting(ctx)
	if existing.installed && existing.version == i.config.Version {
		return &InstallResult{Completed: true, Success: true, Version: existing.version}, nil
	}

	script := i.generateInstallScript()

	result, err := i.sshConn.ExecuteScript(ctx, script)
	if err != nil {
		return &InstallResult{Completed: false, Success: false, Error: fmt.Sprintf("CSI installation failed: %v, stderr: %s", err, result.Stderr)}, err
	}

	if result.ExitCode != 0 {
		return &InstallResult{Completed: false, Success: false, Error: fmt.Sprintf("CSI installation failed with exit code %d: %s", result.ExitCode, result.Stderr)}, fmt.Errorf("CSI installation failed with exit code %d", result.ExitCode)
	}

	return &InstallResult{Completed: true, Success: true, Version: i.config.Version}, nil
}

type existingCSI struct {
	installed bool
	version   string
}

func (i *Installer) checkExisting(ctx context.Context) (*existingCSI, error) {
	ec := &existingCSI{}

	switch i.config.Driver {
	case "ceph-csi":
		result, _ := i.sshConn.ExecuteCommand(ctx, "kubectl get deployment ceph-csi-rbdplugin-provisioner -n ceph-csi -o jsonpath='{.spec.template.spec.containers[0].image}' 2>/dev/null || echo ''")
		if strings.Contains(result.Stdout, "v") {
			parts := strings.Split(result.Stdout, "v")
			if len(parts) > 1 {
				ec.version = strings.Split(parts[1], ":")[0]
				ec.installed = true
			}
		}
	case "local-csi":
		result, _ := i.sshConn.ExecuteCommand(ctx, "kubectl get deployment local-path-provisioner -n local-path-storage -o jsonpath='{.spec.template.spec.containers[0].image}' 2>/dev/null || echo ''")
		if strings.Contains(result.Stdout, "v") {
			parts := strings.Split(result.Stdout, "v")
			if len(parts) > 1 {
				ec.version = strings.Split(parts[1], ":")[0]
				ec.installed = true
			}
		}
	}

	return ec, nil
}

func (i *Installer) generateInstallScript() string {
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

	switch i.config.Driver {
	case "ceph-csi":
		return i.generateCephCSIScript(installSource, releaseServer, localPath)
	case "local-csi":
		return i.generateLocalCSIScript(installSource, releaseServer, localPath)
	default:
		return i.generateLocalCSIScript(installSource, releaseServer, localPath)
	}
}

func (i *Installer) generateCephCSIScript(installSource, releaseServer, localPath string) string {
	clusterID := ""
	monitors := ""
	rbdPool := "kubernetes"
	scName := "ceph-rbd"

	if i.config.Config != nil && i.config.Config.CephCsi != nil {
		cc := i.config.Config.CephCsi
		clusterID = cc.ClusterID
		for _, m := range cc.Monitors {
			if monitors != "" {
				monitors += ","
			}
			monitors += m
		}
		if cc.RBD != nil && cc.RBD.Pool != "" {
			rbdPool = cc.RBD.Pool
		}
		if cc.StorageClass != nil && cc.StorageClass.Name != "" {
			scName = cc.StorageClass.Name
		}
	}

	script := `#!/bin/bash
set -euo pipefail

CSI_DRIVER="${CSI_DRIVER:-ceph-csi}"
CSI_VERSION="${CSI_VERSION}"
INSTALL_MODE="${INSTALL_MODE:-Helm}"
INSTALL_SOURCE="${INSTALL_SOURCE:-online}"
RELEASE_SERVER="${RELEASE_SERVER:-}"
LOCAL_PATH="${LOCAL_PATH:-}"
CEPH_CLUSTER_ID="${CEPH_CLUSTER_ID}"
CEPH_MONITORS="${CEPH_MONITORS}"
CEPH_RBD_POOL="${CEPH_RBD_POOL:-kubernetes}"
SC_NAME="${SC_NAME:-ceph-rbd}"

echo "=== CSI 安装开�?(driver=$CSI_DRIVER, version=$CSI_VERSION, source=$INSTALL_SOURCE) ==="

fetch_resource() {
    local resource="$1"
    local dest="$2"
    case "$INSTALL_SOURCE" in
        online) curl -fsSL "$resource" -o "$dest" ;;
        http)   curl -fsSL "${RELEASE_SERVER}/${resource}" -o "$dest" ;;
        local)  cp "${LOCAL_PATH}/${resource}" "$dest" ;;
        *)      echo "ERROR: 不支持的安装�? $INSTALL_SOURCE"; exit 1 ;;
    esac
}

load_csi_images() {
    case "$INSTALL_SOURCE" in
        online)
            echo "在线模式：镜像从 registry 拉取"
            ;;
        http|local)
            echo "=== 加载 Ceph-CSI 镜像 ==="
            for image in cephcsi.tar csi-attacher.tar csi-provisioner.tar csi-snapshotter.tar csi-resizer.tar csi-node-driver-registrar.tar; do
                echo "下载: $image"
                fetch_resource "images/ceph-csi/v${CSI_VERSION}/$image" "/tmp/$image"
                echo "导入: $image"
                ctr -n k8s.io images import "/tmp/$image"
                rm -f "/tmp/$image"
            done
            echo "=== Ceph-CSI 镜像加载完成 ==="
            ;;
    esac
}

install_ceph_csi_helm() {
    case "$INSTALL_SOURCE" in
        online)
            helm repo add ceph-csi https://ceph.github.io/csi-charts || true
            helm repo update
            local monitors_json="["
            local first=true
            IFS=',' read -ra MON_ARRAY <<< "$CEPH_MONITORS"
            for mon in "${MON_ARRAY[@]}"; do
                $first && { monitors_json="${monitors_json}\"${mon}\""; first=false; } || monitors_json="${monitors_json},\"${mon}\""
            done
            monitors_json="${monitors_json}]"
            helm upgrade --install ceph-csi ceph-csi/ceph-csi \
                --namespace ceph-csi --create-namespace --version "v${CSI_VERSION}" \
                --set "csiConfig[0].clusterID=${CEPH_CLUSTER_ID}" \
                --set "csiConfig[0].monitors=${monitors_json}" \
                --set "storageClass.create=true" \
                --set "storageClass.name=${SC_NAME}" \
                --set "storageClass.pool=${CEPH_RBD_POOL}" \
                --wait --timeout=300s
            ;;
        http|local)
            local chart=$(mktemp)
            fetch_resource "csi/ceph-csi/v${CSI_VERSION}/ceph-csi.tgz" "$chart"
            local monitors_json="["
            local first=true
            IFS=',' read -ra MON_ARRAY <<< "$CEPH_MONITORS"
            for mon in "${MON_ARRAY[@]}"; do
                $first && { monitors_json="${monitors_json}\"${mon}\""; first=false; } || monitors_json="${monitors_json},\"${mon}\""
            done
            monitors_json="${monitors_json}]"
            helm upgrade --install ceph-csi "$chart" \
                --namespace ceph-csi --create-namespace \
                --set "csiConfig[0].clusterID=${CEPH_CLUSTER_ID}" \
                --set "csiConfig[0].monitors=${monitors_json}" \
                --set "storageClass.create=true" \
                --set "storageClass.name=${SC_NAME}" \
                --wait --timeout=300s
            rm -f "$chart"
            ;;
    esac
    echo "Ceph-CSI 部署完成"
}

install_ceph_csi_manifest() {
    kubectl create namespace ceph-csi --dry-run=client -o yaml | kubectl apply -f -
    local manifest=$(mktemp)
    case "$INSTALL_SOURCE" in
        online)
            local base="https://raw.githubusercontent.com/ceph/ceph-csi/v${CSI_VERSION}/deploy/cephcsi/kubernetes"
            for f in csi-config-map.yaml csi-rbdplugin.yaml csi-rbdplugin-provisioner.yaml; do
                curl -fsSL "${base}/${f}" | kubectl apply -n ceph-csi -f -
            done
            ;;
        http|local)
            fetch_resource "csi/ceph-csi-${CSI_VERSION}.yaml" "$manifest"
            kubectl apply -n ceph-csi -f "$manifest"
            rm -f "$manifest"
            ;;
    esac
    cat <<EOF | kubectl apply -f -
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: ${SC_NAME}
provisioner: rbd.csi.ceph.com
parameters:
  clusterID: ${CEPH_CLUSTER_ID}
  pool: ${CEPH_RBD_POOL}
  imageFeatures: layering
  csi.storage.k8s.io/provisioner-secret-name: csi-rbd-secret
  csi.storage.k8s.io/provisioner-secret-namespace: ceph-csi
  csi.storage.k8s.io/controller-expand-secret-name: csi-rbd-secret
  csi.storage.k8s.io/controller-expand-secret-namespace: ceph-csi
  csi.storage.k8s.io/node-stage-secret-name: csi-rbd-secret
  csi.storage.k8s.io/node-stage-secret-namespace: ceph-csi
reclaimPolicy: Delete
volumeBindingMode: Immediate
allowVolumeExpansion: true
EOF
    kubectl rollout status deployment/ceph-csi-rbdplugin-provisioner -n ceph-csi --timeout=300s
    echo "Ceph-CSI 部署完成"
}

verify_csi() {
    kubectl get storageclass "$SC_NAME" &>/dev/null && echo "StorageClass ($SC_NAME): OK" || { echo "ERROR: StorageClass 不存�?; return 1; }
    echo "CSI 验证完成"
}

load_csi_images
case "$INSTALL_MODE" in
    Helm)     install_ceph_csi_helm ;;
    Manifest) install_ceph_csi_manifest ;;
esac
verify_csi
echo "=== CSI 安装完成 ==="
`

	envVars := map[string]string{
		"CSI_DRIVER":      "ceph-csi",
		"CSI_VERSION":     i.config.Version,
		"INSTALL_MODE":    i.config.InstallMode,
		"INSTALL_SOURCE":  installSource,
		"CEPH_CLUSTER_ID": clusterID,
		"CEPH_MONITORS":   monitors,
		"CEPH_RBD_POOL":   rbdPool,
		"SC_NAME":         scName,
	}

	if releaseServer != "" {
		envVars["RELEASE_SERVER"] = releaseServer
	}
	if localPath != "" {
		envVars["LOCAL_PATH"] = localPath
	}

	return prependEnvVars(script, envVars)
}

func (i *Installer) generateLocalCSIScript(installSource, releaseServer, localPath string) string {
	scName := "local-path"

	if i.config.Config != nil && i.config.Config.LocalCsi != nil && i.config.Config.LocalCsi.StorageClass != nil {
		if i.config.Config.LocalCsi.StorageClass.Name != "" {
			scName = i.config.Config.LocalCsi.StorageClass.Name
		}
	}

	script := `#!/bin/bash
set -euo pipefail

CSI_VERSION="${CSI_VERSION}"
INSTALL_SOURCE="${INSTALL_SOURCE:-online}"
RELEASE_SERVER="${RELEASE_SERVER:-}"
LOCAL_PATH="${LOCAL_PATH:-}"
SC_NAME="${SC_NAME:-local-path}"

echo "=== Local-CSI 安装开�?(source=$INSTALL_SOURCE) ==="

fetch_resource() {
    local resource="$1"
    local dest="$2"
    case "$INSTALL_SOURCE" in
        online) curl -fsSL "$resource" -o "$dest" ;;
        http)   curl -fsSL "${RELEASE_SERVER}/${resource}" -o "$dest" ;;
        local)  cp "${LOCAL_PATH}/${resource}" "$dest" ;;
        *)      echo "ERROR: 不支持的安装�? $INSTALL_SOURCE"; exit 1 ;;
    esac
}

install_local_csi() {
    local manifest=$(mktemp)
    case "$INSTALL_SOURCE" in
        online)
            curl -fsSL "https://raw.githubusercontent.com/rancher/local-path-provisioner/v${CSI_VERSION}/deploy/local-path-storage.yaml" -o "$manifest"
            ;;
        http|local)
            fetch_resource "csi/local-path-provisioner-${CSI_VERSION}.yaml" "$manifest"
            ;;
    esac
    kubectl apply -f "$manifest"
    rm -f "$manifest"
    kubectl rollout status deployment/local-path-provisioner -n local-path-storage --timeout=300s
    echo "Local-CSI 部署完成"
}

verify_csi() {
    kubectl get storageclass "$SC_NAME" &>/dev/null && echo "StorageClass ($SC_NAME): OK" || { echo "ERROR: StorageClass 不存�?; return 1; }
    echo "CSI 验证完成"
}

install_local_csi
verify_csi
echo "=== Local-CSI 安装完成 ==="
`

	envVars := map[string]string{
		"CSI_VERSION":    i.config.Version,
		"INSTALL_SOURCE": installSource,
		"SC_NAME":        scName,
	}

	if releaseServer != "" {
		envVars["RELEASE_SERVER"] = releaseServer
	}
	if localPath != "" {
		envVars["LOCAL_PATH"] = localPath
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
