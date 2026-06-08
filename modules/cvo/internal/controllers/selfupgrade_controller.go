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
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/yaml"

	cfov1 "github.com/BetaWater/cluster-api-provider-baremetal/modules/cvo/api/v1beta1"
	"github.com/BetaWater/cluster-api-provider-baremetal/modules/cvo/internal/upgrader"
)

type SelfUpgradeReconciler struct {
	client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
	Puller   *upgrader.OCIPuller
}

func (r *SelfUpgradeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	su := &cfov1.SelfUpgrade{}
	if err := r.Get(ctx, req.NamespacedName, su); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	patchHelper, err := patch.NewHelper(su, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}
	defer func() {
		if err := patchHelper.Patch(ctx, su); err != nil {
			log.Error(err, "Failed to patch SelfUpgrade status")
		}
	}()

	if !su.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, su)
	}

	controllerutil.AddFinalizer(su, cfov1.SelfUpgradeFinalizer)

	// Check if upgrade is paused
	if su.Spec.Paused {
		su.Status.Phase = cfov1.PhasePending
		r.setCondition(su, cfov1.SelfUpgradeValidating, metav1.ConditionTrue, "Paused", "Upgrade is paused")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	switch su.Status.Phase {
	case "":
		return r.initializeUpgrade(ctx, su)
	case cfov1.PhaseValidating:
		return r.validateUpgrade(ctx, su)
	case cfov1.PhasePreUpgrade:
		return r.executePreUpgrade(ctx, su)
	case cfov1.PhaseUpgrading:
		return r.executeUpgrade(ctx, su)
	case cfov1.PhaseVerifying:
		return r.verifyUpgrade(ctx, su)
	case cfov1.PhaseRollingBack:
		return r.executeRollback(ctx, su)
	case cfov1.PhaseCompleted, cfov1.PhaseFailed:
		return ctrl.Result{}, nil
	default:
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}
}

func (r *SelfUpgradeReconciler) initializeUpgrade(ctx context.Context, su *cfov1.SelfUpgrade) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Initializing self-upgrade", "targetVersion", su.Spec.TargetVersion)

	if su.Status.StartedTime.IsZero() {
		su.Status.StartedTime = metav1.Now()
	}

	su.Status.Phase = cfov1.PhaseValidating
	r.setCondition(su, cfov1.SelfUpgradeValidating, metav1.ConditionTrue, "Initializing", "Starting upgrade validation")

	r.Recorder.Event(su, "Normal", "UpgradeStarted", fmt.Sprintf("Self-upgrade to %s started", su.Spec.TargetVersion))

	return ctrl.Result{Requeue: true}, nil
}

func (r *SelfUpgradeReconciler) validateUpgrade(ctx context.Context, su *cfov1.SelfUpgrade) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Validating self-upgrade")

	if su.Spec.TargetVersion == "" {
		su.Status.Phase = cfov1.PhaseFailed
		r.setCondition(su, cfov1.SelfUpgradeFailed, metav1.ConditionTrue, "ValidationFailed", "Target version is required")
		return ctrl.Result{}, nil
	}

	if len(su.Spec.Components) == 0 {
		su.Status.Phase = cfov1.PhaseFailed
		r.setCondition(su, cfov1.SelfUpgradeFailed, metav1.ConditionTrue, "ValidationFailed", "No components defined")
		return ctrl.Result{}, nil
	}

	if err := r.validateComponents(su); err != nil {
		su.Status.Phase = cfov1.PhaseFailed
		r.setCondition(su, cfov1.SelfUpgradeFailed, metav1.ConditionTrue, "ValidationFailed", err.Error())
		return ctrl.Result{}, nil
	}

	if err := r.validateClusterHealth(ctx); err != nil {
		su.Status.Phase = cfov1.PhaseFailed
		r.setCondition(su, cfov1.SelfUpgradeFailed, metav1.ConditionTrue, "ValidationFailed", err.Error())
		return ctrl.Result{}, nil
	}

	su.Status.Phase = cfov1.PhasePreUpgrade
	r.setCondition(su, cfov1.SelfUpgradePreUpgrade, metav1.ConditionTrue, "PreUpgrade", "Executing pre-upgrade hooks")

	return ctrl.Result{Requeue: true}, nil
}

