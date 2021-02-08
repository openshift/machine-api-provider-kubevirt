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

package main

import (
	"context"
	"flag"
	"time"

	"github.com/openshift/cluster-api-provider-kubevirt/pkg/actuator"
	"github.com/openshift/cluster-api-provider-kubevirt/pkg/clients/infracluster"
	"github.com/openshift/cluster-api-provider-kubevirt/pkg/clients/tenantcluster"
	"github.com/openshift/cluster-api-provider-kubevirt/pkg/controller/nodeupdate"
	"github.com/openshift/cluster-api-provider-kubevirt/pkg/kubevirt"
	"github.com/openshift/cluster-api-provider-kubevirt/pkg/machinescope"
	mapiv1beta1 "github.com/openshift/machine-api-operator/pkg/apis/machine/v1beta1"
	"github.com/openshift/machine-api-operator/pkg/controller/machine"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	ctrl "sigs.k8s.io/controller-runtime/pkg/manager/signals"
)

// The default durations for the leader election operations.
var (
	leaseDuration = 120 * time.Second
	renewDeadline = 110 * time.Second
	retryPeriod   = 20 * time.Second
	syncPeriod    = 10 * time.Minute
)

func main() {
	watchNamespace := flag.String(
		"namespace",
		"",
		"Namespace that the controller watches to reconcile machine-api objects. If unspecified, the controller watches for machine-api objects across all namespaces.",
	)

	metricsAddr := flag.String(
		"metrics-addr",
		":8081",
		"The address the metric endpoint binds to.",
	)

	healthAddr := flag.String(
		"health-addr",
		":9440",
		"The address for health checking.",
	)

	leaderElectResourceNamespace := flag.String(
		"leader-elect-resource-namespace",
		"",
		"The namespace of resource object that is used for locking during leader election. If unspecified and running in cluster, defaults to the service account namespace for the controller. Required for leader-election outside of a cluster.",
	)

	leaderElect := flag.Bool(
		"leader-elect",
		false,
		"Start a leader election client and gain leadership before executing the main loop. Enable this when running replicated components for high availability.",
	)

	leaderElectLeaseDuration := flag.Duration(
		"leader-elect-lease-duration",
		leaseDuration,
		"The duration that non-leader candidates will wait after observing a leadership renewal until attempting to acquire leadership of a led but unrenewed leader slot. This is effectively the maximum duration that a leader can be stopped before it is replaced by another candidate. This is only applicable if leader election is enabled.",
	)

	klog.InitFlags(nil)
	flag.Set("logtostderr", "true")
	flag.Parse()

	klog.Info("start kubevirt machine controller")

	// Get a config to talk to the apiserver
	cfg, err := config.GetConfig()
	if err != nil {
		klog.Fatalf("Error getting configuration: %v", err)
	}

	// Setup a Manager
	opts := manager.Options{
		LeaderElection:          *leaderElect,
		LeaderElectionNamespace: *leaderElectResourceNamespace,
		LeaderElectionID:        "cluster-api-provider-kubevirt-leader",
		LeaseDuration:           leaderElectLeaseDuration,
		MetricsBindAddress:      *metricsAddr,
		HealthProbeBindAddress:  *healthAddr,
		RetryPeriod:             &retryPeriod,
		RenewDeadline:           &renewDeadline,
	}

	if *watchNamespace != "" {
		opts.Namespace = *watchNamespace
		klog.Infof("Watching machine-api objects only in namespace %q for reconciliation.", opts.Namespace)
	}

	mgr, err := manager.New(cfg, opts)
	if err != nil {
		klog.Fatalf("failed to set up overall controller manager, with error: %v", err)
	}

	// Setup Scheme for all resources
	if err := mapiv1beta1.AddToScheme(mgr.GetScheme()); err != nil {
		klog.Fatalf("failed to set up scheme, with error: %v", err)
	}

	// Initialize tenant-cluster clients
	tenantClusterClient, err := tenantcluster.New(mgr)
	if err != nil {
		klog.Fatalf("failed to create tenantcluster client from configuration, with error: %v", err)
	}

	// Initialize infra-cluster clients
	infraClusterClient, err := infracluster.New(context.Background(), tenantClusterClient)
	if err != nil {
		klog.Fatalf("failed to create infracluster client from configuration, with error: %v", err)
	}

	// Initialize machineScope creator
	machineScopeCreator := machinescope.New()

	// Initialize provider vm manager (infraClusterClientBuilder would be the function infracluster.New)
	kubevirtVM := kubevirt.New(infraClusterClient)

	// Initialize machine actuator.
	machineActuator, err := actuator.New(kubevirtVM, mgr.GetEventRecorderFor("kubevirtcontroller"),
		machineScopeCreator, tenantClusterClient)
	if err != nil {
		klog.Fatalf("failed to create actuator, with error: %v", err)
	}

	// Register Actuator on machine-controller
	if err := machine.AddWithActuator(mgr, machineActuator); err != nil {
		klog.Fatalf("failed to add actuator, with error: %v", err)
	}

	// Register the providerID controller
	if err := nodeupdate.Add(mgr, infraClusterClient, tenantClusterClient); err != nil {
		klog.Fatalf("failed to add providerID reconciler, with error: %v", err)

	}

	if err := mgr.AddReadyzCheck("ping", healthz.Ping); err != nil {
		klog.Fatalf("failed to add ReadyzCheck, with error: %v", err)
	}

	if err := mgr.AddHealthzCheck("ping", healthz.Ping); err != nil {
		klog.Fatalf("failed to add HealthzCheck, with error: %v", err)
	}

	// Start the Cmd
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		klog.Fatalf("failed to start manager, with error: %v", err)
	}
}
