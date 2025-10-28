# Echo — Visión General (i0)

Echo es un trade copier orientado a baja latencia para replicar operaciones desde uno o más Masters hacia uno o más Slaves utilizando EAs (MT4/MT5), un Agent intermedio y un Core orquestador.

## Componentes

- Agent (Go, Windows):
  - Crea servidores de Named Pipes y espera conexión de EAs (Master y Slaves).
  - Traduce JSON↔Proto usando `sdk/domain`.
  - Envía `TradeIntent` y `ExecutionResult` al Core por gRPC bidi.
  - Recibe `ExecuteOrder`/`CloseOrder` desde el Core y los despacha a los Slaves por pipe.

- Core (Go):
  - Servidor gRPC bidi para Agents.
  - Router orquesta: valida, deduplica, transforma `TradeIntent`→`ExecuteOrder` y hace broadcast a Agents.
  - Mantiene métricas y logs con `sdk/telemetry`.

- SDK (Go):
  - Contratos Proto v1 (`sdk/pb/v1`) con `TimestampMetadata` t0…t7.
  - Transformadores JSON↔Proto y validaciones de dominio.

- EAs (MQL4/5):
  - Master: emite `trade_intent` y `trade_close` por pipe.
  - Slave: recibe `execute_order`/`close_order`, ejecuta en broker y reporta `execution_result`.

## Flujo E2E (2 Masters × 2 Slaves)

1) Master EA → Agent
   - Master se conecta (cliente) a pipe del Agent (servidor) y envía JSON `trade_intent`.
   - Agent parsea, valida, añade t1 y envía `TradeIntent` al Core por gRPC.

2) Agent → Core (gRPC bidi)
   - Core recibe (t2), valida símbolo, dedupe `trade_id`.
   - Core genera N `ExecuteOrder` (uno por Slave), clona timestamps y marca t3.

3) Core → Agent(s)
   - Core hace broadcast por los streams bidi activos.
   - Agent al recibir marca t4 y rutea por `target_account_id` al pipe del Slave correspondiente.

4) Agent → Slave EA (pipe)
   - Agent transforma Proto→JSON y escribe en el pipe del Slave.
   - Slave ejecuta; al enviar `execution_result`, incluye `trade_id` y t5-t7.

5) Slave EA → Agent → Core
   - Agent parsea `execution_result` y lo envía al Core.
   - Core actualiza dedupe a FILLED/REJECTED y registra latencia E2E si t0 y t7 están presentes.

6) Cierres (Close)
   - Master envía `trade_close` por pipe.
   - Core crea `CloseOrder` por cada Slave, rellena `target_*`, inicializa timestamps y marca t3.
   - Agent marca t4 y lo despacha al Slave destino.

## Telemetría y Timestamps

- `TimestampMetadata` (t0…t7) permite medir latencia por hop.
- Agent: t1 y t4; Core: t2 y t3; Slave: t5, t6, t7; Master: t0.
- Logs/metrics/traces via `sdk/telemetry`.

## Idempotencia y Dedupe

- Core: dedupe por `trade_id` (estado PENDING→FILLED/REJECTED) y registro de `command_id` emitidos.
- Slave EA: dedupe de `command_id` para evitar ejecuciones duplicadas.

## Routing

- Los `ExecuteOrder`/`CloseOrder` incluyen `target_client_id` y `target_account_id`.
- Agent rutea al pipe del Slave por `target_account_id`.

## Roles de Conexión

- Pipes: Agent (servidor), EAs (clientes).
- gRPC: Core (servidor), Agent (cliente). `agent-id` se envía por metadata.

## Notas i0

- Config simplificada (listas estáticas de slaves y símbolos).
- Procesamiento secuencial en Core; sin persistencia.

# Echo - Trading Copier

