package metricbundle

import (
	"context"
	"sync"

	"github.com/xKoRx/sdk/pkg/shared/telemetry/semconv"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// TradeMetrics representa métricas relacionadas a operaciones de trading completadas
type TradeMetrics struct {
	*BaseMetrics
	// Si se necesitan métricas específicas adicionales, se añadirían aquí
}

// NewTradeMetrics inicializa un nuevo bundle de métricas para trades
func NewTradeMetrics(client MetricsClient) *TradeMetrics {
	// Creamos la base con namespace "trading" y entity "trade"
	base := NewBaseMetrics(client, "trading", "trade")

	return &TradeMetrics{
		BaseMetrics: base,
	}
}

// ----------------------------------------------------------------------------------
// Bundle global singleton con inicialización segura para concurrencia
// ----------------------------------------------------------------------------------

var (
	globalTradeMetrics   *TradeMetrics
	onceInitTradeMetrics sync.Once
)

// InitGlobalTradeBundle inicializa el bundle global para uso compartido
func InitGlobalTradeBundle(client MetricsClient) {
	onceInitTradeMetrics.Do(func() {
		globalTradeMetrics = NewTradeMetrics(client)
	})
}

// GetGlobalTradeMetrics retorna el bundle global ya inicializado
func GetGlobalTradeMetrics() *TradeMetrics {
	return globalTradeMetrics // nil si no inicializado (no-op seguro)
}

// ----------------------------------------------------------------------------------
// Métodos específicos para trades
// ----------------------------------------------------------------------------------

// AddDefaultTradeAttributes añade atributos comunes para métricas de trades
func AddDefaultTradeAttributes(exchange, symbol, strategy string) []attribute.KeyValue {
	return []attribute.KeyValue{
		semconv.Metrics.Exchange.String(exchange),
		semconv.Metrics.Symbol.String(symbol),
		semconv.Metrics.Strategy.String(strategy),
		semconv.Metrics.Service.String("trade-service"),
	}
}

// ----------------------------------------------------------------------------------
// Helpers para casos de uso comunes
// ----------------------------------------------------------------------------------

// RecordTradeCompleted registra métricas para un trade completado
func (tm *TradeMetrics) RecordTradeCompleted(
	ctx context.Context,
	exchange string,
	symbol string,
	strategy string,
	direction string,
	profit float64,
	success bool,
	additionalAttrs ...attribute.KeyValue,
) {
	// Combinar atributos por defecto con adicionales
	attrs := AddDefaultTradeAttributes(exchange, symbol, strategy)
	attrs = append(attrs, additionalAttrs...)

	// Añadir dirección del trade (long/short)
	attrs = append(attrs, attribute.String("direction", direction))

	// Añadir profit/loss del trade
	attrs = append(attrs, attribute.Float64("profit", profit))

	// Añadir el resultado (éxito o error)
	status := "success"
	if !success {
		status = "error"
	}
	attrs = append(attrs, semconv.Metrics.Status.String(status))

	// Añadir la acción realizada
	attrs = append(attrs, semconv.Metrics.Action.String("complete"))

	// Registrar en el contador de resultados
	tm.RecordResult(ctx, attrs...)
}

// RecordTradeVolume registra un volumen de trades procesados
func (tm *TradeMetrics) RecordTradeVolume(
	ctx context.Context,
	exchange string,
	symbol string,
	strategy string,
	count int64,
	additionalAttrs ...attribute.KeyValue,
) {
	// Combinar atributos por defecto con adicionales
	attrs := AddDefaultTradeAttributes(exchange, symbol, strategy)
	attrs = append(attrs, additionalAttrs...)

	// Añadir el conteo
	attrs = append(attrs, semconv.Metrics.Count.Int64(count))

	// Añadir la acción realizada
	attrs = append(attrs, semconv.Metrics.Action.String("count"))

	// Registrar en el contador de resultados, multiplicando por el conteo
	// para que represente la cantidad total de trades
	for i := int64(0); i < count; i++ {
		tm.RecordResult(ctx, attrs...)
	}
}

// RecordTradeProfitLoss registra métricas para análisis de rendimiento de trades
func (tm *TradeMetrics) RecordTradeProfitLoss(
	ctx context.Context,
	exchange string,
	symbol string,
	strategy string,
	profitAmount float64,
	durationSeconds float64,
	additionalAttrs ...attribute.KeyValue,
) {
	// Combinar atributos por defecto con adicionales
	attrs := AddDefaultTradeAttributes(exchange, symbol, strategy)
	attrs = append(attrs, additionalAttrs...)

	// Añadir profit/loss del trade
	attrs = append(attrs, attribute.Float64("profit_amount", profitAmount))

	// Añadir duración del trade
	attrs = append(attrs, attribute.Float64("duration_seconds", durationSeconds))

	// Añadir resultado (profit o loss)
	result := "profit"
	if profitAmount < 0 {
		result = "loss"
	}
	attrs = append(attrs, semconv.Metrics.Result.String(result))

	// Añadir la acción realizada
	attrs = append(attrs, semconv.Metrics.Action.String("analyze"))

	// Registrar en el contador de resultados
	tm.RecordResult(ctx, attrs...)

	// Registrar la duración del trade en el histograma
	if tm.DurationHistogram != nil {
		tm.DurationHistogram.Record(ctx, durationSeconds, metric.WithAttributes(attrs...))
	}
}