func (r *SelfUpgradeReconciler) executePreUpgrade(ctx context.Context, su *cfov1.SelfUpgrade) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Executing pre-upgrade hooks")

	for _, hook := range su.Spec.PreUpgradeHooks {
		log.Info("Executing pre-upgrade hook", "hook", hook.Name)
		if err := r.executeHook(ctx, hook); err != nil {
			if hook.OnFailure == "Abort" || hook.OnFailure == "" {
				if su.Spec.Strategy.AutoRollback {
					su.Status.Phase = cfov1.PhaseRollingBack
					r.setCondition(su, cfov1.SelfUpgradeRollingBack, metav1.ConditionTrue, "RollingBack", fmt.Sprintf("Pre-upgrade hook %s failed: %v", hook.Name, err))
					return ctrl.Result{Requeue: true}, nil
				}
				su.Status.Phase = cfov1.PhaseFailed
				r.setCondition(su, cfov1.SelfUpgradeFailed, metav1.ConditionTrue, "PreUpgradeFailed", fmt.Sprintf("Hook %s failed: %v", hook.Name, err))
				return ctrl.Result{}, nil
			}
			log.Error(err, "Pre-upgrade hook failed, continuing", "hook", hook.Name)
		}
	}

	if err := r.backupCurrentConfig(ctx, su); err != nil {
		log.Error(err, "Failed to backup current config")
		if su.Spec.Strategy.AutoRollback {
			su.Status.Phase = cfov1.PhaseRollingBack
			return ctrl.Result{Requeue: true}, nil
		}
	}

	su.Status.Phase = cfov1.PhaseUpgrading
	r.setCondition(su, cfov1.SelfUpgradeUpgrading, metav1.ConditionTrue, "Upgrading", "Executing upgrade")

	return ctrl.Result{Requeue: true}, nil
}

func (r *SelfUpgradeReconciler) executeUpgrade(ctx context.Context, su *cfov1.SelfUpgrade) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	sortedComponents, err := r.topologicalSort(su.Spec.Components)
	if err != nil {
		su.Status.Phase = cfov1.PhaseFailed
		r.setCondition(su, cfov1.SelfUpgradeFailed, metav1.ConditionTrue, "UpgradeFailed", err.Error())
		return ctrl.Result{}, nil
	}

	for i, component := range sortedComponents {
		if r.isComponentCompleted(su, component.Name) {
			continue
		}

		log.Info("Upgrading component", "component", component.Name, "type", component.Type)

		if err := r.upgradeComponent(ctx, su, component); err != nil {
			log.Error(err, "Failed to upgrade component", "component", component.Name)

			r.updateComponentStatus(su, component.Name, "Failed", err.Error())

			if component.Blocking || su.Spec.Strategy.AutoRollback {
				su.Status.Phase = cfov1.PhaseRollingBack
				r.setCondition(su, cfov1.SelfUpgradeRollingBack, metav1.ConditionTrue, "RollingBack", fmt.Sprintf("Component %s failed: %v", component.Name, err))
				return ctrl.Result{Requeue: true}, nil
			}
		}

		r.updateComponentStatus(su, component.Name, "Completed", "")

		if i < len(sortedComponents)-1 {
			return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
		}
	}

	su.Status.Phase = cfov1.PhaseVerifying
	r.setCondition(su, cfov1.SelfUpgradeVerifying, metav1.ConditionTrue, "Verifying", "Verifying upgrade")

	return ctrl.Result{Requeue: true}, nil
}

