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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	bootstrapv1 "github.com/verrazzano/cluster-api-provider-ocne/bootstrap/ocne/api/v1alpha1"
	"github.com/verrazzano/cluster-api-provider-ocne/errors"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// RolloutStrategyType defines the rollout strategies for a OCNEControlPlane.
type RolloutStrategyType string

const (
	// RollingUpdateStrategyType replaces the old control planes by new one using rolling update
	// i.e. gradually scale up or down the old control planes and scale up or down the new one.
	RollingUpdateStrategyType RolloutStrategyType = "RollingUpdate"
)

const (
	// OCNEControlPlaneFinalizer is the finalizer applied to OCNEControlPlane resources
	// by its managing controller.
	OCNEControlPlaneFinalizer = "ocne.controlplane.cluster.x-k8s.io"

	// SkipCoreDNSAnnotation annotation explicitly skips reconciling CoreDNS if set.
	SkipCoreDNSAnnotation = "controlplane.cluster.x-k8s.io/skip-coredns"

	// SkipKubeProxyAnnotation annotation explicitly skips reconciling kube-proxy if set.
	SkipKubeProxyAnnotation = "controlplane.cluster.x-k8s.io/skip-kube-proxy"

	// OCNEClusterConfigurationAnnotation is a machine annotation that stores the json-marshalled string of KCP ClusterConfiguration.
	// This annotation is used to detect any changes in ClusterConfiguration and trigger machine rollout in KCP.
	OCNEClusterConfigurationAnnotation = "controlplane.cluster.x-k8s.io/ocne-cluster-configuration"
)

// OCNEControlPlaneSpec defines the desired state of OCNEControlPlane.
type OCNEControlPlaneSpec struct {
	// Number of desired machines. Defaults to 1. When stacked etcd is used only
	// odd numbers are permitted, as per [etcd best practice](https://etcd.io/docs/v3.3.12/faq/#why-an-odd-number-of-cluster-members).
	// This is a pointer to distinguish between explicit zero and not specified.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// Version defines the desired Kubernetes version.
	// Please note that if controlPlaneConfig.ClusterConfiguration.imageRepository is not set
	// we don't allow upgrades to versions >= v1.22.0 for which kubeadm uses the old registry (k8s.gcr.io).
	// Please use a newer patch version with the new registry instead. The default registries of kubeadm are:
	//   * registry.k8s.io (new registry): >= v1.22.17, >= v1.23.15, >= v1.24.9, >= v1.25.0
	//   * k8s.gcr.io (old registry): all older versions
	Version string `json:"version"`

	// MachineTemplate contains information about how machines
	// should be shaped when creating or updating a control plane.
	MachineTemplate OCNEControlPlaneMachineTemplate `json:"machineTemplate"`

	// ControlPlaneConfig is a bootstrap OCNEConfigSpec
	// to use for initializing and joining machines to the control plane.
	ControlPlaneConfig bootstrapv1.OCNEConfigSpec `json:"controlPlaneConfig"`

	// RolloutBefore is a field to indicate a rollout should be performed
	// if the specified criteria is met.
	// +optional
	RolloutBefore *RolloutBefore `json:"rolloutBefore,omitempty"`

	// RolloutAfter is a field to indicate a rollout should be performed
	// after the specified time even if no changes have been made to the
	// OCNEControlPlane.
	// +optional
	RolloutAfter *metav1.Time `json:"rolloutAfter,omitempty"`

	// The RolloutStrategy to use to replace control plane machines with
	// new ones.
	// +optional
	// +kubebuilder:default={type: "RollingUpdate", rollingUpdate: {maxSurge: 1}}
	RolloutStrategy *RolloutStrategy `json:"rolloutStrategy,omitempty"`

	// ModuleOperator deploys the OCNE module operator to the worker cluster post installation.
	// +optional
	ModuleOperator *ModuleOperator `json:"moduleOperator,omitempty"`

	// VerrazzanoModuleOperator deploys the Verrazzano Platform operator to the worker cluster post installation.
	// +optional
	VerrazzanoModuleOperator *ModuleOperator `json:"verrazzanoModuleOperator,omitempty"`
}

