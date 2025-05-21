# Variables
CLUSTER_NAME = secretless-test
WEBHOOK_IMAGE = esi-pod-webhook:latest22
WEBHOOK_DEBUG_IMAGE = esi-pod-webhook:debug
ESO_IMAGE = esi-cli:latest
ESO_INIT_IMAGE = esi-cli-init:test
ESO_SIDECAR_IMAGE = esi-cli-sidecar:test
NAMESPACE = secretless-system
VAULT_NAMESPACE = vault

.PHONY: all
all: cluster setup-vault setup-eso deploy-webhook build-eso test-vault

.PHONY: build-eso
build-eso: build-eso-init build-eso-sidecar

.PHONY: build-eso-init
build-eso-init:
	@echo "Building secretless-eso init container image..."
	docker build -t $(ESO_INIT_IMAGE) -f ../esi-cli/Dockerfile.init ../esi-cli
	kind load docker-image $(ESO_INIT_IMAGE) --name $(CLUSTER_NAME)

.PHONY: build-eso-sidecar
build-eso-sidecar:
	@echo "Building secretless-eso sidecar container image..."
	docker build -t $(ESO_SIDECAR_IMAGE) -f ../esi-cli/Dockerfile ../esi-cli
	kind load docker-image $(ESO_SIDECAR_IMAGE) --name $(CLUSTER_NAME)

.PHONY: cluster
cluster:
	@echo "Creating Kind cluster..."
	kind create cluster --name $(CLUSTER_NAME) --config kind-config.yaml || true
	@echo "Installing cert-manager..."
	kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.14.2/cert-manager.yaml
	@echo "Waiting for cert-manager to be ready..."
	kubectl wait --for=condition=Available deployment --all -n cert-manager --timeout=120s

.PHONY: setup-vault
setup-vault:
	@echo "Installing Vault..."
	helm repo add hashicorp https://helm.releases.hashicorp.com || true
	helm repo update
	helm install vault hashicorp/vault \
		--namespace $(VAULT_NAMESPACE) \
		--create-namespace \
		--set "server.dev.enabled=true" \
		--set "server.dev.devRootToken=root"
	@echo "Waiting for Vault to be ready..."
	@echo "Sleeping for 15 seconds to give Vault time to create the pod..."
	sleep 15
	kubectl wait --for=condition=Ready pod/vault-0 -n $(VAULT_NAMESPACE) --timeout=120s
	@echo "Configuring Vault..."
	kubectl exec -n $(VAULT_NAMESPACE) vault-0 -- vault auth enable kubernetes
	kubectl exec -n $(VAULT_NAMESPACE) vault-0 -- /bin/sh -c '\
		echo "path \"secret/*\" { capabilities = [\"read\"] }" | vault policy write secretless-reader -'
	kubectl exec -n $(VAULT_NAMESPACE) vault-0 -- /bin/sh -c '\
		vault write auth/kubernetes/role/secretless-reader \
			bind_namespace="*" \
			bound_service_account_names="*" \
			bound_service_account_namespaces="*" \
			policies=secretless-reader \
			ttl=1h'
	kubectl exec -n $(VAULT_NAMESPACE) vault-0 -- /bin/sh -c '\
		vault write auth/kubernetes/config \
			kubernetes_host="https://kubernetes.default.svc.cluster.local" \
			token_reviewer_jwt="$(cat /var/run/secrets/kubernetes.io/serviceaccount/token)" \
			kubernetes_ca_cert="$(cat /var/run/secrets/kubernetes.io/serviceaccount/ca.crt)" \
			issuer="https://kubernetes.default.svc.cluster.local"'
	@echo "Creating test secrets in Vault..."
	kubectl exec -n $(VAULT_NAMESPACE) vault-0 -- vault kv put secret/test-app/config \
		API_KEY=test-api-key \
		API_SECRET=test-api-secret

.PHONY: setup-eso
setup-eso:
	@echo "Installing External Secrets Operator..."
	helm repo add external-secrets https://charts.external-secrets.io || true
	helm repo update
	helm install external-secrets external-secrets/external-secrets -n external-secrets --create-namespace
	@echo "Waiting for ESO to be ready..."
	kubectl wait --for=condition=Available deployment --all -n external-secrets --timeout=120s
	@echo "Waiting for ESO CRDs to be ready..."
	kubectl wait --for=condition=Established crd/secretstores.external-secrets.io --timeout=120s

.PHONY: build-esi-cli
build-esi-cli:
	@echo "Building esi-cli binary..."
	cd ../esi-cli && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/esi-cli-linux-amd64

