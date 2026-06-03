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

package node

import (
	"encoding/base64"
	"fmt"
	"strings"
)

const (
	// HostsDirPath is the path to containerd hosts.d directory.
	HostsDirPath = "/etc/containerd/hosts.d"
	// CertsDirPath is the path to containerd certs.d directory.
	CertsDirPath = "/etc/containerd/certs.d"
)

// RegistryAuthConfig holds registry authentication configuration.
type RegistryAuthConfig struct {
	// Registry is the registry URL (e.g., registry.example.com).
	Registry string
	// Username is the registry username.
	Username string
	// Password is the registry password.
	Password string
	// InsecureSkipVerify skips TLS verification.
	InsecureSkipVerify bool
	// CACert is the CA certificate content.
	CACert string
}

// GenerateHostsToml generates the hosts.toml content for containerd registry authentication.
func GenerateHostsToml(config RegistryAuthConfig) string {
	var sb strings.Builder

	// Server URL
	fmt.Fprintf(&sb, "server = \"https://%s\"\n", config.Registry)
	sb.WriteString("\n")

	// Host configuration
	fmt.Fprintf(&sb, "[host.\"https://%s\"]\n", config.Registry)
	sb.WriteString("  capabilities = [\"pull\", \"resolve\"]\n")

	// Add authentication if credentials provided
	if config.Username != "" && config.Password != "" {
		auth := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", config.Username, config.Password)))
		sb.WriteString("\n")
		fmt.Fprintf(&sb, "[host.\"https://%s\".header]\n", config.Registry)
		fmt.Fprintf(&sb, "  Authorization = \"Basic %s\"\n", auth)
	}

	// Add CA certificate if provided
	if config.CACert != "" {
		sb.WriteString("\n")
		fmt.Fprintf(&sb, "[host.\"https://%s\"]\n", config.Registry)
		fmt.Fprintf(&sb, "  ca = \"%s/%s/ca.crt\"\n", CertsDirPath, config.Registry)
	}

	// Add insecure skip verify if enabled
	if config.InsecureSkipVerify {
		sb.WriteString("\n")
		sb.WriteString(fmt.Sprintf("[host.\"https://%s\"]\n", config.Registry))
		sb.WriteString("  skip_verify = true\n")
	}

	return sb.String()
}

// GenerateContainerdConfigPatch generates the containerd config patch for registry mirrors.
func GenerateContainerdConfigPatch(registries []string) string {
	var sb strings.Builder

	sb.WriteString("\n[plugins.\"io.containerd.grpc.v1.cri\".registry]\n")
	sb.WriteString(fmt.Sprintf("  config_path = \"%s\"\n", HostsDirPath))
	sb.WriteString("\n")
	sb.WriteString("  [plugins.\"io.containerd.grpc.v1.cri\".registry.mirrors]\n")

	for _, registry := range registries {
		sb.WriteString(fmt.Sprintf("    [plugins.\"io.containerd.grpc.v1.cri\".registry.mirrors.\"%s\"]\n", registry))
		sb.WriteString(fmt.Sprintf("      endpoint = [\"https://%s\"]\n", registry))
	}

	return sb.String()
}

// GenerateConfigureScript generates the shell script to configure containerd registry authentication.
func GenerateConfigureScript(config RegistryAuthConfig) string {
	return fmt.Sprintf(`#!/bin/bash
set -euo pipefail

REGISTRY="%s"
USERNAME="%s"
PASSWORD="%s"
CA_CERT="%s"
INSECURE="%t"

# Configure containerd hosts.d
HOSTS_DIR="%s/${REGISTRY}"
mkdir -p "$HOSTS_DIR"

# Generate hosts.toml
cat > "${HOSTS_DIR}/hosts.toml" << 'EOF'
%s
EOF

# If CA certificate provided, save it
if [ -n "$CA_CERT" ]; then
  CERTS_DIR="%s/${REGISTRY}"
  mkdir -p "$CERTS_DIR"
  echo "$CA_CERT" > "${CERTS_DIR}/ca.crt"
fi

# Restart containerd
systemctl restart containerd

echo "Containerd registry authentication configured for ${REGISTRY}"
`,
		config.Registry,
		config.Username,
		config.Password,
		config.CACert,
		config.InsecureSkipVerify,
		HostsDirPath,
		GenerateHostsToml(config),
		CertsDirPath,
	)
}
