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

// Package clc generates bootstrap data in Ignition format using Container Linux Config Transpiler.
//
// CLC configuration defined in this package will run kubeadm command by creating a /etc/kubeadm.sh script
// file containing both pre and post kubeadm commands as well as the kubeadm command itself.
//
// /etc/kubeadm.sh script will be executed using kubeadm.service systemd unit, which will only happen
// if /etc/kubeadm.yml file exists, which ensures the script will run only once, as by the end of the
// script, /etc/kubeadm.yml is moved to /tmp filesystem, so it gets automatically cleaned up after a
// reboot. This is to align the implementation with cloud-init, which places kubeadm configuration in
// /tmp directory directly, which is not possible with Ignition.
//
// /etc/kubeadm.yml file contains generated kubeadm configuration and can be customized using pre kubeadm
// commands if needed, as a replacement for Jinja templates supported by cloud-init, for example
// using 'envsubst' or 'sed'.
//
// To override the behavior of kubeadm.service unit, one should create an override drop-in
// using AdditionalConfig field. Data from this field takes precedence and will be merged with
// configuration generated by the bootstrap provider, overriding already defined fields following the
// merge strategy described in https://coreos.github.io/ignition/operator-notes/#config-merging.
package clc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"

	clct "github.com/flatcar/container-linux-config-transpiler/config"
	ignition "github.com/flatcar/ignition/config/v2_3"
	ignitionTypes "github.com/flatcar/ignition/config/v2_3/types"
	"github.com/pkg/errors"

	bootstrapv1 "github.com/verrazzano/cluster-api-provider-ocne/bootstrap/ocne/api/v1beta1"
	"github.com/verrazzano/cluster-api-provider-ocne/bootstrap/ocne/internal/cloudinit"
)

