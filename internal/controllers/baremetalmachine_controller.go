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
	"sigs.k8s.io/cluster-api/util"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	infrav1 "github.com/BetaWater/cluster-api-provider-baremetal/api/v1beta1"
	"github.com/BetaWater/cluster-api-provider-baremetal/internal/ssh"
)

func setMachineCondition(bmMachine *infrav1.BareMetalMachine, conditionType clusterv1.ConditionType, status corev1.ConditionStatus, reason string, severity clusterv1.ConditionSeverity, message string) {
	condition := clusterv1.Condition{
		Type:               conditionType,
		Status:             status,
		Reason:             reason,
		Severity:           severity,
		Message:            message,
		LastTransitionTime: metav1.Now(),
	}

	conditions := bmMachine.GetConditions()
	for i, c := range conditions {
		if c.Type == conditionType {
			conditions[i] = condition
			bmMachine.SetConditions(conditions)
			return
		}
	}
	conditions = append(conditions, condition)
	bmMachine.SetConditions(conditions)
}

func markMachineConditionFalse(bmMachine *infrav1.BareMetalMachine, conditionType clusterv1.ConditionType, reason string, severity clusterv1.ConditionSeverity, message string) {
	setMachineCondition(bmMachine, conditionType, corev1.ConditionFalse, reason, severity, message)
}

func markMachineConditionTrue(bmMachine *infrav1.BareMetalMachine, conditionType clusterv1.ConditionType) {
	setMachineCondition(bmMachine, conditionType, corev1.ConditionTrue, "", clusterv1.ConditionSeverityInfo, "")
}

// BareMetalMachineReconciler reconciles a BareMetalMachine object.
type BareMetalMachineReconciler struct {
	client.Client
	Log        logr.Logger
	Scheme     *runtime.Scheme
	SSHManager *ssh.SSHManager
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=baremetalmachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=baremetalmachines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=baremetalmachines/finalizers,verbs=update
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=baremetalhostinventories,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=baremetalhostinventories/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines;machines/status,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (r *BareMetalMachineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Reconciling BareMetalMachine")

	bmMachine := &infrav1.BareMetalMachine{}
	if err := r.Get(ctx, req.NamespacedName, bmMachine); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	machine, err := util.GetOwnerMachine(ctx, r.Client, bmMachine.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}
	if machine == nil {
		log.Info("Waiting for Machine Controller to set OwnerRef")
		return ctrl.Result{}, nil
	}

	if !bmMachine.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, bmMachine)
	}

	return r.reconcileNormal(ctx, bmMachine, machine)
}

