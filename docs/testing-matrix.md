# Testing Matrix

## Purpose

This document tracks the package-level unit and smoke coverage for `go-observability`.

It exists to make the current verification surface explicit so future work can add tests without guessing which behavior is already covered and which smoke row should prove it.

## Core Unit Coverage

### Telemetry

Files:

- `telemetry/config_test.go`
- `telemetry/telemetry_test.go`

Covered behavior:

- config defaults
- env overrides
- invalid env fallback behavior
- environment precedence
- provider init and shutdown lifecycle
- global tracer, meter, and propagator registration
- invalid config rejection

### Logger

Files:

- `logger/logger_test.go`

Covered behavior:

- stable service metadata fields
- trace and span ID enrichment from context
- no trace fields when no active span exists

### Gin Middleware

Files:

- `middleware/middleware_test.go`

Covered behavior:

- inbound server span creation
- remote parent extraction
- traced request-context propagation
- route-template metrics
- active-request tracking

### Database

Files:

- `database/gorm_test.go`
- `database/sql_test.go`

Covered behavior:

- GORM query spans
- GORM transaction wrapper spans
- raw SQL query tracing
- raw SQL transaction tracing
- invalid DB or driver input rejection

### Worker

Files:

- `worker/worker_test.go`

Covered behavior:

- root job span creation
- child job span creation when parent context exists
- success and error completion metrics

## Example Compilation Coverage

The examples are verified by compilation-level smoke checks:

- `examples/api/main.go`
- `examples/worker/main.go`

These are not treated as replacement for unit tests. They prove that the current public API can still be wired together into realistic application entrypoints.

## Smoke Mapping

- `SMK-GO-001`: config defaults and env contract
- `SMK-GO-002`: outbound HTTP instrumentation
- `SMK-GO-003`: raw SQL instrumentation
- `SMK-GO-004`: worker instrumentation
- `SMK-GO-006`: telemetry lifecycle
- `SMK-GO-007`: logger behavior
- `SMK-GO-008`: Gin middleware behavior
- `SMK-GO-009`: GORM instrumentation
- `SMK-GO-010`: API example compilation
- `SMK-GO-011`: worker example compilation
- `SMK-GO-012`: core unit test surface presence and alignment
- `SMK-GO-005`: final strict quality gate

## Maintenance Rules

- add or update this matrix when a new core integration surface gains tests
- do not mark a testing task complete unless the matrix and checklist agree
- keep unit coverage and smoke coverage separate; smoke rows prove integrated behavior, not just individual assertions
