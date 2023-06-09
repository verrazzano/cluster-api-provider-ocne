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

package v1alpha1

import (
	"github.com/verrazzano/cluster-api-provider-ocne/internal/util/ocne"
	utildefaulting "github.com/verrazzano/cluster-api-provider-ocne/util/defaulting"
	ocnemeta "github.com/verrazzano/cluster-api-provider-ocne/util/ocne"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	corev1Cli "k8s.io/client-go/kubernetes/typed/core/v1"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	utilfeature "k8s.io/component-base/featuregate/testing"
	"k8s.io/utils/pointer"

	bootstrapv1 "github.com/verrazzano/cluster-api-provider-ocne/bootstrap/ocne/api/v1alpha1"
	"github.com/verrazzano/cluster-api-provider-ocne/feature"
)

const (
	configMapName   = "ocne-metadata"
	k8sversionsFile = "../../../../util/ocne/testdata/kubernetes_versions.yaml"
)

func TestOCNEControlPlaneDefault(t *testing.T) {
	t.Skip("Skipping tests due to unknown failures")
	g := NewWithT(t)

	ocnecp := &OCNEControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "foo",
		},
		Spec: OCNEControlPlaneSpec{
			Version: "v1.18.3",
			MachineTemplate: OCNEControlPlaneMachineTemplate{
				InfrastructureRef: corev1.ObjectReference{
					APIVersion: "test/v1alpha1",
					Kind:       "UnknownInfraMachine",
					Name:       "foo",
				},
			},
			RolloutStrategy: &RolloutStrategy{},
		},
	}
	updateDefaultingValidationKCP := ocnecp.DeepCopy()
	updateDefaultingValidationKCP.Spec.Version = "v1.18.3"
	updateDefaultingValidationKCP.Spec.MachineTemplate.InfrastructureRef = corev1.ObjectReference{
		APIVersion: "test/v1alpha1",
		Kind:       "UnknownInfraMachine",
		Name:       "foo",
		Namespace:  "foo",
	}
	t.Run("for OCNEControlPlane", utildefaulting.DefaultValidateTest(updateDefaultingValidationKCP))
	ocnecp.Default()

	g.Expect(ocnecp.Spec.ControlPlaneConfig.Format).To(Equal(bootstrapv1.CloudConfig))
	g.Expect(ocnecp.Spec.MachineTemplate.InfrastructureRef.Namespace).To(Equal(ocnecp.Namespace))
	g.Expect(ocnecp.Spec.Version).To(Equal("v1.18.3"))
	g.Expect(ocnecp.Spec.RolloutStrategy.Type).To(Equal(RollingUpdateStrategyType))
	g.Expect(ocnecp.Spec.RolloutStrategy.RollingUpdate.MaxSurge.IntVal).To(Equal(int32(1)))
}

func TestOCNEControlPlaneValidateCreate(t *testing.T) {
	valid := &OCNEControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "foo",
		},
		Spec: OCNEControlPlaneSpec{
			MachineTemplate: OCNEControlPlaneMachineTemplate{
				InfrastructureRef: corev1.ObjectReference{
					APIVersion: "test/v1alpha1",
					Kind:       "UnknownInfraMachine",
					Namespace:  "foo",
					Name:       "infraTemplate",
				},
			},
			ControlPlaneConfig: bootstrapv1.OCNEConfigSpec{
				ClusterConfiguration: &bootstrapv1.ClusterConfiguration{},
			},
			Replicas: pointer.Int32(1),
			Version:  "v1.19.0",
			RolloutStrategy: &RolloutStrategy{
				Type: RollingUpdateStrategyType,
				RollingUpdate: &RollingUpdate{
					MaxSurge: &intstr.IntOrString{
						IntVal: 1,
					},
				},
			},
		},
	}

	invalidMaxSurge := valid.DeepCopy()
	invalidMaxSurge.Spec.RolloutStrategy.RollingUpdate.MaxSurge.IntVal = int32(3)

	stringMaxSurge := valid.DeepCopy()
	val := intstr.FromString("1")
	stringMaxSurge.Spec.RolloutStrategy.RollingUpdate.MaxSurge = &val

	invalidNamespace := valid.DeepCopy()
	invalidNamespace.Spec.MachineTemplate.InfrastructureRef.Namespace = "bar"

	missingReplicas := valid.DeepCopy()
	missingReplicas.Spec.Replicas = nil

	zeroReplicas := valid.DeepCopy()
	zeroReplicas.Spec.Replicas = pointer.Int32(0)

	evenReplicas := valid.DeepCopy()
	evenReplicas.Spec.Replicas = pointer.Int32(2)

	evenReplicasExternalEtcd := evenReplicas.DeepCopy()
	evenReplicasExternalEtcd.Spec.ControlPlaneConfig = bootstrapv1.OCNEConfigSpec{
		ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
			Etcd: bootstrapv1.Etcd{
				External: &bootstrapv1.ExternalEtcd{},
			},
		},
	}

	validVersion := valid.DeepCopy()
	validVersion.Spec.Version = "v1.16.6"

	invalidVersion1 := valid.DeepCopy()
	invalidVersion1.Spec.Version = "vv1.16.6"

	invalidVersion2 := valid.DeepCopy()
	invalidVersion2.Spec.Version = "1.16.6"

	invalidCoreDNSVersion := valid.DeepCopy()
	invalidCoreDNSVersion.Spec.ControlPlaneConfig.ClusterConfiguration.DNS.ImageTag = "v1.7" // not a valid semantic version

	invalidRolloutBeforeCertificateExpiryDays := valid.DeepCopy()
	invalidRolloutBeforeCertificateExpiryDays.Spec.RolloutBefore = &RolloutBefore{
		CertificatesExpiryDays: pointer.Int32(5), // less than minimum
	}

	invalidIgnitionConfiguration := valid.DeepCopy()
	invalidIgnitionConfiguration.Spec.ControlPlaneConfig.Ignition = &bootstrapv1.IgnitionSpec{}

	validIgnitionConfiguration := valid.DeepCopy()
	validIgnitionConfiguration.Spec.ControlPlaneConfig.Format = bootstrapv1.Ignition
	validIgnitionConfiguration.Spec.ControlPlaneConfig.Ignition = &bootstrapv1.IgnitionSpec{}

	tests := []struct {
		name                  string
		enableIgnitionFeature bool
		expectErr             bool
		ocnecp                *OCNEControlPlane
	}{
		{
			name:      "should succeed when given a valid config",
			expectErr: false,
			ocnecp:    valid,
		},
		{
			name:      "should return error when kubeadmControlPlane namespace and infrastructureTemplate  namespace mismatch",
			expectErr: true,
			ocnecp:    invalidNamespace,
		},
		{
			name:      "should return error when replicas is nil",
			expectErr: true,
			ocnecp:    missingReplicas,
		},
		{
			name:      "should return error when replicas is zero",
			expectErr: true,
			ocnecp:    zeroReplicas,
		},
		{
			name:      "should return error when replicas is even",
			expectErr: true,
			ocnecp:    evenReplicas,
		},
		{
			name:      "should allow even replicas when using external etcd",
			expectErr: false,
			ocnecp:    evenReplicasExternalEtcd,
		},
		{
			name:      "should succeed when given a valid semantic version with prepended 'v'",
			expectErr: false,
			ocnecp:    validVersion,
		},
		{
			name:      "should error when given a valid semantic version without 'v'",
			expectErr: true,
			ocnecp:    invalidVersion2,
		},
		{
			name:      "should return error when given an invalid semantic version",
			expectErr: true,
			ocnecp:    invalidVersion1,
		},
		{
			name:      "should return error when given an invalid semantic CoreDNS version",
			expectErr: true,
			ocnecp:    invalidCoreDNSVersion,
		},
		{
			name:      "should return error when maxSurge is not 1",
			expectErr: true,
			ocnecp:    invalidMaxSurge,
		},
		{
			name:      "should succeed when maxSurge is a string",
			expectErr: false,
			ocnecp:    stringMaxSurge,
		},
		{
			name:      "should return error when given an invalid rolloutBefore.certificatesExpiryDays value",
			expectErr: true,
			ocnecp:    invalidRolloutBeforeCertificateExpiryDays,
		},

		{
			name:                  "should return error when Ignition configuration is invalid",
			enableIgnitionFeature: true,
			expectErr:             true,
			ocnecp:                invalidIgnitionConfiguration,
		},
		{
			name:                  "should succeed when Ignition configuration is valid",
			enableIgnitionFeature: true,
			expectErr:             false,
			ocnecp:                validIgnitionConfiguration,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.enableIgnitionFeature {
				// NOTE: KubeadmBootstrapFormatIgnition feature flag is disabled by default.
				// Enabling the feature flag temporarily for this test.
				defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.KubeadmBootstrapFormatIgnition, true)()
			}

			g := NewWithT(t)
			ocneMeta, err := ocnemeta.GetMetaDataContents(k8sversionsFile)
			g.Expect(err).To(BeNil())
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      configMapName,
					Namespace: ocne.GetOCNEMetaNamespace(),
				},
				Data: ocneMeta,
			}
			ocne.GetCoreV1Func = func() (corev1Cli.CoreV1Interface, error) {
				return k8sfake.NewSimpleClientset(configMap).CoreV1(), nil
			}
			defer func() { ocne.GetCoreV1Func = ocne.GetCoreV1Client }()

			if tt.expectErr {
				g.Expect(tt.ocnecp.ValidateCreate()).NotTo(Succeed())
			} else {
				g.Expect(tt.ocnecp.ValidateCreate()).To(Succeed())
			}
		})
	}
}

