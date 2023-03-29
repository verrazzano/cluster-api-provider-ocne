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

package internal

import (
	"context"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apiserver/pkg/storage/names"
	"sigs.k8s.io/controller-runtime/pkg/client"

	bootstrapv1 "github.com/verrazzano/cluster-api-provider-ocne/bootstrap/ocne/api/v1beta1"
	controlplanev1 "github.com/verrazzano/cluster-api-provider-ocne/controlplane/ocne/api/v1beta1"
	"github.com/verrazzano/cluster-api-provider-ocne/util/collections"
	"github.com/verrazzano/cluster-api-provider-ocne/util/failuredomains"
	"github.com/verrazzano/cluster-api-provider-ocne/util/patch"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
)

// ControlPlane holds business logic around control planes.
// It should never need to connect to a service, that responsibility lies outside of this struct.
// Going forward we should be trying to add more logic to here and reduce the amount of logic in the reconciler.
type ControlPlane struct {
	KCP                  *controlplanev1.OCNEControlPlane
	Cluster              *clusterv1.Cluster
	Machines             collections.Machines
	machinesPatchHelpers map[string]*patch.Helper

	// reconciliationTime is the time of the current reconciliation, and should be used for all "now" calculations
	reconciliationTime metav1.Time

	// TODO: we should see if we can combine these with the Machine objects so we don't have all these separate lookups
	// See discussion on https://github.com/kubernetes-sigs/cluster-api/pull/3405
	ocneConfigs    map[string]*bootstrapv1.OCNEConfig
	infraResources map[string]*unstructured.Unstructured
}

// NewControlPlane returns an instantiated ControlPlane.
func NewControlPlane(ctx context.Context, client client.Client, cluster *clusterv1.Cluster, ocnecp *controlplanev1.OCNEControlPlane, ownedMachines collections.Machines) (*ControlPlane, error) {
	infraObjects, err := getInfraResources(ctx, client, ownedMachines)
	if err != nil {
		return nil, err
	}
	ocneConfigs, err := getOcneConfigs(ctx, client, ownedMachines)
	if err != nil {
		return nil, err
	}
	patchHelpers := map[string]*patch.Helper{}
	for _, machine := range ownedMachines {
		patchHelper, err := patch.NewHelper(machine, client)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to create patch helper for machine %s", machine.Name)
		}
		patchHelpers[machine.Name] = patchHelper
	}

	return &ControlPlane{
		KCP:                  ocnecp,
		Cluster:              cluster,
		Machines:             ownedMachines,
		machinesPatchHelpers: patchHelpers,
		ocneConfigs:          ocneConfigs,
		infraResources:       infraObjects,
		reconciliationTime:   metav1.Now(),
	}, nil
}

// FailureDomains returns a slice of failure domain objects synced from the infrastructure provider into Cluster.Status.
func (c *ControlPlane) FailureDomains() clusterv1.FailureDomains {
	if c.Cluster.Status.FailureDomains == nil {
		return clusterv1.FailureDomains{}
	}
	return c.Cluster.Status.FailureDomains
}

// Version returns the OCNEControlPlane's version.
func (c *ControlPlane) Version() *string {
	return &c.KCP.Spec.Version
}

// MachineInfrastructureTemplateRef returns the OCNEControlPlane's infrastructure template for Machines.
func (c *ControlPlane) MachineInfrastructureTemplateRef() *corev1.ObjectReference {
	return &c.KCP.Spec.MachineTemplate.InfrastructureRef
}

// AsOwnerReference returns an owner reference to the OCNEControlPlane.
func (c *ControlPlane) AsOwnerReference() *metav1.OwnerReference {
	return &metav1.OwnerReference{
		APIVersion: controlplanev1.GroupVersion.String(),
		Kind:       "OCNEControlPlane",
		Name:       c.KCP.Name,
		UID:        c.KCP.UID,
	}
}