func (r *SelfUpgradeReconciler) verifyUpgrade(ctx context.Context, su *cfov1.SelfUpgrade) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Verifying upgrade")

	for _, component := range su.Spec.Components {
		if component.HealthCheck == nil {
			continue
		}

		if err := r.runHealthCheck(ctx, component); err != nil {
			log.Error(err, "Health check failed", "component", component.Name)
			if su.Spec.Strategy.AutoRollback {
				su.Status.Phase = cfov1.PhaseRollingBack
				return ctrl.Result{Requeue: true}, nil
			}
			su.Status.Phase = cfov1.PhaseFailed
			r.setCondition(su, cfov1.SelfUpgradeFailed, metav1.ConditionTrue, "VerificationFailed", err.Error())
			return ctrl.Result{}, nil
		}
	}

	for _, hook := range su.Spec.PostUpgradeHooks {
		if err := r.executeHook(ctx, hook); err != nil {
			log.Error(err, "Post-upgrade hook failed", "hook", hook.Name)
		}
	}

	su.Status.Phase = cfov1.PhaseCompleted
	su.Status.CurrentVersion = su.Spec.TargetVersion
	su.Status.CompletedTime = &metav1.Time{Time: time.Now()}
	r.setCondition(su, cfov1.SelfUpgradeCompleted, metav1.ConditionTrue, "Completed", "Upgrade completed successfully")

	r.Recorder.Event(su, "Normal", "UpgradeCompleted", fmt.Sprintf("Self-upgrade to %s completed", su.Spec.TargetVersion))

	return ctrl.Result{}, nil
}

func (r *SelfUpgradeReconciler) executeRollback(ctx context.Context, su *cfov1.SelfUpgrade) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Executing rollback")

	for i := len(su.Spec.Components) - 1; i >= 0; i-- {
		component := su.Spec.Components[i]
		log.Info("Rolling back component", "component", component.Name)

		if err := r.rollbackComponent(ctx, su, component); err != nil {
			log.Error(err, "Failed to rollback component", "component", component.Name)
			su.Status.Phase = cfov1.PhaseFailed
			r.setCondition(su, cfov1.SelfUpgradeFailed, metav1.ConditionTrue, "RollbackFailed", err.Error())
			return ctrl.Result{}, nil
		}
	}

	su.Status.Phase = cfov1.PhaseFailed
	su.Status.CompletedTime = &metav1.Time{Time: time.Now()}
	r.setCondition(su, cfov1.SelfUpgradeFailed, metav1.ConditionTrue, "RolledBack", "Upgrade rolled back")

	r.Recorder.Event(su, "Warning", "UpgradeRolledBack", "Self-upgrade rolled back")

	return ctrl.Result{}, nil
}

func (r *SelfUpgradeReconciler) reconcileDelete(ctx context.Context, su *cfov1.SelfUpgrade) (ctrl.Result, error) {
	controllerutil.RemoveFinalizer(su, cfov1.SelfUpgradeFinalizer)
	return ctrl.Result{}, nil
}

func (r *SelfUpgradeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cfov1.SelfUpgrade{}).
		Complete(r)
}

func (r *SelfUpgradeReconciler) setCondition(su *cfov1.SelfUpgrade, conditionType string, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.Now(),
		ObservedGeneration: su.Generation,
	}

	for i, c := range su.Status.Conditions {
		if c.Type == conditionType {
			su.Status.Conditions[i] = condition
			return
		}
	}
	su.Status.Conditions = append(su.Status.Conditions, condition)
}

func (r *SelfUpgradeReconciler) validateComponents(su *cfov1.SelfUpgrade) error {
	componentNames := make(map[string]bool)
	for _, comp := range su.Spec.Components {
		if componentNames[comp.Name] {
			return fmt.Errorf("duplicate component name: %s", comp.Name)
		}
		componentNames[comp.Name] = true
	}

	for _, comp := range su.Spec.Components {
		for _, dep := range comp.DependsOn {
			if !componentNames[dep] {
				return fmt.Errorf("component %s depends on unknown component: %s", comp.Name, dep)
			}
		}
	}

	return nil
}

func (r *SelfUpgradeReconciler) validateClusterHealth(ctx context.Context) error {
	nodes := &corev1.NodeList{}
	if err := r.List(ctx, nodes); err != nil {
		return fmt.Errorf("failed to list nodes: %w", err)
	}

	for _, node := range nodes.Items {
		ready := false
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
				ready = true
				break
			}
		}
		if !ready {
			return fmt.Errorf("node %s is not ready", node.Name)
		}
	}

	return nil
}

