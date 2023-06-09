/*
Copyright 2020 The Kubernetes Authors.

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

const (
	fsSetupTemplate = `{{ define "fs_setup" -}}
{{- if . }}
fs_setup:{{ range .Filesystems }}
  - label: {{ .Label }}
    filesystem: {{ .Filesystem }}
    device: {{ .Device }}
  {{- if .Partition }}
    partition: {{ .Partition }}
  {{- end }}
  {{- if .Overwrite }}
    overwrite: {{ .Overwrite }}
  {{- end }}
  {{- if .ReplaceFS }}
    replace_fs: {{ .ReplaceFS }}
  {{- end }}
  {{- if .ExtraOpts }}
    extra_opts: {{- range .ExtraOpts }}
      - {{ . }}
        {{- end -}}
  {{- end -}}
{{- end -}}
{{- end -}}
{{- end -}}
`
)
