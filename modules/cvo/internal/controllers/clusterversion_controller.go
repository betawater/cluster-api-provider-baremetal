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

	cfov1 "github.com/BetaWater/cluster-api-provider-baremetal/modules/cvo/api/v1beta1"
	"github.com/BetaWater/cluster-api-provider-baremetal/modules/cvo/internal/addon"
	"github.com/BetaWater/cluster-api-provider-baremetal/modules/cvo/internal/upgrader"
)

type ClusterVersionReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
	Puller *upgrader.OCIPuller
}

func (r *ClusterVersionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	cv := &cfov1.ClusterVersion{}
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

	controllerutil.AddFinalizer(cv, cfov1.ClusterVersionFinalizer)

	if err := r.syncUpgradePath(ctx, cv); err != nil {
		log.Error(err, "Failed to sync UpgradePath")
		return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
	}

	if err := r.syncReleaseCatalog(ctx, cv); err != nil {
		log.Error(err, "Failed to sync ReleaseCatalog")
		return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
	}

	r.computeAvailableUpdates(ctx, cv)

	if cv.Spec.DesiredUpdate == nil {
		setCVCondition(cv, cfov1.UpgradeAvailable, metav1.ConditionTrue, cfov1.UpgradeAvailableReason, "")
		setCVCondition(cv, cfov1.UpgradeProgressing, metav1.ConditionFalse, cfov1.UpgradeProgressingReason, "")
		return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
	}

	// Check if we need any upgrades (K8S or Addon)
	needsK8SUpgrade := cv.Status.ActualVersion != cv.Spec.DesiredUpdate.Version

	releaseImage, err := r.fetchReleaseImage(ctx, cv)
	if err != nil {
		setCVCondition(cv, cfov1.UpgradeFailing, metav1.ConditionTrue, cfov1.PullFailedReason, err.Error())
		return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
	}

	needsAddonUpgrade := r.needsAddonUpgrade(ctx, cv, releaseImage)

	if !needsK8SUpgrade && !needsAddonUpgrade {
		setCVCondition(cv, cfov1.UpgradeAvailable, metav1.ConditionTrue, cfov1.UpgradeAvailableReason, "")
		setCVCondition(cv, cfov1.UpgradeProgressing, metav1.ConditionFalse, cfov1.UpgradeProgressingReason, "")
		return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
	}

	if needsK8SUpgrade {
		if err := r.validateUpgrade(ctx, cv); err != nil {
			setCVCondition(cv, cfov1.UpgradeFailing, metav1.ConditionTrue, cfov1.ValidationFailedReason, err.Error())
			setCVCondition(cv, cfov1.UpgradeUpgradeable, metav1.ConditionFalse, cfov1.ValidationFailedReason, err.Error())
			return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
		}

		// Pre-upgrade health check
		if err := r.preUpgradeHealthCheck(ctx, cv); err != nil {
			setCVCondition(cv, cfov1.UpgradeFailing, metav1.ConditionTrue, "PreUpgradeHealthCheckFailed", err.Error())
			setCVCondition(cv, cfov1.UpgradeUpgradeable, metav1.ConditionFalse, "PreUpgradeHealthCheckFailed", err.Error())
			return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
		}

		setCVCondition(cv, cfov1.UpgradeUpgradeable, metav1.ConditionTrue, cfov1.UpgradeUpgradeableReason, "All preconditions passed")
	}

	cv.Status.History = prependHistory(cv.Status.History, cfov1.UpdateHistory{
		State:       cfov1.PartialUpdate,
		Version:     cv.Spec.DesiredUpdate.Version,
		Image:       releaseImage.Spec.Image,
		Verified:    true,
		StartedTime: metav1.Now(),
	})

	if needsK8SUpgrade {
		setCVCondition(cv, cfov1.UpgradeProgressing, metav1.ConditionTrue, "Upgrading", fmt.Sprintf("Upgrading to %s", cv.Spec.DesiredUpdate.Version))
	}

	if err := r.executeUpgrade(ctx, cv, releaseImage, needsK8SUpgrade); err != nil {
		setCVCondition(cv, cfov1.UpgradeFailing, metav1.ConditionTrue, cfov1.UpgradeFailedReason, err.Error())
		return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
	}

	cv.Status.ActualVersion = cv.Spec.DesiredUpdate.Version
	if len(cv.Status.History) > 0 {
		cv.Status.History[0].State = cfov1.CompletedUpdate
		now := metav1.Now()
		cv.Status.History[0].CompletionTime = &now
	}
	setCVCondition(cv, cfov1.UpgradeAvailable, metav1.ConditionTrue, cfov1.UpgradeAvailableReason, "")
	setCVCondition(cv, cfov1.UpgradeProgressing, metav1.ConditionFalse, cfov1.UpgradeProgressingReason, "")
	setCVCondition(cv, cfov1.UpgradeFailing, metav1.ConditionFalse, cfov1.UpgradeFailingReason, "")

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *ClusterVersionReconciler) reconcileDelete(ctx context.Context, cv *cfov1.ClusterVersion) (ctrl.Result, error) {
	controllerutil.RemoveFinalizer(cv, cfov1.ClusterVersionFinalizer)
	return ctrl.Result{}, r.Update(ctx, cv)
}

