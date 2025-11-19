// Package internal contiene la lógica interna del Agent.
//
// El Agent actúa como bridge entre Named Pipes (EAs) y gRPC (Core).
// Es una thin layer que SOLO hace routing, usando SDK para toda la lógica.
package internal

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/xKoRx/echo/sdk/domain/handshake"
	pb "github.com/xKoRx/echo/sdk/pb/v1"
	"github.com/xKoRx/echo/sdk/telemetry"
	"github.com/xKoRx/echo/sdk/telemetry/metricbundle"
)

// Agent representa el servicio de Agent.
//
// Responsabilidades:
//   - Servidor de Named Pipes (1 pipe por EA)
//   - Cliente gRPC al Core (stream bidi persistente)
//   - Routing pipe ↔ stream (usando transformers de SDK)
//   - Telemetría (logs + métricas)
type Agent struct {
	// Config
	config *Config

	// gRPC
	coreClient   *CoreClient // Cliente gRPC wrapper (usa sdk/grpc)
	coreStream   pb.AgentService_StreamBidiClient
	sendToCoreCh chan *pb.AgentMessage // Canal para serializar envíos al Core

	// Named Pipes
	pipeManager *PipeManager // Gestión de pipes (usa sdk/ipc)

	// Telemetría
	telemetry   *telemetry.Client
	echoMetrics *metricbundle.EchoMetrics
	delivery    *DeliveryManager
	// Snapshot de configuración de delivery para masters
	masterDeliveryConfig *MasterDeliveryConfig

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Estado
	mu     sync.RWMutex
	closed bool

	handshakeMu      sync.RWMutex
	handshakeStatus  map[string]handshake.RegistrationStatus
	evaluationMu     sync.RWMutex
	lastEvaluationID map[string]string
}

// New crea una nueva instancia de Agent (i1).
//
// Config se carga desde ETCD automáticamente.
//
// Example:
//
//	agent, err := internal.New(ctx)
//	if err != nil {
//	    return err
//	}
//	defer agent.Shutdown()
func New(ctx context.Context) (*Agent, error) {
	// Crear contexto cancelable
	agentCtx, cancel := context.WithCancel(ctx)

	// i1: Cargar configuración desde ETCD
	config, err := LoadConfig(agentCtx)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to load config from ETCD: %w", err)
	}

	// Inicializar telemetría
	telClient, err := initTelemetry(agentCtx, config)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to init telemetry: %w", err)
	}

	// Obtener bundle EchoMetrics
	echoMetrics := telClient.EchoMetrics()
	if echoMetrics == nil {
		cancel()
		_ = telClient.Shutdown(agentCtx)
		return nil, fmt.Errorf("failed to get EchoMetrics bundle")
	}

	initialBackoff := durationsToUint32(config.Delivery.RetryBackoff)
	maxRetries := config.Delivery.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 100
	}
	masterDeliveryConfig := NewMasterDeliveryConfig(initialBackoff, uint32(maxRetries))

	agent := &Agent{
		config:               config,
		sendToCoreCh:         make(chan *pb.AgentMessage, config.SendQueueSize),
		telemetry:            telClient,
		echoMetrics:          echoMetrics,
		ctx:                  agentCtx,
		cancel:               cancel,
		handshakeStatus:      make(map[string]handshake.RegistrationStatus),
		lastEvaluationID:     make(map[string]string),
		masterDeliveryConfig: masterDeliveryConfig,
	}

	delivery, err := NewDeliveryManager(agentCtx, config, telClient, agent.sendToCoreCh)
	if err != nil {
		cancel()
		_ = telClient.Shutdown(agentCtx)
		return nil, fmt.Errorf("failed to init delivery manager: %w", err)
	}
	agent.delivery = delivery

	return agent, nil
}

// Start inicia el Agent.
//
// Secuencia:
//  1. Conectar a Core via gRPC
//  2. Iniciar goroutines de stream (send/receive)
//  3. Crear Named Pipes y esperar conexiones de EAs
//
// Bloquea hasta que ctx se cancele o haya error fatal.
func (a *Agent) Start() error {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return fmt.Errorf("agent already closed")
	}
	a.mu.Unlock()

	a.logInfo("Agent starting", map[string]interface{}{
		"core_address": a.config.CoreAddress,
		"version":      a.config.ServiceVersion,
	})

	// 1. Conectar a Core via gRPC
	if err := a.connectToCore(); err != nil {
		return fmt.Errorf("failed to connect to core: %w", err)
	}

	// 2. Iniciar goroutines de gRPC stream
	a.wg.Add(2)
	go a.sendToCore()
	go a.receiveFromCore()

	// 3. Iniciar PipeManager
	pipeManager, err := NewPipeManager(a.ctx, a.config, a.telemetry, a.echoMetrics)
	if err != nil {
		return fmt.Errorf("failed to create pipe manager: %w", err)
	}
	pipeManager.SetHandshakeCallback(a.markHandshakePending)
	if a.delivery != nil {
		pipeManager.SetDeliveryManager(a.delivery)
		a.delivery.SetPipeLookup(func(accountID string) (*PipeHandler, bool) {
			pipeName := fmt.Sprintf("%sslave_%s", a.config.PipePrefix, accountID)
			return pipeManager.GetPipe(pipeName)
		})
	}
	if a.masterDeliveryConfig != nil {
		pipeManager.SetMasterDeliveryConfig(a.masterDeliveryConfig)
	}
	a.pipeManager = pipeManager

	// 4. Iniciar gestión de pipes (bloquea hasta ctx.Done)
	if err := a.pipeManager.Start(a.sendToCoreCh, a.config.AgentID); err != nil {
		return fmt.Errorf("pipe manager failed: %w", err)
	}

	a.logInfo("Agent started successfully", nil)

	// Esperar señal de shutdown
	<-a.ctx.Done()

	a.logInfo("Agent shutting down", nil)
	return nil
}

