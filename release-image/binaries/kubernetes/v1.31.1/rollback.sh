#!/bin/bash
set -euo pipefail

echo "=== Rolling back Kubernetes ==="

# Restore backup
if [ -d /etc/kubernetes.bak ]; then
    rm -rf /etc/kubernetes
    cp -r /etc/kubernetes.bak /etc/kubernetes
fi

# Restart kubelet
systemctl daemon-reload
systemctl restart kubelet

# Verify
echo "Kubelet status:"
systemctl is-active kubelet

echo "=== Kubernetes rollback completed ==="
