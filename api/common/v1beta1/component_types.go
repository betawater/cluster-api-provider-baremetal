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

// ComponentType represents the installation type of a component.
type ComponentType string

const (
	ComponentTypeBinary   ComponentType = "binary"
	ComponentTypeManifest ComponentType = "manifest"
	ComponentTypeHelm     ComponentType = "helm"
)

// BinaryComponent defines a binary component with multi-arch support.
type BinaryComponent struct {
	Version       string        `json:"version"`
	Type          ComponentType `json:"type"`
	Path          string        `json:"path"`
	Architectures []string      `json:"architectures"`
	Files         BinaryFiles   `json:"files,omitempty"`

	// InstallStrategy defines installation behavior for binary components.
	// +optional
	InstallStrategy *BinaryInstallStrategy `json:"installStrategy,omitempty"`

	// UpgradeStrategy defines upgrade behavior for binary components.
	// +optional
	UpgradeStrategy *BinaryUpgradeStrategy `json:"upgradeStrategy,omitempty"`

	// PreHooks are scripts/commands to run before install/upgrade.
	// +optional
	PreHooks []AddonHook `json:"preHooks,omitempty"`

	// PostHooks are scripts/commands to run after install/upgrade.
	// +optional
	PostHooks []AddonHook `json:"postHooks,omitempty"`

	// Upgrade defines component-level upgrade configuration (high cohesion).
	// +optional
	Upgrade *ComponentUpgradeConfig `json:"upgrade,omitempty"`
}

// BinaryFiles defines binary component file names.
type BinaryFiles struct {
	Archive string `json:"archive"`
}

// KubernetesComponent defines Kubernetes binaries with OS-specific packages.
type KubernetesComponent struct {
	Version   string                 `json:"version"`
	Type      ComponentType          `json:"type"`
	Path      string                 `json:"path"`
	Platforms map[string]K8SPlatform `json:"platforms"`
	ImageList []string               `json:"imageList,omitempty"`

	// InstallStrategy defines installation behavior for binary components.
	// +optional
	InstallStrategy *BinaryInstallStrategy `json:"installStrategy,omitempty"`

	// UpgradeStrategy defines upgrade behavior for binary components.
	// +optional
	UpgradeStrategy *BinaryUpgradeStrategy `json:"upgradeStrategy,omitempty"`

	// PreHooks are scripts/commands to run before install/upgrade.
	// +optional
	PreHooks []AddonHook `json:"preHooks,omitempty"`

	// PostHooks are scripts/commands to run after install/upgrade.
	// +optional
	PostHooks []AddonHook `json:"postHooks,omitempty"`

	// Upgrade defines component-level upgrade configuration (high cohesion).
	// +optional
	Upgrade *ComponentUpgradeConfig `json:"upgrade,omitempty"`
}

// K8SPlatform defines OS-specific package configuration.
type K8SPlatform struct {
	Architectures []string          `json:"architectures"`
	Packages      map[string]string `json:"packages"`
}

// BinaryInstallStrategy defines installation behavior for binary components.
type BinaryInstallStrategy struct {
	// Timeout is the maximum time allowed for installation.
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// RetryCount is the number of retries on failure.
	// +optional
	// +kubebuilder:default=3
	RetryCount int `json:"retryCount,omitempty"`

	// Method is the installation method: package, archive, or manual.
	// +kubebuilder:validation:Enum=package;archive;manual
	// +optional
	Method string `json:"method,omitempty"`

	// ServiceName is the service name to restart after installation.
	// +optional
	ServiceName string `json:"serviceName,omitempty"`
}

// BinaryUpgradeStrategy defines upgrade behavior for binary components.
type BinaryUpgradeStrategy struct {
	// Type is the upgrade type: Rolling, DrainAndUpgrade, or Parallel.
	// +kubebuilder:validation:Enum=Rolling;DrainAndUpgrade;Parallel
	// +kubebuilder:default=Rolling
	Type string `json:"type"`

	// MaxConcurrent is the maximum number of nodes to upgrade concurrently.
	// +optional
	// +kubebuilder:default=1
	MaxConcurrent int `json:"maxConcurrent,omitempty"`

	// Timeout is the maximum time allowed for upgrade per node.
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// RetryCount is the number of retries on failure.
	// +optional
	// +kubebuilder:default=3
	RetryCount int `json:"retryCount,omitempty"`

	// Force indicates whether to force upgrade even if version path is not recommended.
	// +optional
	Force bool `json:"force,omitempty"`

	// Drain indicates whether to drain node before upgrade.
	// +optional
	Drain bool `json:"drain,omitempty"`
}

// ReleaseComponentVersions defines node-level binary component versions.
// These are installed directly on nodes via SSH.
type ReleaseComponentVersions struct {
	Kubernetes KubernetesComponent `json:"kubernetes"`
	Containerd BinaryComponent     `json:"containerd,omitempty"`
	Helm       BinaryComponent     `json:"helm,omitempty"`
	CNIPlugins BinaryComponent     `json:"cniPlugins,omitempty"`
}

// HTTPServerConfig defines the HTTP server configuration for serving release content.
type HTTPServerConfig struct {
	// Enabled enables the HTTP server.
	// +optional
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`

	// Port is the HTTP server port.
	// +optional
	// +kubebuilder:default=8080
	Port int `json:"port,omitempty"`

	// BasePath is the base path for serving content.
	// +optional
	BasePath string `json:"basePath,omitempty"`

	// BaseURL is the HTTP server URL serving binary packages.
	// +optional
	BaseURL string `json:"baseUrl,omitempty"`

	// TLSSecretRef references a Secret containing TLS client certificate.
	// +optional
	TLSSecretRef string `json:"tlsSecretRef,omitempty"`

	// InsecureSkipVerify skips TLS verification (for internal CAs).
	// +optional
	InsecureSkipVerify bool `json:"insecureSkipVerify,omitempty"`
}

// ImageRegistryConfig defines the target image registry configuration.
type ImageRegistryConfig struct {
	// Enabled enables image registry import.
	// +optional
	// +kubebuilder:default=false
	Enabled bool `json:"enabled,omitempty"`

	// Registry is the target registry URL (e.g., registry.example.com).
	// +optional
	Registry string `json:"registry,omitempty"`

	// Repository is the repository path prefix (e.g., capbm).
	// +optional
	// +kubebuilder:default=capbm
	Repository string `json:"repository,omitempty"`

	// CredentialsSecret is the secret name containing registry credentials.
	// +optional
	CredentialsSecret string `json:"credentialsSecret,omitempty"`

	// InsecureSkipVerify skips TLS verification for the registry.
	// +optional
	InsecureSkipVerify bool `json:"insecureSkipVerify,omitempty"`

	// ImagePrefix is the prefix for image names.
	// +optional
	ImagePrefix string `json:"imagePrefix,omitempty"`

	// CAConfigMap is the ConfigMap name containing the registry CA certificate.
	// +optional
	CAConfigMap string `json:"caConfigMap,omitempty"`
}