const (
	clcTemplate = `---
{{- if .Users }}
passwd:
  users:
    {{- range .Users }}
    - name: {{ .Name }}
      {{- with .Gecos }}
      gecos: {{ . }}
      {{- end }}
      {{- if .Groups }}
      groups:
        {{- range Split .Groups ", " }}
        - {{ . }}
        {{- end }}
      {{- end }}
      {{- with .HomeDir }}
      home_dir: {{ . }}
      {{- end }}
      {{- with .Shell }}
      shell: {{ . }}
      {{- end }}
      {{- with .Passwd }}
      password_hash: {{ . }}
      {{- end }}
      {{- with .PrimaryGroup }}
      primary_group: {{ . }}
      {{- end }}
      {{- if .SSHAuthorizedKeys }}
      ssh_authorized_keys:
        {{- range .SSHAuthorizedKeys }}
        - {{ . }}
        {{- end }}
      {{- end }}
    {{- end }}
{{- end }}
systemd:
  units:
    - name: kubeadm.service
      enabled: true
      contents: |
        [Unit]
        Description=kubeadm
        # Run only once. After successful run, this file is moved to /tmp/.
        ConditionPathExists=/etc/kubeadm.yml
        [Service]
        # To not restart the unit when it exits, as it is expected.
        Type=oneshot
        ExecStart=/etc/kubeadm.sh
        [Install]
        WantedBy=multi-user.target
    {{- if .NTP }}{{ if .NTP.Enabled }}
    - name: ntpd.service
      enabled: true
    {{- end }}{{- end }}
    {{- range .Mounts }}
    {{- $label := index . 0 }}
    {{- $mountpoint := index . 1 }}
    {{- $disk := index $.FilesystemDevicesByLabel $label }}
    {{- $mountOptions := slice . 2 }}
    - name: {{ $mountpoint | MountpointName }}.mount
      enabled: true
      contents: |
        [Unit]
        Description = Mount {{ $label }}

        [Mount]
        What={{ $disk }}
        Where={{ $mountpoint }}
        Options={{ Join $mountOptions "," }}

        [Install]
        WantedBy=multi-user.target
    {{- end }}
storage:
  {{- if .DiskSetup }}{{- if .DiskSetup.Partitions }}
  disks:
    {{- range .DiskSetup.Partitions }}
    - device: {{ .Device }}
      {{- with .Overwrite }}
      wipe_table: {{ . }}
      {{- end }}
      {{- if .Layout }}
      partitions:
      - {}
      {{- end }}
    {{- end }}
  {{- end }}{{- end }}
  {{- if .DiskSetup }}{{- if .DiskSetup.Filesystems }}
  filesystems:
    {{- range .DiskSetup.Filesystems }}
    - name: {{ .Label }}
      mount:
        device: {{ .Device }}
        format: {{ .Filesystem }}
        wipe_filesystem: {{ .Overwrite }}
        label: {{ .Label }}
        {{- if .ExtraOpts }}
        options:
          {{- range .ExtraOpts }}
          - {{ . }}
          {{- end }}
        {{- end }}
    {{- end }}
  {{- end }}{{- end }}
  files:
    {{- range .Users }}
    {{- if .Sudo }}
    - path: /etc/sudoers.d/{{ .Name }}
      mode: 0600
      contents:
        inline: |
          {{ .Name }} {{ .Sudo }}
    {{- end }}
    {{- end }}
    {{- with .UsersWithPasswordAuth }}
    - path: /etc/ssh/sshd_config
      mode: 0600
      contents:
        inline: |
          # Use most defaults for sshd configuration.
          Subsystem sftp internal-sftp
          ClientAliveInterval 180
          UseDNS no
          UsePAM yes
          PrintLastLog no # handled by PAM
          PrintMotd no # handled by PAM

          Match User {{ . }}
            PasswordAuthentication yes
    {{- end }}
    {{- range .WriteFiles }}
    - path: {{ .Path }}
      {{- $owner := ParseOwner .Owner }}
      {{ if $owner.User -}}
      user:
        name: {{ $owner.User }}
      {{- end }}
      {{ if $owner.Group -}}
      group:
        name: {{ $owner.Group }}
      {{- end }}
      # Owner
      {{ if ne .Permissions "" -}}
      mode: {{ .Permissions }}
      {{ end -}}
      contents:
        {{ if eq .Encoding "base64" -}}
        inline: !!binary |
        {{- else -}}
        inline: |
        {{- end }}
          {{ .Content | Indent 10 }}
    {{- end }}
    - path: /etc/kubeadm.sh
      mode: 0700
      contents:
        inline: |
          #!/bin/bash
          set -e
          {{ range .PreOCNECommands }}
          {{ . | Indent 10 }}
          {{- end }}

          {{ .OCNECommand }}
          mkdir -p /run/cluster-api && echo success > /run/cluster-api/bootstrap-success.complete
          mv /etc/kubeadm.yml /tmp/
          {{range .PostOCNECommands }}
          {{ . | Indent 10 }}
          {{- end }}
    - path: /etc/kubeadm.yml
      mode: 0600
      contents:
        inline: |
          ---
          {{ .OCNEConfig | Indent 10 }}
    {{- if .NTP }}{{- if and .NTP.Enabled .NTP.Servers }}
    - path: /etc/ntp.conf
      mode: 0644
      contents:
        inline: |
          # Common pool
          {{- range  .NTP.Servers }}
          server {{ . }}
          {{- end }}

          # Warning: Using default NTP settings will leave your NTP
          # server accessible to all hosts on the Internet.

          # If you want to deny all machines (including your own)
          # from accessing the NTP server, uncomment:
          #restrict default ignore

          # Default configuration:
          # - Allow only time queries, at a limited rate, sending KoD when in excess.
          # - Allow all local queries (IPv4, IPv6)
          restrict default nomodify nopeer noquery notrap limited kod
          restrict 127.0.0.1
          restrict [::1]
    {{- end }}{{- end }}
`
)

type render struct {
	*cloudinit.BaseUserData

	OCNEConfig               string
	UsersWithPasswordAuth    string
	FilesystemDevicesByLabel map[string]string
}

func defaultTemplateFuncMap() template.FuncMap {
	return template.FuncMap{
		"Indent":         templateYAMLIndent,
		"Split":          strings.Split,
		"Join":           strings.Join,
		"MountpointName": mountpointName,
		"ParseOwner":     parseOwner,
	}
}

func mountpointName(name string) string {
	return strings.TrimPrefix(strings.ReplaceAll(name, "/", "-"), "-")
}

