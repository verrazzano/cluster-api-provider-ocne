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

package verrazzanofleet

import (
	"context"

	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	addonsv1alpha1 "github.com/verrazzano/cluster-api-provider-ocne/addons/verrazzano/api/v1alpha1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
)

// VerrazzanoFleetReconciler reconciles a VerrazzanoFleet object
type VerrazzanoFleetReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string
}

// SetupWithManager sets up the controller with the Manager.
func (r *VerrazzanoFleetReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	log := ctrl.LoggerFrom(ctx)

	c, err := ctrl.NewControllerManagedBy(mgr).
		WithOptions(options).
		For(&addonsv1alpha1.VerrazzanoFleet{}).
		WithEventFilter(predicates.ResourceNotPausedAndHasFilterLabel(log, r.WatchFilterValue)).
		Build(r)
	if err != nil {
		return errors.Wrap(err, "error creating controller")
	}

	// Add a watch on clusterv1.Cluster object for changes.
	if err = c.Watch(
		source.Kind(mgr.GetCache(), &clusterv1.Cluster{}),
		handler.EnqueueRequestsFromMapFunc(r.ClusterToVerrazzanoFleetMapper),
		predicates.ResourceNotPausedAndHasFilterLabel(log, r.WatchFilterValue),
	); err != nil {
		return errors.Wrap(err, "failed adding a watch for Clusters")
	}

	// Add a watch on VerrazzanoFleetBinding object for changes.
	if err = c.Watch(
		source.Kind(mgr.GetCache(), &addonsv1alpha1.VerrazzanoFleetBinding{}),
		handler.EnqueueRequestsFromMapFunc(VerrazzanoFleetBindingToHelmChartProxyMapper),
		predicates.ResourceNotPausedAndHasFilterLabel(log, r.WatchFilterValue),
	); err != nil {
		return errors.Wrap(err, "failed adding a watch for VerrazzanoFleetBindings")
	}

	return nil
}

