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
package ocne

import (
	"fmt"
	bootstrapv1 "github.com/verrazzano/cluster-api-provider-ocne/bootstrap/ocne/api/v1alpha1"
	"testing"

	"github.com/blang/semver"
	. "github.com/onsi/gomega"
)

func TestGetDefaultRegistry(t *testing.T) {
	tests := []struct {
		version          string
		expectedRegistry string
	}{
		// < v1.22
		{version: "1.19.1", expectedRegistry: OldDefaultImageRepository},
		{version: "1.20.5", expectedRegistry: OldDefaultImageRepository},
		{version: "1.21.85", expectedRegistry: OldDefaultImageRepository},

		// v1.22
		{version: "1.22.0", expectedRegistry: OldDefaultImageRepository},
		{version: "1.22.16", expectedRegistry: OldDefaultImageRepository},
		{version: "1.22.17", expectedRegistry: DefaultImageRepository},
		{version: "1.22.99", expectedRegistry: DefaultImageRepository},

		// v1.23
		{version: "1.23.0", expectedRegistry: OldDefaultImageRepository},
		{version: "1.23.14", expectedRegistry: OldDefaultImageRepository},
		{version: "1.23.15", expectedRegistry: DefaultImageRepository},
		{version: "1.23.99", expectedRegistry: DefaultImageRepository},

		// v1.24
		{version: "1.24.0", expectedRegistry: OldDefaultImageRepository},
		{version: "1.24.8", expectedRegistry: DefaultOCNEImageRepository},
		{version: "1.24.9", expectedRegistry: DefaultOCNEImageRepository},
		{version: "1.24.99", expectedRegistry: DefaultOCNEImageRepository},

		// > v1.24
		{version: "1.25.0", expectedRegistry: DefaultOCNEImageRepository},
		{version: "1.26.1", expectedRegistry: DefaultOCNEImageRepository},
		{version: "1.27.2", expectedRegistry: DefaultOCNEImageRepository},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s => %s", tt.version, tt.expectedRegistry), func(t *testing.T) {
			g := NewWithT(t)

			g.Expect(GetDefaultRegistry(semver.MustParse(tt.version))).To(Equal(tt.expectedRegistry))
		})
	}
}

// TestGetOCNEOverrides verified the TestGetOCNEOverrides method with various combinations
func TestGetOCNEOverrides(t *testing.T) {
	tests := []struct {
		testName          string
		kubernetesVersion string
		ocneImageRepo     string
		podSubnet         string
		serviceSubnet     string
		proxy             *bootstrapv1.ProxySpec
		expectedError     bool
		overrideLength    int
	}{
		{
			testName:          "Valid K8s version and proxy",
			kubernetesVersion: "1.24.8",
			ocneImageRepo:     "foo",
			podSubnet:         "1.1.1.1/24",
			serviceSubnet:     "2.2.2.2/24",
			proxy: &bootstrapv1.ProxySpec{
				HttpProxy:  "foo",
				HttpsProxy: "bar",
				NoProxy:    "hello",
			},
			expectedError:  false,
			overrideLength: 22,
		},
		{
			testName:          "Not Supported K8s version",
			kubernetesVersion: "1.24.7",
			ocneImageRepo:     "foo",
			podSubnet:         "1.1.1.1/24",
			serviceSubnet:     "2.2.2.2/24",
			proxy: &bootstrapv1.ProxySpec{
				HttpProxy:  "foo",
				HttpsProxy: "bar",
				NoProxy:    "hello",
			},
			expectedError:  true,
			overrideLength: 0,
		},
		{
			testName:          "Not Supported K8s version",
			kubernetesVersion: "1.25.6",
			ocneImageRepo:     "foo",
			podSubnet:         "1.1.1.1/24",
			serviceSubnet:     "2.2.2.2/24",
			proxy: &bootstrapv1.ProxySpec{
				HttpProxy:  "foo",
				HttpsProxy: "bar",
				NoProxy:    "hello",
			},
			expectedError:  true,
			overrideLength: 0,
		},
		{
			testName:          "Supported K8s version and no proxy",
			kubernetesVersion: "1.24.8",
			ocneImageRepo:     "foo",
			podSubnet:         "1.1.1.1/24",
			serviceSubnet:     "2.2.2.2/24",
			expectedError:     false,
			overrideLength:    18,
		},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s", tt.testName), func(t *testing.T) {
			g := NewWithT(t)
			data, err := GetOCNEOverrides(tt.kubernetesVersion, tt.ocneImageRepo, tt.podSubnet, tt.serviceSubnet, tt.proxy)
			if tt.expectedError {
				// if expectedErr is true, then err returned is not nil
				g.Expect(err).To(Not(BeNil()))
			} else {
				// if expectedErr is false, then err returned is nil
				g.Expect(err).To(BeNil())
			}
			g.Expect(len(data)).To(Equal(tt.overrideLength))
		})
	}
}
