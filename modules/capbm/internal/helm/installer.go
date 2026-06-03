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

package helm

import (
	"context"
	"fmt"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

	// JobTypeInstall is the job type for installation.
	JobTypeInstall = "helm-install"
	// JobTypeUpgrade is the job type for upgrade.
	JobTypeUpgrade = "helm-upgrade"

	// ServiceAccountName is the name of the Helm service account.
	ServiceAccountName = "capbm-helm"
	// HelmImage is the default Helm image.
	HelmImage = "alpine/helm:3.15.0"
	// StatusConfigMapName is the name of the component status ConfigMap.
	StatusConfigMapName = "capbm-component-status"
)

// Installer installs components via Helm Jobs.
type Installer struct {
	client        client.Client
	releaseServer string
	namespace     string
}

// NewInstaller creates a new Helm installer.
func NewInstaller(c client.Client, releaseServer, namespace string) *Installer {
	return &Installer{
		client:        c,
		releaseServer: strings.TrimRight(releaseServer, "/"),
		namespace:     namespace,
	}
}

// InstallComponent creates a Helm Job to install a component.
func (i *Installer) InstallComponent(ctx context.Context, component HelmComponent) error {
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

	// Create new job
	newJob := i.buildInstallJob(component)
	return i.client.Create(ctx, newJob)
}

// UpgradeComponent creates a Helm Job to upgrade a component.
func (i *Installer) UpgradeComponent(ctx context.Context, component HelmComponent) error {
	// Create upgrade job
	job := i.buildUpgradeJob(component)
	return i.client.Create(ctx, job)
}

// buildInstallJob builds a Helm install Job.
func (i *Installer) buildInstallJob(component HelmComponent) *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("install-%s", component.Name),
			Namespace: i.namespace,
			Labels: map[string]string{
				LabelComponent: component.Name,
				LabelVersion:   component.Version,
				LabelType:      JobTypeInstall,
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
				Spec: i.buildPodSpec(component, "install"),
			},
		},
	}
}

// buildUpgradeJob builds a Helm upgrade Job.
func (i *Installer) buildUpgradeJob(component HelmComponent) *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("upgrade-%s", component.Name),
			Namespace: i.namespace,
			Labels: map[string]string{
				LabelComponent: component.Name,
				LabelVersion:   component.Version,
				LabelType:      JobTypeUpgrade,
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
				Spec: i.buildPodSpec(component, "upgrade"),
			},
		},
	}
}

// buildPodSpec builds the Pod spec for Helm Jobs.
func (i *Installer) buildPodSpec(component HelmComponent, action string) corev1.PodSpec {
	return corev1.PodSpec{
		ServiceAccountName: ServiceAccountName,
		Containers: []corev1.Container{
			{
				Name:    "helm",
				Image:   HelmImage,
				Command: []string{"sh", "-c", i.buildInstallScript(component, action)},
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

// buildInstallScript builds the installation script.
func (i *Installer) buildInstallScript(component HelmComponent, action string) string {
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
		StatusConfigMapName, i.namespace, component.Name, action)

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
		cases.Title(language.English).String(action),
		imageLoadScript,
		action,
		valuesArgs,
		updateStatusScript,
	)
}

// buildEnvVars builds environment variables for the Helm Job.
func (i *Installer) buildEnvVars(component HelmComponent) []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name:  "RELEASE_SERVER",
			Value: i.releaseServer,
		},
	}
}

// HelmComponent defines a component to be installed via Helm.
type HelmComponent struct {
	// Name is the component name.
	Name string
	// Version is the component version.
	Version string
	// Path is the path in the release image.
	Path string
	// Namespace is the target namespace.
	Namespace string
	// ImageList is the list of image tar files to load.
	ImageList []string
	// HelmValues is the helm --set values.
	HelmValues map[string]string
}
