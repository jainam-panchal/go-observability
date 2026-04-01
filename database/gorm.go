package database

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
	otelgorm "gorm.io/plugin/opentelemetry/tracing"
)

const (
	transactionSpanName = "gorm.Transaction"
	transactionSpanKey  = "go-observability:gorm-transaction-span"
	metricsStartKeyBase = "go-observability:gorm-metrics-start:"
)

var (
	stringLiteralRE = regexp.MustCompile(`'(?:''|[^'])*'`)
	numberLiteralRE = regexp.MustCompile(`\b\d+(?:\.\d+)?\b`)
	spaceRE         = regexp.MustCompile(`\s+`)
)

const (
	dbTemplateKey = "go-observability:db-query-template"
	dbTableKey    = "go-observability:db-table"
)

// InstrumentGORM registers tracing instrumentation on an existing GORM
// database handle and returns the same handle for fluent wiring.
func InstrumentGORM(db *gorm.DB) (*gorm.DB, error) {
	if db == nil {
		return nil, errors.New("gorm db must not be nil")
	}

	if err := db.Use(otelgorm.NewPlugin(
		otelgorm.WithTracerProvider(otel.GetTracerProvider()),
		otelgorm.WithoutMetrics(),
	)); err != nil {
		return nil, fmt.Errorf("register gorm tracing plugin: %w", err)
	}

	if err := db.Use(newTransactionPlugin(otel.GetTracerProvider())); err != nil {
		return nil, fmt.Errorf("register gorm transaction plugin: %w", err)
	}
	if err := db.Use(newOperationMetricsPlugin(otel.GetMeterProvider())); err != nil {
		return nil, fmt.Errorf("register gorm operation metrics plugin: %w", err)
	}
	if err := db.Use(newDBSpanEnrichmentPlugin()); err != nil {
		return nil, fmt.Errorf("register gorm db span enrichment plugin: %w", err)
	}

	return db, nil
}

type callbackRegistrar interface {
	Register(name string, fn func(*gorm.DB)) error
}

type transactionPlugin struct {
	tracer trace.Tracer
}

func newTransactionPlugin(provider trace.TracerProvider) gorm.Plugin {
	return &transactionPlugin{
		tracer: provider.Tracer("github.com/jainam-panchal/go-observability/database"),
	}
}

func (p *transactionPlugin) Name() string {
	return "go-observability-transaction"
}

func (p *transactionPlugin) Initialize(db *gorm.DB) error {
	callbacks := []struct {
		before func(name string) callbackRegistrar
		after  func(name string) callbackRegistrar
		name   string
		otel   string
	}{
		{before: func(name string) callbackRegistrar { return db.Callback().Create().Before(name) }, after: func(name string) callbackRegistrar { return db.Callback().Create().After(name) }, name: "create", otel: "create"},
		{before: func(name string) callbackRegistrar { return db.Callback().Query().Before(name) }, after: func(name string) callbackRegistrar { return db.Callback().Query().After(name) }, name: "query", otel: "select"},
		{before: func(name string) callbackRegistrar { return db.Callback().Delete().Before(name) }, after: func(name string) callbackRegistrar { return db.Callback().Delete().After(name) }, name: "delete", otel: "delete"},
		{before: func(name string) callbackRegistrar { return db.Callback().Update().Before(name) }, after: func(name string) callbackRegistrar { return db.Callback().Update().After(name) }, name: "update", otel: "update"},
		{before: func(name string) callbackRegistrar { return db.Callback().Row().Before(name) }, after: func(name string) callbackRegistrar { return db.Callback().Row().After(name) }, name: "row", otel: "row"},
		{before: func(name string) callbackRegistrar { return db.Callback().Raw().Before(name) }, after: func(name string) callbackRegistrar { return db.Callback().Raw().After(name) }, name: "raw", otel: "raw"},
	}

	for _, callback := range callbacks {
		beforeName := "go-observability:before-transaction:" + callback.name
		afterName := "go-observability:after-transaction:" + callback.name

		if err := callback.before("otel:before:"+callback.otel).Register(beforeName, p.before); err != nil {
			return fmt.Errorf("register %s: %w", beforeName, err)
		}
		if err := callback.after("otel:after:"+callback.otel).Register(afterName, p.after); err != nil {
			return fmt.Errorf("register %s: %w", afterName, err)
		}
	}

	return nil
}

