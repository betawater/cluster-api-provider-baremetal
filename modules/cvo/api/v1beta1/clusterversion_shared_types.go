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

// UpdateState represents the state of an update.
type UpdateState string

const (
	CompletedUpdate UpdateState = "Completed"
	PartialUpdate   UpdateState = "Partial"
)

// Release represents a release version and image.
type Release struct {
	Version string `json:"version"`
	Image   string `json:"image"`
}

// Update represents an update request.
type Update struct {
	Version string `json:"version,omitempty"`
	Image   string `json:"image,omitempty"`
	Force   bool   `json:"force,omitempty"`
}

// UpdateHistory tracks the history of updates.
type UpdateHistory struct {
	State          UpdateState  `json:"state"`
	Version        string       `json:"version"`
	Image          string       `json:"image"`
	Verified       bool         `json:"verified"`
	StartedTime    metav1.Time  `json:"startedTime"`
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`
}

// ComponentStatus tracks the status of a component during upgrade.
type ComponentStatus struct {
	Name          string `json:"name"`
	Version       string `json:"version"`
	TargetVersion string `json:"targetVersion"`
	Phase         string `json:"phase"`
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
