---
title: "RFC-002: Routing Selectivo por Ownership de Cuentas (Iteración 2)"
version: "2.0"
date: "2025-10-29"
status: "Propuesta"
owner: "Arquitectura Echo"
iteration: "2"
replaces: "Broadcast i0/i1"
---

## Resumen Ejecutivo

Este RFC define la especificación técnica completa para eliminar el broadcast indiscriminado de órdenes e implementar **routing selectivo dinámico** basado en ownership de cuentas. A partir de i2, el Core enrutará cada `ExecuteOrder` y `CloseOrder` **exclusivamente** al Agent propietario de la cuenta slave destino.

**Flujo dinámico clave:**
1. Slave EA se conecta a su Agent (Named Pipe) → Agent notifica al Core → Core registra cuenta como disponible.
2. Core enruta órdenes solo al Agent propietario de cada cuenta.
3. Slave EA se desconecta → Agent notifica al Core → Core desregistra cuenta.
4. Agent se desconecta del Core → Core desregistra automáticamente todas sus cuentas.

**Impacto:** Reducción de tráfico gRPC ~66% (3 Agents), trazabilidad completa, base para i3+.

---

## 1. Contexto y Motivación

### 1.1 Situación Actual (i0/i1)

En las iteraciones 0 y 1, el Core implementa un patrón de **broadcast simple**:

```go
// router.go (líneas 281-329)
// Broadcast a todos los Agents
agents := r.core.GetAgents()
for _, agent := range agents {
    for _, order := range orders {
        select {
        case agent.SendCh <- msg:
            sentCount++
        // ...
        }
    }
}
```

**Comportamiento:**
- Cada `ExecuteOrder`/`CloseOrder` se envía a **todos** los Agents conectados (n × m mensajes, donde n=agents, m=slaves).
- Cada Agent decide localmente si el mensaje aplica a alguno de sus pipes conectados (`routeExecuteOrder` en `agent/internal/stream.go:142-189`).
- Si el `target_account_id` no coincide con ningún pipe local, el mensaje se descarta silenciosamente.

**Problemas:**
1. **Tráfico ineficiente:** Si hay 3 Agents y 15 slaves, cada orden genera 3 × 15 = 45 mensajes, cuando solo 15 son necesarios.
2. **Ambigüedad operativa:** No hay registro canónico de qué Agent es propietario de cada cuenta; todo se infiere en runtime.
3. **Dificulta escalabilidad:** Imposible añadir lógica de balanceo, failover o migración de cuentas sin un registro central de ownership.
4. **Observabilidad limitada:** Métricas de "mensajes enviados" vs "mensajes procesados" no reflejan eficiencia real.

### 1.2 Solución Propuesta (i2)

Implementar **routing selectivo dinámico** con notificaciones en tiempo real:

1. **Registro dinámico:** Cuando un Slave EA se conecta al Agent (pipe), el Agent envía `AccountConnected` al Core. El Core registra `account_id → agent_id` en memoria.
2. **Envío dirigido:** El Core consulta el registro y envía cada orden **solo** al Agent propietario.
3. **Desregistro dinámico:** Cuando un Slave EA se desconecta, el Agent envía `AccountDisconnected` al Core.
4. **Limpieza automática:** Si el Agent se desconecta del Core, todas sus cuentas se desregistran automáticamente.

**Beneficios:**
- Reducción del tráfico gRPC en ~(n-1)/n (p. ej., 66% con 3 Agents).
- Trazabilidad: saber determinísticamente qué Agent maneja qué cuenta en cada momento.
- Flexibilidad: soporta cuentas que se conectan/desconectan en cualquier momento.
- Base para features futuras: migración de cuentas, failover, multi-región.
- Métricas precisas de routing (hit/miss, órden dirigida vs broadcast fallback).

---

## 2. Alcance y Exclusiones

### 2.1 En Alcance (i2)

1. **Proto:**
   - Nuevo mensaje `AccountConnected`: notifica al Core cuando un Slave EA se conecta al Agent.
   - Nuevo mensaje `AccountDisconnected`: notifica al Core cuando un Slave EA se desconecta del Agent.
   - `AgentHello` se mantiene para metadata básica (version, hostname), **SIN** lista de cuentas.

2. **Core:**
   - Crear registro de ownership en memoria (`AccountRegistry`).
   - Procesar `AccountConnected`/`AccountDisconnected` en tiempo real.
   - Modificar `Router` para consultar registry antes de enviar.
   - Mantener broadcast como fallback si no se encuentra owner (migración suave).
   - Limpieza automática de cuentas al desconectar Agent.

3. **Agent:**
   - Detectar conexión/desconexión de Slave EA (Named Pipe).
   - Enviar `AccountConnected`/`AccountDisconnected` al Core inmediatamente.
   - Mantener comportamiento de routing local sin cambios.

4. **Telemetría:**
   - Métrica: `echo.routing.mode` (labels: `mode=selective|broadcast|fallback`).
   - Métrica: `echo.routing.account_lookup` (labels: `result=hit|miss`).
   - Logs estructurados con `agent_id` + `account_id` en routing.

5. **Persistencia (opcional, no bloqueante):**
   - PostgreSQL puede tener tabla `accounts` para auditoría/configuración.
   - Registry operacional es **solo en memoria**, reconstrucción dinámica en cada reconexión.

### 2.2 Fuera de Alcance (iteraciones posteriores)

- **Políticas de negocio** (spread, desvío, SL/TP): i6–i9.
- **Money Management avanzado** (riesgo fijo): i5.
- **Migración dinámica de cuentas entre Agents:** fuera de V1.
- **Persistencia de ownership en BD:** opcional i3+ para auditoría histórica.
- **Failover automático de Agents:** fuera de V1.
- **Testing formal:** No se desarrollarán tests hasta completar V1 completo (política del proyecto).

---

## 3. Arquitectura de Solución

### 3.1 Cardinalidad y Restricciones

**Restricciones arquitectónicas:**
- **1 Slave → 1 Agent:** Un slave solo puede estar asociado a UN Agent (exclusividad).
- **1 Agent → N Clients:** Un Agent puede manejar múltiples Masters y Slaves.
- **1 Core → N Agents:** El Core es único (singleton operacional) y maneja múltiples Agents.
- **Core único:** Solo hay un Core operativo en el sistema.

**Implicación:** El `AccountRegistry` en el Core es la fuente de verdad operacional. Si una cuenta aparece en 2 Agents simultáneamente (error de configuración), el último que notifique gana (last-write-wins), con WARNING en logs.

