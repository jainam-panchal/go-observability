package database

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/jainam-panchal/go-observability/internal/jobmeta"
	"github.com/jainam-panchal/go-observability/internal/requestmeta"
	_ "github.com/mattn/go-sqlite3"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestOpenInstrumentedSQLCreatesQuerySpanInParentTrace(t *testing.T) {
	db, spanRecorder, restore := newInstrumentedSQLDB(t)
	defer restore()

	tracer := otel.GetTracerProvider().Tracer("sql-test")
	ctx, parentSpan := tracer.Start(context.Background(), "parent-query")
	ctx = requestmeta.WithHTTPMetadata(ctx, "GET", "/users/:id")

	var got string
	if err := db.QueryRowContext(ctx, `SELECT name FROM sql_test_models WHERE id = ?`, 1).Scan(&got); err != nil {
		t.Fatalf("QueryRowContext().Scan() error = %v", err)
	}
	parentSpan.End()

	if got != "alice" {
		t.Fatalf("queried name = %q, want %q", got, "alice")
	}

	assertHasSQLSpanInTrace(t, spanRecorder.Ended(), parentSpan.SpanContext().TraceID())
	assertHasSQLSpanAttribute(t, spanRecorder.Ended(), parentSpan.SpanContext().TraceID(), "http.route", "/users/:id")
}

func TestOpenInstrumentedSQLKeepsTransactionWorkInParentTrace(t *testing.T) {
	db, spanRecorder, restore := newInstrumentedSQLDB(t)
	defer restore()

	tracer := otel.GetTracerProvider().Tracer("sql-test")
	ctx, parentSpan := tracer.Start(context.Background(), "parent-tx")
	ctx = requestmeta.WithHTTPMetadata(ctx, "POST", "/jobs")

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx() error = %v", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO sql_test_models (name) VALUES (?)`, "bob"); err != nil {
		t.Fatalf("ExecContext() error = %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	parentSpan.End()

	assertHasSQLSpanInTrace(t, spanRecorder.Ended(), parentSpan.SpanContext().TraceID())
	assertHasSQLSpanAttribute(t, spanRecorder.Ended(), parentSpan.SpanContext().TraceID(), "http.route", "/jobs")
}

func TestOpenInstrumentedSQLAddsWorkerJobMetadataToSpans(t *testing.T) {
	db, spanRecorder, restore := newInstrumentedSQLDB(t)
	defer restore()

	tracer := otel.GetTracerProvider().Tracer("sql-test")
	ctx, parentSpan := tracer.Start(context.Background(), "parent-worker-query")
	ctx = jobmeta.WithJobMetadata(ctx, "thumbnail.render")

	var got string
	if err := db.QueryRowContext(ctx, `SELECT name FROM sql_test_models WHERE id = ?`, 1).Scan(&got); err != nil {
		t.Fatalf("QueryRowContext().Scan() error = %v", err)
	}
	parentSpan.End()

	if got != "alice" {
		t.Fatalf("queried name = %q, want %q", got, "alice")
	}

	assertHasSQLSpanAttribute(t, spanRecorder.Ended(), parentSpan.SpanContext().TraceID(), "job.name", "thumbnail.render")
}

func TestOpenInstrumentedSQLRejectsEmptyDriverName(t *testing.T) {
	if _, err := OpenInstrumentedSQL("", "file::memory:?cache=shared"); err == nil {
		t.Fatal("OpenInstrumentedSQL(empty driver) error = nil, want non-nil")
	}
}

func newInstrumentedSQLDB(t *testing.T) (*sql.DB, *tracetest.SpanRecorder, func()) {
	t.Helper()

	previousTracerProvider := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()

	spanRecorder := tracetest.NewSpanRecorder()
	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	otel.SetTracerProvider(tracerProvider)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := OpenInstrumentedSQL("sqlite3", dsn)
	if err != nil {
		t.Fatalf("OpenInstrumentedSQL() error = %v", err)
	}

	if _, err := db.Exec(`CREATE TABLE sql_test_models (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL)`); err != nil {
		t.Fatalf("CREATE TABLE error = %v", err)
	}
	if _, err := db.Exec(`INSERT INTO sql_test_models (name) VALUES (?)`, "alice"); err != nil {
		t.Fatalf("seed INSERT error = %v", err)
	}

	restore := func() {
		_ = db.Close()
		otel.SetTracerProvider(previousTracerProvider)
		otel.SetTextMapPropagator(previousPropagator)
	}

	return db, spanRecorder, restore
}

func assertHasSQLSpanInTrace(t *testing.T, spans []sdktrace.ReadOnlySpan, traceID trace.TraceID) {
	t.Helper()

	for _, span := range spans {
		if span.SpanContext().TraceID() != traceID {
			continue
		}
		if span.SpanKind() == trace.SpanKindClient {
			return
		}
	}

	t.Fatalf("no client SQL span found in trace %s", traceID)
}

func assertHasSQLSpanAttribute(t *testing.T, spans []sdktrace.ReadOnlySpan, traceID trace.TraceID, key, want string) {
	t.Helper()

	for _, span := range spans {
		if span.SpanContext().TraceID() != traceID || span.SpanKind() != trace.SpanKindClient {
			continue
		}
		for _, attr := range span.Attributes() {
			if string(attr.Key) == key && attr.Value.Type() == attribute.STRING && attr.Value.AsString() == want {
				return
			}
		}
	}

	t.Fatalf("client SQL span attribute %q=%q not found in trace %s", key, want, traceID)
}
