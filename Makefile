# Copyright 2024 The CAPBM Authors.
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

# Image URLs to use all building/pushing image targets
CAPBM_IMG ?= ghcr.io/betawater/capbm-manager:v0.8.1
CVO_IMG ?= ghcr.io/betawater/cvo-manager:v0.8.1
RELEASE_IMG ?= ghcr.io/betawater/capbm/release:v1.31.1

# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.31.0

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Setting SHELL to bash allows bash commands to be executed by recipes.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=capbm-manager-role crd webhook paths="./modules/capbm/api/v1beta1" output:crd:artifacts:config=modules/capbm/config/crd/bases
	$(CONTROLLER_GEN) rbac:roleName=cvo-manager-role crd webhook paths="./modules/cvo/api/v1beta1" output:crd:artifacts:config=modules/cvo/config/crd/bases

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./modules/cvo/api/v1beta1"
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./modules/capbm/api/v1beta1"

.PHONY: fmt
fmt: ## Run go fmt against code.
	cd modules/cvo && go fmt ./...
	cd modules/capbm && go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	cd modules/cvo && go vet ./...
	cd modules/capbm && go vet ./...

.PHONY: test
test: manifests generate fmt vet envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test ./modules/cvo/... ./modules/capbm/... -coverprofile cover.out

##@ Build

.PHONY: build
build: build-capbm build-cvo

.PHONY: build-capbm
build-capbm: manifests generate fmt vet ## Build CAPBM manager binary.
	cd modules/capbm && go build -o ../../bin/capbm-manager ./cmd/manager/

.PHONY: build-cvo
build-cvo: manifests generate fmt vet ## Build CVO manager binary.
	cd modules/cvo && go build -o ../../bin/cvo-manager ./cmd/manager/

.PHONY: run-capbm
run-capbm: manifests generate fmt vet ## Run CAPBM controller from your host.
	go run ./modules/capbm/cmd/manager/

.PHONY: run-cvo
run-cvo: manifests generate fmt vet ## Run CVO controller from your host.
	go run ./modules/cvo/cmd/manager/

.PHONY: docker-build-capbm
docker-build-capbm: test ## Build docker image with the CAPBM manager.
	docker build -t ${CAPBM_IMG} -f Dockerfile.capbm .

.PHONY: docker-build-cvo
docker-build-cvo: test ## Build docker image with the CVO manager.
	docker build -t ${CVO_IMG} -f Dockerfile.cvo .

.PHONY: docker-push-capbm
docker-push-capbm: ## Push CAPBM docker image.
	docker push ${CAPBM_IMG}

.PHONY: docker-push-cvo
docker-push-cvo: ## Push CVO docker image.
	docker push ${CVO_IMG}

##@ Deployment

.PHONY: install-capbm
install-capbm: manifests kustomize ## Install CAPBM CRDs into the K8s cluster.
	$(KUSTOMIZE) build modules/capbm/config/crd | kubectl apply -f -

.PHONY: install-cvo
install-cvo: manifests kustomize ## Install CVO CRDs into the K8s cluster.
	$(KUSTOMIZE) build modules/cvo/config/crd | kubectl apply -f -

.PHONY: uninstall-capbm
uninstall-capbm: manifests kustomize ## Uninstall CAPBM CRDs from the K8s cluster.
	$(KUSTOMIZE) build modules/capbm/config/crd | kubectl delete -f -

.PHONY: uninstall-cvo
uninstall-cvo: manifests kustomize ## Uninstall CVO CRDs from the K8s cluster.
	$(KUSTOMIZE) build modules/cvo/config/crd | kubectl delete -f -

.PHONY: deploy-capbm
deploy-capbm: manifests kustomize ## Deploy CAPBM controller to the K8s cluster.
	cd modules/capbm/config/manager && $(KUSTOMIZE) edit set image controller=${CAPBM_IMG}
	$(KUSTOMIZE) build modules/capbm/config | kubectl apply -f -

.PHONY: deploy-cvo
deploy-cvo: manifests kustomize ## Deploy CVO controller to the K8s cluster.
	cd modules/cvo/config/manager && $(KUSTOMIZE) edit set image controller=${CVO_IMG}
	$(KUSTOMIZE) build modules/cvo/config | kubectl apply -f -

.PHONY: undeploy-capbm
undeploy-capbm: ## Undeploy CAPBM controller from the K8s cluster.
	$(KUSTOMIZE) build modules/capbm/config | kubectl delete -f -

.PHONY: undeploy-cvo
undeploy-cvo: ## Undeploy CVO controller from the K8s cluster.
	$(KUSTOMIZE) build modules/cvo/config | kubectl delete -f -

.PHONY: deploy-clusterclass
deploy-clusterclass: manifests kustomize ## Deploy ClusterClass templates.
	$(KUSTOMIZE) build modules/capbm/config/clusterclass | kubectl apply -f -

##@ Release

.PHONY: release-capbm
release-capbm: manifests kustomize ## Generate CAPBM release manifests.
	cd modules/capbm/config/manager && $(KUSTOMIZE) edit set image controller=${CAPBM_IMG}
	$(KUSTOMIZE) build modules/capbm/config > infrastructure-components.yaml

.PHONY: release-cvo
release-cvo: manifests kustomize ## Generate CVO release manifests.
	cd modules/cvo/config/manager && $(KUSTOMIZE) edit set image controller=${CVO_IMG}
	$(KUSTOMIZE) build modules/cvo/config > cvo-components.yaml

.PHONY: release-manifests
release-manifests: release-capbm release-cvo ## Generate release manifests directory.
	mkdir -p releases/$(VERSION)
	cp infrastructure-components.yaml releases/$(VERSION)/infrastructure-components.yaml
	cp cvo-components.yaml releases/$(VERSION)/cvo-components.yaml
	cp metadata.yaml releases/$(VERSION)/metadata.yaml

##@ Release Image

.PHONY: release-image-build
release-image-build: ## Build release image OCI image
	docker build -t ${RELEASE_IMG} -f Dockerfile.release .

.PHONY: release-image-push
release-image-push: ## Push release image OCI image
	docker push ${RELEASE_IMG}

.PHONY: release-image
release-image: release-image-build release-image-push ## Build and push release image

.PHONY: deploy-release-http-server
deploy-release-http-server: ## Deploy ReleaseImage HTTP Server
	kubectl apply -f templates/release-http-server.yaml

.PHONY: undeploy-release-http-server
undeploy-release-http-server: ## Undeploy ReleaseImage HTTP Server
	kubectl delete -f templates/release-http-server.yaml

##@ Build Dependencies

LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest

KUSTOMIZE_VERSION ?= v5.4.3
CONTROLLER_TOOLS_VERSION ?= v0.17.2

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	$(call go-install-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v5,$(KUSTOMIZE_VERSION))

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	$(call go-install-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen,$(CONTROLLER_TOOLS_VERSION))

.PHONY: envtest
envtest: $(ENVTEST) ## Download setup-envtest locally if necessary.
$(ENVTEST): $(LOCALBIN)
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,latest)

define go-install-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
GOBIN=$(LOCALBIN) go install $(2)@$(3) ;\
rm -rf $$TMP_DIR ;\
}
endef
