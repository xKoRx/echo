package internal

import (
	"fmt"
	"os"

	pb "github.com/xKoRx/echo/sdk/pb/v1"
	"github.com/xKoRx/echo/sdk/telemetry/semconv"
	"github.com/xKoRx/echo/sdk/utils"
)

// connectToCore establece la conexión gRPC con el Core.
func (a *Agent) connectToCore() error {
	a.logInfo("Connecting to Core", map[string]interface{}{
		"address": a.config.CoreAddress,
	})

	// Crear cliente usando wrapper (que usa SDK)
	client, err := NewCoreClient(a.ctx, a.config.CoreAddress)
	if err != nil {
		return fmt.Errorf("failed to create core client: %w", err)
	}

	a.coreClient = client

	// Issue #C7: Generar agent-id único (usa hostname)
	agentID, err := generateAgentID()
	if err != nil {
		a.logWarn("Failed to get hostname, using fallback ID", map[string]interface{}{
			"error": err.Error(),
		})
		agentID = "agent-unknown"
	}

	// Crear stream bidireccional (envía agent-id en metadata)
	stream, err := client.StreamBidi(a.ctx, agentID)
	if err != nil {
		return fmt.Errorf("failed to create stream: %w", err)
	}

	a.coreStream = stream
	a.logInfo("Stream connected to Core", map[string]interface{}{
		"agent_id": agentID,
	})

	a.logInfo("Connected to Core", nil)
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

	default:
		a.logWarn("Unknown message type from Core", map[string]interface{}{
			"type": fmt.Sprintf("%T", payload),
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
	pipeName := fmt.Sprintf("%sslave_%s", a.config.PipePrefix, accountID)

	// Obtener handler del pipe
	handler, ok := a.pipeManager.GetPipe(pipeName)
	if !ok {
		return fmt.Errorf("pipe not found: %s (target: %s)", pipeName, order.TargetClientId)
	}

	// Escribir al pipe (transforma Proto → JSON internamente)
	if err := handler.WriteMessage(order); err != nil {
		return fmt.Errorf("failed to write to pipe: %w", err)
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
	pipeName := fmt.Sprintf("%sslave_%s", a.config.PipePrefix, accountID)

	// Obtener handler del pipe
	handler, ok := a.pipeManager.GetPipe(pipeName)
	if !ok {
		return fmt.Errorf("pipe not found: %s (target: %s)", pipeName, order.TargetClientId)
	}

	// Escribir al pipe (transforma Proto → JSON internamente)
	if err := handler.WriteMessage(order); err != nil {
		return fmt.Errorf("failed to write CloseOrder to pipe: %w", err)
	}

	a.logInfo("CloseOrder dispatched to Slave", map[string]interface{}{
		"command_id": order.CommandId,
		"trade_id":   order.TradeId,
		"pipe_name":  pipeName,
	})

	return nil
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
		a.logInfo("Message sent to Core", map[string]interface{}{
			"type": fmt.Sprintf("%T", payload),
		})
	}
}

// generateAgentID genera un ID único para el Agent basado en hostname.
//
// Issue #C7: Usa hostname del sistema para identificación predecible y única.
func generateAgentID() (string, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return "", fmt.Errorf("failed to get hostname: %w", err)
	}
	// Añadir sufijo UUID para evitar colisiones si corren múltiples Agents en el mismo host
	// Conserva el hostname para trazabilidad
	suffix := utils.GenerateUUIDv7()
	return fmt.Sprintf("agent_%s-%s", hostname, suffix), nil
}
