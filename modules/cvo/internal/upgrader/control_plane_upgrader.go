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
	"strings"
	"time"

	cfov1 "github.com/BetaWater/cluster-api-provider-baremetal/modules/cvo/api/v1beta1"
	capbmssh "github.com/BetaWater/cluster-api-provider-baremetal/modules/cvo/pkg/ssh"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/BetaWater/cluster-api-provider-baremetal/modules/cvo/internal/upgrader/metrics"
)

// ControlPlaneUpgrader coordinates control plane in-place upgrades with KCP.
type ControlPlaneUpgrader struct {
	client     client.Client
	clientset  *kubernetes.Clientset
	sshManager *capbmssh.SSHManager
	config     ControlPlaneUpgradeConfig
}

// ControlPlaneUpgradeConfig defines configuration for control plane upgrades.
type ControlPlaneUpgradeConfig struct {
	// KCP coordination settings
	KubeadmControlPlane KCPConfig `json:"kubeadmControlPlane"`
	
	// Rolling update settings
	RollingUpdate RollingUpdateConfig `json:"rollingUpdate"`
	
	// etcd backup settings
	EtcdBackup EtcdBackupConfig `json:"etcdBackup"`
	
	// Rollback settings
	Rollback RollbackConfig `json:"rollback"`
}

// KCPConfig defines KCP coordination settings.
type KCPConfig struct {
	// Enabled indicates whether to use KCP for kubeadm upgrades.
	Enabled bool `json:"enabled"`
	
	// WaitForKCP indicates whether to wait for KCP to complete before upgrading other components.
	WaitForKCP bool `json:"waitForKCP"`
}

// RollingUpdateConfig defines rolling update settings.
type RollingUpdateConfig struct {
	// MaxUnavailable is the maximum number of nodes that can be unavailable during upgrade.
	MaxUnavailable int `json:"maxUnavailable"`
	
	// Drain settings
	Drain DrainConfig `json:"drain"`
	
	// Timeout is the timeout for single node upgrade.
	Timeout time.Duration `json:"timeout"`
}

// DrainConfig defines drain settings.
type DrainConfig struct {
	// Enabled indicates whether to drain pods before upgrade.
	Enabled bool `json:"enabled"`
	
	// Timeout is the timeout for drain operation.
	Timeout time.Duration `json:"timeout"`
	
	// IgnoreDaemonSets indicates whether to ignore daemonsets during drain.
	IgnoreDaemonSets bool `json:"ignoreDaemonSets"`
}

// EtcdBackupConfig defines etcd backup settings.
type EtcdBackupConfig struct {
	// Enabled indicates whether to backup etcd before upgrade.
	Enabled bool `json:"enabled"`
	
	// Timeout is the timeout for etcd backup.
	Timeout time.Duration `json:"timeout"`
	
	// Storage settings
	Storage EtcdBackupStorageConfig `json:"storage"`
}

// EtcdBackupStorageConfig defines etcd backup storage settings.
type EtcdBackupStorageConfig struct {
	// Type is the storage type: Secret or PVC.
	Type string `json:"type"`
	
	// Retention is the number of backups to retain.
	Retention int `json:"retention"`
}

// RollbackConfig defines rollback settings.
type RollbackConfig struct {
	// Enabled indicates whether to enable automatic rollback.
	Enabled bool `json:"enabled"`
	
	// OnTimeout indicates whether to rollback on timeout.
	OnTimeout bool `json:"onTimeout"`
	
	// OnFailure indicates whether to rollback on failure.
	OnFailure bool `json:"onFailure"`
}

// NewControlPlaneUpgrader creates a new control plane upgrader.
func NewControlPlaneUpgrader(c client.Client, clientset *kubernetes.Clientset, sshManager *capbmssh.SSHManager, config ControlPlaneUpgradeConfig) *ControlPlaneUpgrader {
	return &ControlPlaneUpgrader{
		client:     c,
		clientset:  clientset,
		sshManager: sshManager,
		config:     config,
	}
}

