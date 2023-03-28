/*
Copyright 2020 The Kubernetes Authors.

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

	"github.com/blang/semver"
	"github.com/pkg/errors"
	ctrl "sigs.k8s.io/controller-runtime"

	controlplanev1 "github.com/verrazzano/cluster-api-provider-ocne/controlplane/ocne/api/v1beta1"
	"github.com/verrazzano/cluster-api-provider-ocne/controlplane/ocne/internal"
	"github.com/verrazzano/cluster-api-provider-ocne/util"
	"github.com/verrazzano/cluster-api-provider-ocne/util/collections"
	"github.com/verrazzano/cluster-api-provider-ocne/util/version"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

func (r *OcneControlPlaneReconciler) upgradeControlPlane(
	ctx context.Context,
	cluster *clusterv1.Cluster,
	kcp *controlplanev1.OcneControlPlane,
	controlPlane *internal.ControlPlane,
	machinesRequireUpgrade collections.Machines,
) (ctrl.Result, error) {
	logger := ctrl.LoggerFrom(ctx)

	if kcp.Spec.RolloutStrategy == nil || kcp.Spec.RolloutStrategy.RollingUpdate == nil {
		return ctrl.Result{}, errors.New("rolloutStrategy is not set")
	}

	// TODO: handle reconciliation of etcd members and kubeadm config in case they get out of sync with cluster

	workloadCluster, err := r.managementCluster.GetWorkloadCluster(ctx, util.ObjectKey(cluster))
	if err != nil {
		logger.Error(err, "failed to get remote client for workload cluster", "cluster key", util.ObjectKey(cluster))
		return ctrl.Result{}, err
	}

	parsedVersion, err := semver.ParseTolerant(kcp.Spec.Version)
	if err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to parse kubernetes version %q", kcp.Spec.Version)
	}

	if err := workloadCluster.ReconcileKubeletRBACRole(ctx, parsedVersion); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to reconcile the remote kubelet RBAC role")
	}

	if err := workloadCluster.ReconcileKubeletRBACBinding(ctx, parsedVersion); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to reconcile the remote kubelet RBAC binding")
	}

	// Ensure kubeadm cluster role  & bindings for v1.18+
	// as per https://github.com/kubernetes/kubernetes/commit/b117a928a6c3f650931bdac02a41fca6680548c4
	if err := workloadCluster.AllowBootstrapTokensToGetNodes(ctx); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to set role and role binding for kubeadm")
	}

	if err := workloadCluster.UpdateKubernetesVersionInOcneConfigMap(ctx, parsedVersion); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to update the kubernetes version in the kubeadm config map")
	}

	if kcp.Spec.OcneConfigSpec.ClusterConfiguration != nil {
		// We intentionally only parse major/minor/patch so that the subsequent code
		// also already applies to beta versions of new releases.
		parsedVersionTolerant, err := version.ParseMajorMinorPatchTolerant(kcp.Spec.Version)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to parse kubernetes version %q", kcp.Spec.Version)
		}
		// Get the imageRepository or the correct value if nothing is set and a migration is necessary.
		imageRepository := internal.ImageRepositoryFromClusterConfig(kcp.Spec.OcneConfigSpec.ClusterConfiguration, parsedVersionTolerant)

		if err := workloadCluster.UpdateImageRepositoryInOcneConfigMap(ctx, imageRepository, parsedVersion); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to update the image repository in the kubeadm config map")
		}
	}

	if kcp.Spec.OcneConfigSpec.ClusterConfiguration != nil && kcp.Spec.OcneConfigSpec.ClusterConfiguration.Etcd.Local != nil {
		meta := kcp.Spec.OcneConfigSpec.ClusterConfiguration.Etcd.Local.ImageMeta
		if err := workloadCluster.UpdateEtcdVersionInOcneConfigMap(ctx, meta.ImageRepository, meta.ImageTag, parsedVersion); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to update the etcd version in the kubeadm config map")
		}

		extraArgs := kcp.Spec.OcneConfigSpec.ClusterConfiguration.Etcd.Local.ExtraArgs
		if err := workloadCluster.UpdateEtcdExtraArgsInOcneConfigMap(ctx, extraArgs, parsedVersion); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to update the etcd extra args in the kubeadm config map")
		}
	}

	if kcp.Spec.OcneConfigSpec.ClusterConfiguration != nil {
		if err := workloadCluster.UpdateAPIServerInOcneConfigMap(ctx, kcp.Spec.OcneConfigSpec.ClusterConfiguration.APIServer, parsedVersion); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to update api server in the kubeadm config map")
		}

		if err := workloadCluster.UpdateControllerManagerInOcneConfigMap(ctx, kcp.Spec.OcneConfigSpec.ClusterConfiguration.ControllerManager, parsedVersion); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to update controller manager in the kubeadm config map")
		}

		if err := workloadCluster.UpdateSchedulerInOcneConfigMap(ctx, kcp.Spec.OcneConfigSpec.ClusterConfiguration.Scheduler, parsedVersion); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "failed to update scheduler in the kubeadm config map")
		}
	}

	if err := workloadCluster.UpdateKubeletConfigMap(ctx, parsedVersion); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "failed to upgrade kubelet config map")
	}

	switch kcp.Spec.RolloutStrategy.Type {
	case controlplanev1.RollingUpdateStrategyType:
		// RolloutStrategy is currently defaulted and validated to be RollingUpdate
		// We can ignore MaxUnavailable because we are enforcing health checks before we get here.
		maxNodes := *kcp.Spec.Replicas + int32(kcp.Spec.RolloutStrategy.RollingUpdate.MaxSurge.IntValue())
		if int32(controlPlane.Machines.Len()) < maxNodes {
			// scaleUp ensures that we don't continue scaling up while waiting for Machines to have NodeRefs
			return r.scaleUpControlPlane(ctx, cluster, kcp, controlPlane)
		}
		return r.scaleDownControlPlane(ctx, cluster, kcp, controlPlane, machinesRequireUpgrade)
	default:
		logger.Info("RolloutStrategy type is not set to RollingUpdateStrategyType, unable to determine the strategy for rolling out machines")
		return ctrl.Result{}, nil
	}
}
