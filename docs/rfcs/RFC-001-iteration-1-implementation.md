---
title: "RFC-003 — Iteración 1: Persistencia, Idempotencia y Salud Operacional"
version: "1.0"
date: "2025-10-28"
status: "Proposed"
authors: ["Aranea Labs - Trading Copier Team"]
depends_on: ["RFC-001", "RFC-002"]
---

# RFC-003 — Iteración 1 (i1)

## 1. Resumen Ejecutivo

La Iteración 1 incrementa la madurez del sistema Echo desde el POC de i0 hacia un servicio mínimamente confiable: añade persistencia (PostgreSQL), refuerza la idempotencia end-to-end con `UUIDv7`, establece salud operacional (heartbeats, retries acotados, keepalive gRPC correcto) y cierra brechas de auditoría correlacionando de forma determinística `trade_id ↔ ticket(s)` por slave. Se consolidan métricas de latencia y resultado, y se alinea la configuración hacia ETCD para eliminar hardcodes de i0.

Objetivo: mantener el flujo Master → Core → Slaves estable por horas, sin duplicados, con cierres correctamente correlacionados por ticket, y con telemetría accionable para depurar y medir latencia.

---

## 2. Alcance i1 (según RFC-001 · sección Roadmap)

- Persistencia: PostgreSQL 16 para estado mínimo (órdenes y correlación).
- Idempotencia reforzada: `trade_id` UUIDv7 end-to-end; dedupe persistente.
- Salud operacional: heartbeats básicos y retry simple (acotado, sin price-chase).
- Métricas clave: latencia por hop y E2E, resultados (ok/error) y rechazos.
- Correlación determinística de cierres: mapa `trade_id → {account_id → [tickets]}` persistente.
- Cierre por ticket exacto: `CloseOrder` debe incluir el `ticket` del slave (no 0).
- Estampar `trade_id` en `comment` al abrir (fallback de correlación).
- KeepAlive gRPC: parámetros compatibles y documentados (evitar "too many pings").
- Named Pipes: `FlushFileBuffers` desactivado por defecto; sólo benchmark controlado.
- Configuración: transición progresiva a ETCD (mínimo: endpoints, toggles de retries/flush, límites de colas), usando `github.com/xKoRx/echo/sdk/etcd` (copiado de shared).

Fuera de alcance en i1: políticas avanzadas, ventanas de no-ejecución, sizing por riesgo fijo (se mantiene lot fijo 0.10), mapeo de símbolos, SL/TP, concurrencia full (ver 9).

---

## 3. Objetivos y Criterios de Éxito

### Objetivos
1. Persistir correlación `trade_id ↔ slave_ticket(s)` y resultados.
2. Evitar duplicados en reinicios del Core/Agent (dedupe persistente + TTL).
3. Cerrar por `ticket` exacto, sin ambigüedad con `magic_number`.
4. Mantener stream gRPC estable (> 1h) con KeepAlive correcto.
5. Telemetría visible (latencias hop/E2E, tasa de éxito, errores por código).

### Criterios de éxito
- 20 ejecuciones consecutivas sin duplicados (con reinicio de Core entre medias).
- 100% de cierres correlacionados por `ticket` exacto del slave.
- p95 latencia E2E intra-host ≤ 120 ms (igual o mejor que i0).
- gRPC stream sin desconexiones espurias (sin logs de "too many pings").
- Métricas `echo.latency.*` y `echo.*.count` disponibles en Prometheus.

---

## 4. Cambios Arquitectónicos Clave

1) Persistencia mínima (PostgreSQL):
- Registrar órdenes emitidas y resultados por slave. Persistir correlación `trade_id ↔ slave_ticket` por cuenta.
- Dedupe persistente por `trade_id` con estados (`PENDING`, `SENT`, `FILLED`, `REJECTED`).

2) Idempotencia reforzada:
- `trade_id` = UUIDv7 end-to-end (Master EA → Agent → Core → Agent → Slave EA).
- `comment` de apertura en slave incluirá `trade_id` como fallback.

3) Salud operacional:
- Heartbeats suaves en `AgentService` (Agent/Core) y KeepAlive gRPC con umbrales documentados.
- Retries simples y acotados sólo en canales de transporte (no price-chase).

