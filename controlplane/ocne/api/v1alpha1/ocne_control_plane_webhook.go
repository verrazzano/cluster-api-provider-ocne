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

package v1alpha1

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/verrazzano/cluster-api-provider-ocne/internal/util/ocne"
	"strings"

	"github.com/blang/semver"
	"github.com/coredns/corefile-migration/migration"
	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	bootstrapv1 "github.com/verrazzano/cluster-api-provider-ocne/bootstrap/ocne/api/v1alpha1"
	"github.com/verrazzano/cluster-api-provider-ocne/util/container"
	"github.com/verrazzano/cluster-api-provider-ocne/util/version"
)

func (in *OCNEControlPlane) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(in).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/mutate-controlplane-cluster-x-k8s-io-v1alpha1-ocnecontrolplane,mutating=true,failurePolicy=fail,matchPolicy=Equivalent,groups=controlplane.cluster.x-k8s.io,resources=ocnecontrolplanes,versions=v1alpha1,name=default.ocnecontrolplane.controlplane.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1alpha1
// +kubebuilder:webhook:verbs=create;update,path=/validate-controlplane-cluster-x-k8s-io-v1alpha1-ocnecontrolplane,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=controlplane.cluster.x-k8s.io,resources=ocnecontrolplanes,versions=v1alpha1,name=validation.ocnecontrolplane.controlplane.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1alpha1

var _ webhook.Defaulter = &OCNEControlPlane{}
var _ webhook.Validator = &OCNEControlPlane{}

// Default implements webhook.Defaulter so a webhook will be registered for the type.
func (in *OCNEControlPlane) Default() {
	defaultKubeadmControlPlaneSpec(&in.Spec, in.Namespace)
}

func defaultKubeadmControlPlaneSpec(s *OCNEControlPlaneSpec, namespace string) {
	if s.Replicas == nil {
		replicas := int32(1)
		s.Replicas = &replicas
	}

	if s.MachineTemplate.InfrastructureRef.Namespace == "" {
		s.MachineTemplate.InfrastructureRef.Namespace = namespace
	}

	if !strings.HasPrefix(s.Version, "v") {
		s.Version = "v" + s.Version
	}

	bootstrapv1.DefaultOCNEConfigSpec(&s.ControlPlaneConfig)

	s.RolloutStrategy = defaultRolloutStrategy(s.RolloutStrategy)
}

func defaultRolloutStrategy(rolloutStrategy *RolloutStrategy) *RolloutStrategy {
	ios1 := intstr.FromInt(1)

	if rolloutStrategy == nil {
		rolloutStrategy = &RolloutStrategy{}
	}

	// Enforce RollingUpdate strategy and default MaxSurge if not set.
	if rolloutStrategy != nil {
		if len(rolloutStrategy.Type) == 0 {
			rolloutStrategy.Type = RollingUpdateStrategyType
		}
		if rolloutStrategy.Type == RollingUpdateStrategyType {
			if rolloutStrategy.RollingUpdate == nil {
				rolloutStrategy.RollingUpdate = &RollingUpdate{}
			}
			rolloutStrategy.RollingUpdate.MaxSurge = intstr.ValueOrDefault(rolloutStrategy.RollingUpdate.MaxSurge, ios1)
		}
	}

	return rolloutStrategy
}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (in *OCNEControlPlane) ValidateCreate() error {
	spec := in.Spec
	allErrs := validateKubeadmControlPlaneSpec(spec, in.Namespace, field.NewPath("spec"))
	allErrs = append(allErrs, validateClusterConfiguration(spec.ControlPlaneConfig.ClusterConfiguration, nil, field.NewPath("spec", "controlPlaneConfig", "clusterConfiguration"))...)
	allErrs = append(allErrs, in.validateOCNEData(in.Spec.ControlPlaneConfig.ClusterConfiguration, in.Spec.Version)...)
	allErrs = append(allErrs, in.validateOCNESocket(&in.Spec.ControlPlaneConfig)...)
	allErrs = append(allErrs, spec.ControlPlaneConfig.Validate(field.NewPath("spec", "controlPlaneConfig"))...)
	if len(allErrs) > 0 {
		return apierrors.NewInvalid(GroupVersion.WithKind("OCNEControlPlane").GroupKind(), in.Name, allErrs)
	}
	return nil
}

