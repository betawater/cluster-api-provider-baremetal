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

package upgrader

import (
	"context"
	"fmt"
	"time"

	infrav1 "github.com/BetaWater/cluster-api-provider-baremetal/api/v1beta1"
	sshclient "github.com/BetaWater/cluster-api-provider-baremetal/internal/ssh"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// BackupRollbackExecutor handles component backup and rollback with high cohesion.
// Each component defines its own backup/rollback configuration.
type BackupRollbackExecutor struct {
	client     client.Client
	sshManager *sshclient.SSHManager
}

// NewBackupRollbackExecutor creates a new backup/rollback executor.
func NewBackupRollbackExecutor(c client.Client, sshManager *sshclient.SSHManager) *BackupRollbackExecutor {
	return &BackupRollbackExecutor{
		client:     c,
		sshManager: sshManager,
	}
}

// BackupComponents backs up all components before upgrade.
func (e *BackupRollbackExecutor) BackupComponents(ctx context.Context, cluster *infrav1.ClusterVersion, releaseImage *infrav1.ReleaseImage) error {
	// Get all component upgrade configs with names
	componentConfigs := e.getComponentUpgradeConfigsWithNames(releaseImage)

	for name, uc := range componentConfigs {
		if !uc.Backup.Enabled {
			continue
		}

		if err := e.backupComponent(ctx, cluster, name, uc); err != nil {
			return fmt.Errorf("failed to backup component %s: %w", name, err)
		}
	}

	return nil
}

// RollbackComponent rolls back a single component using its high-cohesion config.
func (e *BackupRollbackExecutor) RollbackComponent(ctx context.Context, cluster *infrav1.ClusterVersion, componentName string, releaseImage *infrav1.ReleaseImage) error {
	// Get component upgrade config
	upgradeConfig := e.getComponentUpgradeConfig(releaseImage, componentName)
	if upgradeConfig == nil {
		return fmt.Errorf("no upgrade config found for component %s", componentName)
	}

	// Get rollback script path from high-cohesion config
	scriptPath := upgradeConfig.Rollback.Script
	if scriptPath == "" {
		return fmt.Errorf("no rollback script defined for component %s", componentName)
	}

	// Get timeout from config
	timeout := 5 * time.Minute
	if upgradeConfig.Rollback.Timeout != nil {
		timeout = upgradeConfig.Rollback.Timeout.Duration
	}

	// Execute rollback script on nodes
	nodes, err := e.getClusterNodes(ctx, cluster)
	if err != nil {
		return err
	}

	for _, node := range nodes {
		if err := e.executeRollbackScript(ctx, node, scriptPath, timeout); err != nil {
			return fmt.Errorf("failed to rollback component %s on node %s: %w", componentName, node.Name, err)
		}
	}

	// Run health check after rollback
	if err := e.runHealthCheck(ctx, cluster, &upgradeConfig.HealthCheck); err != nil {
		return fmt.Errorf("health check failed after rollback of %s: %w", componentName, err)
	}

	return nil
}

// backupComponent backs up a single component using its high-cohesion config.
func (e *BackupRollbackExecutor) backupComponent(ctx context.Context, cluster *infrav1.ClusterVersion, componentName string, uc *infrav1.ComponentUpgradeConfig) error {
	// Create backup ConfigMap/Secret
	backupName := fmt.Sprintf("backup-%s-%s", cluster.Name, time.Now().Format("20060102150405"))

	// Backup each config item
	for _, item := range uc.Backup.Config {
		if err := e.backupConfigItem(ctx, cluster, backupName, item); err != nil {
			return err
		}
	}

	// Create etcd snapshot if required
	if uc.Backup.EtcdSnapshot {
		if err := e.createEtcdSnapshot(ctx, cluster, backupName); err != nil {
			return err
		}
	}

	return nil
}

// backupConfigItem backs up a single config item (file or directory).
func (e *BackupRollbackExecutor) backupConfigItem(ctx context.Context, cluster *infrav1.ClusterVersion, backupName string, item infrav1.BackupItem) error {
	// In production, this would:
	// 1. SSH to nodes
	// 2. Read the file/directory content
	// 3. Store in ConfigMap/Secret
	// For now, this is a placeholder.
	_ = ctx
	_ = cluster
	_ = backupName
	_ = item
	return nil
}

// createEtcdSnapshot creates an etcd snapshot for control-plane backup.
func (e *BackupRollbackExecutor) createEtcdSnapshot(ctx context.Context, cluster *infrav1.ClusterVersion, backupName string) error {
	// In production, this would:
	// 1. SSH to control-plane nodes
	// 2. Run etcdctl snapshot save
	// 3. Store snapshot in Secret
	// For now, this is a placeholder.
	_ = ctx
	_ = cluster
	_ = backupName
	return nil
}

// executeRollbackScript executes a rollback script on a node.
func (e *BackupRollbackExecutor) executeRollbackScript(ctx context.Context, node *corev1.Node, scriptPath string, timeout time.Duration) error {
	// In production, this would:
	// 1. Get SSH connection to node
	// 2. Read rollback script from release image
	// 3. Execute script with timeout
	// For now, this is a placeholder.
	_ = ctx
	_ = node
	_ = scriptPath
	_ = timeout
	return nil
}

// runHealthCheck runs health check after upgrade/rollback.
func (e *BackupRollbackExecutor) runHealthCheck(ctx context.Context, cluster *infrav1.ClusterVersion, hc *infrav1.ComponentHealthCheckConfig) error {
	if hc == nil || hc.Command == "" {
		return nil
	}

	timeout := 5 * time.Minute
	if hc.Timeout != nil {
		timeout = hc.Timeout.Duration
	}

	retries := hc.Retries
	if retries == 0 {
		retries = 3
	}

	// In production, this would execute the health check command on nodes
	// For now, this is a placeholder.
	_ = ctx
	_ = cluster
	_ = timeout
	_ = retries
	return nil
}

// getComponentUpgradeConfigsWithNames gets all component upgrade configs from release image with their names.
func (e *BackupRollbackExecutor) getComponentUpgradeConfigsWithNames(releaseImage *infrav1.ReleaseImage) map[string]*infrav1.ComponentUpgradeConfig {
	configs := make(map[string]*infrav1.ComponentUpgradeConfig)

	// Binary components
	if releaseImage.Spec.Components.Kubernetes.Upgrade != nil {
		configs["kubernetes"] = releaseImage.Spec.Components.Kubernetes.Upgrade
	}
	if releaseImage.Spec.Components.Containerd.Upgrade != nil {
		configs["containerd"] = releaseImage.Spec.Components.Containerd.Upgrade
	}

	// Addon components (including CAPI Core)
	for _, addon := range releaseImage.Spec.Addons {
		if addon.Upgrade != nil {
			configs[addon.Name] = addon.Upgrade
		}
	}

	return configs
}

// getComponentUpgradeConfig gets upgrade config for a specific component.
func (e *BackupRollbackExecutor) getComponentUpgradeConfig(releaseImage *infrav1.ReleaseImage, componentName string) *infrav1.ComponentUpgradeConfig {
	// Check binary components
	if componentName == "kubernetes" && releaseImage.Spec.Components.Kubernetes.Upgrade != nil {
		return releaseImage.Spec.Components.Kubernetes.Upgrade
	}
	if componentName == "containerd" && releaseImage.Spec.Components.Containerd.Upgrade != nil {
		return releaseImage.Spec.Components.Containerd.Upgrade
	}

	// Check addon components (including CAPI Core)
	for _, addon := range releaseImage.Spec.Addons {
		if addon.Name == componentName && addon.Upgrade != nil {
			return addon.Upgrade
		}
	}

	return nil
}

// getClusterNodes gets all nodes for a cluster.
func (e *BackupRollbackExecutor) getClusterNodes(ctx context.Context, cluster *infrav1.ClusterVersion) ([]*corev1.Node, error) {
	nodeList := &corev1.NodeList{}
	if err := e.client.List(ctx, nodeList, client.MatchingLabels{
		"cluster.x-k8s.io/cluster-name": cluster.Spec.ClusterRef.Name,
	}); err != nil {
		return nil, err
	}

	nodes := make([]*corev1.Node, len(nodeList.Items))
	for i := range nodeList.Items {
		nodes[i] = &nodeList.Items[i]
	}
	return nodes, nil
}

// getSSHConnection gets SSH connection to a node.
func (e *BackupRollbackExecutor) getSSHConnection(ctx context.Context, node *corev1.Node) (*sshclient.SSHConnection, error) {
	// In production, this would:
	// 1. Get node IP
	// 2. Get SSH credentials from secret
	// 3. Create SSH connection
	// For now, this is a placeholder.
	_ = ctx
	_ = node
	return nil, fmt.Errorf("not implemented")
}