func TestOCNEControlPlaneValidateUpdate(t *testing.T) {
	before := &OCNEControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "foo",
		},
		Spec: OCNEControlPlaneSpec{
			MachineTemplate: OCNEControlPlaneMachineTemplate{
				InfrastructureRef: corev1.ObjectReference{
					APIVersion: "test/v1alpha1",
					Kind:       "UnknownInfraMachine",
					Namespace:  "foo",
					Name:       "infraTemplate",
				},
				NodeDrainTimeout:        &metav1.Duration{Duration: time.Second},
				NodeVolumeDetachTimeout: &metav1.Duration{Duration: time.Second},
				NodeDeletionTimeout:     &metav1.Duration{Duration: time.Second},
			},
			Replicas: pointer.Int32(1),
			RolloutStrategy: &RolloutStrategy{
				Type: RollingUpdateStrategyType,
				RollingUpdate: &RollingUpdate{
					MaxSurge: &intstr.IntOrString{
						IntVal: 1,
					},
				},
			},
			ControlPlaneConfig: bootstrapv1.OCNEConfigSpec{
				InitConfiguration: &bootstrapv1.InitConfiguration{
					LocalAPIEndpoint: bootstrapv1.APIEndpoint{
						AdvertiseAddress: "127.0.0.1",
						BindPort:         int32(443),
					},
					NodeRegistration: bootstrapv1.NodeRegistrationOptions{
						Name: "test",
					},
				},
				ClusterConfiguration: &bootstrapv1.ClusterConfiguration{
					ClusterName: "test",
					DNS: bootstrapv1.DNS{
						ImageMeta: bootstrapv1.ImageMeta{
							ImageRepository: "registry.k8s.io/coredns",
							ImageTag:        "1.6.5",
						},
					},
				},
				JoinConfiguration: &bootstrapv1.JoinConfiguration{
					Discovery: bootstrapv1.Discovery{
						Timeout: &metav1.Duration{
							Duration: 10 * time.Minute,
						},
					},
					NodeRegistration: bootstrapv1.NodeRegistrationOptions{
						Name: "test",
					},
				},
				PreOCNECommands: []string{
					"test", "foo",
				},
				PostOCNECommands: []string{
					"test", "foo",
				},
				Files: []bootstrapv1.File{
					{
						Path: "test",
					},
				},
				Users: []bootstrapv1.User{
					{
						Name: "user",
						SSHAuthorizedKeys: []string{
							"ssh-rsa foo",
						},
					},
				},
				NTP: &bootstrapv1.NTP{
					Servers: []string{"test-server-1", "test-server-2"},
					Enabled: pointer.Bool(true),
				},
			},
			Version: "v1.25.7",
		},
	}

	updateMaxSurgeVal := before.DeepCopy()
	updateMaxSurgeVal.Spec.RolloutStrategy.RollingUpdate.MaxSurge.IntVal = int32(0)
	updateMaxSurgeVal.Spec.Replicas = pointer.Int32(3)

	wrongReplicaCountForScaleIn := before.DeepCopy()
	wrongReplicaCountForScaleIn.Spec.RolloutStrategy.RollingUpdate.MaxSurge.IntVal = int32(0)

	invalidUpdateKubeadmConfigInit := before.DeepCopy()
	invalidUpdateKubeadmConfigInit.Spec.ControlPlaneConfig.InitConfiguration = &bootstrapv1.InitConfiguration{}

	validUpdateKubeadmConfigInit := before.DeepCopy()
	validUpdateKubeadmConfigInit.Spec.ControlPlaneConfig.InitConfiguration.NodeRegistration = bootstrapv1.NodeRegistrationOptions{}

	invalidUpdateKubeadmConfigCluster := before.DeepCopy()
	invalidUpdateKubeadmConfigCluster.Spec.ControlPlaneConfig.ClusterConfiguration = &bootstrapv1.ClusterConfiguration{}

	invalidUpdateKubeadmConfigJoin := before.DeepCopy()
	invalidUpdateKubeadmConfigJoin.Spec.ControlPlaneConfig.JoinConfiguration = &bootstrapv1.JoinConfiguration{}

	validUpdateKubeadmConfigJoin := before.DeepCopy()
	validUpdateKubeadmConfigJoin.Spec.ControlPlaneConfig.JoinConfiguration.NodeRegistration = bootstrapv1.NodeRegistrationOptions{}

	beforeKubeadmConfigFormatSet := before.DeepCopy()
	beforeKubeadmConfigFormatSet.Spec.ControlPlaneConfig.Format = bootstrapv1.CloudConfig
	invalidUpdateKubeadmConfigFormat := beforeKubeadmConfigFormatSet.DeepCopy()
	invalidUpdateKubeadmConfigFormat.Spec.ControlPlaneConfig.Format = bootstrapv1.Ignition

	validUpdate := before.DeepCopy()
	validUpdate.Labels = map[string]string{"blue": "green"}
	validUpdate.Spec.ControlPlaneConfig.PreOCNECommands = []string{"ab", "abc"}
	validUpdate.Spec.ControlPlaneConfig.PostOCNECommands = []string{"ab", "abc"}
	validUpdate.Spec.ControlPlaneConfig.Files = []bootstrapv1.File{
		{
			Path: "ab",
		},
		{
			Path: "abc",
		},
	}
	validUpdate.Spec.Version = "v1.25.7"
	validUpdate.Spec.ControlPlaneConfig.Users = []bootstrapv1.User{
		{
			Name: "bar",
			SSHAuthorizedKeys: []string{
				"ssh-rsa bar",
				"ssh-rsa foo",
			},
		},
	}
	validUpdate.Spec.MachineTemplate.ObjectMeta.Labels = map[string]string{
		"label": "labelValue",
	}
	validUpdate.Spec.MachineTemplate.ObjectMeta.Annotations = map[string]string{
		"annotation": "labelAnnotation",
	}
	validUpdate.Spec.MachineTemplate.InfrastructureRef.APIVersion = "test/v1alpha2"
	validUpdate.Spec.MachineTemplate.InfrastructureRef.Name = "orange"
	validUpdate.Spec.MachineTemplate.NodeDrainTimeout = &metav1.Duration{Duration: 10 * time.Second}
	validUpdate.Spec.MachineTemplate.NodeVolumeDetachTimeout = &metav1.Duration{Duration: 10 * time.Second}
	validUpdate.Spec.MachineTemplate.NodeDeletionTimeout = &metav1.Duration{Duration: 10 * time.Second}
	validUpdate.Spec.Replicas = pointer.Int32(5)
	now := metav1.NewTime(time.Now())
	validUpdate.Spec.RolloutAfter = &now
	validUpdate.Spec.RolloutBefore = &RolloutBefore{
		CertificatesExpiryDays: pointer.Int32(14),
	}
	validUpdate.Spec.ControlPlaneConfig.Format = bootstrapv1.CloudConfig

	scaleToZero := before.DeepCopy()
	scaleToZero.Spec.Replicas = pointer.Int32(0)

	scaleToEven := before.DeepCopy()
	scaleToEven.Spec.Replicas = pointer.Int32(2)

	invalidNamespace := before.DeepCopy()
	invalidNamespace.Spec.MachineTemplate.InfrastructureRef.Namespace = "bar"

	missingReplicas := before.DeepCopy()
	missingReplicas.Spec.Replicas = nil

	etcdLocalImageTag := before.DeepCopy()
	etcdLocalImageTag.Spec.ControlPlaneConfig.ClusterConfiguration.Etcd.Local = &bootstrapv1.LocalEtcd{
		ImageMeta: bootstrapv1.ImageMeta{
			ImageTag: "v9.1.1",
		},
	}

	etcdLocalImageBuildTag := before.DeepCopy()
	etcdLocalImageBuildTag.Spec.ControlPlaneConfig.ClusterConfiguration.Etcd.Local = &bootstrapv1.LocalEtcd{
		ImageMeta: bootstrapv1.ImageMeta{
			ImageTag: "v9.1.1_validBuild1",
		},
	}

	etcdLocalImageInvalidTag := before.DeepCopy()
	etcdLocalImageInvalidTag.Spec.ControlPlaneConfig.ClusterConfiguration.Etcd.Local = &bootstrapv1.LocalEtcd{
		ImageMeta: bootstrapv1.ImageMeta{
			ImageTag: "v9.1.1+invalidBuild1",
		},
	}

	unsetEtcd := etcdLocalImageTag.DeepCopy()
	unsetEtcd.Spec.ControlPlaneConfig.ClusterConfiguration.Etcd.Local = nil

	networking := before.DeepCopy()
	networking.Spec.ControlPlaneConfig.ClusterConfiguration.Networking.DNSDomain = "some dns domain"

	kubernetesVersion := before.DeepCopy()
	kubernetesVersion.Spec.ControlPlaneConfig.ClusterConfiguration.KubernetesVersion = "some kubernetes version"

	controlPlaneEndpoint := before.DeepCopy()
	controlPlaneEndpoint.Spec.ControlPlaneConfig.ClusterConfiguration.ControlPlaneEndpoint = "some control plane endpoint"

	apiServer := before.DeepCopy()
	apiServer.Spec.ControlPlaneConfig.ClusterConfiguration.APIServer = bootstrapv1.APIServer{
		ControlPlaneComponent: bootstrapv1.ControlPlaneComponent{
			ExtraArgs:    map[string]string{"foo": "bar"},
			ExtraVolumes: []bootstrapv1.HostPathMount{{Name: "mount1"}},
		},
		TimeoutForControlPlane: &metav1.Duration{Duration: 5 * time.Minute},
		CertSANs:               []string{"foo", "bar"},
	}

	controllerManager := before.DeepCopy()
	controllerManager.Spec.ControlPlaneConfig.ClusterConfiguration.ControllerManager = bootstrapv1.ControlPlaneComponent{
		ExtraArgs:    map[string]string{"controller manager field": "controller manager value"},
		ExtraVolumes: []bootstrapv1.HostPathMount{{Name: "mount", HostPath: "/foo", MountPath: "bar", ReadOnly: true, PathType: "File"}},
	}

	scheduler := before.DeepCopy()
	scheduler.Spec.ControlPlaneConfig.ClusterConfiguration.Scheduler = bootstrapv1.ControlPlaneComponent{
		ExtraArgs:    map[string]string{"scheduler field": "scheduler value"},
		ExtraVolumes: []bootstrapv1.HostPathMount{{Name: "mount", HostPath: "/foo", MountPath: "bar", ReadOnly: true, PathType: "File"}},
	}

	dns := before.DeepCopy()
	dns.Spec.ControlPlaneConfig.ClusterConfiguration.DNS = bootstrapv1.DNS{
		ImageMeta: bootstrapv1.ImageMeta{
			ImageRepository: "gcr.io/capi-test",
			ImageTag:        "v1.6.6_foobar.1",
		},
	}

	dnsBuildTag := before.DeepCopy()
	dnsBuildTag.Spec.ControlPlaneConfig.ClusterConfiguration.DNS = bootstrapv1.DNS{
		ImageMeta: bootstrapv1.ImageMeta{
			ImageRepository: "gcr.io/capi-test",
			ImageTag:        "1.6.7",
		},
	}

	dnsInvalidTag := before.DeepCopy()
	dnsInvalidTag.Spec.ControlPlaneConfig.ClusterConfiguration.DNS = bootstrapv1.DNS{
		ImageMeta: bootstrapv1.ImageMeta{
			ImageRepository: "gcr.io/capi-test",
			ImageTag:        "v0.20.0+invalidBuild1",
		},
	}

	dnsInvalidCoreDNSToVersion := dns.DeepCopy()
	dnsInvalidCoreDNSToVersion.Spec.ControlPlaneConfig.ClusterConfiguration.DNS = bootstrapv1.DNS{
		ImageMeta: bootstrapv1.ImageMeta{
			ImageRepository: "gcr.io/capi-test",
			ImageTag:        "1.6.5",
		},
	}

	validCoreDNSCustomToVersion := dns.DeepCopy()
	validCoreDNSCustomToVersion.Spec.ControlPlaneConfig.ClusterConfiguration.DNS = bootstrapv1.DNS{
		ImageMeta: bootstrapv1.ImageMeta{
			ImageRepository: "gcr.io/capi-test",
			ImageTag:        "v1.6.6_foobar.2",
		},
	}
	validUnsupportedCoreDNSVersion := dns.DeepCopy()
	validUnsupportedCoreDNSVersion.Spec.ControlPlaneConfig.ClusterConfiguration.DNS = bootstrapv1.DNS{
		ImageMeta: bootstrapv1.ImageMeta{
			ImageRepository: "gcr.io/capi-test",
			ImageTag:        "v99.99.99",
		},
	}

	unsetCoreDNSToVersion := dns.DeepCopy()
	unsetCoreDNSToVersion.Spec.ControlPlaneConfig.ClusterConfiguration.DNS = bootstrapv1.DNS{
		ImageMeta: bootstrapv1.ImageMeta{
			ImageRepository: "",
			ImageTag:        "",
		},
	}

	certificatesDir := before.DeepCopy()
	certificatesDir.Spec.ControlPlaneConfig.ClusterConfiguration.CertificatesDir = "a new certificates directory"

	imageRepository := before.DeepCopy()
	imageRepository.Spec.ControlPlaneConfig.ClusterConfiguration.ImageRepository = "a new image repository"

	featureGates := before.DeepCopy()
	featureGates.Spec.ControlPlaneConfig.ClusterConfiguration.FeatureGates = map[string]bool{"a feature gate": true}

	externalEtcd := before.DeepCopy()
	externalEtcd.Spec.ControlPlaneConfig.ClusterConfiguration.Etcd.External = &bootstrapv1.ExternalEtcd{
		KeyFile: "some key file",
	}

	localDataDir := before.DeepCopy()
	localDataDir.Spec.ControlPlaneConfig.ClusterConfiguration.Etcd.Local = &bootstrapv1.LocalEtcd{
		DataDir: "some local data dir",
	}
	modifyLocalDataDir := localDataDir.DeepCopy()
	modifyLocalDataDir.Spec.ControlPlaneConfig.ClusterConfiguration.Etcd.Local.DataDir = "a different local data dir"

	localPeerCertSANs := before.DeepCopy()
	localPeerCertSANs.Spec.ControlPlaneConfig.ClusterConfiguration.Etcd.Local = &bootstrapv1.LocalEtcd{
		PeerCertSANs: []string{"a cert"},
	}

	localServerCertSANs := before.DeepCopy()
	localServerCertSANs.Spec.ControlPlaneConfig.ClusterConfiguration.Etcd.Local = &bootstrapv1.LocalEtcd{
		ServerCertSANs: []string{"a cert"},
	}

	localExtraArgs := before.DeepCopy()
	localExtraArgs.Spec.ControlPlaneConfig.ClusterConfiguration.Etcd.Local = &bootstrapv1.LocalEtcd{
		ExtraArgs: map[string]string{"an arg": "a value"},
	}

	beforeExternalEtcdCluster := before.DeepCopy()
	beforeExternalEtcdCluster.Spec.ControlPlaneConfig.ClusterConfiguration = &bootstrapv1.ClusterConfiguration{
		Etcd: bootstrapv1.Etcd{
			External: &bootstrapv1.ExternalEtcd{
				Endpoints: []string{"127.0.0.1"},
			},
		},
	}
	scaleToEvenExternalEtcdCluster := beforeExternalEtcdCluster.DeepCopy()
	scaleToEvenExternalEtcdCluster.Spec.Replicas = pointer.Int32(2)

	beforeInvalidEtcdCluster := before.DeepCopy()
	beforeInvalidEtcdCluster.Spec.ControlPlaneConfig.ClusterConfiguration.Etcd = bootstrapv1.Etcd{
		Local: &bootstrapv1.LocalEtcd{
			ImageMeta: bootstrapv1.ImageMeta{
				ImageRepository: "image-repository",
				ImageTag:        "latest",
			},
		},
	}

	afterInvalidEtcdCluster := beforeInvalidEtcdCluster.DeepCopy()
	afterInvalidEtcdCluster.Spec.ControlPlaneConfig.ClusterConfiguration.Etcd = bootstrapv1.Etcd{
		External: &bootstrapv1.ExternalEtcd{
			Endpoints: []string{"127.0.0.1"},
		},
	}

	withoutClusterConfiguration := before.DeepCopy()
	withoutClusterConfiguration.Spec.ControlPlaneConfig.ClusterConfiguration = nil

	afterEtcdLocalDirAddition := before.DeepCopy()
	afterEtcdLocalDirAddition.Spec.ControlPlaneConfig.ClusterConfiguration.Etcd.Local = &bootstrapv1.LocalEtcd{
		DataDir: "/data",
	}

	updateNTPServers := before.DeepCopy()
	updateNTPServers.Spec.ControlPlaneConfig.NTP.Servers = []string{"new-server"}

	disableNTPServers := before.DeepCopy()
	disableNTPServers.Spec.ControlPlaneConfig.NTP.Enabled = pointer.Bool(false)

	invalidRolloutBeforeCertificateExpiryDays := before.DeepCopy()
	invalidRolloutBeforeCertificateExpiryDays.Spec.RolloutBefore = &RolloutBefore{
		CertificatesExpiryDays: pointer.Int32(5), // less than minimum
	}

	invalidIgnitionConfiguration := before.DeepCopy()
	invalidIgnitionConfiguration.Spec.ControlPlaneConfig.Ignition = &bootstrapv1.IgnitionSpec{}

	validIgnitionConfigurationBefore := before.DeepCopy()
	validIgnitionConfigurationBefore.Spec.ControlPlaneConfig.Format = bootstrapv1.Ignition
	validIgnitionConfigurationBefore.Spec.ControlPlaneConfig.Ignition = &bootstrapv1.IgnitionSpec{
		ContainerLinuxConfig: &bootstrapv1.ContainerLinuxConfig{},
	}

	validIgnitionConfigurationAfter := validIgnitionConfigurationBefore.DeepCopy()
	validIgnitionConfigurationAfter.Spec.ControlPlaneConfig.Ignition.ContainerLinuxConfig.AdditionalConfig = "foo: bar"

	updateInitConfigurationPatches := before.DeepCopy()
	updateInitConfigurationPatches.Spec.ControlPlaneConfig.InitConfiguration.Patches = &bootstrapv1.Patches{
		Directory: "/tmp/patches",
	}

	updateJoinConfigurationPatches := before.DeepCopy()
	updateJoinConfigurationPatches.Spec.ControlPlaneConfig.InitConfiguration.Patches = &bootstrapv1.Patches{
		Directory: "/tmp/patches",
	}

	updateInitConfigurationSkipPhases := before.DeepCopy()
	updateInitConfigurationSkipPhases.Spec.ControlPlaneConfig.InitConfiguration.SkipPhases = []string{"addon/kube-proxy"}

	updateJoinConfigurationSkipPhases := before.DeepCopy()
	updateJoinConfigurationSkipPhases.Spec.ControlPlaneConfig.JoinConfiguration.SkipPhases = []string{"addon/kube-proxy"}

	updateDiskSetup := before.DeepCopy()
	updateDiskSetup.Spec.ControlPlaneConfig.DiskSetup = &bootstrapv1.DiskSetup{
		Filesystems: []bootstrapv1.Filesystem{
			{
				Device:     "/dev/sda",
				Filesystem: "ext4",
			},
		},
	}

	tests := []struct {
		name                  string
		enableIgnitionFeature bool
		expectErr             bool
		before                *OCNEControlPlane
		ocnecp                *OCNEControlPlane
	}{
		{
			name:      "should succeed when given a valid config",
			expectErr: false,
			before:    before,
			ocnecp:    validUpdate,
		},
		{
			name:      "should return error when trying to mutate the kubeadmconfigspec initconfiguration",
			expectErr: true,
			before:    before,
			ocnecp:    invalidUpdateKubeadmConfigInit,
		},
		{
			name:      "should not return an error when trying to mutate the kubeadmconfigspec initconfiguration noderegistration",
			expectErr: false,
			before:    before,
			ocnecp:    validUpdateKubeadmConfigInit,
		},
		{
			name:      "should return error when trying to mutate the kubeadmconfigspec clusterconfiguration",
			expectErr: true,
			before:    before,
			ocnecp:    invalidUpdateKubeadmConfigCluster,
		},
		{
			name:      "should return error when trying to mutate the kubeadmconfigspec joinconfiguration",
			expectErr: true,
			before:    before,
			ocnecp:    invalidUpdateKubeadmConfigJoin,
		},
		{
			name:      "should not return an error when trying to mutate the kubeadmconfigspec joinconfiguration noderegistration",
			expectErr: false,
			before:    before,
			ocnecp:    validUpdateKubeadmConfigJoin,
		},
		{
			name:      "should return error when trying to mutate the kubeadmconfigspec format from cloud-config to ignition",
			expectErr: true,
			before:    beforeKubeadmConfigFormatSet,
			ocnecp:    invalidUpdateKubeadmConfigFormat,
		},
		{
			name:      "should return error when trying to scale to zero",
			expectErr: true,
			before:    before,
			ocnecp:    scaleToZero,
		},
		{
			name:      "should return error when trying to scale to an even number",
			expectErr: true,
			before:    before,
			ocnecp:    scaleToEven,
		},
		{
			name:      "should return error when trying to reference cross namespace",
			expectErr: true,
			before:    before,
			ocnecp:    invalidNamespace,
		},
		{
			name:      "should return error when trying to scale to nil",
			expectErr: true,
			before:    before,
			ocnecp:    missingReplicas,
		},
		{
			name:      "should succeed when trying to scale to an even number with external etcd defined in ClusterConfiguration",
			expectErr: false,
			before:    beforeExternalEtcdCluster,
			ocnecp:    scaleToEvenExternalEtcdCluster,
		},
		{
			name:      "should succeed when making a change to the local etcd image tag",
			expectErr: false,
			before:    before,
			ocnecp:    etcdLocalImageTag,
		},
		{
			name:      "should succeed when making a change to the local etcd image tag",
			expectErr: false,
			before:    before,
			ocnecp:    etcdLocalImageBuildTag,
		},
		{
			name:      "should fail when using an invalid etcd image tag",
			expectErr: true,
			before:    before,
			ocnecp:    etcdLocalImageInvalidTag,
		},
		{
			name:      "should fail when making a change to the cluster config's networking struct",
			expectErr: true,
			before:    before,
			ocnecp:    networking,
		},
		{
			name:      "should fail when making a change to the cluster config's kubernetes version",
			expectErr: true,
			before:    before,
			ocnecp:    kubernetesVersion,
		},
		{
			name:      "should fail when making a change to the cluster config's controlPlaneEndpoint",
			expectErr: true,
			before:    before,
			ocnecp:    controlPlaneEndpoint,
		},
		{
			name:      "should allow changes to the cluster config's apiServer",
			expectErr: false,
			before:    before,
			ocnecp:    apiServer,
		},
		{
			name:      "should allow changes to the cluster config's controllerManager",
			expectErr: false,
			before:    before,
			ocnecp:    controllerManager,
		},
		{
			name:      "should allow changes to the cluster config's scheduler",
			expectErr: false,
			before:    before,
			ocnecp:    scheduler,
		},
		{
			name:      "should succeed when making a change to the cluster config's dns",
			expectErr: false,
			before:    before,
			ocnecp:    dns,
		},
		{
			name:      "should succeed when changing to a valid custom CoreDNS version",
			expectErr: false,
			before:    dns,
			ocnecp:    validCoreDNSCustomToVersion,
		},
		{
			name:      "should succeed when CoreDNS ImageTag is unset",
			expectErr: false,
			before:    dns,
			ocnecp:    unsetCoreDNSToVersion,
		},
		{
			name:      "should succeed when using an valid DNS build",
			expectErr: false,
			before:    before,
			ocnecp:    dnsBuildTag,
		},
		{
			name:   "should succeed when using the same CoreDNS version",
			before: dns,
			ocnecp: dns.DeepCopy(),
		},
		{
			name:   "should succeed when using the same CoreDNS version - not supported",
			before: validUnsupportedCoreDNSVersion,
			ocnecp: validUnsupportedCoreDNSVersion,
		},
		{
			name:      "should fail when using an invalid DNS build",
			expectErr: true,
			before:    before,
			ocnecp:    dnsInvalidTag,
		},
		{
			name:      "should fail when using an invalid CoreDNS version",
			expectErr: true,
			before:    dns,
			ocnecp:    dnsInvalidCoreDNSToVersion,
		},

		{
			name:      "should fail when making a change to the cluster config's certificatesDir",
			expectErr: true,
			before:    before,
			ocnecp:    certificatesDir,
		},
		{
			name:      "should fail when making a change to the cluster config's imageRepository",
			expectErr: false,
			before:    before,
			ocnecp:    imageRepository,
		},
		{
			name:      "should fail when making a change to the cluster config's featureGates",
			expectErr: true,
			before:    before,
			ocnecp:    featureGates,
		},
		{
			name:      "should fail when making a change to the cluster config's local etcd's configuration localDataDir field",
			expectErr: true,
			before:    before,
			ocnecp:    localDataDir,
		},
		{
			name:      "should fail when making a change to the cluster config's local etcd's configuration localPeerCertSANs field",
			expectErr: true,
			before:    before,
			ocnecp:    localPeerCertSANs,
		},
		{
			name:      "should fail when making a change to the cluster config's local etcd's configuration localServerCertSANs field",
			expectErr: true,
			before:    before,
			ocnecp:    localServerCertSANs,
		},
		{
			name:      "should succeed when making a change to the cluster config's local etcd's configuration localExtraArgs field",
			expectErr: false,
			before:    before,
			ocnecp:    localExtraArgs,
		},
		{
			name:      "should fail when making a change to the cluster config's external etcd's configuration",
			expectErr: true,
			before:    before,
			ocnecp:    externalEtcd,
		},
		{
			name:      "should fail when attempting to unset the etcd local object",
			expectErr: true,
			before:    etcdLocalImageTag,
			ocnecp:    unsetEtcd,
		},
		{
			name:      "should fail when modifying a field that is not the local etcd image metadata",
			expectErr: true,
			before:    localDataDir,
			ocnecp:    modifyLocalDataDir,
		},
		{
			name:      "should fail if both local and external etcd are set",
			expectErr: true,
			before:    beforeInvalidEtcdCluster,
			ocnecp:    afterInvalidEtcdCluster,
		},
		{
			name:      "should pass if ClusterConfiguration is nil",
			expectErr: false,
			before:    withoutClusterConfiguration,
			ocnecp:    withoutClusterConfiguration,
		},
		{
			name:      "should fail if etcd local dir is changed from missing ClusterConfiguration",
			expectErr: true,
			before:    withoutClusterConfiguration,
			ocnecp:    afterEtcdLocalDirAddition,
		},
		{
			name:      "should not return an error when maxSurge value is updated to 0",
			expectErr: false,
			before:    before,
			ocnecp:    updateMaxSurgeVal,
		},
		{
			name:      "should return an error when maxSurge value is updated to 0, but replica count is < 3",
			expectErr: true,
			before:    before,
			ocnecp:    wrongReplicaCountForScaleIn,
		},
		{
			name:      "should pass if NTP servers are updated",
			expectErr: false,
			before:    before,
			ocnecp:    updateNTPServers,
		},
		{
			name:      "should pass if NTP servers is disabled during update",
			expectErr: false,
			before:    before,
			ocnecp:    disableNTPServers,
		},
		{
			name:      "should allow changes to initConfiguration.patches",
			expectErr: false,
			before:    before,
			ocnecp:    updateInitConfigurationPatches,
		},
		{
			name:      "should allow changes to joinConfiguration.patches",
			expectErr: false,
			before:    before,
			ocnecp:    updateJoinConfigurationPatches,
		},
		{
			name:      "should allow changes to initConfiguration.skipPhases",
			expectErr: false,
			before:    before,
			ocnecp:    updateInitConfigurationSkipPhases,
		},
		{
			name:      "should allow changes to joinConfiguration.skipPhases",
			expectErr: false,
			before:    before,
			ocnecp:    updateJoinConfigurationSkipPhases,
		},
		{
			name:      "should allow changes to diskSetup",
			expectErr: false,
			before:    before,
			ocnecp:    updateDiskSetup,
		},
		{
			name:      "should return error when rolloutBefore.certificatesExpiryDays is invalid",
			expectErr: true,
			before:    before,
			ocnecp:    invalidRolloutBeforeCertificateExpiryDays,
		},
		{
			name:                  "should return error when Ignition configuration is invalid",
			enableIgnitionFeature: true,
			expectErr:             true,
			before:                invalidIgnitionConfiguration,
			ocnecp:                invalidIgnitionConfiguration,
		},
		{
			name:                  "should succeed when Ignition configuration is modified",
			enableIgnitionFeature: true,
			expectErr:             false,
			before:                validIgnitionConfigurationBefore,
			ocnecp:                validIgnitionConfigurationAfter,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.enableIgnitionFeature {
				// NOTE: KubeadmBootstrapFormatIgnition feature flag is disabled by default.
				// Enabling the feature flag temporarily for this test.
				defer utilfeature.SetFeatureGateDuringTest(t, feature.Gates, feature.KubeadmBootstrapFormatIgnition, true)()
			}

			g := NewWithT(t)

			ocneMeta, err := ocnemeta.GetMetaDataContents(k8sversionsFile)
			g.Expect(err).To(BeNil())
			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      configMapName,
					Namespace: ocne.GetOCNEMetaNamespace(),
				},
				Data: ocneMeta,
			}
			ocne.GetCoreV1Func = func() (corev1Cli.CoreV1Interface, error) {
				return k8sfake.NewSimpleClientset(configMap).CoreV1(), nil
			}
			defer func() { ocne.GetCoreV1Func = ocne.GetCoreV1Client }()

			err = tt.ocnecp.ValidateUpdate(tt.before.DeepCopy())
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).To(Succeed())
			}
		})
	}
}

