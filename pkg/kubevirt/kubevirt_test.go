package kubevirt

import (
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	mockInfraClusterClient "github.com/openshift/cluster-api-provider-kubevirt/pkg/clients/infracluster/mock"
	mockMachineScope "github.com/openshift/cluster-api-provider-kubevirt/pkg/machinescope/mock"
	"github.com/openshift/cluster-api-provider-kubevirt/pkg/testutils"
	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	apierr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kubevirtapiv1 "kubevirt.io/client-go/api/v1"
)

const (
	clusterNamespace = "kubevirt-actuator-cluster"
	infraID          = "test-id-asdfg"
	providerIDFmt    = "kubevirt://%s/%s"
)

func TestCreate(t *testing.T) {
	cases := []struct {
		name        string
		expectedErr string
		expect      func(mockInfraClusterClient *mockInfraClusterClient.MockClient, mockMachineScope *mockMachineScope.MockMachineScope)
	}{
		{
			name: "Success",
			expect: func(mockInfraClusterClient *mockInfraClusterClient.MockClient, mockMachineScope *mockMachineScope.MockMachineScope) {
				vm := testutils.StubVirtualMachine(nil, nil, nil)
				vmi := testutils.StubVirtualMachineInstance()
				ignitionSecret := testutils.StubIgnitionSecret()

				mockMachineScope.EXPECT().GetMachineName().Return(testutils.MachineName).Times(1)
				mockMachineScope.EXPECT().CreateIgnitionSecretFromMachine([]byte(fmt.Sprintf(testutils.FullUserDataFmt, testutils.MachineName))).Return(ignitionSecret).Times(1)
				mockInfraClusterClient.EXPECT().CreateSecret(gomock.Any(), testutils.InfraNamespace, ignitionSecret).Return(&corev1.Secret{}, nil).Times(1)
				mockMachineScope.EXPECT().CreateVirtualMachineFromMachine().Return(vm, nil).Times(1)
				mockInfraClusterClient.EXPECT().CreateVirtualMachine(gomock.Any(), testutils.InfraNamespace, vm).Return(vm, nil).Times(1)
				mockInfraClusterClient.EXPECT().GetVirtualMachineInstance(gomock.Any(), testutils.InfraNamespace, testutils.MachineName, gomock.Any()).Return(vmi, nil).Times(1)
				mockMachineScope.EXPECT().SyncMachine(*vm, *vmi, fmt.Sprintf(providerIDFmt, testutils.InfraNamespace, testutils.MachineName)).Return(nil).Times(1)
			},
		},
		{
			name: "Failure create ignition secret",
			expect: func(mockInfraClusterClient *mockInfraClusterClient.MockClient, mockMachineScope *mockMachineScope.MockMachineScope) {
				ignitionSecret := testutils.StubIgnitionSecret()

				mockMachineScope.EXPECT().GetMachineName().Return(testutils.MachineName).Times(1)
				mockMachineScope.EXPECT().CreateIgnitionSecretFromMachine([]byte(fmt.Sprintf(testutils.FullUserDataFmt, testutils.MachineName))).Return(ignitionSecret).Times(1)
				mockInfraClusterClient.EXPECT().CreateSecret(gomock.Any(), testutils.InfraNamespace, ignitionSecret).Return(&corev1.Secret{}, fmt.Errorf("test error")).Times(1)
			},
			expectedErr: "test-machine-name: Error during Create: failed to create ignition secret in infraCluster, with error: test error",
		},
		{
			name: "Failure build virtual machine struct",
			expect: func(mockInfraClusterClient *mockInfraClusterClient.MockClient, mockMachineScope *mockMachineScope.MockMachineScope) {
				vm := testutils.StubVirtualMachine(nil, nil, nil)
				ignitionSecret := testutils.StubIgnitionSecret()

				mockMachineScope.EXPECT().GetMachineName().Return(testutils.MachineName).Times(1)
				mockMachineScope.EXPECT().CreateIgnitionSecretFromMachine([]byte(fmt.Sprintf(testutils.FullUserDataFmt, testutils.MachineName))).Return(ignitionSecret).Times(1)
				mockInfraClusterClient.EXPECT().CreateSecret(gomock.Any(), testutils.InfraNamespace, ignitionSecret).Return(&corev1.Secret{}, nil).Times(1)
				mockMachineScope.EXPECT().CreateVirtualMachineFromMachine().Return(vm, fmt.Errorf("test error")).Times(1)
			},
			expectedErr: "test-machine-name: Error during Create: failed to build Virtual Machine struct, with error: test error",
		},
		{
			name: "Failure create virtual machine",
			expect: func(mockInfraClusterClient *mockInfraClusterClient.MockClient, mockMachineScope *mockMachineScope.MockMachineScope) {
				vm := testutils.StubVirtualMachine(nil, nil, nil)
				ignitionSecret := testutils.StubIgnitionSecret()

				mockMachineScope.EXPECT().GetMachineName().Return(testutils.MachineName).Times(1)
				mockMachineScope.EXPECT().CreateIgnitionSecretFromMachine([]byte(fmt.Sprintf(testutils.FullUserDataFmt, testutils.MachineName))).Return(ignitionSecret).Times(1)
				mockInfraClusterClient.EXPECT().CreateSecret(gomock.Any(), testutils.InfraNamespace, ignitionSecret).Return(&corev1.Secret{}, nil).Times(1)
				mockMachineScope.EXPECT().CreateVirtualMachineFromMachine().Return(vm, nil).Times(1)
				mockInfraClusterClient.EXPECT().CreateVirtualMachine(gomock.Any(), testutils.InfraNamespace, vm).Return(vm, fmt.Errorf("test error")).Times(1)
			},
			expectedErr: "test-machine-name: Error during Create: failed to create Virtual Machine in infraCluster, with error: test error",
		},
		{
			name: "Failure get virtual machine instance",
			expect: func(mockInfraClusterClient *mockInfraClusterClient.MockClient, mockMachineScope *mockMachineScope.MockMachineScope) {
				vm := testutils.StubVirtualMachine(nil, nil, nil)
				vmi := testutils.StubVirtualMachineInstance()
				ignitionSecret := testutils.StubIgnitionSecret()

				mockMachineScope.EXPECT().GetMachineName().Return(testutils.MachineName).Times(1)
				mockMachineScope.EXPECT().CreateIgnitionSecretFromMachine([]byte(fmt.Sprintf(testutils.FullUserDataFmt, testutils.MachineName))).Return(ignitionSecret).Times(1)
				mockInfraClusterClient.EXPECT().CreateSecret(gomock.Any(), testutils.InfraNamespace, ignitionSecret).Return(&corev1.Secret{}, nil).Times(1)
				mockMachineScope.EXPECT().CreateVirtualMachineFromMachine().Return(vm, nil).Times(1)
				mockInfraClusterClient.EXPECT().CreateVirtualMachine(gomock.Any(), testutils.InfraNamespace, vm).Return(vm, nil).Times(1)
				mockInfraClusterClient.EXPECT().GetVirtualMachineInstance(gomock.Any(), testutils.InfraNamespace, testutils.MachineName, gomock.Any()).Return(vmi, fmt.Errorf("test error")).Times(1)
			},
			expectedErr: "test-machine-name: Error during Create: failed to get vmi of the Machine, with error: test error",
		},
		{
			name: "Failure sync machine",
			expect: func(mockInfraClusterClient *mockInfraClusterClient.MockClient, mockMachineScope *mockMachineScope.MockMachineScope) {
				vm := testutils.StubVirtualMachine(nil, nil, nil)
				vmi := testutils.StubVirtualMachineInstance()
				ignitionSecret := testutils.StubIgnitionSecret()

				mockMachineScope.EXPECT().GetMachineName().Return(testutils.MachineName).Times(1)
				mockMachineScope.EXPECT().CreateIgnitionSecretFromMachine([]byte(fmt.Sprintf(testutils.FullUserDataFmt, testutils.MachineName))).Return(ignitionSecret).Times(1)
				mockInfraClusterClient.EXPECT().CreateSecret(gomock.Any(), testutils.InfraNamespace, ignitionSecret).Return(&corev1.Secret{}, nil).Times(1)
				mockMachineScope.EXPECT().CreateVirtualMachineFromMachine().Return(vm, nil).Times(1)
				mockInfraClusterClient.EXPECT().CreateVirtualMachine(gomock.Any(), testutils.InfraNamespace, vm).Return(vm, nil).Times(1)
				mockInfraClusterClient.EXPECT().GetVirtualMachineInstance(gomock.Any(), testutils.InfraNamespace, testutils.MachineName, gomock.Any()).Return(vmi, nil).Times(1)
				mockMachineScope.EXPECT().SyncMachine(*vm, *vmi, fmt.Sprintf(providerIDFmt, testutils.InfraNamespace, testutils.MachineName)).Return(fmt.Errorf("test error")).Times(1)
			},
			expectedErr: "test-machine-name: Error during Create: failed to sync the Machine, with error: test error",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockInfraClusterClient := mockInfraClusterClient.NewMockClient(mockCtrl)
			mockMachineScope := mockMachineScope.NewMockMachineScope(mockCtrl)

			tc.expect(mockInfraClusterClient, mockMachineScope)

			kubevirtVM := New(mockInfraClusterClient)
			err := kubevirtVM.Create(mockMachineScope, []byte(testutils.SrcUserData))
			if tc.expectedErr != "" {
				assert.Error(t, err, tc.expectedErr)
			} else {
				assert.NilError(t, err)
			}
		})
	}
}

