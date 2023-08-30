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

package verrazzanorelease

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

var _ reconcile.Reconciler = &VerrazzanoReleaseReconciler{}

var (
	fakeHelmChartProxy1 = &addonsv1alpha1.VerrazzanoRelease{
		TypeMeta: metav1.TypeMeta{
			APIVersion: addonsv1alpha1.GroupVersion.String(),
			Kind:       "VerrazzanoRelease",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-hcp",
			Namespace: "test-namespace",
		},
		Spec: addonsv1alpha1.VerrazzanoReleaseSpec{
			ReleaseName:      "test-release-name",
			ChartName:        "test-chart-name",
			RepoURL:          "https://test-repo-url",
			ReleaseNamespace: "test-release-namespace",
			Version:          "test-version",
			ValuesTemplate:   "apiServerPort: {{ .Cluster.spec.clusterNetwork.apiServerPort }}",
			Options:          &addonsv1alpha1.HelmOptions{},
		},
	}

	fakeHelmChartProxy2 = &addonsv1alpha1.VerrazzanoRelease{
		TypeMeta: metav1.TypeMeta{
			APIVersion: addonsv1alpha1.GroupVersion.String(),
			Kind:       "VerrazzanoRelease",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-hcp",
			Namespace: "test-namespace",
		},
		Spec: addonsv1alpha1.VerrazzanoReleaseSpec{
			ReleaseName:      "test-release-name",
			ChartName:        "test-chart-name",
			RepoURL:          "https://test-repo-url",
			ReleaseNamespace: "test-release-namespace",
			Version:          "test-version",
			ValuesTemplate:   "cidrBlockList: {{ .Cluster.spec.clusterNetwork.pods.cidrBlocks | join \",\" }}",
			Options:          &addonsv1alpha1.HelmOptions{},
		},
	}

	fakeInvalidHelmChartProxy = &addonsv1alpha1.VerrazzanoRelease{
		TypeMeta: metav1.TypeMeta{
			APIVersion: addonsv1alpha1.GroupVersion.String(),
			Kind:       "VerrazzanoRelease",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-hcp",
			Namespace: "test-namespace",
		},
		Spec: addonsv1alpha1.VerrazzanoReleaseSpec{
			ReleaseName:      "test-release-name",
			ChartName:        "test-chart-name",
			RepoURL:          "https://test-repo-url",
			ReleaseNamespace: "test-release-namespace",
			Version:          "test-version",
			ValuesTemplate:   "apiServerPort: {{ .Cluster.invalid-path }}",
			Options:          &addonsv1alpha1.HelmOptions{},
		},
	}

	fakeReinstallHelmChartProxy = &addonsv1alpha1.VerrazzanoRelease{
		TypeMeta: metav1.TypeMeta{
			APIVersion: addonsv1alpha1.GroupVersion.String(),
			Kind:       "VerrazzanoRelease",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-hcp",
			Namespace: "test-namespace",
		},
		Spec: addonsv1alpha1.VerrazzanoReleaseSpec{
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

	fakeVerrazzanoReleaseBinding = &addonsv1alpha1.VerrazzanoReleaseBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-generated-name",
			Namespace: "test-namespace",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         addonsv1alpha1.GroupVersion.String(),
					Kind:               "VerrazzanoRelease",
					Name:               "test-hcp",
					Controller:         pointer.Bool(true),
					BlockOwnerDeletion: pointer.Bool(true),
				},
			},
			Labels: map[string]string{
				clusterv1.ClusterNameLabel:                "test-cluster",
				addonsv1alpha1.VerrazzanoReleaseLabelName: "test-hcp",
			},
		},
		Spec: addonsv1alpha1.VerrazzanoReleaseBindingSpec{
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
		name                                  string
		verrazzanoRelease                     *addonsv1alpha1.VerrazzanoRelease
		existingVerrazzanoReleaseBinding      *addonsv1alpha1.VerrazzanoReleaseBinding
		cluster                               *clusterv1.Cluster
		expect                                func(g *WithT, hcp *addonsv1alpha1.VerrazzanoRelease, hrp *addonsv1alpha1.VerrazzanoReleaseBinding)
		expectVerrazzanoReleaseBindingToExist bool
		expectedError                         string
	}{
		{
			name:                                  "creates a VerrazzanoReleaseBinding for a VerrazzanoRelease",
			verrazzanoRelease:                     fakeHelmChartProxy1,
			cluster:                               fakeCluster1,
			expectVerrazzanoReleaseBindingToExist: true,
			expect: func(g *WithT, hcp *addonsv1alpha1.VerrazzanoRelease, hrp *addonsv1alpha1.VerrazzanoReleaseBinding) {
				g.Expect(hrp.Spec.Values).To(Equal("apiServerPort: 6443"))

			},
			expectedError: "",
		},
		{
			name:                                  "updates a VerrazzanoReleaseBinding when Cluster value changes",
			verrazzanoRelease:                     fakeHelmChartProxy1,
			existingVerrazzanoReleaseBinding:      fakeVerrazzanoReleaseBinding,
			cluster:                               fakeCluster2,
			expectVerrazzanoReleaseBindingToExist: true,
			expect: func(g *WithT, hcp *addonsv1alpha1.VerrazzanoRelease, hrp *addonsv1alpha1.VerrazzanoReleaseBinding) {
				g.Expect(hrp.Spec.Values).To(Equal("apiServerPort: 1234"))

			},
			expectedError: "",
		},
		{
			name:                                  "updates a VerrazzanoReleaseBinding when valuesTemplate value changes",
			verrazzanoRelease:                     fakeHelmChartProxy2,
			existingVerrazzanoReleaseBinding:      fakeVerrazzanoReleaseBinding,
			cluster:                               fakeCluster2,
			expectVerrazzanoReleaseBindingToExist: true,
			expect: func(g *WithT, hcp *addonsv1alpha1.VerrazzanoRelease, hrp *addonsv1alpha1.VerrazzanoReleaseBinding) {
				g.Expect(hrp.Spec.Values).To(Equal("cidrBlockList: 10.0.0.0/16,20.0.0.0/16"))

			},
			expectedError: "",
		},
		{
			name:                                  "set condition when failing to parse values for a VerrazzanoRelease",
			verrazzanoRelease:                     fakeInvalidHelmChartProxy,
			cluster:                               fakeCluster1,
			expectVerrazzanoReleaseBindingToExist: true,
			expect: func(g *WithT, hcp *addonsv1alpha1.VerrazzanoRelease, hrp *addonsv1alpha1.VerrazzanoReleaseBinding) {
				g.Expect(conditions.Has(hcp, addonsv1alpha1.VerrazzanoReleaseBindingSpecsUpToDateCondition)).To(BeTrue())
				specsReady := conditions.Get(hcp, addonsv1alpha1.VerrazzanoReleaseBindingSpecsUpToDateCondition)
				g.Expect(specsReady.Status).To(Equal(corev1.ConditionFalse))
				g.Expect(specsReady.Reason).To(Equal(addonsv1alpha1.ValueParsingFailedReason))
				g.Expect(specsReady.Severity).To(Equal(clusterv1.ConditionSeverityError))
				g.Expect(specsReady.Message).To(Equal("failed to parse values on cluster test-cluster: template: test-chart-name-test-cluster:1: bad character U+002D '-'"))
			},
			expectedError: "failed to parse values on cluster test-cluster: template: test-chart-name-test-cluster:1: bad character U+002D '-'",
		},
		{
			name:                                  "set condition for reinstalling when requeueing after a deletion",
			verrazzanoRelease:                     fakeReinstallHelmChartProxy,
			existingVerrazzanoReleaseBinding:      fakeVerrazzanoReleaseBinding,
			cluster:                               fakeCluster1,
			expectVerrazzanoReleaseBindingToExist: false,
			expect: func(g *WithT, hcp *addonsv1alpha1.VerrazzanoRelease, hrp *addonsv1alpha1.VerrazzanoReleaseBinding) {
				g.Expect(conditions.Has(hcp, addonsv1alpha1.VerrazzanoReleaseBindingSpecsUpToDateCondition)).To(BeTrue())
				specsReady := conditions.Get(hcp, addonsv1alpha1.VerrazzanoReleaseBindingSpecsUpToDateCondition)
				g.Expect(specsReady.Status).To(Equal(corev1.ConditionFalse))
				g.Expect(specsReady.Reason).To(Equal(addonsv1alpha1.VerrazzanoReleaseBindingReinstallingReason))
				g.Expect(specsReady.Severity).To(Equal(clusterv1.ConditionSeverityInfo))
				g.Expect(specsReady.Message).To(Equal(fmt.Sprintf("VerrazzanoReleaseBinding on cluster '%s' successfully deleted, preparing to reinstall", fakeCluster1.Name)))
			},
			expectedError: "",
		},
	}

	for _, tc := range testcases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			t.Parallel()

			objects := []client.Object{tc.verrazzanoRelease, tc.cluster}
			if tc.existingVerrazzanoReleaseBinding != nil {
				objects = append(objects, tc.existingVerrazzanoReleaseBinding)
			}
			r := &VerrazzanoReleaseReconciler{
				Client: fake.NewClientBuilder().
					WithScheme(fakeScheme).
					WithObjects(objects...).
					WithStatusSubresource(&addonsv1alpha1.VerrazzanoRelease{}).
					WithStatusSubresource(&addonsv1alpha1.VerrazzanoReleaseBinding{}).
					Build(),
			}
			err := r.reconcileForCluster(ctx, tc.verrazzanoRelease, *tc.cluster)

			if tc.expectedError != "" {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err).To(MatchError(tc.expectedError), err.Error())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
				var hrp *addonsv1alpha1.VerrazzanoReleaseBinding
				var err error
				if tc.expectVerrazzanoReleaseBindingToExist {
					hrp, err = r.getExistingVerrazzanoReleaseBinding(ctx, tc.verrazzanoRelease, tc.cluster)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(hrp).NotTo(BeNil())
				}
				tc.expect(g, tc.verrazzanoRelease, hrp)
			}
		})
	}
}

