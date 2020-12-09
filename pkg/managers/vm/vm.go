package vm

import (
	"fmt"
	"time"

	"github.com/openshift/cluster-api-provider-kubevirt/pkg/clients/infracluster"
	machinev1 "github.com/openshift/machine-api-operator/pkg/apis/machine/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"

	"github.com/openshift/cluster-api-provider-kubevirt/pkg/clients/tenantcluster"
	machinecontroller "github.com/openshift/machine-api-operator/pkg/controller/machine"
	k8smetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
	kubevirtapiv1 "kubevirt.io/client-go/api/v1"
)

const (
	requeueAfterSeconds      = 20
	requeueAfterFatalSeconds = 180
	masterLabel              = "node-role.kubevirt.io/master"
)

// ProviderVM runs the logic to reconciles a machine resource towards its desired state
type ProviderVM interface {
	Create(machine *machinev1.Machine) error
	Delete(machine *machinev1.Machine) error
	Update(machine *machinev1.Machine) (bool, error)
	Exists(machine *machinev1.Machine) (bool, error)
}

// manager is the struct which implement ProviderVM interface
// Use tenantClusterClient to access secret params assigned by user
// Use infraClusterClientBuilder to create the infra cluster vms
type manager struct {
	infraClusterClient  infracluster.Client
	tenantClusterClient tenantcluster.Client
}

// New creates provider vm instance
func New(infraClusterClient infracluster.Client, tenantClusterClient tenantcluster.Client) ProviderVM {
	return &manager{
		tenantClusterClient: tenantClusterClient,
		infraClusterClient:  infraClusterClient,
	}
}

// Create creates machine if it does not exists.
func (m *manager) Create(machine *machinev1.Machine) (resultErr error) {
	machineScope, err := newMachineScope(machine, m.tenantClusterClient, m.infraClusterClient)
	if err != nil {
		return err
	}

	secretFromMachine, err := machineScope.createIgnitionSecretFromMachine()
	if err != nil {
		return err
	}
	if _, err := m.createInfraClusterSecret(secretFromMachine, machineScope); err != nil {
		klog.Errorf("%s: error creating ignition secret: %v", machineScope.getMachineName(), err)
		conditionFailed := conditionFailed()
		conditionFailed.Message = err.Error()
		return fmt.Errorf("failed to create ignition secret: %w", err)
	}

	virtualMachineFromMachine, err := machineScope.createVirtualMachineFromMachine()
	if err != nil {
		return err
	}

	klog.Infof("%s: create machine", machineScope.getMachineName())

	defer func() {
		// After the operation is done (success or failure)
		// Update the machine object with the relevant changes
		if err := machineScope.patchMachine(); err != nil {
			resultErr = err
		}
	}()

	createdVM, err := m.createInfraClusterVM(virtualMachineFromMachine, machineScope)

	if err != nil {
		klog.Errorf("%s: error creating machine: %v", machineScope.getMachineName(), err)
		conditionFailed := conditionFailed()
		conditionFailed.Message = err.Error()
		return fmt.Errorf("failed to create virtual machine: %w", err)
	}

	klog.Infof("Created Machine %v", machineScope.getMachineName())

	if err := m.syncMachine(createdVM, machineScope); err != nil {
		klog.Errorf("%s: fail syncing machine from vm: %v", machineScope.getMachineName(), err)
		return err
	}

	return nil
}

// delete deletes machine
func (m *manager) Delete(machine *machinev1.Machine) error {
	machineScope, err := newMachineScope(machine, m.tenantClusterClient, m.infraClusterClient)
	if err != nil {
		return err
	}

	virtualMachineFromMachine, err := machineScope.createVirtualMachineFromMachine()
	if err != nil {
		return err
	}

	klog.Infof("%s: delete machine", machineScope.getMachineName())

	existingVM, err := m.getInraClusterVM(virtualMachineFromMachine.GetName(), virtualMachineFromMachine.GetNamespace(), machineScope)
	if err != nil {
		if errors.IsNotFound(err) {
			klog.Infof("%s: VM does not exist", machineScope.getMachineName())
			return nil
		}

		klog.Errorf("%s: error getting existing VM: %v", machineScope.getMachineName(), err)
		return err
	}

	if existingVM == nil {
		klog.Warningf("%s: VM not found to delete for machine", machineScope.getMachineName())
		return nil
	}

	if err := m.deleteInraClusterVM(existingVM.GetName(), existingVM.GetNamespace(), machineScope); err != nil {
		return fmt.Errorf("failed to delete VM: %w", err)
	}

	klog.Infof("Deleted machine %v", machineScope.getMachineName())

	return nil
}

