package annotations

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

type flagBuilder struct{}

// NewFlagBuilder creates a new flag builder
func NewFlagBuilder() FlagBuilder {
	return &flagBuilder{}
}

// BuildFlags builds CLI flags from pod annotations and original command
func (b *flagBuilder) BuildFlags(annotations map[string]string, mode Mode, opts ...BuildOption) ([]string, error) {
	// Process options
	options := &buildOptions{}
	for _, opt := range opts {
		opt(options)
	}

	flags := []string{}

	// Get external secret name
	externalSecretName := annotations[AnnotationExternalSecret]
	flags = append(flags, "--external-secrets="+externalSecretName)

	// Add mode flag
	flags = append(flags, "--mode="+string(mode))

	// Add mode-specific flags
	switch mode {
	case InitMode:
		if len(options.command) > 0 || len(options.args) > 0 {
			if len(options.command) > 0 {
				flags = append(flags, "--binary-path="+options.command[0])
				if len(options.command) > 1 {
					options.args = append(options.command[1:], options.args...)
				}
			}

			if len(options.args) > 0 {
				flags = append(flags, "--args="+strings.Join(options.args, ","))
			}
		}
	case DaemonMode:
		if options.filePath != "" {
			flags = append(flags, "--inject-on-file="+options.filePath+"="+externalSecretName)
		}
	}

	// Handle federated mode flags
	if serverURL, ok := annotations[AnnotationFederatedServerURL]; ok {
		flags = append(flags, "--federated-server-url="+serverURL)

		// Optional federated flags
		if auth, ok := annotations[AnnotationFederatedAuth]; ok {
			flags = append(flags, "--federated-auth="+auth)
		}
		if generator, ok := annotations[AnnotationFederatedGenerator]; ok {
			flags = append(flags, "--federated-generators="+generator)
		}

		// Kubernetes auth flags
		if tokenPath, ok := annotations[AnnotationFederatedToken]; ok {
			flags = append(flags, "--federated-token="+tokenPath)
		}
		if caCrtPath, ok := annotations[AnnotationFederatedCaCrt]; ok {
			flags = append(flags, "--federated-ca-crt="+caCrtPath)
		}

		// SPIFFE auth flags
		if socketPath, ok := annotations[AnnotationFederatedSocket]; ok {
			flags = append(flags, "--federated-socket-path="+socketPath)
		}
		if serverSpiffeId, ok := annotations[AnnotationFederatedServerSpiffeID]; ok {
			flags = append(flags, "--federated-server-spiffe-id="+serverSpiffeId)
		}

		// Okta auth flags
		if clientID, ok := annotations[AnnotationOktaClientID]; ok {
			flags = append(flags, "--okta-client-id="+clientID)
		}
		if privateKeyPath, ok := annotations[AnnotationOktaPrivateKeyPath]; ok {
			flags = append(flags, "--okta-private-key="+privateKeyPath)
		}
		if domain, ok := annotations[AnnotationOktaDomain]; ok {
			flags = append(flags, "--okta-domain="+domain)
		}
		if authServerID, ok := annotations[AnnotationOktaAuthServerID]; ok {
			flags = append(flags, "--okta-auth-server="+authServerID)
		}
		if scopes, ok := annotations[AnnotationOktaScopes]; ok {
			flags = append(flags, "--okta-scopes="+scopes)
		}

		// PingIdentity auth flags
		if clientID, ok := annotations[AnnotationPingIdentityClientID]; ok {
			flags = append(flags, "--pingidentity-client-id="+clientID)
		}
		if privateKeyPath, ok := annotations[AnnotationPingIdentityPrivateKeyPath]; ok {
			flags = append(flags, "--pingidentity-private-key="+privateKeyPath)
		}
		if region, ok := annotations[AnnotationPingIdentityRegion]; ok {
			flags = append(flags, "--pingidentity-region="+region)
		}
		if environmentID, ok := annotations[AnnotationPingIdentityEnvironmentID]; ok {
			flags = append(flags, "--pingidentity-environment-id="+environmentID)
		}
		if scopes, ok := annotations[AnnotationPingIdentityScopes]; ok {
			flags = append(flags, "--pingidentity-scopes="+scopes)
		}

		// Workload token flags
		if workloadToken, ok := annotations[AnnotationWorkloadToken]; ok {
			flags = append(flags, "--workload-token="+workloadToken)
		}
		if workloadTokenPath, ok := annotations[AnnotationWorkloadTokenPath]; ok {
			flags = append(flags, "--workload-token-path="+workloadTokenPath)
		}
	}

	// Handle inject-on-env flag
	if pattern, ok := annotations[AnnotationInjectOnEnv]; ok {
		flags = append(flags, "--inject-on-env="+pattern)
	} else if mode == InitMode {
		flags = append(flags, "--inject-on-env=*") // Special value to get all keys, only in init mode
	}
	// Handle inject-on-file flag
	if pattern, ok := annotations[AnnotationInjectOnFile]; ok {
		flags = append(flags, "--inject-on-file="+pattern)
	}

	// Handle daemon refresh interval
	if interval, ok := annotations[AnnotationDaemonRefreshInterval]; ok {
		flags = append(flags, "--daemon-refresh-interval="+interval)
	}

	return flags, nil
}

