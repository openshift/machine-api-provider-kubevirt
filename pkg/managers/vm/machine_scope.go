package vm

import (
	"fmt"
	"net"
	"time"

	apimachineryerrors "k8s.io/apimachinery/pkg/api/errors"

	machinecontroller "github.com/openshift/machine-api-operator/pkg/controller/machine"

	kubevirtproviderv1alpha1 "github.com/openshift/cluster-api-provider-kubevirt/pkg/apis/kubevirtprovider/v1alpha1"
	"github.com/openshift/cluster-api-provider-kubevirt/pkg/clients/infracluster"
	"github.com/openshift/cluster-api-provider-kubevirt/pkg/clients/tenantcluster"
	providerctrl "github.com/openshift/cluster-api-provider-kubevirt/pkg/providerid"
	"github.com/openshift/cluster-api-provider-kubevirt/pkg/utils"
	machinev1 "github.com/openshift/machine-api-operator/pkg/apis/machine/v1beta1"
	corev1 "k8s.io/api/core/v1"
	apiresource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/klog"
	kubevirtapiv1 "kubevirt.io/client-go/api/v1"
	cdiv1 "kubevirt.io/containerized-data-importer/pkg/apis/core/v1alpha1"
)

type machineState string

const (
	vmNotCreated      machineState = "vmNotCreated"
	vmCreatedNotReady machineState = "vmWasCreatedButNotReady"
	vmCreatedAndReady machineState = "vmWasCreatedAndReady"
)

const (
	defaultRequestedMemory            = "2048M"
	defaultRequestedStorage           = "35Gi"
	defaultPersistentVolumeAccessMode = corev1.ReadWriteMany
	defaultDataVolumeDiskName         = "datavolumedisk1"
	defaultCloudInitVolumeDiskName    = "cloudinitdisk"
	defaultBootVolumeDiskName         = "bootvolume"
	kubevirtIdAnnotationKey           = "VmId"
	userDataKey                       = "userData"
	defaultBus                        = "virtio"
	APIVersion                        = "kubevirt.io/v1alpha3"
	Kind                              = "VirtualMachine"
	mainNetworkName                   = "main"
)

const providerIDFormat = "kubevirt://%s/%s"

type machineScope struct {
	infraClusterClient    infracluster.Client
	tenantClusterClient   tenantcluster.Client
	machine               *machinev1.Machine
	originMachineCopy     *machinev1.Machine
	machineProviderSpec   *kubevirtproviderv1alpha1.KubevirtMachineProviderSpec
	machineProviderStatus *kubevirtproviderv1alpha1.KubevirtMachineProviderStatus
	vmNamespace           string
	infraID               string
}

func newMachineScope(machine *machinev1.Machine, tenantClusterClient tenantcluster.Client, infraClusterClientBuilder infracluster.ClientBuilderFuncType) (*machineScope, error) {
	if err := validateMachine(*machine); err != nil {
		return nil, fmt.Errorf("%v: failed validating machine provider spec: %w", machine.GetName(), err)
	}

	providerSpec, err := kubevirtproviderv1alpha1.ProviderSpecFromRawExtension(machine.Spec.ProviderSpec.Value)
	if err != nil {
		return nil, machinecontroller.InvalidMachineConfiguration("failed to get machine config: %v", err)
	}

	providerStatus, err := kubevirtproviderv1alpha1.ProviderStatusFromRawExtension(machine.Status.ProviderStatus)
	if err != nil {
		return nil, machinecontroller.InvalidMachineConfiguration("failed to get machine provider status: %v", err.Error())
	}

	infraClusterClient, err := infraClusterClientBuilder(tenantClusterClient, providerSpec.CredentialsSecretName, machine.GetNamespace())
	if err != nil {
		return nil, machinecontroller.InvalidMachineConfiguration("failed to create aKubeVirt client: %v", err.Error())
	}

	vmNamespace, err := tenantClusterClient.GetNamespace()
	if err != nil {
		return nil, err
	}
	infraID, err := tenantClusterClient.GetInfraID()
	if err != nil {
		return nil, err
	}

	return &machineScope{
		infraClusterClient:    infraClusterClient,
		tenantClusterClient:   tenantClusterClient,
		machine:               machine,
		originMachineCopy:     machine.DeepCopy(),
		machineProviderSpec:   providerSpec,
		machineProviderStatus: providerStatus,
		vmNamespace:           vmNamespace,
		infraID:               infraID,
	}, nil
}