func TestDelete(t *testing.T) {
	cases := []struct {
		name        string
		expectedErr string
		expect      func(mockInfraClusterClient *mockInfraClusterClient.MockClient, mockMachineScope *mockMachineScope.MockMachineScope)
	}{
		{
			name: "Success",
			expect: func(mockInfraClusterClient *mockInfraClusterClient.MockClient, mockMachineScope *mockMachineScope.MockMachineScope) {
				vm := testutils.StubVirtualMachine(nil, nil, nil)

				mockMachineScope.EXPECT().GetMachineName().Return(testutils.MachineName).Times(1)
				mockMachineScope.EXPECT().CreateVirtualMachineFromMachine().Return(vm, nil).Times(1)
				mockInfraClusterClient.EXPECT().GetVirtualMachine(gomock.Any(), testutils.InfraNamespace, testutils.MachineName, gomock.Any()).Return(vm, nil).Times(1)
				mockInfraClusterClient.EXPECT().DeleteVirtualMachine(gomock.Any(), testutils.InfraNamespace, testutils.MachineName, gomock.Any()).Return(nil).Times(1)
			},
		},
		{
			name: "Success virtual machine not found",
			expect: func(mockInfraClusterClient *mockInfraClusterClient.MockClient, mockMachineScope *mockMachineScope.MockMachineScope) {
				vm := testutils.StubVirtualMachine(nil, nil, nil)

				notFoundErr := apierr.NewNotFound(schema.GroupResource{Group: "", Resource: "test"}, "3")

				mockMachineScope.EXPECT().GetMachineName().Return(testutils.MachineName).Times(1)
				mockMachineScope.EXPECT().CreateVirtualMachineFromMachine().Return(vm, nil).Times(1)
				mockInfraClusterClient.EXPECT().GetVirtualMachine(gomock.Any(), testutils.InfraNamespace, testutils.MachineName, gomock.Any()).Return(nil, notFoundErr).Times(1)
			},
		},
		{
			name: "Failure create virtual machine struct",
			expect: func(mockInfraClusterClient *mockInfraClusterClient.MockClient, mockMachineScope *mockMachineScope.MockMachineScope) {
				vm := testutils.StubVirtualMachine(nil, nil, nil)

				mockMachineScope.EXPECT().GetMachineName().Return(testutils.MachineName).Times(1)
				mockMachineScope.EXPECT().CreateVirtualMachineFromMachine().Return(vm, fmt.Errorf("test error")).Times(1)
			},
			expectedErr: "test-machine-name: Error during Delete: failed to build Virtual Machine struct, with error: test error",
		},
		{
			name: "Failure get virtual machine",
			expect: func(mockInfraClusterClient *mockInfraClusterClient.MockClient, mockMachineScope *mockMachineScope.MockMachineScope) {
				vm := testutils.StubVirtualMachine(nil, nil, nil)

				mockMachineScope.EXPECT().GetMachineName().Return(testutils.MachineName).Times(1)
				mockMachineScope.EXPECT().CreateVirtualMachineFromMachine().Return(vm, nil).Times(1)
				mockInfraClusterClient.EXPECT().GetVirtualMachine(gomock.Any(), testutils.InfraNamespace, testutils.MachineName, gomock.Any()).Return(nil, fmt.Errorf("test error")).Times(1)
			},
			expectedErr: "test-machine-name: Error during Delete: failed to get Virtual Machine from infraCluster, with error: test error",
		},
		{
			name: "Failure delete virtual machine",
			expect: func(mockInfraClusterClient *mockInfraClusterClient.MockClient, mockMachineScope *mockMachineScope.MockMachineScope) {
				vm := testutils.StubVirtualMachine(nil, nil, nil)

				mockMachineScope.EXPECT().GetMachineName().Return(testutils.MachineName).Times(1)
				mockMachineScope.EXPECT().CreateVirtualMachineFromMachine().Return(vm, nil).Times(1)
				mockInfraClusterClient.EXPECT().GetVirtualMachine(gomock.Any(), testutils.InfraNamespace, testutils.MachineName, gomock.Any()).Return(vm, nil).Times(1)
				mockInfraClusterClient.EXPECT().DeleteVirtualMachine(gomock.Any(), testutils.InfraNamespace, testutils.MachineName, gomock.Any()).Return(fmt.Errorf("test error")).Times(1)
			},
			expectedErr: "test-machine-name: Error during Delete: failed to delete Virtual Machine in infraCluster, with error: test error",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockInfraClusterClient := mockInfraClusterClient.NewMockClient(mockCtrl)
			mockMachineScope := mockMachineScope.NewMockMachineScope(mockCtrl)

			tc.expect(mockInfraClusterClient, mockMachineScope)

			kubevirtVM := New(mockInfraClusterClient)
			err := kubevirtVM.Delete(mockMachineScope)
			if tc.expectedErr != "" {
				assert.Error(t, err, tc.expectedErr)
			} else {
				assert.NilError(t, err)
			}
		})
	}
}

