package internal

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/xKoRx/echo/sdk/domain/handshake"
	pb "github.com/xKoRx/echo/sdk/pb/v1"
	"github.com/xKoRx/echo/sdk/telemetry/semconv"
	"github.com/xKoRx/echo/sdk/utils"
	"go.opentelemetry.io/otel/attribute"
)

// connectToCore establece la conexión gRPC con el Core (i1).
func (a *Agent) connectToCore() error {
	a.logInfo("Connecting to Core (i1)", map[string]interface{}{
		"address":           a.config.CoreAddress,
		"agent_id":          a.config.AgentID,
		"keepalive_time":    a.config.KeepAliveTime.String(),
		"keepalive_timeout": a.config.KeepAliveTimeout.String(),
	})

	// i1: Crear cliente usando wrapper con config completa (KeepAlive)
	client, err := NewCoreClient(a.ctx, a.config)
	if err != nil {
		return fmt.Errorf("failed to create core client: %w", err)
	}

	a.coreClient = client

	// i1: Crear stream bidireccional con agent_id desde config
	stream, err := client.StreamBidi(a.ctx, a.config.AgentID)
	if err != nil {
		return fmt.Errorf("failed to create stream: %w", err)
	}

	a.coreStream = stream
	a.logInfo("Stream connected to Core (i1)", map[string]interface{}{
		"agent_id": a.config.AgentID,
	})

	// NEW i2: Enviar AgentHello SOLO con metadata (sin owned_accounts)
	if err := a.sendAgentHello(); err != nil {
		return fmt.Errorf("failed to send AgentHello: %w", err)
	}

	a.logInfo("Connected to Core (i2)", nil)
	return nil
}

// sendToCore goroutine que envía mensajes al Core.
//
// Lee del canal sendToCoreCh y envía por el stream.
func (a *Agent) sendToCore() {
	defer a.wg.Done()

	a.logInfo("Send loop started", nil)

	for {
		select {
		case msg, ok := <-a.sendToCoreCh:
			if !ok {
				a.logInfo("Send channel closed", nil)
				return
			}

			if err := a.coreStream.Send(msg); err != nil {
				a.logError("Failed to send to Core", err, nil)
				// TODO i0: solo loggear, i1+: reconectar
				continue
			}

			// Log según tipo de mensaje
			a.logSentMessage(msg)

		case <-a.ctx.Done():
			a.logInfo("Send loop stopped", nil)
			return
		}
	}
}

// receiveFromCore goroutine que recibe mensajes del Core.
//
// Lee del stream y rutea a los pipes correspondientes.
func (a *Agent) receiveFromCore() {
	defer a.wg.Done()

	a.logInfo("Receive loop started", nil)

	for {
		select {
		case <-a.ctx.Done():
			a.logInfo("Receive loop stopped", nil)
			return
		default:
		}

		// Recibir mensaje del stream
		msg := &pb.CoreMessage{}
		if err := a.coreStream.RecvMsg(msg); err != nil {
			a.logError("Failed to receive from Core", err, nil)
			// TODO i0: solo loggear, i1+: reconectar
			return
		}

		// Issue #M3: Agregar timestamp t4 (Agent recv from Core)
		t4 := utils.NowUnixMilli()

		// Poplar t4 en el mensaje según su tipo
		switch payload := msg.Payload.(type) {
		case *pb.CoreMessage_ExecuteOrder:
			if payload.ExecuteOrder.Timestamps != nil {
				payload.ExecuteOrder.Timestamps.T4AgentRecvMs = t4
			}
		case *pb.CoreMessage_CloseOrder:
			if payload.CloseOrder.Timestamps != nil {
				payload.CloseOrder.Timestamps.T4AgentRecvMs = t4
			}
		}

		// Rutear según tipo de mensaje
		if err := a.routeCoreMessage(msg); err != nil {
			a.logError("Failed to route message", err, nil)
		}
	}
}

