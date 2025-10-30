package metricbundle

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// EchoMetrics bundle de métricas para Echo Trade Copier.
//
// Incluye métricas del funnel completo de copiado de operaciones:
// - Intent recibidos/enviados
// - Órdenes creadas/enviadas
// - Ejecuciones completadas
// - Latencias por hop
//
// # Métricas de Conteo
//
//	- echo.intent.received: TradeIntents recibidos por Agent
//	- echo.intent.forwarded: TradeIntents enviados al Core
//	- echo.order.created: ExecuteOrders creados por Core
//	- echo.order.sent: ExecuteOrders enviados a Agents
//	- echo.execution.dispatched: ExecuteOrders enviados a Slaves
//	- echo.execution.completed: Ejecuciones finalizadas (success/error)
//
// # Métricas de Latencia
//
//	- echo.latency.e2e: Latencia extremo a extremo (t7 - t0)
//	- echo.latency.agent_to_core: Agent → Core (t2 - t1)
//	- echo.latency.core_process: Procesamiento en Core (t3 - t2)
//	- echo.latency.core_to_agent: Core → Agent (t4 - t3)
//	- echo.latency.agent_to_slave: Agent → Slave (t5 - t4)
//	- echo.latency.slave_execution: Ejecución en Slave (t7 - t6)
//
// # Uso
//
//	client, _ := telemetry.Init(ctx, etcdClient, "echo-agent", "production",
//	    telemetry.Echo,
//	)
//
//	metrics := client.EchoMetrics()
//
//	// Registrar intent recibido
//	metrics.RecordIntentReceived(ctx,
//	    attribute.String("trade_id", "abc123"),
//	    attribute.String("symbol", "XAUUSD"),
//	)
//
//	// Registrar latencia E2E
//	metrics.RecordLatencyE2E(ctx, 85.5,
//	    attribute.String("trade_id", "abc123"),
//	)
type EchoMetrics struct {
	// Counters
	IntentReceived       metric.Int64Counter
	IntentForwarded      metric.Int64Counter
	OrderCreated         metric.Int64Counter
	OrderSent            metric.Int64Counter
	ExecutionDispatched  metric.Int64Counter
	ExecutionCompleted   metric.Int64Counter

	// Histograms
	LatencyE2E           metric.Float64Histogram
	LatencyAgentToCore   metric.Float64Histogram
	LatencyCoreProcess   metric.Float64Histogram
	LatencyCoreToAgent   metric.Float64Histogram
	LatencyAgentToSlave  metric.Float64Histogram
	LatencySlaveExecution metric.Float64Histogram

	// i2: Routing metrics
	RoutingMode     metric.Int64Counter
	AccountLookup   metric.Int64Counter
}

