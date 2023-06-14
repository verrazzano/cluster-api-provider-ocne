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

# If you update this file, please follow
# https://www.thapaliya.com/en/writings/well-documented-makefiles/

# This file from the cluster-api community (https://github.com/kubernetes-sigs/cluster-api) has been modified by Oracle.

# Ensure Make is run with bash shell as some syntax below is bash-specific
SHELL:=/usr/bin/env bash

.DEFAULT_GOAL:=help

#
# Go.
#
GO_CONTAINER_IMAGE ?= ghcr.io/oracle/oraclelinux:8
# Use GOPROXY environment variable if set
GOPROXY := $(shell go env GOPROXY)
ifeq ($(GOPROXY),)
GOPROXY := https://proxy.golang.org
endif
export GOPROXY

# Active module mode, as we use go modules to manage dependencies
export GO111MODULE=on

#
# Kubebuilder.
#
export KUBEBUILDER_ENVTEST_KUBERNETES_VERSION ?= 1.26.0
export KUBEBUILDER_CONTROLPLANE_START_TIMEOUT ?= 60s
export KUBEBUILDER_CONTROLPLANE_STOP_TIMEOUT ?= 60s

# This option is for running docker manifest command
export DOCKER_CLI_EXPERIMENTAL := enabled

# Enables shell script tracing. Enable by running: TRACE=1 make <target>
TRACE ?= 0

#
# Directories.
#
# Full directory of where the Makefile resides
ROOT_DIR:=$(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))
EXP_DIR := exp
BIN_DIR := bin
TEST_DIR := test
TOOLS_DIR := hack/tools
TOOLS_BIN_DIR := $(abspath $(TOOLS_DIR)/$(BIN_DIR))
E2E_FRAMEWORK_DIR := $(TEST_DIR)/framework
CAPD_DIR := $(TEST_DIR)/infrastructure/docker
TEST_EXTENSION_DIR := $(TEST_DIR)/extension
GO_INSTALL := ./scripts/go_install.sh
OBSERVABILITY_DIR := hack/observability

export PATH := $(abspath $(TOOLS_BIN_DIR)):$(PATH)

# Set --output-base for conversion-gen if we are not within GOPATH
ifneq ($(abspath $(ROOT_DIR)),$(shell go env GOPATH)/src/github.com/verrazzano/cluster-api-provider-ocne)
	CONVERSION_GEN_OUTPUT_BASE := --output-base=$(ROOT_DIR)
	CONVERSION_GEN_OUTPUT_BASE_CAPD := --output-base=$(ROOT_DIR)/$(CAPD_DIR)
else
	export GOPATH := $(shell go env GOPATH)
endif

#
# Ginkgo configuration.
#
GINKGO_FOCUS ?=
GINKGO_SKIP ?=
GINKGO_NODES ?= 1
GINKGO_TIMEOUT ?= 2h
GINKGO_POLL_PROGRESS_AFTER ?= 60m
GINKGO_POLL_PROGRESS_INTERVAL ?= 5m
E2E_CONF_FILE ?= $(ROOT_DIR)/$(TEST_DIR)/e2e/config/docker.yaml
SKIP_RESOURCE_CLEANUP ?= false
USE_EXISTING_CLUSTER ?= false
GINKGO_NOCOLOR ?= false

# to set multiple ginkgo skip flags, if any
ifneq ($(strip $(GINKGO_SKIP)),)
_SKIP_ARGS := $(foreach arg,$(strip $(GINKGO_SKIP)),-skip="$(arg)")
endif

#
# Binaries.
#
# Note: Need to use abspath so we can invoke these from subdirectories
KUSTOMIZE_VER := v4.5.2
KUSTOMIZE_BIN := kustomize
KUSTOMIZE := $(abspath $(TOOLS_BIN_DIR)/$(KUSTOMIZE_BIN)-$(KUSTOMIZE_VER))
KUSTOMIZE_PKG := sigs.k8s.io/kustomize/kustomize/v4

SETUP_ENVTEST_VER := v0.0.0-20211110210527-619e6b92dab9
SETUP_ENVTEST_BIN := setup-envtest
SETUP_ENVTEST := $(abspath $(TOOLS_BIN_DIR)/$(SETUP_ENVTEST_BIN)-$(SETUP_ENVTEST_VER))
SETUP_ENVTEST_PKG := sigs.k8s.io/controller-runtime/tools/setup-envtest

