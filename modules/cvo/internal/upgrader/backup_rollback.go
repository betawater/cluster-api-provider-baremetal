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
	"os"
	"time"

	cfov1 "github.com/BetaWater/cluster-api-provider-baremetal/modules/cvo/api/v1beta1"
	capbmssh "github.com/BetaWater/cluster-api-provider-baremetal/modules/cvo/pkg/ssh"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// BackupRollbackExecutor handles component backup and rollback with high cohesion.
// Each component defines its own backup/rollback configuration.
type BackupRollbackExecutor struct {
	client     client.Client
	sshManager *capbmssh.SSHManager
}

// NewBackupRollbackExecutor creates a new backup/rollback executor.
func NewBackupRollbackExecutor(c client.Client, sshManager *capbmssh.SSHManager) *BackupRollbackExecutor {
	return &BackupRollbackExecutor{
		client:     c,
		sshManager: sshManager,
	}
}

// BackupComponents backs up all components before upgrade.
func (e *BackupRollbackExecutor) BackupComponents(ctx context.Context, cluster *cfov1.ClusterVersion, releaseImage *cfov1.ReleaseImage) error {
	// Get all component upgrade configs with names
	componentConfigs := e.getComponentUpgradeConfigsWithNames(releaseImage)

	// Get nodes for hook execution
	nodes, err := e.getClusterNodes(ctx, cluster)
	if err != nil {
		return err
	}

	for name, uc := range componentConfigs {
		if !uc.Backup.Enabled {
			continue
		}

		// Execute pre-backup hooks if defined
		if err := e.executePreBackupHooks(ctx, nodes, name, releaseImage); err != nil {
			return fmt.Errorf("pre-backup hooks failed for component %s: %w", name, err)
		}

		if err := e.backupComponent(ctx, cluster, name, uc); err != nil {
			return fmt.Errorf("failed to backup component %s: %w", name, err)
		}

		// Execute post-backup hooks if defined
		if err := e.executePostBackupHooks(ctx, nodes, name, releaseImage); err != nil {
			return fmt.Errorf("post-backup hooks failed for component %s: %w", name, err)
		}
	}

	return nil
}

// RollbackComponent rolls back a single component using its high-cohesion config.
func (e *BackupRollbackExecutor) RollbackComponent(ctx context.Context, cluster *cfov1.ClusterVersion, componentName string, releaseImage *cfov1.ReleaseImage) error {
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

	// Execute pre-rollback hooks
	if err := e.executePreRollbackHooks(ctx, nodes, componentName, releaseImage); err != nil {
		return fmt.Errorf("pre-rollback hooks failed for component %s: %w", componentName, err)
	}

	for _, node := range nodes {
		if err := e.executeRollbackScript(ctx, node, scriptPath, timeout); err != nil {
			// Try post-rollback hooks even on failure for cleanup
			_ = e.executePostRollbackHooks(ctx, nodes, componentName, releaseImage)
			return fmt.Errorf("failed to rollback component %s on node %s: %w", componentName, node.Name, err)
		}
	}

	// Execute post-rollback hooks
	if err := e.executePostRollbackHooks(ctx, nodes, componentName, releaseImage); err != nil {
		return fmt.Errorf("post-rollback hooks failed for component %s: %w", componentName, err)
	}

	// Run health check after rollback
	if err := e.runHealthCheck(ctx, cluster, &upgradeConfig.HealthCheck); err != nil {
		return fmt.Errorf("health check failed after rollback of %s: %w", componentName, err)
	}

	return nil
}

