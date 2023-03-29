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

package controllers

import (
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	bootstrapv1 "github.com/verrazzano/cluster-api-provider-ocne/bootstrap/ocne/api/v1beta1"
	controlplanev1 "github.com/verrazzano/cluster-api-provider-ocne/controlplane/ocne/api/v1beta1"
	"github.com/verrazzano/cluster-api-provider-ocne/controlplane/ocne/internal"
	"github.com/verrazzano/cluster-api-provider-ocne/internal/test/builder"
	"github.com/verrazzano/cluster-api-provider-ocne/util/conditions"
	"github.com/verrazzano/cluster-api-provider-ocne/util/kubeconfig"
	"github.com/verrazzano/cluster-api-provider-ocne/util/secret"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
)

func TestReconcileKubeconfigEmptyAPIEndpoints(t *testing.T) {
	g := NewWithT(t)

	cluster := &clusterv1.Cluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Cluster",
			APIVersion: clusterv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: clusterv1.ClusterSpec{
			ControlPlaneEndpoint: clusterv1.APIEndpoint{},
		},
	}

	ocnecp := &controlplanev1.OCNEControlPlane{
		TypeMeta: metav1.TypeMeta{
			Kind:       "OCNEControlPlane",
			APIVersion: controlplanev1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: controlplanev1.OcneControlPlaneSpec{
			Version: "v1.16.6",
		},
	}
	clusterName := client.ObjectKey{Namespace: metav1.NamespaceDefault, Name: "foo"}

	fakeClient := newFakeClient(ocnecp.DeepCopy())
	r := &OcneControlPlaneReconciler{
		Client:   fakeClient,
		recorder: record.NewFakeRecorder(32),
	}

	result, err := r.reconcileKubeconfig(ctx, cluster, ocnecp)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(BeZero())

	kubeconfigSecret := &corev1.Secret{}
	secretName := client.ObjectKey{
		Namespace: metav1.NamespaceDefault,
		Name:      secret.Name(clusterName.Name, secret.Kubeconfig),
	}
	g.Expect(r.Client.Get(ctx, secretName, kubeconfigSecret)).To(MatchError(ContainSubstring("not found")))
}

func TestReconcileKubeconfigMissingCACertificate(t *testing.T) {
	g := NewWithT(t)

	cluster := &clusterv1.Cluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Cluster",
			APIVersion: clusterv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: clusterv1.ClusterSpec{
			ControlPlaneEndpoint: clusterv1.APIEndpoint{Host: "test.local", Port: 8443},
		},
	}

	ocnecp := &controlplanev1.OCNEControlPlane{
		TypeMeta: metav1.TypeMeta{
			Kind:       "OCNEControlPlane",
			APIVersion: controlplanev1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: controlplanev1.OcneControlPlaneSpec{
			Version: "v1.16.6",
		},
	}

	fakeClient := newFakeClient(ocnecp.DeepCopy())
	r := &OcneControlPlaneReconciler{
		Client:   fakeClient,
		recorder: record.NewFakeRecorder(32),
	}

	result, err := r.reconcileKubeconfig(ctx, cluster, ocnecp)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(Equal(ctrl.Result{RequeueAfter: dependentCertRequeueAfter}))

	kubeconfigSecret := &corev1.Secret{}
	secretName := client.ObjectKey{
		Namespace: metav1.NamespaceDefault,
		Name:      secret.Name(cluster.Name, secret.Kubeconfig),
	}
	g.Expect(r.Client.Get(ctx, secretName, kubeconfigSecret)).To(MatchError(ContainSubstring("not found")))
}

