//go:build windows
// +build windows

package internal

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/xKoRx/echo/sdk/domain"
	"github.com/xKoRx/echo/sdk/domain/handshake"
	"github.com/xKoRx/echo/sdk/ipc"
	pb "github.com/xKoRx/echo/sdk/pb/v1"
	"github.com/xKoRx/echo/sdk/telemetry"
	"github.com/xKoRx/echo/sdk/telemetry/metricbundle"
	"github.com/xKoRx/echo/sdk/telemetry/semconv"
	"github.com/xKoRx/echo/sdk/utils"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/protobuf/encoding/protojson"
)

// preview devuelve los primeros n caracteres seguros para log
func preview(b []byte, max int) string {
	if len(b) == 0 {
		return ""
	}
	if len(b) > max {
		return string(b[:max]) + "..."
	}
	return string(b)
}

// PipeManager gestiona los Named Pipes y conexiones de EAs.
//
// Responsabilidades:
//   - Crear Named Pipes (1 por EA)
//   - Esperar conexiones de EAs
//   - Leer mensajes JSON de EAs y transformar a proto
//   - Escribir mensajes proto transformados a JSON para EAs
//   - i2: Detectar conexión/desconexión y notificar al Core
type PipeManager struct {
	config      *Config
	telemetry   *telemetry.Client
	echoMetrics *metricbundle.EchoMetrics

	// Pipes activos
	pipes   map[string]*PipeHandler // key: pipe name (ej: "echo_master_12345")
	pipesMu sync.RWMutex

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Canal para enviar mensajes al Core
	sendToCoreCh chan *pb.AgentMessage

	// i2: Agent ID para notificaciones AccountConnected/AccountDisconnected
	agentID string

	onHandshake        func(string)
	protocolMinVersion int
	protocolMaxVersion int
	allowLegacy        bool
}

// NewPipeManager crea un nuevo PipeManager.
func NewPipeManager(ctx context.Context, config *Config, tel *telemetry.Client, metrics *metricbundle.EchoMetrics) (*PipeManager, error) {
	pmCtx, cancel := context.WithCancel(ctx)

	pm := &PipeManager{
		config:             config,
		telemetry:          tel,
		echoMetrics:        metrics,
		pipes:              make(map[string]*PipeHandler),
		ctx:                pmCtx,
		cancel:             cancel,
		protocolMinVersion: config.ProtocolMinVersion,
		protocolMaxVersion: config.ProtocolMaxVersion,
		allowLegacy:        config.ProtocolAllowLegacy,
	}

	return pm, nil
}

func (pm *PipeManager) SetHandshakeCallback(cb func(string)) {
	pm.onHandshake = cb
}

// Start inicia la gestión de pipes.
//
// Crea pipes para masters y slaves y espera conexiones.
func (pm *PipeManager) Start(sendToCoreCh chan *pb.AgentMessage, agentID string) error {
	pm.sendToCoreCh = sendToCoreCh
	pm.agentID = agentID // i2: Guardar agentID

	pm.logInfo("PipeManager starting", map[string]interface{}{
		"master_accounts": len(pm.config.MasterAccounts),
		"slave_accounts":  len(pm.config.SlaveAccounts),
	})

	// Crear pipes para masters
	for _, accountID := range pm.config.MasterAccounts {
		pipeName := fmt.Sprintf("%smaster_%s", pm.config.PipePrefix, accountID)
		if err := pm.createPipe(pipeName, "master", accountID); err != nil {
			return fmt.Errorf("failed to create master pipe %s: %w", pipeName, err)
		}
	}

	// Crear pipes para slaves
	for _, accountID := range pm.config.SlaveAccounts {
		pipeName := fmt.Sprintf("%sslave_%s", pm.config.PipePrefix, accountID)
		if err := pm.createPipe(pipeName, "slave", accountID); err != nil {
			return fmt.Errorf("failed to create slave pipe %s: %w", pipeName, err)
		}
	}

	pm.logInfo("PipeManager started", map[string]interface{}{
		"total_pipes": len(pm.pipes),
	})

	// Esperar señal de cierre
	<-pm.ctx.Done()
	return nil
}

// createPipe crea un Named Pipe y arranca su handler.
func (pm *PipeManager) createPipe(name, role, accountID string) error {
	// Crear pipe server usando SDK
	pipeServer, err := ipc.NewWindowsPipeServer(name)
	if err != nil {
		return fmt.Errorf("failed to create pipe server: %w", err)
	}

	pm.logInfo("Pipe created", map[string]interface{}{
		"pipe_name":  name,
		"role":       role,
		"account_id": accountID,
	})

	// Crear handler
	handler := &PipeHandler{
		name:               name,
		role:               role,
		accountID:          accountID,
		server:             pipeServer,
		canonicalSymbols:   pm.config.CanonicalSymbols,
		telemetry:          pm.telemetry,
		echoMetrics:        pm.echoMetrics,
		sendToCoreCh:       pm.sendToCoreCh,
		agentID:            pm.agentID, // i2: Pasar agentID al handler
		ctx:                pm.ctx,
		onHandshake:        pm.onHandshake,
		allowLegacy:        pm.allowLegacy,
		protocolMinVersion: pm.protocolMinVersion,
		protocolMaxVersion: pm.protocolMaxVersion,
	}

	// Registrar pipe
	pm.pipesMu.Lock()
	pm.pipes[name] = handler
	pm.pipesMu.Unlock()

	// Iniciar handler en goroutine
	pm.wg.Add(1)
	go func() {
		defer pm.wg.Done()
		if err := handler.Run(); err != nil {
			pm.logError("Pipe handler failed", err, map[string]interface{}{
				"pipe_name": name,
			})
		}
	}()

	return nil
}

