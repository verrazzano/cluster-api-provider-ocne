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

package v1alpha1_test

import (
	"testing"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	bootstrapv1 "github.com/verrazzano/cluster-api-provider-ocne/bootstrap/ocne/api/v1alpha1"
	utildefaulting "github.com/verrazzano/cluster-api-provider-ocne/util/defaulting"
)

func TestOCNEConfigTemplateDefault(t *testing.T) {
	g := NewWithT(t)

	kubeadmConfigTemplate := &bootstrapv1.OCNEConfigTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "foo",
		},
	}
	updateDefaultingKubeadmConfigTemplate := kubeadmConfigTemplate.DeepCopy()
	updateDefaultingKubeadmConfigTemplate.Spec.Template.Spec.Verbosity = pointer.Int32(4)
	t.Run("for OCNEConfigTemplate", utildefaulting.DefaultValidateTest(updateDefaultingKubeadmConfigTemplate))

	kubeadmConfigTemplate.Default()

	g.Expect(kubeadmConfigTemplate.Spec.Template.Spec.Format).To(Equal(bootstrapv1.CloudConfig))
}

func TestOCNEConfigTemplateValidation(t *testing.T) {
	cases := map[string]struct {
		in *bootstrapv1.OCNEConfigTemplate
	}{
		"valid configuration": {
			in: &bootstrapv1.OCNEConfigTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "baz",
					Namespace: "default",
				},
				Spec: bootstrapv1.OCNEConfigTemplateSpec{
					Template: bootstrapv1.OCNEConfigTemplateResource{
						Spec: bootstrapv1.OCNEConfigSpec{},
					},
				},
			},
		},
	}

	for name, tt := range cases {
		tt := tt

		t.Run(name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(tt.in.ValidateCreate()).To(Succeed())
			g.Expect(tt.in.ValidateUpdate(nil)).To(Succeed())
		})
	}
}
