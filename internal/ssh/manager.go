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
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// Credentials holds SSH authentication credentials.
type Credentials struct {
	Username string
	Password string
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
	connections map[string]*SSHConnection
	mu          sync.RWMutex
	idleTimeout time.Duration
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

// Connect establishes an SSH connection to the specified host.
func (m *SSHManager) Connect(host string, port int, creds Credentials) (*SSHConnection, error) {
	key := fmt.Sprintf("%s:%d", host, port)

	m.mu.RLock()
	if conn, exists := m.connections[key]; exists {
		if time.Since(conn.LastUsed) < m.idleTimeout {
			if _, _, err := conn.Client.Conn.SendRequest("keepalive@google.com", true, nil); err == nil {
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
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:%d", host, port), config)
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

	conn.Client.Close()
	delete(m.connections, key)
}

// Cleanup removes expired connections from the pool.
func (m *SSHManager) Cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for key, conn := range m.connections {
		if time.Since(conn.LastUsed) > m.idleTimeout {
			conn.Client.Close()
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
