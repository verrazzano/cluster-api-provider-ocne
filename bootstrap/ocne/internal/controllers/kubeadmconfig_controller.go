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
	"strconv"
	"strings"
	"time"

	"github.com/verrazzano/cluster-api-provider-ocne/internal/util/ocne"

	"github.com/blang/semver"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	bootstrapv1 "github.com/verrazzano/cluster-api-provider-ocne/bootstrap/ocne/api/v1alpha1"
	"github.com/verrazzano/cluster-api-provider-ocne/bootstrap/ocne/internal/cloudinit"
	"github.com/verrazzano/cluster-api-provider-ocne/bootstrap/ocne/internal/ignition"
	"github.com/verrazzano/cluster-api-provider-ocne/bootstrap/ocne/internal/locking"
	ocnetypes "github.com/verrazzano/cluster-api-provider-ocne/bootstrap/ocne/types"
	bsutil "github.com/verrazzano/cluster-api-provider-ocne/bootstrap/util"
	"github.com/verrazzano/cluster-api-provider-ocne/feature"
	"github.com/verrazzano/cluster-api-provider-ocne/util"
	"github.com/verrazzano/cluster-api-provider-ocne/util/annotations"
	"github.com/verrazzano/cluster-api-provider-ocne/util/conditions"
	clog "github.com/verrazzano/cluster-api-provider-ocne/util/log"
	"github.com/verrazzano/cluster-api-provider-ocne/util/patch"
	"github.com/verrazzano/cluster-api-provider-ocne/util/predicates"
	"github.com/verrazzano/cluster-api-provider-ocne/util/secret"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/remote"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
)

const (
	// OCNEConfigControllerName defines the controller used when creating clients.
	OCNEConfigControllerName = "ocneconfig-controller"

	// DefaultTokenTTL is the default TTL used for tokens.
	DefaultTokenTTL = 15 * time.Minute
)

// InitLocker is a lock that is used around ocne init.
type InitLocker interface {
	Lock(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine) bool
	Unlock(ctx context.Context, cluster *clusterv1.Cluster) bool
}

// +kubebuilder:rbac:groups=bootstrap.cluster.x-k8s.io,resources=ocneconfigs;ocneconfigs/status;ocneconfigs/finalizers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status;machinesets;machines;machines/status;machinepools;machinepools/status,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets;events;configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=*,verbs=get;list

// OCNEConfigReconciler reconciles a OCNEConfig object.
type OCNEConfigReconciler struct {
	Client          client.Client
	KubeadmInitLock InitLocker

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string

	// TokenTTL is the amount of time a bootstrap token (and therefore a OCNEConfig) will be valid.
	TokenTTL time.Duration

	remoteClientGetter remote.ClusterClientGetter
}

// Scope is a scoped struct used during reconciliation.
type Scope struct {
	logr.Logger
	Config      *bootstrapv1.OCNEConfig
	ConfigOwner *bsutil.ConfigOwner
	Cluster     *clusterv1.Cluster
}

// SetupWithManager sets up the reconciler with the Manager.
func (r *OCNEConfigReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	if r.KubeadmInitLock == nil {
		r.KubeadmInitLock = locking.NewControlPlaneInitMutex(mgr.GetClient())
	}
	if r.remoteClientGetter == nil {
		r.remoteClientGetter = remote.NewClusterClient
	}
	if r.TokenTTL == 0 {
		r.TokenTTL = DefaultTokenTTL
	}

	b := ctrl.NewControllerManagedBy(mgr).
		For(&bootstrapv1.OCNEConfig{}).
		WithOptions(options).
		Watches(
			&source.Kind{Type: &clusterv1.Machine{}},
			handler.EnqueueRequestsFromMapFunc(r.MachineToBootstrapMapFunc),
		).WithEventFilter(predicates.ResourceNotPausedAndHasFilterLabel(ctrl.LoggerFrom(ctx), r.WatchFilterValue))

	if feature.Gates.Enabled(feature.MachinePool) {
		b = b.Watches(
			&source.Kind{Type: &expv1.MachinePool{}},
			handler.EnqueueRequestsFromMapFunc(r.MachinePoolToBootstrapMapFunc),
		).WithEventFilter(predicates.ResourceNotPausedAndHasFilterLabel(ctrl.LoggerFrom(ctx), r.WatchFilterValue))
	}

	c, err := b.Build(r)
	if err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}

	err = c.Watch(
		&source.Kind{Type: &clusterv1.Cluster{}},
		handler.EnqueueRequestsFromMapFunc(r.ClusterToOCNEConfigs),
		predicates.All(ctrl.LoggerFrom(ctx),
			predicates.ClusterUnpausedAndInfrastructureReady(ctrl.LoggerFrom(ctx)),
			predicates.ResourceHasFilterLabel(ctrl.LoggerFrom(ctx), r.WatchFilterValue),
		),
	)
	if err != nil {
		return errors.Wrap(err, "failed adding Watch for Clusters to controller manager")
	}

	return nil
}

