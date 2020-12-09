package vm

import (
	"fmt"

	apiresource "k8s.io/apimachinery/pkg/api/resource"

	cdiv1 "kubevirt.io/containerized-data-importer/pkg/apis/core/v1alpha1"

	corev1 "k8s.io/api/core/v1"

	kubevirtapiv1 "kubevirt.io/client-go/api/v1"

	machineapierros "github.com/openshift/machine-api-operator/pkg/controller/machine"

	"github.com/openshift/cluster-api-provider-kubevirt/pkg/clients/infracluster"
	"github.com/openshift/cluster-api-provider-kubevirt/pkg/clients/tenantcluster"

	kubevirtproviderv1alpha1 "github.com/openshift/cluster-api-provider-kubevirt/pkg/apis/kubevirtprovider/v1alpha1"
	machinev1 "github.com/openshift/machine-api-operator/pkg/apis/machine/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	defaultNamespace         = "default"
	mahcineName              = "machine-test"
	clusterID                = "kubevirt-actuator-cluster"
	clusterName              = "kubevirt-actuator-cluster"
	userDataValue            = "{\"key1\":\"value1\"}"
	workerUserDataSecretName = "worker-user-data"
	SourceTestPvcName        = "SourceTestPvcName"
	NetworkName              = "multus-network"
)

var (
	userDataValueFull = fmt.Sprintf(
		"{\"key1\":\"value1\",\"storage\":{\"files\":[{\"contents\":{\"source\":\"data:,%s\"},\"filesystem\":\"root\",\"mode\":420,\"path\":\"/etc/hostname\"}]}}",
		mahcineName,
	)
)

func stubVmi(vm *kubevirtapiv1.VirtualMachine) (*kubevirtapiv1.VirtualMachineInstance, error) {
	vmi := kubevirtapiv1.VirtualMachineInstance{
		//TypeMeta:   v12.TypeMeta{},
		//ObjectMeta: v12.ObjectMeta{},
		Spec: kubevirtapiv1.VirtualMachineInstanceSpec{},
		Status: kubevirtapiv1.VirtualMachineInstanceStatus{
			Interfaces: []kubevirtapiv1.VirtualMachineInstanceNetworkInterface{},
		},
	}
	vmi.Name = vm.Name
	vmi.Namespace = vm.Namespace
	vmi.Spec = vm.Spec.Template.Spec

	return &vmi, nil
}

func stubMachineScope(machine *machinev1.Machine, tenantClusterClient tenantcluster.Client, infraClusterClient infracluster.Client) (*machineScope, error) {
	providerSpec, err := kubevirtproviderv1alpha1.ProviderSpecFromRawExtension(machine.Spec.ProviderSpec.Value)
	if err != nil {
		return nil, machineapierros.InvalidMachineConfiguration("failed to get machine config: %v", err)
	}

	providerStatus, err := kubevirtproviderv1alpha1.ProviderStatusFromRawExtension(machine.Status.ProviderStatus)
	if err != nil {
		return nil, machineapierros.InvalidMachineConfiguration("failed to get machine provider status: %v", err.Error())
	}

	return &machineScope{
		infraClusterClient:    infraClusterClient,
		tenantClusterClient:   tenantClusterClient,
		machine:               machine,
		originMachineCopy:     machine.DeepCopy(),
		machineProviderSpec:   providerSpec,
		machineProviderStatus: providerStatus,
	}, nil
}

func stubSecret() *corev1.Secret {
	secret := corev1.Secret{
		Data: map[string][]byte{"userData": []byte(userDataValue)},
	}
	return &secret
}

func stubBuildVMITemplate(s *machineScope) *kubevirtapiv1.VirtualMachineInstanceTemplateSpec {
	virtualMachineName := s.machine.GetName()

	template := &kubevirtapiv1.VirtualMachineInstanceTemplateSpec{}

	template.ObjectMeta = metav1.ObjectMeta{
		Labels: map[string]string{"kubevirt.io/vm": virtualMachineName, "name": virtualMachineName},
	}

	template.Spec = kubevirtapiv1.VirtualMachineInstanceSpec{}
	template.Spec.Volumes = []kubevirtapiv1.Volume{
		{
			Name: buildDataVolumeDiskName(virtualMachineName),
			VolumeSource: kubevirtapiv1.VolumeSource{
				DataVolume: &kubevirtapiv1.DataVolumeSource{
					Name: buildBootVolumeName(virtualMachineName),
				},
			},
		},
		{
			Name: buildCloudInitVolumeDiskName(virtualMachineName),
			VolumeSource: kubevirtapiv1.VolumeSource{
				CloudInitConfigDrive: &kubevirtapiv1.CloudInitConfigDriveSource{
					UserDataSecretRef: &corev1.LocalObjectReference{
						Name: fmt.Sprintf("%s-ignition", virtualMachineName),
					},
				},
			},
		},
	}

	multusNetwork := &kubevirtapiv1.MultusNetwork{
		NetworkName: s.machineProviderSpec.NetworkName,
	}
	template.Spec.Networks = []kubevirtapiv1.Network{
		{
			Name: mainNetworkName,
			NetworkSource: kubevirtapiv1.NetworkSource{
				Multus: multusNetwork,
			},
		},
	}

	template.Spec.Domain = kubevirtapiv1.DomainSpec{}

	requests := corev1.ResourceList{}

	requestedMemory := s.machineProviderSpec.RequestedMemory
	if requestedMemory == "" {
		requestedMemory = defaultRequestedMemory
	}
	requests[corev1.ResourceMemory] = apiresource.MustParse(requestedMemory)

	if s.machineProviderSpec.RequestedCPU != 0 {
		requests[corev1.ResourceCPU] = apiresource.MustParse(fmt.Sprint(s.machineProviderSpec.RequestedCPU))
	}

	template.Spec.Domain.Resources = kubevirtapiv1.ResourceRequirements{
		Requests: requests,
	}
	template.Spec.Domain.Devices = kubevirtapiv1.Devices{
		Disks: []kubevirtapiv1.Disk{
			{
				Name: buildDataVolumeDiskName(virtualMachineName),
				DiskDevice: kubevirtapiv1.DiskDevice{
					Disk: &kubevirtapiv1.DiskTarget{
						Bus: defaultBus,
					},
				},
			},
			{
				Name: buildCloudInitVolumeDiskName(virtualMachineName),
				DiskDevice: kubevirtapiv1.DiskDevice{
					Disk: &kubevirtapiv1.DiskTarget{
						Bus: defaultBus,
					},
				},
			},
		},
		Interfaces: []kubevirtapiv1.Interface{
			{
				Name: mainNetworkName,
				InterfaceBindingMethod: kubevirtapiv1.InterfaceBindingMethod{
					Bridge: &kubevirtapiv1.InterfaceBridge{},
				},
			},
		},
	}

	return template
}