### 3.2 Diagrama de Componentes

```
┌─────────────────────────────────────────────────────────────────┐
│                         Echo Core (ÚNICO)                        │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │                    AgentService                           │  │
│  │  StreamBidi(stream AgentService_StreamBidiServer) error  │  │
│  │                                                             │  │
│  │  1. Recibe AgentHello (metadata, sin cuentas)             │  │
│  │  2. Recibe AccountConnected → registra en registry        │  │
│  │  3. Recibe AccountDisconnected → desregistra cuenta       │  │
│  │  4. Al desconectar Agent → limpia todas sus cuentas       │  │
│  └────────────────┬────────────────────────────────────────────┘  │
│                   │                                               │
│  ┌────────────────▼─────────────────────────────────────────┐    │
│  │            AccountRegistry (nuevo)                       │    │
│  │  - Map: account_id → agent_id (estado OPERACIONAL)       │    │
│  │  - RegisterAccount(agentID, accountID)                   │    │
│  │  - UnregisterAccount(accountID)                          │    │
│  │  - UnregisterAgent(agentID)  // limpia todas sus cuentas│    │
│  │  - GetOwner(accountID) (agentID, found)                  │    │
│  └────────────────┬───────────────────────────────────────────┘    │
│                   │                                               │
│  ┌────────────────▼───────────────────────────────────────────┐  │
│  │                    Router                                  │  │
│  │  handleTradeIntent(...) {                                 │  │
│  │    orders := createExecuteOrders(...)                      │  │
│  │    for order in orders {                                  │  │
│  │      agentID, found := registry.GetOwner(order.TargetAccountId) │  │
│  │      if found {                                           │  │
│  │        sendToAgent(agentID, order)  // SELECTIVE         │  │
│  │      } else {                                             │  │
│  │        broadcastToAll(order)        // FALLBACK          │  │
│  │      }                                                     │  │
│  │    }                                                       │  │
│  │  }                                                         │  │
│  └──────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│                      Echo Agent (N instancias)                   │
│  ┌───────────────────────────────────────────────────────────┐  │
│  │  PipeManager detecta nueva conexión de Slave EA:         │  │
│  │  - Pipe: \\.\pipe\slave_123456 (Named Pipe)              │  │
│  │  - Extrae account_id = "123456"                           │  │
│  │  - Envía AccountConnected al Core                         │  │
│  │                                                             │  │
│  │  PipeManager detecta desconexión de Slave EA:             │  │
│  │  - Envía AccountDisconnected al Core                      │  │
│  └───────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│                      Slave EA (MQL4/MQL5)                        │
│  - Se conecta a Named Pipe del Agent en cualquier momento       │
│  - El Agent lo detecta y notifica al Core                        │
│  - Puede desconectarse/reconectarse dinámicamente                │
└─────────────────────────────────────────────────────────────────┘
```

### 3.3 Flujo Dinámico de Registro (i2)

**Escenario:** Core y Agent ya están corriendo. Un Slave EA se conecta posteriormente.

**Secuencia:**

1. **Slave EA (MQL4/5) se conecta:**
   - Abre Named Pipe: `\\.\pipe\slave_123456`.
   - Envía primer mensaje (puede ser `TradeIntent`, `StateSnapshot`, etc.).

2. **Agent detecta nueva conexión:**
   - `PipeManager` detecta que el pipe `slave_123456` tiene un cliente conectado.
   - Extrae `account_id = "123456"` del nombre del pipe.
   - Construye mensaje `AccountConnected`:
     ```protobuf
     message AccountConnected {
       string account_id = 1;
       int64 connected_at_ms = 2;
       optional string client_type = 3;  // "slave" o "master"
     }
     ```
   - Envía al Core vía stream gRPC.

3. **Core recibe `AccountConnected`:**
   - `StreamBidi` detecta mensaje de tipo `AccountConnected`.
   - Extrae `agent_id` (del contexto del stream) y `account_id` (del mensaje).
   - Llama `accountRegistry.RegisterAccount(agent_id, account_id)`.
   - Log estructurado: `Account registered (i2)`.
   - La cuenta ahora está disponible para routing selectivo.

4. **Core enruta orden al Agent propietario:**
   - Master emite `TradeIntent` para slaves incluido `123456`.
   - Core genera `ExecuteOrder` con `target_account_id=123456`.
   - `router.go` llama `registry.GetOwner("123456")` → `"agent-xyz"`.
   - Envía mensaje solo a `agent-xyz` (routing selectivo).

5. **Slave EA se desconecta:**
   - Cierra Named Pipe.
   - `PipeManager` detecta desconexión.
   - Envía `AccountDisconnected` al Core.
   - Core llama `accountRegistry.UnregisterAccount("123456")`.
   - La cuenta ya no está disponible para routing; futuros intentos harán fallback a broadcast.

6. **Agent se desconecta del Core:**
   - Stream gRPC se cierra (error o graceful shutdown).
   - Core en `defer` de `StreamBidi` llama `accountRegistry.UnregisterAgent(agent_id)`.
   - Todas las cuentas del Agent se desregistran de una sola vez.

### 3.4 Flujo de Handshake Inicial

**Cambio vs RFC v1.0:** `AgentHello` ya NO incluye lista de cuentas, solo metadata.

**Secuencia:**

1. **Agent conecta al Core:**
   - Abre stream gRPC bidireccional.
   - Envía `AgentHello` con metadata básica:
     ```protobuf
     message AgentHello {
       string agent_id = 1;
       string version = 2;
       string hostname = 3;
       string os = 4;
       // NO incluye owned_accounts (ya no es necesario)
     }
     ```

2. **Core registra Agent:**
   - Añade a mapa `agents[agent_id]`.
   - Log: `Agent connected (i2)`.
   - El Agent aún no tiene cuentas registradas.

3. **Agent comienza a escuchar pipes:**
   - `PipeManager` espera conexiones de EAs (bloqueante).
   - A medida que los EAs se conectan, envía `AccountConnected` para cada uno.

---

## 4. Especificación de Cambios

### 4.1 Proto (SDK)

**Archivo:** `sdk/proto/v1/agent.proto`

**Cambio 1: Modificar AgentHello (eliminar owned_accounts)**