// ValidateAnnotations validates the annotations
func (b *flagBuilder) ValidateAnnotations(annotations map[string]string) error {
	// Check for required external secret annotation
	if _, ok := annotations[AnnotationExternalSecret]; !ok {
		return fmt.Errorf("missing required annotation: %s", AnnotationExternalSecret)
	}

	// If federated mode is enabled, validate required flags
	if _, ok := annotations[AnnotationFederatedServerURL]; ok {
		// Validate server URL format
		if _, err := url.ParseRequestURI(annotations[AnnotationFederatedServerURL]); err != nil {
			return fmt.Errorf("invalid server URL: %v", err)
		}

		// If Okta auth is selected, validate required Okta parameters
		if auth, ok := annotations[AnnotationFederatedAuth]; ok && auth == "okta" {
			if _, ok := annotations[AnnotationOktaClientID]; !ok {
				return fmt.Errorf("okta auth requires annotation: %s", AnnotationOktaClientID)
			}
			if _, ok := annotations[AnnotationOktaPrivateKeyPath]; !ok {
				return fmt.Errorf("okta auth requires annotation: %s", AnnotationOktaPrivateKeyPath)
			}
			if _, ok := annotations[AnnotationOktaDomain]; !ok {
				return fmt.Errorf("okta auth requires annotation: %s", AnnotationOktaDomain)
			}
			// Validate Okta domain URL format
			if oktaDomain, ok := annotations[AnnotationOktaDomain]; ok {
				if _, err := url.ParseRequestURI(oktaDomain); err != nil {
					return fmt.Errorf("invalid Okta domain URL: %v", err)
				}
			}
		}

		// If PingIdentity auth is selected, validate required PingIdentity parameters
		if auth, ok := annotations[AnnotationFederatedAuth]; ok && auth == "pingidentity" {
			if _, ok := annotations[AnnotationPingIdentityClientID]; !ok {
				return fmt.Errorf("pingidentity auth requires annotation: %s", AnnotationPingIdentityClientID)
			}
			if _, ok := annotations[AnnotationPingIdentityPrivateKeyPath]; !ok {
				return fmt.Errorf("pingidentity auth requires annotation: %s", AnnotationPingIdentityPrivateKeyPath)
			}
			if _, ok := annotations[AnnotationPingIdentityRegion]; !ok {
				return fmt.Errorf("pingidentity auth requires annotation: %s", AnnotationPingIdentityRegion)
			}
			if _, ok := annotations[AnnotationPingIdentityEnvironmentID]; !ok {
				return fmt.Errorf("pingidentity auth requires annotation: %s", AnnotationPingIdentityEnvironmentID)
			}
			// Validate region is one of the supported values
			if region, ok := annotations[AnnotationPingIdentityRegion]; ok {
				validRegions := map[string]bool{
					"com":  true,
					"eu":   true,
					"asia": true,
					"ca":   true,
				}
				if !validRegions[region] {
					return fmt.Errorf("invalid PingIdentity region '%s': must be one of com, eu, asia, ca", region)
				}
			}
		}
	}

	// Validate daemon refresh interval if specified
	if interval, ok := annotations[AnnotationDaemonRefreshInterval]; ok {
		if _, err := time.ParseDuration(interval); err != nil {
			return fmt.Errorf("invalid daemon refresh interval: %v", err)
		}
	}

	return nil
}
