package jobmeta

import "context"

type contextKey struct{}

// Metadata holds low-cardinality worker job metadata for context propagation.
type Metadata struct {
	Name string
}

// WithJobMetadata returns a context carrying worker job metadata.
func WithJobMetadata(ctx context.Context, jobName string) context.Context {
	return context.WithValue(ctx, contextKey{}, Metadata{Name: jobName})
}

// FromContext returns worker job metadata when present.
func FromContext(ctx context.Context) (Metadata, bool) {
	metadata, ok := ctx.Value(contextKey{}).(Metadata)
	if !ok || metadata.Name == "" {
		return Metadata{}, false
	}

	return metadata, true
}