// EtcdImageData returns the etcd image data embedded in the ClusterConfiguration or empty strings if none are defined.
func (c *ControlPlane) EtcdImageData() (string, string) {
	if c.KCP.Spec.OcneConfigSpec.ClusterConfiguration != nil && c.KCP.Spec.OcneConfigSpec.ClusterConfiguration.Etcd.Local != nil {
		meta := c.KCP.Spec.OcneConfigSpec.ClusterConfiguration.Etcd.Local.ImageMeta
		return meta.ImageRepository, meta.ImageTag
	}
	return "", ""
}

// MachineInFailureDomainWithMostMachines returns the first matching failure domain with machines that has the most control-plane machines on it.
func (c *ControlPlane) MachineInFailureDomainWithMostMachines(machines collections.Machines) (*clusterv1.Machine, error) {
	fd := c.FailureDomainWithMostMachines(machines)
	machinesInFailureDomain := machines.Filter(collections.InFailureDomains(fd))
	machineToMark := machinesInFailureDomain.Oldest()
	if machineToMark == nil {
		return nil, errors.New("failed to pick control plane Machine to mark for deletion")
	}
	return machineToMark, nil
}

// MachineWithDeleteAnnotation returns a machine that has been annotated with DeleteMachineAnnotation key.
func (c *ControlPlane) MachineWithDeleteAnnotation(machines collections.Machines) collections.Machines {
	// See if there are any machines with DeleteMachineAnnotation key.
	annotatedMachines := machines.Filter(collections.HasAnnotationKey(clusterv1.DeleteMachineAnnotation))
	// If there are, return list of annotated machines.
	return annotatedMachines
}

// FailureDomainWithMostMachines returns a fd which exists both in machines and control-plane machines and has the most
// control-plane machines on it.
func (c *ControlPlane) FailureDomainWithMostMachines(machines collections.Machines) *string {
	// See if there are any Machines that are not in currently defined failure domains first.
	notInFailureDomains := machines.Filter(
		collections.Not(collections.InFailureDomains(c.FailureDomains().FilterControlPlane().GetIDs()...)),
	)
	if len(notInFailureDomains) > 0 {
		// return the failure domain for the oldest Machine not in the current list of failure domains
		// this could be either nil (no failure domain defined) or a failure domain that is no longer defined
		// in the cluster status.
		return notInFailureDomains.Oldest().Spec.FailureDomain
	}
	return failuredomains.PickMost(c.Cluster.Status.FailureDomains.FilterControlPlane(), c.Machines, machines)
}

// NextFailureDomainForScaleUp returns the failure domain with the fewest number of up-to-date machines.
func (c *ControlPlane) NextFailureDomainForScaleUp() *string {
	if len(c.Cluster.Status.FailureDomains.FilterControlPlane()) == 0 {
		return nil
	}
	return failuredomains.PickFewest(c.FailureDomains().FilterControlPlane(), c.UpToDateMachines())
}

// InitialControlPlaneConfig returns a new OCNEConfigSpec that is to be used for an initializing control plane.
func (c *ControlPlane) InitialControlPlaneConfig() *bootstrapv1.OCNEConfigSpec {
	bootstrapSpec := c.KCP.Spec.OcneConfigSpec.DeepCopy()
	bootstrapSpec.JoinConfiguration = nil
	return bootstrapSpec
}

// JoinControlPlaneConfig returns a new OCNEConfigSpec that is to be used for joining control planes.
func (c *ControlPlane) JoinControlPlaneConfig() *bootstrapv1.OCNEConfigSpec {
	bootstrapSpec := c.KCP.Spec.OcneConfigSpec.DeepCopy()
	bootstrapSpec.InitConfiguration = nil
	// NOTE: For the joining we are preserving the ClusterConfiguration in order to determine if the
	// cluster is using an external etcd in the ocne bootstrap provider (even if this is not required by ocne Join).
	// TODO: Determine if this copy of cluster configuration can be used for rollouts (thus allowing to remove the annotation at machine level)
	return bootstrapSpec
}

