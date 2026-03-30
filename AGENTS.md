# AGENTS.md

## Purpose

This repository contains the reusable Go observability module published as:

- `github.com/jainam-panchal/go-observability`

It exists to provide a stable integration layer for Go Gin APIs and Go workers using:

- OpenTelemetry
- Zap-compatible structured logging
- GORM tracing
- outbound HTTP tracing
- worker/job instrumentation

## Repository Rules

- keep this repository application-agnostic
- keep public APIs small, explicit, and documented
- support GORM as a first-class integration path
- do not add Grafana, Prometheus, Loki, Tempo, or collector deployment assets here
- do not couple core bootstrap logic to one framework when adapters can stay separate

## Package Structure

Preferred package areas:

- `telemetry/`
- `logger/`
- `middleware/`
- `database/`
- `httpclient/`
- `worker/`

## Go Standards

- all blocking work must take `context.Context`
- wrap errors with `%w`
- avoid panic for normal control flow
- add table-driven tests with subtests where practical
- exported APIs must have doc comments

Required validation before marking implementation verified:

- `gofmt`
- `go vet ./...`
- `go test ./...`
- `go test -race ./...`
- `golangci-lint run`

## Checklist Rules

- update the repository checklist when a task is implemented or verified
- implementation rows must not move to `verified` until their referenced smoke-test rows pass
- smoke tests are release gates, not optional notes
- record important evolving implementation context in `AGENTS.md` whenever it will materially affect later work, validation, or integration expectations
- when a checklist verification mapping is too coarse to prove one task honestly, split the smoke row before marking the task complete

## Implementation Context

- `logger.New` is the canonical constructor for container-friendly JSON stdout logging and must always attach `service`, `service_version`, and `deployment_environment`
- `logger.WithContext` and `logger.L` are the contextual logging entry points and must inject `trace_id` and `span_id` only when the incoming span context is valid
- logger verification is tracked independently under `SMK-GO-007`; do not reuse broader Gin/http smoke rows to prove logger behavior
- `middleware.RegisterGinMiddlewares` is the canonical Gin entry point and must create one server span per request, extract remote parent context, preserve the traced request context for handlers, and record low-cardinality request metrics using route templates
- Gin middleware verification is tracked independently under `SMK-GO-008`; do not reuse outbound HTTP smoke rows to prove inbound request instrumentation
- `httpclient.NewTransport` and `httpclient.NewClient` are the canonical outbound HTTP entry points and must preserve parent context, inject `traceparent`, and create client spans without forcing callers to replace their existing `http.Client` lifecycle
- outbound HTTP verification remains under `SMK-GO-002`; keep it separate from inbound Gin middleware proof
- `database.InstrumentGORM` is the canonical GORM entry point and must instrument an existing `*gorm.DB`, preserve `WithContext(ctx)`, emit query/create spans via the upstream plugin, and add a lightweight transaction wrapper span for transactional flows
- GORM verification is tracked independently under `SMK-GO-009`; do not bundle it with future raw `database/sql` smoke rows
- `database.OpenInstrumentedSQL` is the secondary raw SQL entry point and must open an instrumented `*sql.DB`, preserve parent context through `QueryContext` and `BeginTx`, and keep the raw SQL path clearly secondary to GORM
- raw SQL verification remains under `SMK-GO-003`; do not let it redefine the primary GORM integration contract
- `worker.StartJob` is the canonical worker entry point and must create a root job span when no parent exists, create a child job span when parent context exists, and emit stable low-cardinality job metrics
- worker verification remains under `SMK-GO-004`; it must prove root-span creation, parent propagation, and worker metric emission together
- `docs/gin-integration-guide.md` is the canonical generic integration guide and must stay application-agnostic while still carrying concrete wiring snippets for startup, Gin, GORM, raw SQL, outbound HTTP, and worker flows
- `examples/api/main.go` is the canonical generic API example and must compile as a self-contained reference for startup, Gin middleware, contextual logging, GORM wiring, and outbound HTTP usage
- `examples/worker/main.go` is the canonical generic worker example and must compile as a self-contained reference for startup, job span lifecycle, contextual logging, and traced SQL work inside a job
- `docs/testing-matrix.md` is the canonical testing inventory and must stay aligned with the checklist smoke rows and the current unit test files
- `docs/why-not-go-auto-sdk.md` records the instrumentation-model decision; do not introduce Go auto-instrumentation as a default path without explicitly revisiting that design note
