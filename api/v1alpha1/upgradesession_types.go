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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// UpgradePhase represents the phase of an upgrade session.
type UpgradePhase string

const (
	// UpgradePhasePending indicates the upgrade is pending.
	UpgradePhasePending UpgradePhase = "Pending"
	
	// UpgradePhaseBackingUp indicates the upgrade is backing up etcd.
	UpgradePhaseBackingUp UpgradePhase = "BackingUp"
	
	// UpgradePhaseUpgrading indicates the upgrade is in progress.
	UpgradePhaseUpgrading UpgradePhase = "Upgrading"
	
	// UpgradePhaseVerifying indicates the upgrade is being verified.
	UpgradePhaseVerifying UpgradePhase = "Verifying"
	
	// UpgradePhaseCompleted indicates the upgrade is completed.
	UpgradePhaseCompleted UpgradePhase = "Completed"
	
	// UpgradePhaseFailed indicates the upgrade has failed.
	UpgradePhaseFailed UpgradePhase = "Failed"
	
	// UpgradePhaseRollingBack indicates the upgrade is rolling back.
	UpgradePhaseRollingBack UpgradePhase = "RollingBack"
)

// UpgradeSessionSpec defines the desired state of UpgradeSession.
type UpgradeSessionSpec struct {
	// ClusterRef is the reference to the cluster being upgraded.
	ClusterRef corev1.ObjectReference `json:"clusterRef"`
	
	// TargetVersion is the target Kubernetes version.
	TargetVersion string `json:"targetVersion"`
	
	// SourceVersion is the current Kubernetes version before upgrade.
	SourceVersion string `json:"sourceVersion"`
	
	// UpgradeStrategy defines the upgrade strategy.
	UpgradeStrategy UpgradeStrategyConfig `json:"upgradeStrategy"`
}

// UpgradeStrategyConfig defines the upgrade strategy configuration.
type UpgradeStrategyConfig struct {
	// Type is the upgrade type: InPlace or Replace.
	// +kubebuilder:validation:Enum=InPlace;Replace
	Type string `json:"type"`
	
	// RollingUpdate defines rolling update settings.
	RollingUpdate RollingUpdateConfig `json:"rollingUpdate"`
	
	// EtcdBackup defines etcd backup settings.
	EtcdBackup EtcdBackupConfig `json:"etcdBackup"`
	
	// Rollback defines rollback settings.
	Rollback RollbackConfig `json:"rollback"`
}

// RollingUpdateConfig defines rolling update settings.
type RollingUpdateConfig struct {
	// MaxUnavailable is the maximum number of nodes that can be unavailable during upgrade.
	// +kubebuilder:default=1
	MaxUnavailable int `json:"maxUnavailable"`
	
	// Drain defines drain settings.
	Drain DrainConfig `json:"drain"`
	
	// Timeout is the timeout for single node upgrade.
	Timeout *metav1.Duration `json:"timeout,omitempty"`
}

// DrainConfig defines drain settings.
type DrainConfig struct {
	// Enabled indicates whether to drain pods before upgrade.
	// +kubebuilder:default=true
	Enabled bool `json:"enabled"`
	
	// Timeout is the timeout for drain operation.
	Timeout *metav1.Duration `json:"timeout,omitempty"`
	
	// IgnoreDaemonSets indicates whether to ignore daemonsets during drain.
	// +kubebuilder:default=true
	IgnoreDaemonSets bool `json:"ignoreDaemonSets"`
}

// EtcdBackupConfig defines etcd backup settings.
type EtcdBackupConfig struct {
	// Enabled indicates whether to backup etcd before upgrade.
	// +kubebuilder:default=true
	Enabled bool `json:"enabled"`
	
	// Timeout is the timeout for etcd backup.
	Timeout *metav1.Duration `json:"timeout,omitempty"`
	
	// Storage defines etcd backup storage settings.
	Storage EtcdBackupStorageConfig `json:"storage"`
}

// EtcdBackupStorageConfig defines etcd backup storage settings.
type EtcdBackupStorageConfig struct {
	// Type is the storage type: Secret or PVC.
	// +kubebuilder:validation:Enum=Secret;PVC
	Type string `json:"type"`
	
	// Retention is the number of backups to retain.
	// +kubebuilder:default=3
	Retention int `json:"retention"`
}

// RollbackConfig defines rollback settings.
type RollbackConfig struct {
	// Enabled indicates whether to enable automatic rollback.
	// +kubebuilder:default=true
	Enabled bool `json:"enabled"`
	
	// OnTimeout indicates whether to rollback on timeout.
	// +kubebuilder:default=true
	OnTimeout bool `json:"onTimeout"`
	
	// OnFailure indicates whether to rollback on failure.
	// +kubebuilder:default=true
	OnFailure bool `json:"onFailure"`
}

// UpgradeSessionStatus defines the observed state of UpgradeSession.
type UpgradeSessionStatus struct {
	// Phase is the current phase of the upgrade.
	Phase UpgradePhase `json:"phase"`
	
	// CurrentNode is the node currently being upgraded.
	CurrentNode string `json:"currentNode,omitempty"`
	
	// CompletedNodes is the list of nodes that have been successfully upgraded.
	CompletedNodes []string `json:"completedNodes,omitempty"`
	
	// FailedNodes is the list of nodes that failed to upgrade.
	FailedNodes []NodeFailureInfo `json:"failedNodes,omitempty"`
	
	// StartTime is the time when the upgrade started.
	StartTime metav1.Time `json:"startTime"`
	
	// CompletionTime is the time when the upgrade completed.
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`
	
	// BackupRef is the reference to the etcd backup.
	BackupRef *corev1.ObjectReference `json:"backupRef,omitempty"`
	
	// Conditions represents the observations of the upgrade session's current state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// NodeFailureInfo contains information about a node upgrade failure.
type NodeFailureInfo struct {
	// Node is the name of the failed node.
	Node string `json:"node"`
	
	// Error is the error message.
	Error string `json:"error"`
	
	// Timestamp is the time when the failure occurred.
	Timestamp metav1.Time `json:"timestamp"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="TargetVersion",type="string",JSONPath=".spec.targetVersion"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// UpgradeSession is the Schema for the upgradesessions API.
type UpgradeSession struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	
	Spec   UpgradeSessionSpec   `json:"spec,omitempty"`
	Status UpgradeSessionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// UpgradeSessionList contains a list of UpgradeSession.
type UpgradeSessionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []UpgradeSession `json:"items"`
}