CONTROLLER_GEN_VER := v0.10.0
CONTROLLER_GEN_BIN := controller-gen
CONTROLLER_GEN := $(abspath $(TOOLS_BIN_DIR)/$(CONTROLLER_GEN_BIN)-$(CONTROLLER_GEN_VER))
CONTROLLER_GEN_PKG := sigs.k8s.io/controller-tools/cmd/controller-gen

GOTESTSUM_VER := v1.6.4
GOTESTSUM_BIN := gotestsum
GOTESTSUM := $(abspath $(TOOLS_BIN_DIR)/$(GOTESTSUM_BIN)-$(GOTESTSUM_VER))
GOTESTSUM_PKG := gotest.tools/gotestsum

CONVERSION_GEN_VER := v0.25.0
CONVERSION_GEN_BIN := conversion-gen
# We are intentionally using the binary without version suffix, to avoid the version
# in generated files.
CONVERSION_GEN := $(abspath $(TOOLS_BIN_DIR)/$(CONVERSION_GEN_BIN))
CONVERSION_GEN_PKG := k8s.io/code-generator/cmd/conversion-gen

ENVSUBST_VER := v2.0.0-20210730161058-179042472c46
ENVSUBST_BIN := envsubst
ENVSUBST := $(abspath $(TOOLS_BIN_DIR)/$(ENVSUBST_BIN)-$(ENVSUBST_VER))
ENVSUBST_PKG := github.com/drone/envsubst/v2/cmd/envsubst

GO_APIDIFF_VER := v0.5.0
GO_APIDIFF_BIN := go-apidiff
GO_APIDIFF := $(abspath $(TOOLS_BIN_DIR)/$(GO_APIDIFF_BIN)-$(GO_APIDIFF_VER))
GO_APIDIFF_PKG := github.com/joelanford/go-apidiff

SHELLCHECK_VER := v0.9.0

KPROMO_VER := v3.4.5
KPROMO_BIN := kpromo
KPROMO :=  $(abspath $(TOOLS_BIN_DIR)/$(KPROMO_BIN)-$(KPROMO_VER))
KPROMO_PKG := sigs.k8s.io/promo-tools/v3/cmd/kpromo


GINGKO_VER := v2.5.0
GINKGO_BIN := ginkgo
GINKGO := $(abspath $(TOOLS_BIN_DIR)/$(GINKGO_BIN)-$(GINGKO_VER))
GINKGO_PKG := github.com/onsi/ginkgo/v2/ginkgo

CONVERSION_VERIFIER_BIN := conversion-verifier
CONVERSION_VERIFIER := $(abspath $(TOOLS_BIN_DIR)/$(CONVERSION_VERIFIER_BIN))

OPENAPI_GEN_VER := 5e7f5fd
OPENAPI_GEN_BIN := openapi-gen
# We are intentionally using the binary without version suffix, to avoid the version
# in generated files.
OPENAPI_GEN := $(abspath $(TOOLS_BIN_DIR)/$(OPENAPI_GEN_BIN))
OPENAPI_GEN_PKG := k8s.io/kube-openapi/cmd/openapi-gen

RUNTIME_OPENAPI_GEN_BIN := runtime-openapi-gen
RUNTIME_OPENAPI_GEN := $(abspath $(TOOLS_BIN_DIR)/$(RUNTIME_OPENAPI_GEN_BIN))


GOLANGCI_LINT_BIN := golangci-lint
GOLANGCI_LINT := $(abspath $(TOOLS_BIN_DIR)/$(GOLANGCI_LINT_BIN))

# Define Docker related variables. Releases should modify and double check these vars.
REGISTRY ?= ghcr.io/verrazzano
# core
IMAGE_NAME ?= cluster-api-controller
CONTROLLER_IMG ?= $(REGISTRY)/$(IMAGE_NAME)

# bootstrap
OCNE_BOOTSTRAP_IMAGE_NAME ?= cluster-api-ocne-bootstrap-controller
OCNE_BOOTSTRAP_CONTROLLER_IMG ?= $(REGISTRY)/$(OCNE_BOOTSTRAP_IMAGE_NAME)

# control plane
OCNE_CONTROL_PLANE_IMAGE_NAME ?= cluster-api-ocne-control-plane-controller
OCNE_CONTROL_PLANE_CONTROLLER_IMG ?= $(REGISTRY)/$(OCNE_CONTROL_PLANE_IMAGE_NAME)


