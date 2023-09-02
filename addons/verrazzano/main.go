/*
Copyright 2022 The Kubernetes Authors.

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

// This file from the cluster-api community (https://github.com/kubernetes-sigs/cluster-api) has been modified by Oracle.

package main

import (
	"flag"
	"os"
	"time"

	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/klogr"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	addonsv1alpha1 "github.com/verrazzano/cluster-api-provider-ocne/addons/verrazzano/api/v1alpha1"
	hcpController "github.com/verrazzano/cluster-api-provider-ocne/addons/verrazzano/controllers/verrazzanofleet"
	hrpController "github.com/verrazzano/cluster-api-provider-ocne/addons/verrazzano/controllers/verrazzanofleetbinding"
	"github.com/verrazzano/cluster-api-provider-ocne/version"

	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	kcpv1 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1beta1"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(addonsv1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var leaderElectionLeaseDuration time.Duration
	var leaderElectionRenewDeadline time.Duration
	var leaderElectionRetryPeriod time.Duration
	var probeAddr string
	var verrazzanoFleetConcurrency int
	var verrazzanoFleetBindingConcurrency int
	var syncPeriod time.Duration

	klog.InitFlags(nil)

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.DurationVar(&leaderElectionLeaseDuration, "leader-elect-lease-duration", 15*time.Second,
		"Interval at which non-leader candidates will wait to force acquire leadership (duration string)")
	flag.DurationVar(&leaderElectionRenewDeadline, "leader-elect-renew-deadline", 10*time.Second,
		"Duration that the leading controller manager will retry refreshing leadership before giving up (duration string)")
	flag.DurationVar(&leaderElectionRetryPeriod, "leader-elect-retry-period", 2*time.Second,
		"Duration the LeaderElector clients should wait between tries of actions (duration string)")
	flag.IntVar(&verrazzanoFleetConcurrency, "helm-chart-proxy-concurrency", 10, "The number of VerrazzanoFleets to process concurrently.")
	flag.IntVar(&verrazzanoFleetBindingConcurrency, "helm-release-proxy-concurrency", 10, "The number of VerrazzanoFleetBindings to process concurrently.")
	flag.DurationVar(&syncPeriod, "sync-period", 10*time.Minute,
		"The minimum interval at which watched resources are reconciled (e.g. 15m)")
	// Set log level 2 as default.
	if err := flag.Set("v", "2"); err != nil {
		setupLog.Error(err, "failed to set log level: %v")
		os.Exit(1)
	}
	flag.Parse()

	ctrl.SetLogger(klogr.NewWithOptions(klogr.WithFormat(klogr.FormatKlog)))
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "5a2dee3e.cluster.x-k8s.io",
		LeaseDuration:          &leaderElectionLeaseDuration,
		RenewDeadline:          &leaderElectionRenewDeadline,
		RetryPeriod:            &leaderElectionRetryPeriod,
		SyncPeriod:             &syncPeriod,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Any type we want the client to know about has to be added in the scheme.
	scheme := mgr.GetScheme()
	_ = clusterv1.AddToScheme(scheme)
	_ = kcpv1.AddToScheme(scheme)

	ctx := ctrl.SetupSignalHandler()

	if err = (&hcpController.VerrazzanoFleetReconciler{
		Client: mgr.GetClient(),
		Scheme: scheme,
	}).SetupWithManager(ctx, mgr, controller.Options{MaxConcurrentReconciles: verrazzanoFleetConcurrency}); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "VerrazzanoFleet")
		os.Exit(1)
	}
	if err = (&addonsv1alpha1.VerrazzanoFleet{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "VerrazzanoFleet")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if err = (&hrpController.VerrazzanoFleetBindingReconciler{
		Client: mgr.GetClient(),
		Scheme: scheme,
	}).SetupWithManager(ctx, mgr, controller.Options{MaxConcurrentReconciles: verrazzanoFleetBindingConcurrency}); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "VerrazzanoFleetBinding")
		os.Exit(1)
	}
	if err = (&addonsv1alpha1.VerrazzanoFleetBinding{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "VerrazzanoFleetBinding")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager", "version", version.Get().String())
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