// Reconcile handles OCNEConfig events.
func (r *OCNEConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, rerr error) {
	log := ctrl.LoggerFrom(ctx)

	// Lookup the ocne config
	config := &bootstrapv1.OCNEConfig{}
	if err := r.Client.Get(ctx, req.NamespacedName, config); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get config")
		return ctrl.Result{}, err
	}

	// AddOwners adds the owners of OCNEConfig as k/v pairs to the logger.
	// Specifically, it will add KubeadmControlPlane, MachineSet and MachineDeployment.
	ctx, log, err := clog.AddOwners(ctx, r.Client, config)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Look up the owner of this ocne config if there is one
	configOwner, err := bsutil.GetConfigOwner(ctx, r.Client, config)
	if apierrors.IsNotFound(err) {
		// Could not find the owner yet, this is not an error and will rereconcile when the owner gets set.
		return ctrl.Result{}, nil
	}
	if err != nil {
		log.Error(err, "Failed to get owner")
		return ctrl.Result{}, err
	}
	if configOwner == nil {
		return ctrl.Result{}, nil
	}
	log = log.WithValues(configOwner.GetKind(), klog.KRef(configOwner.GetNamespace(), configOwner.GetName()), "resourceVersion", configOwner.GetResourceVersion())

	log = log.WithValues("Cluster", klog.KRef(configOwner.GetNamespace(), configOwner.ClusterName()))
	ctx = ctrl.LoggerInto(ctx, log)

	// Lookup the cluster the config owner is associated with
	cluster, err := util.GetClusterByName(ctx, r.Client, configOwner.GetNamespace(), configOwner.ClusterName())
	if err != nil {
		if errors.Cause(err) == util.ErrNoCluster {
			log.Info(fmt.Sprintf("%s does not belong to a cluster yet, waiting until it's part of a cluster", configOwner.GetKind()))
			return ctrl.Result{}, nil
		}

		if apierrors.IsNotFound(err) {
			log.Info("Cluster does not exist yet, waiting until it is created")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Could not get cluster with metadata")
		return ctrl.Result{}, err
	}

	if annotations.IsPaused(cluster, config) {
		log.Info("Reconciliation is paused for this object")
		return ctrl.Result{}, nil
	}

	scope := &Scope{
		Logger:      log,
		Config:      config,
		ConfigOwner: configOwner,
		Cluster:     cluster,
	}

	// Initialize the patch helper.
	patchHelper, err := patch.NewHelper(config, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Attempt to Patch the OCNEConfig object and status after each reconciliation if no error occurs.
	defer func() {
		// always update the readyCondition; the summary is represented using the "1 of x completed" notation.
		conditions.SetSummary(config,
			conditions.WithConditions(
				bootstrapv1.DataSecretAvailableCondition,
				bootstrapv1.CertificatesAvailableCondition,
			),
		)
		// Patch ObservedGeneration only if the reconciliation completed successfully
		patchOpts := []patch.Option{}
		if rerr == nil {
			patchOpts = append(patchOpts, patch.WithStatusObservedGeneration{})
		}
		if err := patchHelper.Patch(ctx, config, patchOpts...); err != nil {
			log.Error(rerr, "Failed to patch config")
			if rerr == nil {
				rerr = err
			}
		}
	}()
	// Ensure the bootstrap secret associated with this OCNEConfig has the correct ownerReference.
	if err := r.ensureBootstrapSecretOwnersRef(ctx, scope); err != nil {
		return ctrl.Result{}, err
	}
	switch {
	// Wait for the infrastructure to be ready.
	case !cluster.Status.InfrastructureReady:
		log.Info("Cluster infrastructure is not ready, waiting")
		conditions.MarkFalse(config, bootstrapv1.DataSecretAvailableCondition, bootstrapv1.WaitingForClusterInfrastructureReason, clusterv1.ConditionSeverityInfo, "")
		return ctrl.Result{}, nil
	// Reconcile status for machines that already have a secret reference, but our status isn't up to date.
	// This case solves the pivoting scenario (or a backup restore) which doesn't preserve the status subresource on objects.
	case configOwner.DataSecretName() != nil && (!config.Status.Ready || config.Status.DataSecretName == nil):
		config.Status.Ready = true
		config.Status.DataSecretName = configOwner.DataSecretName()
		conditions.MarkTrue(config, bootstrapv1.DataSecretAvailableCondition)
		return ctrl.Result{}, nil
	// Status is ready means a config has been generated.
	case config.Status.Ready:
		if config.Spec.JoinConfiguration != nil && config.Spec.JoinConfiguration.Discovery.BootstrapToken != nil {
			if !configOwner.HasNodeRefs() {
				// If the BootstrapToken has been generated for a join but the config owner has no nodeRefs,
				// this indicates that the node has not yet joined and the token in the join config has not
				// been consumed and it may need a refresh.
				return r.refreshBootstrapToken(ctx, config, cluster)
			}
			if configOwner.IsMachinePool() {
				// If the BootstrapToken has been generated and infrastructure is ready but the configOwner is a MachinePool,
				// we rotate the token to keep it fresh for future scale ups.
				return r.rotateMachinePoolBootstrapToken(ctx, config, cluster, scope)
			}
		}
		// In any other case just return as the config is already generated and need not be generated again.
		return ctrl.Result{}, nil
	}

	// Note: can't use IsFalse here because we need to handle the absence of the condition as well as false.
	if !conditions.IsTrue(cluster, clusterv1.ControlPlaneInitializedCondition) {
		return r.handleClusterNotInitialized(ctx, scope)
	}

	// Every other case it's a join scenario
	// Nb. in this case ClusterConfiguration and InitConfiguration should not be defined by users, but in case of misconfigurations, CABPOCNE simply ignore them

	// Unlock any locks that might have been set during init process
	r.KubeadmInitLock.Unlock(ctx, cluster)

	// if the JoinConfiguration is missing, create a default one
	if config.Spec.JoinConfiguration == nil {
		log.Info("Creating default JoinConfiguration")
		config.Spec.JoinConfiguration = &bootstrapv1.JoinConfiguration{}
	}

	// it's a control plane join
	if configOwner.IsControlPlaneMachine() {
		return r.joinControlplane(ctx, scope)
	}

	// It's a worker join
	return r.joinWorker(ctx, scope)
}

func (r *OCNEConfigReconciler) refreshBootstrapToken(ctx context.Context, config *bootstrapv1.OCNEConfig, cluster *clusterv1.Cluster) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	token := config.Spec.JoinConfiguration.Discovery.BootstrapToken.Token

	remoteClient, err := r.remoteClientGetter(ctx, OCNEConfigControllerName, r.Client, util.ObjectKey(cluster))
	if err != nil {
		log.Error(err, "Error creating remote cluster client")
		return ctrl.Result{}, err
	}

	log.Info("Refreshing token until the infrastructure has a chance to consume it")
	if err := refreshToken(ctx, remoteClient, token, r.TokenTTL); err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to refresh bootstrap token")
	}
	return ctrl.Result{
		RequeueAfter: r.TokenTTL / 2,
	}, nil
}

func (r *OCNEConfigReconciler) rotateMachinePoolBootstrapToken(ctx context.Context, config *bootstrapv1.OCNEConfig, cluster *clusterv1.Cluster, scope *Scope) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.V(2).Info("Config is owned by a MachinePool, checking if token should be rotated")
	remoteClient, err := r.remoteClientGetter(ctx, OCNEConfigControllerName, r.Client, util.ObjectKey(cluster))
	if err != nil {
		return ctrl.Result{}, err
	}

	token := config.Spec.JoinConfiguration.Discovery.BootstrapToken.Token
	shouldRotate, err := shouldRotate(ctx, remoteClient, token, r.TokenTTL)
	if err != nil {
		return ctrl.Result{}, err
	}
	if shouldRotate {
		log.Info("Creating new bootstrap token, the existing one should be rotated")
		token, err := createToken(ctx, remoteClient, r.TokenTTL)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to create new bootstrap token")
		}

		config.Spec.JoinConfiguration.Discovery.BootstrapToken.Token = token
		log.V(3).Info("Altering JoinConfiguration.Discovery.BootstrapToken.Token")

		// update the bootstrap data
		return r.joinWorker(ctx, scope)
	}
	return ctrl.Result{
		RequeueAfter: r.TokenTTL / 3,
	}, nil
}

