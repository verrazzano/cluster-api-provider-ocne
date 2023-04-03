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

package controllers

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"sync"
	"testing"
	"time"

	"github.com/blang/semver"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2/klogr"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	bootstrapv1 "github.com/verrazzano/cluster-api-provider-ocne/bootstrap/ocne/api/v1beta1"
	controlplanev1 "github.com/verrazzano/cluster-api-provider-ocne/controlplane/ocne/api/v1beta1"
	"github.com/verrazzano/cluster-api-provider-ocne/controlplane/ocne/internal"
	"github.com/verrazzano/cluster-api-provider-ocne/feature"
	"github.com/verrazzano/cluster-api-provider-ocne/internal/test/builder"
	"github.com/verrazzano/cluster-api-provider-ocne/util"
	"github.com/verrazzano/cluster-api-provider-ocne/util/certs"
	"github.com/verrazzano/cluster-api-provider-ocne/util/collections"
	"github.com/verrazzano/cluster-api-provider-ocne/util/conditions"
	"github.com/verrazzano/cluster-api-provider-ocne/util/kubeconfig"
	"github.com/verrazzano/cluster-api-provider-ocne/util/secret"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/controllers/external"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
)

func TestClusterToKubeadmControlPlane(t *testing.T) {
	g := NewWithT(t)
	fakeClient := newFakeClient()

	cluster := newCluster(&types.NamespacedName{Name: "foo", Namespace: metav1.NamespaceDefault})
	cluster.Spec = clusterv1.ClusterSpec{
		ControlPlaneRef: &corev1.ObjectReference{
			Kind:       "OCNEControlPlane",
			Namespace:  metav1.NamespaceDefault,
			Name:       "ocnecp-foo",
			APIVersion: controlplanev1.GroupVersion.String(),
		},
	}

	expectedResult := []ctrl.Request{
		{
			NamespacedName: client.ObjectKey{
				Namespace: cluster.Spec.ControlPlaneRef.Namespace,
				Name:      cluster.Spec.ControlPlaneRef.Name},
		},
	}

	r := &OCNEControlPlaneReconciler{
		Client:   fakeClient,
		recorder: record.NewFakeRecorder(32),
	}

	got := r.ClusterToKubeadmControlPlane(cluster)
	g.Expect(got).To(Equal(expectedResult))
}

func TestClusterToKubeadmControlPlaneNoControlPlane(t *testing.T) {
	g := NewWithT(t)
	fakeClient := newFakeClient()

	cluster := newCluster(&types.NamespacedName{Name: "foo", Namespace: metav1.NamespaceDefault})

	r := &OCNEControlPlaneReconciler{
		Client:   fakeClient,
		recorder: record.NewFakeRecorder(32),
	}

	got := r.ClusterToKubeadmControlPlane(cluster)
	g.Expect(got).To(BeNil())
}

func TestClusterToKubeadmControlPlaneOtherControlPlane(t *testing.T) {
	g := NewWithT(t)
	fakeClient := newFakeClient()

	cluster := newCluster(&types.NamespacedName{Name: "foo", Namespace: metav1.NamespaceDefault})
	cluster.Spec = clusterv1.ClusterSpec{
		ControlPlaneRef: &corev1.ObjectReference{
			Kind:       "OtherControlPlane",
			Namespace:  metav1.NamespaceDefault,
			Name:       "other-foo",
			APIVersion: controlplanev1.GroupVersion.String(),
		},
	}

	r := &OCNEControlPlaneReconciler{
		Client:   fakeClient,
		recorder: record.NewFakeRecorder(32),
	}

	got := r.ClusterToKubeadmControlPlane(cluster)
	g.Expect(got).To(BeNil())
}

func TestReconcileReturnErrorWhenOwnerClusterIsMissing(t *testing.T) {
	g := NewWithT(t)

	ns, err := env.CreateNamespace(ctx, "test-reconcile-return-error")
	g.Expect(err).ToNot(HaveOccurred())

	cluster, ocnecp, _ := createClusterWithControlPlane(ns.Name)
	g.Expect(env.Create(ctx, cluster)).To(Succeed())
	g.Expect(env.Create(ctx, ocnecp)).To(Succeed())
	defer func(do ...client.Object) {
		g.Expect(env.Cleanup(ctx, do...)).To(Succeed())
	}(ocnecp, ns)

	r := &OCNEControlPlaneReconciler{
		Client:   env,
		recorder: record.NewFakeRecorder(32),
	}

	result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: util.ObjectKey(ocnecp)})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(result).To(Equal(ctrl.Result{}))

	// calling reconcile should return error
	g.Expect(env.CleanupAndWait(ctx, cluster)).To(Succeed())

	g.Eventually(func() error {
		_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: util.ObjectKey(ocnecp)})
		return err
	}, 10*time.Second).Should(HaveOccurred())
}

func TestReconcileUpdateObservedGeneration(t *testing.T) {
	t.Skip("Disabling this test temporarily until we can get a fix for https://github.com/kubernetes/kubernetes/issues/80609 in controller runtime + switch to a live client in test env.")

	g := NewWithT(t)
	r := &OCNEControlPlaneReconciler{
		Client:            env,
		recorder:          record.NewFakeRecorder(32),
		managementCluster: &internal.Management{Client: env.Client, Tracker: nil},
	}

	ns, err := env.CreateNamespace(ctx, "test-reconcile-upd-og")
	g.Expect(err).ToNot(HaveOccurred())

	cluster, ocnecp, _ := createClusterWithControlPlane(ns.Name)
	g.Expect(env.Create(ctx, cluster)).To(Succeed())
	g.Expect(env.Create(ctx, ocnecp)).To(Succeed())
	defer func(do ...client.Object) {
		g.Expect(env.Cleanup(ctx, do...)).To(Succeed())
	}(cluster, ocnecp, ns)

	// read ocnecp.Generation after create
	errGettingObject := env.Get(ctx, util.ObjectKey(ocnecp), ocnecp)
	g.Expect(errGettingObject).NotTo(HaveOccurred())
	generation := ocnecp.Generation

	// Set cluster.status.InfrastructureReady so we actually enter in the reconcile loop
	patch := client.RawPatch(types.MergePatchType, []byte(fmt.Sprintf("{\"status\":{\"infrastructureReady\":%t}}", true)))
	g.Expect(env.Status().Patch(ctx, cluster, patch)).To(Succeed())

	// call reconcile the first time, so we can check if observedGeneration is set when adding a finalizer
	result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: util.ObjectKey(ocnecp)})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(result).To(Equal(ctrl.Result{}))

	g.Eventually(func() int64 {
		errGettingObject = env.Get(ctx, util.ObjectKey(ocnecp), ocnecp)
		g.Expect(errGettingObject).NotTo(HaveOccurred())
		return ocnecp.Status.ObservedGeneration
	}, 10*time.Second).Should(Equal(generation))

	// triggers a generation change by changing the spec
	ocnecp.Spec.Replicas = pointer.Int32(*ocnecp.Spec.Replicas + 2)
	g.Expect(env.Update(ctx, ocnecp)).To(Succeed())

	// read ocnecp.Generation after the update
	errGettingObject = env.Get(ctx, util.ObjectKey(ocnecp), ocnecp)
	g.Expect(errGettingObject).NotTo(HaveOccurred())
	generation = ocnecp.Generation

	// call reconcile the second time, so we can check if observedGeneration is set when calling defer patch
	// NB. The call to reconcile fails because KCP is not properly setup (e.g. missing InfrastructureTemplate)
	// but this is not important because what we want is KCP to be patched
	_, _ = r.Reconcile(ctx, ctrl.Request{NamespacedName: util.ObjectKey(ocnecp)})

	g.Eventually(func() int64 {
		errGettingObject = env.Get(ctx, util.ObjectKey(ocnecp), ocnecp)
		g.Expect(errGettingObject).NotTo(HaveOccurred())
		return ocnecp.Status.ObservedGeneration
	}, 10*time.Second).Should(Equal(generation))
}

func TestReconcileNoClusterOwnerRef(t *testing.T) {
	g := NewWithT(t)

	ocnecp := &controlplanev1.OCNEControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: metav1.NamespaceDefault,
			Name:      "foo",
		},
		Spec: controlplanev1.OCNEControlPlaneSpec{
			Version: "v1.16.6",
			MachineTemplate: controlplanev1.OCNEControlPlaneMachineTemplate{
				InfrastructureRef: corev1.ObjectReference{
					Kind:       "UnknownInfraMachine",
					APIVersion: "test/v1alpha1",
					Name:       "foo",
					Namespace:  metav1.NamespaceDefault,
				},
			},
		},
	}
	ocnecp.Default()
	g.Expect(ocnecp.ValidateCreate()).To(Succeed())

	fakeClient := newFakeClient(ocnecp.DeepCopy())
	r := &OCNEControlPlaneReconciler{
		Client:   fakeClient,
		recorder: record.NewFakeRecorder(32),
	}

	result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: util.ObjectKey(ocnecp)})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(result).To(Equal(ctrl.Result{}))

	machineList := &clusterv1.MachineList{}
	g.Expect(fakeClient.List(ctx, machineList, client.InNamespace(metav1.NamespaceDefault))).To(Succeed())
	g.Expect(machineList.Items).To(BeEmpty())
}

