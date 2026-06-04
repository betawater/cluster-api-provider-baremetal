#!/bin/bash
set -euo pipefail

echo "=== Upgrading containerd ==="

# Backup current configuration
cp /etc/containerd/config.toml /etc/containerd/config.toml.bak 2>/dev/null || true

# Stop containerd
systemctl stop containerd

# Install new version (assuming binaries are in /opt/capbm/binaries/containerd/)
if [ -d /opt/capbm/binaries/containerd ]; then
    tar xzf /opt/capbm/binaries/containerd/containerd-*.tar.gz -C /usr/local/bin 2>/dev/null || true
fi

# Restore configuration
if [ -f /etc/containerd/config.toml.bak ]; then
    cp /etc/containerd/config.toml.bak /etc/containerd/config.toml
fi

# Start containerd
systemctl start containerd

# Verify
echo "Containerd version:"
containerd --version || true
echo "Containerd status:"
systemctl is-active containerd

echo "=== containerd upgrade completed ==="
