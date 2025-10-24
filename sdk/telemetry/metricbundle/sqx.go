package metricbundle

import (
	"context"
	"sync"
	"time"

	"github.com/xKoRx/sdk/pkg/shared/telemetry/semconv"
	"go.opentelemetry.io/otel/attribute"
)

// SQXMetrics agrega métricas de alto nivel para el dominio SQX
// Namespace: symphony.sqx
// Entidades: watcher, workflow, activity
type SQXMetrics struct {
	Watcher  *BaseMetrics // symphony.sqx.watcher.*
	Workflow *BaseMetrics // symphony.sqx.workflow.*
	Activity *BaseMetrics // symphony.sqx.activity.*
}

// NewSQXMetrics crea el bundle SQX
func NewSQXMetrics(client MetricsClient) *SQXMetrics {
	watcher := NewBaseMetrics(client, "symphony.sqx", "watcher")
	workflow := NewBaseMetrics(client, "symphony.sqx", "workflow")
	activity := NewBaseMetrics(client, "symphony.sqx", "activity")

	return &SQXMetrics{
		Watcher:  watcher,
		Workflow: workflow,
		Activity: activity,
	}
}

// ----------------------------------------------------------------------------------
// Timers por entidad
// ----------------------------------------------------------------------------------
func (s *SQXMetrics) StartWatcherTimer(ctx context.Context, attrs ...attribute.KeyValue) func() {
	return s.Watcher.StartDurationTimer(ctx, attrs...)
}

func (s *SQXMetrics) StartWorkflowTimer(ctx context.Context, attrs ...attribute.KeyValue) func() {
	return s.Workflow.StartDurationTimer(ctx, attrs...)
}

func (s *SQXMetrics) StartActivityTimer(ctx context.Context, attrs ...attribute.KeyValue) func() {
	return s.Activity.StartDurationTimer(ctx, attrs...)
}

// ----------------------------------------------------------------------------------
// Helpers de atributos por entidad
// ----------------------------------------------------------------------------------

// AddDefaultSQXWatcherAttributes añade atributos comunes para watcher de SQX
func AddDefaultSQXWatcherAttributes(name string) []attribute.KeyValue {
	return []attribute.KeyValue{
		semconv.Metrics.Component.String("sqx-watcher"),
		attribute.String("watcher", name),
	}
}

// AddDefaultSQXWorkflowAttributes añade atributos comunes para workflow de SQX
func AddDefaultSQXWorkflowAttributes(name string) []attribute.KeyValue {
	return []attribute.KeyValue{
		semconv.Metrics.Component.String("sqx-workflow"),
		attribute.String("workflow", name),
	}
}

// AddDefaultSQXActivityAttributes añade atributos comunes para activity de SQX
func AddDefaultSQXActivityAttributes(name string) []attribute.KeyValue {
	return []attribute.KeyValue{
		semconv.Metrics.Component.String("sqx-activity"),
		attribute.String("activity", name),
	}
}

// ----------------------------------------------------------------------------------
// Métodos específicos por entidad (alineados a BaseMetrics.RecordResult)
// ----------------------------------------------------------------------------------

// RecordWatcherEvent registra el resultado de un evento del watcher
func (s *SQXMetrics) RecordWatcherEvent(
	ctx context.Context,
	watcherName string,
	success bool,
	additionalAttrs ...attribute.KeyValue,
) {
	attrs := AddDefaultSQXWatcherAttributes(watcherName)
	attrs = append(attrs, additionalAttrs...)

	status := "success"
	if !success {
		status = "error"
	}
	attrs = append(attrs, semconv.Metrics.Status.String(status))

	s.Watcher.RecordResult(ctx, attrs...)
}

// RecordWorkflowRun registra el resultado de la ejecución de un workflow
func (s *SQXMetrics) RecordWorkflowRun(
	ctx context.Context,
	workflowName string,
	success bool,
	additionalAttrs ...attribute.KeyValue,
) {
	attrs := AddDefaultSQXWorkflowAttributes(workflowName)
	attrs = append(attrs, additionalAttrs...)

	status := "success"
	if !success {
		status = "error"
	}
	attrs = append(attrs, semconv.Metrics.Status.String(status))

	s.Workflow.RecordResult(ctx, attrs...)
}

