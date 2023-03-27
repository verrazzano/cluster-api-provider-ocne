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

package v1beta1

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilfeature "k8s.io/component-base/featuregate/testing"

	bootstrapv1 "github.com/verrazzano/cluster-api-provider-ocne/bootstrap/ocne/api/v1beta1"
	"github.com/verrazzano/cluster-api-provider-ocne/feature"
	utildefaulting "github.com/verrazzano/cluster-api-provider-ocne/util/defaulting"
)

func TestKubeadmControlPlaneTemplateDefault(t *testing.T) {
	defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)()

	g := NewWithT(t)

	kcpTemplate := &OcneControlPlaneTemplate{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "foo",
		},
		Spec: OcneControlPlaneTemplateSpec{
			Template: OcneControlPlaneTemplateResource{
				Spec: OcneControlPlaneTemplateResourceSpec{
					MachineTemplate: &OcneControlPlaneTemplateMachineTemplate{
						NodeDrainTimeout: &metav1.Duration{Duration: 10 * time.Second},
					},
				},
			},
		},
	}
	updateDefaultingValidationKCPTemplate := kcpTemplate.DeepCopy()
	updateDefaultingValidationKCPTemplate.Spec.Template.Spec.MachineTemplate.NodeDrainTimeout = &metav1.Duration{Duration: 20 * time.Second}
	t.Run("for OcneControlPlaneTemplate", utildefaulting.DefaultValidateTest(updateDefaultingValidationKCPTemplate))
	kcpTemplate.Default()

	g.Expect(kcpTemplate.Spec.Template.Spec.OcneConfigSpec.Format).To(Equal(bootstrapv1.CloudConfig))
	g.Expect(kcpTemplate.Spec.Template.Spec.RolloutStrategy.Type).To(Equal(RollingUpdateStrategyType))
	g.Expect(kcpTemplate.Spec.Template.Spec.RolloutStrategy.RollingUpdate.MaxSurge.IntVal).To(Equal(int32(1)))
}

func TestKubeadmControlPlaneTemplateValidationFeatureGateEnabled(t *testing.T) {
	defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.ClusterTopology, true)()

	t.Run("create ocnecontrolplanetemplate should pass if gate enabled and valid ocnecontrolplanetemplate", func(t *testing.T) {
		testnamespace := "test"
		g := NewWithT(t)
		kcpTemplate := &OcneControlPlaneTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ocnecontrolplanetemplate-test",
				Namespace: testnamespace,
			},
			Spec: OcneControlPlaneTemplateSpec{
				Template: OcneControlPlaneTemplateResource{
					Spec: OcneControlPlaneTemplateResourceSpec{
						MachineTemplate: &OcneControlPlaneTemplateMachineTemplate{
							NodeDrainTimeout: &metav1.Duration{Duration: time.Second},
						},
					},
				},
			},
		}
		g.Expect(kcpTemplate.ValidateCreate()).To(Succeed())
	})
}

func TestKubeadmControlPlaneTemplateValidationFeatureGateDisabled(t *testing.T) {
	// NOTE: ClusterTopology feature flag is disabled by default, thus preventing to create OcneControlPlaneTemplate.
	t.Run("create ocnecontrolplanetemplate should not pass if gate disabled and valid ocnecontrolplanetemplate", func(t *testing.T) {
		testnamespace := "test"
		g := NewWithT(t)
		kcpTemplate := &OcneControlPlaneTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ocnecontrolplanetemplate-test",
				Namespace: testnamespace,
			},
			Spec: OcneControlPlaneTemplateSpec{
				Template: OcneControlPlaneTemplateResource{
					Spec: OcneControlPlaneTemplateResourceSpec{
						MachineTemplate: &OcneControlPlaneTemplateMachineTemplate{
							NodeDrainTimeout: &metav1.Duration{Duration: time.Second},
						},
					},
				},
			},
		}
		g.Expect(kcpTemplate.ValidateCreate()).NotTo(Succeed())
	})
}
