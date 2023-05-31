package helm

import (
	controlplanev1 "github.com/verrazzano/cluster-api-provider-ocne/controlplane/ocne/api/v1alpha1"
)

type HelmModuleAddons struct {
	// ChartLocation is the URL of the Helm chart repository.
	RepoURL string `json:"repoURL"`

	// ChartName is the name of the Helm chart in the repository.
	ChartName string `json:"chartName"`

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

	// valuesTemplate is an inline YAML representing the values for the Helm chart. This YAML supports Go templating to reference
	// fields from each selected workload Cluster and programatically create and set values.
	// +optional
	ValuesTemplate string `json:"valuesTemplate,omitempty"`

	// Local indicates whether chart is local or needs to be pulled in from the internet.
	// When Local is set to true the RepoURL would be the local path to the chart directory in the container.
	// By default, Local is set to false.
	// +optional
	Local bool `json:"local,omitempty"`
}

type HelmValuesTemplate struct {
	Repository       string                      `json:"repository,omitempty"`
	Tag              string                      `json:"tag,omitempty"`
	PullPolicy       string                      `json:"pullPolicy,omitempty"`
	ImagePullSecrets []controlplanev1.SecretName `json:"imagePullSecrets,omitempty"`
}