func (s *machineScope) assertMandatoryParams() error {
	switch {
	case s.machineProviderSpec.SourcePvcName == "":
		return machinecontroller.InvalidMachineConfiguration("%v: missing value for SourcePvcName", s.machine.GetName())
	case s.machineProviderSpec.IgnitionSecretName == "":
		return machinecontroller.InvalidMachineConfiguration("%v: missing value for IgnitionSecretName", s.machine.GetName())
	case s.machineProviderSpec.NetworkName == "":
		return machinecontroller.InvalidMachineConfiguration("%v: missing value for NetworkName", s.machine.GetName())
	default:
		return nil
	}
}

func (s *machineScope) createIgnitionSecretFromMachine() (*corev1.Secret, error) {
	virtualMachineName := s.machine.GetName()
	ignitionSecretName := buildIgnitionSecretName(virtualMachineName)
	namespace := s.vmNamespace
	labels := utils.BuildLabels(s.infraID)
	userData, err := s.getUserData(namespace, virtualMachineName)
	if err != nil {
		return nil, err
	}

	resultSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ignitionSecretName,
			Namespace: namespace,
			Labels:    labels,
		},
		Data: map[string][]byte{
			"userdata": userData,
		},
	}

	return resultSecret, nil
}

func (s *machineScope) createVirtualMachineFromMachine() (*kubevirtapiv1.VirtualMachine, error) {
	if err := s.assertMandatoryParams(); err != nil {
		return nil, err
	}
	runAlways := kubevirtapiv1.RunStrategyAlways

	vmiTemplate, err := s.buildVMITemplate(s.vmNamespace)
	if err != nil {
		return nil, err
	}

	pvcRequestsStorage := s.machineProviderSpec.RequestedStorage
	if pvcRequestsStorage == "" {
		pvcRequestsStorage = defaultRequestedStorage
	}
	PVCAccessMode := defaultPersistentVolumeAccessMode
	if s.machineProviderSpec.PersistentVolumeAccessMode != "" {
		accessMode := corev1.PersistentVolumeAccessMode(s.machineProviderSpec.PersistentVolumeAccessMode)
		switch accessMode {
		case corev1.ReadWriteMany:
			PVCAccessMode = corev1.ReadWriteMany
		case corev1.ReadOnlyMany:
			PVCAccessMode = corev1.ReadOnlyMany
		case corev1.ReadWriteOnce:
			PVCAccessMode = corev1.ReadWriteOnce
		default:
			return nil, machinecontroller.InvalidMachineConfiguration("%v: Value of PersistentVolumeAccessMode, can be only one of: %v, %v, %v",
				s.machine.GetName(), corev1.ReadWriteMany, corev1.ReadOnlyMany, corev1.ReadWriteOnce)
		}
	}

	virtualMachine := kubevirtapiv1.VirtualMachine{
		Spec: kubevirtapiv1.VirtualMachineSpec{
			RunStrategy: &runAlways,
			DataVolumeTemplates: []cdiv1.DataVolume{
				*buildBootVolumeDataVolumeTemplate(s.machine.GetName(), s.machineProviderSpec.SourcePvcName, s.vmNamespace, s.machineProviderSpec.StorageClassName, pvcRequestsStorage, PVCAccessMode),
			},
			Template: vmiTemplate,
		},
	}

	labels := utils.BuildLabels(s.infraID)
	for k, v := range s.machine.Labels {
		labels[k] = v
	}

	virtualMachine.APIVersion = APIVersion
	virtualMachine.Kind = Kind
	virtualMachine.ObjectMeta = metav1.ObjectMeta{
		Name:            s.machine.Name,
		Namespace:       s.vmNamespace,
		Labels:          labels,
		Annotations:     s.machine.Annotations,
		OwnerReferences: nil,
		ClusterName:     s.machine.ClusterName,
	}

	return &virtualMachine, nil
}

func (s *machineScope) getMachineName() string {
	return s.machine.GetName()
}

func (s *machineScope) getMachineNamespace() string {
	return s.machine.GetNamespace()
}

// setProviderID adds providerID in the machine spec
func (s *machineScope) setProviderID(vm *kubevirtapiv1.VirtualMachine) {
	// TODO: return an error when the setting is failed
	existingProviderID := s.machine.Spec.ProviderID
	if vm == nil {
		return
	}

	providerID := providerctrl.FormatProviderID(vm.GetNamespace(), vm.GetName())

	if existingProviderID != nil && *existingProviderID == providerID {
		klog.Infof("%s: ProviderID already set in the machine Spec with value:%s", s.getMachineName(), *existingProviderID)
		return
	}

	s.machine.Spec.ProviderID = &providerID
	klog.Infof("%s: ProviderID set at machine spec: %s", s.getMachineName(), providerID)
}

