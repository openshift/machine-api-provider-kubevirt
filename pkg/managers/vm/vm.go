package vm

import (
	"context"
	"fmt"
	"time"

	"github.com/openshift/cluster-api-provider-kubevirt/pkg/clients/infracluster"
	"github.com/openshift/cluster-api-provider-kubevirt/pkg/machinescope"
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
	Create(machineScope machinescope.MachineScope) error
	Delete(machineScope machinescope.MachineScope) error
	Update(machineScope machinescope.MachineScope) (bool, error)
	Exists(machineScope machinescope.MachineScope) (bool, error)
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
func (m *manager) Create(machineScope machinescope.MachineScope) (resultErr error) {
	secretFromMachine, err := machineScope.CreateIgnitionSecretFromMachine()
	if err != nil {
		return err
	}
	if _, err := m.createInfraClusterSecret(secretFromMachine, machineScope); err != nil {
		klog.Errorf("%s: error creating ignition secret: %v", machineScope.GetMachineName(), err)
		conditionFailed := conditionFailed()
		conditionFailed.Message = err.Error()
		return fmt.Errorf("failed to create ignition secret: %w", err)
	}

	virtualMachineFromMachine, err := machineScope.CreateVirtualMachineFromMachine()
	if err != nil {
		return err
	}

	klog.Infof("%s: create machine", machineScope.GetMachineName())

	defer func() {
		// After the operation is done (success or failure)
		// Update the machine object with the relevant changes
		if err := machineScope.PatchMachine(); err != nil {
			resultErr = err
		}
	}()

	createdVM, err := m.createInfraClusterVM(virtualMachineFromMachine, machineScope)

	if err != nil {
		klog.Errorf("%s: error creating machine: %v", machineScope.GetMachineName(), err)
		conditionFailed := conditionFailed()
		conditionFailed.Message = err.Error()
		return fmt.Errorf("failed to create virtual machine: %w", err)
	}

	klog.Infof("Created Machine %v", machineScope.GetMachineName())

	if err := m.syncMachine(createdVM, machineScope); err != nil {
		klog.Errorf("%s: fail syncing machine from vm: %v", machineScope.GetMachineName(), err)
		return err
	}

	return nil
}

// delete deletes machine
func (m *manager) Delete(machineScope machinescope.MachineScope) error {
	virtualMachineFromMachine, err := machineScope.CreateVirtualMachineFromMachine()
	if err != nil {
		return err
	}

	klog.Infof("%s: delete machine", machineScope.GetMachineName())

	existingVM, err := m.getInraClusterVM(virtualMachineFromMachine.GetName(), virtualMachineFromMachine.GetNamespace(), machineScope)
	if err != nil {
		if errors.IsNotFound(err) {
			klog.Infof("%s: VM does not exist", machineScope.GetMachineName())
			return nil
		}

		klog.Errorf("%s: error getting existing VM: %v", machineScope.GetMachineName(), err)
		return err
	}

	if existingVM == nil {
		klog.Warningf("%s: VM not found to delete for machine", machineScope.GetMachineName())
		return nil
	}

	if err := m.deleteInraClusterVM(existingVM.GetName(), existingVM.GetNamespace(), machineScope); err != nil {
		return fmt.Errorf("failed to delete VM: %w", err)
	}

	klog.Infof("Deleted machine %v", machineScope.GetMachineName())

	return nil
}

// update finds a vm and reconciles the machine resource status against it.
func (m *manager) Update(machineScope machinescope.MachineScope) (wasUpdated bool, resultErr error) {
	virtualMachineFromMachine, err := machineScope.CreateVirtualMachineFromMachine()
	if err != nil {
		return false, err
	}

	klog.Infof("%s: update machine", machineScope.GetMachineName())

	defer func() {
		// After the operation is done (success or failure)
		// Update the machine object with the relevant changes
		if err := machineScope.PatchMachine(); err != nil {
			resultErr = err
		}
	}()

	wasUpdated, updatedVM, err := m.updateVM(err, virtualMachineFromMachine, machineScope)
	if err != nil {
		return false, err
	}

	if err := m.syncMachine(updatedVM, machineScope); err != nil {
		klog.Errorf("%s: fail syncing machine from vm: %v", machineScope.GetMachineName(), err)
		return false, err
	}
	return wasUpdated, nil
}

func (m *manager) updateVM(err error, virtualMachineFromMachine *kubevirtapiv1.VirtualMachine, machineScope machinescope.MachineScope) (bool, *kubevirtapiv1.VirtualMachine, error) {
	existingVM, err := m.getInraClusterVM(virtualMachineFromMachine.GetName(), virtualMachineFromMachine.GetNamespace(), machineScope)
	if err != nil {
		klog.Errorf("%s: error getting existing VM: %v", machineScope.GetMachineName(), err)
		return false, nil, err
	}
	if existingVM == nil {
		if machineScope.UpdateAllowed(requeueAfterSeconds) {
			klog.Infof("%s: Possible eventual-consistency discrepancy; returning an error to requeue", machineScope.GetMachineName())
			return false, nil, &machinecontroller.RequeueAfterError{RequeueAfter: requeueAfterSeconds * time.Second}
		}
		klog.Warningf("%s: attempted to update machine but the VM found", machineScope.GetMachineName())

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

	klog.Infof("Updated machine %s", machineScope.GetMachineName())

	wasUpdated := previousResourceVersion != currentResourceVersion
	return wasUpdated, updatedVM, nil
}

func (m *manager) syncMachine(vm *kubevirtapiv1.VirtualMachine, machineScope machinescope.MachineScope) error {
	vmi, err := m.getInraClusterVMI(vm.Name, vm.Namespace, machineScope)
	if err != nil {
		klog.Errorf("%s: error getting vmi for machine: %v", machineScope.GetMachineName(), err)
	}
	if err := machineScope.SyncMachineFromVm(vm, vmi); err != nil {
		klog.Errorf("%s: fail syncing machine from vm: %v", machineScope.GetMachineName(), err)
		return err
	}
	return nil
}

// exists returns true if machine exists.
func (m *manager) Exists(machineScope machinescope.MachineScope) (bool, error) {
	klog.Infof("%s: check if machine exists", machineScope.GetMachineName())
	existingVM, err := m.getInraClusterVM(machineScope.GetMachineName(), machineScope.VmNamespace(), machineScope)
	if err != nil {
		if errors.IsNotFound(err) {
			klog.Infof("%s: VM does not exist", machineScope.GetMachineName())
			return false, nil
		}
		klog.Errorf("%s: error getting existing VM: %v", machineScope.GetMachineName(), err)
		return false, err
	}

	if existingVM == nil {
		klog.Infof("%s: VM does not exist", machineScope.GetMachineName())
		return false, nil
	}

	return true, nil
}

func (m *manager) createInfraClusterVM(virtualMachine *kubevirtapiv1.VirtualMachine, machineScope machinescope.MachineScope) (*kubevirtapiv1.VirtualMachine, error) {
	return m.infraClusterClient.CreateVirtualMachine(context.Background(), virtualMachine.Namespace, virtualMachine)
}

func (m *manager) createInfraClusterSecret(secret *corev1.Secret, machineScope machinescope.MachineScope) (*corev1.Secret, error) {
	return m.infraClusterClient.CreateSecret(context.Background(), secret.Namespace, secret)
}

func (m *manager) getInraClusterVM(vmName, vmNamespace string, machineScope machinescope.MachineScope) (*kubevirtapiv1.VirtualMachine, error) {
	return m.infraClusterClient.GetVirtualMachine(context.Background(), vmNamespace, vmName, &k8smetav1.GetOptions{})
}
func (m *manager) getInraClusterVMI(vmName, vmNamespace string, machineScope machinescope.MachineScope) (*kubevirtapiv1.VirtualMachineInstance, error) {
	return m.infraClusterClient.GetVirtualMachineInstance(context.Background(), vmNamespace, vmName, &k8smetav1.GetOptions{})
}

func (m *manager) deleteInraClusterVM(vmName, vmNamespace string, machineScope machinescope.MachineScope) error {
	gracePeriod := int64(10)
	return m.infraClusterClient.DeleteVirtualMachine(context.Background(), vmNamespace, vmName, &k8smetav1.DeleteOptions{GracePeriodSeconds: &gracePeriod})
}

func (m *manager) updateInraClusterVM(updatedVM *kubevirtapiv1.VirtualMachine, machineScope machinescope.MachineScope) (*kubevirtapiv1.VirtualMachine, error) {
	return m.infraClusterClient.UpdateVirtualMachine(context.Background(), updatedVM.Namespace, updatedVM)
}
