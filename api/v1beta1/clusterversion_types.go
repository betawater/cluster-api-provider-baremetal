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

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

const (
	ClusterVersionFinalizer = "clusterversion.infrastructure.cluster.x-k8s.io"

	UpgradeAvailable   clusterv1.ConditionType = "Available"
	UpgradeProgressing clusterv1.ConditionType = "Progressing"
	UpgradeFailing     clusterv1.ConditionType = "Failing"
	UpgradeUpgradeable clusterv1.ConditionType = "Upgradeable"
	UpgradeRetrieved   clusterv1.ConditionType = "RetrievedUpdates"

	UpgradeAvailableReason   = "AsExpected"
	UpgradeProgressingReason = "AsExpected"
	UpgradeFailingReason     = "AsExpected"
	UpgradeUpgradeableReason = "PreconditionsPassed"
	UpgradeRetrievedReason   = "AsExpected"
	ValidationFailedReason   = "ValidationFailed"
	PullFailedReason         = "PullFailed"
	UpgradeFailedReason      = "UpgradeFailed"
)

type UpdateState string

const (
	CompletedUpdate UpdateState = "Completed"
	PartialUpdate   UpdateState = "Partial"
)

type ClusterVersion struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterVersionSpec   `json:"spec,omitempty"`
	Status ClusterVersionStatus `json:"status,omitempty"`
}

type ClusterVersionSpec struct {
	ClusterRef    corev1.ObjectReference `json:"clusterRef"`
	Channel       string                 `json:"channel,omitempty"`
	DesiredUpdate *Update                `json:"desiredUpdate,omitempty"`
}

type Update struct {
	Version string `json:"version,omitempty"`
	Image   string `json:"image,omitempty"`
	Force   bool   `json:"force,omitempty"`
}

type ClusterVersionStatus struct {
	ObservedGeneration int64              `json:"observedGeneration"`
	Desired            Release            `json:"desired"`
	ActualVersion      string             `json:"actualVersion"`
	History            []UpdateHistory    `json:"history,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
	AvailableUpdates   []Release          `json:"availableUpdates,omitempty"`
	ComponentStatus    []ComponentStatus  `json:"componentStatus,omitempty"`
}

type Release struct {
	Version string `json:"version"`
	Image   string `json:"image"`
}

type UpdateHistory struct {
	State          UpdateState  `json:"state"`
	Version        string       `json:"version"`
	Image          string       `json:"image"`
	Verified       bool         `json:"verified"`
	StartedTime    metav1.Time  `json:"startedTime"`
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`
}

type ComponentStatus struct {
	Name          string `json:"name"`
	Version       string `json:"version"`
	TargetVersion string `json:"targetVersion"`
	Phase         string `json:"phase"`
}

type ClusterVersionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterVersion `json:"items"`
}
