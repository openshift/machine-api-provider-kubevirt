package vm

import (
	"errors"
	"fmt"
	"testing"

	kubevirtapiv1 "kubevirt.io/client-go/api/v1"

	machinev1 "github.com/openshift/machine-api-operator/pkg/apis/machine/v1beta1"

	"github.com/golang/mock/gomock"
	"github.com/openshift/cluster-api-provider-kubevirt/pkg/clients/infracluster"
	mockInfraClusterClient "github.com/openshift/cluster-api-provider-kubevirt/pkg/clients/infracluster/mock"
	"github.com/openshift/cluster-api-provider-kubevirt/pkg/clients/tenantcluster"
	mockTenantClusterClient "github.com/openshift/cluster-api-provider-kubevirt/pkg/clients/tenantcluster/mock"
	"gotest.tools/assert"
)

const (
	clusterNamespace = "kubevirt-actuator-cluster"
	infraID          = "test-id-asdfg"
)

func initializeMachine(t *testing.T, labels map[string]string, providerID string, useDefaultCredentialsSecretName bool) *machinev1.Machine {
	machine, stubMachineErr := stubMachine(labels, providerID, useDefaultCredentialsSecretName)

	if stubMachineErr != nil {
		t.Fatalf("Unable to build test machine manifest: %v", stubMachineErr)
		return nil
	}

	return machine
}

func TestCreate(t *testing.T) {
	// TODO add a case of setProviderID and setMachineAnnotationsAndLabels failure
	// TODO add excpect times per
	cases := []struct {
		name                            string
		wantValidateMachineErr          string
		wantCreateVMErr                 string
		ClientCreateVMError             error
		ClientCreateSecretError         error
		labels                          map[string]string
		providerID                      string
		wantVMToBeReady                 bool
		useDefaultCredentialsSecretName bool
	}{
		{
			name:                   "Create a VM",
			wantValidateMachineErr: "",
			wantCreateVMErr:        "",
			ClientCreateVMError:    nil,
			labels:                 nil,
			providerID:             "",
			wantVMToBeReady:        true,
		},
		{
			name:                            "Create a VM with default CredentialsSecretName",
			wantValidateMachineErr:          "",
			wantCreateVMErr:                 "",
			ClientCreateVMError:             nil,
			labels:                          nil,
			providerID:                      "",
			wantVMToBeReady:                 true,
			useDefaultCredentialsSecretName: true,
		},
		{
			name:                   "Create a VM from unlabeled machine and fail",
			wantValidateMachineErr: fmt.Sprintf("%s: failed validating machine provider spec: %v: missing %q label", mahcineName, mahcineName, machinev1.MachineClusterIDLabel),
			wantCreateVMErr:        "",
			ClientCreateVMError:    nil,
			labels:                 map[string]string{machinev1.MachineClusterIDLabel: ""},
			providerID:             "",
			wantVMToBeReady:        true,
		},
		{
			name:                   "Create a VM with an error in the client-go and fail",
			wantValidateMachineErr: "",
			wantCreateVMErr:        "failed to create virtual machine: client error",
			ClientCreateVMError:    errors.New("client error"),
			labels:                 nil,
			providerID:             "",
			wantVMToBeReady:        true,
		},
		{
			name:                    "Create Ignition Secret Error",
			wantCreateVMErr:         "failed to create ignition secret: secret creation error",
			ClientCreateSecretError: errors.New("secret creation error"),
			labels:                  nil,
			providerID:              "",
			wantVMToBeReady:         true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			newMockInfraClusterClient := mockInfraClusterClient.NewMockClient(mockCtrl)
			newMockTenantClusterClient := mockTenantClusterClient.NewMockClient(mockCtrl)
			machine := initializeMachine(t, tc.labels, tc.providerID, tc.useDefaultCredentialsSecretName)
			if machine == nil {
				t.Fatalf("Unable to create the stub machine object")
			}

			infraClusterClientMockBuilder := func(tenantClusterClient tenantcluster.Client, secretName, namespace string) (infracluster.Client, error) {
				return newMockInfraClusterClient, nil
			}

			machineScope, err := stubMachineScope(machine, newMockTenantClusterClient, infraClusterClientMockBuilder)
			if err != nil {
				t.Fatalf("Unable to build virtual machine with error: %v", err)
			}

			ignitionSecret := stubIgnitionSecret(machineScope)
			virtualMachine := stubVirtualMachine(machineScope)
			vmi, _ := stubVmi(virtualMachine)

			returnVM := stubVirtualMachine(machineScope)
			returnSecret := stubIgnitionSecret(machineScope)
			returnVM.Status.Ready = tc.wantVMToBeReady

			// TODO: test negative flow, return err != nil
			newMockInfraClusterClient.EXPECT().CreateSecret(clusterID, ignitionSecret).Return(returnSecret, tc.ClientCreateSecretError).AnyTimes()
			newMockInfraClusterClient.EXPECT().CreateVirtualMachine(clusterID, virtualMachine).Return(returnVM, tc.ClientCreateVMError).AnyTimes()
			newMockInfraClusterClient.EXPECT().GetVirtualMachineInstance(clusterID, virtualMachine.Name, gomock.Any()).Return(vmi, nil).AnyTimes()

			newMockTenantClusterClient.EXPECT().PatchMachine(machine, machine.DeepCopy()).Return(nil).AnyTimes()
			newMockTenantClusterClient.EXPECT().StatusPatchMachine(machine, machine.DeepCopy()).Return(nil).AnyTimes()
			newMockTenantClusterClient.EXPECT().GetSecret(workerUserDataSecretName, machine.Namespace).Return(stubSecret(), nil).AnyTimes()
			newMockTenantClusterClient.EXPECT().GetNamespace().Return(clusterNamespace, nil).AnyTimes()
			newMockTenantClusterClient.EXPECT().GetInfraID().Return(infraID, nil).AnyTimes()

			providerVMInstance := New(infraClusterClientMockBuilder, newMockTenantClusterClient)
			err = providerVMInstance.Create(machine)
			if tc.wantValidateMachineErr != "" {
				assert.Equal(t, tc.wantValidateMachineErr, err.Error())
			} else if tc.wantCreateVMErr != "" {
				assert.Equal(t, tc.wantCreateVMErr, err.Error())
			} else {
				assert.Equal(t, err, nil)
			}
		})
	}

}