func TestReconcileNoKCP(t *testing.T) {
	g := NewWithT(t)

	ocnecp := &controlplanev1.OCNEControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: metav1.NamespaceDefault,
			Name:      "foo",
		},
		Spec: controlplanev1.OCNEControlPlaneSpec{
			Version: "v1.16.6",
			MachineTemplate: controlplanev1.OCNEControlPlaneMachineTemplate{
				InfrastructureRef: corev1.ObjectReference{
					Kind:       "UnknownInfraMachine",
					APIVersion: "test/v1alpha1",
					Name:       "foo",
					Namespace:  metav1.NamespaceDefault,
				},
			},
		},
	}

	fakeClient := newFakeClient()
	r := &OCNEControlPlaneReconciler{
		Client:   fakeClient,
		recorder: record.NewFakeRecorder(32),
	}

	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: util.ObjectKey(ocnecp)})
	g.Expect(err).NotTo(HaveOccurred())
}

func TestReconcileNoCluster(t *testing.T) {
	g := NewWithT(t)

	ocnecp := &controlplanev1.OCNEControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: metav1.NamespaceDefault,
			Name:      "foo",
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind:       "Cluster",
					APIVersion: clusterv1.GroupVersion.String(),
					Name:       "foo",
				},
			},
		},
		Spec: controlplanev1.OCNEControlPlaneSpec{
			Version: "v1.16.6",
			MachineTemplate: controlplanev1.OCNEControlPlaneMachineTemplate{
				InfrastructureRef: corev1.ObjectReference{
					Kind:       "UnknownInfraMachine",
					APIVersion: "test/v1alpha1",
					Name:       "foo",
					Namespace:  metav1.NamespaceDefault,
				},
			},
		},
	}
	ocnecp.Default()
	g.Expect(ocnecp.ValidateCreate()).To(Succeed())

	fakeClient := newFakeClient(ocnecp.DeepCopy())
	r := &OCNEControlPlaneReconciler{
		Client:   fakeClient,
		recorder: record.NewFakeRecorder(32),
	}

	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: util.ObjectKey(ocnecp)})
	g.Expect(err).To(HaveOccurred())

	machineList := &clusterv1.MachineList{}
	g.Expect(fakeClient.List(ctx, machineList, client.InNamespace(metav1.NamespaceDefault))).To(Succeed())
	g.Expect(machineList.Items).To(BeEmpty())
}

func TestReconcilePaused(t *testing.T) {
	g := NewWithT(t)

	clusterName := "foo"

	// Test: cluster is paused and ocnecp is not
	cluster := newCluster(&types.NamespacedName{Namespace: metav1.NamespaceDefault, Name: clusterName})
	cluster.Spec.Paused = true
	ocnecp := &controlplanev1.OCNEControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: metav1.NamespaceDefault,
			Name:      clusterName,
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind:       "Cluster",
					APIVersion: clusterv1.GroupVersion.String(),
					Name:       clusterName,
				},
			},
		},
		Spec: controlplanev1.OCNEControlPlaneSpec{
			Version: "v1.16.6",
			MachineTemplate: controlplanev1.OCNEControlPlaneMachineTemplate{
				InfrastructureRef: corev1.ObjectReference{
					Kind:       "UnknownInfraMachine",
					APIVersion: "test/v1alpha1",
					Name:       "foo",
					Namespace:  metav1.NamespaceDefault,
				},
			},
		},
	}
	ocnecp.Default()
	g.Expect(ocnecp.ValidateCreate()).To(Succeed())
	fakeClient := newFakeClient(ocnecp.DeepCopy(), cluster.DeepCopy())
	r := &OCNEControlPlaneReconciler{
		Client:   fakeClient,
		recorder: record.NewFakeRecorder(32),
	}

	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: util.ObjectKey(ocnecp)})
	g.Expect(err).NotTo(HaveOccurred())

	machineList := &clusterv1.MachineList{}
	g.Expect(fakeClient.List(ctx, machineList, client.InNamespace(metav1.NamespaceDefault))).To(Succeed())
	g.Expect(machineList.Items).To(BeEmpty())

	// Test: ocnecp is paused and cluster is not
	cluster.Spec.Paused = false
	ocnecp.ObjectMeta.Annotations = map[string]string{}
	ocnecp.ObjectMeta.Annotations[clusterv1.PausedAnnotation] = "paused"
	_, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: util.ObjectKey(ocnecp)})
	g.Expect(err).NotTo(HaveOccurred())
}

func TestReconcileClusterNoEndpoints(t *testing.T) {
	g := NewWithT(t)

	cluster := newCluster(&types.NamespacedName{Name: "foo", Namespace: metav1.NamespaceDefault})
	cluster.Status = clusterv1.ClusterStatus{InfrastructureReady: true}

	ocnecp := &controlplanev1.OCNEControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cluster.Namespace,
			Name:      "foo",
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind:       "Cluster",
					APIVersion: clusterv1.GroupVersion.String(),
					Name:       cluster.Name,
				},
			},
		},
		Spec: controlplanev1.OCNEControlPlaneSpec{
			Version: "v1.16.6",
			MachineTemplate: controlplanev1.OCNEControlPlaneMachineTemplate{
				InfrastructureRef: corev1.ObjectReference{
					Kind:       "UnknownInfraMachine",
					APIVersion: "test/v1alpha1",
					Name:       "foo",
					Namespace:  metav1.NamespaceDefault,
				},
			},
		},
	}
	ocnecp.Default()
	g.Expect(ocnecp.ValidateCreate()).To(Succeed())

	fakeClient := newFakeClient(ocnecp.DeepCopy(), cluster.DeepCopy())
	r := &OCNEControlPlaneReconciler{
		Client:   fakeClient,
		recorder: record.NewFakeRecorder(32),
		managementCluster: &fakeManagementCluster{
			Management: &internal.Management{Client: fakeClient},
			Workload:   fakeWorkloadCluster{},
		},
	}

	result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: util.ObjectKey(ocnecp)})
	g.Expect(err).NotTo(HaveOccurred())
	// this first requeue is to add finalizer
	g.Expect(result).To(Equal(ctrl.Result{}))
	g.Expect(r.Client.Get(ctx, util.ObjectKey(ocnecp), ocnecp)).To(Succeed())
	g.Expect(ocnecp.Finalizers).To(ContainElement(controlplanev1.OCNEControlPlaneFinalizer))

	result, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: util.ObjectKey(ocnecp)})
	g.Expect(err).NotTo(HaveOccurred())
	// TODO: this should stop to re-queue as soon as we have a proper remote cluster cache in place.
	g.Expect(result).To(Equal(ctrl.Result{Requeue: false, RequeueAfter: 20 * time.Second}))
	g.Expect(r.Client.Get(ctx, util.ObjectKey(ocnecp), ocnecp)).To(Succeed())

	// Always expect that the Finalizer is set on the passed in resource
	g.Expect(ocnecp.Finalizers).To(ContainElement(controlplanev1.OCNEControlPlaneFinalizer))

	g.Expect(ocnecp.Status.Selector).NotTo(BeEmpty())

	_, err = secret.GetFromNamespacedName(ctx, fakeClient, client.ObjectKey{Namespace: metav1.NamespaceDefault, Name: "foo"}, secret.ClusterCA)
	g.Expect(err).NotTo(HaveOccurred())

	machineList := &clusterv1.MachineList{}
	g.Expect(fakeClient.List(ctx, machineList, client.InNamespace(metav1.NamespaceDefault))).To(Succeed())
	g.Expect(machineList.Items).To(BeEmpty())
}

