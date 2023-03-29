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

// Package kubeadm contains utils related to kubeadm.
package ocne

import (
	"fmt"
	"github.com/blang/semver"
	bootstrapv1 "github.com/verrazzano/cluster-api-provider-ocne/bootstrap/ocne/api/v1beta1"
	"regexp"
	"strings"
)

const (
	// DefaultImageRepository is the new default Kubernetes image registry.
	DefaultImageRepository = "registry.k8s.io"
	// OldDefaultImageRepository is the old default Kubernetes image registry.
	OldDefaultImageRepository = "k8s.gcr.io"
	// DefaultOCNEImageRepository is the default ocne image repository
	DefaultOCNEImageRepository = "container-registry.oracle.com/olcne"
)

var (
	// MinKubernetesVersionImageRegistryMigration is the first Kubernetes minor version which
	// has patch versions where the default image registry in kubeadm is registry.k8s.io instead of k8s.gcr.io.
	MinKubernetesVersionImageRegistryMigration = semver.MustParse("1.22.0")

	// NextKubernetesVersionImageRegistryMigration is the next minor version after
	// the default image registry in kubeadm changed to registry.k8s.io.
	NextKubernetesVersionImageRegistryMigration = semver.MustParse("1.26.0")
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
		version.LTE(semver.MustParse("1.24.8")) {
		return OldDefaultImageRepository
	}

	// If v1.24.9  <= version return registry.k8s.io
	return DefaultImageRepository
}

func constructNoProxy(noProxy, podSubnet, serviceSubnet string) string {
	localHostPresent := strings.Contains(noProxy, "localhost")
	socketPresent := strings.Contains(noProxy, "/var/run/shared-tmpfs/csi.sock")

	if noProxy == "" {
		return fmt.Sprintf("localhost,%s,%s,/var/run/shared-tmpfs/csi.sock", podSubnet, serviceSubnet)
	}

	if localHostPresent && socketPresent {
		return noProxy
	}

	if !localHostPresent && !socketPresent {
		return fmt.Sprintf("localhost,%s,/var/run/shared-tmpfs/csi.sock", noProxy)
	}

	if !localHostPresent {
		// returning since if this is true then socket must be specified in string
		return fmt.Sprintf("localhost,%s", noProxy)
	}

	// case for !socketPresent
	return fmt.Sprintf("%s,/var/run/shared-tmpfs/csi.sock", noProxy)
}

func GetOCNEOverrides(kubernetesVersion, ocneImageRepo, podSubnet, serviceSubnet string, proxy *bootstrapv1.ProxySpec) []string {
	var ocneNodeOverrrides []string
	k8sversion := extractVersionString(kubernetesVersion)
	yumOrdnfProxyOverrides := []string{
		fmt.Sprintf(`echo "proxy=%s"| sudo tee -a /etc/yum.conf`, proxy.HttpProxy),
		fmt.Sprintf(`echo "proxy=%s"| sudo tee -a /etc/dnf/dnf.conf`, proxy.HttpProxy),
	}

	// noProxy should be of type localhost,podSubnet,serviceSubnet,/var/run/shared-tmpfs/csi.sock
	crioProxyOverrides := []string{
		`mkdir -p /etc/systemd/system/crio.service.d && sudo touch /etc/systemd/system/crio.service.d/proxy.conf`,
		fmt.Sprintf(`echo -e "[Service]\nEnvironment="HTTP_PROXY=%s"\nEnvironment="HTTPS_PROXY=%s"\nEnvironment="NO_PROXY=%s""| sudo tee /etc/systemd/system/crio.service.d/proxy.conf`, proxy.HttpProxy, proxy.HttpsProxy, constructNoProxy(proxy.NoProxy, podSubnet, serviceSubnet)),
	}

	ocneUtilsInstall := []string{
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
		`sudo dnf install -y oracle-olcne-release-el8`,
		`sudo dnf config-manager --enable ol8_olcne15 ol8_addons ol8_baseos_latest ol8_appstream ol8_UEKR6`,
		`sudo dnf config-manager --disable ol8_olcne14 ol8_olcne13 ol8_olcne12 ol8_developer`,
		fmt.Sprintf("sudo dnf install -y kubelet-%s-1.el8 kubeadm-%s-1.el8 kubectl-%s-1.el8", k8sversion, k8sversion, k8sversion),
		`sudo dnf install -y oraclelinux-developer-release-el8 python36-oci-cli olcnectl olcne-api-server olcne-utils`,
		fmt.Sprintf(`sudo sh -c 'echo -e "[ crio.image ]\nregistries = [\"docker.io\", \"%s\" ]\n[ crio.network ]\nplugin_dirs = [\"/opt/cni/bin\"]\n[ crio.runtime ]\ncgroup_manager = \"systemd\"\nconmon = \"/usr/bin/conmon\"\nconmon_cgroup = \"system.slice\"\nmanage_network_ns_lifecycle = true\nmanage_ns_lifecycle = true\nselinux = true\n[ crio.runtime.runtimes ]\n[ crio.runtime.runtimes.kata ]\nruntime_path = \"/usr/bin/kata-runtime\"\nruntime_type = \"oci\""| sudo tee /etc/crio/crio.conf'`, ocneImageRepo),
	}

	ocneServicesStart := []string{
		`sudo systemctl enable crio && sudo systemctl restart crio && sudo systemctl enable kubelet`,
		`sudo systemctl disable firewalld && sudo systemctl stop firewalld`,
	}

	// This is required in the beginning to help download utilities
	if proxy != nil {
		ocneNodeOverrrides = append(ocneNodeOverrrides, yumOrdnfProxyOverrides...)
	}

	ocneNodeOverrrides = append(ocneNodeOverrrides, ocneUtilsInstall...)

	// This is required after crio is installed
	if proxy != nil {
		ocneNodeOverrrides = append(ocneNodeOverrrides, crioProxyOverrides...)
	}

	return append(ocneNodeOverrrides, ocneServicesStart...)
}

func extractVersionString(version string) string {
	re := regexp.MustCompile(`\d[\d]*`)
	submatchall := re.FindAllString(version, -1)
	var newversion []string
	for _, element := range submatchall {
		newversion = append(newversion, element)
	}
	return strings.Join(newversion, ".")
}
