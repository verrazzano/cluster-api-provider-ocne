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
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2/klogr"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	controlplanev1 "github.com/verrazzano/cluster-api-provider-ocne/controlplane/ocne/api/v1alpha1"
	"github.com/verrazzano/cluster-api-provider-ocne/controlplane/ocne/internal"
	"github.com/verrazzano/cluster-api-provider-ocne/util/conditions"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

func TestOCNEControlPlaneReconciler_updateStatusNoMachines(t *testing.T) {
	g := NewWithT(t)

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: metav1.NamespaceDefault,
		},
	}

	ocnecp := &controlplanev1.OCNEControlPlane{
		TypeMeta: metav1.TypeMeta{
			Kind:       "OCNEControlPlane",
			APIVersion: controlplanev1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cluster.Namespace,
			Name:      "foo",
		},
		Spec: controlplanev1.OCNEControlPlaneSpec{
			Version: "v1.16.6",
			MachineTemplate: controlplanev1.OCNEControlPlaneMachineTemplate{
				InfrastructureRef: corev1.ObjectReference{
					APIVersion: "test/v1alpha1",
					Kind:       "UnknownInfraMachine",
					Name:       "foo",
				},
			},
		},
	}
	ocnecp.Default()
	g.Expect(ocnecp.ValidateCreate()).To(Succeed())

	fakeClient := newFakeClient(ocnecp.DeepCopy(), cluster.DeepCopy())
	log.SetLogger(klogr.New())

	r := &OCNEControlPlaneReconciler{
		Client: fakeClient,
		managementCluster: &fakeManagementCluster{
			Machines: map[string]*clusterv1.Machine{},
			Workload: fakeWorkloadCluster{},
		},
		recorder: record.NewFakeRecorder(32),
	}

	g.Expect(r.updateStatus(ctx, ocnecp, cluster)).To(Succeed())
	g.Expect(ocnecp.Status.Replicas).To(BeEquivalentTo(0))
	g.Expect(ocnecp.Status.ReadyReplicas).To(BeEquivalentTo(0))
	g.Expect(ocnecp.Status.UnavailableReplicas).To(BeEquivalentTo(0))
	g.Expect(ocnecp.Status.Initialized).To(BeFalse())
	g.Expect(ocnecp.Status.Ready).To(BeFalse())
	g.Expect(ocnecp.Status.Selector).NotTo(BeEmpty())
	g.Expect(ocnecp.Status.FailureMessage).To(BeNil())
	g.Expect(ocnecp.Status.FailureReason).To(BeEquivalentTo(""))
}

func TestOCNEControlPlaneReconciler_updateStatusAllMachinesNotReady(t *testing.T) {
	g := NewWithT(t)

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: metav1.NamespaceDefault,
		},
	}

	ocnecp := &controlplanev1.OCNEControlPlane{
		TypeMeta: metav1.TypeMeta{
			Kind:       "OCNEControlPlane",
			APIVersion: controlplanev1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cluster.Namespace,
			Name:      "foo",
		},
		Spec: controlplanev1.OCNEControlPlaneSpec{
			Version: "v1.16.6",
			MachineTemplate: controlplanev1.OCNEControlPlaneMachineTemplate{
				InfrastructureRef: corev1.ObjectReference{
					APIVersion: "test/v1alpha1",
					Kind:       "UnknownInfraMachine",
					Name:       "foo",
				},
			},
		},
	}
	ocnecp.Default()
	g.Expect(ocnecp.ValidateCreate()).To(Succeed())

	machines := map[string]*clusterv1.Machine{}
	objs := []client.Object{cluster.DeepCopy(), ocnecp.DeepCopy()}
	for i := 0; i < 3; i++ {
		name := fmt.Sprintf("test-%d", i)
		m, n := createMachineNodePair(name, cluster, ocnecp, false)
		objs = append(objs, n, m)
		machines[m.Name] = m
	}

	fakeClient := newFakeClient(objs...)
	log.SetLogger(klogr.New())

	r := &OCNEControlPlaneReconciler{
		Client: fakeClient,
		managementCluster: &fakeManagementCluster{
			Machines: machines,
			Workload: fakeWorkloadCluster{},
		},
		recorder: record.NewFakeRecorder(32),
	}

	g.Expect(r.updateStatus(ctx, ocnecp, cluster)).To(Succeed())
	g.Expect(ocnecp.Status.Replicas).To(BeEquivalentTo(3))
	g.Expect(ocnecp.Status.ReadyReplicas).To(BeEquivalentTo(0))
	g.Expect(ocnecp.Status.UnavailableReplicas).To(BeEquivalentTo(3))
	g.Expect(ocnecp.Status.Selector).NotTo(BeEmpty())
	g.Expect(ocnecp.Status.FailureMessage).To(BeNil())
	g.Expect(ocnecp.Status.FailureReason).To(BeEquivalentTo(""))
	g.Expect(ocnecp.Status.Initialized).To(BeFalse())
	g.Expect(ocnecp.Status.Ready).To(BeFalse())
}

