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

package installer

import (
	"context"
	"fmt"
	"strings"
	"time"

	infrav1 "github.com/BetaWater/cluster-api-provider-baremetal/api/v1beta1"
	sshclient "github.com/BetaWater/cluster-api-provider-baremetal/internal/ssh"
)

type Installer struct {
	sshConn  *sshclient.SSHConnection
	config   *infrav1.ComponentInstallConfig
	k8sVersion string
	role     string
}

type InstallResult struct {
	Completed       bool              `json:"completed"`
	Success         bool              `json:"success"`
	Progress        string            `json:"progress"`
	ComponentVersions ComponentVersions `json:"componentVersions,omitempty"`
	Error           string            `json:"error,omitempty"`
	RetryCount      int               `json:"retryCount,omitempty"`
}

type ComponentVersions struct {
	ContainerRuntime string `json:"containerRuntime,omitempty"`
	Kubeadm          string `json:"kubeadm,omitempty"`
	Kubelet          string `json:"kubelet,omitempty"`
	Kubectl          string `json:"kubectl,omitempty"`
	OSType           string `json:"osType,omitempty"`
	OSVersion        string `json:"osVersion,omitempty"`
}

func New(sshConn *sshclient.SSHConnection, config *infrav1.ComponentInstallConfig, k8sVersion string, role string) *Installer {
	return &Installer{
		sshConn:    sshConn,
		config:     config,
		k8sVersion: k8sVersion,
		role:       role,
	}
}

func (i *Installer) Install(ctx context.Context) (*InstallResult, error) {
	if i.config == nil || !i.config.Enabled {
		return &InstallResult{Completed: true, Success: true, Progress: "Installation disabled"}, nil
	}

	if i.config.Strategy == infrav1.Skip {
		return &InstallResult{Completed: true, Success: true, Progress: "Installation skipped"}, nil
	}

	timeout := 5 * time.Minute
	if i.config.Timeout != nil {
		timeout = i.config.Timeout.Duration
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	detector := NewOSDetector(i.sshConn)
	osInfo, err := detector.Detect(ctx)
	if err != nil {
		return &InstallResult{Completed: false, Success: false, Progress: "OS detection failed", Error: err.Error()}, err
	}

	if i.config.Strategy == infrav1.InstallIfMissing {
		existing, err := i.checkExistingComponents(ctx, osInfo)
		if err != nil {
			return &InstallResult{Completed: false, Success: false, Progress: "Component check failed", Error: err.Error()}, err
		}
		if existing.allMatch(i.k8sVersion, i.config.ContainerRuntime.Version) {
			return &InstallResult{
				Completed: true,
				Success:   true,
				Progress:  "All components already installed with matching versions",
				ComponentVersions: ComponentVersions{
					ContainerRuntime: existing.ContainerRuntime,
					Kubeadm:          existing.Kubeadm,
					Kubelet:          existing.Kubelet,
					Kubectl:          existing.Kubectl,
					OSType:           osInfo.Type,
					OSVersion:        osInfo.Version,
				},
			}, nil
		}
	}

	maxRetries := i.config.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			updateProgress(i.sshConn, "retrying", fmt.Sprintf("Attempt %d/%d", attempt+1, maxRetries+1))
			time.Sleep(10 * time.Second)
		}

		result, err := i.executeInstall(ctx, osInfo, attempt)
		if err == nil && result.Success {
			return result, nil
		}
		lastErr = err
	}

	if i.config.RollbackOnError {
		_ = i.rollback(ctx)
	}

	return &InstallResult{
		Completed:  true,
		Success:    false,
		Progress:   "Installation failed after retries",
		Error:      lastErr.Error(),
		RetryCount: maxRetries,
	}, lastErr
}

func (i *Installer) executeInstall(ctx context.Context, osInfo *OSInfo, attempt int) (*InstallResult, error) {
	updateProgress(i.sshConn, "starting", "Beginning component installation")

	if err := i.installContainerRuntime(ctx, osInfo); err != nil {
		return &InstallResult{Completed: false, Success: false, Progress: "Container runtime installation failed", Error: err.Error()}, err
	}

	updateProgress(i.sshConn, "container_runtime_done", "Container runtime installed successfully")

	if err := i.installKubernetesComponents(ctx, osInfo); err != nil {
		return &InstallResult{Completed: false, Success: false, Progress: "Kubernetes components installation failed", Error: err.Error()}, err
	}

	updateProgress(i.sshConn, "kubernetes_done", "Kubernetes components installed successfully")

	versions, err := i.collectVersions(ctx)
	if err != nil {
		return &InstallResult{Completed: false, Success: false, Progress: "Version collection failed", Error: err.Error()}, err
	}
	versions.OSType = osInfo.Type
	versions.OSVersion = osInfo.Version

	return &InstallResult{
		Completed:       true,
		Success:         true,
		Progress:        "Installation completed successfully",
		ComponentVersions: *versions,
	}, nil
}

