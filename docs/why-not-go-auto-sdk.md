# Why Not Go Auto SDK

## Decision

`go-observability` does not use Go auto-instrumentation as the primary instrumentation model.

The package uses explicit library instrumentation instead:

- `telemetry.Init(...)`
- Gin middleware
- trace-aware Zap logging
- GORM instrumentation
- raw `database/sql` instrumentation
- outbound HTTP client instrumentation
- worker/job instrumentation

## Why

### 1. We need application-aware behavior, not just generic spans

This package is not only trying to emit traces.

It also needs to provide:

- normalized Gin route handling
- trace-aware Zap logging
- worker/job span lifecycle
- worker metrics
- GORM transaction visibility
- shared naming and attribute conventions

Go auto-instrumentation would not replace those application-specific requirements.

### 2. Explicit instrumentation is the production-standard path in Go

For Go services, explicit OTel library instrumentation is still the most predictable and supportable model.

It gives direct control over:

- span names
- metric names
- attributes
- sampling behavior
- middleware ordering
- what is intentionally instrumented and what is not

That matters for a shared package that multiple services will depend on.

### 3. The package exists to define a reusable contract

The purpose of `go-observability` is not only to emit telemetry. It is also to define a stable integration contract for Go services.

That contract includes:

- startup behavior
- env var conventions
- logging expectations
- HTTP middleware behavior
- DB integration patterns
- worker instrumentation patterns
- example wiring

Auto-instrumentation does not remove the need for that contract.

### 4. We need deterministic service-level semantics

This package must keep service behavior stable across projects.

Examples:

- HTTP spans should use normalized route templates
- logs should include `trace_id` and `span_id`
- worker spans should use job-oriented naming
- metrics should stay low-cardinality

Those are deliberate semantic choices. They are easier to maintain with explicit instrumentation than with a mixed auto/manual approach.

### 5. Mixed instrumentation would increase support cost

If the package combined auto-instrumentation with manual instrumentation, it would raise the risk of:

- duplicate spans
- inconsistent span naming
- overlapping metrics
- unclear ownership of instrumentation behavior
- harder debugging when behavior differs across services

For a shared package, that is the wrong default.

## What This Means

The current design intentionally favors:

- small explicit integration points
- strong context propagation
- package-owned observability semantics
- clear testing and smoke verification

This is not a rejection of all future auto-instrumentation use.

It means:

- auto-instrumentation is not the default model for this package
- any future use of it should be evaluated explicitly
- it should only be adopted where it does not conflict with the package’s manual instrumentation contract

## Bottom Line

We did not choose the Go Auto SDK because it does not solve the most important requirements of this package:

- trace-aware logging
- Gin-specific semantics
- GORM integration
- worker/job instrumentation
- stable reusable conventions for all Go services

Explicit instrumentation gives more control, clearer ownership, and a more maintainable shared package.
