# Okta Federation Support in ESI Pod Webhook

This document explains how to use Okta authentication with the ESI Pod Webhook to enable pods to access federated External Secrets servers using Okta OAuth2 credentials.

## Overview

The ESI Pod Webhook now supports Okta OAuth2 client credentials flow for federation authentication. This allows pods to authenticate to federation servers using Okta access tokens instead of Kubernetes service account tokens or SPIFFE mTLS.

## Supported Federation Auth Methods

The webhook supports three federation authentication methods via the `secretless.externalsecrets.com/federated-auth` annotation:

- **`kubernetes`** - Service account token authentication (default)
- **`spiffe`** - SPIFFE/SPIRE mTLS authentication  
- **`okta`** - Okta OAuth2 client credentials with private_key_jwt (NEW)

## Prerequisites

1. **Federation Server** deployed with Okta authentication support
2. **Okta OAuth2 Application** configured with:
   - Grant type: Client Credentials
   - Client authentication: Public key / Private key
   - Your public key registered in Okta
3. **OktaFederation and Authorization CRDs** deployed in the cluster
4. **Private Key** stored as a Kubernetes Secret

## Annotations

### Required Annotations

| Annotation | Description | Example |
|------------|-------------|---------|
| `secretless.externalsecrets.com/externalsecret` | Name of the ExternalSecret | `"test-app-secrets"` |
| `secretless.externalsecrets.com/federated-server-url` | Federation server URL | `"http://federation-server:8080"` |
| `secretless.externalsecrets.com/federated-auth` | Authentication method | `"okta"` |
| `secretless.externalsecrets.com/okta-client-id` | Okta OAuth2 client ID | `"0oawl3l22qkmQK274697"` |
| `secretless.externalsecrets.com/okta-private-key` | Path to private key in pod | `"/okta/private_key.pem"` |
| `secretless.externalsecrets.com/okta-domain` | Okta domain URL | `"https://trial-1038013.okta.com"` |

### Optional Annotations

| Annotation | Description | Default | Example |
|------------|-------------|---------|---------|
| `secretless.externalsecrets.com/okta-auth-server` | Okta authorization server ID | `""` (org server) | `"custom-auth-server"` |
| `secretless.externalsecrets.com/okta-scopes` | Space-separated OAuth2 scopes | `""` | `"openid profile"` |
| `secretless.externalsecrets.com/inject-on-env` | Environment variable mapping | All keys | `"DB_PASS=vault.db-password"` |
| `secretless.externalsecrets.com/inject-on-file` | File injection mapping | - | `"/secrets/db.txt=vault.db-password"` |

## Example: Init Container Mode with Okta Federation

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: my-app
  annotations:
    secretless.externalsecrets.com/externalsecret: "app-secrets"
    secretless.externalsecrets.com/env-vars: "true"
    
    # Federation configuration
    secretless.externalsecrets.com/federated-server-url: "http://federation-server.external-secrets-system:8080"
    secretless.externalsecrets.com/federated-auth: "okta"
    
    # Okta configuration
    secretless.externalsecrets.com/okta-client-id: "0oawl3l22qkmQK274697"
    secretless.externalsecrets.com/okta-private-key: "/okta/private_key.pem"
    secretless.externalsecrets.com/okta-domain: "https://trial-1038013.okta.com"
    secretless.externalsecrets.com/okta-auth-server: ""
spec:
  containers:
  - name: app
    image: my-app:latest
    volumeMounts:
    - name: okta-key
      mountPath: /okta
      readOnly: true
  
  volumes:
  - name: okta-key
    secret:
      secretName: okta-private-key
      defaultMode: 0400
```

## Example: Daemon/Sidecar Mode with File Injection

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: my-app
  annotations:
    secretless.externalsecrets.com/externalsecret: "app-secrets"
    secretless.externalsecrets.com/file-secrets: "true"
    secretless.externalsecrets.com/daemon-refresh-interval: "5m"
    
    # Federation + Okta configuration
    secretless.externalsecrets.com/federated-server-url: "http://federation-server:8080"
    secretless.externalsecrets.com/federated-auth: "okta"
    secretless.externalsecrets.com/okta-client-id: "0oawl3l22qkmQK274697"
    secretless.externalsecrets.com/okta-private-key: "/okta/private_key.pem"
    secretless.externalsecrets.com/okta-domain: "https://trial-1038013.okta.com"
    
    # Custom file injection
    secretless.externalsecrets.com/inject-on-file: "/secrets/db.password=vault-backend.database-password"
spec:
  containers:
  - name: app
    image: my-app:latest
    volumeMounts:
    - name: secrets
      mountPath: /secrets
    - name: okta-key
      mountPath: /okta
      readOnly: true
  
  volumes:
  - name: secrets
    emptyDir: {}
  - name: okta-key
    secret:
      secretName: okta-private-key
```

## Managing the Okta Private Key

### Option 1: Kubernetes Secret (Recommended)