func (r *SelfUpgradeReconciler) topologicalSort(components []cfov1.SelfUpgradeComponent) ([]cfov1.SelfUpgradeComponent, error) {
	sorted := make([]cfov1.SelfUpgradeComponent, 0, len(components))
	visited := make(map[string]bool)

	var visit func(cfov1.SelfUpgradeComponent) error
	visit = func(comp cfov1.SelfUpgradeComponent) error {
		if visited[comp.Name] {
			return nil
		}

		for _, depName := range comp.DependsOn {
			for _, dep := range components {
				if dep.Name == depName {
					if err := visit(dep); err != nil {
						return err
					}
				}
			}
		}

		visited[comp.Name] = true
		sorted = append(sorted, comp)
		return nil
	}

	for _, comp := range components {
		if err := visit(comp); err != nil {
			return nil, err
		}
	}

	return sorted, nil
}

func (r *SelfUpgradeReconciler) isComponentCompleted(su *cfov1.SelfUpgrade, name string) bool {
	for _, status := range su.Status.ComponentStatus {
		if status.Name == name && status.Phase == "Completed" {
			return true
		}
	}
	return false
}

func (r *SelfUpgradeReconciler) updateComponentStatus(su *cfov1.SelfUpgrade, name, phase, message string) {
	for i, status := range su.Status.ComponentStatus {
		if status.Name == name {
			su.Status.ComponentStatus[i].Phase = phase
			su.Status.ComponentStatus[i].Message = message
			if phase == "Completed" {
				now := metav1.Now()
				su.Status.ComponentStatus[i].CompletedTime = &now
			}
			return
		}
	}

	status := cfov1.ComponentUpgradeStatus{
		Name:        name,
		Phase:       phase,
		Message:     message,
		StartedTime: metav1.Now(),
	}
	if phase == "Completed" {
		now := metav1.Now()
		status.CompletedTime = &now
	}
	su.Status.ComponentStatus = append(su.Status.ComponentStatus, status)
}

func (r *SelfUpgradeReconciler) upgradeComponent(ctx context.Context, su *cfov1.SelfUpgrade, comp cfov1.SelfUpgradeComponent) error {
	switch comp.Type {
	case cfov1.SelfUpgradeComponentTypeCRD:
		return r.upgradeCRDs(ctx, su)
	case cfov1.SelfUpgradeComponentTypeRBAC:
		return r.upgradeRBAC(ctx, su)
	case cfov1.SelfUpgradeComponentTypeWebhook:
		return r.upgradeWebhooks(ctx, su)
	case cfov1.SelfUpgradeComponentTypeDeployment:
		return r.upgradeDeployment(ctx, su, comp)
	default:
		return fmt.Errorf("unknown component type: %s", comp.Type)
	}
}

func (r *SelfUpgradeReconciler) runHealthCheck(ctx context.Context, comp cfov1.SelfUpgradeComponent) error {
	if comp.HealthCheck == nil {
		return nil
	}

	checker := upgrader.NewHealthChecker(r.Client, comp.HealthCheck.Timeout.Duration)

	switch comp.HealthCheck.Type {
	case "DeploymentReady":
		return checker.WaitForDeploymentReady(ctx, comp.HealthCheck.Namespace, comp.HealthCheck.Name, comp.HealthCheck.Timeout.Duration)
	case "DaemonSetReady":
		return checker.WaitForDaemonSetReady(ctx, comp.HealthCheck.Namespace, comp.HealthCheck.Name, comp.HealthCheck.Timeout.Duration)
	case "CRDEstablished":
		return nil
	case "EndpointHealthy":
		return nil
	default:
		return fmt.Errorf("unknown health check type: %s", comp.HealthCheck.Type)
	}
}

