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

package verrazzanorelease

import (
	"context"
	"fmt"
	"github.com/verrazzano/cluster-api-provider-ocne/addons/verrazzano/internal"

	addonsv1alpha1 "github.com/verrazzano/cluster-api-provider-ocne/addons/verrazzano/api/v1alpha1"

	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// deleteOrphanedVerrazzanoReleaseBindings deletes any VerrazzanoReleaseBinding resources that belong to a Cluster that is not selected by its parent VerrazzanoRelease.
func (r *VerrazzanoReleaseReconciler) deleteOrphanedVerrazzanoReleaseBindings(ctx context.Context, verrazzanoRelease *addonsv1alpha1.VerrazzanoRelease, clusters []clusterv1.Cluster, verrazzanoReleaseBindings []addonsv1alpha1.VerrazzanoReleaseBinding) error {
	log := ctrl.LoggerFrom(ctx)

	releasesToDelete := getOrphanedVerrazzanoReleaseBindings(ctx, clusters, verrazzanoReleaseBindings)
	log.V(2).Info("Deleting orphaned releases")
	for _, release := range releasesToDelete {
		log.V(2).Info("Deleting release", "release", release)
		if err := r.deleteVerrazzanoReleaseBinding(ctx, &release); err != nil {
			conditions.MarkFalse(verrazzanoRelease, addonsv1alpha1.VerrazzanoReleaseBindingSpecsUpToDateCondition, addonsv1alpha1.VerrazzanoReleaseBindingDeletionFailedReason, clusterv1.ConditionSeverityError, err.Error())
			return err
		}
	}

	return nil
}

// reconcileForCluster will create or update a VerrazzanoReleaseBinding for the given cluster.
func (r *VerrazzanoReleaseReconciler) reconcileForCluster(ctx context.Context, verrazzanoRelease *addonsv1alpha1.VerrazzanoRelease, cluster clusterv1.Cluster) error {
	log := ctrl.LoggerFrom(ctx)

	existingVerrazzanoReleaseBinding, err := r.getExistingVerrazzanoReleaseBinding(ctx, verrazzanoRelease, &cluster)
	if err != nil {
		// TODO: Should we set a condition here?
		return errors.Wrapf(err, "failed to get VerrazzanoReleaseBinding for cluster %s", cluster.Name)
	}
	// log.V(2).Info("Found existing VerrazzanoReleaseBinding", "cluster", cluster.Name, "release", existingVerrazzanoReleaseBinding.Name)

	if existingVerrazzanoReleaseBinding != nil && shouldReinstallHelmRelease(ctx, existingVerrazzanoReleaseBinding, verrazzanoRelease) {
		log.V(2).Info("Reinstalling Helm release by deleting and creating VerrazzanoReleaseBinding", "verrazzanoReleaseBinding", existingVerrazzanoReleaseBinding.Name)
		if err := r.deleteVerrazzanoReleaseBinding(ctx, existingVerrazzanoReleaseBinding); err != nil {
			conditions.MarkFalse(verrazzanoRelease, addonsv1alpha1.VerrazzanoReleaseBindingSpecsUpToDateCondition, addonsv1alpha1.VerrazzanoReleaseBindingDeletionFailedReason, clusterv1.ConditionSeverityError, err.Error())

			return err
		}

		// TODO: Add a check on requeue to make sure that the VerrazzanoReleaseBinding isn't still deleting
		log.V(2).Info("Successfully deleted VerrazzanoReleaseBinding on cluster, returning to requeue for reconcile", "cluster", cluster.Name)
		conditions.MarkFalse(verrazzanoRelease, addonsv1alpha1.VerrazzanoReleaseBindingSpecsUpToDateCondition, addonsv1alpha1.VerrazzanoReleaseBindingReinstallingReason, clusterv1.ConditionSeverityInfo, "VerrazzanoReleaseBinding on cluster '%s' successfully deleted, preparing to reinstall", cluster.Name)
		return nil // Try returning early so it will requeue
		// TODO: should we continue in the loop or just requeue?
	}

	values, err := internal.ParseValues(ctx, r.Client, verrazzanoRelease.Spec, &cluster)
	if err != nil {
		conditions.MarkFalse(verrazzanoRelease, addonsv1alpha1.VerrazzanoReleaseBindingSpecsUpToDateCondition, addonsv1alpha1.ValueParsingFailedReason, clusterv1.ConditionSeverityError, err.Error())

		return errors.Wrapf(err, "failed to parse values on cluster %s", cluster.Name)
	}

	log.V(2).Info("Values for cluster", "cluster", cluster.Name, "values", values)
	if err := r.createOrUpdateVerrazzanoReleaseBinding(ctx, existingVerrazzanoReleaseBinding, verrazzanoRelease, &cluster, values); err != nil {
		conditions.MarkFalse(verrazzanoRelease, addonsv1alpha1.VerrazzanoReleaseBindingSpecsUpToDateCondition, addonsv1alpha1.VerrazzanoReleaseBindingCreationFailedReason, clusterv1.ConditionSeverityError, err.Error())

		return errors.Wrapf(err, "failed to create or update VerrazzanoReleaseBinding on cluster %s", cluster.Name)
	}
	return nil
}

// getExistingVerrazzanoReleaseBinding returns the VerrazzanoReleaseBinding for the given cluster if it exists.
func (r *VerrazzanoReleaseReconciler) getExistingVerrazzanoReleaseBinding(ctx context.Context, verrazzanoRelease *addonsv1alpha1.VerrazzanoRelease, cluster *clusterv1.Cluster) (*addonsv1alpha1.VerrazzanoReleaseBinding, error) {
	log := ctrl.LoggerFrom(ctx)

	verrazzanoReleaseBindingList := &addonsv1alpha1.VerrazzanoReleaseBindingList{}

	listOpts := []client.ListOption{
		client.MatchingLabels{
			clusterv1.ClusterNameLabel:                cluster.Name,
			addonsv1alpha1.VerrazzanoReleaseLabelName: verrazzanoRelease.Name,
		},
	}

	// TODO: Figure out if we want this search to be cross-namespaces.

	log.V(2).Info("Attempting to fetch existing VerrazzanoReleaseBinding with Cluster and VerrazzanoRelease labels", "cluster", cluster.Name, "verrazzanoRelease", verrazzanoRelease.Name)
	if err := r.Client.List(context.TODO(), verrazzanoReleaseBindingList, listOpts...); err != nil {
		return nil, err
	}

	if verrazzanoReleaseBindingList.Items == nil || len(verrazzanoReleaseBindingList.Items) == 0 {
		log.V(2).Info("No VerrazzanoReleaseBinding found matching the cluster and VerrazzanoRelease", "cluster", cluster.Name, "verrazzanoRelease", verrazzanoRelease.Name)
		return nil, nil
	} else if len(verrazzanoReleaseBindingList.Items) > 1 {
		log.V(2).Info("Multiple VerrazzanoReleaseBindings found matching the cluster and VerrazzanoRelease", "cluster", cluster.Name, "verrazzanoRelease", verrazzanoRelease.Name)
		return nil, errors.Errorf("multiple VerrazzanoReleaseBindings found matching the cluster and VerrazzanoRelease")
	}

	log.V(2).Info("Found existing matching VerrazzanoReleaseBinding", "cluster", cluster.Name, "verrazzanoRelease", verrazzanoRelease.Name)

	return &verrazzanoReleaseBindingList.Items[0], nil
}

// createOrUpdateVerrazzanoReleaseBinding creates or updates the VerrazzanoReleaseBinding for the given cluster.
func (r *VerrazzanoReleaseReconciler) createOrUpdateVerrazzanoReleaseBinding(ctx context.Context, existing *addonsv1alpha1.VerrazzanoReleaseBinding, verrazzanoRelease *addonsv1alpha1.VerrazzanoRelease, cluster *clusterv1.Cluster, parsedValues string) error {
	log := ctrl.LoggerFrom(ctx)
	verrazzanoReleaseBinding := constructVerrazzanoReleaseBinding(existing, verrazzanoRelease, parsedValues, cluster)
	if verrazzanoReleaseBinding == nil {
		log.V(2).Info("VerrazzanoReleaseBinding is up to date, nothing to do", "verrazzanoReleaseBinding", existing.Name, "cluster", cluster.Name)
		return nil
	}
	if existing == nil {
		if err := r.Client.Create(ctx, verrazzanoReleaseBinding); err != nil {
			return errors.Wrapf(err, "failed to create VerrazzanoReleaseBinding '%s' for cluster: %s/%s", verrazzanoReleaseBinding.Name, cluster.Namespace, cluster.Name)
		}
	} else {
		// TODO: should this use patchVerrazzanoReleaseBinding() instead of Update() in case there's a race condition?
		if err := r.Client.Update(ctx, verrazzanoReleaseBinding); err != nil {
			return errors.Wrapf(err, "failed to update VerrazzanoReleaseBinding '%s' for cluster: %s/%s", verrazzanoReleaseBinding.Name, cluster.Namespace, cluster.Name)
		}
	}

	return nil
}

// deleteVerrazzanoReleaseBinding deletes the VerrazzanoReleaseBinding for the given cluster.
func (r *VerrazzanoReleaseReconciler) deleteVerrazzanoReleaseBinding(ctx context.Context, verrazzanoReleaseBinding *addonsv1alpha1.VerrazzanoReleaseBinding) error {
	log := ctrl.LoggerFrom(ctx)

	if err := r.Client.Delete(ctx, verrazzanoReleaseBinding); err != nil {
		if apierrors.IsNotFound(err) {
			log.V(2).Info("VerrazzanoReleaseBinding already deleted, nothing to do", "verrazzanoReleaseBinding", verrazzanoReleaseBinding.Name)
			return nil
		}
		return errors.Wrapf(err, "failed to delete verrazzanoReleaseBinding: %s", verrazzanoReleaseBinding.Name)
	}

	return nil
}

// constructVerrazzanoReleaseBinding constructs a new VerrazzanoReleaseBinding for the given Cluster or updates the existing VerrazzanoReleaseBinding if needed.
// If no update is needed, this returns nil. Note that this does not check if we need to reinstall the VerrazzanoReleaseBinding, i.e. immutable fields changed.
func constructVerrazzanoReleaseBinding(existing *addonsv1alpha1.VerrazzanoReleaseBinding, verrazzanoRelease *addonsv1alpha1.VerrazzanoRelease, parsedValues string, cluster *clusterv1.Cluster) *addonsv1alpha1.VerrazzanoReleaseBinding {
	verrazzanoReleaseBinding := &addonsv1alpha1.VerrazzanoReleaseBinding{}
	if existing == nil {
		verrazzanoReleaseBinding.GenerateName = fmt.Sprintf("%s-%s-", verrazzanoRelease.Spec.ChartName, cluster.Name)
		verrazzanoReleaseBinding.Namespace = verrazzanoRelease.Namespace
		verrazzanoReleaseBinding.OwnerReferences = util.EnsureOwnerRef(verrazzanoReleaseBinding.OwnerReferences, *metav1.NewControllerRef(verrazzanoRelease, verrazzanoRelease.GroupVersionKind()))

		newLabels := map[string]string{}
		newLabels[clusterv1.ClusterNameLabel] = cluster.Name
		newLabels[addonsv1alpha1.VerrazzanoReleaseLabelName] = verrazzanoRelease.Name
		verrazzanoReleaseBinding.Labels = newLabels

		verrazzanoReleaseBinding.Spec.ClusterRef = corev1.ObjectReference{
			Kind:       cluster.Kind,
			APIVersion: cluster.APIVersion,
			Name:       cluster.Name,
			Namespace:  cluster.Namespace,
		}

		verrazzanoReleaseBinding.Spec.ReleaseName = verrazzanoRelease.Spec.ReleaseName
		verrazzanoReleaseBinding.Spec.ChartName = verrazzanoRelease.Spec.ChartName
		verrazzanoReleaseBinding.Spec.RepoURL = verrazzanoRelease.Spec.RepoURL
		verrazzanoReleaseBinding.Spec.ReleaseNamespace = verrazzanoRelease.Spec.ReleaseNamespace
		verrazzanoReleaseBinding.Spec.Options = verrazzanoRelease.Spec.Options

		// verrazzanoRelease.ObjectMeta.SetAnnotations(verrazzanoReleaseBinding.Annotations)
	} else {
		verrazzanoReleaseBinding = existing
		changed := false
		if existing.Spec.Version != verrazzanoRelease.Spec.Version {
			changed = true
		}
		if !cmp.Equal(existing.Spec.Values, parsedValues) {
			changed = true
		}

		if !changed {
			return nil
		}
	}

	verrazzanoReleaseBinding.Spec.Version = verrazzanoRelease.Spec.Version
	verrazzanoReleaseBinding.Spec.Values = parsedValues
	verrazzanoReleaseBinding.Spec.Options = verrazzanoRelease.Spec.Options

	return verrazzanoReleaseBinding
}

// shouldReinstallHelmRelease returns true if the VerrazzanoReleaseBinding needs to be reinstalled. This is the case if any of the immutable fields changed.
func shouldReinstallHelmRelease(ctx context.Context, existing *addonsv1alpha1.VerrazzanoReleaseBinding, verrazzanoRelease *addonsv1alpha1.VerrazzanoRelease) bool {
	log := ctrl.LoggerFrom(ctx)

	log.V(2).Info("Checking if VerrazzanoReleaseBinding needs to be reinstalled by by checking if immutable fields changed", "verrazzanoReleaseBinding", existing.Name)

	annotations := existing.GetAnnotations()
	result, ok := annotations[addonsv1alpha1.IsReleaseNameGeneratedAnnotation]

	isReleaseNameGenerated := ok && result == "true"
	switch {
	case existing.Spec.ChartName != verrazzanoRelease.Spec.ChartName:
		log.V(2).Info("ChartName changed", "existing", existing.Spec.ChartName, "verrazzanoRelease", verrazzanoRelease.Spec.ChartName)
		return true
	case existing.Spec.RepoURL != verrazzanoRelease.Spec.RepoURL:
		log.V(2).Info("RepoURL changed", "existing", existing.Spec.RepoURL, "verrazzanoRelease", verrazzanoRelease.Spec.RepoURL)
		return true
	case isReleaseNameGenerated && verrazzanoRelease.Spec.ReleaseName != "":
		log.V(2).Info("Generated ReleaseName changed", "existing", existing.Spec.ReleaseName, "verrazzanoRelease", verrazzanoRelease.Spec.ReleaseName)
		return true
	case !isReleaseNameGenerated && existing.Spec.ReleaseName != verrazzanoRelease.Spec.ReleaseName:
		log.V(2).Info("Non-generated ReleaseName changed", "existing", existing.Spec.ReleaseName, "verrazzanoRelease", verrazzanoRelease.Spec.ReleaseName)
		return true
	case existing.Spec.ReleaseNamespace != verrazzanoRelease.Spec.ReleaseNamespace:
		log.V(2).Info("ReleaseNamespace changed", "existing", existing.Spec.ReleaseNamespace, "verrazzanoRelease", verrazzanoRelease.Spec.ReleaseNamespace)
		return true
	}

	return false
}

// getOrphanedVerrazzanoReleaseBindings returns a list of VerrazzanoReleaseBindings that are not associated with any of the selected Clusters for a given VerrazzanoRelease.
func getOrphanedVerrazzanoReleaseBindings(ctx context.Context, clusters []clusterv1.Cluster, verrazzanoReleaseBindings []addonsv1alpha1.VerrazzanoReleaseBinding) []addonsv1alpha1.VerrazzanoReleaseBinding {
	log := ctrl.LoggerFrom(ctx)
	log.V(2).Info("Getting VerrazzanoReleaseBindings to delete")

	selectedClusters := map[string]struct{}{}
	for _, cluster := range clusters {
		key := cluster.GetNamespace() + "/" + cluster.GetName()
		selectedClusters[key] = struct{}{}
	}
	log.V(2).Info("Selected clusters", "clusters", selectedClusters)

	releasesToDelete := []addonsv1alpha1.VerrazzanoReleaseBinding{}
	for _, verrazzanoReleaseBinding := range verrazzanoReleaseBindings {
		clusterRef := verrazzanoReleaseBinding.Spec.ClusterRef
		key := clusterRef.Namespace + "/" + clusterRef.Name
		if _, ok := selectedClusters[key]; !ok {
			releasesToDelete = append(releasesToDelete, verrazzanoReleaseBinding)
		}
	}

	names := make([]string, len(releasesToDelete))
	for _, release := range releasesToDelete {
		names = append(names, release.Name)
	}
	log.V(2).Info("Releases to delete", "releases", names)

	return releasesToDelete
}
