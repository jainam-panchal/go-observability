# Go Gin Integration Guide

## 1. Purpose

This guide explains how to integrate `go-observability` into a generic containerized Go Gin application.

It is intentionally application-agnostic. It does not assume any specific project structure, DI framework, or deployment repository. The integration contract is designed to work for most Go Gin services that use:

- Gin for HTTP routing
- Zap or another structured logger
- GORM or raw SQL for database access
- outbound HTTP clients
- optional background jobs or workers

## 2. What the Application Must Provide

The application is expected to:

- initialize observability during startup
- defer telemetry shutdown on process exit
- register Gin middleware early in the HTTP stack
- write structured JSON logs to `stdout`
- propagate `context.Context` through DB and HTTP operations
- set service and environment metadata through environment variables

If the application uses workers, it should also:

- create a root span for each job execution when no upstream span exists
- include stable job identifiers in logs and metrics

## 3. Required Environment Variables

At minimum, the application should define:

- `OTEL_SERVICE_NAME`
- `OTEL_SERVICE_VERSION`
- `DEPLOYMENT_ENVIRONMENT`
- `OTEL_EXPORTER_OTLP_ENDPOINT`
- `OTEL_TRACE_SAMPLING_RATE`
- `OTEL_TRACES_ENABLED`
- `OTEL_METRICS_ENABLED`
- `LOG_LEVEL`

Typical containerized value for collector endpoint:

```text
OTEL_EXPORTER_OTLP_ENDPOINT=otel-collector:4317
```

If the collector runs separately on the same Docker host and the app uses bridge networking, the endpoint may instead be:

```text
OTEL_EXPORTER_OTLP_ENDPOINT=host.docker.internal:4317
```

## 4. Startup Integration

The application should initialize observability as early as possible in process startup.

Typical sequence:

1. load configuration
2. initialize logger
3. initialize observability
4. defer telemetry shutdown
5. build application dependencies
6. start HTTP server or worker loop

The observability module should expose a startup API that:

- loads or accepts config
- initializes providers
- registers resource attributes
- returns a shutdown function

Current bootstrap API:

- `telemetry.DefaultConfig()`
- `telemetry.LoadConfigFromEnv()`
- `telemetry.Init(cfg)`
- `telemetry.MustInit(cfg)`

## 5. Gin Middleware Integration

Register observability middleware near the top of the Gin middleware stack.

Expected behavior:

- start one inbound request span per HTTP request
- capture route template rather than raw path
- record status code
- record request duration
- track active requests
- preserve request context for handlers and downstream calls

Middleware ordering should preserve:

- tracing early
- authentication and business middleware after trace context exists
- recovery middleware still active for panic handling

Current middleware entry point:

- `middleware.RegisterGinMiddlewares(router)`

Current metric names emitted by the Gin middleware:

- `http.server.request.count`
- `http.server.request.duration`
- `http.server.active_requests`

## 6. Logging Integration

The module should support structured logging for Gin applications without forcing a full logger rewrite.

Preferred behavior:

- logs are JSON formatted
- logs go to `stdout` in containers
- trace and span IDs are added when the caller passes a traced context
- service metadata is included consistently

For applications that already use Zap:

- keep Zap
- add a context-aware logging path
- support request and job log enrichment from `context.Context`

Applications that use global loggers only should gradually adopt context-aware logging in request and worker paths where trace correlation matters most.

## 7. GORM Integration

GORM is a required first-class integration path.

Applications using GORM should be able to instrument an existing `*gorm.DB` instance without restructuring the rest of the application.

Expected integration shape:

1. build `*gorm.DB`
2. register the observability GORM adapter or plugin
3. pass request or job context using `WithContext(ctx)`
4. preserve tracing through transactions

Requirements:

- normal queries must be traced
- transactions using `WithContext(ctx).Transaction(...)` must be traced
- failing queries should appear as errored spans
- API and worker traces should include GORM child spans where DB access occurs

If the application does not use GORM, raw SQL helpers may be used instead, but GORM support is the required baseline.

## 8. Outbound HTTP Integration

All outbound HTTP clients used for service-to-service calls should use an instrumented transport or helper from the module.

Expected behavior:

- propagate trace context downstream
- create child spans for outbound requests
- record request method, host, status, and latency

Applications should avoid creating ad hoc uninstrumented HTTP clients once the module is integrated.

## 9. Worker Integration

Applications with background jobs, queue consumers, or scheduled tasks should instrument job execution explicitly.

Expected behavior:

- create a root job span if no incoming trace context exists
- log stable job metadata such as `job_name`, `queue`, and job execution identifiers
- emit worker metrics for started, completed, failed, and duration
- propagate context into DB and outbound HTTP calls inside the job

## 10. Container Logging Expectations

For containerized applications, logs should be emitted to `stdout`.

Why this matters:

- Docker captures container logs from `stdout`
- the local collector can ingest those logs from the Docker host
- platform-level log collection becomes independent of application-local file rotation

Containerized applications may still write to files if they have a local operational need, but `stdout` must remain the primary sink for observability.

## 11. Validation Checklist

An integration should not be considered complete until the following are verified:

- one API request produces an inbound trace
- the same API request produces logs containing `trace_id`
- request metrics appear in Prometheus and Grafana
- one DB-backed request shows GORM child spans if the service uses GORM
- one outbound HTTP call shows a child span if the service makes HTTP requests
- one worker job produces a root job span and worker metrics if the service has workers
- logs are queryable in Loki by service and environment
- trace-to-log correlation works in Grafana

## 12. Required Smoke Tests

Use dedicated smoke checks for each integration area, and treat them as release gates.

Required smoke scope:

- bootstrap lifecycle and env contract
- Gin middleware and request telemetry
- Zap-compatible contextual logging and trace field injection
- outbound HTTP child spans
- GORM query and transaction tracing
- worker root spans and worker metrics

Required Go quality gate before marking implementation as verified:

```text
gofmt
go vet ./...
go test ./...
go test -race ./...
golangci-lint run
```

A feature should not be marked complete until relevant smoke checks pass and results are recorded in the implementation checklist.

## 13. Common Mistakes to Avoid

- using raw request paths instead of normalized route templates in metrics
- emitting high-cardinality labels such as user IDs or request IDs
- keeping logs only in rotating files inside the container
- creating DB operations without `context.Context`
- adding middleware too late in the Gin stack
- assuming `localhost:4317` works in all container networking modes
