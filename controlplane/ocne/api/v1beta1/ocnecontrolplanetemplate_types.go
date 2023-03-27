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

package v1beta1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	bootstrapv1 "github.com/verrazzano/cluster-api-provider-ocne/bootstrap/ocne/api/v1beta1"
)

// OcneControlPlaneTemplateSpec defines the desired state of OcneControlPlaneTemplate.
type OcneControlPlaneTemplateSpec struct {
	Template OcneControlPlaneTemplateResource `json:"template"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=ocnecontrolplanetemplates,scope=Namespaced,categories=cluster-api
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of OcneControlPlaneTemplate"

// OcneControlPlaneTemplate is the Schema for the ocnecontrolplanetemplates API.
type OcneControlPlaneTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec OcneControlPlaneTemplateSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// OcneControlPlaneTemplateList contains a list of OcneControlPlaneTemplate.
type OcneControlPlaneTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OcneControlPlaneTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OcneControlPlaneTemplate{}, &OcneControlPlaneTemplateList{})
}

// OcneControlPlaneTemplateResource describes the data needed to create a OcneControlPlane from a template.
type OcneControlPlaneTemplateResource struct {
	Spec OcneControlPlaneTemplateResourceSpec `json:"spec"`
}

// OcneControlPlaneTemplateResourceSpec defines the desired state of OcneControlPlane.
// NOTE: OcneControlPlaneTemplateResourceSpec is similar to OcneControlPlaneSpec but
// omits Replicas and Version fields. These fields do not make sense on the OcneControlPlaneTemplate,
// because they are calculated by the Cluster topology reconciler during reconciliation and thus cannot
// be configured on the OcneControlPlaneTemplate.
type OcneControlPlaneTemplateResourceSpec struct {
	// MachineTemplate contains information about how machines
	// should be shaped when creating or updating a control plane.
	// +optional
	MachineTemplate *OcneControlPlaneTemplateMachineTemplate `json:"machineTemplate,omitempty"`

	// OcneConfigSpec is a OcneConfigSpec
	// to use for initializing and joining machines to the control plane.
	OcneConfigSpec bootstrapv1.OcneConfigSpec `json:"ocneConfigSpec"`

	// RolloutBefore is a field to indicate a rollout should be performed
	// if the specified criteria is met.
	//
	// +optional
	RolloutBefore *RolloutBefore `json:"rolloutBefore,omitempty"`

	// RolloutAfter is a field to indicate a rollout should be performed
	// after the specified time even if no changes have been made to the
	// OcneControlPlane.
	//
	// +optional
	RolloutAfter *metav1.Time `json:"rolloutAfter,omitempty"`

	// The RolloutStrategy to use to replace control plane machines with
	// new ones.
	// +optional
	// +kubebuilder:default={type: "RollingUpdate", rollingUpdate: {maxSurge: 1}}
	RolloutStrategy *RolloutStrategy `json:"rolloutStrategy,omitempty"`
}

// OcneControlPlaneTemplateMachineTemplate defines the template for Machines
// in a OcneControlPlaneTemplate object.
// NOTE: OcneControlPlaneTemplateMachineTemplate is similar to OcneControlPlaneMachineTemplate but
// omits ObjectMeta and InfrastructureRef fields. These fields do not make sense on the OcneControlPlaneTemplate,
// because they are calculated by the Cluster topology reconciler during reconciliation and thus cannot
// be configured on the OcneControlPlaneTemplate.
type OcneControlPlaneTemplateMachineTemplate struct {
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