```bash
# Create secret from PEM file
kubectl create secret generic okta-private-key \
  --from-file=private_key.pem=./key.pem \
  --namespace=default

# Or create from literal (not recommended for production)
kubectl create secret generic okta-private-key \
  --from-literal=private_key.pem="$(cat ./key.pem)" \
  --namespace=default
```

### Option 2: External Secrets (Most Secure)

```yaml
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: okta-private-key
spec:
  refreshInterval: 1h
  secretStoreRef:
    name: vault-backend
    kind: ClusterSecretStore
  target:
    name: okta-private-key
    template:
      data:
        private_key.pem: "{{ .privateKey }}"
  data:
  - secretKey: privateKey
    remoteRef:
      key: okta/credentials
      property: private_key
```

### Option 3: ConfigMap (Development Only)

⚠️ **Warning**: Never use ConfigMaps for private keys in production!

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: okta-private-key
data:
  private_key.pem: |
    -----BEGIN RSA PRIVATE KEY-----
    MIIEpAIBAAKCAQEA...
    -----END RSA PRIVATE KEY-----
```

## How It Works

1. **Pod Creation**: User creates a pod with Okta federation annotations
2. **Webhook Intercepts**: The mutating webhook intercepts the pod creation
3. **Validation**: Webhook validates required Okta annotations are present
4. **Init Container Injection**: Webhook injects an init container (or sidecar) with `esi-cli`
5. **CLI Flag Construction**: Webhook builds CLI flags from annotations:
   ```bash
   esi-cli \
     --mode=init \
     --federated-server-url=http://federation-server:8080 \
     --federated-auth=okta \
     --okta-client-id=0oawl3l22qkmQK274697 \
     --okta-private-key=/okta/private_key.pem \
     --okta-domain=https://trial-1038013.okta.com \
     --okta-auth-server="" \
     --inject-on-env=*
   ```
6. **JWT Generation**: `esi-cli` generates signed JWT using the private key
7. **Token Exchange**: `esi-cli` exchanges JWT for Okta access token
8. **Federation Request**: `esi-cli` calls federation server with Bearer token
9. **Secret Injection**: Secrets are injected into the main container

## Security Best Practices

1. **Store private keys in Kubernetes Secrets** with restrictive RBAC
2. **Use `defaultMode: 0400`** for Secret volume mounts (read-only for owner)
3. **Rotate keys regularly** and update Okta configuration
4. **Use namespace isolation** to limit Secret access
5. **Enable audit logging** in Okta to track token usage
6. **Set pod security policies** to prevent privilege escalation
7. **Use network policies** to restrict federation server access

## Troubleshooting

### Pod Fails to Start

**Check webhook logs:**
```bash
kubectl logs -n external-secrets-system deployment/esi-pod-webhook
```

**Common issues:**
- Missing required Okta annotations
- Invalid Okta domain URL format
- Private key Secret not found
- Wrong private key path annotation

### Init Container Fails

**Check init container logs:**
```bash
kubectl logs <pod-name> -c esi-cli-init
```

**Common issues:**
- `"okta-client-id is required"` - Missing or empty client ID annotation
- `"failed to load Okta private key"` - Private key file not mounted or wrong path
- `"token endpoint returned status 400"` - Scope consent error or invalid credentials
- `"no authorization configured for issuer"` - Authorization CRD not configured

### Secrets Not Injected

**Verify annotations:**
```bash
kubectl get pod <pod-name> -o jsonpath='{.metadata.annotations}' | jq
```

**Check if webhook mutated the pod:**
```bash
kubectl get pod <pod-name> -o yaml | grep -A 10 initContainers
```

## Complete Example

See [`k8s/test-pod-okta-federation.yaml`](k8s/test-pod-okta-federation.yaml) for a complete working example.

## Validation

The webhook validates:
- ✅ Required annotations are present when using Okta auth
- ✅ Okta domain is a valid URL
- ✅ Federation server URL is valid
- ✅ Daemon refresh interval is parseable (if specified)

If validation fails, pod creation is rejected with a descriptive error message.

## Annotation Reference

All Okta-related annotations use the prefix: `secretless.externalsecrets.com/`

| Short Name | Full Annotation |
|------------|-----------------|
| `okta-client-id` | `secretless.externalsecrets.com/okta-client-id` |
| `okta-private-key` | `secretless.externalsecrets.com/okta-private-key` |
| `okta-domain` | `secretless.externalsecrets.com/okta-domain` |
| `okta-auth-server` | `secretless.externalsecrets.com/okta-auth-server` |
| `okta-scopes` | `secretless.externalsecrets.com/okta-scopes` |

## Related Documentation

- [esi-cli Okta Federation Guide](../esi-cli/OKTA_FEDERATION.md)
- [Federation Server Setup](../external-secrets-enterprise/docs/federation.md)
- [Okta OAuth2 Configuration](https://developer.okta.com/docs/guides/implement-oauth-for-okta/main/)
