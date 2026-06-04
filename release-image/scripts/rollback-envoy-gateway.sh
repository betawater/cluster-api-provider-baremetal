#!/bin/bash
set -euo pipefail

echo "=== Rolling back Envoy Gateway ==="

# Restore backup
if [ -d /etc/envoy-gateway.bak ]; then
    rm -rf /etc/envoy-gateway
    cp -r /etc/envoy-gateway.bak /etc/envoy-gateway
fi

# Verify
echo "Envoy Gateway deployment:"
kubectl get deployment envoy-gateway -n envoy-gateway-system || true

echo "=== Envoy Gateway rollback completed ==="
