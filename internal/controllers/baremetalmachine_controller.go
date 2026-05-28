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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	infrav1 "github.com/BetaWater/cluster-api-provider-baremetal/api/v1beta1"
	"github.com/BetaWater/cluster-api-provider-baremetal/internal/ssh"
)

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

	creds, err := r.getCredentials(ctx, bmMachine)
	if err != nil {
		conditions.MarkFalse(bmMachine, infrav1.MachineReadyCondition, infrav1.CredentialsNotFoundReason, clusterv1.ConditionSeverityError, err.Error())
		return ctrl.Result{RequeueAfter: 10 * time.Second}, r.Status().Update(ctx, bmMachine)
	}

	sshConn, err := r.SSHManager.Connect(bmMachine.Spec.IPAddress, bmMachine.Spec.SSHPort, *creds)
	if err != nil {
		conditions.MarkFalse(bmMachine, infrav1.SSHConnectedCondition, infrav1.SSHConnectionFailedReason, clusterv1.ConditionSeverityWarning, err.Error())
		return ctrl.Result{RequeueAfter: 30 * time.Second}, r.Status().Update(ctx, bmMachine)
	}
	defer func() {
		if sshConn != nil {
			r.SSHManager.Close(sshConn)
		}
	}()

	conditions.MarkTrue(bmMachine, infrav1.SSHConnectedCondition)

	preflightConfig := ssh.DefaultPreflightConfig()
	preflightResult, err := ssh.RunPreflightChecks(ctx, sshConn, preflightConfig)
	if err != nil {
		log.Error(err, "Failed to run pre-flight checks")
	}

	if !preflightResult.Passed {
		conditions.MarkFalse(bmMachine, infrav1.PreFlightChecksPassedCondition, infrav1.PreFlightChecksFailedReason, clusterv1.ConditionSeverityError, fmt.Sprintf("pre-flight checks failed: %v", preflightResult.Errors))
		return ctrl.Result{RequeueAfter: 60 * time.Second}, r.Status().Update(ctx, bmMachine)
	}
	conditions.MarkTrue(bmMachine, infrav1.PreFlightChecksPassedCondition)

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
	conditions.MarkTrue(bmMachine, infrav1.MachineReadyCondition)

	return ctrl.Result{}, r.Status().Update(ctx, bmMachine)
}

func (r *BareMetalMachineReconciler) reconcileDelete(ctx context.Context, bmMachine *infrav1.BareMetalMachine) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Deleting BareMetalMachine")

	controllerutil.RemoveFinalizer(bmMachine, infrav1.MachineFinalizer)
	if err := r.Update(ctx, bmMachine); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
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
