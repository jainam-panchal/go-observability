package worker

import (
	"context"
	"fmt"
	"time"

	"github.com/jainam-panchal/go-observability/internal/jobmeta"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const instrumentationName = "github.com/jainam-panchal/go-observability/worker"

// FinishFunc closes a job span and records the terminal outcome.
type FinishFunc func(err error)

// StartJob starts a worker span for a single job execution and returns a
// context carrying that span plus a finish function for completion recording.
func StartJob(ctx context.Context, jobName string) (context.Context, FinishFunc) {
	instrumenter := newInstrumenter()
	return instrumenter.StartJob(ctx, jobName)
}

type instrumenter struct {
	tracer       trace.Tracer
	jobStarted   metric.Int64Counter
	jobCompleted metric.Int64Counter
	jobDuration  metric.Float64Histogram
}

func newInstrumenter() instrumenter {
	meter := otel.GetMeterProvider().Meter(instrumentationName)

	jobStarted, _ := meter.Int64Counter(
		"worker.job.started",
		metric.WithDescription("Total number of worker jobs started."),
	)
	jobCompleted, _ := meter.Int64Counter(
		"worker.job.completed",
		metric.WithDescription("Total number of worker jobs completed by terminal status."),
	)
	jobDuration, _ := meter.Float64Histogram(
		"worker.job.duration",
		metric.WithDescription("Duration of worker job execution in seconds."),
		metric.WithUnit("s"),
	)

	return instrumenter{
		tracer:       otel.GetTracerProvider().Tracer(instrumentationName),
		jobStarted:   jobStarted,
		jobCompleted: jobCompleted,
		jobDuration:  jobDuration,
	}
}

func (i instrumenter) StartJob(ctx context.Context, jobName string) (context.Context, FinishFunc) {
	jobAttrs := []attribute.KeyValue{
		attribute.String("job.name", jobName),
	}

	ctx, span := i.tracer.Start(
		ctx,
		fmt.Sprintf("job %s", jobName),
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(jobAttrs...),
	)
	ctx = jobmeta.WithJobMetadata(ctx, jobName)
	i.jobStarted.Add(ctx, 1, metric.WithAttributes(jobAttrs...))

	start := time.Now()
	return ctx, func(err error) {
		status := "success"
		if err != nil {
			status = "error"
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}

		completionAttrs := append([]attribute.KeyValue{}, jobAttrs...)
		completionAttrs = append(completionAttrs, attribute.String("job.status", status))
		durationSeconds := time.Since(start).Seconds()

		span.SetAttributes(attribute.String("job.status", status))
		i.jobCompleted.Add(ctx, 1, metric.WithAttributes(completionAttrs...))
		i.jobDuration.Record(ctx, durationSeconds, metric.WithAttributes(jobAttrs...))
		span.End()
	}
}