func TestValidateVersion(t *testing.T) {
	tests := []struct {
		name                 string
		clusterConfiguration *bootstrapv1.ClusterConfiguration
		oldVersion           string
		newVersion           string
		expectErr            bool
	}{
		// Basic validation of old and new version.
		{
			name:       "error when old version is empty",
			oldVersion: "",
			newVersion: "v1.16.6",
			expectErr:  true,
		},
		{
			name:       "error when old version is invalid",
			oldVersion: "invalid-version",
			newVersion: "v1.18.1",
			expectErr:  true,
		},
		{
			name:       "error when new version is empty",
			oldVersion: "v1.16.6",
			newVersion: "",
			expectErr:  true,
		},
		{
			name:       "error when new version is invalid",
			oldVersion: "v1.18.1",
			newVersion: "invalid-version",
			expectErr:  true,
		},
		// Validation that we block upgrade to v1.19.0.
		// Note: Upgrading to v1.19.0 is not supported, because of issues in v1.19.0,
		// see: https://github.com/kubernetes-sigs/cluster-api/issues/3564
		{
			name:       "error when upgrading to v1.19.0",
			oldVersion: "v1.18.8",
			newVersion: "v1.19.0",
			expectErr:  true,
		},
		{
			name:       "pass when both versions are v1.19.0",
			oldVersion: "v1.19.0",
			newVersion: "v1.19.0",
			expectErr:  false,
		},
		// Validation for skip-level upgrades.
		{
			name:       "error when upgrading two minor versions",
			oldVersion: "v1.18.8",
			newVersion: "v1.20.0-alpha.0.734_ba502ee555924a",
			expectErr:  true,
		},
		{
			name:       "pass when upgrading one minor version",
			oldVersion: "v1.20.1",
			newVersion: "v1.21.18",
			expectErr:  false,
		},
		// Validation for usage of the old registry.
		// Notes:
		// * kubeadm versions < v1.22 are always using the old registry.
		// * kubeadm versions >= v1.25.0 are always using the new registry.
		// * kubeadm versions in between are using the new registry
		//   starting with certain patch versions.
		// This test validates that we don't block upgrades for < v1.22.0 and >= v1.25.0
		// and block upgrades to kubeadm versions in between with the old registry.
		{
			name: "pass when imageRepository is set",
			clusterConfiguration: &bootstrapv1.ClusterConfiguration{
				ImageRepository: "k8s.gcr.io",
			},
			oldVersion: "v1.21.1",
			newVersion: "v1.22.16",
			expectErr:  false,
		},
		{
			name:       "pass when version didn't change",
			oldVersion: "v1.22.16",
			newVersion: "v1.22.16",
			expectErr:  false,
		},
		{
			name:       "pass when new version is < v1.22.0",
			oldVersion: "v1.20.10",
			newVersion: "v1.21.5",
			expectErr:  false,
		},
		{
			name:       "pass when new version is using new registry (>= v1.24.9)",
			oldVersion: "v1.23.1",
			newVersion: "v1.24.9", // first patch release using new registry
			expectErr:  false,
		},
		{
			name:       "pass when new version is using new registry (>= v1.25.0)",
			oldVersion: "v1.25.7",
			newVersion: "v1.25.0", // uses new registry
			expectErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			ocnecp := OCNEControlPlane{
				Spec: OCNEControlPlaneSpec{
					ControlPlaneConfig: bootstrapv1.OCNEConfigSpec{
						ClusterConfiguration: tt.clusterConfiguration,
					},
					Version: tt.newVersion,
				},
			}

			allErrs := ocnecp.validateVersion(tt.oldVersion)
			if tt.expectErr {
				g.Expect(allErrs).ToNot(HaveLen(0))
			} else {
				g.Expect(allErrs).To(HaveLen(0))
			}
		})
	}
}

