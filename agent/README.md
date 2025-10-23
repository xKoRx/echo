# Agent - Bridge gRPC ↔ IPC

El **agent** es un servicio Windows que actúa como puente entre:

- **Clientes** (EAs MT4/MT5) vía **Named Pipes** (IPC)
- **Core** vía **gRPC bidireccional**

## Responsabilidades

- Conectar al core vía gRPC
- Exponer servidor Named Pipes para EAs locales
- Routing de mensajes en ambas direcciones
- Coalescing de eventos (ticks, account info)
- Heartbeats periódicos

## Estructura

```
agent/
├── cmd/
│   └── echo-agent/
│       └── main.go         # Entry point (Windows Service)
├── internal/
│   ├── grpc/               # Cliente gRPC al core
│   ├── ipc/                # Servidor Named Pipes
│   └── bridge/             # Lógica de routing
└── README.md
```

## Flujo de Datos

### Master → Core

```
Master EA → Named Pipe (TradeIntent JSON) → Agent → gRPC → Core
```

### Core → Slave

```
Core → gRPC (ExecuteOrder) → Agent → Named Pipe (JSON) → Slave EA
```

## Named Pipes

El agent crea pipes con nombres predecibles:

```
\\.\pipe\echo_master_{EA_ID}
\\.\pipe\echo_slave_{EA_ID}
```

Cada EA se conecta al pipe correspondiente al iniciar.

## Configuración

El agent recibe config inicial del core vía gRPC al conectar.

Parámetros mínimos:

```yaml
core_address: "localhost:50051"
agent_id: "AGENT-WIN-001"
```

## Ejecución

```bash
cd agent
go run ./cmd/echo-agent

# Compilar para Windows:
GOOS=windows GOARCH=amd64 go build -o echo-agent.exe ./cmd/echo-agent
```

## Windows Service

Para instalarlo como servicio Windows:

```powershell
# Crear servicio
sc create EchoAgent binPath= "C:\path\to\echo-agent.exe"

# Iniciar
sc start EchoAgent

# Estado
sc query EchoAgent
```

## Tests

```bash
cd agent
go test -v ./...
```

## Dependencias

- `github.com/xKoRx/echo/sdk`: Proto, IPC helpers, telemetry
- `google.golang.org/grpc`: Cliente gRPC
- Named Pipes (stdlib `net` package)

## Próximos Pasos (Iteración 0)

- [ ] Cliente gRPC funcional
- [ ] Servidor Named Pipes básico
- [ ] Bridge simple entre ambos
- [ ] Heartbeats cada 10s

