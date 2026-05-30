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
	Channels         []string                 `json:"channels,omitempty"`
	PreviousVersions []string                 `json:"previousVersions,omitempty"`
	Components       ReleaseComponentVersions `json:"components"`
	UpgradeGraph     []UpgradePhase           `json:"upgradeGraph"`
	ContentHash      string                   `json:"contentHash,omitempty"`
}

type ReleaseComponentVersions struct {
	Kubernetes   map[string]string `json:"kubernetes"`
	Containerd   string            `json:"containerd,omitempty"`
	Calico       string            `json:"calico,omitempty"`
	Cilium       string            `json:"cilium,omitempty"`
	CephCsi      string            `json:"cephCsi,omitempty"`
	GatewayAPI   string            `json:"gatewayAPI,omitempty"`
	EnvoyGateway string            `json:"envoyGateway,omitempty"`
	MetalLB      string            `json:"metalLB,omitempty"`
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