func TestOCNEControlPlaneValidateUpdateAfterDefaulting(t *testing.T) {
	t.Skip("Skipping tests due to unknown failures")
	before := &OCNEControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "foo",
		},
		Spec: OCNEControlPlaneSpec{
			Version: "v1.19.0",
			MachineTemplate: OCNEControlPlaneMachineTemplate{
				InfrastructureRef: corev1.ObjectReference{
					APIVersion: "test/v1alpha1",
					Kind:       "UnknownInfraMachine",
					Namespace:  "foo",
					Name:       "infraTemplate",
				},
			},
		},
	}

	afterDefault := before.DeepCopy()
	afterDefault.Default()

	tests := []struct {
		name      string
		expectErr bool
		before    *OCNEControlPlane
		ocnecp    *OCNEControlPlane
	}{
		{
			name:      "update should succeed after defaulting",
			expectErr: false,
			before:    before,
			ocnecp:    afterDefault,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			err := tt.ocnecp.ValidateUpdate(tt.before.DeepCopy())
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).To(Succeed())
				g.Expect(tt.ocnecp.Spec.MachineTemplate.InfrastructureRef.Namespace).To(Equal(tt.before.Namespace))
				g.Expect(tt.ocnecp.Spec.Version).To(Equal("v1.19.0"))
				g.Expect(tt.ocnecp.Spec.RolloutStrategy.Type).To(Equal(RollingUpdateStrategyType))
				g.Expect(tt.ocnecp.Spec.RolloutStrategy.RollingUpdate.MaxSurge.IntVal).To(Equal(int32(1)))
				g.Expect(tt.ocnecp.Spec.Replicas).To(Equal(pointer.Int32(1)))
			}
		})
	}
}

