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
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cfov1 "github.com/BetaWater/cluster-api-provider-baremetal/modules/cvo/api/v1beta1"
)

const (
	// LabelReleaseVersion is the label key for release version.
	LabelReleaseVersion = "capbm.capbm.io/release-version"
	// LabelImportType is the label key for import type.
	LabelImportType = "capbm.capbm.io/type"

	// JobTypeImageImport is the job type for image import.
	JobTypeImageImport = "image-import"

	// ImageImporterServiceAccount is the service account for image import.
	ImageImporterServiceAccount = "capbm-image-importer"
	// ImageImporterImage is the default image importer image (contains ctr command).
	ImageImporterImage = "registry.k8s.io/cri-tools:latest"
	// ContainerdNamespace is the containerd namespace for Kubernetes.
	ContainerdNamespace = "k8s.io"
	// ContainerdSocketPath is the path to the containerd socket.
	ContainerdSocketPath = "/run/containerd/containerd.sock"
	// HostsDirPath is the path to containerd hosts.d directory.
	HostsDirPath = "/etc/containerd/hosts.d"
)

// Importer imports images from ReleaseImage to target registry using containerd (ctr).
type Importer struct {
	client        client.Client
	releaseServer string
	namespace     string
}

// NewImporter creates a new image importer.
func NewImporter(c client.Client, releaseServer, namespace string) *Importer {
	return &Importer{
		client:        c,
		releaseServer: strings.TrimRight(releaseServer, "/"),
		namespace:     namespace,
	}
}

// ImportImages creates a Job to import images from ReleaseImage to target registry.
func (i *Importer) ImportImages(ctx context.Context, releaseImage *cfov1.ReleaseImage) error {
	if releaseImage.Spec.ImageRegistry == nil || !releaseImage.Spec.ImageRegistry.Enabled {
		return nil
	}

	// Check if import job already exists
	job := &batchv1.Job{}
	err := i.client.Get(ctx, types.NamespacedName{
		Name:      fmt.Sprintf("import-images-%s", releaseImage.Spec.Version),
		Namespace: i.namespace,
	}, job)
	if err == nil {
		// Job exists, check if it's for the same version
		if job.Labels[LabelReleaseVersion] == releaseImage.Spec.Version {
			return nil
		}
		// Different version, delete old job
		if err := i.client.Delete(ctx, job); err != nil {
			return fmt.Errorf("failed to delete old import job: %w", err)
		}
	}

	// Create new import job
	newJob := i.buildImportJob(releaseImage)
	return i.client.Create(ctx, newJob)
}

// buildImportJob builds an image import Job using containerd (ctr).
func (i *Importer) buildImportJob(releaseImage *cfov1.ReleaseImage) *batchv1.Job {
	registryConfig := releaseImage.Spec.ImageRegistry

	// Build image list for all components and addons
	var componentList []string
	var imageLists []string

	// Collect components that have images
	if len(releaseImage.Spec.Components.Kubernetes.ImageList) > 0 {
		componentList = append(componentList, "kubernetes")
		imageLists = append(imageLists, fmt.Sprintf("kubernetes:%s", strings.Join(releaseImage.Spec.Components.Kubernetes.ImageList, ",")))
	}

	// Collect addons that have images
	for _, addon := range releaseImage.Spec.Addons {
		// Build addon image list from addon definition
		addonImages := buildAddonImageList(addon)
		if len(addonImages) > 0 {
			componentList = append(componentList, addon.Name)
			imageLists = append(imageLists, fmt.Sprintf("%s:%s", addon.Name, strings.Join(addonImages, ",")))
		}
	}

	// Build environment variables
	envVars := []corev1.EnvVar{
		{
			Name:  "RELEASE_SERVER",
			Value: i.releaseServer,
		},
		{
			Name:  "REGISTRY",
			Value: registryConfig.Registry,
		},
		{
			Name:  "REPOSITORY",
			Value: registryConfig.Repository,
		},
		{
			Name:  "IMAGE_PREFIX",
			Value: registryConfig.ImagePrefix,
		},
		{
			Name:  "INSECURE",
			Value: fmt.Sprintf("%t", registryConfig.InsecureSkipVerify),
		},
		{
			Name:  "COMPONENTS",
			Value: strings.Join(componentList, ","),
		},
		{
			Name:  "IMAGE_LISTS",
			Value: strings.Join(imageLists, ";"),
		},
	}

	// Add credentials if configured
	if registryConfig.CredentialsSecret != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name: "REGISTRY_USER",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: registryConfig.CredentialsSecret,
					},
					Key: "username",
				},
			},
		})
		envVars = append(envVars, corev1.EnvVar{
			Name: "REGISTRY_PASSWORD",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: registryConfig.CredentialsSecret,
					},
					Key: "password",
				},
			},
		})
	}

	// Volume mounts for containerd socket and hosts.d
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "containerd-socket",
			MountPath: ContainerdSocketPath,
		},
		{
			Name:      "hosts-dir",
			MountPath: HostsDirPath,
		},
	}

	// Add CA certificate volume if configured
	if registryConfig.CAConfigMap != "" {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "registry-ca",
			MountPath: "/etc/containerd/certs.d",
			ReadOnly:  true,
		})
	}

	// Volumes
	volumes := []corev1.Volume{
		{
			Name: "containerd-socket",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: ContainerdSocketPath,
					Type: ptr.To(corev1.HostPathSocket),
				},
			},
		},
		{
			Name: "hosts-dir",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}

	// Add CA certificate volume if configured
	if registryConfig.CAConfigMap != "" {
		volumes = append(volumes, corev1.Volume{
			Name: "registry-ca",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: registryConfig.CAConfigMap,
					},
				},
			},
		})
	}

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("import-images-%s", releaseImage.Spec.Version),
			Namespace: i.namespace,
			Labels: map[string]string{
				LabelReleaseVersion: releaseImage.Spec.Version,
				LabelImportType:     JobTypeImageImport,
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:          ptr.To(int32(3)),
			ActiveDeadlineSeconds: ptr.To(int64(3600)),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					ServiceAccountName: ImageImporterServiceAccount,
					Containers: []corev1.Container{
						{
							Name:    "image-importer",
							Image:   ImageImporterImage,
							Command: []string{"sh", "-c", i.buildImportScript()},
							Env:     envVars,
							SecurityContext: &corev1.SecurityContext{
								Privileged: ptr.To(false),
								Capabilities: &corev1.Capabilities{
									Add: []corev1.Capability{"SYS_CHROOT"},
								},
							},
							VolumeMounts: volumeMounts,
						},
					},
					Volumes:       volumes,
					RestartPolicy: corev1.RestartPolicyNever,
				},
			},
		},
	}
}

