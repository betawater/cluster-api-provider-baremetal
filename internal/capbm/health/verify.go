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

package health

import (
	"context"
	"fmt"
	"strings"

	sshclient "github.com/BetaWater/cluster-api-provider-baremetal/internal/capbm/ssh"
)

type VerificationResult struct {
	Passed bool     `json:"passed"`
	Errors []string `json:"errors,omitempty"`
}

type Verifier struct {
	sshConn *sshclient.SSHConnection
}

func NewVerifier(sshConn *sshclient.SSHConnection) *Verifier {
	return &Verifier{sshConn: sshConn}
}

func (v *Verifier) VerifyInstallation(ctx context.Context) (*VerificationResult, error) {
	result := &VerificationResult{Passed: true}

	checks := []struct {
		name string
		fn   func(context.Context, *VerificationResult) error
	}{
		{"containerd", v.checkContainerd},
		{"kubeadm", v.checkKubeadm},
		{"kubelet", v.checkKubelet},
		{"services", v.checkServices},
		{"filesystems", v.checkFilesystems},
	}

	for _, check := range checks {
		if err := check.fn(ctx, result); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", check.name, err))
			result.Passed = false
		}
	}

	return result, nil
}

func (v *Verifier) checkContainerd(ctx context.Context, result *VerificationResult) error {
	cmdResult, err := v.sshConn.ExecuteCommand(ctx, "command -v containerd 2>/dev/null && containerd --version || echo 'NOT_FOUND'")
	if err != nil {
		return fmt.Errorf("failed to check containerd: %w", err)
	}

	if strings.Contains(cmdResult.Stdout, "NOT_FOUND") {
		return fmt.Errorf("containerd binary not found")
	}

	cmdResult, err = v.sshConn.ExecuteCommand(ctx, "systemctl is-active --quiet containerd && echo 'running' || echo 'not_running'")
	if err != nil {
		return fmt.Errorf("failed to check containerd service: %w", err)
	}

	if strings.TrimSpace(cmdResult.Stdout) != "running" {
		return fmt.Errorf("containerd service is not running")
	}

	return nil
}

func (v *Verifier) checkKubeadm(ctx context.Context, result *VerificationResult) error {
	cmdResult, err := v.sshConn.ExecuteCommand(ctx, "command -v kubeadm 2>/dev/null && kubeadm version -o short || echo 'NOT_FOUND'")
	if err != nil {
		return fmt.Errorf("failed to check kubeadm: %w", err)
	}

	if strings.Contains(cmdResult.Stdout, "NOT_FOUND") {
		return fmt.Errorf("kubeadm binary not found")
	}

	return nil
}

func (v *Verifier) checkKubelet(ctx context.Context, result *VerificationResult) error {
	cmdResult, err := v.sshConn.ExecuteCommand(ctx, "command -v kubelet 2>/dev/null && kubelet --version || echo 'NOT_FOUND'")
	if err != nil {
		return fmt.Errorf("failed to check kubelet: %w", err)
	}

	if strings.Contains(cmdResult.Stdout, "NOT_FOUND") {
		return fmt.Errorf("kubelet binary not found")
	}

	return nil
}

func (v *Verifier) checkServices(ctx context.Context, result *VerificationResult) error {
	cmdResult, err := v.sshConn.ExecuteCommand(ctx, "systemctl is-active --quiet kubelet && echo 'kubelet:running' || echo 'kubelet:not_running'")
	if err != nil {
		return fmt.Errorf("failed to check kubelet service: %w", err)
	}

	output := strings.TrimSpace(cmdResult.Stdout)
	if strings.Contains(output, "not_running") {
		return fmt.Errorf("kubelet service is not active (may be normal before kubeadm init)")
	}

	return nil
}

func (v *Verifier) checkFilesystems(ctx context.Context, result *VerificationResult) error {
	dirs := []string{"/etc/kubernetes", "/var/lib/kubelet", "/etc/containerd"}

	for _, dir := range dirs {
		cmdResult, err := v.sshConn.ExecuteCommand(ctx, fmt.Sprintf("[ -d %s ] && echo 'exists' || echo 'missing'", dir))
		if err != nil {
			return fmt.Errorf("failed to check directory %s: %w", dir, err)
		}

		if strings.TrimSpace(cmdResult.Stdout) == "missing" {
			return fmt.Errorf("directory %s does not exist", dir)
		}
	}

	return nil
}