func TestOCNEControlPlaneReconciler_adoption(t *testing.T) {
	version := "v2.0.0"
	t.Run("adopts existing Machines", func(t *testing.T) {
		g := NewWithT(t)

		cluster, ocnecp, tmpl := createClusterWithControlPlane(metav1.NamespaceDefault)
		cluster.Spec.ControlPlaneEndpoint.Host = "bar"
		cluster.Spec.ControlPlaneEndpoint.Port = 6443
		cluster.Status.InfrastructureReady = true
		ocnecp.Spec.Version = version

		fmc := &fakeManagementCluster{
			Machines: collections.Machines{},
			Workload: fakeWorkloadCluster{},
		}
		objs := []client.Object{fakeGenericMachineTemplateCRD, cluster.DeepCopy(), ocnecp.DeepCopy(), tmpl.DeepCopy()}
		for i := 0; i < 3; i++ {
			name := fmt.Sprintf("test-%d", i)
			m := &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: cluster.Namespace,
					Name:      name,
					Labels:    internal.ControlPlaneMachineLabelsForCluster(ocnecp, cluster.Name),
				},
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: &corev1.ObjectReference{
							APIVersion: bootstrapv1.GroupVersion.String(),
							Kind:       "OCNEConfig",
							Name:       name,
						},
					},
					Version: &version,
				},
			}
			cfg := &bootstrapv1.OCNEConfig{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: cluster.Namespace,
					Name:      name,
				},
			}
			objs = append(objs, m, cfg)
			fmc.Machines.Insert(m)
		}

		fakeClient := newFakeClient(objs...)
		fmc.Reader = fakeClient
		r := &OCNEControlPlaneReconciler{
			Client:                    fakeClient,
			APIReader:                 fakeClient,
			managementCluster:         fmc,
			managementClusterUncached: fmc,
		}

		g.Expect(r.reconcile(ctx, cluster, ocnecp)).To(Equal(ctrl.Result{}))

		machineList := &clusterv1.MachineList{}
		g.Expect(fakeClient.List(ctx, machineList, client.InNamespace(cluster.Namespace))).To(Succeed())
		g.Expect(machineList.Items).To(HaveLen(3))
		for _, machine := range machineList.Items {
			g.Expect(machine.OwnerReferences).To(HaveLen(1))
			g.Expect(machine.OwnerReferences).To(ContainElement(*metav1.NewControllerRef(ocnecp, controlplanev1.GroupVersion.WithKind("OCNEControlPlane"))))
			// Machines are adopted but since they are not originally created by KCP, infra template annotation will be missing.
			g.Expect(machine.GetAnnotations()).NotTo(HaveKey(clusterv1.TemplateClonedFromGroupKindAnnotation))
			g.Expect(machine.GetAnnotations()).NotTo(HaveKey(clusterv1.TemplateClonedFromNameAnnotation))
		}
	})

	t.Run("adopts v1alpha2 cluster secrets", func(t *testing.T) {
		g := NewWithT(t)

		cluster, ocnecp, tmpl := createClusterWithControlPlane(metav1.NamespaceDefault)
		cluster.Spec.ControlPlaneEndpoint.Host = "validhost"
		cluster.Spec.ControlPlaneEndpoint.Port = 6443
		cluster.Status.InfrastructureReady = true
		ocnecp.Spec.Version = version

		fmc := &fakeManagementCluster{
			Machines: collections.Machines{},
			Workload: fakeWorkloadCluster{},
		}
		objs := []client.Object{fakeGenericMachineTemplateCRD, cluster.DeepCopy(), ocnecp.DeepCopy(), tmpl.DeepCopy()}
		for i := 0; i < 3; i++ {
			name := fmt.Sprintf("test-%d", i)
			m := &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: cluster.Namespace,
					Name:      name,
					Labels:    internal.ControlPlaneMachineLabelsForCluster(ocnecp, cluster.Name),
				},
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: &corev1.ObjectReference{
							APIVersion: bootstrapv1.GroupVersion.String(),
							Kind:       "OCNEConfig",
							Name:       name,
						},
					},
					Version: &version,
				},
			}
			cfg := &bootstrapv1.OCNEConfig{
				TypeMeta: metav1.TypeMeta{
					APIVersion: bootstrapv1.GroupVersion.String(),
					Kind:       "OCNEConfig",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: cluster.Namespace,
					Name:      name,
					UID:       types.UID(fmt.Sprintf("my-uid-%d", i)),
				},
			}

			// A simulacrum of the various Certificate and kubeconfig secrets
			// it's a little weird that this is one per OCNEConfig rather than just whichever config was "first,"
			// but the intent is to ensure that the owner is changed regardless of which Machine we start with
			clusterSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: cluster.Namespace,
					Name:      fmt.Sprintf("important-cluster-secret-%d", i),
					Labels: map[string]string{
						"cluster.x-k8s.io/cluster-name": cluster.Name,
						"previous-owner":                "kubeadmconfig",
					},
					// See: https://github.com/kubernetes-sigs/cluster-api-bootstrap-provider-kubeadm/blob/38af74d92056e64e571b9ea1d664311769779453/internal/cluster/certificates.go#L323-L330
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: bootstrapv1.GroupVersion.String(),
							Kind:       "OCNEConfig",
							Name:       cfg.Name,
							UID:        cfg.UID,
						},
					},
				},
			}
			objs = append(objs, m, cfg, clusterSecret)
			fmc.Machines.Insert(m)
		}

		fakeClient := newFakeClient(objs...)
		fmc.Reader = fakeClient
		r := &OCNEControlPlaneReconciler{
			Client:                    fakeClient,
			APIReader:                 fakeClient,
			managementCluster:         fmc,
			managementClusterUncached: fmc,
		}

		g.Expect(r.reconcile(ctx, cluster, ocnecp)).To(Equal(ctrl.Result{}))

		machineList := &clusterv1.MachineList{}
		g.Expect(fakeClient.List(ctx, machineList, client.InNamespace(cluster.Namespace))).To(Succeed())
		g.Expect(machineList.Items).To(HaveLen(3))
		for _, machine := range machineList.Items {
			g.Expect(machine.OwnerReferences).To(HaveLen(1))
			g.Expect(machine.OwnerReferences).To(ContainElement(*metav1.NewControllerRef(ocnecp, controlplanev1.GroupVersion.WithKind("OCNEControlPlane"))))
			// Machines are adopted but since they are not originally created by KCP, infra template annotation will be missing.
			g.Expect(machine.GetAnnotations()).NotTo(HaveKey(clusterv1.TemplateClonedFromGroupKindAnnotation))
			g.Expect(machine.GetAnnotations()).NotTo(HaveKey(clusterv1.TemplateClonedFromNameAnnotation))
		}

		secrets := &corev1.SecretList{}
		g.Expect(fakeClient.List(ctx, secrets, client.InNamespace(cluster.Namespace), client.MatchingLabels{"previous-owner": "kubeadmconfig"})).To(Succeed())
		g.Expect(secrets.Items).To(HaveLen(3))
		for _, secret := range secrets.Items {
			g.Expect(secret.OwnerReferences).To(HaveLen(1))
			g.Expect(secret.OwnerReferences).To(ContainElement(*metav1.NewControllerRef(ocnecp, controlplanev1.GroupVersion.WithKind("OCNEControlPlane"))))
		}
	})

	t.Run("Deleted KubeadmControlPlanes don't adopt machines", func(t *testing.T) {
		// Usually we won't get into the inner reconcile with a deleted control plane, but it's possible when deleting with "oprhanDependents":
		// 1. The deletion timestamp is set in the API server, but our cache has not yet updated
		// 2. The garbage collector removes our ownership reference from a Machine, triggering a re-reconcile (or we get unlucky with the periodic reconciliation)
		// 3. We get into the inner reconcile function and re-adopt the Machine
		// 4. The update to our cache for our deletion timestamp arrives
		g := NewWithT(t)

		cluster, ocnecp, tmpl := createClusterWithControlPlane(metav1.NamespaceDefault)
		cluster.Spec.ControlPlaneEndpoint.Host = "nodomain.example.com1"
		cluster.Spec.ControlPlaneEndpoint.Port = 6443
		cluster.Status.InfrastructureReady = true
		ocnecp.Spec.Version = version

		now := metav1.Now()
		ocnecp.DeletionTimestamp = &now

		fmc := &fakeManagementCluster{
			Machines: collections.Machines{},
			Workload: fakeWorkloadCluster{},
		}
		objs := []client.Object{fakeGenericMachineTemplateCRD, cluster.DeepCopy(), ocnecp.DeepCopy(), tmpl.DeepCopy()}
		for i := 0; i < 3; i++ {
			name := fmt.Sprintf("test-%d", i)
			m := &clusterv1.Machine{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: cluster.Namespace,
					Name:      name,
					Labels:    internal.ControlPlaneMachineLabelsForCluster(ocnecp, cluster.Name),
				},
				Spec: clusterv1.MachineSpec{
					Bootstrap: clusterv1.Bootstrap{
						ConfigRef: &corev1.ObjectReference{
							APIVersion: bootstrapv1.GroupVersion.String(),
							Kind:       "OCNEConfig",
							Name:       name,
						},
					},
					Version: &version,
				},
			}
			cfg := &bootstrapv1.OCNEConfig{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: cluster.Namespace,
					Name:      name,
				},
			}
			objs = append(objs, m, cfg)
			fmc.Machines.Insert(m)
		}
		fakeClient := newFakeClient(objs...)
		fmc.Reader = fakeClient
		r := &OCNEControlPlaneReconciler{
			Client:                    fakeClient,
			APIReader:                 fakeClient,
			managementCluster:         fmc,
			managementClusterUncached: fmc,
		}

		result, err := r.reconcile(ctx, cluster, ocnecp)
		g.Expect(result).To(Equal(ctrl.Result{}))
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("has just been deleted"))

		machineList := &clusterv1.MachineList{}
		g.Expect(fakeClient.List(ctx, machineList, client.InNamespace(cluster.Namespace))).To(Succeed())
		g.Expect(machineList.Items).To(HaveLen(3))
		for _, machine := range machineList.Items {
			g.Expect(machine.OwnerReferences).To(BeEmpty())
		}
	})

	t.Run("refuses to adopt Machines that are more than one version old", func(t *testing.T) {
		g := NewWithT(t)

		cluster, ocnecp, tmpl := createClusterWithControlPlane(metav1.NamespaceDefault)
		cluster.Spec.ControlPlaneEndpoint.Host = "nodomain.example.com2"
		cluster.Spec.ControlPlaneEndpoint.Port = 6443
		cluster.Status.InfrastructureReady = true
		ocnecp.Spec.Version = "v1.17.0"

		fmc := &fakeManagementCluster{
			Machines: collections.Machines{
				"test0": &clusterv1.Machine{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: cluster.Namespace,
						Name:      "test0",
						Labels:    internal.ControlPlaneMachineLabelsForCluster(ocnecp, cluster.Name),
					},
					Spec: clusterv1.MachineSpec{
						Bootstrap: clusterv1.Bootstrap{
							ConfigRef: &corev1.ObjectReference{
								APIVersion: bootstrapv1.GroupVersion.String(),
								Kind:       "OCNEConfig",
							},
						},
						Version: pointer.String("v1.15.0"),
					},
				},
			},
			Workload: fakeWorkloadCluster{},
		}

		fakeClient := newFakeClient(fakeGenericMachineTemplateCRD, cluster.DeepCopy(), ocnecp.DeepCopy(), tmpl.DeepCopy(), fmc.Machines["test0"].DeepCopy())
		fmc.Reader = fakeClient
		recorder := record.NewFakeRecorder(32)
		r := &OCNEControlPlaneReconciler{
			Client:                    fakeClient,
			APIReader:                 fakeClient,
			recorder:                  recorder,
			managementCluster:         fmc,
			managementClusterUncached: fmc,
		}

		g.Expect(r.reconcile(ctx, cluster, ocnecp)).To(Equal(ctrl.Result{}))
		// Message: Warning AdoptionFailed Could not adopt Machine test/test0: its version ("v1.15.0") is outside supported +/- one minor version skew from KCP's ("v1.17.0")
		g.Expect(recorder.Events).To(Receive(ContainSubstring("minor version")))

		machineList := &clusterv1.MachineList{}
		g.Expect(fakeClient.List(ctx, machineList, client.InNamespace(cluster.Namespace))).To(Succeed())
		g.Expect(machineList.Items).To(HaveLen(1))
		for _, machine := range machineList.Items {
			g.Expect(machine.OwnerReferences).To(BeEmpty())
		}
	})
}