// ExecuteUpgrade executes the control plane rolling upgrade.
func (u *ControlPlaneUpgrader) ExecuteUpgrade(ctx context.Context, cv *cfov1.ClusterVersion, releaseImage *cfov1.ReleaseImage) error {
	// 1. Pre-upgrade checks
	if err := u.preUpgradeChecks(ctx, cv); err != nil {
		return fmt.Errorf("pre-upgrade checks failed: %w", err)
	}
	
	// 2. Backup before upgrade
	if err := u.backupBeforeUpgrade(ctx, cv, releaseImage); err != nil {
		return fmt.Errorf("backup failed: %w", err)
	}
	
	// 3. Get control plane nodes
	nodes, err := u.getControlPlaneNodes(ctx, cv)
	if err != nil {
		return err
	}
	
	// 4. Upgrade nodes one by one
	for _, node := range nodes {
		if err := u.upgradeNode(ctx, cv, node, releaseImage); err != nil {
			// Try rollback on failure
			if u.config.Rollback.Enabled {
				if rollbackErr := u.rollback(ctx, cv, releaseImage); rollbackErr != nil {
					return fmt.Errorf("upgrade failed on node %s and rollback also failed: %w, rollback error: %v", node.Name, err, rollbackErr)
				}
			}
			return fmt.Errorf("upgrade failed on node %s: %w", node.Name, err)
		}
	}
	
	// 5. Wait for KCP to complete upgrade (if enabled)
	if u.config.KubeadmControlPlane.WaitForKCP {
		if err := u.waitForKCPUpgrade(ctx, cv); err != nil {
			return fmt.Errorf("KCP upgrade failed: %w", err)
		}
	}
	
	// 6. Post-upgrade verification
	return u.postUpgradeVerification(ctx, cv, releaseImage)
}

// upgradeNode upgrades a single control plane node.
func (u *ControlPlaneUpgrader) upgradeNode(ctx context.Context, cv *cfov1.ClusterVersion, node *corev1.Node, releaseImage *cfov1.ReleaseImage) error {
	// 1. Execute component pre-hooks (containerd)
	if err := u.executeComponentPreHooks(ctx, node, releaseImage.Spec.Components.Containerd.PreHooks, "containerd"); err != nil {
		return err
	}

	// 2. Drain pods (if enabled by strategy or config)
	shouldDrain := u.config.RollingUpdate.Drain.Enabled
	if releaseImage.Spec.Components.Containerd.UpgradeStrategy != nil && releaseImage.Spec.Components.Containerd.UpgradeStrategy.Drain {
		shouldDrain = true
	}
	if shouldDrain {
		if err := u.drainNode(ctx, node); err != nil {
			return err
		}
	}
	
	// 3. CAPBM: Upgrade containerd with strategy
	if err := u.upgradeContainerd(ctx, node, releaseImage); err != nil {
		return err
	}
	
	// 4. Execute component post-hooks (containerd)
	if err := u.executeComponentPostHooks(ctx, node, releaseImage.Spec.Components.Containerd.PostHooks, "containerd"); err != nil {
		return err
	}

	// 5. CAPBM: Upgrade CNI components
	if err := u.upgradeCNI(ctx, node, releaseImage); err != nil {
		return err
	}
	
	// 6. CAPBM: Upgrade CSI components
	if err := u.upgradeCSI(ctx, node, releaseImage); err != nil {
		return err
	}
	
	// 7. KCP: kubeadm upgrade node (handled by KCP Controller)
	// KCP will automatically detect version change and execute kubeadm upgrade node
	
	// 8. Verify node upgrade
	if err := u.verifyNodeUpgrade(ctx, node, releaseImage); err != nil {
		return err
	}
	
	// 9. Uncordon node (if drained)
	if shouldDrain {
		return u.uncordonNode(ctx, node)
	}
	
	return nil
}

// preUpgradeChecks performs pre-upgrade checks.
func (u *ControlPlaneUpgrader) preUpgradeChecks(ctx context.Context, cv *cfov1.ClusterVersion) error {
	// 1. Check cluster health
	if err := u.checkClusterHealth(ctx, cv); err != nil {
		return err
	}
	
	// 2. Check control plane node count
	nodes, err := u.getControlPlaneNodes(ctx, cv)
	if err != nil {
		return err
	}
	if len(nodes) < 3 {
		return fmt.Errorf("at least 3 control plane nodes required for HA, got %d", len(nodes))
	}
	
	// 3. Check etcd cluster health
	if err := u.checkEtcdHealth(ctx, cv); err != nil {
		return err
	}
	
	return nil
}

