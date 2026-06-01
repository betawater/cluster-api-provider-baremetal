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

package v1beta1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ComponentUpgradeConfig defines component-level upgrade configuration (high cohesion).
// Backup, rollback, and health check are defined together with the component.
type ComponentUpgradeConfig struct {
	// Backup configuration for this component.
	// +optional
	Backup ComponentBackupConfig `json:"backup"`

	// Rollback configuration for this component.
	// +optional
	Rollback ComponentRollbackConfig `json:"rollback"`

	// HealthCheck configuration after upgrade.
	// +optional
	HealthCheck ComponentHealthCheckConfig `json:"healthCheck"`
}

// ComponentBackupConfig defines backup configuration for a component.
type ComponentBackupConfig struct {
	// Enabled indicates whether backup is enabled for this component.
	// +optional
	// +kubebuilder:default=true
	Enabled bool `json:"enabled"`

	// Config lists the configuration items to backup.
	// +optional
	Config []BackupItem `json:"config,omitempty"`

	// EtcdSnapshot indicates whether etcd snapshot is required (for control-plane components).
	// +optional
	EtcdSnapshot bool `json:"etcdSnapshot,omitempty"`
}

// BackupItem defines a single backup item.
type BackupItem struct {
	// Path is the file or directory path to backup.
	Path string `json:"path"`

	// Type is the backup item type: file or directory.
	// +kubebuilder:validation:Enum=file;directory
	Type string `json:"type"`
}

// ComponentRollbackConfig defines rollback configuration for a component.
type ComponentRollbackConfig struct {
	// Script is the path to the rollback script (relative to release image scripts directory).
	Script string `json:"script"`

	// Timeout is the maximum time allowed for rollback.
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`
}

// ComponentHealthCheckConfig defines health check configuration.
type ComponentHealthCheckConfig struct {
	// Command is the health check command to execute.
	Command string `json:"command"`

	// Timeout is the maximum time allowed for health check.
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// Retries is the number of retries before considering health check failed.
	// +optional
	// +kubebuilder:default=3
	Retries int `json:"retries,omitempty"`
}

// UpgradePhase defines a phase in the upgrade graph.
type UpgradePhase struct {
	Name          string             `json:"name"`
	Order         int                `json:"order"`
	Blocking      bool               `json:"blocking"`
	RollingUpdate *RollingUpdate     `json:"rollingUpdate,omitempty"`
	Components    []UpgradeComponent `json:"components"`
}

// RollingUpdate defines rolling update settings.
type RollingUpdate struct {
	MaxUnavailable int `json:"maxUnavailable,omitempty"`
}

// UpgradeComponent defines a component in an upgrade phase.
type UpgradeComponent struct {
	Name        string       `json:"name"`
	Manifests   []string     `json:"manifests,omitempty"`
	Scripts     []string     `json:"scripts,omitempty"`
	Blocking    bool         `json:"blocking"`
	DependsOn   []string     `json:"dependsOn,omitempty"`
	HealthCheck *HealthCheck `json:"healthCheck,omitempty"`
}

// HealthCheck defines health check settings for an upgrade component.
type HealthCheck struct {
	Type          string          `json:"type"`
	Namespace     string          `json:"namespace,omitempty"`
	Name          string          `json:"name,omitempty"`
	LabelSelector string          `json:"labelSelector,omitempty"`
	Endpoint      string          `json:"endpoint,omitempty"`
	Timeout       metav1.Duration `json:"timeout,omitempty"`
}