// GetPipe retorna un handler por nombre.
//
// Útil para enviar mensajes a un pipe específico (ej: ExecuteOrder a slave).
func (pm *PipeManager) GetPipe(name string) (*PipeHandler, bool) {
	pm.pipesMu.RLock()
	defer pm.pipesMu.RUnlock()
	handler, ok := pm.pipes[name]
	return handler, ok
}

// Close cierra todos los pipes.
func (pm *PipeManager) Close() error {
	pm.cancel()

	pm.pipesMu.Lock()
	defer pm.pipesMu.Unlock()

	for name, handler := range pm.pipes {
		if err := handler.Close(); err != nil {
			pm.logError("Failed to close pipe", err, map[string]interface{}{
				"pipe_name": name,
			})
		}
	}

	pm.wg.Wait()
	return nil
}

// logInfo loggea un mensaje INFO.
func (pm *PipeManager) logInfo(message string, fields map[string]interface{}) {
	attrs := mapToAttrs(fields)
	pm.telemetry.Info(pm.ctx, message, attrs...)
}

// logError loggea un mensaje ERROR.
func (pm *PipeManager) logError(message string, err error, fields map[string]interface{}) {
	attrs := mapToAttrs(fields)
	pm.telemetry.Error(pm.ctx, message, err, attrs...)
}

// PipeHandler maneja un Named Pipe específico (master o slave).
type PipeHandler struct {
	name      string
	role      string // "master" o "slave"
	accountID string
	server    ipc.PipeServer
	// Configuración compartida
	canonicalSymbols []string

	// Telemetría
	telemetry   *telemetry.Client
	echoMetrics *metricbundle.EchoMetrics

	// Canal para enviar al Core
	sendToCoreCh chan *pb.AgentMessage

	// i2: Agent ID para notificaciones AccountConnected/AccountDisconnected
	agentID string

	// Lifecycle
	ctx    context.Context
	closed bool
	mu     sync.Mutex

	// Cache local para filtrar reportes repetidos
	lastSpecReportMs int64

	onHandshake        func(string)
	protocolMinVersion int
	protocolMaxVersion int
	allowLegacy        bool
}

// Run ejecuta el loop principal del handler.
//
// i2b FIX: Loop externo para reconexión automática.
// Cuando el EA se desconecta (EOF), cierra solo la conexión actual (NO el listener)
// y vuelve a esperar una nueva conexión.
func (h *PipeHandler) Run() error {
	// Backoff para WaitForConnection en caso de errores repetidos
	reconnectDelay := 100 * time.Millisecond
	maxReconnectDelay := 5 * time.Second

	// Loop externo: aceptar conexiones indefinidamente
	for {
		select {
		case <-h.ctx.Done():
			return nil
		default:
		}

		h.logInfo("Waiting for EA connection", map[string]interface{}{
			"pipe_name": h.name,
			"role":      h.role,
		})

		// Esperar conexión del EA
		err := h.server.WaitForConnection(h.ctx)
		if err != nil {
			if h.ctx.Err() != nil {
				// Contexto cancelado - salir limpiamente
				return nil
			}

			// Error al aceptar conexión - backoff y reintentar
			h.logError("Failed to wait for connection", err, map[string]interface{}{
				"pipe_name":       h.name,
				"reconnect_delay": reconnectDelay.String(),
			})

			// Backoff exponencial con jitter
			select {
			case <-h.ctx.Done():
				return nil
			case <-time.After(reconnectDelay):
			}

			// Incrementar delay con jitter
			reconnectDelay = reconnectDelay * 2
			if reconnectDelay > maxReconnectDelay {
				reconnectDelay = maxReconnectDelay
			}
			// Agregar jitter (±25%)
			jitter := time.Duration(float64(reconnectDelay) * 0.25 * (2*rand.Float64() - 1))
			reconnectDelay += jitter

			continue
		}

		// Reset backoff tras conexión exitosa
		reconnectDelay = 100 * time.Millisecond

		h.logInfo("EA connected", map[string]interface{}{
			"pipe_name": h.name,
			"role":      h.role,
		})

		// NEW i2: Notificar al Core que la cuenta se conectó
		if h.role == "slave" || h.role == "master" {
			h.notifyAccountConnected()
		}

		// Procesar sesión (loop interno de lectura)
		sessionErr := h.handleSession()

		// Sesión terminada (EOF o error fatal)
		h.logInfo("Session ended", map[string]interface{}{
			"pipe_name": h.name,
			"role":      h.role,
			"reason":    sessionErr,
		})

		// NEW i2: Notificar desconexión
		if h.role == "slave" || h.role == "master" {
			reason := "client_disconnected"
			if sessionErr != nil && sessionErr.Error() != "EOF" {
				reason = "session_error"
			}
			h.notifyAccountDisconnected(reason)
		}

		// i2b FIX CRÍTICO: Cerrar SOLO la conexión actual, NO el listener
		if err := h.server.DisconnectClient(); err != nil {
			h.logError("Failed to disconnect client", err, map[string]interface{}{
				"pipe_name": h.name,
			})
		}

		h.logInfo("Client disconnected, listener still open", map[string]interface{}{
			"pipe_name": h.name,
			"role":      h.role,
		})

		// Volver al inicio del loop externo para aceptar nueva conexión
	}
}