4) Configuración central:
- Mover a ETCD (v3) las claves mínimas: endpoints Core/OTLP, toggles (`agent.flush_force`, `agent.retry_enabled`), límites (colas, buffers), y parámetros de KeepAlive. Cliente oficial: `github.com/xKoRx/echo/sdk/etcd` (copiado de shared).

5) Observabilidad:
- Consolidar métricas/trazas/logs con `github.com/xKoRx/echo/sdk/telemetry`. Mantener `EchoMetrics` como bundle de métricas de dominio.

6) IPC Named Pipes:
- Desactivar `FlushFileBuffers` por defecto (bloqueos). Ofrecer toggle sólo para escenarios de benchmark controlado.

7) Proto y compatibilidad:
- Normalizar a paquete `echo.v1` y `github.com/xKoRx/echo/sdk/pb/v1`. Deprecar cualquier referencia a `v0` en binarios i1.

---

## 5. Modelo de Datos (PostgreSQL 16)

> Nota: esquemas mínimos para i1. Se consolidarán en i2+ con normalización adicional.

```sql
-- Estados de orden y ejecución
-- order_status: PENDING | SENT | FILLED | REJECTED | CANCELLED

CREATE TABLE IF NOT EXISTS trades (
  trade_id        TEXT PRIMARY KEY,
  source_master_id TEXT NOT NULL,
  master_account_id TEXT NOT NULL,
  master_ticket   INTEGER NOT NULL,
  magic_number    BIGINT NOT NULL,
  symbol          TEXT NOT NULL,
  side            TEXT NOT NULL,         -- BUY/SELL
  opened_at_ms    BIGINT NOT NULL,
  status          TEXT NOT NULL DEFAULT 'PENDING',
  attempt         INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS executions (
  command_id      TEXT PRIMARY KEY,
  trade_id        TEXT NOT NULL REFERENCES trades(trade_id) ON DELETE CASCADE,
  slave_account_id TEXT NOT NULL,
  agent_id        TEXT NOT NULL,
  slave_ticket    INTEGER NOT NULL,      -- Ticket concreto en el slave
  executed_price  DOUBLE PRECISION,
  success         BOOLEAN NOT NULL,
  error_code      TEXT NOT NULL,         -- ej: ERR_NO_ERROR, ERR_REQUOTE
  error_message   TEXT NOT NULL,
  timestamps_ms   JSONB NOT NULL,        -- t0..t7 (ver RFC-002)
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Índice para correlación determinística de cierres
CREATE UNIQUE INDEX IF NOT EXISTS ux_trade_slave_ticket
ON executions (trade_id, slave_account_id, slave_ticket);

-- Dedupe persistente por trade_id
CREATE TABLE IF NOT EXISTS dedupe (
  trade_id   TEXT PRIMARY KEY,
  status     TEXT NOT NULL,              -- PENDING/SENT/FILLED/REJECTED
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Cierres (auditoría) - opcional en i1 (puede inferirse de updates de executions)
CREATE TABLE IF NOT EXISTS closes (
  close_id        TEXT PRIMARY KEY,
  trade_id        TEXT NOT NULL REFERENCES trades(trade_id) ON DELETE CASCADE,
  slave_account_id TEXT NOT NULL,
  slave_ticket    INTEGER NOT NULL,
  close_price     DOUBLE PRECISION,
  success         BOOLEAN NOT NULL,
  error_code      TEXT NOT NULL,
  error_message   TEXT NOT NULL,
  closed_at_ms    BIGINT NOT NULL
);
```

Consideraciones:
- `executions` materializa la relación `trade_id → [tickets]` por slave, suficiente para cerrar por ticket exacto.
- `dedupe` permite sobrevivir reinicios. TTL de limpieza: 1 hora para estados terminales.
- `timestamps_ms` permanece como JSONB para simplicidad i1; se normalizará en i2 si es necesario.

---

## 6. Contratos Proto (i1)

Estándar único: `echo.v1` (`github.com/xKoRx/echo/sdk/pb/v1`). Cambios i1:

