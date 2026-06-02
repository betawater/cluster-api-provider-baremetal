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
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cfov1 "github.com/BetaWater/cluster-api-provider-baremetal/modules/cvo/api/v1beta1"
)

// ManifestInstaller installs addons from manifest templates.
type ManifestInstaller struct {
	client        client.Client
	releaseServer string
	namespace     string
}

// NewManifestInstaller creates a new manifest installer.
func NewManifestInstaller(c client.Client, releaseServer, namespace string) *ManifestInstaller {
	return &ManifestInstaller{
		client:        c,
		releaseServer: releaseServer,
		namespace:     namespace,
	}
}

// Install installs a manifest-based addon.
func (i *ManifestInstaller) Install(ctx context.Context, addon *cfov1.ClusterAddon, releaseImage *cfov1.ReleaseImage, addonDef *cfov1.AddonDefinition) error {
	// 1. Validate variables
	if err := ValidateVariables(addon.Spec.Values, addonDef.Variables); err != nil {
		return err
	}

	// 2. Execute pre-hooks
	if err := i.executeHooks(ctx, addon, addonDef.PreHooks, "pre-install"); err != nil {
		return fmt.Errorf("pre-install hooks failed: %w", err)
	}

	// 3. Merge default values with user values
	mergedValues := MergeValues(addonDef.DefaultValues, addon.Spec.Values)

	// 4. Fetch manifest content from ReleaseImage
	contentFetcher := NewContentFetcher(i.releaseServer)
	content, err := contentFetcher.FetchFromReleaseImage(ctx, releaseImage, addon.Spec.AddonName)
	if err != nil {
		return err
	}

	// 5. Process manifest with variables
	processor := &ManifestProcessor{}
	processedContent, err := processor.Process(content, mergedValues)
	if err != nil {
		return fmt.Errorf("failed to process manifest template: %w", err)
	}

	// 6. Create ConfigMap to store processed manifest
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-manifest", addon.Name),
			Namespace: addon.Namespace,
		},
		Data: map[string]string{
			"addon.yaml": string(processedContent),
		},
	}
	if err := i.client.Create(ctx, configMap); err != nil && !isAlreadyExists(err) {
		return err
	}

	// 7. Create Job to apply manifest
	job := i.buildApplyJob(addon, addonDef.InstallStrategy)
	if err := i.client.Create(ctx, job); err != nil {
		return err
	}

	// 8. Wait for job completion if strategy specifies
	if err := i.waitForJob(ctx, job, addonDef.InstallStrategy); err != nil {
		return err
	}

	// 9. Execute post-hooks
	return i.executeHooks(ctx, addon, addonDef.PostHooks, "post-install")
}

// buildApplyJob creates a Kubernetes Job to apply the manifest.
func (i *ManifestInstaller) buildApplyJob(addon *cfov1.ClusterAddon, strategy *cfov1.AddonInstallStrategy) *batchv1.Job {
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

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("install-%s", addon.Name),
			Namespace: addon.Namespace,
			Labels: map[string]string{
				"capbm.capbm.io/addon": addon.Name,
				"capbm.capbm.io/type":  "manifest",
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: ptr.To(retryCount),
			ActiveDeadlineSeconds: ptr.To(int64(parseTimeoutSeconds(timeout))),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					ServiceAccountName: "capbm-addon-installer",
					Containers: []corev1.Container{
						{
							Name:    "kubectl",
							Image:   "bitnami/kubectl:latest",
							Command: []string{"sh", "-c", "kubectl apply -f /manifests/addon.yaml"},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "manifests",
									MountPath: "/manifests",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "manifests",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: fmt.Sprintf("%s-manifest", addon.Name),
									},
								},
							},
						},
					},
					RestartPolicy: corev1.RestartPolicyNever,
				},
			},
		},
	}
}

func isAlreadyExists(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(interface{ IsAlreadyExists() bool })
	return ok
}

// executeHooks executes a list of hooks.
func (i *ManifestInstaller) executeHooks(ctx context.Context, addon *cfov1.ClusterAddon, hooks []cfov1.AddonHook, phase string) error {
	for _, hook := range hooks {
		if err := i.executeHook(ctx, addon, hook, phase); err != nil {
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
func (i *ManifestInstaller) executeHook(ctx context.Context, addon *cfov1.ClusterAddon, hook cfov1.AddonHook, phase string) error {
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

	return i.client.Create(hookCtx, hookJob)
}

// waitForJob waits for a job to complete.
func (i *ManifestInstaller) waitForJob(ctx context.Context, job *batchv1.Job, strategy *cfov1.AddonInstallStrategy) error {
	if strategy == nil || !strategy.Wait {
		return nil
	}

	timeout := 5 * time.Minute
	if strategy.Timeout != nil {
		timeout = strategy.Timeout.Duration
	}

	return wait.PollUntilContextTimeout(ctx, 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		currentJob := &batchv1.Job{}
		if err := i.client.Get(ctx, client.ObjectKeyFromObject(job), currentJob); err != nil {
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

// parseTimeoutSeconds parses a timeout string like "300s" to seconds.
func parseTimeoutSeconds(timeout string) int {
	var seconds int
	fmt.Sscanf(timeout, "%ds", &seconds)
	if seconds == 0 {
		seconds = 300
	}
	return seconds
}

// ContentFetcher fetches addon content from ReleaseImage.
type ContentFetcher struct {
	releaseServer string
}

// NewContentFetcher creates a new content fetcher.
func NewContentFetcher(releaseServer string) *ContentFetcher {
	return &ContentFetcher{
		releaseServer: releaseServer,
	}
}

// FetchFromReleaseImage fetches addon content from the ReleaseImage HTTP server.
func (f *ContentFetcher) FetchFromReleaseImage(ctx context.Context, releaseImage *cfov1.ReleaseImage, addonName string) ([]byte, error) {
	// Find addon definition
	var addonDef *cfov1.AddonDefinition
	for i := range releaseImage.Spec.Addons {
		if releaseImage.Spec.Addons[i].Name == addonName {
			addonDef = &releaseImage.Spec.Addons[i]
			break
		}
	}
	if addonDef == nil {
		return nil, fmt.Errorf("addon %s not found in release image %s", addonName, releaseImage.Name)
	}

	// Build URL
	url := fmt.Sprintf("%s/%s", f.releaseServer, addonDef.ContentPath)

	// In production, use http.Client to fetch the content
	// This is a placeholder - actual implementation would use http.Get
	_ = url

	return nil, fmt.Errorf("content fetcher not fully implemented")
}