func (r *OCNEConfigReconciler) handleClusterNotInitialized(ctx context.Context, scope *Scope) (_ ctrl.Result, reterr error) {
	// initialize the DataSecretAvailableCondition if missing.
	// this is required in order to avoid the condition's LastTransitionTime to flicker in case of errors surfacing
	// using the DataSecretGeneratedFailedReason
	if conditions.GetReason(scope.Config, bootstrapv1.DataSecretAvailableCondition) != bootstrapv1.DataSecretGenerationFailedReason {
		conditions.MarkFalse(scope.Config, bootstrapv1.DataSecretAvailableCondition, clusterv1.WaitingForControlPlaneAvailableReason, clusterv1.ConditionSeverityInfo, "")
	}

	// if it's NOT a control plane machine, requeue
	if !scope.ConfigOwner.IsControlPlaneMachine() {
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// if the machine has not ClusterConfiguration and InitConfiguration, requeue
	if scope.Config.Spec.InitConfiguration == nil && scope.Config.Spec.ClusterConfiguration == nil {
		scope.Info("Control plane is not ready, requeing joining control planes until ready.")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	machine := &clusterv1.Machine{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(scope.ConfigOwner.Object, machine); err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "cannot convert %s to Machine", scope.ConfigOwner.GetKind())
	}

	// acquire the init lock so that only the first machine configured
	// as control plane get processed here
	// if not the first, requeue
	if !r.KubeadmInitLock.Lock(ctx, scope.Cluster, machine) {
		scope.Info("A control plane is already being initialized, requeing until control plane is ready")
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	defer func() {
		if reterr != nil {
			if !r.KubeadmInitLock.Unlock(ctx, scope.Cluster) {
				reterr = kerrors.NewAggregate([]error{reterr, errors.New("failed to unlock the ocne init lock")})
			}
		}
	}()

	scope.Info("Creating BootstrapData for the first control plane")

	// Nb. in this case JoinConfiguration should not be defined by users, but in case of misconfigurations, CABPOCNE simply ignore it

	// get both of ClusterConfiguration and InitConfiguration strings to pass to the cloud init control plane generator
	// ocne allows one of these values to be empty; CABPOCNE replace missing values with an empty config, so the cloud init generation
	// should not handle special cases.

	kubernetesVersion := scope.ConfigOwner.KubernetesVersion()
	parsedVersion, err := semver.ParseTolerant(kubernetesVersion)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to parse kubernetes version %q", kubernetesVersion)
	}

	if scope.Config.Spec.InitConfiguration == nil {
		scope.Config.Spec.InitConfiguration = &bootstrapv1.InitConfiguration{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "ocne.k8s.io/v1alpha1",
				Kind:       "InitConfiguration",
			},
		}
	}

	newInitSpec := setOCNEControlPlaneInitConfigurationDefaults(scope.Config.Spec.InitConfiguration)

	initdata, err := ocnetypes.MarshalInitConfigurationForVersion(newInitSpec, parsedVersion)
	if err != nil {
		scope.Error(err, "Failed to marshal init configuration")
		return ctrl.Result{}, err
	}

	if scope.Config.Spec.ClusterConfiguration == nil {
		scope.Config.Spec.ClusterConfiguration = &bootstrapv1.ClusterConfiguration{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "ocne.k8s.io/v1alpha1",
				Kind:       "ClusterConfiguration",
			},
		}
	}

	// injects into config.ClusterConfiguration values from top level object
	r.reconcileTopLevelObjectSettings(ctx, scope.Cluster, machine, scope.Config)

	newSpec, err := setOCNEControlPlaneClusterConfigurationDefaults(ctx, scope.Config.Spec.ClusterConfiguration, kubernetesVersion)
	if err != nil {
		scope.Error(err, "Failed to set OCNE defaults for kubeadm")
		return ctrl.Result{}, err
	}

	clusterdata, err := ocnetypes.MarshalClusterConfigurationForVersion(newSpec, parsedVersion)
	if err != nil {
		scope.Error(err, "Failed to marshal cluster configuration")
		return ctrl.Result{}, err
	}

	certificates := secret.NewCertificatesForInitialControlPlane(scope.Config.Spec.ClusterConfiguration)

	// If the Cluster does not have a ControlPlane reference look up and generate the certificates.
	// Otherwise rely on certificates generated by the ControlPlane controller.
	// Note: A cluster does not have a ControlPlane reference when using standalone CP machines.
	if scope.Cluster.Spec.ControlPlaneRef == nil {
		err = certificates.LookupOrGenerate(
			ctx,
			r.Client,
			util.ObjectKey(scope.Cluster),
			*metav1.NewControllerRef(scope.Config, bootstrapv1.GroupVersion.WithKind("OCNEConfig")))
	} else {
		err = certificates.Lookup(ctx,
			r.Client,
			util.ObjectKey(scope.Cluster))
	}
	if err != nil {
		conditions.MarkFalse(scope.Config, bootstrapv1.CertificatesAvailableCondition, bootstrapv1.CertificatesGenerationFailedReason, clusterv1.ConditionSeverityWarning, err.Error())
		return ctrl.Result{}, err
	}

	conditions.MarkTrue(scope.Config, bootstrapv1.CertificatesAvailableCondition)

	verbosityFlag := ""
	if scope.Config.Spec.Verbosity != nil {
		verbosityFlag = fmt.Sprintf("--v %s", strconv.Itoa(int(*scope.Config.Spec.Verbosity)))
	}

	files, err := r.resolveFiles(ctx, scope.Config)
	if err != nil {
		conditions.MarkFalse(scope.Config, bootstrapv1.DataSecretAvailableCondition, bootstrapv1.DataSecretGenerationFailedReason, clusterv1.ConditionSeverityWarning, err.Error())
		return ctrl.Result{}, err
	}

	users, err := r.resolveUsers(ctx, scope.Config)
	if err != nil {
		conditions.MarkFalse(scope.Config, bootstrapv1.DataSecretAvailableCondition, bootstrapv1.DataSecretGenerationFailedReason, clusterv1.ConditionSeverityWarning, err.Error())
		return ctrl.Result{}, err
	}

	// Setting the image repository to ocne container repository rather than k8s.io
	ocneRepository := ocne.DefaultOCNEImageRepository
	if scope.Config.Spec.ClusterConfiguration.ImageRepository != "" {
		ocneRepository = scope.Config.Spec.ClusterConfiguration.ImageRepository
	}

	var podSubnet, serviceSubnet string
	if scope.Cluster.Spec.ClusterNetwork != nil {
		if scope.Cluster.Spec.ClusterNetwork.Pods != nil {
			if scope.Cluster.Spec.ClusterNetwork.Pods.CIDRBlocks != nil {
				podSubnet = strings.Join(scope.Cluster.Spec.ClusterNetwork.Pods.CIDRBlocks, ",")
			}
		}

		if scope.Cluster.Spec.ClusterNetwork.Services != nil {
			if scope.Cluster.Spec.ClusterNetwork.Services.CIDRBlocks != nil {
				serviceSubnet = strings.Join(scope.Cluster.Spec.ClusterNetwork.Services.CIDRBlocks, ",")
			}
		}
	}

	skipInstall := false
	dockerInfra := false
	var proxy *bootstrapv1.ProxySpec
	if scope.Config.Spec.ImageConfiguration != nil {
		if scope.Config.Spec.ImageConfiguration.Dependencies != nil {
			skipInstall = scope.Config.Spec.ImageConfiguration.Dependencies.SkipInstall
			dockerInfra = scope.Config.Spec.ImageConfiguration.Dependencies.DockerInfraStructure
		}
		if scope.Config.Spec.ImageConfiguration.Proxy != nil {
			proxy = scope.Config.Spec.ImageConfiguration.Proxy
		}
	}

	controlPlaneInput := &cloudinit.ControlPlaneInput{
		BaseUserData: cloudinit.BaseUserData{
			AdditionalFiles:      files,
			NTP:                  scope.Config.Spec.NTP,
			PreOCNECommands:      scope.Config.Spec.PreOCNECommands,
			PostOCNECommands:     scope.Config.Spec.PostOCNECommands,
			Users:                users,
			Mounts:               scope.Config.Spec.Mounts,
			DiskSetup:            scope.Config.Spec.DiskSetup,
			OCNEVerbosity:        verbosityFlag,
			KubernetesVersion:    kubernetesVersion,
			OCNEImageRepository:  ocneRepository,
			Proxy:                proxy,
			PodSubnet:            podSubnet,
			ServiceSubnet:        serviceSubnet,
			SkipInstall:          skipInstall,
			DockerInfrastructure: dockerInfra,
		},
		InitConfiguration:    initdata,
		ClusterConfiguration: clusterdata,
		Certificates:         certificates,
	}

	var bootstrapInitData []byte
	switch scope.Config.Spec.Format {
	case bootstrapv1.Ignition:
		bootstrapInitData, _, err = ignition.NewInitControlPlane(&ignition.ControlPlaneInput{
			ControlPlaneInput: controlPlaneInput,
			Ignition:          scope.Config.Spec.Ignition,
		})
	default:
		bootstrapInitData, err = cloudinit.NewInitControlPlane(controlPlaneInput)
	}

	if err != nil {
		scope.Error(err, "Failed to generate user data for bootstrap control plane")
		return ctrl.Result{}, err
	}

	if err := r.storeBootstrapData(ctx, scope, bootstrapInitData); err != nil {
		scope.Error(err, "Failed to store bootstrap data")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *OCNEConfigReconciler) joinWorker(ctx context.Context, scope *Scope) (ctrl.Result, error) {
	scope.Info("Creating BootstrapData for the worker node")

	certificates := secret.NewCertificatesForWorker(scope.Config.Spec.JoinConfiguration.CACertPath)
	err := certificates.Lookup(
		ctx,
		r.Client,
		util.ObjectKey(scope.Cluster),
	)
	if err != nil {
		conditions.MarkFalse(scope.Config, bootstrapv1.CertificatesAvailableCondition, bootstrapv1.CertificatesCorruptedReason, clusterv1.ConditionSeverityError, err.Error())
		return ctrl.Result{}, err
	}
	if err := certificates.EnsureAllExist(); err != nil {
		conditions.MarkFalse(scope.Config, bootstrapv1.CertificatesAvailableCondition, bootstrapv1.CertificatesCorruptedReason, clusterv1.ConditionSeverityError, err.Error())
		return ctrl.Result{}, err
	}
	conditions.MarkTrue(scope.Config, bootstrapv1.CertificatesAvailableCondition)

	// Ensure that joinConfiguration.Discovery is properly set for joining node on the current cluster.
	if res, err := r.reconcileDiscovery(ctx, scope.Cluster, scope.Config, certificates); err != nil {
		return ctrl.Result{}, err
	} else if !res.IsZero() {
		return res, nil
	}

	kubernetesVersion := scope.ConfigOwner.KubernetesVersion()
	parsedVersion, err := semver.ParseTolerant(kubernetesVersion)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to parse kubernetes version %q", kubernetesVersion)
	}

	newJoinSpec := setOCNEJoinConfigurationDefaults(scope.Config.Spec.JoinConfiguration)

	joinData, err := ocnetypes.MarshalJoinConfigurationForVersion(newJoinSpec, parsedVersion)
	if err != nil {
		scope.Error(err, "Failed to marshal join configuration")
		return ctrl.Result{}, err
	}

	if scope.Config.Spec.JoinConfiguration.ControlPlane != nil {
		return ctrl.Result{}, errors.New("Machine is a Worker, but JoinConfiguration.ControlPlane is set in the OCNEConfig object")
	}

	verbosityFlag := ""
	if scope.Config.Spec.Verbosity != nil {
		verbosityFlag = fmt.Sprintf("--v %s", strconv.Itoa(int(*scope.Config.Spec.Verbosity)))
	}

	files, err := r.resolveFiles(ctx, scope.Config)
	if err != nil {
		conditions.MarkFalse(scope.Config, bootstrapv1.DataSecretAvailableCondition, bootstrapv1.DataSecretGenerationFailedReason, clusterv1.ConditionSeverityWarning, err.Error())
		return ctrl.Result{}, err
	}

	users, err := r.resolveUsers(ctx, scope.Config)
	if err != nil {
		conditions.MarkFalse(scope.Config, bootstrapv1.DataSecretAvailableCondition, bootstrapv1.DataSecretGenerationFailedReason, clusterv1.ConditionSeverityWarning, err.Error())
		return ctrl.Result{}, err
	}

	// Setting the image repository to ocne container repository rather than k8s.io
	ocneRepository := ocne.DefaultOCNEImageRepository
	if scope.Config.Spec.ClusterConfiguration != nil && scope.Config.Spec.ClusterConfiguration.ImageRepository != "" {
		ocneRepository = scope.Config.Spec.ClusterConfiguration.ImageRepository
		scope.Info(fmt.Sprintf("Worker join configuration image repository from input configuration: %s", ocneRepository))
	}

	var podSubnet, serviceSubnet string
	if scope.Cluster.Spec.ClusterNetwork != nil {
		if scope.Cluster.Spec.ClusterNetwork.Pods != nil {
			if scope.Cluster.Spec.ClusterNetwork.Pods.CIDRBlocks != nil {
				podSubnet = strings.Join(scope.Cluster.Spec.ClusterNetwork.Pods.CIDRBlocks, ",")
			}
		}

		if scope.Cluster.Spec.ClusterNetwork.Services != nil {
			if scope.Cluster.Spec.ClusterNetwork.Services.CIDRBlocks != nil {
				serviceSubnet = strings.Join(scope.Cluster.Spec.ClusterNetwork.Services.CIDRBlocks, ",")
			}
		}
	}

	skipInstall := false
	dockerInfra := false
	var proxy *bootstrapv1.ProxySpec
	if scope.Config.Spec.ImageConfiguration != nil {
		if scope.Config.Spec.ImageConfiguration.Dependencies != nil {
			skipInstall = scope.Config.Spec.ImageConfiguration.Dependencies.SkipInstall
			dockerInfra = scope.Config.Spec.ImageConfiguration.Dependencies.DockerInfraStructure
		}
		if scope.Config.Spec.ImageConfiguration.Proxy != nil {
			proxy = scope.Config.Spec.ImageConfiguration.Proxy
		}
	}

	nodeInput := &cloudinit.NodeInput{
		BaseUserData: cloudinit.BaseUserData{
			AdditionalFiles:      files,
			NTP:                  scope.Config.Spec.NTP,
			PreOCNECommands:      scope.Config.Spec.PreOCNECommands,
			PostOCNECommands:     scope.Config.Spec.PostOCNECommands,
			Users:                users,
			Mounts:               scope.Config.Spec.Mounts,
			DiskSetup:            scope.Config.Spec.DiskSetup,
			OCNEVerbosity:        verbosityFlag,
			UseExperimentalRetry: scope.Config.Spec.UseExperimentalRetryJoin,
			OCNEImageRepository:  ocneRepository,
			Proxy:                proxy,
			PodSubnet:            podSubnet,
			ServiceSubnet:        serviceSubnet,
			KubernetesVersion:    kubernetesVersion,
			SkipInstall:          skipInstall,
			DockerInfrastructure: dockerInfra,
		},
		JoinConfiguration: joinData,
	}

	var bootstrapJoinData []byte
	switch scope.Config.Spec.Format {
	case bootstrapv1.Ignition:
		bootstrapJoinData, _, err = ignition.NewNode(&ignition.NodeInput{
			NodeInput: nodeInput,
			Ignition:  scope.Config.Spec.Ignition,
		})
	default:
		bootstrapJoinData, err = cloudinit.NewNode(nodeInput)
	}

	if err != nil {
		scope.Error(err, "Failed to create a worker join configuration")
		return ctrl.Result{}, err
	}

	if err := r.storeBootstrapData(ctx, scope, bootstrapJoinData); err != nil {
		scope.Error(err, "Failed to store bootstrap data")
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *OCNEConfigReconciler) joinControlplane(ctx context.Context, scope *Scope) (ctrl.Result, error) {
	scope.Info("Creating BootstrapData for the joining control plane")

	if !scope.ConfigOwner.IsControlPlaneMachine() {
		return ctrl.Result{}, fmt.Errorf("%s is not a valid control plane kind, only Machine is supported", scope.ConfigOwner.GetKind())
	}

	if scope.Config.Spec.JoinConfiguration.ControlPlane == nil {
		scope.Config.Spec.JoinConfiguration.ControlPlane = &bootstrapv1.JoinControlPlane{}
	}

	certificates := secret.NewControlPlaneJoinCerts(scope.Config.Spec.ClusterConfiguration)
	err := certificates.Lookup(
		ctx,
		r.Client,
		util.ObjectKey(scope.Cluster),
	)
	if err != nil {
		conditions.MarkFalse(scope.Config, bootstrapv1.CertificatesAvailableCondition, bootstrapv1.CertificatesCorruptedReason, clusterv1.ConditionSeverityError, err.Error())
		return ctrl.Result{}, err
	}
	if err := certificates.EnsureAllExist(); err != nil {
		conditions.MarkFalse(scope.Config, bootstrapv1.CertificatesAvailableCondition, bootstrapv1.CertificatesCorruptedReason, clusterv1.ConditionSeverityError, err.Error())
		return ctrl.Result{}, err
	}

	conditions.MarkTrue(scope.Config, bootstrapv1.CertificatesAvailableCondition)

	// Ensure that joinConfiguration.Discovery is properly set for joining node on the current cluster.
	if res, err := r.reconcileDiscovery(ctx, scope.Cluster, scope.Config, certificates); err != nil {
		return ctrl.Result{}, err
	} else if !res.IsZero() {
		return res, nil
	}

	kubernetesVersion := scope.ConfigOwner.KubernetesVersion()
	parsedVersion, err := semver.ParseTolerant(kubernetesVersion)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to parse kubernetes version %q", kubernetesVersion)
	}

	newJoinSpec := setOCNEJoinConfigurationDefaults(scope.Config.Spec.JoinConfiguration)

	joinData, err := ocnetypes.MarshalJoinConfigurationForVersion(newJoinSpec, parsedVersion)
	if err != nil {
		scope.Error(err, "Failed to marshal join configuration")
		return ctrl.Result{}, err
	}

	verbosityFlag := ""
	if scope.Config.Spec.Verbosity != nil {
		verbosityFlag = fmt.Sprintf("--v %s", strconv.Itoa(int(*scope.Config.Spec.Verbosity)))
	}

	files, err := r.resolveFiles(ctx, scope.Config)
	if err != nil {
		conditions.MarkFalse(scope.Config, bootstrapv1.DataSecretAvailableCondition, bootstrapv1.DataSecretGenerationFailedReason, clusterv1.ConditionSeverityWarning, err.Error())
		return ctrl.Result{}, err
	}

	users, err := r.resolveUsers(ctx, scope.Config)
	if err != nil {
		conditions.MarkFalse(scope.Config, bootstrapv1.DataSecretAvailableCondition, bootstrapv1.DataSecretGenerationFailedReason, clusterv1.ConditionSeverityWarning, err.Error())
		return ctrl.Result{}, err
	}

	// Setting the image repository to ocne container repository rather than k8s.io
	ocneRepository := ocne.DefaultOCNEImageRepository
	if scope.Config.Spec.ClusterConfiguration != nil && scope.Config.Spec.ClusterConfiguration.ImageRepository != "" {
		ocneRepository = scope.Config.Spec.ClusterConfiguration.ImageRepository
	}

	var podSubnet, serviceSubnet string
	if scope.Cluster.Spec.ClusterNetwork != nil {
		if scope.Cluster.Spec.ClusterNetwork.Pods != nil {
			if scope.Cluster.Spec.ClusterNetwork.Pods.CIDRBlocks != nil {
				podSubnet = strings.Join(scope.Cluster.Spec.ClusterNetwork.Pods.CIDRBlocks, ",")
			}
		}

		if scope.Cluster.Spec.ClusterNetwork.Services != nil {
			if scope.Cluster.Spec.ClusterNetwork.Services.CIDRBlocks != nil {
				serviceSubnet = strings.Join(scope.Cluster.Spec.ClusterNetwork.Services.CIDRBlocks, ",")
			}
		}
	}

	skipInstall := false
	dockerInfra := false
	var proxy *bootstrapv1.ProxySpec
	if scope.Config.Spec.ImageConfiguration != nil {
		if scope.Config.Spec.ImageConfiguration.Dependencies != nil {
			skipInstall = scope.Config.Spec.ImageConfiguration.Dependencies.SkipInstall
			dockerInfra = scope.Config.Spec.ImageConfiguration.Dependencies.DockerInfraStructure
		}
		if scope.Config.Spec.ImageConfiguration.Proxy != nil {
			proxy = scope.Config.Spec.ImageConfiguration.Proxy
		}
	}

	controlPlaneJoinInput := &cloudinit.ControlPlaneJoinInput{
		JoinConfiguration: joinData,
		Certificates:      certificates,
		BaseUserData: cloudinit.BaseUserData{
			AdditionalFiles:      files,
			NTP:                  scope.Config.Spec.NTP,
			PreOCNECommands:      scope.Config.Spec.PreOCNECommands,
			PostOCNECommands:     scope.Config.Spec.PostOCNECommands,
			Users:                users,
			Mounts:               scope.Config.Spec.Mounts,
			DiskSetup:            scope.Config.Spec.DiskSetup,
			OCNEVerbosity:        verbosityFlag,
			UseExperimentalRetry: scope.Config.Spec.UseExperimentalRetryJoin,
			KubernetesVersion:    kubernetesVersion,
			OCNEImageRepository:  ocneRepository,
			Proxy:                proxy,
			PodSubnet:            podSubnet,
			ServiceSubnet:        serviceSubnet,
			SkipInstall:          skipInstall,
			DockerInfrastructure: dockerInfra,
		},
	}

	var bootstrapJoinData []byte
	switch scope.Config.Spec.Format {
	case bootstrapv1.Ignition:
		bootstrapJoinData, _, err = ignition.NewJoinControlPlane(&ignition.ControlPlaneJoinInput{
			ControlPlaneJoinInput: controlPlaneJoinInput,
			Ignition:              scope.Config.Spec.Ignition,
		})
	default:
		bootstrapJoinData, err = cloudinit.NewJoinControlPlane(controlPlaneJoinInput)
	}

	if err != nil {
		scope.Error(err, "Failed to create a control plane join configuration")
		return ctrl.Result{}, err
	}

	if err := r.storeBootstrapData(ctx, scope, bootstrapJoinData); err != nil {
		scope.Error(err, "Failed to store bootstrap data")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// resolveFiles maps .Spec.Files into cloudinit.Files, resolving any object references
// along the way.
func (r *OCNEConfigReconciler) resolveFiles(ctx context.Context, cfg *bootstrapv1.OCNEConfig) ([]bootstrapv1.File, error) {
	collected := make([]bootstrapv1.File, 0, len(cfg.Spec.Files))

	for i := range cfg.Spec.Files {
		in := cfg.Spec.Files[i]
		if in.ContentFrom != nil {
			data, err := r.resolveSecretFileContent(ctx, cfg.Namespace, in)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to resolve file source")
			}
			in.ContentFrom = nil
			in.Content = string(data)
		}
		collected = append(collected, in)
	}

	return collected, nil
}

