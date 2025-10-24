package metricbundle

import (
	"context"
	"sync"

	"github.com/xKoRx/sdk/pkg/shared/telemetry/semconv"
	"go.opentelemetry.io/otel/attribute"
)

// TemporalMetrics agrega métricas para el ecosistema Temporal (workflows y activities)
// Namespace: temporal
// Entidades: activity, workflow
type TemporalMetrics struct {
	Activity *BaseMetrics // temporal.activity.*
	Workflow *BaseMetrics // temporal.workflow.*
}

// NewTemporalMetrics crea el bundle Temporal
func NewTemporalMetrics(client MetricsClient) *TemporalMetrics {
	activity := NewBaseMetrics(client, "temporal", "activity")
	workflow := NewBaseMetrics(client, "temporal", "workflow")

	return &TemporalMetrics{
		Activity: activity,
		Workflow: workflow,
	}
}

// ----------------------------------------------------------------------------------
// Temporizadores por entidad
// ----------------------------------------------------------------------------------
func (t *TemporalMetrics) StartActivityTimer(ctx context.Context, attrs ...attribute.KeyValue) func() {
	return t.Activity.StartDurationTimer(ctx, attrs...)
}

func (t *TemporalMetrics) StartWorkflowTimer(ctx context.Context, attrs ...attribute.KeyValue) func() {
	return t.Workflow.StartDurationTimer(ctx, attrs...)
}

// ----------------------------------------------------------------------------------
// Helpers de atributos por entidad
// ----------------------------------------------------------------------------------

// AddDefaultTemporalActivityAttributes añade atributos comunes para activities de Temporal
func AddDefaultTemporalActivityAttributes(name string) []attribute.KeyValue {
	return []attribute.KeyValue{
		semconv.Metrics.Component.String("temporal-activity"),
		attribute.String("temporal.activity", name),
	}
}

// AddDefaultTemporalWorkflowAttributes añade atributos comunes para workflows de Temporal
func AddDefaultTemporalWorkflowAttributes(name string) []attribute.KeyValue {
	return []attribute.KeyValue{
		semconv.Metrics.Component.String("temporal-workflow"),
		attribute.String("temporal.workflow", name),
	}
}

// ----------------------------------------------------------------------------------
// Métodos específicos por entidad (alineados a BaseMetrics.RecordResult)
// ----------------------------------------------------------------------------------

// RecordActivityResult registra el resultado de la ejecución de una activity
func (t *TemporalMetrics) RecordActivityResult(
	ctx context.Context,
	activityName string,
	success bool,
	additionalAttrs ...attribute.KeyValue,
) {
	attrs := AddDefaultTemporalActivityAttributes(activityName)
	attrs = append(attrs, additionalAttrs...)

	status := "success"
	if !success {
		status = "error"
	}
	attrs = append(attrs, semconv.Metrics.Status.String(status))

	t.Activity.RecordResult(ctx, attrs...)
}

// RecordWorkflowResult registra el resultado de la ejecución de un workflow
func (t *TemporalMetrics) RecordWorkflowResult(
	ctx context.Context,
	workflowName string,
	success bool,
	additionalAttrs ...attribute.KeyValue,
) {
	attrs := AddDefaultTemporalWorkflowAttributes(workflowName)
	attrs = append(attrs, additionalAttrs...)

	status := "success"
	if !success {
		status = "error"
	}
	attrs = append(attrs, semconv.Metrics.Status.String(status))

	t.Workflow.RecordResult(ctx, attrs...)
}

// RecordActivityRun incrementa el contador de ejecuciones de una activity
func (t *TemporalMetrics) RecordActivityRun(
	ctx context.Context,
	activityName string,
	additionalAttrs ...attribute.KeyValue,
) {
	attrs := AddDefaultTemporalActivityAttributes(activityName)
	attrs = append(attrs, additionalAttrs...)
	name := MetricName("temporal", "activity", "runs")
	// Usar el cliente para que incluya Common+Metric attrs del contexto
	t.Activity.client.RecordCounter(ctx, name, 1, attrs...)
}

// ----------------------------------------------------------------------------------
// Singleton global del bundle Temporal
// ----------------------------------------------------------------------------------
var (
	globalTemporalMetrics   *TemporalMetrics
	onceInitTemporalMetrics sync.Once
)

// InitGlobalTemporalBundle inicializa el bundle global
func InitGlobalTemporalBundle(client MetricsClient) {
	onceInitTemporalMetrics.Do(func() {
		globalTemporalMetrics = NewTemporalMetrics(client)
	})
}

// GetGlobalTemporalMetrics retorna el bundle global
func GetGlobalTemporalMetrics() *TemporalMetrics {
	return globalTemporalMetrics // nil si no inicializado (no-op seguro)
}
