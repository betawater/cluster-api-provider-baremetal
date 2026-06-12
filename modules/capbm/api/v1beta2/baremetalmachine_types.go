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
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"

	cfov1 "github.com/BetaWater/cluster-api-provider-baremetal/modules/cvo/api/v1beta1"
)

const (
	// MachineFinalizer allows ReconcileBareMetalMachine to clean up resources associated with BareMetalMachine before
	// removing it from the apiserver.
	MachineFinalizer = "baremetalmachine.infrastructure.cluster.x-k8s.io"

	// MachineReadyCondition reports the current status of the machine.
	MachineReadyCondition = clusterv1.ReadyCondition

	// PreFlightChecksPassedCondition reports whether pre-flight checks have passed.
	//nolint:staticcheck // ConditionType deprecated in CAPI v1beta2, will migrate when ready
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

	// UseSudo indicates whether to use sudo for privileged operations.
	// When true, all installation scripts will be executed with sudo.
	// +optional
	UseSudo bool `json:"useSudo,omitempty"`

	// PowerManagement holds optional power management configuration.
	// +optional
	PowerManagement *PowerManagementConfig `json:"powerManagement,omitempty"`

	// Role indicates the role of this machine (control-plane or worker).
	// Used to filter hosts from the inventory.
	// +optional
	Role string `json:"role,omitempty"`

	// ReleaseImageRef references the ReleaseImage for component versions.
	// When set, component versions are sourced from the ReleaseImage.
	// +optional
	ReleaseImageRef *corev1.LocalObjectReference `json:"releaseImageRef,omitempty"`

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
	// When enabled, CAPBM will automatically configure the node
	// after pre-flight checks pass.
	// +optional
	NodeBootstrap *NodeBootstrapConfig `json:"nodeBootstrap,omitempty"`
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

	// CNI defines the CNI plugin installation configuration.
	// +optional
	CNI CNIConfig `json:"cni,omitempty"`

	// CSI defines the CSI driver installation configuration.
	// +optional
	CSI CSIConfig `json:"csi,omitempty"`
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

	// RegistryMirrors is a list of registry mirror URLs (legacy, use Config instead).
	// +optional
	RegistryMirrors []string `json:"registryMirrors,omitempty"`

	// Config holds runtime-specific configuration.
	// Applied during installation/upgrade.
	// +optional
	Config *RuntimeConfig `json:"config,omitempty"`
}

// RuntimeConfig holds container runtime configuration.
type RuntimeConfig struct {
	// SystemdCgroup enables systemd cgroup driver.
	// +optional
	SystemdCgroup *bool `json:"systemdCgroup,omitempty"`

	// SandboxImage is the pause/sandbox image.
	// +optional
	SandboxImage string `json:"sandboxImage,omitempty"`

	// RegistryMirrors declares registry mirror configuration.
	// +optional
	RegistryMirrors []RegistryMirrorEntry `json:"registryMirrors,omitempty"`

	// MaxConcurrentDownloads sets max concurrent downloads.
	// +optional
	MaxConcurrentDownloads *int `json:"maxConcurrentDownloads,omitempty"`

	// RawConfig is raw TOML configuration appended to the final config.
	// +optional
	RawConfig string `json:"rawConfig,omitempty"`
}

// RegistryMirrorEntry declares a registry mirror configuration.
type RegistryMirrorEntry struct {
	// Host is the registry host to mirror.
	Host string `json:"host"`
	// Endpoints is the list of mirror endpoints.
	Endpoints []string `json:"endpoints"`
}

// KubernetesComponentsConfig specifies the Kubernetes components configuration.
type KubernetesComponentsConfig struct {
	// Version is the desired Kubernetes version.
	// +optional
	Version string `json:"version,omitempty"`

	// Repository is the custom package repository configuration.
	// +optional
	Repository *PackageRepository `json:"repository,omitempty"`

	// Config holds kubernetes component configuration.
	// Applied during installation/upgrade.
	// +optional
	Config *KubernetesConfig `json:"config,omitempty"`
}

// KubernetesConfig holds kubernetes component configuration.
type KubernetesConfig struct {
	// Kubelet holds kubelet-specific configuration.
	// +optional
	Kubelet *KubeletConfig `json:"kubelet,omitempty"`
}