func TestPathsMatch(t *testing.T) {
	tests := []struct {
		name          string
		allowed, path []string
		match         bool
	}{
		{
			name:    "a simple match case",
			allowed: []string{"a", "b", "c"},
			path:    []string{"a", "b", "c"},
			match:   true,
		},
		{
			name:    "a case can't match",
			allowed: []string{"a", "b", "c"},
			path:    []string{"a"},
			match:   false,
		},
		{
			name:    "an empty path for whatever reason",
			allowed: []string{"a"},
			path:    []string{""},
			match:   false,
		},
		{
			name:    "empty allowed matches nothing",
			allowed: []string{},
			path:    []string{"a"},
			match:   false,
		},
		{
			name:    "wildcard match",
			allowed: []string{"a", "b", "c", "d", "*"},
			path:    []string{"a", "b", "c", "d", "e", "f", "g"},
			match:   true,
		},
		{
			name:    "long path",
			allowed: []string{"a"},
			path:    []string{"a", "b", "c", "d", "e", "f", "g"},
			match:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(pathsMatch(tt.allowed, tt.path)).To(Equal(tt.match))
		})
	}
}

func TestAllowed(t *testing.T) {
	tests := []struct {
		name      string
		allowList [][]string
		path      []string
		match     bool
	}{
		{
			name: "matches the first and none of the others",
			allowList: [][]string{
				{"a", "b", "c"},
				{"b", "d", "x"},
			},
			path:  []string{"a", "b", "c"},
			match: true,
		},
		{
			name: "matches none in the allow list",
			allowList: [][]string{
				{"a", "b", "c"},
				{"b", "c", "d"},
				{"e", "*"},
			},
			path:  []string{"a"},
			match: false,
		},
		{
			name: "an empty path matches nothing",
			allowList: [][]string{
				{"a", "b", "c"},
				{"*"},
				{"b", "c"},
			},
			path:  []string{},
			match: false,
		},
		{
			name:      "empty allowList matches nothing",
			allowList: [][]string{},
			path:      []string{"a"},
			match:     false,
		},
		{
			name: "length test check",
			allowList: [][]string{
				{"a", "b", "c", "d", "e", "f"},
				{"a", "b", "c", "d", "e", "f", "g", "h"},
			},
			path:  []string{"a", "b", "c", "d", "e", "f", "g"},
			match: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(allowed(tt.allowList, tt.path)).To(Equal(tt.match))
		})
	}
}