const (
	spec                 = "spec"
	controlPlaneConfig   = "controlPlaneConfig"
	clusterConfiguration = "clusterConfiguration"
	initConfiguration    = "initConfiguration"
	joinConfiguration    = "joinConfiguration"
	nodeRegistration     = "nodeRegistration"
	skipPhases           = "skipPhases"
	patches              = "patches"
	directory            = "directory"
	preOCNECommands      = "preOCNECommands"
	postOCNECommands     = "postOCNECommands"
	files                = "files"
	users                = "users"
	apiServer            = "apiServer"
	controllerManager    = "controllerManager"
	scheduler            = "scheduler"
	ntp                  = "ntp"
	ignition             = "ignition"
	diskSetup            = "diskSetup"
	moduleOperator       = "moduleOperator"
)

const minimumCertificatesExpiryDays = 7

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (in *OCNEControlPlane) ValidateUpdate(old runtime.Object) error {
	// add a * to indicate everything beneath is ok.
	// For example, {"spec", "*"} will allow any path under "spec" to change.
	allowedPaths := [][]string{
		{"metadata", "*"},
		{spec, controlPlaneConfig, clusterConfiguration, "etcd", "local", "imageRepository"},
		{spec, controlPlaneConfig, clusterConfiguration, "etcd", "local", "imageTag"},
		{spec, controlPlaneConfig, clusterConfiguration, "etcd", "local", "extraArgs", "*"},
		{spec, controlPlaneConfig, clusterConfiguration, "dns", "imageRepository"},
		{spec, controlPlaneConfig, clusterConfiguration, "dns", "imageTag"},
		{spec, controlPlaneConfig, clusterConfiguration, "imageRepository"},
		{spec, controlPlaneConfig, clusterConfiguration, apiServer, "*"},
		{spec, controlPlaneConfig, clusterConfiguration, controllerManager, "*"},
		{spec, controlPlaneConfig, clusterConfiguration, scheduler, "*"},
		{spec, controlPlaneConfig, initConfiguration, nodeRegistration, "*"},
		{spec, controlPlaneConfig, initConfiguration, patches, directory},
		{spec, controlPlaneConfig, initConfiguration, skipPhases},
		{spec, controlPlaneConfig, joinConfiguration, nodeRegistration, "*"},
		{spec, controlPlaneConfig, joinConfiguration, patches, directory},
		{spec, controlPlaneConfig, joinConfiguration, skipPhases},
		{spec, controlPlaneConfig, preOCNECommands},
		{spec, controlPlaneConfig, postOCNECommands},
		{spec, controlPlaneConfig, files},
		{spec, controlPlaneConfig, "verbosity"},
		{spec, controlPlaneConfig, users},
		{spec, controlPlaneConfig, ntp, "*"},
		{spec, controlPlaneConfig, ignition, "*"},
		{spec, controlPlaneConfig, diskSetup, "*"},
		{spec, "machineTemplate", "metadata", "*"},
		{spec, "machineTemplate", "infrastructureRef", "apiVersion"},
		{spec, "machineTemplate", "infrastructureRef", "name"},
		{spec, "machineTemplate", "infrastructureRef", "kind"},
		{spec, "machineTemplate", "nodeDrainTimeout"},
		{spec, "machineTemplate", "nodeVolumeDetachTimeout"},
		{spec, "machineTemplate", "nodeDeletionTimeout"},
		{spec, "replicas"},
		{spec, "version"},
		{spec, "rolloutAfter"},
		{spec, "rolloutBefore", "*"},
		{spec, "rolloutStrategy", "*"},
		{spec, moduleOperator},
		{spec, moduleOperator, "*"},
	}

	allErrs := validateKubeadmControlPlaneSpec(in.Spec, in.Namespace, field.NewPath("spec"))

	prev, ok := old.(*OCNEControlPlane)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expecting OCNEControlPlane but got a %T", old))
	}

	// NOTE: Defaulting for the format field has been added in v1.1.0 after implementing ignition support.
	// This allows existing KCP objects to pick up the new default.
	if prev.Spec.ControlPlaneConfig.Format == "" && in.Spec.ControlPlaneConfig.Format == bootstrapv1.CloudConfig {
		allowedPaths = append(allowedPaths, []string{spec, controlPlaneConfig, "format"})
	}

	originalJSON, err := json.Marshal(prev)
	if err != nil {
		return apierrors.NewInternalError(err)
	}
	modifiedJSON, err := json.Marshal(in)
	if err != nil {
		return apierrors.NewInternalError(err)
	}

	diff, err := jsonpatch.CreateMergePatch(originalJSON, modifiedJSON)
	if err != nil {
		return apierrors.NewInternalError(err)
	}
	jsonPatch := map[string]interface{}{}
	if err := json.Unmarshal(diff, &jsonPatch); err != nil {
		return apierrors.NewInternalError(err)
	}
	// Build a list of all paths that are trying to change
	diffpaths := paths([]string{}, jsonPatch)
	// Every path in the diff must be valid for the update function to work.
	for _, path := range diffpaths {
		// Ignore paths that are empty
		if len(path) == 0 {
			continue
		}
		if !allowed(allowedPaths, path) {
			if len(path) == 1 {
				allErrs = append(allErrs, field.Forbidden(field.NewPath(path[0]), "cannot be modified"))
				continue
			}
			allErrs = append(allErrs, field.Forbidden(field.NewPath(path[0], path[1:]...), "cannot be modified"))
		}
	}

	allErrs = append(allErrs, in.validateVersion(prev.Spec.Version)...)
	allErrs = append(allErrs, validateClusterConfiguration(in.Spec.ControlPlaneConfig.ClusterConfiguration, prev.Spec.ControlPlaneConfig.ClusterConfiguration, field.NewPath("spec", "controlPlaneConfig", "clusterConfiguration"))...)
	allErrs = append(allErrs, in.validateOCNEVersionOnUpgrade()...)
	allErrs = append(allErrs, in.validateOCNESocket(&in.Spec.ControlPlaneConfig)...)
	allErrs = append(allErrs, in.validateCoreDNSVersion(prev)...)
	allErrs = append(allErrs, in.Spec.ControlPlaneConfig.Validate(field.NewPath("spec", "controlPlaneConfig"))...)

	if len(allErrs) > 0 {
		return apierrors.NewInvalid(GroupVersion.WithKind("OCNEControlPlane").GroupKind(), in.Name, allErrs)
	}

	return nil
}