- `ExecutionResult` (sin cambios de forma; énfasis en obligatoriedad de timestamps t0..t7 y `success/error_code`).
- `CloseOrder` debe especificar `ticket` del slave (no 0), derivado de `executions`.
- `TradeIntent` y `ExecuteOrder` refuerzan `trade_id` = UUIDv7. Validación de forma en SDK.

Recomendación EA (no-proto): incluir `trade_id` en el `comment` del `OrderSend` del slave.

---

## 7. KeepAlive gRPC y Heartbeats

Parámetros recomendados (compatibles servidor/cliente para evitar "too many pings"):

Cliente (Agent → Core):
- ping time: ≥ 60s
- ping timeout: 20s
- permit without stream: false

Servidor (Core):
- min time entre pings aceptables: ≥ 10s
- timeout: 20s

Heartbeats de aplicación (ligeros):
- `AgentHeartbeat` y `CoreHeartbeat` cada 30s-60s para señal de vida y timestamp.
- Sin payload de estado pesado en i1 (eso llega en i2+).

---

## 8. Retries Simples (transporte)

- Reintentos sólo para errores transitorios de transporte (gRPC `Unavailable`, `DeadlineExceeded`).
- Backoff exponencial con jitter, máx 3 intentos, ventana total ≤ 5s.
- No se reintenta `OrderSend/OrderClose` en el slave (sigue siendo i0: "omit with comment").

---

## 9. Concurrencia y Orden

Estado actual i0: procesamiento secuencial en Core. En i1 mantenemos el pipeline secuencial para minimizar riesgo, pero:

- Envío a Agents no bloqueante (canales con buffer + goroutine de escritura por stream).
- Se introduce un "serialized processor" por `trade_id` en SDK (hash por `trade_id` opcional), quedando desactivado por defecto en i1; activación prevista en i2.

Regla: mantener orden FIFO por `trade_id` y por stream. Evitar reordenamientos que rompan idempotencia.

---

## 10. Configuración en ETCD (mínima i1)

Prefijo: `/echo/`.

```text
/echo/
  /endpoints/
    /core_addr                → ej: "localhost:50051"
    /otel/otlp_endpoint       → ej: "192.168.31.60:4317"
  /agent/
    /retry_enabled            → true|false (default: true)
    /max_retries              → 3
    /flush_force              → false (usar solo en benchmark controlado)
  /grpc/
    /keepalive/time_s         → 60
    /keepalive/timeout_s      → 20
    /keepalive/min_time_s     → 10
```

Cliente: `github.com/xKoRx/echo/sdk/etcd`. Carga única al inicio + watches para cambios.

---

## 17. Aplicaciones a modificar (alto nivel)

- Core (Go, gRPC server):
  - Integrar Postgres para `trades`, `executions`, `dedupe`.
  - Dedupe persistente por `trade_id` (estados) y correlación de cierres por `ticket`.
  - KeepAlive/heartbeats en gRPC; política de reintentos de transporte.
  - Carga de configuración desde ETCD (endpoints, toggles, límites, KA).
  - Telemetría con `echo/sdk/telemetry` + `EchoMetrics`.

- Agent (Go, Windows Service):
  - Heartbeats a Core; respeto de KeepAlive cliente.
  - Toggle `FlushFileBuffers=false` por defecto; sólo benchmark bajo flag.
  - Enriquecimiento de timestamps (t4) y forwarding sin duplicación.
  - Carga de configuración desde ETCD.

- SDK de Echo (Go):
  - `pb/v1` estable; validadores de forma (UUIDv7, campos obligatorios).
  - `telemetry`: bundle `EchoMetrics` (latencias hop/E2E, resultados, errores).
  - `etcd`: cliente y helpers de carga/watches.
  - `domain`: transformers y validaciones; helpers de timestamps.
  - (Opcional) `grpc` helpers para KeepAlive y backoff standard.

- EAs MQL4 (Master/Slave):
  - Master: sin cambios funcionales, asegurar `trade_id` UUIDv7.
  - Slave: incluir `trade_id` en `comment` de `OrderSend`; timestamps t5–t7 completos en `ExecutionResult`.

---

## 18. Mapa de capacidades i1