// routeCoreMessage rutea un mensaje del Core al pipe correspondiente.
func (a *Agent) routeCoreMessage(msg *pb.CoreMessage) error {
	switch payload := msg.Payload.(type) {
	case *pb.CoreMessage_ExecuteOrder:
		return a.routeExecuteOrder(payload.ExecuteOrder)

	case *pb.CoreMessage_CloseOrder:
		return a.routeCloseOrder(payload.CloseOrder)

	case *pb.CoreMessage_SymbolRegistrationResult:
		return a.routeSymbolRegistrationResult(payload.SymbolRegistrationResult)

	case *pb.CoreMessage_CoreHello:
		a.handleCoreHello(payload.CoreHello)
		return nil

	case *pb.CoreMessage_DeliveryHeartbeat:
		a.handleDeliveryHeartbeat(payload.DeliveryHeartbeat)
		return nil

	default:
		a.logWarn("Unknown message type from Core", map[string]interface{}{
			"type":    fmt.Sprintf("%T", payload),
			"raw_msg": msg.String(),
		})
		return nil
	}
}

// routeExecuteOrder rutea un ExecuteOrder al Slave EA correspondiente.
//
// Issue #C4/#C5: Rutea al slave específico usando target_account_id.
func (a *Agent) routeExecuteOrder(order *pb.ExecuteOrder) error {
	a.logInfo("ExecuteOrder received from Core", map[string]interface{}{
		"command_id":        order.CommandId,
		"trade_id":          order.TradeId,
		"symbol":            order.Symbol,
		"side":              order.Side.String(),
		"lot_size":          order.LotSize,
		"target_client_id":  order.TargetClientId,  // Issue #C5
		"target_account_id": order.TargetAccountId, // Issue #C5
	})

	// Issue #C4/#C5: Usar target_account_id para routing específico
	if order.TargetAccountId == "" {
		return fmt.Errorf("target_account_id is empty, cannot route")
	}

	accountID := order.TargetAccountId
	status := a.getHandshakeStatus(accountID)
	if status == handshake.RegistrationStatusRejected || status == handshake.RegistrationStatusUnspecified {
		reason := "pending"
		if status == handshake.RegistrationStatusRejected {
			reason = "rejected"
		}
		a.echoMetrics.RecordAgentHandshakeBlocked(a.ctx, reason,
			semconv.Echo.AccountID.String(accountID),
			semconv.Echo.CommandID.String(order.CommandId),
		)
		a.logWarn("Blocking ExecuteOrder due to handshake status", map[string]interface{}{
			"account_id": accountID,
			"status":     handshakeStatusString(status),
			"command_id": order.CommandId,
		})
		return fmt.Errorf("handshake status %s blocks execute order", handshakeStatusString(status))
	}
	if status == handshake.RegistrationStatusWarning {
		a.logInfo("ExecuteOrder under handshake warning", map[string]interface{}{
			"account_id": accountID,
			"command_id": order.CommandId,
		})
	}

	pipeName := fmt.Sprintf("%sslave_%s", a.config.PipePrefix, accountID)

	// Obtener handler del pipe
	handler, ok := a.pipeManager.GetPipe(pipeName)
	if !ok {
		return fmt.Errorf("pipe not found: %s (target: %s)", pipeName, order.TargetClientId)
	}

	if a.delivery != nil {
		if err := a.delivery.HandleExecuteOrder(order); err != nil {
			return fmt.Errorf("delivery register execute: %w", err)
		}
	}

	// Escribir al pipe (transforma Proto → JSON internamente)
	start := time.Now()
	if err := handler.WriteMessage(order); err != nil {
		if a.delivery != nil {
			a.delivery.HandlePipeResult(order.CommandId, false, err)
		}
		a.recordPipeDeliveryLatency(accountID, "error", time.Since(start))
		return fmt.Errorf("failed to write to pipe: %w", err)
	}
	a.recordPipeDeliveryLatency(accountID, "ok", time.Since(start))

	if a.delivery != nil {
		a.delivery.HandlePipeResult(order.CommandId, true, nil)
	}

	// Registrar métrica
	a.echoMetrics.RecordExecutionDispatched(a.ctx,
		semconv.Echo.CommandID.String(order.CommandId),
		semconv.Echo.TradeID.String(order.TradeId),
	)

	a.logInfo("ExecuteOrder dispatched to Slave EA", map[string]interface{}{
		"command_id":        order.CommandId,
		"trade_id":          order.TradeId,
		"symbol":            order.Symbol,
		"side":              order.Side.String(),
		"lot_size":          order.LotSize,
		"target_account_id": order.TargetAccountId,
		"pipe_name":         pipeName,
	})

	return nil
}

