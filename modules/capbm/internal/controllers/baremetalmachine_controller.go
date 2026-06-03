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

	capbmv1 "github.com/BetaWater/cluster-api-provider-baremetal/modules/capbm/api/v1beta1"
	
	cfov1 "github.com/BetaWater/cluster-api-provider-baremetal/modules/cvo/api/v1beta1"
	"github.com/BetaWater/cluster-api-provider-baremetal/modules/capbm/internal/cni"
	"github.com/BetaWater/cluster-api-provider-baremetal/modules/capbm/internal/csi"
	"github.com/BetaWater/cluster-api-provider-baremetal/modules/capbm/internal/health"
	"github.com/BetaWater/cluster-api-provider-baremetal/modules/capbm/internal/installer"
	"github.com/BetaWater/cluster-api-provider-baremetal/modules/capbm/internal/network"
	"github.com/BetaWater/cluster-api-provider-baremetal/modules/cvo/pkg/ssh"
)

//nolint:staticcheck // ConditionType deprecated in CAPI v1beta2, will migrate when ready
func setMachineCondition(bmMachine *capbmv1.BareMetalMachine, conditionType clusterv1.ConditionType, status corev1.ConditionStatus, reason string, severity clusterv1.ConditionSeverity, message string) {
	//nolint:staticcheck // Condition deprecated in CAPI v1beta2, will migrate when ready
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

//nolint:staticcheck // ConditionType deprecated in CAPI v1beta2, will migrate when ready
func markMachineConditionFalse(bmMachine *capbmv1.BareMetalMachine, conditionType clusterv1.ConditionType, reason string, severity clusterv1.ConditionSeverity, message string) {
	setMachineCondition(bmMachine, conditionType, corev1.ConditionFalse, reason, severity, message)
}

//nolint:staticcheck // ConditionType deprecated in CAPI v1beta2, will migrate when ready
func markMachineConditionTrue(bmMachine *capbmv1.BareMetalMachine, conditionType clusterv1.ConditionType) {
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

	bmMachine := &capbmv1.BareMetalMachine{}
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

func (r *BareMetalMachineReconciler) reconcileNormal(ctx context.Context, bmMachine *capbmv1.BareMetalMachine, machine *clusterv1.Machine) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	controllerutil.AddFinalizer(bmMachine, capbmv1.MachineFinalizer)
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
			markMachineConditionFalse(bmMachine, capbmv1.MachineReadyCondition, capbmv1.InvalidConfigurationReason, clusterv1.ConditionSeverityError, "hostInventoryRef is required when hostName/ipAddress not specified")
			return ctrl.Result{}, nil
		}

		host, err := r.allocateHostFromInventory(ctx, bmMachine)
		if err != nil {
			log.Error(err, "Failed to allocate host from inventory")
			markMachineConditionFalse(bmMachine, capbmv1.MachineReadyCondition, "HostAllocationFailed", clusterv1.ConditionSeverityError, err.Error())
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
		markMachineConditionFalse(bmMachine, capbmv1.MachineReadyCondition, capbmv1.InvalidConfigurationReason, clusterv1.ConditionSeverityError, "credentialsRef is required")
		return ctrl.Result{}, nil
	}

	creds, err := r.getCredentials(ctx, bmMachine)
	if err != nil {
		markMachineConditionFalse(bmMachine, capbmv1.MachineReadyCondition, capbmv1.CredentialsNotFoundReason, clusterv1.ConditionSeverityError, err.Error())
		return ctrl.Result{RequeueAfter: 10 * time.Second}, r.Status().Update(ctx, bmMachine)
	}

	sshConn, err := r.SSHManager.Connect(bmMachine.Spec.IPAddress, bmMachine.Spec.SSHPort, *creds)
	if err != nil {
		markMachineConditionFalse(bmMachine, capbmv1.SSHConnectedCondition, capbmv1.SSHConnectionFailedReason, clusterv1.ConditionSeverityWarning, err.Error())
		return ctrl.Result{RequeueAfter: 30 * time.Second}, r.Status().Update(ctx, bmMachine)
	}
	defer func() {
		if sshConn != nil {
			r.SSHManager.Close(sshConn)
		}
	}()

	markMachineConditionTrue(bmMachine, capbmv1.SSHConnectedCondition)

	preflightConfig := ssh.DefaultPreflightConfig()
	preflightResult, err := ssh.RunPreflightChecks(ctx, sshConn, preflightConfig)
	if err != nil {
		log.Error(err, "Failed to run pre-flight checks")
	}

	if !preflightResult.Passed {
		markMachineConditionFalse(bmMachine, capbmv1.PreFlightChecksPassedCondition, capbmv1.PreFlightChecksFailedReason, clusterv1.ConditionSeverityError, fmt.Sprintf("pre-flight checks failed: %v", preflightResult.Errors))
		return ctrl.Result{RequeueAfter: 60 * time.Second}, r.Status().Update(ctx, bmMachine)
	}
	markMachineConditionTrue(bmMachine, capbmv1.PreFlightChecksPassedCondition)

	if err := r.configureFirewall(ctx, bmMachine, sshConn); err != nil {
		log.Error(err, "Failed to configure firewall")
		markMachineConditionFalse(bmMachine, capbmv1.FirewallConfiguredCondition, capbmv1.FirewallConfigFailedReason, clusterv1.ConditionSeverityWarning, err.Error())
	} else {
		markMachineConditionTrue(bmMachine, capbmv1.FirewallConfiguredCondition)
	}

	if err := r.configureSELinux(ctx, bmMachine, sshConn); err != nil {
		log.Error(err, "Failed to configure SELinux")
		markMachineConditionFalse(bmMachine, capbmv1.SELinuxConfiguredCondition, capbmv1.SELinuxConfigFailedReason, clusterv1.ConditionSeverityWarning, err.Error())
	} else {
		markMachineConditionTrue(bmMachine, capbmv1.SELinuxConfiguredCondition)
	}

	k8sVersion := extractK8sVersion(machine)
	installResult, err := r.installComponents(ctx, bmMachine, sshConn, k8sVersion)
	if err != nil {
		log.Error(err, "Failed to install components")
		markMachineConditionFalse(bmMachine, capbmv1.ComponentsInstalledCondition, capbmv1.ComponentInstallFailedReason, clusterv1.ConditionSeverityError, err.Error())
		return ctrl.Result{RequeueAfter: 60 * time.Second}, r.Status().Update(ctx, bmMachine)
	}

	if !installResult.Completed {
		log.Info("Component installation in progress", "progress", installResult.Progress)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	if !installResult.Success {
		log.Error(fmt.Errorf("installation failed"), "Component installation failed", "error", installResult.Error)
		markMachineConditionFalse(bmMachine, capbmv1.ComponentsInstalledCondition, capbmv1.ComponentInstallFailedReason, clusterv1.ConditionSeverityError, installResult.Error)
		return ctrl.Result{RequeueAfter: 60 * time.Second}, r.Status().Update(ctx, bmMachine)
	}

	markMachineConditionTrue(bmMachine, capbmv1.ComponentsInstalledCondition)

	bmMachine.Status.InstalledComponents = capbmv1.ComponentVersions{
		ContainerRuntime: installResult.ComponentVersions.ContainerRuntime,
		Kubeadm:          installResult.ComponentVersions.Kubeadm,
		Kubelet:          installResult.ComponentVersions.Kubelet,
		Kubectl:          installResult.ComponentVersions.Kubectl,
		OSType:           installResult.ComponentVersions.OSType,
		OSVersion:        installResult.ComponentVersions.OSVersion,
	}

	if result, err := r.installCNI(ctx, bmMachine, sshConn); err != nil || !result.Completed {
		if err != nil {
			log.Error(err, "Failed to install CNI")
			markMachineConditionFalse(bmMachine, capbmv1.CNIInstalledCondition, capbmv1.CNIInstallFailedReason, clusterv1.ConditionSeverityError, err.Error())
			return ctrl.Result{RequeueAfter: 60 * time.Second}, r.Status().Update(ctx, bmMachine)
		}
		log.Info("CNI installation in progress", "progress", result.Error)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	} else if !result.Success {
		log.Error(fmt.Errorf("CNI installation failed"), "CNI installation failed", "error", result.Error)
		markMachineConditionFalse(bmMachine, capbmv1.CNIInstalledCondition, capbmv1.CNIInstallFailedReason, clusterv1.ConditionSeverityError, result.Error)
		return ctrl.Result{RequeueAfter: 60 * time.Second}, r.Status().Update(ctx, bmMachine)
	} else {
		markMachineConditionTrue(bmMachine, capbmv1.CNIInstalledCondition)
		bmMachine.Status.InstalledComponents.CNI = result.Version
		if config := bmMachine.Spec.ComponentInstall; config != nil {
			bmMachine.Status.InstalledComponents.CNIType = config.CNI.Type
		}
	}

	if result, err := r.installCSI(ctx, bmMachine, sshConn); err != nil || !result.Completed {
		if err != nil {
			log.Error(err, "Failed to install CSI")
			markMachineConditionFalse(bmMachine, capbmv1.CSIInstalledCondition, capbmv1.CSIInstallFailedReason, clusterv1.ConditionSeverityError, err.Error())
			return ctrl.Result{RequeueAfter: 60 * time.Second}, r.Status().Update(ctx, bmMachine)
		}
		log.Info("CSI installation in progress", "progress", result.Error)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	} else if !result.Success {
		log.Error(fmt.Errorf("CSI installation failed"), "CSI installation failed", "error", result.Error)
		markMachineConditionFalse(bmMachine, capbmv1.CSIInstalledCondition, capbmv1.CSIInstallFailedReason, clusterv1.ConditionSeverityError, result.Error)
		return ctrl.Result{RequeueAfter: 60 * time.Second}, r.Status().Update(ctx, bmMachine)
	} else {
		markMachineConditionTrue(bmMachine, capbmv1.CSIInstalledCondition)
		bmMachine.Status.InstalledComponents.CSI = result.Version
		if config := bmMachine.Spec.ComponentInstall; config != nil {
			bmMachine.Status.InstalledComponents.CSIDriver = config.CSI.Driver
		}
	}

	verifier := health.NewVerifier(sshConn)
	verificationResult, err := verifier.VerifyInstallation(ctx)
	if err != nil {
		log.Error(err, "Failed to verify installation")
	} else if !verificationResult.Passed {
		log.Info("Installation verification failed", "errors", verificationResult.Errors)
	}

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
	markMachineConditionTrue(bmMachine, capbmv1.MachineReadyCondition)

	return ctrl.Result{}, r.Status().Update(ctx, bmMachine)
}

func (r *BareMetalMachineReconciler) reconcileDelete(ctx context.Context, bmMachine *capbmv1.BareMetalMachine) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Deleting BareMetalMachine")

	// Release host back to inventory if it was allocated
	if bmMachine.Spec.HostInventoryRef != nil && bmMachine.Spec.HostName != "" {
		if err := r.releaseHostToInventory(ctx, bmMachine); err != nil {
			log.Error(err, "Failed to release host to inventory", "hostName", bmMachine.Spec.HostName)
		}
	}

	controllerutil.RemoveFinalizer(bmMachine, capbmv1.MachineFinalizer)
	if err := r.Update(ctx, bmMachine); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *BareMetalMachineReconciler) allocateHostFromInventory(ctx context.Context, bmMachine *capbmv1.BareMetalMachine) (*capbmv1.HostEntry, error) {
	inventory := &capbmv1.BareMetalHostInventory{}
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
			if inventory.Status.HostsStatus[i].State != capbmv1.HostStateAvailable {
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
			inventory.Status.HostsStatus = make([]capbmv1.HostStatusEntry, len(inventory.Spec.Hosts))
		}
		if i >= len(inventory.Status.HostsStatus) {
			inventory.Status.HostsStatus = append(inventory.Status.HostsStatus, make([]capbmv1.HostStatusEntry, i-len(inventory.Status.HostsStatus)+1)...)
		}

		clusterName := bmMachine.Labels[clusterv1.ClusterNameLabel]
		inventory.Status.HostsStatus[i] = capbmv1.HostStatusEntry{
			Name:  host.Name,
			State: capbmv1.HostStateAllocated,
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

func (r *BareMetalMachineReconciler) releaseHostToInventory(ctx context.Context, bmMachine *capbmv1.BareMetalMachine) error {
	inventory := &capbmv1.BareMetalHostInventory{}
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
				inventory.Status.HostsStatus[i].State = capbmv1.HostStateAvailable
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

func (r *BareMetalMachineReconciler) getAllocatedMachine(ctx context.Context, inventory *capbmv1.BareMetalHostInventory, hostName string) (*capbmv1.BareMetalMachine, error) {
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

func (r *BareMetalMachineReconciler) getBareMetalCluster(ctx context.Context, bmMachine *capbmv1.BareMetalMachine) (*capbmv1.BareMetalCluster, error) {
	clusterName := bmMachine.Labels[clusterv1.ClusterNameLabel]
	if clusterName == "" {
		return nil, nil
	}

	baremetalCluster := &capbmv1.BareMetalCluster{}
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

func (r *BareMetalMachineReconciler) getCredentials(ctx context.Context, bmMachine *capbmv1.BareMetalMachine) (*ssh.Credentials, error) {
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

func (r *BareMetalMachineReconciler) installComponents(ctx context.Context, bmMachine *capbmv1.BareMetalMachine, sshConn *ssh.SSHConnection, k8sVersion string) (*installer.InstallResult, error) {
	log := ctrl.LoggerFrom(ctx)

	config := bmMachine.Spec.ComponentInstall
	if config == nil {
		config = &capbmv1.ComponentInstallConfig{
			Enabled:  true,
			Strategy: capbmv1.InstallIfMissing,
			ContainerRuntime: capbmv1.ContainerRuntimeConfig{
				Type: "containerd",
			},
			MaxRetries: 3,
		}
	}

	if !config.Enabled || config.Strategy == capbmv1.Skip {
		log.Info("Component installation disabled or skipped")
		return &installer.InstallResult{Completed: true, Success: true, Progress: "Installation disabled or skipped"}, nil
	}

	role := bmMachine.Spec.Role
	if role == "" {
		role = "worker"
	}

	// Try to install via ReleaseImage if ReleaseImageRef is set
	if bmMachine.Spec.ReleaseImageRef != nil {
		releaseImage := &cfov1.ReleaseImage{}
		if err := r.Get(ctx, types.NamespacedName{Name: bmMachine.Spec.ReleaseImageRef.Name}, releaseImage); err != nil {
			log.Error(err, "Failed to get ReleaseImage, falling back to legacy install", "releaseImageRef", bmMachine.Spec.ReleaseImageRef.Name)
		} else {
			inst := installer.NewWithReleaseImage(sshConn, releaseImage, config, role)
			return inst.Install(ctx)
		}
	}

	// Fallback to legacy install with explicit k8sVersion
	inst := installer.New(sshConn, config, k8sVersion, role)
	return inst.Install(ctx)
}

func (r *BareMetalMachineReconciler) configureFirewall(ctx context.Context, bmMachine *capbmv1.BareMetalMachine, sshConn *ssh.SSHConnection) error {
	role := bmMachine.Spec.Role
	if role == "" {
		role = "worker"
	}

	fwManager := network.NewFirewallManager(sshConn, bmMachine.Spec.Firewall, role)
	return fwManager.Configure(ctx)
}

func (r *BareMetalMachineReconciler) configureSELinux(ctx context.Context, bmMachine *capbmv1.BareMetalMachine, sshConn *ssh.SSHConnection) error {
	selinuxManager := network.NewSELinuxManager(sshConn, bmMachine.Spec.SELinux)
	return selinuxManager.Configure(ctx)
}

func extractK8sVersion(machine *clusterv1.Machine) string {
	if machine == nil {
		return ""
	}
	version := machine.Spec.Version
	if version == "" {
		return ""
	}
	if len(version) > 0 && version[0] == 'v' {
		return version[1:]
	}
	return version
}

func (r *BareMetalMachineReconciler) installCNI(ctx context.Context, bmMachine *capbmv1.BareMetalMachine, sshConn *ssh.SSHConnection) (*cni.InstallResult, error) {
	log := ctrl.LoggerFrom(ctx)

	if bmMachine.Spec.ComponentInstall == nil {
		return &cni.InstallResult{Completed: true, Success: true}, nil
	}

	cniConfig := bmMachine.Spec.ComponentInstall.CNI
	if !cniConfig.Enabled {
		log.Info("CNI installation disabled")
		return &cni.InstallResult{Completed: true, Success: true}, nil
	}

	podCIDR := "10.244.0.0/16"
	if cniConfig.Config != nil && cniConfig.Config.PodCIDR != "" {
		podCIDR = cniConfig.Config.PodCIDR
	}

	// Try to use ReleaseImage if available
	if bmMachine.Spec.ReleaseImageRef != nil {
		releaseImage := &cfov1.ReleaseImage{}
		if err := r.Get(ctx, types.NamespacedName{Name: bmMachine.Spec.ReleaseImageRef.Name}, releaseImage); err == nil {
			inst := cni.NewFromReleaseImage(sshConn, releaseImage, cniConfig, podCIDR)
			return inst.Install(ctx)
		}
		log.Info("Failed to get ReleaseImage for CNI, falling back to legacy install", "releaseImageRef", bmMachine.Spec.ReleaseImageRef.Name)
	}

	inst := cni.New(sshConn, cniConfig, podCIDR)
	return inst.Install(ctx)
}

func (r *BareMetalMachineReconciler) installCSI(ctx context.Context, bmMachine *capbmv1.BareMetalMachine, sshConn *ssh.SSHConnection) (*csi.InstallResult, error) {
	log := ctrl.LoggerFrom(ctx)

	if bmMachine.Spec.ComponentInstall == nil {
		return &csi.InstallResult{Completed: true, Success: true}, nil
	}

	csiConfig := bmMachine.Spec.ComponentInstall.CSI
	if !csiConfig.Enabled {
		log.Info("CSI installation disabled")
		return &csi.InstallResult{Completed: true, Success: true}, nil
	}

	// Try to use ReleaseImage if available
	if bmMachine.Spec.ReleaseImageRef != nil {
		releaseImage := &cfov1.ReleaseImage{}
		if err := r.Get(ctx, types.NamespacedName{Name: bmMachine.Spec.ReleaseImageRef.Name}, releaseImage); err == nil {
			inst := csi.NewFromReleaseImage(sshConn, releaseImage, csiConfig)
			return inst.Install(ctx)
		}
		log.Info("Failed to get ReleaseImage for CSI, falling back to legacy install", "releaseImageRef", bmMachine.Spec.ReleaseImageRef.Name)
	}

	inst := csi.New(sshConn, csiConfig)
	return inst.Install(ctx)
}

// SetupWithManager sets up the controller with the Manager.
func (r *BareMetalMachineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&capbmv1.BareMetalMachine{}).
		Watches(
			&clusterv1.Machine{},
			handler.EnqueueRequestsFromMapFunc(util.MachineToInfrastructureMapFunc(capbmv1.GroupVersion.WithKind("BareMetalMachine"))),
		).
		Complete(r)
}