func validateKubeadmControlPlaneSpec(s OCNEControlPlaneSpec, namespace string, pathPrefix *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if s.Replicas == nil {
		allErrs = append(
			allErrs,
			field.Required(
				pathPrefix.Child("replicas"),
				"is required",
			),
		)
	} else if *s.Replicas <= 0 {
		// The use of the scale subresource should provide a guarantee that negative values
		// should not be accepted for this field, but since we have to validate that Replicas != 0
		// it doesn't hurt to also additionally validate for negative numbers here as well.
		allErrs = append(
			allErrs,
			field.Forbidden(
				pathPrefix.Child("replicas"),
				"cannot be less than or equal to 0",
			),
		)
	}

	externalEtcd := false
	if s.ControlPlaneConfig.ClusterConfiguration != nil {
		if s.ControlPlaneConfig.ClusterConfiguration.Etcd.External != nil {
			externalEtcd = true
		}
	}

	if !externalEtcd {
		if s.Replicas != nil && *s.Replicas%2 == 0 {
			allErrs = append(
				allErrs,
				field.Forbidden(
					pathPrefix.Child("replicas"),
					"cannot be an even number when etcd is stacked",
				),
			)
		}
	}

	if s.MachineTemplate.InfrastructureRef.APIVersion == "" {
		allErrs = append(
			allErrs,
			field.Invalid(
				pathPrefix.Child("machineTemplate", "infrastructure", "apiVersion"),
				s.MachineTemplate.InfrastructureRef.APIVersion,
				"cannot be empty",
			),
		)
	}
	if s.MachineTemplate.InfrastructureRef.Kind == "" {
		allErrs = append(
			allErrs,
			field.Invalid(
				pathPrefix.Child("machineTemplate", "infrastructure", "kind"),
				s.MachineTemplate.InfrastructureRef.Kind,
				"cannot be empty",
			),
		)
	}
	if s.MachineTemplate.InfrastructureRef.Name == "" {
		allErrs = append(
			allErrs,
			field.Invalid(
				pathPrefix.Child("machineTemplate", "infrastructure", "name"),
				s.MachineTemplate.InfrastructureRef.Name,
				"cannot be empty",
			),
		)
	}
	if s.MachineTemplate.InfrastructureRef.Namespace != namespace {
		allErrs = append(
			allErrs,
			field.Invalid(
				pathPrefix.Child("machineTemplate", "infrastructure", "namespace"),
				s.MachineTemplate.InfrastructureRef.Namespace,
				"must match metadata.namespace",
			),
		)
	}

	if !version.KubeSemver.MatchString(s.Version) {
		allErrs = append(allErrs, field.Invalid(pathPrefix.Child("version"), s.Version, "must be a valid semantic version"))
	}

	allErrs = append(allErrs, validateRolloutBefore(s.RolloutBefore, pathPrefix.Child("rolloutBefore"))...)
	allErrs = append(allErrs, validateRolloutStrategy(s.RolloutStrategy, s.Replicas, pathPrefix.Child("rolloutStrategy"))...)

	return allErrs
}

