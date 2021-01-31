// nodeupdate package implements a controller to reconcile updates on the Node of the Machine:
// - Update providerID spec property on nodes in order to identify a machine by a node and vice versa.
// - In case the infrastructure machine (kubevirt VirtualMachine) was delete, delete its node
// - In case the infrastructure machine (kubevirt VirtualMachine) is not ready, requeue to re-check
// This functionality is traditionally (but not mandatory) a part of a
// cloud-provider implementation and it is what makes auto-scaling works.
package nodeupdate

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/openshift/cluster-api-provider-kubevirt/pkg/clients/infracluster"
	"github.com/openshift/cluster-api-provider-kubevirt/pkg/clients/tenantcluster"
	"github.com/openshift/cluster-api-provider-kubevirt/pkg/kubevirt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	configMapNamespace             = "openshift-config"
	configMapName                  = "cloud-provider-config"
	configMapDataKeyName           = "config"
	configMapInfraNamespaceKeyName = "namespace"
	configMapInfraIDKeyName        = "infraID"
	requeueDurationWhenVMNotReady  = 60 * time.Second
)

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
	klog.Infof("%s: Reconciling node", request.NamespacedName)

	// Fetch the Node instance
	node := corev1.Node{}
	err := r.client.Get(context.Background(), request.NamespacedName, &node)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			klog.Infof("%s: Node not found - do nothing", request.NamespacedName)
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, fmt.Errorf("error getting node: %v", err)
	}

	cMap, err := r.tenantClusterClient.GetConfigMapValue(context.Background(), configMapName, configMapNamespace, configMapDataKeyName)
	if err != nil {
		return reconcile.Result{}, err
	}

	infraClusterNamespace, ok := (*cMap)[configMapInfraNamespaceKeyName]
	if !ok {
		return reconcile.Result{}, fmt.Errorf("ProviderID: configMap %s/%s: The map extracted with key %s doesn't contain key %s",
			configMapNamespace, configMapName, configMapDataKeyName, configMapInfraNamespaceKeyName)
	}

	vm, err := r.infraClusterClient.GetVirtualMachine(context.Background(), infraClusterNamespace, node.Name, &metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			klog.Infof("%s: Virtual Machine of this node doesn't exists - delete the node", node.Name)
			if err := r.client.Delete(context.Background(), &node); err != nil {
				return reconcile.Result{}, fmt.Errorf("%s: Error deleting Node, with error: %v", node.Name, err)
			}
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("%s: Error getting Virtual Machine, with error: %v", node.Name, err)
	}

	if !vm.Status.Ready {
		klog.Infof("%s: Virtual Machine of this node isn't ready - requeue for 1 minute", node.Name)
		return reconcile.Result{Requeue: true, RequeueAfter: requeueDurationWhenVMNotReady}, nil
	}

	if node.Spec.ProviderID != "" {
		return reconcile.Result{}, nil
	}

	klog.Infof("%s: ProviderID is not updated in the node - update it", node.Name)

	node.Spec.ProviderID = kubevirt.FormatProviderID(infraClusterNamespace, node.Name)

	if err = r.client.Update(context.Background(), &node); err != nil {
		return reconcile.Result{}, fmt.Errorf("%s: failed updating node, with error: %v", node.Name, err)
	}

	return reconcile.Result{}, nil
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