// updateAllowed validates that updates come in the right order
// if there is an update that was supposes to be done after that update - return an error
func (s *machineScope) updateAllowed() bool {
	return s.machine.Spec.ProviderID != nil && *s.machine.Spec.ProviderID != "" && (s.machine.Status.LastUpdated == nil || s.machine.Status.LastUpdated.Add(requeueAfterSeconds*time.Second).After(time.Now()))
}

func buildDataVolumeDiskName(virtualMachineName string) string {
	return buildVolumeName(virtualMachineName, defaultDataVolumeDiskName)
}
func buildCloudInitVolumeDiskName(virtualMachineName string) string {
	return buildVolumeName(virtualMachineName, defaultCloudInitVolumeDiskName)
}

func buildBootVolumeName(virtualMachineName string) string {
	return buildVolumeName(virtualMachineName, defaultBootVolumeDiskName)
}

func buildVolumeName(virtualMachineName, suffixVolumeName string) string {
	return fmt.Sprintf("%s-%s", virtualMachineName, suffixVolumeName)
}

func (s *machineScope) buildVMITemplate(namespace string) (*kubevirtapiv1.VirtualMachineInstanceTemplateSpec, error) {
	virtualMachineName := s.machine.GetName()

	template := &kubevirtapiv1.VirtualMachineInstanceTemplateSpec{}

	template.ObjectMeta = metav1.ObjectMeta{
		Labels: map[string]string{"kubevirt.io/vm": virtualMachineName, "name": virtualMachineName},
	}

	ignitionSecretName := buildIgnitionSecretName(virtualMachineName)

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
						Name: ignitionSecretName,
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

	return template, nil
}

func buildIgnitionSecretName(virtualMachineName string) string {
	return fmt.Sprintf("%s-ignition", virtualMachineName)
}

func (s *machineScope) getUserData(namespace string, virtualMachineName string) ([]byte, error) {
	secretName := s.machineProviderSpec.IgnitionSecretName
	userDataSecret, err := s.tenantClusterClient.GetSecret(secretName, s.machine.GetNamespace())
	if err != nil {
		if apimachineryerrors.IsNotFound(err) {
			return nil, machinecontroller.InvalidMachineConfiguration("Tenant-cluster credentials secret %s/%s: %v not found", namespace, secretName, err)
		}
		return nil, err
	}
	userDataByte, ok := userDataSecret.Data[userDataKey]
	if !ok {
		return nil, machinecontroller.InvalidMachineConfiguration("Tenant-cluster credentials secret %s/%s: %v doesn't contain the key", namespace, secretName, userDataKey)
	}
	fullUserData, err := addHostnameToUserData(userDataByte, virtualMachineName)
	if err != nil {
		return nil, err
	}
	return fullUserData, nil
}

func buildBootVolumeDataVolumeTemplate(virtualMachineName, pvcName, dvNamespace, storageClassName,
	pvcRequestsStorage string, accessMode corev1.PersistentVolumeAccessMode) *cdiv1.DataVolume {

	persistentVolumeClaimSpec := corev1.PersistentVolumeClaimSpec{
		AccessModes: []corev1.PersistentVolumeAccessMode{
			accessMode,
		},
		// TODO: Where to get it?? - add as a list
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceStorage: apiresource.MustParse(pvcRequestsStorage),
			},
		},
	}
	if storageClassName != "" {
		persistentVolumeClaimSpec.StorageClassName = &storageClassName
	}

	return &cdiv1.DataVolume{
		TypeMeta: metav1.TypeMeta{APIVersion: cdiv1.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Name:      buildBootVolumeName(virtualMachineName),
			Namespace: dvNamespace,
		},
		Spec: cdiv1.DataVolumeSpec{
			Source: cdiv1.DataVolumeSource{
				PVC: &cdiv1.DataVolumeSourcePVC{
					Name:      pvcName,
					Namespace: dvNamespace,
				},
			},
			PVC: &persistentVolumeClaimSpec,
		},
	}
}

func (s *machineScope) SyncMachineFromVm(vm *kubevirtapiv1.VirtualMachine, vmi *kubevirtapiv1.VirtualMachineInstance) error {
	s.setProviderID(vm)

	if err := s.setMachineAnnotationsAndLabels(vm); err != nil {
		return fmt.Errorf("failed to set machine cloud provider specifics: %w", err)
	}

	if err := s.setProviderStatus(vm, vmi, conditionSuccess()); err != nil {
		return machinecontroller.InvalidMachineConfiguration("failed to set machine provider status: %v", err.Error())
	}

	klog.Infof("Updated machine %s", s.getMachineName())
	return nil
}