// resolveSecretFileContent returns file content fetched from a referenced secret object.
func (r *OCNEConfigReconciler) resolveSecretFileContent(ctx context.Context, ns string, source bootstrapv1.File) ([]byte, error) {
	secret := &corev1.Secret{}
	key := types.NamespacedName{Namespace: ns, Name: source.ContentFrom.Secret.Name}
	if err := r.Client.Get(ctx, key, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, errors.Wrapf(err, "secret not found: %s", key)
		}
		return nil, errors.Wrapf(err, "failed to retrieve Secret %q", key)
	}
	data, ok := secret.Data[source.ContentFrom.Secret.Key]
	if !ok {
		return nil, errors.Errorf("secret references non-existent secret key: %q", source.ContentFrom.Secret.Key)
	}
	return data, nil
}

// resolveUsers maps .Spec.Users into cloudinit.Users, resolving any object references
// along the way.
func (r *OCNEConfigReconciler) resolveUsers(ctx context.Context, cfg *bootstrapv1.OCNEConfig) ([]bootstrapv1.User, error) {
	collected := make([]bootstrapv1.User, 0, len(cfg.Spec.Users))

	for i := range cfg.Spec.Users {
		in := cfg.Spec.Users[i]
		if in.PasswdFrom != nil {
			data, err := r.resolveSecretPasswordContent(ctx, cfg.Namespace, in)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to resolve passwd source")
			}
			in.PasswdFrom = nil
			passwdContent := string(data)
			in.Passwd = &passwdContent
		}
		collected = append(collected, in)
	}

	return collected, nil
}