func TestOCNEControlPlaneReconciler_ensureOwnerReferences(t *testing.T) {
	g := NewWithT(t)

	cluster, ocnecp, tmpl := createClusterWithControlPlane(metav1.NamespaceDefault)
	cluster.Spec.ControlPlaneEndpoint.Host = "bar"
	cluster.Spec.ControlPlaneEndpoint.Port = 6443
	cluster.Status.InfrastructureReady = true
	ocnecp.Spec.Version = "v1.21.0"
	key, err := certs.NewPrivateKey()
	g.Expect(err).To(BeNil())
	crt, err := getTestCACert(key)
	g.Expect(err).To(BeNil())

	fmc := &fakeManagementCluster{
		Machines: collections.Machines{},
		Workload: fakeWorkloadCluster{},
	}

	clusterSecret := &corev1.Secret{
		// The Secret's Type is used by KCP to determine whether it is user-provided.
		// clusterv1.ClusterSecretType signals that the Secret is CAPI-provided.
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cluster.Namespace,
			Name:      "",
			Labels: map[string]string{
				"cluster.x-k8s.io/cluster-name": cluster.Name,
				"testing":                       "yes",
			},
		},
		Data: map[string][]byte{
			secret.TLSCrtDataName: certs.EncodeCertPEM(crt),
			secret.TLSKeyDataName: certs.EncodePrivateKeyPEM(key),
		},
	}

	t.Run("add KCP owner for secrets with no controller reference", func(t *testing.T) {
		objs := []client.Object{fakeGenericMachineTemplateCRD, cluster.DeepCopy(), ocnecp.DeepCopy(), tmpl.DeepCopy()}
		for _, purpose := range []secret.Purpose{secret.ClusterCA, secret.FrontProxyCA, secret.ServiceAccount, secret.EtcdCA} {
			s := clusterSecret.DeepCopy()
			// Set the secret name to the purpose
			s.Name = secret.Name(cluster.Name, purpose)
			// Set the Secret Type to clusterv1.ClusterSecretType which signals this Secret was generated by CAPI.
			s.Type = clusterv1.ClusterSecretType

			objs = append(objs, s)
		}

		fakeClient := newFakeClient(objs...)
		fmc.Reader = fakeClient
		r := &OCNEControlPlaneReconciler{
			Client:                    fakeClient,
			APIReader:                 fakeClient,
			managementCluster:         fmc,
			managementClusterUncached: fmc,
		}

		_, err := r.reconcile(ctx, cluster, ocnecp)
		g.Expect(err).To(BeNil())

		secrets := &corev1.SecretList{}
		g.Expect(fakeClient.List(ctx, secrets, client.InNamespace(cluster.Namespace), client.MatchingLabels{"testing": "yes"})).To(Succeed())
		for _, secret := range secrets.Items {
			g.Expect(secret.OwnerReferences).To(ContainElement(*metav1.NewControllerRef(ocnecp, controlplanev1.GroupVersion.WithKind("OCNEControlPlane"))))
		}
	})

	t.Run("replace non-KCP controller with KCP controller reference", func(t *testing.T) {
		objs := []client.Object{fakeGenericMachineTemplateCRD, cluster.DeepCopy(), ocnecp.DeepCopy(), tmpl.DeepCopy()}
		for _, purpose := range []secret.Purpose{secret.ClusterCA, secret.FrontProxyCA, secret.ServiceAccount, secret.EtcdCA} {
			s := clusterSecret.DeepCopy()
			// Set the secret name to the purpose
			s.Name = secret.Name(cluster.Name, purpose)
			// Set the Secret Type to clusterv1.ClusterSecretType which signals this Secret was generated by CAPI.
			s.Type = clusterv1.ClusterSecretType

			// Set the a controller owner reference of an unknown type on the secret.
			s.SetOwnerReferences([]metav1.OwnerReference{
				{
					APIVersion: bootstrapv1.GroupVersion.String(),
					// KCP should take ownership of any Secret of the correct type linked to the Cluster.
					Kind:       "OtherController",
					Name:       "name",
					UID:        "uid",
					Controller: pointer.Bool(true),
				},
			})
			objs = append(objs, s)
		}

		fakeClient := newFakeClient(objs...)
		fmc.Reader = fakeClient
		r := &OCNEControlPlaneReconciler{
			Client:                    fakeClient,
			APIReader:                 fakeClient,
			managementCluster:         fmc,
			managementClusterUncached: fmc,
		}

		_, err := r.reconcile(ctx, cluster, ocnecp)
		g.Expect(err).To(BeNil())

		secrets := &corev1.SecretList{}
		g.Expect(fakeClient.List(ctx, secrets, client.InNamespace(cluster.Namespace), client.MatchingLabels{"testing": "yes"})).To(Succeed())
		for _, secret := range secrets.Items {
			g.Expect(secret.OwnerReferences).To(HaveLen(1))
			g.Expect(secret.OwnerReferences).To(ContainElement(*metav1.NewControllerRef(ocnecp, controlplanev1.GroupVersion.WithKind("OCNEControlPlane"))))
		}
	})

	t.Run("does not add owner reference to user-provided secrets", func(t *testing.T) {
		g := NewWithT(t)
		objs := []client.Object{fakeGenericMachineTemplateCRD, cluster.DeepCopy(), ocnecp.DeepCopy(), tmpl.DeepCopy()}
		for _, purpose := range []secret.Purpose{secret.ClusterCA, secret.FrontProxyCA, secret.ServiceAccount, secret.EtcdCA} {
			s := clusterSecret.DeepCopy()
			// Set the secret name to the purpose
			s.Name = secret.Name(cluster.Name, purpose)
			// Set the Secret Type to any type which signals this Secret was is user-provided.
			s.Type = corev1.SecretTypeOpaque
			// Set the a controller owner reference of an unknown type on the secret.
			s.SetOwnerReferences([]metav1.OwnerReference{
				{
					APIVersion: bootstrapv1.GroupVersion.String(),
					// This owner reference to a different controller should be preserved.
					Kind:               "OtherController",
					Name:               ocnecp.Name,
					UID:                ocnecp.UID,
					Controller:         pointer.Bool(true),
					BlockOwnerDeletion: pointer.Bool(true),
				},
			})

			objs = append(objs, s)
		}

		fakeClient := newFakeClient(objs...)
		fmc.Reader = fakeClient
		r := &OCNEControlPlaneReconciler{
			Client:                    fakeClient,
			APIReader:                 fakeClient,
			managementCluster:         fmc,
			managementClusterUncached: fmc,
		}

		_, err := r.reconcile(ctx, cluster, ocnecp)
		g.Expect(err).To(BeNil())

		secrets := &corev1.SecretList{}
		g.Expect(fakeClient.List(ctx, secrets, client.InNamespace(cluster.Namespace), client.MatchingLabels{"testing": "yes"})).To(Succeed())
		for _, secret := range secrets.Items {
			g.Expect(secret.OwnerReferences).To(HaveLen(1))
			g.Expect(secret.OwnerReferences).To(ContainElement(*metav1.NewControllerRef(ocnecp, bootstrapv1.GroupVersion.WithKind("OtherController"))))
		}
	})
}

