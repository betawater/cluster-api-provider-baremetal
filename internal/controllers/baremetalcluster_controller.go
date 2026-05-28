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

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	infrav1 "github.com/BetaWater/cluster-api-provider-baremetal/api/v1beta1"
)

// BareMetalClusterReconciler reconciles a BareMetalCluster object.
type BareMetalClusterReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=baremetalclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=baremetalclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=baremetalclusters/finalizers,verbs=update
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch

func (r *BareMetalClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Reconciling BareMetalCluster")

	cluster := &infrav1.BareMetalCluster{}
	if err := r.Get(ctx, req.NamespacedName, cluster); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !cluster.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, cluster)
	}

	return r.reconcileNormal(ctx, cluster)
}

func (r *BareMetalClusterReconciler) reconcileNormal(ctx context.Context, cluster *infrav1.BareMetalCluster) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	controllerutil.AddFinalizer(cluster, infrav1.ClusterFinalizer)
	if err := r.Update(ctx, cluster); err != nil {
		return ctrl.Result{}, err
	}

	capiCluster, err := r.getOwnerCluster(ctx, cluster)
	if err != nil {
		log.Error(err, "failed to get owner Cluster")
		return ctrl.Result{}, err
	}

	if capiCluster == nil {
		log.Info("Waiting for Cluster Controller to set OwnerRef")
		return ctrl.Result{}, nil
	}

	if !capiCluster.Spec.ControlPlaneEndpoint.IsValid() {
		log.Info("ControlPlaneEndpoint not set on Cluster yet")
		return ctrl.Result{}, nil
	}

	if err := r.reconcileControlPlaneEndpoint(ctx, cluster, capiCluster); err != nil {
		log.Error(err, "failed to reconcile ControlPlaneEndpoint")
		return ctrl.Result{}, err
	}

	cluster.Status.Ready = true
	if cluster.Status.Conditions == nil {
		cluster.Status.Conditions = clusterv1.Conditions{}
	}
	setCondition(&cluster.Status.Conditions, clusterv1.ReadyCondition, metav1.ConditionTrue, infrav1.ClusterReadyReason, clusterv1.ConditionSeverityInfo, "Cluster infrastructure is ready")

	return ctrl.Result{}, r.Status().Update(ctx, cluster)
}

func (r *BareMetalClusterReconciler) reconcileControlPlaneEndpoint(ctx context.Context, bmCluster *infrav1.BareMetalCluster, capiCluster *clusterv1.Cluster) error {
	bmCluster.Spec.ControlPlaneEndpoint = capiCluster.Spec.ControlPlaneEndpoint

	if bmCluster.Spec.Network.PodCIDR == "" && capiCluster.Spec.ClusterNetwork != nil && capiCluster.Spec.ClusterNetwork.Pods != nil {
		if len(capiCluster.Spec.ClusterNetwork.Pods.CIDRBlocks) > 0 {
			bmCluster.Spec.Network.PodCIDR = capiCluster.Spec.ClusterNetwork.Pods.CIDRBlocks[0]
		}
	}

	if bmCluster.Spec.Network.ServiceCIDR == "" && capiCluster.Spec.ClusterNetwork != nil && capiCluster.Spec.ClusterNetwork.Services != nil {
		if len(capiCluster.Spec.ClusterNetwork.Services.CIDRBlocks) > 0 {
			bmCluster.Spec.Network.ServiceCIDR = capiCluster.Spec.ClusterNetwork.Services.CIDRBlocks[0]
		}
	}

	return r.Update(ctx, bmCluster)
}

func (r *BareMetalClusterReconciler) reconcileDelete(ctx context.Context, cluster *infrav1.BareMetalCluster) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Deleting BareMetalCluster")

	controllerutil.RemoveFinalizer(cluster, infrav1.ClusterFinalizer)
	if err := r.Update(ctx, cluster); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *BareMetalClusterReconciler) getOwnerCluster(ctx context.Context, bmCluster *infrav1.BareMetalCluster) (*clusterv1.Cluster, error) {
	for _, ref := range bmCluster.OwnerReferences {
		if ref.Kind == "Cluster" && ref.APIVersion == clusterv1.GroupVersion.String() {
			cluster := &clusterv1.Cluster{}
			key := client.ObjectKey{
				Namespace: bmCluster.Namespace,
				Name:      ref.Name,
			}
			if err := r.Get(ctx, key, cluster); err != nil {
				return nil, err
			}
			return cluster, nil
		}
	}
	return nil, nil
}

func setCondition(conditions *clusterv1.Conditions, conditionType clusterv1.ConditionType, status metav1.ConditionStatus, reason string, severity clusterv1.ConditionSeverity, message string) {
	for i, c := range *conditions {
		if c.Type == conditionType {
			(*conditions)[i] = clusterv1.Condition{
				Type:               conditionType,
				Status:             status,
				Reason:             reason,
				Severity:           severity,
				Message:            message,
				LastTransitionTime: metav1.Now(),
			}
			return
		}
	}
	*conditions = append(*conditions, clusterv1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Severity:           severity,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *BareMetalClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1.BareMetalCluster{}).
		Complete(r)
}