// backupBeforeUpgrade performs backup before upgrade.
func (u *ControlPlaneUpgrader) backupBeforeUpgrade(ctx context.Context, cv *cfov1.ClusterVersion, releaseImage *cfov1.ReleaseImage) error {
	if !u.config.EtcdBackup.Enabled {
		return nil
	}
	
	// Backup etcd on all control plane nodes
	nodes, err := u.getControlPlaneNodes(ctx, cv)
	if err != nil {
		return err
	}
	
	for _, node := range nodes {
		if err := u.backupEtcd(ctx, node); err != nil {
			return fmt.Errorf("failed to backup etcd on node %s: %w", node.Name, err)
		}
	}
	
	return nil
}

// backupEtcd backs up etcd on a control plane node.
func (u *ControlPlaneUpgrader) backupEtcd(ctx context.Context, node *corev1.Node) error {
	startTime := time.Now()

	sshConn, err := u.sshManager.Connect(node.Name, 22, capbmssh.Credentials{})
	if err != nil {
		metrics.EtcdBackupDuration.WithLabelValues("", node.Name, "failed").Observe(time.Since(startTime).Seconds())
		return fmt.Errorf("failed to connect to node %s: %w", node.Name, err)
	}
	defer u.sshManager.Close(sshConn)

	script := `#!/bin/bash
set -euo pipefail

BACKUP_DIR="/tmp/etcd-backup"
TIMESTAMP=$(date +%Y%m%d%H%M%S)
SNAPSHOT_FILE="${BACKUP_DIR}/etcd-snapshot-${TIMESTAMP}.db"

mkdir -p "$BACKUP_DIR"

# Run etcdctl snapshot save
ETCDCTL_API=3 etcdctl snapshot save "$SNAPSHOT_FILE" \
  --endpoints=https://127.0.0.1:2379 \
  --cacert=/etc/kubernetes/pki/etcd/ca.crt \
  --cert=/etc/kubernetes/pki/etcd/server.crt \
  --key=/etc/kubernetes/pki/etcd/server.key

echo "etcd snapshot saved to $SNAPSHOT_FILE"

# Clean up old backups (keep only last N)
ls -t ${BACKUP_DIR}/etcd-snapshot-*.db 2>/dev/null | tail -n +4 | xargs rm -f 2>/dev/null || true
`

	result, err := sshConn.ExecuteScript(ctx, script)
	duration := time.Since(startTime).Seconds()

	if err != nil {
		metrics.EtcdBackupDuration.WithLabelValues("", node.Name, "failed").Observe(duration)
		return fmt.Errorf("failed to execute etcd backup script: %w", err)
	}
	if result.ExitCode != 0 {
		metrics.EtcdBackupDuration.WithLabelValues("", node.Name, "failed").Observe(duration)
		return fmt.Errorf("etcd backup script failed: %s", result.Stderr)
	}

	metrics.EtcdBackupDuration.WithLabelValues("", node.Name, "success").Observe(duration)
	return nil
}

// drainNode drains pods from a node.
func (u *ControlPlaneUpgrader) drainNode(ctx context.Context, node *corev1.Node) error {
	if u.clientset == nil {
		return fmt.Errorf("kubernetes clientset not initialized")
	}

	// List all pods on the node
	pods, err := u.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("spec.nodeName=%s", node.Name),
	})
	if err != nil {
		return fmt.Errorf("failed to list pods on node %s: %w", node.Name, err)
	}

	// Evict each pod
	for _, pod := range pods.Items {
		// Skip static pods and DaemonSet pods
		if pod.OwnerReferences != nil {
			isDaemonSet := false
			for _, ref := range pod.OwnerReferences {
				if ref.Kind == "DaemonSet" {
					isDaemonSet = true
					break
				}
			}
			if isDaemonSet {
				continue
			}
		}

		eviction := &policyv1.Eviction{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pod.Name,
				Namespace: pod.Namespace,
			},
		}

		if err := u.clientset.PolicyV1().Evictions(pod.Namespace).Evict(ctx, eviction); err != nil {
			if !apierrors.IsTooManyRequests(err) {
				return fmt.Errorf("failed to evict pod %s/%s: %w", pod.Namespace, pod.Name, err)
			}
		}
	}

	// Wait for pods to be evicted
	return wait.PollUntilContextTimeout(ctx, 5*time.Second, u.config.RollingUpdate.Drain.Timeout, true, func(ctx context.Context) (bool, error) {
		pods, err := u.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{
			FieldSelector: fmt.Sprintf("spec.nodeName=%s", node.Name),
		})
		if err != nil {
			return false, err
		}

		for _, pod := range pods.Items {
			if pod.OwnerReferences != nil {
				isDaemonSet := false
				for _, ref := range pod.OwnerReferences {
					if ref.Kind == "DaemonSet" {
						isDaemonSet = true
						break
					}
				}
				if isDaemonSet {
					continue
				}
			}
			return false, nil
		}

		return true, nil
	})
}