// routeCloseOrder rutea un CloseOrder al Slave EA correspondiente.
//
// Issue #A4/#C3: Rutea al slave específico usando target_account_id.
func (a *Agent) routeCloseOrder(order *pb.CloseOrder) error {
	a.logInfo("CloseOrder received from Core", map[string]interface{}{
		"command_id":        order.CommandId,
		"trade_id":          order.TradeId,
		"ticket":            order.Ticket,
		"target_client_id":  order.TargetClientId,  // Issue #C3
		"target_account_id": order.TargetAccountId, // Issue #C3
	})

	// Issue #A4/#C3: Usar target_account_id para routing específico
	if order.TargetAccountId == "" {
		return fmt.Errorf("target_account_id is empty, cannot route CloseOrder")
	}

	accountID := order.TargetAccountId
	status := a.getHandshakeStatus(accountID)
	if status == handshake.RegistrationStatusRejected || status == handshake.RegistrationStatusUnspecified {
		reason := "pending"
		if status == handshake.RegistrationStatusRejected {
			reason = "rejected"
		}
		a.echoMetrics.RecordAgentHandshakeBlocked(a.ctx, reason,
			semconv.Echo.AccountID.String(accountID),
			semconv.Echo.CommandID.String(order.CommandId),
		)
		a.logWarn("Blocking CloseOrder due to handshake status", map[string]interface{}{
			"account_id": accountID,
			"status":     handshakeStatusString(status),
			"command_id": order.CommandId,
		})
		return fmt.Errorf("handshake status %s blocks close order", handshakeStatusString(status))
	}

	pipeName := fmt.Sprintf("%sslave_%s", a.config.PipePrefix, accountID)

	// Obtener handler del pipe
	handler, ok := a.pipeManager.GetPipe(pipeName)
	if !ok {
		return fmt.Errorf("pipe not found: %s (target: %s)", pipeName, order.TargetClientId)
	}

	if a.delivery != nil {
		if err := a.delivery.HandleCloseOrder(order); err != nil {
			return fmt.Errorf("delivery register close: %w", err)
		}
	}

	// Escribir al pipe (transforma Proto → JSON internamente)
	start := time.Now()
	if err := handler.WriteMessage(order); err != nil {
		if a.delivery != nil {
			a.delivery.HandlePipeResult(order.CommandId, false, err)
		}
		a.recordPipeDeliveryLatency(accountID, "error", time.Since(start))
		return fmt.Errorf("failed to write CloseOrder to pipe: %w", err)
	}
	a.recordPipeDeliveryLatency(accountID, "ok", time.Since(start))

	if a.delivery != nil {
		a.delivery.HandlePipeResult(order.CommandId, true, nil)
	}

	a.logInfo("CloseOrder dispatched to Slave", map[string]interface{}{
		"command_id": order.CommandId,
		"trade_id":   order.TradeId,
		"pipe_name":  pipeName,
	})

	return nil
}

func (a *Agent) routeSymbolRegistrationResult(result *pb.SymbolRegistrationResult) error {
	if result == nil {
		return fmt.Errorf("nil symbol registration result")
	}

	accountID := result.GetAccountId()
	status := handshake.RegistrationStatus(result.GetStatus())
	a.setHandshakeStatus(accountID, status)

	evaluationID := strings.TrimSpace(result.GetEvaluationId())
	if evaluationID != "" {
		if last := a.getLastEvaluationID(accountID); last == evaluationID {
			a.logDebug("SymbolRegistrationResult skipped (duplicate evaluation_id)", map[string]interface{}{
				"account_id":    accountID,
				"evaluation_id": evaluationID,
				"pipe_role":     result.GetPipeRole(),
				"status":        handshakeStatusString(status),
			})
			return nil
		}
	}

	pipeRole := result.GetPipeRole()
	if pipeRole == "" {
		pipeRole = "slave"
	}

	pipeName := fmt.Sprintf("%s%s_%s", a.config.PipePrefix, pipeRole, accountID)
	handler, ok := a.pipeManager.GetPipe(pipeName)
	if !ok {
		a.logWarn("Pipe not found for SymbolRegistrationResult", map[string]interface{}{
			"pipe_name":  pipeName,
			"account_id": accountID,
		})
		a.echoMetrics.RecordAgentHandshakeForwardError(a.ctx, "pipe_not_found",
			semconv.Echo.AccountID.String(accountID),
			attribute.String("pipe_role", pipeRole),
		)
		return nil
	}

	if err := handler.WriteSymbolRegistrationResult(result); err != nil {
		a.echoMetrics.RecordAgentHandshakeForwardError(a.ctx, err.Error(),
			semconv.Echo.AccountID.String(accountID),
			attribute.String("pipe_role", pipeRole),
		)
		return fmt.Errorf("failed to forward symbol registration result: %w", err)
	}

	a.echoMetrics.RecordAgentHandshakeForwarded(a.ctx, handshakeStatusString(status),
		semconv.Echo.AccountID.String(accountID),
		attribute.String("pipe_role", pipeRole),
	)

	a.logDebug("SymbolRegistrationResult routed", map[string]interface{}{
		"account_id": accountID,
		"pipe_role":  pipeRole,
		"status":     handshakeStatusString(status),
	})

	if evaluationID != "" {
		a.setLastEvaluationID(accountID, evaluationID)
	}

	return nil
}

