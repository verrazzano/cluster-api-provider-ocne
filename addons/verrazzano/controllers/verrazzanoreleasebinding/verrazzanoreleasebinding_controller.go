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

package verrazzanoreleasebinding

import (
	"context"
	"fmt"
	"github.com/verrazzano/cluster-api-provider-ocne/addons/verrazzano/internal"

	"github.com/pkg/errors"
	helmRelease "helm.sh/helm/v3/pkg/release"
	helmDriver "helm.sh/helm/v3/pkg/storage/driver"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	addonsv1alpha1 "github.com/verrazzano/cluster-api-provider-ocne/addons/verrazzano/api/v1alpha1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
)

// VerrazzanoReleaseBindingReconciler reconciles a VerrazzanoReleaseBinding object
type VerrazzanoReleaseBindingReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string
}

// SetupWithManager sets up the controller with the Manager.
func (r *VerrazzanoReleaseBindingReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	log := ctrl.LoggerFrom(ctx)

	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(options).
		For(&addonsv1alpha1.VerrazzanoReleaseBinding{}).
		WithEventFilter(predicate.GenerationChangedPredicate{}).
		WithEventFilter(predicates.ResourceNotPausedAndHasFilterLabel(log, r.WatchFilterValue)).
		Complete(r)
}