![Version](https://img.shields.io/badge/version-0.0.1-blue)
![Go](https://img.shields.io/badge/go-1.25-00ADD8)
![License](https://img.shields.io/badge/license-Proprietary-red)

**Echo** es un sistema de copiado de operaciones de trading algorítmico de baja latencia, diseñado para replicar operaciones desde cuentas **master** (MT4/MT5) hacia múltiples cuentas **slave** con tolerancias configurables, Money Management centralizado y observabilidad completa.

## 🎯 Objetivo

Sincronizar operaciones de trading entre cuentas manteniendo:
- **Latencia < 1s** extremo a extremo (objetivo < 100ms intra-host)
- **MagicNumber idéntico** para trazabilidad
- **Tolerancias configurables** (spread, slippage, ventanas de no-ejecución)
- **Observabilidad total** (métricas, logs, trazas)

## 📚 Documentación

- [PRD V1](docs/PRD-copiador-V1.md) - Product Requirements Document
- [Contexto Técnico](docs/trade-copier-context.md) - Problemas y estrategias
- [Roadmap](docs/roadmap-copiear-v1.md) - Plan de iteraciones
- [RFC-001: Arquitectura](docs/RFC-001-architecture.md) - Decisiones arquitectónicas

## 🏗️ Arquitectura

```
Master EA (MT4/MT5)
    ↓ Named Pipes
Agent (Go, Windows Service)
    ↓ gRPC Bidi
Core (Go, Orquestador)
    ↓ gRPC Bidi
Agent (Go, Windows Service)
    ↓ Named Pipes
Slave EA (MT4/MT5)
```

### Componentes

- **core/**: Servicio orquestador central (gRPC, motor de copia, MM, políticas)
- **agent/**: Bridge entre IPC (Named Pipes) y gRPC
- **sdk/**: Librería transversal (proto, telemetry, IPC, domain)
- **clients/**: Conectores (MT4, MT5, Ninja, etc.)
- **test_e2e/**: Tests end-to-end cross-component

## 🚀 Quick Start

### Prerrequisitos

- Go 1.25+
- PostgreSQL 16+
- etcd v3
- Buf CLI (para proto generation)

### Instalación

```bash
# Clonar
git clone https://github.com/xKoRx/echo.git
cd echo

# Instalar herramientas
make install-tools

# Generar proto
make proto

# Compilar
make build

# Ejecutar core
./bin/echo-core

# Ejecutar agent (en otra terminal)
./bin/echo-agent
```

## 📦 Estructura del Proyecto

```
echo/
├── core/          # Servicio orquestador
├── agent/         # Bridge gRPC ↔ IPC
├── sdk/           # Librería transversal
│   ├── proto/     # Contratos gRPC
│   ├── pb/        # Código generado
│   ├── telemetry/ # Observabilidad
│   └── ipc/       # Named Pipes helpers
├── clients/       # Conectores (MT4, MT5, etc.)
├── test_e2e/      # Tests E2E
├── docs/          # Documentación y RFCs
└── tools/         # Herramientas de desarrollo
```

## 🧪 Testing

```bash
# Tests unitarios
make test

# Tests E2E
make test-e2e

# Linters
make lint

# Coverage
go test -coverprofile=coverage.out ./...
```

## 🛠️ Desarrollo

El proyecto usa **Go workspaces** para facilitar el desarrollo local:

```bash
# go.work permite trabajar con múltiples módulos
cd core && go test ./...
cd agent && go test ./...
cd sdk && go test ./...
```

### Workflow

1. Crear feature branch: `git checkout -b feature/nombre`
2. Hacer cambios y tests
3. Ejecutar linters: `make lint`
4. Commit: `git commit -m "feat: descripción"`
5. Push y PR

## 📋 Roadmap

- [x] **Iteración 0**: POC 48h (1 master → 1 slave, market only)
- [ ] **Iteración 1**: Persistencia robusta
- [ ] **Iteración 2**: SL catastrófico + filtros
- [ ] **Iteración 3**: Mapeo símbolos
- [ ] **Iteración 4**: Sizing configurable
- [ ] **Iteración 5**: Multi-slave
- [ ] **Iteración 6**: SL/TP con offset
- [ ] **Iteración 7**: Empaquetado V1

Ver [roadmap completo](docs/roadmap-copiear-v1.md)

## 🤝 Contribuir

Este es un proyecto privado. Contactar al equipo para acceso.

## 📄 Licencia

Proprietary - Aranea Labs © 2025

## 📞 Contacto

- Equipo: Aranea Labs - Trading Copier Team
- Documentación: `/docs`
