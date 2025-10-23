package telemetry

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// RecordCounter incrementa un contador
func (c *Client) RecordCounter(ctx context.Context, name string, value int64, attrs ...attribute.KeyValue) {
	counter, err := c.GetOrCreateCounter(name, "")
	if err != nil {
		c.Error(ctx, "failed to get counter", err, attribute.String("counter_name", name))
		return
	}
	
	counter.Add(ctx, value, metric.WithAttributes(attrs...))
}

// RecordHistogram registra un valor en un histograma
func (c *Client) RecordHistogram(ctx context.Context, name string, value float64, attrs ...attribute.KeyValue) {
	histogram, err := c.GetOrCreateHistogram(name, "")
	if err != nil {
		c.Error(ctx, "failed to get histogram", err, attribute.String("histogram_name", name))
		return
	}
	
	histogram.Record(ctx, value, metric.WithAttributes(attrs...))
}

// RecordLatency es un helper para registrar latencias en milisegundos
func (c *Client) RecordLatency(ctx context.Context, operation string, latencyMs float64, attrs ...attribute.KeyValue) {
	metricName := operation + ".latency_ms"
	c.RecordHistogram(ctx, metricName, latencyMs, attrs...)
}

