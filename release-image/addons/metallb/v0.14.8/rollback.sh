#!/bin/bash
set -euo pipefail

echo "=== Rolling back MetalLB ==="

# Restore backup
if [ -d /etc/metallb.bak ]; then
    rm -rf /etc/metallb
    cp -r /etc/metallb.bak /etc/metallb
fi

# Verify
echo "MetalLB pods:"
kubectl get pods -n metallb-system -l app=metallb || true

echo "=== MetalLB rollback completed ==="
