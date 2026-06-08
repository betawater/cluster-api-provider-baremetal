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
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewSSHManager(t *testing.T) {
	m := NewSSHManager(5 * time.Minute)
	if m == nil {
		t.Fatal("expected non-nil SSHManager")
	}
	if m.idleTimeout != 5*time.Minute {
		t.Errorf("expected idle timeout 5m, got %v", m.idleTimeout)
	}
	if m.connections == nil {
		t.Error("expected non-nil connections map")
	}
}

func TestSSHManagerWithKnownHosts(t *testing.T) {
	tmpDir := t.TempDir()
	knownHostsFile := filepath.Join(tmpDir, "known_hosts")

	m := NewSSHManager(5 * time.Minute).WithKnownHosts(knownHostsFile)
	if m.knownHostsFile != knownHostsFile {
		t.Errorf("expected known hosts file %s, got %s", knownHostsFile, m.knownHostsFile)
	}
}

func TestSSHManagerConnectionCount(t *testing.T) {
	m := NewSSHManager(5 * time.Minute)
	if m.ConnectionCount() != 0 {
		t.Errorf("expected 0 connections, got %d", m.ConnectionCount())
	}
}

func TestSSHManagerCloseNil(t *testing.T) {
	m := NewSSHManager(5 * time.Minute)
	m.Close(nil) // Should not panic
}

func TestAddHostKey(t *testing.T) {
	tmpDir := t.TempDir()
	knownHostsFile := filepath.Join(tmpDir, "known_hosts")

	err := AddHostKey("192.168.1.100", 22, "ssh-ed25519", []byte("AAAAC3NzaC1lZDI1NTE5AAAAI..."), knownHostsFile)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	content, err := os.ReadFile(knownHostsFile)
	if err != nil {
		t.Fatalf("expected no error reading file, got %v", err)
	}

	expected := "192.168.1.100:22 ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI...\n"
	if string(content) != expected {
		t.Errorf("expected content %q, got %q", expected, string(content))
	}
}

func TestAddHostKeyAppend(t *testing.T) {
	tmpDir := t.TempDir()
	knownHostsFile := filepath.Join(tmpDir, "known_hosts")

	// Add first key
	err := AddHostKey("192.168.1.100", 22, "ssh-ed25519", []byte("key1"), knownHostsFile)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Add second key
	err = AddHostKey("192.168.1.101", 22, "ssh-rsa", []byte("key2"), knownHostsFile)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	content, err := os.ReadFile(knownHostsFile)
	if err != nil {
		t.Fatalf("expected no error reading file, got %v", err)
	}

	expected := "192.168.1.100:22 ssh-ed25519 key1\n192.168.1.101:22 ssh-rsa key2\n"
	if string(content) != expected {
		t.Errorf("expected content %q, got %q", expected, string(content))
	}
}