# test extension
TEST_EXTENSION_IMAGE_NAME ?= test-extension
TEST_EXTENSION_IMG ?= $(REGISTRY)/$(TEST_EXTENSION_IMAGE_NAME)


# It is set by Prow GIT_TAG, a git-based tag of the form vYYYYMMDD-hash, e.g., v20210120-v0.3.10-308-gc61521971

MAJOR_VERSION ?= 1
MINOR_VERSION ?= 6
PATCH_VERSION ?= 1
SHORT_COMMIT_SHA := $(shell git rev-parse --short HEAD)
TAG ?= v${MAJOR_VERSION}.${MINOR_VERSION}.${PATCH_VERSION}-${SHORT_COMMIT_SHA}
ARCH ?= $(shell go env GOARCH)
ALL_ARCH = amd64

# Allow overriding the imagePullPolicy
PULL_POLICY ?= Always

# Hosts running SELinux need :z added to volume mounts
SELINUX_ENABLED := $(shell cat /sys/fs/selinux/enforce 2> /dev/null || echo 0)

ifeq ($(SELINUX_ENABLED),1)
  DOCKER_VOL_OPTS?=:z
endif

# Set build time variables including version details
LDFLAGS := $(shell hack/version.sh)

# Branch for obtaining module charts
VERRAZZANO_MODULE_BRANCH ?= release/1.6
VERRAZZANO_MODULE_COMMIT ?= f0808d7d525b20bbe2c8b575635d15fde343f906
VERRAZZANO_MODULE_OPERATOR_TAG ?= v0.1.0-20230613202926-e88f7d2b

all: test managers

help:  # Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[0-9A-Za-z_-]+:.*?##/ { printf "  \033[36m%-45s\033[0m %s\n", $$1, $$2 } /^\$$\([0-9A-Za-z_-]+\):.*?##/ { gsub("_","-", $$1); printf "  \033[36m%-45s\033[0m %s\n", tolower(substr($$1, 3, length($$1)-7)), $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

## --------------------------------------
## Generate / Manifests
## --------------------------------------

##@ generate:

ALL_GENERATE_MODULES = ocne-bootstrap ocne-control-plane

.PHONY: generate
generate: ## Run all generate-manifests-*, generate-go-deepcopy-*
	$(MAKE) generate-modules generate-manifests generate-go-deepcopy

.PHONY: generate-manifests
generate-manifests: $(addprefix generate-manifests-,$(ALL_GENERATE_MODULES)) ## Run all generate-manifests-* targets

.PHONY: generate-manifests-ocne-bootstrap
generate-manifests-ocne-bootstrap: $(CONTROLLER_GEN) ## Generate manifests e.g. CRD, RBAC etc. for ocne bootstrap
	$(MAKE) clean-generated-yaml SRC_DIRS="./bootstrap/ocne/config/crd/bases"
	$(CONTROLLER_GEN) \
		paths=./bootstrap/ocne/api/... \
		paths=./bootstrap/ocne/internal/controllers/... \
		crd:crdVersions=v1 \
		rbac:roleName=manager-role \
		output:crd:dir=./bootstrap/ocne/config/crd/bases \
		output:rbac:dir=./bootstrap/ocne/config/rbac \
		output:webhook:dir=./bootstrap/ocne/config/webhook \
		webhook

.PHONY: generate-manifests-ocne-control-plane
generate-manifests-ocne-control-plane: $(CONTROLLER_GEN) ## Generate manifests e.g. CRD, RBAC etc. for ocne control plane
	$(MAKE) clean-generated-yaml SRC_DIRS="./controlplane/ocne/config/crd/bases"
	$(CONTROLLER_GEN) \
		paths=./controlplane/ocne/api/... \
		paths=./controlplane/ocne/internal/controllers/... \
		paths=./controlplane/ocne/internal/webhooks/... \
		crd:crdVersions=v1 \
		rbac:roleName=manager-role \
		output:crd:dir=./controlplane/ocne/config/crd/bases \
		output:rbac:dir=./controlplane/ocne/config/rbac \
		output:webhook:dir=./controlplane/ocne/config/webhook \
		webhook

.PHONY: generate-go-deepcopy
generate-go-deepcopy:  ## Run all generate-go-deepcopy-* targets
	$(MAKE) $(addprefix generate-go-deepcopy-,$(ALL_GENERATE_MODULES))

