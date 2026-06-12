/*
Copyright 2024 The CAPBM Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package bootstrap

import (
	"context"
	"fmt"
	"strings"

	capbmv1 "github.com/BetaWater/cluster-api-provider-baremetal/modules/capbm/api/v1beta2"
	"github.com/BetaWater/cluster-api-provider-baremetal/modules/cvo/pkg/ssh"
)

// NodeBootstrapper handles node bootstrapping configuration.
type NodeBootstrapper struct {
	sshConn *ssh.SSHConnection
	config  *capbmv1.NodeBootstrapConfig
	role    string
	hostname string
}

// BootstrapResult holds the results of node bootstrapping.
type BootstrapResult struct {
	Steps []string `json:"steps,omitempty"`
}

// NewNodeBootstrapper creates a new NodeBootstrapper.
func NewNodeBootstrapper(sshConn *ssh.SSHConnection, config *capbmv1.NodeBootstrapConfig, role string, hostname string) *NodeBootstrapper {
	return &NodeBootstrapper{
		sshConn:  sshConn,
		config:   config,
		role:     role,
		hostname: hostname,
	}
}

// Bootstrap executes all node bootstrapping steps.
func (b *NodeBootstrapper) Bootstrap(ctx context.Context) (*BootstrapResult, error) {
	result := &BootstrapResult{}

	if b.config.Hostname != "" || b.hostname != "" {
		if err := b.configureHostname(ctx); err != nil {
			return result, fmt.Errorf("failed to configure hostname: %w", err)
		}
		result.Steps = append(result.Steps, "hostname")
	}

	if len(b.config.HostsEntries) > 0 {
		if err := b.configureHosts(ctx); err != nil {
			return result, fmt.Errorf("failed to configure hosts: %w", err)
		}
		result.Steps = append(result.Steps, "hosts")
	}

	if b.config.DisableSwap {
		if err := b.disableSwap(ctx); err != nil {
			return result, fmt.Errorf("failed to disable swap: %w", err)
		}
		result.Steps = append(result.Steps, "swap")
	}

	if len(b.config.KernelModules) > 0 {
		if err := b.loadKernelModules(ctx); err != nil {
			return result, fmt.Errorf("failed to load kernel modules: %w", err)
		}
		result.Steps = append(result.Steps, "kernel_modules")
	}

	if len(b.config.SysctlParams) > 0 {
		if err := b.configureSysctl(ctx); err != nil {
			return result, fmt.Errorf("failed to configure sysctl: %w", err)
		}
		result.Steps = append(result.Steps, "sysctl")
	}

	if b.config.TimeSync {
		if err := b.configureTimeSync(ctx); err != nil {
			return result, fmt.Errorf("failed to configure time sync: %w", err)
		}
		result.Steps = append(result.Steps, "time_sync")
	}

	return result, nil
}

func (b *NodeBootstrapper) configureHostname(ctx context.Context) error {
	hostname := b.config.Hostname
	if hostname == "" {
		hostname = b.hostname
	}
	if hostname == "" {
		return nil
	}

	script := fmt.Sprintf(`#!/bin/bash
set -euo pipefail

echo "=== Configuring hostname ==="

CURRENT_HOSTNAME=$(hostname)
DESIRED_HOSTNAME="%s"

if [ "$CURRENT_HOSTNAME" = "$DESIRED_HOSTNAME" ]; then
    echo "Hostname already set to $DESIRED_HOSTNAME"
    exit 0
fi

hostnamectl set-hostname "$DESIRED_HOSTNAME"

# Update /etc/hostname
echo "$DESIRED_HOSTNAME" > /etc/hostname

echo "Hostname set to $DESIRED_HOSTNAME"
echo "=== Hostname configured ==="
`, hostname)

	result, err := b.sshConn.ExecuteScript(ctx, script)
	if err != nil {
		return fmt.Errorf("configure hostname failed: %w, stderr: %s", err, result.Stderr)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("configure hostname failed with exit code %d: %s", result.ExitCode, result.Stderr)
	}
	return nil
}

func (b *NodeBootstrapper) configureHosts(ctx context.Context) error {
	var hostsLines []string
	for _, entry := range b.config.HostsEntries {
		hostnames := strings.Join(entry.Hostnames, " ")
		hostsLines = append(hostsLines, fmt.Sprintf("%s %s", entry.IP, hostnames))
	}
	hostsContent := strings.Join(hostsLines, "\n")

	script := fmt.Sprintf(`#!/bin/bash
set -euo pipefail

echo "=== Configuring /etc/hosts ==="

# Backup existing hosts file
cp /etc/hosts /etc/hosts.bak.$(date +%%Y%%m%%d%%H%%M%%S) 2>/dev/null || true

# Add entries if not already present
HOSTS_ENTRIES='%s'

while IFS= read -r line; do
    ip=$(echo "$line" | awk '{print $1}')
    if ! grep -q "^${ip} " /etc/hosts; then
        echo "$line" >> /etc/hosts
        echo "Added: $line"
    else
        echo "Already exists: $line"
    fi
done <<< "$HOSTS_ENTRIES"

echo "=== /etc/hosts configured ==="
`, hostsContent)

	result, err := b.sshConn.ExecuteScript(ctx, script)
	if err != nil {
		return fmt.Errorf("configure hosts failed: %w, stderr: %s", err, result.Stderr)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("configure hosts failed with exit code %d: %s", result.ExitCode, result.Stderr)
	}
	return nil
}

func (b *NodeBootstrapper) disableSwap(ctx context.Context) error {
	script := `#!/bin/bash
set -euo pipefail

echo "=== Disabling swap ==="

# Check if swap is enabled
if ! swapon --show | grep -q .; then
    echo "Swap is already disabled"
    exit 0
fi

# Temporarily disable swap
swapoff -a

# Permanently disable swap by commenting out swap entries in /etc/fstab
if grep -q "[[:space:]]swap[[:space:]]" /etc/fstab; then
    sed -i '/[[:space:]]swap[[:space:]]/s/^/#/' /etc/fstab
    echo "Swap disabled permanently in /etc/fstab"
else
    echo "No swap entry found in /etc/fstab"
fi

# Verify
if swapon --show | grep -q .; then
    echo "ERROR: Swap is still enabled"
    exit 1
fi

echo "=== Swap disabled successfully ==="
`

	result, err := b.sshConn.ExecuteScript(ctx, script)
	if err != nil {
		return fmt.Errorf("disable swap failed: %w, stderr: %s", err, result.Stderr)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("disable swap failed with exit code %d: %s", result.ExitCode, result.Stderr)
	}
	return nil
}

func (b *NodeBootstrapper) loadKernelModules(ctx context.Context) error {
	modules := strings.Join(b.config.KernelModules, " ")
	script := fmt.Sprintf(`#!/bin/bash
set -euo pipefail

echo "=== Loading kernel modules ==="

MODULES="%s"

mkdir -p /etc/modules-load.d

for module in $MODULES; do
    if lsmod | grep -q "^${module} "; then
        echo "Module $module already loaded"
    else
        echo "Loading module $module"
        if ! modprobe "$module" 2>/dev/null; then
            echo "WARNING: Failed to load module $module (may not be available)"
        fi
    fi
    
    # Ensure module loads on boot
    if [ ! -f "/etc/modules-load.d/k8s.conf" ] || ! grep -q "^${module}$" /etc/modules-load.d/k8s.conf; then
        echo "$module" >> /etc/modules-load.d/k8s.conf
    fi
done

echo "=== Kernel modules configured ==="
`, modules)

	result, err := b.sshConn.ExecuteScript(ctx, script)
	if err != nil {
		return fmt.Errorf("load kernel modules failed: %w, stderr: %s", err, result.Stderr)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("load kernel modules failed with exit code %d: %s", result.ExitCode, result.Stderr)
	}
	return nil
}

func (b *NodeBootstrapper) configureSysctl(ctx context.Context) error {
	sysctlContent := "# Kubernetes sysctl settings\n"
	for key, value := range b.config.SysctlParams {
		sysctlContent += fmt.Sprintf("%s = %s\n", key, value)
	}

	script := fmt.Sprintf(`#!/bin/bash
set -euo pipefail

echo "=== Configuring sysctl ==="

cat > /etc/sysctl.d/k8s.conf << 'EOF'
%s
EOF

sysctl --system

echo "=== Sysctl configured ==="
`, sysctlContent)

	result, err := b.sshConn.ExecuteScript(ctx, script)
	if err != nil {
		return fmt.Errorf("configure sysctl failed: %w, stderr: %s", err, result.Stderr)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("configure sysctl failed with exit code %d: %s", result.ExitCode, result.Stderr)
	}
	return nil
}

func (b *NodeBootstrapper) configureTimeSync(ctx context.Context) error {
	script := `#!/bin/bash
set -euo pipefail

echo "=== Configuring time synchronization ==="

# Detect available time sync service
if command -v chronyd &>/dev/null || command -v chronyc &>/dev/null; then
    echo "Chrony is installed, ensuring it's enabled"
    systemctl enable --now chronyd 2>/dev/null || systemctl enable --now chrony 2>/dev/null || true
elif command -v ntpd &>/dev/null; then
    echo "NTP is installed, ensuring it's enabled"
    systemctl enable --now ntpd 2>/dev/null || systemctl enable --now ntp 2>/dev/null || true
elif command -v timedatectl &>/dev/null; then
    echo "Using systemd-timesyncd"
    timedatectl set-ntp true 2>/dev/null || true
    systemctl enable --now systemd-timesyncd 2>/dev/null || true
else
    echo "WARNING: No time synchronization service found, installing chrony"
    if command -v apt-get &>/dev/null; then
        apt-get update && apt-get install -y chrony 2>/dev/null || true
        systemctl enable --now chrony 2>/dev/null || true
    elif command -v dnf &>/dev/null; then
        dnf install -y chrony 2>/dev/null || true
        systemctl enable --now chronyd 2>/dev/null || true
    elif command -v yum &>/dev/null; then
        yum install -y chrony 2>/dev/null || true
        systemctl enable --now chronyd 2>/dev/null || true
    fi
fi

echo "=== Time synchronization configured ==="
`

	result, err := b.sshConn.ExecuteScript(ctx, script)
	if err != nil {
		return fmt.Errorf("configure time sync failed: %w, stderr: %s", err, result.Stderr)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("configure time sync failed with exit code %d: %s", result.ExitCode, result.Stderr)
	}
	return nil
}