func handshakeStatusString(status handshake.RegistrationStatus) string {
	switch status {
	case handshake.RegistrationStatusAccepted:
		return "ACCEPTED"
	case handshake.RegistrationStatusWarning:
		return "WARNING"
	case handshake.RegistrationStatusRejected:
		return "REJECTED"
	default:
		return "UNSPECIFIED"
	}
}

func (a *Agent) handleCoreHello(hello *pb.CoreHello) {
	if hello == nil {
		return
	}
	a.logInfo("CoreHello received", map[string]interface{}{
		"required_protocol_version": hello.RequiredProtocolVersion,
		"lossless_required":         hello.LosslessRequired,
		"service_version":           hello.ServiceVersion,
		"compat_mode":               hello.CompatModeActive,
	})
}

func (a *Agent) handleDeliveryHeartbeat(hb *pb.DeliveryHeartbeat) {
	if hb == nil {
		return
	}
	if a.delivery != nil {
		a.delivery.UpdateConfig(hb)
	}
	a.updateMasterDeliveryConfig(hb)
}

// logSentMessage loggea un mensaje enviado al Core según su tipo.
func (a *Agent) logSentMessage(msg *pb.AgentMessage) {
	switch payload := msg.Payload.(type) {
	case *pb.AgentMessage_TradeIntent:
		a.logInfo("TradeIntent sent to Core via gRPC", map[string]interface{}{
			"trade_id":     payload.TradeIntent.TradeId,
			"symbol":       payload.TradeIntent.Symbol,
			"side":         payload.TradeIntent.Side.String(),
			"lot_size":     payload.TradeIntent.LotSize,
			"price":        payload.TradeIntent.Price,
			"client_id":    payload.TradeIntent.ClientId,
			"magic_number": payload.TradeIntent.MagicNumber,
		})

	case *pb.AgentMessage_ExecutionResult:
		a.logInfo("ExecutionResult sent to Core", map[string]interface{}{
			"command_id": payload.ExecutionResult.CommandId,
		})

	case *pb.AgentMessage_TradeClose:
		a.logInfo("TradeClose sent to Core", map[string]interface{}{
			"trade_id": payload.TradeClose.TradeId,
		})

	default:
		a.logDebug("Message sent to Core", map[string]interface{}{
			"type": fmt.Sprintf("%T", payload),
		})
	}
}

// NEW i2: sendAgentHello envía el handshake inicial (solo metadata).
func (a *Agent) sendAgentHello() error {
	hostname, _ := os.Hostname() // Excepción permitida por reglas

	hello := &pb.AgentHello{
		AgentId:  a.config.AgentID,
		Version:  a.config.ServiceVersion,
		Hostname: hostname,
		Os:       runtime.GOOS,
		Symbols:  make(map[string]*pb.SymbolInfo), // TODO i3: reportar símbolos
		// NO incluye owned_accounts ni connected_clients (deprecated i2)
		ProtocolVersion:          uint32(a.config.ProtocolMaxVersion),
		SupportsLosslessDelivery: true,
	}

	msg := &pb.AgentMessage{
		AgentId:     a.config.AgentID,
		TimestampMs: utils.NowUnixMilli(),
		Payload: &pb.AgentMessage_Hello{
			Hello: hello,
		},
	}

	if err := a.coreStream.Send(msg); err != nil {
		return fmt.Errorf("failed to send AgentHello: %w", err)
	}

	a.logInfo("AgentHello sent to Core (i2)", nil)
	return nil
}