func TestConstructVerrazzanoReleaseBinding(t *testing.T) {
	testCases := []struct {
		name              string
		existing          *addonsv1alpha1.VerrazzanoReleaseBinding
		verrazzanoRelease *addonsv1alpha1.VerrazzanoRelease
		parsedValues      string
		cluster           *clusterv1.Cluster
		expected          *addonsv1alpha1.VerrazzanoReleaseBinding
	}{
		{
			name: "existing up to date, nothing to do",
			existing: &addonsv1alpha1.VerrazzanoReleaseBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-generated-name",
					Namespace: "test-namespace",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         addonsv1alpha1.GroupVersion.String(),
							Kind:               "VerrazzanoRelease",
							Name:               "test-hcp",
							Controller:         pointer.Bool(true),
							BlockOwnerDeletion: pointer.Bool(true),
						},
					},
					Labels: map[string]string{
						clusterv1.ClusterNameLabel:                "test-cluster",
						addonsv1alpha1.VerrazzanoReleaseLabelName: "test-hcp",
					},
				},
				Spec: addonsv1alpha1.VerrazzanoReleaseBindingSpec{
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
			verrazzanoRelease: &addonsv1alpha1.VerrazzanoRelease{
				TypeMeta: metav1.TypeMeta{
					APIVersion: addonsv1alpha1.GroupVersion.String(),
					Kind:       "VerrazzanoRelease",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hcp",
					Namespace: "test-namespace",
				},
				Spec: addonsv1alpha1.VerrazzanoReleaseSpec{
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
			verrazzanoRelease: &addonsv1alpha1.VerrazzanoRelease{
				TypeMeta: metav1.TypeMeta{
					APIVersion: addonsv1alpha1.GroupVersion.String(),
					Kind:       "VerrazzanoRelease",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hcp",
					Namespace: "test-namespace",
				},
				Spec: addonsv1alpha1.VerrazzanoReleaseSpec{
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
			expected: &addonsv1alpha1.VerrazzanoReleaseBinding{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-chart-name-test-cluster-",
					Namespace:    "test-namespace",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         addonsv1alpha1.GroupVersion.String(),
							Kind:               "VerrazzanoRelease",
							Name:               "test-hcp",
							Controller:         pointer.Bool(true),
							BlockOwnerDeletion: pointer.Bool(true),
						},
					},
					Labels: map[string]string{
						clusterv1.ClusterNameLabel:                "test-cluster",
						addonsv1alpha1.VerrazzanoReleaseLabelName: "test-hcp",
					},
				},
				Spec: addonsv1alpha1.VerrazzanoReleaseBindingSpec{
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
			existing: &addonsv1alpha1.VerrazzanoReleaseBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-generated-name",
					Namespace: "test-namespace",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         addonsv1alpha1.GroupVersion.String(),
							Kind:               "VerrazzanoRelease",
							Name:               "test-hcp",
							Controller:         pointer.Bool(true),
							BlockOwnerDeletion: pointer.Bool(true),
						},
					},
					Labels: map[string]string{
						clusterv1.ClusterNameLabel:                "test-cluster",
						addonsv1alpha1.VerrazzanoReleaseLabelName: "test-hcp",
					},
				},
				Spec: addonsv1alpha1.VerrazzanoReleaseBindingSpec{
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
			verrazzanoRelease: &addonsv1alpha1.VerrazzanoRelease{
				TypeMeta: metav1.TypeMeta{
					APIVersion: addonsv1alpha1.GroupVersion.String(),
					Kind:       "VerrazzanoRelease",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hcp",
					Namespace: "test-namespace",
				},
				Spec: addonsv1alpha1.VerrazzanoReleaseSpec{
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
			expected: &addonsv1alpha1.VerrazzanoReleaseBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-generated-name",
					Namespace: "test-namespace",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         addonsv1alpha1.GroupVersion.String(),
							Kind:               "VerrazzanoRelease",
							Name:               "test-hcp",
							Controller:         pointer.Bool(true),
							BlockOwnerDeletion: pointer.Bool(true),
						},
					},
					Labels: map[string]string{
						clusterv1.ClusterNameLabel:                "test-cluster",
						addonsv1alpha1.VerrazzanoReleaseLabelName: "test-hcp",
					},
				},
				Spec: addonsv1alpha1.VerrazzanoReleaseBindingSpec{
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
			existing: &addonsv1alpha1.VerrazzanoReleaseBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-generated-name",
					Namespace: "test-namespace",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         addonsv1alpha1.GroupVersion.String(),
							Kind:               "VerrazzanoRelease",
							Name:               "test-hcp",
							Controller:         pointer.Bool(true),
							BlockOwnerDeletion: pointer.Bool(true),
						},
					},
					Labels: map[string]string{
						clusterv1.ClusterNameLabel:                "test-cluster",
						addonsv1alpha1.VerrazzanoReleaseLabelName: "test-hcp",
					},
				},
				Spec: addonsv1alpha1.VerrazzanoReleaseBindingSpec{
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
			verrazzanoRelease: &addonsv1alpha1.VerrazzanoRelease{
				TypeMeta: metav1.TypeMeta{
					APIVersion: addonsv1alpha1.GroupVersion.String(),
					Kind:       "VerrazzanoRelease",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hcp",
					Namespace: "test-namespace",
				},
				Spec: addonsv1alpha1.VerrazzanoReleaseSpec{
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
			expected: &addonsv1alpha1.VerrazzanoReleaseBinding{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-generated-name",
					Namespace: "test-namespace",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion:         addonsv1alpha1.GroupVersion.String(),
							Kind:               "VerrazzanoRelease",
							Name:               "test-hcp",
							Controller:         pointer.Bool(true),
							BlockOwnerDeletion: pointer.Bool(true),
						},
					},
					Labels: map[string]string{
						clusterv1.ClusterNameLabel:                "test-cluster",
						addonsv1alpha1.VerrazzanoReleaseLabelName: "test-hcp",
					},
				},
				Spec: addonsv1alpha1.VerrazzanoReleaseBindingSpec{
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

			result := constructVerrazzanoReleaseBinding(tc.existing, tc.verrazzanoRelease, tc.parsedValues, tc.cluster)
			diff := cmp.Diff(tc.expected, result)
			g.Expect(diff).To(BeEmpty())
		})
	}
}

