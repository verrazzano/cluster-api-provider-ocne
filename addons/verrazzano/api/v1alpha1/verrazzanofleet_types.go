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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

const (
	// HelmChartFinalizer is the finalizer used by the VerrazzanoFleet controller to cleanup add-on resources when
	// a VerrazzanoFleet is being deleted.
	VerrazzanoFleetFinalizer = "verrazzanofleet.addons.cluster.x-k8s.io"
)

// VerrazzanoFleetSpec defines the desired state of VerrazzanoFleet.
type VerrazzanoFleetSpec struct {
	// ClusterSelector selects Clusters in the same namespace with a label that matches the specified label selector. The Helm
	// chart will be installed on all selected Clusters. If a Cluster is no longer selected, the Helm release will be uninstalled.
	ClusterSelector metav1.LabelSelector `json:"clusterSelector"`

	// ChartName is the name of the Helm chart in the repository.
	// e.g. chart-path oci://repo-url/chart-name as chartName: chart-name and https://repo-url/chart-name as chartName: chart-name
	ChartName string `json:"chartName"`

	// RepoURL is the URL of the Helm chart repository.
	// e.g. chart-path oci://repo-url/chart-name as repoURL: oci://repo-url and https://repo-url/chart-name as repoURL: https://repo-url
	RepoURL string `json:"repoURL"`

	// ReleaseName is the release name of the installed Helm chart. If it is not specified, a name will be generated.
	// +optional
	ReleaseName string `json:"releaseName,omitempty"`

	// ReleaseNamespace is the namespace the Helm release will be installed on each selected
	// Cluster. If it is not specified, it will be set to the default namespace.
	// +optional
	ReleaseNamespace string `json:"namespace,omitempty"`

	// Version is the version of the Helm chart. If it is not specified, the chart will use
	// and be kept up to date with the latest version.
	// +optional
	Version string `json:"version,omitempty"`

	// ValuesTemplate is an inline YAML representing the values for the Helm chart. This YAML supports Go templating to reference
	// fields from each selected workload Cluster and programatically create and set values.
	// +optional
	ValuesTemplate string `json:"valuesTemplate,omitempty"`

	// Options represents CLI flags passed to Helm operations (i.e. install, upgrade, delete) and
	// include options such as wait, skipCRDs, timeout, waitForJobs, etc.
	// +optional
	Options *HelmOptions `json:"options,omitempty"`
}

type HelmOptions struct {
	// DisableHooks prevents hooks from running during the Helm install action.
	// +optional
	DisableHooks bool `json:"disableHooks,omitempty"`

	// Wait enables the waiting for resources to be ready after a Helm install/upgrade has been performed.
	// +optional
	Wait bool `json:"wait,omitempty"`

	// WaitForJobs enables waiting for jobs to complete after a Helm install/upgrade has been performed.
	// +optional
	WaitForJobs bool `json:"waitForJobs,omitempty"`

	// DependencyUpdate indicates the Helm install/upgrade action to get missing dependencies.
	// +optional
	DependencyUpdate bool `json:"dependencyUpdate,omitempty"`

	// Timeout is the time to wait for any individual Kubernetes operation (like
	// resource creation, Jobs for hooks, etc.) during the performance of a Helm install action.
	// Defaults to '10 min'.
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// SkipCRDs controls whether CRDs should be installed during install/upgrade operation.
	// By default, CRDs are installed if not already present.
	// If set, no CRDs will be installed.
	// +optional
	SkipCRDs bool `json:"skipCRDs,omitempty"`

	// SubNotes determines whether sub-notes should be rendered in the chart.
	// +optional
	SubNotes bool `json:"options,omitempty"`

	// DisableOpenAPIValidation controls whether OpenAPI validation is enforced.
	// +optional
	DisableOpenAPIValidation bool `json:"disableOpenAPIValidation,omitempty"`

	// Atomic indicates the installation/upgrade process to delete the installation or rollback on failure.
	// If 'Atomic' is set, wait will be enabled automatically during helm install/upgrade operation.
	// +optional
	Atomic bool `json:"atomic,omitempty"`

	// Install represents CLI flags passed to Helm install operation which can be used to control
	// behaviour of helm Install operations via options like wait, skipCrds, timeout, waitForJobs, etc.
	// +optional
	Install *HelmInstallOptions `json:"install,omitempty"`

	// Upgrade represents CLI flags passed to Helm upgrade operation which can be used to control
	// behaviour of helm Upgrade operations via options like wait, skipCrds, timeout, waitForJobs, etc.
	// +optional
	Upgrade *HelmUpgradeOptions `json:"upgrade,omitempty"`

	// Uninstall represents CLI flags passed to Helm uninstall operation which can be used to control
	// behaviour of helm Uninstall operation via options like wait, timeout, etc.
	// +optional
	Uninstall *HelmUninstallOptions `json:"uninstall,omitempty"`
}