func (p *transactionPlugin) before(tx *gorm.DB) {
	if tx == nil || tx.Statement == nil || tx.Statement.Context == nil {
		return
	}
	if !isTransaction(tx) {
		return
	}
	if _, exists := tx.InstanceGet(transactionSpanKey); exists {
		return
	}

	parentCtx := tx.Statement.Context
	ctx, span := p.tracer.Start(parentCtx, transactionSpanName, trace.WithSpanKind(trace.SpanKindClient))
	span.SetAttributes(dbContextAttributes(parentCtx, "transaction", dbSystemName(tx))...)
	tx.Statement.Context = ctx
	tx.InstanceSet(transactionSpanKey, span)
}

func (p *transactionPlugin) after(tx *gorm.DB) {
	if tx == nil {
		return
	}

	value, exists := tx.InstanceGet(transactionSpanKey)
	if !exists {
		return
	}

	span, ok := value.(trace.Span)
	if !ok {
		return
	}

	if tx.Error != nil {
		span.RecordError(tx.Error)
		span.SetStatus(codes.Error, tx.Error.Error())
	}
	span.End()
	tx.InstanceSet(transactionSpanKey, nil)
}

func isTransaction(tx *gorm.DB) bool {
	type txCommitter interface {
		Commit() error
		Rollback() error
	}

	if tx == nil || tx.Statement == nil || tx.Statement.ConnPool == nil {
		return false
	}

	_, ok := tx.Statement.ConnPool.(txCommitter)
	return ok
}

type operationMetricsPlugin struct {
	queryCount    metric.Int64Counter
	queryDuration metric.Float64Histogram
}

func newOperationMetricsPlugin(provider metric.MeterProvider) gorm.Plugin {
	meter := provider.Meter("github.com/jainam-panchal/go-observability/database")

	queryCount, _ := meter.Int64Counter(
		"db.query.count",
		metric.WithDescription("Total number of traced database operations."),
	)
	queryDuration, _ := meter.Float64Histogram(
		"db.query.duration",
		metric.WithDescription("Duration of traced database operations in seconds."),
		metric.WithUnit("s"),
	)

	return &operationMetricsPlugin{
		queryCount:    queryCount,
		queryDuration: queryDuration,
	}
}

func (p *operationMetricsPlugin) Name() string {
	return "go-observability-operation-metrics"
}

