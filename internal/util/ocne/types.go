package ocne

import bootstrapv1 "github.com/verrazzano/cluster-api-provider-ocne/bootstrap/ocne/api/v1alpha1"

// OCNEOverrideData is shared across all the various types of files written to disk.
type OCNEOverrideData struct {
	KubernetesVersion   string
	OCNEImageRepository string
	Proxy               *bootstrapv1.ProxySpec
	PodSubnet           string
	ServiceSubnet       string
	SkipInstall         bool
	AddonInstall        []bootstrapv1.AddonInstall
}
