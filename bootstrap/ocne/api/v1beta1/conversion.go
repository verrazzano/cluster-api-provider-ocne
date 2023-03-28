/*
Copyright 2021 The Kubernetes Authors.

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

package v1beta1

func (*OcneConfig) Hub()             {}
func (*OcneConfigList) Hub()         {}
func (*OcneConfigTemplate) Hub()     {}
func (*OcneConfigTemplateList) Hub() {}

func (*ClusterConfiguration) Hub() {}
func (*ClusterStatus) Hub()        {}
func (*InitConfiguration) Hub()    {}
func (*JoinConfiguration) Hub()    {}
