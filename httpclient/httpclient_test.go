package httpclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestHTTPClientNewClientCreatesChildSpanAndInjectsTraceContext(t *testing.T) {
	t.Parallel()

	spanRecorder := tracetest.NewSpanRecorder()
	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))

	previousTracerProvider := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()
	otel.SetTracerProvider(tracerProvider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	defer otel.SetTracerProvider(previousTracerProvider)
	defer otel.SetTextMapPropagator(previousPropagator)

	serverTraceParent := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverTraceParent <- r.Header.Get("traceparent")
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	client := NewClient(nil)
	tracer := tracerProvider.Tracer("httpclient-test")
	ctx, parentSpan := tracer.Start(context.Background(), "parent")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext() error = %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	resp.Body.Close()
	parentSpan.End()

	traceParent := <-serverTraceParent
	if traceParent == "" {
		t.Fatal("traceparent header missing from outbound request")
	}

	spans := spanRecorder.Ended()
	if len(spans) < 2 {
		t.Fatalf("ended spans = %d, want at least 2", len(spans))
	}

	var parentSnapshot sdktrace.ReadOnlySpan
	clientSnapshots := make([]sdktrace.ReadOnlySpan, 0, len(spans))
	for _, span := range spans {
		switch span.Name() {
		case "parent":
			parentSnapshot = span
		}
		if span.SpanKind() == trace.SpanKindClient {
			clientSnapshots = append(clientSnapshots, span)
		}
	}

	if parentSnapshot == nil {
		t.Fatal("parent span missing")
	}
	if len(clientSnapshots) == 0 {
		t.Fatal("client span missing")
	}

	for _, clientSnapshot := range clientSnapshots {
		if clientSnapshot.Parent().SpanID() == parentSnapshot.SpanContext().SpanID() &&
			clientSnapshot.Parent().TraceID() == parentSnapshot.SpanContext().TraceID() {
			return
		}
	}

	t.Fatalf("no client span found with parent span id %s", parentSnapshot.SpanContext().SpanID())
}

func TestHTTPClientNewTransportUsesProvidedBaseRoundTripper(t *testing.T) {
	t.Parallel()

	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != "https://example.com/users" {
			t.Fatalf("request URL = %s", req.URL.String())
		}

		return &http.Response{
			StatusCode: http.StatusNoContent,
			Body:       http.NoBody,
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})

	client := &http.Client{Transport: NewTransport(base)}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodDelete, "https://example.com/users", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext() error = %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	resp.Body.Close()
}

func TestHTTPClientNewTransportUsesDefaultTransportWhenNil(t *testing.T) {
	t.Parallel()

	transport := NewTransport(nil)
	if transport == nil {
		t.Fatal("transport is nil")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
