package machinescope

import (
	"fmt"
	"testing"
	"time"

	kubevirtproviderv1alpha1 "github.com/openshift/cluster-api-provider-kubevirt/pkg/apis/kubevirtprovider/v1alpha1"
	"github.com/openshift/cluster-api-provider-kubevirt/pkg/testutils"
	machinev1 "github.com/openshift/machine-api-operator/pkg/apis/machine/v1beta1"
	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	apiresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubevirtapiv1 "kubevirt.io/client-go/api/v1"
)

func TestUpdateAllowed(t *testing.T) {
	requeueAfterSeconds := 20

	cases := []struct {
		name           string
		expectedResult bool
		modifyMachine  func(machine *machinev1.Machine) error
	}{
		{
			name:           "allowed LastUpdated empty",
			expectedResult: true,
		},
		{
			name:           "allowed LastUpdated not empty",
			expectedResult: true,
			modifyMachine: func(machine *machinev1.Machine) error {
				now := time.Now()
				duration := time.Duration(-1*(requeueAfterSeconds-1)) * time.Second
				lastUpdated := now.Add(duration)

				machine.Status.LastUpdated = &metav1.Time{
					Time: lastUpdated,
				}
				return nil
			},
		},
		{
			name:           "not allowed ProviderID nil",
			expectedResult: false,
			modifyMachine: func(machine *machinev1.Machine) error {
				machine.Spec.ProviderID = nil
				return nil
			},
		},
		{
			name:           "not allowed ProviderID empty",
			expectedResult: false,
			modifyMachine: func(machine *machinev1.Machine) error {
				emptyProviderID := ""
				machine.Spec.ProviderID = &emptyProviderID
				return nil
			},
		},
		{
			name:           "not allowed time passed since LastUpdated too short",
			expectedResult: false,
			modifyMachine: func(machine *machinev1.Machine) error {
				now := time.Now()
				duration := time.Duration(-1*requeueAfterSeconds) * time.Second
				lastUpdated := now.Add(duration)

				machine.Status.LastUpdated = &metav1.Time{
					Time: lastUpdated,
				}
				return nil
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			machineScope, _ := initializeMachineScope(t, tc.modifyMachine)
			result := machineScope.UpdateAllowed(time.Duration(requeueAfterSeconds))
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}

func initializeMachineScope(t *testing.T, modifyMachine func(machine *machinev1.Machine) error) (MachineScope, *machinev1.Machine) {
	machine, err := testutils.StubMachine()
	if err != nil {
		t.Fatalf("Error durring stubMachine creation: %v", err)
	}
	if modifyMachine != nil {
		if err := modifyMachine(machine); err != nil {
			t.Fatalf("Error durring modify machine: %v", err)
		}
	}
	machineScope, err := New().CreateMachineScope(machine, testutils.InfraNamespace, testutils.InfraID)
	if err != nil {
		t.Fatalf("Error durring machineScope creation: %v", err)
	}
	return machineScope, machine
}

func TestCreateIgnitionSecretFromMachine(t *testing.T) {
	machineScope, _ := initializeMachineScope(t, nil)
	expectedResult := testutils.StubIgnitionSecret()
	result := machineScope.CreateIgnitionSecretFromMachine([]byte(fmt.Sprintf(testutils.FullUserDataFmt, testutils.MachineName)))
	assert.DeepEqual(t, expectedResult, result)
}

func TestSyncMachine(t *testing.T) {
	cases := []struct {
		name                  string
		expectedErr           string
		modifyExpectedMachine func(machine *machinev1.Machine)
		modifyVM              func(vm *kubevirtapiv1.VirtualMachine)
		providerIDExists      bool
	}{
		{
			name: "success status created and ready",
			modifyExpectedMachine: func(machine *machinev1.Machine) {
				machine.Annotations["machine.openshift.io/instance-state"] = "vmWasCreatedAndReady"
			},
			modifyVM: func(vm *kubevirtapiv1.VirtualMachine) {
				vm.Status.Created = true
				vm.Status.Ready = true
			},
		},
		{
			name: "success status created and not ready",
			modifyExpectedMachine: func(machine *machinev1.Machine) {
				machine.Annotations["machine.openshift.io/instance-state"] = "vmWasCreatedButNotReady"
			},
			modifyVM: func(vm *kubevirtapiv1.VirtualMachine) {
				vm.Status.Created = true
				vm.Status.Ready = false
			},
		},
		{
			name: "success status not Created",
			modifyExpectedMachine: func(machine *machinev1.Machine) {
				machine.Annotations["machine.openshift.io/instance-state"] = "vmNotCreated"
			},
			modifyVM: func(vm *kubevirtapiv1.VirtualMachine) {
				vm.Status.Created = false
				vm.Status.Ready = false
			},
		},
		{
			name: "success status created and ready",
			modifyExpectedMachine: func(machine *machinev1.Machine) {
				machine.Annotations["machine.openshift.io/instance-state"] = "vmWasCreatedAndReady"
				delete(machine.Labels, "machine.openshift.io/instance-type")
			},
			modifyVM: func(vm *kubevirtapiv1.VirtualMachine) {
				vm.Status.Created = true
				vm.Status.Ready = true
				vm.Spec.Template = nil
			},
		},
		{
			name: "success providerID exists",
			modifyExpectedMachine: func(machine *machinev1.Machine) {
				machine.Annotations["machine.openshift.io/instance-state"] = "vmWasCreatedAndReady"
			},
			modifyVM: func(vm *kubevirtapiv1.VirtualMachine) {
				vm.Status.Created = true
				vm.Status.Ready = true
			},
			providerIDExists: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			machineScope, machine := initializeMachineScope(t, nil)

			machineType := "test-machine-type"

			vm := testutils.StubVirtualMachine(testutils.StringPointer("test-vm-name"), testutils.StringPointer("test-vm-namespace"), testutils.StringPointer("test-vm-id"))
			vm.Spec.Template = &kubevirtapiv1.VirtualMachineInstanceTemplateSpec{}
			vm.Spec.Template.Spec.Domain.Machine.Type = machineType
			tc.modifyVM(vm)

			providerID := fmt.Sprintf("kubevirt://%s/%s", vm.Namespace, vm.Name)

			if tc.providerIDExists {
				machine.Spec.ProviderID = &providerID
			}

			vmi := testutils.StubVirtualMachineInstance()

			expectedResultMachine := stubExpectedResultMachine(t, vm, vmi, providerID, machineType, tc.modifyExpectedMachine)

			err := machineScope.SyncMachine(*vm, *vmi, providerID)
			if tc.expectedErr != "" {
				assert.Error(t, err, tc.expectedErr)
			} else {
				assert.NilError(t, err)
				assert.DeepEqual(t, machine, expectedResultMachine)
			}
		})
	}
}

func stubExpectedResultMachine(t *testing.T, vm *kubevirtapiv1.VirtualMachine, vmi *kubevirtapiv1.VirtualMachineInstance,
	providerID string, machineType string, modifyExpectedMachine func(machine *machinev1.Machine)) *machinev1.Machine {
	providerStatus, err := kubevirtproviderv1alpha1.RawExtensionFromProviderStatus(&kubevirtproviderv1alpha1.KubevirtMachineProviderStatus{
		VirtualMachineStatus: vm.Status,
	})
	if err != nil {
		t.Fatalf("Error durring providerStatus creation: %v", err)
	}
	expectedResultMachine, err := testutils.StubMachine()
	if err != nil {
		t.Fatalf("Error durring stubMachine creation: %v", err)
	}
	expectedResultMachine.Annotations = map[string]string{"VmId": string(vm.UID)}
	expectedResultMachine.Spec.ProviderID = &providerID
	expectedResultMachine.Labels["machine.openshift.io/instance-type"] = machineType
	expectedResultMachine.Status.ProviderStatus = providerStatus
	expectedResultMachine.Status.Addresses = []corev1.NodeAddress{
		{Address: vmi.Name, Type: corev1.NodeInternalDNS},
		{Type: corev1.NodeInternalIP, Address: "127.0.0.1"},
	}
	modifyExpectedMachine(expectedResultMachine)
	return expectedResultMachine
}

func TestCreateVirtualMachineFromMachine(t *testing.T) {
	cases := []struct {
		name             string
		modifyMachine    func(machine *machinev1.Machine) error
		modifyExpectedVM func(vm *kubevirtapiv1.VirtualMachine)
		expectedErr      string
	}{
		{
			name: "success",
		},
		{
			name: "success default accessMode",
			modifyMachine: func(machine *machinev1.Machine) error {
				modifyProviderSpec := testutils.ProviderSpec
				modifyProviderSpec.PersistentVolumeAccessMode = ""
				val, err := kubevirtproviderv1alpha1.RawExtensionFromProviderSpec(&modifyProviderSpec)
				machine.Spec.ProviderSpec = machinev1.ProviderSpec{Value: val}

				return err
			},
			modifyExpectedVM: func(vm *kubevirtapiv1.VirtualMachine) {
				vm.Spec.DataVolumeTemplates[0].Spec.PVC.AccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany}
			},
		},
		{
			name: "success default storageClass",
			modifyMachine: func(machine *machinev1.Machine) error {
				modifyProviderSpec := testutils.ProviderSpec
				modifyProviderSpec.StorageClassName = ""
				val, err := kubevirtproviderv1alpha1.RawExtensionFromProviderSpec(&modifyProviderSpec)
				machine.Spec.ProviderSpec = machinev1.ProviderSpec{Value: val}

				return err
			},
			modifyExpectedVM: func(vm *kubevirtapiv1.VirtualMachine) {
				vm.Spec.DataVolumeTemplates[0].Spec.PVC.StorageClassName = nil
			},
		},
		{
			name: "success default storage size",
			modifyMachine: func(machine *machinev1.Machine) error {
				modifyProviderSpec := testutils.ProviderSpec
				modifyProviderSpec.RequestedStorage = ""
				val, err := kubevirtproviderv1alpha1.RawExtensionFromProviderSpec(&modifyProviderSpec)
				machine.Spec.ProviderSpec = machinev1.ProviderSpec{Value: val}

				return err
			},
			modifyExpectedVM: func(vm *kubevirtapiv1.VirtualMachine) {
				vm.Spec.DataVolumeTemplates[0].Spec.PVC.Resources.Requests[corev1.ResourceStorage] = apiresource.MustParse("35Gi")
			},
		},
		{
			name: "success default memory",
			modifyMachine: func(machine *machinev1.Machine) error {
				modifyProviderSpec := testutils.ProviderSpec
				modifyProviderSpec.RequestedMemory = ""
				val, err := kubevirtproviderv1alpha1.RawExtensionFromProviderSpec(&modifyProviderSpec)
				machine.Spec.ProviderSpec = machinev1.ProviderSpec{Value: val}

				return err
			},
			modifyExpectedVM: func(vm *kubevirtapiv1.VirtualMachine) {
				vm.Spec.Template.Spec.Domain.Resources.Requests[corev1.ResourceMemory] = apiresource.MustParse("2048M")
			},
		},
		{
			name: "failure source pvc name empty",
			modifyMachine: func(machine *machinev1.Machine) error {
				modifyProviderSpec := testutils.ProviderSpec
				modifyProviderSpec.SourcePvcName = ""
				val, err := kubevirtproviderv1alpha1.RawExtensionFromProviderSpec(&modifyProviderSpec)
				machine.Spec.ProviderSpec = machinev1.ProviderSpec{Value: val}

				return err
			},
			expectedErr: "test-machine-name: missing value for SourcePvcName",
		},
		{
			name: "failure ignition secret name empty",
			modifyMachine: func(machine *machinev1.Machine) error {
				modifyProviderSpec := testutils.ProviderSpec
				modifyProviderSpec.IgnitionSecretName = ""
				val, err := kubevirtproviderv1alpha1.RawExtensionFromProviderSpec(&modifyProviderSpec)
				machine.Spec.ProviderSpec = machinev1.ProviderSpec{Value: val}

				return err
			},
			expectedErr: "test-machine-name: missing value for IgnitionSecretName",
		},
		{
			name: "failure network name empty",
			modifyMachine: func(machine *machinev1.Machine) error {
				modifyProviderSpec := testutils.ProviderSpec
				modifyProviderSpec.NetworkName = ""
				val, err := kubevirtproviderv1alpha1.RawExtensionFromProviderSpec(&modifyProviderSpec)
				machine.Spec.ProviderSpec = machinev1.ProviderSpec{Value: val}

				return err
			},
			expectedErr: "test-machine-name: missing value for NetworkName",
		},
		{
			name: "failure access mode not valid",
			modifyMachine: func(machine *machinev1.Machine) error {
				modifyProviderSpec := testutils.ProviderSpec
				modifyProviderSpec.PersistentVolumeAccessMode = "NotValid"
				val, err := kubevirtproviderv1alpha1.RawExtensionFromProviderSpec(&modifyProviderSpec)
				machine.Spec.ProviderSpec = machinev1.ProviderSpec{Value: val}

				return err
			},
			expectedErr: "test-machine-name: Value of PersistentVolumeAccessMode, can be only one of: ReadWriteMany, ReadOnlyMany, ReadWriteOnce",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			machineScope, _ := initializeMachineScope(t, tc.modifyMachine)
			expectedVM := testutils.StubVirtualMachine(nil, nil, nil)
			if tc.modifyExpectedVM != nil {
				tc.modifyExpectedVM(expectedVM)
			}

			result, err := machineScope.CreateVirtualMachineFromMachine()
			if tc.expectedErr != "" {
				assert.Error(t, err, tc.expectedErr)
			} else {
				assert.NilError(t, err)
				assert.DeepEqual(t, result, expectedVM)
			}
		})
	}
}