//+kubebuilder:rbac:groups=addons.cluster.x-k8s.io,resources=verrazzanoreleasebindings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=addons.cluster.x-k8s.io,resources=verrazzanoreleasebindings/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=addons.cluster.x-k8s.io,resources=verrazzanoreleasebindings/finalizers,verbs=update
//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters,verbs=get;watch
//+kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;watch
//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=secrets,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io;bootstrap.cluster.x-k8s.io;controlplane.cluster.x-k8s.io;clusterctl.cluster.x-k8s.io,resources=*,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *VerrazzanoReleaseBindingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	log := ctrl.LoggerFrom(ctx)

	log.V(2).Info("Beginning reconcilation for VerrazzanoReleaseBinding", "requestNamespace", req.Namespace, "requestName", req.Name)

	// Fetch the VerrazzanoReleaseBinding instance.
	verrazzanoReleaseBinding := &addonsv1alpha1.VerrazzanoReleaseBinding{}
	if err := r.Client.Get(ctx, req.NamespacedName, verrazzanoReleaseBinding); err != nil {
		if apierrors.IsNotFound(err) {
			log.V(2).Info("VerrazzanoReleaseBinding resource not found, skipping reconciliation", "verrazzanoReleaseBinding", req.NamespacedName)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// TODO: should patch helper return an error when the object has been deleted?
	patchHelper, err := patch.NewHelper(verrazzanoReleaseBinding, r.Client)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to init patch helper")
	}

	initalizeConditions(ctx, patchHelper, verrazzanoReleaseBinding)

	defer func() {
		log.V(2).Info("Preparing to patch VerrazzanoReleaseBinding with return error", "verrazzanoReleaseBinding", verrazzanoReleaseBinding.Name, "reterr", reterr)
		if err := patchVerrazzanoReleaseBinding(ctx, patchHelper, verrazzanoReleaseBinding); err != nil && reterr == nil {
			reterr = err
			log.Error(err, "failed to patch VerrazzanoReleaseBinding", "verrazzanoReleaseBinding", verrazzanoReleaseBinding.Name)
			return
		}
		log.V(2).Info("Successfully patched VerrazzanoReleaseBinding", "verrazzanoReleaseBinding", verrazzanoReleaseBinding.Name)
	}()

	cluster := &clusterv1.Cluster{}
	clusterKey := client.ObjectKey{
		Namespace: verrazzanoReleaseBinding.Spec.ClusterRef.Namespace,
		Name:      verrazzanoReleaseBinding.Spec.ClusterRef.Name,
	}

	k := internal.KubeconfigGetter{}
	client := &internal.HelmClient{}

	// examine DeletionTimestamp to determine if object is under deletion
	if verrazzanoReleaseBinding.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object. This is equivalent
		// registering our finalizer.
		if !controllerutil.ContainsFinalizer(verrazzanoReleaseBinding, addonsv1alpha1.VerrazzanoReleaseBindingFinalizer) {
			controllerutil.AddFinalizer(verrazzanoReleaseBinding, addonsv1alpha1.VerrazzanoReleaseBindingFinalizer)
			if err := patchVerrazzanoReleaseBinding(ctx, patchHelper, verrazzanoReleaseBinding); err != nil {
				// TODO: Should we try to set the error here? If we can't remove the finalizer we likely can't update the status either.
				return ctrl.Result{}, err
			}
		}
	} else {
		// The object is being deleted
		if controllerutil.ContainsFinalizer(verrazzanoReleaseBinding, addonsv1alpha1.VerrazzanoReleaseBindingFinalizer) {
			// our finalizer is present, so lets handle any external dependency
			if err := r.Client.Get(ctx, clusterKey, cluster); err == nil {
				log.V(2).Info("Getting kubeconfig for cluster", "cluster", cluster.Name)
				kubeconfig, err := k.GetClusterKubeconfig(ctx, cluster)
				if err != nil {
					wrappedErr := errors.Wrapf(err, "failed to get kubeconfig for cluster")
					conditions.MarkFalse(verrazzanoReleaseBinding, addonsv1alpha1.ClusterAvailableCondition, addonsv1alpha1.GetKubeconfigFailedReason, clusterv1.ConditionSeverityError, wrappedErr.Error())

					return ctrl.Result{}, wrappedErr
				}
				conditions.MarkTrue(verrazzanoReleaseBinding, addonsv1alpha1.ClusterAvailableCondition)

				if err := r.reconcileDelete(ctx, verrazzanoReleaseBinding, client, kubeconfig); err != nil {
					// if fail to delete the external dependency here, return with error
					// so that it can be retried
					return ctrl.Result{}, err
				}
			} else if apierrors.IsNotFound(err) {
				// Cluster is gone, so we should remove our finalizer from the list and delete
				log.V(2).Info("Cluster not found, no need to delete external dependency", "cluster", cluster.Name)
				// TODO: should we set a condition here?
			} else {
				wrappedErr := errors.Wrapf(err, "failed to get cluster %s/%s", clusterKey.Namespace, clusterKey.Name)
				conditions.MarkFalse(verrazzanoReleaseBinding, addonsv1alpha1.ClusterAvailableCondition, addonsv1alpha1.GetClusterFailedReason, clusterv1.ConditionSeverityError, wrappedErr.Error())

				return ctrl.Result{}, wrappedErr
			}

			// remove our finalizer from the list and update it.
			controllerutil.RemoveFinalizer(verrazzanoReleaseBinding, addonsv1alpha1.VerrazzanoReleaseBindingFinalizer)
			if err := patchVerrazzanoReleaseBinding(ctx, patchHelper, verrazzanoReleaseBinding); err != nil {
				// TODO: Should we try to set the error here? If we can't remove the finalizer we likely can't update the status either.
				return ctrl.Result{}, err
			}
		}

		// Stop reconciliation as the item is being deleted
		return ctrl.Result{}, nil
	}

	if err := r.Client.Get(ctx, clusterKey, cluster); err != nil {
		// TODO: add check to tell if Cluster is deleted so we can remove the VerrazzanoReleaseBinding.
		wrappedErr := errors.Wrapf(err, "failed to get cluster %s/%s", clusterKey.Namespace, clusterKey.Name)
		conditions.MarkFalse(verrazzanoReleaseBinding, addonsv1alpha1.ClusterAvailableCondition, addonsv1alpha1.GetClusterFailedReason, clusterv1.ConditionSeverityError, wrappedErr.Error())

		return ctrl.Result{}, wrappedErr
	}

	log.V(2).Info("Getting kubeconfig for cluster", "cluster", cluster.Name)
	kubeconfig, err := k.GetClusterKubeconfig(ctx, cluster)
	if err != nil {
		wrappedErr := errors.Wrapf(err, "failed to get kubeconfig for cluster")
		conditions.MarkFalse(verrazzanoReleaseBinding, addonsv1alpha1.ClusterAvailableCondition, addonsv1alpha1.GetKubeconfigFailedReason, clusterv1.ConditionSeverityError, wrappedErr.Error())

		return ctrl.Result{}, wrappedErr
	}
	conditions.MarkTrue(verrazzanoReleaseBinding, addonsv1alpha1.ClusterAvailableCondition)

	log.V(2).Info("Reconciling VerrazzanoReleaseBinding", "releaseProxyName", verrazzanoReleaseBinding.Name)
	err = r.reconcileNormal(ctx, verrazzanoReleaseBinding, client, kubeconfig)

	return ctrl.Result{}, err
}

