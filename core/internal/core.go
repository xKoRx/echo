// Package internal contiene la lógica interna del Core.
//
// El Core actúa como orquestador central del sistema Echo.
// Es una thin layer que SOLO hace orquestación y routing, usando SDK para toda la lógica.
package internal

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/xKoRx/echo/core/internal/repository"
	"github.com/xKoRx/echo/sdk/domain"
	pb "github.com/xKoRx/echo/sdk/pb/v1"
	"github.com/xKoRx/echo/sdk/telemetry"
	"github.com/xKoRx/echo/sdk/telemetry/metricbundle"
	"github.com/xKoRx/echo/sdk/telemetry/semconv"
	"github.com/xKoRx/echo/sdk/utils"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
)

// Core representa el servicio principal de Echo Core.
//
// Responsabilidades i1:
//   - Servidor gRPC bidi con KeepAlive (acepta streams de Agents)
//   - Validación de TradeIntents (usando SDK)
//   - Deduplicación PERSISTENTE (PostgreSQL + TTL cleanup)
//   - Transformación TradeIntent → ExecuteOrder (usando SDK)
//   - Routing de ExecuteOrders a Agents
//   - Persistencia de trades/executions/closes
//   - Correlación trade_id ↔ tickets por slave
//   - Telemetría (logs + métricas + trazas)
type Core struct {
	// Embedder para implementar AgentServiceServer
	pb.UnimplementedAgentServiceServer

	// Config (cargada desde ETCD)
	config *Config

	// gRPC Server
	grpcServer *grpc.Server
	listener   net.Listener

	// Agent connections
	agents   map[string]*AgentConnection // key: agent_id
	agentsMu sync.RWMutex

	// PostgreSQL
	db *sql.DB

	// Repositories (i1)
	repoFactory    domain.RepositoryFactory
	correlationSvc domain.CorrelationService

	// Deduplicación persistente (i1)
	dedupeService *DedupeService

	// Router/Processor
	router *Router

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

// DEPRECATED: Config moved to config.go (i1).
// Use LoadConfig() instead of DefaultConfig().
//
// Kept for backwards compatibility during migration.
func DefaultConfig() *Config {
	return nil // Deprecated in i1
}

// AgentConnection representa una conexión de Agent al Core.
type AgentConnection struct {
	AgentID   string
	Stream    pb.AgentService_StreamBidiServer
	SendCh    chan *pb.CoreMessage // Canal para serializar envíos
	ctx       context.Context
	cancel    context.CancelFunc
	createdAt time.Time
}

// New crea una nueva instancia de Core (i1).
//
// Cambios i1:
//   - Config cargada desde ETCD (no manual)
//   - PostgreSQL para persistencia
//   - Dedupe service persistente
//   - Repositories y CorrelationService
//
// Example:
//
//	core, err := internal.New(ctx)
//	if err != nil {
//	    return err
//	}
//	defer core.Shutdown()
func New(ctx context.Context) (*Core, error) {
	// Crear contexto cancelable
	coreCtx, cancel := context.WithCancel(ctx)

	// 1. Cargar configuración desde ETCD
	config, err := LoadConfig(coreCtx)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to load config from ETCD: %w", err)
	}

	// 2. Conectar a PostgreSQL
	db, err := sql.Open("postgres", config.PostgresConnStr())
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to open postgres connection: %w", err)
	}

	// Verificar conexión
	if err := db.PingContext(coreCtx); err != nil {
		cancel()
		db.Close()
		return nil, fmt.Errorf("failed to ping postgres: %w", err)
	}

	// Configurar pool
	db.SetMaxOpenConns(config.PostgresPoolMaxConn)
	db.SetMaxIdleConns(config.PostgresPoolMinConn)
	db.SetConnMaxLifetime(1 * time.Hour)

	// 3. Crear repository factory
	repoFactory := repository.NewPostgresFactory(db)
	correlationSvc := repoFactory.CorrelationService()

	// 4. Crear dedupe service persistente (i1)
	dedupeService := NewDedupeService(repoFactory.DedupeRepository())

	// 5. Inicializar telemetría usando SDK
	telOpts := []telemetry.Option{
		telemetry.WithVersion(config.ServiceVersion),
	}
	if config.OTLPEndpoint != "" {
		telOpts = append(telOpts, telemetry.WithOTLPEndpoint(config.OTLPEndpoint))
	}
	if config.MetricsEndpoint != "" {
		telOpts = append(telOpts, telemetry.WithMetricsEndpoint(config.MetricsEndpoint))
	}

	telClient, err := telemetry.New(
		coreCtx,
		config.ServiceName,
		config.Environment,
		telOpts...,
	)
	if err != nil {
		cancel()
		db.Close()
		return nil, fmt.Errorf("failed to initialize telemetry: %w", err)
	}

	// Obtener bundle de métricas Echo
	echoMetrics := telClient.EchoMetrics()
	if echoMetrics == nil {
		cancel()
		telClient.Shutdown(coreCtx)
		db.Close()
		return nil, fmt.Errorf("failed to get EchoMetrics bundle")
	}

	// Configurar atributos comunes en contexto
	coreCtx = telemetry.AppendCommonAttrs(coreCtx,
		semconv.Echo.Component.String(semconv.ComponentValues.Core),
	)

	// 6. Crear Core
	core := &Core{
		config:         config,
		db:             db,
		repoFactory:    repoFactory,
		correlationSvc: correlationSvc,
		dedupeService:  dedupeService,
		agents:         make(map[string]*AgentConnection),
		telemetry:      telClient,
		echoMetrics:    echoMetrics,
		ctx:            coreCtx,
		cancel:         cancel,
	}

	// 7. Crear router
	core.router = NewRouter(core)

	// Log de inicio
	telClient.Info(coreCtx, "Core initialized (i1)",
		attribute.Int("grpc_port", config.GRPCPort),
		attribute.StringSlice("symbol_whitelist", config.SymbolWhitelist),
		attribute.Float64("default_lot_size", config.DefaultLotSize),
		attribute.String("postgres_host", config.PostgresHost),
		attribute.String("postgres_database", config.PostgresDatabase),
	)

	return core, nil
}

