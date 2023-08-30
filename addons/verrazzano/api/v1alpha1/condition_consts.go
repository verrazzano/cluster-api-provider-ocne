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

package v1alpha1

import clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"

// VerrazzanoRelease Conditions and Reasons.
const (
	// VerrazzanoReleaseBindingSpecsUpToDateCondition indicates that the VerrazzanoReleaseBinding specs are up to date with the VerrazzanoRelease specs,
	// meaning that the VerrazzanoReleaseBindings are created/updated, value template parsing succeeded, and the orphaned VerrazzanoReleaseBindings are deleted.
	VerrazzanoReleaseBindingSpecsUpToDateCondition clusterv1.ConditionType = "VerrazzanoReleaseBindingSpecsUpToDate"

	// VerrazzanoReleaseBindingCreationFailedReason indicates that the VerrazzanoRelease controller failed to create a VerrazzanoReleaseBinding.
	VerrazzanoReleaseBindingCreationFailedReason = "VerrazzanoReleaseBindingCreationFailed"

	// VerrazzanoReleaseBindingDeletionFailedReason indicates that the VerrazzanoRelease controller failed to delete a VerrazzanoReleaseBinding.
	VerrazzanoReleaseBindingDeletionFailedReason = "VerrazzanoReleaseBindingDeletionFailed"

	// VerrazzanoReleaseBindingReinstallingReason indicates that the VerrazzanoRelease controller is reinstalling a VerrazzanoReleaseBinding.
	VerrazzanoReleaseBindingReinstallingReason = "VerrazzanoReleaseBindingReinstalling"

	// ValueParsingFailedReason indicates that the VerrazzanoRelease controller failed to parse the values.
	ValueParsingFailedReason = "ValueParsingFailed"

	// ClusterSelectionFailedReason indicates that the VerrazzanoRelease controller failed to select the workload Clusters.
	ClusterSelectionFailedReason = "ClusterSelectionFailed"

	// VerrazzanoReleaseBindingsReadyCondition indicates that the VerrazzanoReleaseBindings are ready, meaning that the Helm installation, upgrade
	// or deletion is complete.
	VerrazzanoReleaseBindingsReadyCondition clusterv1.ConditionType = "VerrazzanoReleaseBindingsReady"
)

// VerrazzanoReleaseBinding Conditions and Reasons.
const (
	// HelmReleaseReadyCondition indicates the current status of the underlying Helm release managed by the VerrazzanoReleaseBinding.
	HelmReleaseReadyCondition clusterv1.ConditionType = "HelmReleaseReady"

	// PreparingToHelmInstallReason indicates that the VerrazzanoReleaseBinding is preparing to install the Helm release.
	PreparingToHelmInstallReason = "PreparingToHelmInstall"

	// HelmReleasePendingReason indicates that the VerrazzanoReleaseBinding is pending either install, upgrade, or rollback.
	HelmReleasePendingReason = "HelmReleasePending"

	// HelmInstallOrUpgradeFailedReason indicates that the VerrazzanoReleaseBinding failed to install or upgrade the Helm release.
	HelmInstallOrUpgradeFailedReason = "HelmInstallOrUpgradeFailed"

	// HelmReleaseDeletionFailedReason is indicates that the VerrazzanoReleaseBinding failed to delete the Helm release.
	HelmReleaseDeletionFailedReason = "HelmReleaseDeletionFailed"

	// HelmReleaseDeletedReason indicates that the VerrazzanoReleaseBinding deleted the Helm release.
	HelmReleaseDeletedReason = "HelmReleaseDeleted"

	// HelmReleaseGetFailedReason indicates that the VerrazzanoReleaseBinding failed to get the Helm release.
	HelmReleaseGetFailedReason = "HelmReleaseGetFailed"

	// ClusterAvailableCondition indicates that the Cluster to install the Helm release on is available.
	ClusterAvailableCondition clusterv1.ConditionType = "ClusterAvailable"

	// GetClusterFailedReason indicates that the VerrazzanoReleaseBinding failed to get the Cluster.
	GetClusterFailedReason = "GetClusterFailed"

	// GetKubeconfigFailedReason indicates that the VerrazzanoReleaseBinding failed to get the kubeconfig for the Cluster.
	GetKubeconfigFailedReason = "GetKubeconfigFailed"
)
