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

	sshclient "github.com/BetaWater/cluster-api-provider-baremetal/internal/ssh"
)

type OSInfo struct {
	Type        string `json:"type"`
	Version     string `json:"version"`
	ID          string `json:"id"`
	IDLike      string `json:"idLike"`
	PackageMgr  string `json:"packageMgr"`
	Arch        string `json:"arch"`
}

type OSDetector struct {
	sshConn *sshclient.SSHConnection
}

func NewOSDetector(sshConn *sshclient.SSHConnection) *OSDetector {
	return &OSDetector{sshConn: sshConn}
}

func (d *OSDetector) Detect(ctx context.Context) (*OSInfo, error) {
	osInfo := &OSInfo{}

	result, err := d.sshConn.ExecuteCommand(ctx, "cat /etc/os-release 2>/dev/null || echo ''")
	if err != nil || result.Stdout == "" {
		return nil, fmt.Errorf("failed to detect OS: cannot read /etc/os-release")
	}

	osInfo.Type, osInfo.Version, osInfo.ID, osInfo.IDLike = parseOSRelease(result.Stdout)

	if osInfo.Type == "" {
		return nil, fmt.Errorf("unsupported OS: %s", result.Stdout)
	}

	osInfo.PackageMgr = detectPackageManager(osInfo.Type)

	archResult, _ := d.sshConn.ExecuteCommand(ctx, "uname -m")
	if archResult.Stdout != "" {
		osInfo.Arch = strings.TrimSpace(archResult.Stdout)
	} else {
		osInfo.Arch = "amd64"
	}

	return osInfo, nil
}

func parseOSRelease(content string) (osType, version, id, idLike string) {
	lines := strings.Split(content, "\n")
	values := make(map[string]string)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := parts[0]
			val := strings.Trim(parts[1], "\"'")
			values[key] = val
		}
	}

	rawID := strings.ToLower(values["ID"])
	version = values["VERSION_ID"]
	idLike = strings.ToLower(values["ID_LIKE"])

	switch {
	case strings.Contains(rawID, "ubuntu"):
		osType = "ubuntu"
		id = "ubuntu"
	case strings.Contains(rawID, "debian"):
		osType = "debian"
		id = "debian"
	case strings.Contains(rawID, "centos"):
		osType = "rhel"
		id = "centos"
	case strings.Contains(rawID, "rhel"):
		osType = "rhel"
		id = "rhel"
	case strings.Contains(rawID, "rocky"):
		osType = "rhel"
		id = "rocky"
	case strings.Contains(rawID, "alma"):
		osType = "rhel"
		id = "almalinux"
	case strings.Contains(rawID, "fedora"):
		osType = "rhel"
		id = "fedora"
	case strings.Contains(rawID, "amzn"):
		osType = "rhel"
		id = "amazonlinux"
	case strings.Contains(rawID, "opensuse") || strings.Contains(rawID, "sles") || strings.Contains(rawID, "suse"):
		osType = "suse"
		id = rawID
	case strings.Contains(rawID, "flatcar"):
		osType = "flatcar"
		id = "flatcar"
	case strings.Contains(idLike, "debian") || strings.Contains(idLike, "ubuntu"):
		osType = "debian"
		id = rawID
	case strings.Contains(idLike, "rhel") || strings.Contains(idLike, "centos") || strings.Contains(idLike, "fedora"):
		osType = "rhel"
		id = rawID
	default:
		osType = ""
		id = rawID
	}

	return osType, version, id, idLike
}

func detectPackageManager(osType string) string {
	switch osType {
	case "ubuntu", "debian":
		return "apt"
	case "rhel", "centos", "rocky", "almalinux", "fedora", "amazonlinux":
		return "yum"
	case "suse", "opensuse", "sles":
		return "zypper"
	case "flatcar":
		return "none"
	default:
		return "unknown"
	}
}
