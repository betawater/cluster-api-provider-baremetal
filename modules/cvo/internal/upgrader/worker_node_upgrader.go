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

	cfov1 "github.com/BetaWater/cluster-api-provider-baremetal/modules/cvo/api/v1beta1"
	capbmssh "github.com/BetaWater/cluster-api-provider-baremetal/modules/cvo/pkg/ssh"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// WorkerNodeUpgrader handles worker node in-place upgrades.
type WorkerNodeUpgrader struct {
	client     client.Client
	clientset  *kubernetes.Clientset
	sshManager *capbmssh.SSHManager
	config     WorkerUpgradeConfig
}

// WorkerUpgradeConfig defines configuration for worker node upgrades.
type WorkerUpgradeConfig struct {
	// Rolling update settings
	RollingUpdate RollingUpdateConfig `json:"rollingUpdate"`

	// Rollback settings
	Rollback RollbackConfig `json:"rollback"`
}

// NewWorkerNodeUpgrader creates a new worker node upgrader.
func NewWorkerNodeUpgrader(c client.Client, clientset *kubernetes.Clientset, sshManager *capbmssh.SSHManager, config WorkerUpgradeConfig) *WorkerNodeUpgrader {
	return &WorkerNodeUpgrader{
		client:     c,
		clientset:  clientset,
		sshManager: sshManager,
		config:     config,
	}
}

// ExecuteUpgrade executes the worker node rolling upgrade.
func (u *WorkerNodeUpgrader) ExecuteUpgrade(ctx context.Context, cv *cfov1.ClusterVersion, releaseImage *cfov1.ReleaseImage) error {
	// 1. Pre-upgrade checks
	if err := u.preUpgradeChecks(ctx, cv); err != nil {
		return fmt.Errorf("pre-upgrade checks failed: %w", err)
	}

	// 2. Get worker nodes
	nodes, err := u.getWorkerNodes(ctx, cv)
	if err != nil {
		return err
	}

	if len(nodes) == 0 {
		return nil
	}

	// 3. Use rolling upgrade coordinator
	coordinator := NewRollingUpgradeCoordinator(
		u.config.RollingUpdate.MaxUnavailable,
		1,
	)

	return coordinator.ExecuteRollingUpgrade(ctx, nodes, func(ctx context.Context, node *corev1.Node) error {
		return u.upgradeNode(ctx, node, releaseImage)
	})
}

// upgradeNode upgrades a single worker node.
func (u *WorkerNodeUpgrader) upgradeNode(ctx context.Context, node *corev1.Node, releaseImage *cfov1.ReleaseImage) error {
	// 1. Drain pods
	if u.config.RollingUpdate.Drain.Enabled {
		if err := u.drainNode(ctx, node); err != nil {
			return err
		}
	}

	// 2. Upgrade containerd
	if err := u.upgradeContainerd(ctx, node, releaseImage); err != nil {
		return err
	}

	// 3. Upgrade kubelet
	if err := u.upgradeKubelet(ctx, node, releaseImage); err != nil {
		return err
	}

	// 4. Verify node upgrade
	if err := u.verifyNodeUpgrade(ctx, node, releaseImage); err != nil {
		return err
	}

	// 5. Uncordon node
	if u.config.RollingUpdate.Drain.Enabled {
		return u.uncordonNode(ctx, node)
	}

	return nil
}

// preUpgradeChecks performs pre-upgrade checks.
func (u *WorkerNodeUpgrader) preUpgradeChecks(ctx context.Context, cv *cfov1.ClusterVersion) error {
	nodes, err := u.getWorkerNodes(ctx, cv)
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
			return fmt.Errorf("worker node %s is not Ready", node.Name)
		}
	}

	return nil
}

