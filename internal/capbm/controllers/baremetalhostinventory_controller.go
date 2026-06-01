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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	capbmv1 "github.com/BetaWater/cluster-api-provider-baremetal/api/capbm/v1beta1"
)

// BareMetalHostInventoryReconciler reconciles a BareMetalHostInventory object.
type BareMetalHostInventoryReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=baremetalhostinventories,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=baremetalhostinventories/status,verbs=get;update;patch

func (r *BareMetalHostInventoryReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Reconciling BareMetalHostInventory")

	inventory := &capbmv1.BareMetalHostInventory{}
	if err := r.Get(ctx, req.NamespacedName, inventory); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	return r.reconcileNormal(ctx, inventory)
}

func (r *BareMetalHostInventoryReconciler) reconcileNormal(ctx context.Context, inventory *capbmv1.BareMetalHostInventory) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Updating host inventory status")

	newStatus := capbmv1.BareMetalHostInventoryStatus{
		TotalHosts: len(inventory.Spec.Hosts),
	}

	for _, host := range inventory.Spec.Hosts {
		hostStatus := capbmv1.HostStatusEntry{
			Name:  host.Name,
			State: capbmv1.HostStateAvailable,
		}

		allocatedMachine, err := r.getAllocatedMachine(ctx, inventory, host.Name)
		if err != nil {
			log.Error(err, "failed to check machine allocation for host", "host", host.Name)
			hostStatus.State = capbmv1.HostStateMaintenance
		} else if allocatedMachine != nil {
			hostStatus.State = capbmv1.HostStateAllocated
			hostStatus.ClusterRef = &corev1.ObjectReference{
				Name:      allocatedMachine.Labels["cluster.x-k8s.io/cluster-name"],
				Namespace: allocatedMachine.Namespace,
			}
		}

		newStatus.HostsStatus = append(newStatus.HostsStatus, hostStatus)

		if hostStatus.State == capbmv1.HostStateAllocated {
			newStatus.AllocatedHosts++
		} else {
			newStatus.AvailableHosts++
		}
	}

	inventory.Status = newStatus

	return ctrl.Result{}, r.Status().Update(ctx, inventory)
}

func (r *BareMetalHostInventoryReconciler) getAllocatedMachine(ctx context.Context, inventory *capbmv1.BareMetalHostInventory, hostName string) (*capbmv1.BareMetalMachine, error) {
	machineList := &capbmv1.BareMetalMachineList{}
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

// SetupWithManager sets up the controller with the Manager.
func (r *BareMetalHostInventoryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&capbmv1.BareMetalHostInventory{}).
		Complete(r)
}
