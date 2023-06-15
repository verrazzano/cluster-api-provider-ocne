## ⚙️  Building the Project
Intended audience: developers who want to build images locally and try out the OCNE provider in a local cluster.


```shell
git clone https://github.com/verrazzano/cluster-api-provider-ocne.git && cd $_
export MAJOR_VERSION=1
export MINOR_VERSION=6
export PATCH_VERSION=1
export TAG="<image tag>"
export REGISTRY="<image-registry>" # default is ghcr.io/verrazzano
make ocnebuild
```

These commands will create the release manifests and also push the images for the OCNE bootstrap and OCNE control plane providers to the specified registry and tag.

The release artifacts are created in the following folder structure:

```shell
release
├── bootstrap-ocne
│   └── v1.6.1
│       ├── bootstrap-components.yaml
│       └── metadata.yaml
└── control-plane-ocne
    └── v1.6.1
        ├── control-plane-components.yaml
        └── metadata.yaml
```

The above folder structure can then be referenced in the `clusterctl` configuration file `~/.cluster-api/clusterctl.yaml`.

```shell
providers:
  - name: "ocne"
    url: "${GOPATH}/src/github.com/verrazzano/cluster-api-provider-ocne/release/bootstrap-ocne/v0.6.1/bootstrap-components.yaml"
    type: "BootstrapProvider"
  - name: "ocne"
    url: "${GOPATH}/src/github.com/verrazzano/cluster-api-provider-ocne/release/control-plane-ocne/v0.6.1/control-plane-components.yaml"
    type: "ControlPlaneProvider"
```

Then initialize `clusterctl` with the locally built OCNE providers:

```
clusterctl init --bootstrap ocne --control-plane ocne -i oci:v0.9.0
```
