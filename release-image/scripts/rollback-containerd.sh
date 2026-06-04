#!/bin/bash
set -euo pipefail

echo "=== Rolling back containerd ==="

# Restore backup
if [ -f /etc/containerd/config.toml.bak ]; then
    cp /etc/containerd/config.toml.bak /etc/containerd/config.toml
fi

# Restart containerd
systemctl restart containerd

# Verify
echo "Containerd status:"
systemctl is-active containerd

echo "=== containerd rollback completed ==="
