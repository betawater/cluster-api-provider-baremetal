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

// AddonType represents the type of addon.
type AddonType string

const (
	AddonTypeManifest AddonType = "manifest"
	AddonTypeHelm     AddonType = "helm"
)

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
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Variables defines the variables supported by this addon.
	// +optional
	Variables []AddonVariable `json:"variables,omitempty"`

	// DefaultValues are the default values for the addon.
	// For Helm: helm values
	// For Manifest: variable substitutions
	// +optional
	DefaultValues map[string]apiextensionsv1.JSON `json:"defaultValues,omitempty"`

	// Dependencies lists other addons that must be installed first.
	// +optional
	Dependencies []string `json:"dependencies,omitempty"`

	// InstallStrategy defines installation behavior.
	// +optional
	InstallStrategy *AddonInstallStrategy `json:"installStrategy,omitempty"`

	// UpgradeStrategy defines upgrade behavior.
	// +optional
	UpgradeStrategy *AddonUpgradeStrategy `json:"upgradeStrategy,omitempty"`

	// PreHooks are scripts/commands to run before install/upgrade.
	// +optional
	PreHooks []AddonHook `json:"preHooks,omitempty"`

	// PostHooks are scripts/commands to run after install/upgrade.
	// +optional
	PostHooks []AddonHook `json:"postHooks,omitempty"`

	// Upgrade defines component-level upgrade configuration (high cohesion).
	// +optional
	Upgrade *ComponentUpgradeConfig `json:"upgrade,omitempty"`

	// HealthCheck defines health check configuration (deprecated, use Upgrade.HealthCheck).
	// +optional
	HealthCheck *AddonHealthCheck `json:"healthCheck,omitempty"`
}

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

// AddonInstallStrategy defines installation behavior.
type AddonInstallStrategy struct {
	// Timeout is the maximum time allowed for installation.
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// RetryCount is the number of retries on failure.
	// +optional
	// +kubebuilder:default=3
	RetryCount int `json:"retryCount,omitempty"`

	// CreateNamespace indicates whether to create the namespace if not exists.
	// +optional
	// +kubebuilder:default=true
	CreateNamespace bool `json:"createNamespace,omitempty"`

	// Wait indicates whether to wait for resources to be ready.
	// +optional
	// +kubebuilder:default=true
	Wait bool `json:"wait,omitempty"`
}

// AddonUpgradeStrategy defines upgrade behavior.
type AddonUpgradeStrategy struct {
	// Type is the upgrade type: Rolling, Recreate, or BlueGreen.
	// +kubebuilder:validation:Enum=Rolling;Recreate;BlueGreen
	// +kubebuilder:default=Rolling
	Type string `json:"type"`

	// RollingUpdate defines rolling update configuration.
	// +optional
	RollingUpdate *AddonRollingUpdateConfig `json:"rollingUpdate,omitempty"`

	// MaxUnavailable is the maximum number of pods that can be unavailable during upgrade.
	// +optional
	// +kubebuilder:default=1
	MaxUnavailable int `json:"maxUnavailable,omitempty"`

	// Timeout is the maximum time allowed for upgrade.
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// RetryCount is the number of retries on failure.
	// +optional
	// +kubebuilder:default=3
	RetryCount int `json:"retryCount,omitempty"`

	// Force indicates whether to force upgrade even if version path is not recommended.
	// +optional
	Force bool `json:"force,omitempty"`
}

// AddonRollingUpdateConfig defines rolling update configuration.
type AddonRollingUpdateConfig struct {
	// MaxSurge is the maximum number of pods that can be created above the desired count.
	// +optional
	// +kubebuilder:default=1
	MaxSurge int `json:"maxSurge,omitempty"`

	// Partition indicates the ordinal at which to start the rolling update.
	// +optional
	Partition int `json:"partition,omitempty"`
}

// AddonHook defines a pre/post hook for install/upgrade.
type AddonHook struct {
	// Name is the hook name.
	Name string `json:"name"`

	// Command is the command to execute.
	Command string `json:"command"`

	// Timeout is the maximum time allowed for the hook.
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// OnFailure indicates behavior on hook failure: Continue, Abort, or Ignore.
	// +kubebuilder:validation:Enum=Continue;Abort;Ignore
	// +kubebuilder:default=Abort
	OnFailure string `json:"onFailure,omitempty"`
}

// AddonPhase represents the phase of an addon.
type AddonPhase string

const (
	AddonPhasePending    AddonPhase = "Pending"
	AddonPhaseInstalling AddonPhase = "Installing"
	AddonPhaseInstalled  AddonPhase = "Installed"
	AddonPhaseUpgrading  AddonPhase = "Upgrading"
	AddonPhaseFailed     AddonPhase = "Failed"
	AddonPhaseDeleting   AddonPhase = "Deleting"
)