func TestOCNEControlPlaneReconciler_updateStatusAllMachinesReady(t *testing.T) {
	g := NewWithT(t)

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: metav1.NamespaceDefault,
			Name:      "foo",
		},
	}

	ocnecp := &controlplanev1.OCNEControlPlane{
		TypeMeta: metav1.TypeMeta{
			Kind:       "OCNEControlPlane",
			APIVersion: controlplanev1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cluster.Namespace,
			Name:      "foo",
		},
		Spec: controlplanev1.OCNEControlPlaneSpec{
			Version: "v1.16.6",
			MachineTemplate: controlplanev1.OCNEControlPlaneMachineTemplate{
				InfrastructureRef: corev1.ObjectReference{
					APIVersion: "test/v1alpha1",
					Kind:       "UnknownInfraMachine",
					Name:       "foo",
				},
			},
		},
	}
	ocnecp.Default()
	g.Expect(ocnecp.ValidateCreate()).To(Succeed())

	objs := []client.Object{cluster.DeepCopy(), ocnecp.DeepCopy(), kubeadmConfigMap()}
	machines := map[string]*clusterv1.Machine{}
	for i := 0; i < 3; i++ {
		name := fmt.Sprintf("test-%d", i)
		m, n := createMachineNodePair(name, cluster, ocnecp, true)
		objs = append(objs, n, m)
		machines[m.Name] = m
	}

	fakeClient := newFakeClient(objs...)
	log.SetLogger(klogr.New())

	r := &OCNEControlPlaneReconciler{
		Client: fakeClient,
		managementCluster: &fakeManagementCluster{
			Machines: machines,
			Workload: fakeWorkloadCluster{
				Status: internal.ClusterStatus{
					Nodes:         3,
					ReadyNodes:    3,
					HasOCNEConfig: true,
				},
			},
		},
		recorder: record.NewFakeRecorder(32),
	}

	g.Expect(r.updateStatus(ctx, ocnecp, cluster)).To(Succeed())
	g.Expect(ocnecp.Status.Replicas).To(BeEquivalentTo(3))
	g.Expect(ocnecp.Status.ReadyReplicas).To(BeEquivalentTo(3))
	g.Expect(ocnecp.Status.UnavailableReplicas).To(BeEquivalentTo(0))
	g.Expect(ocnecp.Status.Selector).NotTo(BeEmpty())
	g.Expect(ocnecp.Status.FailureMessage).To(BeNil())
	g.Expect(ocnecp.Status.FailureReason).To(BeEquivalentTo(""))
	g.Expect(ocnecp.Status.Initialized).To(BeTrue())
	g.Expect(conditions.IsTrue(ocnecp, controlplanev1.AvailableCondition)).To(BeTrue())
	g.Expect(conditions.IsTrue(ocnecp, controlplanev1.MachinesCreatedCondition)).To(BeTrue())
	g.Expect(ocnecp.Status.Ready).To(BeTrue())
}

