# Echo Agent

**Versión:** 0.1.0 (Iteración 0 - POC)

## Descripción

El Agent es el componente bridge entre los Expert Advisors (MQL4/MQL5) y el Core de Echo. Actúa como una **thin layer** que solo hace routing, utilizando exclusivamente la SDK para toda la lógica.

## Responsabilidades

- **Named Pipes Server**: Crea y gestiona Named Pipes para comunicación con EAs (1 pipe por EA)
- **gRPC Client**: Mantiene stream bidireccional persistente con el Core
- **Routing**: Transforma mensajes JSON ↔ Proto usando `sdk/domain`
- **Telemetría**: Logs estructurados + métricas EchoMetrics usando `sdk/telemetry`

## Arquitectura SDK-First

El Agent **NO reimplementa** lógica de:
- ❌ Parsing JSON (usa `sdk/domain.JSONToTradeIntent()`)
- ❌ Transformaciones Proto ↔ JSON (usa `sdk/domain`)
- ❌ Named Pipes (usa `sdk/ipc.WindowsPipeServer`)
- ❌ gRPC (usa `sdk/grpc.Client`)
- ❌ Telemetría (usa `sdk/telemetry.Client` + `EchoMetrics`)

✅ Solo implementa: **routing** y **orquestación** específica del Agent.

## Componentes Internos

### `agent.go`
Componente principal que orquesta:
- Conexión al Core via gRPC
- Gestión del PipeManager
- Lifecycle (Start/Shutdown)

### `core_client.go`
Wrapper sobre `sdk/grpc.Client`:
- Conecta al Core
- Crea stream bidireccional
- Health checks (Ping)

### `pipe_manager.go`
Gestiona Named Pipes:
- Crea pipes para masters y slaves
- Spawns `PipeHandler` por cada pipe
- Registry de pipes activos

### `PipeHandler`
Maneja un pipe específico:
- Lee mensajes JSON (usa `sdk/ipc.JSONReader`)
- Transforma JSON → Proto (usa `sdk/domain`)
- Escribe mensajes Proto → JSON (usa `sdk/ipc.JSONWriter`)
- Envía mensajes al Core via canal

### `stream.go`
Maneja el stream gRPC:
- Goroutine de envío (`sendToCore`)
- Goroutine de recepción (`receiveFromCore`)
- Routing de mensajes del Core a pipes

### `telemetry.go`
Inicialización de telemetría:
- Usa `sdk/telemetry.New()`
- Bundle EchoMetrics incluido automáticamente

### `pipe_manager_stub.go`
Stubs para plataformas no-Windows:
- Permite compilar en Linux/Mac para desarrollo
- Retorna errores si se intenta usar (Named Pipes solo en Windows)
- Activado con build tag `!windows`

## Configuración (i0 - Hardcoded)

```go
config := &Config{
    CoreAddress:    "localhost:50051",
    PipePrefix:     "echo_",
    MasterAccounts: []string{"12345"},  // TODO i1: desde etcd
    SlaveAccounts:  []string{"67890"},  // TODO i1: desde etcd
    ServiceName:    "echo-agent",
    ServiceVersion: "0.1.0",
    Environment:    "dev",
}
```

## Flujo de Mensajes

### Master EA → Core

1. Master EA escribe JSON a pipe `\\.\pipe\echo_master_12345`
2. `PipeHandler` lee con `sdk/ipc.JSONReader`
3. Transforma JSON → Proto con `sdk/domain.JSONToTradeIntent()`
4. Envía `AgentMessage{TradeIntent}` al canal `sendToCoreCh`
5. Goroutine `sendToCore()` envía por stream gRPC

### Core → Slave EA

1. Goroutine `receiveFromCore()` recibe `CoreMessage{ExecuteOrder}`
2. `routeExecuteOrder()` busca pipe del slave
3. `PipeHandler.WriteMessage()` transforma Proto → JSON con `sdk/domain.ExecuteOrderToJSON()`
4. Escribe JSON al pipe con `sdk/ipc.JSONWriter`
5. Slave EA lee del pipe

## Named Pipes

### Nomenclatura
- Masters: `\\.\pipe\echo_master_<account_id>`
- Slaves: `\\.\pipe\echo_slave_<account_id>`

### Protocolo
- Formato: JSON line-delimited (cada mensaje termina con `\n`)
- Codificación: UTF-8
- Buffering: 1MB (via `sdk/ipc`)

## Métricas (EchoMetrics)

