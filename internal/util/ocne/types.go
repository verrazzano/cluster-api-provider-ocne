package ocne

import bootstrapv1 "github.com/verrazzano/cluster-api-provider-ocne/bootstrap/ocne/api/v1alpha1"

type packages struct {
	packageName    string
	packageVersion string
}

type containerImages struct {
	containerImageName    string
	containerImageVersion string
}

type OCNEVersionData struct {
	version            string
	packageData        []packages
	containerImageData []containerImages
}

type OCNEVersionMappings struct {
	versionData []OCNEVersionData
}

// OCNEOverrideData is shared across all the various types of files written to disk.
type OCNEOverrideData struct {
	KubernetesVersion   string
	OCNEImageRepository string
	Proxy               *bootstrapv1.ProxySpec
	PodSubnet           string
	ServiceSubnet       string
	SkipInstall         bool
}