func TestReconcileKubeconfigSecretAdoptsV1alpha2Secrets(t *testing.T) {
	g := NewWithT(t)

	cluster := &clusterv1.Cluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Cluster",
			APIVersion: clusterv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: clusterv1.ClusterSpec{
			ControlPlaneEndpoint: clusterv1.APIEndpoint{Host: "test.local", Port: 8443},
		},
	}

	ocnecp := &controlplanev1.OCNEControlPlane{
		TypeMeta: metav1.TypeMeta{
			Kind:       "OCNEControlPlane",
			APIVersion: controlplanev1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: controlplanev1.OcneControlPlaneSpec{
			Version: "v1.16.6",
		},
	}

	existingKubeconfigSecret := kubeconfig.GenerateSecretWithOwner(
		client.ObjectKey{Name: "foo", Namespace: metav1.NamespaceDefault},
		[]byte{},
		metav1.OwnerReference{
			APIVersion: clusterv1.GroupVersion.String(),
			Kind:       "Cluster",
			Name:       cluster.Name,
			UID:        cluster.UID,
		}, // the Cluster ownership defines v1alpha2 controlled secrets
	)

	fakeClient := newFakeClient(ocnecp.DeepCopy(), existingKubeconfigSecret.DeepCopy())
	r := &OcneControlPlaneReconciler{
		Client:   fakeClient,
		recorder: record.NewFakeRecorder(32),
	}

	result, err := r.reconcileKubeconfig(ctx, cluster, ocnecp)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(Equal(ctrl.Result{}))

	kubeconfigSecret := &corev1.Secret{}
	secretName := client.ObjectKey{
		Namespace: metav1.NamespaceDefault,
		Name:      secret.Name(cluster.Name, secret.Kubeconfig),
	}
	g.Expect(r.Client.Get(ctx, secretName, kubeconfigSecret)).To(Succeed())
	g.Expect(kubeconfigSecret.Labels).To(Equal(existingKubeconfigSecret.Labels))
	g.Expect(kubeconfigSecret.Data).To(Equal(existingKubeconfigSecret.Data))
	g.Expect(kubeconfigSecret.OwnerReferences).ToNot(ContainElement(metav1.OwnerReference{
		APIVersion: clusterv1.GroupVersion.String(),
		Kind:       "Cluster",
		Name:       cluster.Name,
		UID:        cluster.UID,
	}))
	g.Expect(kubeconfigSecret.OwnerReferences).To(ContainElement(*metav1.NewControllerRef(ocnecp, controlplanev1.GroupVersion.WithKind("OCNEControlPlane"))))
}

func TestReconcileKubeconfigSecretDoesNotAdoptsUserSecrets(t *testing.T) {
	g := NewWithT(t)

	cluster := &clusterv1.Cluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Cluster",
			APIVersion: clusterv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: clusterv1.ClusterSpec{
			ControlPlaneEndpoint: clusterv1.APIEndpoint{Host: "test.local", Port: 8443},
		},
	}

	ocnecp := &controlplanev1.OCNEControlPlane{
		TypeMeta: metav1.TypeMeta{
			Kind:       "OCNEControlPlane",
			APIVersion: controlplanev1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: controlplanev1.OcneControlPlaneSpec{
			Version: "v1.16.6",
		},
	}

	existingKubeconfigSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secret.Name("foo", secret.Kubeconfig),
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				clusterv1.ClusterLabelName: "foo",
			},
			OwnerReferences: []metav1.OwnerReference{},
		},
		Data: map[string][]byte{
			secret.KubeconfigDataName: {},
		},
		// KCP identifies CAPI-created Secrets using the clusterv1.ClusterSecretType. Setting any other type allows
		// the controllers to treat it as a user-provided Secret.
		Type: corev1.SecretTypeOpaque,
	}

	fakeClient := newFakeClient(ocnecp.DeepCopy(), existingKubeconfigSecret.DeepCopy())
	r := &OcneControlPlaneReconciler{
		Client:   fakeClient,
		recorder: record.NewFakeRecorder(32),
	}

	result, err := r.reconcileKubeconfig(ctx, cluster, ocnecp)
	g.Expect(err).To(Succeed())
	g.Expect(result).To(BeZero())

	kubeconfigSecret := &corev1.Secret{}
	secretName := client.ObjectKey{
		Namespace: metav1.NamespaceDefault,
		Name:      secret.Name(cluster.Name, secret.Kubeconfig),
	}
	g.Expect(r.Client.Get(ctx, secretName, kubeconfigSecret)).To(Succeed())
	g.Expect(kubeconfigSecret.Labels).To(Equal(existingKubeconfigSecret.Labels))
	g.Expect(kubeconfigSecret.Data).To(Equal(existingKubeconfigSecret.Data))
	g.Expect(kubeconfigSecret.OwnerReferences).ToNot(ContainElement(*metav1.NewControllerRef(ocnecp, controlplanev1.GroupVersion.WithKind("OCNEControlPlane"))))
}

