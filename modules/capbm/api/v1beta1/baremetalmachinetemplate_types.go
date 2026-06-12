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
)

// BareMetalMachineTemplateSpec defines the desired state of BareMetalMachineTemplate.
type BareMetalMachineTemplateSpec struct {
	// Template defines the template for BareMetalMachine.
	Template BareMetalMachineTemplateResource `json:"template"`
}

// BareMetalMachineTemplateResource defines the template resource.
type BareMetalMachineTemplateResource struct {
	// Standard object's metadata.
	// +optional
	ObjectMeta ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the desired state of BareMetalMachine.
	Spec BareMetalMachineTemplateSpecInner `json:"spec"`
}

// BareMetalMachineTemplateSpecInner defines the spec for the template.
type BareMetalMachineTemplateSpecInner struct {
	// SSHPort is the SSH port to connect to.
	// +optional
	// +kubebuilder:default=22
	SSHPort int `json:"sshPort,omitempty"`

	// HostInventoryRef references the BareMetalHostInventory to allocate a host from.
	// +optional
	HostInventoryRef *corev1.LocalObjectReference `json:"hostInventoryRef,omitempty"`

	// CredentialsRef references the Secret containing SSH credentials.
	// +optional
	CredentialsRef *corev1.LocalObjectReference `json:"credentialsRef,omitempty"`

	// PowerManagement holds optional power management configuration.
	// +optional
	PowerManagement *PowerManagementConfig `json:"powerManagement,omitempty"`

	// Role indicates the role of this machine (control-plane or worker).
	// +optional
	Role string `json:"role,omitempty"`

	// ComponentInstall holds configuration for automatic component installation.
	// +optional
	ComponentInstall *ComponentInstallConfig `json:"componentInstall,omitempty"`

	// Firewall holds configuration for firewall management.
	// +optional
	Firewall *FirewallConfig `json:"firewall,omitempty"`

	// SELinux holds configuration for SELinux management.
	// +optional
	SELinux *SELinuxConfig `json:"selinux,omitempty"`

	// NodeBootstrap holds configuration for node bootstrapping.
	// +optional
	NodeBootstrap *NodeBootstrapConfig `json:"nodeBootstrap,omitempty"`
}

// +kubebuilder:metadata:labels="cluster.x-k8s.io/v1beta1=v1beta1"
// +kubebuilder:metadata:labels="cluster.x-k8s.io/v1beta2=v1beta2"
// +kubebuilder:object:root=true

// BareMetalMachineTemplate is the Schema for the baremetalmachinetemplates API.
type BareMetalMachineTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec BareMetalMachineTemplateSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// BareMetalMachineTemplateList contains a list of BareMetalMachineTemplate.
type BareMetalMachineTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BareMetalMachineTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BareMetalMachineTemplate{}, &BareMetalMachineTemplateList{})
}