```protobuf
// agent.proto (líneas 47-55)
message AgentHello {
  string agent_id = 1;
  string version = 2;
  string hostname = 3;
  string os = 4;
  // connected_clients deprecated i2, será eliminado i3+
  repeated string connected_clients = 5 [deprecated = true];
  // owned_accounts deprecated i2 (ya no se usa, reemplazado por AccountConnected/Disconnected)
  repeated string owned_accounts = 6 [deprecated = true];
  map<string, SymbolInfo> symbols = 7;
}
```

**Cambio 2: Nuevos mensajes AccountConnected y AccountDisconnected**

```protobuf
// agent.proto (añadir después de AgentHello, línea ~56)

// AccountConnected notifica al Core que una cuenta se conectó al Agent (i2).
//
// El Agent envía este mensaje cuando un Slave EA abre el Named Pipe.
// El Core registra la cuenta como disponible para routing selectivo.
message AccountConnected {
  string account_id = 1;             // Account ID del slave (extraído del pipe name)
  int64 connected_at_ms = 2;         // Timestamp de conexión
  optional string client_type = 3;   // "slave" | "master" (para diagnóstico)
  optional string broker = 4;        // Broker del EA (opcional i3+)
}

// AccountDisconnected notifica al Core que una cuenta se desconectó del Agent (i2).
//
// El Agent envía este mensaje cuando un Slave EA cierra el Named Pipe.
// El Core desregistra la cuenta y deja de enrutar órdenes al Agent.
message AccountDisconnected {
  string account_id = 1;             // Account ID del slave
  int64 disconnected_at_ms = 2;      // Timestamp de desconexión
  optional string reason = 3;        // Razón de desconexión (opcional)
}
```

**Cambio 3: Añadir a AgentMessage**

```protobuf
// agent.proto (modificar AgentMessage, líneas 19-31)
message AgentMessage {
  string agent_id = 1;
  int64 timestamp_ms = 2;
  
  oneof payload {
    AgentHello hello = 10;
    TradeIntent trade_intent = 11;
    TradeClose trade_close = 12;
    TradeModify trade_modify = 13;
    ExecutionResult execution_result = 14;
    StateSnapshot state_snapshot = 15;
    AgentHeartbeat heartbeat = 16;
    AccountConnected account_connected = 17;      // NEW i2
    AccountDisconnected account_disconnected = 18; // NEW i2
  }
}
```

**Validación SDK (nuevo):**

Añadir en `sdk/domain/account_validation.go` (nuevo archivo):

```go
package domain

import (
    "errors"
    "fmt"
)

// ValidateAccountID valida que un account_id es válido.
func ValidateAccountID(accountID string) error {
    if accountID == "" {
        return errors.New("account_id cannot be empty")
    }
    // Opcional: validar formato (solo dígitos, longitud, etc.)
    // Por ahora solo validamos no-vacío
    return nil
}

// ValidateAccountConnected valida un mensaje AccountConnected.
func ValidateAccountConnected(msg *pb.AccountConnected) error {
    if err := ValidateAccountID(msg.AccountId); err != nil {
        return fmt.Errorf("invalid account_id: %w", err)
    }
    if msg.ConnectedAtMs <= 0 {
        return errors.New("connected_at_ms must be positive")
    }
    return nil
}

// ValidateAccountDisconnected valida un mensaje AccountDisconnected.
func ValidateAccountDisconnected(msg *pb.AccountDisconnected) error {
    if err := ValidateAccountID(msg.AccountId); err != nil {
        return fmt.Errorf("invalid account_id: %w", err)
    }
    if msg.DisconnectedAtMs <= 0 {
        return errors.New("disconnected_at_ms must be positive")
    }
    return nil
}
```

### 4.2 Core: AccountRegistry (nuevo componente)

**Archivo:** `core/internal/account_registry.go` (nuevo)

**Cambio vs RFC v1.0:** Registra/desregistra cuentas **individualmente**, no en batch.

