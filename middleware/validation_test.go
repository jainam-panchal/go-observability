package middleware

import (
	"context"
	"errors"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestRunValidationStepSuccess(t *testing.T) {
	restore, recorder := installValidationTestTracerProvider()
	defer restore()

	ctx, err := RunValidationStep(context.Background(), "bind_json", func(stepCtx context.Context) error {
		if !traceContextValid(stepCtx) {
			t.Fatal("validation context does not contain active span")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("RunValidationStep() error = %v", err)
	}
	if !traceContextValid(ctx) {
		t.Fatal("returned context does not contain active span context")
	}

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("ended spans = %d, want 1", len(spans))
	}
	span := spans[0]
	if span.Name() != "app.validation.bind_json" {
		t.Fatalf("span name = %q, want %q", span.Name(), "app.validation.bind_json")
	}
	if got, ok := attrBool(span.Attributes(), "validation.ok"); !ok || !got {
		t.Fatalf("validation.ok = %v (ok=%v), want true", got, ok)
	}
}

func TestRunValidationStepError(t *testing.T) {
	restore, recorder := installValidationTestTracerProvider()
	defer restore()

	_, err := RunValidationStep(context.Background(), "validate_body", func(context.Context) error {
		return errors.New("bad payload")
	})
	if err == nil {
		t.Fatal("RunValidationStep() error = nil, want non-nil")
	}

	spans := recorder.Ended()
	if len(spans) != 1 {
		t.Fatalf("ended spans = %d, want 1", len(spans))
	}
	span := spans[0]
	if got, ok := attrBool(span.Attributes(), "validation.ok"); !ok || got {
		t.Fatalf("validation.ok = %v (ok=%v), want false", got, ok)
	}
}

func TestRunValidationStepRejectsNilFn(t *testing.T) {
	_, err := RunValidationStep(context.Background(), "bind_uri", nil)
	if err == nil {
		t.Fatal("RunValidationStep() with nil fn error = nil, want non-nil")
	}
}

func installValidationTestTracerProvider() (func(), *tracetest.SpanRecorder) {
	previous := otel.GetTracerProvider()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	otel.SetTracerProvider(provider)

	return func() {
		otel.SetTracerProvider(previous)
	}, recorder
}

func traceContextValid(ctx context.Context) bool {
	return trace.SpanContextFromContext(ctx).IsValid()
}

func attrBool(attrs []attribute.KeyValue, key string) (bool, bool) {
	for _, attr := range attrs {
		if string(attr.Key) == key && attr.Value.Type() == attribute.BOOL {
			return attr.Value.AsBool(), true
		}
	}
	return false, false
}
