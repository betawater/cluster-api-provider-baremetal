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

package installer

import (
	"fmt"

	infrav1 "github.com/BetaWater/cluster-api-provider-baremetal/api/v1beta1"
)

func getContainerRuntimeScript(runtimeType, osType string, config *infrav1.ComponentInstallConfig) (string, error) {
	switch runtimeType {
	case "containerd":
		return getContainerdScript(osType, config)
	case "cri-o":
		return getCRIOScript(osType, config)
	case "docker":
		return getDockerScript(osType, config)
	default:
		return getContainerdScript(osType, config)
	}
}

func getContainerdScript(osType string, config *infrav1.ComponentInstallConfig) (string, error) {
	switch osType {
	case "ubuntu", "debian":
		return containerdUbuntuScript, nil
	case "rhel":
		return containerdRHELScript, nil
	case "suse":
		return containerdSUSEScript, nil
	case "flatcar":
		return containerdFlatcarScript, nil
	default:
		return "", fmt.Errorf("unsupported OS for containerd installation: %s", osType)
	}
}

func getKubernetesScript(osType string, config *infrav1.ComponentInstallConfig) (string, error) {
	switch osType {
	case "ubuntu", "debian":
		return kubernetesUbuntuScript, nil
	case "rhel":
		return kubernetesRHELScript, nil
	case "suse":
		return kubernetesSUSEScript, nil
	case "flatcar":
		return kubernetesFlatcarScript, nil
	default:
		return "", fmt.Errorf("unsupported OS for kubernetes installation: %s", osType)
	}
}

func getCRIOScript(osType string, config *infrav1.ComponentInstallConfig) (string, error) {
	switch osType {
	case "ubuntu", "debian":
		return crioUbuntuScript, nil
	case "rhel":
		return crioRHELScript, nil
	default:
		return "", fmt.Errorf("unsupported OS for CRI-O installation: %s", osType)
	}
}

func getDockerScript(osType string, config *infrav1.ComponentInstallConfig) (string, error) {
	return dockerScript, nil
}

const containerdUbuntuScript = `#!/bin/bash
set -euo pipefail

CONFIG_FILE="/etc/containerd/config.toml"
SYSTEMD_CGROUP="${SYSTEMD_CGROUP:-true}"
SANDBOX_IMAGE="${SANDBOX_IMAGE:-registry.k8s.io/pause:3.9}"
REGISTRY_MIRRORS="${REGISTRY_MIRRORS:-}"
MAX_CONCURRENT_DOWNLOADS="${MAX_CONCURRENT_DOWNLOADS:-}"
RAW_CONFIG="${RAW_CONFIG:-}"

echo "=== Installing containerd (Ubuntu/Debian) ==="

if command -v containerd &>/dev/null; then
    current_version=$(containerd --version | awk '{print $3}')
    if [ -n "$CONTAINERD_VERSION" ] && [ "$current_version" != "$CONTAINERD_VERSION" ]; then
        echo "Upgrading containerd: $current_version -> $CONTAINERD_VERSION"
        apt-get remove -y containerd || true
        apt-get install -y containerd
    else
        echo "containerd already installed: $current_version"
    fi
else
    apt-get update
    apt-get install -y containerd
fi

generate_containerd_config() {
    local temp_config=$(mktemp)
    containerd config default > "$temp_config"

    if [ "$SYSTEMD_CGROUP" = "true" ]; then
        sed -i 's/SystemdCgroup = false/SystemdCgroup = true/g' "$temp_config"
    fi

    if [ -n "$SANDBOX_IMAGE" ]; then
        sed -i "s|sandbox_image = .*|sandbox_image = \"${SANDBOX_IMAGE}\"|g" "$temp_config"
    fi

    if [ -n "$MAX_CONCURRENT_DOWNLOADS" ]; then
        sed -i "s/max_concurrent_downloads = .*/max_concurrent_downloads = ${MAX_CONCURRENT_DOWNLOADS}/g" "$temp_config"
    fi

    if [ -n "$REGISTRY_MIRRORS" ]; then
        IFS=';' read -ra MIRROR_ENTRIES <<< "$REGISTRY_MIRRORS"
        for entry in "${MIRROR_ENTRIES[@]}"; do
            host="${entry%%=*}"
            endpoints="${entry#*=}"
            IFS=',' read -ra ENDPOINT_LIST <<< "$endpoints"
            endpoint_config=""
            for ep in "${ENDPOINT_LIST[@]}"; do
                endpoint_config="${endpoint_config}    endpoint = [\"${ep}\"]\n"
            done
            cat >> "$temp_config" << EOF

[plugins."io.containerd.grpc.v1.cri".registry.mirrors."${host}"]
$(echo -e "$endpoint_config")
EOF
        done
    fi

    if [ -n "$RAW_CONFIG" ]; then
        echo "$RAW_CONFIG" >> "$temp_config"
    fi

    if containerd --config "$temp_config" config dump > /dev/null 2>&1; then
        mv "$temp_config" "$CONFIG_FILE"
    else
        rm -f "$temp_config"
        echo "ERROR: Invalid containerd configuration"
        return 1
    fi
}

generate_containerd_config
systemctl restart containerd
systemctl enable containerd

echo "=== containerd installation completed ==="
`

