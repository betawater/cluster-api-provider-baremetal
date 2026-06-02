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


)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ReleaseImage is the Schema for the releaseimages API.
type ReleaseImage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ReleaseImageSpec   `json:"spec,omitempty"`
	Status ReleaseImageStatus `json:"status,omitempty"`
}

// ReleaseImageSpec defines the desired state of ReleaseImage.
type ReleaseImageSpec struct {
	Version          string                       `json:"version"`
	Image            string                       `json:"image"`
	HTTPServer       *HTTPServerConfig            `json:"httpServer,omitempty"`
	ImageRegistry    *ImageRegistryConfig         `json:"imageRegistry,omitempty"`
	Channels         []string                     `json:"channels,omitempty"`
	PreviousVersions []string                     `json:"previousVersions,omitempty"`
	Components       ReleaseComponentVersions `json:"components"`
	Addons           []AddonDefinition   `json:"addons,omitempty"`
	UpgradeGraph     []UpgradePhase      `json:"upgradeGraph"`
	ContentHash      string                       `json:"contentHash,omitempty"`
}

// ReleaseImageStatus defines the observed state of ReleaseImage.
type ReleaseImageStatus struct {
	Verified       bool                          `json:"verified"`
	ManifestCount  int                           `json:"manifestCount"`
	ImagesImported bool                          `json:"imagesImported,omitempty"`
	ImportJobName  string                        `json:"importJobName,omitempty"`
	ImportStatus   string                        `json:"importStatus,omitempty"`
	ImportMessage  string                        `json:"importMessage,omitempty"`
	ImportedImages []ImportedImageStatus `json:"importedImages,omitempty"`
}

// +kubebuilder:object:root=true

// ReleaseImageList contains a list of ReleaseImage.
type ReleaseImageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ReleaseImage `json:"items"`
}