.PHONY: generate-go-deepcopy-ocne-bootstrap
generate-go-deepcopy-ocne-bootstrap: $(CONTROLLER_GEN) ## Generate deepcopy go code for ocne bootstrap
	$(MAKE) clean-generated-deepcopy SRC_DIRS="./bootstrap/ocne/api,./bootstrap/ocne/types"
	$(CONTROLLER_GEN) \
		object:headerFile=./hack/boilerplate/boilerplate.generatego.txt \
		paths=./bootstrap/ocne/api/... \
		paths=./bootstrap/ocne/types/...

.PHONY: generate-go-deepcopy-ocne-control-plane
generate-go-deepcopy-ocne-control-plane: $(CONTROLLER_GEN) ## Generate deepcopy go code for ocne control plane
	$(MAKE) clean-generated-deepcopy SRC_DIRS="./controlplane/ocne/api"
	$(CONTROLLER_GEN) \
		object:headerFile=./hack/boilerplate/boilerplate.generatego.txt \
		paths=./controlplane/ocne/api/...

.PHONY: generate-modules
generate-modules: ## Run go mod tidy to ensure modules are up to date
	go mod tidy

## --------------------------------------
## Lint / Verify
## --------------------------------------

##@ lint and verify:

.PHONY: lint
lint: $(GOLANGCI_LINT) ## Lint the codebase
	$(GOLANGCI_LINT) run -v $(GOLANGCI_LINT_EXTRA_ARGS)
	cd $(TEST_DIR); $(GOLANGCI_LINT) run --path-prefix $(TEST_DIR) -v $(GOLANGCI_LINT_EXTRA_ARGS)
	cd $(TOOLS_DIR); $(GOLANGCI_LINT) run --path-prefix $(TOOLS_DIR) -v $(GOLANGCI_LINT_EXTRA_ARGS)


.PHONY: lint-fix
lint-fix: $(GOLANGCI_LINT) ## Lint the codebase and run auto-fixers if supported by the linter
	GOLANGCI_LINT_EXTRA_ARGS=--fix $(MAKE) lint


ALL_VERIFY_CHECKS = modules gen

.PHONY: verify
verify: $(addprefix verify-,$(ALL_VERIFY_CHECKS))  ## Run all verify-* targets

.PHONY: verify-modules
verify-modules: generate-modules  ## Verify go modules are up to date
	@if !(git diff --quiet HEAD -- go.sum go.mod $(TOOLS_DIR)/go.mod $(TOOLS_DIR)/go.sum $(TEST_DIR)/go.mod $(TEST_DIR)/go.sum); then \
		git diff; \
		echo "go module files are out of date"; exit 1; \
	fi
	@if (find . -name 'go.mod' | xargs -n1 grep -q -i 'k8s.io/client-go.*+incompatible'); then \
		find . -name "go.mod" -exec grep -i 'k8s.io/client-go.*+incompatible' {} \; -print; \
		echo "go module contains an incompatible client-go version"; exit 1; \
	fi

.PHONY: verify-gen
verify-gen: generate  ## Verify go generated files are up to date
	@if !(git diff --quiet HEAD); then \
		git diff; \
		echo "generated files are out of date, run make generate"; exit 1; \
	fi


## --------------------------------------
## Binaries
## --------------------------------------

##@ build:

ALL_MANAGERS = ocne-bootstrap ocne-control-plane

.PHONY: managers
managers: $(addprefix manager-,$(ALL_MANAGERS)) ## Run all manager-* targets

.PHONY: manager-ocne-bootstrap
manager-ocne-bootstrap: ## Build the ocne bootstrap manager binary into the ./bin folder
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/ocne-bootstrap-manager github.com/verrazzano/cluster-api-provider-ocne/bootstrap/ocne

.PHONY: manager-ocne-control-plane
manager-ocne-control-plane: ## Build the ocne control plane manager binary into the ./bin folder
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/ocne-control-plane-manager github.com/verrazzano/cluster-api-provider-ocne/controlplane/ocne

.PHONY: docker-pull-prerequisites
docker-pull-prerequisites:
	docker pull ghcr.io/oracle/oraclelinux:8
	docker pull ghcr.io/oracle/oraclelinux:8-slim

