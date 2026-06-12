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

import clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"

const (
	// SSHConnectedCondition reports the current status of SSH connection.
	//nolint:staticcheck // ConditionType deprecated in CAPI v1beta2, will migrate when ready
	SSHConnectedCondition clusterv1.ConditionType = "SSHConnected"

	// ComponentsInstalledCondition reports whether required components are installed.
	//nolint:staticcheck // ConditionType deprecated in CAPI v1beta2, will migrate when ready
	ComponentsInstalledCondition clusterv1.ConditionType = "ComponentsInstalled"

	// ContainerRuntimeReadyCondition reports whether the container runtime is ready.
	//nolint:staticcheck // ConditionType deprecated in CAPI v1beta2, will migrate when ready
	ContainerRuntimeReadyCondition clusterv1.ConditionType = "ContainerRuntimeReady"

	// FirewallConfiguredCondition reports whether firewall is configured.
	//nolint:staticcheck // ConditionType deprecated in CAPI v1beta2, will migrate when ready
	FirewallConfiguredCondition clusterv1.ConditionType = "FirewallConfigured"

	// SELinuxConfiguredCondition reports whether SELinux is configured.
	//nolint:staticcheck // ConditionType deprecated in CAPI v1beta2, will migrate when ready
	SELinuxConfiguredCondition clusterv1.ConditionType = "SELinuxConfigured"

	// CNIInstalledCondition reports whether CNI plugin is installed.
	//nolint:staticcheck // ConditionType deprecated in CAPI v1beta2, will migrate when ready
	CNIInstalledCondition clusterv1.ConditionType = "CNIInstalled"

	// CSIInstalledCondition reports whether CSI driver is installed.
	//nolint:staticcheck // ConditionType deprecated in CAPI v1beta2, will migrate when ready
	CSIInstalledCondition clusterv1.ConditionType = "CSIInstalled"

	// LoadBalancerReadyCondition reports whether the load balancer is ready.
	//nolint:staticcheck // ConditionType deprecated in CAPI v1beta2, will migrate when ready
	LoadBalancerReadyCondition clusterv1.ConditionType = "LoadBalancerReady"

	// IngressLoadBalancerReadyCondition reports whether the ingress load balancer is ready.
	//nolint:staticcheck // ConditionType deprecated in CAPI v1beta2, will migrate when ready
	IngressLoadBalancerReadyCondition clusterv1.ConditionType = "IngressLoadBalancerReady"

	// GatewayAPIReadyCondition reports whether the Gateway API components are ready.
	//nolint:staticcheck // ConditionType deprecated in CAPI v1beta2, will migrate when ready
	GatewayAPIReadyCondition clusterv1.ConditionType = "GatewayAPIReady"

	// NodeBootstrapCondition reports whether node bootstrapping is completed.
	//nolint:staticcheck // ConditionType deprecated in CAPI v1beta2, will migrate when ready
	NodeBootstrapCondition clusterv1.ConditionType = "NodeBootstrap"
)

// Common condition reasons
const (
	// ConnectionErrorReason indicates an error occurred during connection.
	ConnectionErrorReason = "ConnectionError"

	// InvalidConfigurationReason indicates the configuration is invalid.
	InvalidConfigurationReason = "InvalidConfiguration"

	// DeletionFailedReason indicates deletion failed.
	DeletionFailedReason = "DeletionFailed"

	// ComponentInstallFailedReason indicates component installation failed.
	ComponentInstallFailedReason = "ComponentInstallFailed"

	// ComponentsInstalledReason indicates components are installed.
	ComponentsInstalledReason = "ComponentsInstalled"

	// ContainerRuntimeNotReadyReason indicates container runtime is not ready.
	ContainerRuntimeNotReadyReason = "ContainerRuntimeNotReady"

	// FirewallConfigFailedReason indicates firewall configuration failed.
	FirewallConfigFailedReason = "FirewallConfigFailed"

	// SELinuxConfigFailedReason indicates SELinux configuration failed.
	SELinuxConfigFailedReason = "SELinuxConfigFailed"

	// InstallationInProgressReason indicates installation is in progress.
	InstallationInProgressReason = "InstallationInProgress"

	// InstallationRetryingReason indicates installation is retrying after failure.
	InstallationRetryingReason = "InstallationRetrying"

	// CNIInstallFailedReason indicates CNI installation failed.
	CNIInstallFailedReason = "CNIInstallFailed"

	// CNIInstalledReason indicates CNI is installed.
	CNIInstalledReason = "CNIInstalled"

	// CSIInstallFailedReason indicates CSI installation failed.
	CSIInstallFailedReason = "CSIInstallFailed"

	// CSIInstalledReason indicates CSI is installed.
	CSIInstalledReason = "CSIInstalled"

	// LBSyncFailedReason indicates load balancer sync failed.
	LBSyncFailedReason = "LBSyncFailed"

	// LoadBalancerReadyReason indicates load balancer is ready.
	LoadBalancerReadyReason = "LoadBalancerReady"

	// LBProviderNotSupportedReason indicates the load balancer provider is not supported.
	LBProviderNotSupportedReason = "LBProviderNotSupported"

	// IngressLBSyncFailedReason indicates ingress load balancer sync failed.
	IngressLBSyncFailedReason = "IngressLBSyncFailed"

	// IngressLoadBalancerReadyReason indicates ingress load balancer is ready.
	IngressLoadBalancerReadyReason = "IngressLoadBalancerReady"

	// GatewayAPIInstallFailedReason indicates Gateway API installation failed.
	GatewayAPIInstallFailedReason = "GatewayAPIInstallFailed"

	// GatewayAPIReadyReason indicates Gateway API components are ready.
	GatewayAPIReadyReason = "GatewayAPIReady"

	// NodeBootstrapFailedReason indicates node bootstrapping failed.
	NodeBootstrapFailedReason = "NodeBootstrapFailed"

	// NodeBootstrapCompletedReason indicates node bootstrapping is completed.
	NodeBootstrapCompletedReason = "NodeBootstrapCompleted"
)