func (r *SelfUpgradeReconciler) executeHook(ctx context.Context, hook cfov1.Hook) error {
	if hook.Command == "" {
		return nil
	}

	timeout := 5 * time.Minute
	if hook.Timeout != nil {
		timeout = hook.Timeout.Duration
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	log := ctrl.LoggerFrom(ctx)
	log.Info("Executing hook", "hook", hook.Name, "command", hook.Command)

	// Execute hook command locally via exec
	cmd := exec.CommandContext(ctx, "bash", "-c", hook.Command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if hook.OnFailure == "Ignore" || hook.OnFailure == "Continue" {
			log.Error(err, "Hook failed but continuing", "hook", hook.Name, "output", string(output))
			return nil
		}
		return fmt.Errorf("hook %s failed: %w, output: %s", hook.Name, err, string(output))
	}

	log.Info("Hook completed", "hook", hook.Name, "output", string(output))
	return nil
}

func (r *SelfUpgradeReconciler) backupCurrentConfig(ctx context.Context, su *cfov1.SelfUpgrade) error {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Backing up current configuration")

	backupName := fmt.Sprintf("backup-%s-%s", su.Name, time.Now().Format("20060102150405"))

	// Backup CRDs
	crds := &apiextensionsv1.CustomResourceDefinitionList{}
	if err := r.List(ctx, crds, client.MatchingLabels{"app.kubernetes.io/part-of": "capbm"}); err != nil {
		return fmt.Errorf("failed to list CRDs: %w", err)
	}

	backupData := make(map[string]string)
	for i := range crds.Items {
		crd := &crds.Items[i]
		crdYAML, err := yaml.Marshal(crd)
		if err != nil {
			log.Error(err, "Failed to marshal CRD", "name", crd.Name)
			continue
		}
		backupData[fmt.Sprintf("crd-%s.yaml", crd.Name)] = string(crdYAML)
	}

	// Backup Deployments
	deployments := []types.NamespacedName{
		{Namespace: "cvo-system", Name: "cvo-controller-manager"},
		{Namespace: "capbm-system", Name: "capbm-controller-manager"},
	}

	for _, ns := range deployments {
		deploy := &appsv1.Deployment{}
		if err := r.Get(ctx, ns, deploy); err != nil {
			log.Error(err, "Failed to get deployment for backup", "namespace", ns.Namespace, "name", ns.Name)
			continue
		}
		deployYAML, err := yaml.Marshal(deploy)
		if err != nil {
			log.Error(err, "Failed to marshal deployment", "name", deploy.Name)
			continue
		}
		backupData[fmt.Sprintf("deployment-%s-%s.yaml", deploy.Namespace, deploy.Name)] = string(deployYAML)
	}

	// Store backup in ConfigMap
	backupConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backupName,
			Namespace: su.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/part-of": "capbm",
				"cvo.capbm.io/backup":       "true",
				"cvo.capbm.io/self-upgrade": su.Name,
			},
		},
		Data: backupData,
	}

	if err := r.Create(ctx, backupConfigMap); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create backup ConfigMap: %w", err)
		}
	}

	log.Info("Backup stored in ConfigMap", "name", backupName, "items", len(backupData))
	return nil
}

func (r *SelfUpgradeReconciler) upgradeCRDs(ctx context.Context, su *cfov1.SelfUpgrade) error {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Upgrading CRDs")

	crds := &apiextensionsv1.CustomResourceDefinitionList{}
	if err := r.List(ctx, crds, client.MatchingLabels{"app.kubernetes.io/part-of": "capbm"}); err != nil {
		return fmt.Errorf("failed to list CRDs: %w", err)
	}

	for i := range crds.Items {
		crd := &crds.Items[i]
		original := crd.DeepCopy()

		if crd.Annotations == nil {
			crd.Annotations = make(map[string]string)
		}
		crd.Annotations["cvo.capbm.io/last-upgraded-version"] = su.Spec.TargetVersion

		if err := r.Patch(ctx, crd, client.MergeFrom(original)); err != nil {
			return fmt.Errorf("failed to update CRD %s: %w", crd.Name, err)
		}

		if err := r.waitForCRDEstablished(ctx, crd.Name, 60*time.Second); err != nil {
			return fmt.Errorf("CRD %s not established: %w", crd.Name, err)
		}
	}

	return nil
}