// RecordActivity registra el resultado de la ejecución de una activity
func (s *SQXMetrics) RecordActivity(
	ctx context.Context,
	activityName string,
	success bool,
	additionalAttrs ...attribute.KeyValue,
) {
	attrs := AddDefaultSQXActivityAttributes(activityName)
	attrs = append(attrs, additionalAttrs...)

	status := "success"
	if !success {
		status = "error"
	}
	attrs = append(attrs, semconv.Metrics.Status.String(status))

	s.Activity.RecordResult(ctx, attrs...)
}

// RecordActivityRun incrementa el contador de ejecuciones de una activity
func (s *SQXMetrics) RecordActivityRun(
	ctx context.Context,
	activityName string,
	additionalAttrs ...attribute.KeyValue,
) {
	attrs := AddDefaultSQXActivityAttributes(activityName)
	attrs = append(attrs, additionalAttrs...)
	name := MetricName("symphony.sqx", "activity", "runs")
	// Usar el cliente para que incluya Common+Metric attrs del contexto
	s.Activity.client.RecordCounter(ctx, name, 1, attrs...)
}

// StartActivityStepTimer mide la duración de un paso dentro de una activity
// y registra en el histograma symphony.sqx.activity.step_duration en segundos.
func (s *SQXMetrics) StartActivityStepTimer(
	ctx context.Context,
	activityName string,
	step string,
	additionalAttrs ...attribute.KeyValue,
) func() {
	start := time.Now()
	return func() {
		duration := time.Since(start).Seconds()
		attrs := AddDefaultSQXActivityAttributes(activityName)
		attrs = append(attrs, semconv.SQX.Step.String(step))
		attrs = append(attrs, additionalAttrs...)
		name := MetricName("symphony.sqx", "activity", "step_duration")
		s.Activity.client.RecordHistogram(ctx, name, duration, attrs...)
	}
}

// RecordActivityVolume registra un volumen de eventos para una activity
func (s *SQXMetrics) RecordActivityVolume(
	ctx context.Context,
	activityName string,
	count int64,
	additionalAttrs ...attribute.KeyValue,
) {
	if count <= 0 {
		return
	}
	attrs := AddDefaultSQXActivityAttributes(activityName)
	attrs = append(attrs, additionalAttrs...)
	// Añadir el conteo como atributo y una acción por defecto
	attrs = append(attrs, semconv.Metrics.Count.Int64(count))
	attrs = append(attrs, semconv.Metrics.Action.String("count"))
	for i := int64(0); i < count; i++ {
		s.Activity.RecordResult(ctx, attrs...)
	}
}

// Compatibilidad hacia atrás con contadores de éxito/error por actividad
func (s *SQXMetrics) IncActivitySuccess(ctx context.Context, attrs ...attribute.KeyValue) {
	merged := append([]attribute.KeyValue{}, attrs...)
	merged = append(merged, semconv.Metrics.Status.String("success"))
	s.Activity.RecordResult(ctx, merged...)
}

func (s *SQXMetrics) IncActivityError(ctx context.Context, attrs ...attribute.KeyValue) {
	merged := append([]attribute.KeyValue{}, attrs...)
	merged = append(merged, semconv.Metrics.Status.String("error"))
	s.Activity.RecordResult(ctx, merged...)
}

// Singleton global del bundle SQX
var (
	globalSQXMetrics   *SQXMetrics
	onceInitSQXMetrics sync.Once
)

func InitGlobalSQXBundle(client MetricsClient) {
	onceInitSQXMetrics.Do(func() {
		globalSQXMetrics = NewSQXMetrics(client)
	})
}

func GetGlobalSQXMetrics() *SQXMetrics {
	return globalSQXMetrics // nil si no inicializado (no-op seguro)
}

// Atributos convenientes
func ActivityAttrs(name string, extra ...attribute.KeyValue) []attribute.KeyValue {
	attrs := AddDefaultSQXActivityAttributes(name)
	return append(attrs, extra...)
}
