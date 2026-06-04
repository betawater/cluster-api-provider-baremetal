#!/bin/bash
set -euo pipefail

echo "=== Rolling back CAPI Core Controller ==="

# Restore backup
if [ -d /etc/capi-core-controller.bak ]; then
    rm -rf /etc/capi-core-controller
    cp -r /etc/capi-core-controller.bak /etc/capi-core-controller
fi

# Verify
echo "CAPI Core Controller deployment:"
kubectl get deployment capi-controller-manager -n capi-system || true

echo "=== CAPI Core Controller rollback completed ==="