func TestReconcileCertificateExpiries(t *testing.T) {
	g := NewWithT(t)

	preExistingExpiry := time.Now().Add(5 * 24 * time.Hour)
	detectedExpiry := time.Now().Add(25 * 24 * time.Hour)

	cluster := newCluster(&types.NamespacedName{Name: "foo", Namespace: metav1.NamespaceDefault})
	ocnecp := &controlplanev1.OCNEControlPlane{
		Status: controlplanev1.OCNEControlPlaneStatus{Initialized: true},
	}
	machineWithoutExpiryAnnotation := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name: "machineWithoutExpiryAnnotation",
		},
		Spec: clusterv1.MachineSpec{
			InfrastructureRef: corev1.ObjectReference{
				Kind:       "GenericMachine",
				APIVersion: "generic.io/v1",
				Namespace:  metav1.NamespaceDefault,
				Name:       "machineWithoutExpiryAnnotation-infra",
			},
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: &corev1.ObjectReference{
					Kind:       "OCNEConfig",
					APIVersion: bootstrapv1.GroupVersion.String(),
					Namespace:  metav1.NamespaceDefault,
					Name:       "machineWithoutExpiryAnnotation-bootstrap",
				},
			},
		},
		Status: clusterv1.MachineStatus{
			NodeRef: &corev1.ObjectReference{
				Name: "machineWithoutExpiryAnnotation",
			},
		},
	}
	machineWithoutExpiryAnnotationKubeadmConfig := &bootstrapv1.OCNEConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "machineWithoutExpiryAnnotation-bootstrap",
		},
	}
	machineWithExpiryAnnotation := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name: "machineWithExpiryAnnotation",
		},
		Spec: clusterv1.MachineSpec{
			InfrastructureRef: corev1.ObjectReference{
				Kind:       "GenericMachine",
				APIVersion: "generic.io/v1",
				Namespace:  metav1.NamespaceDefault,
				Name:       "machineWithExpiryAnnotation-infra",
			},
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: &corev1.ObjectReference{
					Kind:       "OCNEConfig",
					APIVersion: bootstrapv1.GroupVersion.String(),
					Namespace:  metav1.NamespaceDefault,
					Name:       "machineWithExpiryAnnotation-bootstrap",
				},
			},
		},
		Status: clusterv1.MachineStatus{
			NodeRef: &corev1.ObjectReference{
				Name: "machineWithExpiryAnnotation",
			},
		},
	}
	machineWithExpiryAnnotationKubeadmConfig := &bootstrapv1.OCNEConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "machineWithExpiryAnnotation-bootstrap",
			Annotations: map[string]string{
				clusterv1.MachineCertificatesExpiryDateAnnotation: preExistingExpiry.Format(time.RFC3339),
			},
		},
	}
	machineWithDeletionTimestamp := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "machineWithDeletionTimestamp",
			DeletionTimestamp: &metav1.Time{Time: time.Now()},
		},
		Spec: clusterv1.MachineSpec{
			InfrastructureRef: corev1.ObjectReference{
				Kind:       "GenericMachine",
				APIVersion: "generic.io/v1",
				Namespace:  metav1.NamespaceDefault,
				Name:       "machineWithDeletionTimestamp-infra",
			},
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: &corev1.ObjectReference{
					Kind:       "OCNEConfig",
					APIVersion: bootstrapv1.GroupVersion.String(),
					Namespace:  metav1.NamespaceDefault,
					Name:       "machineWithDeletionTimestamp-bootstrap",
				},
			},
		},
		Status: clusterv1.MachineStatus{
			NodeRef: &corev1.ObjectReference{
				Name: "machineWithDeletionTimestamp",
			},
		},
	}
	machineWithDeletionTimestampKubeadmConfig := &bootstrapv1.OCNEConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "machineWithDeletionTimestamp-bootstrap",
		},
	}
	machineWithoutNodeRef := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name: "machineWithoutNodeRef",
		},
		Spec: clusterv1.MachineSpec{
			InfrastructureRef: corev1.ObjectReference{
				Kind:       "GenericMachine",
				APIVersion: "generic.io/v1",
				Namespace:  metav1.NamespaceDefault,
				Name:       "machineWithoutNodeRef-infra",
			},
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: &corev1.ObjectReference{
					Kind:       "OCNEConfig",
					APIVersion: bootstrapv1.GroupVersion.String(),
					Namespace:  metav1.NamespaceDefault,
					Name:       "machineWithoutNodeRef-bootstrap",
				},
			},
		},
	}
	machineWithoutNodeRefKubeadmConfig := &bootstrapv1.OCNEConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: "machineWithoutNodeRef-bootstrap",
		},
	}
	machineWithoutKubeadmConfig := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name: "machineWithoutKubeadmConfig",
		},
		Spec: clusterv1.MachineSpec{
			InfrastructureRef: corev1.ObjectReference{
				Kind:       "GenericMachine",
				APIVersion: "generic.io/v1",
				Namespace:  metav1.NamespaceDefault,
				Name:       "machineWithoutKubeadmConfig-infra",
			},
			Bootstrap: clusterv1.Bootstrap{
				ConfigRef: &corev1.ObjectReference{
					Kind:       "OCNEConfig",
					APIVersion: bootstrapv1.GroupVersion.String(),
					Namespace:  metav1.NamespaceDefault,
					Name:       "machineWithoutKubeadmConfig-bootstrap",
				},
			},
		},
		Status: clusterv1.MachineStatus{
			NodeRef: &corev1.ObjectReference{
				Name: "machineWithoutKubeadmConfig",
			},
		},
	}

	ownedMachines := collections.FromMachines(
		machineWithoutExpiryAnnotation,
		machineWithExpiryAnnotation,
		machineWithDeletionTimestamp,
		machineWithoutNodeRef,
		machineWithoutKubeadmConfig,
	)

	fakeClient := newFakeClient(
		machineWithoutExpiryAnnotationKubeadmConfig,
		machineWithExpiryAnnotationKubeadmConfig,
		machineWithDeletionTimestampKubeadmConfig,
		machineWithoutNodeRefKubeadmConfig,
	)

	controlPlane, err := internal.NewControlPlane(ctx, fakeClient, cluster, ocnecp, ownedMachines)
	g.Expect(err).ToNot(HaveOccurred())

	r := &OCNEControlPlaneReconciler{
		Client: fakeClient,
		managementCluster: &fakeManagementCluster{
			Workload: fakeWorkloadCluster{
				APIServerCertificateExpiry: &detectedExpiry,
			},
		},
	}

	_, err = r.reconcileCertificateExpiries(ctx, controlPlane)
	g.Expect(err).NotTo(HaveOccurred())

	// Verify machineWithoutExpiryAnnotation has detectedExpiry.
	actualKubeadmConfig := bootstrapv1.OCNEConfig{}
	err = fakeClient.Get(ctx, client.ObjectKeyFromObject(machineWithoutExpiryAnnotationKubeadmConfig), &actualKubeadmConfig)
	g.Expect(err).NotTo(HaveOccurred())
	actualExpiry := actualKubeadmConfig.Annotations[clusterv1.MachineCertificatesExpiryDateAnnotation]
	g.Expect(actualExpiry).To(Equal(detectedExpiry.Format(time.RFC3339)))

	// Verify machineWithExpiryAnnotation has still preExistingExpiry.
	err = fakeClient.Get(ctx, client.ObjectKeyFromObject(machineWithExpiryAnnotationKubeadmConfig), &actualKubeadmConfig)
	g.Expect(err).NotTo(HaveOccurred())
	actualExpiry = actualKubeadmConfig.Annotations[clusterv1.MachineCertificatesExpiryDateAnnotation]
	g.Expect(actualExpiry).To(Equal(preExistingExpiry.Format(time.RFC3339)))

	// Verify machineWithDeletionTimestamp has still no expiry annotation.
	err = fakeClient.Get(ctx, client.ObjectKeyFromObject(machineWithDeletionTimestampKubeadmConfig), &actualKubeadmConfig)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(actualKubeadmConfig.Annotations).ToNot(ContainElement(clusterv1.MachineCertificatesExpiryDateAnnotation))

	// Verify machineWithoutNodeRef has still no expiry annotation.
	err = fakeClient.Get(ctx, client.ObjectKeyFromObject(machineWithoutNodeRefKubeadmConfig), &actualKubeadmConfig)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(actualKubeadmConfig.Annotations).ToNot(ContainElement(clusterv1.MachineCertificatesExpiryDateAnnotation))
}

