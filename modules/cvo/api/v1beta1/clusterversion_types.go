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
	ClusterVersionFinalizer = "clusterversion.cvo.capbm.io"

	//nolint:staticcheck // ConditionType deprecated in CAPI v1beta2, will migrate when ready
	UpgradeAvailable   clusterv1.ConditionType = "Available"
	//nolint:staticcheck // ConditionType deprecated in CAPI v1beta2, will migrate when ready
	UpgradeProgressing clusterv1.ConditionType = "Progressing"
	//nolint:staticcheck // ConditionType deprecated in CAPI v1beta2, will migrate when ready
	UpgradeFailing     clusterv1.ConditionType = "Failing"
	//nolint:staticcheck // ConditionType deprecated in CAPI v1beta2, will migrate when ready
	UpgradeUpgradeable clusterv1.ConditionType = "Upgradeable"
	//nolint:staticcheck // ConditionType deprecated in CAPI v1beta2, will migrate when ready
	UpgradeRetrieved   clusterv1.ConditionType = "RetrievedUpdates"

	UpgradeAvailableReason   = "AsExpected"
	UpgradeProgressingReason = "AsExpected"
	UpgradeFailingReason     = "AsExpected"
	UpgradeUpgradeableReason = "PreconditionsPassed"
	UpgradeRetrievedReason   = "AsExpected"
	ValidationFailedReason   = "ValidationFailed"
	PullFailedReason         = "PullFailed"
	UpgradeFailedReason      = "UpgradeFailed"

	// Addon upgrade conditions
	//nolint:staticcheck // ConditionType deprecated in CAPI v1beta2, will migrate when ready
	AddonUpgradeProgressing clusterv1.ConditionType = "AddonUpgradeProgressing"
	//nolint:staticcheck // ConditionType deprecated in CAPI v1beta2, will migrate when ready
	AddonUpgradeFailing     clusterv1.ConditionType = "AddonUpgradeFailing"
	//nolint:staticcheck // ConditionType deprecated in CAPI v1beta2, will migrate when ready
	AddonUpgradeCompleted   clusterv1.ConditionType = "AddonUpgradeCompleted"

	AddonUpgradeProgressingReason = "AddonUpgrading"
	AddonUpgradeFailingReason     = "AddonUpgradeFailed"
	AddonUpgradeCompletedReason   = "AddonUpgradeComplete"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ClusterVersion is the Schema for the clusterversions API.
type ClusterVersion struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterVersionSpec   `json:"spec,omitempty"`
	Status ClusterVersionStatus `json:"status,omitempty"`
}

// ClusterVersionSpec defines the desired state of ClusterVersion.
type ClusterVersionSpec struct {
	ClusterRef    corev1.ObjectReference `json:"clusterRef"`
	Channel       string                 `json:"channel,omitempty"`
	DesiredUpdate *Update       `json:"desiredUpdate,omitempty"`
}

// ClusterVersionStatus defines the observed state of ClusterVersion.
type ClusterVersionStatus struct {
	ObservedGeneration int64                    `json:"observedGeneration"`
	Desired            Release         `json:"desired"`
	ActualVersion      string                   `json:"actualVersion"`
	History            []UpdateHistory `json:"history,omitempty"`
	Conditions         []metav1.Condition       `json:"conditions,omitempty"`
	AvailableUpdates   []Release       `json:"availableUpdates,omitempty"`
	ComponentStatus    []ComponentStatus `json:"componentStatus,omitempty"`

	// AddonStatus tracks addon versions after upgrade.
	// +optional
	AddonStatus []AddonVersionStatus `json:"addonStatus,omitempty"`
}

// AddonVersionStatus tracks the status of a single addon.
type AddonVersionStatus struct {
	// Name is the addon name.
	Name string `json:"name"`

	// Version is the currently installed version.
	Version string `json:"version"`

	// TargetVersion is the desired version from the target ReleaseImage.
	// +optional
	TargetVersion string `json:"targetVersion,omitempty"`

	// Phase is the current upgrade phase.
	// +optional
	Phase AddonPhase `json:"phase,omitempty"`

	// LastTransitionTime is the last time the status changed.
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterVersionList contains a list of ClusterVersion.
type ClusterVersionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterVersion `json:"items"`
}
