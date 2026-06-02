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
	"bytes"
	"context"
	"fmt"
	"time"

	"golang.org/x/crypto/ssh"
)

// CommandResult holds the result of a remote command execution.
type CommandResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// ExecuteCommand runs a command on the remote host via SSH.
func (c *SSHConnection) ExecuteCommand(ctx context.Context, command string) (*CommandResult, error) {
	if c.Client == nil {
		return nil, fmt.Errorf("SSH client is not initialized")
	}

	session, err := c.Client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("failed to create SSH session: %w", err)
	}
	defer session.Close()

	var stdout, stderr bytes.Buffer
	session.Stdout = &stdout
	session.Stderr = &stderr

	done := make(chan error, 1)
	go func() {
		done <- session.Run(command)
	}()

	select {
	case err := <-done:
		c.LastUsed = time.Now()
		exitCode := 0
		if err != nil {
			if exitErr, ok := err.(*ssh.ExitError); ok {
				exitCode = exitErr.ExitStatus()
			} else {
				return nil, fmt.Errorf("command execution failed: %w", err)
			}
		}
		return &CommandResult{
			Stdout:   stdout.String(),
			Stderr:   stderr.String(),
			ExitCode: exitCode,
		}, nil
	case <-ctx.Done():
		session.Signal(ssh.SIGKILL)
		return nil, ctx.Err()
	}
}

// ExecuteScript runs a bash script on the remote host via SSH.
func (c *SSHConnection) ExecuteScript(ctx context.Context, script string) (*CommandResult, error) {
	escapedScript := bytes.ReplaceAll([]byte(script), []byte("'"), []byte("'\\''"))
	command := fmt.Sprintf("bash -c '%s'", string(escapedScript))
	return c.ExecuteCommand(ctx, command)
}

// IsAlive checks if the SSH connection is still alive.
func (c *SSHConnection) IsAlive() bool {
	if c.Client == nil {
		return false
	}
	_, _, err := c.Client.SendRequest("keepalive@google.com", true, nil)
	return err == nil
}
