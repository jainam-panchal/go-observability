package database

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"go.opentelemetry.io/otel"
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

	var got string
	if err := db.QueryRowContext(ctx, `SELECT name FROM sql_test_models WHERE id = ?`, 1).Scan(&got); err != nil {
		t.Fatalf("QueryRowContext().Scan() error = %v", err)
	}
	parentSpan.End()

	if got != "alice" {
		t.Fatalf("queried name = %q, want %q", got, "alice")
	}

	assertHasSQLSpanInTrace(t, spanRecorder.Ended(), parentSpan.SpanContext().TraceID())
}

func TestOpenInstrumentedSQLKeepsTransactionWorkInParentTrace(t *testing.T) {
	db, spanRecorder, restore := newInstrumentedSQLDB(t)
	defer restore()

	tracer := otel.GetTracerProvider().Tracer("sql-test")
	ctx, parentSpan := tracer.Start(context.Background(), "parent-tx")

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