type existingComponents struct {
	ContainerRuntime string
	Kubeadm          string
	Kubelet          string
	Kubectl          string
}

func (i *Installer) checkExistingComponents(ctx context.Context, osInfo *OSInfo) (*existingComponents, error) {
	ec := &existingComponents{}

	containerRuntime := i.config.ContainerRuntime.Type
	if containerRuntime == "" {
		containerRuntime = "containerd"
	}

	switch containerRuntime {
	case "containerd":
		result, _ := i.sshConn.ExecuteCommand(ctx, "containerd --version 2>/dev/null || echo ''")
		if result.Stdout != "" {
			ec.ContainerRuntime = extractVersion(result.Stdout)
		}
	case "cri-o":
		result, _ := i.sshConn.ExecuteCommand(ctx, "crio --version 2>/dev/null | head -1 || echo ''")
		if result.Stdout != "" {
			ec.ContainerRuntime = extractVersion(result.Stdout)
		}
	case "docker":
		result, _ := i.sshConn.ExecuteCommand(ctx, "docker --version 2>/dev/null || echo ''")
		if result.Stdout != "" {
			ec.ContainerRuntime = extractVersion(result.Stdout)
		}
	}

	result, _ := i.sshConn.ExecuteCommand(ctx, "kubeadm version -o short 2>/dev/null || echo ''")
	if result.Stdout != "" {
		ec.Kubeadm = result.Stdout
	}

	result, _ = i.sshConn.ExecuteCommand(ctx, "kubelet --version 2>/dev/null || echo ''")
	if result.Stdout != "" {
		ec.Kubelet = extractVersion(result.Stdout)
	}

	result, _ = i.sshConn.ExecuteCommand(ctx, "kubectl version --client --short 2>/dev/null || kubectl version --client 2>/dev/null | grep 'Client Version' || echo ''")
	if result.Stdout != "" {
		ec.Kubectl = extractVersion(result.Stdout)
	}

	return ec, nil
}

func (ec *existingComponents) allMatch(k8sVersion string, crVersion string) bool {
	expectedK8s := "v" + k8sVersion
	if ec.Kubeadm != "" && ec.Kubeadm != expectedK8s {
		return false
	}
	if ec.Kubelet != "" && ec.Kubelet != expectedK8s {
		return false
	}
	if crVersion != "" && ec.ContainerRuntime != "" && ec.ContainerRuntime != crVersion {
		return false
	}
	return ec.Kubeadm != "" || ec.Kubelet != "" || ec.ContainerRuntime != ""
}

func (i *Installer) installContainerRuntime(ctx context.Context, osInfo *OSInfo) error {
	runtimeType := i.config.ContainerRuntime.Type
	if runtimeType == "" {
		runtimeType = "containerd"
	}

	script, err := getContainerRuntimeScript(runtimeType, osInfo.Type, i.config)
	if err != nil {
		return fmt.Errorf("failed to get container runtime script: %w", err)
	}

	script = prependEnvVars(script, map[string]string{
		"CONTAINERD_VERSION": i.config.ContainerRuntime.Version,
	})

	if len(i.config.ContainerRuntime.RegistryMirrors) > 0 {
		mirrors := ""
		for _, m := range i.config.ContainerRuntime.RegistryMirrors {
			if mirrors != "" {
				mirrors += ","
			}
			mirrors += m
		}
		script = prependEnvVar(script, "REGISTRY_MIRRORS", mirrors)
	}

	if i.config.AirGap != nil && i.config.AirGap.Enabled {
		script = prependEnvVar(script, "AIR_GAP_MODE", "true")
		if i.config.AirGap.HTTPServerConfig != nil {
			script = prependEnvVar(script, "BASE_URL", i.config.AirGap.HTTPServerConfig.BaseURL)
		}
		if i.config.AirGap.LocalPath != "" {
			script = prependEnvVar(script, "LOCAL_PATH", i.config.AirGap.LocalPath)
		}
	}

	result, err := i.sshConn.ExecuteScript(ctx, script)
	if err != nil {
		return fmt.Errorf("container runtime installation failed: %w, stderr: %s", err, result.Stderr)
	}

	if result.ExitCode != 0 {
		return fmt.Errorf("container runtime installation failed with exit code %d: %s", result.ExitCode, result.Stderr)
	}

	return nil
}

