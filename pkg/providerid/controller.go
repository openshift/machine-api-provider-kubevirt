// providerID package implements a controller to reconcile the providerID spec
// property on nodes in order to identify a machine by a node and vice versa.
// This functionality is traditionally (but not mandatory) a part of a
// cloud-provider implementation and it is what makes auto-scaling works.
package providerid

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	k8smetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/openshift/cluster-api-provider-kubevirt/pkg/clients/infracluster"
	"github.com/openshift/cluster-api-provider-kubevirt/pkg/clients/tenantcluster"
)

const IDFormat = "kubevirt://%s/%s"
const RETRY_INTERVAL_VM_NOTREADY = 60 * time.Second

var _ reconcile.Reconciler = &providerIDReconciler{}

type providerIDReconciler struct {
	client              client.Client
	infraClusterClient  infracluster.Client
	tenantClusterClient tenantcluster.Client
}

// Reconcile make sure a node has a ProviderID set. The providerID is the ID
// of the machine on kubevirt. The ID is the VM.metadata.namespace/VM.metadata.name
// as its guarantee to be unique in a cluster.
func (r *providerIDReconciler) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	klog.V(3).Info("Reconciling", "node", request.NamespacedName)

	// Fetch the Node instance
	node := corev1.Node{}
	err := r.client.Get(context.Background(), request.NamespacedName, &node)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, fmt.Errorf("error getting node: %v", err)
	}
	id, err := r.getVMName(node.Name)
	if id == "" {
		// Node doesn't exist in provider, deleting node object
		klog.Info(
			"Deleting Node from cluster since it has been removed in provider",
			"node", request.NamespacedName)
		return deleteNode(r.client, &node)
	}
	infraClusterNamespace, err := r.tenantClusterClient.GetNamespace()

	if node.Spec.ProviderID != "" {
		existingVM, err := r.infraClusterClient.GetVirtualMachine(context.Background(), infraClusterNamespace, id, &k8smetav1.GetOptions{})
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("failed getting VM: %v", err)
		}
		if !existingVM.Status.Ready {
			klog.Info("Node VM status is not ready, requeuing for 1 min",
				"Node", node.Name)
			return reconcile.Result{Requeue: true, RequeueAfter: RETRY_INTERVAL_VM_NOTREADY}, nil
		}
	} else {
		klog.Info("spec.ProviderID is empty, fetching from the infra-cluster", "node", request.NamespacedName)
		if err != nil {
			return reconcile.Result{}, err
		}

		if err != nil {
			return reconcile.Result{}, err
		}

		node.Spec.ProviderID = FormatProviderID(infraClusterNamespace, id)
		err = r.client.Update(context.Background(), &node)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("failed updating node %s: %v", node.Name, err)
		}
	}
	return reconcile.Result{}, nil
}

func deleteNode(client client.Client, node *corev1.Node) (reconcile.Result, error) {
	if err := client.Delete(context.Background(), node); err != nil {
		return reconcile.Result{}, fmt.Errorf("Error deleting node: %v, error is: %v", node.Name, err)
	}
	return reconcile.Result{}, nil
}

// FormatProviderID consumes the provider ID of the VM and returns
// a standard format to be used by machine and node reconcilers.
// See IDFormat
func FormatProviderID(namespace, name string) string {
	return fmt.Sprintf(IDFormat, namespace, name)
}

func (r *providerIDReconciler) getVMName(nodeName string) (string, error) {
	infraClusterNamespace, err := r.tenantClusterClient.GetNamespace()
	if err != nil {
		return "", err
	}
	vmi, err := r.infraClusterClient.GetVirtualMachineInstance(context.Background(), infraClusterNamespace, nodeName, &v1.GetOptions{})
	if err != nil {
		return "", err
	}
	return vmi.Name, nil
}

// Add registers a new provider ID reconciler controller with the controller manager
func Add(mgr manager.Manager, infraClusterClient infracluster.Client, tenantClusterClient tenantcluster.Client) error {
	reconciler, err := NewProviderIDReconciler(mgr, infraClusterClient, tenantClusterClient)

	if err != nil {
		return fmt.Errorf("error building reconciler: %v", err)
	}

	c, err := controller.New("provdierID-controller", mgr, controller.Options{Reconciler: reconciler})
	if err != nil {
		return err
	}

	//Watch node changes
	err = c.Watch(&source.Kind{Type: &corev1.Node{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	return nil
}

// NewProviderIDReconciler creates a new providerID reconciler
func NewProviderIDReconciler(mgr manager.Manager, infraClusterClient infracluster.Client, tenantClusterClient tenantcluster.Client) (*providerIDReconciler, error) {
	r := providerIDReconciler{
		client:              mgr.GetClient(),
		infraClusterClient:  infraClusterClient,
		tenantClusterClient: tenantClusterClient,
	}
	return &r, nil
}
