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
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	bootstrapv1 "github.com/verrazzano/cluster-api-provider-ocne/bootstrap/ocne/api/v1alpha1"
	controlplanev1 "github.com/verrazzano/cluster-api-provider-ocne/controlplane/ocne/api/v1alpha1"
	"github.com/verrazzano/cluster-api-provider-ocne/controlplane/ocne/internal"
	"github.com/verrazzano/cluster-api-provider-ocne/util/collections"
	"github.com/verrazzano/cluster-api-provider-ocne/util/conditions"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

func TestOCNEControlPlaneReconciler_initializeControlPlane(t *testing.T) {
	g := NewWithT(t)

	cluster, ocnecp, genericMachineTemplate := createClusterWithControlPlane(metav1.NamespaceDefault)

	fakeClient := newFakeClient(cluster.DeepCopy(), ocnecp.DeepCopy(), genericMachineTemplate.DeepCopy())

	r := &OCNEControlPlaneReconciler{
		Client:   fakeClient,
		recorder: record.NewFakeRecorder(32),
		managementClusterUncached: &fakeManagementCluster{
			Management: &internal.Management{Client: fakeClient},
			Workload:   fakeWorkloadCluster{},
		},
	}
	controlPlane := &internal.ControlPlane{
		Cluster: cluster,
		KCP:     ocnecp,
	}

	result, err := r.initializeControlPlane(ctx, cluster, ocnecp, controlPlane)
	g.Expect(result).To(Equal(ctrl.Result{Requeue: true}))
	g.Expect(err).NotTo(HaveOccurred())

	machineList := &clusterv1.MachineList{}
	g.Expect(fakeClient.List(ctx, machineList, client.InNamespace(cluster.Namespace))).To(Succeed())
	g.Expect(machineList.Items).To(HaveLen(1))

	res, err := collections.GetFilteredMachinesForCluster(ctx, fakeClient, cluster, collections.OwnedMachines(ocnecp))
	g.Expect(res).To(HaveLen(1))
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(machineList.Items[0].Namespace).To(Equal(cluster.Namespace))
	g.Expect(machineList.Items[0].Name).To(HavePrefix(ocnecp.Name))

	g.Expect(machineList.Items[0].Spec.InfrastructureRef.Namespace).To(Equal(cluster.Namespace))
	g.Expect(machineList.Items[0].Spec.InfrastructureRef.Name).To(HavePrefix(genericMachineTemplate.GetName()))
	g.Expect(machineList.Items[0].Spec.InfrastructureRef.APIVersion).To(Equal(genericMachineTemplate.GetAPIVersion()))
	g.Expect(machineList.Items[0].Spec.InfrastructureRef.Kind).To(Equal("GenericMachine"))

	g.Expect(machineList.Items[0].Spec.Bootstrap.ConfigRef.Namespace).To(Equal(cluster.Namespace))
	g.Expect(machineList.Items[0].Spec.Bootstrap.ConfigRef.Name).To(HavePrefix(ocnecp.Name))
	g.Expect(machineList.Items[0].Spec.Bootstrap.ConfigRef.APIVersion).To(Equal(bootstrapv1.GroupVersion.String()))
	g.Expect(machineList.Items[0].Spec.Bootstrap.ConfigRef.Kind).To(Equal("OCNEConfig"))
}

