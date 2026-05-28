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

const containerdUbuntuScript = `#!/bin/bash
set -euo pipefail

CONTAINERD_VERSION="${CONTAINERD_VERSION:-}"
REGISTRY_MIRRORS="${REGISTRY_MIRRORS:-}"
AIR_GAP_MODE="${AIR_GAP_MODE:-false}"
BASE_URL="${BASE_URL:-}"
LOCAL_PATH="${LOCAL_PATH:-}"

echo "=== Installing containerd (Ubuntu/Debian) ==="

if command -v containerd &>/dev/null; then
    current_version=$(containerd --version | awk '{print $3}')
    if [ -n "$CONTAINERD_VERSION" ] && [ "$current_version" != "$CONTAINERD_VERSION" ]; then
        echo "Upgrading containerd: $current_version -> $CONTAINERD_VERSION"
        apt-get remove -y containerd || true
    else
        echo "containerd already installed: $current_version"
        systemctl enable --now containerd
        configure_containerd
        exit 0
    fi
fi

if [ "$AIR_GAP_MODE" = "true" ]; then
    install_containerd_offline
else
    apt-get update
    apt-get install -y containerd
fi

systemctl enable --now containerd
configure_containerd

echo "=== containerd installation completed ==="

configure_containerd() {
    mkdir -p /etc/containerd
    if [ ! -f /etc/containerd/config.toml ]; then
        containerd config default > /etc/containerd/config.toml
    fi
    sed -i 's/SystemdCgroup = false/SystemdCgroup = true/' /etc/containerd/config.toml

    if [ -n "$REGISTRY_MIRRORS" ]; then
        IFS=',' read -ra MIRRORS <<< "$REGISTRY_MIRRORS"
        mirror_config=""
        for mirror in "${MIRRORS[@]}"; do
            mirror_config="${mirror_config}    endpoint = [\"${mirror}\"]\n"
        done
        cat >> /etc/containerd/config.toml << EOF
[plugins."io.containerd.grpc.v1.cri".registry.mirrors."docker.io"]
$(echo -e "$mirror_config")
EOF
    fi

    systemctl restart containerd
}

install_containerd_offline() {
    if [ -n "$LOCAL_PATH" ] && [ -d "$LOCAL_PATH" ]; then
        tar -C /usr/local -xzf "${LOCAL_PATH}/containerd.tar.gz"
    elif [ -n "$BASE_URL" ]; then
        curl -fsSL "${BASE_URL}/containerd.tar.gz" | tar -C /usr/local -xz
    else
        echo "ERROR: No offline source configured"
        exit 1
    fi
    cp /usr/local/bin/containerd /usr/bin/ 2>/dev/null || true
}
`

const containerdRHELScript = `#!/bin/bash
set -euo pipefail

CONTAINERD_VERSION="${CONTAINERD_VERSION:-}"
REGISTRY_MIRRORS="${REGISTRY_MIRRORS:-}"
AIR_GAP_MODE="${AIR_GAP_MODE:-false}"
BASE_URL="${BASE_URL:-}"
LOCAL_PATH="${LOCAL_PATH:-}"

echo "=== Installing containerd (RHEL/CentOS/Rocky) ==="

if command -v containerd &>/dev/null; then
    current_version=$(containerd --version | awk '{print $3}')
    if [ -n "$CONTAINERD_VERSION" ] && [ "$current_version" != "$CONTAINERD_VERSION" ]; then
        echo "Upgrading containerd: $current_version -> $CONTAINERD_VERSION"
        dnf remove -y containerd 2>/dev/null || yum remove -y containerd 2>/dev/null || true
    else
        echo "containerd already installed: $current_version"
        systemctl enable --now containerd
        configure_containerd
        exit 0
    fi
fi

if command -v dnf &>/dev/null; then
    PKG_MGR="dnf"
else
    PKG_MGR="yum"
fi

if [ "$AIR_GAP_MODE" = "true" ]; then
    install_containerd_offline
else
    $PKG_MGR install -y containerd
fi

systemctl enable --now containerd
configure_containerd

echo "=== containerd installation completed ==="

configure_containerd() {
    mkdir -p /etc/containerd
    if [ ! -f /etc/containerd/config.toml ]; then
        containerd config default > /etc/containerd/config.toml
    fi
    sed -i 's/SystemdCgroup = false/SystemdCgroup = true/' /etc/containerd/config.toml

    if [ -n "$REGISTRY_MIRRORS" ]; then
        IFS=',' read -ra MIRRORS <<< "$REGISTRY_MIRRORS"
        mirror_config=""
        for mirror in "${MIRRORS[@]}"; do
            mirror_config="${mirror_config}    endpoint = [\"${mirror}\"]\n"
        done
        cat >> /etc/containerd/config.toml << EOF
[plugins."io.containerd.grpc.v1.cri".registry.mirrors."docker.io"]
$(echo -e "$mirror_config")
EOF
    fi

    systemctl restart containerd
}

install_containerd_offline() {
    if [ -n "$LOCAL_PATH" ] && [ -d "$LOCAL_PATH" ]; then
        tar -C /usr/local -xzf "${LOCAL_PATH}/containerd.tar.gz"
    elif [ -n "$BASE_URL" ]; then
        curl -fsSL "${BASE_URL}/containerd.tar.gz" | tar -C /usr/local -xz
    else
        echo "ERROR: No offline source configured"
        exit 1
    fi
    cp /usr/local/bin/containerd /usr/bin/ 2>/dev/null || true
}
`