func validateRolloutBefore(rolloutBefore *RolloutBefore, pathPrefix *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if rolloutBefore == nil {
		return allErrs
	}

	if rolloutBefore.CertificatesExpiryDays != nil {
		if *rolloutBefore.CertificatesExpiryDays < minimumCertificatesExpiryDays {
			allErrs = append(allErrs, field.Invalid(pathPrefix.Child("certificatesExpiryDays"), *rolloutBefore.CertificatesExpiryDays, fmt.Sprintf("must be greater than or equal to %v", minimumCertificatesExpiryDays)))
		}
	}

	return allErrs
}

func validateRolloutStrategy(rolloutStrategy *RolloutStrategy, replicas *int32, pathPrefix *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if rolloutStrategy == nil {
		return allErrs
	}

	if rolloutStrategy.Type != RollingUpdateStrategyType {
		allErrs = append(
			allErrs,
			field.Required(
				pathPrefix.Child("type"),
				"only RollingUpdateStrategyType is supported",
			),
		)
	}

	ios1 := intstr.FromInt(1)
	ios0 := intstr.FromInt(0)

	if rolloutStrategy.RollingUpdate.MaxSurge.IntValue() == ios0.IntValue() && (replicas != nil && *replicas < int32(3)) {
		allErrs = append(
			allErrs,
			field.Required(
				pathPrefix.Child("rollingUpdate"),
				"when OCNEControlPlane is configured to scale-in, replica count needs to be at least 3",
			),
		)
	}

	if rolloutStrategy.RollingUpdate.MaxSurge.IntValue() != ios1.IntValue() && rolloutStrategy.RollingUpdate.MaxSurge.IntValue() != ios0.IntValue() {
		allErrs = append(
			allErrs,
			field.Required(
				pathPrefix.Child("rollingUpdate", "maxSurge"),
				"value must be 1 or 0",
			),
		)
	}

	return allErrs
}