func TestOCNEControlPlaneReconciler_scaleUpControlPlane(t *testing.T) {
	t.Run("creates a control plane Machine if preflight checks pass", func(t *testing.T) {
		g := NewWithT(t)

		cluster, ocnecp, genericMachineTemplate := createClusterWithControlPlane(metav1.NamespaceDefault)
		setKCPHealthy(ocnecp)
		initObjs := []client.Object{cluster.DeepCopy(), ocnecp.DeepCopy(), genericMachineTemplate.DeepCopy()}

		fmc := &fakeManagementCluster{
			Machines: collections.New(),
			Workload: fakeWorkloadCluster{},
		}

		for i := 0; i < 2; i++ {
			m, _ := createMachineNodePair(fmt.Sprintf("test-%d", i), cluster, ocnecp, true)
			setMachineHealthy(m)
			fmc.Machines.Insert(m)
			initObjs = append(initObjs, m.DeepCopy())
		}

		fakeClient := newFakeClient(initObjs...)

		r := &OCNEControlPlaneReconciler{
			Client:                    fakeClient,
			managementCluster:         fmc,
			managementClusterUncached: fmc,
			recorder:                  record.NewFakeRecorder(32),
		}
		controlPlane := &internal.ControlPlane{
			KCP:      ocnecp,
			Cluster:  cluster,
			Machines: fmc.Machines,
		}

		result, err := r.scaleUpControlPlane(ctx, cluster, ocnecp, controlPlane)
		g.Expect(result).To(Equal(ctrl.Result{Requeue: true}))
		g.Expect(err).ToNot(HaveOccurred())

		controlPlaneMachines := clusterv1.MachineList{}
		g.Expect(fakeClient.List(ctx, &controlPlaneMachines)).To(Succeed())
		g.Expect(controlPlaneMachines.Items).To(HaveLen(3))
	})
	t.Run("does not create a control plane Machine if preflight checks fail", func(t *testing.T) {
		cluster, ocnecp, genericMachineTemplate := createClusterWithControlPlane(metav1.NamespaceDefault)
		initObjs := []client.Object{fakeGenericMachineTemplateCRD, cluster.DeepCopy(), ocnecp.DeepCopy(), genericMachineTemplate.DeepCopy()}
		cluster.Spec.ControlPlaneEndpoint.Host = "nodomain.example.com"
		cluster.Spec.ControlPlaneEndpoint.Port = 6443
		cluster.Status.InfrastructureReady = true

		beforeMachines := collections.New()
		for i := 0; i < 2; i++ {
			m, _ := createMachineNodePair(fmt.Sprintf("test-%d", i), cluster.DeepCopy(), ocnecp.DeepCopy(), true)
			beforeMachines.Insert(m)
			initObjs = append(initObjs, m.DeepCopy())
		}

		g := NewWithT(t)

		fakeClient := newFakeClient(initObjs...)
		fmc := &fakeManagementCluster{
			Machines: beforeMachines.DeepCopy(),
			Workload: fakeWorkloadCluster{},
		}

		r := &OCNEControlPlaneReconciler{
			Client:                    fakeClient,
			APIReader:                 fakeClient,
			managementCluster:         fmc,
			managementClusterUncached: fmc,
			recorder:                  record.NewFakeRecorder(32),
		}

		result, err := r.reconcile(context.Background(), cluster, ocnecp)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result).To(Equal(ctrl.Result{RequeueAfter: preflightFailedRequeueAfter}))

		// scaleUpControlPlane is never called due to health check failure and new machine is not created to scale up.
		controlPlaneMachines := &clusterv1.MachineList{}
		g.Expect(fakeClient.List(context.Background(), controlPlaneMachines)).To(Succeed())
		g.Expect(controlPlaneMachines.Items).To(HaveLen(len(beforeMachines)))

		endMachines := collections.FromMachineList(controlPlaneMachines)
		for _, m := range endMachines {
			bm, ok := beforeMachines[m.Name]
			bm.SetResourceVersion("999")
			g.Expect(ok).To(BeTrue())
			g.Expect(m).To(Equal(bm))
		}
	})
}

