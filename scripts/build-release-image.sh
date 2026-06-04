#!/bin/bash
set -euo pipefail

# =============================================================================
# CAPBM Release Image Builder
# =============================================================================
# This script builds a complete release image with all components for
# offline/air-gapped installation.
#
# Supported Platforms:
#   - Linux amd64
#   - Linux arm64
#
# Supported OS Families:
#   - Ubuntu (deb)
#   - Debian (deb)
#   - CentOS/RHEL (rpm)
#
# Requirements:
#   - Docker
#   - Helm
#   - curl
#   - sha256sum
#   - Internet connection
# =============================================================================

# Version Configuration
RELEASE_VERSION="${RELEASE_VERSION:-v1.31.1}"
CONTAINERD_VERSION="${CONTAINERD_VERSION:-1.7.24}"
HELM_VERSION="${HELM_VERSION:-v3.15.0}"
CNI_PLUGINS_VERSION="${CNI_PLUGINS_VERSION:-v1.5.0}"
CALICO_VERSION="${CALICO_VERSION:-v3.28.1}"
CEPH_CSI_VERSION="${CEPH_CSI_VERSION:-v3.11.0}"
METALLB_VERSION="${METALLB_VERSION:-v0.14.8}"
GATEWAY_API_VERSION="${GATEWAY_API_VERSION:-v1.1.0}"
CAPI_CORE_VERSION="${CAPI_CORE_VERSION:-v1.7.0}"

# Architecture and OS Configuration
ARCHS=("amd64" "arm64")
OS_FAMILIES=("ubuntu" "debian" "centos")

# Output Directory
OUTPUT_DIR="${OUTPUT_DIR:-release-image}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# =============================================================================
# Helper Functions
# =============================================================================

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

check_requirements() {
    log_info "Checking requirements..."
    
    local missing=0
    
    if ! command -v docker &> /dev/null; then
        log_error "Docker is not installed"
        missing=1
    fi
    
    if ! command -v helm &> /dev/null; then
        log_error "Helm is not installed"
        missing=1
    fi
    
    if ! command -v curl &> /dev/null; then
        log_error "curl is not installed"
        missing=1
    fi
    
    if ! command -v sha256sum &> /dev/null; then
        log_error "sha256sum is not installed"
        missing=1
    fi
    
    if [ $missing -eq 1 ]; then
        log_error "Please install missing requirements and try again"
        exit 1
    fi
    
    log_success "All requirements met"
}

# =============================================================================
# Directory Structure
# =============================================================================

