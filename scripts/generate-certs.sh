#!/bin/bash
set -e

# Create directory for certs
mkdir -p certs

# Generate CA key and certificate
openssl genrsa -out certs/ca.key 2048
openssl req -x509 -new -nodes -key certs/ca.key -subj "/CN=Webhook CA" -days 365 -out certs/ca.crt

# Generate server key and CSR
openssl genrsa -out certs/tls.key 2048
openssl req -new -key certs/tls.key \
    -subj "/CN=esi-pod-webhook.secretless-system.svc" \
    -out certs/tls.csr

# Create config for SAN
cat > certs/tls.conf << EOF
[req]
req_extensions = v3_req
distinguished_name = req_distinguished_name

[req_distinguished_name]

[v3_req]
basicConstraints = CA:FALSE
keyUsage = nonRepudiation, digitalSignature, keyEncipherment
extendedKeyUsage = serverAuth
subjectAltName = @alt_names

[alt_names]
DNS.1 = esi-pod-webhook.secretless-system.svc
DNS.2 = esi-pod-webhook.secretless-system.svc.cluster.local
EOF

# Generate server certificate
openssl x509 -req -in certs/tls.csr \
    -CA certs/ca.crt \
    -CAkey certs/ca.key \
    -CAcreateserial \
    -out certs/tls.crt \
    -days 365 \
    -extensions v3_req \
    -extfile certs/tls.conf

# Create Kubernetes secret
kubectl create secret tls esi-pod-webhook-cert \
    --cert=certs/tls.crt \
    --key=certs/tls.key \
    -n secretless-system \
    --dry-run=client -o yaml | kubectl apply -f -