func TestExists(t *testing.T) {
	cases := []struct {
		name           string
		expectedErr    string
		expectedResult bool
		expect         func(mockInfraClusterClient *mockInfraClusterClient.MockClient)
	}{
		{
			name: "Success virtual machine exists",
			expect: func(mockInfraClusterClient *mockInfraClusterClient.MockClient) {
				vm := testutils.StubVirtualMachine(nil, nil, nil)

				mockInfraClusterClient.EXPECT().GetVirtualMachine(gomock.Any(), testutils.InfraNamespace, testutils.MachineName, gomock.Any()).Return(vm, nil).Times(1)
			},
			expectedResult: true,
		},
		{
			name: "Success virtual machine doesn't exist",
			expect: func(mockInfraClusterClient *mockInfraClusterClient.MockClient) {
				notFoundErr := apierr.NewNotFound(schema.GroupResource{Group: "", Resource: "test"}, "3")

				mockInfraClusterClient.EXPECT().GetVirtualMachine(gomock.Any(), testutils.InfraNamespace, testutils.MachineName, gomock.Any()).Return(nil, notFoundErr).Times(1)
			},
			expectedResult: false,
		},
		{
			name: "Failure get virtual machine",
			expect: func(mockInfraClusterClient *mockInfraClusterClient.MockClient) {
				mockInfraClusterClient.EXPECT().GetVirtualMachine(gomock.Any(), testutils.InfraNamespace, testutils.MachineName, gomock.Any()).Return(nil, fmt.Errorf("test error")).Times(1)
			},
			expectedErr: "test-machine-name: Error during Exists: failed to get vm of the Machine, with error: test error",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockInfraClusterClient := mockInfraClusterClient.NewMockClient(mockCtrl)

			tc.expect(mockInfraClusterClient)

			kubevirtVM := New(mockInfraClusterClient)
			result, err := kubevirtVM.Exists(testutils.MachineName, testutils.InfraNamespace)
			if tc.expectedErr != "" {
				assert.Error(t, err, tc.expectedErr)
			} else {
				assert.NilError(t, err)
				assert.Equal(t, result, tc.expectedResult)
			}
		})
	}
}