.PHONY: build-deploy
build-deploy: build-amd64 build-esi-cli
	@echo "Building webhook image..."
	docker build -t $(WEBHOOK_IMAGE) -f Dockerfile .
	@echo "Building ESI images..."
	docker build -t $(ESO_IMAGE) -f ../esi-cli/Dockerfile ../esi-cli
	docker build -t $(ESO_INIT_IMAGE) -f ../esi-cli/Dockerfile.init ../esi-cli
	docker build -t $(ESO_SIDECAR_IMAGE) -f ../esi-cli/Dockerfile ../esi-cli
	@echo "Loading images into Kind cluster..."
	kind load docker-image $(WEBHOOK_IMAGE) --name $(CLUSTER_NAME)
	kind load docker-image $(ESO_IMAGE) --name $(CLUSTER_NAME)
	kind load docker-image $(ESO_INIT_IMAGE) --name $(CLUSTER_NAME)
	kind load docker-image $(ESO_SIDECAR_IMAGE) --name $(CLUSTER_NAME)

.PHONY: deploy-webhook
deploy-webhook: build-deploy
	@echo "Creating namespace..."
	kubectl create namespace $(NAMESPACE) || true
	@echo "Deploying webhook..."
	kubectl apply -f k8s/webhook.yaml

.PHONY: deploy-webhook-debug
deploy-webhook-debug:
	@echo "Building debug webhook image..."
	docker build -t $(WEBHOOK_DEBUG_IMAGE) -f Dockerfile.debug .
	@echo "Loading debug image into Kind cluster..."
	kind load docker-image $(WEBHOOK_DEBUG_IMAGE) --name $(CLUSTER_NAME)
	@echo "Creating namespace..."
	kubectl create namespace $(NAMESPACE) || true
	@echo "Deploying webhook in debug mode..."
	kubectl apply -f k8s/webhook.yaml

.PHONY: test-vault
test-vault:
	@echo "Waiting for webhook to be ready..."
	kubectl wait --for=condition=Available deployment -n $(NAMESPACE) esi-pod-webhook --timeout=120s
	@echo "Creating Vault SecretStore..."
	kubectl apply -f k8s/vault-secretstore.yaml
	@echo "Creating ExternalSecret..."
	kubectl apply -f k8s/test-externalsecret.yaml
	@echo "Deleting old test pod if it exists..."
	kubectl delete pod test-pod --ignore-not-found=true
	@echo "Creating SA to be used by the getter..."
	kubectl apply -f k8s/eso-sa.yaml
	@echo "Creating SA to be used by test pod with permissions..."
	kubectl apply -f k8s/test-pod-sa.yaml
	@echo "Creating test pod..."
	kubectl apply -f k8s/test-pod.yaml

.PHONY: clean-light
clean-light:
	@echo "Removing webhook..."
	kubectl delete -f k8s/webhook.yaml || true
	@echo "Removing ESO..."
	helm uninstall external-secrets -n external-secrets || true
	@echo "Removing Vault..."
	helm uninstall vault -n $(VAULT_NAMESPACE) || true

.PHONY: clean
clean:
	@echo "Deleting Kind cluster..."
	kind delete cluster --name $(CLUSTER_NAME)

.PHONY: logs
logs:
	@echo "Webhook logs:"
	kubectl logs -n $(NAMESPACE) -l app=esi-pod-webhook
	@echo "\nTest pod logs:"
	kubectl logs -n default -l app=test-pod
	@echo "\nVault logs:"
	kubectl logs -n $(VAULT_NAMESPACE) vault-0

# Image URL to use all building/pushing image targets
IMG ?= controller:latest
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.31.0

SUITE ?= .*
AWS_REGION ?= eu-west-1
EKS_CLUSTER_NAME ?= ar-cluster
ACCOUNT_ID ?= $(shell aws sts get-caller-identity --query Account --output text)
ECR_REPO_NAME ?= esi-pod-webhook
ECR_URI ?= $(ACCOUNT_ID).dkr.ecr.$(AWS_REGION).amazonaws.com/$(ECR_REPO_NAME):latest
ARTIFACT_REG:=us-central1-docker.pkg.dev
CHARTS_REPO := oci://$(ARTIFACT_REG)/external-secrets-inc-registry/public/charts
ARCH ?= amd64 arm64 ppc64le
BUILD_ARGS ?= CGO_ENABLED=0
DOCKER_BUILD_ARGS ?=
DOCKERFILE ?= Dockerfile
OUTPUT_DIR  ?= bin
# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif
# ====================================================================================
# Logger

TIME_LONG	= `date +%Y-%m-%d' '%H:%M:%S`
TIME_SHORT	= `date +%H:%M:%S`
TIME		= $(TIME_SHORT)