func TestDelete(t *testing.T) {
	// TODO add a case of setProviderID and setMachineAnnotationsAndLabels failure
	cases := []struct {
		name                            string
		wantValidateMachineErr          string
		wantGetVMErr                    string
		wantDeleteVMErr                 string
		clientGetVMError                error
		clientDeleteVMError             error
		emptyGetVM                      bool
		labels                          map[string]string
		providerID                      string
		useDefaultCredentialsSecretName bool
	}{
		{
			name:                   "Delete a VM successfully",
			wantValidateMachineErr: "",
			wantGetVMErr:           "",
			clientGetVMError:       nil,
			wantDeleteVMErr:        "",
			clientDeleteVMError:    nil,
			emptyGetVM:             false,
			labels:                 nil,
			providerID:             "",
		},
		{
			name:                            "Delete a VM successfully with default CredentialsSecretName",
			wantValidateMachineErr:          "",
			wantGetVMErr:                    "",
			clientGetVMError:                nil,
			wantDeleteVMErr:                 "",
			clientDeleteVMError:             nil,
			emptyGetVM:                      false,
			labels:                          nil,
			providerID:                      "",
			useDefaultCredentialsSecretName: true,
		},
		{
			name:                   "Delete a VM from unlabeled machine and fail",
			wantValidateMachineErr: fmt.Sprintf("%s: failed validating machine provider spec: %v: missing %q label", mahcineName, mahcineName, machinev1.MachineClusterIDLabel),
			wantGetVMErr:           "",
			clientGetVMError:       nil,
			wantDeleteVMErr:        "",
			clientDeleteVMError:    nil,
			emptyGetVM:             false,
			labels:                 map[string]string{machinev1.MachineClusterIDLabel: ""},
			providerID:             "",
		},
		{
			name:                   "Delete deleting VM with getting error and fail",
			wantValidateMachineErr: "",
			wantGetVMErr:           "client error",
			clientGetVMError:       errors.New("client error"),
			wantDeleteVMErr:        "",
			clientDeleteVMError:    nil,
			emptyGetVM:             false,
			labels:                 nil,
			providerID:             "",
		},
		{
			name:                   "Delete a nonexistent VM and fail",
			wantValidateMachineErr: "",
			wantGetVMErr:           "",
			clientGetVMError:       nil,
			wantDeleteVMErr:        "",
			clientDeleteVMError:    nil,
			emptyGetVM:             true,
			labels:                 nil,
			providerID:             "",
		},
		{
			name:                   "Delete a VM with an error in the client-go and fail",
			wantValidateMachineErr: "",
			wantGetVMErr:           "",
			clientGetVMError:       nil,
			wantDeleteVMErr:        "failed to delete VM: client error",
			clientDeleteVMError:    errors.New("client error"),
			emptyGetVM:             false,
			labels:                 nil,
			providerID:             "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			newMockInfraClusterClient := mockInfraClusterClient.NewMockClient(mockCtrl)
			newMockTenantClusterClient := mockTenantClusterClient.NewMockClient(mockCtrl)

			machine := initializeMachine(t, tc.labels, tc.providerID, tc.useDefaultCredentialsSecretName)
			if machine == nil {
				t.Fatalf("Unable to create the stub machine object")
			}

			infraClusterClientMockBuilder := func(tenantClusterClient tenantcluster.Client, secretName, namespace string) (infracluster.Client, error) {
				return newMockInfraClusterClient, nil
			}

			machineScope, err := stubMachineScope(machine, newMockTenantClusterClient, infraClusterClientMockBuilder)
			if err != nil {
				t.Fatalf("Unable to build virtual machine with error: %v", err)
			}

			virtualMachine := stubVirtualMachine(machineScope)
			vmi, _ := stubVmi(virtualMachine)

			var returnVM *kubevirtapiv1.VirtualMachine
			if !tc.emptyGetVM {
				returnVM = virtualMachine
			}

			//InfraCluster mocks
			newMockInfraClusterClient.EXPECT().GetVirtualMachine(clusterID, virtualMachine.Name, gomock.Any()).Return(returnVM, tc.clientGetVMError).AnyTimes()
			newMockInfraClusterClient.EXPECT().DeleteVirtualMachine(clusterID, virtualMachine.Name, gomock.Any()).Return(tc.clientDeleteVMError).AnyTimes()
			newMockInfraClusterClient.EXPECT().GetVirtualMachineInstance(clusterID, virtualMachine.Name, gomock.Any()).Return(vmi, nil).AnyTimes()

			//TenantCluster mocks
			// TODO: test negative flow, return err != nil
			newMockTenantClusterClient.EXPECT().PatchMachine(machine, machine.DeepCopy()).Return(nil).AnyTimes()
			newMockTenantClusterClient.EXPECT().StatusPatchMachine(machine, machine.DeepCopy()).Return(nil).AnyTimes()
			newMockTenantClusterClient.EXPECT().GetSecret(workerUserDataSecretName, machine.Namespace).Return(stubSecret(), nil).AnyTimes()
			newMockTenantClusterClient.EXPECT().GetNamespace().Return("kubevirt-actuator-cluster", nil).AnyTimes()
			newMockTenantClusterClient.EXPECT().GetInfraID().Return(infraID, nil).AnyTimes()

			providerVMInstance := New(infraClusterClientMockBuilder, newMockTenantClusterClient)
			err = providerVMInstance.Delete(machine)

			// getServicErr
			// deleteServiceErr
			if tc.wantValidateMachineErr != "" {
				assert.Equal(t, tc.wantValidateMachineErr, err.Error())
			} else if tc.wantGetVMErr != "" {
				assert.Equal(t, tc.wantGetVMErr, err.Error())
			} else if tc.wantDeleteVMErr != "" {
				assert.Equal(t, tc.wantDeleteVMErr, err.Error())
			} else {
				assert.Equal(t, err, nil)
			}
		})
	}

}

