package testutils

import (
	"fmt"

	kubevirtproviderv1alpha1 "github.com/openshift/cluster-api-provider-kubevirt/pkg/apis/kubevirtprovider/v1alpha1"
	machinev1 "github.com/openshift/machine-api-operator/pkg/apis/machine/v1beta1"
	corev1 "k8s.io/api/core/v1"
	apiresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	kubevirtapiv1 "kubevirt.io/client-go/api/v1"
	cdiv1 "kubevirt.io/containerized-data-importer/pkg/apis/core/v1alpha1"
)

const (
	MachineName        = "test-machine-name"
	InfraID            = "test-infra-id"
	InfraNamespace     = "test-infra-namespace"
	IgnitionSecretName = "test-ignition-secret-name"
	SrcUserData        = "{\"ignition\":{\"config\":{\"merge\":[{\"source\":\"https://192.168.123.15:22623/config/worker\"}]},\"security\":{\"tls\":{\"certificateAuthorities\":[{\"source\":\"data:text/plain;charset=utf-8;base64,LS0tLCg==\"}]}},\"version\":\"3.1.0\"}}"
	FullUserDataFmt    = "{\"ignition\":{\"config\":{\"merge\":[{\"source\":\"https://192.168.123.15:22623/config/worker\"}]},\"security\":{\"tls\":{\"certificateAuthorities\":[{\"source\":\"data:text/plain;charset=utf-8;base64,LS0tLCg==\"}]}},\"version\":\"3.1.0\"},\"storage\":{\"files\":[{\"contents\":{\"source\":\"data:,%s\"},\"filesystem\":\"root\",\"mode\":420,\"path\":\"/etc/hostname\"}]}}"

	machineNamespace = "test-machine-namespace"
	clusterID        = "test-cluster-id"
	clusterName      = "test-cluster-name"
	sourcePvcName    = "test-source-pvc-name"
	networkName      = "test-network-name"
	accessMode       = corev1.ReadWriteOnce
	memory           = "123456M"
	storage          = "666Gi"
	cpu              = 77
	storageClassName = "test-storage-class"
)

var (
	labels = map[string]string{
		machinev1.MachineClusterIDLabel: clusterID,
	}

	ProviderSpec = kubevirtproviderv1alpha1.KubevirtMachineProviderSpec{
		SourcePvcName:              sourcePvcName,
		IgnitionSecretName:         IgnitionSecretName,
		CredentialsSecretName:      "test-credentials-secret-name",
		NetworkName:                networkName,
		RequestedMemory:            memory,
		RequestedCPU:               cpu,
		RequestedStorage:           storage,
		StorageClassName:           storageClassName,
		PersistentVolumeAccessMode: string(accessMode),
	}
)

func StubVirtualMachineInstance() *kubevirtapiv1.VirtualMachineInstance {
	return &kubevirtapiv1.VirtualMachineInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name: "localhost",
		},
	}
}

func StubIgnitionSecret() *corev1.Secret {
	ignitionSecretName := fmt.Sprintf("%s-ignition", MachineName)
	labels := map[string]string{
		fmt.Sprintf("tenantcluster-%s-machine.openshift.io", InfraID): "owned",
	}

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ignitionSecretName,
			Namespace: InfraNamespace,
			Labels:    labels,
		},
		Data: map[string][]byte{
			"userdata": []byte(fmt.Sprintf(FullUserDataFmt, MachineName)),
		},
	}
}

func StubMachine() (*machinev1.Machine, error) {
	providerSpecValue, providerSpecValueErr := kubevirtproviderv1alpha1.RawExtensionFromProviderSpec(&ProviderSpec)

	if providerSpecValueErr != nil {
		return nil, fmt.Errorf("codec.EncodeProviderSpec failed: %v", providerSpecValueErr)
	}
	providerID := "kubevirt"
	machine := &machinev1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:                       MachineName,
			Namespace:                  machineNamespace,
			Generation:                 0,
			CreationTimestamp:          metav1.Time{},
			DeletionTimestamp:          nil,
			DeletionGracePeriodSeconds: nil,
			Labels:                     deepCopyMap(labels),
			ClusterName:                clusterName,
		},
		Spec: machinev1.MachineSpec{
			ObjectMeta:   machinev1.ObjectMeta{},
			ProviderSpec: machinev1.ProviderSpec{Value: providerSpecValue},
			ProviderID:   &providerID,
		},
		Status: machinev1.MachineStatus{},
	}

	return machine, nil
}

