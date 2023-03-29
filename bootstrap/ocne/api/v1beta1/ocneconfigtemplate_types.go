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

package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// OCNEConfigTemplateSpec defines the desired state of OCNEConfigTemplate.
type OCNEConfigTemplateSpec struct {
	Template OCNEConfigTemplateResource `json:"template"`
}

// OCNEConfigTemplateResource defines the Template structure.
type OCNEConfigTemplateResource struct {
	Spec OCNEConfigSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=ocneconfigtemplates,scope=Namespaced,categories=cluster-api
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of OCNEConfigTemplate"

// OCNEConfigTemplate is the Schema for the ocneconfigtemplates API.
type OCNEConfigTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec OCNEConfigTemplateSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// OCNEConfigTemplateList contains a list of OCNEConfigTemplate.
type OCNEConfigTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OCNEConfigTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OCNEConfigTemplate{}, &OCNEConfigTemplateList{})
}