func TestExists(t *testing.T) {
	// TODO add a case of setProviderID and setMachineAnnotationsAndLabels failure
	cases := []struct {
		name                            string
		clientGetError                  error
		emptyGetVM                      bool
		isExist                         bool
		labels                          map[string]string
		providerID                      string
		useDefaultCredentialsSecretName bool
	}{
		{
			name:           "Validate existence VM",
			clientGetError: nil,
			emptyGetVM:     false,
			isExist:        true,
			labels:         nil,
			providerID:     "",
		},
		{
			name:                            "Validate existence VM with default CredentialsSecretName",
			clientGetError:                  nil,
			emptyGetVM:                      false,
			isExist:                         true,
			labels:                          nil,
			providerID:                      "",
			useDefaultCredentialsSecretName: true,
		},
		{
			name:           "Validate non existence VM",
			clientGetError: nil,
			emptyGetVM:     true,
			isExist:        false,
			labels:         nil,
			providerID:     "",
		},
		{
			name:           "Validate an error in get VM",
			clientGetError: errors.New("client error"),
			emptyGetVM:     true,
			isExist:        false,
			labels:         nil,
			providerID:     "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			newMockInfraClusterClient := mockInfraClusterClient.NewMockClient(mockCtrl)
			newMockTenantClusterClient := mockTenantClusterClient.NewMockClient(mockCtrl)

			machine := initializeMachine(t, tc.labels, tc.providerID, tc.useDefaultCredentialsSecretName)
			if machine == nil {
				t.Fatalf("Unable to create the stub machine object")
			}

			infraClusterClientMockBuilder := func(tenantClusterClient tenantcluster.Client, secretName, namespace string) (infracluster.Client, error) {
				return newMockInfraClusterClient, nil
			}

			machineScope, err := stubMachineScope(machine, newMockTenantClusterClient, infraClusterClientMockBuilder)
			if err != nil {
				t.Fatalf("Unable to build virtual machine with error: %v", err)
			}

			virtualMachine := stubVirtualMachine(machineScope)
			vmi, _ := stubVmi(virtualMachine)

			var returnVM *kubevirtapiv1.VirtualMachine
			if !tc.emptyGetVM {
				returnVM = virtualMachine
			}

			//InfraCluster mocks
			newMockInfraClusterClient.EXPECT().GetVirtualMachine(clusterID, virtualMachine.Name, gomock.Any()).Return(returnVM, tc.clientGetError).AnyTimes()
			newMockInfraClusterClient.EXPECT().GetVirtualMachineInstance(clusterID, virtualMachine.Name, gomock.Any()).Return(vmi, nil).AnyTimes()
			newMockTenantClusterClient.EXPECT().GetSecret(workerUserDataSecretName, machine.Namespace).Return(stubSecret(), nil).AnyTimes()
			newMockTenantClusterClient.EXPECT().GetNamespace().Return("kubevirt-actuator-cluster", nil).AnyTimes()
			newMockTenantClusterClient.EXPECT().GetInfraID().Return(infraID, nil).AnyTimes()

			providerVMInstance := New(infraClusterClientMockBuilder, newMockTenantClusterClient)
			existsVM, err := providerVMInstance.Exists(machine)

			if tc.clientGetError != nil {
				assert.Equal(t, tc.clientGetError.Error(), err.Error())
			} else if tc.emptyGetVM {
				assert.Equal(t, err, nil)
				assert.Equal(t, existsVM, false)
			} else {
				assert.Equal(t, err, nil)
				assert.Equal(t, existsVM, tc.isExist)
			}
		})
	}

}