func templateYAMLIndent(i int, input string) string {
	split := strings.Split(input, "\n")
	ident := "\n" + strings.Repeat(" ", i)
	return strings.Join(split, ident)
}

type owner struct {
	User  *string
	Group *string
}

func parseOwner(ownerRaw string) owner {
	if ownerRaw == "" {
		return owner{}
	}

	ownerSlice := strings.Split(ownerRaw, ":")

	parseEntity := func(entity string) *string {
		if entity == "" {
			return nil
		}

		entityTrimmed := strings.TrimSpace(entity)

		return &entityTrimmed
	}

	if len(ownerSlice) == 1 {
		return owner{
			User: parseEntity(ownerSlice[0]),
		}
	}

	return owner{
		User:  parseEntity(ownerSlice[0]),
		Group: parseEntity(ownerSlice[1]),
	}
}

func renderCLC(input *cloudinit.BaseUserData, ocneConfig string) ([]byte, error) {
	t := template.Must(template.New("template").Funcs(defaultTemplateFuncMap()).Parse(clcTemplate))

	usersWithPasswordAuth := []string{}
	for _, user := range input.Users {
		if user.LockPassword != nil && !*user.LockPassword {
			usersWithPasswordAuth = append(usersWithPasswordAuth, user.Name)
		}
	}

	filesystemDevicesByLabel := map[string]string{}
	if input.DiskSetup != nil {
		for _, filesystem := range input.DiskSetup.Filesystems {
			filesystemDevicesByLabel[filesystem.Label] = filesystem.Device
		}
	}

	data := render{
		BaseUserData:             input,
		OCNEConfig:               ocneConfig,
		UsersWithPasswordAuth:    strings.Join(usersWithPasswordAuth, ","),
		FilesystemDevicesByLabel: filesystemDevicesByLabel,
	}

	var out bytes.Buffer
	if err := t.Execute(&out, data); err != nil {
		return nil, errors.Wrapf(err, "failed to render template")
	}

	return out.Bytes(), nil
}

// Render renders the provided user data and CLC snippets into Ignition config.
func Render(input *cloudinit.BaseUserData, clc *bootstrapv1.ContainerLinuxConfig, ocneConfig string) ([]byte, string, error) {
	if input == nil {
		return nil, "", errors.New("empty base user data")
	}

	clcBytes, err := renderCLC(input, ocneConfig)
	if err != nil {
		return nil, "", errors.Wrapf(err, "rendering CLC configuration")
	}

	userData, warnings, err := buildIgnitionConfig(clcBytes, clc)
	if err != nil {
		return nil, "", errors.Wrapf(err, "building Ignition config")
	}

	return userData, warnings, nil
}

func buildIgnitionConfig(baseCLC []byte, clc *bootstrapv1.ContainerLinuxConfig) ([]byte, string, error) {
	// We control baseCLC config, so treat it as strict.
	ign, _, err := clcToIgnition(baseCLC, true)
	if err != nil {
		return nil, "", errors.Wrapf(err, "converting generated CLC to Ignition")
	}

	var clcWarnings string

	if clc != nil && clc.AdditionalConfig != "" {
		additionalIgn, warnings, err := clcToIgnition([]byte(clc.AdditionalConfig), clc.Strict)
		if err != nil {
			return nil, "", errors.Wrapf(err, "converting additional CLC to Ignition")
		}

		clcWarnings = warnings

		ign = ignition.Append(ign, additionalIgn)
	}

	userData, err := json.Marshal(&ign)
	if err != nil {
		return nil, "", errors.Wrapf(err, "marshaling generated Ignition config into JSON")
	}

	return userData, clcWarnings, nil
}

func clcToIgnition(data []byte, strict bool) (ignitionTypes.Config, string, error) {
	clc, ast, reports := clct.Parse(data)

	if (len(reports.Entries) > 0 && strict) || reports.IsFatal() {
		return ignitionTypes.Config{}, "", fmt.Errorf("error parsing Container Linux Config: %v", reports.String())
	}

	ign, report := clct.Convert(clc, "", ast)
	if (len(report.Entries) > 0 && strict) || report.IsFatal() {
		return ignitionTypes.Config{}, "", fmt.Errorf("error converting to Ignition: %v", report.String())
	}

	reports.Merge(report)

	return ign, reports.String(), nil
}
