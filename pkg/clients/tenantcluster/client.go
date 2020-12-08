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

package tenantcluster

import (
	"context"
	"encoding/json"

	machinecontroller "github.com/openshift/machine-api-operator/pkg/controller/machine"

	machinev1 "github.com/openshift/machine-api-operator/pkg/apis/machine/v1beta1"
	corev1 "k8s.io/api/core/v1"
	k8smetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

//go:generate mockgen -source=./client.go -destination=./mock/client_generated.go -package=mock
const (
	ConfigMapNamespace        = "openshift-config"
	ConfigMapName             = "cloud-provider-config"
	ConfigMapDataKeyName      = "config"
	ConfigMapNamespaceKeyName = "namespace"
	ConfigMapInfraIDKeyName   = "infraID"
)

// Client is a wrapper object for actual tenant-cluster clients: kubernetesClient and runtimeClient
type Client interface {
	PatchMachine(machine *machinev1.Machine, originMachineCopy *machinev1.Machine) error
	StatusPatchMachine(machine *machinev1.Machine, originMachineCopy *machinev1.Machine) error
	GetSecret(ctx context.Context, secretName string, namespace string) (*corev1.Secret, error)
	GetNamespace() (string, error)
	GetInfraID() (string, error)
}

type kubeClient struct {
	kubernetesClient *kubernetes.Clientset
	runtimeClient    client.Client
}

// New creates our client wrapper object for the actual KubeVirt and VirtCtl clients we use.
func New(mgr manager.Manager) (Client, error) {
	kubernetesClient, err := kubernetes.NewForConfig(mgr.GetConfig())
	if err != nil {
		return nil, err
	}

	return &kubeClient{
		kubernetesClient: kubernetesClient,
		runtimeClient:    mgr.GetClient(),
	}, nil
}

func (c *kubeClient) PatchMachine(machine *machinev1.Machine, originMachineCopy *machinev1.Machine) error {
	return c.runtimeClient.Patch(context.Background(), machine, client.MergeFrom(originMachineCopy))
}

func (c *kubeClient) StatusPatchMachine(machine *machinev1.Machine, originMachineCopy *machinev1.Machine) error {
	return c.runtimeClient.Status().Patch(context.Background(), machine, client.MergeFrom(originMachineCopy))
}

func (c *kubeClient) GetSecret(ctx context.Context, secretName string, namespace string) (*corev1.Secret, error) {
	return c.kubernetesClient.CoreV1().Secrets(namespace).Get(ctx, secretName, k8smetav1.GetOptions{})
}

func (c *kubeClient) GetInfraID() (string, error) {
	cMap, err := c.getConfigMap(context.Background())
	if err != nil {
		return "", nil
	}
	infraID, ok := (*cMap)[ConfigMapInfraIDKeyName]
	if !ok {
		return "", machinecontroller.InvalidMachineConfiguration("Tenant-cluster configMap %s/%s: The map extracted with key %s doesn't contain key %s", ConfigMapNamespace, ConfigMapName, ConfigMapDataKeyName, ConfigMapInfraIDKeyName)
	}
	return infraID, nil
}

func (c *kubeClient) GetNamespace() (string, error) {
	cMap, err := c.getConfigMap(context.Background())
	if err != nil {
		return "", nil
	}
	vmNamespace, ok := (*cMap)[ConfigMapNamespaceKeyName]
	if !ok {
		return "", machinecontroller.InvalidMachineConfiguration("Tenant-cluster configMap %s/%s: The map extracted with key %s doesn't contain key %s", ConfigMapNamespace, ConfigMapName, ConfigMapDataKeyName, ConfigMapNamespaceKeyName)
	}
	return vmNamespace, nil
}

func (c *kubeClient) getConfigMap(ctx context.Context) (*map[string]string, error) {
	configMap, err := c.kubernetesClient.CoreV1().ConfigMaps(ConfigMapNamespace).Get(ctx, ConfigMapName, k8smetav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	config, ok := configMap.Data[ConfigMapDataKeyName]
	if !ok {
		return nil, machinecontroller.InvalidMachineConfiguration("Tenant-cluster configMap %s/%s doesn't contain the key %s", ConfigMapNamespace, ConfigMapName, ConfigMapDataKeyName)
	}
	var cMap map[string]string
	if err := json.Unmarshal([]byte(config), &cMap); err != nil {
		return nil, machinecontroller.InvalidMachineConfiguration("Tenant-cluster configMap %s/%s: Data of key %s is not of type map[string]string", ConfigMapNamespace, ConfigMapName, ConfigMapDataKeyName)
	}
	return &cMap, nil
}
