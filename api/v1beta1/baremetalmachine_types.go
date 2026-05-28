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
	// CredentialsNotFoundReason indicates credentials secret was not found.
	CredentialsNotFoundReason = "CredentialsNotFound"

	// SSHConnectionFailedReason indicates SSH connection failed.
	SSHConnectionFailedReason = "SSHConnectionFailed"

	// PreFlightChecksFailedReason indicates pre-flight checks failed.
	PreFlightChecksFailedReason = "PreFlightChecksFailed"

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

	// HostInventoryRef references the BareMetalHostInventory to allocate a host from.
	// +optional
	HostInventoryRef *corev1.LocalObjectReference `json:"hostInventoryRef,omitempty"`

	// HostName is the hostname of the bare metal machine.
	// This can be specified directly or allocated from HostInventory.
	// +optional
	HostName string `json:"hostName,omitempty"`

	// IPAddress is the IP address of the bare metal machine.
	// This can be specified directly or allocated from HostInventory.
	// +optional
	IPAddress string `json:"ipAddress,omitempty"`

	// SSHPort is the SSH port to connect to.
	// +optional
	// +kubebuilder:default=22
	SSHPort int `json:"sshPort,omitempty"`

	// CredentialsRef references the Secret containing SSH credentials.
	// This can be specified directly or allocated from HostInventory.
	// +optional
	CredentialsRef *corev1.LocalObjectReference `json:"credentialsRef,omitempty"`

	// PowerManagement holds optional power management configuration.
	// +optional
	PowerManagement *PowerManagementConfig `json:"powerManagement,omitempty"`

	// Role indicates the role of this machine (control-plane or worker).
	// Used to filter hosts from the inventory.
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
}