// uncordonNode uncordons a node.
func (u *ControlPlaneUpgrader) uncordonNode(ctx context.Context, node *corev1.Node) error {
	if u.clientset == nil {
		return fmt.Errorf("kubernetes clientset not initialized")
	}

	// Patch node to remove Unschedulable taint
	patch := []byte(`{"spec":{"unschedulable":false}}`)
	_, err := u.clientset.CoreV1().Nodes().Patch(ctx, node.Name, types.MergePatchType, patch, metav1.PatchOptions{})
	return err
}

// upgradeContainerd upgrades containerd on a node using the component's strategy.
func (u *ControlPlaneUpgrader) upgradeContainerd(ctx context.Context, node *corev1.Node, releaseImage *cfov1.ReleaseImage) error {
	containerd := releaseImage.Spec.Components.Containerd

	// Get upgrade strategy or use defaults
	strategy := containerd.UpgradeStrategy
	if strategy == nil {
		strategy = &cfov1.BinaryUpgradeStrategy{
			Type:        "Rolling",
			RetryCount:  3,
			Timeout:     &metav1.Duration{Duration: 10 * time.Minute},
			MaxConcurrent: 1,
		}
	}

	// Get install strategy for service restart info
	installStrategy := containerd.InstallStrategy
	if installStrategy == nil {
		installStrategy = &cfov1.BinaryInstallStrategy{
			Timeout:     &metav1.Duration{Duration: 5 * time.Minute},
			RetryCount:  3,
			Method:      "package",
			ServiceName: "containerd",
		}
	}

	// Execute upgrade based on strategy type
	switch strategy.Type {
	case "Rolling":
		return u.upgradeContainerdRolling(ctx, node, containerd, strategy, installStrategy)
	case "DrainAndUpgrade":
		return u.upgradeContainerdDrainAndUpgrade(ctx, node, containerd, strategy, installStrategy)
	case "Parallel":
		return u.upgradeContainerdParallel(ctx, node, containerd, strategy, installStrategy)
	default:
		return u.upgradeContainerdRolling(ctx, node, containerd, strategy, installStrategy)
	}
}

