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