//+kubebuilder:rbac:groups=addons.cluster.x-k8s.io,resources=verrazzanofleets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=addons.cluster.x-k8s.io,resources=verrazzanofleets/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=addons.cluster.x-k8s.io,resources=verrazzanofleets/finalizers,verbs=update
//+kubebuilder:rbac:groups=addons.cluster.x-k8s.io,resources=verrazzanofleetbindings,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters,verbs=list;watch
//+kubebuilder:rbac:groups=controlplane.cluster.x-k8s.io,resources=kubeadmcontrolplanes,verbs=list;get;watch
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io;bootstrap.cluster.x-k8s.io;controlplane.cluster.x-k8s.io;clusterctl.cluster.x-k8s.io,resources=*,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=namespaces,verbs=list;

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *VerrazzanoFleetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	log := ctrl.LoggerFrom(ctx)

	log.V(2).Info("Beginning reconcilation for VerrazzanoFleet", "requestNamespace", req.Namespace, "requestName", req.Name)

	// Fetch the VerrazzanoFleet instance.
	verrazzanoFleet := &addonsv1alpha1.VerrazzanoFleet{}
	if err := r.Client.Get(ctx, req.NamespacedName, verrazzanoFleet); err != nil {
		if apierrors.IsNotFound(err) {
			log.V(2).Info("VerrazzanoFleet resource not found, skipping reconciliation", "verrazzanoFleet", req.NamespacedName)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// TODO: should patch helper return an error when the object has been deleted?
	patchHelper, err := patch.NewHelper(verrazzanoFleet, r.Client)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to init patch helper")
	}

	defer func() {
		log.V(2).Info("Preparing to patch VerrazzanoFleet", "verrazzanoFleet", verrazzanoFleet.Name)
		if err := patchHelmChartProxy(ctx, patchHelper, verrazzanoFleet); err != nil && reterr == nil {
			reterr = err
			log.Error(err, "failed to patch VerrazzanoFleet", "verrazzanoFleet", verrazzanoFleet.Name)
			return
		}
		log.V(2).Info("Successfully patched VerrazzanoFleet", "verrazzanoFleet", verrazzanoFleet.Name)
	}()

	selector := verrazzanoFleet.Spec.ClusterSelector

	log.V(2).Info("Finding matching clusters for VerrazzanoFleet with selector selector", "verrazzanoFleet", verrazzanoFleet.Name, "selector", selector)
	// TODO: When a Cluster is being deleted, it will show up in the list of clusters even though we can't Reconcile on it.
	// This is because of ownerRefs and how the Cluster gets deleted. It will be eventually consistent but it would be better
	// to not have errors. An idea would be to check the deletion timestamp.
	clusterList, err := r.listClustersWithLabels(ctx, verrazzanoFleet.Namespace, selector)
	if err != nil {
		conditions.MarkFalse(verrazzanoFleet, addonsv1alpha1.VerrazzanoFleetBindingSpecsUpToDateCondition, addonsv1alpha1.ClusterSelectionFailedReason, clusterv1.ConditionSeverityError, err.Error())

		return ctrl.Result{}, err
	}
	// conditions.MarkTrue(verrazzanoFleet, addonsv1alpha1.VerrazzanoFleetBindingSpecsReadyCondition)
	verrazzanoFleet.SetMatchingClusters(clusterList.Items)

	log.V(2).Info("Finding HelmRelease for VerrazzanoFleet", "verrazzanoFleet", verrazzanoFleet.Name)
	label := map[string]string{
		addonsv1alpha1.VerrazzanoFleetLabelName: verrazzanoFleet.Name,
	}
	releaseList, err := r.listInstalledReleases(ctx, verrazzanoFleet.Namespace, label)
	if err != nil {
		return ctrl.Result{}, err
	}

	// examine DeletionTimestamp to determine if object is under deletion
	if verrazzanoFleet.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object. This is equivalent
		// registering our finalizer.
		if !controllerutil.ContainsFinalizer(verrazzanoFleet, addonsv1alpha1.VerrazzanoFleetFinalizer) {
			controllerutil.AddFinalizer(verrazzanoFleet, addonsv1alpha1.VerrazzanoFleetFinalizer)
			if err := patchHelmChartProxy(ctx, patchHelper, verrazzanoFleet); err != nil {
				// TODO: Should we try to set the error here? If we can't add the finalizer we likely can't update the status either.
				return ctrl.Result{}, err
			}
		}
	} else {
		// The object is being deleted
		if controllerutil.ContainsFinalizer(verrazzanoFleet, addonsv1alpha1.VerrazzanoFleetFinalizer) {
			// our finalizer is present, so lets handle any external dependency
			if err := r.reconcileDelete(ctx, verrazzanoFleet, releaseList.Items); err != nil {
				// if fail to delete the external dependency here, return with error
				// so that it can be retried
				return ctrl.Result{}, err
			}

			// remove our finalizer from the list and update it.
			controllerutil.RemoveFinalizer(verrazzanoFleet, addonsv1alpha1.VerrazzanoFleetFinalizer)
			if err := patchHelmChartProxy(ctx, patchHelper, verrazzanoFleet); err != nil {
				// TODO: Should we try to set the error here? If we can't remove the finalizer we likely can't update the status either.
				return ctrl.Result{}, err
			}
		}

		// Stop reconciliation as the item is being deleted
		return ctrl.Result{}, nil
	}

	log.V(2).Info("Reconciling VerrazzanoFleet", "randomName", verrazzanoFleet.Name)
	err = r.reconcileNormal(ctx, verrazzanoFleet, clusterList.Items, releaseList.Items)
	if err != nil {
		return ctrl.Result{}, err
	}
	conditions.MarkTrue(verrazzanoFleet, addonsv1alpha1.VerrazzanoFleetBindingSpecsUpToDateCondition)

	err = r.aggregateVerrazzanoFleetBindingReadyCondition(ctx, verrazzanoFleet)
	if err != nil {
		log.Error(err, "failed to aggregate VerrazzanoFleetBinding ready condition", "verrazzanoFleet", verrazzanoFleet.Name)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// reconcileNormal handles the reconciliation of a VerrazzanoFleet when it is not being deleted. It takes a list of selected Clusters and VerrazzanoFleetBindings
// to uninstall the Helm chart from any Clusters that are no longer selected and to install or update the Helm chart on any Clusters that currently selected.
func (r *VerrazzanoFleetReconciler) reconcileNormal(ctx context.Context, verrazzanoFleet *addonsv1alpha1.VerrazzanoFleet, clusters []clusterv1.Cluster, verrazzanoFleetBindings []addonsv1alpha1.VerrazzanoFleetBinding) error {
	log := ctrl.LoggerFrom(ctx)

	log.V(2).Info("Starting reconcileNormal for chart proxy", "name", verrazzanoFleet.Name)

	err := r.deleteOrphanedVerrazzanoFleetBindings(ctx, verrazzanoFleet, clusters, verrazzanoFleetBindings)
	if err != nil {
		return err
	}

	for _, cluster := range clusters {
		// Don't reconcile if the Cluster is being deleted
		if !cluster.ObjectMeta.DeletionTimestamp.IsZero() {
			continue
		}

		err := r.reconcileForCluster(ctx, verrazzanoFleet, cluster)
		if err != nil {
			return err
		}
	}

	return nil
}

// reconcileDelete handles the deletion of a VerrazzanoFleet. It takes a list of VerrazzanoFleetBindings to uninstall the Helm chart from all selected Clusters.
func (r *VerrazzanoFleetReconciler) reconcileDelete(ctx context.Context, verrazzanoFleet *addonsv1alpha1.VerrazzanoFleet, releases []addonsv1alpha1.VerrazzanoFleetBinding) error {
	log := ctrl.LoggerFrom(ctx)

	log.V(2).Info("Deleting all VerrazzanoFleetBindings as part of VerrazzanoFleet deletion", "verrazzanoFleet", verrazzanoFleet.Name)

	for _, release := range releases {
		log.V(2).Info("Deleting release", "releaseName", release.Name, "cluster", release.Spec.ClusterRef.Name)
		if err := r.deleteVerrazzanoFleetBinding(ctx, &release); err != nil {
			// TODO: will this fail if clusterRef is nil
			return errors.Wrapf(err, "failed to delete release %s from cluster %s", release.Name, release.Spec.ClusterRef.Name)
		}
	}

	return nil
}

// listClustersWithLabels returns a list of Clusters that match the given label selector.
func (r *VerrazzanoFleetReconciler) listClustersWithLabels(ctx context.Context, namespace string, selector metav1.LabelSelector) (*clusterv1.ClusterList, error) {
	clusterList := &clusterv1.ClusterList{}
	// To support for the matchExpressions field, convert LabelSelector to labels.Selector to specify labels.Selector for ListOption. (Issue #15)
	labelselector, err := metav1.LabelSelectorAsSelector(&selector)
	if err != nil {
		return nil, err
	}

	if err := r.Client.List(ctx, clusterList, client.InNamespace(namespace), client.MatchingLabelsSelector{Selector: labelselector}); err != nil {
		return nil, err
	}

	return clusterList, nil
}

// listInstalledReleases returns a list of VerrazzanoFleetBindings that match the given label selector.
func (r *VerrazzanoFleetReconciler) listInstalledReleases(ctx context.Context, namespace string, labels map[string]string) (*addonsv1alpha1.VerrazzanoFleetBindingList, error) {
	releaseList := &addonsv1alpha1.VerrazzanoFleetBindingList{}

	// TODO: should we use client.MatchingLabels or try to use the labelSelector itself?
	if err := r.Client.List(ctx, releaseList, client.InNamespace(namespace), client.MatchingLabels(labels)); err != nil {
		return nil, err
	}

	return releaseList, nil
}

// aggregateVerrazzanoFleetBindingReadyCondition VerrazzanoFleetBindingReadyCondition from all VerrazzanoFleetBindings that match the given label selector.
func (r *VerrazzanoFleetReconciler) aggregateVerrazzanoFleetBindingReadyCondition(ctx context.Context, verrazzanoFleet *addonsv1alpha1.VerrazzanoFleet) error {
	log := ctrl.LoggerFrom(ctx)

	log.V(2).Info("Aggregating VerrazzanoFleetBindingReadyCondition")

	labels := map[string]string{
		addonsv1alpha1.VerrazzanoFleetLabelName: verrazzanoFleet.Name,
	}
	releaseList, err := r.listInstalledReleases(ctx, verrazzanoFleet.Namespace, labels)
	if err != nil {
		// conditions.MarkFalse(verrazzanoFleet, addonsv1alpha1.VerrazzanoFleetBindingsReadyCondition, addonsv1alpha1.VerrazzanoFleetBindingListFailedReason, clusterv1.ConditionSeverityError, err.Error())
		return err
	}

	if len(releaseList.Items) == 0 {
		// Consider it to be vacuously true if there are no releases. This should only be reached if we previously had VerrazzanoFleetBindings but they were all deleted
		// due to the Clusters being unselected. In that case, we should consider the condition to be true.
		conditions.MarkTrue(verrazzanoFleet, addonsv1alpha1.VerrazzanoFleetBindingsReadyCondition)
		return nil
	}

	getters := make([]conditions.Getter, 0, len(releaseList.Items))
	for _, r := range releaseList.Items {
		getters = append(getters, &r)
	}

	conditions.SetAggregate(verrazzanoFleet, addonsv1alpha1.VerrazzanoFleetBindingsReadyCondition, getters, conditions.AddSourceRef(), conditions.WithStepCounterIf(false))

	return nil
}

// patchHelmChartProxy patches the VerrazzanoFleet object and sets the ReadyCondition as an aggregate of the other condition set.
// TODO: Is this preferrable to client.Update() calls? Based on testing it seems like it avoids race conditions.
func patchHelmChartProxy(ctx context.Context, patchHelper *patch.Helper, verrazzanoFleet *addonsv1alpha1.VerrazzanoFleet) error {
	conditions.SetSummary(verrazzanoFleet,
		conditions.WithConditions(
			addonsv1alpha1.VerrazzanoFleetBindingSpecsUpToDateCondition,
			addonsv1alpha1.VerrazzanoFleetBindingsReadyCondition,
		),
	)

	// Patch the object, ignoring conflicts on the conditions owned by this controller.
	return patchHelper.Patch(
		ctx,
		verrazzanoFleet,
		patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
			clusterv1.ReadyCondition,
			addonsv1alpha1.VerrazzanoFleetBindingSpecsUpToDateCondition,
			addonsv1alpha1.VerrazzanoFleetBindingsReadyCondition,
		}},
		patch.WithStatusObservedGeneration{},
	)
}