// upgradeContainerdRolling performs a rolling upgrade of containerd.
func (u *ControlPlaneUpgrader) upgradeContainerdRolling(ctx context.Context, node *corev1.Node, containerd cfov1.BinaryComponent, strategy *cfov1.BinaryUpgradeStrategy, installStrategy *cfov1.BinaryInstallStrategy) error {
	startTime := time.Now()

	sshConn, err := u.sshManager.Connect(node.Name, 22, capbmssh.Credentials{})
	if err != nil {
		metrics.NodeUpgradeDuration.WithLabelValues("", node.Name, "containerd", "failed").Observe(time.Since(startTime).Seconds())
		return fmt.Errorf("failed to connect to node %s: %w", node.Name, err)
	}
	defer u.sshManager.Close(sshConn)

	serviceName := "containerd"
	if installStrategy.ServiceName != "" {
		serviceName = installStrategy.ServiceName
	}

	script := fmt.Sprintf(`#!/bin/bash
set -euo pipefail

SERVICE_NAME="%s"
VERSION="%s"

echo "=== Upgrading containerd to $VERSION ==="

# Stop service
systemctl stop "$SERVICE_NAME"

# Backup configuration
cp -r /etc/containerd /etc/containerd.bak.$(date +%%Y%%m%%d%%H%%M%%S) 2>/dev/null || true

# Upgrade based on install method
case "%s" in
    package)
        apt-get update && apt-get install -y containerd=%s || \
        dnf install -y containerd-%s || \
        zypper install -y containerd=%s
        ;;
    archive)
        curl -fsSL "%s" -o /tmp/containerd.tar.gz
        tar -C /usr/local -xzf /tmp/containerd.tar.gz
        rm -f /tmp/containerd.tar.gz
        ;;
esac

# Restore configuration
if [ -d /etc/containerd.bak.* ]; then
    LATEST_BAK=$(ls -d /etc/containerd.bak.* | tail -1)
    cp -r "$LATEST_BAK"/* /etc/containerd/ 2>/dev/null || true
fi

# Start service
systemctl daemon-reload
systemctl enable --now "$SERVICE_NAME"

# Verify service is running
systemctl is-active "$SERVICE_NAME"

echo "=== containerd upgrade completed ==="
`, serviceName, containerd.Version, installStrategy.Method,
		containerd.Version, containerd.Version, containerd.Version,
		containerd.Files.Archive)

	timeout := 10 * time.Minute
	if strategy.Timeout != nil {
		timeout = strategy.Timeout.Duration
	}

	scriptCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result, err := sshConn.ExecuteScript(scriptCtx, script)
	duration := time.Since(startTime).Seconds()

	if err != nil {
		metrics.NodeUpgradeDuration.WithLabelValues("", node.Name, "containerd", "failed").Observe(duration)
		return fmt.Errorf("failed to execute containerd upgrade script: %w", err)
	}
	if result.ExitCode != 0 {
		metrics.NodeUpgradeDuration.WithLabelValues("", node.Name, "containerd", "failed").Observe(duration)
		return fmt.Errorf("containerd upgrade script failed: %s", result.Stderr)
	}

	metrics.NodeUpgradeDuration.WithLabelValues("", node.Name, "containerd", "success").Observe(duration)
	return nil
}

// upgradeContainerdDrainAndUpgrade performs drain then upgrade.
func (u *ControlPlaneUpgrader) upgradeContainerdDrainAndUpgrade(ctx context.Context, node *corev1.Node, containerd cfov1.BinaryComponent, strategy *cfov1.BinaryUpgradeStrategy, installStrategy *cfov1.BinaryInstallStrategy) error {
	// Drain node first
	if err := u.drainNode(ctx, node); err != nil {
		return err
	}

	// Then perform rolling upgrade
	return u.upgradeContainerdRolling(ctx, node, containerd, strategy, installStrategy)
}

// upgradeContainerdParallel performs parallel upgrade (not recommended for production).
func (u *ControlPlaneUpgrader) upgradeContainerdParallel(ctx context.Context, node *corev1.Node, containerd cfov1.BinaryComponent, strategy *cfov1.BinaryUpgradeStrategy, installStrategy *cfov1.BinaryInstallStrategy) error {
	// Parallel upgrade - upgrade multiple nodes concurrently up to MaxConcurrent
	// For single node, just use rolling upgrade
	return u.upgradeContainerdRolling(ctx, node, containerd, strategy, installStrategy)
}

// upgradeCNI upgrades CNI components on a node.
func (u *ControlPlaneUpgrader) upgradeCNI(ctx context.Context, node *corev1.Node, releaseImage *cfov1.ReleaseImage) error {
	// Update CNI DaemonSet images via Kubernetes API
	for _, addon := range releaseImage.Spec.Addons {
		if addon.Type != "helm" && addon.Type != "manifest" {
			continue
		}

		// Skip non-CNI addons
		if addon.Name != "calico" && addon.Name != "cilium" && addon.Name != "flannel" {
			continue
		}

		// For DaemonSets, update the image
		dsList := &appsv1.DaemonSetList{}
		if err := u.client.List(ctx, dsList, client.InNamespace(addon.Namespace)); err != nil {
			return fmt.Errorf("failed to list DaemonSets in %s: %w", addon.Namespace, err)
		}

		for i := range dsList.Items {
			ds := &dsList.Items[i]
			original := ds.DeepCopy()

			// Update container images based on addon version
			for j := range ds.Spec.Template.Spec.Containers {
				container := &ds.Spec.Template.Spec.Containers[j]
				if container.Image != "" {
					// Extract registry and image name, update tag
					if idx := strings.LastIndex(container.Image, ":"); idx > 0 {
						container.Image = container.Image[:idx+1] + addon.Version
					}
				}
			}

			if err := u.client.Patch(ctx, ds, client.MergeFrom(original)); err != nil {
				return fmt.Errorf("failed to update DaemonSet %s/%s: %w", ds.Namespace, ds.Name, err)
			}
		}
	}

	return nil
}

