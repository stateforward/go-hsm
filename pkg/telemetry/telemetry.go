package telemetry

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type Provider struct {
	trace.TracerProvider
}

var (
	provider    = &Provider{}
	tracer      = &Tracer{}
	span        = &Span{}
	spanContext = trace.SpanContext{}
)

func NewProvider() *Provider {
	return provider
}

func (provider *Provider) Tracer(name string, options ...trace.TracerOption) trace.Tracer {
	return tracer
}

type Tracer struct {
	trace.Tracer
}

func (tracer *Tracer) Start(ctx context.Context, name string, options ...trace.SpanStartOption) (context.Context, trace.Span) {
	return ctx, span
}

type Span struct {
	trace.Span
}

func (span *Span) End(options ...trace.SpanEndOption)                  {}
func (span *Span) AddEvent(name string, options ...trace.EventOption)  {}
func (span *Span) AddLink(link trace.Link)                             {}
func (span *Span) IsRecording() bool                                   { return false }
func (span *Span) RecordError(err error, options ...trace.EventOption) {}
func (span *Span) SetAttributes(kv ...attribute.KeyValue)              {}
func (span *Span) SetName(name string)                                 {}
func (span *Span) SetStatus(code codes.Code, description string)       {}
func (span *Span) SpanContext() trace.SpanContext                      { return spanContext }
func (span *Span) TracerProvider() trace.TracerProvider                { return provider }