// backupComponent backs up a single component using its high-cohesion config.
func (e *BackupRollbackExecutor) backupComponent(ctx context.Context, cluster *cfov1.ClusterVersion, componentName string, uc *cfov1.ComponentUpgradeConfig) error {
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
func (e *BackupRollbackExecutor) backupConfigItem(ctx context.Context, cluster *cfov1.ClusterVersion, backupName string, item cfov1.BackupItem) error {
	nodes, err := e.getClusterNodes(ctx, cluster)
	if err != nil {
		return err
	}

	for _, node := range nodes {
		sshConn, err := e.sshManager.Connect(node.Name, 22, capbmssh.Credentials{})
		if err != nil {
			return fmt.Errorf("failed to connect to node %s: %w", node.Name, err)
		}

		script := fmt.Sprintf(`#!/bin/bash
set -euo pipefail

PATH="%s"
TYPE="%s"

if [ "$TYPE" = "file" ]; then
    if [ ! -f "$PATH" ]; then
        echo "File not found: $PATH"
        exit 1
    fi
    cat "$PATH"
elif [ "$TYPE" = "directory" ]; then
    if [ ! -d "$PATH" ]; then
        echo "Directory not found: $PATH"
        exit 1
    fi
    tar -czf - -C "$(dirname "$PATH")" "$(basename "$PATH")" | base64
else
    echo "Unknown type: $TYPE"
    exit 1
fi
`, item.Path, item.Type)

		result, err := sshConn.ExecuteScript(ctx, script)
		e.sshManager.Close(sshConn)
		if err != nil {
			return fmt.Errorf("failed to backup %s on node %s: %w", item.Path, node.Name, err)
		}
		if result.ExitCode != 0 {
			return fmt.Errorf("backup script failed for %s on node %s: %s", item.Path, node.Name, result.Stderr)
		}

		// Store backup data in ConfigMap
		backupData := map[string]string{
			"node":      node.Name,
			"path":      item.Path,
			"type":      item.Type,
			"timestamp": time.Now().Format(time.RFC3339),
			"content":   result.Stdout,
		}

		backupConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      backupName,
				Namespace: "cvo-system",
				Labels: map[string]string{
					"app.kubernetes.io/part-of":    "capbm",
					"cvo.capbm.io/backup":          "true",
					"cvo.capbm.io/backup-component": item.Path,
				},
			},
			Data: backupData,
		}

		if err := e.client.Create(ctx, backupConfigMap); err != nil {
			if !apierrors.IsAlreadyExists(err) {
				return fmt.Errorf("failed to create backup ConfigMap: %w", err)
			}
			// Update existing ConfigMap
			existing := &corev1.ConfigMap{}
			if err := e.client.Get(ctx, types.NamespacedName{Name: backupName, Namespace: "cvo-system"}, existing); err != nil {
				return fmt.Errorf("failed to get existing backup ConfigMap: %w", err)
			}
			existing.Data[fmt.Sprintf("%s-%s", node.Name, item.Path)] = result.Stdout
			if err := e.client.Update(ctx, existing); err != nil {
				return fmt.Errorf("failed to update backup ConfigMap: %w", err)
			}
		}
	}

	return nil
}

// createEtcdSnapshot creates an etcd snapshot for control-plane backup.
func (e *BackupRollbackExecutor) createEtcdSnapshot(ctx context.Context, cluster *cfov1.ClusterVersion, backupName string) error {
	nodes, err := e.getClusterNodes(ctx, cluster)
	if err != nil {
		return err
	}

	// Find control-plane nodes
	var controlPlaneNodes []*corev1.Node
	for _, node := range nodes {
		if _, ok := node.Labels["node-role.kubernetes.io/control-plane"]; ok {
			controlPlaneNodes = append(controlPlaneNodes, node)
		}
	}

	if len(controlPlaneNodes) == 0 {
		return fmt.Errorf("no control-plane nodes found for etcd backup")
	}

	// Backup etcd on first control-plane node
	node := controlPlaneNodes[0]
	sshConn, err := e.sshManager.Connect(node.Name, 22, capbmssh.Credentials{})
	if err != nil {
		return fmt.Errorf("failed to connect to node %s: %w", node.Name, err)
	}
	defer e.sshManager.Close(sshConn)

	script := `#!/bin/bash
set -euo pipefail

BACKUP_DIR="/tmp/etcd-backup"
TIMESTAMP=$(date +%Y%m%d%H%M%S)
SNAPSHOT_FILE="${BACKUP_DIR}/etcd-snapshot-${TIMESTAMP}.db"

mkdir -p "$BACKUP_DIR"

ETCDCTL_API=3 etcdctl snapshot save "$SNAPSHOT_FILE" \
  --endpoints=https://127.0.0.1:2379 \
  --cacert=/etc/kubernetes/pki/etcd/ca.crt \
  --cert=/etc/kubernetes/pki/etcd/server.crt \
  --key=/etc/kubernetes/pki/etcd/server.key

# Output snapshot as base64
base64 "$SNAPSHOT_FILE"

# Clean up old backups
ls -t ${BACKUP_DIR}/etcd-snapshot-*.db 2>/dev/null | tail -n +4 | xargs rm -f 2>/dev/null || true
`

	result, err := sshConn.ExecuteScript(ctx, script)
	if err != nil {
		return fmt.Errorf("failed to execute etcd snapshot script: %w", err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("etcd snapshot script failed: %s", result.Stderr)
	}

	// Store snapshot in Secret
	snapshotSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backupName,
			Namespace: "cvo-system",
			Labels: map[string]string{
				"app.kubernetes.io/part-of": "capbm",
				"cvo.capbm.io/backup":       "true",
				"cvo.capbm.io/backup-type":  "etcd-snapshot",
			},
		},
		Data: map[string][]byte{
			"snapshot": []byte(result.Stdout),
			"node":     []byte(node.Name),
		},
	}

	if err := e.client.Create(ctx, snapshotSecret); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create etcd snapshot Secret: %w", err)
		}
	}

	return nil
}

