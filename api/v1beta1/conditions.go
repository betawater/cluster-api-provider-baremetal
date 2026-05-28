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
)

// Common condition reasons
const (
	// ConnectionErrorReason indicates an error occurred during connection.
	ConnectionErrorReason = "ConnectionError"

	// InvalidConfigurationReason indicates the configuration is invalid.
	InvalidConfigurationReason = "InvalidConfiguration"

	// DeletionFailedReason indicates deletion failed.
	DeletionFailedReason = "DeletionFailed"
)