// buildImportScript builds the image import script using containerd (ctr).
func (i *Importer) buildImportScript() string {
	return `#!/bin/sh
set -euo pipefail

RELEASE_SERVER="${RELEASE_SERVER}"
REGISTRY="${REGISTRY}"
REPOSITORY="${REPOSITORY}"
IMAGE_PREFIX="${IMAGE_PREFIX}"
REGISTRY_USER="${REGISTRY_USER:-}"
REGISTRY_PASSWORD="${REGISTRY_PASSWORD:-}"
INSECURE="${INSECURE:-false}"
COMPONENTS="${COMPONENTS}"
IMAGE_LISTS="${IMAGE_LISTS}"
CONTAINERD_NS="k8s.io"
HOSTS_DIR="/etc/containerd/hosts.d"

echo "=== Starting image import to $REGISTRY/$REPOSITORY ==="

# Configure containerd hosts.d for registry authentication
mkdir -p "${HOSTS_DIR}/${REGISTRY}"

# Generate hosts.toml
cat > "${HOSTS_DIR}/${REGISTRY}/hosts.toml" << EOF
server = "https://${REGISTRY}"

[host."https://${REGISTRY}"]
  capabilities = ["pull", "resolve", "push"]
EOF

# Add authentication if credentials provided
if [ -n "$REGISTRY_USER" ] && [ -n "$REGISTRY_PASSWORD" ]; then
  AUTH=$(echo -n "${REGISTRY_USER}:${REGISTRY_PASSWORD}" | base64)
  cat >> "${HOSTS_DIR}/${REGISTRY}/hosts.toml" << EOF

[host."https://${REGISTRY}".header]
  Authorization = "Basic ${AUTH}"
EOF
fi

# Add insecure skip verify if enabled
if [ "$INSECURE" = "true" ]; then
  cat >> "${HOSTS_DIR}/${REGISTRY}/hosts.toml" << EOF

[host."https://${REGISTRY}"]
  skip_verify = true
EOF
fi

# Parse components and image lists
IFS=',' read -ra COMPONENT_ARRAY <<< "$COMPONENTS"
IFS=';' read -ra IMAGE_LIST_ARRAY <<< "$IMAGE_LISTS"

# Build component to images mapping
declare -A COMPONENT_IMAGES
for entry in "${IMAGE_LIST_ARRAY[@]}"; do
  component="${entry%%:*}"
  images="${entry#*:}"
  COMPONENT_IMAGES[$component]="$images"
done

# Import images for each component
for component in "${COMPONENT_ARRAY[@]}"; do
  echo "=== Importing $component images ==="
  
  # Get component version from index.json
  VERSION=$(curl -fsSL "${RELEASE_SERVER}/index.json" | jq -r ".components.\"$component\".version")
  if [ "$VERSION" = "null" ] || [ -z "$VERSION" ]; then
    echo "Skipping $component (not in release)"
    continue
  fi
  
  # Get image list
  IMAGES="${COMPONENT_IMAGES[$component]}"
  if [ -z "$IMAGES" ]; then
    echo "Skipping $component (no images)"
    continue
  fi
  
  IFS=',' read -ra IMAGE_ARRAY <<< "$IMAGES"
  
  for image_tar in "${IMAGE_ARRAY[@]}"; do
    echo "Processing $image_tar..."
    
    # Download tar
    curl -fsSL "${RELEASE_SERVER}/images/${component}/${VERSION}/${image_tar}" -o "/tmp/${image_tar}"
    
    # Import to containerd
    ctr -n "$CONTAINERD_NS" images import "/tmp/${image_tar}"
    
    # Get image name from tar filename (remove .tar extension)
    IMAGE_NAME=$(echo "$image_tar" | sed 's/\.tar$//')
    
    # Tag image
    TARGET_IMAGE="${REGISTRY}/${REPOSITORY}/${IMAGE_PREFIX}/${component}/${IMAGE_NAME}:${VERSION}"
    ctr -n "$CONTAINERD_NS" images tag "${IMAGE_NAME}" "$TARGET_IMAGE"
    
    # Push to registry using hosts.d for authentication
    ctr -n "$CONTAINERD_NS" images push \
      --hosts-dir "$HOSTS_DIR" \
      "$TARGET_IMAGE"
    
    echo "Pushed: $TARGET_IMAGE"
    rm -f "/tmp/${image_tar}"
  done
  
  echo "=== $component images imported ==="
done

echo "=== All images imported successfully ==="
`
}