```go
package internal

import (
    "sync"
    "time"
    "go.opentelemetry.io/otel/attribute"
    "github.com/xKoRx/echo/sdk/telemetry"
)

// AccountRegistry mantiene el mapeo de cuentas a Agents (estado OPERACIONAL).
//
// Thread-safe. Operaciones:
//   - RegisterAccount: registra UNA cuenta para un Agent (i2 dinámico).
//   - UnregisterAccount: desregistra UNA cuenta (i2 dinámico).
//   - UnregisterAgent: elimina TODAS las cuentas de un Agent (cleanup al desconectar).
//   - GetOwner: retorna el Agent propietario de una cuenta.
//   - GetAccountsByAgent: retorna todas las cuentas de un Agent (diagnóstico).
type AccountRegistry struct {
    // account_id → OwnershipRecord
    accountToOwner map[string]*OwnershipRecord
    // agent_id → []account_id (índice inverso para cleanup)
    agentToAccounts map[string][]string
    
    mu        sync.RWMutex
    telemetry *telemetry.Client
}

// OwnershipRecord registra ownership de una cuenta (i2).
type OwnershipRecord struct {
    AgentID       string
    AccountID     string
    RegisteredAt  time.Time
    LastSeenAt    time.Time  // actualizado en cada heartbeat (opcional i3+)
}

// NewAccountRegistry crea un nuevo registry.
func NewAccountRegistry(tel *telemetry.Client) *AccountRegistry {
    return &AccountRegistry{
        accountToOwner:  make(map[string]*OwnershipRecord),
        agentToAccounts: make(map[string][]string),
        telemetry:       tel,
    }
}

// RegisterAccount registra UNA cuenta para un Agent (i2 dinámico).
//
// Si la cuenta ya está registrada a OTRO Agent, sobreescribe (last-write-wins).
// Log WARNING si hay cambio de ownership.
func (r *AccountRegistry) RegisterAccount(agentID string, accountID string) {
    r.mu.Lock()
    defer r.mu.Unlock()

    now := time.Now()

    // Verificar si ya existe
    if existing, exists := r.accountToOwner[accountID]; exists {
        if existing.AgentID != agentID {
            // Conflicto de ownership: la cuenta estaba en otro Agent
            r.telemetry.Warn(nil, "Account ownership conflict (i2)",
                attribute.String("account_id", accountID),
                attribute.String("previous_agent", existing.AgentID),
                attribute.String("new_agent", agentID),
            )
            
            // Limpiar del agente anterior
            r.removeAccountFromAgent(existing.AgentID, accountID)
        } else {
            // Mismo agente, actualizar timestamp (re-registro, posible reconexión)
            existing.LastSeenAt = now
            r.telemetry.Info(nil, "Account re-registered to same Agent (i2)",
                attribute.String("account_id", accountID),
                attribute.String("agent_id", agentID),
            )
            return
        }
    }

    // Registrar nueva cuenta
    r.accountToOwner[accountID] = &OwnershipRecord{
        AgentID:      agentID,
        AccountID:    accountID,
        RegisteredAt: now,
        LastSeenAt:   now,
    }

    // Añadir a índice inverso
    r.agentToAccounts[agentID] = append(r.agentToAccounts[agentID], accountID)

    r.telemetry.Info(nil, "Account registered to Agent (i2)",
        attribute.String("agent_id", agentID),
        attribute.String("account_id", accountID),
    )
}

// UnregisterAccount desregistra UNA cuenta (i2 dinámico).
//
// Se llama cuando el Slave EA se desconecta del Agent.
func (r *AccountRegistry) UnregisterAccount(accountID string) {
    r.mu.Lock()
    defer r.mu.Unlock()

    record, exists := r.accountToOwner[accountID]
    if !exists {
        r.telemetry.Warn(nil, "Attempted to unregister non-existent account (i2)",
            attribute.String("account_id", accountID),
        )
        return
    }

    agentID := record.AgentID

    // Eliminar del mapa principal
    delete(r.accountToOwner, accountID)

    // Eliminar del índice inverso
    r.removeAccountFromAgent(agentID, accountID)

    r.telemetry.Info(nil, "Account unregistered from Agent (i2)",
        attribute.String("agent_id", agentID),
        attribute.String("account_id", accountID),
    )
}

// UnregisterAgent elimina TODAS las cuentas de un Agent (i2 cleanup).
//
// Se llama al desconectar el Agent del Core.
func (r *AccountRegistry) UnregisterAgent(agentID string) {
    r.mu.Lock()
    defer r.mu.Unlock()

    accounts, exists := r.agentToAccounts[agentID]
    if !exists {
        return
    }

    for _, acc := range accounts {
        delete(r.accountToOwner, acc)
    }
    delete(r.agentToAccounts, agentID)

    r.telemetry.Info(nil, "Agent unregistered, all accounts released (i2)",
        attribute.String("agent_id", agentID),
        attribute.Int("accounts_count", len(accounts)),
    )
}

// GetOwner retorna el Agent propietario de una cuenta.
//
// Retorna ("", false) si la cuenta no está registrada (o se desconectó).
func (r *AccountRegistry) GetOwner(accountID string) (string, bool) {
    r.mu.RLock()
    defer r.mu.RUnlock()

    record, found := r.accountToOwner[accountID]
    if !found {
        return "", false
    }
    return record.AgentID, true
}

// GetAccountsByAgent retorna todas las cuentas de un Agent (diagnóstico).
func (r *AccountRegistry) GetAccountsByAgent(agentID string) []string {
    r.mu.RLock()
    defer r.mu.RUnlock()

    accounts := r.agentToAccounts[agentID]
    // Retornar copia para evitar modificaciones externas
    result := make([]string, len(accounts))
    copy(result, accounts)
    return result
}

// GetStats retorna estadísticas del registry (diagnóstico/métricas).
func (r *AccountRegistry) GetStats() (totalAccounts int, totalAgents int) {
    r.mu.RLock()
    defer r.mu.RUnlock()

    return len(r.accountToOwner), len(r.agentToAccounts)
}

// removeAccountFromAgent elimina una cuenta del índice inverso (helper interno).
//
// DEBE llamarse con lock ya adquirido.
func (r *AccountRegistry) removeAccountFromAgent(agentID string, accountID string) {
    accounts := r.agentToAccounts[agentID]
    for i, acc := range accounts {
        if acc == accountID {
            // Eliminar usando swap-and-truncate (eficiente)
            accounts[i] = accounts[len(accounts)-1]
            r.agentToAccounts[agentID] = accounts[:len(accounts)-1]
            break
        }
    }
    
    // Si el Agent ya no tiene cuentas, eliminar entrada
    if len(r.agentToAccounts[agentID]) == 0 {
        delete(r.agentToAccounts, agentID)
    }
}
```

### 4.3 Core: Modificar core.go

**Archivo:** `core/internal/core.go`

**Cambio 1: Añadir campo AccountRegistry**

```go
// core.go (líneas 39-79)
type Core struct {
    pb.UnimplementedAgentServiceServer

    config *Config

    grpcServer *grpc.Server
    listener   net.Listener

    // Agent connections
    agents   map[string]*AgentConnection
    agentsMu sync.RWMutex

    // i2: Registry de ownership de cuentas (estado operacional en memoria)
    accountRegistry *AccountRegistry  // NEW

    // PostgreSQL
    db *sql.DB

    repoFactory    domain.RepositoryFactory
    correlationSvc domain.CorrelationService

    dedupeService *DedupeService

    router *Router

    telemetry   *telemetry.Client
    echoMetrics *metricbundle.EchoMetrics

    ctx    context.Context
    cancel context.CancelFunc
    wg     sync.WaitGroup

    mu     sync.RWMutex
    closed bool
}
```

**Cambio 2: Inicializar AccountRegistry en New()**

```go
// core.go (líneas 114-215)
func New(ctx context.Context) (*Core, error) {
    // ... código existente hasta línea 200 ...

    // 6. Crear Core
    core := &Core{
        config:          config,
        db:              db,
        repoFactory:     repoFactory,
        correlationSvc:  correlationSvc,
        dedupeService:   dedupeService,
        agents:          make(map[string]*AgentConnection),
        accountRegistry: NewAccountRegistry(telClient),  // NEW i2
        telemetry:       telClient,
        echoMetrics:     echoMetrics,
        ctx:             coreCtx,
        cancel:          cancel,
    }

    // 7. Crear router
    core.router = NewRouter(core)

    // ... resto del código ...
}
```

**Cambio 3: Procesar AccountConnected/Disconnected en StreamBidi**

