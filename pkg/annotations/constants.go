package annotations

// Annotation prefixes
const (
	AnnotationPrefix = "secretless.externalsecrets.com/"

	// Core annotations
	AnnotationEnvVars        = AnnotationPrefix + "env-vars"
	AnnotationFileSecrets    = AnnotationPrefix + "file-secrets"
	AnnotationExternalSecret = AnnotationPrefix + "externalsecret"
	AnnotationSkip           = AnnotationPrefix + "skip"

	// Federation mode annotations
	AnnotationFederatedServerURL      = AnnotationPrefix + "federated-server-url"
	AnnotationFederatedAuth           = AnnotationPrefix + "federated-auth"
	AnnotationFederatedGenerator      = AnnotationPrefix + "federated-generators"
	AnnotationFederatedToken          = AnnotationPrefix + "federated-token"
	AnnotationFederatedCaCrt          = AnnotationPrefix + "federated-ca-crt"
	AnnotationFederatedSocket         = AnnotationPrefix + "federated-socket-path"
	AnnotationFederatedServerSpiffeID = AnnotationPrefix + "federated-server-spiffe-id"

	// Okta federation annotations
	AnnotationOktaClientID       = AnnotationPrefix + "okta-client-id"
	AnnotationOktaPrivateKeyPath = AnnotationPrefix + "okta-private-key"
	AnnotationOktaDomain         = AnnotationPrefix + "okta-domain"
	AnnotationOktaAuthServerID   = AnnotationPrefix + "okta-auth-server"
	AnnotationOktaScopes         = AnnotationPrefix + "okta-scopes"

	// PingIdentity federation annotations
	AnnotationPingIdentityClientID       = AnnotationPrefix + "pingidentity-client-id"
	AnnotationPingIdentityPrivateKeyPath = AnnotationPrefix + "pingidentity-private-key"
	AnnotationPingIdentityRegion         = AnnotationPrefix + "pingidentity-region"
	AnnotationPingIdentityEnvironmentID  = AnnotationPrefix + "pingidentity-environment-id"
	AnnotationPingIdentityScopes         = AnnotationPrefix + "pingidentity-scopes"

	// Workload token annotations
	AnnotationWorkloadToken     = AnnotationPrefix + "workload-token"
	AnnotationWorkloadTokenPath = AnnotationPrefix + "workload-token-path"

	// Daemon mode annotations
	AnnotationDaemonRefreshInterval = AnnotationPrefix + "daemon-refresh-interval"

	// Environment variable injection annotations
	AnnotationInjectOnEnv  = AnnotationPrefix + "inject-on-env"
	AnnotationInjectOnFile = AnnotationPrefix + "inject-on-file"

	// Image pull secrets annotation
	AnnotationImagePullSecrets = AnnotationPrefix + "image-pull-secrets"
)
