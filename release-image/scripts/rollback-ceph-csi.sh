#!/bin/bash
set -euo pipefail

echo "=== Rolling back Ceph CSI ==="

# Restore backup
if [ -d /etc/csi.bak ]; then
    rm -rf /etc/csi
    cp -r /etc/csi.bak /etc/csi
fi

# Verify
echo "Ceph CSI pods:"
kubectl get pods -n ceph-csi -l app=ceph-csi-rbdplugin || true

echo "=== Ceph CSI rollback completed ==="
