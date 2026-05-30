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

	infrav1 "github.com/BetaWater/cluster-api-provider-baremetal/api/v1beta1"
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
func (i *ManifestInstaller) Install(ctx context.Context, addon *infrav1.ClusterAddon, releaseImage *infrav1.ReleaseImage, addonDef *infrav1.AddonDefinition) error {
	// 1. Validate variables
	if err := ValidateVariables(addon.Spec.Values, addonDef.Variables); err != nil {
		return err
	}

	// 2. Merge default values with user values
	mergedValues := MergeValues(addonDef.DefaultValues, addon.Spec.Values)

	// 3. Fetch manifest content from ReleaseImage
	contentFetcher := NewContentFetcher(i.releaseServer)
	content, err := contentFetcher.FetchFromReleaseImage(ctx, releaseImage, addon.Spec.AddonName)
	if err != nil {
		return err
	}

	// 4. Process manifest with variables
	processor := &ManifestProcessor{}
	processedContent, err := processor.Process(content, mergedValues)
	if err != nil {
		return fmt.Errorf("failed to process manifest template: %w", err)
	}

	// 5. Create ConfigMap to store processed manifest
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

	// 6. Create Job to apply manifest
	job := i.buildApplyJob(addon)
	return i.client.Create(ctx, job)
}

// buildApplyJob creates a Kubernetes Job to apply the manifest.
func (i *ManifestInstaller) buildApplyJob(addon *infrav1.ClusterAddon) *batchv1.Job {
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
			BackoffLimit: ptr.To(int32(3)),
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
func (f *ContentFetcher) FetchFromReleaseImage(ctx context.Context, releaseImage *infrav1.ReleaseImage, addonName string) ([]byte, error) {
	// Find addon definition
	var addonDef *infrav1.AddonDefinition
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
