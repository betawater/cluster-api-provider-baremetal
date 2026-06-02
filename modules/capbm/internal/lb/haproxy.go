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

package lb

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	capbmv1 "github.com/BetaWater/cluster-api-provider-baremetal/modules/capbm/api/v1beta1"
)

// HAProxyProvider implements the Provider interface for HAProxy.
type HAProxyProvider struct {
	config *capbmv1.HAProxyConfig
}

// NewHAProxyProvider creates a new HAProxy provider.
func NewHAProxyProvider(config *capbmv1.HAProxyConfig) (Provider, error) {
	if config == nil {
		return nil, fmt.Errorf("HAProxy configuration is required")
	}
	if config.BackendName == "" {
		config.BackendName = "k8s-apiserver"
	}
	if config.AdminPort == 0 {
		config.AdminPort = 9999
	}
	return &HAProxyProvider{config: config}, nil
}

// RegisterBackend adds a backend server to the HAProxy pool.
func (p *HAProxyProvider) RegisterBackend(ctx context.Context, backend Backend) error {
	if p.config.AdminHost != "" {
		return p.registerViaRuntimeAPI(ctx, backend)
	} else if p.config.SSHHost != "" {
		return p.registerViaSSH(ctx, backend)
	}
	return fmt.Errorf("HAProxy: neither adminHost nor sshHost configured")
}

// UnregisterBackend removes a backend server from the HAProxy pool.
func (p *HAProxyProvider) UnregisterBackend(ctx context.Context, backend Backend) error {
	if p.config.AdminHost != "" {
		return p.unregisterViaRuntimeAPI(ctx, backend)
	} else if p.config.SSHHost != "" {
		return p.unregisterViaSSH(ctx, backend)
	}
	return fmt.Errorf("HAProxy: neither adminHost nor sshHost configured")
}

// GetBackends returns the current list of backend servers.
func (p *HAProxyProvider) GetBackends(ctx context.Context) ([]Backend, error) {
	if p.config.AdminHost != "" {
		return p.getBackendsViaRuntimeAPI(ctx)
	} else if p.config.SSHHost != "" {
		return p.getBackendsViaSSH(ctx)
	}
	return nil, fmt.Errorf("HAProxy: neither adminHost nor sshHost configured")
}

// HealthCheck checks if a backend is healthy.
func (p *HAProxyProvider) HealthCheck(ctx context.Context, backend Backend) (bool, error) {
	addr := net.JoinHostPort(backend.IP, fmt.Sprintf("%d", backend.Port))
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return false, nil
	}
	conn.Close()
	return true, nil
}

// registerViaRuntimeAPI registers a backend using HAProxy Runtime API (socat).
func (p *HAProxyProvider) registerViaRuntimeAPI(ctx context.Context, backend Backend) error {
	cmd := fmt.Sprintf("add server %s/%s %s:%d check inter 5s fall 3 rise 2",
		p.config.BackendName, backend.Name, backend.IP, backend.Port)

	addr := net.JoinHostPort(p.config.AdminHost, fmt.Sprintf("%d", p.config.AdminPort))
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return fmt.Errorf("failed to connect to HAProxy Runtime API: %w", err)
	}
	defer conn.Close()

	_, err = conn.Write([]byte(cmd + "\n"))
	if err != nil {
		return fmt.Errorf("failed to send command to HAProxy: %w", err)
	}

	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		return fmt.Errorf("failed to read response from HAProxy: %w", err)
	}

	response := strings.TrimSpace(string(buf[:n]))
	if strings.Contains(strings.ToLower(response), "error") {
		return fmt.Errorf("HAProxy returned error: %s", response)
	}

	return nil
}

// unregisterViaRuntimeAPI unregisters a backend using HAProxy Runtime API.
func (p *HAProxyProvider) unregisterViaRuntimeAPI(ctx context.Context, backend Backend) error {
	cmd := fmt.Sprintf("del server %s/%s", p.config.BackendName, backend.Name)

	addr := net.JoinHostPort(p.config.AdminHost, fmt.Sprintf("%d", p.config.AdminPort))
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return fmt.Errorf("failed to connect to HAProxy Runtime API: %w", err)
	}
	defer conn.Close()

	_, err = conn.Write([]byte(cmd + "\n"))
	if err != nil {
		return fmt.Errorf("failed to send command to HAProxy: %w", err)
	}

	return nil
}

// getBackendsViaRuntimeAPI gets backends using HAProxy Runtime API.
func (p *HAProxyProvider) getBackendsViaRuntimeAPI(ctx context.Context) ([]Backend, error) {
	cmd := fmt.Sprintf("show servers state %s", p.config.BackendName)

	addr := net.JoinHostPort(p.config.AdminHost, fmt.Sprintf("%d", p.config.AdminPort))
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to HAProxy Runtime API: %w", err)
	}
	defer conn.Close()

	_, err = conn.Write([]byte(cmd + "\n"))
	if err != nil {
		return nil, fmt.Errorf("failed to send command to HAProxy: %w", err)
	}

	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, fmt.Errorf("failed to read response from HAProxy: %w", err)
	}

	return p.parseBackendsFromResponse(string(buf[:n]))
}

