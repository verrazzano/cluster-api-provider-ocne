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

	"github.com/pkg/errors"
	ctrl "sigs.k8s.io/controller-runtime"

	controlplanev1 "github.com/verrazzano/cluster-api-provider-ocne/controlplane/ocne/api/v1beta1"
	"github.com/verrazzano/cluster-api-provider-ocne/controlplane/ocne/internal"
	"github.com/verrazzano/cluster-api-provider-ocne/util"
	"github.com/verrazzano/cluster-api-provider-ocne/util/collections"
	"github.com/verrazzano/cluster-api-provider-ocne/util/conditions"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// updateStatus is called after every reconcilitation loop in a defer statement to always make sure we have the
// resource status subresourcs up-to-date.
func (r *OCNEControlPlaneReconciler) updateStatus(ctx context.Context, ocnecp *controlplanev1.OCNEControlPlane, cluster *clusterv1.Cluster) error {
	log := ctrl.LoggerFrom(ctx)

	selector := collections.ControlPlaneSelectorForCluster(cluster.Name)
	// Copy label selector to its status counterpart in string format.
	// This is necessary for CRDs including scale subresources.
	ocnecp.Status.Selector = selector.String()

	ownedMachines, err := r.managementCluster.GetMachinesForCluster(ctx, cluster, collections.OwnedMachines(ocnecp))
	if err != nil {
		return errors.Wrap(err, "failed to get list of owned machines")
	}

	controlPlane, err := internal.NewControlPlane(ctx, r.Client, cluster, ocnecp, ownedMachines)
	if err != nil {
		log.Error(err, "failed to initialize control plane")
		return err
	}
	ocnecp.Status.UpdatedReplicas = int32(len(controlPlane.UpToDateMachines()))

	replicas := int32(len(ownedMachines))
	desiredReplicas := *ocnecp.Spec.Replicas

	// set basic data that does not require interacting with the workload cluster
	ocnecp.Status.Replicas = replicas
	ocnecp.Status.ReadyReplicas = 0
	ocnecp.Status.UnavailableReplicas = replicas

	// Return early if the deletion timestamp is set, because we don't want to try to connect to the workload cluster
	// and we don't want to report resize condition (because it is set to deleting into reconcile delete).
	if !ocnecp.DeletionTimestamp.IsZero() {
		return nil
	}

	machinesWithHealthAPIServer := ownedMachines.Filter(collections.HealthyAPIServer())
	lowestVersion := machinesWithHealthAPIServer.LowestVersion()
	if lowestVersion != nil {
		ocnecp.Status.Version = lowestVersion
	}

	switch {
	// We are scaling up
	case replicas < desiredReplicas:
		conditions.MarkFalse(ocnecp, controlplanev1.ResizedCondition, controlplanev1.ScalingUpReason, clusterv1.ConditionSeverityWarning, "Scaling up control plane to %d replicas (actual %d)", desiredReplicas, replicas)
	// We are scaling down
	case replicas > desiredReplicas:
		conditions.MarkFalse(ocnecp, controlplanev1.ResizedCondition, controlplanev1.ScalingDownReason, clusterv1.ConditionSeverityWarning, "Scaling down control plane to %d replicas (actual %d)", desiredReplicas, replicas)

		// This means that there was no error in generating the desired number of machine objects
		conditions.MarkTrue(ocnecp, controlplanev1.MachinesCreatedCondition)
	default:
		// make sure last resize operation is marked as completed.
		// NOTE: we are checking the number of machines ready so we report resize completed only when the machines
		// are actually provisioned (vs reporting completed immediately after the last machine object is created).
		readyMachines := ownedMachines.Filter(collections.IsReady())
		if int32(len(readyMachines)) == replicas {
			conditions.MarkTrue(ocnecp, controlplanev1.ResizedCondition)
		}

		// This means that there was no error in generating the desired number of machine objects
		conditions.MarkTrue(ocnecp, controlplanev1.MachinesCreatedCondition)
	}

	workloadCluster, err := r.managementCluster.GetWorkloadCluster(ctx, util.ObjectKey(cluster))
	if err != nil {
		return errors.Wrap(err, "failed to create remote cluster client")
	}
	status, err := workloadCluster.ClusterStatus(ctx)
	if err != nil {
		return err
	}
	ocnecp.Status.ReadyReplicas = status.ReadyNodes
	ocnecp.Status.UnavailableReplicas = replicas - status.ReadyNodes

	// This only gets initialized once and does not change if the kubeadm config map goes away.
	if status.HasOCNEConfig {
		ocnecp.Status.Initialized = true
		conditions.MarkTrue(ocnecp, controlplanev1.AvailableCondition)
	}

	if ocnecp.Status.ReadyReplicas > 0 {
		ocnecp.Status.Ready = true
	}

	return nil
}
