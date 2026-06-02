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

package component

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

	"github.com/BetaWater/cluster-api-provider-baremetal/modules/capbm/internal/helm"
)

const (
	// LabelComponent is the label key for component name.
	LabelComponent = "capbm.capbm.io/component"
	// LabelVersion is the label key for component version.
	LabelVersion = "capbm.capbm.io/version"
	// LabelType is the label key for job type.
	LabelType = "capbm.capbm.io/type"
	// LabelAction is the label key for action (install/upgrade).
	LabelAction = "capbm.capbm.io/action"

	// JobTypeHelmInstall is the job type for Helm installation.
	JobTypeHelmInstall = "helm-install"
	// JobTypeHelmUpgrade is the job type for Helm upgrade.
	JobTypeHelmUpgrade = "helm-upgrade"
	// JobTypeManifestInstall is the job type for Manifest installation.
	JobTypeManifestInstall = "manifest-install"
	// JobTypeManifestUpgrade is the job type for Manifest upgrade.
	JobTypeManifestUpgrade = "manifest-upgrade"

	// KubectlImage is the default kubectl image.
	KubectlImage = "bitnami/kubectl:1.31.0"
	// ManifestServiceAccountName is the name of the Manifest service account.
	ManifestServiceAccountName = "capbm-manifest"
)

// ComponentType represents the type of component installation.
type ComponentType string

const (
	ComponentTypeHelm     ComponentType = "helm"
	ComponentTypeManifest ComponentType = "manifest"
	ComponentTypeBinary   ComponentType = "binary"
)

// Component defines a component to be installed.
type Component struct {
	// Name is the component name.
	Name string
	// Type is the installation type (helm/manifest/binary).
	Type ComponentType
	// Version is the component version.
	Version string
	// Path is the path in the release image.
	Path string
	// Namespace is the target namespace.
	Namespace string
	// ImageList is the list of image tar files to load.
	ImageList []string
	// HelmValues is the helm --set values (for Helm components).
	HelmValues map[string]string
	// ManifestFiles is the list of manifest files to apply (for Manifest components).
	ManifestFiles []string
}

// Installer installs components via Jobs.
type Installer struct {
	client        client.Client
	releaseServer string
	namespace     string
}

// NewInstaller creates a new component installer.
func NewInstaller(c client.Client, releaseServer, namespace string) *Installer {
	return &Installer{
		client:        c,
		releaseServer: strings.TrimRight(releaseServer, "/"),
		namespace:     namespace,
	}
}

// InstallComponent creates a Job to install a component.
func (i *Installer) InstallComponent(ctx context.Context, component Component) error {
	// Check if job already exists
	job := &batchv1.Job{}
	err := i.client.Get(ctx, types.NamespacedName{
		Name:      fmt.Sprintf("install-%s", component.Name),
		Namespace: i.namespace,
	}, job)
	if err == nil {
		// Job exists, check if it's for the same version
		if job.Labels[LabelVersion] == component.Version {
			return nil
		}
		// Different version, delete old job
		if err := i.client.Delete(ctx, job); err != nil {
			return fmt.Errorf("failed to delete old job: %w", err)
		}
	}

	// Create new job based on component type
	var newJob *batchv1.Job
	switch component.Type {
	case ComponentTypeHelm:
		newJob = i.buildHelmInstallJob(component)
	case ComponentTypeManifest:
		newJob = i.buildManifestInstallJob(component)
	default:
		return fmt.Errorf("unsupported component type: %s", component.Type)
	}

	return i.client.Create(ctx, newJob)
}

// UpgradeComponent creates a Job to upgrade a component.
func (i *Installer) UpgradeComponent(ctx context.Context, component Component) error {
	// Create upgrade job based on component type
	var job *batchv1.Job
	switch component.Type {
	case ComponentTypeHelm:
		job = i.buildHelmUpgradeJob(component)
	case ComponentTypeManifest:
		job = i.buildManifestUpgradeJob(component)
	default:
		return fmt.Errorf("unsupported component type: %s", component.Type)
	}

	return i.client.Create(ctx, job)
}

// buildHelmInstallJob builds a Helm install Job.
func (i *Installer) buildHelmInstallJob(component Component) *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("install-%s", component.Name),
			Namespace: i.namespace,
			Labels: map[string]string{
				LabelComponent: component.Name,
				LabelVersion:   component.Version,
				LabelType:      JobTypeHelmInstall,
				LabelAction:    "install",
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:          ptr.To(int32(3)),
			ActiveDeadlineSeconds: ptr.To(int64(600)),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						LabelComponent: component.Name,
						LabelVersion:   component.Version,
					},
				},
				Spec: i.buildHelmPodSpec(component, "install"),
			},
		},
	}
}