func (i *Installer) installKubernetesComponents(ctx context.Context, osInfo *OSInfo) error {
	script, err := getKubernetesScript(osInfo.Type, i.config)
	if err != nil {
		return fmt.Errorf("failed to get kubernetes installation script: %w", err)
	}

	script = prependEnvVars(script, map[string]string{
		"K8S_VERSION": i.k8sVersion,
		"ROLE":        i.role,
	})

	if i.config.Kubernetes.Repository != nil {
		if i.config.Kubernetes.Repository.BaseURL != "" {
			script = prependEnvVar(script, "REPO_BASE_URL", i.config.Kubernetes.Repository.BaseURL)
		}
		if i.config.Kubernetes.Repository.GPGKey != "" {
			script = prependEnvVar(script, "REPO_GPG_KEY", i.config.Kubernetes.Repository.GPGKey)
		}
	}

	if i.config.AirGap != nil && i.config.AirGap.Enabled {
		script = prependEnvVar(script, "AIR_GAP_MODE", "true")
		if i.config.AirGap.HTTPServerConfig != nil {
			script = prependEnvVar(script, "BASE_URL", i.config.AirGap.HTTPServerConfig.BaseURL)
		}
	}

	result, err := i.sshConn.ExecuteScript(ctx, script)
	if err != nil {
		return fmt.Errorf("kubernetes components installation failed: %w, stderr: %s", err, result.Stderr)
	}

	if result.ExitCode != 0 {
		return fmt.Errorf("kubernetes components installation failed with exit code %d: %s", result.ExitCode, result.Stderr)
	}

	return nil
}

func (i *Installer) collectVersions(ctx context.Context) (*ComponentVersions, error) {
	versions := &ComponentVersions{}

	result, _ := i.sshConn.ExecuteCommand(ctx, "containerd --version 2>/dev/null || crio --version 2>/dev/null | head -1 || docker --version 2>/dev/null || echo ''")
	if result.Stdout != "" {
		versions.ContainerRuntime = extractVersion(result.Stdout)
	}

	result, _ = i.sshConn.ExecuteCommand(ctx, "kubeadm version -o short 2>/dev/null || echo ''")
	if result.Stdout != "" {
		versions.Kubeadm = result.Stdout
	}

	result, _ = i.sshConn.ExecuteCommand(ctx, "kubelet --version 2>/dev/null || echo ''")
	if result.Stdout != "" {
		versions.Kubelet = extractVersion(result.Stdout)
	}

	result, _ = i.sshConn.ExecuteCommand(ctx, "kubectl version --client --short 2>/dev/null || echo ''")
	if result.Stdout != "" {
		versions.Kubectl = extractVersion(result.Stdout)
	}

	return versions, nil
}

func (i *Installer) rollback(ctx context.Context) error {
	script := `#!/bin/bash
set -euo pipefail
echo "=== Rolling back component installation ==="

systemctl stop kubelet 2>/dev/null || true
systemctl stop containerd 2>/dev/null || true
systemctl stop crio 2>/dev/null || true
systemctl stop docker 2>/dev/null || true

if command -v apt-get &>/dev/null; then
    apt-get remove -y kubelet kubeadm kubectl containerd 2>/dev/null || true
elif command -v dnf &>/dev/null; then
    dnf remove -y kubelet kubeadm kubectl containerd 2>/dev/null || true
elif command -v yum &>/dev/null; then
    yum remove -y kubelet kubeadm kubectl containerd 2>/dev/null || true
elif command -v zypper &>/dev/null; then
    zypper remove -y kubelet kubeadm kubectl containerd 2>/dev/null || true
fi

rm -f /tmp/.capbm_install_progress
echo "=== Rollback completed ==="
`
	result, err := i.sshConn.ExecuteScript(ctx, script)
	if err != nil {
		return fmt.Errorf("rollback failed: %w, stderr: %s", err, result.Stderr)
	}
	return nil
}

func prependEnvVars(script string, envVars map[string]string) string {
	for key, val := range envVars {
		if val != "" {
			script = prependEnvVar(script, key, val)
		}
	}
	return script
}

func prependEnvVar(script string, key, val string) string {
	if val == "" {
		return script
	}
	return fmt.Sprintf("%s=%q\n%s", key, val, script)
}

func extractVersion(output string) string {
	output = strings.TrimSpace(output)
	for i := 0; i < len(output); i++ {
		if output[i] >= '0' && output[i] <= '9' {
			end := i + 1
			for end < len(output) && (output[end] >= '0' && output[end] <= '9' || output[end] == '.') {
				end++
			}
			start := i
			for start > 0 && (output[start-1] >= '0' && output[start-1] <= '9' || output[start-1] == 'v' || output[start-1] == '.') {
				if output[start-1] == 'v' || output[start-1] == '.' {
					start--
				} else {
					break
				}
			}
			return output[start:end]
		}
	}
	return output
}
