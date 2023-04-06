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

package builder

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"

	bootstrapv1 "github.com/verrazzano/cluster-api-provider-ocne/bootstrap/ocne/api/v1beta1"
)

// OCNEConfigBuilder contains the information needed to produce a OCNEConfig.
type OCNEConfigBuilder struct {
	name          string
	namespace     string
	joinConfig    *bootstrapv1.JoinConfiguration
	initConfig    *bootstrapv1.InitConfiguration
	clusterConfig *bootstrapv1.ClusterConfiguration
}

// OCNEConfig returns a OCNEConfigBuilder with the supplied name and namespace.
func OCNEConfig(namespace, name string) *OCNEConfigBuilder {
	return &OCNEConfigBuilder{
		name:      name,
		namespace: namespace,
	}
}

// WithJoinConfig adds the passed JoinConfig to the OCNEConfigBuilder.
func (k *OCNEConfigBuilder) WithJoinConfig(joinConf *bootstrapv1.JoinConfiguration) *OCNEConfigBuilder {
	k.joinConfig = joinConf
	return k
}

// WithClusterConfig adds the passed ClusterConfig to the OCNEConfigBuilder.
func (k *OCNEConfigBuilder) WithClusterConfig(clusterConf *bootstrapv1.ClusterConfiguration) *OCNEConfigBuilder {
	k.clusterConfig = clusterConf
	return k
}

// WithInitConfig adds the passed InitConfig to the OCNEConfigBuilder.
func (k *OCNEConfigBuilder) WithInitConfig(initConf *bootstrapv1.InitConfiguration) *OCNEConfigBuilder {
	k.initConfig = initConf
	return k
}

// Unstructured produces a OCNEConfig as an unstructured Kubernetes object.
func (k *OCNEConfigBuilder) Unstructured() *unstructured.Unstructured {
	config := k.Build()
	rawMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(config)
	if err != nil {
		panic(err)
	}
	return &unstructured.Unstructured{Object: rawMap}
}

// Build produces a OCNEConfig from the variable in the OCNEConfigBuilder.
func (k *OCNEConfigBuilder) Build() *bootstrapv1.OCNEConfig {
	config := &bootstrapv1.OCNEConfig{
		TypeMeta: metav1.TypeMeta{
			Kind:       "OCNEConfig",
			APIVersion: bootstrapv1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: k.namespace,
			Name:      k.name,
		},
	}
	if k.initConfig != nil {
		config.Spec.InitConfiguration = k.initConfig
	}
	if k.joinConfig != nil {
		config.Spec.JoinConfiguration = k.joinConfig
	}
	if k.clusterConfig != nil {
		config.Spec.ClusterConfiguration = k.clusterConfig
	}
	return config
}