// handleSession procesa la sesión de lectura con el EA conectado.
//
// Retorna cuando:
// - EOF (cliente se desconecta)
// - Error fatal de lectura
// - Contexto cancelado
func (h *PipeHandler) handleSession() error {
	// Loop de lectura
	for {
		select {
		case <-h.ctx.Done():
			return h.ctx.Err()
		default:
		}

		// Leer una línea (timeout de 1s para no bloquear indefinidamente)
		lr := ipc.NewLineReader(h.server)
		lr.SetTimeout(1 * time.Second)
		line, err := lr.ReadLine()

		if err == nil && len(line) > 0 {
			h.logDebug("Raw line received", map[string]interface{}{
				"pipe_name": h.name,
				"bytes":     len(line),
				"preview":   preview(line, 160),
			})
		}

		// Parsear JSON
		var msgMap map[string]interface{}
		if err == nil {
			msgMap, err = ipc.ParseJSONLine(line)
		}

		if err != nil {
			errStr := err.Error()

			// i2b: EOF significa desconexión del cliente - salir de la sesión
			if errStr == "EOF" {
				h.logInfo("Client disconnected (EOF detected)", map[string]interface{}{
					"pipe_name": h.name,
					"role":      h.role,
				})
				return fmt.Errorf("EOF")
			}

			// Timeout es NORMAL cuando EA no envía nada - continuar esperando
			if errStr == "i/o timeout" {
				continue
			}

			// Otros errores: loggear y continuar (pueden ser transitorios)
			h.logError("Failed to read message", err, map[string]interface{}{
				"pipe_name": h.name,
			})

			// Dormir 100ms antes de reintentar (evitar CPU spinning)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// Procesar mensaje según rol
		if h.role == "master" {
			if err := h.handleMasterMessage(msgMap); err != nil {
				h.logError("Failed to handle master message", err, nil)
			}
		} else if h.role == "slave" {
			if err := h.handleSlaveMessage(msgMap); err != nil {
				h.logError("Failed to handle slave message", err, nil)
			}
		}
	}
}

// handleMasterMessage procesa mensajes del Master EA.
//
// Transforma JSON → Proto y envía al Core.
func (h *PipeHandler) handleMasterMessage(msgMap map[string]interface{}) error {
	msgType := utils.ExtractString(msgMap, "type")

	switch msgType {
	case "handshake":
		return h.handleHandshake(msgMap)

	case "symbol_spec_report":
		return h.handleSymbolSpecReport(msgMap)

	case "quote_snapshot":
		return h.handleQuoteSnapshot(msgMap)

	case "ping":
		return h.handlePing(msgMap)

	case "trade_intent":
		return h.handleTradeIntent(msgMap)

	case "trade_close":
		return h.handleTradeClose(msgMap)

	default:
		h.logWarn("Unknown message type from master", map[string]interface{}{
			"type":      msgType,
			"pipe_name": h.name,
		})
		return nil
	}
}

// handleTradeIntent procesa un TradeIntent del Master EA.
func (h *PipeHandler) handleTradeIntent(msgMap map[string]interface{}) error {
	// Issue #M1: Agregar timestamp t1 (Agent recv from pipe)
	t1 := utils.NowUnixMilli()

	// Transformar JSON → Proto usando SDK
	protoIntent, err := domain.JSONToTradeIntent(msgMap)
	if err != nil {
		return fmt.Errorf("failed to parse trade_intent: %w", err)
	}

	// Issue #M1: Popular timestamp t1 en el proto
	if protoIntent.Timestamps != nil {
		protoIntent.Timestamps.T1AgentRecvMs = t1
	}

	h.logInfo("TradeIntent received from Master EA", map[string]interface{}{
		"trade_id":     protoIntent.TradeId,
		"client_id":    protoIntent.ClientId,
		"symbol":       protoIntent.Symbol,
		"side":         protoIntent.Side.String(),
		"lot_size":     protoIntent.LotSize,
		"price":        protoIntent.Price,
		"ticket":       protoIntent.Ticket,
		"magic_number": protoIntent.MagicNumber,
	})

	// Registrar métrica usando bundle
	h.echoMetrics.RecordIntentReceived(h.ctx,
		semconv.Echo.TradeID.String(protoIntent.TradeId),
		semconv.Echo.Symbol.String(protoIntent.Symbol),
		semconv.Echo.ClientID.String(protoIntent.ClientId),
	)

	// Enviar al Core via canal
	agentMsg := &pb.AgentMessage{
		Payload: &pb.AgentMessage_TradeIntent{
			TradeIntent: protoIntent,
		},
	}

	select {
	case h.sendToCoreCh <- agentMsg:
		h.echoMetrics.RecordIntentForwarded(h.ctx,
			semconv.Echo.TradeID.String(protoIntent.TradeId),
		)
		h.logInfo("TradeIntent forwarded to Core", map[string]interface{}{
			"trade_id": protoIntent.TradeId,
		})
	case <-h.ctx.Done():
		return h.ctx.Err()
	}

	return nil
}

// handleTradeClose procesa un TradeClose del Master EA.
func (h *PipeHandler) handleTradeClose(msgMap map[string]interface{}) error {
	// Transformar JSON → Proto usando SDK
	protoClose, err := domain.JSONToTradeClose(msgMap)
	if err != nil {
		return fmt.Errorf("failed to parse trade_close: %w", err)
	}

	h.logInfo("TradeClose received", map[string]interface{}{
		"trade_id": protoClose.TradeId,
		"ticket":   protoClose.Ticket,
	})

	// Enviar al Core
	agentMsg := &pb.AgentMessage{
		Payload: &pb.AgentMessage_TradeClose{
			TradeClose: protoClose,
		},
	}

	select {
	case h.sendToCoreCh <- agentMsg:
		h.logInfo("TradeClose forwarded to Core", map[string]interface{}{
			"trade_id": protoClose.TradeId,
		})
	case <-h.ctx.Done():
		return h.ctx.Err()
	}

	return nil
}

// handleSlaveMessage procesa mensajes del Slave EA.
func (h *PipeHandler) handleSlaveMessage(msgMap map[string]interface{}) error {
	msgType := utils.ExtractString(msgMap, "type")

	switch msgType {
	case "handshake":
		return h.handleHandshake(msgMap)

	case "symbol_spec_report":
		return h.handleSymbolSpecReport(msgMap)

	case "quote_snapshot":
		return h.handleQuoteSnapshot(msgMap)

	case "ping":
		return h.handlePing(msgMap)

	case "execution_result":
		return h.handleExecutionResult(msgMap)

	case "close_result":
		return h.handleCloseResult(msgMap)

	default:
		h.logWarn("Unknown message type from slave", map[string]interface{}{
			"type":      msgType,
			"pipe_name": h.name,
		})
		return nil
	}
}

// handleExecutionResult procesa un ExecutionResult del Slave EA.
func (h *PipeHandler) handleExecutionResult(msgMap map[string]interface{}) error {
	// Transformar JSON → Proto usando SDK
	protoResult, err := domain.JSONToExecutionResult(msgMap)
	if err != nil {
		return fmt.Errorf("failed to parse execution_result: %w", err)
	}

	h.logInfo("ExecutionResult received", map[string]interface{}{
		"command_id": protoResult.CommandId,
		"success":    protoResult.Success,
		"ticket":     protoResult.Ticket,
	})

	// Enviar al Core
	agentMsg := &pb.AgentMessage{
		Payload: &pb.AgentMessage_ExecutionResult{
			ExecutionResult: protoResult,
		},
	}

	select {
	case h.sendToCoreCh <- agentMsg:
		h.logInfo("ExecutionResult forwarded to Core", map[string]interface{}{
			"command_id": protoResult.CommandId,
		})
	case <-h.ctx.Done():
		return h.ctx.Err()
	}

	return nil
}

// handleCloseResult procesa un CloseResult del Slave EA mapeándolo a ExecutionResult.
func (h *PipeHandler) handleCloseResult(msgMap map[string]interface{}) error {
	// Transformar JSON → Proto usando SDK (mapear close_result → ExecutionResult)
	protoResult, err := domain.JSONToCloseResult(msgMap)
	if err != nil {
		return fmt.Errorf("failed to parse close_result: %w", err)
	}

	h.logInfo("CloseResult received", map[string]interface{}{
		"command_id": protoResult.CommandId,
		"success":    protoResult.Success,
		"ticket":     protoResult.Ticket,
	})

	// Enviar al Core como ExecutionResult
	agentMsg := &pb.AgentMessage{
		Payload: &pb.AgentMessage_ExecutionResult{
			ExecutionResult: protoResult,
		},
	}

	select {
	case h.sendToCoreCh <- agentMsg:
		h.logInfo("CloseResult forwarded to Core", map[string]interface{}{
			"command_id": protoResult.CommandId,
		})
	case <-h.ctx.Done():
		return h.ctx.Err()
	}

	return nil
}

// handleHandshake procesa un handshake del EA (i3).
//
// Extrae symbols del handshake y envía AccountSymbolsReport al Core si están presentes.
func (h *PipeHandler) handleHandshake(msgMap map[string]interface{}) error {
	normalized, err := handshake.NormalizeHandshakePayload(msgMap)
	if err != nil {
		h.logWarn("Failed to normalize handshake", map[string]interface{}{
			"error":     err.Error(),
			"pipe_name": h.name,
		})
		return err
	}

	accountID := normalized.AccountID
	if accountID == "" {
		accountID = h.accountID
	}
	pipeRole := normalized.PipeRole
	if pipeRole == "" {
		pipeRole = h.role
	}

	if h.onHandshake != nil {
		h.onHandshake(accountID)
	}

	if normalized.Legacy && !h.allowLegacy {
		h.logWarn("Legacy handshake blocked by configuration", map[string]interface{}{
			"account_id": accountID,
			"pipe_role":  pipeRole,
		})
		return nil
	}

	if h.protocolMinVersion > 0 && normalized.ProtocolVersion < h.protocolMinVersion {
		h.logWarn("Handshake protocol version below minimum", map[string]interface{}{
			"account_id":       accountID,
			"protocol_version": normalized.ProtocolVersion,
			"min_version":      h.protocolMinVersion,
		})
	}
	if h.protocolMaxVersion > 0 && normalized.ProtocolVersion > h.protocolMaxVersion {
		h.logWarn("Handshake protocol version above maximum", map[string]interface{}{
			"account_id":       accountID,
			"protocol_version": normalized.ProtocolVersion,
			"max_version":      h.protocolMaxVersion,
		})
	}

	if len(normalized.Symbols) == 0 {
		h.logWarn("Handshake without symbols", map[string]interface{}{
			"account_id": accountID,
			"pipe_name":  h.name,
		})
		return nil
	}

	report := &pb.AccountSymbolsReport{
		AccountId:    accountID,
		Symbols:      normalized.Symbols,
		ReportedAtMs: utils.NowUnixMilli(),
		Metadata:     normalized.ToProtoMetadata(),
	}

	h.logInfo("Handshake normalized", map[string]interface{}{
		"account_id":        accountID,
		"pipe_role":         pipeRole,
		"protocol_version":  normalized.ProtocolVersion,
		"client_semver":     normalized.ClientSemver,
		"symbols_count":     len(normalized.Symbols),
		"required_features": strings.Join(normalized.RequiredFeatures, ","),
	})

	agentMsg := &pb.AgentMessage{
		AgentId:     h.agentID,
		TimestampMs: utils.NowUnixMilli(),
		Payload: &pb.AgentMessage_AccountSymbolsReport{
			AccountSymbolsReport: report,
		},
	}

	select {
	case h.sendToCoreCh <- agentMsg:
		h.logInfo("AccountSymbolsReport sent to Core (i5)", map[string]interface{}{
			"account_id":    accountID,
			"symbols_count": len(report.Symbols),
		})
	case <-h.ctx.Done():
		return h.ctx.Err()
	}

	return nil
}

// handleSymbolSpecReport procesa un reporte de especificaciones desde el Slave EA.
func (h *PipeHandler) handleSymbolSpecReport(msgMap map[string]interface{}) error {
	payload, ok := msgMap["payload"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("symbol_spec_report missing payload")
	}

	accountID := utils.ExtractString(payload, "account_id")
	if accountID == "" {
		accountID = h.accountID
	}

	reportedAt := utils.ExtractInt64(payload, "reported_at_ms")
	if reportedAt <= 0 {
		reportedAt = nowUnixMilli()
	}
	nowMs := nowUnixMilli()
	specAgeMs := nowMs - reportedAt
	if specAgeMs < 0 {
		specAgeMs = 0
	}

	if reportedAt <= h.lastSpecReportMs {
		h.logWarn("SymbolSpecReport ignored due to stale reported_at", map[string]interface{}{
			"account_id":          accountID,
			"reported_at_ms":      reportedAt,
			"last_reported_at_ms": h.lastSpecReportMs,
		})
		h.echoMetrics.RecordAgentSpecsFiltered(h.ctx, accountID, "stale_report",
			attribute.Int64("reported_at_ms", reportedAt),
			attribute.Int64("last_reported_at_ms", h.lastSpecReportMs),
		)
		return nil
	}

	symbolsRaw, ok := payload["symbols"].([]interface{})
	if !ok || len(symbolsRaw) == 0 {
		h.logWarn("SymbolSpecReport without symbols", map[string]interface{}{
			"account_id": accountID,
		})
		h.echoMetrics.RecordAgentSpecsFiltered(h.ctx, accountID, "empty_payload",
			attribute.Int64("reported_at_ms", reportedAt),
		)
		return nil
	}

	validSpecs := make([]*pb.SymbolSpecification, 0, len(symbolsRaw))
	for _, raw := range symbolsRaw {
		symMap, ok := raw.(map[string]interface{})
		if !ok {
			h.logWarn("Invalid symbol specification format", map[string]interface{}{
				"account_id": accountID,
			})
			h.echoMetrics.RecordAgentSpecsFiltered(h.ctx, accountID, "invalid_format",
				attribute.String("reason", "non_object_entry"),
			)
			continue
		}

		spec := &pb.SymbolSpecification{
			CanonicalSymbol: utils.ExtractString(symMap, "canonical_symbol"),
			BrokerSymbol:    utils.ExtractString(symMap, "broker_symbol"),
		}

		if generalMap, ok := symMap["general"].(map[string]interface{}); ok {
			spec.General = parseSymbolGeneral(generalMap)
		}

		if volumeMap, ok := symMap["volume"].(map[string]interface{}); ok {
			spec.Volume = parseVolumeSpec(volumeMap)
		}

		if swapMap, ok := symMap["swap"].(map[string]interface{}); ok {
			spec.Swap = parseSwapSpec(swapMap)
		}

		if sessionsRaw, ok := symMap["sessions"].([]interface{}); ok {
			spec.Sessions = parseSessionWindows(sessionsRaw)
		}

		if err := domain.ValidateSymbolSpecification(spec, h.canonicalSymbols); err != nil {
			h.logWarn("Symbol specification validation failed", map[string]interface{}{
				"account_id":       accountID,
				"canonical_symbol": spec.CanonicalSymbol,
				"error":            err.Error(),
			})
			h.echoMetrics.RecordAgentSpecsFiltered(h.ctx, accountID, "validation_failed",
				attribute.String("canonical_symbol", spec.CanonicalSymbol),
				attribute.String("error", err.Error()),
			)
			continue
		}

		validSpecs = append(validSpecs, spec)
	}

	if len(validSpecs) == 0 {
		h.logWarn("SymbolSpecReport filtered completely", map[string]interface{}{
			"account_id":     accountID,
			"reported_at_ms": reportedAt,
		})
		h.echoMetrics.RecordAgentSpecsFiltered(h.ctx, accountID, "empty_after_validation",
			attribute.Int64("reported_at_ms", reportedAt),
		)
		return nil
	}

	report := &pb.SymbolSpecReport{
		AccountId:    accountID,
		Symbols:      validSpecs,
		ReportedAtMs: reportedAt,
	}

	agentMsg := &pb.AgentMessage{
		AgentId:     h.agentID,
		TimestampMs: nowUnixMilli(),
		Payload: &pb.AgentMessage_SymbolSpecReport{
			SymbolSpecReport: report,
		},
	}

	select {
	case h.sendToCoreCh <- agentMsg:
		h.logDebug("SymbolSpecReport forwarded to Core", map[string]interface{}{
			"account_id":    accountID,
			"symbols_count": len(validSpecs),
			"spec_age_ms":   specAgeMs,
		})
		h.echoMetrics.RecordAgentSpecsForwarded(h.ctx, accountID,
			attribute.Int("symbols_count", len(validSpecs)),
			attribute.Float64("spec_age_ms", float64(specAgeMs)),
			attribute.Int64("reported_at_ms", reportedAt),
		)
		h.lastSpecReportMs = reportedAt
	case <-h.ctx.Done():
		return h.ctx.Err()
	}

	return nil
}

// handleQuoteSnapshot procesa un snapshot de precios Bid/Ask desde el Slave EA.
func (h *PipeHandler) handleQuoteSnapshot(msgMap map[string]interface{}) error {
	payload, ok := msgMap["payload"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("quote_snapshot missing payload")
	}

	accountID := utils.ExtractString(payload, "account_id")
	if accountID == "" {
		accountID = h.accountID
	}

	timestamp := utils.ExtractInt64(payload, "timestamp_ms")
	if timestamp <= 0 {
		timestamp = nowUnixMilli()
	}

	snapshot := &pb.SymbolQuoteSnapshot{
		AccountId:       accountID,
		CanonicalSymbol: utils.ExtractString(payload, "canonical_symbol"),
		BrokerSymbol:    utils.ExtractString(payload, "broker_symbol"),
		Bid:             utils.ExtractFloat64(payload, "bid"),
		Ask:             utils.ExtractFloat64(payload, "ask"),
		SpreadPoints:    utils.ExtractFloat64(payload, "spread_points"),
		TimestampMs:     timestamp,
	}

	agentMsg := &pb.AgentMessage{
		AgentId:     h.agentID,
		TimestampMs: nowUnixMilli(),
		Payload: &pb.AgentMessage_SymbolQuoteSnapshot{
			SymbolQuoteSnapshot: snapshot,
		},
	}

	select {
	case h.sendToCoreCh <- agentMsg:
		h.logDebug("SymbolQuoteSnapshot forwarded to Core", map[string]interface{}{
			"account_id":       accountID,
			"canonical_symbol": snapshot.CanonicalSymbol,
			"bid":              snapshot.Bid,
			"ask":              snapshot.Ask,
		})
	case <-h.ctx.Done():
		return h.ctx.Err()
	}

	return nil
}

func parseSymbolGeneral(m map[string]interface{}) *pb.SymbolGeneral {
	general := &pb.SymbolGeneral{
		SpreadType:            parseSpreadType(utils.ExtractString(m, "spread_type")),
		FixedSpreadPoints:     utils.ExtractFloat64(m, "fixed_spread_points"),
		Digits:                int32(utils.ExtractInt64(m, "digits")),
		StopsLevel:            int32(utils.ExtractInt64(m, "stops_level")),
		ContractSize:          utils.ExtractFloat64(m, "contract_size"),
		MarginCurrency:        utils.ExtractString(m, "margin_currency"),
		ProfitCalculationMode: parseProfitCalculationMode(utils.ExtractString(m, "profit_calculation_mode")),
		MarginCalculationMode: parseMarginCalculationMode(utils.ExtractString(m, "margin_calculation_mode")),
		MarginHedge:           utils.ExtractFloat64(m, "margin_hedge"),
		MarginPercentage:      utils.ExtractFloat64(m, "margin_percentage"),
		TradePermission:       parseTradePermission(utils.ExtractString(m, "trade_permission")),
		ExecutionMode:         parseExecutionMode(utils.ExtractString(m, "execution_mode")),
		GtcMode:               parseGTCMode(utils.ExtractString(m, "gtc_mode")),
	}

	return general
}

func parseVolumeSpec(m map[string]interface{}) *pb.VolumeSpec {
	return &pb.VolumeSpec{
		MinVolume:  utils.ExtractFloat64(m, "min_volume"),
		MaxVolume:  utils.ExtractFloat64(m, "max_volume"),
		VolumeStep: utils.ExtractFloat64(m, "volume_step"),
	}
}

func parseSwapSpec(m map[string]interface{}) *pb.SwapSpec {
	return &pb.SwapSpec{
		SwapType:      parseSwapType(utils.ExtractString(m, "swap_type")),
		SwapLong:      utils.ExtractFloat64(m, "swap_long"),
		SwapShort:     utils.ExtractFloat64(m, "swap_short"),
		TripleSwapDay: parseWeekday(utils.ExtractString(m, "triple_swap_day")),
	}
}

func parseSessionWindows(raw []interface{}) []*pb.SessionWindow {
	windows := make([]*pb.SessionWindow, 0, len(raw))
	for _, entry := range raw {
		sessionMap, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}

		window := &pb.SessionWindow{
			Day: parseWeekday(utils.ExtractString(sessionMap, "day")),
		}
		if quotesRaw, ok := sessionMap["quote_sessions"].([]interface{}); ok {
			window.QuoteSessions = parseSessionRanges(quotesRaw)
		}
		if tradesRaw, ok := sessionMap["trade_sessions"].([]interface{}); ok {
			window.TradeSessions = parseSessionRanges(tradesRaw)
		}

		windows = append(windows, window)
	}
	return windows
}