create_directory_structure() {
    log_info "Creating directory structure..."
    
    mkdir -p "$OUTPUT_DIR"/{binaries/{kubernetes/{ubuntu,debian,centos}/{amd64,arm64},containerd,helm,cni-plugins},charts,images,manifests,scripts,checksums}
    
    # Copy existing scripts
    if [ -d "release-image/scripts" ]; then
        cp release-image/scripts/*.sh "$OUTPUT_DIR/scripts/" 2>/dev/null || true
    fi
    
    log_success "Directory structure created: $OUTPUT_DIR"
}

# =============================================================================
# Kubernetes Binaries
# =============================================================================

download_kubernetes_deb() {
    local os=$1
    local arch=$2
    local version=${RELEASE_VERSION#v}
    local output_dir="$OUTPUT_DIR/binaries/kubernetes/$os/$arch"
    
    log_info "  Downloading Kubernetes $version for $os/$arch (deb)..."
    
    # Add Kubernetes apt repository and download packages
    # This requires apt to be configured, so we'll use direct download from official mirrors
    
    # For Ubuntu/Debian, packages are available from:
    # https://pkgs.k8s.io/core:/stable:/v${version%.*}/deb/
    
    local k8s_minor_version="${version%.*}"
    
    # Download kubeadm, kubelet, kubectl
    local packages=("kubeadm" "kubelet" "kubectl")
    
    for pkg in "${packages[@]}"; do
        local url="https://pkgs.k8s.io/core:/stable:/v${k8s_minor_version}/deb/${os}/pool/${pkg}_${version}-1.1_${arch}.deb"
        local output_file="$output_dir/${pkg}_${version}-1.1_${arch}.deb"
        
        log_info "    Downloading $pkg..."
        if curl -fSL -o "$output_file" "$url"; then
            log_success "    Downloaded $pkg"
        else
            log_warning "    Failed to download $pkg from official repo, trying alternative..."
            # Alternative: GitHub releases or other mirrors
            rm -f "$output_file"
        fi
    done
}

download_kubernetes_rpm() {
    local arch=$2
    local version=${RELEASE_VERSION#v}
    local output_dir="$OUTPUT_DIR/binaries/kubernetes/centos/$arch"
    
    log_info "  Downloading Kubernetes $version for centos/$arch (rpm)..."
    
    local k8s_minor_version="${version%.*}"
    
    # For CentOS/RHEL, packages are available from:
    # https://pkgs.k8s.io/core:/stable:/v${version%.*}/rpm/
    
    local packages=("kubeadm" "kubelet" "kubectl")
    
    for pkg in "${packages[@]}"; do
        local url="https://pkgs.k8s.io/core:/stable:/v${k8s_minor_version}/rpm/${pkg}-${version}-150500.1.1.${arch}.rpm"
        local output_file="$output_dir/${pkg}-${version}-150500.1.1.${arch}.rpm"
        
        log_info "    Downloading $pkg..."
        if curl -fSL -o "$output_file" "$url"; then
            log_success "    Downloaded $pkg"
        else
            log_warning "    Failed to download $pkg from official repo"
            rm -f "$output_file"
        fi
    done
}

download_kubernetes() {
    log_info "Downloading Kubernetes binaries..."
    
    for os in "ubuntu" "debian"; do
        for arch in "${ARCHS[@]}"; do
            download_kubernetes_deb "$os" "$arch"
        done
    done
    
    for arch in "${ARCHS[@]}"; do
        download_kubernetes_rpm "centos" "$arch"
    done
    
    log_success "Kubernetes binaries downloaded"
}

# =============================================================================
# Containerd
# =============================================================================

download_containerd() {
    log_info "Downloading containerd..."
    
    for arch in "${ARCHS[@]}"; do
        log_info "  Downloading for $arch..."
        local url="https://github.com/containerd/containerd/releases/download/v${CONTAINERD_VERSION}/containerd-${CONTAINERD_VERSION}-linux-${arch}.tar.gz"
        local output_file="$OUTPUT_DIR/binaries/containerd/containerd-${CONTAINERD_VERSION}-linux-${arch}.tar.gz"
        
        if curl -fSL -o "$output_file" "$url"; then
            log_success "  Downloaded containerd for $arch"
        else
            log_error "  Failed to download containerd for $arch"
            rm -f "$output_file"
        fi
    done
    
    log_success "Containerd downloaded"
}

# =============================================================================
# Helm
# =============================================================================

download_helm() {
    log_info "Downloading Helm..."
    
    for arch in "${ARCHS[@]}"; do
        log_info "  Downloading for $arch..."
        local url="https://get.helm.sh/helm-${HELM_VERSION}-linux-${arch}.tar.gz"
        local output_file="$OUTPUT_DIR/binaries/helm/helm-${HELM_VERSION}-linux-${arch}.tar.gz"
        
        if curl -fSL -o "$output_file" "$url"; then
            log_success "  Downloaded Helm for $arch"
        else
            log_error "  Failed to download Helm for $arch"
            rm -f "$output_file"
        fi
    done
    
    log_success "Helm downloaded"
}

# =============================================================================
# CNI Plugins
# =============================================================================

download_cni_plugins() {
    log_info "Downloading CNI plugins..."
    
    for arch in "${ARCHS[@]}"; do
        log_info "  Downloading for $arch..."
        local url="https://github.com/containernetworking/plugins/releases/download/${CNI_PLUGINS_VERSION}/cni-plugins-linux-${arch}-${CNI_PLUGINS_VERSION}.tgz"
        local output_file="$OUTPUT_DIR/binaries/cni-plugins/cni-plugins-linux-${arch}-${CNI_PLUGINS_VERSION}.tgz"
        
        if curl -fSL -o "$output_file" "$url"; then
            log_success "  Downloaded CNI plugins for $arch"
        else
            log_error "  Failed to download CNI plugins for $arch"
            rm -f "$output_file"
        fi
    done
    
    log_success "CNI plugins downloaded"
}

# =============================================================================
# Container Images
# =============================================================================

pull_and_save_images() {
    log_info "Pulling and saving container images..."
    
    local images=(
        "registry.k8s.io/kube-apiserver:${RELEASE_VERSION}"
        "registry.k8s.io/kube-controller-manager:${RELEASE_VERSION}"
        "registry.k8s.io/kube-scheduler:${RELEASE_VERSION}"
        "registry.k8s.io/kube-proxy:${RELEASE_VERSION}"
        "registry.k8s.io/pause:3.9"
        "registry.k8s.io/etcd:3.5.15-0"
        "registry.k8s.io/coredns/coredns:v1.11.1"
        "docker.io/calico/node:${CALICO_VERSION}"
        "docker.io/calico/kube-controllers:${CALICO_VERSION}"
        "docker.io/calico/cni:${CALICO_VERSION}"
        "quay.io/cephcsi/cephcsi:${CEPH_CSI_VERSION}"
        "quay.io/k8scsi/csi-attacher:v4.4.0"
        "quay.io/k8scsi/csi-provisioner:v3.6.0"
        "quay.io/k8scsi/csi-snapshotter:v6.3.0"
        "quay.io/k8scsi/csi-resizer:v1.9.0"
        "quay.io/k8scsi/csi-node-driver-registrar:v2.9.0"
        "quay.io/metallb/controller:${METALLB_VERSION}"
        "quay.io/metallb/speaker:${METALLB_VERSION}"
    )
    
    for image in "${images[@]}"; do
        log_info "  Pulling $image..."
        if docker pull "$image"; then
            # Save as tar file
            local safe_name=$(echo "$image" | tr '/:' '_')
            docker save -o "$OUTPUT_DIR/images/${safe_name}.tar" "$image"
            log_success "  Saved $image"
        else
            log_error "  Failed to pull $image"
        fi
    done
    
    log_success "Container images saved"
}

# =============================================================================
# Helm Charts
# =============================================================================

download_charts() {
    log_info "Downloading Helm charts..."
    
    # Add repositories
    helm repo add projectcalico https://docs.tigera.io/calico/charts --force-update
    helm repo add ceph-csi https://ceph.github.io/csi-charts --force-update
    helm repo update
    
    # Calico
    log_info "  Downloading Calico chart..."
    helm pull projectcalico/tigera-operator --version ${CALICO_VERSION} \
        -d "$OUTPUT_DIR/charts"
    log_success "  Downloaded Calico chart"
    
    # Ceph CSI
    log_info "  Downloading Ceph CSI chart..."
    helm pull ceph-csi/ceph-csi-rbd --version ${CEPH_CSI_VERSION} \
        -d "$OUTPUT_DIR/charts"
    log_success "  Downloaded Ceph CSI chart"
    
    # Note: CAPI Core chart needs to be built from CAPI source
    log_warning "  Note: CAPI Core chart needs to be built from CAPI source"
    
    log_success "Helm charts downloaded"
}

# =============================================================================
# Kubernetes Manifests
# =============================================================================

generate_manifests() {
    log_info "Generating Kubernetes manifests..."
    
    # MetalLB
    log_info "  Downloading MetalLB manifest..."
    curl -fSL "https://raw.githubusercontent.com/metallb/metallb/${METALLB_VERSION}/config/manifests/metallb-native.yaml" \
        -o "$OUTPUT_DIR/manifests/metallb-${METALLB_VERSION}.yaml"
    log_success "  Downloaded MetalLB manifest"
    
    # Gateway API
    log_info "  Downloading Gateway API manifest..."
    curl -fSL "https://github.com/kubernetes-sigs/gateway-api/releases/download/${GATEWAY_API_VERSION}/standard-install.yaml" \
        -o "$OUTPUT_DIR/manifests/gateway-api-${GATEWAY_API_VERSION}.yaml"
    log_success "  Downloaded Gateway API manifest"
    
    log_success "Kubernetes manifests generated"
}

# =============================================================================
# Checksums
# =============================================================================

generate_checksums() {
    log_info "Generating checksums..."
    
    cd "$OUTPUT_DIR"
    find . -type f \
        -not -name "*.sig" \
        -not -name "sha256sums.txt" \
        -not -path "./checksums/*" \
        -exec sha256sum {} \; > checksums/sha256sums.txt
    cd ..
    
    log_success "Checksums generated: $OUTPUT_DIR/checksums/sha256sums.txt"
}

# =============================================================================
# Release JSON
# =============================================================================

generate_release_json() {
    log_info "Generating release.json..."
    
    # This would generate a complete release.json matching the downloaded files
    # For now, we'll use the existing template
    if [ -f "release-image/release.json" ]; then
        cp release-image/release.json "$OUTPUT_DIR/release.json"
        log_success "Copied existing release.json"
    else
        log_warning "No existing release.json found, please create manually"
    fi
}

# =============================================================================
# Main
# =============================================================================

main() {
    echo "============================================================"
    echo "CAPBM Release Image Builder"
    echo "============================================================"
    echo "Release Version: $RELEASE_VERSION"
    echo "Containerd: $CONTAINERD_VERSION"
    echo "Helm: $HELM_VERSION"
    echo "CNI Plugins: $CNI_PLUGINS_VERSION"
    echo "Calico: $CALICO_VERSION"
    echo "Ceph CSI: $CEPH_CSI_VERSION"
    echo "MetalLB: $METALLB_VERSION"
    echo "Gateway API: $GATEWAY_API_VERSION"
    echo "Architectures: ${ARCHS[*]}"
    echo "Output Directory: $OUTPUT_DIR"
    echo "============================================================"
    echo ""
    
    check_requirements
    create_directory_structure
    download_kubernetes
    download_containerd
    download_helm
    download_cni_plugins
    pull_and_save_images
    download_charts
    generate_manifests
    generate_checksums
    generate_release_json
    
    echo ""
    echo "============================================================"
    log_success "Release image built successfully: $OUTPUT_DIR"
    echo "============================================================"
    echo ""
    echo "Next steps:"
    echo "1. Review the contents: ls -la $OUTPUT_DIR/"
    echo "2. Apply ReleaseImage: kubectl apply -f $OUTPUT_DIR/release.json"
    echo "3. Create ClusterVersion to trigger upgrade"
    echo "============================================================"
}

main "$@"