func TestOCNEControlPlaneReconciler_reconcileKubeconfig(t *testing.T) {
	g := NewWithT(t)

	cluster := &clusterv1.Cluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Cluster",
			APIVersion: clusterv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: clusterv1.ClusterSpec{
			ControlPlaneEndpoint: clusterv1.APIEndpoint{Host: "test.local", Port: 8443},
		},
	}

	ocnecp := &controlplanev1.OCNEControlPlane{
		TypeMeta: metav1.TypeMeta{
			Kind:       "OCNEControlPlane",
			APIVersion: controlplanev1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: controlplanev1.OcneControlPlaneSpec{
			Version: "v1.16.6",
		},
	}

	clusterCerts := secret.NewCertificatesForInitialControlPlane(&bootstrapv1.ClusterConfiguration{})
	g.Expect(clusterCerts.Generate()).To(Succeed())
	caCert := clusterCerts.GetByPurpose(secret.ClusterCA)
	existingCACertSecret := caCert.AsSecret(
		client.ObjectKey{Namespace: metav1.NamespaceDefault, Name: "foo"},
		*metav1.NewControllerRef(ocnecp, controlplanev1.GroupVersion.WithKind("OCNEControlPlane")),
	)

	fakeClient := newFakeClient(ocnecp.DeepCopy(), existingCACertSecret.DeepCopy())
	r := &OcneControlPlaneReconciler{
		Client:   fakeClient,
		recorder: record.NewFakeRecorder(32),
	}
	result, err := r.reconcileKubeconfig(ctx, cluster, ocnecp)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(result).To(Equal(ctrl.Result{}))

	kubeconfigSecret := &corev1.Secret{}
	secretName := client.ObjectKey{
		Namespace: metav1.NamespaceDefault,
		Name:      secret.Name(cluster.Name, secret.Kubeconfig),
	}
	g.Expect(r.Client.Get(ctx, secretName, kubeconfigSecret)).To(Succeed())
	g.Expect(kubeconfigSecret.OwnerReferences).NotTo(BeEmpty())
	g.Expect(kubeconfigSecret.OwnerReferences).To(ContainElement(*metav1.NewControllerRef(ocnecp, controlplanev1.GroupVersion.WithKind("OCNEControlPlane"))))
	g.Expect(kubeconfigSecret.Labels).To(HaveKeyWithValue(clusterv1.ClusterLabelName, cluster.Name))
}

