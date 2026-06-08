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

const (
	SelfUpgradeFinalizer = "selfupgrade.cvo.capbm.io"

	// SelfUpgrade conditions
	SelfUpgradeValidating  = "Validating"
	SelfUpgradePreUpgrade  = "PreUpgrade"
	SelfUpgradeUpgrading   = "Upgrading"
	SelfUpgradeVerifying   = "Verifying"
	SelfUpgradeCompleted   = "Completed"
	SelfUpgradeRollingBack = "RollingBack"
	SelfUpgradeFailed      = "Failed"
)

// SelfUpgradePhase represents the phase of a self-upgrade.
type SelfUpgradePhase string

const (
	PhasePending       SelfUpgradePhase = "Pending"
	PhaseValidating    SelfUpgradePhase = "Validating"
	PhasePreUpgrade    SelfUpgradePhase = "PreUpgrade"
	PhaseUpgrading     SelfUpgradePhase = "Upgrading"
	PhaseVerifying     SelfUpgradePhase = "Verifying"
	PhaseCompleted     SelfUpgradePhase = "Completed"
	PhaseRollingBack   SelfUpgradePhase = "RollingBack"
	PhaseFailed        SelfUpgradePhase = "Failed"
)

// SelfUpgradeComponentType represents the type of component to upgrade.
type SelfUpgradeComponentType string

const (
	SelfUpgradeComponentTypeCRD       SelfUpgradeComponentType = "crd"
	SelfUpgradeComponentTypeRBAC      SelfUpgradeComponentType = "rbac"
	SelfUpgradeComponentTypeWebhook   SelfUpgradeComponentType = "webhook"
	SelfUpgradeComponentTypeDeployment SelfUpgradeComponentType = "deployment"
)

// StrategyType represents the upgrade strategy type.
type StrategyType string

const (
	StrategyRolling  StrategyType = "Rolling"
	StrategyRecreate StrategyType = "Recreate"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=selfupgrades,scope=Namespaced

// SelfUpgrade is the Schema for the selfupgrades API.
type SelfUpgrade struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SelfUpgradeSpec   `json:"spec,omitempty"`
	Status SelfUpgradeStatus `json:"status,omitempty"`
}

// SelfUpgradeSpec defines the desired state of SelfUpgrade.
type SelfUpgradeSpec struct {
	// TargetVersion is the target version to upgrade to.
	TargetVersion string `json:"targetVersion"`

	// ReleaseImage is the OCI image reference containing upgrade components.
	// +optional
	ReleaseImage string `json:"releaseImage,omitempty"`

	// Strategy defines the upgrade strategy.
	// +optional
	Strategy SelfUpgradeStrategy `json:"strategy,omitempty"`

	// PreUpgradeHooks are hooks to run before upgrade.
	// +optional
	PreUpgradeHooks []Hook `json:"preUpgradeHooks,omitempty"`

	// PostUpgradeHooks are hooks to run after upgrade.
	// +optional
	PostUpgradeHooks []Hook `json:"postUpgradeHooks,omitempty"`

	// Components defines which components to upgrade.
	// +optional
	Components []SelfUpgradeComponent `json:"components,omitempty"`

	// Paused indicates whether the upgrade is paused.
	// When true, the controller will not proceed with the upgrade.
	// +optional
	Paused bool `json:"paused,omitempty"`
}

// SelfUpgradeStrategy defines the upgrade strategy.
type SelfUpgradeStrategy struct {
	// Type is the strategy type (Rolling, Recreate).
	// +optional
	Type StrategyType `json:"type,omitempty"`

	// MaxUnavailable is the maximum number of components that can be unavailable.
	// +optional
	MaxUnavailable int `json:"maxUnavailable,omitempty"`

	// MaxSurge is the maximum number of extra components that can be created.
	// +optional
	MaxSurge int `json:"maxSurge,omitempty"`

	// MinReadySeconds is the minimum time a component must be ready before continuing.
	// +optional
	MinReadySeconds int `json:"minReadySeconds,omitempty"`

	// Timeout is the maximum time for the entire upgrade.
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// AutoRollback enables automatic rollback on failure.
	// +optional
	AutoRollback bool `json:"autoRollback,omitempty"`
}

// Hook defines a pre/post upgrade hook.
type Hook struct {
	// Name is the hook name.
	Name string `json:"name"`

	// Command is the command to execute.
	Command string `json:"command"`

	// Timeout is the maximum time for the hook to complete.
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// OnFailure defines the action to take on failure (Abort, Continue, Retry).
	// +optional
	OnFailure string `json:"onFailure,omitempty"`
}

// SelfUpgradeComponent defines a component to upgrade.
type SelfUpgradeComponent struct {
	// Name is the component name.
	Name string `json:"name"`

	// Type is the component type (deployment, crd, webhook, rbac).
	Type SelfUpgradeComponentType `json:"type"`

	// Order defines the upgrade order.
	// +optional
	Order int `json:"order,omitempty"`

	// Blocking indicates if this component must succeed before continuing.
	// +optional
	Blocking bool `json:"blocking,omitempty"`

	// DependsOn lists component dependencies.
	// +optional
	DependsOn []string `json:"dependsOn,omitempty"`

	// HealthCheck defines the health check for this component.
	// +optional
	HealthCheck *HealthCheck `json:"healthCheck,omitempty"`
}

// SelfUpgradeStatus defines the observed state of SelfUpgrade.
type SelfUpgradeStatus struct {
	// Phase is the current upgrade phase.
	// +optional
	Phase SelfUpgradePhase `json:"phase,omitempty"`

	// StartedTime is when the upgrade started.
	// +optional
	StartedTime metav1.Time `json:"startedTime,omitempty"`

	// CompletedTime is when the upgrade completed.
	// +optional
	CompletedTime *metav1.Time `json:"completedTime,omitempty"`

	// CurrentVersion is the current version after upgrade.
	// +optional
	CurrentVersion string `json:"currentVersion,omitempty"`

	// ComponentStatus tracks status of each component.
	// +optional
	ComponentStatus []ComponentUpgradeStatus `json:"componentStatus,omitempty"`

	// History tracks upgrade history.
	// +optional
	History []UpgradeHistory `json:"history,omitempty"`

	// Conditions represents the latest available observations.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the last generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// ComponentUpgradeStatus tracks the status of a single component upgrade.
type ComponentUpgradeStatus struct {
	// Name is the component name.
	Name string `json:"name"`

	// Type is the component type.
	Type SelfUpgradeComponentType `json:"type"`

	// Phase is the current phase of this component.
	// +optional
	Phase string `json:"phase,omitempty"`

	// StartedTime is when this component upgrade started.
	// +optional
	StartedTime metav1.Time `json:"startedTime,omitempty"`

	// CompletedTime is when this component upgrade completed.
	// +optional
	CompletedTime *metav1.Time `json:"completedTime,omitempty"`

	// Message contains any error or status message.
	// +optional
	Message string `json:"message,omitempty"`
}

// UpgradeHistory tracks upgrade history.
type UpgradeHistory struct {
	// Version is the target version.
	Version string `json:"version"`

	// StartedTime is when the upgrade started.
	StartedTime metav1.Time `json:"startedTime"`

	// CompletionTime is when the upgrade completed.
	// +optional
	CompletionTime *metav1.Time `json:"completionTime,omitempty"`

	// State is the final state (Completed, Failed, RolledBack).
	State string `json:"state"`

	// Message contains any error or status message.
	// +optional
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true

// SelfUpgradeList contains a list of SelfUpgrade.
type SelfUpgradeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SelfUpgrade `json:"items"`
}