// NewEchoMetrics crea un nuevo bundle de métricas Echo.
func NewEchoMetrics(meter metric.Meter) (*EchoMetrics, error) {
	// Counters
	intentReceived, err := meter.Int64Counter(
		"echo.intent.received",
		metric.WithDescription("TradeIntents recibidos por Agent desde Master EA"),
		metric.WithUnit("{intent}"),
	)
	if err != nil {
		return nil, err
	}

	intentForwarded, err := meter.Int64Counter(
		"echo.intent.forwarded",
		metric.WithDescription("TradeIntents enviados al Core"),
		metric.WithUnit("{intent}"),
	)
	if err != nil {
		return nil, err
	}

	orderCreated, err := meter.Int64Counter(
		"echo.order.created",
		metric.WithDescription("ExecuteOrders creados por Core"),
		metric.WithUnit("{order}"),
	)
	if err != nil {
		return nil, err
	}

	orderSent, err := meter.Int64Counter(
		"echo.order.sent",
		metric.WithDescription("ExecuteOrders enviados a Agents"),
		metric.WithUnit("{order}"),
	)
	if err != nil {
		return nil, err
	}

	executionDispatched, err := meter.Int64Counter(
		"echo.execution.dispatched",
		metric.WithDescription("ExecuteOrders enviados a Slave EAs"),
		metric.WithUnit("{order}"),
	)
	if err != nil {
		return nil, err
	}

	executionCompleted, err := meter.Int64Counter(
		"echo.execution.completed",
		metric.WithDescription("Ejecuciones finalizadas (success/error)"),
		metric.WithUnit("{execution}"),
	)
	if err != nil {
		return nil, err
	}

	// Histograms
	latencyE2E, err := meter.Float64Histogram(
		"echo.latency.e2e",
		metric.WithDescription("Latencia extremo a extremo (t7 - t0)"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return nil, err
	}

	latencyAgentToCore, err := meter.Float64Histogram(
		"echo.latency.agent_to_core",
		metric.WithDescription("Latencia Agent → Core (t2 - t1)"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return nil, err
	}

	latencyCoreProcess, err := meter.Float64Histogram(
		"echo.latency.core_process",
		metric.WithDescription("Latencia procesamiento en Core (t3 - t2)"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return nil, err
	}

	latencyCoreToAgent, err := meter.Float64Histogram(
		"echo.latency.core_to_agent",
		metric.WithDescription("Latencia Core → Agent (t4 - t3)"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return nil, err
	}

	latencyAgentToSlave, err := meter.Float64Histogram(
		"echo.latency.agent_to_slave",
		metric.WithDescription("Latencia Agent → Slave (t5 - t4)"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return nil, err
	}

	latencySlaveExecution, err := meter.Float64Histogram(
		"echo.latency.slave_execution",
		metric.WithDescription("Latencia ejecución en Slave (t7 - t6)"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return nil, err
	}

	// i2: Routing metrics
	routingMode, err := meter.Int64Counter(
		"echo.routing.mode",
		metric.WithDescription("Modo de routing usado (selective, broadcast, fallback_broadcast)"),
		metric.WithUnit("{routing}"),
	)
	if err != nil {
		return nil, err
	}

	accountLookup, err := meter.Int64Counter(
		"echo.routing.account_lookup",
		metric.WithDescription("Resultado de lookup en AccountRegistry (hit, miss)"),
		metric.WithUnit("{lookup}"),
	)
	if err != nil {
		return nil, err
	}

	return &EchoMetrics{
		IntentReceived:       intentReceived,
		IntentForwarded:      intentForwarded,
		OrderCreated:         orderCreated,
		OrderSent:            orderSent,
		ExecutionDispatched:  executionDispatched,
		ExecutionCompleted:   executionCompleted,
		LatencyE2E:           latencyE2E,
		LatencyAgentToCore:   latencyAgentToCore,
		LatencyCoreProcess:   latencyCoreProcess,
		LatencyCoreToAgent:   latencyCoreToAgent,
		LatencyAgentToSlave:  latencyAgentToSlave,
		LatencySlaveExecution: latencySlaveExecution,
		RoutingMode:          routingMode,    // i2
		AccountLookup:        accountLookup,  // i2
	}, nil
}

// RecordIntentReceived registra recepción de TradeIntent en Agent.
func (m *EchoMetrics) RecordIntentReceived(ctx context.Context, attrs ...attribute.KeyValue) {
	m.IntentReceived.Add(ctx, 1, metric.WithAttributes(attrs...))
}

// RecordIntentForwarded registra envío de TradeIntent al Core.
func (m *EchoMetrics) RecordIntentForwarded(ctx context.Context, attrs ...attribute.KeyValue) {
	m.IntentForwarded.Add(ctx, 1, metric.WithAttributes(attrs...))
}

// RecordOrderCreated registra creación de ExecuteOrder en Core.
func (m *EchoMetrics) RecordOrderCreated(ctx context.Context, attrs ...attribute.KeyValue) {
	m.OrderCreated.Add(ctx, 1, metric.WithAttributes(attrs...))
}

// RecordOrderSent registra envío de ExecuteOrder a Agent.
func (m *EchoMetrics) RecordOrderSent(ctx context.Context, attrs ...attribute.KeyValue) {
	m.OrderSent.Add(ctx, 1, metric.WithAttributes(attrs...))
}

// RecordExecutionDispatched registra envío de ExecuteOrder a Slave EA.
func (m *EchoMetrics) RecordExecutionDispatched(ctx context.Context, attrs ...attribute.KeyValue) {
	m.ExecutionDispatched.Add(ctx, 1, metric.WithAttributes(attrs...))
}

// RecordExecutionCompleted registra finalización de ejecución.
//
// Debe incluir atributo "status" con valor "success" o "error".
func (m *EchoMetrics) RecordExecutionCompleted(ctx context.Context, attrs ...attribute.KeyValue) {
	m.ExecutionCompleted.Add(ctx, 1, metric.WithAttributes(attrs...))
}

// RecordLatencyE2E registra latencia extremo a extremo (ms).
func (m *EchoMetrics) RecordLatencyE2E(ctx context.Context, latencyMs float64, attrs ...attribute.KeyValue) {
	m.LatencyE2E.Record(ctx, latencyMs, metric.WithAttributes(attrs...))
}

// RecordLatencyAgentToCore registra latencia Agent → Core (ms).
func (m *EchoMetrics) RecordLatencyAgentToCore(ctx context.Context, latencyMs float64, attrs ...attribute.KeyValue) {
	m.LatencyAgentToCore.Record(ctx, latencyMs, metric.WithAttributes(attrs...))
}

// RecordLatencyCoreProcess registra latencia procesamiento en Core (ms).
func (m *EchoMetrics) RecordLatencyCoreProcess(ctx context.Context, latencyMs float64, attrs ...attribute.KeyValue) {
	m.LatencyCoreProcess.Record(ctx, latencyMs, metric.WithAttributes(attrs...))
}

// RecordLatencyCoreToAgent registra latencia Core → Agent (ms).
func (m *EchoMetrics) RecordLatencyCoreToAgent(ctx context.Context, latencyMs float64, attrs ...attribute.KeyValue) {
	m.LatencyCoreToAgent.Record(ctx, latencyMs, metric.WithAttributes(attrs...))
}

// RecordLatencyAgentToSlave registra latencia Agent → Slave (ms).
func (m *EchoMetrics) RecordLatencyAgentToSlave(ctx context.Context, latencyMs float64, attrs ...attribute.KeyValue) {
	m.LatencyAgentToSlave.Record(ctx, latencyMs, metric.WithAttributes(attrs...))
}

// RecordLatencySlaveExecution registra latencia ejecución en Slave (ms).
func (m *EchoMetrics) RecordLatencySlaveExecution(ctx context.Context, latencyMs float64, attrs ...attribute.KeyValue) {
	m.LatencySlaveExecution.Record(ctx, latencyMs, metric.WithAttributes(attrs...))
}

// RecordRoutingMode registra el modo de routing usado (i2).
//
// mode: "selective" | "broadcast" | "fallback_broadcast"
// result: "hit" | "miss"
func (m *EchoMetrics) RecordRoutingMode(ctx context.Context, mode string, result string, attrs ...attribute.KeyValue) {
	baseAttrs := []attribute.KeyValue{
		attribute.String("routing.mode", mode),
		attribute.String("routing.result", result),
	}
	baseAttrs = append(baseAttrs, attrs...)

	m.RoutingMode.Add(ctx, 1, metric.WithAttributes(baseAttrs...))
}

// RecordAccountLookup registra lookups al AccountRegistry (i2).
//
// result: "hit" | "miss"
func (m *EchoMetrics) RecordAccountLookup(ctx context.Context, result string, attrs ...attribute.KeyValue) {
	baseAttrs := []attribute.KeyValue{
		attribute.String("lookup.result", result),
	}
	baseAttrs = append(baseAttrs, attrs...)

	m.AccountLookup.Add(ctx, 1, metric.WithAttributes(baseAttrs...))
}