func TestPaths(t *testing.T) {
	tests := []struct {
		name     string
		path     []string
		diff     map[string]interface{}
		expected [][]string
	}{
		{
			name: "basic check",
			diff: map[string]interface{}{
				"spec": map[string]interface{}{
					"replicas": 4,
					"version":  "1.17.3",
					"controlPlaneConfig": map[string]interface{}{
						"clusterConfiguration": map[string]interface{}{
							"version": "v2.0.1",
						},
						"initConfiguration": map[string]interface{}{
							"bootstrapToken": []string{"abcd", "defg"},
						},
						"joinConfiguration": nil,
					},
				},
			},
			expected: [][]string{
				{"spec", "replicas"},
				{"spec", "version"},
				{"spec", "controlPlaneConfig", "joinConfiguration"},
				{"spec", "controlPlaneConfig", "clusterConfiguration", "version"},
				{"spec", "controlPlaneConfig", "initConfiguration", "bootstrapToken"},
			},
		},
		{
			name:     "empty input makes for empty output",
			path:     []string{"a"},
			diff:     map[string]interface{}{},
			expected: [][]string{},
		},
		{
			name: "long recursive check with two keys",
			diff: map[string]interface{}{
				"spec": map[string]interface{}{
					"controlPlaneConfig": map[string]interface{}{
						"clusterConfiguration": map[string]interface{}{
							"version": "v2.0.1",
							"abc":     "d",
						},
					},
				},
			},
			expected: [][]string{
				{"spec", "controlPlaneConfig", "clusterConfiguration", "version"},
				{"spec", "controlPlaneConfig", "clusterConfiguration", "abc"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(paths(tt.path, tt.diff)).To(ConsistOf(tt.expected))
		})
	}
}
