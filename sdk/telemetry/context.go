package telemetry

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
)

// contextKey es el tipo para las claves de contexto
type contextKey string

const (
	commonAttrsKey contextKey = "telemetry_common_attrs"
	eventAttrsKey  contextKey = "telemetry_event_attrs"
	metricAttrsKey contextKey = "telemetry_metric_attrs"
)

// AppendCommonAttrs añade atributos comunes al contexto (para logs, métricas y trazas)
func AppendCommonAttrs(ctx context.Context, attrs ...attribute.KeyValue) context.Context {
	return appendAttrs(ctx, commonAttrsKey, attrs...)
}

// AppendEventAttrs añade atributos específicos para logs y spans
func AppendEventAttrs(ctx context.Context, attrs ...attribute.KeyValue) context.Context {
	return appendAttrs(ctx, eventAttrsKey, attrs...)
}

// AppendMetricAttrs añade atributos específicos para métricas
func AppendMetricAttrs(ctx context.Context, attrs ...attribute.KeyValue) context.Context {
	return appendAttrs(ctx, metricAttrsKey, attrs...)
}

// GetCommonAttrs extrae atributos comunes del contexto
func GetCommonAttrs(ctx context.Context) []attribute.KeyValue {
	return getAttrs(ctx, commonAttrsKey)
}

// GetEventAttrs extrae atributos de eventos del contexto
func GetEventAttrs(ctx context.Context) []attribute.KeyValue {
	return getAttrs(ctx, eventAttrsKey)
}

// GetMetricAttrs extrae atributos de métricas del contexto
func GetMetricAttrs(ctx context.Context) []attribute.KeyValue {
	return getAttrs(ctx, metricAttrsKey)
}

// appendAttrs es un helper interno para añadir atributos al contexto
func appendAttrs(ctx context.Context, key contextKey, attrs ...attribute.KeyValue) context.Context {
	existing := getAttrs(ctx, key)
	merged := append(existing, attrs...)
	return context.WithValue(ctx, key, merged)
}

// getAttrs es un helper interno para extraer atributos del contexto
func getAttrs(ctx context.Context, key contextKey) []attribute.KeyValue {
	val := ctx.Value(key)
	if val == nil {
		return []attribute.KeyValue{}
	}
	
	attrs, ok := val.([]attribute.KeyValue)
	if !ok {
		return []attribute.KeyValue{}
	}
	
	return attrs
}

