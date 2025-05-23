package admission

import (
	"crypto/tls"
	"net/http"

	"github.com/external-secrets-inc/esi-pod-webhook/pkg/container"
	"github.com/external-secrets-inc/esi-pod-webhook/pkg/k8s"
)

// Config holds configuration for the webhook server
type Config struct {
	Port         int
	TLSCertPath  string
	TLSKeyPath   string
	ContainerCfg *container.Config
}

// Server represents the webhook server
type Server struct {
	httpServer      *http.Server
	k8sClient       *k8s.Client
	tlsConfig       *tls.Config
	initInjector    container.Injector
	sidecarInjector container.Injector
}

// Annotations used by the webhook
const (
	AnnotationExternalSecret     = "secretless.externalsecrets.com/externalsecret"
	AnnotationEnvVars            = "secretless.externalsecrets.com/env-vars"
	AnnotationImagePullSecrets   = "secretless.externalsecrets.com/image-pull-secrets"
	AnnotationFileSecrets        = "secretless.externalsecrets.com/file-secrets"
	AnnotationFederatedServerURL = "secretless.externalsecrets.com/federated-server-url"
	AnnotationInjectOnEnv        = "secretless.externalsecrets.com/inject-on-env"
	AnnotationFederatedGenerator = "secretless.externalsecrets.com/federated-generator"
	AnnotationFederatedStore     = "secretless.externalsecrets.com/federated-store"
	AnnotationSkip               = "secretless.externalsecrets.com/skip"
)