func TestShouldReinstallHelmRelease(t *testing.T) {
	testCases := []struct {
		name                     string
		verrazzanoReleaseBinding *addonsv1alpha1.VerrazzanoReleaseBinding
		verrazzanoRelease        *addonsv1alpha1.VerrazzanoRelease
		reinstall                bool
	}{
		{
			name: "nothing to do",
			verrazzanoReleaseBinding: &addonsv1alpha1.VerrazzanoReleaseBinding{
				Spec: addonsv1alpha1.VerrazzanoReleaseBindingSpec{
					ChartName:        "test-chart",
					RepoURL:          "https://test-repo-url",
					ReleaseName:      "test-release-name",
					ReleaseNamespace: "test-namespace",
				},
			},
			verrazzanoRelease: &addonsv1alpha1.VerrazzanoRelease{
				Spec: addonsv1alpha1.VerrazzanoReleaseSpec{
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
			verrazzanoReleaseBinding: &addonsv1alpha1.VerrazzanoReleaseBinding{
				Spec: addonsv1alpha1.VerrazzanoReleaseBindingSpec{
					ChartName:        "test-chart",
					RepoURL:          "https://test-repo-url",
					ReleaseName:      "test-release-name",
					ReleaseNamespace: "test-namespace",
				},
			},
			verrazzanoRelease: &addonsv1alpha1.VerrazzanoRelease{
				Spec: addonsv1alpha1.VerrazzanoReleaseSpec{
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
			verrazzanoReleaseBinding: &addonsv1alpha1.VerrazzanoReleaseBinding{
				Spec: addonsv1alpha1.VerrazzanoReleaseBindingSpec{
					ChartName:        "test-chart",
					RepoURL:          "https://test-repo-url",
					ReleaseName:      "test-release-name",
					ReleaseNamespace: "test-namespace",
				},
			},
			verrazzanoRelease: &addonsv1alpha1.VerrazzanoRelease{
				Spec: addonsv1alpha1.VerrazzanoReleaseSpec{
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
			verrazzanoReleaseBinding: &addonsv1alpha1.VerrazzanoReleaseBinding{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						addonsv1alpha1.IsReleaseNameGeneratedAnnotation: "true",
					},
				},
				Spec: addonsv1alpha1.VerrazzanoReleaseBindingSpec{
					ChartName:   "test-chart",
					RepoURL:     "https://test-repo-url",
					ReleaseName: "generated-release-name",
				},
			},
			verrazzanoRelease: &addonsv1alpha1.VerrazzanoRelease{
				Spec: addonsv1alpha1.VerrazzanoReleaseSpec{
					ChartName:   "test-chart",
					RepoURL:     "https://test-repo-url",
					ReleaseName: "some-other-release-name",
				},
			},
			reinstall: true,
		},
		{
			name: "generated release name unchanged, nothing to do",
			verrazzanoReleaseBinding: &addonsv1alpha1.VerrazzanoReleaseBinding{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						addonsv1alpha1.IsReleaseNameGeneratedAnnotation: "true",
					},
				},
				Spec: addonsv1alpha1.VerrazzanoReleaseBindingSpec{
					ChartName:   "test-chart",
					RepoURL:     "https://test-repo-url",
					ReleaseName: "generated-release-name",
				},
			},
			verrazzanoRelease: &addonsv1alpha1.VerrazzanoRelease{
				Spec: addonsv1alpha1.VerrazzanoReleaseSpec{
					ChartName:   "test-chart",
					RepoURL:     "https://test-repo-url",
					ReleaseName: "",
				},
			},
			reinstall: false,
		},
		{
			name: "non-generated release name changed, should reinstall",
			verrazzanoReleaseBinding: &addonsv1alpha1.VerrazzanoReleaseBinding{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						addonsv1alpha1.IsReleaseNameGeneratedAnnotation: "true",
					},
				},
				Spec: addonsv1alpha1.VerrazzanoReleaseBindingSpec{
					ChartName:   "test-chart",
					RepoURL:     "https://test-repo-url",
					ReleaseName: "test-release-name",
				},
			},
			verrazzanoRelease: &addonsv1alpha1.VerrazzanoRelease{
				Spec: addonsv1alpha1.VerrazzanoReleaseSpec{
					ChartName:   "test-chart",
					RepoURL:     "https://test-repo-url",
					ReleaseName: "some-other-release-name",
				},
			},
			reinstall: true,
		},
		{
			name: "release namespace changed, should reinstall",
			verrazzanoReleaseBinding: &addonsv1alpha1.VerrazzanoReleaseBinding{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						addonsv1alpha1.IsReleaseNameGeneratedAnnotation: "true",
					},
				},
				Spec: addonsv1alpha1.VerrazzanoReleaseBindingSpec{
					ChartName:        "test-chart",
					RepoURL:          "https://test-repo-url",
					ReleaseName:      "test-release-name",
					ReleaseNamespace: "test-namespace",
				},
			},
			verrazzanoRelease: &addonsv1alpha1.VerrazzanoRelease{
				Spec: addonsv1alpha1.VerrazzanoReleaseSpec{
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

			result := shouldReinstallHelmRelease(ctx, tc.verrazzanoReleaseBinding, tc.verrazzanoRelease)
			g.Expect(result).To(Equal(tc.reinstall))
		})
	}
}