func TestOCNEControlPlaneReconciler_scaleDownControlPlane_NoError(t *testing.T) {
	t.Run("deletes control plane Machine if preflight checks pass", func(t *testing.T) {
		g := NewWithT(t)

		machines := map[string]*clusterv1.Machine{
			"one": machine("one"),
		}
		setMachineHealthy(machines["one"])
		fakeClient := newFakeClient(machines["one"])

		r := &OCNEControlPlaneReconciler{
			recorder: record.NewFakeRecorder(32),
			Client:   fakeClient,
			managementCluster: &fakeManagementCluster{
				Workload: fakeWorkloadCluster{},
			},
		}

		cluster := &clusterv1.Cluster{}
		ocnecp := &controlplanev1.OCNEControlPlane{
			Spec: controlplanev1.OCNEControlPlaneSpec{
				Version: "v1.19.1",
			},
		}
		setKCPHealthy(ocnecp)
		controlPlane := &internal.ControlPlane{
			KCP:      ocnecp,
			Cluster:  cluster,
			Machines: machines,
		}

		result, err := r.scaleDownControlPlane(context.Background(), cluster, ocnecp, controlPlane, controlPlane.Machines)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result).To(Equal(ctrl.Result{Requeue: true}))

		controlPlaneMachines := clusterv1.MachineList{}
		g.Expect(fakeClient.List(context.Background(), &controlPlaneMachines)).To(Succeed())
		g.Expect(controlPlaneMachines.Items).To(HaveLen(0))
	})
	t.Run("deletes the oldest control plane Machine even if preflight checks fails", func(t *testing.T) {
		g := NewWithT(t)

		machines := map[string]*clusterv1.Machine{
			"one":   machine("one", withTimestamp(time.Now().Add(-1*time.Minute))),
			"two":   machine("two", withTimestamp(time.Now())),
			"three": machine("three", withTimestamp(time.Now())),
		}
		setMachineHealthy(machines["two"])
		setMachineHealthy(machines["three"])
		fakeClient := newFakeClient(machines["one"], machines["two"], machines["three"])

		r := &OCNEControlPlaneReconciler{
			recorder: record.NewFakeRecorder(32),
			Client:   fakeClient,
			managementCluster: &fakeManagementCluster{
				Workload: fakeWorkloadCluster{},
			},
		}

		cluster := &clusterv1.Cluster{}
		ocnecp := &controlplanev1.OCNEControlPlane{
			Spec: controlplanev1.OCNEControlPlaneSpec{
				Version: "v1.19.1",
			},
		}
		controlPlane := &internal.ControlPlane{
			KCP:      ocnecp,
			Cluster:  cluster,
			Machines: machines,
		}

		result, err := r.scaleDownControlPlane(context.Background(), cluster, ocnecp, controlPlane, controlPlane.Machines)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result).To(Equal(ctrl.Result{Requeue: true}))

		controlPlaneMachines := clusterv1.MachineList{}
		g.Expect(fakeClient.List(context.Background(), &controlPlaneMachines)).To(Succeed())
		g.Expect(controlPlaneMachines.Items).To(HaveLen(2))
	})

	t.Run("does not scale down if preflight checks fail on any machine other than the one being deleted", func(t *testing.T) {
		g := NewWithT(t)

		machines := map[string]*clusterv1.Machine{
			"one":   machine("one", withTimestamp(time.Now().Add(-1*time.Minute))),
			"two":   machine("two", withTimestamp(time.Now())),
			"three": machine("three", withTimestamp(time.Now())),
		}
		setMachineHealthy(machines["three"])
		fakeClient := newFakeClient(machines["one"], machines["two"], machines["three"])

		r := &OCNEControlPlaneReconciler{
			recorder: record.NewFakeRecorder(32),
			Client:   fakeClient,
			managementCluster: &fakeManagementCluster{
				Workload: fakeWorkloadCluster{},
			},
		}

		cluster := &clusterv1.Cluster{}
		ocnecp := &controlplanev1.OCNEControlPlane{}
		controlPlane := &internal.ControlPlane{
			KCP:      ocnecp,
			Cluster:  cluster,
			Machines: machines,
		}

		result, err := r.scaleDownControlPlane(context.Background(), cluster, ocnecp, controlPlane, controlPlane.Machines)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result).To(Equal(ctrl.Result{RequeueAfter: preflightFailedRequeueAfter}))

		controlPlaneMachines := clusterv1.MachineList{}
		g.Expect(fakeClient.List(context.Background(), &controlPlaneMachines)).To(Succeed())
		g.Expect(controlPlaneMachines.Items).To(HaveLen(3))
	})
}

