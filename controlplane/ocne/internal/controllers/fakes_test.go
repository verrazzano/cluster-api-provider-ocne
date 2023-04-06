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
	"time"

	"github.com/blang/semver"
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	bootstrapv1 "github.com/verrazzano/cluster-api-provider-ocne/bootstrap/ocne/api/v1beta1"
	"github.com/verrazzano/cluster-api-provider-ocne/controlplane/ocne/internal"
	"github.com/verrazzano/cluster-api-provider-ocne/util/collections"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	expv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
)

type fakeManagementCluster struct {
	// TODO: once all client interactions are moved to the Management cluster this can go away
	Management   *internal.Management
	Machines     collections.Machines
	MachinePools *expv1.MachinePoolList
	Workload     fakeWorkloadCluster
	Reader       client.Reader
}

func (f *fakeManagementCluster) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	return f.Reader.Get(ctx, key, obj, opts...)
}

func (f *fakeManagementCluster) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return f.Reader.List(ctx, list, opts...)
}

func (f *fakeManagementCluster) GetWorkloadCluster(_ context.Context, _ client.ObjectKey) (internal.WorkloadCluster, error) {
	return f.Workload, nil
}

func (f *fakeManagementCluster) GetMachinesForCluster(c context.Context, cluster *clusterv1.Cluster, filters ...collections.Func) (collections.Machines, error) {
	if f.Management != nil {
		return f.Management.GetMachinesForCluster(c, cluster, filters...)
	}
	return f.Machines, nil
}

func (f *fakeManagementCluster) GetMachinePoolsForCluster(c context.Context, cluster *clusterv1.Cluster) (*expv1.MachinePoolList, error) {
	if f.Management != nil {
		return f.Management.GetMachinePoolsForCluster(c, cluster)
	}
	return f.MachinePools, nil
}

type fakeWorkloadCluster struct {
	*internal.Workload
	Status                     internal.ClusterStatus
	EtcdMembersResult          []string
	APIServerCertificateExpiry *time.Time
}

func (f fakeWorkloadCluster) ForwardEtcdLeadership(_ context.Context, _ *clusterv1.Machine, leaderCandidate *clusterv1.Machine) error {
	if leaderCandidate == nil {
		return errors.New("leaderCandidate is nil")
	}
	return nil
}

func (f fakeWorkloadCluster) ReconcileEtcdMembers(_ context.Context, _ []string, _ semver.Version) ([]string, error) {
	return nil, nil
}

func (f fakeWorkloadCluster) ClusterStatus(_ context.Context) (internal.ClusterStatus, error) {
	return f.Status, nil
}

func (f fakeWorkloadCluster) GetAPIServerCertificateExpiry(_ context.Context, _ *bootstrapv1.OCNEConfig, _ string) (*time.Time, error) {
	return f.APIServerCertificateExpiry, nil
}

func (f fakeWorkloadCluster) AllowBootstrapTokensToGetNodes(_ context.Context) error {
	return nil
}

func (f fakeWorkloadCluster) ReconcileKubeletRBACRole(_ context.Context, _ semver.Version) error {
	return nil
}

func (f fakeWorkloadCluster) ReconcileKubeletRBACBinding(_ context.Context, _ semver.Version) error {
	return nil
}

func (f fakeWorkloadCluster) UpdateKubernetesVersionInOCNEConfigMap(_ context.Context, _ semver.Version) error {
	return nil
}

func (f fakeWorkloadCluster) UpdateEtcdVersionInOCNEConfigMap(_ context.Context, _, _ string, _ semver.Version) error {
	return nil
}

func (f fakeWorkloadCluster) UpdateKubeletConfigMap(_ context.Context, _ semver.Version) error {
	return nil
}

func (f fakeWorkloadCluster) RemoveEtcdMemberForMachine(_ context.Context, _ *clusterv1.Machine) error {
	return nil
}

func (f fakeWorkloadCluster) RemoveMachineFromOCNEConfigMap(_ context.Context, _ *clusterv1.Machine, _ semver.Version) error {
	return nil
}

func (f fakeWorkloadCluster) EtcdMembers(_ context.Context) ([]string, error) {
	return f.EtcdMembersResult, nil
}

type fakeMigrator struct {
	migrateCalled    bool
	migrateErr       error
	migratedCorefile string
}

func (m *fakeMigrator) Migrate(_, _, _ string, _ bool) (string, error) {
	m.migrateCalled = true
	if m.migrateErr != nil {
		return "", m.migrateErr
	}
	return m.migratedCorefile, nil
}