const containerdRHELScript = `#!/bin/bash
set -euo pipefail

CONFIG_FILE="/etc/containerd/config.toml"
SYSTEMD_CGROUP="${SYSTEMD_CGROUP:-true}"
SANDBOX_IMAGE="${SANDBOX_IMAGE:-registry.k8s.io/pause:3.9}"
REGISTRY_MIRRORS="${REGISTRY_MIRRORS:-}"
MAX_CONCURRENT_DOWNLOADS="${MAX_CONCURRENT_DOWNLOADS:-}"
RAW_CONFIG="${RAW_CONFIG:-}"

echo "=== Installing containerd (RHEL/CentOS/Rocky) ==="

if command -v containerd &>/dev/null; then
    current_version=$(containerd --version | awk '{print $3}')
    if [ -n "$CONTAINERD_VERSION" ] && [ "$current_version" != "$CONTAINERD_VERSION" ]; then
        echo "Upgrading containerd: $current_version -> $CONTAINERD_VERSION"
        dnf remove -y containerd 2>/dev/null || yum remove -y containerd 2>/dev/null || true
        dnf install -y containerd 2>/dev/null || yum install -y containerd 2>/dev/null || true
    else
        echo "containerd already installed: $current_version"
    fi
else
    if command -v dnf &>/dev/null; then
        dnf install -y containerd
    else
        yum install -y containerd
    fi
fi

generate_containerd_config() {
    local temp_config=$(mktemp)
    containerd config default > "$temp_config"

    if [ "$SYSTEMD_CGROUP" = "true" ]; then
        sed -i 's/SystemdCgroup = false/SystemdCgroup = true/g' "$temp_config"
    fi

    if [ -n "$SANDBOX_IMAGE" ]; then
        sed -i "s|sandbox_image = .*|sandbox_image = \"${SANDBOX_IMAGE}\"|g" "$temp_config"
    fi

    if [ -n "$MAX_CONCURRENT_DOWNLOADS" ]; then
        sed -i "s/max_concurrent_downloads = .*/max_concurrent_downloads = ${MAX_CONCURRENT_DOWNLOADS}/g" "$temp_config"
    fi

    if [ -n "$REGISTRY_MIRRORS" ]; then
        IFS=';' read -ra MIRROR_ENTRIES <<< "$REGISTRY_MIRRORS"
        for entry in "${MIRROR_ENTRIES[@]}"; do
            host="${entry%%=*}"
            endpoints="${entry#*=}"
            IFS=',' read -ra ENDPOINT_LIST <<< "$endpoints"
            endpoint_config=""
            for ep in "${ENDPOINT_LIST[@]}"; do
                endpoint_config="${endpoint_config}    endpoint = [\"${ep}\"]\n"
            done
            cat >> "$temp_config" << EOF

[plugins."io.containerd.grpc.v1.cri".registry.mirrors."${host}"]
$(echo -e "$endpoint_config")
EOF
        done
    fi

    if [ -n "$RAW_CONFIG" ]; then
        echo "$RAW_CONFIG" >> "$temp_config"
    fi

    if containerd --config "$temp_config" config dump > /dev/null 2>&1; then
        mv "$temp_config" "$CONFIG_FILE"
    else
        rm -f "$temp_config"
        echo "ERROR: Invalid containerd configuration"
        return 1
    fi
}

generate_containerd_config
systemctl restart containerd
systemctl enable containerd

echo "=== containerd installation completed ==="
`