// update finds a vm and reconciles the machine resource status against it.
func (m *manager) Update(machine *machinev1.Machine) (wasUpdated bool, resultErr error) {
	machineScope, err := newMachineScope(machine, m.tenantClusterClient, m.infraClusterClient)
	if err != nil {
		return false, err
	}

	virtualMachineFromMachine, err := machineScope.createVirtualMachineFromMachine()
	if err != nil {
		return false, err
	}

	klog.Infof("%s: update machine", machineScope.getMachineName())

	defer func() {
		// After the operation is done (success or failure)
		// Update the machine object with the relevant changes
		if err := machineScope.patchMachine(); err != nil {
			resultErr = err
		}
	}()

	wasUpdated, updatedVM, err := m.updateVM(err, virtualMachineFromMachine, machineScope)
	if err != nil {
		return false, err
	}

	if err := m.syncMachine(updatedVM, machineScope); err != nil {
		klog.Errorf("%s: fail syncing machine from vm: %v", machineScope.getMachineName(), err)
		return false, err
	}
	return wasUpdated, nil
}

func (m *manager) updateVM(err error, virtualMachineFromMachine *kubevirtapiv1.VirtualMachine, machineScope *machineScope) (bool, *kubevirtapiv1.VirtualMachine, error) {
	existingVM, err := m.getInraClusterVM(virtualMachineFromMachine.GetName(), virtualMachineFromMachine.GetNamespace(), machineScope)
	if err != nil {
		klog.Errorf("%s: error getting existing VM: %v", machineScope.getMachineName(), err)
		return false, nil, err
	}
	if existingVM == nil {
		if machineScope.updateAllowed() {
			klog.Infof("%s: Possible eventual-consistency discrepancy; returning an error to requeue", machineScope.getMachineName())
			return false, nil, &machinecontroller.RequeueAfterError{RequeueAfter: requeueAfterSeconds * time.Second}
		}
		klog.Warningf("%s: attempted to update machine but the VM found", machineScope.getMachineName())

		// This is an unrecoverable error condition.  We should delay to
		// minimize unnecessary API calls.
		return false, nil, &machinecontroller.RequeueAfterError{RequeueAfter: requeueAfterFatalSeconds * time.Second}
	}

	previousResourceVersion := existingVM.ResourceVersion
	virtualMachineFromMachine.ObjectMeta.ResourceVersion = previousResourceVersion

	//TODO remove it after pushing that PR: https://github.com/kubevirt/kubevirt/pull/3889
	virtualMachineFromMachine.Status = kubevirtapiv1.VirtualMachineStatus{
		Created: existingVM.Status.Created,
		Ready:   existingVM.Status.Ready,
	}

	updatedVM, err := m.updateInraClusterVM(virtualMachineFromMachine, machineScope)
	if err != nil {
		return false, nil, fmt.Errorf("failed to update VM: %w", err)
	}
	currentResourceVersion := updatedVM.ResourceVersion

	klog.Infof("Updated machine %s", machineScope.getMachineName())

	wasUpdated := previousResourceVersion != currentResourceVersion
	return wasUpdated, updatedVM, nil
}

func (m *manager) syncMachine(vm *kubevirtapiv1.VirtualMachine, machineScope *machineScope) error {
	vmi, err := m.getInraClusterVMI(vm.Name, vm.Namespace, machineScope)
	if err != nil {
		klog.Errorf("%s: error getting vmi for machine: %v", machineScope.getMachineName(), err)
	}
	if err := machineScope.SyncMachineFromVm(vm, vmi); err != nil {
		klog.Errorf("%s: fail syncing machine from vm: %v", machineScope.getMachineName(), err)
		return err
	}
	return nil
}