func (r *BareMetalMachineReconciler) reconcileNormal(ctx context.Context, bmMachine *infrav1.BareMetalMachine, machine *clusterv1.Machine) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	controllerutil.AddFinalizer(bmMachine, infrav1.MachineFinalizer)
	if err := r.Update(ctx, bmMachine); err != nil {
		return ctrl.Result{}, err
	}

	baremetalCluster, err := r.getBareMetalCluster(ctx, bmMachine)
	if err != nil {
		log.Info("BareMetalCluster is not ready yet")
		return ctrl.Result{}, nil
	}
	if baremetalCluster == nil || !baremetalCluster.Status.Ready {
		log.Info("Waiting for BareMetalCluster to be ready")
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// Allocate host from inventory if not already allocated
	if bmMachine.Spec.HostName == "" || bmMachine.Spec.IPAddress == "" {
		if bmMachine.Spec.HostInventoryRef == nil {
			markMachineConditionFalse(bmMachine, infrav1.MachineReadyCondition, infrav1.InvalidConfigurationReason, clusterv1.ConditionSeverityError, "hostInventoryRef is required when hostName/ipAddress not specified")
			return ctrl.Result{}, nil
		}

		host, err := r.allocateHostFromInventory(ctx, bmMachine)
		if err != nil {
			log.Error(err, "Failed to allocate host from inventory")
			markMachineConditionFalse(bmMachine, infrav1.MachineReadyCondition, "HostAllocationFailed", clusterv1.ConditionSeverityError, err.Error())
			return ctrl.Result{RequeueAfter: 30 * time.Second}, r.Status().Update(ctx, bmMachine)
		}

		bmMachine.Spec.HostName = host.HostName
		bmMachine.Spec.IPAddress = host.IPAddress
		bmMachine.Spec.SSHPort = host.SSHPort
		bmMachine.Spec.CredentialsRef = &host.CredentialsRef
		if err := r.Update(ctx, bmMachine); err != nil {
			return ctrl.Result{}, err
		}
		log.Info("Allocated host from inventory", "hostName", host.HostName, "ipAddress", host.IPAddress)
	}

	if bmMachine.Spec.CredentialsRef == nil {
		markMachineConditionFalse(bmMachine, infrav1.MachineReadyCondition, infrav1.InvalidConfigurationReason, clusterv1.ConditionSeverityError, "credentialsRef is required")
		return ctrl.Result{}, nil
	}

	creds, err := r.getCredentials(ctx, bmMachine)
	if err != nil {
		markMachineConditionFalse(bmMachine, infrav1.MachineReadyCondition, infrav1.CredentialsNotFoundReason, clusterv1.ConditionSeverityError, err.Error())
		return ctrl.Result{RequeueAfter: 10 * time.Second}, r.Status().Update(ctx, bmMachine)
	}

	sshConn, err := r.SSHManager.Connect(bmMachine.Spec.IPAddress, bmMachine.Spec.SSHPort, *creds)
	if err != nil {
		markMachineConditionFalse(bmMachine, infrav1.SSHConnectedCondition, infrav1.SSHConnectionFailedReason, clusterv1.ConditionSeverityWarning, err.Error())
		return ctrl.Result{RequeueAfter: 30 * time.Second}, r.Status().Update(ctx, bmMachine)
	}
	defer func() {
		if sshConn != nil {
			r.SSHManager.Close(sshConn)
		}
	}()

	markMachineConditionTrue(bmMachine, infrav1.SSHConnectedCondition)

	preflightConfig := ssh.DefaultPreflightConfig()
	preflightResult, err := ssh.RunPreflightChecks(ctx, sshConn, preflightConfig)
	if err != nil {
		log.Error(err, "Failed to run pre-flight checks")
	}

	if !preflightResult.Passed {
		markMachineConditionFalse(bmMachine, infrav1.PreFlightChecksPassedCondition, infrav1.PreFlightChecksFailedReason, clusterv1.ConditionSeverityError, fmt.Sprintf("pre-flight checks failed: %v", preflightResult.Errors))
		return ctrl.Result{RequeueAfter: 60 * time.Second}, r.Status().Update(ctx, bmMachine)
	}
	markMachineConditionTrue(bmMachine, infrav1.PreFlightChecksPassedCondition)

	providerID := fmt.Sprintf("baremetal://%s", bmMachine.Spec.HostName)
	if bmMachine.Spec.ProviderID == nil || *bmMachine.Spec.ProviderID != providerID {
		bmMachine.Spec.ProviderID = &providerID
		if err := r.Update(ctx, bmMachine); err != nil {
			return ctrl.Result{}, err
		}
	}

	bmMachine.Status.Ready = true
	bmMachine.Status.ProviderID = providerID
	bmMachine.Status.Addresses = []clusterv1.MachineAddress{
		{Type: clusterv1.MachineInternalIP, Address: bmMachine.Spec.IPAddress},
		{Type: clusterv1.MachineHostName, Address: bmMachine.Spec.HostName},
	}
	markMachineConditionTrue(bmMachine, infrav1.MachineReadyCondition)

	return ctrl.Result{}, r.Status().Update(ctx, bmMachine)
}

func (r *BareMetalMachineReconciler) reconcileDelete(ctx context.Context, bmMachine *infrav1.BareMetalMachine) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Deleting BareMetalMachine")

	// Release host back to inventory if it was allocated
	if bmMachine.Spec.HostInventoryRef != nil && bmMachine.Spec.HostName != "" {
		if err := r.releaseHostToInventory(ctx, bmMachine); err != nil {
			log.Error(err, "Failed to release host to inventory", "hostName", bmMachine.Spec.HostName)
		}
	}

	controllerutil.RemoveFinalizer(bmMachine, infrav1.MachineFinalizer)
	if err := r.Update(ctx, bmMachine); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *BareMetalMachineReconciler) allocateHostFromInventory(ctx context.Context, bmMachine *infrav1.BareMetalMachine) (*infrav1.HostEntry, error) {
	inventory := &infrav1.BareMetalHostInventory{}
	inventoryKey := types.NamespacedName{
		Namespace: bmMachine.Namespace,
		Name:      bmMachine.Spec.HostInventoryRef.Name,
	}

	if err := r.Get(ctx, inventoryKey, inventory); err != nil {
		return nil, fmt.Errorf("failed to get host inventory %s: %w", inventoryKey, err)
	}

	for i, host := range inventory.Spec.Hosts {
		if bmMachine.Spec.Role != "" && host.Role != bmMachine.Spec.Role {
			continue
		}

		if inventory.Status.HostsStatus != nil && i < len(inventory.Status.HostsStatus) {
			if inventory.Status.HostsStatus[i].State != infrav1.HostStateAvailable {
				continue
			}
		}

		allocatedMachine, err := r.getAllocatedMachine(ctx, inventory, host.Name)
		if err != nil {
			return nil, err
		}
		if allocatedMachine != nil {
			continue
		}

		if inventory.Status.HostsStatus == nil {
			inventory.Status.HostsStatus = make([]infrav1.HostStatusEntry, len(inventory.Spec.Hosts))
		}
		if i >= len(inventory.Status.HostsStatus) {
			inventory.Status.HostsStatus = append(inventory.Status.HostsStatus, make([]infrav1.HostStatusEntry, i-len(inventory.Status.HostsStatus)+1)...)
		}

		clusterName := bmMachine.Labels[clusterv1.ClusterNameLabel]
		inventory.Status.HostsStatus[i] = infrav1.HostStatusEntry{
			Name:  host.Name,
			State: infrav1.HostStateAllocated,
			ClusterRef: &corev1.ObjectReference{
				Name:      clusterName,
				Namespace: bmMachine.Namespace,
			},
		}

		inventory.Status.AvailableHosts--
		inventory.Status.AllocatedHosts++

		if err := r.Status().Update(ctx, inventory); err != nil {
			return nil, fmt.Errorf("failed to update host inventory status: %w", err)
		}

		return &host, nil
	}

	return nil, fmt.Errorf("no available hosts in inventory %s matching role %s", inventory.Name, bmMachine.Spec.Role)
}

