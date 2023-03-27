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

package upstreamv1beta3

import (
	"testing"

	fuzz "github.com/google/gofuzz"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/apitesting/fuzzer"
	"k8s.io/apimachinery/pkg/runtime"
	runtimeserializer "k8s.io/apimachinery/pkg/runtime/serializer"

	bootstrapv1 "github.com/verrazzano/cluster-api-provider-ocne/bootstrap/ocne/api/v1beta1"
	utilconversion "github.com/verrazzano/cluster-api-provider-ocne/util/conversion"
)

func TestFuzzyConversion(t *testing.T) {
	g := NewWithT(t)
	scheme := runtime.NewScheme()
	g.Expect(AddToScheme(scheme)).To(Succeed())
	g.Expect(bootstrapv1.AddToScheme(scheme)).To(Succeed())

	t.Run("for ClusterConfiguration", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Scheme: scheme,
		Hub:    &bootstrapv1.ClusterConfiguration{},
		Spoke:  &ClusterConfiguration{},
		// NOTE: Ocne types does not have ObjectMeta, so we are required to skip data annotation cleanup in the spoke-hub-spoke round trip test.
		SkipSpokeAnnotationCleanup: true,
		FuzzerFuncs:                []fuzzer.FuzzerFuncs{fuzzFuncs},
	}))
	t.Run("for InitConfiguration", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Scheme: scheme,
		Hub:    &bootstrapv1.InitConfiguration{},
		Spoke:  &InitConfiguration{},
		// NOTE: Ocne types does not have ObjectMeta, so we are required to skip data annotation cleanup in the spoke-hub-spoke round trip test.
		SkipSpokeAnnotationCleanup: true,
		FuzzerFuncs:                []fuzzer.FuzzerFuncs{fuzzFuncs},
	}))
	t.Run("for JoinConfiguration", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Scheme: scheme,
		Hub:    &bootstrapv1.JoinConfiguration{},
		Spoke:  &JoinConfiguration{},
		// NOTE: Ocne types does not have ObjectMeta, so we are required to skip data annotation cleanup in the spoke-hub-spoke round trip test.
		SkipSpokeAnnotationCleanup: true,
		FuzzerFuncs:                []fuzzer.FuzzerFuncs{fuzzFuncs},
	}))
}

func fuzzFuncs(_ runtimeserializer.CodecFactory) []interface{} {
	return []interface{}{
		nodeRegistrationOptionsFuzzer,
		initConfigurationFuzzer,
		joinConfigurationFuzzer,
		joinControlPlanesFuzzer,
	}
}

func nodeRegistrationOptionsFuzzer(obj *NodeRegistrationOptions, c fuzz.Continue) {
	c.FuzzNoCustom(obj)

	// NodeRegistrationOptions.IgnorePreflightErrors does not exists in v1alpha4, so setting it to nil in order to avoid v1beta3 --> v1alpha4 --> v1beta3 round trip errors.
	obj.IgnorePreflightErrors = nil
}

func joinControlPlanesFuzzer(obj *JoinControlPlane, c fuzz.Continue) {
	c.FuzzNoCustom(obj)

	// JoinControlPlane.CertificateKey does not exists in v1alpha4, so setting it to empty string in order to avoid v1beta3 --> v1alpha4 --> v1beta3 round trip errors.
	obj.CertificateKey = ""
}

func initConfigurationFuzzer(obj *InitConfiguration, c fuzz.Continue) {
	c.Fuzz(obj)

	// InitConfiguration.CertificateKey does not exists in v1alpha4, so setting it to empty string in order to avoid v1beta3 --> v1alpha4 --> v1beta3 round trip errors.
	obj.CertificateKey = ""

	// InitConfiguration.SkipPhases does not exists in v1alpha4, so setting it to empty string in order to avoid v1beta3 --> v1alpha4 --> v1beta3 round trip errors.
	obj.SkipPhases = nil
}

func joinConfigurationFuzzer(obj *JoinConfiguration, c fuzz.Continue) {
	c.Fuzz(obj)

	// JoinConfiguration.SkipPhases does not exists in v1alpha4, so setting it to empty string in order to avoid v1beta3 --> v1alpha4 --> v1beta3 round trip errors.
	obj.SkipPhases = nil
}
