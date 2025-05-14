# Variables
CLUSTER_NAME = secretless-test
WEBHOOK_IMAGE = secretless-webhook:latest2
WEBHOOK_DEBUG_IMAGE = secretless-webhook:debug
ESO_IMAGE = secretless-eso:latest
ESO_INIT_IMAGE = secretless-eso-init:latest
ESO_SIDECAR_IMAGE = secretless-eso-sidecar:latest
NAMESPACE = secretless-system
VAULT_NAMESPACE = vault

.PHONY: all
all: cluster setup-vault setup-eso deploy-webhook build-eso test-vault

.PHONY: build-eso
build-eso: build-eso-init build-eso-sidecar

.PHONY: build-eso-init
build-eso-init:
	@echo "Building secretless-eso init container image..."
	docker build -t $(ESO_INIT_IMAGE) -f ../secretless-eso/Dockerfile.init ../secretless-eso
	kind load docker-image $(ESO_INIT_IMAGE) --name $(CLUSTER_NAME)

.PHONY: build-eso-sidecar
build-eso-sidecar:
	@echo "Building secretless-eso sidecar container image..."
	docker build -t $(ESO_SIDECAR_IMAGE) -f ../secretless-eso/Dockerfile.sidecar ../secretless-eso
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

.PHONY: build
build:
	@echo "Building webhook image..."
	docker build -t $(WEBHOOK_IMAGE) -f Dockerfile .
	@echo "Building secretless-eso image..."
	docker build -t $(ESO_IMAGE) -f ../secretless-eso/Dockerfile ../secretless-eso
	@echo "Loading images into Kind cluster..."
	kind load docker-image $(WEBHOOK_IMAGE) --name $(CLUSTER_NAME)
	kind load docker-image $(ESO_IMAGE) --name $(CLUSTER_NAME)

.PHONY: deploy-webhook
deploy-webhook: build
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