```go
// core.go (líneas 285-344)
func (c *Core) StreamBidi(stream pb.AgentService_StreamBidiServer) error {
    ctx := stream.Context()

    agentID, err := extractAgentIDFromMetadata(ctx)
    if err != nil {
        c.telemetry.Warn(c.ctx, "Agent connected without agent-id metadata, using generated ID",
            attribute.String("error", err.Error()),
        )
        agentID = fmt.Sprintf("agent_%s", utils.GenerateUUIDv7())
    }

    c.telemetry.Info(c.ctx, "Agent connected",
        attribute.String("agent_id", agentID),
    )

    agentCtx, agentCancel := context.WithCancel(ctx)
    conn := &AgentConnection{
        AgentID:   agentID,
        Stream:    stream,
        SendCh:    make(chan *pb.CoreMessage, 1000),
        ctx:       agentCtx,
        cancel:    agentCancel,
        createdAt: time.Now(),
    }

    c.registerAgent(agentID, conn)
    defer func() {
        c.unregisterAgent(agentID)
        c.accountRegistry.UnregisterAgent(agentID)  // NEW i2: limpiar registry
        agentCancel()
        close(conn.SendCh)
    }()

    c.wg.Add(1)
    go func() {
        defer c.wg.Done()
        c.sendToAgentLoop(conn)
    }()

    // Loop de lectura
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
            continue  // No enviar al router
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

        // Enviar al router para procesamiento
        c.router.HandleAgentMessage(agentCtx, agentID, msg)
    }
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

    c.telemetry.Info(c.ctx, "AccountConnected received (i2)",
        attribute.String("agent_id", agentID),
        attribute.String("account_id", accountID),
        attribute.String("client_type", stringOrDefault(msg.ClientType, "unknown")),
    )

    // Registrar cuenta en registry
    c.accountRegistry.RegisterAccount(agentID, accountID)

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

    c.telemetry.Info(c.ctx, "AccountDisconnected received (i2)",
        attribute.String("agent_id", agentID),
        attribute.String("account_id", accountID),
        attribute.String("reason", stringOrDefault(msg.Reason, "unknown")),
    )

    // Desregistrar cuenta del registry
    c.accountRegistry.UnregisterAccount(accountID)

    // Métrica: número de cuentas registradas
    totalAccounts, totalAgents := c.accountRegistry.GetStats()
    c.telemetry.Info(c.ctx, "Account registry updated (i2)",
        attribute.Int("total_accounts", totalAccounts),
        attribute.Int("total_agents", totalAgents),
    )
}

// stringOrDefault retorna el valor del puntero o un default si es nil.
func stringOrDefault(ptr *string, defaultValue string) string {
    if ptr == nil {
        return defaultValue
    }
    return *ptr
}
```

### 4.4 Core: Modificar router.go

**Archivo:** `core/internal/router.go`

**Cambio: Routing selectivo con fallback** (igual que RFC v1.0, sin cambios)

```go
// router.go (líneas 186-348)
func (r *Router) handleTradeIntent(ctx context.Context, agentID string, intent *pb.TradeIntent) {
    // ... código existente hasta línea 274 (crear orders) ...

    // 5. Routing selectivo (i2) en lugar de broadcast
    sentCount := 0
    broadcastCount := 0
    selectiveCount := 0

    for _, order := range orders {
        // Issue #M2: Agregar timestamp t3 (Core envía)
        if order.Timestamps != nil {
            order.Timestamps.T3CoreSendMs = utils.NowUnixMilli()
        }

        msg := &pb.CoreMessage{
            Payload: &pb.CoreMessage_ExecuteOrder{ExecuteOrder: order},
        }

        // i2: Lookup owner en registry
        targetAccountID := order.TargetAccountId
        ownerAgentID, found := r.core.accountRegistry.GetOwner(targetAccountID)

        if found {
            // Routing selectivo
            agent, agentExists := r.getAgent(ownerAgentID)
            if agentExists {
                if r.sendToAgent(ctx, agent, msg, order) {
                    sentCount++
                    selectiveCount++
                    r.recordRoutingMetric(ctx, "selective", true, order)
                }
            } else {
                // Owner registrado pero desconectado → fallback broadcast
                r.core.telemetry.Warn(ctx, "Owner agent not connected, falling back to broadcast (i2)",
                    attribute.String("target_account_id", targetAccountID),
                    attribute.String("owner_agent_id", ownerAgentID),
                )
                if r.broadcastOrder(ctx, msg, order) > 0 {
                    sentCount++
                    broadcastCount++
                    r.recordRoutingMetric(ctx, "fallback_broadcast", false, order)
                }
            }
        } else {
            // No hay owner registrado → fallback broadcast
            r.core.telemetry.Warn(ctx, "No owner registered for account, falling back to broadcast (i2)",
                attribute.String("target_account_id", targetAccountID),
            )
            if r.broadcastOrder(ctx, msg, order) > 0 {
                sentCount++
                broadcastCount++
                r.recordRoutingMetric(ctx, "fallback_broadcast", false, order)
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
            attribute.Int("selective_count", selectiveCount),
            attribute.Int("broadcast_count", broadcastCount),
        )

        r.core.telemetry.Info(ctx, "ExecuteOrders sent (i2)",
            attribute.Int("sent_count", sentCount),
            attribute.Int("selective_count", selectiveCount),
            attribute.Int("broadcast_count", broadcastCount),
        )
    }
}

// Helpers: sendToAgent, broadcastOrder, recordRoutingMetric (igual que RFC v1.0)
// ... código omitido por brevedad, ver RFC v1.0 sección 4.4 ...
```

**Nota:** Aplicar mismo patrón en `handleTradeClose` (código idéntico al RFC v1.0).

### 4.5 Agent: Modificar PipeManager

**Archivo:** `agent/internal/pipe_manager.go`

**Cambio: Detectar conexión/desconexión de pipes y notificar al Core**

