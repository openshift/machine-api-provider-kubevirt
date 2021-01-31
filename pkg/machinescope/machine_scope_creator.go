package machinescope

import (
	kubevirtproviderv1alpha1 "github.com/openshift/cluster-api-provider-kubevirt/pkg/apis/kubevirtprovider/v1alpha1"
	machinev1 "github.com/openshift/machine-api-operator/pkg/apis/machine/v1beta1"
	machinecontroller "github.com/openshift/machine-api-operator/pkg/controller/machine"
)

type MachineScopeCreator interface {
	// CreateMachineScope creates MachineScope struct
	CreateMachineScope(machine *machinev1.Machine, infraNamespace string, infraID string) (MachineScope, error)
}

type machineScopeCreator struct{}

func New() MachineScopeCreator {
	return machineScopeCreator{}
}

func (creator machineScopeCreator) CreateMachineScope(machine *machinev1.Machine, infraNamespace string, infraID string) (MachineScope, error) {
	// TODO: insert a validation on machine labels
	if machine.Labels[machinev1.MachineClusterIDLabel] == "" {
		return nil, machinecontroller.InvalidMachineConfiguration("%v: missing %q label", machine.GetName(), machinev1.MachineClusterIDLabel)
	}

	providerSpec, err := kubevirtproviderv1alpha1.ProviderSpecFromRawExtension(machine.Spec.ProviderSpec.Value)
	if err != nil {
		return nil, machinecontroller.InvalidMachineConfiguration("failed to get machine config: %v", err)
	}

	if err != nil {
		return nil, machinecontroller.InvalidMachineConfiguration("failed to get machine provider status: %v", err.Error())
	}

	return &machineScope{
		machine:             machine,
		machineProviderSpec: providerSpec,
		infraNamespace:      infraNamespace,
		infraID:             infraID,
	}, nil
}
