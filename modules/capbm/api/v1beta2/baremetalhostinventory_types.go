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
)

// HostState represents the state of a host in the inventory.
type HostState string

const (
	// HostStateAvailable indicates the host is available for allocation.
	HostStateAvailable HostState = "Available"
	// HostStateAllocated indicates the host is allocated to a cluster.
	HostStateAllocated HostState = "Allocated"
	// HostStateMaintenance indicates the host is under maintenance.
	HostStateMaintenance HostState = "Maintenance"
)

// HostEntry defines a single bare metal host in the inventory.
type HostEntry struct {
	// Name is the unique identifier for this host entry.
	Name string `json:"name"`

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

	// Role indicates the role of this host (control-plane or worker).
	// +optional
	Role string `json:"role,omitempty"`

	// Labels are user-defined labels for the host.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
}

// HostStatusEntry defines the status of a single host.
type HostStatusEntry struct {
	// Name is the name of the host.
	Name string `json:"name"`

	// State is the current state of the host.
	State HostState `json:"state"`

	// ClusterRef references the cluster that has allocated this host.
	// +optional
	ClusterRef *corev1.ObjectReference `json:"clusterRef,omitempty"`
}

// BareMetalHostInventorySpec defines the desired state of BareMetalHostInventory.
type BareMetalHostInventorySpec struct {
	// Hosts is the list of bare metal hosts in this inventory.
	Hosts []HostEntry `json:"hosts"`
}

// BareMetalHostInventoryStatus defines the observed state of BareMetalHostInventory.
type BareMetalHostInventoryStatus struct {
	// TotalHosts is the total number of hosts in the inventory.
	TotalHosts int `json:"totalHosts"`

	// AvailableHosts is the number of available hosts.
	AvailableHosts int `json:"availableHosts"`

	// AllocatedHosts is the number of allocated hosts.
	AllocatedHosts int `json:"allocatedHosts"`

	// HostsStatus contains the status of each host.
	// +optional
	HostsStatus []HostStatusEntry `json:"hostsStatus,omitempty"`
}

// +kubebuilder:metadata:labels="cluster.x-k8s.io/v1beta2=v1beta2"
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Total",type="integer",JSONPath=".status.totalHosts",description="Total hosts"
// +kubebuilder:printcolumn:name="Available",type="integer",JSONPath=".status.availableHosts",description="Available hosts"
// +kubebuilder:printcolumn:name="Allocated",type="integer",JSONPath=".status.allocatedHosts",description="Allocated hosts"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation"

// BareMetalHostInventory is the Schema for the baremetalhostinventories API.
type BareMetalHostInventory struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BareMetalHostInventorySpec   `json:"spec,omitempty"`
	Status BareMetalHostInventoryStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BareMetalHostInventoryList contains a list of BareMetalHostInventory.
type BareMetalHostInventoryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BareMetalHostInventory `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BareMetalHostInventory{}, &BareMetalHostInventoryList{})
}