type HelmInstallOptions struct {
	// CreateNamespace indicates the Helm install/upgrade action to create the
	// VerrazzanoFleetSpec.ReleaseNamespace if it does not exist yet.
	// On uninstall, the namespace will not be garbage collected.
	// If it is not specified by user, will be set to default 'true'.
	// +optional
	CreateNamespace *bool `json:"createNamespace,omitempty"`

	// IncludeCRDs determines whether CRDs stored as a part of helm templates directory should be installed.
	// +optional
	IncludeCRDs bool `json:"includeCRDs,omitempty"`
}

type HelmUpgradeOptions struct {
	// Force indicates to ignore certain warnings and perform the helm release upgrade anyway.
	// This should be used with caution.
	// +optional
	Force bool `json:"force,omitempty"`

	// ResetValues will reset the values to the chart's built-ins rather than merging with existing.
	// +optional
	ResetValues bool `json:"resetValues,omitempty"`

	// ReuseValues will re-use the user's last supplied values.
	// +optional
	ReuseValues bool `json:"reuseValues,omitempty"`

	// Recreate will (if true) recreate pods after a rollback.
	// +optional
	Recreate bool `json:"recreate,omitempty"`

	// MaxHistory limits the maximum number of revisions saved per release
	// +optional
	MaxHistory int `json:"maxHistory,omitempty"`

	// CleanupOnFail indicates the upgrade action to delete newly-created resources on a failed update operation.
	// +optional
	CleanupOnFail bool `json:"cleanupOnFail,omitempty"`
}

type HelmUninstallOptions struct {
	// KeepHistory defines whether historical revisions of a release should be saved.
	// If it's set, helm uninstall operation will not delete the history of the release.
	// The helm storage backend (secret, configmap, etc) will be retained in the cluster.
	// +optional
	KeepHistory bool `json:"keepHistory,omitempty"`

	// Description represents human readable information to be shown on release uninstall.
	// +optional
	Description string `json:"description,omitempty"`
}

// VerrazzanoFleetStatus defines the observed state of VerrazzanoFleet.
type VerrazzanoFleetStatus struct {
	// Conditions defines current state of the VerrazzanoFleet.
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`

	// MatchingClusters is the list of references to Clusters selected by the ClusterSelector.
	// +optional
	MatchingClusters []corev1.ObjectReference `json:"matchingClusters"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="Reason",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].reason"
// +kubebuilder:printcolumn:name="Message",type="string",priority=1,JSONPath=".status.conditions[?(@.type=='Ready')].message"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Time duration since creation of VerrazzanoFleet"
// +kubebuilder:resource:shortName=vf;vfs,scope=Namespaced,categories=cluster-api

// VerrazzanoFleet is the Schema for the verrazzanofleets API
type VerrazzanoFleet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VerrazzanoFleetSpec   `json:"spec,omitempty"`
	Status VerrazzanoFleetStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// VerrazzanoFleetList contains a list of VerrazzanoFleet
type VerrazzanoFleetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VerrazzanoFleet `json:"items"`
}

// GetConditions returns the list of conditions for an VerrazzanoFleet API object.
func (c *VerrazzanoFleet) GetConditions() clusterv1.Conditions {
	return c.Status.Conditions
}

// SetConditions will set the given conditions on an VerrazzanoFleet object.
func (c *VerrazzanoFleet) SetConditions(conditions clusterv1.Conditions) {
	c.Status.Conditions = conditions
}

// SetMatchingClusters will set the given list of matching clusters on an VerrazzanoFleet object.
func (c *VerrazzanoFleet) SetMatchingClusters(clusterList []clusterv1.Cluster) {
	matchingClusters := make([]corev1.ObjectReference, 0, len(clusterList))
	for _, cluster := range clusterList {
		matchingClusters = append(matchingClusters, corev1.ObjectReference{
			Kind:       cluster.Kind,
			APIVersion: cluster.APIVersion,
			Name:       cluster.Name,
			Namespace:  cluster.Namespace,
		})
	}

	c.Status.MatchingClusters = matchingClusters
}

func init() {
	SchemeBuilder.Register(&VerrazzanoFleet{}, &VerrazzanoFleetList{})
}