func (p *operationMetricsPlugin) Initialize(db *gorm.DB) error {
	callbacks := []struct {
		afterOtel func(name string) callbackRegistrar
		beforeEnd func(name string) callbackRegistrar
		name      string
		otel      string
	}{
		{afterOtel: func(name string) callbackRegistrar { return db.Callback().Create().After(name) }, beforeEnd: func(name string) callbackRegistrar { return db.Callback().Create().Before(name) }, name: "create", otel: "create"},
		{afterOtel: func(name string) callbackRegistrar { return db.Callback().Query().After(name) }, beforeEnd: func(name string) callbackRegistrar { return db.Callback().Query().Before(name) }, name: "query", otel: "select"},
		{afterOtel: func(name string) callbackRegistrar { return db.Callback().Delete().After(name) }, beforeEnd: func(name string) callbackRegistrar { return db.Callback().Delete().Before(name) }, name: "delete", otel: "delete"},
		{afterOtel: func(name string) callbackRegistrar { return db.Callback().Update().After(name) }, beforeEnd: func(name string) callbackRegistrar { return db.Callback().Update().Before(name) }, name: "update", otel: "update"},
		{afterOtel: func(name string) callbackRegistrar { return db.Callback().Row().After(name) }, beforeEnd: func(name string) callbackRegistrar { return db.Callback().Row().Before(name) }, name: "row", otel: "row"},
		{afterOtel: func(name string) callbackRegistrar { return db.Callback().Raw().After(name) }, beforeEnd: func(name string) callbackRegistrar { return db.Callback().Raw().Before(name) }, name: "raw", otel: "raw"},
	}

	for _, callback := range callbacks {
		afterName := "go-observability:after-otel-start:" + callback.name
		beforeEndName := "go-observability:before-otel-end:" + callback.name

		if err := callback.afterOtel("otel:before:"+callback.otel).Register(afterName, p.before(callback.name)); err != nil {
			return fmt.Errorf("register %s: %w", afterName, err)
		}
		if err := callback.beforeEnd("otel:after:"+callback.otel).Register(beforeEndName, p.after(callback.name)); err != nil {
			return fmt.Errorf("register %s: %w", beforeEndName, err)
		}
	}

	return nil
}

func (p *operationMetricsPlugin) before(operation string) func(*gorm.DB) {
	return func(tx *gorm.DB) {
		if tx == nil || tx.Statement == nil || tx.Statement.Context == nil {
			return
		}

		annotateDBSpan(tx.Statement.Context, operation, dbSystemName(tx))
		tx.InstanceSet(metricsStartKeyBase+operation, time.Now())
	}
}

func (p *operationMetricsPlugin) after(operation string) func(*gorm.DB) {
	return func(tx *gorm.DB) {
		if tx == nil || tx.Statement == nil || tx.Statement.Context == nil {
			return
		}

		value, exists := tx.InstanceGet(metricsStartKeyBase + operation)
		if !exists {
			return
		}

		start, ok := value.(time.Time)
		if !ok || start.IsZero() {
			return
		}

		attrs := dbContextAttributes(tx.Statement.Context, operation, dbSystemName(tx))
		p.queryCount.Add(tx.Statement.Context, 1, metric.WithAttributes(attrs...))
		p.queryDuration.Record(tx.Statement.Context, time.Since(start).Seconds(), metric.WithAttributes(attrs...))
		tx.InstanceSet(metricsStartKeyBase+operation, nil)
	}
}

func annotateDBSpan(ctx context.Context, operation, system string) {
	span := trace.SpanFromContext(ctx)
	if !span.SpanContext().IsValid() {
		return
	}

	span.SetAttributes(dbContextAttributes(ctx, operation, system)...)
}

func dbSystemName(tx *gorm.DB) string {
	if tx == nil || tx.Dialector == nil {
		return ""
	}

	name := tx.Dialector.Name()
	switch strings.ToLower(name) {
	case "postgres", "postgresql", "pgx":
		return "postgresql"
	case "mysql":
		return "mysql"
	case "sqlite", "sqlite3":
		return "sqlite"
	default:
		return name
	}
}

type dbSpanEnrichmentPlugin struct{}

func newDBSpanEnrichmentPlugin() gorm.Plugin {
	return &dbSpanEnrichmentPlugin{}
}

func (p *dbSpanEnrichmentPlugin) Name() string {
	return "go-observability-db-span-enrichment"
}

