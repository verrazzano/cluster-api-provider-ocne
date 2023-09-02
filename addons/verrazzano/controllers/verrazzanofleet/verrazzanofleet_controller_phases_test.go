/*
Copyright 2023 The Kubernetes Authors.

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

package verrazzanofleet

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/gomega"
	addonsv1alpha1 "github.com/verrazzano/cluster-api-provider-ocne/addons/verrazzano/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ reconcile.Reconciler = &VerrazzanoFleetReconciler{}

var (
	fakeHelmChartProxy1 = &addonsv1alpha1.VerrazzanoFleet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: addonsv1alpha1.GroupVersion.String(),
			Kind:       "VerrazzanoFleet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-hcp",
			Namespace: "test-namespace",
		},
		Spec: addonsv1alpha1.VerrazzanoFleetSpec{
			ReleaseName:      "test-release-name",
			ChartName:        "test-chart-name",
			RepoURL:          "https://test-repo-url",
			ReleaseNamespace: "test-release-namespace",
			Version:          "test-version",
			ValuesTemplate:   "apiServerPort: {{ .Cluster.spec.clusterNetwork.apiServerPort }}",
			Options:          &addonsv1alpha1.HelmOptions{},
		},
	}

	fakeHelmChartProxy2 = &addonsv1alpha1.VerrazzanoFleet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: addonsv1alpha1.GroupVersion.String(),
			Kind:       "VerrazzanoFleet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-hcp",
			Namespace: "test-namespace",
		},
		Spec: addonsv1alpha1.VerrazzanoFleetSpec{
			ReleaseName:      "test-release-name",
			ChartName:        "test-chart-name",
			RepoURL:          "https://test-repo-url",
			ReleaseNamespace: "test-release-namespace",
			Version:          "test-version",
			ValuesTemplate:   "cidrBlockList: {{ .Cluster.spec.clusterNetwork.pods.cidrBlocks | join \",\" }}",
			Options:          &addonsv1alpha1.HelmOptions{},
		},
	}

	fakeInvalidHelmChartProxy = &addonsv1alpha1.VerrazzanoFleet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: addonsv1alpha1.GroupVersion.String(),
			Kind:       "VerrazzanoFleet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-hcp",
			Namespace: "test-namespace",
		},
		Spec: addonsv1alpha1.VerrazzanoFleetSpec{
			ReleaseName:      "test-release-name",
			ChartName:        "test-chart-name",
			RepoURL:          "https://test-repo-url",
			ReleaseNamespace: "test-release-namespace",
			Version:          "test-version",
			ValuesTemplate:   "apiServerPort: {{ .Cluster.invalid-path }}",
			Options:          &addonsv1alpha1.HelmOptions{},
		},
	}

	fakeReinstallHelmChartProxy = &addonsv1alpha1.VerrazzanoFleet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: addonsv1alpha1.GroupVersion.String(),
			Kind:       "VerrazzanoFleet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-hcp",
			Namespace: "test-namespace",
		},
		Spec: addonsv1alpha1.VerrazzanoFleetSpec{
			ReleaseName:      "other-release-name",
			ChartName:        "other-chart-name",
			RepoURL:          "https://other-repo-url",
			ReleaseNamespace: "test-release-namespace",
			Version:          "test-version",
			ValuesTemplate:   "apiServerPort: {{ .Cluster.spec.clusterNetwork.apiServerPort }}",
			Options:          &addonsv1alpha1.HelmOptions{},
		},
	}

	fakeCluster1 = &clusterv1.Cluster{
		TypeMeta: metav1.TypeMeta{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "Cluster",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "test-namespace",
		},
		Spec: clusterv1.ClusterSpec{
			ClusterNetwork: &clusterv1.ClusterNetwork{
				APIServerPort: pointer.Int32(6443),
				Pods: &clusterv1.NetworkRanges{
					CIDRBlocks: []string{"10.0.0.0/16", "20.0.0.0/16"},
				},
			},
		},
	}

	fakeCluster2 = &clusterv1.Cluster{
		TypeMeta: metav1.TypeMeta{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "Cluster",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "test-namespace",
		},
		Spec: clusterv1.ClusterSpec{
			ClusterNetwork: &clusterv1.ClusterNetwork{
				APIServerPort: pointer.Int32(1234),
				Pods: &clusterv1.NetworkRanges{
					CIDRBlocks: []string{"10.0.0.0/16", "20.0.0.0/16"},
				},
			},
		},
	}

	fakeVerrazzanoFleetBinding = &addonsv1alpha1.VerrazzanoFleetBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-generated-name",
			Namespace: "test-namespace",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         addonsv1alpha1.GroupVersion.String(),
					Kind:               "VerrazzanoFleet",
					Name:               "test-hcp",
					Controller:         pointer.Bool(true),
					BlockOwnerDeletion: pointer.Bool(true),
				},
			},
			Labels: map[string]string{
				clusterv1.ClusterNameLabel:              "test-cluster",
				addonsv1alpha1.VerrazzanoFleetLabelName: "test-hcp",
			},
		},
		Spec: addonsv1alpha1.VerrazzanoFleetBindingSpec{
			ClusterRef: corev1.ObjectReference{
				APIVersion: clusterv1.GroupVersion.String(),
				Kind:       "Cluster",
				Name:       "test-cluster",
				Namespace:  "test-namespace",
			},
			ReleaseName:      "test-release-name",
			ChartName:        "test-chart-name",
			RepoURL:          "https://test-repo-url",
			ReleaseNamespace: "test-release-namespace",
			Version:          "test-version",
			Values:           "apiServerPort: 6443",
			Options:          &addonsv1alpha1.HelmOptions{},
		},
	}
)

func TestReconcileForCluster(t *testing.T) {
	testcases := []struct {
		name                                string
		verrazzanoFleet                     *addonsv1alpha1.VerrazzanoFleet
		existingVerrazzanoFleetBinding      *addonsv1alpha1.VerrazzanoFleetBinding
		cluster                             *clusterv1.Cluster
		expect                              func(g *WithT, hcp *addonsv1alpha1.VerrazzanoFleet, hrp *addonsv1alpha1.VerrazzanoFleetBinding)
		expectVerrazzanoFleetBindingToExist bool
		expectedError                       string
	}{
		{
			name:                                "creates a VerrazzanoFleetBinding for a VerrazzanoFleet",
			verrazzanoFleet:                     fakeHelmChartProxy1,
			cluster:                             fakeCluster1,
			expectVerrazzanoFleetBindingToExist: true,
			expect: func(g *WithT, hcp *addonsv1alpha1.VerrazzanoFleet, hrp *addonsv1alpha1.VerrazzanoFleetBinding) {
				g.Expect(hrp.Spec.Values).To(Equal("apiServerPort: 6443"))

			},
			expectedError: "",
		},
		{
			name:                                "updates a VerrazzanoFleetBinding when Cluster value changes",
			verrazzanoFleet:                     fakeHelmChartProxy1,
			existingVerrazzanoFleetBinding:      fakeVerrazzanoFleetBinding,
			cluster:                             fakeCluster2,
			expectVerrazzanoFleetBindingToExist: true,
			expect: func(g *WithT, hcp *addonsv1alpha1.VerrazzanoFleet, hrp *addonsv1alpha1.VerrazzanoFleetBinding) {
				g.Expect(hrp.Spec.Values).To(Equal("apiServerPort: 1234"))

			},
			expectedError: "",
		},
		{
			name:                                "updates a VerrazzanoFleetBinding when valuesTemplate value changes",
			verrazzanoFleet:                     fakeHelmChartProxy2,
			existingVerrazzanoFleetBinding:      fakeVerrazzanoFleetBinding,
			cluster:                             fakeCluster2,
			expectVerrazzanoFleetBindingToExist: true,
			expect: func(g *WithT, hcp *addonsv1alpha1.VerrazzanoFleet, hrp *addonsv1alpha1.VerrazzanoFleetBinding) {
				g.Expect(hrp.Spec.Values).To(Equal("cidrBlockList: 10.0.0.0/16,20.0.0.0/16"))

			},
			expectedError: "",
		},
		{
			name:                                "set condition when failing to parse values for a VerrazzanoFleet",
			verrazzanoFleet:                     fakeInvalidHelmChartProxy,
			cluster:                             fakeCluster1,
			expectVerrazzanoFleetBindingToExist: true,
			expect: func(g *WithT, hcp *addonsv1alpha1.VerrazzanoFleet, hrp *addonsv1alpha1.VerrazzanoFleetBinding) {
				g.Expect(conditions.Has(hcp, addonsv1alpha1.VerrazzanoFleetBindingSpecsUpToDateCondition)).To(BeTrue())
				specsReady := conditions.Get(hcp, addonsv1alpha1.VerrazzanoFleetBindingSpecsUpToDateCondition)
				g.Expect(specsReady.Status).To(Equal(corev1.ConditionFalse))
				g.Expect(specsReady.Reason).To(Equal(addonsv1alpha1.ValueParsingFailedReason))
				g.Expect(specsReady.Severity).To(Equal(clusterv1.ConditionSeverityError))
				g.Expect(specsReady.Message).To(Equal("failed to parse values on cluster test-cluster: template: test-chart-name-test-cluster:1: bad character U+002D '-'"))
			},
			expectedError: "failed to parse values on cluster test-cluster: template: test-chart-name-test-cluster:1: bad character U+002D '-'",
		},
		{
			name:                                "set condition for reinstalling when requeueing after a deletion",
			verrazzanoFleet:                     fakeReinstallHelmChartProxy,
			existingVerrazzanoFleetBinding:      fakeVerrazzanoFleetBinding,
			cluster:                             fakeCluster1,
			expectVerrazzanoFleetBindingToExist: false,
			expect: func(g *WithT, hcp *addonsv1alpha1.VerrazzanoFleet, hrp *addonsv1alpha1.VerrazzanoFleetBinding) {
				g.Expect(conditions.Has(hcp, addonsv1alpha1.VerrazzanoFleetBindingSpecsUpToDateCondition)).To(BeTrue())
				specsReady := conditions.Get(hcp, addonsv1alpha1.VerrazzanoFleetBindingSpecsUpToDateCondition)
				g.Expect(specsReady.Status).To(Equal(corev1.ConditionFalse))
				g.Expect(specsReady.Reason).To(Equal(addonsv1alpha1.VerrazzanoFleetBindingReinstallingReason))
				g.Expect(specsReady.Severity).To(Equal(clusterv1.ConditionSeverityInfo))
				g.Expect(specsReady.Message).To(Equal(fmt.Sprintf("VerrazzanoFleetBinding on cluster '%s' successfully deleted, preparing to reinstall", fakeCluster1.Name)))
			},
			expectedError: "",
		},
	}

	for _, tc := range testcases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			t.Parallel()

			objects := []client.Object{tc.verrazzanoFleet, tc.cluster}
			if tc.existingVerrazzanoFleetBinding != nil {
				objects = append(objects, tc.existingVerrazzanoFleetBinding)
			}
			r := &VerrazzanoFleetReconciler{
				Client: fake.NewClientBuilder().
					WithScheme(fakeScheme).
					WithObjects(objects...).
					WithStatusSubresource(&addonsv1alpha1.VerrazzanoFleet{}).
					WithStatusSubresource(&addonsv1alpha1.VerrazzanoFleetBinding{}).
					Build(),
			}
			err := r.reconcileForCluster(ctx, tc.verrazzanoFleet, *tc.cluster)

			if tc.expectedError != "" {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err).To(MatchError(tc.expectedError), err.Error())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
				var hrp *addonsv1alpha1.VerrazzanoFleetBinding
				var err error
				if tc.expectVerrazzanoFleetBindingToExist {
					hrp, err = r.getExistingVerrazzanoFleetBinding(ctx, tc.verrazzanoFleet, tc.cluster)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(hrp).NotTo(BeNil())
				}
				tc.expect(g, tc.verrazzanoFleet, hrp)
			}
		})
	}
}

func TestConstructVerrazzanoFleetBinding(t *testing.T) {
	testCases := []struct {
		name            string
		existing        *addonsv1alpha1.VerrazzanoFleetBinding
		verrazzanoFleet *addonsv1alpha1.VerrazzanoFleet
		parsedValues    string
		cluster         *clusterv1.Cluster
		expected        *addonsv1alpha1.VerrazzanoFleetBinding
	}{
		{
			name: "existing up to date, nothing to do",
			existing: &addonsv1alpha1.VerrazzanoFleetBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-generated-name",
					Namespace: "test-namespace",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         addonsv1alpha1.GroupVersion.String(),
							Kind:               "VerrazzanoFleet",
							Name:               "test-hcp",
							Controller:         pointer.Bool(true),
							BlockOwnerDeletion: pointer.Bool(true),
						},
					},
					Labels: map[string]string{
						clusterv1.ClusterNameLabel:              "test-cluster",
						addonsv1alpha1.VerrazzanoFleetLabelName: "test-hcp",
					},
				},
				Spec: addonsv1alpha1.VerrazzanoFleetBindingSpec{
					ClusterRef: corev1.ObjectReference{
						APIVersion: clusterv1.GroupVersion.String(),
						Kind:       "Cluster",
						Name:       "test-cluster",
						Namespace:  "test-namespace",
					},
					ReleaseName:      "test-release-name",
					ChartName:        "test-chart-name",
					RepoURL:          "https://test-repo-url",
					ReleaseNamespace: "test-release-namespace",
					Values:           "test-parsed-values",
					Version:          "test-version",
					Options:          &addonsv1alpha1.HelmOptions{},
				},
			},
			verrazzanoFleet: &addonsv1alpha1.VerrazzanoFleet{
				TypeMeta: metav1.TypeMeta{
					APIVersion: addonsv1alpha1.GroupVersion.String(),
					Kind:       "VerrazzanoFleet",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hcp",
					Namespace: "test-namespace",
				},
				Spec: addonsv1alpha1.VerrazzanoFleetSpec{
					ReleaseName:      "test-release-name",
					ChartName:        "test-chart-name",
					RepoURL:          "https://test-repo-url",
					ReleaseNamespace: "test-release-namespace",
					Version:          "test-version",
					Options:          &addonsv1alpha1.HelmOptions{},
				},
			},
			parsedValues: "test-parsed-values",
			cluster: &clusterv1.Cluster{
				TypeMeta: metav1.TypeMeta{
					APIVersion: clusterv1.GroupVersion.String(),
					Kind:       "Cluster",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-namespace",
				},
			},
			expected: nil,
		},
		{
			name:     "construct helm release proxy without existing",
			existing: nil,
			verrazzanoFleet: &addonsv1alpha1.VerrazzanoFleet{
				TypeMeta: metav1.TypeMeta{
					APIVersion: addonsv1alpha1.GroupVersion.String(),
					Kind:       "VerrazzanoFleet",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hcp",
					Namespace: "test-namespace",
				},
				Spec: addonsv1alpha1.VerrazzanoFleetSpec{
					ReleaseName:      "test-release-name",
					ChartName:        "test-chart-name",
					RepoURL:          "https://test-repo-url",
					ReleaseNamespace: "test-release-namespace",
					Options:          &addonsv1alpha1.HelmOptions{},
				},
			},
			parsedValues: "test-parsed-values",
			cluster: &clusterv1.Cluster{
				TypeMeta: metav1.TypeMeta{
					APIVersion: clusterv1.GroupVersion.String(),
					Kind:       "Cluster",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-namespace",
				},
			},
			expected: &addonsv1alpha1.VerrazzanoFleetBinding{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-chart-name-test-cluster-",
					Namespace:    "test-namespace",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         addonsv1alpha1.GroupVersion.String(),
							Kind:               "VerrazzanoFleet",
							Name:               "test-hcp",
							Controller:         pointer.Bool(true),
							BlockOwnerDeletion: pointer.Bool(true),
						},
					},
					Labels: map[string]string{
						clusterv1.ClusterNameLabel:              "test-cluster",
						addonsv1alpha1.VerrazzanoFleetLabelName: "test-hcp",
					},
				},
				Spec: addonsv1alpha1.VerrazzanoFleetBindingSpec{
					ClusterRef: corev1.ObjectReference{
						APIVersion: clusterv1.GroupVersion.String(),
						Kind:       "Cluster",
						Name:       "test-cluster",
						Namespace:  "test-namespace",
					},
					ReleaseName:      "test-release-name",
					ChartName:        "test-chart-name",
					RepoURL:          "https://test-repo-url",
					ReleaseNamespace: "test-release-namespace",
					Values:           "test-parsed-values",
					Options:          &addonsv1alpha1.HelmOptions{},
				},
			},
		},
		{
			name: "version changed",
			existing: &addonsv1alpha1.VerrazzanoFleetBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-generated-name",
					Namespace: "test-namespace",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         addonsv1alpha1.GroupVersion.String(),
							Kind:               "VerrazzanoFleet",
							Name:               "test-hcp",
							Controller:         pointer.Bool(true),
							BlockOwnerDeletion: pointer.Bool(true),
						},
					},
					Labels: map[string]string{
						clusterv1.ClusterNameLabel:              "test-cluster",
						addonsv1alpha1.VerrazzanoFleetLabelName: "test-hcp",
					},
				},
				Spec: addonsv1alpha1.VerrazzanoFleetBindingSpec{
					ClusterRef: corev1.ObjectReference{
						APIVersion: clusterv1.GroupVersion.String(),
						Kind:       "Cluster",
						Name:       "test-cluster",
						Namespace:  "test-namespace",
					},
					ReleaseName:      "test-release-name",
					ChartName:        "test-chart-name",
					RepoURL:          "https://test-repo-url",
					ReleaseNamespace: "test-release-namespace",
					Values:           "test-parsed-values",
					Version:          "test-version",
					Options:          &addonsv1alpha1.HelmOptions{},
				},
			},
			verrazzanoFleet: &addonsv1alpha1.VerrazzanoFleet{
				TypeMeta: metav1.TypeMeta{
					APIVersion: addonsv1alpha1.GroupVersion.String(),
					Kind:       "VerrazzanoFleet",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hcp",
					Namespace: "test-namespace",
				},
				Spec: addonsv1alpha1.VerrazzanoFleetSpec{
					ReleaseName:      "test-release-name",
					ChartName:        "test-chart-name",
					RepoURL:          "https://test-repo-url",
					ReleaseNamespace: "test-release-namespace",
					Version:          "another-version",
					Options:          &addonsv1alpha1.HelmOptions{},
				},
			},
			parsedValues: "test-parsed-values",
			cluster: &clusterv1.Cluster{
				TypeMeta: metav1.TypeMeta{
					APIVersion: clusterv1.GroupVersion.String(),
					Kind:       "Cluster",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-namespace",
				},
			},
			expected: &addonsv1alpha1.VerrazzanoFleetBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-generated-name",
					Namespace: "test-namespace",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         addonsv1alpha1.GroupVersion.String(),
							Kind:               "VerrazzanoFleet",
							Name:               "test-hcp",
							Controller:         pointer.Bool(true),
							BlockOwnerDeletion: pointer.Bool(true),
						},
					},
					Labels: map[string]string{
						clusterv1.ClusterNameLabel:              "test-cluster",
						addonsv1alpha1.VerrazzanoFleetLabelName: "test-hcp",
					},
				},
				Spec: addonsv1alpha1.VerrazzanoFleetBindingSpec{
					ClusterRef: corev1.ObjectReference{
						APIVersion: clusterv1.GroupVersion.String(),
						Kind:       "Cluster",
						Name:       "test-cluster",
						Namespace:  "test-namespace",
					},
					ReleaseName:      "test-release-name",
					ChartName:        "test-chart-name",
					RepoURL:          "https://test-repo-url",
					ReleaseNamespace: "test-release-namespace",
					Values:           "test-parsed-values",
					Version:          "another-version",
					Options:          &addonsv1alpha1.HelmOptions{},
				},
			},
		},
		{
			name: "parsed values changed",
			existing: &addonsv1alpha1.VerrazzanoFleetBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-generated-name",
					Namespace: "test-namespace",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         addonsv1alpha1.GroupVersion.String(),
							Kind:               "VerrazzanoFleet",
							Name:               "test-hcp",
							Controller:         pointer.Bool(true),
							BlockOwnerDeletion: pointer.Bool(true),
						},
					},
					Labels: map[string]string{
						clusterv1.ClusterNameLabel:              "test-cluster",
						addonsv1alpha1.VerrazzanoFleetLabelName: "test-hcp",
					},
				},
				Spec: addonsv1alpha1.VerrazzanoFleetBindingSpec{
					ClusterRef: corev1.ObjectReference{
						APIVersion: clusterv1.GroupVersion.String(),
						Kind:       "Cluster",
						Name:       "test-cluster",
						Namespace:  "test-namespace",
					},
					ReleaseName:      "test-release-name",
					ChartName:        "test-chart-name",
					RepoURL:          "https://test-repo-url",
					ReleaseNamespace: "test-release-namespace",
					Values:           "test-parsed-values",
					Version:          "test-version",
					Options:          &addonsv1alpha1.HelmOptions{},
				},
			},
			verrazzanoFleet: &addonsv1alpha1.VerrazzanoFleet{
				TypeMeta: metav1.TypeMeta{
					APIVersion: addonsv1alpha1.GroupVersion.String(),
					Kind:       "VerrazzanoFleet",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hcp",
					Namespace: "test-namespace",
				},
				Spec: addonsv1alpha1.VerrazzanoFleetSpec{
					ReleaseName:      "test-release-name",
					ChartName:        "test-chart-name",
					RepoURL:          "https://test-repo-url",
					ReleaseNamespace: "test-release-namespace",
					Version:          "test-version",
					Options:          &addonsv1alpha1.HelmOptions{},
				},
			},
			parsedValues: "updated-parsed-values",
			cluster: &clusterv1.Cluster{
				TypeMeta: metav1.TypeMeta{
					APIVersion: clusterv1.GroupVersion.String(),
					Kind:       "Cluster",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: "test-namespace",
				},
			},
			expected: &addonsv1alpha1.VerrazzanoFleetBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-generated-name",
					Namespace: "test-namespace",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         addonsv1alpha1.GroupVersion.String(),
							Kind:               "VerrazzanoFleet",
							Name:               "test-hcp",
							Controller:         pointer.Bool(true),
							BlockOwnerDeletion: pointer.Bool(true),
						},
					},
					Labels: map[string]string{
						clusterv1.ClusterNameLabel:              "test-cluster",
						addonsv1alpha1.VerrazzanoFleetLabelName: "test-hcp",
					},
				},
				Spec: addonsv1alpha1.VerrazzanoFleetBindingSpec{
					ClusterRef: corev1.ObjectReference{
						APIVersion: clusterv1.GroupVersion.String(),
						Kind:       "Cluster",
						Name:       "test-cluster",
						Namespace:  "test-namespace",
					},
					ReleaseName:      "test-release-name",
					ChartName:        "test-chart-name",
					RepoURL:          "https://test-repo-url",
					ReleaseNamespace: "test-release-namespace",
					Values:           "updated-parsed-values",
					Version:          "test-version",
					Options:          &addonsv1alpha1.HelmOptions{},
				},
			},
		},
	}

	for _, tc := range testCases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			result := constructVerrazzanoFleetBinding(tc.existing, tc.verrazzanoFleet, tc.parsedValues, tc.cluster)
			diff := cmp.Diff(tc.expected, result)
			g.Expect(diff).To(BeEmpty())
		})
	}
}

func TestShouldReinstallHelmRelease(t *testing.T) {
	testCases := []struct {
		name                   string
		verrazzanoFleetBinding *addonsv1alpha1.VerrazzanoFleetBinding
		verrazzanoFleet        *addonsv1alpha1.VerrazzanoFleet
		reinstall              bool
	}{
		{
			name: "nothing to do",
			verrazzanoFleetBinding: &addonsv1alpha1.VerrazzanoFleetBinding{
				Spec: addonsv1alpha1.VerrazzanoFleetBindingSpec{
					ChartName:        "test-chart",
					RepoURL:          "https://test-repo-url",
					ReleaseName:      "test-release-name",
					ReleaseNamespace: "test-namespace",
				},
			},
			verrazzanoFleet: &addonsv1alpha1.VerrazzanoFleet{
				Spec: addonsv1alpha1.VerrazzanoFleetSpec{
					ChartName:        "test-chart",
					RepoURL:          "https://test-repo-url",
					ReleaseName:      "test-release-name",
					ReleaseNamespace: "test-namespace",
				},
			},
			reinstall: false,
		},
		{
			name: "chart name changed, should reinstall",
			verrazzanoFleetBinding: &addonsv1alpha1.VerrazzanoFleetBinding{
				Spec: addonsv1alpha1.VerrazzanoFleetBindingSpec{
					ChartName:        "test-chart",
					RepoURL:          "https://test-repo-url",
					ReleaseName:      "test-release-name",
					ReleaseNamespace: "test-namespace",
				},
			},
			verrazzanoFleet: &addonsv1alpha1.VerrazzanoFleet{
				Spec: addonsv1alpha1.VerrazzanoFleetSpec{
					ChartName:        "another-chart",
					RepoURL:          "https://test-repo-url",
					ReleaseName:      "test-release-name",
					ReleaseNamespace: "test-namespace",
				},
			},
			reinstall: true,
		},
		{
			name: "repo url changed, should reinstall",
			verrazzanoFleetBinding: &addonsv1alpha1.VerrazzanoFleetBinding{
				Spec: addonsv1alpha1.VerrazzanoFleetBindingSpec{
					ChartName:        "test-chart",
					RepoURL:          "https://test-repo-url",
					ReleaseName:      "test-release-name",
					ReleaseNamespace: "test-namespace",
				},
			},
			verrazzanoFleet: &addonsv1alpha1.VerrazzanoFleet{
				Spec: addonsv1alpha1.VerrazzanoFleetSpec{
					ChartName:        "test-chart",
					RepoURL:          "https://another-repo-url",
					ReleaseName:      "test-release-name",
					ReleaseNamespace: "test-namespace",
				},
			},
			reinstall: true,
		},
		{
			name: "generated release name changed, should reinstall",
			verrazzanoFleetBinding: &addonsv1alpha1.VerrazzanoFleetBinding{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						addonsv1alpha1.IsReleaseNameGeneratedAnnotation: "true",
					},
				},
				Spec: addonsv1alpha1.VerrazzanoFleetBindingSpec{
					ChartName:   "test-chart",
					RepoURL:     "https://test-repo-url",
					ReleaseName: "generated-release-name",
				},
			},
			verrazzanoFleet: &addonsv1alpha1.VerrazzanoFleet{
				Spec: addonsv1alpha1.VerrazzanoFleetSpec{
					ChartName:   "test-chart",
					RepoURL:     "https://test-repo-url",
					ReleaseName: "some-other-release-name",
				},
			},
			reinstall: true,
		},
		{
			name: "generated release name unchanged, nothing to do",
			verrazzanoFleetBinding: &addonsv1alpha1.VerrazzanoFleetBinding{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						addonsv1alpha1.IsReleaseNameGeneratedAnnotation: "true",
					},
				},
				Spec: addonsv1alpha1.VerrazzanoFleetBindingSpec{
					ChartName:   "test-chart",
					RepoURL:     "https://test-repo-url",
					ReleaseName: "generated-release-name",
				},
			},
			verrazzanoFleet: &addonsv1alpha1.VerrazzanoFleet{
				Spec: addonsv1alpha1.VerrazzanoFleetSpec{
					ChartName:   "test-chart",
					RepoURL:     "https://test-repo-url",
					ReleaseName: "",
				},
			},
			reinstall: false,
		},
		{
			name: "non-generated release name changed, should reinstall",
			verrazzanoFleetBinding: &addonsv1alpha1.VerrazzanoFleetBinding{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						addonsv1alpha1.IsReleaseNameGeneratedAnnotation: "true",
					},
				},
				Spec: addonsv1alpha1.VerrazzanoFleetBindingSpec{
					ChartName:   "test-chart",
					RepoURL:     "https://test-repo-url",
					ReleaseName: "test-release-name",
				},
			},
			verrazzanoFleet: &addonsv1alpha1.VerrazzanoFleet{
				Spec: addonsv1alpha1.VerrazzanoFleetSpec{
					ChartName:   "test-chart",
					RepoURL:     "https://test-repo-url",
					ReleaseName: "some-other-release-name",
				},
			},
			reinstall: true,
		},
		{
			name: "release namespace changed, should reinstall",
			verrazzanoFleetBinding: &addonsv1alpha1.VerrazzanoFleetBinding{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						addonsv1alpha1.IsReleaseNameGeneratedAnnotation: "true",
					},
				},
				Spec: addonsv1alpha1.VerrazzanoFleetBindingSpec{
					ChartName:        "test-chart",
					RepoURL:          "https://test-repo-url",
					ReleaseName:      "test-release-name",
					ReleaseNamespace: "test-namespace",
				},
			},
			verrazzanoFleet: &addonsv1alpha1.VerrazzanoFleet{
				Spec: addonsv1alpha1.VerrazzanoFleetSpec{
					ChartName:        "test-chart",
					RepoURL:          "https://test-repo-url",
					ReleaseName:      "test-release-name",
					ReleaseNamespace: "some-other-namespace",
				},
			},
			reinstall: true,
		},
	}

	for _, tc := range testCases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			result := shouldReinstallHelmRelease(ctx, tc.verrazzanoFleetBinding, tc.verrazzanoFleet)
			g.Expect(result).To(Equal(tc.reinstall))
		})
	}
}

func TestGetOrphanedVerrazzanoFleetBindings(t *testing.T) {
	testCases := []struct {
		name                    string
		selectedClusters        []clusterv1.Cluster
		verrazzanoFleetBindings []addonsv1alpha1.VerrazzanoFleetBinding
		releasesToDelete        []addonsv1alpha1.VerrazzanoFleetBinding
	}{
		{
			name: "nothing to do",
			selectedClusters: []clusterv1.Cluster{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-cluster-1",
						Namespace: "test-namespace-1",
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-cluster-2",
						Namespace: "test-namespace-2",
					},
				},
			},
			verrazzanoFleetBindings: []addonsv1alpha1.VerrazzanoFleetBinding{
				{
					Spec: addonsv1alpha1.VerrazzanoFleetBindingSpec{
						ClusterRef: corev1.ObjectReference{
							Name:      "test-cluster-1",
							Namespace: "test-namespace-1",
						},
					},
				},
				{
					Spec: addonsv1alpha1.VerrazzanoFleetBindingSpec{
						ClusterRef: corev1.ObjectReference{
							Name:      "test-cluster-2",
							Namespace: "test-namespace-2",
						},
					},
				},
			},
			releasesToDelete: []addonsv1alpha1.VerrazzanoFleetBinding{},
		},
		{
			name: "delete one release",
			selectedClusters: []clusterv1.Cluster{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-cluster-1",
						Namespace: "test-namespace-1",
					},
				},
			},
			verrazzanoFleetBindings: []addonsv1alpha1.VerrazzanoFleetBinding{
				{
					Spec: addonsv1alpha1.VerrazzanoFleetBindingSpec{
						ClusterRef: corev1.ObjectReference{
							Name:      "test-cluster-1",
							Namespace: "test-namespace-1",
						},
					},
				},
				{
					Spec: addonsv1alpha1.VerrazzanoFleetBindingSpec{
						ClusterRef: corev1.ObjectReference{
							Name:      "test-cluster-2",
							Namespace: "test-namespace-2",
						},
					},
				},
			},
			releasesToDelete: []addonsv1alpha1.VerrazzanoFleetBinding{
				{
					Spec: addonsv1alpha1.VerrazzanoFleetBindingSpec{
						ClusterRef: corev1.ObjectReference{
							Name:      "test-cluster-2",
							Namespace: "test-namespace-2",
						},
					},
				},
			},
		},
		{
			name:             "delete both releases",
			selectedClusters: []clusterv1.Cluster{},
			verrazzanoFleetBindings: []addonsv1alpha1.VerrazzanoFleetBinding{
				{
					Spec: addonsv1alpha1.VerrazzanoFleetBindingSpec{
						ClusterRef: corev1.ObjectReference{
							Name:      "test-cluster-1",
							Namespace: "test-namespace-1",
						},
					},
				},
				{
					Spec: addonsv1alpha1.VerrazzanoFleetBindingSpec{
						ClusterRef: corev1.ObjectReference{
							Name:      "test-cluster-2",
							Namespace: "test-namespace-2",
						},
					},
				},
			},
			releasesToDelete: []addonsv1alpha1.VerrazzanoFleetBinding{
				{
					Spec: addonsv1alpha1.VerrazzanoFleetBindingSpec{
						ClusterRef: corev1.ObjectReference{
							Name:      "test-cluster-1",
							Namespace: "test-namespace-1",
						},
					},
				},
				{
					Spec: addonsv1alpha1.VerrazzanoFleetBindingSpec{
						ClusterRef: corev1.ObjectReference{
							Name:      "test-cluster-2",
							Namespace: "test-namespace-2",
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			result := getOrphanedVerrazzanoFleetBindings(ctx, tc.selectedClusters, tc.verrazzanoFleetBindings)
			g.Expect(result).To(Equal(tc.releasesToDelete))
		})
	}
}
