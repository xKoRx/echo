package metricbundle_test

import (
	"context"
	"fmt"
	"time"

	"github.com/xKoRx/sdk/pkg/shared/telemetry/metricbundle"
	"github.com/xKoRx/sdk/pkg/shared/telemetry/semconv"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// Esta función simula un cliente de métricas para ejemplos
// En un caso real, se usaría telemetry.Init para obtener el cliente
func getExampleClient() metricbundle.MetricsClient {
	// Crear un mock del cliente para el ejemplo
	return &mockMetricsClient{}
}

// ExampleHTTPMetrics_RecordHTTPRequest muestra cómo registrar métricas para una petición HTTP
func ExampleHTTPMetrics_RecordHTTPRequest() {
	// Obtener un cliente de métricas (en una aplicación real sería a través de telemetry.Init)
	client := getExampleClient()

	// Crear un bundle de métricas HTTP
	httpMetrics := metricbundle.NewHTTPMetrics(client)

	// Registrar una petición HTTP exitosa
	ctx := context.Background()
	httpMetrics.RecordHTTPRequest(ctx, "GET", "/api/v1/products", 200,
		semconv.Metrics.Service.String("product-api"),
	)

	// También se puede registrar una petición con error
	httpMetrics.RecordHTTPRequest(ctx, "POST", "/api/v1/orders", 500,
		semconv.Metrics.Service.String("order-api"),
		attribute.String("error_type", "database_connection"),
	)

	fmt.Println("HTTP metrics recorded successfully")
	// Output: HTTP metrics recorded successfully
}

// ExampleBaseMetrics_StartDurationTimer muestra cómo medir y registrar la duración de una operación
func ExampleBaseMetrics_StartDurationTimer() {
	// Obtener un cliente de métricas
	client := getExampleClient()

	// Crear un bundle de métricas base
	baseMetrics := metricbundle.NewBaseMetrics(client, "example", "operation")

	// Iniciar un temporizador para medir la duración
	ctx := context.Background()
	done := baseMetrics.StartDurationTimer(ctx,
		semconv.Metrics.Service.String("example-service"),
		semconv.Metrics.Action.String("process_data"),
	)

	// Simular alguna operación que tome tiempo
	time.Sleep(50 * time.Millisecond)

	// Finalizar la medición y registrar la duración
	done()

	fmt.Println("Duration metric recorded successfully")
	// Output: Duration metric recorded successfully
}

// ExampleFormattedMetricName muestra cómo generar nombres de métrica consistentes
func ExampleFormattedMetricName() {
	// Generar nombres de métrica con el formato estándar
	requestsName := metricbundle.FormattedMetricName("trading", "order", "requests")
	errorsName := metricbundle.FormattedMetricName("trading", "order", "errors")
	latencyName := metricbundle.FormattedMetricName("trading", "order", "latency")

	fmt.Println(requestsName)
	fmt.Println(errorsName)
	fmt.Println(latencyName)
	// Output:
	// trading.order.requests
	// trading.order.errors
	// trading.order.latency
}

// ExampleSignalMetrics_RecordSignalGenerated muestra cómo registrar métricas para una señal generada
func ExampleSignalMetrics_RecordSignalGenerated() {
	// Obtener un cliente de métricas (en una aplicación real sería a través de telemetry.Init)
	client := getExampleClient()

	// Crear un bundle de métricas de señales
	signalMetrics := metricbundle.NewSignalMetrics(client)

	// Registrar una señal de trading generada exitosamente
	ctx := context.Background()
	signalMetrics.RecordSignalGenerated(ctx, "binance", "BTCUSDT", "momentum", "long", true,
		semconv.Metrics.Service.String("signal-service"),
		attribute.Float64("confidence", 0.85),
	)

	// También se puede registrar una señal con error
	signalMetrics.RecordSignalGenerated(ctx, "bybit", "ETHUSDT", "breakout", "short", false,
		semconv.Metrics.Service.String("signal-service"),
		attribute.String("error_type", "signal_validation_failed"),
	)

	fmt.Println("Signal metrics recorded successfully")
	// Output: Signal metrics recorded successfully
}

// ExampleTradeMetrics_RecordTradeCompleted muestra cómo registrar métricas para un trade completado
func ExampleTradeMetrics_RecordTradeCompleted() {
	// Obtener un cliente de métricas (en una aplicación real sería a través de telemetry.Init)
	client := getExampleClient()

	// Crear un bundle de métricas de trades
	tradeMetrics := metricbundle.NewTradeMetrics(client)

	// Registrar un trade completado exitosamente con profit
	ctx := context.Background()
	tradeMetrics.RecordTradeCompleted(ctx, "binance", "BTCUSDT", "momentum", "long", 135.75, true,
		semconv.Metrics.Service.String("trade-service"),
		attribute.Float64("entry_price", 45000.50),
		attribute.Float64("exit_price", 46500.25),
	)

	// También se puede registrar un trade con pérdida
	tradeMetrics.RecordTradeCompleted(ctx, "bybit", "ETHUSDT", "breakout", "short", -42.30, true,
		semconv.Metrics.Service.String("trade-service"),
		attribute.Float64("entry_price", 3200.00),
		attribute.Float64("exit_price", 3250.50),
	)

	// Registrar análisis de rendimiento
	tradeMetrics.RecordTradeProfitLoss(ctx, "binance", "BTCUSDT", "momentum", 135.75, 3600.0,
		semconv.Metrics.Service.String("trade-service"),
		attribute.Int64("trade_id", 12345),
	)

	fmt.Println("Trade metrics recorded successfully")
	// Output: Trade metrics recorded successfully
}

// ExampleTemporalMetrics_RecordActivityResult muestra cómo registrar métricas para una activity de Temporal
func ExampleTemporalMetrics_RecordActivityResult() {
	client := getExampleClient()
	tm := metricbundle.NewTemporalMetrics(client)

	ctx := context.Background()
	tm.RecordActivityResult(ctx, "GenerateReport", true,
		semconv.Metrics.Service.String("temporal-worker"),
	)

	// También se puede medir duración de una activity
	stop := tm.StartActivityTimer(ctx,
		semconv.Metrics.Component.String("temporal-activity"),
	)
	time.Sleep(5 * time.Millisecond)
	stop()

	fmt.Println("Temporal activity metrics recorded successfully")
	// Output: Temporal activity metrics recorded successfully
}

// Mock simple del cliente de métricas para los ejemplos
type mockMetricsClient struct{}

func (m *mockMetricsClient) Counter(name, description string) metric.Int64Counter {
	return nil
}

func (m *mockMetricsClient) Gauge(name, description string) metric.Float64ObservableGauge {
	return nil
}

func (m *mockMetricsClient) Histogram(name, description string) metric.Float64Histogram {
	return nil
}

func (m *mockMetricsClient) RecordCounter(ctx context.Context, name string, value int64, attrs ...attribute.KeyValue) {
	// Simulación
}

func (m *mockMetricsClient) RecordHistogram(ctx context.Context, name string, value float64, attrs ...attribute.KeyValue) {
	// Simulación
}

func (m *mockMetricsClient) RegisterGauge(ctx context.Context, name string, callback func(ctx context.Context) float64, attrs ...attribute.KeyValue) error {
	return nil
}

func (m *mockMetricsClient) Shutdown(ctx context.Context) error {
	return nil
}

func (m *mockMetricsClient) IsInitialized() bool {
	return true
}

func (m *mockMetricsClient) Error() error {
	return nil
}