const containerdSUSEScript = `#!/bin/bash
set -euo pipefail

CONFIG_FILE="/etc/containerd/config.toml"
SYSTEMD_CGROUP="${SYSTEMD_CGROUP:-true}"
SANDBOX_IMAGE="${SANDBOX_IMAGE:-registry.k8s.io/pause:3.9}"

echo "=== Installing containerd (SUSE) ==="

if command -v containerd &>/dev/null; then
    echo "containerd already installed: $(containerd --version)"
else
    zypper refresh
    zypper install -y containerd
fi

generate_containerd_config() {
    local temp_config=$(mktemp)
    containerd config default > "$temp_config"

    if [ "$SYSTEMD_CGROUP" = "true" ]; then
        sed -i 's/SystemdCgroup = false/SystemdCgroup = true/g' "$temp_config"
    fi

    if [ -n "$SANDBOX_IMAGE" ]; then
        sed -i "s|sandbox_image = .*|sandbox_image = \"${SANDBOX_IMAGE}\"|g" "$temp_config"
    fi

    if containerd --config "$temp_config" config dump > /dev/null 2>&1; then
        mv "$temp_config" "$CONFIG_FILE"
    else
        rm -f "$temp_config"
        echo "ERROR: Invalid containerd configuration"
        return 1
    fi
}

generate_containerd_config
systemctl restart containerd
systemctl enable containerd

echo "=== containerd installation completed ==="
`

const containerdFlatcarScript = `#!/bin/bash
set -euo pipefail

CONFIG_FILE="/etc/containerd/config.toml"
SYSTEMD_CGROUP="${SYSTEMD_CGROUP:-true}"
SANDBOX_IMAGE="${SANDBOX_IMAGE:-registry.k8s.io/pause:3.9}"

echo "=== Verifying containerd (Flatcar) ==="

if command -v containerd &>/dev/null; then
    echo "containerd pre-installed: $(containerd --version)"
else
    echo "ERROR: containerd not found on Flatcar"
    exit 1
fi

generate_containerd_config() {
    local temp_config=$(mktemp)
    containerd config default > "$temp_config"

    if [ "$SYSTEMD_CGROUP" = "true" ]; then
        sed -i 's/SystemdCgroup = false/SystemdCgroup = true/g' "$temp_config"
    fi

    if [ -n "$SANDBOX_IMAGE" ]; then
        sed -i "s|sandbox_image = .*|sandbox_image = \"${SANDBOX_IMAGE}\"|g" "$temp_config"
    fi

    if containerd --config "$temp_config" config dump > /dev/null 2>&1; then
        mv "$temp_config" "$CONFIG_FILE"
    else
        rm -f "$temp_config"
        echo "ERROR: Invalid containerd configuration"
        return 1
    fi
}

generate_containerd_config
systemctl restart containerd

echo "=== containerd configuration completed ==="
`