// buildHelmUpgradeJob builds a Helm upgrade Job.
func (i *Installer) buildHelmUpgradeJob(component Component) *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("upgrade-%s", component.Name),
			Namespace: i.namespace,
			Labels: map[string]string{
				LabelComponent: component.Name,
				LabelVersion:   component.Version,
				LabelType:      JobTypeHelmUpgrade,
				LabelAction:    "upgrade",
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:          ptr.To(int32(3)),
			ActiveDeadlineSeconds: ptr.To(int64(600)),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						LabelComponent: component.Name,
						LabelVersion:   component.Version,
					},
				},
				Spec: i.buildHelmPodSpec(component, "upgrade"),
			},
		},
	}
}

// buildManifestInstallJob builds a Manifest install Job.
func (i *Installer) buildManifestInstallJob(component Component) *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("install-%s", component.Name),
			Namespace: i.namespace,
			Labels: map[string]string{
				LabelComponent: component.Name,
				LabelVersion:   component.Version,
				LabelType:      JobTypeManifestInstall,
				LabelAction:    "install",
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:          ptr.To(int32(3)),
			ActiveDeadlineSeconds: ptr.To(int64(600)),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						LabelComponent: component.Name,
						LabelVersion:   component.Version,
					},
				},
				Spec: i.buildManifestPodSpec(component, "install"),
			},
		},
	}
}

// buildManifestUpgradeJob builds a Manifest upgrade Job.
func (i *Installer) buildManifestUpgradeJob(component Component) *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("upgrade-%s", component.Name),
			Namespace: i.namespace,
			Labels: map[string]string{
				LabelComponent: component.Name,
				LabelVersion:   component.Version,
				LabelType:      JobTypeManifestUpgrade,
				LabelAction:    "upgrade",
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:          ptr.To(int32(3)),
			ActiveDeadlineSeconds: ptr.To(int64(600)),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						LabelComponent: component.Name,
						LabelVersion:   component.Version,
					},
				},
				Spec: i.buildManifestPodSpec(component, "upgrade"),
			},
		},
	}
}

// buildHelmPodSpec builds the Pod spec for Helm Jobs.
func (i *Installer) buildHelmPodSpec(component Component, action string) corev1.PodSpec {
	return corev1.PodSpec{
		ServiceAccountName: helm.ServiceAccountName,
		Containers: []corev1.Container{
			{
				Name:    "helm",
				Image:   helm.HelmImage,
				Command: []string{"sh", "-c", i.buildHelmScript(component, action)},
				Env:     i.buildEnvVars(component),
			},
		},
		RestartPolicy: corev1.RestartPolicyNever,
		Tolerations: []corev1.Toleration{
			{
				Key:      "node-role.kubernetes.io/control-plane",
				Operator: corev1.TolerationOpExists,
				Effect:   corev1.TaintEffectNoSchedule,
			},
		},
	}
}

// buildManifestPodSpec builds the Pod spec for Manifest Jobs.
func (i *Installer) buildManifestPodSpec(component Component, action string) corev1.PodSpec {
	return corev1.PodSpec{
		ServiceAccountName: ManifestServiceAccountName,
		Containers: []corev1.Container{
			{
				Name:    "kubectl",
				Image:   KubectlImage,
				Command: []string{"sh", "-c", i.buildManifestScript(component, action)},
				Env:     i.buildEnvVars(component),
			},
		},
		RestartPolicy: corev1.RestartPolicyNever,
		Tolerations: []corev1.Toleration{
			{
				Key:      "node-role.kubernetes.io/control-plane",
				Operator: corev1.TolerationOpExists,
				Effect:   corev1.TaintEffectNoSchedule,
			},
		},
	}
}

