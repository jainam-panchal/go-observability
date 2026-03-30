# go-observability

## Repository Purpose

`go-observability` is the shared Go module for service-level observability.

It provides a consistent integration layer for Go API and worker services so teams can adopt logs, metrics, and traces with minimal service-specific code changes.

Planned repository:

- `github.com/jainam-panchal/go-observability`

Planned Go module:

- `github.com/jainam-panchal/go-observability`

## Responsibilities

This repository will own:

- OpenTelemetry bootstrap and shutdown
- environment-driven configuration
- resource attribute standardization
- trace-aware structured logging
- HTTP middleware for Gin services
- GORM instrumentation adapter
- secondary PostgreSQL or raw SQL instrumentation helpers where needed
- outbound HTTP instrumentation helpers
- worker/job instrumentation helpers
- package-level documentation and examples

## Non-Responsibilities

This repository will not own:

- collector deployment configuration
- Grafana dashboards
- Prometheus alert rules
- Loki, Tempo, or Prometheus infrastructure setup
- Docker Compose for the observability platform

## Design Principles

- keep public APIs small and explicit
- keep framework adapters separate from core bootstrap logic
- use OpenTelemetry as the canonical internal model
- preserve context propagation across all blocking operations
- default to structured JSON logging
- keep metric labels low-cardinality
- support GORM as a first-class database integration path for Go services

## Expected Package Areas

- `telemetry/`
- `logger/`
- `middleware/`
- `database/`
- `httpclient/`
- `worker/`
- `examples/`
- `docs/`

## Required Integration Paths

The first supported application shape is a generic Go Gin service.

The repository must support:

- Gin HTTP request tracing and request metrics
- Zap-compatible logging integration with trace and span correlation
- GORM instrumentation for query and transaction tracing
- outbound HTTP client instrumentation
- worker or background job instrumentation

GORM is a required integration path, not an optional add-on. Services that use GORM should be able to instrument an existing `*gorm.DB` without restructuring the rest of their application architecture.

## Required Documentation

This repository must include documentation for:

- generic Go Gin application integration
- required environment variables
- logger integration expectations
- GORM integration
- outbound HTTP instrumentation
- worker integration

The generic application integration guide should live at:

- `docs/gin-integration-guide.md`
- `docs/testing-matrix.md`
- `docs/why-not-go-auto-sdk.md`

## Expected Consumers

- Go API services using Gin
- Go worker services using custom job runners
- future Go services that need a standard observability baseline

## Success Criteria

- service teams initialize observability with minimal startup code
- logs include trace correlation fields
- traces cover inbound, GORM-backed DB access, outbound HTTP, and worker execution
- metrics expose useful golden signals without excessive cardinality
- package behavior is testable and documented

## Implemented API Surface

Current exported integration points:

- `telemetry.DefaultConfig()`
- `telemetry.LoadConfigFromEnv()`
- `telemetry.Init(...)`
- `telemetry.MustInit(...)`
- `logger.New(...)`
- `logger.MustNew(...)`
- `logger.WithContext(...)`
- `logger.L(ctx)`
- `middleware.RegisterGinMiddlewares(...)`
- `httpclient.NewTransport(...)`
- `httpclient.NewClient(...)`
- `database.InstrumentGORM(...)`
- `database.OpenInstrumentedSQL(...)`
- `worker.StartJob(...)`

Current Gin middleware metric names:

- `http.server.request.count`
- `http.server.request.duration`
- `http.server.active_requests`

Current GORM adapter behavior:

- instruments an existing `*gorm.DB`
- uses `gorm.io/plugin/opentelemetry/tracing` for operation spans
- adds a transaction wrapper span for `db.WithContext(ctx).Transaction(...)` flows
- operation span names follow SQL summaries such as `select table_name` and `insert table_name`

Current secondary raw SQL helper behavior:

- opens an instrumented `*sql.DB`
- preserves context propagation for `QueryContext`, `ExecContext`, and `BeginTx`
- registers DB stats metrics using the global meter provider
- attaches a low-cardinality `db.system` attribute derived from the driver name

Current worker helper behavior:

- starts spans named `job <job_name>`
- creates a root span when no incoming trace exists
- creates a child span when job execution already has a parent context
- emits:
  - `worker.job.started`
  - `worker.job.completed`
  - `worker.job.duration`

Reference examples:

- `examples/api/main.go`
- `examples/worker/main.go`