// exists returns true if machine exists.
func (m *manager) Exists(machine *machinev1.Machine) (bool, error) {
	machineScope, err := newMachineScope(machine, m.tenantClusterClient, m.infraClusterClient)
	if err != nil {
		return false, err
	}

	klog.Infof("%s: check if machine exists", machineScope.getMachineName())
	existingVM, err := m.getInraClusterVM(machine.GetName(), machineScope.vmNamespace, machineScope)
	if err != nil {
		if errors.IsNotFound(err) {
			klog.Infof("%s: VM does not exist", machineScope.getMachineName())
			return false, nil
		}
		klog.Errorf("%s: error getting existing VM: %v", machineScope.getMachineName(), err)
		return false, err
	}

	if existingVM == nil {
		klog.Infof("%s: VM does not exist", machineScope.getMachineName())
		return false, nil
	}

	return true, nil
}

func (m *manager) createInfraClusterVM(virtualMachine *kubevirtapiv1.VirtualMachine, machineScope *machineScope) (*kubevirtapiv1.VirtualMachine, error) {
	return machineScope.infraClusterClient.CreateVirtualMachine(virtualMachine.Namespace, virtualMachine)
}

func (m *manager) createInfraClusterSecret(secret *corev1.Secret, machineScope *machineScope) (*corev1.Secret, error) {
	return machineScope.infraClusterClient.CreateSecret(secret.Namespace, secret)
}

func (m *manager) getInraClusterVM(vmName, vmNamespace string, machineScope *machineScope) (*kubevirtapiv1.VirtualMachine, error) {
	return machineScope.infraClusterClient.GetVirtualMachine(vmNamespace, vmName, &k8smetav1.GetOptions{})
}
func (m *manager) getInraClusterVMI(vmName, vmNamespace string, machineScope *machineScope) (*kubevirtapiv1.VirtualMachineInstance, error) {
	return machineScope.infraClusterClient.GetVirtualMachineInstance(vmNamespace, vmName, &k8smetav1.GetOptions{})
}

func (m *manager) deleteInraClusterVM(vmName, vmNamespace string, machineScope *machineScope) error {
	gracePeriod := int64(10)
	return machineScope.infraClusterClient.DeleteVirtualMachine(vmNamespace, vmName, &k8smetav1.DeleteOptions{GracePeriodSeconds: &gracePeriod})
}

func (m *manager) updateInraClusterVM(updatedVM *kubevirtapiv1.VirtualMachine, machineScope *machineScope) (*kubevirtapiv1.VirtualMachine, error) {
	return machineScope.infraClusterClient.UpdateVirtualMachine(updatedVM.Namespace, updatedVM)
}

// isMaster returns true if the machine is part of a cluster's control plane
func (m *manager) isMaster() (bool, error) {
	// TODO implement
	// if p.machine.Status.NodeRef == nil {
	// 	klog.Errorf("NodeRef not found in machine %s", p.machine.Name)
	// 	return false, nil
	// }
	// node := &corev1.Node{}
	// nodeKey := types.NamespacedName{
	// 	Namespace: p.machine.Status.NodeRef.Namespace,
	// 	Name:      p.machine.Status.NodeRef.Name,
	// }

	// err := p.client.Get(p.Context, nodeKey, node)
	// if err != nil {
	// 	return false, fmt.Errorf("failed to get node from machine %s", p.machine.Name)
	// }

	// if _, exists := node.Labels[masterLabel]; exists {
	// 	return true, nil
	// }
	return false, nil
}

func (m *manager) requeueIfInstancePending(vm *kubevirtapiv1.VirtualMachine, machineName string) error {
	// If machine state is still pending, we will return an error to keep the controllers
	// attempting to update status until it hits a more permanent state. This will ensure
	// we get a public IP populated more quickly.
	if !vm.Status.Ready {
		klog.Infof("%s: VM status is not ready, returning an error to requeue", machineName)
		return &machinecontroller.RequeueAfterError{RequeueAfter: requeueAfterSeconds * time.Second}
	}

	return nil
}