// Start inicia el Core (servidor gRPC con KeepAlive, router, dedupe cleanup persistente).
func (c *Core) Start() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return fmt.Errorf("core already closed")
	}
	c.mu.Unlock()

	// Crear listener TCP
	addr := fmt.Sprintf(":%d", c.config.GRPCPort)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}
	c.listener = lis

	// Crear servidor gRPC con KeepAlive (i1 - RFC-003 sección 7)
	kaParams := keepalive.ServerParameters{
		Time:    c.config.KeepAliveTime,    // default: 60s
		Timeout: c.config.KeepAliveTimeout, // default: 20s
	}
	kaPolicy := keepalive.EnforcementPolicy{
		MinTime:             c.config.KeepAliveMinTime, // default: 10s
		PermitWithoutStream: false,                     // no pings sin stream activo
	}

	c.grpcServer = grpc.NewServer(
		grpc.KeepaliveParams(kaParams),
		grpc.KeepaliveEnforcementPolicy(kaPolicy),
		// TODO i2: agregar interceptors de telemetría
	)

	// Registrar servicio
	pb.RegisterAgentServiceServer(c.grpcServer, c)

	c.telemetry.Info(c.ctx, "gRPC server listening (i1)",
		attribute.String("address", addr),
		attribute.String("keepalive_time", c.config.KeepAliveTime.String()),
		attribute.String("keepalive_timeout", c.config.KeepAliveTimeout.String()),
		attribute.String("keepalive_min_time", c.config.KeepAliveMinTime.String()),
	)

	// Arrancar servidor gRPC en goroutine
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		if err := c.grpcServer.Serve(lis); err != nil {
			c.telemetry.Error(c.ctx, "gRPC server failed", err,
				attribute.String("error", err.Error()),
			)
		}
	}()

	// Arrancar router
	if err := c.router.Start(); err != nil {
		return fmt.Errorf("failed to start router: %w", err)
	}

	// Arrancar dedupe cleanup persistente (i1)
	c.wg.Add(1)
	go c.dedupeCleanupLoop()

	c.telemetry.Info(c.ctx, "Core started successfully (i1)")

	return nil
}

// StreamBidi implementa AgentServiceServer.
//
// Maneja el stream bidireccional con un Agent.
func (c *Core) StreamBidi(stream pb.AgentService_StreamBidiServer) error {
	ctx := stream.Context()

	// Issue #C7: Extraer agent_id de metadata gRPC
	agentID, err := extractAgentIDFromMetadata(ctx)
	if err != nil {
		c.telemetry.Warn(c.ctx, "Agent connected without agent-id metadata, using generated ID",
			attribute.String("error", err.Error()),
		)
		// Fallback: generar ID único
		agentID = fmt.Sprintf("agent_%s", utils.GenerateUUIDv7())
	}

	c.telemetry.Info(c.ctx, "Agent connected",
		attribute.String("agent_id", agentID),
	)

	// Crear conexión
	agentCtx, agentCancel := context.WithCancel(ctx)
	conn := &AgentConnection{
		AgentID:   agentID,
		Stream:    stream,
		SendCh:    make(chan *pb.CoreMessage, 1000), // Issue #C8: aumentar buffer de 100 a 1000
		ctx:       agentCtx,
		cancel:    agentCancel,
		createdAt: time.Now(),
	}

	// Registrar agent
	c.registerAgent(agentID, conn)
	defer func() {
		c.unregisterAgent(agentID)
		agentCancel()
		close(conn.SendCh)
	}()

	// Goroutine de escritura (envía CoreMessages al Agent)
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.sendToAgentLoop(conn)
	}()

	// Goroutine de lectura (recibe AgentMessages del Agent)
	for {
		msg, err := stream.Recv()
		if err != nil {
			c.telemetry.Warn(c.ctx, "Agent disconnected",
				attribute.String("agent_id", agentID),
				attribute.String("error", err.Error()),
			)
			return err
		}

		// Enviar al router para procesamiento
		c.router.HandleAgentMessage(agentCtx, agentID, msg)
	}
}