const kubernetesUbuntuScript = `#!/bin/bash
set -euo pipefail

K8S_VERSION="${K8S_VERSION:-}"
REPO_BASE_URL="${REPO_BASE_URL:-}"
REPO_GPG_KEY="${REPO_GPG_KEY:-}"
CGROUP_DRIVER="${CGROUP_DRIVER:-systemd}"
MAX_PODS="${MAX_PODS:-250}"
EXTRA_ARGS="${EXTRA_ARGS:-}"
KUBELET_RAW_CONFIG="${KUBELET_RAW_CONFIG:-}"
DROP_IN_DIR="/etc/systemd/system/kubelet.service.d"
DROP_IN_FILE="${DROP_IN_DIR}/10-capbm.conf"

echo "=== Installing Kubernetes components (Ubuntu/Debian) ==="

if command -v kubeadm &>/dev/null; then
    current_version=$(kubeadm version -o short 2>/dev/null || echo "")
    if [ "$current_version" = "v${K8S_VERSION}" ]; then
        echo "Kubernetes components already installed: $current_version"
        generate_kubelet_config
        exit 0
    fi
    echo "Version mismatch: current=$current_version, desired=v${K8S_VERSION}"
fi

apt-get update
apt-get install -y apt-transport-https ca-certificates curl gpg

minor_version=$(echo "$K8S_VERSION" | cut -d'.' -f1,2)
gpg_key_url="${REPO_GPG_KEY:-https://pkgs.k8s.io/core:/stable:/v${minor_version}/deb/Release.key}"
repo_url="${REPO_BASE_URL:-https://pkgs.k8s.io/core:/stable:/v${minor_version}/deb/}"

curl -fsSL "$gpg_key_url" | gpg --dearmor -o /etc/apt/keyrings/kubernetes-apt-keyring.gpg
echo "deb [signed-by=/etc/apt/keyrings/kubernetes-apt-keyring.gpg] ${repo_url} /" > /etc/apt/sources.list.d/kubernetes.list

apt-get update
apt-get install -y "kubelet=${K8S_VERSION}-*" "kubeadm=${K8S_VERSION}-*" "kubectl=${K8S_VERSION}-*"
apt-mark hold kubelet kubeadm kubectl

generate_kubelet_config() {
    mkdir -p "$DROP_IN_DIR"

    local extra_args=""
    if [ -n "$CGROUP_DRIVER" ]; then
        extra_args="${extra_args} --cgroup-driver=${CGROUP_DRIVER}"
    fi
    if [ -n "$MAX_PODS" ]; then
        extra_args="${extra_args} --max-pods=${MAX_PODS}"
    fi
    if [ -n "$EXTRA_ARGS" ]; then
        extra_args="${extra_args} ${EXTRA_ARGS}"
    fi

    cat > "$DROP_IN_FILE" << EOF
[Service]
Environment="KUBELET_EXTRA_ARGS=${extra_args}"
EOF

    if [ -n "$KUBELET_RAW_CONFIG" ]; then
        local kubelet_config="/var/lib/kubelet/config.yaml"
        if [ -f "$kubelet_config" ]; then
            echo "$KUBELET_RAW_CONFIG" >> "$kubelet_config"
        else
            echo "$KUBELET_RAW_CONFIG" > "$kubelet_config"
        fi
    fi

    systemctl daemon-reload
    systemctl enable kubelet
}

generate_kubelet_config

echo "=== Kubernetes components installation completed ==="
echo "  kubeadm: $(kubeadm version -o short)"
echo "  kubelet: $(kubelet --version)"
`

