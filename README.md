
# Cluster API Provider for OCNE

### 👋 Welcome to our project! 

## ✨ What is OCNE?

[Oracle Cloud Native Environment](https://docs.oracle.com/en/operating-systems/olcne/) is a fully integrated suite for the development and management of cloud-native applications. Oracle Cloud Native Environment is a curated set of open source projects that are based on open standards, specifications and APIs defined by the Open Container Initiative (OCI) and Cloud Native Computing Foundation (CNCF) that can be easily deployed, have been tested for interoperability and for which enterprise-grade support is offered. Oracle Cloud Native Environment delivers a simplified framework for installations, updates, upgrades and configuration of key features for orchestrating microservices.
Started by the Kubernetes Special Interest Group (SIG) Cluster Lifecycle, the Cluster API project uses Kubernetes-style APIs and patterns to automate cluster lifecycle management for platform operators. The supporting infrastructure, like virtual machines, networks, load balancers, and VPCs, as well as the Kubernetes cluster configuration are all defined in the same way that application developers operate deploying and managing their workloads. This enables consistent and repeatable cluster deployments across a wide variety of infrastructure environments.

### ⚙️ Providers

Cluster API can be extended to support any infrastructure (AWS, Azure, vSphere, etc.), bootstrap or control plane (kubeadm is built-in) provider. There is a growing list of [supported providers](https://cluster-api.sigs.k8s.io/reference/providers.html) available.

### ⚙️ OCNE Provider

OCNE Provider for CAPI is used to bootsrap and control OCNE instances on top of major infrastructure providers like OCI, AWS and Azure. This extends the kubeadm bootstrap and  control plane provider functionality. 