func (p *dbSpanEnrichmentPlugin) Initialize(db *gorm.DB) error {
	callbacks := []struct {
		afterCore func(name string) callbackRegistrar
		beforeEnd func(name string) callbackRegistrar
		name      string
		otel      string
	}{
		{afterCore: func(name string) callbackRegistrar { return db.Callback().Create().After(name) }, beforeEnd: func(name string) callbackRegistrar { return db.Callback().Create().Before(name) }, name: "create", otel: "create"},
		{afterCore: func(name string) callbackRegistrar { return db.Callback().Query().After(name) }, beforeEnd: func(name string) callbackRegistrar { return db.Callback().Query().Before(name) }, name: "query", otel: "select"},
		{afterCore: func(name string) callbackRegistrar { return db.Callback().Delete().After(name) }, beforeEnd: func(name string) callbackRegistrar { return db.Callback().Delete().Before(name) }, name: "delete", otel: "delete"},
		{afterCore: func(name string) callbackRegistrar { return db.Callback().Update().After(name) }, beforeEnd: func(name string) callbackRegistrar { return db.Callback().Update().Before(name) }, name: "update", otel: "update"},
		{afterCore: func(name string) callbackRegistrar { return db.Callback().Row().After(name) }, beforeEnd: func(name string) callbackRegistrar { return db.Callback().Row().Before(name) }, name: "row", otel: "row"},
		{afterCore: func(name string) callbackRegistrar { return db.Callback().Raw().After(name) }, beforeEnd: func(name string) callbackRegistrar { return db.Callback().Raw().Before(name) }, name: "raw", otel: "raw"},
	}

	for _, cb := range callbacks {
		captureName := "go-observability:db-enrich-capture:" + cb.name
		name := "go-observability:db-enrich-before-otel-end:" + cb.name
		if err := cb.afterCore("gorm:"+cb.name).Register(captureName, captureDBStatementMetadata); err != nil {
			return fmt.Errorf("register %s: %w", captureName, err)
		}
		if err := cb.beforeEnd("otel:after:"+cb.otel).Register(name, enrichCurrentDBSpan); err != nil {
			return fmt.Errorf("register %s: %w", name, err)
		}
	}

	return nil
}

func captureDBStatementMetadata(tx *gorm.DB) {
	if tx == nil || tx.Statement == nil {
		return
	}

	sqlText := strings.TrimSpace(tx.Statement.SQL.String())
	if sqlText != "" {
		tx.InstanceSet(dbTemplateKey, normalizeSQLTemplate(sqlText))
	}
	if tx.Statement.Table != "" {
		tx.InstanceSet(dbTableKey, tx.Statement.Table)
	}
}

func enrichCurrentDBSpan(tx *gorm.DB) {
	if tx == nil || tx.Statement == nil || tx.Statement.Context == nil {
		return
	}

	span := trace.SpanFromContext(tx.Statement.Context)
	if !span.SpanContext().IsValid() {
		return
	}

	templateValue, templateExists := tx.InstanceGet(dbTemplateKey)
	tableValue, tableExists := tx.InstanceGet(dbTableKey)

	template, _ := templateValue.(string)
	if !templateExists || template == "" {
		sqlText := strings.TrimSpace(tx.Statement.SQL.String())
		if sqlText != "" {
			template = normalizeSQLTemplate(sqlText)
		}
	}

	if template != "" {
		span.SetAttributes(
			attribute.String("db.query.template", template),
			attribute.String("db.query.fingerprint", fingerprint(template)),
		)
	}

	table, _ := tableValue.(string)
	if !tableExists || table == "" {
		table = tx.Statement.Table
	}
	if table != "" {
		span.SetAttributes(attribute.String("db.table", table))
	}
	if tx.RowsAffected >= 0 {
		span.SetAttributes(attribute.Int64("db.rows_affected", tx.RowsAffected))
	}

	tx.InstanceSet(dbTemplateKey, nil)
	tx.InstanceSet(dbTableKey, nil)
}

func normalizeSQLTemplate(query string) string {
	out := strings.ToLower(query)
	out = stringLiteralRE.ReplaceAllString(out, "?")
	out = numberLiteralRE.ReplaceAllString(out, "?")
	out = spaceRE.ReplaceAllString(strings.TrimSpace(out), " ")
	if len(out) > 240 {
		return out[:240]
	}

	return out
}

func fingerprint(template string) string {
	sum := sha1.Sum([]byte(template))
	return hex.EncodeToString(sum[:8])
}