func TestReconcileInitializeControlPlane(t *testing.T) {
	g := NewWithT(t)

	cluster := newCluster(&types.NamespacedName{Name: "foo", Namespace: metav1.NamespaceDefault})
	cluster.Spec = clusterv1.ClusterSpec{
		ControlPlaneEndpoint: clusterv1.APIEndpoint{
			Host: "test.local",
			Port: 9999,
		},
	}
	cluster.Status = clusterv1.ClusterStatus{InfrastructureReady: true}

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
			Namespace: cluster.Namespace,
			Name:      "foo",
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind:       "Cluster",
					APIVersion: clusterv1.GroupVersion.String(),
					Name:       cluster.Name,
				},
			},
		},
		Spec: controlplanev1.OCNEControlPlaneSpec{
			Replicas: nil,
			Version:  "v1.16.6",
			MachineTemplate: controlplanev1.OCNEControlPlaneMachineTemplate{
				InfrastructureRef: corev1.ObjectReference{
					Kind:       genericMachineTemplate.GetKind(),
					APIVersion: genericMachineTemplate.GetAPIVersion(),
					Name:       genericMachineTemplate.GetName(),
					Namespace:  cluster.Namespace,
				},
			},
			OCNEConfigSpec: bootstrapv1.OCNEConfigSpec{},
		},
	}
	ocnecp.Default()
	g.Expect(ocnecp.ValidateCreate()).To(Succeed())

	corednsCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "coredns",
			Namespace: metav1.NamespaceSystem,
		},
		Data: map[string]string{
			"Corefile": "original-core-file",
		},
	}

	kubeadmCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kubeadm-config",
			Namespace: metav1.NamespaceSystem,
		},
		Data: map[string]string{
			"ClusterConfiguration": `apiServer:
dns:
  type: CoreDNS
imageRepository: registry.k8s.io
kind: ClusterConfiguration
kubernetesVersion: metav1.16.1`,
		},
	}
	corednsDepl := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "coredns",
			Namespace: metav1.NamespaceSystem,
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name: "coredns",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "coredns",
						Image: "registry.k8s.io/coredns:1.6.2",
					}},
				},
			},
		},
	}

	fakeClient := newFakeClient(
		fakeGenericMachineTemplateCRD,
		ocnecp.DeepCopy(),
		cluster.DeepCopy(),
		genericMachineTemplate.DeepCopy(),
		corednsCM.DeepCopy(),
		kubeadmCM.DeepCopy(),
		corednsDepl.DeepCopy(),
	)
	expectedLabels := map[string]string{clusterv1.ClusterLabelName: "foo"}
	r := &OCNEControlPlaneReconciler{
		Client:    fakeClient,
		APIReader: fakeClient,
		recorder:  record.NewFakeRecorder(32),
		managementCluster: &fakeManagementCluster{
			Management: &internal.Management{Client: fakeClient},
			Workload: fakeWorkloadCluster{
				Workload: &internal.Workload{
					Client: fakeClient,
				},
				Status: internal.ClusterStatus{},
			},
		},
		managementClusterUncached: &fakeManagementCluster{
			Management: &internal.Management{Client: fakeClient},
			Workload: fakeWorkloadCluster{
				Workload: &internal.Workload{
					Client: fakeClient,
				},
				Status: internal.ClusterStatus{},
			},
		},
	}

	result, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: util.ObjectKey(ocnecp)})
	g.Expect(err).NotTo(HaveOccurred())
	// this first requeue is to add finalizer
	g.Expect(result).To(Equal(ctrl.Result{}))
	g.Expect(r.Client.Get(ctx, util.ObjectKey(ocnecp), ocnecp)).To(Succeed())
	g.Expect(ocnecp.Finalizers).To(ContainElement(controlplanev1.OCNEControlPlaneFinalizer))

	result, err = r.Reconcile(ctx, ctrl.Request{NamespacedName: util.ObjectKey(ocnecp)})
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(result).To(Equal(ctrl.Result{Requeue: true}))
	g.Expect(r.Client.Get(ctx, client.ObjectKey{Name: ocnecp.Name, Namespace: ocnecp.Namespace}, ocnecp)).To(Succeed())
	// Expect the referenced infrastructure template to have a Cluster Owner Reference.
	g.Expect(fakeClient.Get(ctx, util.ObjectKey(genericMachineTemplate), genericMachineTemplate)).To(Succeed())
	g.Expect(genericMachineTemplate.GetOwnerReferences()).To(ContainElement(metav1.OwnerReference{
		APIVersion: clusterv1.GroupVersion.String(),
		Kind:       "Cluster",
		Name:       cluster.Name,
	}))

	// Always expect that the Finalizer is set on the passed in resource
	g.Expect(ocnecp.Finalizers).To(ContainElement(controlplanev1.OCNEControlPlaneFinalizer))

	g.Expect(ocnecp.Status.Selector).NotTo(BeEmpty())
	g.Expect(ocnecp.Status.Replicas).To(BeEquivalentTo(1))
	g.Expect(conditions.IsFalse(ocnecp, controlplanev1.AvailableCondition)).To(BeTrue())

	s, err := secret.GetFromNamespacedName(ctx, fakeClient, client.ObjectKey{Namespace: metav1.NamespaceDefault, Name: "foo"}, secret.ClusterCA)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(s).NotTo(BeNil())
	g.Expect(s.Data).NotTo(BeEmpty())
	g.Expect(s.Labels).To(Equal(expectedLabels))

	k, err := kubeconfig.FromSecret(ctx, fakeClient, util.ObjectKey(cluster))
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(k).NotTo(BeEmpty())

	machineList := &clusterv1.MachineList{}
	g.Expect(fakeClient.List(ctx, machineList, client.InNamespace(metav1.NamespaceDefault))).To(Succeed())
	g.Expect(machineList.Items).To(HaveLen(1))

	machine := machineList.Items[0]
	g.Expect(machine.Name).To(HavePrefix(ocnecp.Name))
	// Newly cloned infra objects should have the infraref annotation.
	infraObj, err := external.Get(ctx, r.Client, &machine.Spec.InfrastructureRef, machine.Spec.InfrastructureRef.Namespace)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(infraObj.GetAnnotations()).To(HaveKeyWithValue(clusterv1.TemplateClonedFromNameAnnotation, genericMachineTemplate.GetName()))
	g.Expect(infraObj.GetAnnotations()).To(HaveKeyWithValue(clusterv1.TemplateClonedFromGroupKindAnnotation, genericMachineTemplate.GroupVersionKind().GroupKind().String()))
}