const kubernetesRHELScript = `#!/bin/bash
set -euo pipefail

K8S_VERSION="${K8S_VERSION:-}"
REPO_BASE_URL="${REPO_BASE_URL:-}"
REPO_GPG_KEY="${REPO_GPG_KEY:-}"
CGROUP_DRIVER="${CGROUP_DRIVER:-systemd}"
MAX_PODS="${MAX_PODS:-250}"
EXTRA_ARGS="${EXTRA_ARGS:-}"
KUBELET_RAW_CONFIG="${KUBELET_RAW_CONFIG:-}"
DROP_IN_DIR="/etc/systemd/system/kubelet.service.d"
DROP_IN_FILE="${DROP_IN_DIR}/10-capbm.conf"

echo "=== Installing Kubernetes components (RHEL/CentOS/Rocky) ==="

if command -v dnf &>/dev/null; then
    PKG_MGR="dnf"
else
    PKG_MGR="yum"
fi

if command -v kubeadm &>/dev/null; then
    current_version=$(kubeadm version -o short 2>/dev/null || echo "")
    if [ "$current_version" = "v${K8S_VERSION}" ]; then
        echo "Kubernetes components already installed: $current_version"
        generate_kubelet_config
        exit 0
    fi
    echo "Version mismatch: current=$current_version, desired=v${K8S_VERSION}"
fi

minor_version=$(echo "$K8S_VERSION" | cut -d'.' -f1,2)
repo_url="${REPO_BASE_URL:-https://pkgs.k8s.io/core:/stable:/v${minor_version}/rpm/}"
gpg_key="${REPO_GPG_KEY:-https://pkgs.k8s.io/core:/stable:/v${minor_version}/rpm/repodata/repomd.xml.key}"

cat > /etc/yum.repos.d/kubernetes.repo << EOF
[kubernetes]
name=Kubernetes
baseurl=${repo_url}
enabled=1
gpgcheck=1
gpgkey=${gpg_key}
EOF

$PKG_MGR install -y "kubelet-${K8S_VERSION}" "kubeadm-${K8S_VERSION}" "kubectl-${K8S_VERSION}"

generate_kubelet_config() {
    mkdir -p "$DROP_IN_DIR"

    local extra_args=""
    if [ -n "$CGROUP_DRIVER" ]; then
        extra_args="${extra_args} --cgroup-driver=${CGROUP_DRIVER}"
    fi
    if [ -n "$MAX_PODS" ]; then
        extra_args="${extra_args} --max-pods=${MAX_PODS}"
    fi
    if [ -n "$EXTRA_ARGS" ]; then
        extra_args="${extra_args} ${EXTRA_ARGS}"
    fi

    cat > "$DROP_IN_FILE" << EOF
[Service]
Environment="KUBELET_EXTRA_ARGS=${extra_args}"
EOF

    if [ -n "$KUBELET_RAW_CONFIG" ]; then
        local kubelet_config="/var/lib/kubelet/config.yaml"
        if [ -f "$kubelet_config" ]; then
            echo "$KUBELET_RAW_CONFIG" >> "$kubelet_config"
        else
            echo "$KUBELET_RAW_CONFIG" > "$kubelet_config"
        fi
    fi

    systemctl daemon-reload
    systemctl enable kubelet
}

generate_kubelet_config

echo "=== Kubernetes components installation completed ==="
echo "  kubeadm: $(kubeadm version -o short)"
echo "  kubelet: $(kubelet --version)"
`

