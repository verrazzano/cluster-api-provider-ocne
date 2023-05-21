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
	"github.com/verrazzano/cluster-api-provider-ocne/internal/util/ocne"
	ocnemeta "github.com/verrazzano/cluster-api-provider-ocne/util/ocne"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	corev1Cli "k8s.io/client-go/kubernetes/typed/core/v1"
	"os"
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/utils/pointer"

	bootstrapv1 "github.com/verrazzano/cluster-api-provider-ocne/bootstrap/ocne/api/v1alpha1"
	"github.com/verrazzano/cluster-api-provider-ocne/util/certs"
	"github.com/verrazzano/cluster-api-provider-ocne/util/secret"
)

const (
	configMapName        = "ocne-metadata"
	k8sversionsFile      = "../../../../util/ocne/testdata/kubernetes_versions.yaml"
	capiDefaultNamespace = "capi-ocne-control-plane-system"
)

func TestNewInitControlPlaneAdditionalFileEncodings(t *testing.T) {
	g := NewWithT(t)

	ocneMeta, err := ocnemeta.GetMetaDataContents(k8sversionsFile)
	g.Expect(err).To(BeNil())
	namespace, ok := os.LookupEnv("POD_NAMESPACE")
	if !ok {
		namespace = capiDefaultNamespace
	}
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: namespace,
		},
		Data: ocneMeta,
	}
	ocne.GetCoreV1Func = func() (corev1Cli.CoreV1Interface, error) {
		return k8sfake.NewSimpleClientset(configMap).CoreV1(), nil
	}
	defer func() { ocne.GetCoreV1Func = ocne.GetCoreV1Client }()

	cpinput := &ControlPlaneInput{
		BaseUserData: BaseUserData{
			Header:           "test",
			PreOCNECommands:  nil,
			PostOCNECommands: nil,
			AdditionalFiles: []bootstrapv1.File{
				{
					Path:     "/tmp/my-path",
					Encoding: bootstrapv1.Base64,
					Content:  "aGk=",
				},
				{
					Path:    "/tmp/my-other-path",
					Content: "hi",
				},
				{
					Path:    "/tmp/existing-path",
					Append:  true,
					Content: "hi",
				},
			},
			WriteFiles: nil,
			Users:      nil,
			NTP:        nil,
		},
		Certificates:         secret.Certificates{},
		ClusterConfiguration: "my-cluster-config",
		InitConfiguration:    "my-init-config",
	}

	for _, certificate := range cpinput.Certificates {
		certificate.KeyPair = &certs.KeyPair{
			Cert: []byte("some certificate"),
			Key:  []byte("some key"),
		}
	}

	out, err := NewInitControlPlane(cpinput)
	g.Expect(err).NotTo(HaveOccurred())

	expectedFiles := []string{
		`-   path: /tmp/my-path
    encoding: "base64"
    content: |
      aGk=`,
		`-   path: /tmp/my-other-path
    content: |
      hi`,
		`-   path: /tmp/existing-path
    append: true
    content: |
      hi`,
	}
	for _, f := range expectedFiles {
		g.Expect(out).To(ContainSubstring(f))
	}
}

func TestNewInitControlPlaneCommands(t *testing.T) {
	g := NewWithT(t)

	ocneMeta, err := ocnemeta.GetMetaDataContents(k8sversionsFile)
	g.Expect(err).To(BeNil())
	namespace, ok := os.LookupEnv("POD_NAMESPACE")
	if !ok {
		namespace = capiDefaultNamespace
	}
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: namespace,
		},
		Data: ocneMeta,
	}
	ocne.GetCoreV1Func = func() (corev1Cli.CoreV1Interface, error) {
		return k8sfake.NewSimpleClientset(configMap).CoreV1(), nil
	}
	defer func() { ocne.GetCoreV1Func = ocne.GetCoreV1Client }()

	cpinput := &ControlPlaneInput{
		BaseUserData: BaseUserData{
			Header:           "test",
			PreOCNECommands:  []string{`"echo $(date) ': hello world!'"`},
			PostOCNECommands: []string{"echo $(date) ': hello world!'"},
			AdditionalFiles:  nil,
			WriteFiles:       nil,
			Users:            nil,
			NTP:              nil,
		},
		Certificates:         secret.Certificates{},
		ClusterConfiguration: "my-cluster-config",
		InitConfiguration:    "my-init-config",
	}

	for _, certificate := range cpinput.Certificates {
		certificate.KeyPair = &certs.KeyPair{
			Cert: []byte("some certificate"),
			Key:  []byte("some key"),
		}
	}

	out, err := NewInitControlPlane(cpinput)
	g.Expect(err).NotTo(HaveOccurred())

	expectedCommands := []string{
		`"\"echo $(date) ': hello world!'\""`,
		`"echo $(date) ': hello world!'"`,
	}
	for _, f := range expectedCommands {
		g.Expect(out).To(ContainSubstring(f))
	}
}