func (r *SelfUpgradeReconciler) upgradeRBAC(ctx context.Context, su *cfov1.SelfUpgrade) error {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Upgrading RBAC")

	// Get RBAC manifests from release image directory
	rbacDir := fmt.Sprintf("/tmp/capbm-upgrade/release-%s/rbac", safeImageName(su.Spec.ReleaseImage))
	if _, err := os.Stat(rbacDir); os.IsNotExist(err) {
		log.Info("No RBAC directory found, skipping RBAC upgrade")
		return nil
	}

	entries, err := os.ReadDir(rbacDir)
	if err != nil {
		return fmt.Errorf("failed to read RBAC directory: %w", err)
	}

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".yaml") && !strings.HasSuffix(entry.Name(), ".yml") {
			continue
		}

		content, err := os.ReadFile(filepath.Join(rbacDir, entry.Name()))
		if err != nil {
			return fmt.Errorf("failed to read RBAC file %s: %w", entry.Name(), err)
		}

		// Apply RBAC manifest
		obj, err := decodeYAML(content)
		if err != nil {
			return fmt.Errorf("failed to decode RBAC manifest %s: %w", entry.Name(), err)
		}

		if err := r.Patch(ctx, obj, client.Merge, client.FieldOwner("capbm-self-upgrade")); err != nil {
			return fmt.Errorf("failed to apply RBAC manifest %s: %w", entry.Name(), err)
		}

		log.Info("Applied RBAC manifest", "file", entry.Name())
	}

	log.Info("RBAC update complete")
	return nil
}

func (r *SelfUpgradeReconciler) upgradeWebhooks(ctx context.Context, su *cfov1.SelfUpgrade) error {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Upgrading Webhooks")

	// Get webhook manifests from release image directory
	webhookDir := fmt.Sprintf("/tmp/capbm-upgrade/release-%s/webhooks", safeImageName(su.Spec.ReleaseImage))
	if _, err := os.Stat(webhookDir); os.IsNotExist(err) {
		log.Info("No webhooks directory found, skipping webhook upgrade")
		return nil
	}

	entries, err := os.ReadDir(webhookDir)
	if err != nil {
		return fmt.Errorf("failed to read webhooks directory: %w", err)
	}

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".yaml") && !strings.HasSuffix(entry.Name(), ".yml") {
			continue
		}

		content, err := os.ReadFile(filepath.Join(webhookDir, entry.Name()))
		if err != nil {
			return fmt.Errorf("failed to read webhook file %s: %w", entry.Name(), err)
		}

		// Apply webhook manifest
		obj, err := decodeYAML(content)
		if err != nil {
			return fmt.Errorf("failed to decode webhook manifest %s: %w", entry.Name(), err)
		}

		if err := r.Patch(ctx, obj, client.Merge, client.FieldOwner("capbm-self-upgrade")); err != nil {
			return fmt.Errorf("failed to apply webhook manifest %s: %w", entry.Name(), err)
		}

		log.Info("Applied webhook manifest", "file", entry.Name())
	}

	log.Info("Webhook update complete")
	return nil
}

