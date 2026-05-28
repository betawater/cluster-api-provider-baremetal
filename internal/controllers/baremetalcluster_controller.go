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
	"k8s.io/apimachinery/pkg/runtime"
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

	if cluster.Spec.ControlPlaneEndpoint.Host == "" {
		log.Info("ControlPlaneEndpoint not set yet")
		return ctrl.Result{}, nil
	}

	cluster.Status.Ready = true

	return ctrl.Result{}, r.Status().Update(ctx, cluster)
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

// SetupWithManager sets up the controller with the Manager.
func (r *BareMetalClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1.BareMetalCluster{}).
		Complete(r)
}
