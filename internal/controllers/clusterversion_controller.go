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
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	infrav1 "github.com/BetaWater/cluster-api-provider-baremetal/api/v1beta1"
	"github.com/BetaWater/cluster-api-provider-baremetal/internal/upgrader"
)

type ClusterVersionReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
	Puller *upgrader.OCIPuller
}

func (r *ClusterVersionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	cv := &infrav1.ClusterVersion{}
	if err := r.Get(ctx, req.NamespacedName, cv); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	patchHelper, _ := patch.NewHelper(cv, r.Client)
	defer func() {
		_ = patchHelper.Patch(ctx, cv)
	}()

	if !cv.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, cv)
	}

	controllerutil.AddFinalizer(cv, infrav1.ClusterVersionFinalizer)

	if err := r.syncUpgradePath(ctx, cv); err != nil {
		log.Error(err, "Failed to sync UpgradePath")
		return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
	}

	if err := r.syncReleaseCatalog(ctx, cv); err != nil {
		log.Error(err, "Failed to sync ReleaseCatalog")
		return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
	}

	r.computeAvailableUpdates(ctx, cv)

	if cv.Spec.DesiredUpdate == nil || cv.Status.ActualVersion == cv.Spec.DesiredUpdate.Version {
		setCVCondition(cv, infrav1.UpgradeAvailable, metav1.ConditionTrue, infrav1.UpgradeAvailableReason, "")
		setCVCondition(cv, infrav1.UpgradeProgressing, metav1.ConditionFalse, infrav1.UpgradeProgressingReason, "")
		return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
	}

	if err := r.validateUpgrade(ctx, cv); err != nil {
		setCVCondition(cv, infrav1.UpgradeFailing, metav1.ConditionTrue, infrav1.ValidationFailedReason, err.Error())
		setCVCondition(cv, infrav1.UpgradeUpgradeable, metav1.ConditionFalse, infrav1.ValidationFailedReason, err.Error())
		return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
	}

	// Pre-upgrade health check
	if err := r.preUpgradeHealthCheck(ctx, cv); err != nil {
		setCVCondition(cv, infrav1.UpgradeFailing, metav1.ConditionTrue, "PreUpgradeHealthCheckFailed", err.Error())
		setCVCondition(cv, infrav1.UpgradeUpgradeable, metav1.ConditionFalse, "PreUpgradeHealthCheckFailed", err.Error())
		return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
	}

	setCVCondition(cv, infrav1.UpgradeUpgradeable, metav1.ConditionTrue, infrav1.UpgradeUpgradeableReason, "All preconditions passed")

	releaseImage, err := r.fetchReleaseImage(ctx, cv)
	if err != nil {
		setCVCondition(cv, infrav1.UpgradeFailing, metav1.ConditionTrue, infrav1.PullFailedReason, err.Error())
		return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
	}

	cv.Status.History = prependHistory(cv.Status.History, infrav1.UpdateHistory{
		State:       infrav1.PartialUpdate,
		Version:     cv.Spec.DesiredUpdate.Version,
		Image:       releaseImage.Spec.Image,
		Verified:    true,
		StartedTime: metav1.Now(),
	})

	setCVCondition(cv, infrav1.UpgradeProgressing, metav1.ConditionTrue, "Upgrading", fmt.Sprintf("Upgrading to %s", cv.Spec.DesiredUpdate.Version))

	if err := r.executeUpgrade(ctx, cv, releaseImage); err != nil {
		setCVCondition(cv, infrav1.UpgradeFailing, metav1.ConditionTrue, infrav1.UpgradeFailedReason, err.Error())
		return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
	}

	cv.Status.ActualVersion = cv.Spec.DesiredUpdate.Version
	if len(cv.Status.History) > 0 {
		cv.Status.History[0].State = infrav1.CompletedUpdate
		now := metav1.Now()
		cv.Status.History[0].CompletionTime = &now
	}
	setCVCondition(cv, infrav1.UpgradeAvailable, metav1.ConditionTrue, infrav1.UpgradeAvailableReason, "")
	setCVCondition(cv, infrav1.UpgradeProgressing, metav1.ConditionFalse, infrav1.UpgradeProgressingReason, "")
	setCVCondition(cv, infrav1.UpgradeFailing, metav1.ConditionFalse, infrav1.UpgradeFailingReason, "")

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *ClusterVersionReconciler) reconcileDelete(ctx context.Context, cv *infrav1.ClusterVersion) (ctrl.Result, error) {
	controllerutil.RemoveFinalizer(cv, infrav1.ClusterVersionFinalizer)
	return ctrl.Result{}, r.Update(ctx, cv)
}

