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

package addon

import (
	"context"
	"fmt"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	cfov1 "github.com/BetaWater/cluster-api-provider-baremetal/api/cvo/v1beta1"`n`n`tcommonv1 "github.com/BetaWater/cluster-api-provider-baremetal/api/common/v1beta1"
)

// Upgrader handles addon upgrades with high-cohesion configuration.
type Upgrader struct {
	client        client.Client
	releaseServer string
	namespace     string
}

// NewUpgrader creates a new addon upgrader.
func NewUpgrader(c client.Client, releaseServer, namespace string) *Upgrader {
	return &Upgrader{
		client:        c,
		releaseServer: releaseServer,
		namespace:     namespace,
	}
}

// Upgrade upgrades an addon from currentRelease to targetRelease.
func (u *Upgrader) Upgrade(ctx context.Context, addon *cfov1.ClusterAddon, currentRelease, targetRelease *cfov1.ReleaseImage) error {
	// 1. Find addon definitions in both releases
	currentDef := findAddonDefinition(currentRelease, addon.Spec.AddonName)
	targetDef := findAddonDefinition(targetRelease, addon.Spec.AddonName)
	if targetDef == nil {
		return fmt.Errorf("addon %s not found in target release %s", addon.Spec.AddonName, targetRelease.Name)
	}

	// 2. Execute pre-upgrade hooks
	if err := u.executeHooks(ctx, addon, targetDef.PreHooks, "pre-upgrade"); err != nil {
		return fmt.Errorf("pre-upgrade hooks failed: %w", err)
	}

	// 3. Backup current state if configured
	if targetDef.Upgrade != nil && targetDef.Upgrade.Backup.Enabled {
		if err := u.backupAddon(ctx, addon, currentDef, currentRelease); err != nil {
			return fmt.Errorf("backup failed: %w", err)
		}
	}

	// 4. Perform upgrade based on strategy
	if err := u.performUpgrade(ctx, addon, targetDef, targetRelease); err != nil {
		// Try rollback on failure
		if targetDef.Upgrade != nil && targetDef.Upgrade.Rollback.Script != "" {
			if rollbackErr := u.rollbackAddon(ctx, addon, currentDef, currentRelease, targetDef); rollbackErr != nil {
				return fmt.Errorf("upgrade failed and rollback also failed: %w, rollback error: %v", err, rollbackErr)
			}
		}
		return fmt.Errorf("upgrade failed: %w", err)
	}

	// 5. Execute post-upgrade hooks
	if err := u.executeHooks(ctx, addon, targetDef.PostHooks, "post-upgrade"); err != nil {
		return fmt.Errorf("post-upgrade hooks failed: %w", err)
	}

	// 6. Run health check
	if targetDef.Upgrade != nil && targetDef.Upgrade.HealthCheck.Command != "" {
		if err := u.runHealthCheck(ctx, addon, &targetDef.Upgrade.HealthCheck); err != nil {
			return fmt.Errorf("health check failed after upgrade: %w", err)
		}
	}

	// 7. Update addon status
	addon.Status.Version = targetDef.Version
	addon.Status.Phase = commonv1.AddonPhaseInstalled
	addon.Status.LastAppliedRevision = targetDef.Version
	return u.client.Status().Update(ctx, addon)
}

// performUpgrade performs the actual upgrade based on strategy.
func (u *Upgrader) performUpgrade(ctx context.Context, addon *cfov1.ClusterAddon, targetDef *commonv1.AddonDefinition, releaseImage *cfov1.ReleaseImage) error {
	strategy := targetDef.UpgradeStrategy
	if strategy == nil {
		strategy = &commonv1.AddonUpgradeStrategy{
			Type:       "Rolling",
			RetryCount: 3,
			Timeout:    &metav1.Duration{Duration: 5 * time.Minute},
		}
	}

	switch strategy.Type {
	case "Rolling":
		return u.rollingUpgrade(ctx, addon, targetDef, releaseImage, strategy)
	case "Recreate":
		return u.recreateUpgrade(ctx, addon, targetDef, releaseImage, strategy)
	case "BlueGreen":
		return u.blueGreenUpgrade(ctx, addon, targetDef, releaseImage, strategy)
	default:
		return u.rollingUpgrade(ctx, addon, targetDef, releaseImage, strategy)
	}
}

// rollingUpgrade performs a rolling upgrade.
func (u *Upgrader) rollingUpgrade(ctx context.Context, addon *cfov1.ClusterAddon, targetDef *commonv1.AddonDefinition, releaseImage *cfov1.ReleaseImage, strategy *commonv1.AddonUpgradeStrategy) error {
	switch targetDef.Type {
	case commonv1.AddonTypeHelm:
		return u.rollingUpgradeHelm(ctx, addon, targetDef, releaseImage, strategy)
	case commonv1.AddonTypeManifest:
		return u.rollingUpgradeManifest(ctx, addon, targetDef, releaseImage, strategy)
	default:
		return fmt.Errorf("unsupported addon type: %s", targetDef.Type)
	}
}

// rollingUpgradeHelm performs a rolling upgrade for Helm-based addons.
func (u *Upgrader) rollingUpgradeHelm(ctx context.Context, addon *cfov1.ClusterAddon, targetDef *commonv1.AddonDefinition, releaseImage *cfov1.ReleaseImage, strategy *commonv1.AddonUpgradeStrategy) error {
	// Merge values
	mergedValues := MergeValues(targetDef.DefaultValues, addon.Spec.Values)
	valuesYAML, err := yaml.Marshal(mergedValues)
	if err != nil {
		return fmt.Errorf("failed to marshal values: %w", err)
	}

	// Fetch chart
	contentFetcher := NewContentFetcher(u.releaseServer)
	chartContent, err := contentFetcher.FetchFromReleaseImage(ctx, releaseImage, addon.Spec.AddonName)
	if err != nil {
		return err
	}

	// Create ConfigMap
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-helm", addon.Name),
			Namespace: addon.Namespace,
		},
		BinaryData: map[string][]byte{
			"chart.tgz": chartContent,
		},
		Data: map[string]string{
			"values.yaml": string(valuesYAML),
		},
	}
	if err := u.client.Create(ctx, configMap); err != nil && !isAlreadyExists(err) {
		return err
	}

	// Build upgrade job with rolling strategy
	job := u.buildUpgradeJob(addon, targetDef, strategy)
	if err := u.client.Create(ctx, job); err != nil {
		return err
	}

	// Wait for completion
	timeout := 5 * time.Minute
	if strategy.Timeout != nil {
		timeout = strategy.Timeout.Duration
	}

	return wait.PollUntilContextTimeout(ctx, 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		currentJob := &batchv1.Job{}
		if err := u.client.Get(ctx, client.ObjectKeyFromObject(job), currentJob); err != nil {
			return false, nil
		}

		for _, cond := range currentJob.Status.Conditions {
			if cond.Type == batchv1.JobComplete && cond.Status == corev1.ConditionTrue {
				return true, nil
			}
			if cond.Type == batchv1.JobFailed && cond.Status == corev1.ConditionTrue {
				return false, fmt.Errorf("upgrade job failed: %s", cond.Message)
			}
		}
		return false, nil
	})
}

// rollingUpgradeManifest performs a rolling upgrade for manifest-based addons.
func (u *Upgrader) rollingUpgradeManifest(ctx context.Context, addon *cfov1.ClusterAddon, targetDef *commonv1.AddonDefinition, releaseImage *cfov1.ReleaseImage, strategy *commonv1.AddonUpgradeStrategy) error {
	// Merge values
	mergedValues := MergeValues(targetDef.DefaultValues, addon.Spec.Values)

	// Fetch manifest
	contentFetcher := NewContentFetcher(u.releaseServer)
	content, err := contentFetcher.FetchFromReleaseImage(ctx, releaseImage, addon.Spec.AddonName)
	if err != nil {
		return err
	}

	// Process manifest
	processor := &ManifestProcessor{}
	processedContent, err := processor.Process(content, mergedValues)
	if err != nil {
		return fmt.Errorf("failed to process manifest: %w", err)
	}

	// Update ConfigMap
	configMap := &corev1.ConfigMap{}
	if err := u.client.Get(ctx, types.NamespacedName{Name: fmt.Sprintf("%s-manifest", addon.Name), Namespace: addon.Namespace}, configMap); err != nil {
		return err
	}
	configMap.Data["addon.yaml"] = string(processedContent)
	if err := u.client.Update(ctx, configMap); err != nil {
		return err
	}

	// Build upgrade job
	job := u.buildUpgradeJob(addon, targetDef, strategy)
	if err := u.client.Create(ctx, job); err != nil {
		return err
	}

	// Wait for completion
	timeout := 5 * time.Minute
	if strategy.Timeout != nil {
		timeout = strategy.Timeout.Duration
	}

	return wait.PollUntilContextTimeout(ctx, 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		currentJob := &batchv1.Job{}
		if err := u.client.Get(ctx, client.ObjectKeyFromObject(job), currentJob); err != nil {
			return false, nil
		}

		for _, cond := range currentJob.Status.Conditions {
			if cond.Type == batchv1.JobComplete && cond.Status == corev1.ConditionTrue {
				return true, nil
			}
			if cond.Type == batchv1.JobFailed && cond.Status == corev1.ConditionTrue {
				return false, fmt.Errorf("upgrade job failed: %s", cond.Message)
			}
		}
		return false, nil
	})
}

// recreateUpgrade performs a recreate upgrade (delete then create).
func (u *Upgrader) recreateUpgrade(ctx context.Context, addon *cfov1.ClusterAddon, targetDef *commonv1.AddonDefinition, releaseImage *cfov1.ReleaseImage, strategy *commonv1.AddonUpgradeStrategy) error {
	// Delete current resources
	deleteJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("delete-%s", addon.Name),
			Namespace: addon.Namespace,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: ptr.To(int32(1)),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					ServiceAccountName: "capbm-addon-installer",
					Containers: []corev1.Container{
						{
							Name:    "delete",
							Image:   "bitnami/kubectl:latest",
							Command: []string{"sh", "-c", fmt.Sprintf("kubectl delete -f /manifests/addon.yaml --ignore-not-found")},
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
				},
			},
		},
	}
	if err := u.client.Create(ctx, deleteJob); err != nil {
		return err
	}

	// Wait for delete job
	if err := u.waitForJob(ctx, deleteJob, strategy.Timeout); err != nil {
		return err
	}

	// Install new version
	return u.rollingUpgrade(ctx, addon, targetDef, releaseImage, strategy)
}

// blueGreenUpgrade performs a blue-green upgrade.
func (u *Upgrader) blueGreenUpgrade(ctx context.Context, addon *cfov1.ClusterAddon, targetDef *commonv1.AddonDefinition, releaseImage *cfov1.ReleaseImage, strategy *commonv1.AddonUpgradeStrategy) error {
	// Install new version alongside old version
	newAddonName := fmt.Sprintf("%s-green", addon.Name)
	newAddon := addon.DeepCopy()
	newAddon.Name = newAddonName
	newAddon.Spec.AddonName = fmt.Sprintf("%s-green", addon.Spec.AddonName)

	if err := u.rollingUpgrade(ctx, newAddon, targetDef, releaseImage, strategy); err != nil {
		return err
	}

	// Verify new version
	if err := u.verifyAddon(ctx, newAddon); err != nil {
		return err
	}

	// Switch traffic (implementation depends on addon type)
	// For now, delete old version
	deleteJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("delete-%s", addon.Name),
			Namespace: addon.Namespace,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: ptr.To(int32(1)),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					ServiceAccountName: "capbm-addon-installer",
					Containers: []corev1.Container{
						{
							Name:    "delete",
							Image:   "bitnami/kubectl:latest",
							Command: []string{"sh", "-c", fmt.Sprintf("kubectl delete clusteraddon %s", addon.Name)},
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
				},
			},
		},
	}
	return u.client.Create(ctx, deleteJob)
}

// buildUpgradeJob creates a Kubernetes Job for addon upgrade.
func (u *Upgrader) buildUpgradeJob(addon *cfov1.ClusterAddon, targetDef *commonv1.AddonDefinition, strategy *commonv1.AddonUpgradeStrategy) *batchv1.Job {
	retryCount := int32(3)
	timeout := "300s"

	if strategy != nil {
		if strategy.RetryCount > 0 {
			retryCount = int32(strategy.RetryCount)
		}
		if strategy.Timeout != nil {
			timeout = strategy.Timeout.Duration.String()
		}
	}

	var command string
	switch targetDef.Type {
	case commonv1.AddonTypeHelm:
		namespace := addon.Spec.Namespace
		if namespace == "" {
			namespace = "default"
		}
		command = fmt.Sprintf("helm upgrade %s /charts/chart.tgz --namespace %s --values /values/values.yaml --wait --timeout=%s",
			addon.Spec.AddonName, namespace, timeout)
	case commonv1.AddonTypeManifest:
		command = "kubectl apply -f /manifests/addon.yaml"
	}

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("upgrade-%s", addon.Name),
			Namespace: addon.Namespace,
			Labels: map[string]string{
				"capbm.capbm.io/addon": addon.Name,
				"capbm.capbm.io/type":  "upgrade",
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:        ptr.To(retryCount),
			ActiveDeadlineSeconds: ptr.To(int64(parseTimeoutSeconds(timeout))),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					ServiceAccountName: "capbm-addon-installer",
					Containers: []corev1.Container{
						{
							Name:  "upgrade",
							Image: "bitnami/kubectl:latest",
							Command: []string{"sh", "-c", command},
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
				},
			},
		},
	}
}

// backupAddon backs up the current addon state.
func (u *Upgrader) backupAddon(ctx context.Context, addon *cfov1.ClusterAddon, currentDef *commonv1.AddonDefinition, releaseImage *cfov1.ReleaseImage) error {
	backupName := fmt.Sprintf("backup-%s-%d", addon.Name, time.Now().Unix())

	backupConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backupName,
			Namespace: addon.Namespace,
			Labels: map[string]string{
				"capbm.capbm.io/backup-for": addon.Name,
				"capbm.capbm.io/backup-type": "addon",
			},
		},
		Data: map[string]string{
			"version":   currentDef.Version,
			"addonName": addon.Spec.AddonName,
		},
	}

	return u.client.Create(ctx, backupConfigMap)
}

// rollbackAddon rolls back an addon to a previous version.
func (u *Upgrader) rollbackAddon(ctx context.Context, addon *cfov1.ClusterAddon, currentDef *commonv1.AddonDefinition, releaseImage *cfov1.ReleaseImage, targetDef *commonv1.AddonDefinition) error {
	// Execute rollback script
	rollbackJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("rollback-%s", addon.Name),
			Namespace: addon.Namespace,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: ptr.To(int32(1)),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					ServiceAccountName: "capbm-addon-installer",
					Containers: []corev1.Container{
						{
							Name:    "rollback",
							Image:   "bitnami/kubectl:latest",
							Command: []string{"sh", "-c", targetDef.Upgrade.Rollback.Script},
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
				},
			},
		},
	}

	if err := u.client.Create(ctx, rollbackJob); err != nil {
		return err
	}

	timeout := 5 * time.Minute
	if targetDef.Upgrade.Rollback.Timeout != nil {
		timeout = targetDef.Upgrade.Rollback.Timeout.Duration
	}

	return u.waitForJob(ctx, rollbackJob, &metav1.Duration{Duration: timeout})
}

// executeHooks executes a list of hooks.
func (u *Upgrader) executeHooks(ctx context.Context, addon *cfov1.ClusterAddon, hooks []commonv1.AddonHook, phase string) error {
	for _, hook := range hooks {
		if err := u.executeHook(ctx, addon, hook, phase); err != nil {
			switch hook.OnFailure {
			case "Abort":
				return fmt.Errorf("hook %s failed during %s: %w", hook.Name, phase, err)
			case "Ignore":
				continue
			case "Continue":
				continue
			default:
				return fmt.Errorf("hook %s failed during %s: %w", hook.Name, phase, err)
			}
		}
	}
	return nil
}

// executeHook executes a single hook.
func (u *Upgrader) executeHook(ctx context.Context, addon *cfov1.ClusterAddon, hook commonv1.AddonHook, phase string) error {
	timeout := 5 * time.Minute
	if hook.Timeout != nil {
		timeout = hook.Timeout.Duration
	}

	hookCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	hookJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("hook-%s-%s-%d", addon.Name, hook.Name, time.Now().Unix()),
			Namespace: addon.Namespace,
			Labels: map[string]string{
				"capbm.capbm.io/addon": addon.Name,
				"capbm.capbm.io/hook":  hook.Name,
				"capbm.capbm.io/phase": phase,
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: ptr.To(int32(1)),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					ServiceAccountName: "capbm-addon-installer",
					Containers: []corev1.Container{
						{
							Name:    "hook",
							Image:   "bitnami/kubectl:latest",
							Command: []string{"sh", "-c", hook.Command},
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
				},
			},
		},
	}

	return u.client.Create(hookCtx, hookJob)
}

// runHealthCheck runs a health check after upgrade.
func (u *Upgrader) runHealthCheck(ctx context.Context, addon *cfov1.ClusterAddon, hc *commonv1.ComponentUpgradeConfig) error {
	timeout := 5 * time.Minute
	if hc.Timeout != nil {
		timeout = hc.Timeout.Duration
	}

	retries := hc.Retries
	if retries == 0 {
		retries = 3
	}

	var lastErr error
	for i := 0; i < retries; i++ {
		checkCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		checkJob := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("healthcheck-%s-%d", addon.Name, time.Now().Unix()),
				Namespace: addon.Namespace,
			},
			Spec: batchv1.JobSpec{
				BackoffLimit: ptr.To(int32(0)),
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						ServiceAccountName: "capbm-addon-installer",
						Containers: []corev1.Container{
							{
								Name:    "healthcheck",
								Image:   "bitnami/kubectl:latest",
								Command: []string{"sh", "-c", hc.Command},
							},
						},
						RestartPolicy: corev1.RestartPolicyNever,
					},
				},
			},
		}

		if err := u.client.Create(checkCtx, checkJob); err != nil {
			lastErr = err
			continue
		}

		if err := u.waitForJob(checkCtx, checkJob, &metav1.Duration{Duration: timeout}); err != nil {
			lastErr = err
			continue
		}

		return nil
	}

	return fmt.Errorf("health check failed after %d retries: %w", retries, lastErr)
}

// verifyAddon verifies an addon is running.
func (u *Upgrader) verifyAddon(ctx context.Context, addon *cfov1.ClusterAddon) error {
	timeout := 5 * time.Minute
	return wait.PollUntilContextTimeout(ctx, 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		currentAddon := &cfov1.ClusterAddon{}
		if err := u.client.Get(ctx, client.ObjectKeyFromObject(addon), currentAddon); err != nil {
			return false, nil
		}
		return currentAddon.Status.Phase == commonv1.AddonPhaseInstalled, nil
	})
}

// waitForJob waits for a job to complete.
func (u *Upgrader) waitForJob(ctx context.Context, job *batchv1.Job, timeout *metav1.Duration) error {
	if timeout == nil {
		timeout = &metav1.Duration{Duration: 5 * time.Minute}
	}

	return wait.PollUntilContextTimeout(ctx, 5*time.Second, timeout.Duration, true, func(ctx context.Context) (bool, error) {
		currentJob := &batchv1.Job{}
		if err := u.client.Get(ctx, client.ObjectKeyFromObject(job), currentJob); err != nil {
			return false, nil
		}

		for _, cond := range currentJob.Status.Conditions {
			if cond.Type == batchv1.JobComplete && cond.Status == corev1.ConditionTrue {
				return true, nil
			}
			if cond.Type == batchv1.JobFailed && cond.Status == corev1.ConditionTrue {
				return false, fmt.Errorf("job %s failed: %s", job.Name, cond.Message)
			}
		}
		return false, nil
	})
}

// findAddonDefinition finds an addon definition by name in a release.
func findAddonDefinition(release *cfov1.ReleaseImage, name string) *commonv1.AddonDefinition {
	if release == nil {
		return nil
	}
	for i := range release.Spec.Addons {
		if release.Spec.Addons[i].Name == name {
			return &release.Spec.Addons[i]
		}
	}
	return nil
}