func TestCloneConfigsAndGenerateMachine(t *testing.T) {
	g := NewWithT(t)

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: metav1.NamespaceDefault,
		},
	}

	genericMachineTemplate := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "GenericMachineTemplate",
			"apiVersion": "generic.io/v1",
			"metadata": map[string]interface{}{
				"name":      "infra-foo",
				"namespace": cluster.Namespace,
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"hello": "world",
					},
				},
			},
		},
	}

	ocnecp := &controlplanev1.OCNEControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ocnecp-foo",
			Namespace: cluster.Namespace,
		},
		Spec: controlplanev1.OcneControlPlaneSpec{
			MachineTemplate: controlplanev1.OcneControlPlaneMachineTemplate{
				InfrastructureRef: corev1.ObjectReference{
					Kind:       genericMachineTemplate.GetKind(),
					APIVersion: genericMachineTemplate.GetAPIVersion(),
					Name:       genericMachineTemplate.GetName(),
					Namespace:  cluster.Namespace,
				},
			},
			Version: "v1.16.6",
		},
	}

	fakeClient := newFakeClient(cluster.DeepCopy(), ocnecp.DeepCopy(), genericMachineTemplate.DeepCopy())

	r := &OcneControlPlaneReconciler{
		Client:   fakeClient,
		recorder: record.NewFakeRecorder(32),
	}

	bootstrapSpec := &bootstrapv1.OCNEConfigSpec{
		JoinConfiguration: &bootstrapv1.JoinConfiguration{},
	}
	g.Expect(r.cloneConfigsAndGenerateMachine(ctx, cluster, ocnecp, bootstrapSpec, nil)).To(Succeed())

	machineList := &clusterv1.MachineList{}
	g.Expect(fakeClient.List(ctx, machineList, client.InNamespace(cluster.Namespace))).To(Succeed())
	g.Expect(machineList.Items).To(HaveLen(1))

	for _, m := range machineList.Items {
		g.Expect(m.Namespace).To(Equal(cluster.Namespace))
		g.Expect(m.Name).NotTo(BeEmpty())
		g.Expect(m.Name).To(HavePrefix(ocnecp.Name))

		infraObj, err := external.Get(ctx, r.Client, &m.Spec.InfrastructureRef, m.Spec.InfrastructureRef.Namespace)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(infraObj.GetAnnotations()).To(HaveKeyWithValue(clusterv1.TemplateClonedFromNameAnnotation, genericMachineTemplate.GetName()))
		g.Expect(infraObj.GetAnnotations()).To(HaveKeyWithValue(clusterv1.TemplateClonedFromGroupKindAnnotation, genericMachineTemplate.GroupVersionKind().GroupKind().String()))

		g.Expect(m.Spec.InfrastructureRef.Namespace).To(Equal(cluster.Namespace))
		g.Expect(m.Spec.InfrastructureRef.Name).To(HavePrefix(genericMachineTemplate.GetName()))
		g.Expect(m.Spec.InfrastructureRef.APIVersion).To(Equal(genericMachineTemplate.GetAPIVersion()))
		g.Expect(m.Spec.InfrastructureRef.Kind).To(Equal("GenericMachine"))

		g.Expect(m.Spec.Bootstrap.ConfigRef.Namespace).To(Equal(cluster.Namespace))
		g.Expect(m.Spec.Bootstrap.ConfigRef.Name).To(HavePrefix(ocnecp.Name))
		g.Expect(m.Spec.Bootstrap.ConfigRef.APIVersion).To(Equal(bootstrapv1.GroupVersion.String()))
		g.Expect(m.Spec.Bootstrap.ConfigRef.Kind).To(Equal("OCNEConfig"))
	}
}

func TestCloneConfigsAndGenerateMachineFail(t *testing.T) {
	g := NewWithT(t)

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: metav1.NamespaceDefault,
		},
	}

	genericMachineTemplate := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "GenericMachineTemplate",
			"apiVersion": "generic.io/v1",
			"metadata": map[string]interface{}{
				"name":      "infra-foo",
				"namespace": cluster.Namespace,
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"hello": "world",
					},
				},
			},
		},
	}

	ocnecp := &controlplanev1.OCNEControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ocnecp-foo",
			Namespace: cluster.Namespace,
		},
		Spec: controlplanev1.OcneControlPlaneSpec{
			MachineTemplate: controlplanev1.OcneControlPlaneMachineTemplate{
				InfrastructureRef: corev1.ObjectReference{
					Kind:       genericMachineTemplate.GetKind(),
					APIVersion: genericMachineTemplate.GetAPIVersion(),
					Name:       genericMachineTemplate.GetName(),
					Namespace:  cluster.Namespace,
				},
			},
			Version: "v1.16.6",
		},
	}

	fakeClient := newFakeClient(cluster.DeepCopy(), ocnecp.DeepCopy(), genericMachineTemplate.DeepCopy())

	r := &OcneControlPlaneReconciler{
		Client:   fakeClient,
		recorder: record.NewFakeRecorder(32),
	}

	bootstrapSpec := &bootstrapv1.OCNEConfigSpec{
		JoinConfiguration: &bootstrapv1.JoinConfiguration{},
	}

	// Try to break Infra Cloning
	ocnecp.Spec.MachineTemplate.InfrastructureRef.Name = "something_invalid"
	g.Expect(r.cloneConfigsAndGenerateMachine(ctx, cluster, ocnecp, bootstrapSpec, nil)).To(HaveOccurred())
	g.Expect(&ocnecp.GetConditions()[0]).Should(conditions.HaveSameStateOf(&clusterv1.Condition{
		Type:     controlplanev1.MachinesCreatedCondition,
		Status:   corev1.ConditionFalse,
		Severity: clusterv1.ConditionSeverityError,
		Reason:   controlplanev1.InfrastructureTemplateCloningFailedReason,
		Message:  "failed to retrieve GenericMachineTemplate external object \"default\"/\"something_invalid\": genericmachinetemplates.generic.io \"something_invalid\" not found",
	}))
}