type ModuleOperator struct {
	// Enabled sets the operational mode for a specific module.
	// if not set, the Enabled is set to false.
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// Image is used to set various attributes regarding a specific module.
	// If not set, they are set as per the ImageMeta definitions.
	// +optional
	Image *ImageMeta `json:"image,omitempty"`

	// ImagePullSecrets allows to specify secrets if the image is being pulled from an authenticated private registry.
	// if not set, it will be assumed the images are public.
	// +optional
	ImagePullSecrets []SecretName `json:"imagePullSecrets,omitempty"`
}

type ImageMeta struct {
	// Repository sets the container registry to pull images from.
	// if not set, the Repository defined in OCNEMeta will be used instead.
	// +optional
	Repository string `json:"repository,omitempty"`

	// Tag allows to specify a tag for the image.
	// if not set, the Tag defined in OCNEMeta will be used instead.
	// +optional
	Tag string `json:"tag,omitempty"`

	// PullPolicy allows to specify an image pull policy for the container images.
	// if not set, the PullPolicy is IfNotPresent.
	// +optional
	PullPolicy string `json:"pullPolicy,omitempty"`
}

type SecretName struct {
	Name string `json:"name,omitempty"`
}

// OCNEControlPlaneMachineTemplate defines the template for Machines
// in a OCNEControlPlane object.
type OCNEControlPlaneMachineTemplate struct {
	// Standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// +optional
	ObjectMeta clusterv1.ObjectMeta `json:"metadata,omitempty"`

	// InfrastructureRef is a required reference to a custom resource
	// offered by an infrastructure provider.
	InfrastructureRef corev1.ObjectReference `json:"infrastructureRef"`

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

// RolloutBefore describes when a rollout should be performed on the KCP machines.
type RolloutBefore struct {
	// CertificatesExpiryDays indicates a rollout needs to be performed if the
	// certificates of the machine will expire within the specified days.
	// +optional
	CertificatesExpiryDays *int32 `json:"certificatesExpiryDays,omitempty"`
}

// RolloutStrategy describes how to replace existing machines
// with new ones.
type RolloutStrategy struct {
	// Type of rollout. Currently the only supported strategy is
	// "RollingUpdate".
	// Default is RollingUpdate.
	// +optional
	Type RolloutStrategyType `json:"type,omitempty"`

	// Rolling update config params. Present only if
	// RolloutStrategyType = RollingUpdate.
	// +optional
	RollingUpdate *RollingUpdate `json:"rollingUpdate,omitempty"`
}

// RollingUpdate is used to control the desired behavior of rolling update.
type RollingUpdate struct {
	// The maximum number of control planes that can be scheduled above or under the
	// desired number of control planes.
	// Value can be an absolute number 1 or 0.
	// Defaults to 1.
	// Example: when this is set to 1, the control plane can be scaled
	// up immediately when the rolling update starts.
	// +optional
	MaxSurge *intstr.IntOrString `json:"maxSurge,omitempty"`
}

// OCNEControlPlaneStatus defines the observed state of OCNEControlPlane.
type OCNEControlPlaneStatus struct {
	// Selector is the label selector in string format to avoid introspection
	// by clients, and is used to provide the CRD-based integration for the
	// scale subresource and additional integrations for things like kubectl
	// describe.. The string will be in the same format as the query-param syntax.
	// More info about label selectors: http://kubernetes.io/docs/user-guide/labels#label-selectors
	// +optional
	Selector string `json:"selector,omitempty"`

	// Total number of non-terminated machines targeted by this control plane
	// (their labels match the selector).
	// +optional
	Replicas int32 `json:"replicas"`

	// Version represents the minimum Kubernetes version for the control plane machines
	// in the cluster.
	// +optional
	Version *string `json:"version,omitempty"`

	// Total number of non-terminated machines targeted by this control plane
	// that have the desired template spec.
	// +optional
	UpdatedReplicas int32 `json:"updatedReplicas"`

	// Total number of fully running and ready control plane machines.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas"`

	// Total number of unavailable machines targeted by this control plane.
	// This is the total number of machines that are still required for
	// the deployment to have 100% available capacity. They may either
	// be machines that are running but not yet ready or machines
	// that still have not been created.
	// +optional
	UnavailableReplicas int32 `json:"unavailableReplicas"`

	// Initialized denotes whether or not the control plane has the
	// uploaded kubeadm-config configmap.
	// +optional
	Initialized bool `json:"initialized"`

	// Ready denotes that the OCNEControlPlane API Server is ready to
	// receive requests.
	// +optional
	Ready bool `json:"ready"`

	// FailureReason indicates that there is a terminal problem reconciling the
	// state, and will be set to a token value suitable for
	// programmatic interpretation.
	// +optional
	FailureReason errors.OCNEControlPlaneStatusError `json:"failureReason,omitempty"`

	// ErrorMessage indicates that there is a terminal problem reconciling the
	// state, and will be set to a descriptive error message.
	// +optional
	FailureMessage *string `json:"failureMessage,omitempty"`

	// ObservedGeneration is the latest generation observed by the controller.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions defines current service state of the OCNEControlPlane.
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=ocnecontrolplanes,shortName=ocnecp,scope=Namespaced,categories=cluster-api
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:subresource:scale:specpath=.spec.replicas,statuspath=.status.replicas,selectorpath=.status.selector
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".metadata.labels['cluster\\.x-k8s\\.io/cluster-name']",description="Cluster"
// +kubebuilder:printcolumn:name="Initialized",type=boolean,JSONPath=".status.initialized",description="This denotes whether or not the control plane has the uploaded kubeadm-config configmap"
// +kubebuilder:printcolumn:name="API Server Available",type=boolean,JSONPath=".status.ready",description="OCNEControlPlane API Server is ready to receive requests"
// +kubebuilder:printcolumn:name="Desired",type=integer,JSONPath=".spec.replicas",description="Total number of machines desired by this control plane",priority=10
// +kubebuilder:printcolumn:name="Replicas",type=integer,JSONPath=".status.replicas",description="Total number of non-terminated machines targeted by this control plane"
// +kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=".status.readyReplicas",description="Total number of fully running and ready control plane machines"
// +kubebuilder:printcolumn:name="Updated",type=integer,JSONPath=".status.updatedReplicas",description="Total number of non-terminated machines targeted by this control plane that have the desired template spec"
// +kubebuilder:printcolumn:name="Unavailable",type=integer,JSONPath=".status.unavailableReplicas",description="Total number of unavailable machines targeted by this control plane"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of OCNEControlPlane"
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=".spec.version",description="Kubernetes version associated with this control plane"

// OCNEControlPlane is the Schema for the OCNEControlPlane API.
type OCNEControlPlane struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OCNEControlPlaneSpec   `json:"spec,omitempty"`
	Status OCNEControlPlaneStatus `json:"status,omitempty"`
}

// GetConditions returns the set of conditions for this object.
func (in *OCNEControlPlane) GetConditions() clusterv1.Conditions {
	return in.Status.Conditions
}

// SetConditions sets the conditions on this object.
func (in *OCNEControlPlane) SetConditions(conditions clusterv1.Conditions) {
	in.Status.Conditions = conditions
}

// +kubebuilder:object:root=true

// OCNEControlPlaneList contains a list of OCNEControlPlane.
type OCNEControlPlaneList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OCNEControlPlane `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OCNEControlPlane{}, &OCNEControlPlaneList{})
}