```go
// pipe_manager.go (añadir métodos nuevos)

// NEW i2: onPipeConnected se llama cuando un EA se conecta a un pipe.
//
// Envía AccountConnected al Core.
func (pm *PipeManager) onPipeConnected(pipeName string, sendCh chan<- *pb.AgentMessage) {
    // Extraer account_id del nombre del pipe
    accountID := pm.extractAccountIDFromPipe(pipeName)
    if accountID == "" {
        pm.telemetry.Warn(pm.ctx, "Cannot extract account_id from pipe name (i2)",
            attribute.String("pipe_name", pipeName),
        )
        return
    }

    pm.telemetry.Info(pm.ctx, "Pipe connected, sending AccountConnected (i2)",
        attribute.String("pipe_name", pipeName),
        attribute.String("account_id", accountID),
    )

    // Determinar client_type (master o slave)
    clientType := "slave"
    if strings.Contains(pipeName, "master_") {
        clientType = "master"
    }

    // Construir mensaje AccountConnected
    msg := &pb.AgentMessage{
        AgentId:     pm.agentID,
        TimestampMs: utils.NowUnixMilli(),
        Payload: &pb.AgentMessage_AccountConnected{
            AccountConnected: &pb.AccountConnected{
                AccountId:     accountID,
                ConnectedAtMs: utils.NowUnixMilli(),
                ClientType:    &clientType,
            },
        },
    }

    // Enviar al Core
    select {
    case sendCh <- msg:
        pm.telemetry.Info(pm.ctx, "AccountConnected sent to Core (i2)",
            attribute.String("account_id", accountID),
        )
    case <-pm.ctx.Done():
        pm.telemetry.Warn(pm.ctx, "Context done, cannot send AccountConnected (i2)")
    }
}

// NEW i2: onPipeDisconnected se llama cuando un EA se desconecta de un pipe.
//
// Envía AccountDisconnected al Core.
func (pm *PipeManager) onPipeDisconnected(pipeName string, reason string, sendCh chan<- *pb.AgentMessage) {
    accountID := pm.extractAccountIDFromPipe(pipeName)
    if accountID == "" {
        return
    }

    pm.telemetry.Info(pm.ctx, "Pipe disconnected, sending AccountDisconnected (i2)",
        attribute.String("pipe_name", pipeName),
        attribute.String("account_id", accountID),
        attribute.String("reason", reason),
    )

    msg := &pb.AgentMessage{
        AgentId:     pm.agentID,
        TimestampMs: utils.NowUnixMilli(),
        Payload: &pb.AgentMessage_AccountDisconnected{
            AccountDisconnected: &pb.AccountDisconnected{
                AccountId:        accountID,
                DisconnectedAtMs: utils.NowUnixMilli(),
                Reason:           &reason,
            },
        },
    }

    select {
    case sendCh <- msg:
        pm.telemetry.Info(pm.ctx, "AccountDisconnected sent to Core (i2)",
            attribute.String("account_id", accountID),
        )
    case <-pm.ctx.Done():
        pm.telemetry.Warn(pm.ctx, "Context done, cannot send AccountDisconnected (i2)")
    }
}

// NEW i2: extractAccountIDFromPipe extrae el account_id del nombre del pipe.
//
// Formato esperado: \\.\pipe\{prefix}slave_{account_id}
// Ejemplo: \\.\pipe\slave_123456 → "123456"
func (pm *PipeManager) extractAccountIDFromPipe(pipeName string) string {
    // Eliminar prefijo del sistema operativo (\\.\pipe\ en Windows)
    pipeName = strings.TrimPrefix(pipeName, `\\.\pipe\`)
    
    // Buscar patrón "slave_" o "master_"
    if strings.Contains(pipeName, "slave_") {
        parts := strings.Split(pipeName, "slave_")
        if len(parts) >= 2 {
            return parts[1]
        }
    }
    if strings.Contains(pipeName, "master_") {
        parts := strings.Split(pipeName, "master_")
        if len(parts) >= 2 {
            return parts[1]
        }
    }
    
    return ""
}
```

**Cambio en loop de pipe:** Modificar el loop que gestiona cada pipe para llamar a `onPipeConnected`/`onPipeDisconnected`:

```go
// pipe_manager.go (modificar loop de gestión de pipes, líneas ~150-200)
func (pm *PipeManager) managePipe(pipeName string, sendCh chan<- *pb.AgentMessage) {
    defer pm.wg.Done()

    for {
        select {
        case <-pm.ctx.Done():
            return
        default:
        }

        // Esperar conexión de cliente EA
        handler, err := pm.waitForConnection(pipeName)
        if err != nil {
            pm.telemetry.Error(pm.ctx, "Failed to accept connection", err,
                attribute.String("pipe_name", pipeName),
            )
            continue
        }

        pm.telemetry.Info(pm.ctx, "Client connected to pipe",
            attribute.String("pipe_name", pipeName),
        )

        // NEW i2: Notificar al Core que la cuenta se conectó
        pm.onPipeConnected(pipeName, sendCh)

        // Manejar comunicación con el cliente
        pm.handlePipeClient(handler, pipeName, sendCh)

        // NEW i2: Notificar al Core que la cuenta se desconectó
        pm.onPipeDisconnected(pipeName, "client_closed", sendCh)

        pm.telemetry.Info(pm.ctx, "Client disconnected from pipe",
            attribute.String("pipe_name", pipeName),
        )
    }
}
```

### 4.6 Agent: Modificar stream.go

**Archivo:** `agent/internal/stream.go`

**Cambio: AgentHello sin owned_accounts**

```go
// stream.go (líneas 13-42)
func (a *Agent) connectToCore() error {
    a.logInfo("Connecting to Core (i2)", map[string]interface{}{
        "address":           a.config.CoreAddress,
        "agent_id":          a.config.AgentID,
        "keepalive_time":    a.config.KeepAliveTime.String(),
        "keepalive_timeout": a.config.KeepAliveTimeout.String(),
    })

    client, err := NewCoreClient(a.ctx, a.config)
    if err != nil {
        return fmt.Errorf("failed to create core client: %w", err)
    }

    a.coreClient = client

    stream, err := client.StreamBidi(a.ctx, a.config.AgentID)
    if err != nil {
        return fmt.Errorf("failed to create stream: %w", err)
    }

    a.coreStream = stream

    // NEW i2: Enviar AgentHello SOLO con metadata (sin owned_accounts)
    if err := a.sendAgentHello(); err != nil {
        return fmt.Errorf("failed to send AgentHello: %w", err)
    }

    a.logInfo("Connected to Core (i2)", nil)
    return nil
}

// NEW i2: sendAgentHello envía el handshake inicial (solo metadata).
func (a *Agent) sendAgentHello() error {
    hostname, _ := os.Hostname()  // Excepción permitida por reglas

    hello := &pb.AgentHello{
        AgentId:  a.config.AgentID,
        Version:  a.config.ServiceVersion,
        Hostname: hostname,
        Os:       runtime.GOOS,
        Symbols:  make(map[string]*pb.SymbolInfo),  // TODO i3: reportar símbolos
        // NO incluye owned_accounts ni connected_clients (deprecated i2)
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
```

### 4.7 SDK: Añadir métricas de routing

**Archivo:** `sdk/telemetry/metricbundle/echo_metrics.go` (modificar)

**Añadir métodos:** (igual que RFC v1.0, sin cambios)