.PHONY: generate-modules-repo-artifacts
generate-modules-repo-artifacts:
	git clone https://github.com/verrazzano/verrazzano-modules.git
	cd $(ROOT_DIR)/verrazzano-modules/module-operator/manifests/charts/modules; \
    git checkout $(VERRAZZANO_MODULE_BRANCH); \
 	find . -type d -exec helm package -u '{}' \; && helm repo index . ; \
 	mv $(ROOT_DIR)/verrazzano-modules/module-operator/manifests/charts/modules/index.yaml $(ROOT_DIR); \
 	cp -R $(ROOT_DIR)/verrazzano-modules/module-operator/manifests/charts $(ROOT_DIR); \
 	rm -rf $(ROOT_DIR)/charts/modules/*.tgz
	rm -rf verrazzano-modules

.PHONY: docker-build-all
docker-build-all: $(addprefix docker-build-,$(ALL_ARCH)) ## Build docker images for all architectures

docker-build-%:
	$(MAKE) ARCH=$* docker-build

ALL_DOCKER_BUILD = ocne-bootstrap ocne-control-plane

.PHONY: docker-build
docker-build: docker-pull-prerequisites ## Run docker-build-* targets for all the images
	$(MAKE) ARCH=$(ARCH) $(addprefix docker-build-,$(ALL_DOCKER_BUILD))

ALL_DOCKER_BUILD_E2E = ocne-bootstrap ocne-control-plane

.PHONY: docker-build-ocne-bootstrap
docker-build-ocne-bootstrap: ## Build the docker image for ocne bootstrap controller manager
	docker build --build-arg builder_image=$(GO_CONTAINER_IMAGE) --build-arg goproxy=$(GOPROXY) --build-arg ARCH=$(ARCH) --build-arg vz_module_branch=$(VERRAZZANO_MODULE_BRANCH) --build-arg vz_module_commit=$(VERRAZZANO_MODULE_COMMIT) --build-arg vz_module_tag=$(VERRAZZANO_MODULE_OPERATOR_TAG) --build-arg package=./bootstrap/ocne --build-arg ldflags="$(LDFLAGS)" . -t $(OCNE_BOOTSTRAP_CONTROLLER_IMG):$(TAG)
	$(MAKE) set-manifest-image MANIFEST_IMG=$(OCNE_BOOTSTRAP_CONTROLLER_IMG) MANIFEST_TAG=$(TAG) TARGET_RESOURCE="./bootstrap/ocne/config/default/manager_image_patch.yaml"
	$(MAKE) set-manifest-pull-policy TARGET_RESOURCE="./bootstrap/ocne/config/default/manager_pull_policy.yaml"

.PHONY: docker-build-ocne-control-plane
docker-build-ocne-control-plane: ## Build the docker image for ocne control plane controller manager
	docker build --build-arg builder_image=$(GO_CONTAINER_IMAGE) --build-arg goproxy=$(GOPROXY) --build-arg ARCH=$(ARCH) --build-arg vz_module_branch=$(VERRAZZANO_MODULE_BRANCH) --build-arg vz_module_commit=$(VERRAZZANO_MODULE_COMMIT) --build-arg vz_module_tag=$(VERRAZZANO_MODULE_OPERATOR_TAG) --build-arg package=./controlplane/ocne --build-arg ldflags="$(LDFLAGS)" . -t $(OCNE_CONTROL_PLANE_CONTROLLER_IMG):$(TAG)
	$(MAKE) set-manifest-image MANIFEST_IMG=$(OCNE_CONTROL_PLANE_CONTROLLER_IMG) MANIFEST_TAG=$(TAG) TARGET_RESOURCE="./controlplane/ocne/config/default/manager_image_patch.yaml"
	$(MAKE) set-manifest-pull-policy TARGET_RESOURCE="./controlplane/ocne/config/default/manager_pull_policy.yaml"

## --------------------------------------
## Testing
## --------------------------------------

##@ test:

ARTIFACTS ?= ${ROOT_DIR}/_artifacts

KUBEBUILDER_ASSETS  ?= $(shell $(SETUP_ENVTEST) use --use-env -p path $(KUBEBUILDER_ENVTEST_KUBERNETES_VERSION))

.PHONY: test
test: $(SETUP_ENVTEST) ## Run unit and integration tests
	KUBEBUILDER_ASSETS="$(KUBEBUILDER_ASSETS)" go test ./... $(TEST_ARGS)

.PHONY: test-verbose
test-verbose: ## Run unit and integration tests with verbose flag
	$(MAKE) test TEST_ARGS="$(TEST_ARGS) -v"

.PHONY: test-junit
test-junit: $(SETUP_ENVTEST) $(GOTESTSUM) ## Run unit and integration tests and generate a junit report
	set +o errexit; (KUBEBUILDER_ASSETS="$(KUBEBUILDER_ASSETS)" go test -json ./... $(TEST_ARGS); echo $$? > $(ARTIFACTS)/junit.exitcode) | tee $(ARTIFACTS)/junit.stdout
	$(GOTESTSUM) --junitfile $(ARTIFACTS)/junit.xml --raw-command cat $(ARTIFACTS)/junit.stdout
	exit $$(cat $(ARTIFACTS)/junit.exitcode)

.PHONY: test-cover
test-cover: ## Run unit and integration tests and generate a coverage report
	$(MAKE) test TEST_ARGS="$(TEST_ARGS) -coverprofile=out/coverage.out"
	go tool cover -func=out/coverage.out -o out/coverage.txt
	go tool cover -html=out/coverage.out -o out/coverage.html

## --------------------------------------
## Release
## --------------------------------------

##@ release:

## latest git tag for the commit, e.g., v0.3.10
RELEASE_TAG ?= ${TAG}
ifneq (,$(findstring -,$(RELEASE_TAG)))
    PRE_RELEASE=true
endif
# the previous release tag, e.g., v0.3.9, excluding pre-release tags
#PREVIOUS_TAG ?= $(shell git tag -l | grep -E "^v[0-9]+\.[0-9]+\.[0-9]+$$" | sort -V | grep -B1 $(RELEASE_TAG) | head -n 1 2>/dev/null)
## set by Prow, ref name of the base branch, e.g., main
RELEASE_ALIAS_TAG := $(PULL_BASE_REF)
RELEASE_DIR := out
RELEASE_NOTES_DIR := _releasenotes
#USER_FORK ?= $(shell git config --get remote.origin.url | cut -d/ -f4)
IMAGE_REVIEWERS ?= $(shell ./hack/get-project-maintainers.sh)

.PHONY: $(RELEASE_DIR)
$(RELEASE_DIR):
	mkdir -p $(RELEASE_DIR)/

.PHONY: $(RELEASE_NOTES_DIR)
$(RELEASE_NOTES_DIR):
	mkdir -p $(RELEASE_NOTES_DIR)/

.PHONY: release
release: clean-release ## Build and push container images using the latest git tag for the commit
	@if [ -z "${RELEASE_TAG}" ]; then echo "RELEASE_TAG is not set"; exit 1; fi
	@if ! [ -z "$$(git status --porcelain)" ]; then echo "Your local git repository contains uncommitted changes, use git clean before proceeding."; exit 1; fi
	git checkout "${RELEASE_TAG}"
	# Build binaries first.
	GIT_VERSION=$(RELEASE_TAG) $(MAKE) release-binaries
	# Set the manifest images to the staging/production bucket and Builds the manifests to publish with a release.
	$(MAKE) release-manifests-all

.PHONY: release-manifests-all
release-manifests-all: # Set the manifest images to the staging/production bucket and Builds the manifests to publish with a release.
	# Set the manifest image to the production bucket.
	$(MAKE) manifest-modification REGISTRY=$(PROD_REGISTRY)
	## Build the manifests
	$(MAKE) release-manifests
	# Set the development manifest image to the staging bucket.
	$(MAKE) manifest-modification-dev REGISTRY=$(STAGING_REGISTRY)
	## Build the development manifests
	$(MAKE) release-manifests-dev
	## Clean the git artifacts modified in the release process
	$(MAKE) clean-release-git

.PHONY: manifest-modification
manifest-modification: # Set the manifest images to the staging/production bucket.
	$(MAKE) set-manifest-image \
		MANIFEST_IMG=$(REGISTRY)/$(OCNE_BOOTSTRAP_IMAGE_NAME) MANIFEST_TAG=$(RELEASE_TAG) \
		TARGET_RESOURCE="./bootstrap/ocne/config/default/manager_image_patch.yaml"
	$(MAKE) set-manifest-image \
		MANIFEST_IMG=$(REGISTRY)/$(OCNE_CONTROL_PLANE_IMAGE_NAME) MANIFEST_TAG=$(RELEASE_TAG) \
		TARGET_RESOURCE="./controlplane/ocne/config/default/manager_image_patch.yaml"
	$(MAKE) set-manifest-pull-policy PULL_POLICY=IfNotPresent TARGET_RESOURCE="./bootstrap/ocne/config/default/manager_pull_policy.yaml"
	$(MAKE) set-manifest-pull-policy PULL_POLICY=IfNotPresent TARGET_RESOURCE="./controlplane/ocne/config/default/manager_pull_policy.yaml"

.PHONY: release-manifests
release-manifests: $(RELEASE_DIR) $(KUSTOMIZE) ## Build the manifests to publish with a release
	# Build bootstrap-components.
	$(KUSTOMIZE) build bootstrap/ocne/config/default > $(RELEASE_DIR)/bootstrap-components.yaml
	# Build control-plane-components.
	$(KUSTOMIZE) build controlplane/ocne/config/default > $(RELEASE_DIR)/control-plane-components.yaml

	# Add metadata to the release artifacts
	cp metadata.yaml $(RELEASE_DIR)/metadata.yaml

.PHONY: release-manifests-dev
release-manifests-dev: $(RELEASE_DIR) $(KUSTOMIZE) ## Build the development manifests and copies them in the release folder
	cd $(CAPD_DIR); $(KUSTOMIZE) build config/default > ../../../$(RELEASE_DIR)/infrastructure-components-development.yaml
	cp $(CAPD_DIR)/templates/* $(RELEASE_DIR)/

## --------------------------------------
## Docker
## --------------------------------------

.PHONY: docker-push-all
docker-push-all: $(addprefix docker-push-,$(ALL_ARCH))  ## Push the docker images to be included in the release for all architectures + related multiarch manifests
	$(MAKE) docker-push-manifest-ocne-bootstrap
	$(MAKE) docker-push-manifest-ocne-control-plane

docker-push-%:
	$(MAKE) ARCH=$* docker-push

.PHONY: docker-push
docker-push: ## Push the docker images to be included in the release
	docker push $(OCNE_BOOTSTRAP_CONTROLLER_IMG):$(TAG)
	docker push $(OCNE_CONTROL_PLANE_CONTROLLER_IMG):$(TAG)

.PHONY: docker-push-manifest-ocne-bootstrap
docker-push-manifest-ocne-bootstrap: ## Push the multiarch manifest for the ocne bootstrap docker images
	## Minimum docker version 18.06.0 is required for creating and pushing manifest images.
	@for arch in $(ALL_ARCH); do docker push ${OCNE_BOOTSTRAP_CONTROLLER_IMG}:${TAG}; done

.PHONY: docker-push-manifest-ocne-control-plane
docker-push-manifest-ocne-control-plane: ## Push the multiarch manifest for the ocne control plane docker images
	## Minimum docker version 18.06.0 is required for creating and pushing manifest images.
	@for arch in $(ALL_ARCH); do docker push ${OCNE_CONTROL_PLANE_CONTROLLER_IMG}:${TAG}; done

.PHONY: set-manifest-pull-policy
set-manifest-pull-policy:
	$(info Updating kustomize pull policy file for manager resources)
	sed -i'' -e 's@imagePullPolicy: .*@imagePullPolicy: '"$(PULL_POLICY)"'@' $(TARGET_RESOURCE)

.PHONY: set-manifest-image
set-manifest-image:
	$(info Updating kustomize image patch file for manager resource)
	sed -i'' -e 's@image: .*@image: '"${MANIFEST_IMG}:$(MANIFEST_TAG)"'@' $(TARGET_RESOURCE)

## --------------------------------------
## Cleanup / Verification
## --------------------------------------

##@ clean:

.PHONY: clean
clean: ## Remove generated binaries, GitBook files, Helm charts, and Tilt build files
	$(MAKE) clean-bin
	$(MAKE) clean-release
	$(MAKE) clean-generated-yaml
	$(MAKE) clean-generated-deepcopy
	$(MAKE) clean-module-artifacts

.PHONY: clean-bin
clean-bin: ## Remove all generated binaries
	rm -rf $(BIN_DIR)
	rm -rf $(TOOLS_BIN_DIR)

.PHONY: clean-release
clean-release: ## Remove the release folder
	rm -rf $(RELEASE_DIR)

.PHONY: clean-generated-yaml
clean-generated-yaml: ## Remove files generated by conversion-gen from the mentioned dirs. Example SRC_DIRS="./api/v1alpha4"
	(IFS=','; for i in $(SRC_DIRS); do find $$i -type f -name '*.yaml' -exec rm -f {} \;; done)

.PHONY: clean-generated-deepcopy
clean-generated-deepcopy: ## Remove files generated by conversion-gen from the mentioned dirs. Example SRC_DIRS="./api/v1alpha4"
	(IFS=','; for i in $(SRC_DIRS); do find $$i -type f -name 'zz_generated.deepcopy*' -exec rm -f {} \;; done)

.PHONY: clean-module-artifacts
clean-module-artifacts:
	rm -rf modules
	rm -rf index.yaml

## --------------------------------------
## Hack / Tools
## --------------------------------------

##@ hack/tools:

.PHONY: $(CONTROLLER_GEN_BIN)
$(CONTROLLER_GEN_BIN): $(CONTROLLER_GEN) ## Build a local copy of controller-gen.

.PHONY: $(ENVSUBST_BIN)
$(ENVSUBST_BIN): $(ENVSUBST) ## Build a local copy of envsubst.

.PHONY: $(KUSTOMIZE_BIN)
$(KUSTOMIZE_BIN): $(KUSTOMIZE) ## Build a local copy of kustomize.

.PHONY: $(SETUP_ENVTEST_BIN)
$(SETUP_ENVTEST_BIN): $(SETUP_ENVTEST) ## Build a local copy of setup-envtest.

.PHONY: $(GOLANGCI_LINT_BIN)
$(GOLANGCI_LINT_BIN): $(GOLANGCI_LINT) ## Build a local copy of golangci-lint

.PHONY: $(GINKGO_BIN)
$(GINKGO_BIN): $(GINKGO) ## Build a local copy of ginkgo

$(CONTROLLER_GEN): # Build controller-gen from tools folder.
	GOBIN=$(TOOLS_BIN_DIR) $(GO_INSTALL) $(CONTROLLER_GEN_PKG) $(CONTROLLER_GEN_BIN) $(CONTROLLER_GEN_VER)


$(ENVSUBST): # Build gotestsum from tools folder.
	GOBIN=$(TOOLS_BIN_DIR) $(GO_INSTALL) $(ENVSUBST_PKG) $(ENVSUBST_BIN) $(ENVSUBST_VER)

$(KUSTOMIZE): # Build kustomize from tools folder.
	CGO_ENABLED=0 GOBIN=$(TOOLS_BIN_DIR) $(GO_INSTALL) $(KUSTOMIZE_PKG) $(KUSTOMIZE_BIN) $(KUSTOMIZE_VER)

$(SETUP_ENVTEST): # Build setup-envtest from tools folder.
	GOBIN=$(TOOLS_BIN_DIR) $(GO_INSTALL) $(SETUP_ENVTEST_PKG) $(SETUP_ENVTEST_BIN) $(SETUP_ENVTEST_VER)

$(GOLANGCI_LINT): .github/workflows/golangci-lint.yml # Download golangci-lint using hack script into tools folder.
	hack/ensure-golangci-lint.sh \
		-b $(TOOLS_BIN_DIR) \
		$(shell cat .github/workflows/golangci-lint.yml | grep [[:space:]]version | sed 's/.*version: //')

$(GINKGO): # Build ginkgo from tools folder.
	GOBIN=$(TOOLS_BIN_DIR) $(GO_INSTALL) $(GINKGO_PKG) $(GINKGO_BIN) $(GINGKO_VER)


## --------------------------------------
## Build OCNE Artifacts
## --------------------------------------

RELEASE_BOOTSTRAP_DIR := release/bootstrap-ocne/v${MAJOR_VERSION}.${MINOR_VERSION}.${PATCH_VERSION}
RELEASE_CONTROL_PLANE_DIR := release/control-plane-ocne/v${MAJOR_VERSION}.${MINOR_VERSION}.${PATCH_VERSION}
##@ Build OCNE Artifacts:
.PHONY:
ocnebuild: ## Remove files generated by conversion-gen from the mentioned dirs. Example SRC_DIRS="./api/v1alpha4"
	rm -rf $(RELEASE_BOOTSTRAP_DIR) $(RELEASE_CONTROL_PLANE_DIR) out bin
	mkdir -p $(RELEASE_BOOTSTRAP_DIR)
	mkdir -p $(RELEASE_CONTROL_PLANE_DIR)
	$(MAKE) clean
	$(MAKE) generate
	$(MAKE) docker-build
	$(MAKE) docker-push
	$(MAKE) release-manifests
	cp out/bootstrap-components.yaml $(RELEASE_BOOTSTRAP_DIR)/bootstrap-components.yaml
	cp out/metadata.yaml $(RELEASE_BOOTSTRAP_DIR)/metadata.yaml
	cp out/control-plane-components.yaml  $(RELEASE_CONTROL_PLANE_DIR)/control-plane-components.yaml
	cp out/metadata.yaml  $(RELEASE_CONTROL_PLANE_DIR)/metadata.yaml


