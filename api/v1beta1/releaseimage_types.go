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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ReleaseImage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ReleaseImageSpec   `json:"spec,omitempty"`
	Status ReleaseImageStatus `json:"status,omitempty"`
}

type ReleaseImageSpec struct {
	Version          string                   `json:"version"`
	Image            string                   `json:"image"`
	HTTPServer       *HTTPServerConfig        `json:"httpServer,omitempty"`
	ImageRegistry    *ImageRegistryConfig     `json:"imageRegistry,omitempty"`
	Channels         []string                 `json:"channels,omitempty"`
	PreviousVersions []string                 `json:"previousVersions,omitempty"`
	Components       ReleaseComponentVersions `json:"components"`
	Addons           []AddonDefinition        `json:"addons,omitempty"`
	UpgradeGraph     []UpgradePhase           `json:"upgradeGraph"`
	ContentHash      string                   `json:"contentHash,omitempty"`
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
	// Secret type: Opaque with keys: username, password
	// +optional
	CredentialsSecret string `json:"credentialsSecret,omitempty"`

	// InsecureSkipVerify skips TLS verification for the registry.
	// +optional
	InsecureSkipVerify bool `json:"insecureSkipVerify,omitempty"`

	// ImagePrefix is the prefix for image names.
	// Full image: {registry}/{repository}/{imagePrefix}/{component}:{version}
	// +optional
	ImagePrefix string `json:"imagePrefix,omitempty"`

	// CAConfigMap is the ConfigMap name containing the registry CA certificate.
	// +optional
	CAConfigMap string `json:"caConfigMap,omitempty"`
}

// ReleaseComponentVersions defines all component versions with installation metadata.
type ReleaseComponentVersions struct {
	Kubernetes   KubernetesComponent   `json:"kubernetes"`
	Containerd   BinaryComponent       `json:"containerd,omitempty"`
	Helm         BinaryComponent       `json:"helm,omitempty"`
	CNIPlugins   BinaryComponent       `json:"cniPlugins,omitempty"`
	Calico       CNIComponent          `json:"calico,omitempty"`
	Cilium       CNIComponent          `json:"cilium,omitempty"`
	Flannel      CNIComponent          `json:"flannel,omitempty"`
	CephCsi      CSIComponent          `json:"cephCsi,omitempty"`
	LocalPath    CSIComponent          `json:"localPath,omitempty"`
	NfsCsi       CSIComponent          `json:"nfsCsi,omitempty"`
	GatewayAPI   ManifestComponent     `json:"gatewayAPI,omitempty"`
	EnvoyGateway ManifestComponent     `json:"envoyGateway,omitempty"`
	MetalLB      ManifestComponent     `json:"metalLB,omitempty"`
}

// ComponentType represents the installation type of a component.
type ComponentType string

const (
	ComponentTypeBinary   ComponentType = "binary"
	ComponentTypeManifest ComponentType = "manifest"
	ComponentTypeHelm     ComponentType = "helm"
)

// BinaryComponent defines a binary component with multi-arch support.
type BinaryComponent struct {
	Version       string      `json:"version"`
	Type          ComponentType `json:"type"`
	Path          string      `json:"path"`
	Architectures []string    `json:"architectures"`
	Files         BinaryFiles `json:"files,omitempty"`
}

// BinaryFiles defines binary component file names.
type BinaryFiles struct {
	Archive string `json:"archive"`
}

// KubernetesComponent defines Kubernetes binaries with OS-specific packages.
type KubernetesComponent struct {
	Version   string                    `json:"version"`
	Type      ComponentType             `json:"type"`
	Path      string                    `json:"path"`
	Platforms map[string]K8SPlatform    `json:"platforms"`
	ImageList []string                  `json:"imageList,omitempty"`
}

// K8SPlatform defines OS-specific package configuration.
type K8SPlatform struct {
	Architectures []string          `json:"architectures"`
	Packages      map[string]string `json:"packages"`
}

// CNIComponent defines a CNI plugin component.
type CNIComponent struct {
	Version      string      `json:"version"`
	Type         ComponentType `json:"type"`
	Path         string      `json:"path"`
	InstallModes []string    `json:"installModes,omitempty"`
	Files        CNIFiles    `json:"files,omitempty"`
	Images       string      `json:"images,omitempty"`
	ImageList    []string    `json:"imageList,omitempty"`
	HelmValues   map[string]string `json:"helmValues,omitempty"`
}

// CNIFiles defines CNI component file names.
type CNIFiles struct {
	Manifest string `json:"manifest,omitempty"`
	Chart    string `json:"chart,omitempty"`
}

// CSIComponent defines a CSI driver component.
type CSIComponent struct {
	Version      string      `json:"version"`
	Type         ComponentType `json:"type"`
	Path         string      `json:"path"`
	InstallModes []string    `json:"installModes,omitempty"`
	Files        CSIFiles    `json:"files,omitempty"`
	Images       string      `json:"images,omitempty"`
	ImageList    []string    `json:"imageList,omitempty"`
	HelmValues   map[string]string `json:"helmValues,omitempty"`
}

// CSIFiles defines CSI component file names.
type CSIFiles struct {
	Manifest string `json:"manifest,omitempty"`
	Chart    string `json:"chart,omitempty"`
}

// ManifestComponent defines a manifest-based component.
type ManifestComponent struct {
	Version   string      `json:"version"`
	Type      ComponentType `json:"type"`
	Path      string      `json:"path"`
	Manifest  string      `json:"manifest"`
	CRDs      string      `json:"crds,omitempty"`
	Images    string      `json:"images,omitempty"`
	ImageList []string    `json:"imageList,omitempty"`
}