func (r *ClusterVersionReconciler) syncUpgradePath(ctx context.Context, cv *cfov1.ClusterVersion) error {
	upgradePath := &cfov1.UpgradePath{}
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
		upgradePath = &cfov1.UpgradePath{
			ObjectMeta: metav1.ObjectMeta{Name: "global"},
			Spec:       *spec,
			Status: cfov1.UpgradePathStatus{
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

func (r *ClusterVersionReconciler) syncReleaseCatalog(ctx context.Context, cv *cfov1.ClusterVersion) error {
	catalog := &cfov1.ReleaseCatalog{}
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
		catalog = &cfov1.ReleaseCatalog{
			ObjectMeta: metav1.ObjectMeta{Name: "global"},
			Spec: cfov1.ReleaseCatalogSpec{
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

func (r *ClusterVersionReconciler) computeAvailableUpdates(ctx context.Context, cv *cfov1.ClusterVersion) {
	if r.Puller == nil {
		return
	}
	executor := upgrader.NewGraphExecutor(r.Client, r.Puller, nil)
	updates, err := executor.ComputeAvailableUpdates(ctx, cv)
	if err != nil {
		return
	}
	cv.Status.AvailableUpdates = updates
	setCVCondition(cv, cfov1.UpgradeRetrieved, metav1.ConditionTrue, cfov1.UpgradeRetrievedReason, "")
}

func (r *ClusterVersionReconciler) validateUpgrade(ctx context.Context, cv *cfov1.ClusterVersion) error {
	if r.Puller == nil {
		return nil
	}
	executor := upgrader.NewGraphExecutor(r.Client, r.Puller, nil)
	return executor.ValidateUpgradePath(ctx, cv)
}

func (r *ClusterVersionReconciler) preUpgradeHealthCheck(ctx context.Context, cv *cfov1.ClusterVersion) error {
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

func (r *ClusterVersionReconciler) fetchReleaseImage(ctx context.Context, cv *cfov1.ClusterVersion) (*cfov1.ReleaseImage, error) {
	image := cv.Spec.DesiredUpdate.Image
	if image == "" {
		catalog := &cfov1.ReleaseCatalog{}
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

	releaseImage := &cfov1.ReleaseImage{}
	err = r.Get(ctx, types.NamespacedName{Name: versionToName(spec.Version)}, releaseImage)
	if apierrors.IsNotFound(err) {
		releaseImage = &cfov1.ReleaseImage{
			ObjectMeta: metav1.ObjectMeta{Name: versionToName(spec.Version)},
			Spec:       *spec,
			Status:     cfov1.ReleaseImageStatus{Verified: true},
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

	cv.Status.Desired = cfov1.Release{
		Version: spec.Version,
		Image:   spec.Image,
	}

	return releaseImage, nil
}

func (r *ClusterVersionReconciler) executeUpgrade(ctx context.Context, cv *cfov1.ClusterVersion, releaseImage *cfov1.ReleaseImage, needsK8SUpgrade bool) error {
	if r.Puller == nil {
		return nil
	}

	// Phase 1: K8S upgrade (only when K8S version changes)
	if needsK8SUpgrade {
		// Initialize ComponentStatus from ReleaseImage components
		cv.Status.ComponentStatus = r.initComponentStatus(releaseImage)

		// Execute component upgrade graph
		executor := upgrader.NewGraphExecutor(r.Client, r.Puller, nil)
		if err := executor.ExecuteUpgradeGraph(ctx, cv, releaseImage); err != nil {
			return fmt.Errorf("k8s upgrade failed: %w", err)
		}
	}

	// Phase 2: Addon upgrade (always execute)
	if err := r.executeAddonUpgrades(ctx, cv, releaseImage); err != nil {
		return fmt.Errorf("addon upgrades failed: %w", err)
	}

	// Update addon status
	r.updateAddonStatus(cv, releaseImage)

	return nil
}

func (r *ClusterVersionReconciler) initComponentStatus(releaseImage *cfov1.ReleaseImage) []cfov1.ComponentStatus {
	var status []cfov1.ComponentStatus

	// Add containerd
	if releaseImage.Spec.Components.Containerd.Version != "" {
		status = append(status, cfov1.ComponentStatus{
			Name:          "containerd",
			Version:       releaseImage.Spec.Components.Containerd.Version,
			TargetVersion: releaseImage.Spec.Components.Containerd.Version,
			Phase:         "Pending",
		})
	}

	// Add kubernetes
	if releaseImage.Spec.Components.Kubernetes.Version != "" {
		status = append(status, cfov1.ComponentStatus{
			Name:          "kubernetes",
			Version:       releaseImage.Spec.Components.Kubernetes.Version,
			TargetVersion: releaseImage.Spec.Components.Kubernetes.Version,
			Phase:         "Pending",
		})
	}

	// Add all addons (including CAPI Core)
	for _, addonDef := range releaseImage.Spec.Addons {
		if addonDef.Version != "" {
			status = append(status, cfov1.ComponentStatus{
				Name:          addonDef.Name,
				Version:       addonDef.Version,
				TargetVersion: addonDef.Version,
				Phase:         "Pending",
			})
		}
	}

	return status
}

// executeAddonUpgrades executes upgrades for all addons in dependency order.
func (r *ClusterVersionReconciler) executeAddonUpgrades(ctx context.Context, cv *cfov1.ClusterVersion, releaseImage *cfov1.ReleaseImage) error {
	addonUpgrader := addon.NewUpgrader(r.Client, "", cv.Namespace)

	// Build addon dependency graph
	addonGraph := buildAddonDependencyGraph(releaseImage)
	sortedAddons := topologicalSortAddons(addonGraph)

	// Get current release image for comparison
	currentRelease := &cfov1.ReleaseImage{}
	if err := r.Get(ctx, types.NamespacedName{Name: versionToName(cv.Status.ActualVersion)}, currentRelease); err != nil {
		// If current release not found, treat as fresh install
		currentRelease = nil
	}

	// Upgrade addons in order
	for _, addonName := range sortedAddons {
		addonDef := findAddonDefByName(releaseImage, addonName)
		if addonDef == nil {
			continue
		}

		// Get or create ClusterAddon
		clusterAddon := &cfov1.ClusterAddon{}
		err := r.Get(ctx, types.NamespacedName{Name: addonName, Namespace: cv.Namespace}, clusterAddon)
		if apierrors.IsNotFound(err) {
			// Fresh install
			clusterAddon = &cfov1.ClusterAddon{
				ObjectMeta: metav1.ObjectMeta{
					Name:      addonName,
					Namespace: cv.Namespace,
				},
				Spec: cfov1.ClusterAddonSpec{
					ClusterRef:      cv.Spec.ClusterRef,
					ReleaseImageRef: corev1.LocalObjectReference{Name: releaseImage.Name},
					AddonName:       addonDef.Name,
					Namespace:       addonDef.Namespace,
				},
			}
			if err := r.Client.Create(ctx, clusterAddon); err != nil {
				return fmt.Errorf("failed to create addon %s: %w", addonName, err)
			}
		} else if err != nil {
			return err
		}

		// Skip if already at target version
		if clusterAddon.Status.Version == addonDef.Version {
			continue
		}

		// Execute upgrade
		if err := addonUpgrader.Upgrade(ctx, clusterAddon, currentRelease, releaseImage); err != nil {
			return fmt.Errorf("failed to upgrade addon %s: %w", addonName, err)
		}
	}

	return nil
}

// buildAddonDependencyGraph builds a dependency graph from addon definitions.
func buildAddonDependencyGraph(releaseImage *cfov1.ReleaseImage) map[string][]string {
	graph := make(map[string][]string)
	for _, addonDef := range releaseImage.Spec.Addons {
		graph[addonDef.Name] = addonDef.Dependencies
	}
	return graph
}

// topologicalSortAddons performs topological sort on addon dependencies.
func topologicalSortAddons(graph map[string][]string) []string {
	var result []string
	visited := make(map[string]bool)
	inProgress := make(map[string]bool)

	var visit func(name string)
	visit = func(name string) {
		if visited[name] {
			return
		}
		if inProgress[name] {
			return
		}
		inProgress[name] = true
		for _, dep := range graph[name] {
			visit(dep)
		}
		inProgress[name] = false
		visited[name] = true
		result = append(result, name)
	}

	for name := range graph {
		visit(name)
	}

	return result
}

// findAddonDefByName finds an addon definition by name.
func findAddonDefByName(releaseImage *cfov1.ReleaseImage, name string) *cfov1.AddonDefinition {
	for i := range releaseImage.Spec.Addons {
		if releaseImage.Spec.Addons[i].Name == name {
			return &releaseImage.Spec.Addons[i]
		}
	}
	return nil
}

// needsAddonUpgrade checks if any addon needs upgrade by comparing current ClusterAddon versions
// with target ReleaseImage addon versions.
func (r *ClusterVersionReconciler) needsAddonUpgrade(ctx context.Context, cv *cfov1.ClusterVersion, releaseImage *cfov1.ReleaseImage) bool {
	for _, addonDef := range releaseImage.Spec.Addons {
		if addonDef.Version == "" {
			continue
		}

		clusterAddon := &cfov1.ClusterAddon{}
		err := r.Get(ctx, types.NamespacedName{Name: addonDef.Name, Namespace: cv.Namespace}, clusterAddon)

		if apierrors.IsNotFound(err) {
			// New addon, needs installation
			return true
		}

		if err != nil {
			continue
		}

		// Version mismatch, needs upgrade
		if clusterAddon.Status.Version != addonDef.Version {
			return true
		}
	}
	return false
}

// updateAddonStatus updates the ClusterVersion status with current addon versions.
func (r *ClusterVersionReconciler) updateAddonStatus(cv *cfov1.ClusterVersion, releaseImage *cfov1.ReleaseImage) {
	var addonStatus []cfov1.AddonVersionStatus

	for _, addonDef := range releaseImage.Spec.Addons {
		if addonDef.Version == "" {
			continue
		}

		status := cfov1.AddonVersionStatus{
			Name:          addonDef.Name,
			TargetVersion: addonDef.Version,
			Phase:         cfov1.AddonPhaseInstalled,
		}

		clusterAddon := &cfov1.ClusterAddon{}
		err := r.Get(context.Background(), types.NamespacedName{Name: addonDef.Name, Namespace: cv.Namespace}, clusterAddon)

		if apierrors.IsNotFound(err) {
			status.Phase = cfov1.AddonPhasePending
			status.Version = ""
		} else if err == nil {
			status.Version = clusterAddon.Status.Version
			if clusterAddon.Status.Version != addonDef.Version {
				status.Phase = cfov1.AddonPhaseUpgrading
			}
		}

		status.LastTransitionTime = metav1.Now()
		addonStatus = append(addonStatus, status)
	}

	cv.Status.AddonStatus = addonStatus
}

func setCVCondition(cv *cfov1.ClusterVersion, condType clusterv1.ConditionType, status metav1.ConditionStatus, reason, message string) {
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

func prependHistory(history []cfov1.UpdateHistory, entry cfov1.UpdateHistory) []cfov1.UpdateHistory {
	if len(history) > 0 && history[0].Version == entry.Version && history[0].State == cfov1.PartialUpdate {
		return history
	}
	return append([]cfov1.UpdateHistory{entry}, history...)
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
		For(&cfov1.ClusterVersion{}).
		Complete(r)
}
