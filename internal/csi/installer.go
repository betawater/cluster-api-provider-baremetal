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

	infrav1 "github.com/BetaWater/cluster-api-provider-baremetal/api/v1beta1"
	sshclient "github.com/BetaWater/cluster-api-provider-baremetal/internal/ssh"
)

type Installer struct {
	sshConn *sshclient.SSHConnection
	config  infrav1.CSIConfig
}

type InstallResult struct {
	Completed bool   `json:"completed"`
	Success   bool   `json:"success"`
	Version   string `json:"version,omitempty"`
	Error     string `json:"error,omitempty"`
}

func New(sshConn *sshclient.SSHConnection, config infrav1.CSIConfig) *Installer {
	return &Installer{
		sshConn: sshConn,
		config:  config,
	}
}

// NewFromReleaseImage creates a CSI Installer with versions sourced from a ReleaseImage.
func NewFromReleaseImage(sshConn *sshclient.SSHConnection, releaseImage *infrav1.ReleaseImage, config infrav1.CSIConfig) *Installer {
	if config.Driver == "" {
		if releaseImage.Spec.Components.CephCsi != "" {
			config.Driver = "ceph-csi"
			config.Version = releaseImage.Spec.Components.CephCsi
		}
	}
	if config.Version == "" && config.Driver != "" {
		switch config.Driver {
		case "ceph-csi":
			config.Version = releaseImage.Spec.Components.CephCsi
		}
	}
	return &Installer{
		sshConn: sshConn,
		config:  config,
	}
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

	var script string
	if i.config.AirGap != nil && i.config.AirGap.Enabled {
		script = i.generateOfflineInstallScript()
	} else {
		script = i.generateOnlineInstallScript()
	}

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

func (i *Installer) generateOnlineInstallScript() string {
	switch i.config.Driver {
	case "ceph-csi":
		return i.generateCephCSIScript()
	case "local-csi":
		return i.generateLocalCSIScript()
	default:
		return i.generateLocalCSIScript()
	}
}

func (i *Installer) generateCephCSIScript() string {
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
CEPH_CLUSTER_ID="${CEPH_CLUSTER_ID}"
CEPH_MONITORS="${CEPH_MONITORS}"
CEPH_RBD_POOL="${CEPH_RBD_POOL:-kubernetes}"
SC_NAME="${SC_NAME:-ceph-rbd}"

echo "=== CSI 安装开始 (driver=$CSI_DRIVER, version=$CSI_VERSION) ==="

install_ceph_csi_helm() {
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
    echo "Ceph-CSI 部署完成"
}

install_ceph_csi_manifest() {
    kubectl create namespace ceph-csi --dry-run=client -o yaml | kubectl apply -f -
    local base="https://raw.githubusercontent.com/ceph/ceph-csi/v${CSI_VERSION}/deploy/cephcsi/kubernetes"
    for f in csi-config-map.yaml csi-rbdplugin.yaml csi-rbdplugin-provisioner.yaml; do
        curl -fsSL "${base}/${f}" | kubectl apply -n ceph-csi -f -
    done
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
    kubectl get storageclass "$SC_NAME" &>/dev/null && echo "StorageClass ($SC_NAME): OK" || { echo "ERROR: StorageClass 不存在"; return 1; }
    echo "CSI 验证完成"
}

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
		"CEPH_CLUSTER_ID": clusterID,
		"CEPH_MONITORS":   monitors,
		"CEPH_RBD_POOL":   rbdPool,
		"SC_NAME":         scName,
	}

	return prependEnvVars(script, envVars)
}

func (i *Installer) generateLocalCSIScript() string {
	scName := "local-path"

	if i.config.Config != nil && i.config.Config.LocalCsi != nil && i.config.Config.LocalCsi.StorageClass != nil {
		if i.config.Config.LocalCsi.StorageClass.Name != "" {
			scName = i.config.Config.LocalCsi.StorageClass.Name
		}
	}

	script := `#!/bin/bash
set -euo pipefail

CSI_VERSION="${CSI_VERSION}"
SC_NAME="${SC_NAME:-local-path}"

echo "=== Local-CSI 安装开始 ==="

install_local_csi() {
    kubectl apply -f "https://raw.githubusercontent.com/rancher/local-path-provisioner/v${CSI_VERSION}/deploy/local-path-storage.yaml"
    kubectl rollout status deployment/local-path-provisioner -n local-path-storage --timeout=300s
    echo "Local-CSI 部署完成"
}

verify_csi() {
    kubectl get storageclass "$SC_NAME" &>/dev/null && echo "StorageClass ($SC_NAME): OK" || { echo "ERROR: StorageClass 不存在"; return 1; }
    echo "CSI 验证完成"
}

install_local_csi
verify_csi
echo "=== Local-CSI 安装完成 ==="
`

	envVars := map[string]string{
		"CSI_VERSION": i.config.Version,
		"SC_NAME":     scName,
	}

	return prependEnvVars(script, envVars)
}

