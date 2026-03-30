package httpclient

import (
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
)

// NewTransport wraps an HTTP transport so outbound requests create client spans
// and propagate trace context headers downstream.
func NewTransport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}

	return otelhttp.NewTransport(
		base,
		otelhttp.WithTracerProvider(otel.GetTracerProvider()),
		otelhttp.WithPropagators(otel.GetTextMapPropagator()),
		otelhttp.WithMeterProvider(otel.GetMeterProvider()),
		otelhttp.WithSpanNameFormatter(func(_ string, req *http.Request) string {
			return req.Method
		}),
	)
}

// NewClient returns an HTTP client whose transport is instrumented for
// outbound tracing and context propagation.
func NewClient(base *http.Client) *http.Client {
	if base == nil {
		return &http.Client{Transport: NewTransport(nil)}
	}

	client := *base
	client.Transport = NewTransport(base.Transport)

	return &client
}
