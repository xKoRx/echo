package internal

import (
	"context"
	"fmt"
	"strings"
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
	orders := r.createExecuteOrders(ctx, intent, tradeID)

	r.core.telemetry.Info(ctx, "ExecuteOrders created from TradeIntent",
		attribute.Int("num_orders", len(orders)),
		attribute.String("trade_id", tradeID),
	)

	// 5. Broadcast a todos los Agents
	// TODO i0: broadcast simple, i1: routing inteligente por config
	agents := r.core.GetAgents()
	if len(agents) == 0 {
		r.core.telemetry.Warn(ctx, "No agents connected, ExecuteOrder not sent")
		// i1: Actualizar dedupe status a REJECTED
		_ = r.core.dedupeService.UpdateStatus(ctx, tradeID, domain.OrderStatusRejected)
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
func (r *Router) createExecuteOrders(ctx context.Context, intent *pb.TradeIntent, tradeID string) []*pb.ExecuteOrder {
	// Issue #C5: Crear 1 ExecuteOrder por cada Slave configurado
	orders := []*pb.ExecuteOrder{}

	for _, slaveAccountID := range r.core.config.SlaveAccounts {
		// Generar command_id único por slave
		commandID := utils.GenerateUUIDv7()

		// Issue #A2: Verificar dedupe de command_id antes de crear la orden
		if r.isCommandIDDuplicate(commandID) {
			r.core.telemetry.Warn(ctx, "Duplicate command_id detected, skipping",
				attribute.String("command_id", commandID),
				attribute.String("trade_id", tradeID),
			)
			continue
		}

		// Registrar command_id en dedupe
		r.registerCommandID(commandID)

		// i1: Registrar contexto del comando para correlación (usando tradeID normalizado)
		r.registerCommandContext(commandID, tradeID, slaveAccountID, "execute_order")

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
			attribute.String("trade_id", tradeID),
			attribute.String("target_client_id", order.TargetClientId),
			attribute.String("target_account_id", order.TargetAccountId),
			attribute.Float64("lot_size", order.LotSize),
		)

		orders = append(orders, order)
	}

	return orders
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
	agents := r.core.GetAgents()
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

		closeOrder := &pb.CloseOrder{
			CommandId:   closeOrderID,
			TradeId:     tradeID,
			TimestampMs: utils.NowUnixMilli(),
			// i1: Ticket EXACTO del slave (no 0 si se encontró en BD)
			Ticket: ticket,
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