func validateClusterConfiguration(newClusterConfiguration, oldClusterConfiguration *bootstrapv1.ClusterConfiguration, pathPrefix *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if newClusterConfiguration == nil {
		return allErrs
	}

	// TODO: Remove when kubeadm types include OpenAPI validation
	if !container.ImageTagIsValid(newClusterConfiguration.DNS.ImageTag) {
		allErrs = append(
			allErrs,
			field.Forbidden(
				pathPrefix.Child("dns", "imageTag"),
				fmt.Sprintf("tag %s is invalid", newClusterConfiguration.DNS.ImageTag),
			),
		)
	}

	if newClusterConfiguration.DNS.ImageTag != "" {
		if _, err := version.ParseMajorMinorPatchTolerant(newClusterConfiguration.DNS.ImageTag); err != nil {
			allErrs = append(allErrs,
				field.Invalid(
					field.NewPath("dns", "imageTag"),
					newClusterConfiguration.DNS.ImageTag,
					fmt.Sprintf("failed to parse CoreDNS version: %v", err),
				),
			)
		}
	}

	// TODO: Remove when kubeadm types include OpenAPI validation
	if newClusterConfiguration.Etcd.Local != nil && !container.ImageTagIsValid(newClusterConfiguration.Etcd.Local.ImageTag) {
		allErrs = append(
			allErrs,
			field.Forbidden(
				pathPrefix.Child("etcd", "local", "imageTag"),
				fmt.Sprintf("tag %s is invalid", newClusterConfiguration.Etcd.Local.ImageTag),
			),
		)
	}

	if newClusterConfiguration.Etcd.Local != nil && newClusterConfiguration.Etcd.External != nil {
		allErrs = append(
			allErrs,
			field.Forbidden(
				pathPrefix.Child("etcd", "local"),
				"cannot have both external and local etcd",
			),
		)
	}

	// update validations
	if oldClusterConfiguration != nil {
		if newClusterConfiguration.Etcd.External != nil && oldClusterConfiguration.Etcd.Local != nil {
			allErrs = append(
				allErrs,
				field.Forbidden(
					pathPrefix.Child("etcd", "external"),
					"cannot change between external and local etcd",
				),
			)
		}

		if newClusterConfiguration.Etcd.Local != nil && oldClusterConfiguration.Etcd.External != nil {
			allErrs = append(
				allErrs,
				field.Forbidden(
					pathPrefix.Child("etcd", "local"),
					"cannot change between external and local etcd",
				),
			)
		}
	}

	return allErrs
}

func allowed(allowList [][]string, path []string) bool {
	for _, allowed := range allowList {
		if pathsMatch(allowed, path) {
			return true
		}
	}
	return false
}

func pathsMatch(allowed, path []string) bool {
	// if either are empty then no match can be made
	if len(allowed) == 0 || len(path) == 0 {
		return false
	}
	i := 0
	for i = range path {
		// reached the end of the allowed path and no match was found
		if i > len(allowed)-1 {
			return false
		}
		if allowed[i] == "*" {
			return true
		}
		if path[i] != allowed[i] {
			return false
		}
	}
	// path has been completely iterated and has not matched the end of the path.
	// e.g. allowed: []string{"a","b","c"}, path: []string{"a"}
	return i >= len(allowed)-1
}

// paths builds a slice of paths that are being modified.
func paths(path []string, diff map[string]interface{}) [][]string {
	allPaths := [][]string{}
	for key, m := range diff {
		nested, ok := m.(map[string]interface{})
		if !ok {
			// We have to use a copy of path, because otherwise the slice we append to
			// allPaths would be overwritten in another iteration.
			tmp := make([]string, len(path))
			copy(tmp, path)
			allPaths = append(allPaths, append(tmp, key))
			continue
		}
		allPaths = append(allPaths, paths(append(path, key), nested)...)
	}
	return allPaths
}

func (in *OCNEControlPlane) validateCoreDNSVersion(prev *OCNEControlPlane) (allErrs field.ErrorList) {
	if in.Spec.ControlPlaneConfig.ClusterConfiguration == nil || prev.Spec.ControlPlaneConfig.ClusterConfiguration == nil {
		return allErrs
	}
	// return if either current or target versions is empty
	if prev.Spec.ControlPlaneConfig.ClusterConfiguration.DNS.ImageTag == "" || in.Spec.ControlPlaneConfig.ClusterConfiguration.DNS.ImageTag == "" {
		return allErrs
	}

	targetDNS := &in.Spec.ControlPlaneConfig.ClusterConfiguration.DNS

	fromVersion, err := version.ParseMajorMinorPatchTolerant(prev.Spec.ControlPlaneConfig.ClusterConfiguration.DNS.ImageTag)
	if err != nil {
		allErrs = append(allErrs,
			field.Invalid(
				field.NewPath("spec", "controlPlaneConfig", "clusterConfiguration", "dns", "imageTag"),
				prev.Spec.ControlPlaneConfig.ClusterConfiguration.DNS.ImageTag,
				fmt.Sprintf("failed to parse current CoreDNS version: %v", err),
			),
		)
		return allErrs
	}

	toVersion, err := version.ParseMajorMinorPatchTolerant(targetDNS.ImageTag)
	if err != nil {
		allErrs = append(allErrs,
			field.Invalid(
				field.NewPath("spec", "controlPlaneConfig", "clusterConfiguration", "dns", "imageTag"),
				targetDNS.ImageTag,
				fmt.Sprintf("failed to parse target CoreDNS version: %v", err),
			),
		)
		return allErrs
	}
	// If the versions are equal return here without error.
	// This allows an upgrade where the version of CoreDNS in use is not supported by the migration tool.
	if toVersion.Equals(fromVersion) {
		return allErrs
	}
	if err := migration.ValidUpMigration(fromVersion.String(), toVersion.String()); err != nil {
		allErrs = append(
			allErrs,
			field.Forbidden(
				field.NewPath("spec", "controlPlaneConfig", "clusterConfiguration", "dns", "imageTag"),
				fmt.Sprintf("cannot migrate CoreDNS up to '%v' from '%v': %v", toVersion, fromVersion, err),
			),
		)
	}

	return allErrs
}

