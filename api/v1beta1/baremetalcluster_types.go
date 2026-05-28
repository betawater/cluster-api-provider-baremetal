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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
)

const (
	// ClusterFinalizer allows ReconcileBareMetalCluster to clean up resources associated with BareMetalCluster before
	// removing it from the apiserver.
	ClusterFinalizer = "baremetalcluster.infrastructure.cluster.x-k8s.io"

	// ClusterReadyCondition reports the current status of the cluster infrastructure.
	ClusterReadyCondition = clusterv1.ReadyCondition

	// EndpointNotSetReason indicates the control plane endpoint is not set.
	EndpointNotSetReason = "EndpointNotSet"

	// EndpointSourceAnnotation indicates the source of the control plane endpoint.
	EndpointSourceAnnotation = "baremetal.cluster.x-k8s.io/endpoint-source"
)

// BareMetalClusterSpec defines the desired state of BareMetalCluster.
type BareMetalClusterSpec struct {
	// ControlPlaneEndpoint represents the endpoint used to communicate with the control plane.
	// +optional
	ControlPlaneEndpoint clusterv1.APIEndpoint `json:"controlPlaneEndpoint,omitempty"`

	// Network holds the cluster-level network configuration.
	// +optional
	Network NetworkConfig `json:"network,omitempty"`
}

// NetworkConfig holds the network configuration for the cluster.
type NetworkConfig struct {
	// PodCIDR is the CIDR block for pods.
	// +optional
	PodCIDR string `json:"podCIDR,omitempty"`

	// ServiceCIDR is the CIDR block for services.
	// +optional
	ServiceCIDR string `json:"serviceCIDR,omitempty"`

	// DNSDomain is the DNS domain for the cluster.
	// +optional
	DNSDomain string `json:"dnsDomain,omitempty"`
}

// BareMetalClusterInitializationStatus provides observations of the BareMetalCluster initialization process.
type BareMetalClusterInitializationStatus struct {
	// Provisioned is true when the infrastructure provider reports that the Cluster's infrastructure is fully provisioned.
	// +optional
	Provisioned *bool `json:"provisioned,omitempty"`
}

// BareMetalClusterStatus defines the observed state of BareMetalCluster.
type BareMetalClusterStatus struct {
	// Ready indicates that the cluster infrastructure is ready.
	// +optional
	Ready bool `json:"ready,omitempty"`

	// Initialization provides observations of the BareMetalCluster initialization process.
	// +optional
	Initialization *BareMetalClusterInitializationStatus `json:"initialization,omitempty"`

	// Conditions defines current service state of the BareMetalCluster.
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`
}

// GetConditions returns the set of conditions for this object.
func (c *BareMetalCluster) GetConditions() clusterv1.Conditions {
	return c.Status.Conditions
}

// SetConditions sets the conditions on this object.
func (c *BareMetalCluster) SetConditions(conditions clusterv1.Conditions) {
	c.Status.Conditions = conditions
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.ready",description="BareMetalCluster is Ready"
// +kubebuilder:printcolumn:name="Endpoint",type="string",JSONPath=".spec.controlPlaneEndpoint.host",description="API endpoint host"
// +kubebuilder:printcolumn:name="Port",type="integer",JSONPath=".spec.controlPlaneEndpoint.port",description="API endpoint port"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of BareMetalCluster"

// BareMetalCluster is the Schema for the baremetalclusters API.
type BareMetalCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BareMetalClusterSpec   `json:"spec,omitempty"`
	Status BareMetalClusterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BareMetalClusterList contains a list of BareMetalCluster.
type BareMetalClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BareMetalCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BareMetalCluster{}, &BareMetalClusterList{})
}
