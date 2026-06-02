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

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// UpgradePath is the Schema for the upgradepaths API.
type UpgradePath struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   UpgradePathSpec   `json:"spec,omitempty"`
	Status UpgradePathStatus `json:"status,omitempty"`
}

// UpgradePathSpec defines the desired state of UpgradePath.
type UpgradePathSpec struct {
	Image string             `json:"image"`
	Graph UpgradeGraphData   `json:"graph"`
	Rules CompatibilityRules `json:"rules,omitempty"`
}

// UpgradeGraphData defines the upgrade graph structure.
type UpgradeGraphData struct {
	Edges []GraphEdge `json:"edges"`
}

// GraphEdge defines a single edge in the upgrade graph.
type GraphEdge struct {
	From        string `json:"from"`
	To          string `json:"to"`
	Recommended bool   `json:"recommended"`
}

// CompatibilityRules defines compatibility rules for upgrades.
type CompatibilityRules struct {
	MaxVersionSkip  int              `json:"maxVersionSkip,omitempty"`
	BlockedUpgrades []BlockedUpgrade `json:"blockedUpgrades,omitempty"`
}

// BlockedUpgrade defines a blocked upgrade path.
type BlockedUpgrade struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Reason string `json:"reason"`
}

// UpgradePathStatus defines the observed state of UpgradePath.
type UpgradePathStatus struct {
	LastSyncTime  metav1.Time `json:"lastSyncTime,omitempty"`
	SyncSucceeded bool        `json:"syncSucceeded"`
	ImageDigest   string      `json:"imageDigest,omitempty"`
}

// +kubebuilder:object:root=true

// UpgradePathList contains a list of UpgradePath.
type UpgradePathList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []UpgradePath `json:"items"`
}
