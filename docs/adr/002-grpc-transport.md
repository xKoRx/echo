# ADR-002: gRPC Bidi-Streaming para Core ↔ Agent

## Estado
**Aprobado** - 2025-10-23

## Contexto

Necesitamos un protocolo de comunicación eficiente entre el **Core** (orquestador) y los **Agents** (bridges locales) que soporte:

- Comunicación bidireccional
- Baja latencia
- Streaming de eventos
- Tipado fuerte
- Cross-platform (Linux ↔ Windows)

Opciones consideradas:

1. **gRPC bidi-streaming**
2. **WebSocket** con JSON
3. **REST** con polling/long-polling
4. **NATS/MQTT** (message broker)

## Decisión

Usaremos **gRPC bidireccional streaming** con Protobuf.

### Servicio

```protobuf
service AgentService {
  rpc StreamBidi(stream AgentMessage) returns (stream CoreMessage);
  rpc Ping(PingRequest) returns (PingResponse);
}
```

### Flujo

```
Agent → Core: TradeIntent, ExecutionResult, Heartbeat
Core → Agent: ExecuteOrder, CloseOrder, ConfigUpdate, Ack
```

## Consecuencias

### Positivas
- ✅ **Latencia baja**: Protocolo binario eficiente (HTTP/2)
- ✅ **Tipado fuerte**: Contratos Protobuf versionados
- ✅ **Streaming nativo**: Bidi sin polling
- ✅ **Generación de código**: Automática para Go, Python, C#, etc.
- ✅ **Interoperabilidad**: Clientes custom fáciles de integrar
- ✅ **Backpressure**: Manejo nativo en gRPC

### Negativas
- ⚠️ **Complejidad vs REST**: Mayor curva de aprendizaje
- ⚠️ **Debugging**: Requiere herramientas específicas (grpcurl, Postman)
- ⚠️ **Browser support**: No directo (requiere grpc-web)

### Alternativas Descartadas

**WebSocket + JSON**:
- ❌ Sin tipado fuerte
- ❌ Serialización JSON más lenta
- ❌ Generación de código manual

**REST con polling**:
- ❌ Latencia alta por polling
- ❌ No streaming nativo
- ❌ Overhead HTTP/1.1

**NATS/MQTT**:
- ❌ Dependencia externa (broker)
- ❌ Complejidad operacional
- ❌ Overkill para V1 (local)

## Implementación

### Core (Server)

```go
import (
    "google.golang.org/grpc"
    echov1 "github.com/xKoRx/echo/sdk/pb/v1"
)

server := grpc.NewServer()
echov1.RegisterAgentServiceServer(server, &coreService{})
server.Serve(listener)
```

### Agent (Client)

```go
conn, err := grpc.Dial("core:50051", grpc.WithInsecure())
defer conn.Close()

client := echov1.NewAgentServiceClient(conn)
stream, err := client.StreamBidi(ctx)

// Send
stream.Send(&echov1.AgentMessage{...})

// Receive
for {
    msg, err := stream.Recv()
    // ...
}
```

## TLS

- **V1**: Opcional (self-signed para testing)
- **V2+**: mTLS obligatorio en producción

## Métricas

Instrumentar con OTEL:
- `grpc.server.call.duration`
- `grpc.server.call.started`
- `grpc.server.call.succeeded`

## Referencias
- [gRPC Go](https://grpc.io/docs/languages/go/)
- [Protobuf](https://protobuf.dev/)
- [RFC-001](../RFC-001-architecture.md#42-configuraci%C3%B3n-en-etcd)