// drainNode drains pods from a worker node.
func (u *WorkerNodeUpgrader) drainNode(ctx context.Context, node *corev1.Node) error {
	if u.clientset == nil {
		return fmt.Errorf("kubernetes clientset not initialized")
	}

	pods, err := u.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("spec.nodeName=%s", node.Name),
	})
	if err != nil {
		return fmt.Errorf("failed to list pods on node %s: %w", node.Name, err)
	}

	for _, pod := range pods.Items {
		// Skip DaemonSet pods
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

// uncordonNode uncordons a worker node.
func (u *WorkerNodeUpgrader) uncordonNode(ctx context.Context, node *corev1.Node) error {
	if u.clientset == nil {
		return fmt.Errorf("kubernetes clientset not initialized")
	}

	patch := []byte(`{"spec":{"unschedulable":false}}`)
	_, err := u.clientset.CoreV1().Nodes().Patch(ctx, node.Name, types.MergePatchType, patch, metav1.PatchOptions{})
	return err
}

// upgradeContainerd upgrades containerd on a worker node.
func (u *WorkerNodeUpgrader) upgradeContainerd(ctx context.Context, node *corev1.Node, releaseImage *cfov1.ReleaseImage) error {
	sshConn, err := u.sshManager.Connect(node.Name, 22, capbmssh.Credentials{})
	if err != nil {
		return fmt.Errorf("failed to connect to node %s: %w", node.Name, err)
	}
	defer u.sshManager.Close(sshConn)

	containerd := releaseImage.Spec.Components.Containerd
	installStrategy := containerd.InstallStrategy
	if installStrategy == nil {
		installStrategy = &cfov1.BinaryInstallStrategy{
			Timeout:     &metav1.Duration{Duration: 5 * time.Minute},
			RetryCount:  3,
			Method:      "package",
			ServiceName: "containerd",
		}
	}

	serviceName := "containerd"
	if installStrategy.ServiceName != "" {
		serviceName = installStrategy.ServiceName
	}

	script := fmt.Sprintf(`#!/bin/bash
set -euo pipefail

SERVICE_NAME="%s"

echo "=== Upgrading containerd ==="

# Stop service
systemctl stop "$SERVICE_NAME"

# Backup configuration
cp -r /etc/containerd /etc/containerd.bak.$(date +%%Y%%m%%d%%H%%M%%S) 2>/dev/null || true

# Upgrade based on install method
case "%s" in
    package)
        apt-get update && apt-get install -y containerd || \
        dnf install -y containerd || \
        zypper install -y containerd
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
`, serviceName, installStrategy.Method, containerd.Files.Archive)

	timeout := 10 * time.Minute
	if installStrategy.Timeout != nil {
		timeout = installStrategy.Timeout.Duration
	}

	scriptCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result, err := sshConn.ExecuteScript(scriptCtx, script)
	if err != nil {
		return fmt.Errorf("failed to execute containerd upgrade script: %w", err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("containerd upgrade script failed: %s", result.Stderr)
	}

	return nil
}

// upgradeKubelet upgrades kubelet on a worker node.
func (u *WorkerNodeUpgrader) upgradeKubelet(ctx context.Context, node *corev1.Node, releaseImage *cfov1.ReleaseImage) error {
	sshConn, err := u.sshManager.Connect(node.Name, 22, capbmssh.Credentials{})
	if err != nil {
		return fmt.Errorf("failed to connect to node %s: %w", node.Name, err)
	}
	defer u.sshManager.Close(sshConn)

	kubernetes := releaseImage.Spec.Components.Kubernetes

	script := fmt.Sprintf(`#!/bin/bash
set -euo pipefail

K8S_VERSION="%s"

echo "=== Upgrading kubelet ==="

# Stop kubelet
systemctl stop kubelet

# Backup configuration
cp -r /etc/kubernetes /etc/kubernetes.bak.$(date +%%Y%%m%%d%%H%%M%%S) 2>/dev/null || true
cp /var/lib/kubelet/config.yaml /var/lib/kubelet/config.yaml.bak 2>/dev/null || true

# Upgrade kubelet
apt-get update && apt-get install -y kubelet=${K8S_VERSION#v}-00 || \
dnf install -y kubelet-${K8S_VERSION#v} || \
zypper install -y kubelet=${K8S_VERSION#v}

# Start kubelet
systemctl daemon-reload
systemctl enable --now kubelet

# Verify kubelet is running
systemctl is-active kubelet

echo "=== kubelet upgrade completed ==="
`, kubernetes.Version)

	timeout := 10 * time.Minute
	scriptCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result, err := sshConn.ExecuteScript(scriptCtx, script)
	if err != nil {
		return fmt.Errorf("failed to execute kubelet upgrade script: %w", err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("kubelet upgrade script failed: %s", result.Stderr)
	}

	return nil
}

// verifyNodeUpgrade verifies worker node upgrade.
func (u *WorkerNodeUpgrader) verifyNodeUpgrade(ctx context.Context, node *corev1.Node, releaseImage *cfov1.ReleaseImage) error {
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

// getWorkerNodes gets all worker nodes for a cluster.
func (u *WorkerNodeUpgrader) getWorkerNodes(ctx context.Context, cv *cfov1.ClusterVersion) ([]*corev1.Node, error) {
	nodeList := &corev1.NodeList{}
	if err := u.client.List(ctx, nodeList); err != nil {
		return nil, err
	}

	var nodes []*corev1.Node
	for i := range nodeList.Items {
		node := &nodeList.Items[i]
		// Exclude control plane nodes
		if _, ok := node.Labels["node-role.kubernetes.io/control-plane"]; !ok {
			nodes = append(nodes, node)
		}
	}

	return nodes, nil
}