func parseSessionRanges(raw []interface{}) []*pb.SessionRange {
	ranges := make([]*pb.SessionRange, 0, len(raw))
	for _, entry := range raw {
		rangeMap, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		start := utils.ExtractInt64(rangeMap, "start_minute")
		end := utils.ExtractInt64(rangeMap, "end_minute")
		if start < 0 || end <= 0 {
			continue
		}
		ranges = append(ranges, &pb.SessionRange{
			StartMinute: uint32(start),
			EndMinute:   uint32(end),
		})
	}
	return ranges
}

func parseSpreadType(v string) pb.SpreadType {
	normalized := normalizeEnumValue(v)
	switch normalized {
	case "floating":
		return pb.SpreadType_SPREAD_TYPE_FLOATING
	case "fixed":
		return pb.SpreadType_SPREAD_TYPE_FIXED
	default:
		return pb.SpreadType_SPREAD_TYPE_UNSPECIFIED
	}
}

func parseProfitCalculationMode(v string) pb.ProfitCalculationMode {
	normalized := normalizeEnumValue(v)
	switch normalized {
	case "forex":
		return pb.ProfitCalculationMode_PROFIT_CALCULATION_MODE_FOREX
	case "cfd":
		return pb.ProfitCalculationMode_PROFIT_CALCULATION_MODE_CFD
	case "futures":
		return pb.ProfitCalculationMode_PROFIT_CALCULATION_MODE_FUTURES
	case "exchange":
		return pb.ProfitCalculationMode_PROFIT_CALCULATION_MODE_EXCHANGE
	default:
		return pb.ProfitCalculationMode_PROFIT_CALCULATION_MODE_UNSPECIFIED
	}
}