func TestSelectMachineForScaleDown(t *testing.T) {
	ocnecp := controlplanev1.OCNEControlPlane{
		Spec: controlplanev1.OCNEControlPlaneSpec{},
	}
	startDate := time.Date(2000, 1, 1, 1, 0, 0, 0, time.UTC)
	m1 := machine("machine-1", withFailureDomain("one"), withTimestamp(startDate.Add(time.Hour)))
	m2 := machine("machine-2", withFailureDomain("one"), withTimestamp(startDate.Add(-3*time.Hour)))
	m3 := machine("machine-3", withFailureDomain("one"), withTimestamp(startDate.Add(-4*time.Hour)))
	m4 := machine("machine-4", withFailureDomain("two"), withTimestamp(startDate.Add(-time.Hour)))
	m5 := machine("machine-5", withFailureDomain("two"), withTimestamp(startDate.Add(-2*time.Hour)))
	m6 := machine("machine-6", withFailureDomain("two"), withTimestamp(startDate.Add(-7*time.Hour)))
	m7 := machine("machine-7", withFailureDomain("two"), withTimestamp(startDate.Add(-5*time.Hour)), withAnnotation("cluster.x-k8s.io/delete-machine"))
	m8 := machine("machine-8", withFailureDomain("two"), withTimestamp(startDate.Add(-6*time.Hour)), withAnnotation("cluster.x-k8s.io/delete-machine"))

	mc3 := collections.FromMachines(m1, m2, m3, m4, m5)
	mc6 := collections.FromMachines(m6, m7, m8)
	fd := clusterv1.FailureDomains{
		"one": failureDomain(true),
		"two": failureDomain(true),
	}

	needsUpgradeControlPlane := &internal.ControlPlane{
		KCP:      &ocnecp,
		Cluster:  &clusterv1.Cluster{Status: clusterv1.ClusterStatus{FailureDomains: fd}},
		Machines: mc3,
	}
	upToDateControlPlane := &internal.ControlPlane{
		KCP:     &ocnecp,
		Cluster: &clusterv1.Cluster{Status: clusterv1.ClusterStatus{FailureDomains: fd}},
		Machines: mc3.Filter(func(m *clusterv1.Machine) bool {
			return m.Name != "machine-5"
		}),
	}
	annotatedControlPlane := &internal.ControlPlane{
		KCP:      &ocnecp,
		Cluster:  &clusterv1.Cluster{Status: clusterv1.ClusterStatus{FailureDomains: fd}},
		Machines: mc6,
	}

	testCases := []struct {
		name             string
		cp               *internal.ControlPlane
		outDatedMachines collections.Machines
		expectErr        bool
		expectedMachine  clusterv1.Machine
	}{
		{
			name:             "when there are machines needing upgrade, it returns the oldest machine in the failure domain with the most machines needing upgrade",
			cp:               needsUpgradeControlPlane,
			outDatedMachines: collections.FromMachines(m5),
			expectErr:        false,
			expectedMachine:  clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "machine-5"}},
		},
		{
			name:             "when there are no outdated machines, it returns the oldest machine in the largest failure domain",
			cp:               upToDateControlPlane,
			outDatedMachines: collections.New(),
			expectErr:        false,
			expectedMachine:  clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "machine-3"}},
		},
		{
			name:             "when there is a single machine marked with delete annotation key in machine collection, it returns only that marked machine",
			cp:               annotatedControlPlane,
			outDatedMachines: collections.FromMachines(m6, m7),
			expectErr:        false,
			expectedMachine:  clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "machine-7"}},
		},
		{
			name:             "when there are machines marked with delete annotation key in machine collection, it returns the oldest marked machine first",
			cp:               annotatedControlPlane,
			outDatedMachines: collections.FromMachines(m7, m8),
			expectErr:        false,
			expectedMachine:  clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "machine-8"}},
		},
		{
			name:             "when there are annotated machines which are part of the annotatedControlPlane but not in outdatedMachines, it returns the oldest marked machine first",
			cp:               annotatedControlPlane,
			outDatedMachines: collections.New(),
			expectErr:        false,
			expectedMachine:  clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "machine-8"}},
		},
		{
			name:             "when there are machines needing upgrade, it returns the oldest machine in the failure domain with the most machines needing upgrade",
			cp:               needsUpgradeControlPlane,
			outDatedMachines: collections.FromMachines(m7, m3),
			expectErr:        false,
			expectedMachine:  clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "machine-7"}},
		},
		{
			name:             "when there is an up to date machine with delete annotation, while there are any outdated machines without annotatio that still exist, it returns oldest marked machine first",
			cp:               upToDateControlPlane,
			outDatedMachines: collections.FromMachines(m5, m3, m8, m7, m6, m1, m2),
			expectErr:        false,
			expectedMachine:  clusterv1.Machine{ObjectMeta: metav1.ObjectMeta{Name: "machine-8"}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			selectedMachine, err := selectMachineForScaleDown(tc.cp, tc.outDatedMachines)

			if tc.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(tc.expectedMachine.Name).To(Equal(selectedMachine.Name))
		})
	}
}