- Impactadas (afectadas indirectamente):
  - Observabilidad E2E (nuevas métricas y spans; mismas rutas).
  - Transporte gRPC (KeepAlive y estabilidad de stream).
  - IPC Named Pipes (política de flush).

- Modificadas (evolución de existentes):
  - Idempotencia/dedupe: de in-memory a persistente con estados.
  - Cierre: de `magic_number` a `ticket` exacto por correlación persistida.
  - Configuración: de hardcodes a ETCD (mínimo viable i1).

- Nuevas (no existían en i0):
  - Persistencia de `trades`/`executions`/`dedupe` en Postgres.
  - Heartbeats ligeros Agent/Core.
  - Política documentada de KeepAlive gRPC.
  - Estampado de `trade_id` en `comment` del slave (fallback de correlación).

---

## 19. Componentes a crear/actualizar y responsabilidades

- Core
  - `core/repository` (nuevo): interfaces + impl Postgres para `trades`, `executions`, `dedupe`.
  - `core/services/dedupe` (nuevo): transición de estados, TTL de limpieza.
  - `core/services/correlation` (nuevo): `trade_id → {account_id → [tickets]}`; selección de `ticket` para `CloseOrder`.
  - `core/services/telemetry` (actualizar): métricas E2E/hop, errores por código.
  - `core/config` (nuevo): carga/watches de ETCD; exponer opciones inmutables.
  - `core/grpc/stream` (actualizar): KeepAlive server, loop de lectura/escritura con canales con buffer.
  - `core/migrations` (nuevo): migraciones SQL versionadas.

- Agent
  - `agent/config` (nuevo): carga/watches ETCD; flags `flush_force`, `retry_enabled`.
  - `agent/ipc` (actualizar): `FlushFileBuffers=false` por defecto; writer/reader line-delimited.
  - `agent/grpc/client` (actualizar): KeepAlive cliente; backoff de reconexión (transporte).
  - `agent/heartbeat` (nuevo): envío periódico de `AgentHeartbeat`.
  - `agent/telemetry` (actualizar): métricas de forwarding y delivery a pipes.

- SDK Echo
  - `sdk/etcd` (nuevo en echo): cliente con API simple + watches.
  - `sdk/telemetry` (actualizar): `EchoMetrics` consolidado.
  - `sdk/domain` (actualizar): validaciones/transformers i1.
  - `sdk/utils` (actualizar): UUIDv7, timestamps helpers consistentes.

- EAs
  - `SlaveEA`: helper para insertar `trade_id` en comment; reporte completo t5–t7.

---

## 20. Recomendación de abordaje de la iteración

Orden sugerido (SDK-first):
1) SDK Echo: `pb/v1` validado, `telemetry` (EchoMetrics), `etcd`, `domain/utils`.
2) Core: migraciones SQL → repositorios → servicios (dedupe/correlation) → gRPC server (KeepAlive) → config ETCD → telemetría.
3) Agent: config ETCD → gRPC cliente (KeepAlive/backoff) → IPC writer/reader sin flush forzado → heartbeats → telemetría.
4) EAs: ajustes mínimos en SlaveEA (comment con `trade_id`, timestamps completos).
5) Pruebas: unitarias SDK; integración Core/Agent+DB; resiliencia KeepAlive/IPC; métricas visibles.

Buenas prácticas (obligatorias):
- Código escalable, modular, clean code y SOLID (SRP, OCP, LSP, ISP, DIP).
- Programar contra interfaces; inyección de dependencias; bajo acoplamiento entre módulos.
- Errores: jamás ignorarlos; propagación explícita; sin capturas vacías.
- Contexto: propagar `context.Context`; no crear `context.Background()` en hot-path; atributos vía `telemetry`.
- Config: sólo ETCD (`github.com/xKoRx/echo/sdk/etcd`); carga una vez y watches; sin `os.Getenv` (salvo excepciones aprobadas).
- Observabilidad: spans cerrados, `RecordError`, métricas sin cardinalidad explosiva.
- Performance: canales con buffer, no bloquear goroutines de stream; evitar I/O sin necesidad en hot-path.
- Testing: table-driven, mocks de interfaces, cobertura de rutas de error.

