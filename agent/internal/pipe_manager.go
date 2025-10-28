//go:build windows
// +build windows

package internal

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/xKoRx/echo/sdk/domain"
	"github.com/xKoRx/echo/sdk/ipc"
	pb "github.com/xKoRx/echo/sdk/pb/v1"
	"github.com/xKoRx/echo/sdk/telemetry"
	"github.com/xKoRx/echo/sdk/telemetry/metricbundle"
	"github.com/xKoRx/echo/sdk/telemetry/semconv"
	"github.com/xKoRx/echo/sdk/utils"
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
}

// NewPipeManager crea un nuevo PipeManager.
func NewPipeManager(ctx context.Context, config *Config, tel *telemetry.Client, metrics *metricbundle.EchoMetrics) (*PipeManager, error) {
	pmCtx, cancel := context.WithCancel(ctx)

	pm := &PipeManager{
		config:      config,
		telemetry:   tel,
		echoMetrics: metrics,
		pipes:       make(map[string]*PipeHandler),
		ctx:         pmCtx,
		cancel:      cancel,
	}

	return pm, nil
}

// Start inicia la gestión de pipes.
//
// Crea pipes para masters y slaves y espera conexiones.
func (pm *PipeManager) Start(sendToCoreCh chan *pb.AgentMessage) error {
	pm.sendToCoreCh = sendToCoreCh

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
		name:         name,
		role:         role,
		accountID:    accountID,
		server:       pipeServer,
		telemetry:    pm.telemetry,
		echoMetrics:  pm.echoMetrics,
		sendToCoreCh: pm.sendToCoreCh,
		ctx:          pm.ctx,
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

	// Telemetría
	telemetry   *telemetry.Client
	echoMetrics *metricbundle.EchoMetrics

	// Canal para enviar al Core
	sendToCoreCh chan *pb.AgentMessage

	// Lifecycle
	ctx    context.Context
	closed bool
	mu     sync.Mutex
}

// Run ejecuta el loop principal del handler.
//
// Espera conexión del EA y procesa mensajes.
func (h *PipeHandler) Run() error {
	h.logInfo("Waiting for EA connection", map[string]interface{}{
		"pipe_name": h.name,
		"role":      h.role,
	})

	// Esperar conexión del EA
	if err := h.server.WaitForConnection(h.ctx); err != nil {
		return fmt.Errorf("failed to wait for connection: %w", err)
	}

	h.logInfo("EA connected", map[string]interface{}{
		"pipe_name": h.name,
		"role":      h.role,
	})

	// Usaremos un LineReader nuevo por iteración para evitar estados internos
	// de scanner atascados después de timeouts. Esto es liviano y simple para i0.
	// Modo debug: log de línea cruda antes de parsear (temporal)
	// reader.SetTimeout no expone, usamos el default del reader. Para depurar
	// añadimos una lectura de Peek en cada iteración.

	// Loop de lectura
	for {
		select {
		case <-h.ctx.Done():
			return nil
		default:
		}

		// Leer una línea (no bloqueante >1s) y loguear crudo para debug
		lr := ipc.NewLineReader(h.server)
		lr.SetTimeout(1 * time.Second)
		line, err := lr.ReadLine()
		if err == nil {
			if len(line) > 0 {
				h.logInfo("Raw line received", map[string]interface{}{
					"pipe_name": h.name,
					"bytes":     len(line),
					"preview":   preview(line, 160),
				})
			}
		} else {
			// Restaurar comportamiento: si hubo error en ReadLine, tratarlo abajo
		}

		// Si se leyó línea, parsearla; si no, intentar ReadMessage normal
		var msgMap map[string]interface{}
		if err == nil {
			// parsear manualmente la línea
			msgMap, err = ipc.ParseJSONLine(line)
		}
		if err != nil {
			// No llegó línea en este ciclo: continuar bucle
			errStr := err.Error()
			if errStr == "i/o timeout" || errStr == "EOF" {
				select {
				case <-h.ctx.Done():
					return nil
				case <-time.After(100 * time.Millisecond):
				}
				continue
			}
		}
		if err != nil {
			// Timeout es NORMAL cuando EA no envía nada - no es error
			// Solo loggear si es un error real (no timeout/EOF)
			errStr := err.Error()
			if errStr != "i/o timeout" && errStr != "EOF" {
				h.logError("Failed to read message", err, map[string]interface{}{
					"pipe_name": h.name,
				})
			}
			// Si no hay datos, dormir 100ms antes de reintentar (evitar CPU spinning)
			select {
			case <-h.ctx.Done():
				return nil
			case <-time.After(100 * time.Millisecond):
			}
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
		h.logInfo("Handshake received", map[string]interface{}{
			"pipe_name": h.name,
			"role":      h.role,
		})
		// TODO i0: nada más que loggear, i1+: registrar cliente
		return nil

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
		h.logInfo("Handshake received", map[string]interface{}{
			"pipe_name": h.name,
			"role":      h.role,
		})
		return nil

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