const containerdSUSEScript = `#!/bin/bash
set -euo pipefail

CONTAINERD_VERSION="${CONTAINERD_VERSION:-}"

echo "=== Installing containerd (SUSE) ==="

if command -v containerd &>/dev/null; then
    echo "containerd already installed: $(containerd --version)"
    systemctl enable --now containerd
    configure_containerd
    exit 0
fi

zypper refresh
zypper install -y containerd

systemctl enable --now containerd
configure_containerd

echo "=== containerd installation completed ==="

configure_containerd() {
    mkdir -p /etc/containerd
    if [ ! -f /etc/containerd/config.toml ]; then
        containerd config default > /etc/containerd/config.toml
    fi
    sed -i 's/SystemdCgroup = false/SystemdCgroup = true/' /etc/containerd/config.toml
    systemctl restart containerd
}
`

const containerdFlatcarScript = `#!/bin/bash
set -euo pipefail

echo "=== Verifying containerd (Flatcar) ==="

if command -v containerd &>/dev/null; then
    echo "containerd pre-installed: $(containerd --version)"
    mkdir -p /etc/containerd
    if [ ! -f /etc/containerd/config.toml ]; then
        containerd config default > /etc/containerd/config.toml
        sed -i 's/SystemdCgroup = false/SystemdCgroup = true/' /etc/containerd/config.toml
        systemctl restart containerd
    fi
    exit 0
fi

echo "ERROR: containerd not found on Flatcar"
exit 1
`

const kubernetesUbuntuScript = `#!/bin/bash
set -euo pipefail

K8S_VERSION="${K8S_VERSION:-}"
REPO_BASE_URL="${REPO_BASE_URL:-}"
REPO_GPG_KEY="${REPO_GPG_KEY:-}"
AIR_GAP_MODE="${AIR_GAP_MODE:-false}"
BASE_URL="${BASE_URL:-}"
LOCAL_PATH="${LOCAL_PATH:-}"
ROLE="${ROLE:-worker}"

echo "=== Installing Kubernetes components (Ubuntu/Debian) ==="

if command -v kubeadm &>/dev/null; then
    current_version=$(kubeadm version -o short 2>/dev/null || echo "")
    if [ "$current_version" = "v${K8S_VERSION}" ]; then
        echo "Kubernetes components already installed: $current_version"
        systemctl enable kubelet
        exit 0
    fi
    echo "Version mismatch: current=$current_version, desired=v${K8S_VERSION}"
fi

if [ "$AIR_GAP_MODE" = "true" ]; then
    install_k8s_offline
else
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
fi

systemctl enable kubelet

echo "=== Kubernetes components installation completed ==="
echo "  kubeadm: $(kubeadm version -o short)"
echo "  kubelet: $(kubelet --version)"

install_k8s_offline() {
    if [ -n "$LOCAL_PATH" ] && [ -d "$LOCAL_PATH" ]; then
        install -m 0755 "${LOCAL_PATH}/kubeadm" /usr/bin/kubeadm
        install -m 0755 "${LOCAL_PATH}/kubelet" /usr/bin/kubelet
        install -m 0755 "${LOCAL_PATH}/kubectl" /usr/bin/kubectl
    elif [ -n "$BASE_URL" ]; then
        for binary in kubeadm kubelet kubectl; do
            curl -fsSL "${BASE_URL}/${binary}" -o "/tmp/${binary}"
            install -m 0755 "/tmp/${binary}" "/usr/bin/${binary}"
        done
    else
        echo "ERROR: No offline source configured"
        exit 1
    fi

    cat > /etc/systemd/system/kubelet.service << 'EOF'
[Unit]
Description=kubelet: The Kubernetes Node Agent
Documentation=https://kubernetes.io/docs/home/
Wants=network-online.target
After=network-online.target

[Service]
ExecStart=/usr/bin/kubelet
Restart=always
StartLimitInterval=0
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
}
`

