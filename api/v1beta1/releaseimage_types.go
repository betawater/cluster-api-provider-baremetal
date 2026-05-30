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
	Channels         []string                 `json:"channels,omitempty"`
	PreviousVersions []string                 `json:"previousVersions,omitempty"`
	Components       ReleaseComponentVersions `json:"components"`
	UpgradeGraph     []UpgradePhase           `json:"upgradeGraph"`
	ContentHash      string                   `json:"contentHash,omitempty"`
}

// ReleaseComponentVersions defines all component versions with installation metadata.
type ReleaseComponentVersions struct {
	Kubernetes   KubernetesComponent   `json:"kubernetes"`
	Containerd   BinaryComponent       `json:"containerd,omitempty"`
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
	HelmValues   map[string]string `json:"helmValues,omitempty"`
}

// CSIFiles defines CSI component file names.
type CSIFiles struct {
	Manifest string `json:"manifest,omitempty"`
	Chart    string `json:"chart,omitempty"`
}

// ManifestComponent defines a manifest-based component.
type ManifestComponent struct {
	Version  string      `json:"version"`
	Type     ComponentType `json:"type"`
	Path     string      `json:"path"`
	Manifest string      `json:"manifest"`
	CRDs     string      `json:"crds,omitempty"`
	Images   string      `json:"images,omitempty"`
}

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
	Verified      bool `json:"verified"`
	ManifestCount int  `json:"manifestCount"`
}

type ReleaseImageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ReleaseImage `json:"items"`
}
