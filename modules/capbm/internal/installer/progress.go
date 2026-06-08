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
	"encoding/json"
	"fmt"
	"time"

	sshclient "github.com/BetaWater/cluster-api-provider-baremetal/modules/cvo/pkg/ssh"
)

const progressFile = "/tmp/.capbm_install_progress"

type InstallProgress struct {
	Step      string    `json:"step"`
	Status    string    `json:"status"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

func updateProgress(sshConn *sshclient.SSHConnection, step, message string) {
	progress := InstallProgress{
		Step:      step,
		Status:    "running",
		Message:   message,
		Timestamp: time.Now().UTC(),
	}
	writeProgress(sshConn, progress)
}

func writeProgress(sshConn *sshclient.SSHConnection, progress InstallProgress) {
	if sshConn == nil {
		return
	}
	data, err := json.Marshal(progress)
	if err != nil {
		return
	}
	script := fmt.Sprintf("echo '%s' > %s", string(data), progressFile)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = sshConn.ExecuteCommand(ctx, script)
	// Progress write is non-critical; log but don't fail installation
	_ = err
}

func GetProgress(ctx context.Context, sshConn *sshclient.SSHConnection) (*InstallProgress, error) {
	result, err := sshConn.ExecuteCommand(ctx, fmt.Sprintf("cat %s 2>/dev/null || echo '{}'", progressFile))
	if err != nil {
		return nil, err
	}

	var progress InstallProgress
	if err := json.Unmarshal([]byte(result.Stdout), &progress); err != nil {
		return nil, fmt.Errorf("failed to parse progress: %w", err)
	}

	return &progress, nil
}

func MarkProgressFailed(sshConn *sshclient.SSHConnection, step, errMsg string) {
	progress := InstallProgress{
		Step:      step,
		Status:    "failed",
		Message:   errMsg,
		Timestamp: time.Now().UTC(),
	}
	writeProgress(sshConn, progress)
}

func MarkProgressCompleted(sshConn *sshclient.SSHConnection, step string) {
	progress := InstallProgress{
		Step:      step,
		Status:    "completed",
		Message:   "Installation completed successfully",
		Timestamp: time.Now().UTC(),
	}
	writeProgress(sshConn, progress)
}
