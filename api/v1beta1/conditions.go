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

import clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"

const (
	// SSHConnectedCondition reports the current status of SSH connection.
	SSHConnectedCondition clusterv1.ConditionType = "SSHConnected"

	// ComponentsInstalledCondition reports whether required components are installed.
	ComponentsInstalledCondition clusterv1.ConditionType = "ComponentsInstalled"

	// ContainerRuntimeReadyCondition reports whether the container runtime is ready.
	ContainerRuntimeReadyCondition clusterv1.ConditionType = "ContainerRuntimeReady"

	// FirewallConfiguredCondition reports whether firewall is configured.
	FirewallConfiguredCondition clusterv1.ConditionType = "FirewallConfigured"

	// SELinuxConfiguredCondition reports whether SELinux is configured.
	SELinuxConfiguredCondition clusterv1.ConditionType = "SELinuxConfigured"

	// CNIInstalledCondition reports whether CNI plugin is installed.
	CNIInstalledCondition clusterv1.ConditionType = "CNIInstalled"

	// CSIInstalledCondition reports whether CSI driver is installed.
	CSIInstalledCondition clusterv1.ConditionType = "CSIInstalled"

	// LoadBalancerReadyCondition reports whether the load balancer is ready.
	LoadBalancerReadyCondition clusterv1.ConditionType = "LoadBalancerReady"

	// IngressLoadBalancerReadyCondition reports whether the ingress load balancer is ready.
	IngressLoadBalancerReadyCondition clusterv1.ConditionType = "IngressLoadBalancerReady"
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
)
