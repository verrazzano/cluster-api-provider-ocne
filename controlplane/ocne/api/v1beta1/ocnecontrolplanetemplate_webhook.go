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

package v1beta1

import (
	"fmt"
	"reflect"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	bootstrapv1 "github.com/verrazzano/cluster-api-provider-ocne/bootstrap/ocne/api/v1beta1"
	"github.com/verrazzano/cluster-api-provider-ocne/feature"
)

const kubeadmControlPlaneTemplateImmutableMsg = "OcneControlPlaneTemplate spec.template.spec field is immutable. Please create new resource instead."

func (r *OcneControlPlaneTemplate) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/mutate-controlplane-cluster-x-k8s-io-v1beta1-kubeadmcontrolplanetemplate,mutating=true,failurePolicy=fail,groups=controlplane.cluster.x-k8s.io,resources=kubeadmcontrolplanetemplates,versions=v1beta1,name=default.kubeadmcontrolplanetemplate.controlplane.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

var _ webhook.Defaulter = &OcneControlPlaneTemplate{}

// Default implements webhook.Defaulter so a webhook will be registered for the type.
func (r *OcneControlPlaneTemplate) Default() {
	bootstrapv1.DefaultOcneConfigSpec(&r.Spec.Template.Spec.OcneConfigSpec)

	r.Spec.Template.Spec.RolloutStrategy = defaultRolloutStrategy(r.Spec.Template.Spec.RolloutStrategy)
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-controlplane-cluster-x-k8s-io-v1beta1-kubeadmcontrolplanetemplate,mutating=false,failurePolicy=fail,groups=controlplane.cluster.x-k8s.io,resources=kubeadmcontrolplanetemplates,versions=v1beta1,name=validation.kubeadmcontrolplanetemplate.controlplane.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1beta1

var _ webhook.Validator = &OcneControlPlaneTemplate{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (r *OcneControlPlaneTemplate) ValidateCreate() error {
	// NOTE: OcneControlPlaneTemplate is behind ClusterTopology feature gate flag; the web hook
	// must prevent creating new objects in case the feature flag is disabled.
	if !feature.Gates.Enabled(feature.ClusterTopology) {
		return field.Forbidden(
			field.NewPath("spec"),
			"can be set only if the ClusterTopology feature flag is enabled",
		)
	}

	spec := r.Spec.Template.Spec
	allErrs := validateKubeadmControlPlaneTemplateResourceSpec(spec, field.NewPath("spec", "template", "spec"))
	allErrs = append(allErrs, validateClusterConfiguration(spec.OcneConfigSpec.ClusterConfiguration, nil, field.NewPath("spec", "template", "spec", "ocneConfigSpec", "clusterConfiguration"))...)
	allErrs = append(allErrs, spec.OcneConfigSpec.Validate(field.NewPath("spec", "template", "spec", "ocneConfigSpec"))...)
	if len(allErrs) > 0 {
		return apierrors.NewInvalid(GroupVersion.WithKind("OcneControlPlaneTemplate").GroupKind(), r.Name, allErrs)
	}
	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (r *OcneControlPlaneTemplate) ValidateUpdate(oldRaw runtime.Object) error {
	var allErrs field.ErrorList
	old, ok := oldRaw.(*OcneControlPlaneTemplate)
	if !ok {
		return apierrors.NewBadRequest(fmt.Sprintf("expected a OcneControlPlaneTemplate but got a %T", oldRaw))
	}

	if !reflect.DeepEqual(r.Spec.Template.Spec, old.Spec.Template.Spec) {
		allErrs = append(allErrs,
			field.Invalid(field.NewPath("spec", "template", "spec"), r, kubeadmControlPlaneTemplateImmutableMsg),
		)
	}

	if len(allErrs) == 0 {
		return nil
	}
	return apierrors.NewInvalid(GroupVersion.WithKind("OcneControlPlaneTemplate").GroupKind(), r.Name, allErrs)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (r *OcneControlPlaneTemplate) ValidateDelete() error {
	return nil
}

// validateKubeadmControlPlaneTemplateResourceSpec is a copy of validateKubeadmControlPlaneSpec which
// only validates the fields in OcneControlPlaneTemplateResourceSpec we care about.
func validateKubeadmControlPlaneTemplateResourceSpec(s OcneControlPlaneTemplateResourceSpec, pathPrefix *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	allErrs = append(allErrs, validateRolloutBefore(s.RolloutBefore, pathPrefix.Child("rolloutBefore"))...)
	allErrs = append(allErrs, validateRolloutStrategy(s.RolloutStrategy, nil, pathPrefix.Child("rolloutStrategy"))...)

	return allErrs
}
