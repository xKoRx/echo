// Package internal contiene la lógica interna del Agent.
//
// El Agent actúa como bridge entre Named Pipes (EAs) y gRPC (Core).
// Es una thin layer que SOLO hace routing, usando SDK para toda la lógica.
package internal

import (
	"context"
	"fmt"
	"sync"

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

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Estado
	mu     sync.RWMutex
	closed bool
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

	agent := &Agent{
		config:       config,
		sendToCoreCh: make(chan *pb.AgentMessage, config.SendQueueSize),
		telemetry:    telClient,
		echoMetrics:  echoMetrics,
		ctx:          agentCtx,
		cancel:       cancel,
	}

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
	a.pipeManager = pipeManager

	// 4. Iniciar gestión de pipes (bloquea hasta ctx.Done)
	if err := a.pipeManager.Start(a.sendToCoreCh); err != nil {
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

// logError loggea un mensaje ERROR.
func (a *Agent) logError(message string, err error, fields map[string]interface{}) {
	attrs := mapToAttrs(fields)
	a.telemetry.Error(a.ctx, message, err, attrs...)
}

// logWarn loggea un mensaje WARN.
func (a *Agent) logWarn(message string, fields map[string]interface{}) {
	attrs := mapToAttrs(fields)
	a.telemetry.Warn(a.ctx, message, attrs...)
}