// upgradeCSI upgrades CSI components on a node.
func (u *ControlPlaneUpgrader) upgradeCSI(ctx context.Context, node *corev1.Node, releaseImage *cfov1.ReleaseImage) error {
	// Update CSI Controller/Node images via Kubernetes API
	for _, addon := range releaseImage.Spec.Addons {
		if addon.Type != "helm" && addon.Type != "manifest" {
			continue
		}

		// Skip non-CSI addons
		if !strings.Contains(addon.Name, "csi") && !strings.Contains(addon.Name, "ceph") {
			continue
		}

		// Update Deployments
		deployList := &appsv1.DeploymentList{}
		if err := u.client.List(ctx, deployList, client.InNamespace(addon.Namespace)); err != nil {
			return fmt.Errorf("failed to list Deployments in %s: %w", addon.Namespace, err)
		}

		for i := range deployList.Items {
			deploy := &deployList.Items[i]
			original := deploy.DeepCopy()

			for j := range deploy.Spec.Template.Spec.Containers {
				container := &deploy.Spec.Template.Spec.Containers[j]
				if container.Image != "" {
					// Extract registry and image name, update tag
					if idx := strings.LastIndex(container.Image, ":"); idx > 0 {
						container.Image = container.Image[:idx+1] + addon.Version
					}
				}
			}

			if err := u.client.Patch(ctx, deploy, client.MergeFrom(original)); err != nil {
				return fmt.Errorf("failed to update Deployment %s/%s: %w", deploy.Namespace, deploy.Name, err)
			}
		}
	}

	return nil
}

// verifyNodeUpgrade verifies node upgrade.
func (u *ControlPlaneUpgrader) verifyNodeUpgrade(ctx context.Context, node *corev1.Node, releaseImage *cfov1.ReleaseImage) error {
	// Check node Ready status
	updatedNode := &corev1.Node{}
	if err := u.client.Get(ctx, types.NamespacedName{Name: node.Name}, updatedNode); err != nil {
		return fmt.Errorf("failed to get node %s: %w", node.Name, err)
	}

	for _, cond := range updatedNode.Status.Conditions {
		if cond.Type == corev1.NodeReady && cond.Status != corev1.ConditionTrue {
			return fmt.Errorf("node %s is not Ready", node.Name)
		}
	}

	return nil
}

// waitForKCPUpgrade waits for KCP to complete upgrade.
func (u *ControlPlaneUpgrader) waitForKCPUpgrade(ctx context.Context, cv *cfov1.ClusterVersion) error {
	return wait.PollUntilContextTimeout(ctx, 10*time.Second, 10*time.Minute, true, func(ctx context.Context) (bool, error) {
		// Check if actual version matches desired version
		if cv.Status.ActualVersion == cv.Spec.DesiredUpdate.Version {
			return true, nil
		}
		return false, nil
	})
}

// postUpgradeVerification performs post-upgrade verification.
func (u *ControlPlaneUpgrader) postUpgradeVerification(ctx context.Context, cv *cfov1.ClusterVersion, releaseImage *cfov1.ReleaseImage) error {
	// 1. Check all control plane nodes Ready
	if err := u.checkControlPlaneNodesReady(ctx, cv); err != nil {
		return err
	}
	
	// 2. Check etcd cluster health
	if err := u.checkEtcdHealth(ctx, cv); err != nil {
		return err
	}
	
	// 3. Check control plane Pods running
	if err := u.checkControlPlanePods(ctx, cv); err != nil {
		return err
	}
	
	// 4. Check API Server health
	if err := u.checkAPIServerHealth(ctx, cv); err != nil {
		return err
	}
	
	return nil
}

