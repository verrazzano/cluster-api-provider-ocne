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

// Package kubeadm contains utils related to kubeadm.
package ocne

import (
	"context"
	"fmt"
	"github.com/blang/semver"
	ocnemeta "github.com/verrazzano/cluster-api-provider-ocne/util/ocne"
	"strings"
)

const (
	// DefaultImageRepository is the new default Kubernetes image registry.
	DefaultImageRepository = "registry.k8s.io"
	// OldDefaultImageRepository is the old default Kubernetes image registry.
	OldDefaultImageRepository = "k8s.gcr.io"
	// DefaultOCNEImageRepository is the default ocne image repository
	DefaultOCNEImageRepository = "container-registry.oracle.com/olcne"

	// DefaultOCNESocket is the crio socket used for OCNE
	DefaultOCNESocket = "/var/run/crio/crio.sock"

	// DefaultOCNECSISocket is teh default socket for OCI CSI
	DefaultOCNECSISocket = "/var/run/shared-tmpfs/csi.sock"

	K8sVersionOneTwoFourEight = "1.24.8"
)

var (
	// MinKubernetesVersionImageRegistryMigration is the first Kubernetes minor version which
	// has patch versions where the default image registry in kubeadm is registry.k8s.io instead of k8s.gcr.io.
	MinKubernetesVersionImageRegistryMigration = semver.MustParse("1.24.8")

	// NextKubernetesVersionImageRegistryMigration is the next minor version after
	// the default image registry in kubeadm changed to registry.k8s.io.
	NextKubernetesVersionImageRegistryMigration = semver.MustParse("1.25.7")
)

// GetDefaultRegistry returns the default registry of the given kubeadm version.
func GetDefaultRegistry(version semver.Version) string {
	// If version <= v1.22.16 return k8s.gcr.io
	if version.LTE(semver.MustParse("1.22.16")) {
		return OldDefaultImageRepository
	}

	// If v1.22.17 <= version < v1.23.0 return registry.k8s.io
	if version.GTE(semver.MustParse("1.22.17")) &&
		version.LT(semver.MustParse("1.23.0")) {
		return DefaultImageRepository
	}

	// If v1.23.0  <= version <= v1.23.14 return k8s.gcr.io
	if version.GTE(semver.MustParse("1.23.0")) &&
		version.LTE(semver.MustParse("1.23.14")) {
		return OldDefaultImageRepository
	}

	// If v1.23.15 <= version < v1.24.0 return registry.k8s.io
	if version.GTE(semver.MustParse("1.23.15")) &&
		version.LT(semver.MustParse("1.24.0")) {
		return DefaultImageRepository
	}

	// If v1.24.0  <= version <= v1.24.8 return k8s.gcr.io
	if version.GTE(semver.MustParse("1.24.0")) &&
		version.LTE(semver.MustParse("1.24.7")) {
		return OldDefaultImageRepository
	}

	// If v1.24.9  <= version return registry.k8s.io
	return DefaultOCNEImageRepository
}

func constructNoProxy(noProxy, podSubnet, serviceSubnet string) string {
	localHostPresent := strings.Contains(noProxy, "localhost")
	socketPresent := strings.Contains(noProxy, DefaultOCNECSISocket)

	if noProxy == "" {
		return fmt.Sprintf("localhost,%s,%s,%s", podSubnet, serviceSubnet, DefaultOCNECSISocket)
	}

	if localHostPresent && socketPresent {
		return noProxy
	}

	if !localHostPresent && !socketPresent {
		return fmt.Sprintf("localhost,%s,%s", noProxy, DefaultOCNECSISocket)
	}

	if !localHostPresent {
		// returning since if this is true then socket must be specified in string
		return fmt.Sprintf("localhost,%s", noProxy)
	}

	// case for !socketPresent
	return fmt.Sprintf("%s,%s", noProxy, DefaultOCNECSISocket)
}

