/*
Copyright 2019 The Kubernetes Authors.

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

package cloudinit

import (
	"github.com/pkg/errors"
	"github.com/verrazzano/cluster-api-provider-ocne/internal/util/ocne"
	"github.com/verrazzano/cluster-api-provider-ocne/util/secret"
)

const (
	controlPlaneJoinCloudInit = `{{.Header}}
{{template "files" .WriteFiles}}
-   path: /run/kubeadm/kubeadm-join-config.yaml
    owner: root:root
    permissions: '0640'
    content: |
{{.JoinConfiguration | Indent 6}}
-   path: /run/cluster-api/placeholder
    owner: root:root
    permissions: '0640'
    content: "This placeholder file is used to create the /run/cluster-api sub directory in a way that is compatible with both Linux and Windows (mkdir -p /run/cluster-api does not work with Windows)"
runcmd:
{{- template "commands" .OCNEOverrides }}
{{- template "commands" .PreOCNECommands }}
  - {{ .OCNECommand }} && {{ .SentinelFileCommand }}
{{- template "commands" .PostOCNECommands }}
{{- template "ntp" .NTP }}
{{- template "users" .Users }}
{{- template "disk_setup" .DiskSetup}}
{{- template "fs_setup" .DiskSetup}}
{{- template "mounts" .Mounts}}
`
)

// ControlPlaneJoinInput defines context to generate controlplane instance user data for control plane node join.
type ControlPlaneJoinInput struct {
	BaseUserData
	secret.Certificates
	BootstrapToken    string
	JoinConfiguration string
}

// NewJoinControlPlane returns the user data string to be used on a new control plane instance.
func NewJoinControlPlane(input *ControlPlaneJoinInput) ([]byte, error) {
	// TODO: Consider validating that the correct certificates exist. It is different for external/stacked etcd
	ocneData := ocne.OCNEOverrideData{
		KubernetesVersion:   input.KubernetesVersion,
		OCNEImageRepository: input.OCNEImageRepository,
		PodSubnet:           input.PodSubnet,
		ServiceSubnet:       input.ServiceSubnet,
		Proxy:               input.Proxy,
		SkipInstall:         input.SkipInstall,
	}
	ocneCommands, err := ocne.GetOCNEOverrides(&ocneData)
	if err != nil {
		return nil, err
	}
	input.OCNEOverrides = ocneCommands
	input.WriteFiles = input.Certificates.AsFiles()
	input.ControlPlane = true
	if err := input.prepare(); err != nil {
		return nil, err
	}
	userData, err := generate("JoinControlplane", controlPlaneJoinCloudInit, input)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to generate user data for machine joining control plane")
	}

	return userData, err
}