// ImageMetadata defines container image metadata.
type ImageMetadata struct {
	// Path is the directory path in the release image.
	Path string `json:"path"`
	// Images is the list of image tar files.
	Images []string `json:"images"`
}

// AddonDefinition defines an addon included in a release.
type AddonDefinition struct {
	// Name is the addon name.
	Name string `json:"name"`

	// Type is the addon type (manifest/helm).
	Type AddonType `json:"type"`

	// Version is the addon version.
	Version string `json:"version"`

	// ContentPath is the path to the addon content in the release image.
	// For manifest type: path to YAML file
	// For helm type: path to chart tarball
	ContentPath string `json:"contentPath"`

	// Namespace is the default namespace for the addon.
	Namespace string `json:"namespace,omitempty"`

	// Variables defines the variables supported by this addon.
	Variables []AddonVariable `json:"variables,omitempty"`

	// DefaultValues are the default values for the addon.
	// For Helm: helm values
	// For Manifest: variable substitutions
	DefaultValues map[string]apiextensionsv1.JSON `json:"defaultValues,omitempty"`

	// Dependencies lists other addons that must be installed first.
	Dependencies []string `json:"dependencies,omitempty"`

	// HealthCheck defines health check configuration.
	HealthCheck *AddonHealthCheck `json:"healthCheck,omitempty"`
}

// AddonType represents the type of addon.
type AddonType string

const (
	AddonTypeManifest AddonType = "manifest"
	AddonTypeHelm     AddonType = "helm"
)

// AddonVariable defines a variable that can be customized.
type AddonVariable struct {
	// Name is the variable name.
	Name string `json:"name"`

	// Type is the variable type (string/number/boolean/object).
	Type VariableType `json:"type"`

	// Description is the variable description.
	Description string `json:"description,omitempty"`

	// Required indicates if the variable is required.
	Required bool `json:"required,omitempty"`

	// Default is the default value.
	Default *apiextensionsv1.JSON `json:"default,omitempty"`

	// Enum lists allowed values.
	Enum []apiextensionsv1.JSON `json:"enum,omitempty"`

	// Path is the JSON path in the manifest where this variable is used.
	// For Helm: not needed (uses values.yaml)
	// For Manifest: e.g., ".spec.replicas" or ".metadata.namespace"
	Path string `json:"path,omitempty"`
}

// VariableType represents the type of a variable.
type VariableType string

const (
	VariableTypeString  VariableType = "string"
	VariableTypeNumber  VariableType = "number"
	VariableTypeBoolean VariableType = "boolean"
	VariableTypeObject  VariableType = "object"
)

// AddonHealthCheck defines health check configuration for an addon.
type AddonHealthCheck struct {
	// Type is the health check type.
	Type HealthCheckType `json:"type"`

	// Namespace is the namespace to check.
	Namespace string `json:"namespace,omitempty"`

	// Name is the resource name to check.
	Name string `json:"name,omitempty"`

	// Selector is the label selector for resources to check.
	Selector *metav1.LabelSelector `json:"selector,omitempty"`
}

// HealthCheckType represents the type of health check.
type HealthCheckType string

const (
	HealthCheckTypeDeploymentReady HealthCheckType = "DeploymentReady"
	HealthCheckTypeDaemonSetReady  HealthCheckType = "DaemonSetReady"
	HealthCheckTypeCRDEstablished  HealthCheckType = "CRDEstablished"
	HealthCheckTypeEndpointHealthy HealthCheckType = "EndpointHealthy"
)

type UpgradePhase struct {
	Name          string             `json:"name"`
	Order         int                `json:"order"`
	Blocking      bool               `json:"blocking"`
	RollingUpdate *RollingUpdate     `json:"rollingUpdate,omitempty"`
	Components    []UpgradeComponent `json:"components"`
}

type RollingUpdate struct {
	MaxUnavailable int `json:"maxUnavailable,omitempty"`
}

type UpgradeComponent struct {
	Name        string       `json:"name"`
	Manifests   []string     `json:"manifests,omitempty"`
	Scripts     []string     `json:"scripts,omitempty"`
	Blocking    bool         `json:"blocking"`
	DependsOn   []string     `json:"dependsOn,omitempty"`
	HealthCheck *HealthCheck `json:"healthCheck,omitempty"`
}

type HealthCheck struct {
	Type          string          `json:"type"`
	Namespace     string          `json:"namespace,omitempty"`
	Name          string          `json:"name,omitempty"`
	LabelSelector string          `json:"labelSelector,omitempty"`
	Endpoint      string          `json:"endpoint,omitempty"`
	Timeout       metav1.Duration `json:"timeout,omitempty"`
}

type ReleaseImageStatus struct {
	Verified        bool                    `json:"verified"`
	ManifestCount   int                     `json:"manifestCount"`
	ImagesImported  bool                    `json:"imagesImported,omitempty"`
	ImportJobName   string                  `json:"importJobName,omitempty"`
	ImportStatus    string                  `json:"importStatus,omitempty"`
	ImportMessage   string                  `json:"importMessage,omitempty"`
	ImportedImages  []ImportedImageStatus   `json:"importedImages,omitempty"`
}

// ImportedImageStatus tracks the status of a single imported image.
type ImportedImageStatus struct {
	// Component is the component name.
	Component string `json:"component"`
	// Image is the image name.
	Image string `json:"image"`
	// TargetRef is the target registry reference.
	TargetRef string `json:"targetRef"`
	// Status is the import status (pending/imported/failed).
	Status string `json:"status"`
	// Message is an optional status message.
	Message string `json:"message,omitempty"`
}

type ReleaseImageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ReleaseImage `json:"items"`
}