// Ping implementa AgentServiceServer.
func (c *Core) Ping(ctx context.Context, req *pb.PingRequest) (*pb.PingResponse, error) {
	return &pb.PingResponse{
		Status:      "ok",
		TimestampMs: time.Now().UnixMilli(),
	}, nil
}

// sendToAgentLoop envía mensajes al Agent desde el canal.
func (c *Core) sendToAgentLoop(conn *AgentConnection) {
	for {
		select {
		case msg, ok := <-conn.SendCh:
			if !ok {
				return // Canal cerrado
			}

			if err := conn.Stream.Send(msg); err != nil {
				c.telemetry.Error(c.ctx, "Failed to send to agent", err,
					attribute.String("agent_id", conn.AgentID),
					attribute.String("error", err.Error()),
				)
				return
			}

		case <-conn.ctx.Done():
			return
		}
	}
}

// registerAgent registra un nuevo agent conectado.
func (c *Core) registerAgent(agentID string, conn *AgentConnection) {
	c.agentsMu.Lock()
	defer c.agentsMu.Unlock()
	c.agents[agentID] = conn

	c.telemetry.Info(c.ctx, "Agent registered",
		attribute.String("agent_id", agentID),
		attribute.Int("total_agents", len(c.agents)),
	)
}

// unregisterAgent elimina un agent desconectado.
func (c *Core) unregisterAgent(agentID string) {
	c.agentsMu.Lock()
	defer c.agentsMu.Unlock()
	delete(c.agents, agentID)

	c.telemetry.Info(c.ctx, "Agent unregistered",
		attribute.String("agent_id", agentID),
		attribute.Int("total_agents", len(c.agents)),
	)
}

// GetAgents retorna lista de agents conectados (para routing).
func (c *Core) GetAgents() []*AgentConnection {
	c.agentsMu.RLock()
	defer c.agentsMu.RUnlock()

	agents := make([]*AgentConnection, 0, len(c.agents))
	for _, conn := range c.agents {
		agents = append(agents, conn)
	}
	return agents
}

// dedupeCleanupLoop limpia entries antiguos del dedupe store (persistente i1).
func (c *Core) dedupeCleanupLoop() {
	defer c.wg.Done()

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Usar DedupeService que llama a la función SQL cleanup_dedupe_ttl()
			removed, err := c.dedupeService.Cleanup(c.ctx)
			if err != nil {
				c.telemetry.Error(c.ctx, "Dedupe cleanup failed", err,
					attribute.String("error", err.Error()),
				)
				continue
			}

			if removed > 0 {
				c.telemetry.Info(c.ctx, "Dedupe cleanup completed (i1)",
					attribute.Int("removed_entries", removed),
				)
			}

		case <-c.ctx.Done():
			return
		}
	}
}

// Shutdown detiene el Core gracefully (i1).
func (c *Core) Shutdown() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.mu.Unlock()

	c.telemetry.Info(c.ctx, "Core shutting down (i1)...")

	// Detener contexto
	c.cancel()

	// Detener router
	if c.router != nil {
		c.router.Stop()
	}

	// Cerrar conexiones de agents
	c.agentsMu.Lock()
	for _, conn := range c.agents {
		conn.cancel()
	}
	c.agentsMu.Unlock()

	// Detener servidor gRPC
	if c.grpcServer != nil {
		c.grpcServer.GracefulStop()
	}

	// Esperar goroutines
	c.wg.Wait()

	// Cerrar conexión PostgreSQL (i1)
	if c.db != nil {
		if err := c.db.Close(); err != nil {
			c.telemetry.Error(c.ctx, "Failed to close PostgreSQL connection", err)
		} else {
			c.telemetry.Info(c.ctx, "PostgreSQL connection closed")
		}
	}

	// Shutdown telemetría
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := c.telemetry.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("failed to shutdown telemetry: %w", err)
	}

	c.telemetry.Info(c.ctx, "Core stopped successfully (i1)")

	return nil
}

// extractAgentIDFromMetadata extrae agent-id de los metadatos gRPC.
//
// Issue #C7: El Agent debe enviar su ID en metadata para evitar colisiones.
func extractAgentIDFromMetadata(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", fmt.Errorf("no metadata in context")
	}

	agentIDSlice := md.Get("agent-id")
	if len(agentIDSlice) == 0 {
		return "", fmt.Errorf("agent-id not found in metadata")
	}

	agentID := agentIDSlice[0]
	if agentID == "" {
		return "", fmt.Errorf("agent-id is empty")
	}

	return agentID, nil
}