func TestNewInitControlPlaneDiskMounts(t *testing.T) {
	g := NewWithT(t)

	ocneMeta, err := ocnemeta.GetMetaDataContents(k8sversionsFile)
	g.Expect(err).To(BeNil())
	namespace, ok := os.LookupEnv("POD_NAMESPACE")
	if !ok {
		namespace = capiDefaultNamespace
	}
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: namespace,
		},
		Data: ocneMeta,
	}
	ocne.GetCoreV1Func = func() (corev1Cli.CoreV1Interface, error) {
		return k8sfake.NewSimpleClientset(configMap).CoreV1(), nil
	}
	defer func() { ocne.GetCoreV1Func = ocne.GetCoreV1Client }()

	cpinput := &ControlPlaneInput{
		BaseUserData: BaseUserData{
			Header:           "test",
			PreOCNECommands:  nil,
			PostOCNECommands: nil,
			WriteFiles:       nil,
			Users:            nil,
			NTP:              nil,
			DiskSetup: &bootstrapv1.DiskSetup{
				Partitions: []bootstrapv1.Partition{
					{
						Device:    "test-device",
						Layout:    true,
						Overwrite: pointer.Bool(false),
						TableType: pointer.String("gpt"),
					},
				},
				Filesystems: []bootstrapv1.Filesystem{
					{
						Device:     "test-device",
						Filesystem: "ext4",
						Label:      "test_disk",
						ExtraOpts:  []string{"-F", "-E", "lazy_itable_init=1,lazy_journal_init=1"},
					},
				},
			},
			Mounts: []bootstrapv1.MountPoints{
				{"test_disk", "/var/lib/testdir"},
			},
		},
		Certificates:         secret.Certificates{},
		ClusterConfiguration: "my-cluster-config",
		InitConfiguration:    "my-init-config",
	}

	out, err := NewInitControlPlane(cpinput)
	g.Expect(err).NotTo(HaveOccurred())

	expectedDiskSetup := `disk_setup:
  test-device:
    table_type: gpt
    layout: true
    overwrite: false`
	expectedFSSetup := `fs_setup:
  - label: test_disk
    filesystem: ext4
    device: test-device
    extra_opts:
      - -F
      - -E
      - lazy_itable_init=1,lazy_journal_init=1`
	expectedMounts := `mounts:
  - - test_disk
    - /var/lib/testdir`

	g.Expect(string(out)).To(ContainSubstring(expectedDiskSetup))
	g.Expect(string(out)).To(ContainSubstring(expectedFSSetup))
	g.Expect(string(out)).To(ContainSubstring(expectedMounts))
}

func TestNewJoinControlPlaneAdditionalFileEncodings(t *testing.T) {
	g := NewWithT(t)

	ocneMeta, err := ocnemeta.GetMetaDataContents(k8sversionsFile)
	g.Expect(err).To(BeNil())
	namespace, ok := os.LookupEnv("POD_NAMESPACE")
	if !ok {
		namespace = capiDefaultNamespace
	}
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: namespace,
		},
		Data: ocneMeta,
	}
	ocne.GetCoreV1Func = func() (corev1Cli.CoreV1Interface, error) {
		return k8sfake.NewSimpleClientset(configMap).CoreV1(), nil
	}
	defer func() { ocne.GetCoreV1Func = ocne.GetCoreV1Client }()

	cpinput := &ControlPlaneJoinInput{
		BaseUserData: BaseUserData{
			Header:           "test",
			PreOCNECommands:  nil,
			PostOCNECommands: nil,
			AdditionalFiles: []bootstrapv1.File{
				{
					Path:     "/tmp/my-path",
					Encoding: bootstrapv1.Base64,
					Content:  "aGk=",
				},
				{
					Path:    "/tmp/my-other-path",
					Content: "hi",
				},
			},
			WriteFiles: nil,
			Users:      nil,
			NTP:        nil,
		},
		Certificates:      secret.Certificates{},
		BootstrapToken:    "my-bootstrap-token",
		JoinConfiguration: "my-join-config",
	}

	for _, certificate := range cpinput.Certificates {
		certificate.KeyPair = &certs.KeyPair{
			Cert: []byte("some certificate"),
			Key:  []byte("some key"),
		}
	}

	out, err := NewJoinControlPlane(cpinput)
	g.Expect(err).NotTo(HaveOccurred())

	expectedFiles := []string{
		`-   path: /tmp/my-path
    encoding: "base64"
    content: |
      aGk=`,
		`-   path: /tmp/my-other-path
    content: |
      hi`,
	}
	for _, f := range expectedFiles {
		g.Expect(out).To(ContainSubstring(f))
	}
}

func TestNewJoinControlPlaneExperimentalRetry(t *testing.T) {
	g := NewWithT(t)

	ocneMeta, err := ocnemeta.GetMetaDataContents(k8sversionsFile)
	g.Expect(err).To(BeNil())
	namespace, ok := os.LookupEnv("POD_NAMESPACE")
	if !ok {
		namespace = capiDefaultNamespace
	}
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: namespace,
		},
		Data: ocneMeta,
	}
	ocne.GetCoreV1Func = func() (corev1Cli.CoreV1Interface, error) {
		return k8sfake.NewSimpleClientset(configMap).CoreV1(), nil
	}
	defer func() { ocne.GetCoreV1Func = ocne.GetCoreV1Client }()

	cpinput := &ControlPlaneJoinInput{
		BaseUserData: BaseUserData{
			Header:               "test",
			PreOCNECommands:      nil,
			PostOCNECommands:     nil,
			UseExperimentalRetry: true,
			WriteFiles:           nil,
			Users:                nil,
			NTP:                  nil,
		},
		Certificates:      secret.Certificates{},
		BootstrapToken:    "my-bootstrap-token",
		JoinConfiguration: "my-join-config",
	}

	for _, certificate := range cpinput.Certificates {
		certificate.KeyPair = &certs.KeyPair{
			Cert: []byte("some certificate"),
			Key:  []byte("some key"),
		}
	}

	out, err := NewJoinControlPlane(cpinput)
	g.Expect(err).NotTo(HaveOccurred())

	expectedFiles := []string{
		`-   path: ` + retriableJoinScriptName + `
    owner: ` + retriableJoinScriptOwner + `
    permissions: '` + retriableJoinScriptPermissions + `'
    `,
	}
	for _, f := range expectedFiles {
		g.Expect(out).To(ContainSubstring(f))
	}
}