const kubernetesSUSEScript = `#!/bin/bash
set -euo pipefail

K8S_VERSION="${K8S_VERSION:-}"
CGROUP_DRIVER="${CGROUP_DRIVER:-systemd}"
MAX_PODS="${MAX_PODS:-250}"
EXTRA_ARGS="${EXTRA_ARGS:-}"
DROP_IN_DIR="/etc/systemd/system/kubelet.service.d"
DROP_IN_FILE="${DROP_IN_DIR}/10-capbm.conf"

echo "=== Installing Kubernetes components (SUSE) ==="

if command -v kubeadm &>/dev/null; then
    current_version=$(kubeadm version -o short 2>/dev/null || echo "")
    if [ "$current_version" = "v${K8S_VERSION}" ]; then
        echo "Kubernetes components already installed: $current_version"
        generate_kubelet_config
        exit 0
    fi
fi

minor_version=$(echo "$K8S_VERSION" | cut -d'.' -f1,2)

cat > /etc/zypp/repos.d/kubernetes.repo << EOF
[kubernetes]
name=Kubernetes
baseurl=https://pkgs.k8s.io/core:/stable:/v${minor_version}/rpm/
enabled=1
gpgcheck=1
gpgkey=https://pkgs.k8s.io/core:/stable:/v${minor_version}/rpm/repodata/repomd.xml.key
EOF

zypper refresh
zypper install -y "kubelet-${K8S_VERSION}" "kubeadm-${K8S_VERSION}" "kubectl-${K8S_VERSION}"

generate_kubelet_config() {
    mkdir -p "$DROP_IN_DIR"

    local extra_args=""
    if [ -n "$CGROUP_DRIVER" ]; then
        extra_args="${extra_args} --cgroup-driver=${CGROUP_DRIVER}"
    fi
    if [ -n "$MAX_PODS" ]; then
        extra_args="${extra_args} --max-pods=${MAX_PODS}"
    fi
    if [ -n "$EXTRA_ARGS" ]; then
        extra_args="${extra_args} ${EXTRA_ARGS}"
    fi

    cat > "$DROP_IN_FILE" << EOF
[Service]
Environment="KUBELET_EXTRA_ARGS=${extra_args}"
EOF

    systemctl daemon-reload
    systemctl enable kubelet
}

generate_kubelet_config

echo "=== Kubernetes components installation completed ==="
`

const kubernetesFlatcarScript = `#!/bin/bash
set -euo pipefail

K8S_VERSION="${K8S_VERSION:-}"
INSTALL_PREFIX="/opt/bin"
CGROUP_DRIVER="${CGROUP_DRIVER:-systemd}"
MAX_PODS="${MAX_PODS:-250}"
EXTRA_ARGS="${EXTRA_ARGS:-}"
DROP_IN_DIR="/etc/systemd/system/kubelet.service.d"

echo "=== Installing Kubernetes binaries (Flatcar) ==="

mkdir -p "$INSTALL_PREFIX"

base_url="https://dl.k8s.io/v${K8S_VERSION}/bin/linux/amd64"

for binary in kubeadm kubelet kubectl; do
    if [ ! -f "${INSTALL_PREFIX}/${binary}" ]; then
        echo "Downloading $binary"
        curl -fsSL "${base_url}/${binary}" -o "${INSTALL_PREFIX}/${binary}"
        chmod +x "${INSTALL_PREFIX}/${binary}"
    else
        echo "$binary already exists"
    fi
    ln -sf "${INSTALL_PREFIX}/${binary}" "/usr/local/bin/${binary}" 2>/dev/null || true
done

mkdir -p "$DROP_IN_DIR"

extra_args=""
if [ -n "$CGROUP_DRIVER" ]; then
    extra_args="${extra_args} --cgroup-driver=${CGROUP_DRIVER}"
fi
if [ -n "$MAX_PODS" ]; then
    extra_args="${extra_args} --max-pods=${MAX_PODS}"
fi
if [ -n "$EXTRA_ARGS" ]; then
    extra_args="${extra_args} ${EXTRA_ARGS}"
fi

cat > "${DROP_IN_DIR}/10-capbm.conf" << EOF
[Service]
Environment="KUBELET_EXTRA_ARGS=${extra_args}"
Environment="KUBELET_CONTAINER_RUNTIME_ENDPOINT=unix:///run/containerd/containerd.sock"
EOF

systemctl daemon-reload
systemctl enable kubelet

echo "=== Kubernetes binaries installation completed ==="
`

