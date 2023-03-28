<a href="https://cluster-api.sigs.k8s.io"><img alt="capi" src="./logos/kubernetes-cluster-logos_final-02.svg" width="160x" /></a>
<p>
<a href="https://godoc.org/github.com/verrazzano/cluster-api-provider-ocne"><img src="https://godoc.org/github.com/verrazzano/cluster-api-provider-ocne?status.svg"></a>
<!-- join kubernetes slack channel for cluster-api -->
<a href="http://slack.k8s.io/">
<img src="https://img.shields.io/badge/join%20slack-%23cluster--api-brightgreen"></a>
</p>

# Cluster API

### 👋 Welcome to our project! 

## ✨ What is the Cluster API?

Cluster API is a Kubernetes subproject focused on providing declarative APIs and tooling to simplify provisioning, upgrading, and operating multiple Kubernetes clusters.

Started by the Kubernetes Special Interest Group (SIG) Cluster Lifecycle, the Cluster API project uses Kubernetes-style APIs and patterns to automate cluster lifecycle management for platform operators. The supporting infrastructure, like virtual machines, networks, load balancers, and VPCs, as well as the Kubernetes cluster configuration are all defined in the same way that application developers operate deploying and managing their workloads. This enables consistent and repeatable cluster deployments across a wide variety of infrastructure environments.

### ⚙️ Providers

Cluster API can be extended to support any infrastructure (AWS, Azure, vSphere, etc.), bootstrap or control plane (kubeadm is built-in) provider. There is a growing list of [supported providers](https://cluster-api.sigs.k8s.io/reference/providers.html) available.


### ⚙️ OCNE Provider

We have developed [OCNE](https://docs.oracle.com/en/operating-systems/olcne/) for CAPI to be used with major infrastructure providers like OCI, AWS and Azure. This extends the kubeadm bootstrap and  control plane provider functionality. 