// resolveSecretUserContent returns passwd fetched from a referenced secret object.
func (r *OCNEConfigReconciler) resolveSecretPasswordContent(ctx context.Context, ns string, source bootstrapv1.User) ([]byte, error) {
	secret := &corev1.Secret{}
	key := types.NamespacedName{Namespace: ns, Name: source.PasswdFrom.Secret.Name}
	if err := r.Client.Get(ctx, key, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, errors.Wrapf(err, "secret not found: %s", key)
		}
		return nil, errors.Wrapf(err, "failed to retrieve Secret %q", key)
	}
	data, ok := secret.Data[source.PasswdFrom.Secret.Key]
	if !ok {
		return nil, errors.Errorf("secret references non-existent secret key: %q", source.PasswdFrom.Secret.Key)
	}
	return data, nil
}

// ClusterToOCNEConfigs is a handler.ToRequestsFunc to be used to enqueue
// requests for reconciliation of KubeadmConfigs.
func (r *OCNEConfigReconciler) ClusterToOCNEConfigs(o client.Object) []ctrl.Request {
	result := []ctrl.Request{}

	c, ok := o.(*clusterv1.Cluster)
	if !ok {
		panic(fmt.Sprintf("Expected a Cluster but got a %T", o))
	}

	selectors := []client.ListOption{
		client.InNamespace(c.Namespace),
		client.MatchingLabels{
			clusterv1.ClusterLabelName: c.Name,
		},
	}

	machineList := &clusterv1.MachineList{}
	if err := r.Client.List(context.TODO(), machineList, selectors...); err != nil {
		return nil
	}

	for _, m := range machineList.Items {
		if m.Spec.Bootstrap.ConfigRef != nil &&
			m.Spec.Bootstrap.ConfigRef.GroupVersionKind().GroupKind() == bootstrapv1.GroupVersion.WithKind("OCNEConfig").GroupKind() {
			name := client.ObjectKey{Namespace: m.Namespace, Name: m.Spec.Bootstrap.ConfigRef.Name}
			result = append(result, ctrl.Request{NamespacedName: name})
		}
	}

	if feature.Gates.Enabled(feature.MachinePool) {
		machinePoolList := &expv1.MachinePoolList{}
		if err := r.Client.List(context.TODO(), machinePoolList, selectors...); err != nil {
			return nil
		}

		for _, mp := range machinePoolList.Items {
			if mp.Spec.Template.Spec.Bootstrap.ConfigRef != nil &&
				mp.Spec.Template.Spec.Bootstrap.ConfigRef.GroupVersionKind().GroupKind() == bootstrapv1.GroupVersion.WithKind("OCNEConfig").GroupKind() {
				name := client.ObjectKey{Namespace: mp.Namespace, Name: mp.Spec.Template.Spec.Bootstrap.ConfigRef.Name}
				result = append(result, ctrl.Request{NamespacedName: name})
			}
		}
	}

	return result
}

