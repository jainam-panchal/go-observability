package logger

import (
	"context"
	"testing"

	"github.com/jainam-panchal/go-observability/telemetry"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestNewAddsServiceFields(t *testing.T) {
	t.Parallel()

	cfg := telemetry.DefaultConfig()
	cfg.ServiceName = "orders-api"
	cfg.ServiceVersion = "2.0.1"
	cfg.Environment = "production"
	cfg.ServiceRole = "api"

	core, observed := observer.New(zapcore.InfoLevel)
	logger, err := newWithCore(cfg, core)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	logger.Info("service boot")

	entry := observed.All()[0]
	if entry.ContextMap()["service"] != "orders-api" {
		t.Fatalf("service = %v, want %q", entry.ContextMap()["service"], "orders-api")
	}
	if entry.ContextMap()["service_version"] != "2.0.1" {
		t.Fatalf("service_version = %v, want %q", entry.ContextMap()["service_version"], "2.0.1")
	}
	if entry.ContextMap()["deployment_environment"] != "production" {
		t.Fatalf("deployment_environment = %v, want %q", entry.ContextMap()["deployment_environment"], "production")
	}
	if entry.ContextMap()["service_role"] != "api" {
		t.Fatalf("service_role = %v, want %q", entry.ContextMap()["service_role"], "api")
	}
}

func TestWithContextAddsTraceFields(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zapcore.InfoLevel)
	base := zap.New(core)
	ctx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
		SpanID:     trace.SpanID{2, 2, 2, 2, 2, 2, 2, 2},
		TraceFlags: trace.FlagsSampled,
	}))

	WithContext(ctx, base).Info("request")

	entry := observed.All()[0]
	if entry.ContextMap()["trace_id"] == nil {
		t.Fatal("trace_id missing from log context")
	}
	if entry.ContextMap()["span_id"] == nil {
		t.Fatal("span_id missing from log context")
	}
}

func TestWithContextLeavesLoggerUntouchedWithoutSpan(t *testing.T) {
	t.Parallel()

	core, observed := observer.New(zapcore.InfoLevel)
	base := zap.New(core)

	WithContext(context.Background(), base).Info("request")

	entry := observed.All()[0]
	if _, ok := entry.ContextMap()["trace_id"]; ok {
		t.Fatal("trace_id present without valid span context")
	}
	if _, ok := entry.ContextMap()["span_id"]; ok {
		t.Fatal("span_id present without valid span context")
	}
}
