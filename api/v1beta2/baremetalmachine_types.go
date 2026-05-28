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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta2"
)

const (
	// MachineFinalizer allows ReconcileBareMetalMachine to clean up resources associated with BareMetalMachine before
	// removing it from the apiserver.
	MachineFinalizer = "baremetalmachine.infrastructure.cluster.x-k8s.io"

	// MachineReadyCondition reports the current status of the machine.
	MachineReadyCondition = clusterv1.ReadyCondition

	// PreFlightChecksPassedCondition reports whether pre-flight checks have passed.
	PreFlightChecksPassedCondition clusterv1.ConditionType = "PreFlightChecksPassed"
)

// Reasons for conditions
const (
	// EndpointNotSetReason indicates the control plane endpoint is not set.
	EndpointNotSetReason = "EndpointNotSet"

	// CredentialsNotFoundReason indicates credentials secret was not found.
	CredentialsNotFoundReason = "CredentialsNotFound"

	// SSHConnectionFailedReason indicates SSH connection failed.
	SSHConnectionFailedReason = "SSHConnectionFailed"

	// PreFlightChecksFailedReason indicates pre-flight checks failed.
	PreFlightChecksFailedReason = "PreFlightChecksFailed"

	// ClusterReadyReason indicates the cluster is ready.
	ClusterReadyReason = "ClusterReady"

	// SSHConnectedReason indicates SSH connection is established.
	SSHConnectedReason = "SSHConnected"

	// ChecksPassedReason indicates all checks passed.
	ChecksPassedReason = "ChecksPassed"
)

// BareMetalMachineSpec defines the desired state of BareMetalMachine.
type BareMetalMachineSpec struct {
	// ProviderID will be the machine name in ProviderID format (baremetal://<hostname>).
	// +optional
	ProviderID *string `json:"providerID,omitempty"`

	// HostName is the hostname of the bare metal machine.
	HostName string `json:"hostName"`

	// IPAddress is the IP address of the bare metal machine.
	IPAddress string `json:"ipAddress"`

	// SSHPort is the SSH port to connect to.
	// +optional
	// +kubebuilder:default=22
	SSHPort int `json:"sshPort,omitempty"`

	// CredentialsRef references the Secret containing SSH credentials.
	CredentialsRef corev1.LocalObjectReference `json:"credentialsRef"`

	// PowerManagement holds optional power management configuration.
	// +optional
	PowerManagement *PowerManagementConfig `json:"powerManagement,omitempty"`

	// Role indicates the role of this machine (control-plane or worker).
	// +optional
	Role string `json:"role,omitempty"`
}

// PowerManagementConfig holds power management configuration.
type PowerManagementConfig struct {
	// Type is the power management protocol type (ipmi, redfish).
	Type string `json:"type"`

	// Address is the BMC address.
	Address string `json:"address"`

	// CredentialsRef references the Secret containing BMC credentials.
	CredentialsRef corev1.LocalObjectReference `json:"credentialsRef"`
}

// BareMetalMachineStatus defines the observed state of BareMetalMachine.
type BareMetalMachineStatus struct {
	// Ready indicates that the machine is ready.
	// +optional
	Ready bool `json:"ready,omitempty"`

	// ProviderID is the unique identifier for this machine.
	// +optional
	ProviderID string `json:"providerID,omitempty"`

	// Addresses contains the addresses associated with this machine.
	// +optional
	Addresses []clusterv1.MachineAddress `json:"addresses,omitempty"`

	// Conditions defines current service state of the BareMetalMachine.
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`
}

// GetConditions returns the set of conditions for this object.
func (m *BareMetalMachine) GetConditions() clusterv1.Conditions {
	return m.Status.Conditions
}

// SetConditions sets the conditions on this object.
func (m *BareMetalMachine) SetConditions(conditions clusterv1.Conditions) {
	m.Status.Conditions = conditions
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".metadata.labels.cluster\\.x-k8s\\.io/cluster-name",description="Cluster"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.ready",description="Machine is ready"
// +kubebuilder:printcolumn:name="ProviderID",type="string",JSONPath=".status.providerID",description="Provider ID"
// +kubebuilder:printcolumn:name="IPAddress",type="string",JSONPath=".spec.ipAddress",description="IP Address"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation"

// BareMetalMachine is the Schema for the baremetalmachines API.
type BareMetalMachine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BareMetalMachineSpec   `json:"spec,omitempty"`
	Status BareMetalMachineStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BareMetalMachineList contains a list of BareMetalMachine.
type BareMetalMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BareMetalMachine `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BareMetalMachine{}, &BareMetalMachineList{})
}
