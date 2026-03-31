package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/XSAM/otelsql"
	"github.com/jainam-panchal/go-observability/internal/jobmeta"
	"github.com/jainam-panchal/go-observability/internal/requestmeta"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

// OpenInstrumentedSQL opens a database/sql handle with OpenTelemetry
// instrumentation using the configured global tracer and meter providers.
func OpenInstrumentedSQL(driverName, dsn string) (*sql.DB, error) {
	driverName = strings.TrimSpace(driverName)
	if driverName == "" {
		return nil, errors.New("driver name must not be empty")
	}

	restoreSemConv := enableStableDBSemConv()
	defer restoreSemConv()

	options := []otelsql.Option{
		otelsql.WithTracerProvider(otel.GetTracerProvider()),
		otelsql.WithMeterProvider(otel.GetMeterProvider()),
		otelsql.WithAttributes(attribute.String("db.system", sqlSystemName(driverName))),
		otelsql.WithAttributesGetter(func(ctx context.Context, method otelsql.Method, query string, args []driver.NamedValue) []attribute.KeyValue {
			return dbContextAttributes(ctx, string(method), sqlSystemName(driverName))
		}),
		otelsql.WithInstrumentAttributesGetter(func(ctx context.Context, method otelsql.Method, query string, args []driver.NamedValue) []attribute.KeyValue {
			return dbContextAttributes(ctx, string(method), sqlSystemName(driverName))
		}),
	}

	db, err := otelsql.Open(driverName, dsn, options...)
	if err != nil {
		return nil, fmt.Errorf("open instrumented sql db: %w", err)
	}

	if err := otelsql.RegisterDBStatsMetrics(db, options...); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("register sql db stats metrics: %w", err)
	}

	return db, nil
}

func sqlSystemName(driverName string) string {
	switch strings.ToLower(driverName) {
	case "postgres", "postgresql", "pgx":
		return "postgresql"
	case "mysql":
		return "mysql"
	case "sqlite", "sqlite3":
		return "sqlite"
	default:
		return driverName
	}
}

func dbContextAttributes(ctx context.Context, operation, system string) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, 4)
	if system != "" {
		attrs = append(attrs, attribute.String("db.system", system))
	}
	if operation != "" {
		attrs = append(attrs, attribute.String("db.operation", strings.ToLower(operation)))
	}
	if metadata, ok := requestmeta.HTTPMetadataFromContext(ctx); ok {
		if metadata.Method != "" {
			attrs = append(attrs, attribute.String("http.request.method", metadata.Method))
		}
		if metadata.Route != "" {
			attrs = append(attrs, attribute.String("http.route", metadata.Route))
		}
	}
	if metadata, ok := jobmeta.FromContext(ctx); ok && metadata.Name != "" {
		attrs = append(attrs, attribute.String("job.name", metadata.Name))
	}

	return attrs
}

func enableStableDBSemConv() func() {
	const envKey = "OTEL_SEMCONV_STABILITY_OPT_IN"
	current, exists := os.LookupEnv(envKey)
	if exists {
		if strings.Contains(current, "database/dup") || strings.Contains(current, "database") {
			return func() {}
		}
		_ = os.Setenv(envKey, current+",database/dup")
		return func() {
			_ = os.Setenv(envKey, current)
		}
	}

	_ = os.Setenv(envKey, "database/dup")
	return func() {
		_ = os.Unsetenv(envKey)
	}
}