// GenerateOcneConfig generates a new ocne config for creating new control plane nodes.
func (c *ControlPlane) GenerateOcneConfig(spec *bootstrapv1.OCNEConfigSpec) *bootstrapv1.OCNEConfig {
	// Create an owner reference without a controller reference because the owning controller is the machine controller
	owner := metav1.OwnerReference{
		APIVersion: controlplanev1.GroupVersion.String(),
		Kind:       "OCNEControlPlane",
		Name:       c.KCP.Name,
		UID:        c.KCP.UID,
	}

	bootstrapConfig := &bootstrapv1.OCNEConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:            names.SimpleNameGenerator.GenerateName(c.KCP.Name + "-"),
			Namespace:       c.KCP.Namespace,
			Labels:          ControlPlaneMachineLabelsForCluster(c.KCP, c.Cluster.Name),
			Annotations:     c.KCP.Spec.MachineTemplate.ObjectMeta.Annotations,
			OwnerReferences: []metav1.OwnerReference{owner},
		},
		Spec: *spec,
	}
	return bootstrapConfig
}

// NewMachine returns a machine configured to be a part of the control plane.
func (c *ControlPlane) NewMachine(infraRef, bootstrapRef *corev1.ObjectReference, failureDomain *string) *clusterv1.Machine {
	return &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:        names.SimpleNameGenerator.GenerateName(c.KCP.Name + "-"),
			Namespace:   c.KCP.Namespace,
			Labels:      ControlPlaneMachineLabelsForCluster(c.KCP, c.Cluster.Name),
			Annotations: c.KCP.Spec.MachineTemplate.ObjectMeta.Annotations,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(c.KCP, controlplanev1.GroupVersion.WithKind("OCNEControlPlane")),
			},
		},
		Spec: clusterv1.MachineSpec{
			ClusterName:       c.Cluster.Name,
			Version:           c.Version(),
			InfrastructureRef: *infraRef,
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: bootstrapRef,
			},
			FailureDomain: failureDomain,
		},
	}
}

// NeedsReplacementNode determines if the control plane needs to create a replacement node during upgrade.
func (c *ControlPlane) NeedsReplacementNode() bool {
	// Can't do anything with an unknown number of desired replicas.
	if c.KCP.Spec.Replicas == nil {
		return false
	}
	// if the number of existing machines is exactly 1 > than the number of replicas.
	return len(c.Machines)+1 == int(*c.KCP.Spec.Replicas)
}

// HasDeletingMachine returns true if any machine in the control plane is in the process of being deleted.
func (c *ControlPlane) HasDeletingMachine() bool {
	return len(c.Machines.Filter(collections.HasDeletionTimestamp)) > 0
}

// GetOcneConfig returns the OCNEConfig of a given machine.
func (c *ControlPlane) GetOcneConfig(machineName string) (*bootstrapv1.OCNEConfig, bool) {
	ocneConfig, ok := c.ocneConfigs[machineName]
	return ocneConfig, ok
}

// MachinesNeedingRollout return a list of machines that need to be rolled out.
func (c *ControlPlane) MachinesNeedingRollout() collections.Machines {
	// Ignore machines to be deleted.
	machines := c.Machines.Filter(collections.Not(collections.HasDeletionTimestamp))

	// Return machines if they are scheduled for rollout or if with an outdated configuration.
	return machines.AnyFilter(
		// Machines whose certificates are about to expire.
		collections.ShouldRolloutBefore(&c.reconciliationTime, c.KCP.Spec.RolloutBefore),
		// Machines that are scheduled for rollout (KCP.Spec.RolloutAfter set, the RolloutAfter deadline is expired, and the machine was created before the deadline).
		collections.ShouldRolloutAfter(&c.reconciliationTime, c.KCP.Spec.RolloutAfter),
		// Machines that do not match with KCP config.
		collections.Not(MatchesMachineSpec(c.infraResources, c.ocneConfigs, c.KCP)),
	)
}

// UpToDateMachines returns the machines that are up to date with the control
// plane's configuration and therefore do not require rollout.
func (c *ControlPlane) UpToDateMachines() collections.Machines {
	return c.Machines.Filter(
		// Machines that shouldn't be rollout out if their certificates are not about to expire.
		collections.Not(collections.ShouldRolloutBefore(&c.reconciliationTime, c.KCP.Spec.RolloutBefore)),
		// Machines that shouldn't be rolled out after the deadline has expired.
		collections.Not(collections.ShouldRolloutAfter(&c.reconciliationTime, c.KCP.Spec.RolloutAfter)),
		// Machines that match with KCP config.
		MatchesMachineSpec(c.infraResources, c.ocneConfigs, c.KCP),
	)
}

