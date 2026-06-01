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
	"sigs.k8s.io/yaml"

	infrav1 "github.com/BetaWater/cluster-api-provider-baremetal/api/v1beta1"
)

// HelmInstaller installs addons from Helm charts.
type HelmInstaller struct {
	client        client.Client
	releaseServer string
	namespace     string
}

// NewHelmInstaller creates a new helm installer.
func NewHelmInstaller(c client.Client, releaseServer, namespace string) *HelmInstaller {
	return &HelmInstaller{
		client:        c,
		releaseServer: releaseServer,
		namespace:     namespace,
	}
}

// Install installs a helm-based addon.
func (i *HelmInstaller) Install(ctx context.Context, addon *infrav1.ClusterAddon, releaseImage *infrav1.ReleaseImage, addonDef *infrav1.AddonDefinition) error {
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
	valuesYAML, err := yaml.Marshal(mergedValues)
	if err != nil {
		return fmt.Errorf("failed to marshal values: %w", err)
	}

	// 4. Fetch chart content from ReleaseImage
	contentFetcher := NewContentFetcher(i.releaseServer)
	chartContent, err := contentFetcher.FetchFromReleaseImage(ctx, releaseImage, addon.Spec.AddonName)
	if err != nil {
		return err
	}

	// 5. Create ConfigMap to store chart and values
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
	if err := i.client.Create(ctx, configMap); err != nil && !isAlreadyExists(err) {
		return err
	}

	// 6. Create Job to install helm chart
	job := i.buildHelmJob(addon, addonDef.InstallStrategy)
	if err := i.client.Create(ctx, job); err != nil {
		return err
	}

	// 7. Wait for job completion if strategy specifies
	if err := i.waitForJob(ctx, job, addonDef.InstallStrategy); err != nil {
		return err
	}

	// 8. Execute post-hooks
	return i.executeHooks(ctx, addon, addonDef.PostHooks, "post-install")
}

// buildHelmJob creates a Kubernetes Job to install the helm chart.
func (i *HelmInstaller) buildHelmJob(addon *infrav1.ClusterAddon, strategy *infrav1.AddonInstallStrategy) *batchv1.Job {
	namespace := addon.Spec.Namespace
	if namespace == "" {
		namespace = "default"
	}

	timeout := "300s"
	waitFlag := "--wait"
	createNamespaceFlag := "--create-namespace"
	retryCount := int32(3)

	if strategy != nil {
		if strategy.Timeout != nil {
			timeout = strategy.Timeout.Duration.String()
		}
		if !strategy.Wait {
			waitFlag = ""
		}
		if !strategy.CreateNamespace {
			createNamespaceFlag = ""
		}
		if strategy.RetryCount > 0 {
			retryCount = int32(strategy.RetryCount)
		}
	}

	helmArgs := fmt.Sprintf("helm upgrade --install %s /charts/chart.tgz --namespace %s %s --values /values/values.yaml %s --timeout=%s",
		addon.Spec.AddonName, namespace, createNamespaceFlag, waitFlag, timeout)

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("install-%s", addon.Name),
			Namespace: addon.Namespace,
			Labels: map[string]string{
				"capbm.capbm.io/addon": addon.Name,
				"capbm.capbm.io/type":  "helm",
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: ptr.To(retryCount),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					ServiceAccountName: "capbm-addon-installer",
					Containers: []corev1.Container{
						{
							Name:    "helm",
							Image:   "alpine/helm:3.15.0",
							Command: []string{"sh", "-c", helmArgs},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "charts",
									MountPath: "/charts",
								},
								{
									Name:      "values",
									MountPath: "/values",
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "charts",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: fmt.Sprintf("%s-helm", addon.Name),
									},
								},
							},
						},
						{
							Name: "values",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: fmt.Sprintf("%s-helm", addon.Name),
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

// executeHooks executes a list of hooks.
func (i *HelmInstaller) executeHooks(ctx context.Context, addon *infrav1.ClusterAddon, hooks []infrav1.AddonHook, phase string) error {
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
func (i *HelmInstaller) executeHook(ctx context.Context, addon *infrav1.ClusterAddon, hook infrav1.AddonHook, phase string) error {
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
func (i *HelmInstaller) waitForJob(ctx context.Context, job *batchv1.Job, strategy *infrav1.AddonInstallStrategy) error {
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