// ClusterToVerrazzanoFleetMapper is a mapper function that maps a Cluster to the VerrazzanoFleets that would select the Cluster.
func (r *VerrazzanoFleetReconciler) ClusterToVerrazzanoFleetMapper(ctx context.Context, o client.Object) []ctrl.Request {
	log := ctrl.LoggerFrom(ctx)

	cluster, ok := o.(*clusterv1.Cluster)
	if !ok {
		// Suppress the error for now
		log.Error(errors.Errorf("expected a Cluster but got %T", o), "failed to map object to VerrazzanoFleet")
		return nil
	}

	verrazzanoFleets := &addonsv1alpha1.VerrazzanoFleetList{}

	// TODO: Figure out if we want this search to be cross-namespaces.

	if err := r.Client.List(ctx, verrazzanoFleets, client.InNamespace(cluster.Namespace)); err != nil {
		return nil
	}

	results := []ctrl.Request{}
	for _, verrazzanoFleet := range verrazzanoFleets.Items {
		selector, err := metav1.LabelSelectorAsSelector(&verrazzanoFleet.Spec.ClusterSelector)
		if err != nil {
			// Suppress the error for now
			log.Error(err, "failed to parse ClusterSelector for VerrazzanoFleet", "verrazzanoFleet", verrazzanoFleet.Name)
			return nil
		}

		if selector.Matches(labels.Set(cluster.Labels)) {
			results = append(results, ctrl.Request{
				// The VerrazzanoFleetBinding is always in the same namespace as the VerrazzanoFleet.
				NamespacedName: client.ObjectKey{Namespace: verrazzanoFleet.Namespace, Name: verrazzanoFleet.Name},
			})
		}
	}

	return results
}

// VerrazzanoFleetBindingToHelmChartProxyMapper is a mapper function that maps a VerrazzanoFleetBinding to the VerrazzanoFleet that owns it.
// This is used to trigger an update of the VerrazzanoFleet when a VerrazzanoFleetBinding is changed.
func VerrazzanoFleetBindingToHelmChartProxyMapper(ctx context.Context, o client.Object) []ctrl.Request {
	log := ctrl.LoggerFrom(ctx)

	verrazzanoFleetBinding, ok := o.(*addonsv1alpha1.VerrazzanoFleetBinding)
	if !ok {
		// Suppress the error for now
		log.Error(errors.Errorf("expected a VerrazzanoFleetBinding but got %T", o), "failed to map object to VerrazzanoFleet")
		return nil
	}

	// Check if the controller reference is already set and
	// return an empty result when one is found.
	for _, ref := range verrazzanoFleetBinding.ObjectMeta.OwnerReferences {
		if ref.Controller != nil && *ref.Controller {
			name := client.ObjectKey{
				Namespace: verrazzanoFleetBinding.GetNamespace(),
				Name:      ref.Name,
			}
			return []ctrl.Request{
				{
					NamespacedName: name,
				},
			}
		}
	}

	return nil
}
