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

// Package upstreamv1beta1 contains a mirror of kubeadm API v1beta1 API, required because it is not possible to import k/K.
//
// IMPORTANT: Do not change these files!
// IMPORTANT: only for OcneConfig serialization/deserialization, and should not be used for other purposes.
//
// +k8s:conversion-gen=github.com/verrazzano/cluster-api-provider-ocne/bootstrap/kubeadm/api/v1beta1
// +k8s:deepcopy-gen=package
package upstreamv1beta1 // import "github.com/verrazzano/cluster-api-provider-ocne/bootstrap/kubeadm/kubeadm/v1beta1"