```go
// RecordRoutingMode registra el modo de routing usado (i2).
//
// mode: "selective" | "broadcast" | "fallback_broadcast"
// result: "hit" | "miss"
func (e *EchoMetrics) RecordRoutingMode(ctx context.Context, mode string, result string, attrs ...attribute.KeyValue) {
    baseAttrs := []attribute.KeyValue{
        attribute.String("routing.mode", mode),
        attribute.String("routing.result", result),
    }
    baseAttrs = append(baseAttrs, attrs...)

    e.routingModeCounter.Add(ctx, 1, metric.WithAttributes(baseAttrs...))
}

// RecordAccountLookup registra lookups al AccountRegistry (i2).
//
// result: "hit" | "miss"
func (e *EchoMetrics) RecordAccountLookup(ctx context.Context, result string, attrs ...attribute.KeyValue) {
    baseAttrs := []attribute.KeyValue{
        attribute.String("lookup.result", result),
    }
    baseAttrs = append(baseAttrs, attrs...)

    e.accountLookupCounter.Add(ctx, 1, metric.WithAttributes(baseAttrs...))
}
```

---

## 5. Testing y Validación

### 5.1 Nota Importante sobre Testing

**Por política del proyecto, NO se desarrollarán tests formales hasta completar V1 completo.**

Esta sección documenta los escenarios de validación manual que deben ejecutarse durante el desarrollo de i2 para garantizar que el routing selectivo funciona correctamente, pero **no se implementarán como unit tests automatizados ni integration tests hasta V1**.

### 5.2 Escenarios de Validación Manual

**Escenario 1: Conexión dinámica de cuentas**
- Arrancar Core y Agent (sin slaves).
- Arrancar Slave EA 123456 → verificar log en Core: "Account registered (i2)".
- Master emite TradeIntent → verificar log en Core: "routing.mode=selective".
- Verificar que solo el Agent propietario recibe `ExecuteOrder`.

**Escenario 2: Desconexión de cuenta**
- Slave EA 123456 conectado y registrado.
- Cerrar Slave EA → verificar log en Core: "Account unregistered (i2)".
- Master emite TradeIntent para 123456 → verificar log: "fallback_broadcast".

**Escenario 3: Desconexión de Agent**
- Agent con 3 slaves conectados.
- Cerrar Agent → verificar log en Core: "all accounts released (i2)".
- Master emite TradeIntent → todas las órdenes hacen fallback broadcast.

**Escenario 4: Reconexión de Agent**
- Agent desconectado.
- Reconectar Agent → envía `AgentHello`.
- Reconectar Slave EAs → envían `AccountConnected`.
- Verificar que routing selectivo se restaura.

**Escenario 5: Conflicto de ownership**
- Slave EA 123456 conectado a Agent1.
- Intentar conectar mismo slave a Agent2 (error de configuración).
- Verificar log WARNING en Core: "ownership conflict".
- Verificar que Agent2 gana (last-write-wins).

### 5.3 Verificación de Métricas

**Dashboards Grafana (observación manual):**
- Panel "Routing Mode Distribution" debe mostrar aumento de `mode=selective`.
- Panel "Account Lookup Results" debe mostrar mayoría `result=hit`.
- Panel "Messages Sent per Agent" debe mostrar reducción de tráfico.

---

## 6. Criterios de Aceptación

**Bloqueantes para merge:**

1. **Funcional:**
   - [ ] 100% de ExecuteOrder enviados solo al Agent propietario (o broadcast fallback si no hay owner).
   - [ ] 100% de CloseOrder enviados solo al Agent propietario (o broadcast fallback).
   - [ ] Conexión dinámica de Slave EA → registro automático en Core.
   - [ ] Desconexión de Slave EA → desregistro automático en Core.
   - [ ] Desconexión de Agent → todas sus cuentas se desregistran.

2. **Observabilidad:**
   - [ ] Métrica `echo.routing.mode` activa con labels `mode` y `result`.
   - [ ] Métrica `echo.routing.account_lookup` activa con label `result=hit|miss`.
   - [ ] Logs estructurados incluyen `agent_id` + `account_id` en eventos de routing.
   - [ ] Dashboard Grafana actualizado con panel de routing (selectivo vs broadcast).

3. **Calidad:**
   - [ ] Linters (`go vet`, `staticcheck`) limpios.
   - [ ] Sin race conditions visibles en operación manual (`go test -race` en desarrollo, no automatizado).
   - [ ] Documentación inline actualizada (comentarios en código nuevo).

4. **Compatibilidad:**
   - [ ] Core i2 puede interoperar con Agents i1 (broadcast fallback).
   - [ ] Agents i2 pueden conectar a Core i1 (ignoran AccountConnected, funciona con broadcast).
   - [ ] Sin breaking changes en proto (campos nuevos son opcionales).

5. **Operación:**
   - [ ] Validación manual de los 5 escenarios en entorno de desarrollo.
   - [ ] Observación de métricas en Grafana durante 1 hora de operación.
   - [ ] Ausencia de errores críticos en logs.

---

## 7. Plan de Migración y Rollout

### 7.1 Estrategia de Despliegue

**Fase 1: Desarrollo y validación local (semana 1)**
- Implementar cambios en rama `feature/i2-routing-selectivo`.
- Validación manual de escenarios en entorno de desarrollo.
- Linters limpios.

**Fase 2: Testing en staging (semana 2)**
- Desplegar Core i2 + Agents i2 en entorno de staging.
- Validar escenarios E2E manualmente.
- Observar métricas y logs en Grafana/Loki durante 24 horas.

**Fase 3: Rollout gradual en producción (semana 3)**
- **Día 1:** Desplegar Core i2 (mantiene broadcast fallback).
- **Día 2–3:** Desplegar Agents i2 uno por uno.
- **Día 4–5:** Monitorear métricas de routing:
  - % selectivo debe aumentar gradualmente.
  - % broadcast debe disminuir hasta ≤5% (solo transitorios).
- **Día 6:** Validar ausencia de regresiones en latencia/missed trades.

**Fase 4: Consolidación**
- Si métricas son saludables (≥95% routing selectivo, latencia estable), marcar i2 como completo.
- En i3+ puede considerarse eliminar código de broadcast fallback (deprecation path).

### 7.2 Rollback Plan

Si se detectan problemas (latencia, missed trades, desconexiones):

1. **Rollback Agent:** Revertir Agent a i1 (Core i2 sigue funcionando con broadcast fallback).
2. **Rollback Core:** Revertir Core a i1 si hay problemas con `AccountRegistry` (locks, memory leaks).
3. **Feature Flag (opcional i3+):** Añadir flag `enable_selective_routing=false` en config ETCD para desactivar en runtime sin redeployar.

---

## 8. Riesgos y Mitigaciones

