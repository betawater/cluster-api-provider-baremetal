#!/bin/bash
# =============================================================================
# CAPBM Release Image Builder (No Docker/Helm Required)
# =============================================================================
# This script builds a complete release image with all components for
# offline/air-gapped installation without requiring Docker or Helm CLI.
#
# Requirements:
#   - curl
#   - sha256sum
#   - skopeo OR crane (for pulling container images)
#
# Supported Architectures:
#   - linux/amd64
#   - linux/arm64
#
# Supported OS Families:
#   - Ubuntu (deb)
#   - Debian (deb)
#   - CentOS/RHEL (rpm)
# =============================================================================

set -euo pipefail

# Version Configuration
RELEASE_VERSION="${RELEASE_VERSION:-v1.31.1}"
K8S_VERSION="${K8S_VERSION:-v1.31.1}"
CONTAINERD_VERSION="${CONTAINERD_VERSION:-1.7.24}"
CNI_PLUGINS_VERSION="${CNI_PLUGINS_VERSION:-v1.5.0}"
CALICO_VERSION="${CALICO_VERSION:-v3.28.1}"
CEPH_CSI_VERSION="${CEPH_CSI_VERSION:-v3.11.0}"
METALLB_VERSION="${METALLB_VERSION:-v0.14.8}"
GATEWAY_API_VERSION="${GATEWAY_API_VERSION:-v1.1.0}"

# Architecture and OS Configuration
ARCHS=("amd64" "arm64")
OS_FAMILIES=("ubuntu" "debian" "centos")

# Output Directory
OUTPUT_DIR="${OUTPUT_DIR:-release-image}"

# Force Download (set to true to re-download all files)
FORCE_DOWNLOAD="${FORCE_DOWNLOAD:-false}"

# Image Tool (skopeo or crane)
IMAGE_TOOL="${IMAGE_TOOL:-auto}"

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

# =============================================================================
# Requirements Check
# =============================================================================

