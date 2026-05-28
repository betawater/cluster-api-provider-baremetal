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

package utils

import (
	"context"

	"github.com/BetaWater/cluster-api-provider-baremetal/internal/ssh"
)

// MockSSHConnection implements a mock SSH connection for testing.
type MockSSHConnection struct {
	Host         string
	Port         int
	Commands     map[string]string
	CommandError error
}

// NewMockSSHConnection creates a new mock SSH connection.
func NewMockSSHConnection(host string, port int) *MockSSHConnection {
	return &MockSSHConnection{
		Host:     host,
		Port:     port,
		Commands: make(map[string]string),
	}
}

// SetCommandResponse sets a mock response for a command.
func (m *MockSSHConnection) SetCommandResponse(command, response string) {
	m.Commands[command] = response
}

// SetError sets an error to be returned on command execution.
func (m *MockSSHConnection) SetError(err error) {
	m.CommandError = err
}

// ExecuteCommand executes a mock command.
func (m *MockSSHConnection) ExecuteCommand(ctx context.Context, command string) (*ssh.CommandResult, error) {
	if m.CommandError != nil {
		return nil, m.CommandError
	}

	if response, ok := m.Commands[command]; ok {
		return &ssh.CommandResult{
			Stdout:   response,
			ExitCode: 0,
		}, nil
	}

	return &ssh.CommandResult{
		Stdout:   "",
		ExitCode: 0,
	}, nil
}

// IsAlive returns true for mock connection.
func (m *MockSSHConnection) IsAlive() bool {
	return true
}

// MockSSHManager implements a mock SSH manager for testing.
type MockSSHManager struct {
	Connections map[string]*MockSSHConnection
}

// NewMockSSHManager creates a new mock SSH manager.
func NewMockSSHManager() *MockSSHManager {
	return &MockSSHManager{
		Connections: make(map[string]*MockSSHConnection),
	}
}

// Connect returns a mock SSH connection.
func (m *MockSSHManager) Connect(host string, port int, creds ssh.Credentials) (*ssh.SSHConnection, error) {
	key := hostPortKey(host, port)
	if mockConn, exists := m.Connections[key]; exists {
		return &ssh.SSHConnection{
			Host: mockConn.Host,
			Port: mockConn.Port,
		}, nil
	}

	mockConn := NewMockSSHConnection(host, port)
	m.Connections[key] = mockConn
	return &ssh.SSHConnection{
		Host: host,
		Port: port,
	}, nil
}

// Close removes a connection from the mock manager.
func (m *MockSSHManager) Close(conn *ssh.SSHConnection) {
	if conn == nil {
		return
	}
	key := hostPortKey(conn.Host, conn.Port)
	delete(m.Connections, key)
}

// Cleanup removes all connections.
func (m *MockSSHManager) Cleanup() {
	m.Connections = make(map[string]*MockSSHConnection)
}

// ConnectionCount returns the number of mock connections.
func (m *MockSSHManager) ConnectionCount() int {
	return len(m.Connections)
}

func hostPortKey(host string, port int) string {
	return host + ":" + string(rune(port))
}