func TestUpdate(t *testing.T) {
	// TODO add a case of setProviderID and setMachineAnnotationsAndLabels failure
	cases := []struct {
		name                            string
		wantValidateMachineErr          string
		wantUpdateVMErr                 string
		clientGetVMError                error
		clientUpdateVMError             error
		emptyGetVM                      bool
		labels                          map[string]string
		providerID                      string
		wantVMToBeReady                 bool
		useDefaultCredentialsSecretName bool
	}{
		{
			name:                   "Update a VM",
			wantValidateMachineErr: "",
			wantUpdateVMErr:        "",
			clientGetVMError:       nil,
			clientUpdateVMError:    nil,
			emptyGetVM:             false,
			labels:                 nil,
			providerID:             formatProviderID(clusterNamespace, mahcineName),
			wantVMToBeReady:        true,
		},
		{
			name:                            "Update a VM, use default CredentialsSecretName",
			wantValidateMachineErr:          "",
			wantUpdateVMErr:                 "",
			clientGetVMError:                nil,
			clientUpdateVMError:             nil,
			emptyGetVM:                      false,
			labels:                          nil,
			providerID:                      formatProviderID(clusterNamespace, mahcineName),
			wantVMToBeReady:                 true,
			useDefaultCredentialsSecretName: true,
		},
		// TODO: enable that test after pushing the PR: https://github.com/kubevirt/kubevirt/pull/3889 so update wouldn't override the vm Status
		//{
		//	name:                   "Update a VM that never be ready",
		//	wantValidateMachineErr: "",
		//	wantUpdateVMErr:        "",
		//	clientGetVMError:       nil,
		//	clientUpdateVMError:    nil,
		//	emptyGetVM:             false,
		//	labels:                 nil,
		//	providerID:             "",
		//	wantVMToBeReady:        false,
		//},
		{
			name:                   "Update a VM from unlabeled machine and fail",
			wantValidateMachineErr: fmt.Sprintf("%s: failed validating machine provider spec: %v: missing %q label", mahcineName, mahcineName, machinev1.MachineClusterIDLabel),
			wantUpdateVMErr:        "",
			clientGetVMError:       nil,
			clientUpdateVMError:    nil,
			emptyGetVM:             false,
			labels:                 map[string]string{machinev1.MachineClusterIDLabel: ""},
			providerID:             "",
			wantVMToBeReady:        false,
		},
		{
			name:                   "Update a VM with getting error and fail",
			wantValidateMachineErr: "",
			wantUpdateVMErr:        "",
			clientGetVMError:       errors.New("client error"),
			clientUpdateVMError:    nil,
			emptyGetVM:             false,
			labels:                 nil,
			providerID:             "",
			wantVMToBeReady:        false,
		},
		{
			name:                   "Update a nonexistent VM and fail",
			wantValidateMachineErr: "",
			wantUpdateVMErr:        "",
			clientGetVMError:       nil,
			clientUpdateVMError:    nil,
			emptyGetVM:             true,
			labels:                 nil,
			providerID:             "",
			wantVMToBeReady:        false,
		},
		{
			name:                   "Delete a VM with an error in the client-go and fail",
			wantValidateMachineErr: "",
			clientGetVMError:       nil,
			clientUpdateVMError:    errors.New("client error"),
			wantUpdateVMErr:        "failed to update VM: client error",
			emptyGetVM:             false,
			labels:                 nil,
			providerID:             "",
			wantVMToBeReady:        false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			newMockInfraClusterClient := mockInfraClusterClient.NewMockClient(mockCtrl)
			newMockTenantClusterClient := mockTenantClusterClient.NewMockClient(mockCtrl)

			machine := initializeMachine(t, tc.labels, tc.providerID, tc.useDefaultCredentialsSecretName)
			if machine == nil {
				t.Fatalf("Unable to create the stub machine object")
			}

			infraClusterClientMockBuilder := func(tenantClusterClient tenantcluster.Client, secretName, namespace string) (infracluster.Client, error) {
				return newMockInfraClusterClient, nil
			}

			machineScope, err := stubMachineScope(machine, newMockTenantClusterClient, infraClusterClientMockBuilder)
			if err != nil {
				t.Fatalf("Unable to build virtual machine with error: %v", err)
			}

			virtualMachine := stubVirtualMachine(machineScope)
			vmi, _ := stubVmi(virtualMachine)
			var getReturnVM *kubevirtapiv1.VirtualMachine
			if !tc.emptyGetVM {
				returnVMResult := stubVirtualMachine(machineScope)
				getReturnVM = returnVMResult
				getReturnVM.Status = kubevirtapiv1.VirtualMachineStatus{
					Created: true,
					Ready:   tc.wantVMToBeReady,
				}
				getReturnVM.Status.Created = true
				getReturnVM.Status.Ready = tc.wantVMToBeReady

			}

			updateReturnVM := stubVirtualMachine(machineScope)
			updateReturnVM.Status = kubevirtapiv1.VirtualMachineStatus{
				Created: true,
				Ready:   tc.wantVMToBeReady,
			}

			newMockInfraClusterClient.EXPECT().GetVirtualMachine(clusterID, virtualMachine.Name, gomock.Any()).Return(getReturnVM, tc.clientGetVMError).AnyTimes()
			newMockInfraClusterClient.EXPECT().UpdateVirtualMachine(clusterID, getReturnVM).Return(updateReturnVM, tc.clientUpdateVMError).AnyTimes()
			newMockInfraClusterClient.EXPECT().GetVirtualMachineInstance(clusterID, virtualMachine.Name, gomock.Any()).Return(vmi, nil).AnyTimes()

			// TODO: test negative flow, return err != nil
			newMockTenantClusterClient.EXPECT().PatchMachine(machine, machine.DeepCopy()).Return(nil).AnyTimes()
			newMockTenantClusterClient.EXPECT().StatusPatchMachine(machine, machine.DeepCopy()).Return(nil).AnyTimes()
			newMockTenantClusterClient.EXPECT().GetSecret(workerUserDataSecretName, machine.Namespace).Return(stubSecret(), nil).AnyTimes()
			newMockTenantClusterClient.EXPECT().GetNamespace().Return("kubevirt-actuator-cluster", nil).AnyTimes()
			newMockTenantClusterClient.EXPECT().GetInfraID().Return(infraID, nil).AnyTimes()

			providerVMInstance := New(infraClusterClientMockBuilder, newMockTenantClusterClient)
			// TODO: test the bool wasUpdated
			_, err = providerVMInstance.Update(machine)

			if tc.wantValidateMachineErr != "" {
				assert.Equal(t, tc.wantValidateMachineErr, err.Error())
			} else if tc.clientGetVMError != nil {
				assert.Equal(t, tc.clientGetVMError.Error(), err.Error())
			} else if tc.wantUpdateVMErr != "" {
				assert.Equal(t, tc.wantUpdateVMErr, err.Error())
			} else if tc.emptyGetVM {
				assert.Equal(t, err.Error(), "requeue in: 3m0s")
			} else if !tc.wantVMToBeReady {
				assert.Equal(t, err.Error(), "requeue in: 20s")
			} else {
				assert.Equal(t, err, nil)
				assert.Equal(t, *machine.Spec.ProviderID, tc.providerID)
			}
		})
	}

}

