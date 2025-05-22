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
	AnnotationExternalSecret = "secretless.externalsecrets.com/externalsecret"
	AnnotationEnvVars        = "secretless.externalsecrets.com/env-vars"
	AnnotationFileSecrets    = "secretless.externalsecrets.com/file-secrets"
	AnnotationSkip           = "secretless.externalsecrets.com/skip"
)
