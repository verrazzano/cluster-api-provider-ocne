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
	"testing"

	. "github.com/onsi/gomega"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	bootstrapv1 "github.com/verrazzano/cluster-api-provider-ocne/bootstrap/ocne/api/v1beta1"
	"github.com/verrazzano/cluster-api-provider-ocne/internal/test/builder"
)

func TestKubeadmConfigReconciler(t *testing.T) {
	t.Run("Reconcile a OcneConfig", func(t *testing.T) {
		t.Run("should wait until infrastructure is ready", func(t *testing.T) {
			g := NewWithT(t)

			ns, err := env.CreateNamespace(ctx, "test-kubeadm-config-reconciler")
			g.Expect(err).To(BeNil())

			cluster := builder.Cluster(ns.Name, "cluster1").Build()
			g.Expect(env.Create(ctx, cluster)).To(Succeed())
			machine := newWorkerMachineForCluster(cluster)
			g.Expect(env.Create(ctx, machine)).To(Succeed())

			config := newKubeadmConfig(ns.Name, "my-machine-config")
			g.Expect(env.Create(ctx, config)).To(Succeed())
			defer func(do ...client.Object) {
				g.Expect(env.Cleanup(ctx, do...)).To(Succeed())
			}(cluster, machine, config, ns)

			reconciler := OcneConfigReconciler{
				Client: env,
			}
			t.Log("Calling reconcile should requeue")
			result, err := reconciler.Reconcile(ctx, ctrl.Request{
				NamespacedName: client.ObjectKey{
					Namespace: ns.Name,
					Name:      "my-machine-config",
				},
			})
			g.Expect(err).To(Succeed())
			g.Expect(result.Requeue).To(BeFalse())
		})
	})
}

// getKubeadmConfig returns a OcneConfig object from the cluster.
func getKubeadmConfig(c client.Client, name, namespace string) (*bootstrapv1.OcneConfig, error) {
	controlplaneConfigKey := client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}
	config := &bootstrapv1.OcneConfig{}
	err := c.Get(ctx, controlplaneConfigKey, config)
	return config, err
}