### 8.1 Riesgos Técnicos

| Riesgo | Probabilidad | Impacto | Mitigación |
|--------|--------------|---------|------------|
| Race conditions en AccountRegistry | Media | Alto | RWMutex estricto, code review detallado |
| Owner desconectado entre lookup y send | Media | Medio | Fallback broadcast automático, retry en Agent |
| Memory leak en registry si Agents no limpian | Baja | Alto | Cleanup en defer de `StreamBidi`, validación manual de reconexiones |
| Conflictos de ownership (2 Agents misma cuenta) | Baja | Alto | Log WARNING, última escritura gana, auditoría manual en config ETCD |
| Latencia aumenta por lookup adicional | Baja | Medio | Lookup es O(1) con map, negligible |
| Pipe desconecta pero Agent no notifica | Media | Medio | Timeout en PipeManager, cleanup periódico (opcional i3+) |

### 8.2 Riesgos Operacionales

| Riesgo | Probabilidad | Impacto | Mitigación |
|--------|--------------|---------|------------|
| Config ETCD desincronizada (cuenta en 2 Agents) | Media | Medio | Validar config con script pre-deploy, alertas si conflictos |
| Agent reporta cuentas que no existen en config Core | Baja | Bajo | Log WARNING, Core ignora cuentas no configuradas |
| Reconexiones frecuentes saturan registry | Baja | Medio | Rate limit en handshake (opcional i3+) |
| Slave EA no cierra pipe correctamente | Media | Medio | Timeout en PipeManager, cleanup periódico |

---

## 9. Documentación y Comunicación

### 9.1 Documentación Técnica

**Archivos a actualizar:**

1. **README.md (Core y Agent):**
   - Sección "Arquitectura i2: Routing Selectivo Dinámico".
   - Diagrama de flujo de registro dinámico.

2. **IMPLEMENTATION_SUMMARY.md (Core):**
   - Añadir sección "Iteración 2: AccountRegistry y Routing Selectivo Dinámico".

3. **Docs internos:**
   - `docs/01-arquitectura-y-roadmap.md`: Marcar i2 como "Implementado".
   - `docs/rfcs/RFC-002-routing-selectivo.md`: Este documento (archivo permanente).

4. **Código inline:**
   - Comentarios en `account_registry.go`, `router.go`, `pipe_manager.go` y `stream.go`.
   - Ejemplos en docstrings de funciones públicas.

### 9.2 Comunicación al Equipo

**Antes de merge:**
- Sesión de code walkthrough (30 min) con equipo técnico.
- Demo de métricas en Grafana mostrando routing selectivo vs broadcast.

**Después de despliegue:**
- Post-mortem si hay incidentes.
- Update en canal Slack con métricas clave (% routing selectivo, latencia).

---

## 10. Conclusiones y Próximos Pasos

### 10.1 Resumen de Cambios

La Iteración 2 elimina el broadcast indiscriminado e introduce routing selectivo dinámico basado en ownership de cuentas:

- **Proto:** Nuevos mensajes `AccountConnected` y `AccountDisconnected` para registro dinámico.
- **Core:** Nuevo `AccountRegistry` con registro/desregistro individual y lógica de lookup en `Router`.
- **Agent:** Detección de conexión/desconexión de pipes y notificación al Core en tiempo real.
- **Observabilidad:** Métricas y logs de routing selectivo.

**Flujo clave:**
1. Slave conecta → Agent notifica → Core registra → disponible para routing.
2. Slave desconecta → Agent notifica → Core desregistra → no disponible.
3. Agent desconecta → Core limpia todas sus cuentas automáticamente.

**Impacto esperado:**
- Reducción de tráfico gRPC en ~66% (3 Agents).
- Flexibilidad: cuentas pueden conectarse/desconectarse en cualquier momento.
- Base para features futuras (catálogo de símbolos i3, specs de broker i4).

### 10.2 Próximas Iteraciones

- **i3:** Catálogo canónico de símbolos y mapeo por cuenta.
- **i4:** Especificaciones de broker (min_lot, lot_step, StopLevel).
- **i5:** Sizing con riesgo fijo (modo A).

---

## Anexos

### Anexo A: Ejemplo de Config ETCD (i2)

```yaml
# etcd prefix: /echo/core/
grpc_port: 50051
symbol_whitelist:
  - EURUSD
  - GBPUSD
  - XAUUSD

# i2: Slave accounts (Core valida cuentas en config, pero registro es dinámico)
slave_accounts:
  - "123456"  # debe conectarse dinámicamente via AccountConnected
  - "789012"
  - "345678"

default_lot_size: 0.10  # i0/i1, hardcoded hasta i5

postgres:
  host: localhost
  port: 5432
  database: echo_core
  user: echo_user
  password: changeme
  pool_max_conn: 10
  pool_min_conn: 2

keepalive:
  time: 60s
  timeout: 20s
  min_time: 10s
```

### Anexo B: Métricas Clave (Dashboard Grafana)

**Panel 1: Routing Mode Distribution**
- Query: `sum by (mode) (rate(echo_routing_mode_total[5m]))`
- Visualización: Pie chart.
- Labels: `mode=selective`, `mode=broadcast`, `mode=fallback_broadcast`.

**Panel 2: Account Lookup Results**
- Query: `sum by (result) (rate(echo_routing_account_lookup_total[5m]))`
- Visualización: Bar chart.
- Labels: `result=hit`, `result=miss`.

**Panel 3: Messages Sent per Agent**
- Query: `sum by (agent_id) (rate(echo_order_sent_total[5m]))`
- Visualización: Time series.
- Comparar i1 (todos iguales) vs i2 (proporcional a cuentas por Agent).

**Panel 4: Account Registry Stats**
- Query: `echo_account_registry_total_accounts` y `echo_account_registry_total_agents`.
- Visualización: Gauge.
- Muestra número de cuentas y Agents registrados en tiempo real.

### Anexo C: Checklist Pre-Merge

- [ ] Proto regenerado (`make proto`).
- [ ] Linters limpios (`make lint`).
- [ ] Code review aprobado por 2+ reviewers.
- [ ] Validación manual de 5 escenarios completada.
- [ ] Observación de métricas en staging durante 24 horas sin errores críticos.
- [ ] Documentación inline actualizada.
- [ ] README y IMPLEMENTATION_SUMMARY actualizados.
- [ ] Demo técnica presentada al equipo.

---

**Fin del RFC-002 (versión 2.0 - Routing Dinámico)**
