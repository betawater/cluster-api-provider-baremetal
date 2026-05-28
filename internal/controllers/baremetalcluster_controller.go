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
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	infrav1 "github.com/BetaWater/cluster-api-provider-baremetal/api/v1beta1"
)

const (
	// EndpointSourceAnnotation indicates the source of the control plane endpoint.
	EndpointSourceAnnotation = "baremetal.cluster.x-k8s.io/endpoint-source"

	defaultRequeueTime = 10 * time.Second
	defaultDNSDomain   = "cluster.local"
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

func (r *BareMetalClusterReconciler) reconcileNormal(ctx context.Context, bmCluster *infrav1.BareMetalCluster) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	controllerutil.AddFinalizer(bmCluster, infrav1.ClusterFinalizer)
	if err := r.Update(ctx, bmCluster); err != nil {
		return ctrl.Result{}, err
	}

	capiCluster, err := r.getOwnerCluster(ctx, bmCluster)
	if err != nil {
		log.Error(err, "failed to get owner Cluster")
		return ctrl.Result{}, err
	}

	if capiCluster == nil {
		log.Info("Waiting for Cluster Controller to set OwnerRef")
		return ctrl.Result{}, nil
	}

	endpoint, source, err := r.resolveControlPlaneEndpoint(ctx, bmCluster, capiCluster)
	if err != nil {
		markConditionFalse(&bmCluster.Status.Conditions, clusterv1.ReadyCondition, infrav1.EndpointNotSetReason, clusterv1.ConditionSeverityError, err.Error())
		return ctrl.Result{}, r.Status().Update(ctx, bmCluster)
	}

	if !endpoint.IsValid() {
		log.Info("ControlPlaneEndpoint not available from any source",
			"clusterEndpoint", capiCluster.Spec.ControlPlaneEndpoint,
			"infraEndpoint", bmCluster.Spec.ControlPlaneEndpoint)
		markConditionFalse(&bmCluster.Status.Conditions, clusterv1.ReadyCondition, infrav1.EndpointNotSetReason, clusterv1.ConditionSeverityInfo, "Waiting for ControlPlaneEndpoint to be set")
		return ctrl.Result{RequeueAfter: defaultRequeueTime}, nil
	}

	if bmCluster.Annotations == nil {
		bmCluster.Annotations = make(map[string]string)
	}
	bmCluster.Annotations[EndpointSourceAnnotation] = source

	if err := r.reconcileNetworkConfig(ctx, bmCluster, capiCluster); err != nil {
		log.Error(err, "failed to reconcile network config")
		return ctrl.Result{}, err
	}

	bmCluster.Spec.ControlPlaneEndpoint = endpoint
	bmCluster.Status.Ready = true

	provisioned := true
	bmCluster.Status.Initialization = &infrav1.BareMetalClusterInitializationStatus{
		Provisioned: &provisioned,
	}

	markConditionTrue(&bmCluster.Status.Conditions, clusterv1.ReadyCondition, "Cluster infrastructure is ready (endpoint source: "+source+")")

	return ctrl.Result{}, r.Status().Update(ctx, bmCluster)
}

func (r *BareMetalClusterReconciler) resolveControlPlaneEndpoint(ctx context.Context, bmCluster *infrav1.BareMetalCluster, capiCluster *clusterv1.Cluster) (clusterv1.APIEndpoint, string, error) {
	log := ctrl.LoggerFrom(ctx)

	clusterEndpoint := capiCluster.Spec.ControlPlaneEndpoint
	infraEndpoint := bmCluster.Spec.ControlPlaneEndpoint

	clusterValid := clusterEndpoint.IsValid()
	infraValid := infraEndpoint.IsValid()

	switch {
	case clusterValid && infraValid:
		if clusterEndpoint.Host != infraEndpoint.Host || clusterEndpoint.Port != infraEndpoint.Port {
			log.Info("ControlPlaneEndpoint mismatch between Cluster and BareMetalCluster, using Cluster's endpoint",
				"clusterEndpoint", clusterEndpoint,
				"infraEndpoint", infraEndpoint)
		}
		return clusterEndpoint, "cluster", nil

	case clusterValid:
		log.Info("Using ControlPlaneEndpoint from Cluster resource", "endpoint", clusterEndpoint)
		return clusterEndpoint, "cluster", nil

	case infraValid:
		log.Info("Using ControlPlaneEndpoint from BareMetalCluster resource", "endpoint", infraEndpoint)
		return infraEndpoint, "infrastructure", nil

	default:
		return clusterv1.APIEndpoint{}, "", nil
	}
}

func (r *BareMetalClusterReconciler) reconcileNetworkConfig(ctx context.Context, bmCluster *infrav1.BareMetalCluster, capiCluster *clusterv1.Cluster) error {
	clusterNetwork := capiCluster.Spec.ClusterNetwork

	if bmCluster.Spec.Network.PodCIDR == "" {
		if len(clusterNetwork.Pods.CIDRBlocks) > 0 {
			bmCluster.Spec.Network.PodCIDR = clusterNetwork.Pods.CIDRBlocks[0]
		}
	}

	if bmCluster.Spec.Network.ServiceCIDR == "" {
		if len(clusterNetwork.Services.CIDRBlocks) > 0 {
			bmCluster.Spec.Network.ServiceCIDR = clusterNetwork.Services.CIDRBlocks[0]
		}
	}

	if bmCluster.Spec.Network.DNSDomain == "" {
		if clusterNetwork.ServiceDomain != "" {
			bmCluster.Spec.Network.DNSDomain = clusterNetwork.ServiceDomain
		} else {
			bmCluster.Spec.Network.DNSDomain = defaultDNSDomain
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

func markConditionFalse(conditions *clusterv1.Conditions, conditionType clusterv1.ConditionType, reason string, severity clusterv1.ConditionSeverity, message string) {
	setCondition(conditions, conditionType, corev1.ConditionFalse, reason, severity, message)
}

func markConditionTrue(conditions *clusterv1.Conditions, conditionType clusterv1.ConditionType, message string) {
	setCondition(conditions, conditionType, corev1.ConditionTrue, "", clusterv1.ConditionSeverityInfo, message)
}

func setCondition(conditions *clusterv1.Conditions, conditionType clusterv1.ConditionType, status corev1.ConditionStatus, reason string, severity clusterv1.ConditionSeverity, message string) {
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

func clusterToBareMetalCluster(mgr ctrl.Manager) handler.MapFunc {
	return func(ctx context.Context, o client.Object) []reconcile.Request {
		cluster, ok := o.(*clusterv1.Cluster)
		if !ok {
			return nil
		}

		if cluster.Spec.InfrastructureRef.Name == "" {
			return nil
		}

		if cluster.Spec.InfrastructureRef.Kind != "BareMetalCluster" {
			return nil
		}

		return []reconcile.Request{
			{
				NamespacedName: client.ObjectKey{
					Namespace: cluster.Namespace,
					Name:      cluster.Spec.InfrastructureRef.Name,
				},
			},
		}
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *BareMetalClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1.BareMetalCluster{}).
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(clusterToBareMetalCluster(mgr)),
		).
		Complete(r)
}
