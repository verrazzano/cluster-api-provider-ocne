
# Cluster API Provider for OCNE

### 👋 Welcome to our project! 

## ✨ What is OCNE?

[Oracle Cloud Native Environment](https://docs.oracle.com/en/operating-systems/olcne/) is a fully integrated suite for the development and management of cloud-native applications. Oracle Cloud Native Environment is a curated set of open source projects that are based on open standards, specifications and APIs defined by the Open Container Initiative (OCI) and Cloud Native Computing Foundation (CNCF) that can be easily deployed, have been tested for interoperability and for which enterprise-grade support is offered. Oracle Cloud Native Environment delivers a simplified framework for installations, updates, upgrades and configuration of key features for orchestrating microservices.
Started by the Kubernetes Special Interest Group (SIG) Cluster Lifecycle, the Cluster API project uses Kubernetes-style APIs and patterns to automate cluster lifecycle management for platform operators. The supporting infrastructure, like virtual machines, networks, load balancers, and VPCs, as well as the Kubernetes cluster configuration are all defined in the same way that application developers operate deploying and managing their workloads. This enables consistent and repeatable cluster deployments across a wide variety of infrastructure environments.

### ⚙️ Providers

Cluster API can be extended to support any infrastructure (AWS, Azure, vSphere, etc.), bootstrap or control plane (kubeadm is built-in) provider. There is a growing list of [supported providers](https://cluster-api.sigs.k8s.io/reference/providers.html) available.

### ⚙️ OCNE Provider

OCNE Provider for CAPI is used to bootstrap and control OCNE instances on top of major infrastructure providers like OCI, AWS and Azure. This extends the kubeadm bootstrap and  control plane provider functionality.
The OCNE Provider does use pre-baked images. Instead it uses vanilla OL8 images and installs the dependancies at boot time. 

### Prerequisites

* Install clusterctl following the [upstream instructions](https://cluster-api.sigs.k8s.io/user/quick-start.html#install-clusterctl)
```
curl -L https://github.com/kubernetes-sigs/cluster-api/releases/download/v1.3.3/clusterctl-linux-amd64 -o clusterctl
```

* Install a Kubernetes cluster using KinD:
```
cat > kind-cluster-with-extramounts.yaml <<EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  extraMounts:
    - hostPath: /var/run/docker.sock
      containerPath: /var/run/docker.sock
EOF

kind create cluster --config kind-cluster-with-extramounts.yaml
```

### Build OCNE Providers

Before we install the providers we need to build them from source. 

```shell
git clone https://github.com/verrazzano/cluster-api-provider-ocne.git && cd $_
export MAJOR_VERSION=0
export MINOR_VERSION=1
export PATCH_VERSION=0
export TAG="<image tag>"
export REGISTRY="<image-registry>" # default is ghcr.io/verrazzano
make ocnebuild
```

This will create the release manifests and also push the images for OCNE bootstrap and OCNE control plane providers to the specified registry and tag. 

The release artifacts are created in the following folder structure 

```shell
release
├── bootstrap-ocne
│   └── v0.1.0
│       ├── bootstrap-components.yaml
│       └── metadata.yaml
└── control-plane-ocne
    └── v0.1.0
        ├── control-plane-components.yaml
        └── metadata.yaml
```

### Installation

* To install the OCNE providers, convert the existing cluster into a management cluster. Update the `clusterctl` configuration file `~/.cluster-api/clusterctl.yaml` to point to the release artifacts folder

```shell
providers:
  - name: "ocne"
    url: "${GOPATH}/github.com/verrazzano/cluster-api-provider-ocne/release/bootstrap-ocne/v0.1.0/bootstrap-components.yaml"
    type: "BootstrapProvider"
  - name: "ocne"
    url: "${GOPATH}/github.com/verrazzano/cluster-api-provider-ocne/release/control-plane-ocne/v0.1.0/control-plane-components.yaml"
    type: "ControlPlaneProvider"
```

You will now be able now to initialize clusterctl with the OCNE providers:

```
clusterctl init --bootstrap ocne --control-plane ocne -i oci:v0.8.0
```

**NOTE**: Currently OCNE Provider is verified only for OCI infrastructure provider. Follow the instructions [here](https://oracle.github.io/cluster-api-provider-oci/gs/install-cluster-api.html) for setting up OCI provider.

* Once initialization is complete the following pods should be seen on the cluster 
```shell
capi-ocne-bootstrap-system           capi-ocne-bootstrap-controller-manager-7cd89b4bbb-bwsbs           1/1     Running   0              34m
capi-ocne-control-plane-system       capi-ocne-control-plane-controller-manager-5cdf88667f-vqq27       1/1     Running   0              34m
capi-system                          capi-controller-manager-8b477f66b-tcgcg                           1/1     Running   0              34m
cluster-api-provider-oci-system      capoci-controller-manager-5f5d9d49b5-mht2p                        1/1     Running   0              34m
```

### Usage 

When the OCNE bootstrap and control plane clusters are up and running you can apply cluster manifests with the desired specs  to provision a cluster of your choice.
OCNE provider configuration is similar to `Kubeadm` configuration with some additional properties like `proxy`, that is required for dependency install, in case the environment has it configured. Proxy can be set ```via spec.controlPlaneConfig.proxy``` in the `OCNEControlPlane` object and `spec.proxy.httpProxy` in `OCNEConfigTemplate`
You may also find useful the manifests found under the [templates](./templates/) directory.

* Generate and deploy the cluster
```shell
source templates/variables.env
clusterctl generate cluster ocne-cluster --from-file templates/cluster-template-existingvcnwithaddonsandproxy.yaml | kubeactl apply -f -
```

* Once cluster is deployed successfully you should see the following output
```shell
clusterctl describe cluster ocne-cluster
NAME                                                            READY  SEVERITY  REASON  SINCE  MESSAGE
Cluster/ocne-cluster                                             True                     45m
¿¿ClusterInfrastructure - OCICluster/ocne-cluster                True                     56m
¿¿ControlPlane - KubeadmControlPlane/ocne-cluster-control-plane  True                     45m
¿ ¿¿Machine/ocne-cluster-control-plane-z47bj                     True                     55m
¿¿Workers
  ¿¿MachineDeployment/ocne-cluster-md-0                          True                     36m
    ¿¿Machine/ocne-cluster-md-0-846df89cb4-dbrdn                 True                     44m
```


