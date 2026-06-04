#!/bin/bash
set -euo pipefail

echo "=== Rolling back Calico ==="

# Restore backup
if [ -d /etc/cni/net.d.bak ]; then
    rm -rf /etc/cni/net.d
    cp -r /etc/cni/net.d.bak /etc/cni/net.d
fi

# Verify
echo "Calico pods:"
kubectl get pods -n kube-system -l k8s-app=calico-node || true

echo "=== Calico rollback completed ==="
