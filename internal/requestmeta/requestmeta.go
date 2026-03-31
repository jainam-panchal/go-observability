package requestmeta

import "context"

type httpMetadataKey struct{}

type HTTPMetadata struct {
	Method string
	Route  string
}

func WithHTTPMetadata(ctx context.Context, method, route string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}

	return context.WithValue(ctx, httpMetadataKey{}, HTTPMetadata{
		Method: method,
		Route:  route,
	})
}

func HTTPMetadataFromContext(ctx context.Context) (HTTPMetadata, bool) {
	if ctx == nil {
		return HTTPMetadata{}, false
	}

	value := ctx.Value(httpMetadataKey{})
	metadata, ok := value.(HTTPMetadata)
	if !ok {
		return HTTPMetadata{}, false
	}

	if metadata.Method == "" && metadata.Route == "" {
		return HTTPMetadata{}, false
	}

	return metadata, true
}