func TestOCNEControlPlaneReconciler_generateMachine(t *testing.T) {
	g := NewWithT(t)
	fakeClient := newFakeClient()

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testCluster",
			Namespace: metav1.NamespaceDefault,
		},
	}

	ocnecpMachineTemplateObjectMeta := clusterv1.ObjectMeta{
		Labels: map[string]string{
			"machineTemplateLabel": "machineTemplateLabelValue",
		},
		Annotations: map[string]string{
			"machineTemplateAnnotation": "machineTemplateAnnotationValue",
		},
	}
	ocnecp := &controlplanev1.OCNEControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testControlPlane",
			Namespace: cluster.Namespace,
		},
		Spec: controlplanev1.OcneControlPlaneSpec{
			Version: "v1.16.6",
			MachineTemplate: controlplanev1.OcneControlPlaneMachineTemplate{
				ObjectMeta: ocnecpMachineTemplateObjectMeta,
			},
		},
	}

	infraRef := &corev1.ObjectReference{
		Kind:       "InfraKind",
		APIVersion: "infrastructure.cluster.x-k8s.io/v1beta1",
		Name:       "infra",
		Namespace:  cluster.Namespace,
	}
	bootstrapRef := &corev1.ObjectReference{
		Kind:       "BootstrapKind",
		APIVersion: "bootstrap.cluster.x-k8s.io/v1beta1",
		Name:       "bootstrap",
		Namespace:  cluster.Namespace,
	}
	expectedMachineSpec := clusterv1.MachineSpec{
		ClusterName: cluster.Name,
		Version:     pointer.String(ocnecp.Spec.Version),
		Bootstrap: clusterv1.Bootstrap{
			ConfigRef: bootstrapRef.DeepCopy(),
		},
		InfrastructureRef: *infraRef.DeepCopy(),
	}
	r := &OcneControlPlaneReconciler{
		Client:            fakeClient,
		managementCluster: &internal.Management{Client: fakeClient},
		recorder:          record.NewFakeRecorder(32),
	}
	g.Expect(r.generateMachine(ctx, ocnecp, cluster, infraRef, bootstrapRef, nil)).To(Succeed())

	machineList := &clusterv1.MachineList{}
	g.Expect(fakeClient.List(ctx, machineList, client.InNamespace(cluster.Namespace))).To(Succeed())
	g.Expect(machineList.Items).To(HaveLen(1))
	machine := machineList.Items[0]
	g.Expect(machine.Name).To(HavePrefix(ocnecp.Name))
	g.Expect(machine.Namespace).To(Equal(ocnecp.Namespace))
	g.Expect(machine.OwnerReferences).To(HaveLen(1))
	g.Expect(machine.OwnerReferences).To(ContainElement(*metav1.NewControllerRef(ocnecp, controlplanev1.GroupVersion.WithKind("OCNEControlPlane"))))
	g.Expect(machine.Spec).To(Equal(expectedMachineSpec))

	// Verify that the machineTemplate.ObjectMeta has been propagated to the Machine.
	for k, v := range ocnecpMachineTemplateObjectMeta.Labels {
		g.Expect(machine.Labels[k]).To(Equal(v))
	}
	g.Expect(machine.Labels[clusterv1.ClusterLabelName]).To(Equal(cluster.Name))
	g.Expect(machine.Labels[clusterv1.MachineControlPlaneLabelName]).To(Equal(""))
	g.Expect(machine.Labels[clusterv1.MachineControlPlaneNameLabel]).To(Equal(ocnecp.Name))

	for k, v := range ocnecpMachineTemplateObjectMeta.Annotations {
		g.Expect(machine.Annotations[k]).To(Equal(v))
	}

	// Verify that machineTemplate.ObjectMeta in KCP has not been modified.
	g.Expect(ocnecp.Spec.MachineTemplate.ObjectMeta.Labels).NotTo(HaveKey(clusterv1.ClusterLabelName))
	g.Expect(ocnecp.Spec.MachineTemplate.ObjectMeta.Labels).NotTo(HaveKey(clusterv1.MachineControlPlaneLabelName))
	g.Expect(ocnecp.Spec.MachineTemplate.ObjectMeta.Labels).NotTo(HaveKey(clusterv1.MachineControlPlaneNameLabel))
	g.Expect(ocnecp.Spec.MachineTemplate.ObjectMeta.Annotations).NotTo(HaveKey(controlplanev1.OcneClusterConfigurationAnnotation))
}