// executeRollbackScript executes a rollback script on a node.
func (e *BackupRollbackExecutor) executeRollbackScript(ctx context.Context, node *corev1.Node, scriptPath string, timeout time.Duration) error {
	sshConn, err := e.sshManager.Connect(node.Name, 22, capbmssh.Credentials{})
	if err != nil {
		return fmt.Errorf("failed to connect to node %s: %w", node.Name, err)
	}
	defer e.sshManager.Close(sshConn)

	// Read rollback script content
	scriptContent, err := os.ReadFile(scriptPath)
	if err != nil {
		return fmt.Errorf("failed to read rollback script %s: %w", scriptPath, err)
	}

	rollbackCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result, err := sshConn.ExecuteScript(rollbackCtx, string(scriptContent))
	if err != nil {
		return fmt.Errorf("failed to execute rollback script on node %s: %w", node.Name, err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("rollback script failed on node %s: %s", node.Name, result.Stderr)
	}

	return nil
}

// runHealthCheck runs health check after upgrade/rollback.
func (e *BackupRollbackExecutor) runHealthCheck(ctx context.Context, cluster *cfov1.ClusterVersion, hc *cfov1.ComponentHealthCheckConfig) error {
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

	nodes, err := e.getClusterNodes(ctx, cluster)
	if err != nil {
		return err
	}

	var lastErr error
	for i := 0; i < retries; i++ {
		allHealthy := true
		for _, node := range nodes {
			sshConn, err := e.sshManager.Connect(node.Name, 22, capbmssh.Credentials{})
			if err != nil {
				lastErr = fmt.Errorf("failed to connect to node %s: %w", node.Name, err)
				allHealthy = false
				break
			}

			checkCtx, cancel := context.WithTimeout(ctx, timeout)
			result, err := sshConn.ExecuteScript(checkCtx, hc.Command)
			e.sshManager.Close(sshConn)
			cancel()

			if err != nil || result.ExitCode != 0 {
				lastErr = fmt.Errorf("health check failed on node %s: %s", node.Name, result.Stderr)
				allHealthy = false
				break
			}
		}

		if allHealthy {
			return nil
		}

		// Wait before retry
		time.Sleep(10 * time.Second)
	}

	return fmt.Errorf("health check failed after %d retries: %w", retries, lastErr)
}

// getComponentUpgradeConfigsWithNames gets all component upgrade configs from release image with their names.
func (e *BackupRollbackExecutor) getComponentUpgradeConfigsWithNames(releaseImage *cfov1.ReleaseImage) map[string]*cfov1.ComponentUpgradeConfig {
	configs := make(map[string]*cfov1.ComponentUpgradeConfig)

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
func (e *BackupRollbackExecutor) getComponentUpgradeConfig(releaseImage *cfov1.ReleaseImage, componentName string) *cfov1.ComponentUpgradeConfig {
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
func (e *BackupRollbackExecutor) getClusterNodes(ctx context.Context, cluster *cfov1.ClusterVersion) ([]*corev1.Node, error) {
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

// executePreBackupHooks executes pre-backup hooks for a component.
func (e *BackupRollbackExecutor) executePreBackupHooks(ctx context.Context, nodes []*corev1.Node, componentName string, releaseImage *cfov1.ReleaseImage) error {
	hooks := e.getComponentPreHooks(releaseImage, componentName)
	return e.executeHooksOnNodes(ctx, nodes, hooks, componentName, "pre-backup")
}

// executePostBackupHooks executes post-backup hooks for a component.
func (e *BackupRollbackExecutor) executePostBackupHooks(ctx context.Context, nodes []*corev1.Node, componentName string, releaseImage *cfov1.ReleaseImage) error {
	hooks := e.getComponentPostHooks(releaseImage, componentName)
	return e.executeHooksOnNodes(ctx, nodes, hooks, componentName, "post-backup")
}

// executePreRollbackHooks executes pre-rollback hooks for a component.
func (e *BackupRollbackExecutor) executePreRollbackHooks(ctx context.Context, nodes []*corev1.Node, componentName string, releaseImage *cfov1.ReleaseImage) error {
	hooks := e.getComponentPreHooks(releaseImage, componentName)
	return e.executeHooksOnNodes(ctx, nodes, hooks, componentName, "pre-rollback")
}

// executePostRollbackHooks executes post-rollback hooks for a component.
func (e *BackupRollbackExecutor) executePostRollbackHooks(ctx context.Context, nodes []*corev1.Node, componentName string, releaseImage *cfov1.ReleaseImage) error {
	hooks := e.getComponentPostHooks(releaseImage, componentName)
	return e.executeHooksOnNodes(ctx, nodes, hooks, componentName, "post-rollback")
}

// getComponentPreHooks gets pre-hooks for a component.
func (e *BackupRollbackExecutor) getComponentPreHooks(releaseImage *cfov1.ReleaseImage, componentName string) []cfov1.AddonHook {
	// Check binary components
	if componentName == "kubernetes" {
		return releaseImage.Spec.Components.Kubernetes.PreHooks
	}
	if componentName == "containerd" {
		return releaseImage.Spec.Components.Containerd.PreHooks
	}

	// Check addon components
	for _, addon := range releaseImage.Spec.Addons {
		if addon.Name == componentName {
			return addon.PreHooks
		}
	}

	return nil
}

// getComponentPostHooks gets post-hooks for a component.
func (e *BackupRollbackExecutor) getComponentPostHooks(releaseImage *cfov1.ReleaseImage, componentName string) []cfov1.AddonHook {
	// Check binary components
	if componentName == "kubernetes" {
		return releaseImage.Spec.Components.Kubernetes.PostHooks
	}
	if componentName == "containerd" {
		return releaseImage.Spec.Components.Containerd.PostHooks
	}

	// Check addon components
	for _, addon := range releaseImage.Spec.Addons {
		if addon.Name == componentName {
			return addon.PostHooks
		}
	}

	return nil
}

// executeHooksOnNodes executes hooks on all nodes.
func (e *BackupRollbackExecutor) executeHooksOnNodes(ctx context.Context, nodes []*corev1.Node, hooks []cfov1.AddonHook, componentName, phase string) error {
	for _, hook := range hooks {
		for _, node := range nodes {
			if err := e.executeHookOnNode(ctx, node, hook, componentName, phase); err != nil {
				switch hook.OnFailure {
				case "Abort":
					return fmt.Errorf("hook %s failed during %s for component %s on node %s: %w", hook.Name, phase, componentName, node.Name, err)
				case "Ignore":
					continue
				case "Continue":
					continue
				default:
					return fmt.Errorf("hook %s failed during %s for component %s on node %s: %w", hook.Name, phase, componentName, node.Name, err)
				}
			}
		}
	}
	return nil
}

// executeHookOnNode executes a single hook on a node.
func (e *BackupRollbackExecutor) executeHookOnNode(ctx context.Context, node *corev1.Node, hook cfov1.AddonHook, componentName, phase string) error {
	timeout := 5 * time.Minute
	if hook.Timeout != nil {
		timeout = hook.Timeout.Duration
	}

	hookCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// In production, this would:
	// 1. Get SSH connection to node
	// 2. Execute hook.Command via SSH
	// 3. Return error if command fails
	// For now, this is a placeholder.
	_ = hookCtx
	_ = node
	_ = hook
	_ = componentName
	_ = phase
	return nil
}
