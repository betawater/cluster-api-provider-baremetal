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

package helm

import (
	"context"
	"fmt"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Component status constants.
const (
	StatusPending    = "pending"
	StatusInstalling = "installing"
	StatusInstalled  = "installed"
	StatusFailed     = "failed"
	StatusUpgrading  = "upgrading"
	StatusNotInstalled = "not-installed"
)

// ComponentStatus represents the status of a component.
type ComponentStatus struct {
	// Version is the installed version.
	Version string `json:"version,omitempty"`
	// Status is the current status (pending/installing/installed/failed).
	Status string `json:"status"`
	// JobName is the name of the Job that installed/upgraded the component.
	JobName string `json:"jobName,omitempty"`
	// Message is an optional status message.
	Message string `json:"message,omitempty"`
}

// StatusTracker tracks component installation status.
type StatusTracker struct {
	client    client.Client
	namespace string
}

// NewStatusTracker creates a new status tracker.
func NewStatusTracker(c client.Client, namespace string) *StatusTracker {
	return &StatusTracker{
		client:    c,
		namespace: namespace,
	}
}

// GetComponentStatus returns the status of a component.
func (t *StatusTracker) GetComponentStatus(ctx context.Context, componentName string) (*ComponentStatus, error) {
	// First, try to get status from ConfigMap
	cm := &corev1.ConfigMap{}
	err := t.client.Get(ctx, types.NamespacedName{
		Name:      StatusConfigMapName,
		Namespace: t.namespace,
	}, cm)

	if err == nil {
		if statusData, ok := cm.Data[componentName]; ok {
			return parseComponentStatus(statusData)
		}
	}

	// Fallback: check Job status
	jobList := &batchv1.JobList{}
	err = t.client.List(ctx, jobList, client.InNamespace(t.namespace), client.MatchingLabels{
		LabelComponent: componentName,
	})

	if err == nil && len(jobList.Items) > 0 {
		// Get the most recent job
		job := jobList.Items[len(jobList.Items)-1]
		return t.getJobStatus(&job), nil
	}

	return &ComponentStatus{Status: StatusNotInstalled}, nil
}

// UpdateComponentStatus updates the status of a component in the ConfigMap.
func (t *StatusTracker) UpdateComponentStatus(ctx context.Context, componentName string, status *ComponentStatus) error {
	cm := &corev1.ConfigMap{}
	err := t.client.Get(ctx, types.NamespacedName{
		Name:      StatusConfigMapName,
		Namespace: t.namespace,
	}, cm)

	if err != nil {
		// ConfigMap doesn't exist, create it
		cm = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      StatusConfigMapName,
				Namespace: t.namespace,
			},
			Data: map[string]string{},
		}
	}

	// Serialize status
	statusStr := fmt.Sprintf("version: %s\nstatus: %s\njobName: %s", status.Version, status.Status, status.JobName)
	if status.Message != "" {
		statusStr += fmt.Sprintf("\nmessage: %s", status.Message)
	}

	if cm.Data == nil {
		cm.Data = map[string]string{}
	}
	cm.Data[componentName] = statusStr

	if err := t.client.Get(ctx, types.NamespacedName{
		Name:      StatusConfigMapName,
		Namespace: t.namespace,
	}, &corev1.ConfigMap{}); err != nil {
		return t.client.Create(ctx, cm)
	}

	return t.client.Update(ctx, cm)
}

// GetAllComponentStatus returns the status of all components.
func (t *StatusTracker) GetAllComponentStatus(ctx context.Context) (map[string]*ComponentStatus, error) {
	statuses := make(map[string]*ComponentStatus)

	cm := &corev1.ConfigMap{}
	err := t.client.Get(ctx, types.NamespacedName{
		Name:      StatusConfigMapName,
		Namespace: t.namespace,
	}, cm)

	if err == nil {
		for name, data := range cm.Data {
			status, err := parseComponentStatus(data)
			if err == nil {
				statuses[name] = status
			}
		}
	}

	// Also check jobs for components not in ConfigMap
	jobList := &batchv1.JobList{}
	err = t.client.List(ctx, jobList, client.InNamespace(t.namespace), client.MatchingLabels{
		LabelType: JobTypeInstall,
	})

	if err == nil {
		for _, job := range jobList.Items {
			componentName := job.Labels[LabelComponent]
			if _, exists := statuses[componentName]; !exists {
				statuses[componentName] = t.getJobStatus(&job)
			}
		}
	}

	return statuses, nil
}

// getJobStatus returns the status from a Job.
func (t *StatusTracker) getJobStatus(job *batchv1.Job) *ComponentStatus {
	status := &ComponentStatus{
		Version: job.Labels[LabelVersion],
		JobName: job.Name,
	}

	if job.Status.Succeeded > 0 {
		status.Status = StatusInstalled
	} else if job.Status.Failed > 0 {
		status.Status = StatusFailed
		if len(job.Status.Conditions) > 0 {
			status.Message = job.Status.Conditions[0].Message
		}
	} else {
		status.Status = StatusInstalling
	}

	return status
}

// parseComponentStatus parses a component status string.
func parseComponentStatus(data string) (*ComponentStatus, error) {
	status := &ComponentStatus{}

	lines := strings.Split(data, "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, ": ", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "version":
			status.Version = value
		case "status":
			status.Status = value
		case "jobName":
			status.JobName = value
		case "message":
			status.Message = value
		}
	}

	if status.Status == "" {
		return nil, fmt.Errorf("invalid status data: missing status field")
	}

	return status, nil
}

// CreateStatusConfigMap creates the initial status ConfigMap.
func (t *StatusTracker) CreateStatusConfigMap(ctx context.Context, components []string) error {
	data := make(map[string]string)
	for _, name := range components {
		data[name] = fmt.Sprintf("status: %s", StatusPending)
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      StatusConfigMapName,
			Namespace: t.namespace,
		},
		Data: data,
	}

	return t.client.Create(ctx, cm)
}
