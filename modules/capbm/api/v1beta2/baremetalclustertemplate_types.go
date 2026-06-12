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

package v1beta2

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BareMetalClusterTemplateSpec defines the desired state of BareMetalClusterTemplate.
type BareMetalClusterTemplateSpec struct {
	// Template defines the template for BareMetalCluster.
	Template BareMetalClusterTemplateResource `json:"template"`
}

// BareMetalClusterTemplateResource defines the template resource.
type BareMetalClusterTemplateResource struct {
	// Standard object's metadata.
	// +optional
	ObjectMeta ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the desired state of BareMetalCluster.
	Spec BareMetalClusterSpec `json:"spec"`
}

// ObjectMeta is metadata that all persisted resources must have, which includes all objects users must create.
type ObjectMeta struct {
	// Map of string keys and values that can be used to organize and categorize (scope and select) objects.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations is an unstructured key value map stored with a resource.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

// +kubebuilder:metadata:labels="cluster.x-k8s.io/v1beta2=v1beta2"
// +kubebuilder:object:root=true

// BareMetalClusterTemplate is the Schema for the baremetalclustertemplates API.
type BareMetalClusterTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec BareMetalClusterTemplateSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// BareMetalClusterTemplateList contains a list of BareMetalClusterTemplate.
type BareMetalClusterTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BareMetalClusterTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BareMetalClusterTemplate{}, &BareMetalClusterTemplateList{})
}
