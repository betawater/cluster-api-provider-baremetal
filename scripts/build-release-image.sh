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
K8S_VERSION="${K8S_VERSION:-v1.31.1}"
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

# Force Download (set to true to re-download all files)
FORCE_DOWNLOAD="${FORCE_DOWNLOAD:-false}"

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
    
    # Create binary directories for all OS/arch combinations
    for os in "${OS_FAMILIES[@]}"; do
        for arch in "${ARCHS[@]}"; do
            mkdir -p "$OUTPUT_DIR/binaries/kubernetes/${K8S_VERSION}/${os}/${arch}"
        done
    done
    
    # Create binary directories for linux-only components (shared across OS families)
    for arch in "${ARCHS[@]}"; do
        mkdir -p "$OUTPUT_DIR/binaries/containerd/${CONTAINERD_VERSION}/linux/${arch}"
        mkdir -p "$OUTPUT_DIR/binaries/helm/${HELM_VERSION}/linux/${arch}"
        mkdir -p "$OUTPUT_DIR/binaries/cni-plugins/${CNI_PLUGINS_VERSION}/linux/${arch}"
    done
    
    # Create addon directories
    mkdir -p "$OUTPUT_DIR/addons/calico/${CALICO_VERSION}/charts"
    mkdir -p "$OUTPUT_DIR/addons/ceph-csi/${CEPH_CSI_VERSION}/charts"
    mkdir -p "$OUTPUT_DIR/addons/metallb/${METALLB_VERSION}/manifests"
    mkdir -p "$OUTPUT_DIR/addons/gateway-api/${GATEWAY_API_VERSION}/manifests"
    mkdir -p "$OUTPUT_DIR/addons/capi-core/${CAPI_CORE_VERSION}/manifests"
    mkdir -p "$OUTPUT_DIR/addons/envoy-gateway/v1.0.0/charts"
    mkdir -p "$OUTPUT_DIR/checksums"
    
    # Copy existing scripts to component directories
    if [ -d "release-image/binaries/kubernetes" ]; then
        cp release-image/binaries/kubernetes/*/upgrade.sh "$OUTPUT_DIR/binaries/kubernetes/${K8S_VERSION}/" 2>/dev/null || true
        cp release-image/binaries/kubernetes/*/rollback.sh "$OUTPUT_DIR/binaries/kubernetes/${K8S_VERSION}/" 2>/dev/null || true
    fi
    if [ -d "release-image/binaries/containerd" ]; then
        cp release-image/binaries/containerd/*/upgrade.sh "$OUTPUT_DIR/binaries/containerd/${CONTAINERD_VERSION}/" 2>/dev/null || true
        cp release-image/binaries/containerd/*/rollback.sh "$OUTPUT_DIR/binaries/containerd/${CONTAINERD_VERSION}/" 2>/dev/null || true
    fi
    if [ -d "release-image/addons" ]; then
        for addon in calico ceph-csi metallb gateway-api capi-core envoy-gateway; do
            cp release-image/addons/${addon}/*/rollback.sh "$OUTPUT_DIR/addons/${addon}/" 2>/dev/null || true
        done
    fi
    
    log_success "Directory structure created: $OUTPUT_DIR"
}

# =============================================================================
# Kubernetes Binaries
# =============================================================================

download_kubernetes() {
    log_info "Downloading Kubernetes binaries..."
    
    for arch in "${ARCHS[@]}"; do
        # Check if binaries already exist for all OS families
        local all_exist=true
        for os in "${OS_FAMILIES[@]}"; do
            local output_dir="$OUTPUT_DIR/binaries/kubernetes/${K8S_VERSION}/${os}/${arch}"
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
            for os in "${OS_FAMILIES[@]}"; do
                local output_dir="$OUTPUT_DIR/binaries/kubernetes/${K8S_VERSION}/${os}/${arch}"
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
        local output_dir="$OUTPUT_DIR/binaries/containerd/${CONTAINERD_VERSION}/linux/${arch}"
        local output_file="$output_dir/containerd.tar.gz"
        
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
# Helm
# =============================================================================

download_helm() {
    log_info "Downloading Helm..."
    
    for arch in "${ARCHS[@]}"; do
        local output_dir="$OUTPUT_DIR/binaries/helm/${HELM_VERSION}/linux/${arch}"
        local output_file="$output_dir/helm.tar.gz"
        
        if [ -f "$output_file" ] && [ "$FORCE_DOWNLOAD" = "false" ]; then
            log_info "  Helm for $arch already exists, skipping..."
            continue
        fi
        
        log_info "  Downloading for $arch..."
        local url="https://get.helm.sh/helm-${HELM_VERSION}-linux-${arch}.tar.gz"
        
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
        local output_dir="$OUTPUT_DIR/binaries/cni-plugins/${CNI_PLUGINS_VERSION}/linux/${arch}"
        local output_file="$output_dir/cni-plugins.tgz"
        
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
# Container Images
# =============================================================================

get_image_path() {
    local image=$1
    # Parse image: registry/repo/image:tag or registry/image:tag
    local registry repository image_name tag
    
    if [[ "$image" == *"/"*":"* ]]; then
        registry=$(echo "$image" | cut -d'/' -f1)
        local rest=$(echo "$image" | cut -d'/' -f2-)
        image_name=$(echo "$rest" | cut -d':' -f1)
        tag=$(echo "$rest" | cut -d':' -f2)
        # Check if there's a repository (e.g., calico/node)
        if [[ "$image_name" == *"/"* ]]; then
            repository=$(echo "$image_name" | cut -d'/' -f1)
            image_name=$(echo "$image_name" | cut -d'/' -f2)
            echo "${OUTPUT_DIR}/images/${registry}/${repository}/${image_name}/${tag}.tar"
        else
            echo "${OUTPUT_DIR}/images/${registry}/${image_name}/${tag}.tar"
        fi
    else
        # Default for simple images like pause:3.9
        image_name=$(echo "$image" | cut -d':' -f1)
        tag=$(echo "$image" | cut -d':' -f2)
        echo "${OUTPUT_DIR}/images/${image_name}/${tag}.tar"
    fi
}

pull_and_save_images() {
    log_info "Pulling and saving container images..."
    
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
        local output_file=$(get_image_path "$image")
        local output_dir=$(dirname "$output_file")
        mkdir -p "$output_dir"
        
        if [ -f "$output_file" ] && [ "$FORCE_DOWNLOAD" = "false" ]; then
            log_info "  $image already exists, skipping..."
            continue
        fi
        
        log_info "  Pulling $image..."
        if docker pull "$image"; then
            # Save as tar file
            docker save -o "$output_file" "$image"
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
    
    # Add Helm repositories
    log_info "  Adding Helm repositories..."
    helm repo add projectcalico https://docs.tigera.io/calico/charts --force-update
    helm repo add ceph-csi https://ceph.github.io/csi-charts --force-update
    helm repo update
    
    # Calico
    local calico_file="$OUTPUT_DIR/addons/calico/${CALICO_VERSION}/charts/tigera-operator.tgz"
    if [ -f "$calico_file" ] && [ "$FORCE_DOWNLOAD" = "false" ]; then
        log_info "  Calico chart already exists, skipping..."
    else
        log_info "  Downloading Calico chart..."
        helm pull projectcalico/tigera-operator --version ${CALICO_VERSION} \
            -d "$OUTPUT_DIR/addons/calico/${CALICO_VERSION}/charts"
        log_success "  Downloaded Calico chart"
    fi
    
    # Ceph CSI
    local ceph_csi_file="$OUTPUT_DIR/addons/ceph-csi/${CEPH_CSI_VERSION}/charts/ceph-csi-rbd.tgz"
    if [ -f "$ceph_csi_file" ] && [ "$FORCE_DOWNLOAD" = "false" ]; then
        log_info "  Ceph CSI chart already exists, skipping..."
    else
        log_info "  Downloading Ceph CSI chart..."
        helm pull ceph-csi/ceph-csi-rbd --version ${CEPH_CSI_VERSION} \
            -d "$OUTPUT_DIR/addons/ceph-csi/${CEPH_CSI_VERSION}/charts"
        log_success "  Downloaded Ceph CSI chart"
    fi
    
    # CAPI Core - Download manifest from GitHub releases
    local capi_file="$OUTPUT_DIR/addons/capi-core/${CAPI_CORE_VERSION}/manifests/cluster-api-components.yaml"
    if [ -f "$capi_file" ] && [ "$FORCE_DOWNLOAD" = "false" ]; then
        log_info "  CAPI Core manifest already exists, skipping..."
    else
        log_info "  Downloading CAPI Core manifest..."
        mkdir -p "$(dirname "$capi_file")"
        curl -fSL "https://github.com/kubernetes-sigs/cluster-api/releases/download/${CAPI_CORE_VERSION}/cluster-api-components.yaml" \
            -o "$capi_file"
        log_success "  Downloaded CAPI Core manifest"
    fi
    
    log_success "Helm charts downloaded"
}

# =============================================================================
# Kubernetes Manifests
# =============================================================================

generate_manifests() {
    log_info "Generating Kubernetes manifests..."
    
    # MetalLB
    local metallb_dir="$OUTPUT_DIR/addons/metallb/${METALLB_VERSION}/manifests"
    mkdir -p "$metallb_dir"
    local metallb_file="$metallb_dir/metallb-native.yaml"
    if [ -f "$metallb_file" ] && [ "$FORCE_DOWNLOAD" = "false" ]; then
        log_info "  MetalLB manifest already exists, skipping..."
    else
        log_info "  Downloading MetalLB manifest..."
        curl -fSL "https://raw.githubusercontent.com/metallb/metallb/${METALLB_VERSION}/config/manifests/metallb-native.yaml" \
            -o "$metallb_file"
        log_success "  Downloaded MetalLB manifest"
    fi
    
    # Gateway API
    local gateway_dir="$OUTPUT_DIR/addons/gateway-api/${GATEWAY_API_VERSION}/manifests"
    mkdir -p "$gateway_dir"
    local gateway_file="$gateway_dir/standard-install.yaml"
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
    
    # This would generate a complete release.json matching the downloaded files
    # For now, we'll use the existing template
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
    echo "CAPBM Release Image Builder"
    echo "============================================================"
    echo "CAPBM Version: $RELEASE_VERSION"
    echo "Kubernetes: $K8S_VERSION"
    echo "Containerd: $CONTAINERD_VERSION"
    echo "Helm: $HELM_VERSION"
    echo "CNI Plugins: $CNI_PLUGINS_VERSION"
    echo "Calico: $CALICO_VERSION"
    echo "Ceph CSI: $CEPH_CSI_VERSION"
    echo "MetalLB: $METALLB_VERSION"
    echo "Gateway API: $GATEWAY_API_VERSION"
    echo "Architectures: ${ARCHS[*]}"
    echo "Force Download: $FORCE_DOWNLOAD"
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
