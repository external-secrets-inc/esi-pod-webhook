package annotations

// Annotation prefixes
const (
	AnnotationPrefix = "secretless.externalsecrets.com/"

	// Core annotations
	AnnotationEnvVars        = AnnotationPrefix + "env-vars"
	AnnotationFileSecrets    = AnnotationPrefix + "file-secrets"
	AnnotationExternalSecret = AnnotationPrefix + "externalsecret"
	AnnotationSkip          = AnnotationPrefix + "skip"

	// Federation mode annotations
	AnnotationFederatedServerURL  = AnnotationPrefix + "federated-server-url"
	AnnotationFederatedGenerator = AnnotationPrefix + "federated-generators"
	AnnotationFederatedStore     = AnnotationPrefix + "federated-store"
	AnnotationFederatedToken     = AnnotationPrefix + "federated-token"
	AnnotationFederatedCaCrt     = AnnotationPrefix + "federated-ca-crt"

	// Daemon mode annotations
	AnnotationDaemonRefreshInterval = AnnotationPrefix + "daemon-refresh-interval"

	// Environment variable injection annotations
	AnnotationInjectOnEnv = AnnotationPrefix + "inject-on-env"
	AnnotationInjectOnFile = AnnotationPrefix + "inject-on-file"

	// Image pull secrets annotation
	AnnotationImagePullSecrets = AnnotationPrefix + "image-pull-secrets"
)
