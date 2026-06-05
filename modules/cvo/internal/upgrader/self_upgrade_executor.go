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
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

func (e *SelfUpgradeExecutor) UpgradeCRDs(ctx context.Context) error {
	crds := &apiextensionsv1.CustomResourceDefinitionList{}
	if err := e.Client.List(ctx, crds); err != nil {
		return fmt.Errorf("failed to list CRDs: %w", err)
	}

	for _, crd := range crds.Items {
		if err := e.backupCRD(ctx, &crd); err != nil {
			return fmt.Errorf("failed to backup CRD %s: %w", crd.Name, err)
		}
	}

	return nil
}

func (e *SelfUpgradeExecutor) UpgradeRBAC(ctx context.Context) error {
	return nil
}

func (e *SelfUpgradeExecutor) UpgradeWebhooks(ctx context.Context) error {
	return nil
}

func (e *SelfUpgradeExecutor) UpgradeDeployment(ctx context.Context, namespace, name string, newImage string, strategy UpgradeStrategy) error {
	deploy := &appsv1.Deployment{}
	if err := e.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, deploy); err != nil {
		return fmt.Errorf("failed to get deployment %s/%s: %w", namespace, name, err)
	}

	if len(deploy.Spec.Template.Spec.Containers) > 0 {
		deploy.Spec.Template.Spec.Containers[0].Image = newImage
	}

	if strategy.MaxUnavailable != nil {
		deploy.Spec.Strategy.RollingUpdate.MaxUnavailable = strategy.MaxUnavailable
	}
	if strategy.MaxSurge != nil {
		deploy.Spec.Strategy.RollingUpdate.MaxSurge = strategy.MaxSurge
	}

	if err := e.Client.Update(ctx, deploy); err != nil {
		return fmt.Errorf("failed to update deployment %s/%s: %w", namespace, name, err)
	}

	if err := e.waitForDeploymentReady(ctx, namespace, name, strategy.Timeout); err != nil {
		return fmt.Errorf("deployment %s/%s not ready: %w", namespace, name, err)
	}

	return nil
}

func (e *SelfUpgradeExecutor) RollbackDeployment(ctx context.Context, namespace, name string) error {
	deploy := &appsv1.Deployment{}
	if err := e.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, deploy); err != nil {
		return fmt.Errorf("failed to get deployment %s/%s: %w", namespace, name, err)
	}

	if deploy.Status.ObservedGeneration < 2 {
		return fmt.Errorf("no previous revision to rollback to")
	}

	deploy.Spec.Template.Spec.Containers[0].Image = ""
	if err := e.Client.Update(ctx, deploy); err != nil {
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
		if err := e.Client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, deploy); err != nil {
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

func (e *SelfUpgradeExecutor) backupCRD(ctx context.Context, crd *apiextensionsv1.CustomResourceDefinition) error {
	return nil
}

type UpgradeStrategy struct {
	MaxUnavailable *int
	MaxSurge       *int
	MinReadySeconds int32
	Timeout        time.Duration
}