func (r *ClusterVersionReconciler) syncUpgradePath(ctx context.Context, cv *infrav1.ClusterVersion) error {
	upgradePath := &infrav1.UpgradePath{}
	err := r.Get(ctx, types.NamespacedName{Name: "global"}, upgradePath)

	image := upgrader.DefaultUpgradePathImage
	if err == nil && upgradePath.Spec.Image != "" {
		image = upgradePath.Spec.Image
	}

	if r.Puller == nil {
		return nil
	}

	spec, pullErr := r.Puller.PullAndParseUpgradePath(ctx, image)
	if pullErr != nil {
		return pullErr
	}

	if apierrors.IsNotFound(err) {
		upgradePath = &infrav1.UpgradePath{
			ObjectMeta: metav1.ObjectMeta{Name: "global"},
			Spec:       *spec,
			Status: infrav1.UpgradePathStatus{
				LastSyncTime:  metav1.Now(),
				SyncSucceeded: true,
			},
		}
		return r.Create(ctx, upgradePath)
	}

	upgradePath.Spec = *spec
	upgradePath.Status.LastSyncTime = metav1.Now()
	upgradePath.Status.SyncSucceeded = true
	return r.Update(ctx, upgradePath)
}

func (r *ClusterVersionReconciler) syncReleaseCatalog(ctx context.Context, cv *infrav1.ClusterVersion) error {
	catalog := &infrav1.ReleaseCatalog{}
	err := r.Get(ctx, types.NamespacedName{Name: "global"}, catalog)

	image := upgrader.DefaultCatalogImage
	if err == nil && catalog.Spec.Image != "" {
		image = catalog.Spec.Image
	}

	if r.Puller == nil {
		return nil
	}

	status, pullErr := r.Puller.PullAndParseCatalog(ctx, image)
	if pullErr != nil {
		return pullErr
	}

	if apierrors.IsNotFound(err) {
		catalog = &infrav1.ReleaseCatalog{
			ObjectMeta: metav1.ObjectMeta{Name: "global"},
			Spec: infrav1.ReleaseCatalogSpec{
				Image:        image,
				SyncInterval: metav1.Duration{Duration: 1 * time.Hour},
			},
			Status: *status,
		}
		catalog.Status.LastSyncTime = metav1.Now()
		catalog.Status.SyncSucceeded = true
		return r.Create(ctx, catalog)
	}

	catalog.Status = *status
	catalog.Status.LastSyncTime = metav1.Now()
	catalog.Status.SyncSucceeded = true
	return r.Update(ctx, catalog)
}

func (r *ClusterVersionReconciler) computeAvailableUpdates(ctx context.Context, cv *infrav1.ClusterVersion) {
	if r.Puller == nil {
		return
	}
	executor := upgrader.NewGraphExecutor(r.Client, r.Puller, nil)
	updates, err := executor.ComputeAvailableUpdates(ctx, cv)
	if err != nil {
		return
	}
	cv.Status.AvailableUpdates = updates
	setCVCondition(cv, infrav1.UpgradeRetrieved, metav1.ConditionTrue, infrav1.UpgradeRetrievedReason, "")
}

func (r *ClusterVersionReconciler) validateUpgrade(ctx context.Context, cv *infrav1.ClusterVersion) error {
	if r.Puller == nil {
		return nil
	}
	executor := upgrader.NewGraphExecutor(r.Client, r.Puller, nil)
	return executor.ValidateUpgradePath(ctx, cv)
}

func (r *ClusterVersionReconciler) preUpgradeHealthCheck(ctx context.Context, cv *infrav1.ClusterVersion) error {
	// Check that all nodes are Ready before starting upgrade
	nodeList := &corev1.NodeList{}
	if err := r.Client.List(ctx, nodeList); err != nil {
		return fmt.Errorf("failed to list nodes for health check: %w", err)
	}
	for _, node := range nodeList.Items {
		ready := false
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
				ready = true
				break
			}
		}
		if !ready {
			return fmt.Errorf("node %s is not Ready, cannot proceed with upgrade", node.Name)
		}
	}
	return nil
}

