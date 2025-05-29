package annotations

// Mode represents the mode of operation for the esi-cli
type Mode string

const (
	// InitMode represents the init container mode
	InitMode Mode = "init"
	// DaemonMode represents the sidecar container mode
	DaemonMode Mode = "daemon"
)

// FlagBuilder builds CLI flags from pod annotations
type FlagBuilder interface {
	// BuildFlags builds CLI flags from pod annotations and original command
	BuildFlags(annotations map[string]string, mode Mode, opts ...BuildOption) ([]string, error)
	// ValidateAnnotations validates the annotations
	ValidateAnnotations(annotations map[string]string, mode Mode) error
}

// BuildOption represents an option for building flags
type BuildOption func(*buildOptions)

// buildOptions contains options for building flags
type buildOptions struct {
	command  []string
	args     []string
	filePath string
}

// WithOriginalCommand sets the original command for init mode
func WithOriginalCommand(cmd []string) BuildOption {
	return func(o *buildOptions) {
		o.command = cmd
	}
}

// WithOriginalArgs sets the original args for init mode
func WithOriginalArgs(args []string) BuildOption {
	return func(o *buildOptions) {
		o.args = args
	}
}

// WithFilePath sets the file path for daemon mode
func WithFilePath(path string) BuildOption {
	return func(o *buildOptions) {
		o.filePath = path
	}
}