func (in *OCNEControlPlane) validateVersion(previousVersion string) (allErrs field.ErrorList) {
	fromVersion, err := version.ParseMajorMinorPatch(previousVersion)
	if err != nil {
		allErrs = append(allErrs,
			field.InternalError(
				field.NewPath("spec", "version"),
				errors.Wrapf(err, "failed to parse current ocnecontrolplane version: %s", previousVersion),
			),
		)
		return allErrs
	}

	toVersion, err := version.ParseMajorMinorPatch(in.Spec.Version)
	if err != nil {
		allErrs = append(allErrs,
			field.InternalError(
				field.NewPath("spec", "version"),
				errors.Wrapf(err, "failed to parse updated ocnecontrolplane version: %s", in.Spec.Version),
			),
		)
		return allErrs
	}

	// Check if we're trying to upgrade to Kubernetes v1.19.0, which is not supported.
	//
	// See https://github.com/kubernetes-sigs/cluster-api/issues/3564
	if fromVersion.NE(toVersion) && toVersion.Equals(semver.MustParse("1.19.0")) {
		allErrs = append(allErrs,
			field.Forbidden(
				field.NewPath("spec", "version"),
				"cannot update Kubernetes version to v1.19.0, for more information see https://github.com/kubernetes-sigs/cluster-api/issues/3564",
			),
		)
		return allErrs
	}

	// Validate that the update is upgrading at most one minor version.
	// Note: Skipping a minor version is not allowed.
	// Note: Checking against this ceilVersion allows upgrading to the next minor
	// version irrespective of the patch version.
	ceilVersion := semver.Version{
		Major: fromVersion.Major,
		Minor: fromVersion.Minor + 2,
		Patch: 0,
	}
	if toVersion.GTE(ceilVersion) {
		allErrs = append(allErrs,
			field.Forbidden(
				field.NewPath("spec", "version"),
				fmt.Sprintf("cannot update Kubernetes version from %s to %s", previousVersion, in.Spec.Version),
			),
		)
	}

	// The Kubernetes ecosystem has been requested to move users to the new registry due to cost issues.
	// This validation enforces the move to the new registry by forcing users to upgrade to kubeadm versions
	// with the new registry.
	// NOTE: This only affects users relying on the community maintained registry.
	// NOTE: Pinning to the upstream registry is not recommended because it could lead to issues
	// given how the migration has been implemented in kubeadm.
	//
	// Block if imageRepository is not set (i.e. the default registry should be used),
	if (in.Spec.ControlPlaneConfig.ClusterConfiguration == nil ||
		in.Spec.ControlPlaneConfig.ClusterConfiguration.ImageRepository == "") &&
		// the version changed (i.e. we have an upgrade),
		toVersion.NE(fromVersion) &&
		// the version is >= v1.22.0 and < v1.26.0
		toVersion.GTE(ocne.MinKubernetesVersionImageRegistryMigration) &&
		toVersion.LT(ocne.NextKubernetesVersionImageRegistryMigration) &&
		// and the default registry of the new Kubernetes/kubeadm version is the old default registry.
		ocne.GetDefaultRegistry(toVersion) == ocne.OldDefaultImageRepository {
		allErrs = append(allErrs,
			field.Forbidden(
				field.NewPath("spec", "version"),
				"cannot upgrade to a Kubernetes/kubeadm version which is using the old default registry. Please use a newer Kubernetes patch release which is using the new default registry (>= v1.22.17, >= v1.23.15, >= v1.24.9)",
			),
		)
	}

	return allErrs
}

