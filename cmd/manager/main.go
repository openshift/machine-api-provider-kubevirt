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
	"flag"
	"os"
	"time"

	"github.com/openshift/cluster-api-provider-kubevirt/pkg/actuator"
	"github.com/openshift/cluster-api-provider-kubevirt/pkg/clients/infracluster"
	"github.com/openshift/cluster-api-provider-kubevirt/pkg/clients/tenantcluster"
	"github.com/openshift/cluster-api-provider-kubevirt/pkg/managers/vm"
	"github.com/openshift/cluster-api-provider-kubevirt/pkg/providerid"
	mapiv1beta1 "github.com/openshift/machine-api-operator/pkg/apis/machine/v1beta1"
	"github.com/openshift/machine-api-operator/pkg/controller/machine"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/klog"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	ctrl "sigs.k8s.io/controller-runtime/pkg/manager/signals"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

// The default durations for the leader election operations.
var (
	leaseDuration = 120 * time.Second
	renewDeadline = 110 * time.Second
	retryPeriod   = 20 * time.Second
)

func main() {
	var printVersion bool
	flag.BoolVar(&printVersion, "version", false, "print version and exit")

	// TODO Add relevant flags as written in klog initFlags
	//klog.InitFlags(nil)

	watchNamespace := flag.String(
		"namespace",
		"",
		"Namespace that the controller watches to reconcile machine-api objects. If unspecified, the controller watches for machine-api objects across all namespaces.",
	)

	// metricsAddr := flag.String(
	// 	"metrics-addr",
	// 	":8081",
	// 	"The address the metric endpoint binds to.",
	// )

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

	// TODO Remove this flag when stable
	flag.Set("logtostderr", "true")

	flag.Parse()

	log := logf.Log.WithName("infracluster-controller-manager")
	logf.SetLogger(logf.ZapLogger(false))
	entryLog := log.WithName("entrypoint")

	// Get a config to talk to the apiserver
	cfg, err := config.GetConfig()
	if err != nil {
		klog.Fatalf("Error getting configuration: %v", err)
	}

	// Setup a Manager
	opts := manager.Options{
		LeaderElection:          *leaderElect,
		LeaderElectionNamespace: *leaderElectResourceNamespace,
		LeaderElectionID:        "cluster-api-provider-ovirt-leader",
		LeaseDuration:           leaderElectLeaseDuration,
		// Disable metrics serving
		MetricsBindAddress:     "0", // *metricsAddr,
		HealthProbeBindAddress: *healthAddr,
		// Slow the default retry and renew election rate to reduce etcd writes at idle: BZ 1858400
		RetryPeriod:   &retryPeriod,
		RenewDeadline: &renewDeadline,
	}

	if *watchNamespace != "" {
		opts.Namespace = *watchNamespace
		klog.Infof("Watching machine-api objects only in namespace %q for reconciliation.", opts.Namespace)
	}

	mgr, err := manager.New(cfg, opts)
	if err != nil {
		entryLog.Error(err, "Unable to set up overall controller manager")
		os.Exit(1)
	}

	// Setup Scheme for all resources
	if err := mapiv1beta1.AddToScheme(mgr.GetScheme()); err != nil {
		klog.Fatalf("Error setting up scheme: %v", err)
	}

	// Initialize tenant-cluster clients
	kubernetesClient, err := tenantcluster.New(mgr)
	if err != nil {
		entryLog.Error(err, "Failed to create tenantcluster client from configuration")
	}

	// Initialize provider vm manager (infraClusterClientBuilder would be the function infracluster.New)
	providerVM := vm.New(infracluster.New, kubernetesClient)

	// Initialize machine actuator.
	machineActuator := actuator.New(providerVM, mgr.GetEventRecorderFor("kubevirtcontroller"))

	// Register Actuator on machine-controller
	if err := machine.AddWithActuator(mgr, machineActuator); err != nil {
		klog.Fatalf("Error adding actuator: %v", err)
	}

	// Register the providerID controller
	providerid.Add(mgr, manager.Options{})

	if err := mgr.AddReadyzCheck("ping", healthz.Ping); err != nil {
		klog.Fatal(err)
	}

	if err := mgr.AddHealthzCheck("ping", healthz.Ping); err != nil {
		klog.Fatal(err)
	}

	// Start the Cmd
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		klog.Fatalf("Error starting manager: %v", err)
	}
}