El Agent registra:
- `echo.intent.received`: TradeIntents recibidos de Masters
- `echo.intent.forwarded`: TradeIntents enviados al Core
- `echo.execution.dispatched`: ExecuteOrders enviados a Slaves

Atributos:
- `echo.trade_id`: UUID del trade
- `echo.client_id`: ID del EA (master_xxx o slave_xxx)
- `echo.symbol`: Símbolo (XAUUSD en i0)

## Build

### Windows (target platform - producción)
```bash
# Cross-compilation desde Linux/Mac
GOOS=windows GOARCH=amd64 go build -o bin/echo-agent.exe ./cmd/echo-agent

# Desde Windows
go build -o bin/echo-agent.exe ./cmd/echo-agent
```

### Linux/Mac (desarrollo - stub)
```bash
# Compila con stubs de Named Pipes (no funcional, solo para desarrollo)
go build -o bin/echo-agent ./cmd/echo-agent
go build cmd/echo-agent/main.go  # También funciona
```

**Nota**: En plataformas no-Windows, el Agent compila pero no es funcional ya que Named Pipes solo están disponibles en Windows. Los stubs permiten desarrollo y testing de sintaxis.

## Ejecución

```bash
# Windows
bin\echo-agent.exe

# El agent:
# 1. Conecta a Core en localhost:50051
# 2. Crea pipes echo_master_12345 y echo_slave_67890
# 3. Espera conexiones de EAs
# 4. Logs a stdout (JSON estructurado)
```

## TODOs para Iteración 1

- [ ] Configuración desde etcd (accounts dinámicos)
- [ ] Reconexión automática de gRPC con backoff
- [ ] Reconexión automática de Named Pipes
- [ ] Health checks periódicos al Core
- [ ] Métricas de latencia por hop
- [ ] Agregar timestamps t1, t4, t5 a mensajes
- [ ] Registry dinámico de clientes (no hardcoded)
- [ ] Routing inteligente (no broadcast a todos)
- [ ] OTLP Collector para telemetría
- [ ] Servicio de Windows (en lugar de proceso)

## Dependencias

- `github.com/xKoRx/echo/sdk` → Proto, domain, gRPC, IPC, telemetría, utils
- `google.golang.org/grpc` → gRPC (via SDK)
- `github.com/Microsoft/go-winio` → Named Pipes (via SDK)
- `go.opentelemetry.io/otel` → Telemetría (via SDK)

## Testing

### Unit Tests
```bash
go test ./internal/...
```

### Integration Tests (i1+)
```bash
# Requiere Core corriendo
go test -tags=integration ./...
```

## Logs Ejemplo

```json
{"time":"2025-10-26T10:00:00Z","level":"INFO","msg":"Agent starting","core_address":"localhost:50051","version":"0.1.0"}
{"time":"2025-10-26T10:00:00Z","level":"INFO","msg":"Connected to Core"}
{"time":"2025-10-26T10:00:00Z","level":"INFO","msg":"Pipe created","pipe_name":"echo_master_12345","role":"master"}
{"time":"2025-10-26T10:00:00Z","level":"INFO","msg":"Pipe created","pipe_name":"echo_slave_67890","role":"slave"}
{"time":"2025-10-26T10:00:01Z","level":"INFO","msg":"EA connected","pipe_name":"echo_master_12345","role":"master"}
{"time":"2025-10-26T10:00:02Z","level":"INFO","msg":"TradeIntent received","trade_id":"01HKQV8Y...","symbol":"XAUUSD"}
{"time":"2025-10-26T10:00:02Z","level":"INFO","msg":"TradeIntent forwarded to Core","trade_id":"01HKQV8Y..."}
```

## Cumplimiento RFC-002

✅ **Fase 3 Completada**:
- [x] Binario Go que arranca como proceso
- [x] Servidor Named Pipes usando `sdk/ipc` (1 pipe por EA)
- [x] Cliente gRPC al Core usando `sdk/grpc.StreamClient`
- [x] Routing pipe → stream: JSON → Domain → Proto
- [x] Routing stream → pipe: Proto → Domain → JSON
- [x] Telemetría usando `sdk/telemetry.EchoMetrics`
- [x] Agent **NO reimplementa** lógica (todo vía SDK)
- [x] Crea pipes y acepta conexiones de EAs
- [x] Stream gRPC persistente
- [x] Logs muestran mensajes entrantes/salientes
- [x] Métricas registradas correctamente

## Referencias

- [RFC-002: Iteración 0](../../docs/rfcs/RFC-002-iteration-0-implementation.md)
- [SDK Documentation](../sdk/README.md)
- [Named Pipes IPC](../sdk/ipc/README.md)