func (in *OCNEControlPlane) validateOCNEData(inClusterConfiguration *bootstrapv1.ClusterConfiguration, version string) (allErrs field.ErrorList) {

	if inClusterConfiguration == nil {
		return allErrs
	}

	ocneMeta, err := ocne.GetOCNEMetadata(context.Background())
	if err != nil {
		return allErrs
	}

	if inClusterConfiguration.DNS.ImageTag != "" {
		if inClusterConfiguration.DNS.ImageRepository == ocne.DefaultOCNEImageRepository {
			newClusterCoreDNSTag := ocneMeta[version].OCNEImages.CoreDNS
			if inClusterConfiguration.DNS.ImageTag != newClusterCoreDNSTag {
				allErrs = append(allErrs,
					field.Invalid(
						field.NewPath("dns", "imageTag"),
						inClusterConfiguration.DNS.ImageTag,
						fmt.Sprintf("not supported coreDNS image tag for kubernetes version %v.", version),
					),
				)
			}
		}
	}

	if inClusterConfiguration.Etcd.Local != nil {
		if inClusterConfiguration.Etcd.Local.ImageRepository == ocne.DefaultOCNEImageRepository {
			newClusterEtcdTag := ocneMeta[version].OCNEImages.ETCD
			if inClusterConfiguration.Etcd.Local.ImageTag != newClusterEtcdTag {
				allErrs = append(allErrs,
					field.Invalid(
						field.NewPath("etcd", "local", "imageTag"),
						inClusterConfiguration.Etcd.Local.ImageTag,
						fmt.Sprintf("not supported etcd image tag for kubernetes version %v.", version),
					),
				)
			}
		}
	}
	return allErrs
}

func (in *OCNEControlPlane) validateOCNEVersionOnUpgrade() (allErrs field.ErrorList) {

	ocneMeta, err := ocne.GetOCNEMetadata(context.Background())
	if err != nil {
		return allErrs
	}

	if ocneMeta[in.Spec.Version].OCNEPackages.Kubeadm == "" {
		allErrs = append(allErrs,
			field.Invalid(
				field.NewPath("spec", "version"),
				in.Spec.Version,
				fmt.Sprintf("upgrade not supported to kubernetes version %v.", in.Spec.Version),
			),
		)
	}
	return allErrs
}

func (in *OCNEControlPlane) validateOCNESocket(controlPlaneConfigSpec *bootstrapv1.OCNEConfigSpec) (allErrs field.ErrorList) {

	if controlPlaneConfigSpec == nil || controlPlaneConfigSpec.InitConfiguration == nil || controlPlaneConfigSpec.JoinConfiguration == nil {
		return allErrs
	}

	if controlPlaneConfigSpec.InitConfiguration.NodeRegistration.CRISocket != "" {
		if controlPlaneConfigSpec.InitConfiguration.NodeRegistration.CRISocket != ocne.DefaultOCNESocket {
			allErrs = append(allErrs,
				field.Invalid(
					field.NewPath("initConfiguration", "nodeRegistration", "criSocket"),
					controlPlaneConfigSpec.InitConfiguration.NodeRegistration.CRISocket,
					fmt.Sprintf("only '%v' is supported as criSocket with OCNE", ocne.DefaultOCNESocket),
				),
			)
		}
	}

	if controlPlaneConfigSpec.JoinConfiguration.NodeRegistration.CRISocket != "" {
		if controlPlaneConfigSpec.JoinConfiguration.NodeRegistration.CRISocket != ocne.DefaultOCNESocket {
			allErrs = append(allErrs,
				field.Invalid(
					field.NewPath("joinConfiguration", "nodeRegistration", "criSocket"),
					controlPlaneConfigSpec.JoinConfiguration.NodeRegistration.CRISocket,
					fmt.Sprintf("only '%v' is supported as criSocket with OCNE", ocne.DefaultOCNESocket),
				),
			)
		}
	}

	return allErrs
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (in *OCNEControlPlane) ValidateDelete() error {
	return nil
}
