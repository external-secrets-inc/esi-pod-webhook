# ESI Pod Webhook

A Kubernetes admission webhook that automatically injects External Secrets Operator (ESO) secrets into pods using the `esi-cli` tool. This webhook enables seamless integration between your applications and ESO-managed secrets without modifying application code.

## Advantages

- **Automatic Secret Injection**: Injects ESO-managed secrets as environment variables into pods
- **Non-intrusive**: Works with existing applications without code changes
- **Secure**: Uses init containers to handle secret injection, keeping secrets isolated
- **Flexible**: Supports both environment variable and file-based secret injection

## How It Works

1. When a pod is created with the annotation `secretless.externalsecrets.com/env-vars: "true"` (or `secretless.externalsecrets.com/file-secrets: "true"`), the webhook intercepts the creation request
2. The webhook injects an init container (or a sidecar container) that uses `esi-cli` to fetch secrets from ESO
3. The init container processes the secrets and injects them into the main container
4. The main container starts with the secrets available as environment variables (or as files in a specific directory)

Kubernetes docs for reference:

https://kubernetes.io/docs/reference/access-authn-authz/admission-controllers/#mutatingadmissionwebhook

Structure of a mutating webhook:

- MutatingWebhookConfiguration configuring the api server to know about the webhook
- ServiceAccount for the webhook
- ClusterRole and ClusterRoleBinding for the webhook to be able to access the ESO CRDs
- Deployment for the webhook
- Service for the webhook (exposing it so the k8s api server can reach it)

In this case we are intercepting pod creation requests but we could intercept any other requests if that was needed.

ASC2 art with the lifecycle of a request:

```text
+-------------------+
|    kubectl / API  |
|     Client CLI    |
+-------------------+
          |
          v
+-------------------+
| Kubernetes API    |
| Server (Validation|
|     & Admission)  |
+-------------------+
          |
          v
+---------------------------+
|  Authentication &        |
|  Authorization Checks     |
+---------------------------+
          |
          v
+---------------------------+
|  Admission Controllers    |
|  (Invokes Webhooks)       |
+---------------------------+
          |
          |-->+------------------------------+    <------our webhook
          |   |  Mutating Admission Webhook  |
          |   | (Modifies the request object |
          |   |    before persistence)       |
          |   +------------------------------+
          |
          v
+---------------------------+
|  Validating Admission     |
|  Controllers (optional)   |
+---------------------------+
          |
          v
+---------------------------+
| Persist to etcd           |
| (Stores the object state) |
+---------------------------+
          |
          v
+------------------------------------------------------------+
| Kubernetes Scheduler + controllers                         |
| (if it's a Pod or other higher level resources request)    |
+------------------------------------------------------------+
          |
          v
+----------------------------------+
| Kubelet on the Node              |
| (Pulls image, starts Pod... etc) |
+----------------------------------+
```

## Prerequisites

- Kubernetes cluster (v1.16+)
- External Secrets Operator installed and configured (actually only the CRDS are needed)
- cert-manager for certificate management (optional - but easier to set up)

## Installation and usage for development

1. Clone the repository:
   ```bash
   git clone https://github.com/external-secrets-inc/esi-pod-webhook.git
   cd esi-pod-webhook
   ```

2. Clone the esi-cli repository (should be in the same parent directory as esi-pod-webhook):
    ```bash
    cd ..
    git clone https://github.com/external-secrets-inc/esi-cli.git
    cd esi-pod-webhook
    ```


3. create a cluster
    ```bash
    make cluster
    ```
4. setup vault and add secrets to it and configure it to use k8s sa auth
    ```bash
    make setup-vault
    ```
5. install eso and have crds and stuff ready
    ```bash
    make setup-eso
    ```
6. run to build everything and load images to kind
    ```bash
    make deploy-webhook
    ```

7. then to prepare the initial test setup edit the k8s/test-pod.yaml to uncomment the env var annotation, comment the file secret annotation, uncomment the do-env command and comment the cat file command

    ```yaml
    secretless.externalsecrets.com/externalsecret: "test-app-secrets"
    secretless.externalsecrets.com/env-vars: "true"
    # secretless.externalsecrets.com/file-secrets: "true"
    [...]
    [...]
    command: ["/bin/sh", "-c", "while true; do env | grep API; sleep 10; done"]
    # command: ["/bin/sh", "-c", "while ! [ -f /secrets/secrets.json ]; do echo 'Waiting for /secrets/secrets.json...'; sleep 1; done; while true; do env | grep API; cat /secrets/secrets.json; sleep 10; done"]
   
    ```

7. run the makefile target for testing

    ```bash
    make test-vault
    ```

8. check the logs out

    ```bash
    kubectl logs -f test-pod -c app
    ```

9. Then **invert** the commented and uncommented lines in the previous file mentioned and force delete and reapply:

    ```bash
    kubectl delete pod test-pod --force && kubectl apply -f k8s/test-pod.yaml
    ```

10. and check the logs again

      ```bash
    kubectl logs -f test-pod -c app
    ```

## Development

### Prerequisites

- Go 1.20+
- Docker
- kind (for local development)


### Project Structure

```
├── cmd/webhook      # Main entry point
├── pkg/
│   ├── admission/   # Admission webhook logic
│   ├── container/   # Container injection logic
│   ├── k8s/        # Kubernetes client wrappers
│   └── patch/      # JSON patch operations
└── k8s/            # Kubernetes manifests
```

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.