// MachineToBootstrapMapFunc is a handler.ToRequestsFunc to be used to enqueue
// request for reconciliation of OCNEConfig.
func (r *OCNEConfigReconciler) MachineToBootstrapMapFunc(o client.Object) []ctrl.Request {
	m, ok := o.(*clusterv1.Machine)
	if !ok {
		panic(fmt.Sprintf("Expected a Machine but got a %T", o))
	}

	result := []ctrl.Request{}
	if m.Spec.Bootstrap.ConfigRef != nil && m.Spec.Bootstrap.ConfigRef.GroupVersionKind() == bootstrapv1.GroupVersion.WithKind("OCNEConfig") {
		name := client.ObjectKey{Namespace: m.Namespace, Name: m.Spec.Bootstrap.ConfigRef.Name}
		result = append(result, ctrl.Request{NamespacedName: name})
	}
	return result
}

// MachinePoolToBootstrapMapFunc is a handler.ToRequestsFunc to be used to enqueue
// request for reconciliation of OCNEConfig.
func (r *OCNEConfigReconciler) MachinePoolToBootstrapMapFunc(o client.Object) []ctrl.Request {
	m, ok := o.(*expv1.MachinePool)
	if !ok {
		panic(fmt.Sprintf("Expected a MachinePool but got a %T", o))
	}

	result := []ctrl.Request{}
	configRef := m.Spec.Template.Spec.Bootstrap.ConfigRef
	if configRef != nil && configRef.GroupVersionKind().GroupKind() == bootstrapv1.GroupVersion.WithKind("OCNEConfig").GroupKind() {
		name := client.ObjectKey{Namespace: m.Namespace, Name: configRef.Name}
		result = append(result, ctrl.Request{NamespacedName: name})
	}
	return result
}

