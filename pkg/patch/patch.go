package patch

// Operation represents a JSON patch operation
type Operation struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

// Builder helps construct a list of patch operations
type Builder struct {
	patches []Operation
}

// NewBuilder creates a new patch builder
func NewBuilder() *Builder {
	return &Builder{
		patches: make([]Operation, 0),
	}
}

// Add creates an "add" operation
func (b *Builder) Add(path string, value interface{}) *Builder {
	b.patches = append(b.patches, Operation{
		Op:    "add",
		Path:  path,
		Value: value,
	})
	return b
}

// Remove creates a "remove" operation
func (b *Builder) Remove(path string) *Builder {
	b.patches = append(b.patches, Operation{
		Op:   "remove",
		Path: path,
	})
	return b
}

// Replace creates a "replace" operation
func (b *Builder) Replace(path string, value interface{}) *Builder {
	b.patches = append(b.patches, Operation{
		Op:    "replace",
		Path:  path,
		Value: value,
	})
	return b
}

// Build returns the list of patch operations
func (b *Builder) Build() []Operation {
	return b.patches
}