check_requirements() {
    log_info "Checking requirements..."
    
    local missing=0
    
    if ! command -v curl &> /dev/null; then
        log_error "curl is not installed"
        missing=1
    fi
    
    if ! command -v sha256sum &> /dev/null; then
        log_error "sha256sum is not installed"
        missing=1
    fi
    
    # Check for image tool (skopeo or crane)
    if [ "$IMAGE_TOOL" = "auto" ]; then
        if command -v skopeo &> /dev/null; then
            IMAGE_TOOL="skopeo"
            log_info "Using skopeo for image operations"
        elif command -v crane &> /dev/null; then
            IMAGE_TOOL="crane"
            log_info "Using crane for image operations"
        else
            log_error "Neither skopeo nor crane is installed"
            log_info ""
            log_info "Install skopeo:"
            log_info "  Ubuntu/Debian: sudo apt-get install -y skopeo"
            log_info "  CentOS/RHEL:   sudo yum install -y skopeo"
            log_info "  Or download:   https://github.com/containers/skopeo/releases"
            log_info ""
            log_info "Install crane:"
            log_info "  Download: https://github.com/google/go-containerregistry/releases"
            missing=1
        fi
    else
        if ! command -v "$IMAGE_TOOL" &> /dev/null; then
            log_error "$IMAGE_TOOL is not installed"
            missing=1
        else
            log_info "Using $IMAGE_TOOL for image operations"
        fi
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
    
    mkdir -p "$OUTPUT_DIR"/{binaries/{kubernetes/{ubuntu,debian,centos}/{amd64,arm64},containerd,helm,cni-plugins},images,charts,manifests,scripts,checksums}
    
    # Copy existing scripts if available
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
    
    local k8s_minor_version="${version%.*}"
    local packages=("kubeadm" "kubelet" "kubectl")
    
    for pkg in "${packages[@]}"; do
        local url="https://pkgs.k8s.io/core:/stable:/v${k8s_minor_version}/deb/${os}/pool/${pkg}_${version}-1.1_${arch}.deb"
        local output_file="$output_dir/${pkg}_${version}-1.1_${arch}.deb"
        
        log_info "    Downloading $pkg..."
        if curl -fSL -o "$output_file" "$url"; then
            log_success "    Downloaded $pkg"
        else
            log_warning "    Failed to download $pkg from official repo"
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
    
    for arch in "${ARCHS[@]}"; do
        # Check if binaries already exist for all OS families
        local all_exist=true
        for os in "ubuntu" "debian" "centos"; do
            local output_dir="$OUTPUT_DIR/binaries/kubernetes/$os/$arch"
            if [ ! -f "$output_dir/kubeadm" ] || [ ! -f "$output_dir/kubelet" ] || [ ! -f "$output_dir/kubectl" ]; then
                all_exist=false
                break
            fi
        done
        
        if [ "$all_exist" = true ] && [ "$FORCE_DOWNLOAD" = "false" ]; then
            log_info "  Kubernetes binaries for $arch already exist, skipping..."
            continue
        fi
        
        log_info "  Downloading Kubernetes server for $arch..."
        
        # Download Kubernetes server package from dl.k8s.io
        local url="https://dl.k8s.io/${K8S_VERSION}/kubernetes-server-linux-${arch}.tar.gz"
        local output_file="/tmp/kubernetes-server-linux-${arch}.tar.gz"
        
        log_info "    Downloading from $url..."
        if curl -fSL -o "$output_file" "$url"; then
            log_success "    Downloaded Kubernetes server package"
            
            # Extract and copy binaries
            log_info "    Extracting binaries..."
            local temp_dir="/tmp/k8s-extract-${arch}"
            mkdir -p "$temp_dir"
            tar -xzf "$output_file" -C "$temp_dir"
            
            # Copy binaries to output directories for all OS families
            for os in "ubuntu" "debian" "centos"; do
                local output_dir="$OUTPUT_DIR/binaries/kubernetes/$os/$arch"
                mkdir -p "$output_dir"
                cp "$temp_dir/kubernetes/server/bin/kubeadm" "$output_dir/"
                cp "$temp_dir/kubernetes/server/bin/kubelet" "$output_dir/"
                cp "$temp_dir/kubernetes/server/bin/kubectl" "$output_dir/"
                log_success "    Copied binaries to $os/$arch"
            done
            
            # Cleanup
            rm -rf "$temp_dir" "$output_file"
        else
            log_error "    Failed to download Kubernetes server for $arch"
            rm -f "$output_file"
        fi
    done
    
    log_success "Kubernetes binaries downloaded"
}

# =============================================================================
# Containerd
# =============================================================================

