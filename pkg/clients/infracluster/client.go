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

package infracluster

import (
	"github.com/openshift/cluster-api-provider-kubevirt/pkg/clients/tenantcluster"
	machineapiapierrors "github.com/openshift/machine-api-operator/pkg/controller/machine"
	corev1 "k8s.io/api/core/v1"
	apimachineryerrors "k8s.io/apimachinery/pkg/api/errors"
	k8smetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	kubevirtapiv1 "kubevirt.io/client-go/api/v1"
	"kubevirt.io/client-go/kubecli"
)

//go:generate mockgen -source=./client.go -destination=./mock/client_generated.go -package=mock

const (
	// platformCredentialsKey is secret key containing kubeconfig content of the infra-cluster
	platformCredentialsKey                  = "kubeconfig"
	defaultCredentialsSecretSecretName      = "kubevirt-credentials"
	defaultCredentialsSecretSecretNamespace = "openshift-machine-api"
)

// Client is a wrapper object for actual infra-cluster clients: kubernetes and the kubevirt
type Client interface {
	CreateVirtualMachine(namespace string, newVM *kubevirtapiv1.VirtualMachine) (*kubevirtapiv1.VirtualMachine, error)
	DeleteVirtualMachine(namespace string, name string, options *k8smetav1.DeleteOptions) error
	GetVirtualMachine(namespace string, name string, options *k8smetav1.GetOptions) (*kubevirtapiv1.VirtualMachine, error)
	GetVirtualMachineInstance(namespace string, name string, options *k8smetav1.GetOptions) (*kubevirtapiv1.VirtualMachineInstance, error)
	ListVirtualMachine(namespace string, options *k8smetav1.ListOptions) (*kubevirtapiv1.VirtualMachineList, error)
	UpdateVirtualMachine(namespace string, vm *kubevirtapiv1.VirtualMachine) (*kubevirtapiv1.VirtualMachine, error)
	PatchVirtualMachine(namespace string, name string, pt types.PatchType, data []byte, subresources ...string) (result *kubevirtapiv1.VirtualMachine, err error)
	RestartVirtualMachine(namespace string, name string) error
	StartVirtualMachine(namespace string, name string) error
	StopVirtualMachine(namespace string, name string) error
	CreateSecret(namespace string, newSecret *corev1.Secret) (*corev1.Secret, error)
}

type client struct {
	kubevirtClient   kubecli.KubevirtClient
	kubernetesClient *kubernetes.Clientset
}

// New creates our client wrapper object for the actual kubeVirt and kubernetes clients we use.
func New(tenantClusterKubernetesClient tenantcluster.Client) (Client, error) {
	returnedSecret, err := tenantClusterKubernetesClient.GetSecret(defaultCredentialsSecretSecretName, defaultCredentialsSecretSecretNamespace)
	if err != nil {
		if apimachineryerrors.IsNotFound(err) {
			return nil, machineapiapierrors.InvalidMachineConfiguration("Infra-cluster credentials secret %s/%s: %v not found", defaultCredentialsSecretSecretNamespace, defaultCredentialsSecretSecretName, err)
		}
		return nil, err
	}
	platformCredentials, ok := returnedSecret.Data[platformCredentialsKey]
	if !ok {
		return nil, machineapiapierrors.InvalidMachineConfiguration("Infra-cluster credentials secret %v did not contain key %v",
			defaultCredentialsSecretSecretName, platformCredentials)
	}

	clientConfig, err := clientcmd.NewClientConfigFromBytes(platformCredentials)
	if err != nil {
		return nil, err
	}
	kubevirtClient, getClientErr := kubecli.GetKubevirtClientFromClientConfig(clientConfig)
	if getClientErr != nil {
		return nil, getClientErr
	}
	restClientConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, err
	}
	kubernetesClient, err := kubernetes.NewForConfig(restClientConfig)
	if err != nil {
		return nil, err
	}
	return &client{
		kubevirtClient:   kubevirtClient,
		kubernetesClient: kubernetesClient,
	}, nil
}

func (c *client) CreateVirtualMachine(namespace string, newVM *kubevirtapiv1.VirtualMachine) (*kubevirtapiv1.VirtualMachine, error) {
	return c.kubevirtClient.VirtualMachine(namespace).Create(newVM)
}

func (c *client) DeleteVirtualMachine(namespace string, name string, options *k8smetav1.DeleteOptions) error {
	return c.kubevirtClient.VirtualMachine(namespace).Delete(name, options)
}

func (c *client) GetVirtualMachine(namespace string, name string, options *k8smetav1.GetOptions) (*kubevirtapiv1.VirtualMachine, error) {
	return c.kubevirtClient.VirtualMachine(namespace).Get(name, options)
}

func (c *client) GetVirtualMachineInstance(namespace string, name string, options *k8smetav1.GetOptions) (*kubevirtapiv1.VirtualMachineInstance, error) {
	return c.kubevirtClient.VirtualMachineInstance(namespace).Get(name, options)
}

func (c *client) ListVirtualMachine(namespace string, options *k8smetav1.ListOptions) (*kubevirtapiv1.VirtualMachineList, error) {
	return c.kubevirtClient.VirtualMachine(namespace).List(options)
}

func (c *client) UpdateVirtualMachine(namespace string, vm *kubevirtapiv1.VirtualMachine) (*kubevirtapiv1.VirtualMachine, error) {
	return c.kubevirtClient.VirtualMachine(namespace).Update(vm)
}

func (c *client) PatchVirtualMachine(namespace string, name string, pt types.PatchType, data []byte, subresources ...string) (result *kubevirtapiv1.VirtualMachine, err error) {
	return c.kubevirtClient.VirtualMachine(namespace).Patch(name, pt, data, subresources...)
}

func (c *client) RestartVirtualMachine(namespace string, name string) error {
	return c.kubevirtClient.VirtualMachine(namespace).Restart(name)
}

func (c *client) StartVirtualMachine(namespace string, name string) error {
	return c.kubevirtClient.VirtualMachine(namespace).Start(name)
}

func (c *client) StopVirtualMachine(namespace string, name string) error {
	return c.kubevirtClient.VirtualMachine(namespace).Stop(name)
}

func (c *client) CreateSecret(namespace string, newSecret *corev1.Secret) (*corev1.Secret, error) {
	return c.kubernetesClient.CoreV1().Secrets(namespace).Create(newSecret)
}
