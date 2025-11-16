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
//   - echo.intent.received: TradeIntents recibidos por Agent
//   - echo.intent.forwarded: TradeIntents enviados al Core
//   - echo.order.created: ExecuteOrders creados por Core
//   - echo.order.sent: ExecuteOrders enviados a Agents
//   - echo.execution.dispatched: ExecuteOrders enviados a Slaves
//   - echo.execution.completed: Ejecuciones finalizadas (success/error)
//
// # Métricas de Latencia
//
//   - echo.latency.e2e: Latencia extremo a extremo (t7 - t0)
//   - echo.latency.agent_to_core: Agent → Core (t2 - t1)
//   - echo.latency.core_process: Procesamiento en Core (t3 - t2)
//   - echo.latency.core_to_agent: Core → Agent (t4 - t3)
//   - echo.latency.agent_to_slave: Agent → Slave (t5 - t4)
//   - echo.latency.slave_execution: Ejecución en Slave (t7 - t6)
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
	IntentReceived             metric.Int64Counter
	IntentForwarded            metric.Int64Counter
	OrderCreated               metric.Int64Counter
	OrderSent                  metric.Int64Counter
	ExecutionDispatched        metric.Int64Counter
	ExecutionCompleted         metric.Int64Counter
	VolumeClamp                metric.Int64Counter
	VolumeGuardDecision        metric.Int64Counter
	AgentSpecsForwarded        metric.Int64Counter
	AgentSpecsFiltered         metric.Int64Counter
	RiskPolicyLookup           metric.Int64Counter
	FixedRiskCalculation       metric.Int64Counter
	RiskPolicyRejected         metric.Int64Counter
	StopOffsetApplied          metric.Int64Counter
	StopOffsetEdgeRejection    metric.Int64Counter
	StopOffsetFallback         metric.Int64Counter
	HandshakeVersion           metric.Int64Counter
	HandshakeStatus            metric.Int64Counter
	HandshakeSymbolIssue       metric.Int64Counter
	HandshakeReconcileSkipped  metric.Int64Counter
	AgentHandshakeForwarded    metric.Int64Counter
	AgentHandshakeBlocked      metric.Int64Counter
	AgentHandshakeForwardError metric.Int64Counter

	// Histograms
	LatencyE2E               metric.Float64Histogram
	LatencyAgentToCore       metric.Float64Histogram
	LatencyCoreProcess       metric.Float64Histogram
	LatencyCoreToAgent       metric.Float64Histogram
	LatencyAgentToSlave      metric.Float64Histogram
	LatencySlaveExecution    metric.Float64Histogram
	VolumeGuardSpecAge       metric.Float64Histogram
	HandshakeFeedbackLatency metric.Float64Histogram
	FixedRiskExposure        metric.Float64Histogram
	FixedRiskDistancePoints  metric.Float64Histogram
	StopOffsetDistance       metric.Float64Histogram

	// i2: Routing metrics
	RoutingMode   metric.Int64Counter
	AccountLookup metric.Int64Counter

	// i3: Symbol metrics
	SymbolsLookup   metric.Int64Counter // echo.symbols.lookup (hit/miss)
	SymbolsReported metric.Int64Counter // echo.symbols.reported
	SymbolsValidate metric.Int64Counter // echo.symbols.validate (ok/reject)
	SymbolsLoaded   metric.Int64Counter // echo.symbols.loaded (source=etcd/postgres/agent_report)
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

	volumeClamp, err := meter.Int64Counter(
		"echo.core.volume_guard_clamp_total",
		metric.WithDescription("Órdenes ajustadas por el guardián de volumen"),
		metric.WithUnit("{clamp}"),
	)
	if err != nil {
		return nil, err
	}

	volumeGuardDecision, err := meter.Int64Counter(
		"echo.core.volume_guard_decision_total",
		metric.WithDescription("Decisiones del guardián de volumen (clamp/reject/pass_through)"),
		metric.WithUnit("{decision}"),
	)
	if err != nil {
		return nil, err
	}

	agentSpecsForwarded, err := meter.Int64Counter(
		"echo.agent.specs.forwarded_total",
		metric.WithDescription("Reportes de especificaciones reenviados al Core"),
		metric.WithUnit("{report}"),
	)
	if err != nil {
		return nil, err
	}

	agentSpecsFiltered, err := meter.Int64Counter(
		"echo.agent.specs.filtered_total",
		metric.WithDescription("Reportes de especificaciones filtrados en el Agent"),
		metric.WithUnit("{report}"),
	)
	if err != nil {
		return nil, err
	}

	riskPolicyLookup, err := meter.Int64Counter(
		"echo.core.risk_policy_lookup_total",
		metric.WithDescription("Consultas al servicio de políticas de riesgo (hit/miss)"),
		metric.WithUnit("{lookup}"),
	)
	if err != nil {
		return nil, err
	}

	fixedRiskCalculation, err := meter.Int64Counter(
		"echo.core.risk.fixed_risk_calculation_total",
		metric.WithDescription("Cálculos del motor de riesgo fijo por resultado"),
		metric.WithUnit("{calculation}"),
	)
	if err != nil {
		return nil, err
	}

	riskPolicyRejected, err := meter.Int64Counter(
		"echo.core.risk.policy_rejected_total",
		metric.WithDescription("Políticas de riesgo rechazadas por motivo"),
		metric.WithUnit("{reject}"),
	)
	if err != nil {
		return nil, err
	}

	stopOffsetApplied, err := meter.Int64Counter(
		"echo_core.stop_offset_applied_total",
		metric.WithDescription("Offsets SL/TP aplicados/clampados por tipo"),
		metric.WithUnit("{offset}"),
	)
	if err != nil {
		return nil, err
	}

	stopOffsetEdgeRejection, err := meter.Int64Counter(
		"echo_core.stop_offset_edge_rejections_total",
		metric.WithDescription("Clamps obligatorios por StopLevel o distancia mínima"),
		metric.WithUnit("{clamp}"),
	)
	if err != nil {
		return nil, err
	}

	stopOffsetFallback, err := meter.Int64Counter(
		"echo_core.stop_offset_fallback_total",
		metric.WithDescription("Fallbacks solicitados ante ERROR_CODE_INVALID_STOPS"),
		metric.WithUnit("{fallback}"),
	)
	if err != nil {
		return nil, err
	}

	handshakeVersion, err := meter.Int64Counter(
		"echo.core.handshake.version_total",
		metric.WithDescription("Handshakes evaluados por versión y resultado"),
		metric.WithUnit("{handshake}"),
	)
	if err != nil {
		return nil, err
	}

	handshakeStatus, err := meter.Int64Counter(
		"echo.core.handshake.status_total",
		metric.WithDescription("Resultados globales de registro de símbolos"),
		metric.WithUnit("{registration}"),
	)
	if err != nil {
		return nil, err
	}

	handshakeSymbolIssue, err := meter.Int64Counter(
		"echo.core.handshake.symbol_issues_total",
		metric.WithDescription("Conteo de issues detectados durante el registro de símbolos"),
		metric.WithUnit("{issue}"),
	)
	if err != nil {
		return nil, err
	}

	handshakeReconcileSkipped, err := meter.Int64Counter(
		"echo.core.handshake.reconcile_skipped_total",
		metric.WithDescription("Re-evaluaciones de handshake descartadas por no presentar cambios"),
		metric.WithUnit("{evaluation}"),
	)
	if err != nil {
		return nil, err
	}

	agentHandshakeForwarded, err := meter.Int64Counter(
		"echo.agent.handshake.forwarded_total",
		metric.WithDescription("Handshakes reenviados al Core por estado"),
		metric.WithUnit("{handshake}"),
	)
	if err != nil {
		return nil, err
	}

	agentHandshakeBlocked, err := meter.Int64Counter(
		"echo.agent.handshake.blocked_total",
		metric.WithDescription("Handshakes bloqueados en el Agent"),
		metric.WithUnit("{handshake}"),
	)
	if err != nil {
		return nil, err
	}

	agentHandshakeForwardError, err := meter.Int64Counter(
		"echo.agent.handshake.forward_error_total",
		metric.WithDescription("Errores al reenviar feedback de handshake hacia el EA"),
		metric.WithUnit("{error}"),
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

	volumeGuardSpecAge, err := meter.Float64Histogram(
		"echo.core.volume_guard_spec_age_ms",
		metric.WithDescription("Edad de la especificación de símbolo utilizada por el guardián de volumen"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return nil, err
	}

	handshakeFeedbackLatency, err := meter.Float64Histogram(
		"echo.handshake.feedback_latency_ms",
		metric.WithDescription("Latencia entre handshake y feedback entregado al EA"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return nil, err
	}

	fixedRiskExposure, err := meter.Float64Histogram(
		"echo.core.risk.expected_loss",
		metric.WithDescription("Pérdida monetaria esperada por orden en políticas FIXED_RISK"),
		metric.WithUnit("{currency}"),
	)
	if err != nil {
		return nil, err
	}

	fixedRiskDistancePoints, err := meter.Float64Histogram(
		"echo.core.risk.distance_points",
		metric.WithDescription("Distancia en puntos entre precio y stop para políticas FIXED_RISK"),
		metric.WithUnit("{point}"),
	)
	if err != nil {
		return nil, err
	}

	stopOffsetDistance, err := meter.Float64Histogram(
		"echo_core.stop_offset_distance_pips",
		metric.WithDescription("Distancia final en pips tras aplicar offsets SL/TP"),
		metric.WithUnit("pip"),
		metric.WithExplicitBucketBoundaries(0, 1, 2, 5, 10, 20, 50, 100),
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

	// i3: Symbol metrics
	symbolsLookup, err := meter.Int64Counter(
		"echo.symbols.lookup",
		metric.WithDescription("Resultado de lookup de símbolo por cuenta (hit, miss)"),
		metric.WithUnit("{lookup}"),
	)
	if err != nil {
		return nil, err
	}

	symbolsReported, err := meter.Int64Counter(
		"echo.symbols.reported",
		metric.WithDescription("Reportes de símbolos recibidos por cuenta"),
		metric.WithUnit("{report}"),
	)
	if err != nil {
		return nil, err
	}

	symbolsValidate, err := meter.Int64Counter(
		"echo.symbols.validate",
		metric.WithDescription("Validaciones de símbolos canónicos (ok, reject)"),
		metric.WithUnit("{validation}"),
	)
	if err != nil {
		return nil, err
	}

	symbolsLoaded, err := meter.Int64Counter(
		"echo.symbols.loaded",
		metric.WithDescription("Símbolos cargados desde fuente (etcd, postgres, agent_report)"),
		metric.WithUnit("{symbol}"),
	)
	if err != nil {
		return nil, err
	}

	return &EchoMetrics{
		IntentReceived:             intentReceived,
		IntentForwarded:            intentForwarded,
		OrderCreated:               orderCreated,
		OrderSent:                  orderSent,
		ExecutionDispatched:        executionDispatched,
		ExecutionCompleted:         executionCompleted,
		VolumeClamp:                volumeClamp,
		VolumeGuardDecision:        volumeGuardDecision,
		AgentSpecsForwarded:        agentSpecsForwarded,
		AgentSpecsFiltered:         agentSpecsFiltered,
		RiskPolicyLookup:           riskPolicyLookup,
		FixedRiskCalculation:       fixedRiskCalculation,
		RiskPolicyRejected:         riskPolicyRejected,
		StopOffsetApplied:          stopOffsetApplied,
		StopOffsetEdgeRejection:    stopOffsetEdgeRejection,
		StopOffsetFallback:         stopOffsetFallback,
		HandshakeVersion:           handshakeVersion,
		HandshakeStatus:            handshakeStatus,
		HandshakeSymbolIssue:       handshakeSymbolIssue,
		HandshakeReconcileSkipped:  handshakeReconcileSkipped,
		AgentHandshakeForwarded:    agentHandshakeForwarded,
		AgentHandshakeBlocked:      agentHandshakeBlocked,
		AgentHandshakeForwardError: agentHandshakeForwardError,
		LatencyE2E:                 latencyE2E,
		LatencyAgentToCore:         latencyAgentToCore,
		LatencyCoreProcess:         latencyCoreProcess,
		LatencyCoreToAgent:         latencyCoreToAgent,
		LatencyAgentToSlave:        latencyAgentToSlave,
		LatencySlaveExecution:      latencySlaveExecution,
		VolumeGuardSpecAge:         volumeGuardSpecAge,
		HandshakeFeedbackLatency:   handshakeFeedbackLatency,
		FixedRiskExposure:          fixedRiskExposure,
		FixedRiskDistancePoints:    fixedRiskDistancePoints,
		StopOffsetDistance:         stopOffsetDistance,
		RoutingMode:                routingMode,     // i2
		AccountLookup:              accountLookup,   // i2
		SymbolsLookup:              symbolsLookup,   // i3
		SymbolsReported:            symbolsReported, // i3
		SymbolsValidate:            symbolsValidate, // i3
		SymbolsLoaded:              symbolsLoaded,   // i3
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

// RecordSymbolLookup registra lookup de símbolo por cuenta (i3).
//
// result: "hit" | "miss"
func (m *EchoMetrics) RecordSymbolLookup(ctx context.Context, result string, accountID, canonical string, attrs ...attribute.KeyValue) {
	baseAttrs := []attribute.KeyValue{
		attribute.String("lookup.result", result),
		attribute.String("account_id", accountID),
		attribute.String("canonical_symbol", canonical),
	}
	baseAttrs = append(baseAttrs, attrs...)
	m.SymbolsLookup.Add(ctx, 1, metric.WithAttributes(baseAttrs...))
}

// RecordSymbolsReported registra reporte de símbolos por cuenta (i3).
func (m *EchoMetrics) RecordSymbolsReported(ctx context.Context, accountID string, count int, attrs ...attribute.KeyValue) {
	baseAttrs := []attribute.KeyValue{
		attribute.String("account_id", accountID),
		attribute.Int("symbols_count", count),
	}
	baseAttrs = append(baseAttrs, attrs...)
	m.SymbolsReported.Add(ctx, 1, metric.WithAttributes(baseAttrs...))
}

// RecordSymbolValidate registra validación de símbolo canónico (i3).
//
// result: "ok" | "reject"
func (m *EchoMetrics) RecordSymbolValidate(ctx context.Context, result string, symbol string, attrs ...attribute.KeyValue) {
	baseAttrs := []attribute.KeyValue{
		attribute.String("validation.result", result),
		attribute.String("symbol", symbol),
	}
	baseAttrs = append(baseAttrs, attrs...)
	m.SymbolsValidate.Add(ctx, 1, metric.WithAttributes(baseAttrs...))
}

// RecordSymbolsLoaded registra carga de símbolos desde fuente (i3).
//
// source: "etcd" | "postgres" | "agent_report"
func (m *EchoMetrics) RecordSymbolsLoaded(ctx context.Context, source string, count int, attrs ...attribute.KeyValue) {
	baseAttrs := []attribute.KeyValue{
		attribute.String("source", source),
		attribute.Int("symbols_count", count),
	}
	baseAttrs = append(baseAttrs, attrs...)
	m.SymbolsLoaded.Add(ctx, int64(count), metric.WithAttributes(baseAttrs...))
}

// RecordVolumeClamp registra un clamp de lot size por el guardián de volumen.
func (m *EchoMetrics) RecordVolumeClamp(ctx context.Context, attrs ...attribute.KeyValue) {
	m.VolumeClamp.Add(ctx, 1, metric.WithAttributes(attrs...))
}

// RecordVolumeGuardDecision registra la decisión del guardián de volumen.
// decision: clamp | reject | pass_through
func (m *EchoMetrics) RecordVolumeGuardDecision(ctx context.Context, decision string, attrs ...attribute.KeyValue) {
	baseAttrs := []attribute.KeyValue{
		attribute.String("decision", decision),
	}
	baseAttrs = append(baseAttrs, attrs...)
	m.VolumeGuardDecision.Add(ctx, 1, metric.WithAttributes(baseAttrs...))
}

// RecordVolumeGuardSpecAge registra la edad (en ms) de la especificación usada.
func (m *EchoMetrics) RecordVolumeGuardSpecAge(ctx context.Context, ageMs float64, attrs ...attribute.KeyValue) {
	m.VolumeGuardSpecAge.Record(ctx, ageMs, metric.WithAttributes(attrs...))
}

// RecordAgentSpecsForwarded registra reportes de especificaciones enviados al Core.
func (m *EchoMetrics) RecordAgentSpecsForwarded(ctx context.Context, accountID string, attrs ...attribute.KeyValue) {
	baseAttrs := []attribute.KeyValue{
		attribute.String("account_id", accountID),
	}
	baseAttrs = append(baseAttrs, attrs...)
	m.AgentSpecsForwarded.Add(ctx, 1, metric.WithAttributes(baseAttrs...))
}

// RecordAgentSpecsFiltered registra reportes de especificaciones filtrados en el Agent.
func (m *EchoMetrics) RecordAgentSpecsFiltered(ctx context.Context, accountID, reason string, attrs ...attribute.KeyValue) {
	baseAttrs := []attribute.KeyValue{
		attribute.String("account_id", accountID),
		attribute.String("reason", reason),
	}
	baseAttrs = append(baseAttrs, attrs...)
	m.AgentSpecsFiltered.Add(ctx, 1, metric.WithAttributes(baseAttrs...))
}

// RecordRiskPolicyLookup registra consultas al servicio de políticas de riesgo.
// result: hit | miss
func (m *EchoMetrics) RecordRiskPolicyLookup(ctx context.Context, result string, attrs ...attribute.KeyValue) {
	baseAttrs := []attribute.KeyValue{
		attribute.String("result", result),
	}
	baseAttrs = append(baseAttrs, attrs...)
	m.RiskPolicyLookup.Add(ctx, 1, metric.WithAttributes(baseAttrs...))
}

// RecordFixedRiskCalculation registra ejecuciones del motor FIXED_RISK por resultado.
// result: success | reject | fallback
func (m *EchoMetrics) RecordFixedRiskCalculation(ctx context.Context, result string, attrs ...attribute.KeyValue) {
	baseAttrs := []attribute.KeyValue{
		attribute.String("result", result),
	}
	baseAttrs = append(baseAttrs, attrs...)
	m.FixedRiskCalculation.Add(ctx, 1, metric.WithAttributes(baseAttrs...))
}

// RecordRiskPolicyRejected registra rechazos de políticas de riesgo con su razón.
func (m *EchoMetrics) RecordRiskPolicyRejected(ctx context.Context, reason string, attrs ...attribute.KeyValue) {
	baseAttrs := []attribute.KeyValue{}
	if reason != "" {
		baseAttrs = append(baseAttrs, attribute.String("reason", reason))
	}
	baseAttrs = append(baseAttrs, attrs...)
	m.RiskPolicyRejected.Add(ctx, 1, metric.WithAttributes(baseAttrs...))
}

// RecordStopOffsetApplied registra el resultado del offset (applied/clamped/skipped) por tipo (sl/tp).
func (m *EchoMetrics) RecordStopOffsetApplied(ctx context.Context, stopType, segment, result string) {
	if m.StopOffsetApplied == nil {
		return
	}
	attrs := []attribute.KeyValue{
		attribute.String("type", stopType),
		attribute.String("segment", segment),
		attribute.String("result", result),
	}
	m.StopOffsetApplied.Add(ctx, 1, metric.WithAttributes(attrs...))
}

// RecordStopOffsetDistance registra la distancia final en pips tras aplicar offsets.
func (m *EchoMetrics) RecordStopOffsetDistance(ctx context.Context, stopType, segment string, distance float64) {
	if m.StopOffsetDistance == nil || distance <= 0 {
		return
	}
	attrs := []attribute.KeyValue{
		attribute.String("type", stopType),
		attribute.String("segment", segment),
	}
	m.StopOffsetDistance.Record(ctx, distance, metric.WithAttributes(attrs...))
}

// RecordStopOffsetEdgeRejection registra clamps obligatorios.
func (m *EchoMetrics) RecordStopOffsetEdgeRejection(ctx context.Context, reason, segment string) {
	if m.StopOffsetEdgeRejection == nil {
		return
	}
	attrs := []attribute.KeyValue{
		attribute.String("reason", reason),
		attribute.String("segment", segment),
	}
	m.StopOffsetEdgeRejection.Add(ctx, 1, metric.WithAttributes(attrs...))
}

// RecordStopOffsetFallback registra el estado del fallback ante INVALID_STOPS.
func (m *EchoMetrics) RecordStopOffsetFallback(ctx context.Context, stage, result, segment string) {
	if m.StopOffsetFallback == nil {
		return
	}
	attrs := []attribute.KeyValue{
		attribute.String("stage", stage),
		attribute.String("result", result),
		attribute.String("segment", segment),
	}
	m.StopOffsetFallback.Add(ctx, 1, metric.WithAttributes(attrs...))
}

// RecordFixedRiskExposure registra la pérdida esperada calculada para FIXED_RISK.
func (m *EchoMetrics) RecordFixedRiskExposure(ctx context.Context, amount float64, attrs ...attribute.KeyValue) {
	m.FixedRiskExposure.Record(ctx, amount, metric.WithAttributes(attrs...))
}

// RecordFixedRiskDistancePoints registra la distancia en points entre precio y stop.
func (m *EchoMetrics) RecordFixedRiskDistancePoints(ctx context.Context, points float64, attrs ...attribute.KeyValue) {
	m.FixedRiskDistancePoints.Record(ctx, points, metric.WithAttributes(attrs...))
}

// RecordHandshakeVersion registra la evaluación de handshake por versión y estado global.
func (m *EchoMetrics) RecordHandshakeVersion(ctx context.Context, version int, status string, attrs ...attribute.KeyValue) {
	baseAttrs := []attribute.KeyValue{
		attribute.Int("protocol_version", version),
		attribute.String("status", status),
	}
	baseAttrs = append(baseAttrs, attrs...)
	m.HandshakeVersion.Add(ctx, 1, metric.WithAttributes(baseAttrs...))
}

// RecordSymbolRegistration registra el resultado de registro por símbolo.
func (m *EchoMetrics) RecordSymbolRegistration(ctx context.Context, status string, canonical string, attrs ...attribute.KeyValue) {
	baseAttrs := []attribute.KeyValue{
		attribute.String("status", status),
		attribute.String("canonical_symbol", canonical),
	}
	baseAttrs = append(baseAttrs, attrs...)
	m.HandshakeStatus.Add(ctx, 1, metric.WithAttributes(baseAttrs...))
}

// RecordSymbolRegistrationIssue registra issues observados durante la evaluación.
func (m *EchoMetrics) RecordSymbolRegistrationIssue(ctx context.Context, issue string, scope string, canonical string, attrs ...attribute.KeyValue) {
	baseAttrs := []attribute.KeyValue{
		attribute.String("issue", issue),
	}
	if scope != "" {
		baseAttrs = append(baseAttrs, attribute.String("scope", scope))
	}
	if canonical != "" {
		baseAttrs = append(baseAttrs, attribute.String("canonical_symbol", canonical))
	}
	baseAttrs = append(baseAttrs, attrs...)
	m.HandshakeSymbolIssue.Add(ctx, 1, metric.WithAttributes(baseAttrs...))
}

// RecordAgentHandshakeForwarded registra feedback reenviado por el Agent.
func (m *EchoMetrics) RecordAgentHandshakeForwarded(ctx context.Context, status string, attrs ...attribute.KeyValue) {
	baseAttrs := []attribute.KeyValue{
		attribute.String("status", status),
	}
	baseAttrs = append(baseAttrs, attrs...)
	m.AgentHandshakeForwarded.Add(ctx, 1, metric.WithAttributes(baseAttrs...))
}

// RecordAgentHandshakeBlocked registra bloqueos tempranos en el Agent.
func (m *EchoMetrics) RecordAgentHandshakeBlocked(ctx context.Context, reason string, attrs ...attribute.KeyValue) {
	baseAttrs := []attribute.KeyValue{}
	if reason != "" {
		baseAttrs = append(baseAttrs, attribute.String("reason", reason))
	}
	baseAttrs = append(baseAttrs, attrs...)
	m.AgentHandshakeBlocked.Add(ctx, 1, metric.WithAttributes(baseAttrs...))
}

// RecordAgentHandshakeForwardError registra errores al reenviar feedback al EA.
func (m *EchoMetrics) RecordAgentHandshakeForwardError(ctx context.Context, reason string, attrs ...attribute.KeyValue) {
	baseAttrs := []attribute.KeyValue{}
	if reason != "" {
		baseAttrs = append(baseAttrs, attribute.String("reason", reason))
	}
	baseAttrs = append(baseAttrs, attrs...)
	m.AgentHandshakeForwardError.Add(ctx, 1, metric.WithAttributes(baseAttrs...))
}

// RecordHandshakeReconcileSkipped registra re-evaluaciones omitidas por falta de cambios.
func (m *EchoMetrics) RecordHandshakeReconcileSkipped(ctx context.Context, attrs ...attribute.KeyValue) {
	if m.HandshakeReconcileSkipped == nil {
		return
	}
	m.HandshakeReconcileSkipped.Add(ctx, 1, metric.WithAttributes(attrs...))
}

// RecordHandshakeFeedbackLatency registra la latencia total del feedback handshake.
func (m *EchoMetrics) RecordHandshakeFeedbackLatency(ctx context.Context, latencyMs float64, attrs ...attribute.KeyValue) {
	m.HandshakeFeedbackLatency.Record(ctx, latencyMs, metric.WithAttributes(attrs...))
}