const kubernetesRHELScript = `#!/bin/bash
set -euo pipefail

K8S_VERSION="${K8S_VERSION:-}"
REPO_BASE_URL="${REPO_BASE_URL:-}"
REPO_GPG_KEY="${REPO_GPG_KEY:-}"
AIR_GAP_MODE="${AIR_GAP_MODE:-false}"
BASE_URL="${BASE_URL:-}"
LOCAL_PATH="${LOCAL_PATH:-}"
ROLE="${ROLE:-worker}"

echo "=== Installing Kubernetes components (RHEL/CentOS/Rocky) ==="

if command -v kubeadm &>/dev/null; then
    current_version=$(kubeadm version -o short 2>/dev/null || echo "")
    if [ "$current_version" = "v${K8S_VERSION}" ]; then
        echo "Kubernetes components already installed: $current_version"
        systemctl enable kubelet
        exit 0
    fi
    echo "Version mismatch: current=$current_version, desired=v${K8S_VERSION}"
fi

if command -v dnf &>/dev/null; then
    PKG_MGR="dnf"
else
    PKG_MGR="yum"
fi

if [ "$AIR_GAP_MODE" = "true" ]; then
    install_k8s_offline
else
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
fi

systemctl enable kubelet

echo "=== Kubernetes components installation completed ==="
echo "  kubeadm: $(kubeadm version -o short)"
echo "  kubelet: $(kubelet --version)"

install_k8s_offline() {
    if [ -n "$LOCAL_PATH" ] && [ -d "$LOCAL_PATH" ]; then
        install -m 0755 "${LOCAL_PATH}/kubeadm" /usr/bin/kubeadm
        install -m 0755 "${LOCAL_PATH}/kubelet" /usr/bin/kubelet
        install -m 0755 "${LOCAL_PATH}/kubectl" /usr/bin/kubectl
    elif [ -n "$BASE_URL" ]; then
        for binary in kubeadm kubelet kubectl; do
            curl -fsSL "${BASE_URL}/${binary}" -o "/tmp/${binary}"
            install -m 0755 "/tmp/${binary}" "/usr/bin/${binary}"
        done
    else
        echo "ERROR: No offline source configured"
        exit 1
    fi

    cat > /etc/systemd/system/kubelet.service << 'EOF'
[Unit]
Description=kubelet: The Kubernetes Node Agent
Documentation=https://kubernetes.io/docs/home/
Wants=network-online.target
After=network-online.target

[Service]
ExecStart=/usr/bin/kubelet
Restart=always
StartLimitInterval=0
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF

    systemctl daemon-reload
}
`

const kubernetesSUSEScript = `#!/bin/bash
set -euo pipefail

K8S_VERSION="${K8S_VERSION:-}"

echo "=== Installing Kubernetes components (SUSE) ==="

if command -v kubeadm &>/dev/null; then
    current_version=$(kubeadm version -o short 2>/dev/null || echo "")
    if [ "$current_version" = "v${K8S_VERSION}" ]; then
        echo "Kubernetes components already installed: $current_version"
        systemctl enable kubelet
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

systemctl enable kubelet

echo "=== Kubernetes components installation completed ==="
`

const kubernetesFlatcarScript = `#!/bin/bash
set -euo pipefail

K8S_VERSION="${K8S_VERSION:-}"
INSTALL_PREFIX="/opt/bin"

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

mkdir -p /etc/systemd/system/kubelet.service.d
cat > /etc/systemd/system/kubelet.service.d/10-kubeadm.conf << 'EOF'
[Service]
Environment="KUBELET_EXTRA_ARGS=--container-runtime-endpoint=unix:///run/containerd/containerd.sock"
EOF

systemctl daemon-reload
systemctl enable kubelet

echo "=== Kubernetes binaries installation completed ==="
`