const crioUbuntuScript = `#!/bin/bash
set -euo pipefail

CRIO_VERSION="${CRIO_VERSION:-1.31}"
OS="xUbuntu_22.04"

echo "=== Installing CRI-O (Ubuntu/Debian) ==="

if command -v crio &>/dev/null; then
    echo "CRI-O already installed: $(crio --version | head -1)"
    systemctl enable --now crio
    exit 0
fi

echo "deb https://pkgs.k8s.io/addons:/cri-o:/stable:/${CRIO_VERSION}/deb/${OS}/ /" > /etc/apt/sources.list.d/cri-o.list
curl -fsSL "https://pkgs.k8s.io/addons:/cri-o:/stable:/${CRIO_VERSION}/deb/${OS}/Release.key" | gpg --dearmor -o /etc/apt/keyrings/cri-o-apt-keyring.gpg

apt-get update
apt-get install -y "cri-o-${CRIO_VERSION}"

systemctl enable --now crio
echo "=== CRI-O installation completed ==="
`

const crioRHELScript = `#!/bin/bash
set -euo pipefail

CRIO_VERSION="${CRIO_VERSION:-1.31}"

echo "=== Installing CRI-O (RHEL/CentOS/Rocky) ==="

if command -v crio &>/dev/null; then
    echo "CRI-O already installed: $(crio --version | head -1)"
    systemctl enable --now crio
    exit 0
fi

if command -v dnf &>/dev/null; then
    PKG_MGR="dnf"
else
    PKG_MGR="yum"
fi

cat > /etc/yum.repos.d/cri-o.repo << EOF
[cri-o]
name=CRI-O
baseurl=https://pkgs.k8s.io/addons:/cri-o:/stable:/${CRIO_VERSION}/rpm/
enabled=1
gpgcheck=1
gpgkey=https://pkgs.k8s.io/addons:/cri-o:/stable:/${CRIO_VERSION}/rpm/repodata/repomd.xml.key
EOF

$PKG_MGR install -y "cri-o-${CRIO_VERSION}"

systemctl enable --now crio
echo "=== CRI-O installation completed ==="
`

const dockerScript = `#!/bin/bash
set -euo pipefail

DOCKER_VERSION="${DOCKER_VERSION:-24.0}"
CRI_DOCKERD_VERSION="${CRI_DOCKERD_VERSION:-0.3.12}"

echo "=== Installing Docker + cri-dockerd ==="

if ! command -v docker &>/dev/null; then
    curl -fsSL https://get.docker.com | sh -s -- --version "$DOCKER_VERSION" 2>/dev/null || \
    curl -fsSL https://get.docker.com | sh 2>/dev/null || true
    systemctl enable --now docker
fi

if ! command -v cri-dockerd &>/dev/null; then
    arch="amd64"
    curl -fsSL "https://github.com/Mirantis/cri-dockerd/releases/download/v${CRI_DOCKERD_VERSION}/cri-dockerd-${CRI_DOCKERD_VERSION}.${arch}.tgz" | tar -xz -C /tmp
    install -m 0755 /tmp/cri-dockerd/cri-dockerd /usr/local/bin/

    cat > /etc/systemd/system/cri-dockerd.service << 'EOF'
[Unit]
Description=CRI Interface for Docker Application Container Engine
Documentation=https://docs.mirantis.com
After=network-online.target firewalld.service docker.service
Wants=network-online.target

[Service]
Type=notify
ExecStart=/usr/local/bin/cri-dockerd --network-plugin=cni --pod-infra-container-image=registry.k8s.io/pause:3.9
ExecReload=/bin/kill -s HUP $MAINPID
TimeoutSec=0
RestartSec=2
Restart=always

[Install]
WantedBy=multi-user.target
EOF

    cat > /etc/systemd/system/cri-dockerd.socket << 'EOF'
[Unit]
Description=CRI Docker Socket for the API
PartOf=cri-dockerd.service

[Socket]
ListenStream=/run/cri-dockerd.sock

[Install]
WantedBy=sockets.target
EOF

    systemctl daemon-reload
    systemctl enable --now cri-dockerd.socket cri-dockerd
fi

echo "=== Docker + cri-dockerd installation completed ==="
`