INFO	= echo ${TIME} ${BLUE}[ .. ]${CNone}
WARN	= echo ${TIME} ${YELLOW}[WARN]${CNone}
ERR		= echo ${TIME} ${RED}[FAIL]${CNone}
OK		= echo ${TIME} ${GREEN}[ OK ]${CNone}
FAIL	= (echo ${TIME} ${RED}[FAIL]${CNone} && false)
# ============================================================
# CONTAINER_TOOL defines the container tool to be used for building images.
# Be aware that the target commands are only tested with Docker which is
# scaffolded by default. However, you might want to replace it to use other
# tools. (i.e. podman)
CONTAINER_TOOL ?= docker

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

# .PHONY: all
# all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk command is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: fmt vet envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test $$(go list ./... | grep -v /e2e) -coverprofile cover.out

# Utilize Kind or modify the e2e tests to load the image locally, enabling compatibility with other vendors.
.PHONY: test-e2e  # Run the e2e tests against a Kind k8s instance that is spun up.
test-e2e:
	go test ./test/e2e/ -v -ginkgo.vv -ginkgo.skip="EKS Tests" -ginkgo.focus=$(SUITE)

# Create cluster and install ESO and secretless webhook
.PHONY: setup
setup:
	kind create cluster || true
	kubectl apply -f https://raw.githubusercontent.com/external-secrets/external-secrets/refs/heads/main/deploy/crds/bundle.yaml
	make install

.PHONY: lint
lint: golangci-lint ## Run golangci-lint linter
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	$(GOLANGCI_LINT) run --fix

##@ Build

.PHONY: build
build: $(addprefix build-,$(ARCH)) ## Build binary

.PHONY: build-%
build-%: fmt vet ## Build binary for the specified arch
	@$(INFO) go build $*
	$(BUILD_ARGS) GOOS=linux GOARCH=$* \
		go build -o '$(OUTPUT_DIR)/esi-pod-webhook-linux-$*' ./cmd/webhook
	@$(OK) go build $*

.PHONY: run
run: fmt vet ## Run a controller from your host.
	go run ./cmd/webhook

# If you wish to build the manager image targeting other platforms you can use the --platform flag.
# (i.e. docker build --platform linux/arm64). However, you must enable docker buildKit for it.
# More info: https://docs.docker.com/develop/develop-images/build_enhancements/
.PHONY: docker.build
docker.build: $(addprefix build-,$(ARCH)) ## Build the docker image
	@$(INFO) docker build
	echo docker build -f $(DOCKERFILE) . $(DOCKER_BUILD_ARGS) -t ${IMG}
	DOCKER_BUILDKIT=1 docker build -f $(DOCKERFILE) . $(DOCKER_BUILD_ARGS) -t ${IMG}
	@$(OK) docker build


.PHONY: docker-build
docker-build: ## Build docker image with the manager.
	$(CONTAINER_TOOL) build -t ${IMG} --build-arg TARGETOS=$(TARGETOS) --build-arg TARGETARCH=$(TARGETARCH) .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	$(CONTAINER_TOOL) push ${IMG}

# PLATFORMS defines the target platforms for the manager image be built to provide support to multiple
# architectures. (i.e. make docker-buildx IMG=myregistry/mypoperator:0.0.1). To use this option you need to:
# - be able to use docker buildx. More info: https://docs.docker.com/build/buildx/
# - have enabled BuildKit. More info: https://docs.docker.com/develop/develop-images/build_enhancements/
# - be able to push the image to your registry (i.e. if you do not set a valid value via IMG=<myregistry/image:<tag>> then the export will fail)
# To adequately provide solutions that are compatible with multiple platforms, you should consider using this option.
PLATFORMS ?= linux/arm64,linux/amd64
.PHONY: docker-buildx
docker-buildx: ## Build and push docker image for the manager for cross-platform support
	# copy existing Dockerfile and insert --platform=${BUILDPLATFORM} into Dockerfile.cross, and preserve the original Dockerfile
	sed -e '1 s/\(^FROM\)/FROM --platform=\$$\{BUILDPLATFORM\}/; t' -e ' 1,// s//FROM --platform=\$$\{BUILDPLATFORM\}/' Dockerfile > Dockerfile.cross
	- $(CONTAINER_TOOL) buildx create --name esi-pod-webhook-builder
	$(CONTAINER_TOOL) buildx use esi-pod-webhook-builder
	- $(CONTAINER_TOOL) buildx build --push --platform=$(PLATFORMS) --tag ${IMG} --build-arg TARGETOS=$(TARGETOS) --build-arg TARGETARCH=$(TARGETARCH) -f Dockerfile.cross .
	- $(CONTAINER_TOOL) buildx rm esi-pod-webhook-builder
	rm Dockerfile.cross

