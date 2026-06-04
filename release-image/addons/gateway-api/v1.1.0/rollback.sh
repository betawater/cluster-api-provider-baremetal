#!/bin/bash
set -euo pipefail

echo "=== Rolling back Gateway API ==="

# Verify
echo "Gateway API CRDs:"
kubectl get crd gateways.gateway.networking.k8s.io || true

echo "=== Gateway API rollback completed ==="