func TestOCNEControlPlaneReconciler_generateKubeadmConfig(t *testing.T) {
	g := NewWithT(t)
	fakeClient := newFakeClient()

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testCluster",
			Namespace: metav1.NamespaceDefault,
		},
	}

	ocnecp := &controlplanev1.OCNEControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testControlPlane",
			Namespace: cluster.Namespace,
		},
	}

	spec := bootstrapv1.OCNEConfigSpec{}
	expectedReferenceKind := "OCNEConfig"
	expectedReferenceAPIVersion := bootstrapv1.GroupVersion.String()
	expectedOwner := metav1.OwnerReference{
		Kind:       "OCNEControlPlane",
		APIVersion: controlplanev1.GroupVersion.String(),
		Name:       ocnecp.Name,
	}

	r := &OcneControlPlaneReconciler{
		Client:   fakeClient,
		recorder: record.NewFakeRecorder(32),
	}

	got, err := r.generateKubeadmConfig(ctx, ocnecp, cluster, spec.DeepCopy())
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(got).NotTo(BeNil())
	g.Expect(got.Name).To(HavePrefix(ocnecp.Name))
	g.Expect(got.Namespace).To(Equal(ocnecp.Namespace))
	g.Expect(got.Kind).To(Equal(expectedReferenceKind))
	g.Expect(got.APIVersion).To(Equal(expectedReferenceAPIVersion))

	bootstrapConfig := &bootstrapv1.OCNEConfig{}
	key := client.ObjectKey{Name: got.Name, Namespace: got.Namespace}
	g.Expect(fakeClient.Get(ctx, key, bootstrapConfig)).To(Succeed())
	g.Expect(bootstrapConfig.OwnerReferences).To(HaveLen(1))
	g.Expect(bootstrapConfig.OwnerReferences).To(ContainElement(expectedOwner))
	g.Expect(bootstrapConfig.Spec).To(Equal(spec))
}

