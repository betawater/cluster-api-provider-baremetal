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

package registry

import (
	"context"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infrav1 "github.com/BetaWater/cluster-api-provider-baremetal/api/v1beta1"
)

// Import status constants.
const (
	ImportStatusPending   = "pending"
	ImportStatusRunning   = "running"
	ImportStatusCompleted = "completed"
	ImportStatusFailed    = "failed"
)

// StatusTracker tracks image import status.
type StatusTracker struct {
	client client.Client
}

// NewStatusTracker creates a new status tracker.
func NewStatusTracker(c client.Client) *StatusTracker {
	return &StatusTracker{
		client: c,
	}
}

// UpdateReleaseImageStatus updates the ReleaseImage status based on the import Job status.
func (t *StatusTracker) UpdateReleaseImageStatus(ctx context.Context, releaseImage *infrav1.ReleaseImage) error {
	if releaseImage.Spec.ImageRegistry == nil || !releaseImage.Spec.ImageRegistry.Enabled {
		return nil
	}

	jobName := fmt.Sprintf("import-images-%s", releaseImage.Spec.Version)
	job := &batchv1.Job{}
	err := t.client.Get(ctx, types.NamespacedName{
		Name:      jobName,
		Namespace: releaseImage.Namespace,
	}, job)

	if err != nil {
		// Job doesn't exist yet, set pending status
		releaseImage.Status.ImagesImported = false
		releaseImage.Status.ImportStatus = ImportStatusPending
		releaseImage.Status.ImportMessage = "Import job not yet created"
		return nil
	}

	// Update status based on job status
	releaseImage.Status.ImportJobName = jobName

	if job.Status.Succeeded > 0 {
		releaseImage.Status.ImagesImported = true
		releaseImage.Status.ImportStatus = ImportStatusCompleted
		releaseImage.Status.ImportMessage = "All images imported successfully"
	} else if job.Status.Failed > 0 {
		releaseImage.Status.ImagesImported = false
		releaseImage.Status.ImportStatus = ImportStatusFailed
		if len(job.Status.Conditions) > 0 {
			releaseImage.Status.ImportMessage = job.Status.Conditions[0].Message
		}
	} else {
		releaseImage.Status.ImagesImported = false
		releaseImage.Status.ImportStatus = ImportStatusRunning
		releaseImage.Status.ImportMessage = fmt.Sprintf("Import in progress: %d active pods", job.Status.Active)
	}

	// Update imported images status
	releaseImage.Status.ImportedImages = t.buildImportedImageStatus(releaseImage, job)

	return nil
}

// buildImportedImageStatus builds the imported image status list.
func (t *StatusTracker) buildImportedImageStatus(releaseImage *infrav1.ReleaseImage, job *batchv1.Job) []infrav1.ImportedImageStatus {
	var statuses []infrav1.ImportedImageStatus

	registryConfig := releaseImage.Spec.ImageRegistry
	if registryConfig == nil {
		return statuses
	}

	// Build status for each component with images
	components := []struct {
		name      string
		imageList []string
		version   string
	}{
		{"kubernetes", releaseImage.Spec.Components.Kubernetes.ImageList, releaseImage.Spec.Components.Kubernetes.Version},
		{"calico", releaseImage.Spec.Components.Calico.ImageList, releaseImage.Spec.Components.Calico.Version},
		{"cilium", releaseImage.Spec.Components.Cilium.ImageList, releaseImage.Spec.Components.Cilium.Version},
		{"ceph-csi", releaseImage.Spec.Components.CephCsi.ImageList, releaseImage.Spec.Components.CephCsi.Version},
		{"envoy-gateway", releaseImage.Spec.Components.EnvoyGateway.ImageList, releaseImage.Spec.Components.EnvoyGateway.Version},
		{"metallb", releaseImage.Spec.Components.MetalLB.ImageList, releaseImage.Spec.Components.MetalLB.Version},
	}

	for _, comp := range components {
		if len(comp.imageList) == 0 {
			continue
		}

		for _, image := range comp.imageList {
			imageName := image[:len(image)-4] // Remove .tar extension
			targetRef := fmt.Sprintf("%s/%s/%s/%s:%s",
				registryConfig.Registry,
				registryConfig.Repository,
				registryConfig.ImagePrefix,
				comp.name,
				comp.version,
			)

			status := infrav1.ImportedImageStatus{
				Component: comp.name,
				Image:     imageName,
				TargetRef: targetRef,
			}

			if job.Status.Succeeded > 0 {
				status.Status = "imported"
			} else if job.Status.Failed > 0 {
				status.Status = "failed"
			} else {
				status.Status = "pending"
			}

			statuses = append(statuses, status)
		}
	}

	return statuses
}

// GetImportStatus returns the current import status for a ReleaseImage.
func (t *StatusTracker) GetImportStatus(ctx context.Context, releaseImage *infrav1.ReleaseImage) (*infrav1.ReleaseImageStatus, error) {
	status := &infrav1.ReleaseImageStatus{}

	if releaseImage.Spec.ImageRegistry == nil || !releaseImage.Spec.ImageRegistry.Enabled {
		status.ImagesImported = false
		status.ImportStatus = "disabled"
		status.ImportMessage = "Image registry import is not enabled"
		return status, nil
	}

	jobName := fmt.Sprintf("import-images-%s", releaseImage.Spec.Version)
	job := &batchv1.Job{}
	err := t.client.Get(ctx, types.NamespacedName{
		Name:      jobName,
		Namespace: releaseImage.Namespace,
	}, job)

	if err != nil {
		status.ImagesImported = false
		status.ImportStatus = ImportStatusPending
		status.ImportMessage = "Import job not yet created"
		return status, nil
	}

	status.ImportJobName = jobName

	if job.Status.Succeeded > 0 {
		status.ImagesImported = true
		status.ImportStatus = ImportStatusCompleted
		status.ImportMessage = "All images imported successfully"
	} else if job.Status.Failed > 0 {
		status.ImagesImported = false
		status.ImportStatus = ImportStatusFailed
		if len(job.Status.Conditions) > 0 {
			status.ImportMessage = job.Status.Conditions[0].Message
		}
	} else {
		status.ImagesImported = false
		status.ImportStatus = ImportStatusRunning
		status.ImportMessage = fmt.Sprintf("Import in progress: %d active pods", job.Status.Active)
	}

	return status, nil
}