// registerViaSSH registers a backend using SSH to modify HAProxy config.
func (p *HAProxyProvider) registerViaSSH(ctx context.Context, backend Backend) error {
	script := fmt.Sprintf(`#!/bin/bash
set -euo pipefail

CONFIG_FILE="%s"
BACKEND_NAME="%s"
SERVER_NAME="%s"
SERVER_IP="%s"
SERVER_PORT="%d"
RELOAD_CMD="%s"

if grep -q "server ${SERVER_NAME} ${SERVER_IP}:${SERVER_PORT}" "$CONFIG_FILE"; then
    echo "server already exists"
    exit 0
fi

sed -i "/^backend ${BACKEND_NAME}/,/^$/ {
    /^$/ i\\    server ${SERVER_NAME} ${SERVER_IP}:${SERVER_PORT} check inter 5s fall 3 rise 2
}" "$CONFIG_FILE"

haproxy -c -f "$CONFIG_FILE"
$RELOAD_CMD
`, p.config.ConfigFile, p.config.BackendName, backend.Name, backend.IP, backend.Port, p.config.ReloadCommand)

	sshClient, err := p.createSSHClient()
	if err != nil {
		return fmt.Errorf("failed to create SSH client: %w", err)
	}
	defer sshClient.Close()

	session, err := sshClient.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create SSH session: %w", err)
	}
	defer session.Close()

	output, err := session.CombinedOutput(script)
	if err != nil {
		return fmt.Errorf("SSH script failed: %w, output: %s", err, string(output))
	}

	return nil
}

// unregisterViaSSH unregisters a backend using SSH.
func (p *HAProxyProvider) unregisterViaSSH(ctx context.Context, backend Backend) error {
	script := fmt.Sprintf(`#!/bin/bash
set -euo pipefail

CONFIG_FILE="%s"
SERVER_NAME="%s"
SERVER_IP="%s"
SERVER_PORT="%d"
RELOAD_CMD="%s"

sed -i "/server ${SERVER_NAME} ${SERVER_IP}:${SERVER_PORT}/d" "$CONFIG_FILE"
haproxy -c -f "$CONFIG_FILE"
$RELOAD_CMD
`, p.config.ConfigFile, backend.Name, backend.IP, backend.Port, p.config.ReloadCommand)

	sshClient, err := p.createSSHClient()
	if err != nil {
		return fmt.Errorf("failed to create SSH client: %w", err)
	}
	defer sshClient.Close()

	session, err := sshClient.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create SSH session: %w", err)
	}
	defer session.Close()

	output, err := session.CombinedOutput(script)
	if err != nil {
		return fmt.Errorf("SSH script failed: %w, output: %s", err, string(output))
	}

	return nil
}

// getBackendsViaSSH gets backends using SSH.
func (p *HAProxyProvider) getBackendsViaSSH(ctx context.Context) ([]Backend, error) {
	script := fmt.Sprintf("grep -A 100 '^backend %s' %s | grep 'server ' | head -50",
		p.config.BackendName, p.config.ConfigFile)

	sshClient, err := p.createSSHClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create SSH client: %w", err)
	}
	defer sshClient.Close()

	session, err := sshClient.NewSession()
	if err != nil {
		return nil, fmt.Errorf("failed to create SSH session: %w", err)
	}
	defer session.Close()

	output, err := session.CombinedOutput(script)
	if err != nil {
		return nil, fmt.Errorf("SSH command failed: %w, output: %s", err, string(output))
	}

	return p.parseBackendsFromResponse(string(output))
}

// createSSHClient creates a new SSH client connection.
func (p *HAProxyProvider) createSSHClient() (*ssh.Client, error) {
	config := &ssh.ClientConfig{
		User:            "root",
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	addr := net.JoinHostPort(p.config.SSHHost, fmt.Sprintf("%d", p.config.SSHPort))
	return ssh.Dial("tcp", addr, config)
}

// parseBackendsFromResponse parses backend servers from HAProxy response.
func (p *HAProxyProvider) parseBackendsFromResponse(response string) ([]Backend, error) {
	var backends []Backend

	lines := strings.Split(response, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "server ") {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}

		name := parts[1]
		addr := parts[2]

		addrParts := strings.Split(addr, ":")
		if len(addrParts) < 2 {
			continue
		}

		ip := addrParts[0]
		port := 6443
		if len(addrParts) > 1 {
			fmt.Sscanf(addrParts[1], "%d", &port)
		}

		backends = append(backends, Backend{
			Name: name,
			IP:   ip,
			Port: port,
		})
	}

	return backends, nil
}