func parseMarginCalculationMode(v string) pb.MarginCalculationMode {
	normalized := normalizeEnumValue(v)
	switch normalized {
	case "forex":
		return pb.MarginCalculationMode_MARGIN_CALCULATION_MODE_FOREX
	case "cfd":
		return pb.MarginCalculationMode_MARGIN_CALCULATION_MODE_CFD
	case "cfd_leverage":
		return pb.MarginCalculationMode_MARGIN_CALCULATION_MODE_CFD_LEVERAGE
	case "exchange":
		return pb.MarginCalculationMode_MARGIN_CALCULATION_MODE_EXCHANGE
	default:
		return pb.MarginCalculationMode_MARGIN_CALCULATION_MODE_UNSPECIFIED
	}
}

func parseTradePermission(v string) pb.TradePermission {
	normalized := normalizeEnumValue(v)
	switch normalized {
	case "disabled":
		return pb.TradePermission_TRADE_PERMISSION_DISABLED
	case "close_only":
		return pb.TradePermission_TRADE_PERMISSION_CLOSE_ONLY
	case "full", "full_access":
		return pb.TradePermission_TRADE_PERMISSION_FULL
	default:
		return pb.TradePermission_TRADE_PERMISSION_UNSPECIFIED
	}
}

func parseExecutionMode(v string) pb.ExecutionMode {
	normalized := normalizeEnumValue(v)
	switch normalized {
	case "request":
		return pb.ExecutionMode_EXECUTION_MODE_REQUEST
	case "instant":
		return pb.ExecutionMode_EXECUTION_MODE_INSTANT
	case "market":
		return pb.ExecutionMode_EXECUTION_MODE_MARKET
	case "exchange":
		return pb.ExecutionMode_EXECUTION_MODE_EXCHANGE
	default:
		return pb.ExecutionMode_EXECUTION_MODE_UNSPECIFIED
	}
}