// reconcileNormal handles VerrazzanoReleaseBinding reconciliation when it is not being deleted. This will install or upgrade the VerrazzanoReleaseBinding on the Cluster.
// It will set the ReleaseName on the VerrazzanoReleaseBinding if the name is generated and also set the release status and release revision.
func (r *VerrazzanoReleaseBindingReconciler) reconcileNormal(ctx context.Context, verrazzanoReleaseBinding *addonsv1alpha1.VerrazzanoReleaseBinding, client internal.Client, kubeconfig string) error {
	log := ctrl.LoggerFrom(ctx)

	log.V(2).Info("Reconciling VerrazzanoReleaseBinding on cluster", "VerrazzanoReleaseBinding", verrazzanoReleaseBinding.Name, "cluster", verrazzanoReleaseBinding.Spec.ClusterRef.Name)

	// TODO: add this here or in VerrazzanoRelease controller?
	if verrazzanoReleaseBinding.Spec.ReleaseName == "" {
		verrazzanoReleaseBinding.ObjectMeta.SetAnnotations(map[string]string{
			addonsv1alpha1.IsReleaseNameGeneratedAnnotation: "true",
		})
	}
	release, err := client.InstallOrUpgradeHelmRelease(ctx, kubeconfig, verrazzanoReleaseBinding.Spec)
	if err != nil {
		log.Error(err, fmt.Sprintf("Failed to install or upgrade release '%s' on cluster %s", verrazzanoReleaseBinding.Spec.ReleaseName, verrazzanoReleaseBinding.Spec.ClusterRef.Name))
		conditions.MarkFalse(verrazzanoReleaseBinding, addonsv1alpha1.HelmReleaseReadyCondition, addonsv1alpha1.HelmInstallOrUpgradeFailedReason, clusterv1.ConditionSeverityError, err.Error())
	}
	if release != nil {
		log.V(2).Info((fmt.Sprintf("Release '%s' exists on cluster %s, revision = %d", release.Name, verrazzanoReleaseBinding.Spec.ClusterRef.Name, release.Version)))

		status := release.Info.Status
		verrazzanoReleaseBinding.SetReleaseStatus(status.String())
		verrazzanoReleaseBinding.SetReleaseRevision(release.Version)
		verrazzanoReleaseBinding.SetReleaseName(release.Name)

		if status == helmRelease.StatusDeployed {
			conditions.MarkTrue(verrazzanoReleaseBinding, addonsv1alpha1.HelmReleaseReadyCondition)
		} else if status.IsPending() {
			conditions.MarkFalse(verrazzanoReleaseBinding, addonsv1alpha1.HelmReleaseReadyCondition, addonsv1alpha1.HelmReleasePendingReason, clusterv1.ConditionSeverityInfo, fmt.Sprintf("Helm release is in a pending state: %s", status))
		} else if status == helmRelease.StatusFailed && err == nil {
			log.Info("Helm release failed without error, this might be unexpected", "release", release.Name, "cluster", verrazzanoReleaseBinding.Spec.ClusterRef.Name)
			conditions.MarkFalse(verrazzanoReleaseBinding, addonsv1alpha1.HelmReleaseReadyCondition, addonsv1alpha1.HelmInstallOrUpgradeFailedReason, clusterv1.ConditionSeverityError, fmt.Sprintf("Helm release failed: %s", status))
			// TODO: should we set the error state again here?
		}
	}

	return err
}