func (r *SelfUpgradeReconciler) upgradeDeployment(ctx context.Context, su *cfov1.SelfUpgrade, comp cfov1.SelfUpgradeComponent) error {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Upgrading deployment", "component", comp.Name)

	if comp.HealthCheck == nil || comp.HealthCheck.Namespace == "" || comp.HealthCheck.Name == "" {
		return fmt.Errorf("deployment component %s requires healthCheck.namespace and healthCheck.name", comp.Name)
	}

	deploy := &appsv1.Deployment{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: comp.HealthCheck.Namespace, Name: comp.HealthCheck.Name}, deploy); err != nil {
		return fmt.Errorf("failed to get deployment %s/%s: %w", comp.HealthCheck.Namespace, comp.HealthCheck.Name, err)
	}

	original := deploy.DeepCopy()

	if len(deploy.Spec.Template.Spec.Containers) > 0 {
		deploy.Spec.Template.Spec.Containers[0].Image = fmt.Sprintf("%s:%s", deploy.Spec.Template.Spec.Containers[0].Image, su.Spec.TargetVersion)
	}

	deploy.Spec.Strategy.Type = appsv1.RollingUpdateDeploymentStrategyType
	if deploy.Spec.Strategy.RollingUpdate == nil {
		deploy.Spec.Strategy.RollingUpdate = &appsv1.RollingUpdateDeployment{}
	}

	maxUnavailable := intstr.FromInt(su.Spec.Strategy.MaxUnavailable)
	maxSurge := intstr.FromInt(su.Spec.Strategy.MaxSurge)
	deploy.Spec.Strategy.RollingUpdate.MaxUnavailable = &maxUnavailable
	deploy.Spec.Strategy.RollingUpdate.MaxSurge = &maxSurge

	if su.Spec.Strategy.MinReadySeconds > 0 {
		deploy.Spec.MinReadySeconds = int32(su.Spec.Strategy.MinReadySeconds)
	}

	if err := r.Patch(ctx, deploy, client.MergeFrom(original)); err != nil {
		return fmt.Errorf("failed to update deployment %s/%s: %w", comp.HealthCheck.Namespace, comp.HealthCheck.Name, err)
	}

	timeout := 5 * time.Minute
	if comp.HealthCheck.Timeout.Duration != 0 {
		timeout = comp.HealthCheck.Timeout.Duration
	}

	executor := upgrader.NewSelfUpgradeExecutor(r.Client, upgrader.NewHealthChecker(r.Client, timeout))
	if err := executor.WaitForDeploymentReady(ctx, comp.HealthCheck.Namespace, comp.HealthCheck.Name, timeout); err != nil {
		return fmt.Errorf("deployment %s/%s not ready: %w", comp.HealthCheck.Namespace, comp.HealthCheck.Name, err)
	}

	return nil
}

