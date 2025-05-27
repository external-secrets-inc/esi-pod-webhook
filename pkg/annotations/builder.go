package annotations

import (
	"fmt"
	"net/url"
	"strings"
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
		if len(options.originalCommand) == 0 {
			return nil, fmt.Errorf("original command is required for init mode")
		}
		flags = append(flags,
			"--binary-path="+options.originalCommand[0],
			"--args="+strings.Join(options.originalCommand[1:], ","),
		)
	case DaemonMode:
		if options.filePath != "" {
			flags = append(flags, "--inject-on-file="+options.filePath+"="+externalSecretName)
		}
	}

	// Handle federated mode flags
	if serverURL, ok := annotations[AnnotationFederatedServerURL]; ok {
		flags = append(flags, "--federated-server-url="+serverURL)

		// Optional federated flags
		if generator, ok := annotations[AnnotationFederatedGenerator]; ok {
			flags = append(flags, "--federated-generators="+generator)
		}
		if store, ok := annotations[AnnotationFederatedStore]; ok {
			flags = append(flags, "--federated-store="+store)
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
	}

	return nil
}
