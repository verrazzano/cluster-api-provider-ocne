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

// Run go generate to regenerate this mock.
//
//go:generate ../../../../hack/tools/bin/mockgen -destination helm_client_mock.go -package mocks -source ../helm_client.go Client
//go:generate ../../../../hack/tools/bin/mockgen -destination kubeconfig_mock.go -package mocks -source ../kubeconfig.go Getter
//go:generate /usr/bin/env bash -c "cat ../../../../hack/boilerplate/boilerplate.generatego-newline.txt helm_client_mock.go > _helm_client_mock.go && mv _helm_client_mock.go helm_client_mock.go"
//go:generate /usr/bin/env bash -c "cat ../../../../hack/boilerplate/boilerplate.generatego-newline.txt kubeconfig_mock.go > _kubeconfig_mock.go && mv _kubeconfig_mock.go kubeconfig_mock.go"
package mocks
