/*
Copyright 2018 The Kubernetes Authors.

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

package actuator

import (
	"context"
	"fmt"

	machinev1 "github.com/openshift/machine-api-operator/pkg/apis/machine/v1beta1"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog"

	"github.com/openshift/cluster-api-provider-kubevirt/pkg/clients/tenantcluster"
	"github.com/openshift/cluster-api-provider-kubevirt/pkg/kubevirt"
	"github.com/openshift/cluster-api-provider-kubevirt/pkg/machinescope"
	machinecontroller "github.com/openshift/machine-api-operator/pkg/controller/machine"
	apimachineryerrors "k8s.io/apimachinery/pkg/api/errors"
)

type eventAction string

const (
	createEventAction eventAction = "create machine"
	updateEventAction eventAction = "update machine"
	deleteEventAction eventAction = "delete machine"
	existsEventAction eventAction = "check machine exists"

	userDataKey = "userData"

	configMapNamespace             = "openshift-config"
	configMapName                  = "cloud-provider-config"
	configMapDataKeyName           = "config"
	configMapInfraNamespaceKeyName = "namespace"
	configMapInfraIDKeyName        = "infraID"
)

// actuator is responsible for performing machine reconciliation.
type actuator struct {
	eventRecorder       record.EventRecorder
	kubevirtVM          kubevirt.KubevirtVM
	machineScopeCreator machinescope.MachineScopeCreator
	tenantClusterClient tenantcluster.Client
	infraID             string
	infraNamespace      string
}

// New returns an actuator.
func New(kubevirtVM kubevirt.KubevirtVM,
	eventRecorder record.EventRecorder,
	machineScopeCreator machinescope.MachineScopeCreator,
	tenantClusterClient tenantcluster.Client) (machinecontroller.Actuator, error) {

	cMap, err := tenantClusterClient.GetConfigMapValue(context.Background(), configMapName, configMapNamespace, configMapDataKeyName)
	if err != nil {
		return nil, nil
	}
	infraID, ok := (*cMap)[configMapInfraIDKeyName]
	if !ok {
		return nil, machinecontroller.InvalidMachineConfiguration("Actuator: configMap %s/%s: The map extracted with key %s doesn't contain key %s",
			configMapNamespace, configMapName, configMapDataKeyName, configMapInfraIDKeyName)
	}
	infraNamespace, ok := (*cMap)[configMapInfraNamespaceKeyName]
	if !ok {
		return nil, machinecontroller.InvalidMachineConfiguration("Actuator: configMap %s/%s: The map extracted with key %s doesn't contain key %s",
			configMapNamespace, configMapName, configMapDataKeyName, configMapInfraNamespaceKeyName)
	}
	return &actuator{
		kubevirtVM:          kubevirtVM,
		eventRecorder:       eventRecorder,
		machineScopeCreator: machineScopeCreator,
		tenantClusterClient: tenantClusterClient,
		infraID:             infraID,
		infraNamespace:      infraNamespace,
	}, nil
}

func (a *actuator) createMachineScope(machine *machinev1.Machine) (machinescope.MachineScope, error) {
	return a.machineScopeCreator.CreateMachineScope(machine, a.infraNamespace, a.infraID)
}

// Set corresponding event based on error. It also returns the original error
// for convenience, so callers can do "return handleMachineError(...)".
func (a *actuator) handleMachineError(machine *machinev1.Machine, action *eventAction, err error) error {
	errMsg := fmt.Sprintf("%s: kubevirt wrapper failed to %s: %v", machine.GetName(), *action, err)
	klog.Errorf(errMsg)
	if action != nil {
		a.eventRecorder.Eventf(machine, corev1.EventTypeWarning, fmt.Sprintf("%s failed", *action), errMsg)
	}
	return fmt.Errorf(errMsg)
}

func (a *actuator) eventActionPointer(action eventAction) *eventAction {
	return &action
}

// Create creates a machine and is invoked by the machine controller.
func (a *actuator) Create(ctx context.Context, machine *machinev1.Machine) error {
	originMachineCopy := machine.DeepCopy()

	machineScope, err := a.createMachineScope(machine)
	if err != nil {
		return a.handleMachineError(machine, a.eventActionPointer(createEventAction), err)
	}

	klog.Infof("%s: actuator creating machine", machineScope.GetMachineName())

	userData, err := a.getUserData(machineScope)
	if err != nil {
		return a.handleMachineError(machine, a.eventActionPointer(createEventAction), err)
	}

	err = a.kubevirtVM.Create(machineScope, userData)
	patchErr := a.patchMachine(machineScope.GetMachine(), originMachineCopy)
	if patchErr != nil {
		err = patchErr
	}
	if err != nil {
		return a.handleMachineError(machine, a.eventActionPointer(createEventAction), err)
	}

	a.eventRecorder.Eventf(machine, corev1.EventTypeNormal, string(createEventAction), "Created Machine %v", machineScope.GetMachineName())

	return nil
}

func (a *actuator) getUserData(machineScope machinescope.MachineScope) ([]byte, error) {
	secretName := machineScope.GetIgnitionSecretName()
	machineNamespace := machineScope.GetMachineNamespace()
	userDataSecret, err := a.tenantClusterClient.GetSecret(context.Background(), secretName, machineNamespace)
	if err != nil {
		if apimachineryerrors.IsNotFound(err) {
			return nil, machinecontroller.InvalidMachineConfiguration("Tenant-cluster credentials secret %s/%s: %v not found", machineNamespace, secretName, err)
		}
		return nil, err
	}
	userData, ok := userDataSecret.Data[userDataKey]
	if !ok {
		return nil, machinecontroller.InvalidMachineConfiguration("Tenant-cluster credentials secret %s/%s: %v doesn't contain the key", machineNamespace, secretName, userDataKey)
	}
	return userData, nil
}

// Exists determines if the given machine currently exists.
// A machine which is not terminated is considered as existing.
func (a *actuator) Exists(ctx context.Context, machine *machinev1.Machine) (bool, error) {
	klog.Infof("%s: actuator checking if machine exists", machine.GetName())

	return a.kubevirtVM.Exists(machine.GetName(), a.infraNamespace)
}

// Update attempts to sync machine state with an existing instance.
func (a *actuator) Update(ctx context.Context, machine *machinev1.Machine) error {
	originMachineCopy := machine.DeepCopy()

	machineScope, err := a.createMachineScope(machine)
	if err != nil {
		return a.handleMachineError(machine, a.eventActionPointer(updateEventAction), err)
	}

	klog.Infof("%s: actuator updating machine", machineScope.GetMachineName())

	wasUpdated, err := a.kubevirtVM.Update(machineScope)
	patchErr := a.patchMachine(machineScope.GetMachine(), originMachineCopy)
	if patchErr != nil {
		err = patchErr
	}
	if err != nil {
		return a.handleMachineError(machine, a.eventActionPointer(updateEventAction), err)
	}

	// Create event only if machine object was modified
	if wasUpdated {
		a.eventRecorder.Eventf(machine, corev1.EventTypeNormal, string(updateEventAction), "Updated Machine %v", machineScope.GetMachineName())
	}

	return nil
}

// Delete deletes a machine and updates its finalizer
func (a *actuator) Delete(ctx context.Context, machine *machinev1.Machine) error {
	machineScope, err := a.createMachineScope(machine)
	if err != nil {
		return a.handleMachineError(machine, a.eventActionPointer(deleteEventAction), err)
	}

	klog.Infof("%s: actuator deleting machine", machineScope.GetMachineName())

	if err := a.kubevirtVM.Delete(machineScope); err != nil {
		return a.handleMachineError(machine, a.eventActionPointer(deleteEventAction), err)
	}

	a.eventRecorder.Eventf(machine, corev1.EventTypeNormal, string(deleteEventAction), "Deleted machine %v", machineScope.GetMachineName())
	return nil
}

// Patch patches the machine spec and machine status after reconciling.
func (a *actuator) patchMachine(machine *machinev1.Machine, originMachineCopy *machinev1.Machine) error {
	klog.V(3).Infof("%v: patching machine", machine.GetName())

	// patch machine
	statusCopy := *machine.Status.DeepCopy()
	if err := a.tenantClusterClient.PatchMachine(machine, originMachineCopy); err != nil {
		return errors.Wrap(err, "failed to patch machine")
	}

	machine.Status = statusCopy

	// patch status
	if err := a.tenantClusterClient.StatusPatchMachine(machine, originMachineCopy); err != nil {
		return errors.Wrap(err, "failed to patch machine status")
	}

	return nil
}
