package database

import (
	"errors"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
	otelgorm "gorm.io/plugin/opentelemetry/tracing"
)

const (
	transactionSpanName = "gorm.Transaction"
	transactionSpanKey  = "go-observability:gorm-transaction-span"
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
	}{
		{before: func(name string) callbackRegistrar { return db.Callback().Create().Before(name) }, after: func(name string) callbackRegistrar { return db.Callback().Create().After(name) }, name: "create"},
		{before: func(name string) callbackRegistrar { return db.Callback().Query().Before(name) }, after: func(name string) callbackRegistrar { return db.Callback().Query().After(name) }, name: "query"},
		{before: func(name string) callbackRegistrar { return db.Callback().Delete().Before(name) }, after: func(name string) callbackRegistrar { return db.Callback().Delete().After(name) }, name: "delete"},
		{before: func(name string) callbackRegistrar { return db.Callback().Update().Before(name) }, after: func(name string) callbackRegistrar { return db.Callback().Update().After(name) }, name: "update"},
		{before: func(name string) callbackRegistrar { return db.Callback().Row().Before(name) }, after: func(name string) callbackRegistrar { return db.Callback().Row().After(name) }, name: "row"},
		{before: func(name string) callbackRegistrar { return db.Callback().Raw().Before(name) }, after: func(name string) callbackRegistrar { return db.Callback().Raw().After(name) }, name: "raw"},
	}

	for _, callback := range callbacks {
		beforeName := "go-observability:before-transaction:" + callback.name
		afterName := "go-observability:after-transaction:" + callback.name

		if err := callback.before("otel:before:"+callback.name).Register(beforeName, p.before); err != nil {
			return fmt.Errorf("register %s: %w", beforeName, err)
		}
		if err := callback.after("otel:after:"+callback.name).Register(afterName, p.after); err != nil {
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