func (r *ClusterVersionReconciler) fetchReleaseImage(ctx context.Context, cv *infrav1.ClusterVersion) (*infrav1.ReleaseImage, error) {
	image := cv.Spec.DesiredUpdate.Image
	if image == "" {
		catalog := &infrav1.ReleaseCatalog{}
		if err := r.Get(ctx, types.NamespacedName{Name: "global"}, catalog); err != nil {
			return nil, fmt.Errorf("failed to get ReleaseCatalog: %w", err)
		}
		for _, entry := range catalog.Status.Releases {
			if entry.Version == cv.Spec.DesiredUpdate.Version {
				image = entry.Image
				break
			}
		}
		if image == "" {
			return nil, fmt.Errorf("release image not found for version %s", cv.Spec.DesiredUpdate.Version)
		}
	}

	if r.Puller == nil {
		return nil, fmt.Errorf("OCI puller not configured")
	}

	spec, err := r.Puller.PullAndParseReleaseImage(ctx, image)
	if err != nil {
		return nil, err
	}

	releaseImage := &infrav1.ReleaseImage{}
	err = r.Get(ctx, types.NamespacedName{Name: versionToName(spec.Version)}, releaseImage)
	if apierrors.IsNotFound(err) {
		releaseImage = &infrav1.ReleaseImage{
			ObjectMeta: metav1.ObjectMeta{Name: versionToName(spec.Version)},
			Spec:       *spec,
			Status:     infrav1.ReleaseImageStatus{Verified: true},
		}
		if err := r.Create(ctx, releaseImage); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	} else {
		releaseImage.Spec = *spec
		if err := r.Update(ctx, releaseImage); err != nil {
			return nil, err
		}
	}

	cv.Status.Desired = infrav1.Release{
		Version: spec.Version,
		Image:   spec.Image,
	}

	return releaseImage, nil
}

func (r *ClusterVersionReconciler) executeUpgrade(ctx context.Context, cv *infrav1.ClusterVersion, releaseImage *infrav1.ReleaseImage) error {
	if r.Puller == nil {
		return nil
	}

	// Initialize ComponentStatus from ReleaseImage components
	cv.Status.ComponentStatus = r.initComponentStatus(releaseImage)

	executor := upgrader.NewGraphExecutor(r.Client, r.Puller, nil)
	return executor.ExecuteUpgradeGraph(ctx, cv, releaseImage)
}

func (r *ClusterVersionReconciler) initComponentStatus(releaseImage *infrav1.ReleaseImage) []infrav1.ComponentStatus {
	var status []infrav1.ComponentStatus

	// Add containerd
	if releaseImage.Spec.Components.Containerd.Version != "" {
		status = append(status, infrav1.ComponentStatus{
			Name:          "containerd",
			Version:       releaseImage.Spec.Components.Containerd.Version,
			TargetVersion: releaseImage.Spec.Components.Containerd.Version,
			Phase:         "Pending",
		})
	}

	// Add kubernetes
	if releaseImage.Spec.Components.Kubernetes.Version != "" {
		status = append(status, infrav1.ComponentStatus{
			Name:          "kubernetes",
			Version:       releaseImage.Spec.Components.Kubernetes.Version,
			TargetVersion: releaseImage.Spec.Components.Kubernetes.Version,
			Phase:         "Pending",
		})
	}

	// Add CNI/CSI components
	if releaseImage.Spec.Components.Calico.Version != "" {
		status = append(status, infrav1.ComponentStatus{
			Name:          "calico",
			Version:       releaseImage.Spec.Components.Calico.Version,
			TargetVersion: releaseImage.Spec.Components.Calico.Version,
			Phase:         "Pending",
		})
	}
	if releaseImage.Spec.Components.Cilium.Version != "" {
		status = append(status, infrav1.ComponentStatus{
			Name:          "cilium",
			Version:       releaseImage.Spec.Components.Cilium.Version,
			TargetVersion: releaseImage.Spec.Components.Cilium.Version,
			Phase:         "Pending",
		})
	}
	if releaseImage.Spec.Components.CephCsi.Version != "" {
		status = append(status, infrav1.ComponentStatus{
			Name:          "ceph-csi",
			Version:       releaseImage.Spec.Components.CephCsi.Version,
			TargetVersion: releaseImage.Spec.Components.CephCsi.Version,
			Phase:         "Pending",
		})
	}

	return status
}

func setCVCondition(cv *infrav1.ClusterVersion, condType clusterv1.ConditionType, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               string(condType),
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	}
	found := false
	for i, c := range cv.Status.Conditions {
		if c.Type == string(condType) {
			cv.Status.Conditions[i] = condition
			found = true
			break
		}
	}
	if !found {
		cv.Status.Conditions = append(cv.Status.Conditions, condition)
	}
}

func prependHistory(history []infrav1.UpdateHistory, entry infrav1.UpdateHistory) []infrav1.UpdateHistory {
	if len(history) > 0 && history[0].Version == entry.Version && history[0].State == infrav1.PartialUpdate {
		return history
	}
	return append([]infrav1.UpdateHistory{entry}, history...)
}

func versionToName(version string) string {
	name := ""
	for _, c := range version {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			name += string(c)
		} else {
			name += "-"
		}
	}
	return name
}

func (r *ClusterVersionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1.ClusterVersion{}).
		Complete(r)
}
