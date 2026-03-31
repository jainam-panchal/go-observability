package database

import (
	"context"
	"fmt"
	"testing"

	"github.com/jainam-panchal/go-observability/internal/requestmeta"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type gormTestModel struct {
	ID   uint
	Name string
}

func TestInstrumentGORMCreatesQuerySpanWithParentContext(t *testing.T) {
	db, spanRecorder, restore := newInstrumentedTestDB(t)
	defer restore()

	tracer := otel.GetTracerProvider().Tracer("gorm-test")
	ctx, parentSpan := tracer.Start(context.Background(), "parent-query")
	ctx = requestmeta.WithHTTPMetadata(ctx, "GET", "/users/:id")

	var model gormTestModel
	if err := db.WithContext(ctx).First(&model, "name = ?", "alice").Error; err != nil {
		t.Fatalf("First() error = %v", err)
	}
	parentSpan.End()

	assertHasSpanInTraceMatching(t, spanRecorder.Ended(), parentSpan.SpanContext().TraceID(), func(name string) bool {
		return name == "select gorm_test_models"
	})
	assertSpanInTraceHasAttribute(t, spanRecorder.Ended(), parentSpan.SpanContext().TraceID(), "http.route", "/users/:id")
}

func TestInstrumentGORMCreatesTransactionSpansWithParentContext(t *testing.T) {
	db, spanRecorder, restore := newInstrumentedTestDB(t)
	defer restore()

	tracer := otel.GetTracerProvider().Tracer("gorm-test")
	ctx, parentSpan := tracer.Start(context.Background(), "parent-tx")
	ctx = requestmeta.WithHTTPMetadata(ctx, "POST", "/users")

	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Create(&gormTestModel{Name: "bob"}).Error
	})
	if err != nil {
		t.Fatalf("Transaction() error = %v", err)
	}
	parentSpan.End()

	assertHasChildSpan(t, spanRecorder.Ended(), parentSpan.SpanContext(), transactionSpanName)
	assertHasSpanInTraceMatching(t, spanRecorder.Ended(), parentSpan.SpanContext().TraceID(), func(name string) bool {
		return name == "insert gorm_test_models"
	})
	assertSpanInTraceHasAttribute(t, spanRecorder.Ended(), parentSpan.SpanContext().TraceID(), "http.route", "/users")
}

func TestInstrumentGORMRejectsNilDB(t *testing.T) {
	if _, err := InstrumentGORM(nil); err == nil {
		t.Fatal("InstrumentGORM(nil) error = nil, want non-nil")
	}
}

func newInstrumentedTestDB(t *testing.T) (*gorm.DB, *tracetest.SpanRecorder, func()) {
	t.Helper()

	previousTracerProvider := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()

	spanRecorder := tracetest.NewSpanRecorder()
	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	otel.SetTracerProvider(tracerProvider)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}

	db, err = InstrumentGORM(db)
	if err != nil {
		t.Fatalf("InstrumentGORM() error = %v", err)
	}

	if err := db.AutoMigrate(&gormTestModel{}); err != nil {
		t.Fatalf("AutoMigrate() error = %v", err)
	}
	if err := db.Create(&gormTestModel{Name: "alice"}).Error; err != nil {
		t.Fatalf("Create() seed error = %v", err)
	}

	restore := func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
		otel.SetTracerProvider(previousTracerProvider)
		otel.SetTextMapPropagator(previousPropagator)
	}

	return db, spanRecorder, restore
}

func assertHasChildSpan(t *testing.T, spans []sdktrace.ReadOnlySpan, parent trace.SpanContext, name string) {
	t.Helper()

	for _, span := range spans {
		if span.Name() != name {
			continue
		}
		if span.Parent().TraceID() == parent.TraceID() && span.Parent().SpanID() == parent.SpanID() {
			return
		}
	}

	t.Fatalf("child span %q with parent trace=%s span=%s not found; got spans: %v", name, parent.TraceID(), parent.SpanID(), spanNames(spans))
}

func assertHasSpanInTraceMatching(t *testing.T, spans []sdktrace.ReadOnlySpan, traceID trace.TraceID, match func(name string) bool) {
	t.Helper()

	for _, span := range spans {
		if match(span.Name()) && span.SpanContext().TraceID() == traceID {
			return
		}
	}

	t.Fatalf("matching span with trace=%s not found; got spans: %v", traceID, spanNames(spans))
}

func spanNames(spans []sdktrace.ReadOnlySpan) []string {
	names := make([]string, 0, len(spans))
	for _, span := range spans {
		names = append(names, span.Name())
	}

	return names
}

func assertSpanInTraceHasAttribute(t *testing.T, spans []sdktrace.ReadOnlySpan, traceID trace.TraceID, key, want string) {
	t.Helper()

	for _, span := range spans {
		if span.SpanContext().TraceID() != traceID {
			continue
		}
		for _, attr := range span.Attributes() {
			if string(attr.Key) == key && attr.Value.Type() == attribute.STRING && attr.Value.AsString() == want {
				return
			}
		}
	}

	t.Fatalf("attribute %q=%q not found in trace %s", key, want, traceID)
}