func TestOCNEControlPlaneReconciler_updateCoreDNS(t *testing.T) {
	// TODO: (wfernandes) This test could use some refactor love.

	cluster := newCluster(&types.NamespacedName{Name: "foo", Namespace: metav1.NamespaceDefault})
	ocnecp := &controlplanev1.OCNEControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cluster.Namespace,
			Name:      "foo",
		},
		Spec: controlplanev1.OCNEControlPlaneSpec{
			Replicas: nil,
			Version:  "v1.16.6",
			OCNEConfigSpec: bootstrapv1.OCNEConfigSpec{
				ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
					DNS: bootstrapv1.DNS{
						ImageMeta: bootstrapv1.ImageMeta{
							ImageRepository: "registry.k8s.io",
							ImageTag:        "1.7.2",
						},
					},
				},
			},
		},
	}
	depl := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "coredns",
			Namespace: metav1.NamespaceSystem,
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name: "coredns",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "coredns",
						Image: "registry.k8s.io/coredns:1.6.2",
					}},
					Volumes: []corev1.Volume{{
						Name: "config-volume",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: "coredns",
								},
								Items: []corev1.KeyToPath{{
									Key:  "Corefile",
									Path: "Corefile",
								}},
							},
						},
					}},
				},
			},
		},
	}
	originalCorefile := "original core file"
	corednsCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "coredns",
			Namespace: metav1.NamespaceSystem,
		},
		Data: map[string]string{
			"Corefile": originalCorefile,
		},
	}

	kubeadmCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kubeadm-config",
			Namespace: metav1.NamespaceSystem,
		},
		Data: map[string]string{
			"ClusterConfiguration": `apiServer:
dns:
  type: CoreDNS
imageRepository: registry.k8s.io
kind: ClusterConfiguration
kubernetesVersion: metav1.16.1`,
		},
	}

	t.Run("updates configmaps and deployments successfully", func(t *testing.T) {
		t.Skip("Updating the corefile, after updating controller runtime somehow makes this test fail in a conflict, needs investigation")

		g := NewWithT(t)
		objs := []client.Object{
			cluster.DeepCopy(),
			ocnecp.DeepCopy(),
			depl.DeepCopy(),
			corednsCM.DeepCopy(),
			kubeadmCM.DeepCopy(),
		}
		fakeClient := newFakeClient(objs...)
		log.SetLogger(klogr.New())

		workloadCluster := &fakeWorkloadCluster{
			Workload: &internal.Workload{
				Client: fakeClient,
				CoreDNSMigrator: &fakeMigrator{
					migratedCorefile: "new core file",
				},
			},
		}

		g.Expect(workloadCluster.UpdateCoreDNS(ctx, ocnecp, semver.MustParse("1.19.1"))).To(Succeed())

		var actualCoreDNSCM corev1.ConfigMap
		g.Expect(fakeClient.Get(ctx, client.ObjectKey{Name: "coredns", Namespace: metav1.NamespaceSystem}, &actualCoreDNSCM)).To(Succeed())
		g.Expect(actualCoreDNSCM.Data).To(HaveLen(2))
		g.Expect(actualCoreDNSCM.Data).To(HaveKeyWithValue("Corefile", "new core file"))
		g.Expect(actualCoreDNSCM.Data).To(HaveKeyWithValue("Corefile-backup", originalCorefile))

		var actualKubeadmConfig corev1.ConfigMap
		g.Expect(fakeClient.Get(ctx, client.ObjectKey{Name: "kubeadm-config", Namespace: metav1.NamespaceSystem}, &actualKubeadmConfig)).To(Succeed())
		g.Expect(actualKubeadmConfig.Data).To(HaveKey("ClusterConfiguration"))
		g.Expect(actualKubeadmConfig.Data["ClusterConfiguration"]).To(ContainSubstring("1.7.2"))

		expectedVolume := corev1.Volume{
			Name: "config-volume",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "coredns",
					},
					Items: []corev1.KeyToPath{{
						Key:  "Corefile",
						Path: "Corefile",
					}},
				},
			},
		}
		var actualCoreDNSDeployment appsv1.Deployment
		g.Expect(fakeClient.Get(ctx, client.ObjectKey{Name: "coredns", Namespace: metav1.NamespaceSystem}, &actualCoreDNSDeployment)).To(Succeed())
		g.Expect(actualCoreDNSDeployment.Spec.Template.Spec.Containers[0].Image).To(Equal("registry.k8s.io/coredns:1.7.2"))
		g.Expect(actualCoreDNSDeployment.Spec.Template.Spec.Volumes).To(ConsistOf(expectedVolume))
	})

	t.Run("returns no error when no ClusterConfiguration is specified", func(t *testing.T) {
		g := NewWithT(t)
		ocnecp := ocnecp.DeepCopy()
		ocnecp.Spec.OCNEConfigSpec.ClusterConfiguration = nil

		objs := []client.Object{
			cluster.DeepCopy(),
			ocnecp,
			depl.DeepCopy(),
			corednsCM.DeepCopy(),
			kubeadmCM.DeepCopy(),
		}

		fakeClient := newFakeClient(objs...)
		log.SetLogger(klogr.New())

		workloadCluster := fakeWorkloadCluster{
			Workload: &internal.Workload{
				Client: fakeClient,
				CoreDNSMigrator: &fakeMigrator{
					migratedCorefile: "new core file",
				},
			},
		}

		g.Expect(workloadCluster.UpdateCoreDNS(ctx, ocnecp, semver.MustParse("1.19.1"))).To(Succeed())
	})

	t.Run("should not return an error when there is no CoreDNS configmap", func(t *testing.T) {
		g := NewWithT(t)
		objs := []client.Object{
			cluster.DeepCopy(),
			ocnecp.DeepCopy(),
			depl.DeepCopy(),
			kubeadmCM.DeepCopy(),
		}

		fakeClient := newFakeClient(objs...)
		workloadCluster := fakeWorkloadCluster{
			Workload: &internal.Workload{
				Client: fakeClient,
				CoreDNSMigrator: &fakeMigrator{
					migratedCorefile: "new core file",
				},
			},
		}

		g.Expect(workloadCluster.UpdateCoreDNS(ctx, ocnecp, semver.MustParse("1.19.1"))).To(Succeed())
	})

	t.Run("should not return an error when there is no CoreDNS deployment", func(t *testing.T) {
		g := NewWithT(t)
		objs := []client.Object{
			cluster.DeepCopy(),
			ocnecp.DeepCopy(),
			corednsCM.DeepCopy(),
			kubeadmCM.DeepCopy(),
		}

		fakeClient := newFakeClient(objs...)
		log.SetLogger(klogr.New())

		workloadCluster := fakeWorkloadCluster{
			Workload: &internal.Workload{
				Client: fakeClient,
				CoreDNSMigrator: &fakeMigrator{
					migratedCorefile: "new core file",
				},
			},
		}

		g.Expect(workloadCluster.UpdateCoreDNS(ctx, ocnecp, semver.MustParse("1.19.1"))).To(Succeed())
	})

	t.Run("should not return an error when no DNS upgrade is requested", func(t *testing.T) {
		g := NewWithT(t)
		objs := []client.Object{
			cluster.DeepCopy(),
			corednsCM.DeepCopy(),
			kubeadmCM.DeepCopy(),
		}
		ocnecp := ocnecp.DeepCopy()
		ocnecp.Annotations = map[string]string{controlplanev1.SkipCoreDNSAnnotation: ""}

		depl := depl.DeepCopy()

		depl.Spec.Template.Spec.Containers[0].Image = "my-cool-image!!!!" // something very unlikely for getCoreDNSInfo to parse
		objs = append(objs, depl)

		fakeClient := newFakeClient(objs...)
		workloadCluster := fakeWorkloadCluster{
			Workload: &internal.Workload{
				Client: fakeClient,
			},
		}

		g.Expect(workloadCluster.UpdateCoreDNS(ctx, ocnecp, semver.MustParse("1.19.1"))).To(Succeed())

		var actualCoreDNSCM corev1.ConfigMap
		g.Expect(fakeClient.Get(ctx, client.ObjectKey{Name: "coredns", Namespace: metav1.NamespaceSystem}, &actualCoreDNSCM)).To(Succeed())
		g.Expect(actualCoreDNSCM.Data).To(Equal(corednsCM.Data))

		var actualKubeadmConfig corev1.ConfigMap
		g.Expect(fakeClient.Get(ctx, client.ObjectKey{Name: "kubeadm-config", Namespace: metav1.NamespaceSystem}, &actualKubeadmConfig)).To(Succeed())
		g.Expect(actualKubeadmConfig.Data).To(Equal(kubeadmCM.Data))

		var actualCoreDNSDeployment appsv1.Deployment
		g.Expect(fakeClient.Get(ctx, client.ObjectKey{Name: "coredns", Namespace: metav1.NamespaceSystem}, &actualCoreDNSDeployment)).To(Succeed())
		g.Expect(actualCoreDNSDeployment.Spec.Template.Spec.Containers[0].Image).ToNot(ContainSubstring("coredns"))
	})

	t.Run("returns error when unable to UpdateCoreDNS", func(t *testing.T) {
		g := NewWithT(t)
		objs := []client.Object{
			cluster.DeepCopy(),
			ocnecp.DeepCopy(),
			depl.DeepCopy(),
			corednsCM.DeepCopy(),
		}

		fakeClient := newFakeClient(objs...)
		log.SetLogger(klogr.New())

		workloadCluster := fakeWorkloadCluster{
			Workload: &internal.Workload{
				Client: fakeClient,
				CoreDNSMigrator: &fakeMigrator{
					migratedCorefile: "new core file",
				},
			},
		}

		g.Expect(workloadCluster.UpdateCoreDNS(ctx, ocnecp, semver.MustParse("1.19.1"))).ToNot(Succeed())
	})
}

func TestOCNEControlPlaneReconciler_reconcileDelete(t *testing.T) {
	t.Run("removes all control plane Machines", func(t *testing.T) {
		g := NewWithT(t)

		cluster, ocnecp, _ := createClusterWithControlPlane(metav1.NamespaceDefault)
		controllerutil.AddFinalizer(ocnecp, controlplanev1.OCNEControlPlaneFinalizer)
		initObjs := []client.Object{cluster.DeepCopy(), ocnecp.DeepCopy()}

		for i := 0; i < 3; i++ {
			m, _ := createMachineNodePair(fmt.Sprintf("test-%d", i), cluster, ocnecp, true)
			initObjs = append(initObjs, m)
		}

		fakeClient := newFakeClient(initObjs...)

		r := &OCNEControlPlaneReconciler{
			Client: fakeClient,
			managementCluster: &fakeManagementCluster{
				Management: &internal.Management{Client: fakeClient},
				Workload:   fakeWorkloadCluster{},
			},

			recorder: record.NewFakeRecorder(32),
		}

		result, err := r.reconcileDelete(ctx, cluster, ocnecp)
		g.Expect(result).To(Equal(ctrl.Result{RequeueAfter: deleteRequeueAfter}))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(ocnecp.Finalizers).To(ContainElement(controlplanev1.OCNEControlPlaneFinalizer))

		controlPlaneMachines := clusterv1.MachineList{}
		g.Expect(fakeClient.List(ctx, &controlPlaneMachines)).To(Succeed())
		g.Expect(controlPlaneMachines.Items).To(BeEmpty())

		result, err = r.reconcileDelete(ctx, cluster, ocnecp)
		g.Expect(result).To(Equal(ctrl.Result{}))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(ocnecp.Finalizers).To(BeEmpty())
	})

	t.Run("does not remove any control plane Machines if other Machines exist", func(t *testing.T) {
		g := NewWithT(t)

		cluster, ocnecp, _ := createClusterWithControlPlane(metav1.NamespaceDefault)
		controllerutil.AddFinalizer(ocnecp, controlplanev1.OCNEControlPlaneFinalizer)

		workerMachine := &clusterv1.Machine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "worker",
				Namespace: cluster.Namespace,
				Labels: map[string]string{
					clusterv1.ClusterLabelName: cluster.Name,
				},
			},
		}

		initObjs := []client.Object{cluster.DeepCopy(), ocnecp.DeepCopy(), workerMachine.DeepCopy()}

		for i := 0; i < 3; i++ {
			m, _ := createMachineNodePair(fmt.Sprintf("test-%d", i), cluster, ocnecp, true)
			initObjs = append(initObjs, m)
		}

		fakeClient := newFakeClient(initObjs...)

		r := &OCNEControlPlaneReconciler{
			Client: fakeClient,
			managementCluster: &fakeManagementCluster{
				Management: &internal.Management{Client: fakeClient},
				Workload:   fakeWorkloadCluster{},
			},
			recorder: record.NewFakeRecorder(32),
		}

		result, err := r.reconcileDelete(ctx, cluster, ocnecp)
		g.Expect(result).To(Equal(ctrl.Result{RequeueAfter: deleteRequeueAfter}))
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(ocnecp.Finalizers).To(ContainElement(controlplanev1.OCNEControlPlaneFinalizer))

		controlPlaneMachines := clusterv1.MachineList{}
		labels := map[string]string{
			clusterv1.MachineControlPlaneLabelName: "",
		}
		g.Expect(fakeClient.List(ctx, &controlPlaneMachines, client.MatchingLabels(labels))).To(Succeed())
		g.Expect(controlPlaneMachines.Items).To(HaveLen(3))
	})

	t.Run("does not remove any control plane Machines if MachinePools exist", func(t *testing.T) {
		_ = feature.MutableGates.Set("MachinePool=true")
		g := NewWithT(t)

		cluster, ocnecp, _ := createClusterWithControlPlane(metav1.NamespaceDefault)
		controllerutil.AddFinalizer(ocnecp, controlplanev1.OCNEControlPlaneFinalizer)

		workerMachinePool := &expv1.MachinePool{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "worker",
				Namespace: cluster.Namespace,
				Labels: map[string]string{
					clusterv1.ClusterLabelName: cluster.Name,
				},
			},
		}

		initObjs := []client.Object{cluster.DeepCopy(), ocnecp.DeepCopy(), workerMachinePool.DeepCopy()}

		for i := 0; i < 3; i++ {
			m, _ := createMachineNodePair(fmt.Sprintf("test-%d", i), cluster, ocnecp, true)
			initObjs = append(initObjs, m)
		}

		fakeClient := newFakeClient(initObjs...)

		r := &OCNEControlPlaneReconciler{
			Client: fakeClient,
			managementCluster: &fakeManagementCluster{
				Management: &internal.Management{Client: fakeClient},
				Workload:   fakeWorkloadCluster{},
			},
			recorder: record.NewFakeRecorder(32),
		}

		result, err := r.reconcileDelete(ctx, cluster, ocnecp)
		g.Expect(result).To(Equal(ctrl.Result{RequeueAfter: deleteRequeueAfter}))
		g.Expect(err).ToNot(HaveOccurred())

		g.Expect(ocnecp.Finalizers).To(ContainElement(controlplanev1.OCNEControlPlaneFinalizer))

		controlPlaneMachines := clusterv1.MachineList{}
		labels := map[string]string{
			clusterv1.MachineControlPlaneLabelName: "",
		}
		g.Expect(fakeClient.List(ctx, &controlPlaneMachines, client.MatchingLabels(labels))).To(Succeed())
		g.Expect(controlPlaneMachines.Items).To(HaveLen(3))
	})

	t.Run("removes the finalizer if no control plane Machines exist", func(t *testing.T) {
		g := NewWithT(t)

		cluster, ocnecp, _ := createClusterWithControlPlane(metav1.NamespaceDefault)
		controllerutil.AddFinalizer(ocnecp, controlplanev1.OCNEControlPlaneFinalizer)

		fakeClient := newFakeClient(cluster.DeepCopy(), ocnecp.DeepCopy())

		r := &OCNEControlPlaneReconciler{
			Client: fakeClient,
			managementCluster: &fakeManagementCluster{
				Management: &internal.Management{Client: fakeClient},
				Workload:   fakeWorkloadCluster{},
			},
			recorder: record.NewFakeRecorder(32),
		}

		result, err := r.reconcileDelete(ctx, cluster, ocnecp)
		g.Expect(result).To(Equal(ctrl.Result{}))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(ocnecp.Finalizers).To(BeEmpty())
	})
}