.PHONY: build-installer
build-installer: manifests generate kustomize ## Generate a consolidated YAML with CRDs and deployment.
	mkdir -p dist
	cd config/manager-distribution && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/distribution > dist/install.yaml

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy-eso
deploy-eso:
	$(HELM) repo add external-secrets https://charts.external-secrets.io
	$(HELM) install external-secrets external-secrets/external-secrets --namespace external-secrets \
		--create-namespace --set installCRDs=true

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager-local && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | $(KUBECTL) apply -f -

.PHONY: undeploy
undeploy: kustomize ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/default | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

##@ EKS Deployment

.PHONY: login-ecr
login-ecr: ## Authenticate Docker to ECR
	aws ecr get-login-password --region $(AWS_REGION) | \
	$(CONTAINER_TOOL) login --username AWS --password-stdin $(ACCOUNT_ID).dkr.ecr.$(AWS_REGION).amazonaws.com

.PHONY: create-ecr-repo
create-ecr-repo: ## Create ECR repository if it doesn't exist
	aws ecr describe-repositories --repository-names $(ECR_REPO_NAME) --region $(AWS_REGION) >/dev/null 2>&1 || \
	aws ecr create-repository --repository-name $(ECR_REPO_NAME) --region $(AWS_REGION)

.PHONY: configure-kubectl
configure-kubectl: ## Update kubeconfig to point to EKS cluster
	aws eks update-kubeconfig --region $(AWS_REGION) --name $(EKS_CLUSTER_NAME)

.PHONY: build-push-ecr
build-push-ecr: create-ecr-repo login-ecr ## Build and push image to ECR
	$(MAKE) docker-build IMG=$(ECR_URI) PLATFORMS=linux/amd64 TARGETOS=linux TARGETARCH=amd64
	$(MAKE) docker-push IMG=$(ECR_URI)
	@echo "Image built and pushed to ECR: $(ECR_URI)"

.PHONY: deploy-eks
deploy-eks: configure-kubectl build-push-ecr install ## Deploy controller to EKS
	$(MAKE) deploy IMG=$(ECR_URI)

.PHONY: test-eks
test-eks: ## Run E2E tests on EKS cluster
	go test ./test/e2e/ -v -ginkgo.vv -ginkgo.focus="EKS Tests"

.PHONY: clean-eks
clean-eks: configure-kubectl ## Clean up EKS resources
	$(MAKE) undeploy

##@ Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUBECTL ?= kubectl
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest
GOLANGCI_LINT = $(LOCALBIN)/golangci-lint
HELM ?= $(LOCALBIN)/helm

## Tool Versions
KUSTOMIZE_VERSION ?= v5.4.3
CONTROLLER_TOOLS_VERSION ?= v0.16.1
ENVTEST_VERSION ?= release-0.19
GOLANGCI_LINT_VERSION ?= v1.64.5
HELM_VERSION ?= v3.16.2

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
	$(call go-install-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest,$(ENVTEST_VERSION))

.PHONY: golangci-lint
golangci-lint: $(GOLANGCI_LINT) ## Download golangci-lint locally if necessary.
$(GOLANGCI_LINT): $(LOCALBIN)
	$(call go-install-tool,$(GOLANGCI_LINT),github.com/golangci/golangci-lint/cmd/golangci-lint,$(GOLANGCI_LINT_VERSION))

##@ Helm
.PHONY: helm.test
helm.test: ## Run helm tests
	@helm unittest --file tests/*.yaml --file 'tests/**/*.yaml' deploy/charts/esi-pod-webhook

.PHONY: helm.test.update
helm.test.update: ## Run helm tests
	@helm unittest -u --file tests/*.yaml --file 'tests/**/*.yaml' .

helm.login:
	gcloud auth print-access-token | helm registry login -u oauth2accesstoken \
		--password-stdin https://$(ARTIFACT_REG)

.PHONY: helm.push
helm.push: helm.login ## Push helm chart to the repository
	@helm package deploy/charts/esi-pod-webhook
	helm push *.tgz $(CHARTS_REPO)


##@ API Spec
.PHONY: spec-generate
spec-generate: ## generate api reference documentation to go to the website
	./hack/generate.sh docs/api/spec.md

.PHONY: helm
helm: $(HELM) ## Download helm locally if necessary.
$(HELM): $(LOCALBIN)
	$(call go-install-tool,$(HELM),helm.sh/helm/v3/cmd/helm,$(HELM_VERSION))

# go-install-tool will 'go install' any package with custom target and name of binary, if it doesn't exist
# $1 - target path with name of binary
# $2 - package url which can be installed
# $3 - specific version of package
define go-install-tool
@[ -f "$(1)-$(3)" ] || { \
set -e; \
package=$(2)@$(3) ;\
echo "Downloading $${package}" ;\
rm -f $(1) || true ;\
GOBIN=$(LOCALBIN) go install $${package} ;\
mv $(1) $(1)-$(3) ;\
} ;\
ln -sf $(1)-$(3) $(1)
endef
