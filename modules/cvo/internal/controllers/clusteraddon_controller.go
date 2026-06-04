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

package controllers

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	cfov1 "github.com/BetaWater/cluster-api-provider-baremetal/modules/cvo/api/v1beta1"
	"github.com/BetaWater/cluster-api-provider-baremetal/modules/cvo/internal/addon"
)

const (
	ClusterAddonFinalizer = "clusteraddon.cvo.capbm.io"
)

// ClusterAddonReconciler reconciles a ClusterAddon object
type ClusterAddonReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=cvo.capbm.io,resources=clusteraddons,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cvo.capbm.io,resources=clusteraddons/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cvo.capbm.io,resources=clusteraddons/finalizers,verbs=update
// +kubebuilder:rbac:groups=cvo.capbm.io,resources=releaseimages,verbs=get;list;watch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps;secrets,verbs=get;list;watch;create;update;patch;delete

func (r *ClusterAddonReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	// Fetch the ClusterAddon
	addon := &cfov1.ClusterAddon{}
	if err := r.Get(ctx, req.NamespacedName, addon); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion
	if !addon.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, addon)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(addon, ClusterAddonFinalizer) {
		controllerutil.AddFinalizer(addon, ClusterAddonFinalizer)
		if err := r.Update(ctx, addon); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Fetch the ReleaseImage
	releaseImage := &cfov1.ReleaseImage{}
	if err := r.Get(ctx, types.NamespacedName{Name: addon.Spec.ReleaseImageRef.Name}, releaseImage); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("ReleaseImage not found, requeueing")
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		return ctrl.Result{}, err
	}

	// Find the addon definition in the ReleaseImage
	addonDef := r.findAddonDefinition(releaseImage, addon.Spec.AddonName)
	if addonDef == nil {
		log.Error(fmt.Errorf("addon definition not found"), "Addon definition not found in ReleaseImage", "addonName", addon.Spec.AddonName)
		return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
	}

	// Check if upgrade is needed
	if addon.Status.Version != addonDef.Version {
		log.Info("Addon version mismatch, initiating upgrade", "current", addon.Status.Version, "target", addonDef.Version)
		if err := r.upgradeAddon(ctx, addon, addonDef, releaseImage); err != nil {
			log.Error(err, "Failed to upgrade addon")
			addon.Status.Phase = cfov1.AddonPhaseFailed
			_ = r.Status().Update(ctx, addon)
			return ctrl.Result{RequeueAfter: 60 * time.Second}, err
		}
	}

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *ClusterAddonReconciler) reconcileDelete(ctx context.Context, addon *cfov1.ClusterAddon) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Deleting ClusterAddon")

	// Remove finalizer
	controllerutil.RemoveFinalizer(addon, ClusterAddonFinalizer)
	return ctrl.Result{}, r.Update(ctx, addon)
}

func (r *ClusterAddonReconciler) findAddonDefinition(releaseImage *cfov1.ReleaseImage, addonName string) *cfov1.AddonDefinition {
	for i := range releaseImage.Spec.Addons {
		if releaseImage.Spec.Addons[i].Name == addonName {
			return &releaseImage.Spec.Addons[i]
		}
	}
	return nil
}

func (r *ClusterAddonReconciler) upgradeAddon(ctx context.Context, clusterAddon *cfov1.ClusterAddon, addonDef *cfov1.AddonDefinition, releaseImage *cfov1.ReleaseImage) error {
	// Update phase to upgrading
	clusterAddon.Status.Phase = cfov1.AddonPhaseUpgrading
	if err := r.Status().Update(ctx, clusterAddon); err != nil {
		return err
	}

	// Create the addon installer
	contentFetcher := addon.NewContentFetcherWithLocalDir("", getLocalDir())
	_ = contentFetcher // Used by installers internally

	var installer addon.Installer
	switch addonDef.Type {
	case cfov1.AddonTypeHelm:
		installer = addon.NewHelmInstaller(r.Client, "", clusterAddon.Namespace)
	case cfov1.AddonTypeManifest:
		installer = addon.NewManifestInstaller(r.Client, "", clusterAddon.Namespace)
	default:
		return fmt.Errorf("unsupported addon type: %s", addonDef.Type)
	}

	// Install/upgrade the addon
	if err := installer.Install(ctx, clusterAddon, releaseImage, addonDef); err != nil {
		return fmt.Errorf("failed to install addon: %w", err)
	}

	// Wait for the install job to complete
	if err := r.waitForAddonReady(ctx, clusterAddon, addonDef); err != nil {
		return fmt.Errorf("addon installation did not complete: %w", err)
	}

	// Update status
	clusterAddon.Status.Version = addonDef.Version
	clusterAddon.Status.Phase = cfov1.AddonPhaseInstalled
	clusterAddon.Status.LastAppliedRevision = addonDef.Version
	return r.Status().Update(ctx, clusterAddon)
}

func (r *ClusterAddonReconciler) waitForAddonReady(ctx context.Context, clusterAddon *cfov1.ClusterAddon, addonDef *cfov1.AddonDefinition) error {
	if addonDef.InstallStrategy == nil || !addonDef.InstallStrategy.Wait {
		return nil
	}

	timeout := 5 * time.Minute
	if addonDef.InstallStrategy.Timeout != nil {
		timeout = addonDef.InstallStrategy.Timeout.Duration
	}

	return wait.PollUntilContextTimeout(ctx, 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		currentAddon := &cfov1.ClusterAddon{}
		if err := r.Get(ctx, client.ObjectKeyFromObject(clusterAddon), currentAddon); err != nil {
			return false, err
		}

		// Check if the install job completed
		jobList := &batchv1.JobList{}
		if err := r.List(ctx, jobList, client.InNamespace(clusterAddon.Namespace), client.MatchingLabels{
			"capbm.capbm.io/addon": clusterAddon.Spec.AddonName,
		}); err != nil {
			return false, err
		}

		for _, job := range jobList.Items {
			for _, cond := range job.Status.Conditions {
				if cond.Type == batchv1.JobComplete && cond.Status == corev1.ConditionTrue {
					return true, nil
				}
				if cond.Type == batchv1.JobFailed && cond.Status == corev1.ConditionTrue {
					return false, fmt.Errorf("job %s failed", job.Name)
				}
			}
		}

		return false, nil
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterAddonReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cfov1.ClusterAddon{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}

// getLocalDir returns the local directory for release content.
func getLocalDir() string {
	// Check environment variable first
	if dir := os.Getenv("CAPBM_RELEASE_LOCAL_DIR"); dir != "" {
		return dir
	}
	// Default to release-image directory relative to working directory
	return "release-image"
}
