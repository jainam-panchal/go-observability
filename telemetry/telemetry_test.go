package telemetry

import (
	"context"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func TestInitReturnsShutdownWithSignalsDisabled(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.TracesEnabled = false
	cfg.MetricsEnabled = false

	shutdown, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	if shutdown == nil {
		t.Fatal("Init() shutdown = nil, want non-nil")
	}

	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown() error = %v", err)
	}
}

func TestInitSetsGlobals(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.TracesEnabled = false
	cfg.MetricsEnabled = false

	shutdown, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	defer func() {
		if err := shutdown(context.Background()); err != nil {
			t.Fatalf("shutdown() error = %v", err)
		}
	}()

	if otel.GetTracerProvider() == nil {
		t.Fatal("GetTracerProvider() = nil, want non-nil")
	}
	if otel.GetMeterProvider() == nil {
		t.Fatal("GetMeterProvider() = nil, want non-nil")
	}

	propagator := otel.GetTextMapPropagator()
	if propagator == nil {
		t.Fatal("GetTextMapPropagator() = nil, want non-nil")
	}

	carrier := propagation.MapCarrier{}
	ctx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
		SpanID:     trace.SpanID{1, 2, 3, 4, 5, 6, 7, 8},
		TraceFlags: trace.FlagsSampled,
		Remote:     true,
	}))
	propagator.Inject(ctx, carrier)
	if _, ok := carrier["traceparent"]; !ok {
		t.Fatalf("propagator inject = %v, want traceparent header", carrier)
	}
}

func TestInitRejectsInvalidConfig(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.TraceSamplingRate = 2

	shutdown, err := Init(cfg)
	if err == nil {
		t.Fatal("Init() error = nil, want non-nil")
	}
	if shutdown != nil {
		t.Fatal("Init() shutdown != nil, want nil on error")
	}
	if !strings.Contains(err.Error(), "trace sampling rate") {
		t.Fatalf("Init() error = %q, want sampling rate error", err)
	}
}

func TestMustInitPanicsOnInvalidConfig(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.ServiceName = ""

	defer func() {
		recovered := recover()
		if recovered == nil {
			t.Fatal("MustInit() did not panic")
		}
	}()

	_ = MustInit(cfg)
}