func parseGTCMode(v string) pb.GTCMode {
	normalized := normalizeEnumValue(v)
	switch normalized {
	case "gtc", "good_till_cancel":
		return pb.GTCMode_GTC_MODE_GTC
	case "day":
		return pb.GTCMode_GTC_MODE_DAY
	case "gtc_and_day":
		return pb.GTCMode_GTC_MODE_GTC_AND_DAY
	default:
		return pb.GTCMode_GTC_MODE_UNSPECIFIED
	}
}

func parseSwapType(v string) pb.SwapType {
	normalized := normalizeEnumValue(v)
	switch normalized {
	case "points":
		return pb.SwapType_SWAP_TYPE_POINTS
	case "currency":
		return pb.SwapType_SWAP_TYPE_CURRENCY
	case "currency_symbol":
		return pb.SwapType_SWAP_TYPE_CURRENCY_SYMBOL
	case "interest_current":
		return pb.SwapType_SWAP_TYPE_INTEREST_CURRENT
	default:
		if len(normalized) == 3 { // currency code (e.g. USD)
			return pb.SwapType_SWAP_TYPE_CURRENCY
		}
		return pb.SwapType_SWAP_TYPE_UNSPECIFIED
	}
}

func parseWeekday(v string) pb.Weekday {
	normalized := normalizeEnumValue(v)
	switch normalized {
	case "sunday", "sun":
		return pb.Weekday_WEEKDAY_SUNDAY
	case "monday", "mon":
		return pb.Weekday_WEEKDAY_MONDAY
	case "tuesday", "tue":
		return pb.Weekday_WEEKDAY_TUESDAY
	case "wednesday", "wed":
		return pb.Weekday_WEEKDAY_WEDNESDAY
	case "thursday", "thu":
		return pb.Weekday_WEEKDAY_THURSDAY
	case "friday", "fri":
		return pb.Weekday_WEEKDAY_FRIDAY
	case "saturday", "sat":
		return pb.Weekday_WEEKDAY_SATURDAY
	default:
		return pb.Weekday_WEEKDAY_UNSPECIFIED
	}
}

