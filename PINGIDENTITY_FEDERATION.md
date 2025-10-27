# PingIdentity Federation Support in ESI Pod Webhook

This document explains how to use PingOne authentication with the ESI Pod Webhook to enable pods to access federated External Secrets servers using PingIdentity OAuth2 credentials.

## Overview

The ESI Pod Webhook now supports PingOne OAuth2 client credentials flow for federation authentication. This allows pods to authenticate to federation servers using PingOne access tokens instead of Kubernetes service account tokens or SPIFFE mTLS.

## Supported Federation Auth Methods

The webhook supports four federation authentication methods via the `secretless.externalsecrets.com/federated-auth` annotation:

- **`kubernetes`** - Service account token authentication (default)
- **`spiffe`** - SPIFFE/SPIRE mTLS authentication  
- **`okta`** - Okta OAuth2 client credentials with private_key_jwt
- **`pingidentity`** - PingOne OAuth2 client credentials with private_key_jwt (NEW)

## Prerequisites

1. **Federation Server** deployed with PingIdentity authentication support
2. **PingOne Application** configured with:
   - Grant type: Client Credentials
   - Token endpoint authentication method: Private key JWT
   - Your public key registered in PingOne
3. **PingIdentityFederation and Authorization CRDs** deployed in the cluster
4. **Private Key** stored as a Kubernetes Secret

## Annotations

### Required Annotations

| Annotation | Description | Example |
|------------|-------------|---------|
| `secretless.externalsecrets.com/externalsecret` | Name of the ExternalSecret | `"test-app-secrets"` |
| `secretless.externalsecrets.com/federated-server-url` | Federation server URL | `"http://federation-server:8080"` |
| `secretless.externalsecrets.com/federated-auth` | Authentication method | `"pingidentity"` |
| `secretless.externalsecrets.com/pingidentity-client-id` | PingOne application client ID (UUID) | `"12345678-1234-1234-1234-123456789abc"` |
| `secretless.externalsecrets.com/pingidentity-private-key` | Path to private key in pod | `"/pingidentity/private_key.pem"` |
| `secretless.externalsecrets.com/pingidentity-region` | PingOne region | `"com"` (or `eu`, `asia`, `ca`) |
| `secretless.externalsecrets.com/pingidentity-environment-id` | PingOne environment ID (UUID) | `"87654321-4321-4321-4321-cba987654321"` |

### Optional Annotations

| Annotation | Description | Default | Example |
|------------|-------------|---------|---------|
| `secretless.externalsecrets.com/pingidentity-scopes` | Space-separated OAuth2 scopes | `""` | `"openid profile"` |
| `secretless.externalsecrets.com/inject-on-env` | Environment variable mapping | All keys | `"DB_PASS=vault.db-password"` |
| `secretless.externalsecrets.com/inject-on-file` | File injection mapping | - | `"/secrets/db.txt=vault.db-password"` |

## Example: Init Container Mode with PingIdentity Federation

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
    secretless.externalsecrets.com/federated-auth: "pingidentity"
    
    # PingIdentity configuration
    secretless.externalsecrets.com/pingidentity-client-id: "12345678-1234-1234-1234-123456789abc"
    secretless.externalsecrets.com/pingidentity-private-key: "/pingidentity/private_key.pem"
    secretless.externalsecrets.com/pingidentity-region: "com"
    secretless.externalsecrets.com/pingidentity-environment-id: "87654321-4321-4321-4321-cba987654321"
spec:
  containers:
  - name: app
    image: my-app:latest
    volumeMounts:
    - name: pingidentity-key
      mountPath: /pingidentity
      readOnly: true
  
  volumes:
  - name: pingidentity-key
    secret:
      secretName: pingidentity-private-key
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
    
    # Federation + PingIdentity configuration
    secretless.externalsecrets.com/federated-server-url: "http://federation-server:8080"
    secretless.externalsecrets.com/federated-auth: "pingidentity"
    secretless.externalsecrets.com/pingidentity-client-id: "12345678-1234-1234-1234-123456789abc"
    secretless.externalsecrets.com/pingidentity-private-key: "/pingidentity/private_key.pem"
    secretless.externalsecrets.com/pingidentity-region: "eu"
    secretless.externalsecrets.com/pingidentity-environment-id: "87654321-4321-4321-4321-cba987654321"
    
    # Custom file injection
    secretless.externalsecrets.com/inject-on-file: "/secrets/db.password=vault-backend.database-password"
spec:
  containers:
  - name: app
    image: my-app:latest
    volumeMounts:
    - name: secrets
      mountPath: /secrets
    - name: pingidentity-key
      mountPath: /pingidentity
      readOnly: true
  
  volumes:
  - name: secrets
    emptyDir: {}
  - name: pingidentity-key
    secret:
      secretName: pingidentity-private-key
```

## PingOne Regions

PingOne operates in multiple geographic regions. Specify the correct region for your environment:

| Region Code | Geographic Location | Token Endpoint |
|-------------|---------------------|----------------|
| `com` | North America | `https://auth.pingone.com/{envID}/as/token.oauth2` |
| `eu` | Europe | `https://auth.pingone.eu/{envID}/as/token.oauth2` |
| `asia` | Asia Pacific | `https://auth.pingone.asia/{envID}/as/token.oauth2` |
| `ca` | Canada | `https://auth.pingone.ca/{envID}/as/token.oauth2` |

## Managing the PingIdentity Private Key