// test utils.

func newFakeClient(initObjs ...client.Object) client.Client {
	return &fakeClient{
		startTime: time.Now(),
		Client:    fake.NewClientBuilder().WithObjects(initObjs...).Build(),
	}
}

type fakeClient struct {
	startTime time.Time
	mux       sync.Mutex
	client.Client
}

type fakeClientI interface {
	SetCreationTimestamp(timestamp metav1.Time)
}

// controller-runtime's fake client doesn't set a CreationTimestamp
// this sets one that increments by a minute for each object created.
func (c *fakeClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if f, ok := obj.(fakeClientI); ok {
		c.mux.Lock()
		c.startTime = c.startTime.Add(time.Minute)
		f.SetCreationTimestamp(metav1.NewTime(c.startTime))
		c.mux.Unlock()
	}
	return c.Client.Create(ctx, obj, opts...)
}

func createClusterWithControlPlane(namespace string) (*clusterv1.Cluster, *controlplanev1.OCNEControlPlane, *unstructured.Unstructured) {
	ocnecpName := fmt.Sprintf("ocnecp-foo-%s", util.RandomString(6))

	cluster := newCluster(&types.NamespacedName{Name: ocnecpName, Namespace: namespace})
	cluster.Spec = clusterv1.ClusterSpec{
		ControlPlaneRef: &corev1.ObjectReference{
			Kind:       "OCNEControlPlane",
			Namespace:  namespace,
			Name:       ocnecpName,
			APIVersion: controlplanev1.GroupVersion.String(),
		},
	}

	ocnecp := &controlplanev1.OCNEControlPlane{
		TypeMeta: metav1.TypeMeta{
			APIVersion: controlplanev1.GroupVersion.String(),
			Kind:       "OCNEControlPlane",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ocnecpName,
			Namespace: namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind:       "Cluster",
					APIVersion: clusterv1.GroupVersion.String(),
					Name:       ocnecpName,
					UID:        "1",
				},
			},
		},
		Spec: controlplanev1.OCNEControlPlaneSpec{
			MachineTemplate: controlplanev1.OCNEControlPlaneMachineTemplate{
				InfrastructureRef: corev1.ObjectReference{
					Kind:       "GenericMachineTemplate",
					Namespace:  namespace,
					Name:       "infra-foo",
					APIVersion: "generic.io/v1",
				},
			},
			Replicas: pointer.Int32(int32(3)),
			Version:  "v1.16.6",
			RolloutStrategy: &controlplanev1.RolloutStrategy{
				Type: "RollingUpdate",
				RollingUpdate: &controlplanev1.RollingUpdate{
					MaxSurge: &intstr.IntOrString{
						IntVal: 1,
					},
				},
			},
		},
	}

	genericMachineTemplate := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"kind":       "GenericMachineTemplate",
			"apiVersion": "generic.io/v1",
			"metadata": map[string]interface{}{
				"name":      "infra-foo",
				"namespace": namespace,
				"ownerReferences": []interface{}{
					map[string]interface{}{
						"apiVersion": clusterv1.GroupVersion.String(),
						"kind":       "Cluster",
						"name":       ocnecpName,
					},
				},
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{},
				},
			},
		},
	}
	return cluster, ocnecp, genericMachineTemplate
}

func setKCPHealthy(ocnecp *controlplanev1.OCNEControlPlane) {
	conditions.MarkTrue(ocnecp, controlplanev1.ControlPlaneComponentsHealthyCondition)
	conditions.MarkTrue(ocnecp, controlplanev1.EtcdClusterHealthyCondition)
}

func createMachineNodePair(name string, cluster *clusterv1.Cluster, ocnecp *controlplanev1.OCNEControlPlane, ready bool) (*clusterv1.Machine, *corev1.Node) {
	machine := &clusterv1.Machine{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Machine",
			APIVersion: clusterv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: cluster.Namespace,
			Name:      name,
			Labels:    internal.ControlPlaneMachineLabelsForCluster(ocnecp, cluster.Name),
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(ocnecp, controlplanev1.GroupVersion.WithKind("OCNEControlPlane")),
			},
		},
		Spec: clusterv1.MachineSpec{
			ClusterName: cluster.Name,
			InfrastructureRef: corev1.ObjectReference{
				Kind:       builder.GenericInfrastructureMachineCRD.Kind,
				APIVersion: builder.GenericInfrastructureMachineCRD.APIVersion,
				Name:       builder.GenericInfrastructureMachineCRD.Name,
				Namespace:  builder.GenericInfrastructureMachineCRD.Namespace,
			},
		},
		Status: clusterv1.MachineStatus{
			NodeRef: &corev1.ObjectReference{
				Kind:       "Node",
				APIVersion: corev1.SchemeGroupVersion.String(),
				Name:       name,
			},
		},
	}
	machine.Default()

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: map[string]string{"node-role.kubernetes.io/control-plane": ""},
		},
	}

	if ready {
		node.Spec.ProviderID = fmt.Sprintf("test://%s", machine.GetName())
		node.Status.Conditions = []corev1.NodeCondition{
			{
				Type:   corev1.NodeReady,
				Status: corev1.ConditionTrue,
			},
		}
	}
	return machine, node
}

func setMachineHealthy(m *clusterv1.Machine) {
	conditions.MarkTrue(m, controlplanev1.MachineAPIServerPodHealthyCondition)
	conditions.MarkTrue(m, controlplanev1.MachineControllerManagerPodHealthyCondition)
	conditions.MarkTrue(m, controlplanev1.MachineSchedulerPodHealthyCondition)
	conditions.MarkTrue(m, controlplanev1.MachineEtcdPodHealthyCondition)
	conditions.MarkTrue(m, controlplanev1.MachineEtcdMemberHealthyCondition)
}

// newCluster return a CAPI cluster object.
func newCluster(namespacedName *types.NamespacedName) *clusterv1.Cluster {
	return &clusterv1.Cluster{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Cluster",
			APIVersion: clusterv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespacedName.Namespace,
			Name:      namespacedName.Name,
		},
	}
}

func getTestCACert(key *rsa.PrivateKey) (*x509.Certificate, error) {
	cfg := certs.Config{
		CommonName: "kubernetes",
	}

	now := time.Now().UTC()

	tmpl := x509.Certificate{
		SerialNumber: new(big.Int).SetInt64(0),
		Subject: pkix.Name{
			CommonName:   cfg.CommonName,
			Organization: cfg.Organization,
		},
		NotBefore:             now.Add(time.Minute * -5),
		NotAfter:              now.Add(time.Hour * 24), // 1 day
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		MaxPathLenZero:        true,
		BasicConstraintsValid: true,
		MaxPathLen:            0,
		IsCA:                  true,
	}

	b, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, key.Public(), key)
	if err != nil {
		return nil, err
	}

	c, err := x509.ParseCertificate(b)
	return c, err
}
