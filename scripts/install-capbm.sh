#!/bin/bash
set -euo pipefail

# CAPBM Installation Script
# This script installs CAPBM (Cluster API Provider Bare Metal) with all required components.
# It automatically enables the ClusterTopology feature gate required for ClusterClass support.

echo "=== CAPBM Installation Script ==="
echo ""

# Check prerequisites
echo "Checking prerequisites..."

if ! command -v clusterctl &> /dev/null; then
    echo "ERROR: clusterctl is not installed. Please install clusterctl first."
    echo "See: https://cluster-api.sigs.k8s.io/user/quick-start.html#install-clusterctl"
    exit 1
fi

if ! command -v kubectl &> /dev/null; then
    echo "ERROR: kubectl is not installed. Please install kubectl first."
    exit 1
fi

echo "✓ clusterctl is installed"
echo "✓ kubectl is installed"
echo ""

# Step 1: Install CAPI core components with ClusterTopology enabled
echo "Step 1: Installing CAPI core components with ClusterTopology enabled..."
clusterctl init \
  --core cluster-api \
  --bootstrap kubeadm \
  --control-plane kubeadm \
  --feature-gates=ClusterTopology=true

echo "✓ CAPI core components installed"
echo ""

# Step 2: Patch kubeadm control plane deployment to enable ClusterTopology
# This is a workaround because the kubeadm control plane provider defaults to ClusterTopology=false
echo "Step 2: Enabling ClusterTopology on kubeadm control plane provider..."

kubectl patch deployment capi-kubeadm-control-plane-controller-manager \
  -n capi-kubeadm-control-plane-system \
  --type='json' \
  -p='[{"op": "replace", "path": "/spec/template/spec/containers/0/args", "value": ["--feature-gates=MachinePool=true,ClusterTopology=true,KubeadmBootstrapFormatIgnition=false,PriorityQueue=true,ReconcilerRateLimiting=true,InPlaceUpdates=false,MachineTaintPropagation=false","--leader-elect","--diagnostics-address=:8443","--insecure-diagnostics=false"]}]'

# Wait for rollout
echo "Waiting for kubeadm control plane controller to restart..."
kubectl rollout status deployment capi-kubeadm-control-plane-controller-manager -n capi-kubeadm-control-plane-system --timeout=120s

echo "✓ Kubeadm control plane provider updated with ClusterTopology=true"
echo ""

# Step 3: Install CAPBM CRDs and Controller
echo "Step 3: Installing CAPBM CRDs and Controller..."

# Apply CRDs first
kubectl apply -k modules/capbm/config/crd/

# Apply controller and RBAC
kubectl apply -k modules/capbm/config/

# Wait for CAPBM controller to be ready
echo "Waiting for CAPBM controller to be ready..."
kubectl rollout status deployment capbm-controller-manager -n capbm-system --timeout=120s

echo "✓ CAPBM CRDs and Controller installed"
echo ""

# Step 4: Deploy ClusterClass templates
echo "Step 4: Deploying ClusterClass templates..."
kubectl apply -k modules/capbm/config/clusterclass/

echo "✓ ClusterClass templates deployed"
echo ""

# Step 5: Verify installation
echo "Step 5: Verifying installation..."
echo ""

echo "CAPI Controllers:"
kubectl get pods -n capi-system -l control-plane=controller-manager
kubectl get pods -n capi-kubeadm-bootstrap-system -l control-plane=controller-manager
kubectl get pods -n capi-kubeadm-control-plane-system -l control-plane=controller-manager
echo ""

echo "CAPBM Controller:"
kubectl get pods -n capbm-system -l control-plane=controller-manager
echo ""

echo "ClusterClass:"
kubectl get clusterclass baremetal-clusterclass-v0.1.0 -n default
echo ""

echo "=== CAPBM Installation Complete ==="
echo ""
echo "Next steps:"
echo "1. Create a BareMetalHostInventory for your bare metal machines"
echo "2. Create a Cluster resource using the ClusterClass template"
echo "3. Monitor the cluster creation progress"
echo ""
echo "See docs/single-node-guide.md for detailed instructions."
