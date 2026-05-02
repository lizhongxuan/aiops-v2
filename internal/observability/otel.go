package observability

import (
	"context"
	"fmt"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	"go.opentelemetry.io/otel/trace"
)

type Provider struct {
	enabled  bool
	tracer   trace.Tracer
	shutdown func(context.Context) error
}

func Init(ctx context.Context, cfg Config) (*Provider, error) {
	if !cfg.Enabled {
		return &Provider{
			tracer:   trace.NewNoopTracerProvider().Tracer("aiops-v2"),
			shutdown: func(context.Context) error { return nil },
		}, nil
	}
	if strings.TrimSpace(cfg.Endpoint) == "" {
		return nil, fmt.Errorf("otel endpoint is required when AIOPS_OTEL_ENABLED is true")
	}
	serviceName := strings.TrimSpace(cfg.ServiceName)
	if serviceName == "" {
		serviceName = "aiops-v2-agent"
	}
	exporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(strings.TrimSpace(cfg.Endpoint)))
	if err != nil {
		return nil, fmt.Errorf("init otlp trace exporter: %w", err)
	}
	res, err := resource.Merge(resource.Default(), resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(serviceName),
	))
	if err != nil {
		return nil, fmt.Errorf("init otel resource: %w", err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	return &Provider{
		enabled: true,
		tracer:  tp.Tracer("aiops-v2/runtime"),
		shutdown: func(ctx context.Context) error {
			return tp.Shutdown(ctx)
		},
	}, nil
}

func (p *Provider) Enabled() bool {
	return p != nil && p.enabled
}

func (p *Provider) Tracer() trace.Tracer {
	if p == nil || p.tracer == nil {
		return trace.NewNoopTracerProvider().Tracer("aiops-v2")
	}
	return p.tracer
}

func (p *Provider) Shutdown(ctx context.Context) error {
	if p == nil || p.shutdown == nil {
		return nil
	}
	return p.shutdown(ctx)
}
