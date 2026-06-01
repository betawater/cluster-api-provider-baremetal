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

	capbmv1 "github.com/BetaWater/cluster-api-provider-baremetal/api/capbm/v1beta1"
	sshclient "github.com/BetaWater/cluster-api-provider-baremetal/internal/capbm/ssh"
)

type SELinuxManager struct {
	sshConn *sshclient.SSHConnection
	config  *capbmv1.SELinuxConfig
}

func NewSELinuxManager(sshConn *sshclient.SSHConnection, config *capbmv1.SELinuxConfig) *SELinuxManager {
	return &SELinuxManager{
		sshConn: sshConn,
		config:  config,
	}
}

func (m *SELinuxManager) Configure(ctx context.Context) error {
	if m.config == nil || !m.config.Configure {
		return nil
	}

	result, err := m.sshConn.ExecuteScript(ctx, selinuxScript)
	if err != nil {
		return fmt.Errorf("SELinux configuration failed: %w, stderr: %s", err, result.Stderr)
	}

	if result.ExitCode != 0 {
		return fmt.Errorf("SELinux configuration failed with exit code %d: %s", result.ExitCode, result.Stderr)
	}

	return nil
}

const selinuxScript = `#!/bin/bash
set -euo pipefail

echo "=== Configuring SELinux ==="

if ! command -v getenforce &>/dev/null; then
    echo "SELinux not installed, skipping"
    exit 0
fi

SELINUX_STATUS=$(getenforce)
echo "Current SELinux status: $SELINUX_STATUS"

if [ "$SELINUX_STATUS" = "Disabled" ]; then
    echo "SELinux is disabled, skipping"
    exit 0
fi

install_containerd_selinux() {
    echo "--- Configuring containerd SELinux ---"

    if command -v semodule &>/dev/null; then
        if semodule -l 2>/dev/null | grep -q containerd; then
            echo "containerd SELinux policy already installed"
        else
            echo "Installing containerd SELinux policy"
            if command -v dnf &>/dev/null; then
                dnf install -y container-selinux 2>/dev/null || true
            elif command -v yum &>/dev/null; then
                yum install -y container-selinux 2>/dev/null || true
            fi
        fi
    fi
}

configure_kubelet_selinux() {
    echo "--- Configuring kubelet SELinux context ---"

    if command -v semanage &>/dev/null; then
        semanage fcontext -a -t var_lib_t "/var/lib/kubelet(/.*)?" 2>/dev/null || true
        restorecon -Rv /var/lib/kubelet 2>/dev/null || true
    fi
}

configure_cni_selinux() {
    echo "--- Configuring CNI SELinux context ---"

    if [ -d /etc/cni ]; then
        restorecon -Rv /etc/cni 2>/dev/null || true
    fi

    if [ -d /opt/cni ]; then
        restorecon -Rv /opt/cni 2>/dev/null || true
    fi
}

install_containerd_selinux
configure_kubelet_selinux
configure_cni_selinux

echo "=== SELinux configuration completed ==="
`
