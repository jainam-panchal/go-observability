package middleware

import (
	"context"
	"errors"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const validationInstrumentationName = "github.com/jainam-panchal/go-observability/middleware/validation"

// RunValidationStep executes a validation function inside an internal span and
// returns the updated context so callers can continue context propagation.
func RunValidationStep(ctx context.Context, step string, fn func(context.Context) error) (context.Context, error) {
	if fn == nil {
		return ctx, errors.New("validation function must not be nil")
	}

	normalizedStep := strings.TrimSpace(step)
	if normalizedStep == "" {
		normalizedStep = "step"
	}

	ctx, span := otel.GetTracerProvider().
		Tracer(validationInstrumentationName).
		Start(ctx, "app.validation."+normalizedStep, trace.WithSpanKind(trace.SpanKindInternal))
	defer span.End()

	span.SetAttributes(attribute.String("validation.step", normalizedStep))

	err := fn(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		span.SetAttributes(attribute.Bool("validation.ok", false))
		return ctx, err
	}

	span.SetAttributes(attribute.Bool("validation.ok", true))
	return ctx, nil
}