func TestPreflightChecks(t *testing.T) {
	testCases := []struct {
		name         string
		ocnecp       *controlplanev1.OCNEControlPlane
		machines     []*clusterv1.Machine
		expectResult ctrl.Result
	}{
		{
			name:         "control plane without machines (not initialized) should pass",
			ocnecp:       &controlplanev1.OCNEControlPlane{},
			expectResult: ctrl.Result{},
		},
		{
			name:   "control plane with a deleting machine should requeue",
			ocnecp: &controlplanev1.OCNEControlPlane{},
			machines: []*clusterv1.Machine{
				{
					ObjectMeta: metav1.ObjectMeta{
						DeletionTimestamp: &metav1.Time{Time: time.Now()},
					},
				},
			},
			expectResult: ctrl.Result{RequeueAfter: deleteRequeueAfter},
		},
		{
			name:   "control plane with an unhealthy machine condition should requeue",
			ocnecp: &controlplanev1.OCNEControlPlane{},
			machines: []*clusterv1.Machine{
				{
					Status: clusterv1.MachineStatus{
						Conditions: clusterv1.Conditions{
							*conditions.FalseCondition(controlplanev1.MachineAPIServerPodHealthyCondition, "fooReason", clusterv1.ConditionSeverityError, ""),
							*conditions.TrueCondition(controlplanev1.MachineControllerManagerPodHealthyCondition),
							*conditions.TrueCondition(controlplanev1.MachineSchedulerPodHealthyCondition),
							*conditions.TrueCondition(controlplanev1.MachineEtcdPodHealthyCondition),
							*conditions.TrueCondition(controlplanev1.MachineEtcdMemberHealthyCondition),
						},
					},
				},
			},
			expectResult: ctrl.Result{RequeueAfter: preflightFailedRequeueAfter},
		},
		{
			name: "control plane with an healthy machine and an healthy ocnecp condition should pass",
			ocnecp: &controlplanev1.OCNEControlPlane{
				Status: controlplanev1.OCNEControlPlaneStatus{
					Conditions: clusterv1.Conditions{
						*conditions.TrueCondition(controlplanev1.ControlPlaneComponentsHealthyCondition),
						*conditions.TrueCondition(controlplanev1.EtcdClusterHealthyCondition),
					},
				},
			},
			machines: []*clusterv1.Machine{
				{
					Status: clusterv1.MachineStatus{
						Conditions: clusterv1.Conditions{
							*conditions.TrueCondition(controlplanev1.MachineAPIServerPodHealthyCondition),
							*conditions.TrueCondition(controlplanev1.MachineControllerManagerPodHealthyCondition),
							*conditions.TrueCondition(controlplanev1.MachineSchedulerPodHealthyCondition),
							*conditions.TrueCondition(controlplanev1.MachineEtcdPodHealthyCondition),
							*conditions.TrueCondition(controlplanev1.MachineEtcdMemberHealthyCondition),
						},
					},
				},
			},
			expectResult: ctrl.Result{},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			r := &OCNEControlPlaneReconciler{
				recorder: record.NewFakeRecorder(32),
			}
			controlPlane := &internal.ControlPlane{
				Cluster:  &clusterv1.Cluster{},
				KCP:      tt.ocnecp,
				Machines: collections.FromMachines(tt.machines...),
			}
			result, err := r.preflightChecks(context.TODO(), controlPlane)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(result).To(Equal(tt.expectResult))
		})
	}
}

func TestPreflightCheckCondition(t *testing.T) {
	condition := clusterv1.ConditionType("fooCondition")
	testCases := []struct {
		name      string
		machine   *clusterv1.Machine
		expectErr bool
	}{
		{
			name:      "missing condition should return error",
			machine:   &clusterv1.Machine{},
			expectErr: true,
		},
		{
			name: "false condition should return error",
			machine: &clusterv1.Machine{
				Status: clusterv1.MachineStatus{
					Conditions: clusterv1.Conditions{
						*conditions.FalseCondition(condition, "fooReason", clusterv1.ConditionSeverityError, ""),
					},
				},
			},
			expectErr: true,
		},
		{
			name: "unknown condition should return error",
			machine: &clusterv1.Machine{
				Status: clusterv1.MachineStatus{
					Conditions: clusterv1.Conditions{
						*conditions.UnknownCondition(condition, "fooReason", ""),
					},
				},
			},
			expectErr: true,
		},
		{
			name: "true condition should not return error",
			machine: &clusterv1.Machine{
				Status: clusterv1.MachineStatus{
					Conditions: clusterv1.Conditions{
						*conditions.TrueCondition(condition),
					},
				},
			},
			expectErr: false,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			err := preflightCheckCondition("machine", tt.machine, condition)

			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).NotTo(HaveOccurred())
		})
	}
}

func failureDomain(controlPlane bool) clusterv1.FailureDomainSpec {
	return clusterv1.FailureDomainSpec{
		ControlPlane: controlPlane,
	}
}

func withFailureDomain(fd string) machineOpt {
	return func(m *clusterv1.Machine) {
		m.Spec.FailureDomain = &fd
	}
}

func withAnnotation(annotation string) machineOpt {
	return func(m *clusterv1.Machine) {
		m.ObjectMeta.Annotations = map[string]string{annotation: ""}
	}
}

func withTimestamp(t time.Time) machineOpt {
	return func(m *clusterv1.Machine) {
		m.CreationTimestamp = metav1.NewTime(t)
	}
}
