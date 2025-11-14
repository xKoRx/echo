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

	"github.com/xKoRx/echo/core/capabilities"
	"github.com/xKoRx/echo/core/internal/repository"
	"github.com/xKoRx/echo/core/internal/riskengine"
	"github.com/xKoRx/echo/core/internal/volumeguard"
	"github.com/xKoRx/echo/sdk/domain"
	"github.com/xKoRx/echo/sdk/domain/handshake"
	"github.com/xKoRx/echo/sdk/etcd"
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

	// i2: Registry de ownership de cuentas (estado operacional en memoria)
	accountRegistry *AccountRegistry

	// PostgreSQL
	db *sql.DB

	// ETCD client for runtime lookups
	etcdClient *etcd.Client

	// Repositories (i1)
	repoFactory    domain.RepositoryFactory
	correlationSvc domain.CorrelationService

	// Deduplicación persistente (i1)
	dedupeService *DedupeService

	// i4: Políticas de riesgo y guardián de volumen
	riskPolicyService domain.RiskPolicyService
	volumeGuard       volumeguard.Guard

	// i3: Validación y resolución de símbolos
	canonicalValidator  *CanonicalValidator
	symbolResolver      *AccountSymbolResolver
	symbolSpecService   *SymbolSpecService
	symbolQuoteService  *SymbolQuoteService
	accountStateService *AccountStateService
	riskEngine          *riskengine.FixedRiskEngine
	stopLevelGuard      capabilities.StopLevelGuard

	// Router/Processor
	router *Router

	// Telemetría
	telemetry   *telemetry.Client
	echoMetrics *metricbundle.EchoMetrics

	// Handshake
	handshakeEvaluator  *HandshakeEvaluator
	handshakeRegistry   *HandshakeRegistry
	handshakeReconciler *handshakeReconciler

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
	offsetClient, err := etcd.New(
		etcd.WithApp("echo"),
		etcd.WithEnv(config.Environment),
		etcd.WithEndpointsFromEnv(),
	)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create etcd client: %w", err)
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
	if config.LogLevel != "" {
		telOpts = append(telOpts, telemetry.WithLogLevel(config.LogLevel))
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

	// 6. Crear validadores y resolvers de símbolos (i3)
	unknownAction := UnknownActionWarn
	if config.UnknownAction == "reject" {
		unknownAction = UnknownActionReject
	}
	canonicalValidator := NewCanonicalValidator(config.CanonicalSymbols, unknownAction, telClient, echoMetrics)
	symbolResolver := NewAccountSymbolResolver(coreCtx, repoFactory.SymbolRepository(), telClient, echoMetrics, 1000)
	symbolResolver.Start() // Iniciar worker de persistencia async
	symbolSpecService := NewSymbolSpecService(repoFactory.SymbolSpecRepository(), telClient, echoMetrics, config.CanonicalSymbols)
	symbolQuoteService := NewSymbolQuoteService(repoFactory.SymbolQuoteRepository(), telClient)
	accountStateService := NewAccountStateService(telClient)
	riskPolicySvc := NewRiskPolicyService(repoFactory.RiskPolicyRepository(), config.Risk.CacheTTL, telClient, echoMetrics, offsetClient)
	volumeGuard := volumeguard.New(symbolSpecService, config.VolumeGuard, telClient, echoMetrics)
	riskEngineCfg := riskengine.Config{
		MaxQuoteAge:              config.Risk.Engine.QuoteMaxAge,
		MinDistancePoints:        config.Risk.Engine.MinDistancePoints,
		MaxRiskDrift:             config.Risk.Engine.MaxRiskDrift,
		DefaultCurrency:          config.Risk.Engine.DefaultCurrency,
		EnableCurrencyFallback:   config.Risk.Engine.EnableCurrencyFallback,
		RejectOnMissingTickValue: config.Risk.Engine.RejectOnMissingTickValue,
	}
	fixedRiskEngine := riskengine.NewFixedRiskEngine(symbolSpecService, symbolQuoteService, accountStateService, &symbolInfoAdapter{resolver: symbolResolver}, volumeGuard, riskEngineCfg, telClient, echoMetrics)

	var stopGuard capabilities.StopLevelGuard
	if config.EnableStopLevelGuard {
		stopGuard = NewStopLevelGuard(telClient, echoMetrics)
	}

	if rs, ok := riskPolicySvc.(*riskPolicyService); ok {
		if err := rs.StartListener(coreCtx, config.PostgresConnStr()); err != nil {
			telClient.Warn(coreCtx, "Failed to start risk policy listener",
				attribute.String("error", err.Error()),
			)
		}
	}

	// Registrar carga inicial de símbolos canónicos desde ETCD
	echoMetrics.RecordSymbolsLoaded(coreCtx, "etcd", len(config.CanonicalSymbols),
		attribute.StringSlice("canonical_symbols", config.CanonicalSymbols),
	)

	// Handshake evaluator y registry
	handshakeRepo := repoFactory.HandshakeRepository()
	handshakeRegistry := NewHandshakeRegistry()
	var specMaxAge time.Duration
	if config.VolumeGuard != nil {
		specMaxAge = config.VolumeGuard.MaxSpecAge
	}
	protocolValidator := NewProtocolValidator(&config.Protocol)
	handshakeEvaluator := NewHandshakeEvaluator(
		protocolValidator,
		canonicalValidator,
		symbolSpecService,
		riskPolicySvc,
		handshakeRepo,
		telClient,
		echoMetrics,
		&config.Protocol,
		specMaxAge,
	)

	// 7. Crear Core
	core := &Core{
		config:              config,
		db:                  db,
		etcdClient:          offsetClient,
		repoFactory:         repoFactory,
		correlationSvc:      correlationSvc,
		dedupeService:       dedupeService,
		riskPolicyService:   riskPolicySvc,
		volumeGuard:         volumeGuard,
		canonicalValidator:  canonicalValidator, // NEW i3
		symbolResolver:      symbolResolver,     // NEW i3
		symbolSpecService:   symbolSpecService,
		symbolQuoteService:  symbolQuoteService,
		accountStateService: accountStateService,
		riskEngine:          fixedRiskEngine,
		stopLevelGuard:      stopGuard,
		agents:              make(map[string]*AgentConnection),
		accountRegistry:     NewAccountRegistry(telClient), // NEW i2
		telemetry:           telClient,
		echoMetrics:         echoMetrics,
		ctx:                 coreCtx,
		cancel:              cancel,
		handshakeEvaluator:  handshakeEvaluator,
		handshakeRegistry:   handshakeRegistry,
	}

	core.handshakeReconciler = newHandshakeReconciler(
		coreCtx,
		handshakeEvaluator,
		symbolResolver,
		handshakeRegistry,
		core.accountRegistry,
		handshakeRepo,
		telClient,
		echoMetrics,
		config.ServiceVersion,
		func(agentID string, result *pb.SymbolRegistrationResult) error {
			return core.sendSymbolRegistrationResult(coreCtx, agentID, result)
		},
	)

	symbolSpecService.SetOnChange(func(accountID string) {
		core.handshakeReconciler.Notify(accountID)
	})
	if rs, ok := riskPolicySvc.(*riskPolicyService); ok {
		rs.SetOnInvalidate(func(accountID string) {
			core.handshakeReconciler.Notify(accountID)
		})
	}
	if err := core.handshakeReconciler.StartListener(config.PostgresConnStr()); err != nil {
		telClient.Warn(coreCtx, "Failed to start handshake listener",
			attribute.String("error", err.Error()),
		)
	}

	// 8. Crear router
	core.router = NewRouter(core)

	// Log de inicio
	telClient.Info(coreCtx, "Core initialized (i3)",
		attribute.Int("grpc_port", config.GRPCPort),
		attribute.StringSlice("canonical_symbols", config.CanonicalSymbols),
		attribute.StringSlice("symbol_whitelist", config.SymbolWhitelist), // Deprecated pero mantenido
		attribute.String("unknown_action", config.UnknownAction),
		attribute.Float64("default_lot_size", config.DefaultLotSize),
		attribute.String("postgres_host", config.PostgresHost),
		attribute.String("postgres_database", config.PostgresDatabase),
		attribute.String("log_level", config.LogLevel),
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

	if c.handshakeReconciler != nil {
		c.handshakeReconciler.Start()
	}

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
		c.accountRegistry.UnregisterAgent(agentID) // NEW i2: limpiar registry
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

		// NEW i2: Procesar AgentHello para logging (sin ownership)
		if hello := msg.GetHello(); hello != nil {
			c.handleAgentHello(agentID, hello)
			continue // No enviar al router
		}

		// NEW i2: Procesar AccountConnected
		if accountConn := msg.GetAccountConnected(); accountConn != nil {
			c.handleAccountConnected(agentID, accountConn)
			continue
		}

		// NEW i2: Procesar AccountDisconnected
		if accountDisconn := msg.GetAccountDisconnected(); accountDisconn != nil {
			c.handleAccountDisconnected(agentID, accountDisconn)
			continue
		}

		// NEW i3: Procesar AccountSymbolsReport
		if symbolsReport := msg.GetAccountSymbolsReport(); symbolsReport != nil {
			c.handleAccountSymbolsReport(agentCtx, agentID, symbolsReport)
			continue
		}

		// NEW i3+: Procesar reportes de especificaciones
		if specReport := msg.GetSymbolSpecReport(); specReport != nil {
			c.handleSymbolSpecReport(agentCtx, agentID, specReport)
			continue
		}

		// NEW i3+: Procesar snapshots de precios
		if quoteSnapshot := msg.GetSymbolQuoteSnapshot(); quoteSnapshot != nil {
			c.handleSymbolQuoteSnapshot(agentCtx, agentID, quoteSnapshot)
			continue
		}

		// Enviar al router para procesamiento
		c.router.HandleAgentMessage(agentCtx, agentID, msg)
	}
}