download_containerd() {
    log_info "Downloading containerd..."
    
    for arch in "${ARCHS[@]}"; do
        local output_file="$OUTPUT_DIR/binaries/containerd/containerd-${CONTAINERD_VERSION}-linux-${arch}.tar.gz"
        
        if [ -f "$output_file" ] && [ "$FORCE_DOWNLOAD" = "false" ]; then
            log_info "  Containerd for $arch already exists, skipping..."
            continue
        fi
        
        log_info "  Downloading for $arch..."
        local url="https://github.com/containerd/containerd/releases/download/v${CONTAINERD_VERSION}/containerd-${CONTAINERD_VERSION}-linux-${arch}.tar.gz"
        
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
# CNI Plugins
# =============================================================================

download_cni_plugins() {
    log_info "Downloading CNI plugins..."
    
    for arch in "${ARCHS[@]}"; do
        local output_file="$OUTPUT_DIR/binaries/cni-plugins/cni-plugins-linux-${arch}-${CNI_PLUGINS_VERSION}.tgz"
        
        if [ -f "$output_file" ] && [ "$FORCE_DOWNLOAD" = "false" ]; then
            log_info "  CNI plugins for $arch already exist, skipping..."
            continue
        fi
        
        log_info "  Downloading for $arch..."
        local url="https://github.com/containernetworking/plugins/releases/download/${CNI_PLUGINS_VERSION}/cni-plugins-linux-${arch}-${CNI_PLUGINS_VERSION}.tgz"
        
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
# Container Images (using skopeo or crane, no Docker required)
# =============================================================================

pull_image_with_skopeo() {
    local image=$1
    local output_file=$2
    
    skopeo copy "docker://$image" "docker-archive:$output_file"
}

pull_image_with_crane() {
    local image=$1
    local output_file=$2
    
    crane pull "$image" "$output_file"
}

pull_images() {
    log_info "Pulling container images using $IMAGE_TOOL..."
    
    local images=(
        "registry.k8s.io/kube-apiserver:${K8S_VERSION}"
        "registry.k8s.io/kube-controller-manager:${K8S_VERSION}"
        "registry.k8s.io/kube-scheduler:${K8S_VERSION}"
        "registry.k8s.io/kube-proxy:${K8S_VERSION}"
        "registry.k8s.io/pause:3.9"
        "registry.k8s.io/etcd:3.5.15-0"
        "registry.k8s.io/coredns/coredns:v1.11.1"
        "docker.io/calico/node:${CALICO_VERSION}"
        "docker.io/calico/kube-controllers:${CALICO_VERSION}"
        "docker.io/calico/cni:${CALICO_VERSION}"
        "quay.io/cephcsi/cephcsi:${CEPH_CSI_VERSION}"
        "registry.k8s.io/sig-storage/csi-attacher:v4.4.0"
        "registry.k8s.io/sig-storage/csi-provisioner:v3.6.0"
        "registry.k8s.io/sig-storage/csi-snapshotter:v6.3.0"
        "registry.k8s.io/sig-storage/csi-resizer:v1.9.0"
        "registry.k8s.io/sig-storage/csi-node-driver-registrar:v2.9.0"
        "quay.io/metallb/controller:${METALLB_VERSION}"
        "quay.io/metallb/speaker:${METALLB_VERSION}"
    )

    for image in "${images[@]}"; do
        local safe_name=$(echo "$image" | tr '/:' '_')
        local output_file="$OUTPUT_DIR/images/${safe_name}.tar"
        
        if [ -f "$output_file" ] && [ "$FORCE_DOWNLOAD" = "false" ]; then
            log_info "  $image already exists, skipping..."
            continue
        fi
        
        log_info "  Pulling $image..."
        
        if [ "$IMAGE_TOOL" = "skopeo" ]; then
            if pull_image_with_skopeo "$image" "$output_file"; then
                log_success "  Saved $image"
            else
                log_error "  Failed to pull $image"
                rm -f "$output_file"
            fi
        else
            if pull_image_with_crane "$image" "$output_file"; then
                log_success "  Saved $image"
            else
                log_error "  Failed to pull $image"
                rm -f "$output_file"
            fi
        fi
    done
    
    log_success "Container images pulled"
}

# =============================================================================
# Helm Charts (direct download, no Helm CLI required)
# =============================================================================

download_charts() {
    log_info "Downloading Helm charts..."
    
    # Calico - Download from Helm repo index
    local calico_file="$OUTPUT_DIR/charts/tigera-operator-${CALICO_VERSION}.tgz"
    if [ -f "$calico_file" ] && [ "$FORCE_DOWNLOAD" = "false" ]; then
        log_info "  Calico chart already exists, skipping..."
    else
        log_info "  Downloading Calico chart..."
        local calico_chart_url=$(curl -sL "https://docs.tigera.io/calico/charts/index.yaml" | \
            grep -A5 "tigera-operator-${CALICO_VERSION}" | \
            grep "url:" | head -1 | \
            sed 's/.*url: //')
        
        if [ -n "$calico_chart_url" ]; then
            curl -fSL -o "$calico_file" "$calico_chart_url"
            log_success "  Downloaded Calico chart"
        else
            log_warning "  Failed to get Calico chart URL, trying alternative..."
            # Alternative: try direct GitHub URL
            curl -fSL -o "$calico_file" \
              "https://github.com/projectcalico/calico/releases/download/${CALICO_VERSION}/tigera-operator-${CALICO_VERSION}.tgz" || \
            log_error "  Failed to download Calico chart"
        fi
    fi
    
    # Ceph CSI - Download from Helm repo index
    local ceph_csi_file="$OUTPUT_DIR/charts/ceph-csi-rbd-${CEPH_CSI_VERSION}.tgz"
    if [ -f "$ceph_csi_file" ] && [ "$FORCE_DOWNLOAD" = "false" ]; then
        log_info "  Ceph CSI chart already exists, skipping..."
    else
        log_info "  Downloading Ceph CSI chart..."
        local ceph_csi_chart_url=$(curl -sL "https://ceph.github.io/csi-charts/index.yaml" | \
            grep -A5 "ceph-csi-rbd-${CEPH_CSI_VERSION}" | \
            grep "url:" | head -1 | \
            sed 's/.*url: //')
        
        if [ -n "$ceph_csi_chart_url" ]; then
            curl -fSL -o "$ceph_csi_file" "$ceph_csi_chart_url"
            log_success "  Downloaded Ceph CSI chart"
        else
            log_warning "  Failed to get Ceph CSI chart URL, trying alternative..."
            # Alternative: try direct GitHub URL
            curl -fSL -o "$ceph_csi_file" \
              "https://github.com/ceph/ceph-csi/releases/download/${CEPH_CSI_VERSION}/ceph-csi-rbd-${CEPH_CSI_VERSION}.tgz" || \
            log_error "  Failed to download Ceph CSI chart"
        fi
    fi
    
    log_success "Helm charts downloaded"
}

# =============================================================================
# Kubernetes Manifests
# =============================================================================

generate_manifests() {
    log_info "Generating Kubernetes manifests..."
    
    # MetalLB
    local metallb_file="$OUTPUT_DIR/manifests/metallb-${METALLB_VERSION}.yaml"
    if [ -f "$metallb_file" ] && [ "$FORCE_DOWNLOAD" = "false" ]; then
        log_info "  MetalLB manifest already exists, skipping..."
    else
        log_info "  Downloading MetalLB manifest..."
        curl -fSL "https://raw.githubusercontent.com/metallb/metallb/${METALLB_VERSION}/config/manifests/metallb-native.yaml" \
            -o "$metallb_file"
        log_success "  Downloaded MetalLB manifest"
    fi
    
    # Gateway API
    local gateway_file="$OUTPUT_DIR/manifests/gateway-api-${GATEWAY_API_VERSION}.yaml"
    if [ -f "$gateway_file" ] && [ "$FORCE_DOWNLOAD" = "false" ]; then
        log_info "  Gateway API manifest already exists, skipping..."
    else
        log_info "  Downloading Gateway API manifest..."
        curl -fSL "https://github.com/kubernetes-sigs/gateway-api/releases/download/${GATEWAY_API_VERSION}/standard-install.yaml" \
            -o "$gateway_file"
        log_success "  Downloaded Gateway API manifest"
    fi
    
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
    
    local source_file="release-image/release.json"
    local dest_file="$OUTPUT_DIR/release.json"
    
    # Skip if source and destination are the same
    if [ "$(realpath "$source_file" 2>/dev/null)" = "$(realpath "$dest_file" 2>/dev/null)" ]; then
        log_info "release.json already exists in output directory, skipping..."
        return
    fi
    
    if [ -f "$source_file" ]; then
        cp "$source_file" "$dest_file"
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
    echo "CAPBM Release Image Builder (No Docker/Helm Required)"
    echo "============================================================"
    echo "CAPBM Version: $RELEASE_VERSION"
    echo "Kubernetes: $K8S_VERSION"
    echo "Containerd: $CONTAINERD_VERSION"
    echo "CNI Plugins: $CNI_PLUGINS_VERSION"
    echo "Calico: $CALICO_VERSION"
    echo "Ceph CSI: $CEPH_CSI_VERSION"
    echo "MetalLB: $METALLB_VERSION"
    echo "Gateway API: $GATEWAY_API_VERSION"
    echo "Architectures: ${ARCHS[*]}"
    echo "Image Tool: $IMAGE_TOOL"
    echo "Force Download: $FORCE_DOWNLOAD"
    echo "Output Directory: $OUTPUT_DIR"
    echo "============================================================"
    echo ""
    
    check_requirements
    create_directory_structure
    download_kubernetes
    download_containerd
    download_cni_plugins
    pull_images
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
