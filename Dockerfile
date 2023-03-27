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

RUN yum update -y \
    && yum-config-manager --save --setopt=ol7_ociyum_config.skip_if_unavailable=true \
    && yum install -y oracle-golang-release-el7 \
    && yum-config-manager --disable ol7_developer_golang\* \
    && yum-config-manager --enable ol7_developer_golang119 \
    && yum -y install golang \
    && yum clean all \
    && go version

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

# Cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Copy the sources
COPY ./ ./

# Build
ARG package=.
ARG ARCH
ARG ldflags

# Do not force rebuild of up-to-date packages (do not use -a) and use the compiler cache folder
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux GOARCH=${ARCH} \
    go build -trimpath -ldflags "${ldflags} -extldflags '-static'" \
    -o manager ${package}

# Production image
FROM ghcr.io/oracle/oraclelinux:7-slim
RUN yum update -y \
    && yum install -y openssl \
    && yum clean all \
    && rm -rf /var/cache/yum
WORKDIR /
COPY --from=builder /workspace/manager .
RUN groupadd -r ocne \
    && useradd --no-log-init -r -m -d /ocne -g ocne -u 1000 ocne \
    && mkdir -p /home/ocne \
    && chown -R 1000:ocne /manager /home/ocne \
    && chmod 500 /manager
USER 1000
ENTRYPOINT ["/manager"]