func TestGetOrphanedVerrazzanoReleaseBindings(t *testing.T) {
	testCases := []struct {
		name                      string
		selectedClusters          []clusterv1.Cluster
		verrazzanoReleaseBindings []addonsv1alpha1.VerrazzanoReleaseBinding
		releasesToDelete          []addonsv1alpha1.VerrazzanoReleaseBinding
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
			verrazzanoReleaseBindings: []addonsv1alpha1.VerrazzanoReleaseBinding{
				{
					Spec: addonsv1alpha1.VerrazzanoReleaseBindingSpec{
						ClusterRef: corev1.ObjectReference{
							Name:      "test-cluster-1",
							Namespace: "test-namespace-1",
						},
					},
				},
				{
					Spec: addonsv1alpha1.VerrazzanoReleaseBindingSpec{
						ClusterRef: corev1.ObjectReference{
							Name:      "test-cluster-2",
							Namespace: "test-namespace-2",
						},
					},
				},
			},
			releasesToDelete: []addonsv1alpha1.VerrazzanoReleaseBinding{},
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
			verrazzanoReleaseBindings: []addonsv1alpha1.VerrazzanoReleaseBinding{
				{
					Spec: addonsv1alpha1.VerrazzanoReleaseBindingSpec{
						ClusterRef: corev1.ObjectReference{
							Name:      "test-cluster-1",
							Namespace: "test-namespace-1",
						},
					},
				},
				{
					Spec: addonsv1alpha1.VerrazzanoReleaseBindingSpec{
						ClusterRef: corev1.ObjectReference{
							Name:      "test-cluster-2",
							Namespace: "test-namespace-2",
						},
					},
				},
			},
			releasesToDelete: []addonsv1alpha1.VerrazzanoReleaseBinding{
				{
					Spec: addonsv1alpha1.VerrazzanoReleaseBindingSpec{
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
			verrazzanoReleaseBindings: []addonsv1alpha1.VerrazzanoReleaseBinding{
				{
					Spec: addonsv1alpha1.VerrazzanoReleaseBindingSpec{
						ClusterRef: corev1.ObjectReference{
							Name:      "test-cluster-1",
							Namespace: "test-namespace-1",
						},
					},
				},
				{
					Spec: addonsv1alpha1.VerrazzanoReleaseBindingSpec{
						ClusterRef: corev1.ObjectReference{
							Name:      "test-cluster-2",
							Namespace: "test-namespace-2",
						},
					},
				},
			},
			releasesToDelete: []addonsv1alpha1.VerrazzanoReleaseBinding{
				{
					Spec: addonsv1alpha1.VerrazzanoReleaseBindingSpec{
						ClusterRef: corev1.ObjectReference{
							Name:      "test-cluster-1",
							Namespace: "test-namespace-1",
						},
					},
				},
				{
					Spec: addonsv1alpha1.VerrazzanoReleaseBindingSpec{
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

			result := getOrphanedVerrazzanoReleaseBindings(ctx, tc.selectedClusters, tc.verrazzanoReleaseBindings)
			g.Expect(result).To(Equal(tc.releasesToDelete))
		})
	}
}