func StubVirtualMachine(name *string, namespace *string, UID *string) *kubevirtapiv1.VirtualMachine {
	result := &kubevirtapiv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      MachineName,
			Namespace: InfraNamespace,
			Labels: func() map[string]string {
				result := deepCopyMap(labels)
				result[fmt.Sprintf("tenantcluster-%s-machine.openshift.io", InfraID)] = "owned"
				return result
			}(),
			ClusterName: clusterName,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "VirtualMachine",
			APIVersion: kubevirtapiv1.GroupVersion.String(),
		},
		Spec: kubevirtapiv1.VirtualMachineSpec{
			RunStrategy: func(src kubevirtapiv1.VirtualMachineRunStrategy) *kubevirtapiv1.VirtualMachineRunStrategy {
				return &src
			}(kubevirtapiv1.RunStrategyAlways),
			DataVolumeTemplates: []cdiv1.DataVolume{
				{
					TypeMeta: metav1.TypeMeta{APIVersion: cdiv1.SchemeGroupVersion.String()},
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("%s-bootvolume", MachineName),
						Namespace: InfraNamespace,
					},
					Spec: cdiv1.DataVolumeSpec{
						Source: cdiv1.DataVolumeSource{
							PVC: &cdiv1.DataVolumeSourcePVC{
								Name:      sourcePvcName,
								Namespace: InfraNamespace,
							},
						},
						PVC: &corev1.PersistentVolumeClaimSpec{
							AccessModes: []corev1.PersistentVolumeAccessMode{
								accessMode,
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: apiresource.MustParse(storage),
								},
							},
							StorageClassName: func(src string) *string {
								if src == "" {
									return nil
								}
								return &src
							}(storageClassName),
						},
					},
				},
			},
			Template: &kubevirtapiv1.VirtualMachineInstanceTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"kubevirt.io/vm": MachineName, "name": MachineName},
				},
				Spec: kubevirtapiv1.VirtualMachineInstanceSpec{
					TerminationGracePeriodSeconds: func(src int64) *int64 { return &src }(600),
					Volumes: []kubevirtapiv1.Volume{
						{
							Name: "datavolumedisk1",
							VolumeSource: kubevirtapiv1.VolumeSource{
								DataVolume: &kubevirtapiv1.DataVolumeSource{
									Name: fmt.Sprintf("%s-bootvolume", MachineName),
								},
							},
						},
						{
							Name: "cloudinitdisk",
							VolumeSource: kubevirtapiv1.VolumeSource{
								CloudInitConfigDrive: &kubevirtapiv1.CloudInitConfigDriveSource{
									UserDataSecretRef: &corev1.LocalObjectReference{
										Name: fmt.Sprintf("%s-ignition", MachineName),
									},
								},
							},
						},
					},
					Networks: []kubevirtapiv1.Network{
						{
							Name: "main",
							NetworkSource: kubevirtapiv1.NetworkSource{
								Multus: &kubevirtapiv1.MultusNetwork{
									NetworkName: networkName,
								},
							},
						},
					},
					Domain: kubevirtapiv1.DomainSpec{
						Resources: kubevirtapiv1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceMemory: apiresource.MustParse(memory),
								corev1.ResourceCPU:    apiresource.MustParse(fmt.Sprint(cpu)),
							},
						},
						Devices: kubevirtapiv1.Devices{
							Disks: []kubevirtapiv1.Disk{
								{
									Name: "datavolumedisk1",
									DiskDevice: kubevirtapiv1.DiskDevice{
										Disk: &kubevirtapiv1.DiskTarget{
											Bus: "virtio",
										},
									},
								},
								{
									Name: "cloudinitdisk",
									DiskDevice: kubevirtapiv1.DiskDevice{
										Disk: &kubevirtapiv1.DiskTarget{
											Bus: "virtio",
										},
									},
								},
							},
							Interfaces: []kubevirtapiv1.Interface{
								{
									Name: "main",
									InterfaceBindingMethod: kubevirtapiv1.InterfaceBindingMethod{
										Bridge: &kubevirtapiv1.InterfaceBridge{},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	if name != nil {
		result.Name = *name
	}
	if namespace != nil {
		result.Namespace = *namespace
	}
	if UID != nil {
		result.UID = types.UID(*UID)
	}
	return result
}

func deepCopyMap(src map[string]string) map[string]string {
	result := make(map[string]string)
	for k, v := range src {
		result[k] = v
	}
	return result
}

func StringPointer(src string) *string {
	return &src
}