// GetOCNEOverrides Updates the cloud init with OCNE override instructions
func GetOCNEOverrides(userData *OCNEOverrideData) ([]string, error) {
	var ocneNodeOverrrides, yumOrdnfProxyOverrides, crioProxyOverrides []string
	ocneMeta, err := ocnemeta.GetOCNEMetadata(context.Background())
	if err != nil {
		return nil, err
	}
	if userData.KubernetesVersion == "" {
		userData.KubernetesVersion = K8sVersionOneTwoFourEight
	}

	if userData.Proxy != nil {
		yumOrdnfProxyOverrides = []string{
			fmt.Sprintf(`echo "proxy=%s"| sudo tee -a /etc/yum.conf`, userData.Proxy.HttpProxy),
			fmt.Sprintf(`echo "proxy=%s"| sudo tee -a /etc/dnf/dnf.conf`, userData.Proxy.HttpProxy),
		}

		// noProxy should be of type localhost,podSubnet,serviceSubnet,/var/run/shared-tmpfs/csi.sock
		crioProxyOverrides = []string{
			`mkdir -p /etc/systemd/system/crio.service.d && sudo touch /etc/systemd/system/crio.service.d/proxy.conf`,
			fmt.Sprintf(`echo -e "[Service]\nEnvironment="HTTP_PROXY=%s"\nEnvironment="HTTPS_PROXY=%s"\nEnvironment="NO_PROXY=%s""| sudo tee /etc/systemd/system/crio.service.d/proxy.conf`, userData.Proxy.HttpProxy, userData.Proxy.HttpsProxy, constructNoProxy(userData.Proxy.NoProxy, userData.PodSubnet, userData.ServiceSubnet)),
		}
	}

	ocneBasicConfig := []string{
		`sudo dd iflag=direct if=/dev/oracleoci/oraclevda of=/dev/null count=1`,
		"echo 1 | sudo tee /sys/class/block/`readlink /dev/oracleoci/oraclevda | cut -d'/' -f 2`/device/rescan",
		`sudo /usr/libexec/oci-growfs -y`,
		`sed -ri '/\sswap\s/s/^#?/#/' /etc/fstab`,
		`swapoff -a`,
		`sudo modprobe overlay && sudo modprobe br_netfilter`,
		`sudo sh -c 'echo "br_netfilter" > /etc/modules-load.d/br_netfilter.conf'`,
		`sudo sh -c 'echo -e "overlay\nbr_netfilter" | sudo tee /etc/modules-load.d/k8s.conf'`,
		`sudo sh -c 'echo -e "net.bridge.bridge-nf-call-iptables  = 1\nnet.bridge.bridge-nf-call-ip6tables = 1\nnet.ipv4.ip_forward = 1" | sudo tee /etc/sysctl.d/k8s.conf'`,
		`sudo sysctl --system`,
	}

	ocneDependenciesInstall := []string{
		`sudo dnf install -y oracle-olcne-release-el8`,
		`sudo dnf config-manager --enable ol8_olcne16 ol8_olcne15 ol8_addons ol8_baseos_latest ol8_appstream ol8_UEKR6`,
		`sudo dnf config-manager --disable ol8_olcne14 ol8_olcne13 ol8_olcne12 ol8_developer`,
		fmt.Sprintf("sudo dnf install -y kubelet-%s.el8 kubeadm-%s.el8 kubectl-%s.el8 helm-%s.el8", ocneMeta[userData.KubernetesVersion].OCNEPackages.Kubelet, ocneMeta[userData.KubernetesVersion].OCNEPackages.Kubeadm, ocneMeta[userData.KubernetesVersion].OCNEPackages.Kubectl, ocneMeta[userData.KubernetesVersion].OCNEPackages.Helm),
		`sudo dnf install -y oraclelinux-developer-release-el8 python36-oci-cli olcnectl olcne-api-server olcne-utils`,
	}

	ocneServicesStart := []string{
		fmt.Sprintf(`sudo sh -c 'echo -e "[ crio ]\n[ crio.api ]\n[ crio.image ]\npause_image = \"%s/pause:%s\"\npause_image_auth_file = \"/run/containers/0/auth.json\"\nregistries = [\"docker.io\", \"%s\"]\n[ crio.metrics ]\n[ crio.network ]\nplugin_dirs = [\"/opt/cni/bin\"]\n[crio.runtime]\ncgroup_manager = \"systemd\"\nconmon = \"/usr/bin/conmon\"\nconmon_cgroup = \"system.slice\"\nmanage_network_ns_lifecycle = true\nmanage_ns_lifecycle = true\nselinux = false\n[ crio.runtime.runtimes ]\n[ crio.runtime.runtimes.kata ]\nruntime_path = \"/usr/bin/kata-runtime\"\nruntime_type = \"oci\"\n[ crio.runtime.runtimes.runc ]\nallowed_annotations = [\"io.containers.trace-syscall\"]\nmonitor_cgroup = \"system.slice\"\nmonitor_env = [\"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin\"]\nmonitor_exec_cgroup = \"\"\nmonitor_path = \"/usr/bin/conmon\"\nprivileged_without_host_devices = false\nruntime_config_path = \"\"\nruntime_path = \"\"\nruntime_root = \"/run/runc\"\nruntime_type = \"oci\"\n[ crio.stats ]\n[ crio.tracing ]\n"| sudo tee /etc/crio/crio.conf'`, userData.OCNEImageRepository, ocneMeta[userData.KubernetesVersion].OCNEImages.Pause, userData.OCNEImageRepository),
		`sudo rm -rf /etc/cni/net.d/100-crio-bridge.conf && sudo systemctl enable crio && sudo systemctl restart crio && sudo systemctl enable kubelet`,
		`sudo systemctl disable firewalld && sudo systemctl stop firewalld`,
	}

	// This is required in the beginning to help download utilities
	if userData.Proxy != nil {
		if userData.Proxy.HttpProxy != "" || userData.Proxy.HttpsProxy != "" {
			ocneNodeOverrrides = append(ocneNodeOverrrides, yumOrdnfProxyOverrides...)
		}
	}

	ocneNodeOverrrides = append(ocneNodeOverrrides, ocneBasicConfig...)

	// if SkipInstallDependencies is set as true as we skip dependency install via cloudInit
	if !userData.SkipInstall {
		ocneNodeOverrrides = append(ocneNodeOverrrides, ocneDependenciesInstall...)
	}

	// This is required after crio is installed
	if userData.Proxy != nil {
		if userData.Proxy.HttpProxy != "" || userData.Proxy.HttpsProxy != "" {
			ocneNodeOverrrides = append(ocneNodeOverrrides, crioProxyOverrides...)
		}
	}
	return append(ocneNodeOverrrides, ocneServicesStart...), nil
}
