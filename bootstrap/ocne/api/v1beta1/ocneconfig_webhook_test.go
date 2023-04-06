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

package v1beta1

import (
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilfeature "k8s.io/component-base/featuregate/testing"
	"k8s.io/utils/pointer"

	"github.com/verrazzano/cluster-api-provider-ocne/feature"
	utildefaulting "github.com/verrazzano/cluster-api-provider-ocne/util/defaulting"
)

func TestOCNEConfigDefault(t *testing.T) {
	defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)()

	g := NewWithT(t)

	kubeadmConfig := &OCNEConfig{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "foo",
		},
		Spec: OCNEConfigSpec{},
	}
	updateDefaultingKubeadmConfig := kubeadmConfig.DeepCopy()
	updateDefaultingKubeadmConfig.Spec.Verbosity = pointer.Int32(4)
	t.Run("for OCNEConfig", utildefaulting.DefaultValidateTest(updateDefaultingKubeadmConfig))

	kubeadmConfig.Default()

	g.Expect(kubeadmConfig.Spec.Format).To(Equal(CloudConfig))

	ignitionKubeadmConfig := &OCNEConfig{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "foo",
		},
		Spec: OCNEConfigSpec{
			Format: Ignition,
		},
	}
	ignitionKubeadmConfig.Default()
	g.Expect(ignitionKubeadmConfig.Spec.Format).To(Equal(Ignition))
}