Definición de listo (DoD) i1:
- Migraciones aplicadas; dedupe persistente operando; cierres por `ticket` verificados.
- KeepAlive estable (>60 min); 20 ejecuciones sin duplicados; métricas E2E en Prometheus.
- Config desde ETCD activa; toggles funcionales; telemetría visible en Jaeger/Loki.

---

## 11. Observabilidad (i1)

- SDK oficial de Echo: `github.com/xKoRx/echo/sdk/telemetry`.
- Bundle de dominio: `EchoMetrics` (latencias E2E/hop, contadores de funnel, rechazos, errores por código).
- Atributos por contexto: usar `AppendCommonAttrs`, `AppendEventAttrs`, `AppendMetricAttrs` al inicio del flujo para no repetir.

Métricas mínimas i1:
- `echo.intent.received`, `echo.intent.forwarded`, `echo.order.created`, `echo.order.sent`, `echo.execution.completed`.
- `echo.latency.e2e`, `echo.latency.agent_to_core`, `echo.latency.core_process`, `echo.latency.core_to_agent`, `echo.latency.slave_execution`.
- Contadores de errores por `error_code` y resultados `success|rejected`.

---

## 12. Plan de Pruebas i1

Unitarias (SDK):
- Validación de UUIDv7, transformadores Proto↔Domain↔JSON, dedupe persistente.

Integración (Core/Agent con DB):
- E2E: BUY/SELL desde Master → ExecuteOrder → ExecutionResult, con persistencia y correlación por ticket.
- Reinicio de Core entre intents 5 y 6: sin duplicados.
- Cierre: TradeClose → CloseOrder con `ticket` exacto.
- KeepAlive estable (stream > 60 min) sin picos de pings.

Métricas/Observabilidad:
- Verificar publicación de métricas y spans básicos.

Cobertura objetivo i1:
- SDK ≥ 85% líneas (≥ 95% rutas críticas). Core/Agent componentes nuevos ≥ 80%.

---

## 13. Migración desde i0

1) Actualizar a `pb/v1` en todos los binarios; eliminar referencias a `v0`.
2) Introducir uso de ETCD para endpoints y toggles mínimos.
3) Habilitar persistencia PostgreSQL (migraciones iniciales incluidas).
4) Alinear telemetry al SDK de Echo y mantener `EchoMetrics` como bundle de dominio.
5) EAs: asegurar `trade_id` en comment al abrir.

Compatibilidad: i1 debe aceptar mensajes i0 si vienen por `pb/v1` y contienen campos conocidos; mensajes `pb/v0` quedan no soportados.

---

## 14. Riesgos y Mitigaciones (alto nivel)

- Duplicados tras reinicio: mitigado con `dedupe` persistente + actualización de estado en `executions`.
- KeepAlive agresivo: parámetros mínimos recomendados, test de resiliencia documentado.
- Bloqueos por `FlushFileBuffers`: deshabilitado por defecto; uso sólo bajo flag.
- Cardinalidad de métricas: atributos controlados; evitar IDs de usuario en labels.
- Divergencias de timestamps (GetTickCount): latencias relativas; documentar reinicios y overflow.

---

## 15. Entregables i1

- Código: migraciones SQL, integración Postgres en Core; adopción ETCD mínima; refactor de telemetry a cliente compartido; ajustes de KeepAlive; heartbeats.
- Docs: guía de despliegue i1, configuración ETCD, parámetros de KeepAlive, troubleshooting.
- Evidencia: logs, JSON de respuestas, métricas en Prometheus, capturas de Jaeger y Loki.

---

## 16. Preguntas Abiertas

1) ¿Concurrencia por `trade_id` se activa en i1 o se pospone formalmente a i2?
2) ¿Extender `echo/sdk/telemetry` con utilidades necesarias o crear adaptadores internos mínimos?
3) ¿Campos extra en `ExecutionResult` para slippage y spread local desde el slave (útil ya en i1)?
4) ¿Dominios adicionales en métricas (por cuenta/símbolo) sin explotar cardinalidad? Definir lista blanca.

---

Fin RFC-003 i1


