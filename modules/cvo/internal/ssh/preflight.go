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
	"context"
	"fmt"
	"strconv"
	"strings"
)

// PreflightConfig holds the configuration for pre-flight checks.
type PreflightConfig struct {
	MinDiskGB     int
	MinMemoryGB   int
	KernelVersion string
	OSVersions    []string
}

// DefaultPreflightConfig returns the default pre-flight configuration.
func DefaultPreflightConfig() PreflightConfig {
	return PreflightConfig{
		MinDiskGB:     20,
		MinMemoryGB:   2,
		KernelVersion: "3.10",
		OSVersions:    []string{"centos", "rhel", "almalinux", "rocky", "ubuntu", "debian"},
	}
}

// PreflightResult holds the results of pre-flight checks.
type PreflightResult struct {
	OSVersion     string
	KernelVersion string
	DiskAvailableGB int
	MemoryTotalGB  int
	SwapEnabled    bool
	NetworkReachable bool
	Passed         bool
	Errors         []string
	Warnings       []string
}

// RunPreflightChecks executes all pre-flight checks on the remote host.
func RunPreflightChecks(ctx context.Context, conn *SSHConnection, config PreflightConfig) (*PreflightResult, error) {
	result := &PreflightResult{
		Passed: true,
	}

	if err := checkOS(ctx, conn, config, result); err != nil {
		return result, err
	}

	if err := checkKernel(ctx, conn, config, result); err != nil {
		return result, err
	}

	if err := checkDisk(ctx, conn, config, result); err != nil {
		return result, err
	}

	if err := checkMemory(ctx, conn, config, result); err != nil {
		return result, err
	}

	checkNetwork(ctx, conn, result)
	checkSwap(ctx, conn, result)

	if len(result.Errors) > 0 {
		result.Passed = false
	}

	return result, nil
}

func checkOS(ctx context.Context, conn *SSHConnection, config PreflightConfig, result *PreflightResult) error {
	cmdResult, err := conn.ExecuteCommand(ctx, "cat /etc/os-release 2>/dev/null || echo 'NO_OS_RELEASE'")
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("failed to detect OS: %v", err))
		return nil
	}

	if strings.Contains(cmdResult.Stdout, "NO_OS_RELEASE") {
		result.Errors = append(result.Errors, "cannot detect OS: /etc/os-release not found")
		return nil
	}

	osID := extractOSField(cmdResult.Stdout, "ID")
	osVersion := extractOSField(cmdResult.Stdout, "VERSION_ID")
	result.OSVersion = fmt.Sprintf("%s:%s", osID, osVersion)

	supported := false
	for _, supportedOS := range config.OSVersions {
		if osID == supportedOS {
			supported = true
			break
		}
	}

	if !supported {
		result.Errors = append(result.Errors, fmt.Sprintf("unsupported OS: %s", osID))
	}

	return nil
}

func checkKernel(ctx context.Context, conn *SSHConnection, config PreflightConfig, result *PreflightResult) error {
	cmdResult, err := conn.ExecuteCommand(ctx, "uname -r")
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("failed to get kernel version: %v", err))
		return nil
	}

	kernelVersion := strings.TrimSpace(cmdResult.Stdout)
	kernelMajor := strings.Split(kernelVersion, "-")[0]
	result.KernelVersion = kernelMajor

	if !versionGreaterOrEqual(kernelMajor, config.KernelVersion) {
		result.Errors = append(result.Errors, fmt.Sprintf("kernel version %s is too old, need >= %s", kernelMajor, config.KernelVersion))
	}

	return nil
}

func checkDisk(ctx context.Context, conn *SSHConnection, config PreflightConfig, result *PreflightResult) error {
	cmdResult, err := conn.ExecuteCommand(ctx, "df -BG / | awk 'NR==2 {print $4}' | tr -d 'G'")
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("failed to check disk space: %v", err))
		return nil
	}

	availableGB, err := strconv.Atoi(strings.TrimSpace(cmdResult.Stdout))
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("failed to parse disk space: %v", err))
		return nil
	}

	result.DiskAvailableGB = availableGB
	if availableGB < config.MinDiskGB {
		result.Errors = append(result.Errors, fmt.Sprintf("insufficient disk space: %dGB available, need %dGB", availableGB, config.MinDiskGB))
	}

	return nil
}

func checkMemory(ctx context.Context, conn *SSHConnection, config PreflightConfig, result *PreflightResult) error {
	cmdResult, err := conn.ExecuteCommand(ctx, "free -g | awk '/^Mem:/{print $2}'")
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("failed to check memory: %v", err))
		return nil
	}

	totalGB, err := strconv.Atoi(strings.TrimSpace(cmdResult.Stdout))
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("failed to parse memory: %v", err))
		return nil
	}

	result.MemoryTotalGB = totalGB
	if totalGB < config.MinMemoryGB {
		result.Errors = append(result.Errors, fmt.Sprintf("insufficient memory: %dGB, need %dGB", totalGB, config.MinMemoryGB))
	}

	return nil
}

func checkNetwork(ctx context.Context, conn *SSHConnection, result *PreflightResult) {
	cmdResult, err := conn.ExecuteCommand(ctx, "ping -c 1 -W 2 8.8.8.8 &>/dev/null && echo 'reachable' || echo 'unreachable'")
	if err != nil {
		result.Warnings = append(result.Warnings, "failed to check network connectivity")
		return
	}

	result.NetworkReachable = strings.Contains(cmdResult.Stdout, "reachable")
	if !result.NetworkReachable {
		result.Warnings = append(result.Warnings, "cannot reach external network")
	}
}

func checkSwap(ctx context.Context, conn *SSHConnection, result *PreflightResult) {
	cmdResult, err := conn.ExecuteCommand(ctx, "swapon --show | grep -q . && echo 'enabled' || echo 'disabled'")
	if err != nil {
		result.Warnings = append(result.Warnings, "failed to check swap status")
		return
	}

	result.SwapEnabled = strings.Contains(cmdResult.Stdout, "enabled")
	if result.SwapEnabled {
		result.Warnings = append(result.Warnings, "swap is enabled, should be disabled for Kubernetes")
	}
}

func extractOSField(content, field string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, field+"=") {
			value := strings.TrimPrefix(line, field+"=")
			return strings.Trim(value, "\"'")
		}
	}
	return ""
}

func versionGreaterOrEqual(version, required string) bool {
	vParts := strings.Split(version, ".")
	rParts := strings.Split(required, ".")

	for i := 0; i < len(rParts); i++ {
		if i >= len(vParts) {
			return false
		}

		vNum, vErr := strconv.Atoi(vParts[i])
		rNum, rErr := strconv.Atoi(rParts[i])

		// If parsing fails, treat as 0
		if vErr != nil {
			vNum = 0
		}
		if rErr != nil {
			rNum = 0
		}

		if vNum > rNum {
			return true
		}
		if vNum < rNum {
			return false
		}
	}

	return true
}
