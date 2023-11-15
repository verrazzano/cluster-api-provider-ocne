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
ARG helper_image
ARG final_image
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
ARG vz_module_branch
ARG vz_module_commit
ARG vz_module_tag


# Run this with docker build --build-arg package=./controlplane/kubeadm or --build-arg package=./bootstrap/kubeadm
ENV GOPROXY=$goproxy

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

FROM ${helper_image} as helper
WORKDIR /workspace

ENV VERRAZZANO_MODULE_BRANCH=$vz_module_branch
ENV VERRAZZANO_MODULE_COMMIT=$vz_module_commit
ENV VERRAZZANO_MODULE_OPERATOR_TAG=$vz_module_tag
ENV VERRAZZANO_MODULE_DIRECTORY=verrazzano-modules

RUN dnf install -y oracle-olcne-release-el8 oraclelinux-developer-release-el8 && \
    dnf config-manager --enable ol8_olcne16 ol8_developer && \
    dnf update -y && \
    dnf install -y jq helm-3.11.1-1.el8 tar git && \
    go version

RUN git clone -b $VERRAZZANO_MODULE_BRANCH https://github.com/verrazzano/verrazzano-modules.git && \
    git -C $VERRAZZANO_MODULE_DIRECTORY checkout $VERRAZZANO_MODULE_COMMIT && \
    cd verrazzano-modules/module-operator/manifests/charts/modules && \
    find . -type d -exec helm package -u '{}' \; && helm repo index .

RUN dnf module install -y python39 && \
    dnf install -y python39-pip python39-requests && \
	yum clean all && \
	python3 -m pip install -U pip && \
	python3 -m pip install yq


# update K8s version file
RUN yq -iy ".\"1.25.7\".\"container-images\".\"module-operator\" = \"${VERRAZZANO_MODULE_OPERATOR_TAG}\"" kubernetes-versions.yaml
RUN yq -iy ".\"1.25.11\".\"container-images\".\"module-operator\" = \"${VERRAZZANO_MODULE_OPERATOR_TAG}\"" kubernetes-versions.yaml
RUN yq -iy ".\"1.26.6\".\"container-images\".\"module-operator\" = \"${VERRAZZANO_MODULE_OPERATOR_TAG}\"" kubernetes-versions.yaml


# Production image
FROM ${final_image}
RUN microdnf update \
    && microdnf clean all


WORKDIR /
COPY --from=builder /workspace/manager .
COPY --from=helper /workspace/kubernetes-versions.yaml .
COPY --from=helper /workspace/verrazzano-modules/module-operator/manifests/charts/modules/index.yaml .
COPY --from=helper /workspace/verrazzano-modules/module-operator/manifests/charts /charts/
RUN groupadd -r ocne \
    && useradd --no-log-init -r -m -d /ocne -g ocne -u 1000 ocne \
    && mkdir -p /home/ocne \
    && chown -R 1000:ocne /manager /home/ocne \
    && chmod 500 /manager
RUN mkdir -p /license
COPY LICENSE README.md THIRD_PARTY_LICENSES.txt /license/
USER 1000
ENTRYPOINT ["/manager"]