// KubeletConfig holds kubelet-specific configuration.
type KubeletConfig struct {
	// CgroupDriver sets the cgroup driver (cgroupfs or systemd).
	// +optional
	// +kubebuilder:default=systemd
	CgroupDriver string `json:"cgroupDriver,omitempty"`

	// MaxPods sets the maximum number of pods.
	// +optional
	MaxPods *int `json:"maxPods,omitempty"`

	// ExtraArgs declares additional kubelet command-line arguments.
	// +optional
	ExtraArgs map[string]string `json:"extraArgs,omitempty"`

	// RawConfig is raw kubelet configuration (YAML format).
	// +optional
	RawConfig string `json:"rawConfig,omitempty"`
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
	HTTPServerConfig *cfov1.HTTPServerConfig `json:"httpServerConfig,omitempty"`

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

// NodeBootstrapConfig defines configuration for node bootstrapping.
type NodeBootstrapConfig struct {
	// Enabled indicates whether node bootstrapping is enabled.
	// +optional
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`

	// Hostname is the desired hostname for the node.
	// If empty, uses the hostName from spec.
	// +optional
	Hostname string `json:"hostname,omitempty"`

	// HostsEntries is a list of additional /etc/hosts entries.
	// +optional
	HostsEntries []HostsEntry `json:"hostsEntries,omitempty"`

	// DisableSwap indicates whether to disable swap.
	// +optional
	// +kubebuilder:default=true
	DisableSwap bool `json:"disableSwap,omitempty"`

	// KernelModules is a list of kernel modules to load.
	// +optional
	KernelModules []string `json:"kernelModules,omitempty"`

	// SysctlParams is a list of sysctl parameters to configure.
	// +optional
	SysctlParams map[string]string `json:"sysctlParams,omitempty"`

	// TimeSync indicates whether to configure time synchronization.
	// +optional
	// +kubebuilder:default=true
	TimeSync bool `json:"timeSync,omitempty"`
}

// HostsEntry defines a /etc/hosts entry.
type HostsEntry struct {
	// IP is the IP address.
	IP string `json:"ip"`
	// Hostnames is the list of hostnames for this IP.
	Hostnames []string `json:"hostnames"`
}

// CNIConfig defines the CNI plugin installation configuration.
type CNIConfig struct {
	// Enabled indicates whether CNI installation is enabled.
	// +optional
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`

	// Type is the CNI plugin type (calico, cilium, flannel).
	// +optional
	// +kubebuilder:default=calico
	Type string `json:"type,omitempty"`

	// Version is the desired CNI plugin version.
	// +optional
	Version string `json:"version,omitempty"`

	// InstallMode defines how the CNI plugin is installed (Manifest or Helm).
	// +optional
	// +kubebuilder:default=Manifest
	InstallMode string `json:"installMode,omitempty"`

	// UpgradeStrategy defines the upgrade strategy for CNI plugin.
	// +optional
	// +kubebuilder:default=RollingUpdate
	UpgradeStrategy string `json:"upgradeStrategy,omitempty"`

	// Config holds CNI-specific configuration.
	// +optional
	Config *CNIPluginConfig `json:"config,omitempty"`

	// AirGap defines air-gapped installation configuration for CNI.
	// +optional
	AirGap *CNIAirGapConfig `json:"airGap,omitempty"`
}

// CNIPluginConfig holds CNI plugin specific configuration.
type CNIPluginConfig struct {
	// PodCIDR is the Pod network CIDR (auto-detected from Cluster if empty).
	// +optional
	PodCIDR string `json:"podCIDR,omitempty"`

	// Calico holds Calico-specific configuration.
	// +optional
	Calico *CalicoConfig `json:"calico,omitempty"`

	// Cilium holds Cilium-specific configuration.
	// +optional
	Cilium *CiliumConfig `json:"cilium,omitempty"`

	// Flannel holds Flannel-specific configuration.
	// +optional
	Flannel *FlannelConfig `json:"flannel,omitempty"`
}

// CalicoConfig defines Calico CNI configuration.
type CalicoConfig struct {
	// IPAM is the IP address management mode (CalicoIPAM or HostLocal).
	// +optional
	// +kubebuilder:default=CalicoIPAM
	IPAM string `json:"ipam,omitempty"`

	// MTU is the network MTU (0 = auto-detect).
	// +optional
	MTU int `json:"mtu,omitempty"`

	// BGP holds BGP peering configuration.
	// +optional
	BGP *CalicoBGPConfig `json:"bgp,omitempty"`

	// Typha holds Typha deployment configuration.
	// +optional
	Typha *CalicoTyphaConfig `json:"typha,omitempty"`
}

// CalicoBGPConfig defines Calico BGP configuration.
type CalicoBGPConfig struct {
	// Enabled enables BGP peering.
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// PeerIPs is the list of BGP peer IPs.
	// +optional
	PeerIPs []string `json:"peerIPs,omitempty"`
}

// CalicoTyphaConfig defines Calico Typha configuration.
type CalicoTyphaConfig struct {
	// Enabled enables Typha deployment.
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// Replicas is the number of Typha replicas.
	// +optional
	Replicas int `json:"replicas,omitempty"`
}

// CiliumConfig defines Cilium CNI configuration.
type CiliumConfig struct {
	// KubeProxyReplacement enables kube-proxy replacement.
	// +optional
	// +kubebuilder:default=partial
	KubeProxyReplacement string `json:"kubeProxyReplacement,omitempty"`

	// RoutingMode is the routing mode (native or tunnel).
	// +optional
	// +kubebuilder:default=tunnel
	RoutingMode string `json:"routingMode,omitempty"`

	// IPv4NativeRoutingCIDR is the native routing CIDR.
	// +optional
	IPv4NativeRoutingCIDR string `json:"ipv4NativeRoutingCIDR,omitempty"`

	// Hubble holds Hubble observability configuration.
	// +optional
	Hubble *CiliumHubbleConfig `json:"hubble,omitempty"`
}

// CiliumHubbleConfig defines Cilium Hubble configuration.
type CiliumHubbleConfig struct {
	// Enabled enables Hubble observability.
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// Relay enables Hubble Relay.
	// +optional
	Relay bool `json:"relay,omitempty"`

	// UI enables Hubble UI.
	// +optional
	UI bool `json:"ui,omitempty"`
}

// FlannelConfig defines Flannel CNI configuration.
type FlannelConfig struct {
	// Backend is the backend type (vxlan, host-gw, wireguard).
	// +optional
	// +kubebuilder:default=vxlan
	Backend string `json:"backend,omitempty"`

	// MTU is the network MTU (0 = auto-detect).
	// +optional
	MTU int `json:"mtu,omitempty"`
}

// CNIAirGapConfig defines air-gapped installation configuration for CNI.
type CNIAirGapConfig struct {
	// Enabled indicates whether air-gapped installation is used.
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// ManifestSource is the source for manifest files (HTTPServer or LocalPath).
	// +optional
	// +kubebuilder:default=HTTPServer
	ManifestSource string `json:"manifestSource,omitempty"`

	// HTTPServerConfig is the HTTP server configuration.
	// +optional
	HTTPServerConfig *cfov1.HTTPServerConfig `json:"httpServerConfig,omitempty"`

	// LocalPath is the local path for manifest files.
	// +optional
	LocalPath string `json:"localPath,omitempty"`

	// ChartArchive is the path to the Helm chart archive (for Helm mode).
	// +optional
	ChartArchive string `json:"chartArchive,omitempty"`

	// CNIPluginsArchive is the path to the CNI plugins binary archive.
	// +optional
	CNIPluginsArchive string `json:"cniPluginsArchive,omitempty"`
}

// CSIConfig defines the CSI driver installation configuration.
type CSIConfig struct {
	// Enabled indicates whether CSI installation is enabled.
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// Driver is the CSI driver type (ceph-csi, cinder-csi, local-csi, nfs-csi).
	// +optional
	Driver string `json:"driver,omitempty"`

	// Version is the desired CSI driver version.
	// +optional
	Version string `json:"version,omitempty"`

	// InstallMode defines how the CSI driver is installed (Manifest or Helm).
	// +optional
	// +kubebuilder:default=Helm
	InstallMode string `json:"installMode,omitempty"`

	// Config holds CSI driver specific configuration.
	// +optional
	Config *CSIDriverConfig `json:"config,omitempty"`

	// AirGap defines air-gapped installation configuration for CSI.
	// +optional
	AirGap *CSIAirGapConfig `json:"airGap,omitempty"`
}

// CSIDriverConfig holds CSI driver specific configuration.
type CSIDriverConfig struct {
	// CephCsi holds Ceph-CSI configuration.
	// +optional
	CephCsi *CephCsiConfig `json:"cephCsi,omitempty"`

	// CinderCsi holds Cinder-CSI configuration.
	// +optional
	CinderCsi *CinderCsiConfig `json:"cinderCsi,omitempty"`

	// LocalCsi holds Local-CSI configuration.
	// +optional
	LocalCsi *LocalCsiConfig `json:"localCsi,omitempty"`

	// NfsCsi holds NFS-CSI configuration.
	// +optional
	NfsCsi *NfsCsiConfig `json:"nfsCsi,omitempty"`
}

// CephCsiConfig defines Ceph-CSI configuration.
type CephCsiConfig struct {
	// ClusterID is the Ceph cluster ID.
	// +required
	ClusterID string `json:"clusterID"`

	// Monitors is the list of Ceph monitor endpoints.
	// +required
	Monitors []string `json:"monitors"`

	// CephFS holds CephFS configuration.
	// +optional
	CephFS *CephFSConfig `json:"cephfs,omitempty"`

	// RBD holds RBD configuration.
	// +optional
	RBD *RBDConfig `json:"rbd,omitempty"`

	// StorageClass defines the StorageClass to create.
	// +optional
	StorageClass *CSIStorageClass `json:"storageClass,omitempty"`
}

// CephFSConfig defines CephFS configuration.
type CephFSConfig struct {
	// Enabled enables CephFS support.
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// KernelMount enables kernel mount.
	// +optional
	KernelMount bool `json:"kernelMount,omitempty"`

	// FuseMount enables FUSE mount.
	// +optional
	FuseMount bool `json:"fuseMount,omitempty"`
}

// RBDConfig defines RBD configuration.
type RBDConfig struct {
	// Enabled enables RBD support.
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// Pool is the Ceph RBD pool name.
	// +optional
	Pool string `json:"pool,omitempty"`
}

// CinderCsiConfig defines Cinder-CSI configuration.
type CinderCsiConfig struct {
	// OpenstackCloudConfigSecret references the Secret containing cloud-config.
	// +required
	OpenstackCloudConfigSecret string `json:"openstackCloudConfigSecret"`

	// StorageClass defines the StorageClass to create.
	// +optional
	StorageClass *CSIStorageClass `json:"storageClass,omitempty"`
}

// LocalCsiConfig defines Local-CSI (hostPath) configuration.
type LocalCsiConfig struct {
	// StorageClass defines the StorageClass to create.
	// +optional
	StorageClass *CSIStorageClass `json:"storageClass,omitempty"`
}

// NfsCsiConfig defines NFS-CSI configuration.
type NfsCsiConfig struct {
	// Server is the NFS server address.
	// +required
	Server string `json:"server"`

	// Share is the NFS export path.
	// +required
	Share string `json:"share"`

	// StorageClass defines the StorageClass to create.
	// +optional
	StorageClass *CSIStorageClass `json:"storageClass,omitempty"`
}

// CSIStorageClass defines a StorageClass configuration.
type CSIStorageClass struct {
	// Name is the StorageClass name.
	// +required
	Name string `json:"name"`

	// ReclaimPolicy is the reclaim policy (Delete or Retain).
	// +optional
	// +kubebuilder:default=Delete
	ReclaimPolicy string `json:"reclaimPolicy,omitempty"`

	// FSType is the filesystem type.
	// +optional
	FSType string `json:"fsType,omitempty"`

	// VolumeBindingMode is the volume binding mode.
	// +optional
	VolumeBindingMode string `json:"volumeBindingMode,omitempty"`

	// MountOptions is the list of mount options.
	// +optional
	MountOptions []string `json:"mountOptions,omitempty"`

	// Parameters is the storage class parameters.
	// +optional
	Parameters map[string]string `json:"parameters,omitempty"`
}

// CSIAirGapConfig defines air-gapped installation configuration for CSI.
type CSIAirGapConfig struct {
	// Enabled indicates whether air-gapped installation is used.
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// ManifestSource is the source for manifest files.
	// +optional
	ManifestSource string `json:"manifestSource,omitempty"`

	// HTTPServerConfig is the HTTP server configuration.
	// +optional
	HTTPServerConfig *cfov1.HTTPServerConfig `json:"httpServerConfig,omitempty"`

	// LocalPath is the local path for manifest files.
	// +optional
	LocalPath string `json:"localPath,omitempty"`

	// ChartArchive is the path to the Helm chart archive.
	// +optional
	ChartArchive string `json:"chartArchive,omitempty"`
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
	//nolint:staticcheck // Conditions deprecated in CAPI v1beta2, will migrate when ready
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

	// CNI is the installed CNI plugin version.
	// +optional
	CNI string `json:"cni,omitempty"`

	// CNIType is the installed CNI plugin type.
	// +optional
	CNIType string `json:"cniType,omitempty"`

	// CSI is the installed CSI driver version.
	// +optional
	CSI string `json:"csi,omitempty"`

	// CSIDriver is the installed CSI driver type.
	// +optional
	CSIDriver string `json:"csiDriver,omitempty"`

	// OSType is the detected OS type.
	// +optional
	OSType string `json:"osType,omitempty"`

	// OSVersion is the detected OS version.
	// +optional
	OSVersion string `json:"osVersion,omitempty"`
}

// GetConditions returns the set of conditions for this object.
//nolint:staticcheck // Conditions deprecated in CAPI v1beta2, will migrate when ready
func (m *BareMetalMachine) GetConditions() clusterv1.Conditions {
	return m.Status.Conditions
}

// SetConditions sets the conditions on this object.
//nolint:staticcheck // Conditions deprecated in CAPI v1beta2, will migrate when ready
func (m *BareMetalMachine) SetConditions(conditions clusterv1.Conditions) {
	m.Status.Conditions = conditions
}

// +kubebuilder:metadata:labels="cluster.x-k8s.io/v1beta2=v1beta2"
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
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
