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
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"


)

// ClusterAddonSpec defines the desired state of ClusterAddon.
type ClusterAddonSpec struct {
	// ClusterRef is the reference to the target workload cluster.
	ClusterRef corev1.ObjectReference `json:"clusterRef"`

	// ReleaseImageRef is the reference to the ReleaseImage containing this addon.
	ReleaseImageRef corev1.LocalObjectReference `json:"releaseImageRef"`

	// AddonName is the name of the addon in the ReleaseImage.
	AddonName string `json:"addonName"`

	// Values are the configuration values that override defaults from ReleaseImage.
	// For Helm: helm values
	// For Manifest: variable substitutions
	Values map[string]apiextensionsv1.JSON `json:"values,omitempty"`

	// Namespace overrides the default namespace from ReleaseImage.
	Namespace string `json:"namespace,omitempty"`

	// Prune enables garbage collection of resources no longer in the addon.
	Prune bool `json:"prune,omitempty"`

	// HealthCheck overrides the health check from ReleaseImage.
	HealthCheck *AddonHealthCheck `json:"healthCheck,omitempty"`
}

// ClusterAddonStatus defines the observed state of ClusterAddon.
type ClusterAddonStatus struct {
	// Phase is the current phase of the addon.
	Phase AddonPhase `json:"phase,omitempty"`

	// Version is the currently installed version.
	Version string `json:"version,omitempty"`

	// Conditions represents the observations of the addon's current state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// LastAppliedRevision is the last successfully applied revision.
	LastAppliedRevision string `json:"lastAppliedRevision,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase",description="Addon phase"
// +kubebuilder:printcolumn:name="Version",type="string",JSONPath=".status.version",description="Addon version"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation"

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase",description="Addon phase"
// +kubebuilder:printcolumn:name="Version",type="string",JSONPath=".status.version",description="Addon version"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation"

// ClusterAddon is the Schema for the clusteraddons API.
type ClusterAddon struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterAddonSpec   `json:"spec,omitempty"`
	Status ClusterAddonStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// +kubebuilder:object:root=true

// ClusterAddonList contains a list of ClusterAddon.
type ClusterAddonList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterAddon `json:"items"`
}
