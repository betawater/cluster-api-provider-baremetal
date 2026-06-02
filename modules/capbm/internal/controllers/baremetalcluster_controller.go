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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	capbmv1 "github.com/BetaWater/cluster-api-provider-baremetal/modules/capbm/api/v1beta1"
	"github.com/BetaWater/cluster-api-provider-baremetal/modules/capbm/internal/lb"
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
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines;machines/status,verbs=get;list;watch

func (r *BareMetalClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Reconciling BareMetalCluster")

	cluster := &capbmv1.BareMetalCluster{}
	if err := r.Get(ctx, req.NamespacedName, cluster); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !cluster.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, cluster)
	}

	return r.reconcileNormal(ctx, cluster)
}

func (r *BareMetalClusterReconciler) reconcileNormal(ctx context.Context, bmCluster *capbmv1.BareMetalCluster) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	controllerutil.AddFinalizer(bmCluster, capbmv1.ClusterFinalizer)
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
		markConditionFalse(&bmCluster.Status.Conditions, clusterv1.ReadyCondition, capbmv1.EndpointNotSetReason, clusterv1.ConditionSeverityError, err.Error())
		return ctrl.Result{}, r.Status().Update(ctx, bmCluster)
	}

	if !endpoint.IsValid() {
		log.Info("ControlPlaneEndpoint not available from any source",
			"clusterEndpoint", capiCluster.Spec.ControlPlaneEndpoint,
			"infraEndpoint", bmCluster.Spec.ControlPlaneEndpoint)
		markConditionFalse(&bmCluster.Status.Conditions, clusterv1.ReadyCondition, capbmv1.EndpointNotSetReason, clusterv1.ConditionSeverityInfo, "Waiting for ControlPlaneEndpoint to be set")
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

	if err := r.reconcileLoadBalancer(ctx, bmCluster, capiCluster); err != nil {
		log.Error(err, "failed to sync load balancer backends")
		markConditionFalse(&bmCluster.Status.Conditions, capbmv1.LoadBalancerReadyCondition, capbmv1.LBSyncFailedReason, clusterv1.ConditionSeverityWarning, err.Error())
		if err := r.Status().Update(ctx, bmCluster); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	if bmCluster.Spec.LoadBalancer != nil {
		markConditionTrue(&bmCluster.Status.Conditions, capbmv1.LoadBalancerReadyCondition, "Load balancer backends are synced")
	}

	if err := r.reconcileIngressLoadBalancer(ctx, bmCluster, capiCluster); err != nil {
		log.Error(err, "failed to sync ingress load balancer backends")
		markConditionFalse(&bmCluster.Status.Conditions, capbmv1.IngressLoadBalancerReadyCondition, capbmv1.IngressLBSyncFailedReason, clusterv1.ConditionSeverityWarning, err.Error())
		if err := r.Status().Update(ctx, bmCluster); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	if bmCluster.Spec.IngressLoadBalancer != nil && bmCluster.Spec.IngressLoadBalancer.Enabled {
		markConditionTrue(&bmCluster.Status.Conditions, capbmv1.IngressLoadBalancerReadyCondition, "Ingress load balancer backends are synced")
	}

	bmCluster.Spec.ControlPlaneEndpoint = endpoint
	bmCluster.Status.Ready = true

	provisioned := true
	bmCluster.Status.Initialization = &capbmv1.BareMetalClusterInitializationStatus{
		Provisioned: &provisioned,
	}

	markConditionTrue(&bmCluster.Status.Conditions, clusterv1.ReadyCondition, "Cluster infrastructure is ready (endpoint source: "+source+")")

	return ctrl.Result{}, r.Status().Update(ctx, bmCluster)
}

func (r *BareMetalClusterReconciler) resolveControlPlaneEndpoint(ctx context.Context, bmCluster *capbmv1.BareMetalCluster, capiCluster *clusterv1.Cluster) (clusterv1.APIEndpoint, string, error) {
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

func (r *BareMetalClusterReconciler) reconcileNetworkConfig(ctx context.Context, bmCluster *capbmv1.BareMetalCluster, capiCluster *clusterv1.Cluster) error {
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

func (r *BareMetalClusterReconciler) reconcileLoadBalancer(ctx context.Context, bmCluster *capbmv1.BareMetalCluster, capiCluster *clusterv1.Cluster) error {
	log := ctrl.LoggerFrom(ctx)

	lbConfig := bmCluster.Spec.LoadBalancer
	if lbConfig == nil {
		return nil
	}

	provider, err := lb.NewProvider(lbConfig)
	if err != nil {
		return fmt.Errorf("failed to create load balancer provider: %w", err)
	}
	if provider == nil {
		return nil
	}

	cpMachines, err := r.getControlPlaneMachines(ctx, capiCluster)
	if err != nil {
		return fmt.Errorf("failed to get control plane machines: %w", err)
	}

	desiredBackends := make(map[string]lb.Backend)
	for _, m := range cpMachines {
		if m.Status.Phase == string(clusterv1.MachinePhaseRunning) {
			ip := getMachineIP(m)
			if ip != "" {
				desiredBackends[m.Name] = lb.Backend{
					Name: m.Name,
					IP:   ip,
					Port: int(bmCluster.Spec.ControlPlaneEndpoint.Port),
				}
			}
		}
	}

	for name, backend := range desiredBackends {
		log.Info("Registering backend to load balancer", "name", name, "ip", backend.IP, "port", backend.Port)
	}

	return r.syncBackends(ctx, provider, desiredBackends)
}

func (r *BareMetalClusterReconciler) getControlPlaneMachines(ctx context.Context, capiCluster *clusterv1.Cluster) ([]clusterv1.Machine, error) {
	machineList := &clusterv1.MachineList{}
	if err := r.List(ctx, machineList, client.InNamespace(capiCluster.Namespace), client.MatchingLabels{
		clusterv1.ClusterNameLabel: capiCluster.Name,
	}); err != nil {
		return nil, err
	}

	var cpMachines []clusterv1.Machine
	for _, m := range machineList.Items {
		if m.Spec.ClusterName == capiCluster.Name {
			if m.Labels[clusterv1.MachineControlPlaneLabel] == "" {
				continue
			}
			cpMachines = append(cpMachines, m)
		}
	}

	return cpMachines, nil
}

func getMachineIP(machine clusterv1.Machine) string {
	for _, addr := range machine.Status.Addresses {
		if addr.Type == clusterv1.MachineInternalIP {
			return addr.Address
		}
	}
	for _, addr := range machine.Status.Addresses {
		if addr.Type == clusterv1.MachineExternalIP {
			return addr.Address
		}
	}
	return ""
}

func (r *BareMetalClusterReconciler) reconcileIngressLoadBalancer(ctx context.Context, bmCluster *capbmv1.BareMetalCluster, capiCluster *clusterv1.Cluster) error {
	ingressConfig := bmCluster.Spec.IngressLoadBalancer
	if ingressConfig == nil || !ingressConfig.Enabled {
		return nil
	}

	workerMachines, err := r.getWorkerMachines(ctx, capiCluster)
	if err != nil {
		return fmt.Errorf("failed to get worker machines: %w", err)
	}

	switch ingressConfig.Provider {
	case "haproxy":
		return r.syncIngressHAProxy(ctx, ingressConfig.HAProxy, workerMachines)
	case "f5":
		return r.syncIngressF5(ctx, ingressConfig.F5, workerMachines)
	case "metal-lb":
		return r.syncIngressMetalLB(ctx, ingressConfig.MetalLB, workerMachines)
	default:
		return fmt.Errorf("unsupported ingress load balancer provider: %s", ingressConfig.Provider)
	}
}

func (r *BareMetalClusterReconciler) syncIngressHAProxy(ctx context.Context, config *capbmv1.IngressHAProxyConfig, workerMachines []clusterv1.Machine) error {
	if config == nil {
		return nil
	}

	httpPort := config.HTTPPort
	if httpPort == 0 {
		httpPort = 30080
	}
	httpsPort := config.HTTPSPort
	if httpsPort == 0 {
		httpsPort = 30443
	}

	backendName := config.BackendName
	if backendName == "" {
		backendName = "k8s-ingress"
	}

	desiredBackends := make(map[string]lb.Backend)
	for _, m := range workerMachines {
		if m.Status.Phase != string(clusterv1.MachinePhaseRunning) {
			continue
		}
		ip := getMachineIP(m)
		if ip == "" {
			continue
		}
		desiredBackends[m.Name+"-http"] = lb.Backend{
			Name: m.Name + "-http",
			IP:   ip,
			Port: httpPort,
		}
		desiredBackends[m.Name+"-https"] = lb.Backend{
			Name: m.Name + "-https",
			IP:   ip,
			Port: httpsPort,
		}
	}

	haproxyConfig := &capbmv1.HAProxyConfig{
		AdminHost:   config.AdminHost,
		AdminPort:   config.AdminPort,
		SSHHost:     config.SSHHost,
		SSHPort:     config.SSHPort,
		BackendName: backendName + "-http",
	}

	provider, err := lb.NewHAProxyProvider(haproxyConfig)
	if err != nil {
		return err
	}

	if err := r.syncBackends(ctx, provider, desiredBackends); err != nil {
		return fmt.Errorf("failed to sync HTTP backends: %w", err)
	}

	haproxyConfig.BackendName = backendName + "-https"
	provider, err = lb.NewHAProxyProvider(haproxyConfig)
	if err != nil {
		return err
	}

	return r.syncBackends(ctx, provider, desiredBackends)
}

func (r *BareMetalClusterReconciler) syncIngressF5(ctx context.Context, config *capbmv1.IngressF5Config, workerMachines []clusterv1.Machine) error {
	if config == nil {
		return nil
	}

	httpPort := config.HTTPPort
	if httpPort == 0 {
		httpPort = 30080
	}
	httpsPort := config.HTTPSPort
	if httpsPort == 0 {
		httpsPort = 30443
	}

	httpPoolName := config.HTTPPoolName
	if httpPoolName == "" {
		httpPoolName = "k8s-ingress-http"
	}
	httpsPoolName := config.HTTPSPoolName
	if httpsPoolName == "" {
		httpsPoolName = "k8s-ingress-https"
	}

	desiredHTTPBackends := make(map[string]lb.Backend)
	desiredHTTPSBackends := make(map[string]lb.Backend)
	for _, m := range workerMachines {
		if m.Status.Phase != string(clusterv1.MachinePhaseRunning) {
			continue
		}
		ip := getMachineIP(m)
		if ip == "" {
			continue
		}
		desiredHTTPBackends[m.Name+"-http"] = lb.Backend{
			Name: m.Name + "-http",
			IP:   ip,
			Port: httpPort,
		}
		desiredHTTPSBackends[m.Name+"-https"] = lb.Backend{
			Name: m.Name + "-https",
			IP:   ip,
			Port: httpsPort,
		}
	}

	f5Config := &capbmv1.F5Config{
		Host:           config.Host,
		Port:           config.Port,
		CredentialsRef: config.CredentialsRef,
		Partition:      config.Partition,
		PoolName:       httpPoolName,
	}

	provider, err := lb.NewF5Provider(f5Config)
	if err != nil {
		return err
	}

	if err := r.syncBackends(ctx, provider, desiredHTTPBackends); err != nil {
		return fmt.Errorf("failed to sync HTTP backends: %w", err)
	}

	f5Config.PoolName = httpsPoolName
	provider, err = lb.NewF5Provider(f5Config)
	if err != nil {
		return err
	}

	return r.syncBackends(ctx, provider, desiredHTTPSBackends)
}

func (r *BareMetalClusterReconciler) syncIngressMetalLB(ctx context.Context, config *capbmv1.IngressMetalLBConfig, workerMachines []clusterv1.Machine) error {
	if config == nil {
		return nil
	}
	return nil
}

func (r *BareMetalClusterReconciler) getWorkerMachines(ctx context.Context, capiCluster *clusterv1.Cluster) ([]clusterv1.Machine, error) {
	machineList := &clusterv1.MachineList{}
	if err := r.List(ctx, machineList, client.InNamespace(capiCluster.Namespace), client.MatchingLabels{
		clusterv1.ClusterNameLabel: capiCluster.Name,
	}); err != nil {
		return nil, err
	}

	var workerMachines []clusterv1.Machine
	for _, m := range machineList.Items {
		if m.Spec.ClusterName == capiCluster.Name {
			if _, isCP := m.Labels[clusterv1.MachineControlPlaneLabel]; !isCP {
				workerMachines = append(workerMachines, m)
			}
		}
	}

	return workerMachines, nil
}

func (r *BareMetalClusterReconciler) syncBackends(ctx context.Context, provider lb.Provider, desiredBackends map[string]lb.Backend) error {
	currentBackends, err := provider.GetBackends(ctx)
	if err != nil {
		return fmt.Errorf("failed to get current backends: %w", err)
	}

	currentBackendMap := make(map[string]lb.Backend)
	for _, b := range currentBackends {
		currentBackendMap[b.Name] = b
	}

	toAdd := make(map[string]lb.Backend)
	toRemove := make(map[string]lb.Backend)

	for name, backend := range desiredBackends {
		if _, exists := currentBackendMap[name]; !exists {
			toAdd[name] = backend
		}
	}
	for name, backend := range currentBackendMap {
		if _, exists := desiredBackends[name]; !exists {
			toRemove[name] = backend
		}
	}

	for name, backend := range toAdd {
		if err := provider.RegisterBackend(ctx, backend); err != nil {
			return fmt.Errorf("failed to register backend %s: %w", name, err)
		}
	}

	for name, backend := range toRemove {
		if err := provider.UnregisterBackend(ctx, backend); err != nil {
			return fmt.Errorf("failed to unregister backend %s: %w", name, err)
		}
	}

	return nil
}

func (r *BareMetalClusterReconciler) reconcileDelete(ctx context.Context, cluster *capbmv1.BareMetalCluster) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Deleting BareMetalCluster")

	controllerutil.RemoveFinalizer(cluster, capbmv1.ClusterFinalizer)
	if err := r.Update(ctx, cluster); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *BareMetalClusterReconciler) getOwnerCluster(ctx context.Context, bmCluster *capbmv1.BareMetalCluster) (*clusterv1.Cluster, error) {
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
		For(&capbmv1.BareMetalCluster{}).
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(clusterToBareMetalCluster(mgr)),
		).
		Complete(r)
}
