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

package network

import (
	"context"
	"fmt"

	capbmv1 "github.com/BetaWater/cluster-api-provider-baremetal/modules/capbm/api/v1beta2"
	sshclient "github.com/BetaWater/cluster-api-provider-baremetal/modules/cvo/pkg/ssh"
)

type FirewallManager struct {
	sshConn *sshclient.SSHConnection
	config  *capbmv1.FirewallConfig
	role    string
}

func NewFirewallManager(sshConn *sshclient.SSHConnection, config *capbmv1.FirewallConfig, role string) *FirewallManager {
	return &FirewallManager{
		sshConn: sshConn,
		config:  config,
		role:    role,
	}
}

func (m *FirewallManager) Configure(ctx context.Context) error {
	if m.config == nil || !m.config.Configure {
		return nil
	}

	script := firewallScript
	script = fmt.Sprintf("ROLE=%q\n%s", m.role, script)

	result, err := m.sshConn.ExecuteScript(ctx, script)
	if err != nil {
		return fmt.Errorf("firewall configuration failed: %w, stderr: %s", err, result.Stderr)
	}

	if result.ExitCode != 0 {
		return fmt.Errorf("firewall configuration failed with exit code %d: %s", result.ExitCode, result.Stderr)
	}

	additionalPorts := ""
	if m.config != nil {
		for _, port := range m.config.AdditionalPorts {
			protocol := port.Protocol
			if protocol == "" {
				protocol = "tcp"
			}
			additionalPorts += fmt.Sprintf("open_port %d %s \"%s\"\n", port.Port, protocol, port.Description)
		}
	}

	if additionalPorts != "" {
		additionalScript := fmt.Sprintf(`#!/bin/bash
set -euo pipefail

detect_firewall() {
    if command -v firewall-cmd &>/dev/null && systemctl is-active --quiet firewalld; then
        echo "firewalld"
    elif command -v ufw &>/dev/null && ufw status 2>/dev/null | grep -q "Status: active"; then
        echo "ufw"
    else
        echo "iptables"
    fi
}

open_port() {
    local port="$1"
    local proto="${2:-tcp}"
    local fw_type
    fw_type=$(detect_firewall)

    case "$fw_type" in
        firewalld)
            firewall-cmd --permanent --add-port="${port}/${proto}"
            ;;
        ufw)
            ufw allow "${port}/${proto}"
            ;;
        iptables)
            iptables -A INPUT -p "$proto" --dport "$port" -j ACCEPT
            ;;
    esac
}

%s

fw_type=$(detect_firewall)
if [ "$fw_type" = "firewalld" ]; then
    firewall-cmd --reload
fi
`, additionalPorts)

		result, err := m.sshConn.ExecuteScript(ctx, additionalScript)
		if err != nil {
			return fmt.Errorf("additional ports configuration failed: %w, stderr: %s", err, result.Stderr)
		}
	}

	return nil
}

const firewallScript = `#!/bin/bash
set -euo pipefail

ROLE="${ROLE:-worker}"

detect_firewall() {
    if command -v firewall-cmd &>/dev/null && systemctl is-active --quiet firewalld; then
        echo "firewalld"
    elif command -v ufw &>/dev/null && ufw status 2>/dev/null | grep -q "Status: active"; then
        echo "ufw"
    else
        echo "iptables"
    fi
}

open_port() {
    local port="$1"
    local proto="${2:-tcp}"
    local desc="$3"
    local fw_type
    fw_type=$(detect_firewall)

    echo "Opening port: $port/$proto ($desc) using $fw_type"

    case "$fw_type" in
        firewalld)
            firewall-cmd --permanent --add-port="${port}/${proto}"
            ;;
        ufw)
            ufw allow "${port}/${proto}" comment "$desc"
            ;;
        iptables)
            iptables -A INPUT -p "$proto" --dport "$port" -j ACCEPT
            ;;
    esac
}

echo "=== Configuring firewall ==="
fw_type=$(detect_firewall)
echo "Detected firewall: $fw_type"

open_port 10250 tcp "kubelet API"

if [ "$ROLE" = "control-plane" ]; then
    open_port 6443 tcp "kube-apiserver"
    open_port 2379 tcp "etcd client"
    open_port 2380 tcp "etcd peer"
    open_port 10257 tcp "kube-controller-manager"
    open_port 10259 tcp "kube-scheduler"
fi

case "$fw_type" in
    firewalld)
        firewall-cmd --reload
        ;;
    ufw)
        ufw reload
        ;;
esac

echo "=== Firewall configuration completed ==="
`