func (s *machineScope) setMachineAnnotationsAndLabels(vm *kubevirtapiv1.VirtualMachine) error {
	if vm == nil {
		return nil
	}

	if s.machine.Labels == nil {
		s.machine.Labels = make(map[string]string)
	}

	if s.machine.Spec.Labels == nil {
		s.machine.Spec.Labels = make(map[string]string)
	}

	if s.machine.Annotations == nil {
		s.machine.Annotations = make(map[string]string)
	}
	vmId := vm.UID
	vmType := vm.Spec.Template.Spec.Domain.Machine.Type

	vmState := vmNotCreated
	if vm.Status.Created {
		vmState = vmCreatedNotReady
		if vm.Status.Ready {
			vmState = vmCreatedAndReady
		}
	}

	s.machine.ObjectMeta.Annotations[kubevirtIdAnnotationKey] = string(vmId)
	s.machine.Labels[machinecontroller.MachineInstanceTypeLabelName] = vmType
	s.machine.Annotations[machinecontroller.MachineInstanceStateAnnotationName] = string(vmState)

	return nil
}

// Patch patches the machine spec and machine status after reconciling.
func (s *machineScope) patchMachine() error {

	klog.V(3).Infof("%v: patching machine", s.machine.GetName())

	providerStatus, err := kubevirtproviderv1alpha1.RawExtensionFromProviderStatus(s.machineProviderStatus)
	if err != nil {
		return machinecontroller.InvalidMachineConfiguration("failed to get machine provider status: %v", err.Error())
	}
	s.machine.Status.ProviderStatus = providerStatus

	// patch machine
	statusCopy := *s.machine.Status.DeepCopy()
	if err := s.tenantClusterClient.PatchMachine(s.machine, s.originMachineCopy); err != nil {
		klog.Errorf("Failed to patch machine %q: %v", s.machine.GetName(), err)
		return err
	}

	s.machine.Status = statusCopy

	// patch status
	if err := s.tenantClusterClient.StatusPatchMachine(s.machine, s.originMachineCopy); err != nil {
		klog.Errorf("Failed to patch machine status %q: %v", s.machine.GetName(), err)
		return err
	}

	return nil
}

func machineProviderStatusFromVirtualMachine(virtualMachine *kubevirtapiv1.VirtualMachine) *kubevirtproviderv1alpha1.KubevirtMachineProviderStatus {
	return &kubevirtproviderv1alpha1.KubevirtMachineProviderStatus{
		VirtualMachineStatus: virtualMachine.Status,
	}
}

func (s *machineScope) setProviderStatus(vm *kubevirtapiv1.VirtualMachine, vmi *kubevirtapiv1.VirtualMachineInstance, condition kubevirtapiv1.VirtualMachineCondition) error {
	if vm == nil {
		klog.Infof("%s: couldn't calculate KubeVirt status - the provided vm is empty", s.machine.GetName())
		return nil
	}
	klog.Infof("%s: Updating status", s.machine.GetName())
	var networkAddresses []corev1.NodeAddress
	s.machineProviderStatus = machineProviderStatusFromVirtualMachine(vm)

	// update nodeAddresses
	networkAddresses = append(networkAddresses, corev1.NodeAddress{Address: vm.Name, Type: corev1.NodeInternalDNS})

	klog.V(5).Infof("using hostname %s to resolve addresses", vm.Name)
	ips, err := net.LookupIP(vm.Name)
	if err == nil {
		for _, ip := range ips {
			if ip.To4() != nil {
				networkAddresses = append(networkAddresses, corev1.NodeAddress{Type: corev1.NodeInternalIP, Address: ip.String()})
			}
		}
	}

	klog.Infof("%s: finished calculating KubeVirt status", s.machine.GetName())

	s.machine.Status.Addresses = networkAddresses
	// TODO: update the phase of the machine
	//s.machine.Status.Phase = setKubevirtMachineProviderCondition(condition, vm.Status.Conditions)

	return nil
}

// GetMachineName return the name of the provided Machine
func GetMachineName(machine *machinev1.Machine) string {
	return machine.GetName()
}

func formatProviderID(namespace, name string) string {
	return fmt.Sprintf(providerIDFormat, namespace, name)
}