// ComponentInstallConfig defines configuration for automatic component installation.
type ComponentInstallConfig struct {
	// Enabled indicates whether automatic component installation is enabled.
	// +optional
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`

	// Strategy defines the installation strategy.
	// +optional
	// +kubebuilder:default=InstallIfMissing
	Strategy InstallStrategy `json:"strategy,omitempty"`

	// ContainerRuntime specifies the container runtime configuration.
	// +optional
	ContainerRuntime ContainerRuntimeConfig `json:"containerRuntime,omitempty"`

	// Kubernetes specifies the Kubernetes components configuration.
	// +optional
	Kubernetes KubernetesComponentsConfig `json:"kubernetes,omitempty"`

	// Timeout is the maximum time to wait for installation to complete.
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// AirGap defines configuration for offline/air-gapped installations.
	// +optional
	AirGap *AirGapConfig `json:"airGap,omitempty"`

	// RollbackOnError indicates whether to rollback on installation failure.
	// +optional
	RollbackOnError bool `json:"rollbackOnError,omitempty"`

	// MaxRetries is the maximum number of retries for installation.
	// +optional
	// +kubebuilder:default=3
	MaxRetries int `json:"maxRetries,omitempty"`
}

// InstallStrategy defines the component installation strategy.
type InstallStrategy string

const (
	// InstallIfMissing installs only if components are not present or version mismatch.
	InstallIfMissing InstallStrategy = "InstallIfMissing"
	// AlwaysInstall always reinstalls components.
	AlwaysInstall InstallStrategy = "AlwaysInstall"
	// Skip skips installation (assumes components are pre-installed).
	Skip InstallStrategy = "Skip"
)

// ContainerRuntimeConfig specifies the container runtime configuration.
type ContainerRuntimeConfig struct {
	// Type is the container runtime type (containerd, cri-o, docker).
	// +optional
	// +kubebuilder:default=containerd
	Type string `json:"type,omitempty"`

	// Version is the desired version of the container runtime.
	// +optional
	Version string `json:"version,omitempty"`

	// RegistryMirrors is a list of registry mirror URLs.
	// +optional
	RegistryMirrors []string `json:"registryMirrors,omitempty"`
}

// KubernetesComponentsConfig specifies the Kubernetes components configuration.
type KubernetesComponentsConfig struct {
	// Version is the desired Kubernetes version.
	// +optional
	Version string `json:"version,omitempty"`

	// Repository is the custom package repository configuration.
	// +optional
	Repository *PackageRepository `json:"repository,omitempty"`
}

// PackageRepository is the custom package repository configuration.
type PackageRepository struct {
	// BaseURL is the base URL of the package repository.
	// +optional
	BaseURL string `json:"baseUrl,omitempty"`

	// GPGKey is the URL to the GPG key for the repository.
	// +optional
	GPGKey string `json:"gpgKey,omitempty"`
}

// AirGapConfig defines configuration for offline/air-gapped installations.
type AirGapConfig struct {
	// Enabled indicates whether air-gapped installation mode is used.
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// BinarySource specifies how binaries are delivered in air-gapped mode.
	// Options: HTTPServer | ConfigMap | LocalPath
	// +optional
	// +kubebuilder:default=HTTPServer
	BinarySource string `json:"binarySource,omitempty"`

	// HTTPServerConfig is the configuration for HTTP binary source.
	// +optional
	HTTPServerConfig *HTTPServerConfig `json:"httpServerConfig,omitempty"`

	// LocalPath is the path on target machine where binaries are pre-placed.
	// +optional
	LocalPath string `json:"localPath,omitempty"`

	// PreloadImages is a list of container images to preload into containerd.
	// +optional
	PreloadImages []string `json:"preloadImages,omitempty"`
}

// HTTPServerConfig is the configuration for HTTP binary source.
type HTTPServerConfig struct {
	// BaseURL is the HTTP server URL serving binary packages.
	BaseURL string `json:"baseUrl"`

	// TLSSecretRef references a Secret containing TLS client certificate.
	// +optional
	TLSSecretRef *corev1.LocalObjectReference `json:"tlsSecretRef,omitempty"`

	// InsecureSkipVerify skips TLS verification (for internal CAs).
	// +optional
	InsecureSkipVerify bool `json:"insecureSkipVerify,omitempty"`
}

// FirewallConfig defines configuration for firewall management.
type FirewallConfig struct {
	// Configure indicates whether to automatically configure firewall rules.
	// +optional
	// +kubebuilder:default=true
	Configure bool `json:"configure,omitempty"`

	// AutoDetect indicates whether to auto-detect the firewall manager.
	// +optional
	// +kubebuilder:default=true
	AutoDetect bool `json:"autoDetect,omitempty"`

	// AdditionalPorts is a list of additional ports to open.
	// +optional
	AdditionalPorts []PortRule `json:"additionalPorts,omitempty"`
}

// PortRule defines a port to open in the firewall.
type PortRule struct {
	// Port is the port number.
	Port int `json:"port"`

	// Protocol is the protocol (tcp/udp).
	// +optional
	// +kubebuilder:default=tcp
	Protocol string `json:"protocol,omitempty"`

	// Description is a human-readable description of the port.
	// +optional
	Description string `json:"description,omitempty"`
}

// SELinuxConfig defines configuration for SELinux management.
type SELinuxConfig struct {
	// Configure indicates whether to automatically configure SELinux.
	// +optional
	// +kubebuilder:default=true
	Configure bool `json:"configure,omitempty"`
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

	// InstalledComponents tracks the versions of installed components.
	// +optional
	InstalledComponents ComponentVersions `json:"installedComponents,omitempty"`

	// Conditions defines current service state of the BareMetalMachine.
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`
}

// ComponentVersions tracks the versions of installed components.
type ComponentVersions struct {
	// ContainerRuntime is the installed container runtime version.
	// +optional
	ContainerRuntime string `json:"containerRuntime,omitempty"`

	// Kubeadm is the installed kubeadm version.
	// +optional
	Kubeadm string `json:"kubeadm,omitempty"`

	// Kubelet is the installed kubelet version.
	// +optional
	Kubelet string `json:"kubelet,omitempty"`

	// Kubectl is the installed kubectl version.
	// +optional
	Kubectl string `json:"kubectl,omitempty"`

	// OSType is the detected OS type.
	// +optional
	OSType string `json:"osType,omitempty"`

	// OSVersion is the detected OS version.
	// +optional
	OSVersion string `json:"osVersion,omitempty"`
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
