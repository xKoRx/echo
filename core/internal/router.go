package internal

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/xKoRx/echo/core/internal/riskengine"
	"github.com/xKoRx/echo/core/internal/volumeguard"
	"github.com/xKoRx/echo/sdk/domain"
	"github.com/xKoRx/echo/sdk/domain/handshake"
	pb "github.com/xKoRx/echo/sdk/pb/v1"
	"github.com/xKoRx/echo/sdk/telemetry"
	"github.com/xKoRx/echo/sdk/telemetry/semconv"
	"github.com/xKoRx/echo/sdk/utils"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/protobuf/proto"
)

// Router procesa mensajes de Agents.
//
// Responsabilidades:
//   - Recibir AgentMessages (TradeIntent, ExecutionResult, etc.)
//   - Validar usando SDK
//   - Dedupe
//   - Transformar TradeIntent → ExecuteOrder usando SDK
//   - Routing a Agents
//   - Telemetría
//
// Procesamiento SECUENCIAL en i0 (canal FIFO).
// TODO i1: procesamiento concurrente con locks por trade_id.
type Router struct {
	core *Core

	// Canal de procesamiento secuencial
	// TODO i0: procesamiento secuencial, i1: concurrente
	processCh chan *routerMessage

	// Issue #A2: Dedupe de command_id para idempotencia
	commandDedupe   map[string]int64 // command_id → timestamp_ms
	commandDedupeMu sync.RWMutex

	// i1: Índice de correlación para resolver slave_account_id y trade_id
	// command_id → CommandContext
	commandContext   map[string]*CommandContext
	commandContextMu sync.RWMutex

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// CommandContext contiene el contexto de un comando emitido (i1).
//
// Permite resolver slave_account_id y trade_id al recibir ExecutionResult o CloseResult.
type CommandContext struct {
	TradeID        string
	SlaveAccountID string
	CommandType    string // "execute_order" | "close_order"
	CreatedAtMs    int64
}

// routerMessage mensaje interno del router.
type routerMessage struct {
	ctx      context.Context
	agentID  string
	agentMsg *pb.AgentMessage
}

// NewRouter crea un nuevo router.
func NewRouter(core *Core) *Router {
	ctx, cancel := context.WithCancel(core.ctx)

	return &Router{
		core:             core,
		processCh:        make(chan *routerMessage, 1000), // Buffer generoso
		commandDedupe:    make(map[string]int64),          // Issue #A2
		commandDedupeMu:  sync.RWMutex{},
		commandContext:   make(map[string]*CommandContext), // i1: Índice de correlación
		commandContextMu: sync.RWMutex{},
		ctx:              ctx,
		cancel:           cancel,
	}
}

// Start inicia el router (loop de procesamiento).
func (r *Router) Start() error {
	r.wg.Add(1)
	go r.processLoop()

	r.core.telemetry.Info(r.ctx, "Router started")
	return nil
}

// Stop detiene el router.
func (r *Router) Stop() {
	r.cancel()
	close(r.processCh)
	r.wg.Wait()
	r.core.telemetry.Info(r.ctx, "Router stopped")
}

// HandleAgentMessage encola un mensaje del Agent para procesamiento.
//
// No-blocking: usa canal buffered.
func (r *Router) HandleAgentMessage(ctx context.Context, agentID string, msg *pb.AgentMessage) {
	select {
	case r.processCh <- &routerMessage{
		ctx:      ctx,
		agentID:  agentID,
		agentMsg: msg,
	}:
		// Encolado exitoso

	case <-r.ctx.Done():
		// Router detenido
		r.core.telemetry.Warn(r.ctx, "Router stopped, message dropped",
			attribute.String("agent_id", agentID),
		)

	default:
		// Canal lleno (no debería pasar con buffer de 1000)
		r.core.telemetry.Error(r.ctx, "Router queue full, message dropped", nil,
			attribute.String("agent_id", agentID),
		)
	}
}

// processLoop procesa mensajes secuencialmente (FIFO).
//
// TODO i0: procesamiento secuencial.
// TODO i1: procesamiento concurrente con locks por trade_id.
func (r *Router) processLoop() {
	defer r.wg.Done()

	for {
		select {
		case msg, ok := <-r.processCh:
			if !ok {
				return // Canal cerrado
			}

			r.processMessage(msg)

		case <-r.ctx.Done():
			return
		}
	}
}

// processMessage procesa un mensaje según su tipo.
func (r *Router) processMessage(msg *routerMessage) {
	// Extraer payload
	switch payload := msg.agentMsg.Payload.(type) {
	case *pb.AgentMessage_TradeIntent:
		r.handleTradeIntent(msg.ctx, msg.agentID, payload.TradeIntent)

	case *pb.AgentMessage_ExecutionResult:
		// i1: ExecutionResult puede venir de execute_order o close_order
		// Determinamos el tipo por el command_id (buscando en el índice)
		cmdCtx := r.getCommandContext(payload.ExecutionResult.CommandId)
		if cmdCtx != nil && cmdCtx.CommandType == "close_order" {
			r.handleCloseResult(msg.ctx, msg.agentID, payload.ExecutionResult)
		} else {
			r.handleExecutionResult(msg.ctx, msg.agentID, payload.ExecutionResult)
		}

	case *pb.AgentMessage_TradeClose:
		r.handleTradeClose(msg.ctx, msg.agentID, payload.TradeClose)

	case *pb.AgentMessage_StateSnapshot:
		r.handleStateSnapshot(msg.ctx, msg.agentID, payload.StateSnapshot)

	default:
		r.core.telemetry.Warn(r.ctx, "Unknown AgentMessage type",
			attribute.String("agent_id", msg.agentID),
		)
	}
}

// handleTradeIntent procesa un TradeIntent del Master.
//
// Flujo:
//  1. Agregar timestamp t2
//  2. Validar símbolo (whitelist)
//  3. Dedupe (rechazar duplicados)
//  4. Transformar → ExecuteOrder (lot size hardcoded)
//  5. Broadcast a todos los Agents (i0: sin routing inteligente)
//  6. Actualizar dedupe status
//  7. Métricas
func (r *Router) handleTradeIntent(ctx context.Context, agentID string, intent *pb.TradeIntent) {
	// i1: Normalizar trade_id a minúsculas DIRECTAMENTE en el protobuf (Master EA envía en mayúsculas)
	intent.TradeId = strings.ToLower(intent.TradeId)
	tradeID := intent.TradeId

	// Issue #M2: Agregar timestamp t2 (Core recibe)
	if intent.Timestamps != nil {
		intent.Timestamps.T2CoreRecvMs = utils.NowUnixMilli()
	}

	strategyID := intent.GetStrategyId()
	if strategyID == "" {
		strategyID = "default"
	}

	// Configurar contexto con atributos del evento (usando funciones del paquete)
	ctx = telemetry.AppendEventAttrs(ctx,
		semconv.Echo.TradeID.String(tradeID),
		semconv.Echo.Symbol.String(intent.Symbol),
		semconv.Echo.OrderSide.String(orderSideToString(intent.Side)),
		semconv.Echo.ClientID.String(intent.ClientId),
		semconv.Echo.Strategy.String(strategyID),
	)

	r.core.telemetry.Info(ctx, "TradeIntent received from Agent",
		attribute.String("agent_id", agentID),
		attribute.String("client_id", intent.ClientId),
		attribute.Int64("magic_number", intent.MagicNumber),
		attribute.Float64("lot_size", intent.LotSize),
		attribute.Float64("price", intent.Price),
		attribute.Int("ticket", int(intent.Ticket)),
		attribute.String("strategy_id", strategyID),
	)

	// 2. Validar símbolo canónico (i3)
	if err := r.core.canonicalValidator.Validate(ctx, intent.Symbol); err != nil {
		// unknown_action ya se aplicó en Validate (warn/reject)
		// Si es reject, el error se retorna aquí y se rechaza la orden
		return
	}

	// 3. Dedupe persistente (i1)
	if err := r.core.dedupeService.Add(ctx, tradeID, domain.OrderStatusPending); err != nil {
		if dedupeErr, ok := err.(*DedupeError); ok {
			r.core.telemetry.Warn(ctx, "Duplicate TradeIntent rejected (i1)",
				attribute.String("existing_status", dedupeErr.ExistingStatus.String()),
			)
			return
		}

		r.core.telemetry.Error(ctx, "Dedupe check failed (i1)", err,
			attribute.String("error", err.Error()),
		)
		return
	}

	// 3a. Persistir trade en BD (i1)
	attempt := int32(0)
	if intent.Attempt != nil {
		attempt = *intent.Attempt
	}

	trade := &domain.Trade{
		TradeID:         tradeID,
		SourceMasterID:  intent.ClientId,
		MasterAccountID: intent.ClientId, // TODO i2: separar master_id de account_id
		MasterTicket:    intent.Ticket,
		MagicNumber:     intent.MagicNumber,
		Symbol:          intent.Symbol,
		Side:            orderSideToDomain(intent.Side),
		LotSize:         intent.LotSize,
		Price:           intent.Price,
		StopLoss:        intent.StopLoss,
		TakeProfit:      intent.TakeProfit,
		Comment:         intent.Comment,
		Status:          domain.OrderStatusPending,
		Attempt:         attempt,
		OpenedAtMs:      intent.TimestampMs,
	}

	if err := r.core.repoFactory.TradeRepository().Create(ctx, trade); err != nil {
		r.core.telemetry.Error(ctx, "Failed to persist trade (i1)", err,
			attribute.String("error", err.Error()),
		)
		// Continuar aunque falle persistencia (no es bloqueante en i1)
	} else {
		r.core.telemetry.Info(ctx, "Trade persisted successfully (i1)",
			attribute.String("trade_id", tradeID),
		)
	}

	// 4. Transformar TradeIntent → ExecuteOrder (usando SDK)
	// i1: Pasar tradeID normalizado a createExecuteOrders
	orders := r.createExecuteOrders(ctx, intent, tradeID, strategyID)

	r.core.telemetry.Info(ctx, "ExecuteOrders created from TradeIntent",
		attribute.Int("num_orders", len(orders)),
		attribute.String("trade_id", tradeID),
	)

	// 5. Routing selectivo (i2) en lugar de broadcast
	sentCount := 0
	broadcastCount := 0
	selectiveCount := 0

	for _, order := range orders {
		// Issue #M2: Agregar timestamp t3 (Core envía)
		if order.Timestamps != nil {
			order.Timestamps.T3CoreSendMs = utils.NowUnixMilli()
		}

		msg := &pb.CoreMessage{
			Payload: &pb.CoreMessage_ExecuteOrder{ExecuteOrder: order},
		}

		// i2: Lookup owner en registry
		targetAccountID := order.TargetAccountId
		ownerAgentID, found := r.core.accountRegistry.GetOwner(targetAccountID)

		// Registrar métrica de lookup
		if found {
			r.core.echoMetrics.RecordAccountLookup(ctx, "hit",
				attribute.String("target_account_id", targetAccountID),
			)
		} else {
			r.core.echoMetrics.RecordAccountLookup(ctx, "miss",
				attribute.String("target_account_id", targetAccountID),
			)
		}

		if found {
			// Routing selectivo
			agent, agentExists := r.getAgent(ownerAgentID)
			if agentExists {
				if r.sendToAgent(ctx, agent, msg, order) {
					sentCount++
					selectiveCount++
					r.recordRoutingMetric(ctx, "selective", true, order)
				}
			} else {
				// Owner registrado pero desconectado → fallback broadcast
				r.core.telemetry.Warn(ctx, "Owner agent not connected, falling back to broadcast (i2)",
					attribute.String("target_account_id", targetAccountID),
					attribute.String("owner_agent_id", ownerAgentID),
				)
				if r.broadcastOrder(ctx, msg, order) > 0 {
					sentCount++
					broadcastCount++
					r.recordRoutingMetric(ctx, "fallback_broadcast", false, order)
				}
			}
		} else {
			// No hay owner registrado → fallback broadcast
			r.core.telemetry.Warn(ctx, "No owner registered for account, falling back to broadcast (i2)",
				attribute.String("target_account_id", targetAccountID),
			)
			if r.broadcastOrder(ctx, msg, order) > 0 {
				sentCount++
				broadcastCount++
				r.recordRoutingMetric(ctx, "fallback_broadcast", false, order)
			}
		}
	}

	// 6. Métricas
	r.core.echoMetrics.RecordOrderCreated(ctx,
		semconv.Echo.TradeID.String(tradeID),
		semconv.Echo.Symbol.String(intent.Symbol),
	)

	if sentCount > 0 {
		r.core.echoMetrics.RecordOrderSent(ctx,
			semconv.Echo.TradeID.String(tradeID),
			attribute.Int("sent_count", sentCount),
			attribute.Int("selective_count", selectiveCount),
			attribute.Int("broadcast_count", broadcastCount),
		)

		r.core.telemetry.Info(ctx, "ExecuteOrders sent (i2)",
			attribute.Int("sent_count", sentCount),
			attribute.Int("selective_count", selectiveCount),
			attribute.Int("broadcast_count", broadcastCount),
		)
	}
}

// createExecuteOrders crea ExecuteOrders a partir de un TradeIntent.
//
// TODO i0: broadcast a todos (sin routing), lot size hardcoded.
// TODO i1: routing inteligente por configuración, MM central.
func (r *Router) createExecuteOrders(ctx context.Context, intent *pb.TradeIntent, tradeID, strategyID string) []*pb.ExecuteOrder {
	orders := make([]*pb.ExecuteOrder, 0, len(r.core.config.SlaveAccounts))
	canonicalSymbol := intent.Symbol

	for _, slaveAccountID := range r.core.config.SlaveAccounts {
		handshakeStatus := r.core.handshakeRegistry.Status(slaveAccountID)
		if handshakeStatus == handshake.RegistrationStatusRejected || handshakeStatus == handshake.RegistrationStatusUnspecified {
			r.core.telemetry.Warn(ctx, "Skipping account due to handshake status",
				attribute.String("account_id", slaveAccountID),
				attribute.String("status", registrationStatusString(handshakeStatus)),
				attribute.String("trade_id", tradeID),
			)
			continue
		}
		if handshakeStatus == handshake.RegistrationStatusWarning {
			r.core.telemetry.Info(ctx, "Routing with handshake warning",
				attribute.String("account_id", slaveAccountID),
				attribute.String("trade_id", tradeID),
			)
		}

		policy, err := r.core.riskPolicyService.Get(ctx, slaveAccountID, strategyID)
		if err != nil {
			r.core.telemetry.Error(ctx, "Failed to load risk policy",
				err,
				attribute.String("trade_id", tradeID),
				attribute.String("account_id", slaveAccountID),
				attribute.String("strategy_id", strategyID),
			)
			continue
		}

		policyAttrs := []attribute.KeyValue{
			attribute.String("account_id", slaveAccountID),
			attribute.String("strategy_id", strategyID),
			attribute.String("canonical_symbol", canonicalSymbol),
		}

		if policy == nil {
			r.core.telemetry.Warn(ctx, "Risk policy missing",
				attribute.String("trade_id", tradeID),
				attribute.String("account_id", slaveAccountID),
				attribute.String("strategy_id", strategyID),
			)
			r.core.echoMetrics.RecordRiskPolicyRejected(ctx, "missing",
				policyAttrs...,
			)
			continue
		}

		ctxPolicy := telemetry.AppendEventAttrs(ctx, semconv.Echo.PolicyType.String(string(policy.Type)))
		ctxPolicy = telemetry.AppendMetricAttrs(ctxPolicy, semconv.Echo.PolicyType.String(string(policy.Type)))

		var (
			lotSize                float64
			expectedLoss           float64
			commissionTotalResult  float64
			commissionPerLotResult float64
			commissionRateResult   float64
			commissionFixedResult  float64
		)

		switch policy.Type {
		case domain.RiskPolicyTypeFixedRisk:
			policyAttrs = append(policyAttrs, attribute.String("policy_type", string(policy.Type)))
			if policy.FixedRisk == nil {
				r.core.telemetry.Warn(ctxPolicy, "Fixed risk policy missing configuration",
					attribute.String("trade_id", tradeID),
				)
				r.core.echoMetrics.RecordRiskPolicyRejected(ctxPolicy, "config_missing", policyAttrs...)
				continue
			}

			commissionPerLotFixed := 0.0
			if policy.FixedRisk.CommissionPerLot != nil && *policy.FixedRisk.CommissionPerLot > 0 {
				commissionPerLotFixed = *policy.FixedRisk.CommissionPerLot
			}
			commissionRatePercent := 0.0
			commissionRate := 0.0
			if policy.FixedRisk.CommissionRate != nil && *policy.FixedRisk.CommissionRate > 0 {
				commissionRatePercent = *policy.FixedRisk.CommissionRate
				commissionRate = commissionRatePercent / 100.0
			}
			ctxPolicy = telemetry.AppendEventAttrs(ctxPolicy,
				attribute.Float64("commission_fixed_per_lot", commissionPerLotFixed),
				attribute.Float64("commission_rate_percent", commissionRatePercent),
				semconv.Echo.RiskCommissionRate.Float64(commissionRate),
			)
			ctxPolicy = telemetry.AppendMetricAttrs(ctxPolicy,
				attribute.Float64("commission_fixed_per_lot", commissionPerLotFixed),
				attribute.Float64("commission_rate_percent", commissionRatePercent),
				semconv.Echo.RiskCommissionRate.Float64(commissionRate),
			)
			policyAttrs = append(policyAttrs,
				attribute.Float64("commission_fixed_per_lot", commissionPerLotFixed),
				attribute.Float64("commission_rate_percent", commissionRatePercent),
				semconv.Echo.RiskCommissionRate.Float64(commissionRate),
			)

			if r.core.riskEngine == nil {
				r.core.telemetry.Error(ctxPolicy, "Fixed risk engine not initialized", nil,
					attribute.String("trade_id", tradeID),
				)
				r.core.echoMetrics.RecordRiskPolicyRejected(ctxPolicy, "engine_not_available", policyAttrs...)
				continue
			}

			riskResult, err := r.core.riskEngine.ComputeLot(ctxPolicy, slaveAccountID, strategyID, canonicalSymbol, intent, policy.FixedRisk)
			if err != nil {
				r.core.telemetry.Info(ctxPolicy, "Fixed risk engine returned error",
					attribute.String("trade_id", tradeID),
					attribute.String("account_id", slaveAccountID),
					attribute.String("strategy_id", strategyID),
					attribute.String("canonical_symbol", canonicalSymbol),
					attribute.String("error", err.Error()),
				)
			}
			if err != nil {
				r.core.telemetry.Warn(ctxPolicy, "Fixed risk calculation failed",
					attribute.String("trade_id", tradeID),
					attribute.String("error", err.Error()),
				)
				continue
			}
			r.core.telemetry.Info(ctxPolicy, "Fixed risk engine decision",
				attribute.String("trade_id", tradeID),
				attribute.String("account_id", slaveAccountID),
				attribute.String("strategy_id", strategyID),
				attribute.String("canonical_symbol", canonicalSymbol),
				attribute.String("decision", string(riskResult.Decision)),
				attribute.Float64("lot", riskResult.Lot),
				attribute.Float64("expected_loss", riskResult.ExpectedLoss),
				attribute.Float64("commission_fixed_per_lot", riskResult.CommissionFixedPerLot),
				attribute.Float64("commission_rate_percent", commissionRatePercent),
				attribute.Float64("commission_rate", riskResult.CommissionRate),
				attribute.Float64("commission_per_lot", riskResult.CommissionPerLot),
				attribute.Float64("commission_total", riskResult.CommissionTotal),
				attribute.String("reason", riskResult.Reason),
			)

			if riskResult.Decision != riskengine.DecisionProceed {
				r.core.telemetry.Warn(ctxPolicy, "Fixed risk decision rejected",
					attribute.String("trade_id", tradeID),
					attribute.String("reason", riskResult.Reason),
				)
				r.core.echoMetrics.RecordRiskPolicyRejected(ctxPolicy, riskResult.Reason, policyAttrs...)
				continue
			}

			lotSize = riskResult.Lot
			expectedLoss = riskResult.ExpectedLoss
			commissionTotalResult = riskResult.CommissionTotal
			commissionPerLotResult = riskResult.CommissionPerLot
			commissionRateResult = riskResult.CommissionRate
			commissionFixedResult = riskResult.CommissionFixedPerLot
			policyAttrs = append(policyAttrs,
				attribute.Float64("commission_fixed_per_lot_result", riskResult.CommissionFixedPerLot),
				semconv.Echo.RiskCommissionPerLot.Float64(riskResult.CommissionPerLot),
				semconv.Echo.RiskCommissionTotal.Float64(riskResult.CommissionTotal),
				semconv.Echo.RiskCommissionRate.Float64(riskResult.CommissionRate),
			)
			ctxPolicy = telemetry.AppendEventAttrs(ctxPolicy,
				semconv.Echo.RiskDecision.String(string(riskResult.Decision)),
				attribute.Float64("expected_loss", expectedLoss),
				attribute.Float64("commission_fixed_per_lot_result", riskResult.CommissionFixedPerLot),
				semconv.Echo.RiskCommissionPerLot.Float64(riskResult.CommissionPerLot),
				semconv.Echo.RiskCommissionTotal.Float64(riskResult.CommissionTotal),
				semconv.Echo.RiskCommissionRate.Float64(riskResult.CommissionRate),
			)
			ctxPolicy = telemetry.AppendMetricAttrs(ctxPolicy,
				semconv.Echo.RiskDecision.String(string(riskResult.Decision)),
				attribute.Float64("commission_fixed_per_lot_result", riskResult.CommissionFixedPerLot),
				semconv.Echo.RiskCommissionPerLot.Float64(riskResult.CommissionPerLot),
				semconv.Echo.RiskCommissionTotal.Float64(riskResult.CommissionTotal),
				semconv.Echo.RiskCommissionRate.Float64(riskResult.CommissionRate),
			)

		case domain.RiskPolicyTypeFixedLot:
			if policy.FixedLot == nil || policy.FixedLot.LotSize <= 0 {
				r.core.telemetry.Warn(ctxPolicy, "Risk policy FIXED_LOT missing lot_size",
					attribute.String("trade_id", tradeID),
				)
				r.core.echoMetrics.RecordVolumeGuardDecision(ctxPolicy, string(volumeguard.DecisionReject),
					append(policyAttrs, attribute.String("reason", "risk_policy_missing"))...,
				)
				continue
			}

			requiredLot := policy.FixedLot.LotSize
			lotSize = requiredLot
			decision := volumeguard.DecisionPassThrough
			var guardErr error
			if r.core.volumeGuard != nil {
				lotSize, decision, guardErr = r.core.volumeGuard.Execute(ctxPolicy, slaveAccountID, canonicalSymbol, strategyID, requiredLot)
			}
			r.core.telemetry.Info(ctxPolicy, "Fixed lot evaluation",
				attribute.String("trade_id", tradeID),
				attribute.String("account_id", slaveAccountID),
				attribute.String("strategy_id", strategyID),
				attribute.String("canonical_symbol", canonicalSymbol),
				attribute.Float64("requested_lot", requiredLot),
				attribute.Float64("final_lot", lotSize),
				attribute.String("volume_guard_decision", string(decision)),
			)
			if guardErr != nil {
				if decision == volumeguard.DecisionReject {
					r.core.telemetry.Warn(ctxPolicy, "Volume guard rejected lot",
						attribute.String("trade_id", tradeID),
						attribute.String("account_id", slaveAccountID),
						attribute.String("strategy_id", strategyID),
						attribute.String("canonical_symbol", canonicalSymbol),
						attribute.String("error", guardErr.Error()),
					)
				} else {
					r.core.telemetry.Error(ctxPolicy, "Volume guard failed",
						guardErr,
						attribute.String("trade_id", tradeID),
						attribute.String("account_id", slaveAccountID),
						attribute.String("strategy_id", strategyID),
						attribute.String("canonical_symbol", canonicalSymbol),
					)
				}
				continue
			}
			if decision == volumeguard.DecisionReject {
				continue
			}
			r.core.echoMetrics.RecordVolumeGuardDecision(ctxPolicy, string(decision),
				append(policyAttrs, attribute.Float64("requested_lot", requiredLot), attribute.Float64("final_lot", lotSize))...,
			)

		default:
			r.core.telemetry.Warn(ctxPolicy, "Unsupported risk policy type",
				attribute.String("trade_id", tradeID),
				attribute.String("policy_type", string(policy.Type)),
			)
			r.core.echoMetrics.RecordRiskPolicyRejected(ctxPolicy, "unsupported_type", policyAttrs...)
			continue
		}

		commandID := utils.GenerateUUIDv7()
		if r.isCommandIDDuplicate(commandID) {
			r.core.telemetry.Warn(ctxPolicy, "Duplicate command_id detected, skipping",
				attribute.String("command_id", commandID),
				attribute.String("trade_id", tradeID),
			)
			continue
		}

		r.registerCommandID(commandID)
		r.registerCommandContext(commandID, tradeID, slaveAccountID, "execute_order")

		opts := &domain.TransformOptions{
			LotSize:   lotSize,
			CommandID: commandID,
			ClientID:  fmt.Sprintf("slave_%s", slaveAccountID),
			AccountID: slaveAccountID,
		}

		order := domain.TradeIntentToExecuteOrder(intent, opts)

		ctxForOrder := ctxPolicy

		brokerSymbol, info, found := r.core.symbolResolver.ResolveForAccount(ctx, slaveAccountID, canonicalSymbol)
		if !found {
			if r.core.canonicalValidator.UnknownAction() == UnknownActionReject {
				r.core.telemetry.Warn(ctxForOrder, "Symbol mapping missing, order rejected (i3)",
					attribute.String("account_id", slaveAccountID),
					attribute.String("canonical_symbol", canonicalSymbol),
				)
				continue
			}
			r.core.telemetry.Warn(ctxForOrder, "Symbol mapping missing, using canonical symbol (i3)",
				attribute.String("account_id", slaveAccountID),
				attribute.String("canonical_symbol", canonicalSymbol),
			)
		} else {
			order.Symbol = brokerSymbol
			r.core.telemetry.Debug(ctxForOrder, "Symbol mapping applied (i3)",
				attribute.String("account_id", slaveAccountID),
				attribute.String("canonical", canonicalSymbol),
				attribute.String("broker", brokerSymbol),
			)
		}

		var spec *pb.SymbolSpecification
		if r.core.symbolSpecService != nil {
			if specEntry, _, ok := r.core.symbolSpecService.GetSpecification(ctx, slaveAccountID, canonicalSymbol); ok {
				spec = specEntry
			}
		}
		if spec != nil && r.core.symbolSpecService != nil && r.core.config.VolumeGuard != nil {
			if r.core.symbolSpecService.IsStale(slaveAccountID, canonicalSymbol, r.core.config.VolumeGuard.MaxSpecAge) {
				r.core.telemetry.Warn(ctxForOrder, "Symbol specification stale for stop adjustment",
					attribute.String("account_id", slaveAccountID),
					attribute.String("canonical_symbol", canonicalSymbol),
				)
				spec = nil
			}
		}

		var quote *pb.SymbolQuoteSnapshot
		if r.core.symbolQuoteService != nil {
			if q, ok := r.core.symbolQuoteService.Get(slaveAccountID, canonicalSymbol); ok {
				quote = q
			}
		}

		if quote != nil {
			r.adjustStopsAndTargets(ctxForOrder, order, intent, quote, info, spec, slaveAccountID)
		} else {
			r.core.telemetry.Debug(ctxForOrder, "No quote snapshot available for stop adjustment",
				attribute.String("account_id", slaveAccountID),
				attribute.String("canonical_symbol", canonicalSymbol),
			)
		}

		debugAttrs := []attribute.KeyValue{
			attribute.String("command_id", commandID),
			attribute.String("trade_id", tradeID),
			attribute.String("target_client_id", order.TargetClientId),
			attribute.String("target_account_id", order.TargetAccountId),
			attribute.String("symbol", order.Symbol),
			attribute.Float64("lot_size", order.LotSize),
			attribute.String("strategy_id", strategyID),
			attribute.String("policy_type", string(policy.Type)),
		}
		if expectedLoss > 0 {
			debugAttrs = append(debugAttrs, attribute.Float64("expected_loss", expectedLoss))
		}
		if commissionTotalResult > 0 {
			debugAttrs = append(debugAttrs,
				attribute.Float64("commission_total", commissionTotalResult),
				attribute.Float64("commission_per_lot", commissionPerLotResult),
				attribute.Float64("commission_rate", commissionRateResult),
				attribute.Float64("commission_fixed_per_lot", commissionFixedResult),
			)
		}
		if commissionTotalResult > 0 {
			debugAttrs = append(debugAttrs,
				attribute.Float64("commission_total", commissionTotalResult),
				attribute.Float64("commission_per_lot", commissionPerLotResult),
				attribute.Float64("commission_rate", commissionRateResult),
			)
		}

		r.core.telemetry.Debug(ctxForOrder, "ExecuteOrder created", debugAttrs...)

		orders = append(orders, order)
	}

	if len(orders) == 0 {
		r.core.telemetry.Warn(ctx, "No ExecuteOrders generated after guard",
			attribute.String("trade_id", tradeID),
			attribute.String("strategy_id", strategyID),
		)
		if err := r.core.dedupeService.UpdateStatus(ctx, tradeID, domain.OrderStatusRejected); err != nil {
			r.core.telemetry.Error(ctx, "Failed to update dedupe status after rejection", err,
				attribute.String("trade_id", tradeID),
			)
		}
	}

	return orders
}

func (r *Router) adjustStopsAndTargets(ctx context.Context, order *pb.ExecuteOrder, intent *pb.TradeIntent, quote *pb.SymbolQuoteSnapshot, info *domain.AccountSymbolInfo, spec *pb.SymbolSpecification, accountID string) {
	if intent == nil || order == nil {
		return
	}

	// Fallback para mantener los valores originales si el ajuste no aplica
	if order.StopLoss == nil && intent.StopLoss != nil {
		origSL := intent.GetStopLoss()
		order.StopLoss = proto.Float64(origSL)
	}
	if order.TakeProfit == nil && intent.TakeProfit != nil {
		origTP := intent.GetTakeProfit()
		order.TakeProfit = proto.Float64(origTP)
	}

	if quote == nil {
		r.core.telemetry.Debug(ctx, "No quote snapshot available for stop adjustment",
			attribute.String("account_id", accountID),
			attribute.String("canonical_symbol", intent.Symbol),
		)
		return
	}

	entryPrice := quote.Ask
	if intent.Side == pb.OrderSide_ORDER_SIDE_SELL {
		entryPrice = quote.Bid
	}

	digits := 5
	if info != nil && info.Digits > 0 {
		digits = int(info.Digits)
	} else if spec != nil && spec.General != nil && spec.General.Digits > 0 {
		digits = int(spec.General.Digits)
	}

	point := 0.0
	if info != nil {
		point = info.Point
	}

	minDistance := computeMinDistance(point, spec)

	if intent.StopLoss != nil && *intent.StopLoss != 0 {
		distance := computeStopDistance(intent.Side, intent.Price, *intent.StopLoss)
		if distance <= 0 {
			// Si distance no es válida, mantener SL original
			if order.StopLoss == nil {
				origSL := intent.GetStopLoss()
				order.StopLoss = proto.Float64(origSL)
			}
			goto adjustTP
		}

		newSL := computeStopPrice(intent.Side, entryPrice, distance)
		if minDistance > 0 && math.Abs(entryPrice-newSL) < minDistance {
			if intent.Side == pb.OrderSide_ORDER_SIDE_BUY {
				newSL = entryPrice - minDistance
			} else {
				newSL = entryPrice + minDistance
			}
			r.core.telemetry.Debug(ctx, "StopLoss adjusted to satisfy stop level",
				attribute.String("account_id", accountID),
				attribute.Float64("min_distance", minDistance),
			)
		}

		rounded := roundToDigits(newSL, digits)
		order.StopLoss = proto.Float64(rounded)
	}

adjustTP:
	if intent.TakeProfit != nil && *intent.TakeProfit != 0 {
		distance := computeTakeProfitDistance(intent.Side, intent.Price, *intent.TakeProfit)
		if distance <= 0 {
			if order.TakeProfit == nil {
				origTP := intent.GetTakeProfit()
				order.TakeProfit = proto.Float64(origTP)
			}
			return
		}

		newTP := computeTakeProfitPrice(intent.Side, entryPrice, distance)
		rounded := roundToDigits(newTP, digits)
		order.TakeProfit = proto.Float64(rounded)
	}
}

func computeStopDistance(side pb.OrderSide, price, stop float64) float64 {
	if price <= 0 || stop <= 0 {
		return 0
	}
	if side == pb.OrderSide_ORDER_SIDE_BUY {
		return price - stop
	}
	return stop - price
}

func computeTakeProfitDistance(side pb.OrderSide, price, tp float64) float64 {
	if price <= 0 || tp <= 0 {
		return 0
	}
	if side == pb.OrderSide_ORDER_SIDE_BUY {
		return tp - price
	}
	return price - tp
}

func computeStopPrice(side pb.OrderSide, entry float64, distance float64) float64 {
	if side == pb.OrderSide_ORDER_SIDE_BUY {
		return entry - distance
	}
	return entry + distance
}

func computeTakeProfitPrice(side pb.OrderSide, entry float64, distance float64) float64 {
	if side == pb.OrderSide_ORDER_SIDE_BUY {
		return entry + distance
	}
	return entry - distance
}

func computeMinDistance(point float64, spec *pb.SymbolSpecification) float64 {
	if spec == nil || spec.General == nil {
		return 0
	}
	if spec.General.StopsLevel <= 0 || point <= 0 {
		return 0
	}
	return float64(spec.General.StopsLevel) * point
}

func roundToDigits(value float64, digits int) float64 {
	if digits < 0 {
		digits = 0
	}
	factor := math.Pow10(digits)
	return math.Round(value*factor) / factor
}

// handleExecutionResult procesa un ExecutionResult del Slave (i1).
//
// Flujo:
//  1. Métricas y logs
//  2. Persistir execution usando CorrelationService (i1)
//  3. Actualizar dedupe status (i1)
//  4. Calcular latencias
func (r *Router) handleExecutionResult(ctx context.Context, agentID string, result *pb.ExecutionResult) {
	// i1: Normalizar trade_id a minúsculas DIRECTAMENTE en el protobuf (EA envía en mayúsculas)
	result.TradeId = strings.ToLower(result.TradeId)

	commandID := result.CommandId
	tradeID := result.TradeId

	// Configurar contexto (usando funciones del paquete)
	ctx = telemetry.AppendEventAttrs(ctx,
		semconv.Echo.CommandID.String(commandID),
		semconv.Echo.TradeID.String(tradeID),
		semconv.Echo.Status.String(statusToString(result.Success)),
	)

	// 1. Convertir timestamps a map para persistencia (i1)
	timestampsMap := make(map[string]int64)
	if result.Timestamps != nil {
		timestampsMap["t0"] = result.Timestamps.T0MasterEaMs
		timestampsMap["t1"] = result.Timestamps.T1AgentRecvMs
		timestampsMap["t2"] = result.Timestamps.T2CoreRecvMs
		timestampsMap["t3"] = result.Timestamps.T3CoreSendMs
		timestampsMap["t4"] = result.Timestamps.T4AgentRecvMs
		timestampsMap["t5"] = result.Timestamps.T5SlaveEaRecvMs
		timestampsMap["t6"] = result.Timestamps.T6OrderSendMs
		timestampsMap["t7"] = result.Timestamps.T7OrderFilledMs
	}

	// 2. Resolver slave_account_id y trade_id desde el índice de correlación (i1)
	cmdCtx := r.getCommandContext(commandID)
	if cmdCtx == nil {
		r.core.telemetry.Warn(ctx, "CommandContext not found for ExecutionResult (i1)",
			attribute.String("command_id", commandID),
			attribute.String("trade_id_from_result", tradeID),
		)
		// Fallback: usar valores del result (pueden estar incompletos)
		// Aún así persistir para auditoría
	}

	// Usar valores del índice si existen, sino del result
	finalTradeID := tradeID
	slaveAccountID := "UNKNOWN"
	if cmdCtx != nil {
		finalTradeID = cmdCtx.TradeID
		slaveAccountID = cmdCtx.SlaveAccountID

		r.core.telemetry.Info(ctx, "CommandContext resolved successfully (i1)",
			attribute.String("command_id", commandID),
			attribute.String("trade_id", finalTradeID),
			attribute.String("slave_account_id", slaveAccountID),
		)
	}

	// Extraer error message si existe
	errMsg := ""
	if result.ErrorMessage != nil {
		errMsg = *result.ErrorMessage
	}

	// i1: Normalizar error_code según success
	errorCode := "NO_ERROR"
	if !result.Success {
		errorCode = result.ErrorCode.String()
		if errorCode == "ERROR_CODE_UNSPECIFIED" || errorCode == "" {
			errorCode = "ERR_UNKNOWN"
		}
	}

	execution := &domain.Execution{
		ExecutionID:    commandID,
		TradeID:        finalTradeID,
		SlaveAccountID: slaveAccountID,
		AgentID:        agentID,
		SlaveTicket:    result.Ticket,
		ExecutedPrice:  result.ExecutedPrice,
		Success:        result.Success,
		ErrorCode:      errorCode, // i1: Normalizado
		ErrorMessage:   errMsg,
		TimestampsMs:   timestampsMap,
	}

	// Persistir usando CorrelationService (también actualiza dedupe)
	if err := r.core.correlationSvc.RecordExecution(ctx, execution); err != nil {
		r.core.telemetry.Error(ctx, "Failed to record execution (i1)", err,
			attribute.String("error", err.Error()),
		)
		// Continuar para métricas aunque falle persistencia
	} else {
		r.core.telemetry.Info(ctx, "Execution recorded successfully (i1)",
			attribute.String("command_id", commandID),
			attribute.String("trade_id", tradeID),
			attribute.Bool("success", result.Success),
			attribute.Int("ticket", int(result.Ticket)),
		)
	}

	// 3. Log según resultado
	if result.Success {
		r.core.telemetry.Info(ctx, "Order filled successfully (i1)",
			attribute.Int("ticket", int(result.Ticket)),
		)
	} else {
		r.core.telemetry.Warn(ctx, "Order rejected by broker (i1)",
			attribute.String("error_code", result.ErrorCode.String()),
		)
	}

	// 4. Calcular latencia E2E si hay timestamps completos (Issue #C1)
	if result.Timestamps != nil && result.Timestamps.T0MasterEaMs > 0 && result.Timestamps.T7OrderFilledMs > 0 {
		latencyE2E := result.Timestamps.T7OrderFilledMs - result.Timestamps.T0MasterEaMs
		r.core.echoMetrics.RecordLatencyE2E(ctx, float64(latencyE2E),
			semconv.Echo.TradeID.String(tradeID),
			semconv.Echo.CommandID.String(commandID),
		)
		r.core.telemetry.Info(ctx, "E2E latency measured (i1)",
			attribute.Int64("latency_ms", latencyE2E),
		)
	}

	// 5. Métricas
	r.core.echoMetrics.RecordExecutionCompleted(ctx,
		semconv.Echo.CommandID.String(commandID),
		semconv.Echo.TradeID.String(tradeID),
		semconv.Echo.Status.String(statusToString(result.Success)),
		semconv.Echo.ErrorCode.String(result.ErrorCode.String()),
	)
}

// handleTradeClose procesa un TradeClose del Master.
//
// Flujo:
//  1. Crear CloseOrders (uno por slave configurado)
//  2. Broadcast a Agents
//  3. Métricas
//
// Issue #C3: Core llena campos target_* correctamente.
func (r *Router) handleTradeClose(ctx context.Context, agentID string, close *pb.TradeClose) {
	// i1: Normalizar trade_id a minúsculas DIRECTAMENTE en el protobuf (Master EA envía en mayúsculas)
	close.TradeId = strings.ToLower(close.TradeId)
	tradeID := close.TradeId

	// Configurar contexto (usando funciones del paquete)
	ctx = telemetry.AppendEventAttrs(ctx,
		semconv.Echo.TradeID.String(tradeID),
	)

	r.core.telemetry.Info(ctx, "TradeClose received",
		attribute.Int("ticket", int(close.Ticket)),
		attribute.Float64("close_price", close.ClosePrice),
		attribute.String("symbol", close.Symbol),
		attribute.Int64("magic_number", close.MagicNumber),
	)

	// Obtener tickets por slave usando CorrelationService (i1)
	ticketsBySlave, err := r.core.correlationSvc.GetTicketsByTrade(ctx, tradeID)
	if err != nil {
		r.core.telemetry.Error(ctx, "Failed to get tickets for trade (i1)", err,
			attribute.String("error", err.Error()),
		)
		// Continuar con ticket=0 (fallback a i0 behavior)
		ticketsBySlave = make(map[string]int32)
	}

	r.core.telemetry.Info(ctx, "Tickets resolved for trade (i1)",
		attribute.String("trade_id", tradeID),
		attribute.Int("tickets_count", len(ticketsBySlave)),
	)

	// Issue #C3: Crear CloseOrder por cada slave configurado (i1 con ticket exacto)
	totalSent := 0

	for _, slaveAccountID := range r.core.config.SlaveAccounts {
		closeOrderID := utils.GenerateUUIDv7()

		// i1: Registrar contexto del CloseOrder para correlación
		r.registerCommandContext(closeOrderID, tradeID, slaveAccountID, "close_order")

		// Resolver ticket exacto del slave (i1 - RFC-003)
		ticket := ticketsBySlave[slaveAccountID]
		if ticket == 0 {
			r.core.telemetry.Warn(ctx, "No ticket found for slave (i1), using magic+symbol fallback",
				attribute.String("slave_account_id", slaveAccountID),
				attribute.String("trade_id", tradeID),
			)
			// Continuar con ticket=0 (fallback a búsqueda por magic+symbol en slave)
		}

		// i3: Traducir símbolo canónico a broker_symbol por cuenta
		canonicalSymbol := close.Symbol
		brokerSymbol, _, found := r.core.symbolResolver.ResolveForAccount(ctx, slaveAccountID, canonicalSymbol)
		symbolToUse := canonicalSymbol // Fallback a canonical si no hay mapeo
		if found {
			symbolToUse = brokerSymbol
			r.core.telemetry.Debug(ctx, "Symbol mapping applied for CloseOrder (i3)",
				attribute.String("account_id", slaveAccountID),
				attribute.String("canonical", canonicalSymbol),
				attribute.String("broker", brokerSymbol),
			)
		} else {
			// No hay mapeo - aplicar política (pero no rechazar cierre, solo warn)
			r.core.telemetry.Warn(ctx, "Symbol mapping missing for CloseOrder, using canonical (i3)",
				attribute.String("account_id", slaveAccountID),
				attribute.String("canonical_symbol", canonicalSymbol),
			)
		}

		closeOrder := &pb.CloseOrder{
			CommandId:   closeOrderID,
			TradeId:     tradeID,
			TimestampMs: utils.NowUnixMilli(),
			// i1: Ticket EXACTO del slave (no 0 si se encontró en BD)
			Ticket: ticket,
			// Issue #C3: Llenar campos target_* con datos del TradeClose
			TargetClientId:  fmt.Sprintf("slave_%s", slaveAccountID),
			TargetAccountId: slaveAccountID,
			Symbol:          symbolToUse, // i3: Traducido a broker_symbol si existe mapeo
			MagicNumber:     close.MagicNumber,
			// Inicializar timestamps para permitir que el Agent agregue t4
			Timestamps: &pb.TimestampMetadata{},
		}

		// Registrar timestamp t3 (Core send) en CloseOrder
		if closeOrder.Timestamps != nil {
			closeOrder.Timestamps.T3CoreSendMs = utils.NowUnixMilli()
		}

		// i2: Routing selectivo para CloseOrder
		msg := &pb.CoreMessage{
			Payload: &pb.CoreMessage_CloseOrder{CloseOrder: closeOrder},
		}

		ownerAgentID, found := r.core.accountRegistry.GetOwner(slaveAccountID)

		if found {
			// Routing selectivo
			agent, agentExists := r.getAgent(ownerAgentID)
			if agentExists {
				select {
				case agent.SendCh <- msg:
					totalSent++
					r.core.telemetry.Info(ctx, "CloseOrder sent to Agent (selective i2)",
						attribute.String("close_order_id", closeOrderID),
						attribute.String("agent_id", ownerAgentID),
						attribute.String("target_account_id", slaveAccountID),
						attribute.String("symbol", close.Symbol),
						attribute.Int64("magic_number", close.MagicNumber),
					)

				case <-ctx.Done():
					r.core.telemetry.Error(ctx, "Context cancelled while sending CloseOrder", ctx.Err(),
						attribute.String("close_order_id", closeOrderID),
						attribute.String("target_account_id", slaveAccountID),
					)
					return
				}
			} else {
				// Owner registrado pero desconectado → fallback broadcast
				r.core.telemetry.Warn(ctx, "Owner agent not connected for CloseOrder, falling back to broadcast (i2)",
					attribute.String("target_account_id", slaveAccountID),
					attribute.String("owner_agent_id", ownerAgentID),
				)
				r.broadcastCloseOrder(ctx, msg, closeOrder, slaveAccountID)
				totalSent++
			}
		} else {
			// No hay owner registrado → fallback broadcast
			r.core.telemetry.Warn(ctx, "No owner registered for account in CloseOrder, falling back to broadcast (i2)",
				attribute.String("target_account_id", slaveAccountID),
			)
			r.broadcastCloseOrder(ctx, msg, closeOrder, slaveAccountID)
			totalSent++
		}
	}

	r.core.telemetry.Info(ctx, "All CloseOrders sent (i2)",
		attribute.String("trade_id", tradeID),
		attribute.Int("total_slaves", len(r.core.config.SlaveAccounts)),
		attribute.Int("total_sent", totalSent),
	)
}

// handleCloseResult procesa un CloseResult del Slave (i1).
//
// Flujo:
//  1. Resolver slave_account_id y trade_id desde índice
//  2. Persistir close en echo.closes usando CorrelationService
//  3. Limpiar command_id del índice
//  4. Métricas y logs
func (r *Router) handleCloseResult(ctx context.Context, agentID string, result *pb.ExecutionResult) {
	// i1: Normalizar trade_id a minúsculas DIRECTAMENTE en el protobuf (EA envía en mayúsculas)
	result.TradeId = strings.ToLower(result.TradeId)

	commandID := result.CommandId
	tradeID := result.TradeId

	// Configurar contexto (usando funciones del paquete)
	ctx = telemetry.AppendEventAttrs(ctx,
		semconv.Echo.CommandID.String(commandID),
		semconv.Echo.TradeID.String(tradeID),
		semconv.Echo.Status.String(statusToString(result.Success)),
	)

	// 1. Resolver slave_account_id y trade_id desde el índice de correlación (i1)
	cmdCtx := r.getCommandContext(commandID)
	if cmdCtx == nil {
		r.core.telemetry.Warn(ctx, "CommandContext not found for CloseResult (i1)",
			attribute.String("command_id", commandID),
			attribute.String("trade_id_from_result", tradeID),
		)
		// Sin contexto no podemos persistir correctamente; log y return
		return
	}

	finalTradeID := cmdCtx.TradeID
	slaveAccountID := cmdCtx.SlaveAccountID

	closePrice := 0.0
	if result.ExecutedPrice != nil {
		closePrice = *result.ExecutedPrice
	}

	r.core.telemetry.Info(ctx, "CloseResult received (i1)",
		attribute.String("command_id", commandID),
		attribute.String("trade_id", finalTradeID),
		attribute.String("slave_account_id", slaveAccountID),
		attribute.Bool("success", result.Success),
		attribute.Int("ticket", int(result.Ticket)),
		attribute.Float64("close_price", closePrice),
	)

	// 2. Extraer error message si existe
	errMsg := ""
	if result.ErrorMessage != nil {
		errMsg = *result.ErrorMessage
	}

	// i1: Normalizar error_code según success
	errorCode := "NO_ERROR"
	if !result.Success {
		errorCode = result.ErrorCode.String()
		if errorCode == "ERROR_CODE_UNSPECIFIED" || errorCode == "" {
			errorCode = "ERR_UNKNOWN"
		}
	}

	// 3. Persistir close usando CorrelationService (i1)
	close := &domain.Close{
		CloseID:        commandID,
		TradeID:        finalTradeID,
		SlaveAccountID: slaveAccountID,
		SlaveTicket:    result.Ticket,
		ClosePrice:     &closePrice, // puntero para compatibilidad con optional field
		Success:        result.Success,
		ErrorCode:      errorCode,
		ErrorMessage:   errMsg,
		ClosedAtMs:     utils.NowUnixMilli(),
	}

	if err := r.core.correlationSvc.RecordClose(ctx, close); err != nil {
		r.core.telemetry.Error(ctx, "Failed to record close (i1)", err,
			attribute.String("error", err.Error()),
		)
		// Continuar aunque falle persistencia (no es bloqueante)
	} else {
		r.core.telemetry.Info(ctx, "Close recorded successfully (i1)",
			attribute.String("close_id", commandID),
			attribute.String("trade_id", finalTradeID),
			attribute.String("slave_account_id", slaveAccountID),
			attribute.Bool("success", result.Success),
		)
	}

	// 4. Limpiar command_id del índice (i1 cleanup)
	r.deleteCommandContext(commandID)

	// 5. Log según resultado
	if result.Success {
		r.core.telemetry.Info(ctx, "Order closed successfully (i1)",
			attribute.Int("ticket", int(result.Ticket)),
			attribute.Float64("close_price", closePrice),
		)
	} else {
		r.core.telemetry.Warn(ctx, "Order close rejected by broker (i1)",
			attribute.String("error_code", errorCode),
			attribute.String("error_message", errMsg),
		)
	}

	// 6. Métricas
	r.core.echoMetrics.RecordExecutionCompleted(ctx,
		semconv.Echo.CommandID.String(commandID),
		semconv.Echo.TradeID.String(finalTradeID),
		semconv.Echo.Status.String(statusToString(result.Success)),
		semconv.Echo.ErrorCode.String(errorCode),
	)
}

// handleStateSnapshot procesa un StateSnapshot del Slave (i1).
//
// Flujo:
//  1. Persistir el estado del Slave en el registro de estado (i1)
//  2. Actualizar el estado del Slave en el registro de estado (i1)
//  3. Métricas
func (r *Router) handleStateSnapshot(ctx context.Context, agentID string, snapshot *pb.StateSnapshot) {
	if snapshot == nil {
		return
	}

	r.core.telemetry.Info(ctx, "StateSnapshot received from Agent",
		attribute.String("agent_id", agentID),
		attribute.Int("accounts", len(snapshot.Accounts)),
		attribute.Int("positions", len(snapshot.Positions)),
		attribute.Int64("timestamp_ms", snapshot.TimestampMs),
	)

	r.core.accountStateService.Update(ctx, agentID, snapshot)
}

// Helper: orderSideToString convierte OrderSide a string.
func orderSideToString(side pb.OrderSide) string {
	switch side {
	case pb.OrderSide_ORDER_SIDE_BUY:
		return semconv.OrderSideValues.Buy
	case pb.OrderSide_ORDER_SIDE_SELL:
		return semconv.OrderSideValues.Sell
	default:
		return "UNKNOWN"
	}
}

// Helper: statusToString convierte success bool a string.
func statusToString(success bool) string {
	if success {
		return semconv.StatusValues.Success
	}
	return semconv.StatusValues.Rejected
}

// isCommandIDDuplicate verifica si un command_id ya fue procesado.
//
// Issue #A2: Dedupe de command_id para prevenir ejecución duplicada.
func (r *Router) isCommandIDDuplicate(commandID string) bool {
	r.commandDedupeMu.RLock()
	defer r.commandDedupeMu.RUnlock()

	_, exists := r.commandDedupe[commandID]
	return exists
}

// registerCommandID registra un command_id como procesado.
//
// Issue #A2: Dedupe de command_id.
// TODO i1: Implementar TTL y limpieza periódica del mapa (ej: 1 hora).
func (r *Router) registerCommandID(commandID string) {
	r.commandDedupeMu.Lock()
	defer r.commandDedupeMu.Unlock()

	r.commandDedupe[commandID] = utils.NowUnixMilli()

	// TODO i1: Cleanup periódico
	// Si el mapa crece mucho (>10k entries), limpiar entries antiguas (>1h)
	// por ahora en i0 el mapa se mantiene en memoria hasta restart
}

// registerCommandContext registra el contexto de un comando emitido (i1).
//
// Permite resolver slave_account_id y trade_id al recibir ExecutionResult o CloseResult.
func (r *Router) registerCommandContext(commandID, tradeID, slaveAccountID, commandType string) {
	r.commandContextMu.Lock()
	defer r.commandContextMu.Unlock()

	r.commandContext[commandID] = &CommandContext{
		TradeID:        tradeID,
		SlaveAccountID: slaveAccountID,
		CommandType:    commandType,
		CreatedAtMs:    utils.NowUnixMilli(),
	}
}

// getCommandContext obtiene el contexto de un comando (i1).
//
// Retorna nil si no existe (comando desconocido o ya limpiado).
func (r *Router) getCommandContext(commandID string) *CommandContext {
	r.commandContextMu.RLock()
	defer r.commandContextMu.RUnlock()

	return r.commandContext[commandID]
}

// deleteCommandContext elimina el contexto de un comando (i1).
//
// Se llama al cerrar una orden para liberar memoria.
func (r *Router) deleteCommandContext(commandID string) {
	r.commandContextMu.Lock()
	defer r.commandContextMu.Unlock()

	delete(r.commandContext, commandID)
}

// orderSideToDomain convierte pb.OrderSide a domain.OrderSide (i1).
func orderSideToDomain(side pb.OrderSide) domain.OrderSide {
	switch side {
	case pb.OrderSide_ORDER_SIDE_BUY:
		return domain.OrderSideBuy
	case pb.OrderSide_ORDER_SIDE_SELL:
		return domain.OrderSideSell
	default:
		return "" // TODO: mejor manejo de error
	}
}

// getAgent retorna un Agent por ID (i2 helper).
func (r *Router) getAgent(agentID string) (*AgentConnection, bool) {
	return r.core.GetAgent(agentID)
}

// sendToAgent envía un mensaje a un Agent específico (i2b helper con timeout).
//
// Retorna true si el envío fue exitoso.
func (r *Router) sendToAgent(ctx context.Context, agent *AgentConnection, msg *pb.CoreMessage, order *pb.ExecuteOrder) bool {
	// i2b: Timeout de 500ms para evitar bloqueos indefinidos
	timeout := time.NewTimer(500 * time.Millisecond)
	defer timeout.Stop()

	select {
	case agent.SendCh <- msg:
		// Envío exitoso - logging reducido para hot path
		r.core.telemetry.Debug(ctx, "ExecuteOrder sent to Agent (selective i2b)",
			attribute.String("agent_id", agent.AgentID),
			attribute.String("command_id", order.CommandId),
			attribute.String("trade_id", order.TradeId),
			attribute.String("target_account_id", order.TargetAccountId),
		)
		return true

	case <-timeout.C:
		// i2b: Canal lleno o Agent lento - registrar warning y fallar
		r.core.telemetry.Warn(ctx, "Timeout sending to Agent, channel may be full (i2b)",
			attribute.String("agent_id", agent.AgentID),
			attribute.String("command_id", order.CommandId),
			attribute.String("target_account_id", order.TargetAccountId),
		)
		return false

	case <-ctx.Done():
		r.core.telemetry.Error(ctx, "Context cancelled while sending ExecuteOrder", ctx.Err(),
			attribute.String("agent_id", agent.AgentID),
			attribute.String("command_id", order.CommandId),
		)
		return false
	}
}

// broadcastOrder envía una orden a todos los Agents (i2b fallback con timeout).
//
// Retorna el número de Agents que recibieron el mensaje.
func (r *Router) broadcastOrder(ctx context.Context, msg *pb.CoreMessage, order *pb.ExecuteOrder) int {
	agents := r.core.GetAgents()
	if len(agents) == 0 {
		r.core.telemetry.Warn(ctx, "No agents connected, broadcast failed (i2b)")
		return 0
	}

	sentCount := 0
	timeoutCount := 0

	for _, agent := range agents {
		// i2b: Timeout de 500ms por Agent para evitar bloqueos acumulados
		timeout := time.NewTimer(500 * time.Millisecond)

		select {
		case agent.SendCh <- msg:
			sentCount++
			timeout.Stop()
			// i2b: Logging reducido - solo debug level en hot path

		case <-timeout.C:
			// i2b: Timeout en este Agent - continuar con los demás
			timeoutCount++
			r.core.telemetry.Warn(ctx, "Timeout broadcasting to Agent (i2b)",
				attribute.String("agent_id", agent.AgentID),
				attribute.String("command_id", order.CommandId),
			)

		case <-ctx.Done():
			timeout.Stop()
			r.core.telemetry.Error(ctx, "Context cancelled during broadcast", ctx.Err(),
				attribute.String("agent_id", agent.AgentID),
			)
			// Continuar con otros agents
		}
	}

	// i2b: Log consolidado después del broadcast (reducir logging en hot path)
	if sentCount > 0 || timeoutCount > 0 {
		r.core.telemetry.Info(ctx, "Broadcast completed (fallback i2b)",
			attribute.String("command_id", order.CommandId),
			attribute.String("trade_id", order.TradeId),
			attribute.String("target_account_id", order.TargetAccountId),
			attribute.Int("sent_count", sentCount),
			attribute.Int("timeout_count", timeoutCount),
		)
	}

	return sentCount
}

// recordRoutingMetric registra métrica de routing (i2).
//
// mode: "selective" | "broadcast" | "fallback_broadcast"
// result: true para "hit", false para "miss"
func (r *Router) recordRoutingMetric(ctx context.Context, mode string, result bool, order *pb.ExecuteOrder) {
	resultStr := "miss"
	if result {
		resultStr = "hit"
	}

	r.core.echoMetrics.RecordRoutingMode(ctx, mode, resultStr,
		attribute.String("target_account_id", order.TargetAccountId),
		attribute.String("trade_id", order.TradeId),
	)
}

// broadcastCloseOrder envía un CloseOrder a todos los Agents (i2b fallback con timeout).
func (r *Router) broadcastCloseOrder(ctx context.Context, msg *pb.CoreMessage, order *pb.CloseOrder, slaveAccountID string) {
	agents := r.core.GetAgents()
	if len(agents) == 0 {
		r.core.telemetry.Warn(ctx, "No agents connected, CloseOrder broadcast failed (i2b)")
		return
	}

	sentCount := 0
	timeoutCount := 0

	for _, agent := range agents {
		// i2b: Timeout de 500ms por Agent para evitar bloqueos acumulados
		timeout := time.NewTimer(500 * time.Millisecond)

		select {
		case agent.SendCh <- msg:
			sentCount++
			timeout.Stop()
			// i2b: Logging reducido - solo debug level en hot path

		case <-timeout.C:
			// i2b: Timeout en este Agent - continuar con los demás
			timeoutCount++
			r.core.telemetry.Warn(ctx, "Timeout broadcasting CloseOrder to Agent (i2b)",
				attribute.String("agent_id", agent.AgentID),
				attribute.String("command_id", order.CommandId),
			)

		case <-ctx.Done():
			timeout.Stop()
			r.core.telemetry.Error(ctx, "Context cancelled during CloseOrder broadcast", ctx.Err(),
				attribute.String("agent_id", agent.AgentID),
			)
			// Continuar con otros agents
		}
	}

	// i2b: Log consolidado después del broadcast (reducir logging en hot path)
	if sentCount > 0 || timeoutCount > 0 {
		r.core.telemetry.Info(ctx, "CloseOrder broadcast completed (fallback i2b)",
			attribute.String("command_id", order.CommandId),
			attribute.String("trade_id", order.TradeId),
			attribute.String("target_account_id", slaveAccountID),
			attribute.Int("sent_count", sentCount),
			attribute.Int("timeout_count", timeoutCount),
		)
	}
}
