/*
Copyright 2019 The Kubernetes Authors.

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

package controllers

import (
	"context"
	"fmt"
	"github.com/verrazzano/cluster-api-provider-ocne/internal/util/ocne"
	"time"

	"github.com/blang/semver"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	bootstrapv1 "github.com/verrazzano/cluster-api-provider-ocne/bootstrap/ocne/api/v1alpha1"
	controlplanev1 "github.com/verrazzano/cluster-api-provider-ocne/controlplane/ocne/api/v1alpha1"
	"github.com/verrazzano/cluster-api-provider-ocne/controlplane/ocne/internal"
	"github.com/verrazzano/cluster-api-provider-ocne/feature"
	"github.com/verrazzano/cluster-api-provider-ocne/internal/labels"
	"github.com/verrazzano/cluster-api-provider-ocne/util"
	"github.com/verrazzano/cluster-api-provider-ocne/util/annotations"
	"github.com/verrazzano/cluster-api-provider-ocne/util/collections"
	"github.com/verrazzano/cluster-api-provider-ocne/util/conditions"
	"github.com/verrazzano/cluster-api-provider-ocne/util/patch"
	"github.com/verrazzano/cluster-api-provider-ocne/util/predicates"
	"github.com/verrazzano/cluster-api-provider-ocne/util/secret"
	"github.com/verrazzano/cluster-api-provider-ocne/util/version"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/remote"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
)

// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io;bootstrap.cluster.x-k8s.io;controlplane.cluster.x-k8s.io,resources=*,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines;machines/status,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch

// OCNEControlPlaneReconciler reconciles a OCNEControlPlane object.
type OCNEControlPlaneReconciler struct {
	Client          client.Client
	APIReader       client.Reader
	controller      controller.Controller
	recorder        record.EventRecorder
	Tracker         *remote.ClusterCacheTracker
	EtcdDialTimeout time.Duration

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string

	managementCluster         internal.ManagementCluster
	managementClusterUncached internal.ManagementCluster
}

func (r *OCNEControlPlaneReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	c, err := ctrl.NewControllerManagedBy(mgr).
		For(&controlplanev1.OCNEControlPlane{}).
		Owns(&clusterv1.Machine{}).
		WithOptions(options).
		WithEventFilter(predicates.ResourceNotPausedAndHasFilterLabel(ctrl.LoggerFrom(ctx), r.WatchFilterValue)).
		Build(r)
	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}

	err = c.Watch(
		&source.Kind{Type: &clusterv1.Cluster{}},
		handler.EnqueueRequestsFromMapFunc(r.ClusterToKubeadmControlPlane),
		predicates.All(ctrl.LoggerFrom(ctx),
			predicates.ResourceHasFilterLabel(ctrl.LoggerFrom(ctx), r.WatchFilterValue),
			predicates.ClusterUnpausedAndInfrastructureReady(ctrl.LoggerFrom(ctx)),
		),
	)
	if err != nil {
		return errors.Wrap(err, "failed adding Watch for Clusters to controller manager")
	}

	r.controller = c
	r.recorder = mgr.GetEventRecorderFor("ocne-control-plane-controller")

	if r.managementCluster == nil {
		if r.Tracker == nil {
			return errors.New("cluster cache tracker is nil, cannot create the internal management cluster resource")
		}
		r.managementCluster = &internal.Management{
			Client:          r.Client,
			Tracker:         r.Tracker,
			EtcdDialTimeout: r.EtcdDialTimeout,
		}
	}

	if r.managementClusterUncached == nil {
		r.managementClusterUncached = &internal.Management{Client: mgr.GetAPIReader()}
	}

	return nil
}

func (r *OCNEControlPlaneReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, reterr error) {
	log := ctrl.LoggerFrom(ctx)

	// Fetch the OCNEControlPlane instance.
	ocnecp := &controlplanev1.OCNEControlPlane{}
	if err := r.Client.Get(ctx, req.NamespacedName, ocnecp); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Fetch the Cluster.
	cluster, err := util.GetOwnerCluster(ctx, r.Client, ocnecp.ObjectMeta)
	if err != nil {
		log.Error(err, "Failed to retrieve owner Cluster from the API Server")
		return ctrl.Result{}, err
	}

	if cluster == nil {
		log.Info("Cluster Controller has not yet set OwnerRef")
		return ctrl.Result{}, nil
	}
	log = log.WithValues("Cluster", klog.KObj(cluster))
	ctx = ctrl.LoggerInto(ctx, log)

	if annotations.IsPaused(cluster, ocnecp) {
		log.Info("Reconciliation is paused for this object")
		return ctrl.Result{}, nil
	}

	// Initialize the patch helper.
	patchHelper, err := patch.NewHelper(ocnecp, r.Client)
	if err != nil {
		log.Error(err, "Failed to configure the patch helper")
		return ctrl.Result{Requeue: true}, nil
	}

	// Add finalizer first if not exist to avoid the race condition between init and delete
	if !controllerutil.ContainsFinalizer(ocnecp, controlplanev1.OCNEControlPlaneFinalizer) {
		controllerutil.AddFinalizer(ocnecp, controlplanev1.OCNEControlPlaneFinalizer)

		// patch and return right away instead of reusing the main defer,
		// because the main defer may take too much time to get cluster status
		// Patch ObservedGeneration only if the reconciliation completed successfully
		patchOpts := []patch.Option{patch.WithStatusObservedGeneration{}}
		if err := patchHelper.Patch(ctx, ocnecp, patchOpts...); err != nil {
			log.Error(err, "Failed to patch OCNEControlPlane to add finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	defer func() {
		// Always attempt to update status.
		if err := r.updateStatus(ctx, ocnecp, cluster); err != nil {
			var connFailure *internal.RemoteClusterConnectionError
			if errors.As(err, &connFailure) {
				log.Info("Could not connect to workload cluster to fetch status", "err", err.Error())
			} else {
				log.Error(err, "Failed to update OCNEControlPlane Status")
				reterr = kerrors.NewAggregate([]error{reterr, err})
			}
		}

		//value, _ := os.LookupEnv("DEV")
		//if value != "true" {
		//	if err := setOCNEControlPlaneDefaults(ctx, ocnecp); err != nil {
		//		log.Error(err, "Failed to set defaults for OCNEControlPlane")
		//		reterr = kerrors.NewAggregate([]error{reterr, err})
		//	}
		//}

		if err := setOCNEControlPlaneDefaults(ctx, ocnecp); err != nil {
			log.Error(err, "Failed to set defaults for OCNEControlPlane")
			reterr = kerrors.NewAggregate([]error{reterr, err})
		}

		// Always attempt to Patch the OCNEControlPlane object and status after each reconciliation.
		if err := patchOCNEControlPlane(ctx, patchHelper, ocnecp); err != nil {
			log.Error(err, "Failed to patch OCNEControlPlane")
			reterr = kerrors.NewAggregate([]error{reterr, err})
		}

		// TODO: remove this as soon as we have a proper remote cluster cache in place.
		// Make KCP to requeue in case status is not ready, so we can check for node status without waiting for a full resync (by default 10 minutes).
		// Only requeue if we are not going in exponential backoff due to error, or if we are not already re-queueing, or if the object has a deletion timestamp.
		if reterr == nil && !res.Requeue && res.RequeueAfter <= 0 && ocnecp.ObjectMeta.DeletionTimestamp.IsZero() {
			if !ocnecp.Status.Ready {
				res = ctrl.Result{RequeueAfter: 20 * time.Second}
			}
		}
	}()

	if !ocnecp.ObjectMeta.DeletionTimestamp.IsZero() {
		// Handle deletion reconciliation loop.
		res, err = r.reconcileDelete(ctx, cluster, ocnecp)
		// Requeue if the reconcile failed because the ClusterCacheTracker was locked for
		// the current cluster because of concurrent access.
		if errors.Is(err, remote.ErrClusterLocked) {
			log.V(5).Info("Requeueing because another worker has the lock on the ClusterCacheTracker")
			return ctrl.Result{Requeue: true}, nil
		}
		return res, err
	}

	// Handle normal reconciliation loop.
	res, err = r.reconcile(ctx, cluster, ocnecp)
	// Requeue if the reconcile failed because the ClusterCacheTracker was locked for
	// the current cluster because of concurrent access.
	if errors.Is(err, remote.ErrClusterLocked) {
		log.V(5).Info("Requeueing because another worker has the lock on the ClusterCacheTracker")
		return ctrl.Result{Requeue: true}, nil
	}
	return res, err
}

func patchOCNEControlPlane(ctx context.Context, patchHelper *patch.Helper, ocnecp *controlplanev1.OCNEControlPlane) error {
	// Always update the readyCondition by summarizing the state of other conditions.
	conditions.SetSummary(ocnecp,
		conditions.WithConditions(
			controlplanev1.MachinesCreatedCondition,
			controlplanev1.MachinesSpecUpToDateCondition,
			controlplanev1.ResizedCondition,
			controlplanev1.MachinesReadyCondition,
			controlplanev1.AvailableCondition,
			controlplanev1.CertificatesAvailableCondition,
		),
	)

	// Patch the object, ignoring conflicts on the conditions owned by this controller.
	return patchHelper.Patch(
		ctx,
		ocnecp,
		patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
			controlplanev1.MachinesCreatedCondition,
			clusterv1.ReadyCondition,
			controlplanev1.MachinesSpecUpToDateCondition,
			controlplanev1.ResizedCondition,
			controlplanev1.MachinesReadyCondition,
			controlplanev1.AvailableCondition,
			controlplanev1.CertificatesAvailableCondition,
		}},
		patch.WithStatusObservedGeneration{},
	)
}

func setOCNEControlPlaneDefaults(ctx context.Context, ocnecp *controlplanev1.OCNEControlPlane) error {
	ocneMeta, err := ocne.GetOCNEMetadata(ctx)
	if err != nil {
		return err
	}

	if ocnecp.Spec.ControlPlaneConfig.ClusterConfiguration != nil {
		if ocnecp.Spec.ControlPlaneConfig.ClusterConfiguration.DNS.ImageTag == "" {
			ocnecp.Spec.ControlPlaneConfig.ClusterConfiguration.DNS.ImageTag = ocneMeta[ocnecp.Spec.Version].CoreDNS
		}
		if ocnecp.Spec.ControlPlaneConfig.ClusterConfiguration.DNS.ImageRepository == "" {
			ocnecp.Spec.ControlPlaneConfig.ClusterConfiguration.DNS.ImageRepository = ocne.DefaultOCNEImageRepository
		}

		etcdLocal := bootstrapv1.LocalEtcd{
			ImageMeta: bootstrapv1.ImageMeta{
				ImageTag:        ocneMeta[ocnecp.Spec.Version].ETCD,
				ImageRepository: ocne.DefaultOCNEImageRepository,
			},
		}

		if ocnecp.Spec.ControlPlaneConfig.ClusterConfiguration.Etcd.Local == nil {
			ocnecp.Spec.ControlPlaneConfig.ClusterConfiguration.Etcd.Local = &etcdLocal
		}

		if ocnecp.Spec.ControlPlaneConfig.ClusterConfiguration.ImageRepository == "" {
			ocnecp.Spec.ControlPlaneConfig.ClusterConfiguration.ImageRepository = ocne.DefaultOCNEImageRepository
		}

		if ocnecp.Spec.ControlPlaneConfig.JoinConfiguration.NodeRegistration.CRISocket == "" {
			ocnecp.Spec.ControlPlaneConfig.JoinConfiguration.NodeRegistration.CRISocket = ocne.DefaultOCNESocket
		}

		if ocnecp.Spec.ControlPlaneConfig.InitConfiguration.NodeRegistration.CRISocket == "" {
			ocnecp.Spec.ControlPlaneConfig.InitConfiguration.NodeRegistration.CRISocket = ocne.DefaultOCNESocket
		}
	}

	return nil
}

// reconcile handles OCNEControlPlane reconciliation.
func (r *OCNEControlPlaneReconciler) reconcile(ctx context.Context, cluster *clusterv1.Cluster, ocnecp *controlplanev1.OCNEControlPlane) (res ctrl.Result, reterr error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Reconcile OCNEControlPlane")

	// Make sure to reconcile the external infrastructure reference.
	if err := r.reconcileExternalReference(ctx, cluster, &ocnecp.Spec.MachineTemplate.InfrastructureRef); err != nil {
		return ctrl.Result{}, err
	}

	// Wait for the cluster infrastructure to be ready before creating machines
	if !cluster.Status.InfrastructureReady {
		log.Info("Cluster infrastructure is not ready yet")
		return ctrl.Result{}, nil
	}

	// Generate Cluster Certificates if needed
	config := ocnecp.Spec.ControlPlaneConfig.DeepCopy()
	config.JoinConfiguration = nil
	if config.ClusterConfiguration == nil {
		config.ClusterConfiguration = &bootstrapv1.ClusterConfiguration{}
	}
	certificates := secret.NewCertificatesForInitialControlPlane(config.ClusterConfiguration)
	controllerRef := metav1.NewControllerRef(ocnecp, controlplanev1.GroupVersion.WithKind("OCNEControlPlane"))
	if err := certificates.LookupOrGenerate(ctx, r.Client, util.ObjectKey(cluster), *controllerRef); err != nil {
		log.Error(err, "unable to lookup or create cluster certificates")
		conditions.MarkFalse(ocnecp, controlplanev1.CertificatesAvailableCondition, controlplanev1.CertificatesGenerationFailedReason, clusterv1.ConditionSeverityWarning, err.Error())
		return ctrl.Result{}, err
	}
	conditions.MarkTrue(ocnecp, controlplanev1.CertificatesAvailableCondition)

	// If ControlPlaneEndpoint is not set, return early
	if !cluster.Spec.ControlPlaneEndpoint.IsValid() {
		log.Info("Cluster does not yet have a ControlPlaneEndpoint defined")
		return ctrl.Result{}, nil
	}

	// Generate Cluster Kubeconfig if needed
	if result, err := r.reconcileKubeconfig(ctx, cluster, ocnecp); !result.IsZero() || err != nil {
		if err != nil {
			log.Error(err, "failed to reconcile Kubeconfig")
		}
		return result, err
	}

	controlPlaneMachines, err := r.managementClusterUncached.GetMachinesForCluster(ctx, cluster, collections.ControlPlaneMachines(cluster.Name))
	if err != nil {
		log.Error(err, "failed to retrieve control plane machines for cluster")
		return ctrl.Result{}, err
	}

	adoptableMachines := controlPlaneMachines.Filter(collections.AdoptableControlPlaneMachines(cluster.Name))
	if len(adoptableMachines) > 0 {
		// We adopt the Machines and then wait for the update event for the ownership reference to re-queue them so the cache is up-to-date
		err = r.adoptMachines(ctx, ocnecp, adoptableMachines, cluster)
		return ctrl.Result{}, err
	}
	if err := ensureCertificatesOwnerRef(ctx, r.Client, util.ObjectKey(cluster), certificates, *controllerRef); err != nil {
		return ctrl.Result{}, err
	}

	ownedMachines := controlPlaneMachines.Filter(collections.OwnedMachines(ocnecp))
	if len(ownedMachines) != len(controlPlaneMachines) {
		log.Info("Not all control plane machines are owned by this OCNEControlPlane, refusing to operate in mixed management mode")
		return ctrl.Result{}, nil
	}

	controlPlane, err := internal.NewControlPlane(ctx, r.Client, cluster, ocnecp, ownedMachines)
	if err != nil {
		log.Error(err, "failed to initialize control plane")
		return ctrl.Result{}, err
	}

	// Aggregate the operational state of all the machines; while aggregating we are adding the
	// source ref (reason@machine/name) so the problem can be easily tracked down to its source machine.
	conditions.SetAggregate(controlPlane.KCP, controlplanev1.MachinesReadyCondition, ownedMachines.ConditionGetters(), conditions.AddSourceRef(), conditions.WithStepCounterIf(false))

	// Ensure all required labels exist on the controlled Machines.
	// This logic is needed to add the `cluster.x-k8s.io/control-plane-name` label to Machines
	// which were created before the `cluster.x-k8s.io/control-plane-name` label was introduced
	// or if a user manually removed the label.
	// NOTE: Changes will be applied to the Machines in reconcileControlPlaneConditions.
	// NOTE: cluster.x-k8s.io/control-plane is already set at this stage (it is used when reading controlPlane.Machines).
	for i := range controlPlane.Machines {
		machine := controlPlane.Machines[i]
		// Note: MustEqualValue and MustFormatValue is used here as the label value can be a hash if the control plane
		// name is longer than 63 characters.
		if value, ok := machine.Labels[clusterv1.MachineControlPlaneNameLabel]; !ok || !labels.MustEqualValue(ocnecp.Name, value) {
			machine.Labels[clusterv1.MachineControlPlaneNameLabel] = labels.MustFormatValue(ocnecp.Name)
		}
	}

	// Updates conditions reporting the status of static pods and the status of the etcd cluster.
	// NOTE: Conditions reporting KCP operation progress like e.g. Resized or SpecUpToDate are inlined with the rest of the execution.
	if result, err := r.reconcileControlPlaneConditions(ctx, controlPlane); err != nil || !result.IsZero() {
		return result, err
	}

	// Ensures the number of etcd members is in sync with the number of machines/nodes.
	// NOTE: This is usually required after a machine deletion.
	if result, err := r.reconcileEtcdMembers(ctx, controlPlane); err != nil || !result.IsZero() {
		return result, err
	}

	// Reconcile unhealthy machines by triggering deletion and requeue if it is considered safe to remediate,
	// otherwise continue with the other KCP operations.
	if result, err := r.reconcileUnhealthyMachines(ctx, controlPlane); err != nil || !result.IsZero() {
		return result, err
	}

	// Reconcile certificate expiry for machines that don't have the expiry annotation on OCNEConfig yet.
	if result, err := r.reconcileCertificateExpiries(ctx, controlPlane); err != nil || !result.IsZero() {
		return result, err
	}

	// Control plane machines rollout due to configuration changes (e.g. upgrades) takes precedence over other operations.
	needRollout := controlPlane.MachinesNeedingRollout()
	switch {
	case len(needRollout) > 0:
		log.Info("Rolling out Control Plane machines", "needRollout", needRollout.Names())
		conditions.MarkFalse(controlPlane.KCP, controlplanev1.MachinesSpecUpToDateCondition, controlplanev1.RollingUpdateInProgressReason, clusterv1.ConditionSeverityWarning, "Rolling %d replicas with outdated spec (%d replicas up to date)", len(needRollout), len(controlPlane.Machines)-len(needRollout))
		return r.upgradeControlPlane(ctx, cluster, ocnecp, controlPlane, needRollout)
	default:
		// make sure last upgrade operation is marked as completed.
		// NOTE: we are checking the condition already exists in order to avoid to set this condition at the first
		// reconciliation/before a rolling upgrade actually starts.
		if conditions.Has(controlPlane.KCP, controlplanev1.MachinesSpecUpToDateCondition) {
			conditions.MarkTrue(controlPlane.KCP, controlplanev1.MachinesSpecUpToDateCondition)
		}
	}

	// If we've made it this far, we can assume that all ownedMachines are up to date
	numMachines := len(ownedMachines)
	desiredReplicas := int(*ocnecp.Spec.Replicas)

	switch {
	// We are creating the first replica
	case numMachines < desiredReplicas && numMachines == 0:
		// Create new Machine w/ init
		log.Info("Initializing control plane", "Desired", desiredReplicas, "Existing", numMachines)
		conditions.MarkFalse(controlPlane.KCP, controlplanev1.AvailableCondition, controlplanev1.WaitingForOCNEInitReason, clusterv1.ConditionSeverityInfo, "")
		return r.initializeControlPlane(ctx, cluster, ocnecp, controlPlane)
	// We are scaling up
	case numMachines < desiredReplicas && numMachines > 0:
		// Create a new Machine w/ join
		log.Info("Scaling up control plane", "Desired", desiredReplicas, "Existing", numMachines)
		return r.scaleUpControlPlane(ctx, cluster, ocnecp, controlPlane)
	// We are scaling down
	case numMachines > desiredReplicas:
		log.Info("Scaling down control plane", "Desired", desiredReplicas, "Existing", numMachines)
		// The last parameter (i.e. machines needing to be rolled out) should always be empty here.
		return r.scaleDownControlPlane(ctx, cluster, ocnecp, controlPlane, collections.Machines{})
	}

	// Get the workload cluster client.
	workloadCluster, err := r.managementCluster.GetWorkloadCluster(ctx, util.ObjectKey(cluster))
	if err != nil {
		log.V(2).Info("cannot get remote client to workload cluster, will requeue", "cause", err)
		return ctrl.Result{Requeue: true}, nil
	}

	// Ensure kubeadm role bindings for v1.18+
	if err := workloadCluster.AllowBootstrapTokensToGetNodes(ctx); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to set role and role binding for kubeadm")
	}

	// We intentionally only parse major/minor/patch so that the subsequent code
	// also already applies to beta versions of new releases.
	parsedVersion, err := version.ParseMajorMinorPatchTolerant(ocnecp.Spec.Version)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to parse kubernetes version %q", ocnecp.Spec.Version)
	}

	// Update kube-proxy daemonset.
	if err := workloadCluster.UpdateKubeProxyImageInfo(ctx, ocnecp, parsedVersion); err != nil {
		log.Error(err, "failed to update kube-proxy daemonset")
		return ctrl.Result{}, err
	}

	// Update CoreDNS deployment.
	if err := workloadCluster.UpdateCoreDNS(ctx, ocnecp, parsedVersion); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to update CoreDNS deployment")
	}

	return ctrl.Result{}, nil
}

// reconcileDelete handles OCNEControlPlane deletion.
// The implementation does not take non-control plane workloads into consideration. This may or may not change in the future.
// Please see https://github.com/kubernetes-sigs/cluster-api/issues/2064.
func (r *OCNEControlPlaneReconciler) reconcileDelete(ctx context.Context, cluster *clusterv1.Cluster, ocnecp *controlplanev1.OCNEControlPlane) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Reconcile OCNEControlPlane deletion")

	// Gets all machines, not just control plane machines.
	allMachines, err := r.managementCluster.GetMachinesForCluster(ctx, cluster)
	if err != nil {
		return ctrl.Result{}, err
	}
	ownedMachines := allMachines.Filter(collections.OwnedMachines(ocnecp))

	// If no control plane machines remain, remove the finalizer
	if len(ownedMachines) == 0 {
		controllerutil.RemoveFinalizer(ocnecp, controlplanev1.OCNEControlPlaneFinalizer)
		return ctrl.Result{}, nil
	}

	controlPlane, err := internal.NewControlPlane(ctx, r.Client, cluster, ocnecp, ownedMachines)
	if err != nil {
		log.Error(err, "failed to initialize control plane")
		return ctrl.Result{}, err
	}

	// Updates conditions reporting the status of static pods and the status of the etcd cluster.
	// NOTE: Ignoring failures given that we are deleting
	if _, err := r.reconcileControlPlaneConditions(ctx, controlPlane); err != nil {
		log.Info("failed to reconcile conditions", "error", err.Error())
	}

	// Aggregate the operational state of all the machines; while aggregating we are adding the
	// source ref (reason@machine/name) so the problem can be easily tracked down to its source machine.
	// However, during delete we are hiding the counter (1 of x) because it does not make sense given that
	// all the machines are deleted in parallel.
	conditions.SetAggregate(ocnecp, controlplanev1.MachinesReadyCondition, ownedMachines.ConditionGetters(), conditions.AddSourceRef(), conditions.WithStepCounterIf(false))

	allMachinePools := &expv1.MachinePoolList{}
	// Get all machine pools.
	if feature.Gates.Enabled(feature.MachinePool) {
		allMachinePools, err = r.managementCluster.GetMachinePoolsForCluster(ctx, cluster)
		if err != nil {
			return ctrl.Result{}, err
		}
	}
	// Verify that only control plane machines remain
	if len(allMachines) != len(ownedMachines) || len(allMachinePools.Items) != 0 {
		log.Info("Waiting for worker nodes to be deleted first")
		conditions.MarkFalse(ocnecp, controlplanev1.ResizedCondition, clusterv1.DeletingReason, clusterv1.ConditionSeverityInfo, "Waiting for worker nodes to be deleted first")
		return ctrl.Result{RequeueAfter: deleteRequeueAfter}, nil
	}

	// Delete control plane machines in parallel
	machinesToDelete := ownedMachines.Filter(collections.Not(collections.HasDeletionTimestamp))
	var errs []error
	for i := range machinesToDelete {
		m := machinesToDelete[i]
		logger := log.WithValues("Machine", klog.KObj(m))
		if err := r.Client.Delete(ctx, machinesToDelete[i]); err != nil && !apierrors.IsNotFound(err) {
			logger.Error(err, "Failed to cleanup owned machine")
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		err := kerrors.NewAggregate(errs)
		r.recorder.Eventf(ocnecp, corev1.EventTypeWarning, "FailedDelete",
			"Failed to delete control plane Machines for cluster %s/%s control plane: %v", cluster.Namespace, cluster.Name, err)
		return ctrl.Result{}, err
	}
	conditions.MarkFalse(ocnecp, controlplanev1.ResizedCondition, clusterv1.DeletingReason, clusterv1.ConditionSeverityInfo, "")
	return ctrl.Result{RequeueAfter: deleteRequeueAfter}, nil
}

// ClusterToKubeadmControlPlane is a handler.ToRequestsFunc to be used to enqueue requests for reconciliation
// for OCNEControlPlane based on updates to a Cluster.
func (r *OCNEControlPlaneReconciler) ClusterToKubeadmControlPlane(o client.Object) []ctrl.Request {
	c, ok := o.(*clusterv1.Cluster)
	if !ok {
		panic(fmt.Sprintf("Expected a Cluster but got a %T", o))
	}

	controlPlaneRef := c.Spec.ControlPlaneRef
	if controlPlaneRef != nil && controlPlaneRef.Kind == "OCNEControlPlane" {
		return []ctrl.Request{{NamespacedName: client.ObjectKey{Namespace: controlPlaneRef.Namespace, Name: controlPlaneRef.Name}}}
	}

	return nil
}

// reconcileControlPlaneConditions is responsible of reconciling conditions reporting the status of static pods and
// the status of the etcd cluster.
func (r *OCNEControlPlaneReconciler) reconcileControlPlaneConditions(ctx context.Context, controlPlane *internal.ControlPlane) (ctrl.Result, error) {
	// If the cluster is not yet initialized, there is no way to connect to the workload cluster and fetch information
	// for updating conditions. Return early.
	if !controlPlane.KCP.Status.Initialized {
		return ctrl.Result{}, nil
	}

	workloadCluster, err := r.managementCluster.GetWorkloadCluster(ctx, util.ObjectKey(controlPlane.Cluster))
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "cannot get remote client to workload cluster")
	}

	// Update conditions status
	workloadCluster.UpdateStaticPodConditions(ctx, controlPlane)
	workloadCluster.UpdateEtcdConditions(ctx, controlPlane)

	// Patch machines with the updated conditions.
	if err := controlPlane.PatchMachines(ctx); err != nil {
		return ctrl.Result{}, err
	}

	// KCP will be patched at the end of Reconcile to reflect updated conditions, so we can return now.
	return ctrl.Result{}, nil
}

// reconcileEtcdMembers ensures the number of etcd members is in sync with the number of machines/nodes.
// This is usually required after a machine deletion.
//
// NOTE: this func uses KCP conditions, it is required to call reconcileControlPlaneConditions before this.
func (r *OCNEControlPlaneReconciler) reconcileEtcdMembers(ctx context.Context, controlPlane *internal.ControlPlane) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	// If etcd is not managed by KCP this is a no-op.
	if !controlPlane.IsEtcdManaged() {
		return ctrl.Result{}, nil
	}

	// If there is no KCP-owned control-plane machines, then control-plane has not been initialized yet.
	if controlPlane.Machines.Len() == 0 {
		return ctrl.Result{}, nil
	}

	// Collect all the node names.
	nodeNames := []string{}
	for _, machine := range controlPlane.Machines {
		if machine.Status.NodeRef == nil {
			// If there are provisioning machines (machines without a node yet), return.
			return ctrl.Result{}, nil
		}
		nodeNames = append(nodeNames, machine.Status.NodeRef.Name)
	}

	// Potential inconsistencies between the list of members and the list of machines/nodes are
	// surfaced using the EtcdClusterHealthyCondition; if this condition is true, meaning no inconsistencies exists, return early.
	if conditions.IsTrue(controlPlane.KCP, controlplanev1.EtcdClusterHealthyCondition) {
		return ctrl.Result{}, nil
	}

	workloadCluster, err := r.managementCluster.GetWorkloadCluster(ctx, util.ObjectKey(controlPlane.Cluster))
	if err != nil {
		// Failing at connecting to the workload cluster can mean workload cluster is unhealthy for a variety of reasons such as etcd quorum loss.
		return ctrl.Result{}, errors.Wrap(err, "cannot get remote client to workload cluster")
	}

	parsedVersion, err := semver.ParseTolerant(controlPlane.KCP.Spec.Version)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to parse kubernetes version %q", controlPlane.KCP.Spec.Version)
	}

	removedMembers, err := workloadCluster.ReconcileEtcdMembers(ctx, nodeNames, parsedVersion)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed attempt to reconcile etcd members")
	}

	if len(removedMembers) > 0 {
		log.Info("Etcd members without nodes removed from the cluster", "members", removedMembers)
	}

	return ctrl.Result{}, nil
}

func (r *OCNEControlPlaneReconciler) reconcileCertificateExpiries(ctx context.Context, controlPlane *internal.ControlPlane) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	// Return if there are no KCP-owned control-plane machines.
	if controlPlane.Machines.Len() == 0 {
		return ctrl.Result{}, nil
	}

	// Return if KCP is not yet initialized (no API server to contact for checking certificate expiration).
	if !controlPlane.KCP.Status.Initialized {
		return ctrl.Result{}, nil
	}

	// Ignore machines which are being deleted.
	machines := controlPlane.Machines.Filter(collections.Not(collections.HasDeletionTimestamp))

	workloadCluster, err := r.managementCluster.GetWorkloadCluster(ctx, util.ObjectKey(controlPlane.Cluster))
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to reconcile certificate expiries: cannot get remote client to workload cluster")
	}

	for _, m := range machines {
		log := log.WithValues("Machine", klog.KObj(m))

		ocneConfig, ok := controlPlane.GetOCNEConfig(m.Name)
		if !ok {
			// Skip if the Machine doesn't have a OCNEConfig.
			continue
		}

		annotations := ocneConfig.GetAnnotations()
		if _, ok := annotations[clusterv1.MachineCertificatesExpiryDateAnnotation]; ok {
			// Skip if annotation is already set.
			continue
		}

		if m.Status.NodeRef == nil {
			// Skip if the Machine is still provisioning.
			continue
		}
		nodeName := m.Status.NodeRef.Name
		log = log.WithValues("Node", klog.KRef("", nodeName))

		log.V(3).Info("Reconciling certificate expiry")
		certificateExpiry, err := workloadCluster.GetAPIServerCertificateExpiry(ctx, ocneConfig, nodeName)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to reconcile certificate expiry for Machine/%s", m.Name)
		}
		expiry := certificateExpiry.Format(time.RFC3339)

		log.V(2).Info(fmt.Sprintf("Setting certificate expiry to %s", expiry))
		patchHelper, err := patch.NewHelper(ocneConfig, r.Client)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to reconcile certificate expiry for Machine/%s: failed to create PatchHelper for OCNEConfig/%s", m.Name, ocneConfig.Name)
		}

		if annotations == nil {
			annotations = map[string]string{}
		}
		annotations[clusterv1.MachineCertificatesExpiryDateAnnotation] = expiry
		ocneConfig.SetAnnotations(annotations)

		if err := patchHelper.Patch(ctx, ocneConfig); err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to reconcile certificate expiry for Machine/%s: failed to patch OCNEConfig/%s", m.Name, ocneConfig.Name)
		}
	}

	return ctrl.Result{}, nil
}

func (r *OCNEControlPlaneReconciler) adoptMachines(ctx context.Context, ocnecp *controlplanev1.OCNEControlPlane, machines collections.Machines, cluster *clusterv1.Cluster) error {
	// We do an uncached full quorum read against the KCP to avoid re-adopting Machines the garbage collector just intentionally orphaned
	// See https://github.com/kubernetes/kubernetes/issues/42639
	uncached := controlplanev1.OCNEControlPlane{}
	err := r.managementClusterUncached.Get(ctx, client.ObjectKey{Namespace: ocnecp.Namespace, Name: ocnecp.Name}, &uncached)
	if err != nil {
		return errors.Wrapf(err, "failed to check whether %v/%v was deleted before adoption", ocnecp.GetNamespace(), ocnecp.GetName())
	}
	if !uncached.DeletionTimestamp.IsZero() {
		return errors.Errorf("%v/%v has just been deleted at %v", ocnecp.GetNamespace(), ocnecp.GetName(), ocnecp.GetDeletionTimestamp())
	}

	ocnecpVersion, err := semver.ParseTolerant(ocnecp.Spec.Version)
	if err != nil {
		return errors.Wrapf(err, "failed to parse kubernetes version %q", ocnecp.Spec.Version)
	}

	for _, m := range machines {
		ref := m.Spec.Bootstrap.ConfigRef

		// TODO instead of returning error here, we should instead Event and add a watch on potentially adoptable Machines
		if ref == nil || ref.Kind != "OCNEConfig" {
			return errors.Errorf("unable to adopt Machine %v/%v: expected a ConfigRef of kind OCNEConfig but instead found %v", m.Namespace, m.Name, ref)
		}

		// TODO instead of returning error here, we should instead Event and add a watch on potentially adoptable Machines
		if ref.Namespace != "" && ref.Namespace != ocnecp.Namespace {
			return errors.Errorf("could not adopt resources from OCNEConfig %v/%v: cannot adopt across namespaces", ref.Namespace, ref.Name)
		}

		if m.Spec.Version == nil {
			// if the machine's version is not immediately apparent, assume the operator knows what they're doing
			continue
		}

		machineVersion, err := semver.ParseTolerant(*m.Spec.Version)
		if err != nil {
			return errors.Wrapf(err, "failed to parse kubernetes version %q", *m.Spec.Version)
		}

		if !util.IsSupportedVersionSkew(ocnecpVersion, machineVersion) {
			r.recorder.Eventf(ocnecp, corev1.EventTypeWarning, "AdoptionFailed", "Could not adopt Machine %s/%s: its version (%q) is outside supported +/- one minor version skew from KCP's (%q)", m.Namespace, m.Name, *m.Spec.Version, ocnecp.Spec.Version)
			// avoid returning an error here so we don't cause the KCP controller to spin until the operator clarifies their intent
			return nil
		}
	}

	for _, m := range machines {
		ref := m.Spec.Bootstrap.ConfigRef
		cfg := &bootstrapv1.OCNEConfig{}

		if err := r.Client.Get(ctx, client.ObjectKey{Name: ref.Name, Namespace: ocnecp.Namespace}, cfg); err != nil {
			return err
		}

		if err := r.adoptOwnedSecrets(ctx, ocnecp, cfg, cluster.Name); err != nil {
			return err
		}

		patchHelper, err := patch.NewHelper(m, r.Client)
		if err != nil {
			return err
		}

		if err := controllerutil.SetControllerReference(ocnecp, m, r.Client.Scheme()); err != nil {
			return err
		}

		// Note that ValidateOwnerReferences() will reject this patch if another
		// OwnerReference exists with controller=true.
		if err := patchHelper.Patch(ctx, m); err != nil {
			return err
		}
	}
	return nil
}

func (r *OCNEControlPlaneReconciler) adoptOwnedSecrets(ctx context.Context, ocnecp *controlplanev1.OCNEControlPlane, currentOwner *bootstrapv1.OCNEConfig, clusterName string) error {
	secrets := corev1.SecretList{}
	if err := r.Client.List(ctx, &secrets, client.InNamespace(ocnecp.Namespace), client.MatchingLabels{clusterv1.ClusterLabelName: clusterName}); err != nil {
		return errors.Wrap(err, "error finding secrets for adoption")
	}

	for i := range secrets.Items {
		s := secrets.Items[i]
		if !util.IsOwnedByObject(&s, currentOwner) {
			continue
		}
		// avoid taking ownership of the bootstrap data secret
		if currentOwner.Status.DataSecretName != nil && s.Name == *currentOwner.Status.DataSecretName {
			continue
		}

		ss := s.DeepCopy()

		ss.SetOwnerReferences(util.ReplaceOwnerRef(ss.GetOwnerReferences(), currentOwner, metav1.OwnerReference{
			APIVersion:         controlplanev1.GroupVersion.String(),
			Kind:               "OCNEControlPlane",
			Name:               ocnecp.Name,
			UID:                ocnecp.UID,
			Controller:         pointer.Bool(true),
			BlockOwnerDeletion: pointer.Bool(true),
		}))

		if err := r.Client.Update(ctx, ss); err != nil {
			return errors.Wrapf(err, "error changing secret %v ownership from OCNEConfig/%v to OCNEControlPlane/%v", s.Name, currentOwner.GetName(), ocnecp.Name)
		}
	}

	return nil
}

// ensureCertificatesOwnerRef ensures an ownerReference to the owner is added on the Secrets holding certificates.
func ensureCertificatesOwnerRef(ctx context.Context, ctrlclient client.Client, clusterKey client.ObjectKey, certificates secret.Certificates, owner metav1.OwnerReference) error {
	for _, c := range certificates {
		s := &corev1.Secret{}
		secretKey := client.ObjectKey{Namespace: clusterKey.Namespace, Name: secret.Name(clusterKey.Name, c.Purpose)}
		if err := ctrlclient.Get(ctx, secretKey, s); err != nil {
			return errors.Wrapf(err, "failed to get Secret %s", secretKey)
		}
		// If the Type doesn't match the type used for secrets created by core components, KCP included
		if s.Type != clusterv1.ClusterSecretType {
			continue
		}
		patchHelper, err := patch.NewHelper(s, ctrlclient)
		if err != nil {
			return errors.Wrapf(err, "failed to create patchHelper for Secret %s", secretKey)
		}

		// Remove the current controller if one exists.
		if controller := metav1.GetControllerOf(s); controller != nil {
			s.SetOwnerReferences(util.RemoveOwnerRef(s.OwnerReferences, *controller))
		}

		s.OwnerReferences = util.EnsureOwnerRef(s.OwnerReferences, owner)
		if err := patchHelper.Patch(ctx, s); err != nil {
			return errors.Wrapf(err, "failed to patch Secret %s with ownerReference %s", secretKey, owner.String())
		}
	}
	return nil
}
