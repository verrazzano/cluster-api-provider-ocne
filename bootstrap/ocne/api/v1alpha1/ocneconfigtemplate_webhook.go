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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func (r *OCNEConfigTemplate) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// +kubebuilder:webhook:verbs=create;update,path=/mutate-bootstrap-cluster-x-k8s-io-v1alpha1-ocneconfigtemplate,mutating=true,failurePolicy=fail,groups=bootstrap.cluster.x-k8s.io,resources=ocneconfigtemplates,versions=v1alpha1,name=default.ocneconfigtemplate.bootstrap.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1alpha1

var _ webhook.Defaulter = &OCNEConfigTemplate{}

// Default implements webhook.Defaulter so a webhook will be registered for the type.
func (r *OCNEConfigTemplate) Default() {
	DefaultOCNEConfigSpec(&r.Spec.Template.Spec)
}

// +kubebuilder:webhook:verbs=create;update,path=/validate-bootstrap-cluster-x-k8s-io-v1alpha1-ocneconfigtemplate,mutating=false,failurePolicy=fail,matchPolicy=Equivalent,groups=bootstrap.cluster.x-k8s.io,resources=ocneconfigtemplates,versions=v1alpha1,name=validation.ocneconfigtemplate.bootstrap.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1;v1alpha1

var _ webhook.Validator = &OCNEConfigTemplate{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type.
func (r *OCNEConfigTemplate) ValidateCreate() (admission.Warnings, error) {
	return nil, r.Spec.validate(r.Name)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type.
func (r *OCNEConfigTemplate) ValidateUpdate(_ runtime.Object) (admission.Warnings, error) {
	return nil, r.Spec.validate(r.Name)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type.
func (r *OCNEConfigTemplate) ValidateDelete() (admission.Warnings, error) {
	return nil, nil
}

func (r *OCNEConfigTemplateSpec) validate(name string) error {
	var allErrs field.ErrorList

	allErrs = append(allErrs, r.Template.Spec.Validate(field.NewPath("spec", "template", "spec"))...)

	if len(allErrs) == 0 {
		return nil
	}

	return apierrors.NewInvalid(GroupVersion.WithKind("OCNEConfigTemplate").GroupKind(), name, allErrs)
}
