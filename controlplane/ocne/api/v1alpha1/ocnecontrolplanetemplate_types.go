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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	bootstrapv1 "github.com/verrazzano/cluster-api-provider-ocne/bootstrap/ocne/api/v1alpha1"
)

// OCNEControlPlaneTemplateSpec defines the desired state of OCNEControlPlaneTemplate.
type OCNEControlPlaneTemplateSpec struct {
	Template OCNEControlPlaneTemplateResource `json:"template"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=ocnecontrolplanetemplates,scope=Namespaced,categories=cluster-api
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of OCNEControlPlaneTemplate"

// OCNEControlPlaneTemplate is the Schema for the ocnecontrolplanetemplates API.
type OCNEControlPlaneTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec OCNEControlPlaneTemplateSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// OCNEControlPlaneTemplateList contains a list of OCNEControlPlaneTemplate.
type OCNEControlPlaneTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OCNEControlPlaneTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OCNEControlPlaneTemplate{}, &OCNEControlPlaneTemplateList{})
}

// OCNEControlPlaneTemplateResource describes the data needed to create a OCNEControlPlane from a template.
type OCNEControlPlaneTemplateResource struct {
	Spec OCNEControlPlaneTemplateResourceSpec `json:"spec"`
}

// OCNEControlPlaneTemplateResourceSpec defines the desired state of OCNEControlPlane.
// NOTE: OCNEControlPlaneTemplateResourceSpec is similar to OCNEControlPlaneSpec but
// omits Replicas and Version fields. These fields do not make sense on the OCNEControlPlaneTemplate,
// because they are calculated by the Cluster topology reconciler during reconciliation and thus cannot
// be configured on the OCNEControlPlaneTemplate.
type OCNEControlPlaneTemplateResourceSpec struct {
	// MachineTemplate contains information about how machines
	// should be shaped when creating or updating a control plane.
	// +optional
	MachineTemplate *OCNEControlPlaneTemplateMachineTemplate `json:"machineTemplate,omitempty"`

	// OCNEConfigSpec is a OCNEConfigSpec
	// to use for initializing and joining machines to the control plane.
	OCNEConfigSpec bootstrapv1.OCNEConfigSpec `json:"controlPlaneConfig"`

	// RolloutBefore is a field to indicate a rollout should be performed
	// if the specified criteria is met.
	//
	// +optional
	RolloutBefore *RolloutBefore `json:"rolloutBefore,omitempty"`

	// RolloutAfter is a field to indicate a rollout should be performed
	// after the specified time even if no changes have been made to the
	// OCNEControlPlane.
	//
	// +optional
	RolloutAfter *metav1.Time `json:"rolloutAfter,omitempty"`

	// The RolloutStrategy to use to replace control plane machines with
	// new ones.
	// +optional
	// +kubebuilder:default={type: "RollingUpdate", rollingUpdate: {maxSurge: 1}}
	RolloutStrategy *RolloutStrategy `json:"rolloutStrategy,omitempty"`
}

// OCNEControlPlaneTemplateMachineTemplate defines the template for Machines
// in a OCNEControlPlaneTemplate object.
// NOTE: OCNEControlPlaneTemplateMachineTemplate is similar to OCNEControlPlaneMachineTemplate but
// omits ObjectMeta and InfrastructureRef fields. These fields do not make sense on the OCNEControlPlaneTemplate,
// because they are calculated by the Cluster topology reconciler during reconciliation and thus cannot
// be configured on the OCNEControlPlaneTemplate.
type OCNEControlPlaneTemplateMachineTemplate struct {
	// NodeDrainTimeout is the total amount of time that the controller will spend on draining a controlplane node
	// The default value is 0, meaning that the node can be drained without any time limitations.
	// NOTE: NodeDrainTimeout is different from `kubectl drain --timeout`
	// +optional
	NodeDrainTimeout *metav1.Duration `json:"nodeDrainTimeout,omitempty"`

	// NodeVolumeDetachTimeout is the total amount of time that the controller will spend on waiting for all volumes
	// to be detached. The default value is 0, meaning that the volumes can be detached without any time limitations.
	// +optional
	NodeVolumeDetachTimeout *metav1.Duration `json:"nodeVolumeDetachTimeout,omitempty"`

	// NodeDeletionTimeout defines how long the machine controller will attempt to delete the Node that the Machine
	// hosts after the Machine is marked for deletion. A duration of 0 will retry deletion indefinitely.
	// If no value is provided, the default value for this property of the Machine resource will be used.
	// +optional
	NodeDeletionTimeout *metav1.Duration `json:"nodeDeletionTimeout,omitempty"`
}