func TestOCNEControlPlaneReconciler_updateStatusMachinesReadyMixed(t *testing.T) {
	g := NewWithT(t)

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: metav1.NamespaceDefault,
		},
	}

	ocnecp := &controlplanev1.OCNEControlPlane{
		TypeMeta: metav1.TypeMeta{
			Kind:       "OCNEControlPlane",
			APIVersion: controlplanev1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cluster.Namespace,
			Name:      "foo",
		},
		Spec: controlplanev1.OCNEControlPlaneSpec{
			Version: "v1.16.6",
			MachineTemplate: controlplanev1.OCNEControlPlaneMachineTemplate{
				InfrastructureRef: corev1.ObjectReference{
					APIVersion: "test/v1alpha1",
					Kind:       "UnknownInfraMachine",
					Name:       "foo",
				},
			},
		},
	}
	ocnecp.Default()
	g.Expect(ocnecp.ValidateCreate()).To(Succeed())
	machines := map[string]*clusterv1.Machine{}
	objs := []client.Object{cluster.DeepCopy(), ocnecp.DeepCopy()}
	for i := 0; i < 4; i++ {
		name := fmt.Sprintf("test-%d", i)
		m, n := createMachineNodePair(name, cluster, ocnecp, false)
		machines[m.Name] = m
		objs = append(objs, n, m)
	}
	m, n := createMachineNodePair("testReady", cluster, ocnecp, true)
	objs = append(objs, n, m, kubeadmConfigMap())
	machines[m.Name] = m
	fakeClient := newFakeClient(objs...)
	log.SetLogger(klogr.New())

	r := &OCNEControlPlaneReconciler{
		Client: fakeClient,
		managementCluster: &fakeManagementCluster{
			Machines: machines,
			Workload: fakeWorkloadCluster{
				Status: internal.ClusterStatus{
					Nodes:         5,
					ReadyNodes:    1,
					HasOCNEConfig: true,
				},
			},
		},
		recorder: record.NewFakeRecorder(32),
	}

	g.Expect(r.updateStatus(ctx, ocnecp, cluster)).To(Succeed())
	g.Expect(ocnecp.Status.Replicas).To(BeEquivalentTo(5))
	g.Expect(ocnecp.Status.ReadyReplicas).To(BeEquivalentTo(1))
	g.Expect(ocnecp.Status.UnavailableReplicas).To(BeEquivalentTo(4))
	g.Expect(ocnecp.Status.Selector).NotTo(BeEmpty())
	g.Expect(ocnecp.Status.FailureMessage).To(BeNil())
	g.Expect(ocnecp.Status.FailureReason).To(BeEquivalentTo(""))
	g.Expect(ocnecp.Status.Initialized).To(BeTrue())
	g.Expect(ocnecp.Status.Ready).To(BeTrue())
}

func TestOCNEControlPlaneReconciler_machinesCreatedIsIsTrueEvenWhenTheNodesAreNotReady(t *testing.T) {
	g := NewWithT(t)

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: metav1.NamespaceDefault,
		},
	}

	ocnecp := &controlplanev1.OCNEControlPlane{
		TypeMeta: metav1.TypeMeta{
			Kind:       "OCNEControlPlane",
			APIVersion: controlplanev1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cluster.Namespace,
			Name:      "foo",
		},
		Spec: controlplanev1.OCNEControlPlaneSpec{
			Version:  "v1.16.6",
			Replicas: pointer.Int32(3),
			MachineTemplate: controlplanev1.OCNEControlPlaneMachineTemplate{
				InfrastructureRef: corev1.ObjectReference{
					APIVersion: "test/v1alpha1",
					Kind:       "UnknownInfraMachine",
					Name:       "foo",
				},
			},
		},
	}
	ocnecp.Default()
	g.Expect(ocnecp.ValidateCreate()).To(Succeed())
	machines := map[string]*clusterv1.Machine{}
	objs := []client.Object{cluster.DeepCopy(), ocnecp.DeepCopy()}
	// Create the desired number of machines
	for i := 0; i < 3; i++ {
		name := fmt.Sprintf("test-%d", i)
		m, n := createMachineNodePair(name, cluster, ocnecp, false)
		machines[m.Name] = m
		objs = append(objs, n, m)
	}

	fakeClient := newFakeClient(objs...)
	log.SetLogger(klogr.New())

	// Set all the machines to `not ready`
	r := &OCNEControlPlaneReconciler{
		Client: fakeClient,
		managementCluster: &fakeManagementCluster{
			Machines: machines,
			Workload: fakeWorkloadCluster{
				Status: internal.ClusterStatus{
					Nodes:         0,
					ReadyNodes:    0,
					HasOCNEConfig: true,
				},
			},
		},
		recorder: record.NewFakeRecorder(32),
	}

	g.Expect(r.updateStatus(ctx, ocnecp, cluster)).To(Succeed())
	g.Expect(ocnecp.Status.Replicas).To(BeEquivalentTo(3))
	g.Expect(ocnecp.Status.ReadyReplicas).To(BeEquivalentTo(0))
	g.Expect(ocnecp.Status.UnavailableReplicas).To(BeEquivalentTo(3))
	g.Expect(ocnecp.Status.Ready).To(BeFalse())
	g.Expect(conditions.IsTrue(ocnecp, controlplanev1.MachinesCreatedCondition)).To(BeTrue())
}

func kubeadmConfigMap() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kubeadm-config",
			Namespace: metav1.NamespaceSystem,
		},
	}
}