### Option 1: Kubernetes Secret (Recommended)

```bash
# Create secret from PEM file
kubectl create secret generic pingidentity-private-key \
  --from-file=private_key.pem=./key.pem \
  --namespace=default

# Or create from literal (not recommended for production)
kubectl create secret generic pingidentity-private-key \
  --from-literal=private_key.pem="$(cat ./key.pem)" \
  --namespace=default
```

### Option 2: External Secrets (Most Secure)

```yaml
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: pingidentity-private-key
spec:
  refreshInterval: 1h
  secretStoreRef:
    name: vault-backend
    kind: ClusterSecretStore
  target:
    name: pingidentity-private-key
    template:
      data:
        private_key.pem: "{{ .privateKey }}"
  data:
  - secretKey: privateKey
    remoteRef:
      key: pingidentity/credentials
      property: private_key
```

### Option 3: ConfigMap (Development Only)

⚠️ **Warning**: Never use ConfigMaps for private keys in production!

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: pingidentity-private-key
data:
  private_key.pem: |
    -----BEGIN RSA PRIVATE KEY-----
    MIIEpAIBAAKCAQEA...
    -----END RSA PRIVATE KEY-----
```

## How It Works

1. **Pod Creation**: User creates a pod with PingIdentity federation annotations
2. **Webhook Intercepts**: The mutating webhook intercepts the pod creation
3. **Validation**: Webhook validates required PingIdentity annotations are present
4. **Init Container Injection**: Webhook injects an init container (or sidecar) with `esi-cli`
5. **CLI Flag Construction**: Webhook builds CLI flags from annotations:
   ```bash
   esi-cli \
     --mode=init \
     --federated-server-url=http://federation-server:8080 \
     --federated-auth=pingidentity \
     --pingidentity-client-id=12345678-1234-1234-1234-123456789abc \
     --pingidentity-private-key=/pingidentity/private_key.pem \
     --pingidentity-region=com \
     --pingidentity-environment-id=87654321-4321-4321-4321-cba987654321 \
     --inject-on-env=*
   ```
6. **OIDC Discovery**: `esi-cli` discovers PingOne endpoints via `.well-known/openid-configuration`
7. **JWT Generation**: `esi-cli` generates signed JWT using the private key
8. **Token Exchange**: `esi-cli` exchanges JWT for PingOne access token
9. **Federation Request**: `esi-cli` calls federation server with Bearer token
10. **Secret Injection**: Secrets are injected into the main container

## Security Best Practices

1. **Store private keys in Kubernetes Secrets** with restrictive RBAC
2. **Use `defaultMode: 0400`** for Secret volume mounts (read-only for owner)
3. **Rotate keys regularly** and update PingOne configuration
4. **Use namespace isolation** to limit Secret access
5. **Enable audit logging** in PingOne to track token usage
6. **Set pod security policies** to prevent privilege escalation
7. **Use network policies** to restrict federation server access
8. **Use separate PingOne environments** for dev, staging, and production

## Troubleshooting

### Pod Fails to Start

**Check webhook logs:**
```bash
kubectl logs -n external-secrets-system deployment/esi-pod-webhook
```

**Common issues:**
- Missing required PingIdentity annotations
- Invalid region (must be: com, eu, asia, or ca)
- Private key Secret not found
- Wrong private key path annotation

### Init Container Fails

**Check init container logs:**
```bash
kubectl logs <pod-name> -c esi-cli-init
```

**Common issues:**
- `"pingidentity-client-id is required"` - Missing or empty client ID annotation
- `"pingidentity-region is required"` - Missing region annotation
- `"pingidentity-environment-id is required"` - Missing environment ID annotation
- `"failed to load PingIdentity private key"` - Private key file not mounted or wrong path
- `"token endpoint returned status 400"` - Invalid credentials or configuration
- `"failed to fetch discovery document"` - Network issue or invalid region/environment ID
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

See [`k8s/test-pod-pingidentity-federation.yaml`](k8s/test-pod-pingidentity-federation.yaml) for a complete working example.

## Validation

The webhook validates:
- ✅ Required annotations are present when using PingIdentity auth
- ✅ Region is one of the valid values: `com`, `eu`, `asia`, `ca`
- ✅ Federation server URL is valid
- ✅ Daemon refresh interval is parseable (if specified)

If validation fails, pod creation is rejected with a descriptive error message.

## Annotation Reference

All PingIdentity-related annotations use the prefix: `secretless.externalsecrets.com/`

| Short Name | Full Annotation |
|------------|-----------------|
| `pingidentity-client-id` | `secretless.externalsecrets.com/pingidentity-client-id` |
| `pingidentity-private-key` | `secretless.externalsecrets.com/pingidentity-private-key` |
| `pingidentity-region` | `secretless.externalsecrets.com/pingidentity-region` |
| `pingidentity-environment-id` | `secretless.externalsecrets.com/pingidentity-environment-id` |
| `pingidentity-scopes` | `secretless.externalsecrets.com/pingidentity-scopes` |

## Related Documentation

- [esi-cli PingIdentity Federation Guide](../esi-cli/PINGIDENTITY_FEDERATION.md)
- [Federation Server Setup](../external-secrets-enterprise/docs/federation.md)
- [PingOne OAuth2 Configuration](https://docs.pingidentity.com/r/en-us/pingone/p1_c_oauth_2_0)
- [PingOne Regions](https://docs.pingidentity.com/r/en-us/pingone/pingone_c_pingoneregions)
