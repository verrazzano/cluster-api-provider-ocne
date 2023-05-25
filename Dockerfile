# Copyright 2018 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# This file from the cluster-api community (https://github.com/kubernetes-sigs/cluster-api) has been modified by Oracle.

# Build the manager binary
# Run this with docker build --build-arg builder_image=<golang:x.y.z>
ARG builder_image

# Build architecture
ARG ARCH

# Ignore Hadolint rule "Always tag the version of an image explicitly."
# It's an invalid finding since the image is explicitly set in the Makefile.
# https://github.com/hadolint/hadolint/wiki/DL3006
# hadolint ignore=DL3006
FROM ${builder_image} as builder
WORKDIR /workspace

# Run this with docker build --build-arg goproxy=$(go env GOPROXY) to override the goproxy
ARG goproxy=https://proxy.golang.org
# Run this with docker build --build-arg package=./controlplane/kubeadm or --build-arg package=./bootstrap/kubeadm
ENV GOPROXY=$goproxy

ENV GOURL=https://yum.oracle.com/repo/OracleLinux/OL8/developer/x86_64/getPackage

RUN dnf install -y oracle-olcne-release-el8 && \
    dnf config-manager --enable ol8_olcne16 && \
    dnf install -y openssl-devel delve gcc cpio yq helm-3.11.1-1.el8 tar git && \
    rpm -ivh ${GOURL}/go-toolset-1.19.4-1.module+el8.7.0+20922+47ac84ba.x86_64.rpm \
    ${GOURL}/golang-1.19.4-2.0.1.module+el8.7.0+20922+47ac84ba.x86_64.rpm \
    ${GOURL}/golang-src-1.19.4-2.0.1.module+el8.7.0+20922+47ac84ba.noarch.rpm \
    ${GOURL}/golang-bin-1.19.4-2.0.1.module+el8.7.0+20922+47ac84ba.x86_64.rpm && \
    go version && \
    curl https://yum.oracle.com/repo/OracleLinux/OL8/olcne16/x86_64/getPackage/olcne-api-server-1.6.0-4.el8.x86_64.rpm |\
      rpm2cpio |\
      cpio -idmv \
    && sed -n '/---/q;p' ./etc/olcne/modules/kubernetes/1.0.0/kubernetes.yaml|\
      yq r - 'data.versions' >./kubernetes-versions.yaml

RUN git clone https://github.com/verrazzano/verrazzano-modules.git && \
    cd verrazzano-modules/module-operator/manifests/charts/modules && \
    find . -type d -exec helm package -u '{}' \; && helm repo index .

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

# Cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the sources
COPY ./ ./

# Build
ARG package=.
ARG ARCH
ARG ldflags

# Do not force rebuild of up-to-date packages (do not use -a) and use the compiler cache folder
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${ARCH} \
    go build -trimpath -ldflags "${ldflags} -extldflags '-static'" \
    -o manager ${package}


# Production image
FROM ghcr.io/oracle/oraclelinux:8-slim
RUN microdnf update \
    && microdnf clean all


WORKDIR /
COPY --from=builder /workspace/manager .
COPY --from=builder /workspace/kubernetes-versions.yaml .
COPY --from=builder /workspace/verrazzano-modules/module-operator/manifests/charts/modules/index.yaml .
COPY --from=builder /workspace/verrazzano-modules/module-operator/manifests/charts /charts/
RUN groupadd -r ocne \
    && useradd --no-log-init -r -m -d /ocne -g ocne -u 1000 ocne \
    && mkdir -p /home/ocne \
    && chown -R 1000:ocne /manager /home/ocne \
    && chmod 500 /manager
RUN mkdir -p /license
COPY LICENSE README.md THIRD_PARTY_LICENSES.txt /license/
USER 1000
ENTRYPOINT ["/manager"]