func TestOCNEConfigValidate(t *testing.T) {
	cases := map[string]struct {
		in                    *OCNEConfig
		enableIgnitionFeature bool
		expectErr             bool
	}{
		"valid content": {
			in: &OCNEConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "baz",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: OCNEConfigSpec{
					Files: []File{
						{
							Content: "foo",
						},
					},
				},
			},
		},
		"valid contentFrom": {
			in: &OCNEConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "baz",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: OCNEConfigSpec{
					Files: []File{
						{
							ContentFrom: &FileSource{
								Secret: SecretFileSource{
									Name: "foo",
									Key:  "bar",
								},
							},
						},
					},
				},
			},
		},
		"invalid content and contentFrom": {
			in: &OCNEConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "baz",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: OCNEConfigSpec{
					Files: []File{
						{
							ContentFrom: &FileSource{},
							Content:     "foo",
						},
					},
				},
			},
			expectErr: true,
		},
		"invalid contentFrom without name": {
			in: &OCNEConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "baz",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: OCNEConfigSpec{
					Files: []File{
						{
							ContentFrom: &FileSource{
								Secret: SecretFileSource{
									Key: "bar",
								},
							},
							Content: "foo",
						},
					},
				},
			},
			expectErr: true,
		},
		"invalid contentFrom without key": {
			in: &OCNEConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "baz",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: OCNEConfigSpec{
					Files: []File{
						{
							ContentFrom: &FileSource{
								Secret: SecretFileSource{
									Name: "foo",
								},
							},
							Content: "foo",
						},
					},
				},
			},
			expectErr: true,
		},
		"invalid with duplicate file path": {
			in: &OCNEConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "baz",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: OCNEConfigSpec{
					Files: []File{
						{
							Content: "foo",
						},
						{
							Content: "bar",
						},
					},
				},
			},
			expectErr: true,
		},
		"valid passwd": {
			in: &OCNEConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "baz",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: OCNEConfigSpec{
					Users: []User{
						{
							Passwd: pointer.String("foo"),
						},
					},
				},
			},
		},
		"valid passwdFrom": {
			in: &OCNEConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "baz",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: OCNEConfigSpec{
					Users: []User{
						{
							PasswdFrom: &PasswdSource{
								Secret: SecretPasswdSource{
									Name: "foo",
									Key:  "bar",
								},
							},
						},
					},
				},
			},
		},
		"invalid passwd and passwdFrom": {
			in: &OCNEConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "baz",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: OCNEConfigSpec{
					Users: []User{
						{
							PasswdFrom: &PasswdSource{},
							Passwd:     pointer.String("foo"),
						},
					},
				},
			},
			expectErr: true,
		},
		"invalid passwdFrom without name": {
			in: &OCNEConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "baz",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: OCNEConfigSpec{
					Users: []User{
						{
							PasswdFrom: &PasswdSource{
								Secret: SecretPasswdSource{
									Key: "bar",
								},
							},
							Passwd: pointer.String("foo"),
						},
					},
				},
			},
			expectErr: true,
		},
		"invalid passwdFrom without key": {
			in: &OCNEConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "baz",
					Namespace: metav1.NamespaceDefault,
				},
				Spec: OCNEConfigSpec{
					Users: []User{
						{
							PasswdFrom: &PasswdSource{
								Secret: SecretPasswdSource{
									Name: "foo",
								},
							},
							Passwd: pointer.String("foo"),
						},
					},
				},
			},
			expectErr: true,
		},
		"Ignition field is set, format is not Ignition": {
			enableIgnitionFeature: true,
			in: &OCNEConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "baz",
					Namespace: "default",
				},
				Spec: OCNEConfigSpec{
					Ignition: &IgnitionSpec{},
				},
			},
			expectErr: true,
		},
		"Ignition field is not set, format is Ignition": {
			enableIgnitionFeature: true,
			in: &OCNEConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "baz",
					Namespace: "default",
				},
				Spec: OCNEConfigSpec{
					Format: Ignition,
				},
			},
		},
		"format is Ignition, user is inactive": {
			enableIgnitionFeature: true,
			in: &OCNEConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "baz",
					Namespace: "default",
				},
				Spec: OCNEConfigSpec{
					Format: Ignition,
					Users: []User{
						{
							Inactive: pointer.Bool(true),
						},
					},
				},
			},
			expectErr: true,
		},
		"format is Ignition, non-GPT partition configured": {
			enableIgnitionFeature: true,
			in: &OCNEConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "baz",
					Namespace: "default",
				},
				Spec: OCNEConfigSpec{
					Format: Ignition,
					DiskSetup: &DiskSetup{
						Partitions: []Partition{
							{
								TableType: pointer.String("MS-DOS"),
							},
						},
					},
				},
			},
			expectErr: true,
		},
		"format is Ignition, experimental retry join is set": {
			enableIgnitionFeature: true,
			in: &OCNEConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "baz",
					Namespace: "default",
				},
				Spec: OCNEConfigSpec{
					Format:                   Ignition,
					UseExperimentalRetryJoin: true,
				},
			},
			expectErr: true,
		},
		"feature gate disabled, format is Ignition": {
			in: &OCNEConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "baz",
					Namespace: "default",
				},
				Spec: OCNEConfigSpec{
					Format: Ignition,
				},
			},
			expectErr: true,
		},
		"feature gate disabled, Ignition field is set": {
			in: &OCNEConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "baz",
					Namespace: "default",
				},
				Spec: OCNEConfigSpec{
					Format: Ignition,
					Ignition: &IgnitionSpec{
						ContainerLinuxConfig: &ContainerLinuxConfig{},
					},
				},
			},
			expectErr: true,
		},
		"replaceFS specified with Ignition": {
			enableIgnitionFeature: true,
			in: &OCNEConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "baz",
					Namespace: "default",
				},
				Spec: OCNEConfigSpec{
					Format: Ignition,
					DiskSetup: &DiskSetup{
						Filesystems: []Filesystem{
							{
								ReplaceFS: pointer.String("ntfs"),
							},
						},
					},
				},
			},
			expectErr: true,
		},
		"filesystem partition specified with Ignition": {
			enableIgnitionFeature: true,
			in: &OCNEConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "baz",
					Namespace: "default",
				},
				Spec: OCNEConfigSpec{
					Format: Ignition,
					DiskSetup: &DiskSetup{
						Filesystems: []Filesystem{
							{
								Partition: pointer.String("1"),
							},
						},
					},
				},
			},
			expectErr: true,
		},
		"file encoding gzip specified with Ignition": {
			enableIgnitionFeature: true,
			in: &OCNEConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "baz",
					Namespace: "default",
				},
				Spec: OCNEConfigSpec{
					Format: Ignition,
					Files: []File{
						{
							Encoding: Gzip,
						},
					},
				},
			},
			expectErr: true,
		},
		"file encoding gzip+base64 specified with Ignition": {
			enableIgnitionFeature: true,
			in: &OCNEConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "baz",
					Namespace: "default",
				},
				Spec: OCNEConfigSpec{
					Format: Ignition,
					Files: []File{
						{
							Encoding: GzipBase64,
						},
					},
				},
			},
			expectErr: true,
		},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			if tt.enableIgnitionFeature {
				// NOTE: KubeadmBootstrapFormatIgnition feature flag is disabled by default.
				// Enabling the feature flag temporarily for this test.
				defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.KubeadmBootstrapFormatIgnition, true)()
			}
			g := NewWithT(t)
			if tt.expectErr {
				g.Expect(tt.in.ValidateCreate()).NotTo(Succeed())
				g.Expect(tt.in.ValidateUpdate(nil)).NotTo(Succeed())
			} else {
				g.Expect(tt.in.ValidateCreate()).To(Succeed())
				g.Expect(tt.in.ValidateUpdate(nil)).To(Succeed())
			}
		})
	}
}
