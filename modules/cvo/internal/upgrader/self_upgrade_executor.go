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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

type SelfUpgradeExecutor struct {
	client.Client
	HealthChecker *HealthChecker
	BackupPath    string
}

func NewSelfUpgradeExecutor(c client.Client, healthChecker *HealthChecker) *SelfUpgradeExecutor {
	return &SelfUpgradeExecutor{
		Client:        c,
		HealthChecker: healthChecker,
		BackupPath:    "/tmp/self-upgrade-backup",
	}
}

func (e *SelfUpgradeExecutor) BackupCRDs(ctx context.Context) error {
	crds := &apiextensionsv1.CustomResourceDefinitionList{}
	if err := e.List(ctx, crds); err != nil {
		return fmt.Errorf("failed to list CRDs: %w", err)
	}

	for i := range crds.Items {
		crd := &crds.Items[i]
		if err := e.backupCRD(ctx, crd); err != nil {
			return fmt.Errorf("failed to backup CRD %s: %w", crd.Name, err)
		}
	}

	return nil
}

func (e *SelfUpgradeExecutor) UpgradeDeployment(ctx context.Context, namespace, name string, newImage string, strategy UpgradeStrategy) error {
	deploy := &appsv1.Deployment{}
	if err := e.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, deploy); err != nil {
		return fmt.Errorf("failed to get deployment %s/%s: %w", namespace, name, err)
	}

	original := deploy.DeepCopy()

	if len(deploy.Spec.Template.Spec.Containers) > 0 {
		deploy.Spec.Template.Spec.Containers[0].Image = newImage
	}

	deploy.Spec.Strategy.Type = appsv1.RollingUpdateDeploymentStrategyType
	if deploy.Spec.Strategy.RollingUpdate == nil {
		deploy.Spec.Strategy.RollingUpdate = &appsv1.RollingUpdateDeployment{}
	}

	if strategy.MaxUnavailable != nil {
		deploy.Spec.Strategy.RollingUpdate.MaxUnavailable = strategy.MaxUnavailable
	}
	if strategy.MaxSurge != nil {
		deploy.Spec.Strategy.RollingUpdate.MaxSurge = strategy.MaxSurge
	}
	if strategy.MinReadySeconds > 0 {
		deploy.Spec.MinReadySeconds = strategy.MinReadySeconds
	}

	if err := e.Patch(ctx, deploy, client.MergeFrom(original)); err != nil {
		return fmt.Errorf("failed to update deployment %s/%s: %w", namespace, name, err)
	}

	if err := e.WaitForDeploymentReady(ctx, namespace, name, strategy.Timeout); err != nil {
		return fmt.Errorf("deployment %s/%s not ready: %w", namespace, name, err)
	}

	return nil
}

func (e *SelfUpgradeExecutor) RollbackDeployment(ctx context.Context, namespace, name string) error {
	deploy := &appsv1.Deployment{}
	if err := e.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, deploy); err != nil {
		return fmt.Errorf("failed to get deployment %s/%s: %w", namespace, name, err)
	}

	if deploy.Status.ObservedGeneration < 2 {
		return fmt.Errorf("no previous revision to rollback to for %s/%s", namespace, name)
	}

	// Rollback by removing the current image tag to use the previous revision
	// Or restore from backup if available
	original := deploy.DeepCopy()

	// Check if we have a backup annotation with the previous image
	if prevImage, ok := deploy.Annotations["cvo.capbm.io/previous-image"]; ok && prevImage != "" {
		if len(deploy.Spec.Template.Spec.Containers) > 0 {
			deploy.Spec.Template.Spec.Containers[0].Image = prevImage
		}
	} else if len(deploy.Spec.Template.Spec.Containers) > 0 {
		// Fallback: remove tag to use untagged (previous) image
		currentImage := deploy.Spec.Template.Spec.Containers[0].Image
		if idx := strings.LastIndex(currentImage, ":"); idx > 0 {
			deploy.Spec.Template.Spec.Containers[0].Image = currentImage[:idx]
		}
	}

	if err := e.Patch(ctx, deploy, client.MergeFrom(original)); err != nil {
		return fmt.Errorf("failed to rollback deployment %s/%s: %w", namespace, name, err)
	}

	return nil
}

func (e *SelfUpgradeExecutor) WaitForDeploymentReady(ctx context.Context, namespace, name string, timeout time.Duration) error {
	if timeout == 0 {
		timeout = 5 * time.Minute
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return wait.PollUntilContextCancel(ctx, 5*time.Second, true, func(ctx context.Context) (bool, error) {
		deploy := &appsv1.Deployment{}
		if err := e.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, deploy); err != nil {
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}

		if deploy.Generation != deploy.Status.ObservedGeneration {
			return false, nil
		}

		if deploy.Spec.Replicas != nil && deploy.Status.ReadyReplicas < *deploy.Spec.Replicas {
			return false, nil
		}

		if deploy.Status.UnavailableReplicas > 0 {
			return false, nil
		}

		return true, nil
	})
}

func (e *SelfUpgradeExecutor) WaitForCRDEstablished(ctx context.Context, name string, timeout time.Duration) error {
	if timeout == 0 {
		timeout = 2 * time.Minute
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return wait.PollUntilContextCancel(ctx, 5*time.Second, true, func(ctx context.Context) (bool, error) {
		crd := &apiextensionsv1.CustomResourceDefinition{}
		if err := e.Get(ctx, types.NamespacedName{Name: name}, crd); err != nil {
			return false, nil
		}

		for _, cond := range crd.Status.Conditions {
			if cond.Type == apiextensionsv1.Established && cond.Status == apiextensionsv1.ConditionTrue {
				return true, nil
			}
		}

		return false, nil
	})
}

func (e *SelfUpgradeExecutor) backupCRD(ctx context.Context, crd *apiextensionsv1.CustomResourceDefinition) error {
	// Serialize CRD to YAML
	crdYAML, err := yaml.Marshal(crd)
	if err != nil {
		return fmt.Errorf("failed to marshal CRD %s: %w", crd.Name, err)
	}

	// Create backup ConfigMap
	backupName := fmt.Sprintf("crd-backup-%s-%s", crd.Name, time.Now().Format("20060102150405"))
	backupConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backupName,
			Namespace: "cvo-system",
			Labels: map[string]string{
				"app.kubernetes.io/part-of": "capbm",
				"cvo.capbm.io/backup":       "true",
				"cvo.capbm.io/backup-type":  "crd",
				"cvo.capbm.io/crd-name":     crd.Name,
			},
		},
		Data: map[string]string{
			"crd.yaml": string(crdYAML),
		},
	}

	if err := e.Create(ctx, backupConfigMap); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create CRD backup ConfigMap: %w", err)
		}
	}

	return nil
}

type UpgradeStrategy struct {
	MaxUnavailable  *intstr.IntOrString
	MaxSurge        *intstr.IntOrString
	MinReadySeconds int32
	Timeout         time.Duration
}
