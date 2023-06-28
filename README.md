
# Cluster API Provider for OCNE

### 👋 Welcome to our project! 

## ✨ What is OCNE?

[Oracle Cloud Native Environment](https://docs.oracle.com/en/operating-systems/olcne/) is a fully integrated suite for the development and management of cloud-native applications. Oracle Cloud Native Environment is a curated set of open source projects that are based on open standards, specifications and APIs defined by the Open Container Initiative (OCI) and Cloud Native Computing Foundation (CNCF) that can be easily deployed, have been tested for interoperability and for which enterprise-grade support is offered. Oracle Cloud Native Environment delivers a simplified framework for installations, updates, upgrades and configuration of key features for orchestrating microservices.

### ⚙️ Providers

Cluster API can be extended to support any infrastructure (AWS, Azure, vSphere, etc.), bootstrap or control plane provider (kubeadm is built in). There is a growing list of [supported providers](https://cluster-api.sigs.k8s.io/reference/providers.html) available.

### ⚙️ OCNE Provider

OCNE Provider for CAPI (CAPOCNE) is used to bootstrap and control OCNE instances on top of major infrastructure providers like OCI, AWS and Azure. This extends the kubeadm bootstrap and  control plane provider functionality.
The OCNE Provider does not use pre-baked images but rather uses vanilla OL8 images and installs the dependancies at boot time. 

### ⚙️ Cluster API Versions

CAPOCNE supports the following Cluster API versions.

|                          | Cluster API `v1beta1` (`v1.x.x`) |
|--------------------------|----------------------------------|
| OCNE Provider `(v0.x.x)` | ✓                                |


### ⚙️ CAPOCNE Operating System Support

CAPOCNE enables dynamic installation of dependencies without the need to maintain images per region. The following is the support matrix for CAPOCNE:

| Operating System | Infrastructure Provider |
|------------------|-------------------------|
| Oracle Linux 8   | CAPOCI                  |


### ⚙️ CAPOCNE Kubernetes Version Support

CAPOCNE provider follows the below support matrix with regards Kubernetes distribution. All images are hosted at `container-registry.oracle.com`. 


| K8s Version   | DNS Image Tag | ETCD Image Tag |
|---------------|---------------|----------------|
| 1.25.7        | v1.9.3        | 3.5.6          |


### ⚙️ Prerequisites

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

### ⚙️ Build OCNE Providers

Before we install the providers we need to build them from source: 

```shell
git clone https://github.com/verrazzano/cluster-api-provider-ocne.git && cd $_
export MAJOR_VERSION=0
export MINOR_VERSION=1
export PATCH_VERSION=0
export TAG="<image tag>"
export REGISTRY="<image-registry>" # default is ghcr.io/verrazzano
make ocnebuild
```

These commands will create the release manifests and also push the images for the OCNE bootstrap and OCNE control plane providers to the specified registry and tag. 

The release artifacts are created in the following folder structure: 

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

### ⚙️ Installation

* To install the OCNE providers, convert the existing KIND cluster created above into a management cluster. Update the `clusterctl` configuration file `~/.cluster-api/clusterctl.yaml` to point to the release artifacts folder:

```shell
providers:
  - name: "ocne"
    url: "${GOPATH}/src/github.com/verrazzano/cluster-api-provider-ocne/release/bootstrap-ocne/v0.1.0/bootstrap-components.yaml"
    type: "BootstrapProvider"
  - name: "ocne"
    url: "${GOPATH}/src/github.com/verrazzano/cluster-api-provider-ocne/release/control-plane-ocne/v0.1.0/control-plane-components.yaml"
    type: "ControlPlaneProvider"
```

You will now be able to initialize clusterctl with the OCNE providers:

```
clusterctl init --bootstrap ocne --control-plane ocne -i oci:v0.8.0
```

**NOTE**: Currently OCNE Provider is verified only for the OCI infrastructure provider. Follow the instructions [here](https://oracle.github.io/cluster-api-provider-oci/gs/install-cluster-api.html) for setting up OCI provider.

* Once initialization is complete pods such as the following should be running in the cluster 
```shell
capi-ocne-bootstrap-system           capi-ocne-bootstrap-controller-manager-7cd89b4bbb-bwsbs           1/1     Running   0              34m
capi-ocne-control-plane-system       capi-ocne-control-plane-controller-manager-5cdf88667f-vqq27       1/1     Running   0              34m
capi-system                          capi-controller-manager-8b477f66b-tcgcg                           1/1     Running   0              34m
cluster-api-provider-oci-system      capoci-controller-manager-5f5d9d49b5-mht2p                        1/1     Running   0              34m
```

### ⚙️ Usage 

When the OCNE bootstrap and control plane clusters are up and running you can apply cluster manifests with the desired specs to provision a cluster of your choice.
OCNE provider configuration is similar to `Kubeadm` configuration with some additional properties.  For example, the `proxy` property may be required to enable dependency installation in environmentis that have a proxy configured. For such a case, the ```spec.controlPlaneConfig.proxy``` property can be set in the `OCNEControlPlane` object and `spec.proxy.httpProxy` in the `OCNEConfigTemplate`.
You may also find useful manifests for property configuration under the [examples](./examples/) directory.

* Generate and deploy the cluster
```shell
source examples/variables.env
clusterctl generate cluster ocne-cluster --from-file examples/cluster-template-existingvcnwithaddonsandproxy.yaml | kubectl create -f -
```

* Once the cluster is deployed successfully you should see a cluster description similar to the following:
```shell
clusterctl describe cluster ocne-cluster
NAME                                                            READY  SEVERITY  REASON  SINCE  MESSAGE
Cluster/ocne-cluster                                             True                     45m
  ClusterInfrastructure - OCICluster/ocne-cluster                True                     56m
  ControlPlane - KubeadmControlPlane/ocne-cluster-control-plane  True                     45m
    Machine/ocne-cluster-control-plane-z47bj                     True                     55m
  Workers
    MachineDeployment/ocne-cluster-md-0                          True                     36m
      Machine/ocne-cluster-md-0-846df89cb4-dbrdn                 True                     44m
```


