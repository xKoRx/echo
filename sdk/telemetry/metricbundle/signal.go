package metricbundle

import (
	"context"
	"sync"

	"github.com/xKoRx/sdk/pkg/shared/telemetry/semconv"
	"go.opentelemetry.io/otel/attribute"
)

// SignalMetrics representa métricas relacionadas a señales de trading
type SignalMetrics struct {
	*BaseMetrics
	// Si se necesitan métricas específicas adicionales, se añadirían aquí
}

// NewSignalMetrics inicializa un nuevo bundle de métricas para señales
func NewSignalMetrics(client MetricsClient) *SignalMetrics {
	// Creamos la base con namespace "trading" y entity "signal"
	base := NewBaseMetrics(client, "trading", "signal")

	return &SignalMetrics{
		BaseMetrics: base,
	}
}

// ----------------------------------------------------------------------------------
// Bundle global singleton con inicialización segura para concurrencia
// ----------------------------------------------------------------------------------

var (
	globalSignalMetrics   *SignalMetrics
	onceInitSignalMetrics sync.Once
)

// InitGlobalSignalBundle inicializa el bundle global para uso compartido
func InitGlobalSignalBundle(client MetricsClient) {
	onceInitSignalMetrics.Do(func() {
		globalSignalMetrics = NewSignalMetrics(client)
	})
}

// GetGlobalSignalMetrics retorna el bundle global ya inicializado
func GetGlobalSignalMetrics() *SignalMetrics {
	return globalSignalMetrics // nil si no inicializado (no-op seguro)
}

// ----------------------------------------------------------------------------------
// Métodos específicos para señales
// ----------------------------------------------------------------------------------

// AddDefaultSignalAttributes añade atributos comunes para métricas de señales
func AddDefaultSignalAttributes(exchange, symbol, strategy string) []attribute.KeyValue {
	return []attribute.KeyValue{
		semconv.Metrics.Exchange.String(exchange),
		semconv.Metrics.Symbol.String(symbol),
		semconv.Metrics.Strategy.String(strategy),
		semconv.Metrics.Service.String("signal-generator"),
	}
}

// ----------------------------------------------------------------------------------
// Helpers para casos de uso comunes
// ----------------------------------------------------------------------------------

// RecordSignalGenerated registra métricas para una señal generada
func (sm *SignalMetrics) RecordSignalGenerated(
	ctx context.Context,
	exchange string,
	symbol string,
	strategy string,
	direction string,
	success bool,
	additionalAttrs ...attribute.KeyValue,
) {
	// Combinar atributos por defecto con adicionales
	attrs := AddDefaultSignalAttributes(exchange, symbol, strategy)
	attrs = append(attrs, additionalAttrs...)

	// Añadir dirección de la señal (long/short)
	attrs = append(attrs, attribute.String("direction", direction))

	// Añadir el resultado (éxito o error)
	status := "success"
	if !success {
		status = "error"
	}
	attrs = append(attrs, semconv.Metrics.Status.String(status))

	// Añadir la acción realizada
	attrs = append(attrs, semconv.Metrics.Action.String("generate"))

	// Registrar en el contador de resultados
	sm.RecordResult(ctx, attrs...)
}

// RecordSignalExecuted registra métricas para una señal ejecutada
func (sm *SignalMetrics) RecordSignalExecuted(
	ctx context.Context,
	exchange string,
	symbol string,
	strategy string,
	direction string,
	success bool,
	additionalAttrs ...attribute.KeyValue,
) {
	// Combinar atributos por defecto con adicionales
	attrs := AddDefaultSignalAttributes(exchange, symbol, strategy)
	attrs = append(attrs, additionalAttrs...)

	// Añadir dirección de la señal (long/short)
	attrs = append(attrs, attribute.String("direction", direction))

	// Añadir el resultado (éxito o error)
	status := "success"
	if !success {
		status = "error"
	}
	attrs = append(attrs, semconv.Metrics.Status.String(status))

	// Añadir la acción realizada
	attrs = append(attrs, semconv.Metrics.Action.String("execute"))

	// Registrar en el contador de resultados
	sm.RecordResult(ctx, attrs...)
}

// RecordSignalVolume registra un volumen de señales procesadas
func (sm *SignalMetrics) RecordSignalVolume(
	ctx context.Context,
	exchange string,
	symbol string,
	strategy string,
	count int64,
	additionalAttrs ...attribute.KeyValue,
) {
	// Combinar atributos por defecto con adicionales
	attrs := AddDefaultSignalAttributes(exchange, symbol, strategy)
	attrs = append(attrs, additionalAttrs...)

	// Añadir el conteo
	attrs = append(attrs, semconv.Metrics.Count.Int64(count))

	// Añadir la acción realizada
	attrs = append(attrs, semconv.Metrics.Action.String("count"))

	// Registrar en el contador de resultados, multiplicando por el conteo
	// para que represente la cantidad total de señales
	for i := int64(0); i < count; i++ {
		sm.RecordResult(ctx, attrs...)
	}
}
