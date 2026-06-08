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

package ssh

import (
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// Credentials holds SSH authentication credentials.
type Credentials struct {
	Username     string
	Password     string
	KnownHostsFile string // Path to known_hosts file for host key verification
}

// SSHConnection represents an active SSH connection.
type SSHConnection struct {
	Client   *ssh.Client
	Host     string
	Port     int
	LastUsed time.Time
}

// SSHManager manages a pool of SSH connections.
type SSHManager struct {
	connections    map[string]*SSHConnection
	mu             sync.RWMutex
	idleTimeout    time.Duration
	knownHostsFile string
}

// NewSSHManager creates a new SSH connection manager.
func NewSSHManager(idleTimeout time.Duration) *SSHManager {
	if idleTimeout == 0 {
		idleTimeout = 5 * time.Minute
	}
	return &SSHManager{
		connections: make(map[string]*SSHConnection),
		idleTimeout: idleTimeout,
	}
}

// WithKnownHosts sets the known_hosts file for host key verification.
func (m *SSHManager) WithKnownHosts(path string) *SSHManager {
	m.knownHostsFile = path
	return m
}

// Connect establishes an SSH connection to the specified host.
func (m *SSHManager) Connect(host string, port int, creds Credentials) (*SSHConnection, error) {
	key := fmt.Sprintf("%s:%d", host, port)

	m.mu.RLock()
	if conn, exists := m.connections[key]; exists {
		if time.Since(conn.LastUsed) < m.idleTimeout {
			if _, _, err := conn.Client.SendRequest("keepalive@google.com", true, nil); err == nil {
				conn.LastUsed = time.Now()
				m.mu.RUnlock()
				return conn, nil
			}
		}
	}
	m.mu.RUnlock()

	config := &ssh.ClientConfig{
		User: creds.Username,
		Auth: []ssh.AuthMethod{
			ssh.Password(creds.Password),
		},
		Timeout: 10 * time.Second,
	}

	// Configure host key verification
	knownHostsPath := creds.KnownHostsFile
	if knownHostsPath == "" {
		knownHostsPath = m.knownHostsFile
	}

	if knownHostsPath != "" {
		callback, err := knownhosts.New(knownHostsPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load known hosts file %s: %w", knownHostsPath, err)
		}
		config.HostKeyCallback = callback
	} else {
		// Use insecure callback for backward compatibility (not recommended for production)
		config.HostKeyCallback = ssh.InsecureIgnoreHostKey()
	}

	client, err := ssh.Dial("tcp", net.JoinHostPort(host, fmt.Sprintf("%d", port)), config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s:%d: %w", host, port, err)
	}

	conn := &SSHConnection{
		Client:   client,
		Host:     host,
		Port:     port,
		LastUsed: time.Now(),
	}

	m.mu.Lock()
	m.connections[key] = conn
	m.mu.Unlock()

	return conn, nil
}

// Close closes an SSH connection and removes it from the pool.
func (m *SSHManager) Close(conn *SSHConnection) {
	if conn == nil || conn.Client == nil {
		return
	}

	key := fmt.Sprintf("%s:%d", conn.Host, conn.Port)

	m.mu.Lock()
	defer m.mu.Unlock()

	// Close error is non-critical; connection will be removed from pool regardless
	_ = conn.Client.Close()
	delete(m.connections, key)
}

// Cleanup removes expired connections from the pool.
func (m *SSHManager) Cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for key, conn := range m.connections {
		if time.Since(conn.LastUsed) > m.idleTimeout {
			// Close error is non-critical; connection will be removed from pool regardless
			_ = conn.Client.Close()
			delete(m.connections, key)
		}
	}
}

// ConnectionCount returns the number of active connections.
func (m *SSHManager) ConnectionCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.connections)
}

// AddHostKey adds a host key to the known_hosts file.
func AddHostKey(host string, port int, keyType string, keyData []byte, knownHostsFile string) error {
	f, err := os.OpenFile(knownHostsFile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		return fmt.Errorf("failed to open known_hosts file: %w", err)
	}
	defer func() {
		// Close error is non-critical for known_hosts write
		_ = f.Close()
	}()

	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	_, err = fmt.Fprintf(f, "%s %s %s\n", addr, keyType, string(keyData))
	return err
}