func TestGetMachine(t *testing.T) {
	machine, err := testutils.StubMachine()
	if err != nil {
		t.Fatalf("Error durring stubMachine creation: %v", err)
	}
	machineScope, err := New().CreateMachineScope(machine, testutils.InfraNamespace, testutils.InfraID)
	if err != nil {
		t.Fatalf("Error durring machineScope creation: %v", err)
	}
	result := machineScope.GetMachine()
	assert.Equal(t, machine, result)
}

func TestGetMachineName(t *testing.T) {
	machine, err := testutils.StubMachine()
	if err != nil {
		t.Fatalf("Error durring stubMachine creation: %v", err)
	}
	machineScope, err := New().CreateMachineScope(machine, testutils.InfraNamespace, testutils.InfraID)
	if err != nil {
		t.Fatalf("Error durring machineScope creation: %v", err)
	}
	result := machineScope.GetMachineName()
	assert.Equal(t, machine.GetName(), result)
}

func TestGetMachineNamespace(t *testing.T) {
	machine, err := testutils.StubMachine()
	if err != nil {
		t.Fatalf("Error durring stubMachine creation: %v", err)
	}
	machineScope, err := New().CreateMachineScope(machine, testutils.InfraNamespace, testutils.InfraID)
	if err != nil {
		t.Fatalf("Error durring machineScope creation: %v", err)
	}
	result := machineScope.GetMachineNamespace()
	assert.Equal(t, machine.GetNamespace(), result)
}

func TestGetInfraNamespace(t *testing.T) {
	machine, err := testutils.StubMachine()
	if err != nil {
		t.Fatalf("Error durring stubMachine creation: %v", err)
	}
	machineScope, err := New().CreateMachineScope(machine, testutils.InfraNamespace, testutils.InfraID)
	if err != nil {
		t.Fatalf("Error durring machineScope creation: %v", err)
	}
	result := machineScope.GetInfraNamespace()
	assert.Equal(t, testutils.InfraNamespace, result)
}

func TestGetIgnitionSecretName(t *testing.T) {
	machine, err := testutils.StubMachine()
	if err != nil {
		t.Fatalf("Error durring stubMachine creation: %v", err)
	}
	machineScope, err := New().CreateMachineScope(machine, testutils.InfraNamespace, testutils.InfraID)
	if err != nil {
		t.Fatalf("Error durring machineScope creation: %v", err)
	}
	result := machineScope.GetIgnitionSecretName()
	assert.Equal(t, testutils.IgnitionSecretName, result)
}