// reconcileDiscovery ensures that config.JoinConfiguration.Discovery is properly set for the joining node.
// The implementation func respect user provided discovery configurations, but in case some of them are missing, a valid BootstrapToken object
// is automatically injected into config.JoinConfiguration.Discovery.
// This allows to simplify configuration UX, by providing the option to delegate to CABPOCNE the configuration of ocne join discovery.
func (r *OCNEConfigReconciler) reconcileDiscovery(ctx context.Context, cluster *clusterv1.Cluster, config *bootstrapv1.OCNEConfig, certificates secret.Certificates) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	// if config already contains a file discovery configuration, respect it without further validations
	if config.Spec.JoinConfiguration.Discovery.File != nil {
		return ctrl.Result{}, nil
	}

	// otherwise it is necessary to ensure token discovery is properly configured
	if config.Spec.JoinConfiguration.Discovery.BootstrapToken == nil {
		config.Spec.JoinConfiguration.Discovery.BootstrapToken = &bootstrapv1.BootstrapTokenDiscovery{}
	}

	// calculate the ca cert hashes if they are not already set
	if len(config.Spec.JoinConfiguration.Discovery.BootstrapToken.CACertHashes) == 0 {
		hashes, err := certificates.GetByPurpose(secret.ClusterCA).Hashes()
		if err != nil {
			log.Error(err, "Unable to generate Cluster CA certificate hashes")
			return ctrl.Result{}, err
		}
		config.Spec.JoinConfiguration.Discovery.BootstrapToken.CACertHashes = hashes
	}

	// if BootstrapToken already contains an APIServerEndpoint, respect it; otherwise inject the APIServerEndpoint endpoint defined in cluster status
	apiServerEndpoint := config.Spec.JoinConfiguration.Discovery.BootstrapToken.APIServerEndpoint
	if apiServerEndpoint == "" {
		if !cluster.Spec.ControlPlaneEndpoint.IsValid() {
			log.V(1).Info("Waiting for Cluster Controller to set Cluster.Spec.ControlPlaneEndpoint")
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}

		apiServerEndpoint = cluster.Spec.ControlPlaneEndpoint.String()
		config.Spec.JoinConfiguration.Discovery.BootstrapToken.APIServerEndpoint = apiServerEndpoint
		log.V(3).Info("Altering JoinConfiguration.Discovery.BootstrapToken.APIServerEndpoint", "APIServerEndpoint", apiServerEndpoint)
	}

	// if BootstrapToken already contains a token, respect it; otherwise create a new bootstrap token for the node to join
	if config.Spec.JoinConfiguration.Discovery.BootstrapToken.Token == "" {
		remoteClient, err := r.remoteClientGetter(ctx, OCNEConfigControllerName, r.Client, util.ObjectKey(cluster))
		if err != nil {
			return ctrl.Result{}, err
		}

		token, err := createToken(ctx, remoteClient, r.TokenTTL)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to create new bootstrap token")
		}

		config.Spec.JoinConfiguration.Discovery.BootstrapToken.Token = token
		log.V(3).Info("Altering JoinConfiguration.Discovery.BootstrapToken.Token")
	}

	// If the BootstrapToken does not contain any CACertHashes then force skip CA Verification
	if len(config.Spec.JoinConfiguration.Discovery.BootstrapToken.CACertHashes) == 0 {
		log.Info("No CAs were provided. Falling back to insecure discover method by skipping CA Cert validation")
		config.Spec.JoinConfiguration.Discovery.BootstrapToken.UnsafeSkipCAVerification = true
	}

	return ctrl.Result{}, nil
}

// reconcileTopLevelObjectSettings injects into config.ClusterConfiguration values from top level objects like cluster and machine.
// The implementation func respect user provided config values, but in case some of them are missing, values from top level objects are used.
func (r *OCNEConfigReconciler) reconcileTopLevelObjectSettings(ctx context.Context, cluster *clusterv1.Cluster, machine *clusterv1.Machine, config *bootstrapv1.OCNEConfig) {
	log := ctrl.LoggerFrom(ctx)

	// If there is no ControlPlaneEndpoint defined in ClusterConfiguration but
	// there is a ControlPlaneEndpoint defined at Cluster level (e.g. the load balancer endpoint),
	// then use Cluster's ControlPlaneEndpoint as a control plane endpoint for the Kubernetes cluster.
	if config.Spec.ClusterConfiguration.ControlPlaneEndpoint == "" && cluster.Spec.ControlPlaneEndpoint.IsValid() {
		config.Spec.ClusterConfiguration.ControlPlaneEndpoint = cluster.Spec.ControlPlaneEndpoint.String()
		log.V(3).Info("Altering ClusterConfiguration.ControlPlaneEndpoint", "ControlPlaneEndpoint", config.Spec.ClusterConfiguration.ControlPlaneEndpoint)
	}

	// If there are no ClusterName defined in ClusterConfiguration, use Cluster.Name
	if config.Spec.ClusterConfiguration.ClusterName == "" {
		config.Spec.ClusterConfiguration.ClusterName = cluster.Name
		log.V(3).Info("Altering ClusterConfiguration.ClusterName", "ClusterName", config.Spec.ClusterConfiguration.ClusterName)
	}

	// If there are no Network settings defined in ClusterConfiguration, use ClusterNetwork settings, if defined
	if cluster.Spec.ClusterNetwork != nil {
		if config.Spec.ClusterConfiguration.Networking.DNSDomain == "" && cluster.Spec.ClusterNetwork.ServiceDomain != "" {
			config.Spec.ClusterConfiguration.Networking.DNSDomain = cluster.Spec.ClusterNetwork.ServiceDomain
			log.V(3).Info("Altering ClusterConfiguration.Networking.DNSDomain", "DNSDomain", config.Spec.ClusterConfiguration.Networking.DNSDomain)
		}
		if config.Spec.ClusterConfiguration.Networking.ServiceSubnet == "" &&
			cluster.Spec.ClusterNetwork.Services != nil &&
			len(cluster.Spec.ClusterNetwork.Services.CIDRBlocks) > 0 {
			config.Spec.ClusterConfiguration.Networking.ServiceSubnet = cluster.Spec.ClusterNetwork.Services.String()
			log.V(3).Info("Altering ClusterConfiguration.Networking.ServiceSubnet", "ServiceSubnet", config.Spec.ClusterConfiguration.Networking.ServiceSubnet)
		}
		if config.Spec.ClusterConfiguration.Networking.PodSubnet == "" &&
			cluster.Spec.ClusterNetwork.Pods != nil &&
			len(cluster.Spec.ClusterNetwork.Pods.CIDRBlocks) > 0 {
			config.Spec.ClusterConfiguration.Networking.PodSubnet = cluster.Spec.ClusterNetwork.Pods.String()
			log.V(3).Info("Altering ClusterConfiguration.Networking.PodSubnet", "PodSubnet", config.Spec.ClusterConfiguration.Networking.PodSubnet)
		}
	}

	// If there are no KubernetesVersion settings defined in ClusterConfiguration, use Version from machine, if defined
	if config.Spec.ClusterConfiguration.KubernetesVersion == "" && machine.Spec.Version != nil {
		config.Spec.ClusterConfiguration.KubernetesVersion = *machine.Spec.Version
		log.V(3).Info("Altering ClusterConfiguration.KubernetesVersion", "KubernetesVersion", config.Spec.ClusterConfiguration.KubernetesVersion)
	}
}