// reconcileDelete handles VerrazzanoReleaseBinding deletion. This will uninstall the VerrazzanoReleaseBinding on the Cluster or return nil if the VerrazzanoReleaseBinding is not found.
func (r *VerrazzanoReleaseBindingReconciler) reconcileDelete(ctx context.Context, verrazzanoReleaseBinding *addonsv1alpha1.VerrazzanoReleaseBinding, client internal.Client, kubeconfig string) error {
	log := ctrl.LoggerFrom(ctx)

	log.V(2).Info("Deleting VerrazzanoReleaseBinding on cluster", "VerrazzanoReleaseBinding", verrazzanoReleaseBinding.Name, "cluster", verrazzanoReleaseBinding.Spec.ClusterRef.Name)

	_, err := client.GetHelmRelease(ctx, kubeconfig, verrazzanoReleaseBinding.Spec)
	if err != nil {
		log.V(2).Error(err, "error getting release from cluster", "cluster", verrazzanoReleaseBinding.Spec.ClusterRef.Name)

		if err == helmDriver.ErrReleaseNotFound {
			log.V(2).Info(fmt.Sprintf("Release '%s' not found on cluster %s, nothing to do for uninstall", verrazzanoReleaseBinding.Spec.ReleaseName, verrazzanoReleaseBinding.Spec.ClusterRef.Name))
			conditions.MarkFalse(verrazzanoReleaseBinding, addonsv1alpha1.HelmReleaseReadyCondition, addonsv1alpha1.HelmReleaseDeletedReason, clusterv1.ConditionSeverityInfo, "")

			return nil
		}

		conditions.MarkFalse(verrazzanoReleaseBinding, addonsv1alpha1.HelmReleaseReadyCondition, addonsv1alpha1.HelmReleaseGetFailedReason, clusterv1.ConditionSeverityError, err.Error())

		return err
	}

	log.V(2).Info("Preparing to uninstall release on cluster", "releaseName", verrazzanoReleaseBinding.Spec.ReleaseName, "clusterName", verrazzanoReleaseBinding.Spec.ClusterRef.Name)

	response, err := client.UninstallHelmRelease(ctx, kubeconfig, verrazzanoReleaseBinding.Spec)
	if err != nil {
		log.V(2).Info("Error uninstalling chart with Helm:", err)
		conditions.MarkFalse(verrazzanoReleaseBinding, addonsv1alpha1.HelmReleaseReadyCondition, addonsv1alpha1.HelmReleaseDeletionFailedReason, clusterv1.ConditionSeverityError, err.Error())
		return errors.Wrapf(err, "error uninstalling chart with Helm on cluster %s", verrazzanoReleaseBinding.Spec.ClusterRef.Name)
	}

	log.V(2).Info((fmt.Sprintf("Chart '%s' successfully uninstalled on cluster %s", verrazzanoReleaseBinding.Spec.ChartName, verrazzanoReleaseBinding.Spec.ClusterRef.Name)))
	conditions.MarkFalse(verrazzanoReleaseBinding, addonsv1alpha1.HelmReleaseReadyCondition, addonsv1alpha1.HelmReleaseDeletedReason, clusterv1.ConditionSeverityInfo, "")
	if response != nil && response.Info != "" {
		log.V(2).Info(fmt.Sprintf("Response is %s", response.Info))
	}

	return nil
}

func initalizeConditions(ctx context.Context, patchHelper *patch.Helper, verrazzanoReleaseBinding *addonsv1alpha1.VerrazzanoReleaseBinding) {
	log := ctrl.LoggerFrom(ctx)
	if len(verrazzanoReleaseBinding.GetConditions()) == 0 {
		conditions.MarkFalse(verrazzanoReleaseBinding, addonsv1alpha1.HelmReleaseReadyCondition, addonsv1alpha1.PreparingToHelmInstallReason, clusterv1.ConditionSeverityInfo, "Preparing to to install Helm chart")
		if err := patchVerrazzanoReleaseBinding(ctx, patchHelper, verrazzanoReleaseBinding); err != nil {
			log.Error(err, "failed to patch VerrazzanoReleaseBinding with initial conditions")
		}
	}
}

// patchVerrazzanoReleaseBinding patches the VerrazzanoReleaseBinding object and sets the ReadyCondition as an aggregate of the other condition set.
// TODO: Is this preferrable to client.Update() calls? Based on testing it seems like it avoids race conditions.
func patchVerrazzanoReleaseBinding(ctx context.Context, patchHelper *patch.Helper, verrazzanoReleaseBinding *addonsv1alpha1.VerrazzanoReleaseBinding) error {
	conditions.SetSummary(verrazzanoReleaseBinding,
		conditions.WithConditions(
			addonsv1alpha1.HelmReleaseReadyCondition,
			addonsv1alpha1.ClusterAvailableCondition,
		),
	)

	// Patch the object, ignoring conflicts on the conditions owned by this controller.
	return patchHelper.Patch(
		ctx,
		verrazzanoReleaseBinding,
		patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
			clusterv1.ReadyCondition,
			addonsv1alpha1.HelmReleaseReadyCondition,
			addonsv1alpha1.ClusterAvailableCondition,
		}},
		patch.WithStatusObservedGeneration{},
	)
}
