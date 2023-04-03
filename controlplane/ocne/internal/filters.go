/*
Copyright 2021 The Kubernetes Authors.

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

// This file from the cluster-api community (https://github.com/kubernetes-sigs/cluster-api) has been modified by Oracle.

package internal

import (
	"encoding/json"
	"reflect"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	bootstrapv1 "github.com/verrazzano/cluster-api-provider-ocne/bootstrap/ocne/api/v1beta1"
	controlplanev1 "github.com/verrazzano/cluster-api-provider-ocne/controlplane/ocne/api/v1beta1"
	"github.com/verrazzano/cluster-api-provider-ocne/util/collections"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// MatchesMachineSpec returns a filter to find all machines that matches with KCP config and do not require any rollout.
// Kubernetes version, infrastructure template, and OCNEConfig field need to be equivalent.
func MatchesMachineSpec(infraConfigs map[string]*unstructured.Unstructured, machineConfigs map[string]*bootstrapv1.OCNEConfig, ocnecp *controlplanev1.OCNEControlPlane) func(machine *clusterv1.Machine) bool {
	return collections.And(
		func(machine *clusterv1.Machine) bool {
			return matchMachineTemplateMetadata(ocnecp, machine)
		},
		collections.MatchesKubernetesVersion(ocnecp.Spec.Version),
		MatchesOCNEBootstrapConfig(machineConfigs, ocnecp),
		MatchesTemplateClonedFrom(infraConfigs, ocnecp),
	)
}

// MatchesTemplateClonedFrom returns a filter to find all machines that match a given KCP infra template.
func MatchesTemplateClonedFrom(infraConfigs map[string]*unstructured.Unstructured, ocnecp *controlplanev1.OCNEControlPlane) collections.Func {
	return func(machine *clusterv1.Machine) bool {
		if machine == nil {
			return false
		}
		infraObj, found := infraConfigs[machine.Name]
		if !found {
			// Return true here because failing to get infrastructure machine should not be considered as unmatching.
			return true
		}

		clonedFromName, ok1 := infraObj.GetAnnotations()[clusterv1.TemplateClonedFromNameAnnotation]
		clonedFromGroupKind, ok2 := infraObj.GetAnnotations()[clusterv1.TemplateClonedFromGroupKindAnnotation]
		if !ok1 || !ok2 {
			// All ocnecp cloned infra machines should have this annotation.
			// Missing the annotation may be due to older version machines or adopted machines.
			// Should not be considered as mismatch.
			return true
		}

		// Check if the machine's infrastructure reference has been created from the current KCP infrastructure template.
		if clonedFromName != ocnecp.Spec.MachineTemplate.InfrastructureRef.Name ||
			clonedFromGroupKind != ocnecp.Spec.MachineTemplate.InfrastructureRef.GroupVersionKind().GroupKind().String() {
			return false
		}

		// Check if the machine template metadata matches with the infrastructure object.
		if !matchMachineTemplateMetadata(ocnecp, infraObj) {
			return false
		}
		return true
	}
}

// MatchesOCNEBootstrapConfig checks if machine's OCNEConfigSpec is equivalent with KCP's OCNEConfigSpec.
func MatchesOCNEBootstrapConfig(machineConfigs map[string]*bootstrapv1.OCNEConfig, ocnecp *controlplanev1.OCNEControlPlane) collections.Func {
	return func(machine *clusterv1.Machine) bool {
		if machine == nil {
			return false
		}

		// Check if KCP and machine ClusterConfiguration matches, if not return
		if match := matchClusterConfiguration(ocnecp, machine); !match {
			return false
		}

		bootstrapRef := machine.Spec.Bootstrap.ConfigRef
		if bootstrapRef == nil {
			// Missing bootstrap reference should not be considered as unmatching.
			// This is a safety precaution to avoid selecting machines that are broken, which in the future should be remediated separately.
			return true
		}

		machineConfig, found := machineConfigs[machine.Name]
		if !found {
			// Return true here because failing to get OCNEConfig should not be considered as unmatching.
			// This is a safety precaution to avoid rolling out machines if the client or the api-server is misbehaving.
			return true
		}

		// Check if the machine template metadata matches with the infrastructure object.
		if !matchMachineTemplateMetadata(ocnecp, machineConfig) {
			return false
		}

		// Check if KCP and machine InitConfiguration or JoinConfiguration matches
		// NOTE: only one between init configuration and join configuration is set on a machine, depending
		// on the fact that the machine was the initial control plane node or a joining control plane node.
		return matchInitOrJoinConfiguration(machineConfig, ocnecp)
	}
}

// matchClusterConfiguration verifies if KCP and machine ClusterConfiguration matches.
// NOTE: Machines that have OCNEClusterConfigurationAnnotation will have to match with KCP ClusterConfiguration.
// If the annotation is not present (machine is either old or adopted), we won't roll out on any possible changes
// made in KCP's ClusterConfiguration given that we don't have enough information to make a decision.
// Users should use KCP.Spec.RolloutAfter field to force a rollout in this case.
func matchClusterConfiguration(ocnecp *controlplanev1.OCNEControlPlane, machine *clusterv1.Machine) bool {
	machineClusterConfigStr, ok := machine.GetAnnotations()[controlplanev1.OCNEClusterConfigurationAnnotation]
	if !ok {
		// We don't have enough information to make a decision; don't' trigger a roll out.
		return true
	}

	machineClusterConfig := &bootstrapv1.ClusterConfiguration{}
	// ClusterConfiguration annotation is not correct, only solution is to rollout.
	// The call to json.Unmarshal has to take a pointer to the pointer struct defined above,
	// otherwise we won't be able to handle a nil ClusterConfiguration (that is serialized into "null").
	// See https://github.com/kubernetes-sigs/cluster-api/issues/3353.
	if err := json.Unmarshal([]byte(machineClusterConfigStr), &machineClusterConfig); err != nil {
		return false
	}

	// If any of the compared values are nil, treat them the same as an empty ClusterConfiguration.
	if machineClusterConfig == nil {
		machineClusterConfig = &bootstrapv1.ClusterConfiguration{}
	}
	ocnecpLocalClusterConfiguration := ocnecp.Spec.OCNEConfigSpec.ClusterConfiguration
	if ocnecpLocalClusterConfiguration == nil {
		ocnecpLocalClusterConfiguration = &bootstrapv1.ClusterConfiguration{}
	}

	// Compare and return.
	return reflect.DeepEqual(machineClusterConfig, ocnecpLocalClusterConfiguration)
}

// matchInitOrJoinConfiguration verifies if KCP and machine InitConfiguration or JoinConfiguration matches.
// NOTE: By extension this method takes care of detecting changes in other fields of the OCNEConfig configuration (e.g. Files, Mounts etc.)
func matchInitOrJoinConfiguration(machineConfig *bootstrapv1.OCNEConfig, ocnecp *controlplanev1.OCNEControlPlane) bool {
	if machineConfig == nil {
		// Return true here because failing to get OCNEConfig should not be considered as unmatching.
		// This is a safety precaution to avoid rolling out machines if the client or the api-server is misbehaving.
		return true
	}

	// takes the OCNEConfigSpec from KCP and applies the transformations required
	// to allow a comparison with the OCNEConfig referenced from the machine.
	ocnecpConfig := getAdjustedKcpConfig(ocnecp, machineConfig)

	// Default both OCNEConfigSpecs before comparison.
	// *Note* This assumes that newly added default values never
	// introduce a semantic difference to the unset value.
	// But that is something that is ensured by our API guarantees.
	bootstrapv1.DefaultOCNEConfigSpec(ocnecpConfig)
	bootstrapv1.DefaultOCNEConfigSpec(&machineConfig.Spec)

	// cleanups all the fields that are not relevant for the comparison.
	cleanupConfigFields(ocnecpConfig, machineConfig)

	return reflect.DeepEqual(&machineConfig.Spec, ocnecpConfig)
}

// getAdjustedKcpConfig takes the OCNEConfigSpec from KCP and applies the transformations required
// to allow a comparison with the OCNEConfig referenced from the machine.
// NOTE: The KCP controller applies a set of transformations when creating a OCNEConfig referenced from the machine,
// mostly depending on the fact that the machine was the initial control plane node or a joining control plane node.
// In this function we don't have such information, so we are making the OCNEConfigSpec similar to the OCNEConfig.
func getAdjustedKcpConfig(ocnecp *controlplanev1.OCNEControlPlane, machineConfig *bootstrapv1.OCNEConfig) *bootstrapv1.OCNEConfigSpec {
	ocnecpConfig := ocnecp.Spec.OCNEConfigSpec.DeepCopy()

	// Machine's join configuration is nil when it is the first machine in the control plane.
	if machineConfig.Spec.JoinConfiguration == nil {
		ocnecpConfig.JoinConfiguration = nil
	}

	// Machine's init configuration is nil when the control plane is already initialized.
	if machineConfig.Spec.InitConfiguration == nil {
		ocnecpConfig.InitConfiguration = nil
	}

	return ocnecpConfig
}

// cleanupConfigFields cleanups all the fields that are not relevant for the comparison.
func cleanupConfigFields(ocnecpConfig *bootstrapv1.OCNEConfigSpec, machineConfig *bootstrapv1.OCNEConfig) {
	// KCP ClusterConfiguration will only be compared with a machine's ClusterConfiguration annotation, so
	// we are cleaning up from the reflect.DeepEqual comparison.
	ocnecpConfig.ClusterConfiguration = nil
	machineConfig.Spec.ClusterConfiguration = nil

	// If KCP JoinConfiguration is not present, set machine JoinConfiguration to nil (nothing can trigger rollout here).
	// NOTE: this is required because CABPOCNE applies an empty joinConfiguration in case no one is provided.
	if ocnecpConfig.JoinConfiguration == nil {
		machineConfig.Spec.JoinConfiguration = nil
	}

	// Cleanup JoinConfiguration.Discovery from ocnecpConfig and machineConfig, because those info are relevant only for
	// the join process and not for comparing the configuration of the machine.
	emptyDiscovery := bootstrapv1.Discovery{}
	if ocnecpConfig.JoinConfiguration != nil {
		ocnecpConfig.JoinConfiguration.Discovery = emptyDiscovery
	}
	if machineConfig.Spec.JoinConfiguration != nil {
		machineConfig.Spec.JoinConfiguration.Discovery = emptyDiscovery
	}

	// If KCP JoinConfiguration.ControlPlane is not present, set machine join configuration to nil (nothing can trigger rollout here).
	// NOTE: this is required because CABPOCNE applies an empty joinConfiguration.ControlPlane in case no one is provided.
	if ocnecpConfig.JoinConfiguration != nil && ocnecpConfig.JoinConfiguration.ControlPlane == nil &&
		machineConfig.Spec.JoinConfiguration != nil {
		machineConfig.Spec.JoinConfiguration.ControlPlane = nil
	}

	// If KCP's join NodeRegistration is empty, set machine's node registration to empty as no changes should trigger rollout.
	emptyNodeRegistration := bootstrapv1.NodeRegistrationOptions{}
	if ocnecpConfig.JoinConfiguration != nil && reflect.DeepEqual(ocnecpConfig.JoinConfiguration.NodeRegistration, emptyNodeRegistration) &&
		machineConfig.Spec.JoinConfiguration != nil {
		machineConfig.Spec.JoinConfiguration.NodeRegistration = emptyNodeRegistration
	}

	// Clear up the TypeMeta information from the comparison.
	// NOTE: KCP types don't carry this information.
	if machineConfig.Spec.InitConfiguration != nil && ocnecpConfig.InitConfiguration != nil {
		machineConfig.Spec.InitConfiguration.TypeMeta = ocnecpConfig.InitConfiguration.TypeMeta
	}
	if machineConfig.Spec.JoinConfiguration != nil && ocnecpConfig.JoinConfiguration != nil {
		machineConfig.Spec.JoinConfiguration.TypeMeta = ocnecpConfig.JoinConfiguration.TypeMeta
	}
}

// matchMachineTemplateMetadata matches the machine template object meta information,
// specifically annotations and labels, against an object.
func matchMachineTemplateMetadata(ocnecp *controlplanev1.OCNEControlPlane, obj client.Object) bool {
	// Check if annotations and labels match.
	if !isSubsetMapOf(ocnecp.Spec.MachineTemplate.ObjectMeta.Annotations, obj.GetAnnotations()) {
		return false
	}
	if !isSubsetMapOf(ocnecp.Spec.MachineTemplate.ObjectMeta.Labels, obj.GetLabels()) {
		return false
	}
	return true
}

func isSubsetMapOf(base map[string]string, existing map[string]string) bool {
loopBase:
	for key, value := range base {
		for existingKey, existingValue := range existing {
			if existingKey == key && existingValue == value {
				continue loopBase
			}
		}
		// Return false right away if a key value pair wasn't found.
		return false
	}
	return true
}