// storeBootstrapData creates a new secret with the data passed in as input,
// sets the reference in the configuration status and ready to true.
func (r *OCNEConfigReconciler) storeBootstrapData(ctx context.Context, scope *Scope, data []byte) error {
	log := ctrl.LoggerFrom(ctx)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      scope.Config.Name,
			Namespace: scope.Config.Namespace,
			Labels: map[string]string{
				clusterv1.ClusterLabelName: scope.Cluster.Name,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: bootstrapv1.GroupVersion.String(),
					Kind:       "OCNEConfig",
					Name:       scope.Config.Name,
					UID:        scope.Config.UID,
					Controller: pointer.Bool(true),
				},
			},
		},
		Data: map[string][]byte{
			"value":  data,
			"format": []byte(scope.Config.Spec.Format),
		},
		Type: clusterv1.ClusterSecretType,
	}

	// as secret creation and scope.Config status patch are not atomic operations
	// it is possible that secret creation happens but the config.Status patches are not applied
	if err := r.Client.Create(ctx, secret); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return errors.Wrapf(err, "failed to create bootstrap data secret for OCNEConfig %s/%s", scope.Config.Namespace, scope.Config.Name)
		}
		log.Info("bootstrap data secret for OCNEConfig already exists, updating", "Secret", klog.KObj(secret))
		if err := r.Client.Update(ctx, secret); err != nil {
			return errors.Wrapf(err, "failed to update bootstrap data secret for OCNEConfig %s/%s", scope.Config.Namespace, scope.Config.Name)
		}
	}
	scope.Config.Status.DataSecretName = pointer.String(secret.Name)
	scope.Config.Status.Ready = true
	conditions.MarkTrue(scope.Config, bootstrapv1.DataSecretAvailableCondition)
	return nil
}

// Ensure the bootstrap secret has the OCNEConfig as a controller OwnerReference.
func (r *OCNEConfigReconciler) ensureBootstrapSecretOwnersRef(ctx context.Context, scope *Scope) error {
	secret := &corev1.Secret{}
	err := r.Client.Get(ctx, client.ObjectKey{Namespace: scope.Config.Namespace, Name: scope.Config.Name}, secret)
	if err != nil {
		// If the secret has not been created yet return early.
		if apierrors.IsNotFound(err) {
			return nil
		}
		return errors.Wrapf(err, "failed to add OCNEConfig %s as ownerReference to bootstrap Secret %s", scope.ConfigOwner.GetName(), secret.GetName())
	}
	patchHelper, err := patch.NewHelper(secret, r.Client)
	if err != nil {
		return errors.Wrapf(err, "failed to add OCNEConfig %s as ownerReference to bootstrap Secret %s", scope.ConfigOwner.GetName(), secret.GetName())
	}
	if c := metav1.GetControllerOf(secret); c != nil && c.Kind != "OCNEConfig" {
		secret.OwnerReferences = util.RemoveOwnerRef(secret.OwnerReferences, *c)
	}
	secret.OwnerReferences = util.EnsureOwnerRef(secret.OwnerReferences, metav1.OwnerReference{
		APIVersion: bootstrapv1.GroupVersion.String(),
		Kind:       "OCNEConfig",
		UID:        scope.Config.UID,
		Name:       scope.Config.Name,
		Controller: pointer.Bool(true),
	})
	err = patchHelper.Patch(ctx, secret)
	if err != nil {
		return errors.Wrapf(err, "could not add OCNEConfig %s as ownerReference to bootstrap Secret %s", scope.ConfigOwner.GetName(), secret.GetName())
	}
	return nil
}

// Ensures that default values are set for kubeadm cluster configuration when input is empty
func setOCNEControlPlaneClusterConfigurationDefaults(ctx context.Context, in *bootstrapv1.ClusterConfiguration, k8sVersion string) (*bootstrapv1.ClusterConfiguration, error) {
	ocneMeta, err := ocne.GetOCNEMetadata(ctx)
	if err != nil {
		return nil, err
	}

	ocneClusterConfig := in.DeepCopy()

	if in.DNS.ImageRepository == "" {
		ocneClusterConfig.DNS.ImageRepository = ocne.DefaultOCNEImageRepository
	}

	if in.DNS.ImageTag == "" {
		ocneClusterConfig.DNS.ImageTag = ocneMeta[k8sVersion].CoreDNS
	} else {
		if in.DNS.ImageTag != ocneMeta[k8sVersion].CoreDNS {
			// Set to OCNE tag only if user overrride uses containerregistry
			if in.DNS.ImageRepository == ocne.DefaultOCNEImageRepository {
				ocneClusterConfig.DNS.ImageTag = ocneMeta[k8sVersion].CoreDNS
			}
		}
	}

	etcdLocal := bootstrapv1.LocalEtcd{
		ImageMeta: bootstrapv1.ImageMeta{
			ImageTag:        ocneMeta[k8sVersion].ETCD,
			ImageRepository: ocne.DefaultOCNEImageRepository,
		},
	}

	if in.Etcd.Local == nil {
		ocneClusterConfig.Etcd.Local = &etcdLocal
	} else {
		// Verify and update user supplied values, helps in an upgrade case as well
		if in.Etcd.Local.ImageMeta.ImageRepository == ocne.DefaultOCNEImageRepository {
			if in.Etcd.Local.ImageMeta.ImageTag != ocneMeta[k8sVersion].ETCD {
				ocneClusterConfig.Etcd.Local = &etcdLocal
			}
		}
	}

	if in.ImageRepository == "" {
		ocneClusterConfig.ImageRepository = ocne.DefaultOCNEImageRepository
	}

	return ocneClusterConfig, nil
}

// Ensures that default values are set for kubeadm init configuration when input is empty
func setOCNEControlPlaneInitConfigurationDefaults(in *bootstrapv1.InitConfiguration) *bootstrapv1.InitConfiguration {
	ocneInitConfig := in.DeepCopy()
	if in.NodeRegistration.CRISocket == "" {
		ocneInitConfig.NodeRegistration.CRISocket = ocne.DefaultOCNESocket
	}
	return ocneInitConfig
}

// Ensures that default values are set for kubeadm join configuration when input is empty
func setOCNEJoinConfigurationDefaults(in *bootstrapv1.JoinConfiguration) *bootstrapv1.JoinConfiguration {
	ocneJoinConfig := in.DeepCopy()
	if in.NodeRegistration.CRISocket == "" {
		ocneJoinConfig.NodeRegistration.CRISocket = ocne.DefaultOCNESocket
	}
	return ocneJoinConfig

}
