package internal

import (
	"context"
	"fmt"
	"sync"

	"github.com/xKoRx/echo/sdk/domain"
	pb "github.com/xKoRx/echo/sdk/pb/v1"
	"github.com/xKoRx/echo/sdk/telemetry"
	"github.com/xKoRx/echo/sdk/telemetry/semconv"
	"github.com/xKoRx/echo/sdk/utils"
	"go.opentelemetry.io/otel/attribute"
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

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
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
		core:            core,
		processCh:       make(chan *routerMessage, 1000), // Buffer generoso
		commandDedupe:   make(map[string]int64),          // Issue #A2
		commandDedupeMu: sync.RWMutex{},
		ctx:             ctx,
		cancel:          cancel,
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
		r.handleExecutionResult(msg.ctx, msg.agentID, payload.ExecutionResult)

	case *pb.AgentMessage_TradeClose:
		r.handleTradeClose(msg.ctx, msg.agentID, payload.TradeClose)

	// TODO i0: CloseResult se reporta también con ExecutionResult
	// TODO i1: implementar mensaje específico CloseResult si necesario

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
	// Issue #M2: Agregar timestamp t2 (Core recibe)
	if intent.Timestamps != nil {
		intent.Timestamps.T2CoreRecvMs = utils.NowUnixMilli()
	}

	tradeID := intent.TradeId

	// Configurar contexto con atributos del evento (usando funciones del paquete)
	ctx = telemetry.AppendEventAttrs(ctx,
		semconv.Echo.TradeID.String(tradeID),
		semconv.Echo.Symbol.String(intent.Symbol),
		semconv.Echo.OrderSide.String(orderSideToString(intent.Side)),
		semconv.Echo.ClientID.String(intent.ClientId),
	)

	r.core.telemetry.Info(ctx, "TradeIntent received from Agent",
		attribute.String("agent_id", agentID),
		attribute.String("client_id", intent.ClientId),
		attribute.Int64("magic_number", intent.MagicNumber),
		attribute.Float64("lot_size", intent.LotSize),
		attribute.Float64("price", intent.Price),
		attribute.Int("ticket", int(intent.Ticket)),
	)

	// 2. Validar símbolo (usando SDK)
	if err := domain.ValidateSymbol(intent.Symbol, r.core.config.SymbolWhitelist); err != nil {
		r.core.telemetry.Warn(ctx, "Invalid symbol, TradeIntent rejected",
			attribute.String("error", err.Error()),
		)
		// TODO i1: enviar rechazo al Agent
		return
	}

	// 3. Dedupe
	if err := r.core.dedupe.Add(tradeID, intent.ClientId, intent.Symbol, pb.OrderStatus_ORDER_STATUS_PENDING); err != nil {
		if dedupeErr, ok := err.(*DedupeError); ok {
			r.core.telemetry.Warn(ctx, "Duplicate TradeIntent rejected",
				attribute.String("existing_status", dedupeErr.ExistingStatus.String()),
			)
			return
		}

		r.core.telemetry.Error(ctx, "Dedupe check failed", err,
			attribute.String("error", err.Error()),
		)
		return
	}

	// 4. Transformar TradeIntent → ExecuteOrder (usando SDK)
	orders := r.createExecuteOrders(ctx, intent)

	r.core.telemetry.Info(ctx, "ExecuteOrders created from TradeIntent",
		attribute.Int("num_orders", len(orders)),
		attribute.String("trade_id", tradeID),
	)

	// 5. Broadcast a todos los Agents
	// TODO i0: broadcast simple, i1: routing inteligente por config
	agents := r.core.GetAgents()
	if len(agents) == 0 {
		r.core.telemetry.Warn(ctx, "No agents connected, ExecuteOrder not sent")
		r.core.dedupe.UpdateStatus(tradeID, pb.OrderStatus_ORDER_STATUS_REJECTED)
		return
	}

	sentCount := 0
	for _, agent := range agents {
		for _, order := range orders {
			// Issue #M2: Agregar timestamp t3 (Core envía)
			if order.Timestamps != nil {
				order.Timestamps.T3CoreSendMs = utils.NowUnixMilli()
			}

			// Issue #C8: Blocking send con timeout (no usar default que pierde mensajes)
			msg := &pb.CoreMessage{
				Payload: &pb.CoreMessage_ExecuteOrder{ExecuteOrder: order},
			}

			select {
			case agent.SendCh <- msg:
				sentCount++
				r.core.telemetry.Info(ctx, "ExecuteOrder sent to Agent",
					attribute.String("agent_id", agent.AgentID),
					attribute.String("command_id", order.CommandId),
					attribute.String("trade_id", order.TradeId),
					attribute.String("target_account_id", order.TargetAccountId),
					attribute.String("symbol", order.Symbol),
					attribute.String("side", order.Side.String()),
					attribute.Float64("lot_size", order.LotSize),
				)

			case <-ctx.Done():
				r.core.telemetry.Error(ctx, "Context cancelled while sending ExecuteOrder", ctx.Err(),
					attribute.String("agent_id", agent.AgentID),
					attribute.String("command_id", order.CommandId),
				)
				// Continuar intentando con otros agents/orders

				// TODO i1: agregar timeout configurable (2-5 segundos)
				// case <-time.After(2 * time.Second):
				//     r.core.telemetry.Error(ctx, "Timeout sending ExecuteOrder", nil, ...)
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
		)

		r.core.telemetry.Info(ctx, "ExecuteOrders sent to agents",
			attribute.Int("sent_count", sentCount),
			attribute.Int("total_agents", len(agents)),
		)
	}
}

// createExecuteOrders crea ExecuteOrders a partir de un TradeIntent.
//
// TODO i0: broadcast a todos (sin routing), lot size hardcoded.
// TODO i1: routing inteligente por configuración, MM central.
func (r *Router) createExecuteOrders(ctx context.Context, intent *pb.TradeIntent) []*pb.ExecuteOrder {
	// Issue #C5: Crear 1 ExecuteOrder por cada Slave configurado
	orders := []*pb.ExecuteOrder{}

	for _, slaveAccountID := range r.core.config.SlaveAccounts {
		// Generar command_id único por slave
		commandID := utils.GenerateUUIDv7()

		// Issue #A2: Verificar dedupe de command_id antes de crear la orden
		if r.isCommandIDDuplicate(commandID) {
			r.core.telemetry.Warn(ctx, "Duplicate command_id detected, skipping",
				attribute.String("command_id", commandID),
				attribute.String("trade_id", intent.TradeId),
			)
			continue
		}

		// Registrar command_id en dedupe
		r.registerCommandID(commandID)

		// Opciones de transformación con target slave
		opts := &domain.TransformOptions{
			LotSize:   r.core.config.DefaultLotSize, // TODO i0: hardcoded 0.10
			CommandID: commandID,
			ClientID:  fmt.Sprintf("slave_%s", slaveAccountID), // Issue #C5
			AccountID: slaveAccountID,                          // Issue #C5
		}

		// Transformar usando SDK (propaga timestamps automáticamente)
		order := domain.TradeIntentToExecuteOrder(intent, opts)

		r.core.telemetry.Debug(ctx, "ExecuteOrder created",
			attribute.String("command_id", commandID),
			attribute.String("trade_id", intent.TradeId),
			attribute.String("target_client_id", order.TargetClientId),
			attribute.String("target_account_id", order.TargetAccountId),
			attribute.Float64("lot_size", order.LotSize),
		)

		orders = append(orders, order)
	}

	return orders
}

// handleExecutionResult procesa un ExecutionResult del Slave.
//
// Flujo:
//  1. Métricas y logs
//  2. TODO i1: actualizar dedupe con trade_id del command_id
//  3. TODO i1: calcular latencias
//  4. TODO i1: persistir en Postgres
func (r *Router) handleExecutionResult(ctx context.Context, agentID string, result *pb.ExecutionResult) {
	commandID := result.CommandId
	tradeID := result.TradeId // Issue #C2 resuelto

	// Configurar contexto (usando funciones del paquete)
	ctx = telemetry.AppendEventAttrs(ctx,
		semconv.Echo.CommandID.String(commandID),
		semconv.Echo.TradeID.String(tradeID),
		semconv.Echo.Status.String(statusToString(result.Success)),
	)

	// Issue #C6: Actualizar dedupe status según resultado
	if result.Success {
		r.core.dedupe.UpdateStatus(tradeID, pb.OrderStatus_ORDER_STATUS_FILLED)
		r.core.telemetry.Info(ctx, "Order filled successfully",
			attribute.Int("ticket", int(result.Ticket)),
		)
	} else {
		r.core.dedupe.UpdateStatus(tradeID, pb.OrderStatus_ORDER_STATUS_REJECTED)
		r.core.telemetry.Warn(ctx, "Order rejected by broker",
			attribute.String("error_code", result.ErrorCode.String()),
		)
	}

	// Calcular latencia E2E si hay timestamps completos (Issue #C1)
	if result.Timestamps != nil && result.Timestamps.T0MasterEaMs > 0 && result.Timestamps.T7OrderFilledMs > 0 {
		latencyE2E := result.Timestamps.T7OrderFilledMs - result.Timestamps.T0MasterEaMs
		r.core.echoMetrics.RecordLatencyE2E(ctx, float64(latencyE2E),
			semconv.Echo.TradeID.String(tradeID),
			semconv.Echo.CommandID.String(commandID),
		)
		r.core.telemetry.Info(ctx, "E2E latency measured",
			attribute.Int64("latency_ms", latencyE2E),
		)
	}

	// Métricas
	r.core.echoMetrics.RecordExecutionCompleted(ctx,
		semconv.Echo.CommandID.String(commandID),
		semconv.Echo.TradeID.String(tradeID),
		semconv.Echo.Status.String(statusToString(result.Success)),
		semconv.Echo.ErrorCode.String(result.ErrorCode.String()),
	)

	// TODO i1: persistir en Postgres
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

	// Issue #C3: Crear CloseOrder por cada slave configurado
	// Opción A (i0): Broadcast a todos los slaves
	// TODO i1: Opción B - routing inteligente (solo a slaves que ejecutaron el TradeIntent original)
	agents := r.core.GetAgents()
	totalSent := 0

	for _, slaveAccountID := range r.core.config.SlaveAccounts {
		closeOrderID := utils.GenerateUUIDv7()
		closeOrder := &pb.CloseOrder{
			CommandId:   closeOrderID,
			TradeId:     tradeID,
			TimestampMs: utils.NowUnixMilli(),
			// En i0 no conocemos el ticket del slave; forzamos búsqueda por magic+symbol
			Ticket: 0,
			// Issue #C3: Llenar campos target_* con datos del TradeClose
			TargetClientId:  fmt.Sprintf("slave_%s", slaveAccountID),
			TargetAccountId: slaveAccountID,
			Symbol:          close.Symbol,
			MagicNumber:     close.MagicNumber,
			// Inicializar timestamps para permitir que el Agent agregue t4
			Timestamps: &pb.TimestampMetadata{},
		}

		// Registrar timestamp t3 (Core send) en CloseOrder
		if closeOrder.Timestamps != nil {
			closeOrder.Timestamps.T3CoreSendMs = utils.NowUnixMilli()
		}

		// Broadcast a todos los Agents
		sentCount := 0
		for _, agent := range agents {
			select {
			case agent.SendCh <- &pb.CoreMessage{
				Payload: &pb.CoreMessage_CloseOrder{CloseOrder: closeOrder},
			}:
				sentCount++

			case <-ctx.Done():
				r.core.telemetry.Error(ctx, "Context cancelled while sending CloseOrder", nil,
					attribute.String("close_order_id", closeOrderID),
					attribute.String("target_account_id", slaveAccountID),
				)
				return

				// TODO i1: agregar timeout configurable (2-5 segundos)
				// case <-time.After(2 * time.Second):
				//     r.core.telemetry.Error(ctx, "Timeout sending CloseOrder", nil, ...)
			}
		}

		totalSent += sentCount

		r.core.telemetry.Info(ctx, "CloseOrder sent to agents",
			attribute.String("close_order_id", closeOrderID),
			attribute.String("target_account_id", slaveAccountID),
			attribute.String("symbol", close.Symbol),
			attribute.Int64("magic_number", close.MagicNumber),
			attribute.Int("sent_count", sentCount),
		)
	}

	r.core.telemetry.Info(ctx, "All CloseOrders broadcasted",
		attribute.String("trade_id", tradeID),
		attribute.Int("total_slaves", len(r.core.config.SlaveAccounts)),
		attribute.Int("total_sent", totalSent),
	)
}

// TODO i0: handleCloseResult no existe en i0
// Los resultados de cierre se reportan con ExecutionResult igual que las aperturas

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