// buildHelmScript builds the Helm installation script.
func (i *Installer) buildHelmScript(component Component, action string) string {
	// Build helm values string
	var valuesArgs string
	for k, v := range component.HelmValues {
		valuesArgs += fmt.Sprintf(" --set %s=%s", k, v)
	}

	// Build image loading script
	var imageLoadScript string
	if len(component.ImageList) > 0 {
		imageLoadScript = fmt.Sprintf(`
          # Load container images (offline mode)
          if [ "${INSTALL_SOURCE}" != "online" ]; then
            echo "Loading images for $COMPONENT..."
            IMAGE_PATH="${RELEASE_SERVER}/images/${COMPONENT}/${VERSION}"
            for image_tar in %s; do
              curl -fsSL "${IMAGE_PATH}/${image_tar}" -o "/tmp/${image_tar}"
              ctr -n k8s.io images import "/tmp/${image_tar}"
              rm -f "/tmp/${image_tar}"
            done
          fi`, strings.Join(component.ImageList, " "))
	}

	// Build update status script
	updateStatusScript := fmt.Sprintf(`
          # Update status ConfigMap
          kubectl patch configmap %s -n %s \
            --type merge \
            -p '{"data":{"%s":"version: ${VERSION}\nstatus: installed\ninstalledAt: $(date -u +%%Y-%%m-%%dT%%H:%%M:%%SZ)\njobName: %s-${COMPONENT}"}}'`,
		helm.StatusConfigMapName, i.namespace, component.Name, action)

	return fmt.Sprintf(`#!/bin/sh
set -euo pipefail

COMPONENT="%s"
VERSION="%s"
RELEASE_SERVER="${RELEASE_SERVER}"
NAMESPACE="%s"
COMPONENT_PATH="%s"
INSTALL_SOURCE="${INSTALL_SOURCE:-online}"

echo "=== %s $COMPONENT v$VERSION ==="

# 1. Download chart package
CHART_URL="${RELEASE_SERVER}/${COMPONENT_PATH}/${VERSION}/${COMPONENT}.tgz"
CHART_PATH="/tmp/${COMPONENT}.tgz"
curl -fsSL "$CHART_URL" -o "$CHART_PATH"
%s
# 2. Install/Upgrade component
echo "Installing $COMPONENT via Helm..."
helm %s "$COMPONENT" "$CHART_PATH" \
  --namespace "$NAMESPACE" \
  --create-namespace \
  --wait \
  --timeout=300s%s

echo "=== $COMPONENT v$VERSION installed successfully ==="
%s
`,
		component.Name,
		component.Version,
		i.namespace,
		component.Path,
		strings.Title(action),
		imageLoadScript,
		action,
		valuesArgs,
		updateStatusScript,
	)
}

// buildManifestScript builds the Manifest installation script.
func (i *Installer) buildManifestScript(component Component, action string) string {
	// Build manifest apply commands
	var manifestCommands string
	for _, manifestFile := range component.ManifestFiles {
		manifestCommands += fmt.Sprintf(`
          echo "Applying %s..."
          kubectl apply -f "/tmp/%s"
`, manifestFile, manifestFile)
	}

	// Build image loading script
	var imageLoadScript string
	if len(component.ImageList) > 0 {
		imageLoadScript = fmt.Sprintf(`
          # Load container images (offline mode)
          if [ "${INSTALL_SOURCE}" != "online" ]; then
            echo "Loading images for $COMPONENT..."
            IMAGE_PATH="${RELEASE_SERVER}/images/${COMPONENT}/${VERSION}"
            for image_tar in %s; do
              curl -fsSL "${IMAGE_PATH}/${image_tar}" -o "/tmp/${image_tar}"
              ctr -n k8s.io images import "/tmp/${image_tar}"
              rm -f "/tmp/${image_tar}"
            done
          fi`, strings.Join(component.ImageList, " "))
	}

	// Build download commands
	var downloadCommands string
	for _, manifestFile := range component.ManifestFiles {
		downloadCommands += fmt.Sprintf(`
          echo "Downloading %s..."
          curl -fsSL "${RELEASE_SERVER}/${COMPONENT_PATH}/${VERSION}/%s" -o "/tmp/%s"
`, manifestFile, manifestFile, manifestFile)
	}

	// Build update status script
	updateStatusScript := fmt.Sprintf(`
          # Update status ConfigMap
          kubectl patch configmap %s -n %s \
            --type merge \
            -p '{"data":{"%s":"version: ${VERSION}\nstatus: installed\ninstalledAt: $(date -u +%%Y-%%m-%%dT%%H:%%M:%%SZ)\njobName: %s-${COMPONENT}"}}'`,
		helm.StatusConfigMapName, i.namespace, component.Name, action)

	return fmt.Sprintf(`#!/bin/sh
set -euo pipefail

COMPONENT="%s"
VERSION="%s"
RELEASE_SERVER="${RELEASE_SERVER}"
NAMESPACE="%s"
COMPONENT_PATH="%s"
INSTALL_SOURCE="${INSTALL_SOURCE:-online}"

echo "=== %s $COMPONENT v$VERSION ==="

# 1. Download manifests
%s
%s
# 2. Apply manifests
echo "Applying $COMPONENT manifests..."
%s
echo "=== $COMPONENT v$VERSION installed successfully ==="
%s
`,
		component.Name,
		component.Version,
		i.namespace,
		component.Path,
		strings.Title(action),
		downloadCommands,
		imageLoadScript,
		manifestCommands,
		updateStatusScript,
	)
}

// buildEnvVars builds environment variables for the Job.
func (i *Installer) buildEnvVars(component Component) []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name:  "RELEASE_SERVER",
			Value: i.releaseServer,
		},
	}
}