func (r *BareMetalMachineReconciler) releaseHostToInventory(ctx context.Context, bmMachine *infrav1.BareMetalMachine) error {
	inventory := &infrav1.BareMetalHostInventory{}
	inventoryKey := types.NamespacedName{
		Namespace: bmMachine.Namespace,
		Name:      bmMachine.Spec.HostInventoryRef.Name,
	}

	if err := r.Get(ctx, inventoryKey, inventory); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get host inventory %s: %w", inventoryKey, err)
	}

	for i, host := range inventory.Spec.Hosts {
		if host.Name == bmMachine.Spec.HostName {
			if inventory.Status.HostsStatus != nil && i < len(inventory.Status.HostsStatus) {
				inventory.Status.HostsStatus[i].State = infrav1.HostStateAvailable
				inventory.Status.HostsStatus[i].ClusterRef = nil
			}

			inventory.Status.AvailableHosts++
			if inventory.Status.AllocatedHosts > 0 {
				inventory.Status.AllocatedHosts--
			}

			if err := r.Status().Update(ctx, inventory); err != nil {
				return fmt.Errorf("failed to update host inventory status: %w", err)
			}
			break
		}
	}

	return nil
}

func (r *BareMetalMachineReconciler) getAllocatedMachine(ctx context.Context, inventory *infrav1.BareMetalHostInventory, hostName string) (*infrav1.BareMetalMachine, error) {
	machineList := &infrav1.BareMetalMachineList{}
	if err := r.List(ctx, machineList, client.InNamespace(inventory.Namespace)); err != nil {
		return nil, err
	}

	for _, machine := range machineList.Items {
		if machine.Spec.HostName == hostName && machine.DeletionTimestamp.IsZero() {
			return &machine, nil
		}
	}

	return nil, nil
}

func (r *BareMetalMachineReconciler) getBareMetalCluster(ctx context.Context, bmMachine *infrav1.BareMetalMachine) (*infrav1.BareMetalCluster, error) {
	clusterName := bmMachine.Labels[clusterv1.ClusterNameLabel]
	if clusterName == "" {
		return nil, nil
	}

	baremetalCluster := &infrav1.BareMetalCluster{}
	baremetalClusterKey := types.NamespacedName{
		Namespace: bmMachine.Namespace,
		Name:      clusterName,
	}

	if err := r.Get(ctx, baremetalClusterKey, baremetalCluster); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	return baremetalCluster, nil
}

func (r *BareMetalMachineReconciler) getCredentials(ctx context.Context, bmMachine *infrav1.BareMetalMachine) (*ssh.Credentials, error) {
	if bmMachine.Spec.CredentialsRef == nil {
		return nil, fmt.Errorf("credentialsRef is not set")
	}

	secret := &corev1.Secret{}
	secretKey := types.NamespacedName{
		Namespace: bmMachine.Namespace,
		Name:      bmMachine.Spec.CredentialsRef.Name,
	}

	if err := r.Get(ctx, secretKey, secret); err != nil {
		return nil, fmt.Errorf("failed to get credentials secret %s: %w", bmMachine.Spec.CredentialsRef.Name, err)
	}

	username, ok := secret.Data["username"]
	if !ok {
		return nil, fmt.Errorf("credentials secret %s missing username field", bmMachine.Spec.CredentialsRef.Name)
	}

	password, ok := secret.Data["password"]
	if !ok {
		return nil, fmt.Errorf("credentials secret %s missing password field", bmMachine.Spec.CredentialsRef.Name)
	}

	return &ssh.Credentials{
		Username: string(username),
		Password: string(password),
	}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BareMetalMachineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1.BareMetalMachine{}).
		Watches(
			&clusterv1.Machine{},
			handler.EnqueueRequestsFromMapFunc(util.MachineToInfrastructureMapFunc(infrav1.GroupVersion.WithKind("BareMetalMachine"))),
		).
		Complete(r)
}
