#!/bin/bash
set -euo pipefail

echo "=== Upgrading Kubernetes ==="

# Detect OS
if [ -f /etc/os-release ]; then
    . /etc/os-release
    OS_ID=$ID
else
    echo "Cannot detect OS"
    exit 1
fi

# Backup current configuration
cp -r /etc/kubernetes /etc/kubernetes.bak 2>/dev/null || true

case "$OS_ID" in
    ubuntu|debian)
        # Install new version using apt
        apt-get update
        apt-get install -y kubeadm=1.31.1-00 kubelet=1.31.1-00 kubectl=1.31.1-00
        ;;
    centos|rhel|rocky|almalinux)
        # Install new version using yum
        yum install -y kubeadm-1.31.1-0 kubelet-1.31.1-0 kubectl-1.31.1-0
        ;;
    *)
        echo "Unsupported OS: $OS_ID"
        exit 1
        ;;
esac

# Restart kubelet
systemctl daemon-reload
systemctl restart kubelet

# Verify
echo "Kubernetes version:"
kubelet --version || true
echo "Kubelet status:"
systemctl is-active kubelet

echo "=== Kubernetes upgrade completed ==="