// Shutdown detiene el Agent gracefully.
func (a *Agent) Shutdown() error {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return nil
	}
	a.closed = true
	a.mu.Unlock()

	a.logInfo("Agent shutdown initiated", nil)

	// 1. Cancelar contexto (detiene goroutines)
	a.cancel()

	// 2. Cerrar pipe manager
	if a.pipeManager != nil {
		_ = a.pipeManager.Close()
	}

	if a.delivery != nil {
		a.delivery.Close()
	}

	// 3. Cerrar gRPC
	if a.coreClient != nil {
		_ = a.coreClient.Close()
	}

	// 4. Cerrar canal de envío
	close(a.sendToCoreCh)

	// 5. Esperar goroutines
	a.wg.Wait()

	// 6. Shutdown telemetría
	shutdownCtx := context.Background()
	if err := a.telemetry.Shutdown(shutdownCtx); err != nil {
		a.logError("Failed to shutdown telemetry", err, nil)
	}

	a.logInfo("Agent stopped", nil)
	return nil
}

// logInfo loggea un mensaje INFO.
func (a *Agent) logInfo(message string, fields map[string]interface{}) {
	attrs := mapToAttrs(fields)
	a.telemetry.Info(a.ctx, message, attrs...)
}

func (a *Agent) recordPipeDeliveryLatency(accountID, result string, duration time.Duration) {
	if a.echoMetrics == nil {
		return
	}
	a.echoMetrics.RecordAgentPipeDeliveryLatency(
		a.ctx,
		a.config.AgentID,
		accountID,
		result,
		duration,
	)
}

func (a *Agent) logDebug(message string, fields map[string]interface{}) {
	attrs := mapToAttrs(fields)
	a.telemetry.Debug(a.ctx, message, attrs...)
}

// logError loggea un mensaje ERROR.
func (a *Agent) logError(message string, err error, fields map[string]interface{}) {
	attrs := mapToAttrs(fields)

	if err != nil {
		a.telemetry.Error(a.ctx, message, err, attrs...)
	} else {
		a.telemetry.Error(a.ctx, message, fmt.Errorf("unknown error"), attrs...)
	}
}

func (a *Agent) setHandshakeStatus(accountID string, status handshake.RegistrationStatus) {
	if accountID == "" {
		return
	}
	a.handshakeMu.Lock()
	a.handshakeStatus[accountID] = status
	a.handshakeMu.Unlock()
}

func (a *Agent) setLastEvaluationID(accountID, evaluationID string) {
	if accountID == "" || evaluationID == "" {
		return
	}
	a.evaluationMu.Lock()
	a.lastEvaluationID[accountID] = evaluationID
	a.evaluationMu.Unlock()
}

func (a *Agent) getLastEvaluationID(accountID string) string {
	if accountID == "" {
		return ""
	}
	a.evaluationMu.RLock()
	evaluationID := a.lastEvaluationID[accountID]
	a.evaluationMu.RUnlock()
	return evaluationID
}

func (a *Agent) getHandshakeStatus(accountID string) handshake.RegistrationStatus {
	if accountID == "" {
		return handshake.RegistrationStatusUnspecified
	}
	a.handshakeMu.RLock()
	status, ok := a.handshakeStatus[accountID]
	a.handshakeMu.RUnlock()
	if !ok {
		return handshake.RegistrationStatusUnspecified
	}
	return status
}

func (a *Agent) markHandshakePending(accountID string) {
	a.setHandshakeStatus(accountID, handshake.RegistrationStatusUnspecified)
	a.clearLastEvaluationID(accountID)
}

func (a *Agent) clearLastEvaluationID(accountID string) {
	if accountID == "" {
		return
	}
	a.evaluationMu.Lock()
	delete(a.lastEvaluationID, accountID)
	a.evaluationMu.Unlock()
}

func (a *Agent) updateMasterDeliveryConfig(hb *pb.DeliveryHeartbeat) {
	if hb == nil || a.masterDeliveryConfig == nil {
		return
	}
	var backoff []uint32
	if len(hb.RetryBackoffMs) > 0 {
		backoff = append([]uint32(nil), hb.RetryBackoffMs...)
	}
	maxRetries := uint32(hb.MaxRetries)
	if a.masterDeliveryConfig.Update(backoff, maxRetries) && a.pipeManager != nil {
		snapshotBackoff, snapshotMax := a.masterDeliveryConfig.Snapshot()
		a.pipeManager.BroadcastDeliveryConfig(snapshotBackoff, snapshotMax)
	}
}

func durationsToUint32(values []time.Duration) []uint32 {
	if len(values) == 0 {
		return nil
	}
	result := make([]uint32, 0, len(values))
	for _, d := range values {
		if d <= 0 {
			continue
		}
		result = append(result, uint32(d.Milliseconds()))
	}
	return result
}

// logWarn loggea un mensaje WARN.
func (a *Agent) logWarn(message string, fields map[string]interface{}) {
	attrs := mapToAttrs(fields)
	a.telemetry.Warn(a.ctx, message, attrs...)
}