func normalizeEnumValue(v string) string {
	n := strings.TrimSpace(strings.ToLower(v))
	n = strings.ReplaceAll(n, "-", "_")
	n = strings.ReplaceAll(n, " ", "_")
	n = strings.ReplaceAll(n, "__", "_")
	return n
}

// handlePing procesa un ping del EA y responde con pong.
//
// i2b: Implementación de ping/pong para liveness checking.
func (h *PipeHandler) handlePing(msgMap map[string]interface{}) error {
	pingID := utils.ExtractString(msgMap, "id")
	echoMs := utils.ExtractInt64(msgMap, "timestamp_ms")

	if pingID == "" {
		h.logWarn("Ping without id", map[string]interface{}{
			"pipe_name": h.name,
		})
		return nil
	}

	// Construir pong response
	pongMsg := map[string]interface{}{
		"type":         "pong",
		"id":           pingID,
		"timestamp_ms": utils.NowUnixMilli(),
		"echo_ms":      echoMs,
	}

	// i2b FIX: Escribir pong directamente con flush inmediato
	writer := ipc.NewJSONWriter(h.server)
	if err := writer.WriteMessage(pongMsg); err != nil {
		h.logError("Failed to send pong", err, map[string]interface{}{
			"ping_id": pingID,
		})
		return err
	}

	// i2b FIX: Flush explícito para asegurar envío inmediato
	// Aunque Named Pipes son auto-flush, esto garantiza que no quede en buffers del SO
	if err := writer.Flush(); err != nil {
		h.logError("Failed to flush pong", err, map[string]interface{}{
			"ping_id": pingID,
		})
		return err
	}

	h.logDebug("Pong sent", map[string]interface{}{
		"ping_id":  pingID,
		"echo_ms":  echoMs,
		"rtt_calc": utils.NowUnixMilli() - echoMs,
	})

	return nil
}