func TestOCNEControlPlaneReconciler_adoptKubeconfigSecret(t *testing.T) {
	g := NewWithT(t)
	otherOwner := metav1.OwnerReference{
		Name:               "testcontroller",
		UID:                "5",
		Kind:               "OtherController",
		APIVersion:         clusterv1.GroupVersion.String(),
		Controller:         pointer.Bool(true),
		BlockOwnerDeletion: pointer.Bool(true),
	}
	clusterName := "test1"
	cluster := builder.Cluster(metav1.NamespaceDefault, clusterName).Build()

	// A OCNEConfig secret created by CAPI controllers with no owner references.
	capiKubeadmConfigSecretNoOwner := kubeconfig.GenerateSecretWithOwner(
		client.ObjectKey{Name: clusterName, Namespace: metav1.NamespaceDefault},
		[]byte{},
		metav1.OwnerReference{})
	capiKubeadmConfigSecretNoOwner.OwnerReferences = []metav1.OwnerReference{}

	// A OCNEConfig secret created by CAPI controllers with a non-KCP owner reference.
	capiKubeadmConfigSecretOtherOwner := capiKubeadmConfigSecretNoOwner.DeepCopy()
	capiKubeadmConfigSecretOtherOwner.OwnerReferences = []metav1.OwnerReference{otherOwner}

	// A user provided OCNEConfig secret with no owner reference.
	userProvidedKubeadmConfigSecretNoOwner := kubeconfig.GenerateSecretWithOwner(
		client.ObjectKey{Name: clusterName, Namespace: metav1.NamespaceDefault},
		[]byte{},
		metav1.OwnerReference{})
	userProvidedKubeadmConfigSecretNoOwner.Type = corev1.SecretTypeOpaque

	// A user provided OCNEConfig with a non-KCP owner reference.
	userProvidedKubeadmConfigSecretOtherOwner := userProvidedKubeadmConfigSecretNoOwner.DeepCopy()
	userProvidedKubeadmConfigSecretOtherOwner.OwnerReferences = []metav1.OwnerReference{otherOwner}

	ocnecp := &controlplanev1.OCNEControlPlane{
		TypeMeta: metav1.TypeMeta{
			Kind:       "OCNEControlPlane",
			APIVersion: controlplanev1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "testControlPlane",
			Namespace: cluster.Namespace,
		},
	}
	tests := []struct {
		name             string
		configSecret     *corev1.Secret
		expectedOwnerRef metav1.OwnerReference
	}{
		{
			name:         "add KCP owner reference on kubeconfig secret generated by CAPI",
			configSecret: capiKubeadmConfigSecretNoOwner,
			expectedOwnerRef: metav1.OwnerReference{
				Name:               ocnecp.Name,
				UID:                ocnecp.UID,
				Kind:               ocnecp.Kind,
				APIVersion:         ocnecp.APIVersion,
				Controller:         pointer.Bool(true),
				BlockOwnerDeletion: pointer.Bool(true),
			},
		},
		{
			name:         "replace owner reference with KCP on kubeconfig secret generated by CAPI with other owner",
			configSecret: capiKubeadmConfigSecretOtherOwner,
			expectedOwnerRef: metav1.OwnerReference{
				Name:               ocnecp.Name,
				UID:                ocnecp.UID,
				Kind:               ocnecp.Kind,
				APIVersion:         ocnecp.APIVersion,
				Controller:         pointer.Bool(true),
				BlockOwnerDeletion: pointer.Bool(true),
			},
		},
		{
			name:             "don't add ownerReference on kubeconfig secret provided by user",
			configSecret:     userProvidedKubeadmConfigSecretNoOwner,
			expectedOwnerRef: metav1.OwnerReference{},
		},
		{
			name:             "don't replace ownerReference on kubeconfig secret provided by user",
			configSecret:     userProvidedKubeadmConfigSecretOtherOwner,
			expectedOwnerRef: otherOwner,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := newFakeClient(cluster, ocnecp, tt.configSecret)
			r := &OcneControlPlaneReconciler{
				APIReader: fakeClient,
				Client:    fakeClient,
			}
			g.Expect(r.adoptKubeconfigSecret(ctx, cluster, tt.configSecret, ocnecp)).To(Succeed())
			actualSecret := &corev1.Secret{}
			g.Expect(fakeClient.Get(ctx, client.ObjectKey{Namespace: tt.configSecret.Namespace, Name: tt.configSecret.Namespace}, actualSecret))
			g.Expect(tt.configSecret.GetOwnerReferences()).To(ConsistOf(tt.expectedOwnerRef))
		})
	}
}