func (r *SelfUpgradeReconciler) rollbackComponent(ctx context.Context, su *cfov1.SelfUpgrade, comp cfov1.SelfUpgradeComponent) error {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Rolling back component", "component", comp.Name)

	switch comp.Type {
	case cfov1.SelfUpgradeComponentTypeDeployment:
		if comp.HealthCheck == nil || comp.HealthCheck.Namespace == "" || comp.HealthCheck.Name == "" {
			return fmt.Errorf("deployment component %s requires healthCheck.namespace and healthCheck.name", comp.Name)
		}

		deploy := &appsv1.Deployment{}
		if err := r.Get(ctx, types.NamespacedName{Namespace: comp.HealthCheck.Namespace, Name: comp.HealthCheck.Name}, deploy); err != nil {
			return fmt.Errorf("failed to get deployment %s/%s: %w", comp.HealthCheck.Namespace, comp.HealthCheck.Name, err)
		}

		if deploy.Status.ObservedGeneration < 2 {
			return fmt.Errorf("no previous revision to rollback to for %s/%s", comp.HealthCheck.Namespace, comp.HealthCheck.Name)
		}

		// Rollback to previous revision
		original := deploy.DeepCopy()
		if len(deploy.Spec.Template.Spec.Containers) > 0 {
			// Remove tag to use previous image
			currentImage := deploy.Spec.Template.Spec.Containers[0].Image
			if idx := strings.LastIndex(currentImage, ":"); idx > 0 {
				deploy.Spec.Template.Spec.Containers[0].Image = currentImage[:idx]
			}
		}
		if err := r.Patch(ctx, deploy, client.MergeFrom(original)); err != nil {
			return fmt.Errorf("failed to rollback deployment %s/%s: %w", comp.HealthCheck.Namespace, comp.HealthCheck.Name, err)
		}

	case cfov1.SelfUpgradeComponentTypeCRD:
		// Restore CRDs from backup ConfigMap
		backupList := &corev1.ConfigMapList{}
		if err := r.List(ctx, backupList, client.InNamespace(su.Namespace), client.MatchingLabels{
			"cvo.capbm.io/self-upgrade": su.Name,
		}); err != nil {
			return fmt.Errorf("failed to list backup ConfigMaps: %w", err)
		}

		if len(backupList.Items) == 0 {
			return fmt.Errorf("no backup found for self-upgrade %s", su.Name)
		}

		// Get latest backup
		latestBackup := &backupList.Items[0]
		for key, content := range latestBackup.Data {
			if strings.HasPrefix(key, "crd-") {
				obj, err := decodeYAML([]byte(content))
				if err != nil {
					log.Error(err, "Failed to decode CRD backup", "key", key)
					continue
				}
				if err := r.Patch(ctx, obj, client.Merge, client.FieldOwner("capbm-self-upgrade-rollback")); err != nil {
					log.Error(err, "Failed to restore CRD from backup", "key", key)
				}
			}
		}

	case cfov1.SelfUpgradeComponentTypeRBAC, cfov1.SelfUpgradeComponentTypeWebhook:
		// Restore from backup ConfigMap
		backupList := &corev1.ConfigMapList{}
		if err := r.List(ctx, backupList, client.InNamespace(su.Namespace), client.MatchingLabels{
			"cvo.capbm.io/self-upgrade": su.Name,
		}); err != nil {
			return fmt.Errorf("failed to list backup ConfigMaps: %w", err)
		}

		if len(backupList.Items) == 0 {
			return fmt.Errorf("no backup found for self-upgrade %s", su.Name)
		}

		latestBackup := &backupList.Items[0]
		prefix := "rbac-"
		if comp.Type == cfov1.SelfUpgradeComponentTypeWebhook {
			prefix = "webhook-"
		}

		for key, content := range latestBackup.Data {
			if strings.HasPrefix(key, prefix) {
				obj, err := decodeYAML([]byte(content))
				if err != nil {
					log.Error(err, "Failed to decode backup", "key", key)
					continue
				}
				if err := r.Patch(ctx, obj, client.Merge, client.FieldOwner("capbm-self-upgrade-rollback")); err != nil {
					log.Error(err, "Failed to restore from backup", "key", key)
				}
			}
		}

	default:
		return fmt.Errorf("rollback not implemented for component type: %s", comp.Type)
	}

	return nil
}

func (r *SelfUpgradeReconciler) waitForCRDEstablished(ctx context.Context, name string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(5 * time.Second):
			crd := &apiextensionsv1.CustomResourceDefinition{}
			if err := r.Get(ctx, types.NamespacedName{Name: name}, crd); err != nil {
				continue
			}

			for _, cond := range crd.Status.Conditions {
				if cond.Type == apiextensionsv1.Established && cond.Status == apiextensionsv1.ConditionTrue {
					return nil
				}
			}
		}
	}
}

// decodeYAML decodes a YAML manifest into a runtime.Object.
func decodeYAML(content []byte) (client.Object, error) {
	var typeMeta metav1.TypeMeta
	if err := yaml.Unmarshal(content, &typeMeta); err != nil {
		return nil, fmt.Errorf("failed to unmarshal TypeMeta: %w", err)
	}

	gv, err := schema.ParseGroupVersion(typeMeta.APIVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to parse apiVersion %s: %w", typeMeta.APIVersion, err)
	}

	gvk := gv.WithKind(typeMeta.Kind)
	runtimeObj, err := scheme.Scheme.New(gvk)
	if err != nil {
		return nil, fmt.Errorf("failed to create object for GVK %v: %w", gvk, err)
	}

	objClient, ok := runtimeObj.(client.Object)
	if !ok {
		return nil, fmt.Errorf("object %v does not implement client.Object", gvk)
	}

	if err := yaml.Unmarshal(content, objClient); err != nil {
		return nil, fmt.Errorf("failed to unmarshal into object: %w", err)
	}

	return objClient, nil
}

// safeImageName converts an image reference to a safe directory name.
func safeImageName(image string) string {
	result := ""
	for _, c := range image {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			result += string(c)
		} else {
			result += "-"
		}
	}
	return result
}
