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

import (
	"fmt"
	"net/url"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
	"time"

	metaV1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var verrazzanofleetlog = logf.Log.WithName("verrazzanofleet-resource")

func (r *VerrazzanoFleet) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate-addons-cluster-x-k8s-io-v1alpha1-verrazzanofleet,mutating=true,failurePolicy=fail,sideEffects=None,groups=addons.cluster.x-k8s.io,resources=verrazzanofleets,verbs=create;update,versions=v1alpha1,name=verrazzanofleet.kb.io,admissionReviewVersions=v1

var _ webhook.Defaulter = &VerrazzanoFleet{}

const helmTimeoutSeconds = time.Second * 600

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (p *VerrazzanoFleet) Default() {
	verrazzanofleetlog.Info("default", "name", p.Name)

	if p.Spec.ReleaseNamespace == "" {
		p.Spec.ReleaseNamespace = "default"
	}

	// If 'Wait' is set, we need to set default 'Timeout' to make install successful.
	if p.Spec.Options != nil && p.Spec.Options.Wait && p.Spec.Options.Timeout == nil {
		p.Spec.Options.Timeout = &metaV1.Duration{Duration: helmTimeoutSeconds}
	}

	// If 'CreateNamespace' is not specified by user, set default value to 'true'
	if p.Spec.Options != nil {
		if p.Spec.Options.Install == nil {
			p.Spec.Options.Install = &HelmInstallOptions{CreateNamespace: pointer.Bool(true)}
		} else if p.Spec.Options.Install.CreateNamespace == nil {
			p.Spec.Options.Install.CreateNamespace = pointer.Bool(true)
		}
	}
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-addons-cluster-x-k8s-io-v1alpha1-verrazzanofleet,mutating=false,failurePolicy=fail,sideEffects=None,groups=addons.cluster.x-k8s.io,resources=verrazzanofleets,verbs=create;update,versions=v1alpha1,name=vverrazzanofleet.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &VerrazzanoFleet{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *VerrazzanoFleet) ValidateCreate() (admission.Warnings, error) {
	verrazzanofleetlog.Info("validate create", "name", r.Name)

	if err := isUrlValid(r.Spec.RepoURL); err != nil {
		return nil, err
	}

	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *VerrazzanoFleet) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	verrazzanofleetlog.Info("validate update", "name", r.Name)

	if err := isUrlValid(r.Spec.RepoURL); err != nil {
		return nil, err
	}

	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *VerrazzanoFleet) ValidateDelete() (admission.Warnings, error) {
	verrazzanofleetlog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil, nil
}

// isUrlValid returns true if specifed repoURL is valid as per go doc https://pkg.go.dev/net/url#ParseRequestURI.
func isUrlValid(repoURL string) error {
	_, err := url.ParseRequestURI(repoURL)
	if err != nil {
		return fmt.Errorf("specified repoURL: %s is not valid. Error - %s", repoURL, err)
	}

	return nil
}
