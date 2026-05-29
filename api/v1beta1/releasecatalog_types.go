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

type ReleaseCatalog struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ReleaseCatalogSpec   `json:"spec,omitempty"`
	Status ReleaseCatalogStatus `json:"status,omitempty"`
}

type ReleaseCatalogSpec struct {
	Image        string          `json:"image"`
	SyncInterval metav1.Duration `json:"syncInterval,omitempty"`
}

type ReleaseCatalogStatus struct {
	LastSyncTime  metav1.Time                 `json:"lastSyncTime,omitempty"`
	SyncSucceeded bool                        `json:"syncSucceeded"`
	ImageDigest   string                      `json:"imageDigest,omitempty"`
	Releases      []ReleaseEntry              `json:"releases,omitempty"`
	Channels      map[string][]ChannelVersion `json:"channels,omitempty"`
}

type ReleaseEntry struct {
	Version     string   `json:"version"`
	Image       string   `json:"image"`
	Channels    []string `json:"channels,omitempty"`
	ReleaseDate string   `json:"releaseDate,omitempty"`
}

type ChannelVersion struct {
	Version string `json:"version"`
}

type ReleaseCatalogList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ReleaseCatalog `json:"items"`
}