func stubIgnitionSecret(machineScope *machineScope) *corev1.Secret {
	name := fmt.Sprintf("%s-ignition", machineScope.machine.Name)
	namespace := machineScope.machine.Labels[machinev1.MachineClusterIDLabel]
	labels := map[string]string{
		"tenantcluster-test-id-asdfg-machine.openshift.io": "owned",
	}
	resultSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Data: map[string][]byte{
			"userdata": []byte(userDataValueFull),
		},
	}

	return resultSecret
}

func stubVirtualMachine(machineScope *machineScope) *kubevirtapiv1.VirtualMachine {
	runAlways := kubevirtapiv1.RunStrategyAlways
	namespace := machineScope.machine.Labels[machinev1.MachineClusterIDLabel]
	vmiTemplate := stubBuildVMITemplate(machineScope)
	storageClassName := ""
	virtualMachine := kubevirtapiv1.VirtualMachine{
		Spec: kubevirtapiv1.VirtualMachineSpec{
			RunStrategy: &runAlways,
			DataVolumeTemplates: []cdiv1.DataVolume{
				*buildBootVolumeDataVolumeTemplate(machineScope.machine.GetName(), machineScope.machineProviderSpec.SourcePvcName, namespace, storageClassName, defaultRequestedStorage, defaultPersistentVolumeAccessMode),
			},
			Template: vmiTemplate,
		},
	}

	labels := machineScope.machine.Labels
	labels["tenantcluster-test-id-asdfg-machine.openshift.io"] = "owned"

	virtualMachine.APIVersion = APIVersion
	virtualMachine.Kind = Kind
	virtualMachine.ObjectMeta = metav1.ObjectMeta{
		Name:            machineScope.machine.Name,
		Namespace:       namespace,
		Labels:          labels,
		Annotations:     machineScope.machine.Annotations,
		OwnerReferences: nil,
		ClusterName:     machineScope.machine.ClusterName,
	}

	return &virtualMachine
}
func stubMachine(labels map[string]string, providerID string, useDefaultCredentialsSecretName bool) (*machinev1.Machine, error) {
	kubevirtMachineProviderSpec := &kubevirtproviderv1alpha1.KubevirtMachineProviderSpec{
		SourcePvcName:         SourceTestPvcName,
		IgnitionSecretName:    workerUserDataSecretName,
		CredentialsSecretName: workerUserDataSecretName,
		NetworkName:           NetworkName,
	}
	if useDefaultCredentialsSecretName {
		kubevirtMachineProviderSpec.CredentialsSecretName = ""
	}
	providerSpecValue, providerSpecValueErr := kubevirtproviderv1alpha1.RawExtensionFromProviderSpec(kubevirtMachineProviderSpec)

	if labels == nil {
		labels = map[string]string{
			machinev1.MachineClusterIDLabel: clusterID,
		}
	}
	if providerSpecValueErr != nil {
		return nil, fmt.Errorf("codec.EncodeProviderSpec failed: %v", providerSpecValueErr)
	}
	machine := &machinev1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:                       mahcineName,
			Namespace:                  defaultNamespace,
			Generation:                 0,
			CreationTimestamp:          metav1.Time{},
			DeletionTimestamp:          nil,
			DeletionGracePeriodSeconds: nil,
			Labels:                     labels,
			//Annotations:                nil,
			ClusterName: clusterName,
		},
		Spec: machinev1.MachineSpec{
			ObjectMeta:   metav1.ObjectMeta{},
			ProviderSpec: machinev1.ProviderSpec{Value: providerSpecValue},
			ProviderID:   &providerID,
		},
		Status: machinev1.MachineStatus{},
	}

	return machine, nil
}
