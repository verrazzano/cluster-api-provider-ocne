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
	"bytes"
	_ "embed"
	"fmt"
	"github.com/verrazzano/cluster-api-provider-ocne/internal/util/ocne"
	"strings"
	"text/template"

	"github.com/pkg/errors"

	bootstrapv1 "github.com/verrazzano/cluster-api-provider-ocne/bootstrap/ocne/api/v1beta1"
)

const (
	standardJoinCommand = "kubeadm join --config /run/kubeadm/kubeadm-join-config.yaml %s"
	// sentinelFileCommand writes a file to /run/cluster-api to signal successful Kubernetes bootstrapping in a way that
	// works both for Linux and Windows OS.
	sentinelFileCommand            = "echo success > /run/cluster-api/bootstrap-success.complete"
	retriableJoinScriptName        = "/usr/local/bin/kubeadm-bootstrap-script"
	retriableJoinScriptOwner       = "root"
	retriableJoinScriptPermissions = "0755"
	cloudConfigHeader              = `## template: jinja
#cloud-config
`
)

// BaseUserData is shared across all the various types of files written to disk.
type BaseUserData struct {
	Header               string
	PreOCNECommands      []string
	PostOCNECommands     []string
	AdditionalFiles      []bootstrapv1.File
	WriteFiles           []bootstrapv1.File
	Users                []bootstrapv1.User
	NTP                  *bootstrapv1.NTP
	DiskSetup            *bootstrapv1.DiskSetup
	Mounts               []bootstrapv1.MountPoints
	ControlPlane         bool
	UseExperimentalRetry bool
	OCNECommand          string
	OCNEVerbosity        string
	SentinelFileCommand  string
	KubernetesVersion    string
	OCNEImageRepository  string
	Proxy                *bootstrapv1.ProxySpec
	PodSubnet            string
	ServiceSubnet        string
	KubeadmVerbosity     string
}

func (input *BaseUserData) prepare() error {
	var err error
	if strings.ToLower(input.Header) != "test" {
		input.PreOCNECommands, err = ocne.GetOCNEOverrides(input.KubernetesVersion, input.OCNEImageRepository, input.PodSubnet, input.ServiceSubnet, input.Proxy)
		if err != nil {
			return err
		}
	}
	input.Header = cloudConfigHeader
	input.WriteFiles = append(input.WriteFiles, input.AdditionalFiles...)
	input.OCNECommand = fmt.Sprintf(standardJoinCommand, input.OCNEVerbosity)
	if input.UseExperimentalRetry {
		input.OCNECommand = retriableJoinScriptName
		joinScriptFile, err := generateBootstrapScript(input)
		if err != nil {
			return errors.Wrap(err, "failed to generate user data for machine joining control plane")
		}
		input.WriteFiles = append(input.WriteFiles, *joinScriptFile)
	}
	input.SentinelFileCommand = sentinelFileCommand
	return nil
}

func generate(kind string, tpl string, data interface{}) ([]byte, error) {
	tm := template.New(kind).Funcs(defaultTemplateFuncMap)
	if _, err := tm.Parse(filesTemplate); err != nil {
		return nil, errors.Wrap(err, "failed to parse files template")
	}

	if _, err := tm.Parse(commandsTemplate); err != nil {
		return nil, errors.Wrap(err, "failed to parse commands template")
	}

	if _, err := tm.Parse(ntpTemplate); err != nil {
		return nil, errors.Wrap(err, "failed to parse ntp template")
	}

	if _, err := tm.Parse(usersTemplate); err != nil {
		return nil, errors.Wrap(err, "failed to parse users template")
	}

	if _, err := tm.Parse(diskSetupTemplate); err != nil {
		return nil, errors.Wrap(err, "failed to parse disk setup template")
	}

	if _, err := tm.Parse(fsSetupTemplate); err != nil {
		return nil, errors.Wrap(err, "failed to parse fs setup template")
	}

	if _, err := tm.Parse(mountsTemplate); err != nil {
		return nil, errors.Wrap(err, "failed to parse mounts template")
	}

	t, err := tm.Parse(tpl)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse %s template", kind)
	}

	var out bytes.Buffer
	if err := t.Execute(&out, data); err != nil {
		return nil, errors.Wrapf(err, "failed to generate %s template", kind)
	}

	return out.Bytes(), nil
}

var (
	//go:embed kubeadm-bootstrap-script.sh
	kubeadmBootstrapScript string
)

func generateBootstrapScript(input interface{}) (*bootstrapv1.File, error) {
	joinScript, err := generate("JoinScript", kubeadmBootstrapScript, input)
	if err != nil {
		return nil, errors.Wrap(err, "failed to bootstrap script for machine joins")
	}
	return &bootstrapv1.File{
		Path:        retriableJoinScriptName,
		Owner:       retriableJoinScriptOwner,
		Permissions: retriableJoinScriptPermissions,
		Content:     string(joinScript),
	}, nil
}