// checkClusterHealth checks cluster health.
func (u *ControlPlaneUpgrader) checkClusterHealth(ctx context.Context, cv *cfov1.ClusterVersion) error {
	nodeList := &corev1.NodeList{}
	if err := u.client.List(ctx, nodeList); err != nil {
		return fmt.Errorf("failed to list nodes: %w", err)
	}

	for _, node := range nodeList.Items {
		ready := false
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
				ready = true
				break
			}
		}
		if !ready {
			return fmt.Errorf("node %s is not Ready", node.Name)
		}
	}

	return nil
}

// checkControlPlaneNodesReady checks all control plane nodes are Ready.
func (u *ControlPlaneUpgrader) checkControlPlaneNodesReady(ctx context.Context, cv *cfov1.ClusterVersion) error {
	nodes, err := u.getControlPlaneNodes(ctx, cv)
	if err != nil {
		return err
	}

	for _, node := range nodes {
		ready := false
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
				ready = true
				break
			}
		}
		if !ready {
			return fmt.Errorf("control plane node %s is not Ready", node.Name)
		}
	}

	return nil
}

// checkEtcdHealth checks etcd cluster health.
func (u *ControlPlaneUpgrader) checkEtcdHealth(ctx context.Context, cv *cfov1.ClusterVersion) error {
	nodes, err := u.getControlPlaneNodes(ctx, cv)
	if err != nil {
		return err
	}

	if len(nodes) == 0 {
		return fmt.Errorf("no control plane nodes found")
	}

	// Check etcd health on first control plane node
	sshConn, err := u.sshManager.Connect(nodes[0].Name, 22, capbmssh.Credentials{})
	if err != nil {
		return fmt.Errorf("failed to connect to node %s: %w", nodes[0].Name, err)
	}
	defer u.sshManager.Close(sshConn)

	script := `ETCDCTL_API=3 etcdctl endpoint health \
  --endpoints=https://127.0.0.1:2379 \
  --cacert=/etc/kubernetes/pki/etcd/ca.crt \
  --cert=/etc/kubernetes/pki/etcd/server.crt \
  --key=/etc/kubernetes/pki/etcd/server.key`

	result, err := sshConn.ExecuteScript(ctx, script)
	if err != nil {
		return fmt.Errorf("failed to check etcd health: %w", err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("etcd health check failed: %s", result.Stderr)
	}

	return nil
}

// checkControlPlanePods checks control plane Pods are running.
func (u *ControlPlaneUpgrader) checkControlPlanePods(ctx context.Context, cv *cfov1.ClusterVersion) error {
	podList := &corev1.PodList{}
	if err := u.client.List(ctx, podList, client.InNamespace("kube-system"), client.MatchingLabels{
		"tier": "control-plane",
	}); err != nil {
		return fmt.Errorf("failed to list control plane pods: %w", err)
	}

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			return fmt.Errorf("control plane pod %s/%s is not Running (phase: %s)", pod.Namespace, pod.Name, pod.Status.Phase)
		}
	}

	return nil
}

// checkAPIServerHealth checks API Server health.
func (u *ControlPlaneUpgrader) checkAPIServerHealth(ctx context.Context, cv *cfov1.ClusterVersion) error {
	if u.clientset == nil {
		return fmt.Errorf("kubernetes clientset not initialized")
	}

	_, err := u.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{Limit: 1})
	return err
}

// rollback performs rollback on failure.
func (u *ControlPlaneUpgrader) rollback(ctx context.Context, cv *cfov1.ClusterVersion, releaseImage *cfov1.ReleaseImage) error {
	nodes, err := u.getControlPlaneNodes(ctx, cv)
	if err != nil {
		return err
	}

	for _, node := range nodes {
		sshConn, err := u.sshManager.Connect(node.Name, 22, capbmssh.Credentials{})
		if err != nil {
			return fmt.Errorf("failed to connect to node %s: %w", node.Name, err)
		}

		script := `#!/bin/bash
set -euo pipefail

BACKUP_DIR="/tmp/etcd-backup"
LATEST_SNAPSHOT=$(ls -t ${BACKUP_DIR}/etcd-snapshot-*.db 2>/dev/null | head -1)

if [ -z "$LATEST_SNAPSHOT" ]; then
    echo "No etcd snapshot found for rollback"
    exit 1
fi

echo "Restoring etcd from snapshot: $LATEST_SNAPSHOT"

# Stop etcd
systemctl stop etcd

# Restore etcd from snapshot
ETCDCTL_API=3 etcdctl snapshot restore "$LATEST_SNAPSHOT" \
  --data-dir=/var/lib/etcd-restore \
  --name=$(hostname) \
  --initial-cluster=$(hostname)=https://$(hostname -i):2380 \
  --initial-advertise-peer-urls=https://$(hostname -i):2380

# Replace etcd data directory
mv /var/lib/etcd /var/lib/etcd.old
mv /var/lib/etcd-restore /var/lib/etcd

# Start etcd
systemctl start etcd

echo "etcd rollback completed"
`

		result, err := sshConn.ExecuteScript(ctx, script)
		u.sshManager.Close(sshConn)
		if err != nil {
			return fmt.Errorf("failed to execute rollback script on node %s: %w", node.Name, err)
		}
		if result.ExitCode != 0 {
			return fmt.Errorf("rollback script failed on node %s: %s", node.Name, result.Stderr)
		}
	}

	return nil
}