func (c *Core) sendSymbolRegistrationResult(ctx context.Context, agentID string, result *pb.SymbolRegistrationResult) error {
	if agentID == "" || result == nil {
		return fmt.Errorf("datos inválidos para enviar handshake")
	}

	conn, ok := c.GetAgent(agentID)
	if !ok {
		return fmt.Errorf("agent %s no conectado", agentID)
	}

	msg := &pb.CoreMessage{
		TimestampMs: utils.NowUnixMilli(),
		Payload: &pb.CoreMessage_SymbolRegistrationResult{
			SymbolRegistrationResult: result,
		},
	}

	select {
	case conn.SendCh <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return fmt.Errorf("canal de envío lleno para agent %s", agentID)
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

// GetAgent retorna un Agent específico por ID (i2 helper).
func (c *Core) GetAgent(agentID string) (*AgentConnection, bool) {
	c.agentsMu.RLock()
	defer c.agentsMu.RUnlock()

	conn, exists := c.agents[agentID]
	return conn, exists
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

	if c.handshakeReconciler != nil {
		c.handshakeReconciler.Stop()
	}

	// i3: Detener symbol resolver
	if c.symbolResolver != nil {
		c.symbolResolver.Stop()
	}

	// Detener listener de políticas de riesgo
	if rs, ok := c.riskPolicyService.(*riskPolicyService); ok {
		rs.StopListener()
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

	if c.etcdClient != nil {
		if err := c.etcdClient.Close(); err != nil {
			c.telemetry.Warn(c.ctx, "Failed to close ETCD client", attribute.String("error", err.Error()))
		} else {
			c.telemetry.Info(c.ctx, "ETCD client closed")
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

// EvaluateHandshakeForAccount fuerza una re-evaluación de handshake para una cuenta específica.
// send indica si el resultado debe reenviarse al Agent conectado.
func (c *Core) EvaluateHandshakeForAccount(ctx context.Context, accountID string, send bool) (*handshake.Evaluation, error) {
	if accountID == "" {
		return nil, fmt.Errorf("accountID vacío")
	}
	if c.handshakeReconciler == nil {
		return nil, fmt.Errorf("handshake reconciler no inicializado")
	}
	if ctx == nil {
		ctx = c.ctx
	}
	return c.handshakeReconciler.EvaluateNow(ctx, accountID, send)
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

// NEW i2: handleAgentHello procesa el handshake inicial del Agent (solo metadata).
func (c *Core) handleAgentHello(agentID string, hello *pb.AgentHello) {
	c.telemetry.Info(c.ctx, "AgentHello received (i2)",
		attribute.String("agent_id", agentID),
		attribute.String("version", hello.Version),
		attribute.String("hostname", hello.Hostname),
		attribute.String("os", hello.Os),
	)
	// No registra cuentas aquí; eso se hace dinámicamente con AccountConnected
}

// NEW i2: handleAccountConnected procesa la conexión de una cuenta al Agent.
func (c *Core) handleAccountConnected(agentID string, msg *pb.AccountConnected) {
	// Validar usando SDK
	if err := domain.ValidateAccountConnected(msg); err != nil {
		c.telemetry.Warn(c.ctx, "Invalid AccountConnected message (i2)",
			attribute.String("agent_id", agentID),
			attribute.String("error", err.Error()),
		)
		return
	}

	accountID := msg.AccountId

	clientType := "unknown"
	if msg.ClientType != nil {
		clientType = *msg.ClientType
	}

	c.telemetry.Info(c.ctx, "AccountConnected received (i2)",
		attribute.String("agent_id", agentID),
		attribute.String("account_id", accountID),
		attribute.String("client_type", clientType),
	)

	// Registrar cuenta en registry
	c.accountRegistry.RegisterAccount(agentID, accountID, clientType)

	// Métrica: número de cuentas registradas
	totalAccounts, totalAgents := c.accountRegistry.GetStats()
	c.telemetry.Info(c.ctx, "Account registry updated (i2)",
		attribute.Int("total_accounts", totalAccounts),
		attribute.Int("total_agents", totalAgents),
	)
}

// NEW i2: handleAccountDisconnected procesa la desconexión de una cuenta del Agent.
func (c *Core) handleAccountDisconnected(agentID string, msg *pb.AccountDisconnected) {
	// Validar usando SDK
	if err := domain.ValidateAccountDisconnected(msg); err != nil {
		c.telemetry.Warn(c.ctx, "Invalid AccountDisconnected message (i2)",
			attribute.String("agent_id", agentID),
			attribute.String("error", err.Error()),
		)
		return
	}

	accountID := msg.AccountId

	reason := "unknown"
	if msg.Reason != nil {
		reason = *msg.Reason
	}

	c.telemetry.Info(c.ctx, "AccountDisconnected received (i2)",
		attribute.String("agent_id", agentID),
		attribute.String("account_id", accountID),
		attribute.String("reason", reason),
	)

	// Desregistrar cuenta del registry
	c.accountRegistry.UnregisterAccount(accountID)
	if c.handshakeRegistry != nil {
		c.handshakeRegistry.Clear(accountID)
	}

	// i3: Invalidar caché de símbolos por cuenta
	if err := c.symbolResolver.InvalidateAccount(c.ctx, accountID); err != nil {
		c.telemetry.Warn(c.ctx, "Failed to invalidate symbol cache for account (i3)",
			attribute.String("account_id", accountID),
			attribute.String("error", err.Error()),
		)
	}

	// Limpiar especificaciones y quotes en caché
	if c.symbolSpecService != nil {
		c.symbolSpecService.Invalidate(accountID)
	}
	if c.symbolQuoteService != nil {
		c.symbolQuoteService.Invalidate(accountID)
	}
	if c.riskPolicyService != nil {
		c.riskPolicyService.Invalidate(accountID, "")
	}

	// Métrica: número de cuentas registradas
	totalAccounts, totalAgents := c.accountRegistry.GetStats()
	c.telemetry.Info(c.ctx, "Account registry updated (i2)",
		attribute.Int("total_accounts", totalAccounts),
		attribute.Int("total_agents", totalAgents),
	)
}

// NEW i3: handleAccountSymbolsReport procesa el reporte de símbolos por cuenta.
func (c *Core) handleAccountSymbolsReport(ctx context.Context, agentID string, report *pb.AccountSymbolsReport) {
	start := time.Now()

	// Validar usando SDK
	if err := domain.ValidateAccountSymbolsReport(report, c.config.CanonicalSymbols); err != nil {
		c.telemetry.Warn(ctx, "Invalid AccountSymbolsReport (i3)",
			attribute.String("agent_id", agentID),
			attribute.String("account_id", report.AccountId),
			attribute.String("error", err.Error()),
		)
		return
	}

	accountID := report.AccountId

	// Configurar contexto con atributos del evento
	ctx = telemetry.AppendEventAttrs(ctx,
		semconv.Echo.AccountID.String(accountID),
		attribute.String("source", "agent_report"),
	)

	ctx, span := c.telemetry.StartSpan(ctx, "core.handle_account_symbols_report")
	defer span.End()

	// Convertir proto mappings a domain mappings
	mappings := make([]*domain.SymbolMapping, 0, len(report.Symbols))
	for _, protoMapping := range report.Symbols {
		if protoMapping == nil {
			continue
		}
		mapping := &domain.SymbolMapping{
			CanonicalSymbol: protoMapping.CanonicalSymbol,
			BrokerSymbol:    protoMapping.BrokerSymbol,
			Digits:          protoMapping.Digits,
			Point:           protoMapping.Point,
			TickSize:        protoMapping.TickSize,
			MinLot:          protoMapping.MinLot,
			MaxLot:          protoMapping.MaxLot,
			LotStep:         protoMapping.LotStep,
			StopLevel:       protoMapping.StopLevel,
		}
		if protoMapping.ContractSize != nil {
			mapping.ContractSize = protoMapping.ContractSize
		}
		mappings = append(mappings, mapping)
	}

	// Upsert mappings en resolver (actualiza caché y encola persistencia async)
	if err := c.symbolResolver.UpsertMappings(ctx, accountID, mappings, report.ReportedAtMs); err != nil {
		c.telemetry.RecordError(ctx, err)
		c.telemetry.Error(ctx, "Failed to upsert symbol mappings (i3)", err,
			attribute.String("account_id", accountID),
			attribute.Int("mappings_count", len(mappings)),
		)
		return
	}

	// Evaluar handshake con metadata adjunta
	metadata := report.Metadata
	if metadata == nil {
		metadata = &pb.HandshakeMetadata{}
	}

	ownership, _ := c.accountRegistry.GetRecord(accountID)
	pipeRole := ownership.PipeRole
	if pipeRole == "" {
		pipeRole = "unknown"
	}

	evaluation, _, err := c.handshakeEvaluator.Evaluate(ctx, accountID, agentID, pipeRole, c.config.ServiceVersion, metadata, mappings)
	if err != nil {
		c.telemetry.RecordError(ctx, err)
		c.telemetry.Error(ctx, "Failed to evaluate handshake",
			err,
			attribute.String("account_id", accountID),
			attribute.String("agent_id", agentID),
		)
		return
	}

	if c.handshakeRegistry != nil {
		c.handshakeRegistry.Set(evaluation)
	}

	result := evaluation.ToProtoResult()
	if result != nil {
		if err := c.sendSymbolRegistrationResult(ctx, agentID, result); err != nil {
			c.telemetry.Warn(ctx, "Failed to send SymbolRegistrationResult",
				attribute.String("account_id", accountID),
				attribute.String("agent_id", agentID),
				attribute.String("error", err.Error()),
			)
			c.echoMetrics.RecordAgentHandshakeForwardError(ctx, err.Error(),
				semconv.Echo.AccountID.String(accountID),
				attribute.String("pipe_role", pipeRole),
			)
		}
	}

	latencyMs := float64(time.Since(start).Milliseconds())
	c.echoMetrics.RecordHandshakeFeedbackLatency(ctx, latencyMs,
		semconv.Echo.AccountID.String(accountID),
		attribute.String("pipe_role", pipeRole),
	)

	c.telemetry.Info(ctx, "AccountSymbolsReport processed successfully (i5)",
		attribute.String("account_id", accountID),
		attribute.Int("mappings_count", len(mappings)),
		attribute.Int64("reported_at_ms", report.ReportedAtMs),
		attribute.String("status", registrationStatusString(evaluation.Status)),
	)
}

// handleSymbolSpecReport procesa el reporte de especificaciones de un Agent.
func (c *Core) handleSymbolSpecReport(ctx context.Context, agentID string, report *pb.SymbolSpecReport) {
	accountID := report.GetAccountId()
	ctx = telemetry.AppendEventAttrs(ctx,
		semconv.Echo.AccountID.String(accountID),
	)

	c.telemetry.Debug(ctx, "SymbolSpecReport received",
		attribute.String("agent_id", agentID),
		attribute.String("account_id", accountID),
		attribute.Int("symbols_count", len(report.Symbols)),
		attribute.Int64("reported_at_ms", report.ReportedAtMs),
	)

	ctx, span := c.telemetry.StartSpan(ctx, "core.handle_symbol_spec_report")
	defer span.End()

	if err := c.symbolSpecService.Upsert(ctx, report); err != nil {
		c.telemetry.RecordError(ctx, err)
		c.telemetry.Error(ctx, "Failed to process SymbolSpecReport", err,
			attribute.String("agent_id", agentID),
			attribute.String("account_id", accountID),
			attribute.String("error", err.Error()),
		)
		return
	}

	c.telemetry.Debug(ctx, "SymbolSpecReport processed",
		attribute.String("agent_id", agentID),
		attribute.String("account_id", accountID),
		attribute.Int("symbols_count", len(report.Symbols)),
		attribute.Int64("reported_at_ms", report.ReportedAtMs),
	)
}

// handleSymbolQuoteSnapshot procesa snapshots de precios bid/ask recientes.
func (c *Core) handleSymbolQuoteSnapshot(ctx context.Context, agentID string, snapshot *pb.SymbolQuoteSnapshot) {
	if err := domain.ValidateSymbolQuoteSnapshot(snapshot, c.config.CanonicalSymbols); err != nil {
		c.telemetry.Warn(ctx, "Invalid SymbolQuoteSnapshot",
			attribute.String("agent_id", agentID),
			attribute.String("account_id", snapshot.GetAccountId()),
			attribute.String("canonical_symbol", snapshot.GetCanonicalSymbol()),
			attribute.String("error", err.Error()),
		)
		return
	}

	ctx = telemetry.AppendEventAttrs(ctx,
		semconv.Echo.AccountID.String(snapshot.AccountId),
		semconv.Echo.Symbol.String(snapshot.CanonicalSymbol),
	)

	ctx, span := c.telemetry.StartSpan(ctx, "core.handle_symbol_quote_snapshot")
	defer span.End()

	if err := c.symbolQuoteService.Record(ctx, snapshot); err != nil {
		c.telemetry.RecordError(ctx, err)
		c.telemetry.Error(ctx, "Failed to record symbol quote snapshot", err,
			attribute.String("agent_id", agentID),
			attribute.String("account_id", snapshot.AccountId),
			attribute.String("canonical_symbol", snapshot.CanonicalSymbol),
			attribute.String("error", err.Error()),
		)
		return
	}

	c.echoMetrics.RecordSymbolLookup(ctx, "quote_received", snapshot.AccountId, snapshot.CanonicalSymbol)
	c.telemetry.Debug(ctx, "Symbol quote snapshot processed",
		attribute.String("agent_id", agentID),
		attribute.Float64("bid", snapshot.Bid),
		attribute.Float64("ask", snapshot.Ask),
	)
}

type symbolInfoAdapter struct {
	resolver *AccountSymbolResolver
}

func (a *symbolInfoAdapter) Resolve(ctx context.Context, accountID, canonical string) (*domain.AccountSymbolInfo, bool) {
	if a == nil || a.resolver == nil {
		return nil, false
	}
	_, info, found := a.resolver.ResolveForAccount(ctx, accountID, canonical)
	return info, found
}
