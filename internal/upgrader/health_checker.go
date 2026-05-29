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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type HealthChecker struct {
	client  client.Client
	timeout time.Duration
}

func NewHealthChecker(c client.Client, timeout time.Duration) *HealthChecker {
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	return &HealthChecker{client: c, timeout: timeout}
}

func (h *HealthChecker) CheckClusterHealthy(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()

	nodeList := &corev1.NodeList{}
	if err := h.client.List(ctx, nodeList); err != nil {
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

func (h *HealthChecker) WaitForPodsReady(ctx context.Context, namespace, labelSelector string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return wait.PollUntilContextCancel(ctx, 5*time.Second, true, func(ctx context.Context) (bool, error) {
		podList := &corev1.PodList{}
		opts := []client.ListOption{client.InNamespace(namespace)}
		if labelSelector != "" {
			label, err := metav1.ParseToLabelSelector(labelSelector)
			if err == nil {
				selector, err := metav1.LabelSelectorAsSelector(label)
				if err == nil {
					opts = append(opts, client.MatchingLabelsSelector{Selector: selector})
				}
			}
		}
		if err := h.client.List(ctx, podList, opts...); err != nil {
			return false, nil
		}

		if len(podList.Items) == 0 {
			return false, nil
		}

		for _, pod := range podList.Items {
			if pod.Status.Phase != corev1.PodRunning {
				return false, nil
			}
		}
		return true, nil
	})
}

func (h *HealthChecker) WaitForDeploymentReady(ctx context.Context, namespace, name string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return wait.PollUntilContextCancel(ctx, 5*time.Second, true, func(ctx context.Context) (bool, error) {
		deploy := &appsv1.Deployment{}
		if err := h.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, deploy); err != nil {
			return false, nil
		}
		if deploy.Status.ReadyReplicas == deploy.Status.Replicas && deploy.Status.Replicas > 0 {
			return true, nil
		}
		return false, nil
	})
}

func (h *HealthChecker) WaitForDaemonSetReady(ctx context.Context, namespace, name string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return wait.PollUntilContextCancel(ctx, 5*time.Second, true, func(ctx context.Context) (bool, error) {
		ds := &appsv1.DaemonSet{}
		if err := h.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, ds); err != nil {
			return false, nil
		}
		if ds.Status.NumberReady == ds.Status.DesiredNumberScheduled && ds.Status.DesiredNumberScheduled > 0 {
			return true, nil
		}
		return false, nil
	})
}