// func DefaultVirtualMachine(started bool) (*kubevirtapiv1.VirtualMachine, *kubevirtapiv1.VirtualMachineInstance) {
// 	return DefaultVirtualMachineWithNames(started, "testvmi", "testvmi")
// }

// func DefaultVirtualMachineWithNames(started bool, vmName string, vmiName string) (*kubevirtapiv1.VirtualMachine, *kubevirtapiv1.VirtualMachineInstance) {
// 	vmi := kubevirtapiv1.NewMinimalVMI(vmiName)
// 	vmi.Status.Phase = kubevirtapiv1.Running
// 	vm := VirtualMachineFromVMI(vmName, vmi, started)
// 	t := true
// 	vmi.OwnerReferences = []metav1.OwnerReference{{
// 		APIVersion:         kubevirtapiv1.VirtualMachineGroupVersionKind.GroupVersion().String(),
// 		Kind:               kubevirtapiv1.VirtualMachineGroupVersionKind.Kind,
// 		Name:               vm.ObjectMeta.Name,
// 		UID:                vm.ObjectMeta.UID,
// 		Controller:         &t,
// 		BlockOwnerDeletion: &t,
// 	}}
// 	return vm, vmi
// }

// func VirtualMachineFromVMI(name string, vmi *kubevirtapiv1.VirtualMachineInstance, started bool) *kubevirtapiv1.VirtualMachine {
// 	vm := &kubevirtapiv1.VirtualMachine{
// 		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: vmi.ObjectMeta.Namespace, ResourceVersion: "1"},
// 		Spec: kubevirtapiv1.VirtualMachineSpec{
// 			Running: &started,
// 			Template: &kubevirtapiv1.VirtualMachineInstanceTemplateSpec{
// 				ObjectMeta: metav1.ObjectMeta{
// 					Name:   vmi.ObjectMeta.Name,
// 					Labels: vmi.ObjectMeta.Labels,
// 				},
// 				Spec: vmi.Spec,
// 			},
// 		},
// 	}
// 	return vm
// }