// getInfraResources fetches the external infrastructure resource for each machine in the collection and returns a map of machine.Name -> infraResource.
func getInfraResources(ctx context.Context, cl client.Client, machines collections.Machines) (map[string]*unstructured.Unstructured, error) {
	result := map[string]*unstructured.Unstructured{}
	for _, m := range machines {
		infraObj, err := external.Get(ctx, cl, &m.Spec.InfrastructureRef, m.Namespace)
		if err != nil {
			if apierrors.IsNotFound(errors.Cause(err)) {
				continue
			}
			return nil, errors.Wrapf(err, "failed to retrieve infra obj for machine %q", m.Name)
		}
		result[m.Name] = infraObj
	}
	return result, nil
}

// getOcneConfigs fetches the ocne config for each machine in the collection and returns a map of machine.Name -> OCNEConfig.
func getOcneConfigs(ctx context.Context, cl client.Client, machines collections.Machines) (map[string]*bootstrapv1.OCNEConfig, error) {
	result := map[string]*bootstrapv1.OCNEConfig{}
	for _, m := range machines {
		bootstrapRef := m.Spec.Bootstrap.ConfigRef
		if bootstrapRef == nil {
			continue
		}
		machineConfig := &bootstrapv1.OCNEConfig{}
		if err := cl.Get(ctx, client.ObjectKey{Name: bootstrapRef.Name, Namespace: m.Namespace}, machineConfig); err != nil {
			if apierrors.IsNotFound(errors.Cause(err)) {
				continue
			}
			return nil, errors.Wrapf(err, "failed to retrieve bootstrap config for machine %q", m.Name)
		}
		result[m.Name] = machineConfig
	}
	return result, nil
}

// IsEtcdManaged returns true if the control plane relies on a managed etcd.
func (c *ControlPlane) IsEtcdManaged() bool {
	return c.KCP.Spec.OcneConfigSpec.ClusterConfiguration == nil || c.KCP.Spec.OcneConfigSpec.ClusterConfiguration.Etcd.External == nil
}

// UnhealthyMachines returns the list of control plane machines marked as unhealthy by MHC.
func (c *ControlPlane) UnhealthyMachines() collections.Machines {
	return c.Machines.Filter(collections.HasUnhealthyCondition)
}

// HealthyMachines returns the list of control plane machines not marked as unhealthy by MHC.
func (c *ControlPlane) HealthyMachines() collections.Machines {
	return c.Machines.Filter(collections.Not(collections.HasUnhealthyCondition))
}

// HasUnhealthyMachine returns true if any machine in the control plane is marked as unhealthy by MHC.
func (c *ControlPlane) HasUnhealthyMachine() bool {
	return len(c.UnhealthyMachines()) > 0
}

// PatchMachines patches all the machines conditions.
func (c *ControlPlane) PatchMachines(ctx context.Context) error {
	errList := []error{}
	for i := range c.Machines {
		machine := c.Machines[i]
		if helper, ok := c.machinesPatchHelpers[machine.Name]; ok {
			if err := helper.Patch(ctx, machine, patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
				controlplanev1.MachineAPIServerPodHealthyCondition,
				controlplanev1.MachineControllerManagerPodHealthyCondition,
				controlplanev1.MachineSchedulerPodHealthyCondition,
				controlplanev1.MachineEtcdPodHealthyCondition,
				controlplanev1.MachineEtcdMemberHealthyCondition,
			}}); err != nil {
				errList = append(errList, errors.Wrapf(err, "failed to patch machine %s", machine.Name))
			}
			continue
		}
		errList = append(errList, errors.Errorf("failed to get patch helper for machine %s", machine.Name))
	}
	return kerrors.NewAggregate(errList)
}