type vmsForUpdate struct {
	createdVM  *kubevirtapiv1.VirtualMachine
	existingVM *kubevirtapiv1.VirtualMachine
	updateVM   *kubevirtapiv1.VirtualMachine
	resultVM   *kubevirtapiv1.VirtualMachine
}

func TestUpdate(t *testing.T) {
	cases := []struct {
		name           string
		expectedErr    string
		expectedResult bool
		expect         func(mockInfraClusterClient *mockInfraClusterClient.MockClient, mockMachineScope *mockMachineScope.MockMachineScope, vms vmsForUpdate)
	}{
		{
			name: "Success",
			expect: func(mockInfraClusterClient *mockInfraClusterClient.MockClient, mockMachineScope *mockMachineScope.MockMachineScope, vms vmsForUpdate) {
				vmi := testutils.StubVirtualMachineInstance()

				mockMachineScope.EXPECT().GetMachineName().Return(testutils.MachineName).Times(1)
				mockMachineScope.EXPECT().CreateVirtualMachineFromMachine().Return(vms.createdVM, nil).Times(1)
				mockInfraClusterClient.EXPECT().GetVirtualMachine(gomock.Any(), testutils.InfraNamespace, testutils.MachineName, gomock.Any()).Return(vms.existingVM, nil).Times(1)
				mockInfraClusterClient.EXPECT().UpdateVirtualMachine(gomock.Any(), testutils.InfraNamespace, vms.updateVM).Return(vms.resultVM, nil).Times(1)
				mockInfraClusterClient.EXPECT().GetVirtualMachineInstance(gomock.Any(), testutils.InfraNamespace, testutils.MachineName, gomock.Any()).Return(vmi, nil).Times(1)
				mockMachineScope.EXPECT().SyncMachine(*vms.resultVM, *vmi, fmt.Sprintf(providerIDFmt, testutils.InfraNamespace, testutils.MachineName)).Return(nil).Times(1)
			},
			expectedResult: true,
		},
		{
			name: "Success wasn't update",
			expect: func(mockInfraClusterClient *mockInfraClusterClient.MockClient, mockMachineScope *mockMachineScope.MockMachineScope, vms vmsForUpdate) {
				vmi := testutils.StubVirtualMachineInstance()

				vms.resultVM.ResourceVersion = "1234"

				mockMachineScope.EXPECT().GetMachineName().Return(testutils.MachineName).Times(1)
				mockMachineScope.EXPECT().CreateVirtualMachineFromMachine().Return(vms.createdVM, nil).Times(1)
				mockInfraClusterClient.EXPECT().GetVirtualMachine(gomock.Any(), testutils.InfraNamespace, testutils.MachineName, gomock.Any()).Return(vms.existingVM, nil).Times(1)
				mockInfraClusterClient.EXPECT().UpdateVirtualMachine(gomock.Any(), testutils.InfraNamespace, vms.updateVM).Return(vms.resultVM, nil).Times(1)
				mockInfraClusterClient.EXPECT().GetVirtualMachineInstance(gomock.Any(), testutils.InfraNamespace, testutils.MachineName, gomock.Any()).Return(vmi, nil).Times(1)
				mockMachineScope.EXPECT().SyncMachine(*vms.resultVM, *vmi, fmt.Sprintf(providerIDFmt, testutils.InfraNamespace, testutils.MachineName)).Return(nil).Times(1)
			},
			expectedResult: false,
		},
		{
			name: "Failure build virtual machine struct",
			expect: func(mockInfraClusterClient *mockInfraClusterClient.MockClient, mockMachineScope *mockMachineScope.MockMachineScope, vms vmsForUpdate) {
				mockMachineScope.EXPECT().GetMachineName().Return(testutils.MachineName).Times(1)
				mockMachineScope.EXPECT().CreateVirtualMachineFromMachine().Return(nil, fmt.Errorf("test error")).Times(1)
			},
			expectedErr: "test-machine-name: Error during Update: failed to build Virtual Machine struct, with error: test error",
		},
		{
			name: "Failure get virtual machine",
			expect: func(mockInfraClusterClient *mockInfraClusterClient.MockClient, mockMachineScope *mockMachineScope.MockMachineScope, vms vmsForUpdate) {
				mockMachineScope.EXPECT().GetMachineName().Return(testutils.MachineName).Times(1)
				mockMachineScope.EXPECT().CreateVirtualMachineFromMachine().Return(vms.createdVM, nil).Times(1)
				mockInfraClusterClient.EXPECT().GetVirtualMachine(gomock.Any(), testutils.InfraNamespace, testutils.MachineName, gomock.Any()).Return(nil, fmt.Errorf("test error")).Times(1)
			},
			expectedErr: "test-machine-name: Error during Update: failed to get Virtual Machine from infraCluster, with error: test error",
		},
		{
			name: "Failure update virtual machine",
			expect: func(mockInfraClusterClient *mockInfraClusterClient.MockClient, mockMachineScope *mockMachineScope.MockMachineScope, vms vmsForUpdate) {
				mockMachineScope.EXPECT().GetMachineName().Return(testutils.MachineName).Times(1)
				mockMachineScope.EXPECT().CreateVirtualMachineFromMachine().Return(vms.createdVM, nil).Times(1)
				mockInfraClusterClient.EXPECT().GetVirtualMachine(gomock.Any(), testutils.InfraNamespace, testutils.MachineName, gomock.Any()).Return(vms.existingVM, nil).Times(1)
				mockInfraClusterClient.EXPECT().UpdateVirtualMachine(gomock.Any(), testutils.InfraNamespace, vms.updateVM).Return(nil, fmt.Errorf("test error")).Times(1)
			},
			expectedErr: "test-machine-name: Error during Update: failed to update Virtual Machine in infraCluster, with error: test error",
		},
		{
			name: "Failure get virtual machine instance",
			expect: func(mockInfraClusterClient *mockInfraClusterClient.MockClient, mockMachineScope *mockMachineScope.MockMachineScope, vms vmsForUpdate) {
				mockMachineScope.EXPECT().GetMachineName().Return(testutils.MachineName).Times(1)
				mockMachineScope.EXPECT().CreateVirtualMachineFromMachine().Return(vms.createdVM, nil).Times(1)
				mockInfraClusterClient.EXPECT().GetVirtualMachine(gomock.Any(), testutils.InfraNamespace, testutils.MachineName, gomock.Any()).Return(vms.existingVM, nil).Times(1)
				mockInfraClusterClient.EXPECT().UpdateVirtualMachine(gomock.Any(), testutils.InfraNamespace, vms.updateVM).Return(vms.resultVM, nil).Times(1)
				mockInfraClusterClient.EXPECT().GetVirtualMachineInstance(gomock.Any(), testutils.InfraNamespace, testutils.MachineName, gomock.Any()).Return(nil, fmt.Errorf("test error")).Times(1)
			},
			expectedErr: "test-machine-name: Error during Update: failed to get vmi of the Machine, with error: test error",
		},
		{
			name: "Failure sync virtual machine",
			expect: func(mockInfraClusterClient *mockInfraClusterClient.MockClient, mockMachineScope *mockMachineScope.MockMachineScope, vms vmsForUpdate) {
				vmi := testutils.StubVirtualMachineInstance()

				mockMachineScope.EXPECT().GetMachineName().Return(testutils.MachineName).Times(1)
				mockMachineScope.EXPECT().CreateVirtualMachineFromMachine().Return(vms.createdVM, nil).Times(1)
				mockInfraClusterClient.EXPECT().GetVirtualMachine(gomock.Any(), testutils.InfraNamespace, testutils.MachineName, gomock.Any()).Return(vms.existingVM, nil).Times(1)
				mockInfraClusterClient.EXPECT().UpdateVirtualMachine(gomock.Any(), testutils.InfraNamespace, vms.updateVM).Return(vms.resultVM, nil).Times(1)
				mockInfraClusterClient.EXPECT().GetVirtualMachineInstance(gomock.Any(), testutils.InfraNamespace, testutils.MachineName, gomock.Any()).Return(vmi, nil).Times(1)
				mockMachineScope.EXPECT().SyncMachine(*vms.resultVM, *vmi, fmt.Sprintf(providerIDFmt, testutils.InfraNamespace, testutils.MachineName)).Return(fmt.Errorf("test error")).Times(1)
			},
			expectedErr: "test-machine-name: Error during Update: failed to sync the Machine, with error: test error",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockInfraClusterClient := mockInfraClusterClient.NewMockClient(mockCtrl)
			mockMachineScope := mockMachineScope.NewMockMachineScope(mockCtrl)

			vms := vmsForUpdate{
				createdVM:  testutils.StubVirtualMachine(nil, nil, nil),
				existingVM: testutils.StubVirtualMachine(nil, nil, nil),
				updateVM:   testutils.StubVirtualMachine(nil, nil, nil),
				resultVM:   testutils.StubVirtualMachine(nil, nil, nil),
			}
			vms.existingVM.ObjectMeta.ResourceVersion = "1234"
			vms.existingVM.Status.Ready = true
			vms.existingVM.Status.Created = true
			vms.updateVM.ObjectMeta.ResourceVersion = "1234"
			vms.resultVM.ObjectMeta.ResourceVersion = "12345"

			tc.expect(mockInfraClusterClient, mockMachineScope, vms)

			kubevirtVM := New(mockInfraClusterClient)
			result, err := kubevirtVM.Update(mockMachineScope)
			if tc.expectedErr != "" {
				assert.Error(t, err, tc.expectedErr)
			} else {
				assert.NilError(t, err)
				assert.Equal(t, result, tc.expectedResult)
			}
		})
	}
}

func TestAddHostNameToUserData(t *testing.T) {
	result, _ := addHostnameToUserData([]byte(testutils.SrcUserData), testutils.MachineName)
	assert.Equal(t, string(result), fmt.Sprintf(testutils.FullUserDataFmt, testutils.MachineName))

}