// WriteMessage escribe un mensaje al pipe.
//
// Transforma Proto → JSON y escribe al EA.
func (h *PipeHandler) WriteMessage(msg interface{}) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.closed {
		return fmt.Errorf("pipe is closed")
	}

	// Crear JSONWriter usando SDK
	writer := ipc.NewJSONWriter(h.server)

	// Transformar según tipo de mensaje
	var jsonMap map[string]interface{}
	var err error

	switch m := msg.(type) {
	case *pb.ExecuteOrder:
		jsonMap, err = domain.ExecuteOrderToJSON(m)
	case *pb.CloseOrder:
		// Issue #C3: Agregar soporte para CloseOrder
		jsonMap, err = domain.CloseOrderToJSON(m)
	default:
		return fmt.Errorf("unsupported message type: %T", msg)
	}

	if err != nil {
		return fmt.Errorf("failed to transform to JSON: %w", err)
	}

	// Escribir usando SDK
	if err := writer.WriteMessage(jsonMap); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	return nil
}

func (h *PipeHandler) WriteSymbolRegistrationResult(result *pb.SymbolRegistrationResult) error {
	if result == nil {
		return fmt.Errorf("nil symbol registration result")
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.closed {
		return fmt.Errorf("pipe is closed")
	}

	writer := ipc.NewJSONWriter(h.server)
	marshaler := protojson.MarshalOptions{EmitUnpopulated: true}
	data, err := marshaler.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal symbol registration result: %w", err)
	}

	payload, err := utils.JSONToMap(data)
	if err != nil {
		return fmt.Errorf("transform symbol registration payload: %w", err)
	}

	message := map[string]interface{}{
		"type":         "symbol_registration_result",
		"timestamp_ms": utils.NowUnixMilli(),
		"payload":      payload,
	}

	if err := writer.WriteMessage(message); err != nil {
		return fmt.Errorf("write symbol registration result: %w", err)
	}

	return nil
}

// Close cierra el handler.
func (h *PipeHandler) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.closed {
		return nil
	}

	h.closed = true
	return h.server.Close()
}

// logInfo loggea un mensaje INFO.
func (h *PipeHandler) logInfo(message string, fields map[string]interface{}) {
	if fields == nil {
		fields = make(map[string]interface{})
	}
	fields["component"] = "pipe_handler"
	fields["pipe_name"] = h.name
	fields["role"] = h.role
	attrs := mapToAttrs(fields)
	h.telemetry.Info(h.ctx, message, attrs...)
}

// logWarn loggea un mensaje WARN.
func (h *PipeHandler) logWarn(message string, fields map[string]interface{}) {
	if fields == nil {
		fields = make(map[string]interface{})
	}
	fields["component"] = "pipe_handler"
	fields["pipe_name"] = h.name
	attrs := mapToAttrs(fields)
	h.telemetry.Warn(h.ctx, message, attrs...)
}

// logError loggea un mensaje ERROR.
func (h *PipeHandler) logError(message string, err error, fields map[string]interface{}) {
	if fields == nil {
		fields = make(map[string]interface{})
	}
	fields["component"] = "pipe_handler"
	fields["pipe_name"] = h.name
	attrs := mapToAttrs(fields)
	h.telemetry.Error(h.ctx, message, err, attrs...)
}

// logDebug loggea un mensaje DEBUG.
func (h *PipeHandler) logDebug(message string, fields map[string]interface{}) {
	if fields == nil {
		fields = make(map[string]interface{})
	}
	fields["component"] = "pipe_handler"
	fields["pipe_name"] = h.name
	attrs := mapToAttrs(fields)
	h.telemetry.Debug(h.ctx, message, attrs...)
}

// NEW i2: notifyAccountConnected notifica al Core que una cuenta se conectó.
func (h *PipeHandler) notifyAccountConnected() {
	if h.agentID == "" || h.accountID == "" {
		h.logWarn("Cannot notify AccountConnected: missing agentID or accountID", map[string]interface{}{
			"agent_id":   h.agentID,
			"account_id": h.accountID,
		})
		return
	}

	clientType := h.role // "slave" o "master"

	msg := &pb.AgentMessage{
		AgentId:     h.agentID,
		TimestampMs: utils.NowUnixMilli(),
		Payload: &pb.AgentMessage_AccountConnected{
			AccountConnected: &pb.AccountConnected{
				AccountId:     h.accountID,
				ConnectedAtMs: utils.NowUnixMilli(),
				ClientType:    &clientType,
			},
		},
	}

	select {
	case h.sendToCoreCh <- msg:
		h.logInfo("AccountConnected sent to Core (i2)", map[string]interface{}{
			"account_id":  h.accountID,
			"client_type": clientType,
		})
	case <-h.ctx.Done():
		h.logWarn("Context done, cannot send AccountConnected (i2)", nil)
	}
}

// NEW i2: notifyAccountDisconnected notifica al Core que una cuenta se desconectó.
func (h *PipeHandler) notifyAccountDisconnected(reason string) {
	if h.agentID == "" || h.accountID == "" {
		return // Silencioso si no hay datos
	}

	msg := &pb.AgentMessage{
		AgentId:     h.agentID,
		TimestampMs: utils.NowUnixMilli(),
		Payload: &pb.AgentMessage_AccountDisconnected{
			AccountDisconnected: &pb.AccountDisconnected{
				AccountId:        h.accountID,
				DisconnectedAtMs: utils.NowUnixMilli(),
				Reason:           &reason,
			},
		},
	}

	select {
	case h.sendToCoreCh <- msg:
		h.logInfo("AccountDisconnected sent to Core (i2)", map[string]interface{}{
			"account_id": h.accountID,
			"reason":     reason,
		})
	case <-h.ctx.Done():
		h.logWarn("Context done, cannot send AccountDisconnected (i2)", nil)
	}
}