func (i *Installer) generateOfflineInstallScript() string {
	airGap := i.config.AirGap
	chartArchive := "/opt/capbm/csi/ceph-csi-" + i.config.Version + ".tgz"
	imagesArchive := "/opt/capbm/csi/ceph-csi-images.tar"
	manifestPath := "/opt/capbm/csi/" + i.config.Driver + ".yaml"

	if airGap != nil {
		if airGap.ChartArchive != "" {
			chartArchive = airGap.ChartArchive
		}
		if airGap.LocalPath != "" {
			manifestPath = airGap.LocalPath + "/" + i.config.Driver + ".yaml"
			chartArchive = airGap.LocalPath + "/ceph-csi-" + i.config.Version + ".tgz"
		}
	}

	script := `#!/bin/bash
set -euo pipefail

CSI_DRIVER="${CSI_DRIVER:-ceph-csi}"
CSI_VERSION="${CSI_VERSION}"
INSTALL_MODE="${INSTALL_MODE:-Helm}"
CSI_CHART_ARCHIVE="${CSI_CHART_ARCHIVE}"
CSI_IMAGES_ARCHIVE="${CSI_IMAGES_ARCHIVE}"
CSI_MANIFEST_PATH="${CSI_MANIFEST_PATH}"

echo "=== CSI 离线安装开始 ==="

load_csi_images() {
    [ -f "$CSI_IMAGES_ARCHIVE" ] && { ctr -n k8s.io images import "$CSI_IMAGES_ARCHIVE"; echo "CSI 镜像加载完成"; } || echo "WARNING: 镜像包不存在, 跳过"
}

install_csi_helm_offline() {
    [ ! -f "$CSI_CHART_ARCHIVE" ] && { echo "ERROR: Helm Chart 不存在: $CSI_CHART_ARCHIVE"; return 1; }
    helm upgrade --install "$CSI_DRIVER" "$CSI_CHART_ARCHIVE" \
        --namespace "${CSI_DRIVER}" --create-namespace \
        --set "csiConfig[0].clusterID=${CEPH_CLUSTER_ID}" \
        --set "csiConfig[0].monitors=${CEPH_MONITORS}" \
        --set "storageClass.create=true" \
        --set "storageClass.name=${SC_NAME}" \
        --wait --timeout=300s
    echo "CSI 离线部署完成 (Helm)"
}

install_csi_manifest_offline() {
    [ ! -f "$CSI_MANIFEST_PATH" ] && { echo "ERROR: Manifest 不存在: $CSI_MANIFEST_PATH"; return 1; }
    kubectl create namespace "${CSI_DRIVER}" --dry-run=client -o yaml | kubectl apply -f -
    kubectl apply -n "${CSI_DRIVER}" -f "$CSI_MANIFEST_PATH"
    echo "CSI 离线部署完成 (Manifest)"
}

verify_csi() {
    kubectl get storageclass "$SC_NAME" &>/dev/null && echo "StorageClass: OK" || { echo "ERROR: StorageClass 不存在"; return 1; }
    echo "CSI 验证完成"
}

load_csi_images
case "$INSTALL_MODE" in
    Helm)     install_csi_helm_offline ;;
    Manifest) install_csi_manifest_offline ;;
esac
verify_csi
echo "=== CSI 离线安装完成 ==="
`

	clusterID := ""
	monitors := ""
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
		if cc.StorageClass != nil && cc.StorageClass.Name != "" {
			scName = cc.StorageClass.Name
		}
	}

	envVars := map[string]string{
		"CSI_DRIVER":         i.config.Driver,
		"CSI_VERSION":        i.config.Version,
		"INSTALL_MODE":       i.config.InstallMode,
		"CSI_CHART_ARCHIVE":  chartArchive,
		"CSI_IMAGES_ARCHIVE": imagesArchive,
		"CSI_MANIFEST_PATH":  manifestPath,
		"CEPH_CLUSTER_ID":    clusterID,
		"CEPH_MONITORS":      monitors,
		"SC_NAME":            scName,
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