// getControlPlaneNodes gets all control plane nodes for a cluster.
func (u *ControlPlaneUpgrader) getControlPlaneNodes(ctx context.Context, cv *cfov1.ClusterVersion) ([]*corev1.Node, error) {
	nodeList := &corev1.NodeList{}
	if err := u.client.List(ctx, nodeList, client.MatchingLabels{
		"node-role.kubernetes.io/control-plane": "",
	}); err != nil {
		return nil, err
	}
	
	nodes := make([]*corev1.Node, len(nodeList.Items))
	for i := range nodeList.Items {
		nodes[i] = &nodeList.Items[i]
	}
	return nodes, nil
}

// executeComponentPreHooks executes pre-install/upgrade hooks for a component.
func (u *ControlPlaneUpgrader) executeComponentPreHooks(ctx context.Context, node *corev1.Node, hooks []cfov1.AddonHook, componentName string) error {
	for _, hook := range hooks {
		if err := u.executeHookOnNode(ctx, node, hook, componentName, "pre"); err != nil {
			switch hook.OnFailure {
			case "Abort":
				return fmt.Errorf("pre-hook %s failed for component %s on node %s: %w", hook.Name, componentName, node.Name, err)
			case "Ignore":
				continue
			case "Continue":
				continue
			default:
				return fmt.Errorf("pre-hook %s failed for component %s on node %s: %w", hook.Name, componentName, node.Name, err)
			}
		}
	}
	return nil
}

// executeComponentPostHooks executes post-install/upgrade hooks for a component.
func (u *ControlPlaneUpgrader) executeComponentPostHooks(ctx context.Context, node *corev1.Node, hooks []cfov1.AddonHook, componentName string) error {
	for _, hook := range hooks {
		if err := u.executeHookOnNode(ctx, node, hook, componentName, "post"); err != nil {
			switch hook.OnFailure {
			case "Abort":
				return fmt.Errorf("post-hook %s failed for component %s on node %s: %w", hook.Name, componentName, node.Name, err)
			case "Ignore":
				continue
			case "Continue":
				continue
			default:
				return fmt.Errorf("post-hook %s failed for component %s on node %s: %w", hook.Name, componentName, node.Name, err)
			}
		}
	}
	return nil
}

// executeHookOnNode executes a single hook on a node via SSH.
func (u *ControlPlaneUpgrader) executeHookOnNode(ctx context.Context, node *corev1.Node, hook cfov1.AddonHook, componentName, phase string) error {
	timeout := 5 * time.Minute
	if hook.Timeout != nil {
		timeout = hook.Timeout.Duration
	}

	hookCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Get SSH connection to node
	sshConn, err := u.sshManager.Connect(node.Name, 22, capbmssh.Credentials{})
	if err != nil {
		return fmt.Errorf("failed to connect to node %s: %w", node.Name, err)
	}
	defer u.sshManager.Close(sshConn)

	// Execute hook command via SSH
	result, err := sshConn.ExecuteScript(hookCtx, hook.Command)
	if err != nil {
		return fmt.Errorf("%s-hook %s failed for component %s on node %s: %w", phase, hook.Name, componentName, node.Name, err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("%s-hook %s failed for component %s on node %s: %s", phase, hook.Name, componentName, node.Name, result.Stderr)
	}

	return nil
}
