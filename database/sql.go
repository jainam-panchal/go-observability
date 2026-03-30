package database

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/XSAM/otelsql"
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

	options := []otelsql.Option{
		otelsql.WithTracerProvider(otel.GetTracerProvider()),
		otelsql.WithMeterProvider(otel.GetMeterProvider()),
		otelsql.WithAttributes(attribute.String("db.system", sqlSystemName(driverName))),
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
