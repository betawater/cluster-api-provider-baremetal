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

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	// 2. Merge default values with user values
	mergedValues := MergeValues(addonDef.DefaultValues, addon.Spec.Values)
	valuesYAML, err := yaml.Marshal(mergedValues)
	if err != nil {
		return fmt.Errorf("failed to marshal values: %w", err)
	}

	// 3. Fetch chart content from ReleaseImage
	contentFetcher := NewContentFetcher(i.releaseServer)
	chartContent, err := contentFetcher.FetchFromReleaseImage(ctx, releaseImage, addon.Spec.AddonName)
	if err != nil {
		return err
	}

	// 4. Create ConfigMap to store chart and values
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

	// 5. Create Job to install helm chart
	job := i.buildHelmJob(addon)
	return i.client.Create(ctx, job)
}

// buildHelmJob creates a Kubernetes Job to install the helm chart.
func (i *HelmInstaller) buildHelmJob(addon *infrav1.ClusterAddon) *batchv1.Job {
	namespace := addon.Spec.Namespace
	if namespace == "" {
		namespace = "default"
	}

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
			BackoffLimit: ptr.To(int32(3)),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					ServiceAccountName: "capbm-addon-installer",
					Containers: []corev1.Container{
						{
							Name:    "helm",
							Image:   "alpine/helm:3.15.0",
							Command: []string{"sh", "-c", fmt.Sprintf(
								"helm upgrade --install %s /charts/chart.tgz --namespace %s --create-namespace --values /values/values.yaml --wait --timeout=300s",
								addon.Spec.AddonName, namespace,
							)},
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
