# Plan de implementación i17 — end-to-end-lossless-retry

## 0. Resumen
- Objetivo: garantizar delivery lossless Master↔Agent↔Core↔Slave con journal, buffer persistente y acks en todos los hops según RFC-i17.
- Alcance: delivery service de Core + migraciones, DeliveryManager/AckLedger del Agent, `TradeIntentAck` + `PipeDeliveryAck` en pipes MT4, métricas/telemetría y wiring de config/heartbeat.
- Exclusiones: tooling CLI/manual, histéresis >5 s/80 % (pasa a i18) y pipelines CI/CD nuevos (documentados como riesgo).

## 1. Plan de commits por pasos
- Paso 1 → 1  
  - Paths: `echo/sdk/proto/v1`, `echo/sdk/domain`, `echo/sdk/telemetry`.  
  - Cambios: nuevos contratos (`TradeIntentAck`, `PipeDeliveryAck`), helpers JSON↔proto y métricas de compat/retry.  
  - Rollback: revertir SDK + regenerar pb.
- Paso 2 → 1  
  - Paths: `deploy/postgres/migrations`, `echo/core/internal`.  
  - Cambios: tablas `delivery_journal`/`delivery_retry_event`, repos Postgres, `DeliveryService` + reconciler integrado al router y CommandAck.  
  - Rollback: revertir paquete core + migración.
- Paso 3 → 1  
  - Paths: `echo/agent/internal`.  
  - Cambios: DeliveryManager fail-fast, bloqueo de CommandAck, manejo de `PipeDeliveryAck`, `TradeIntentAck` en `pipe_manager`.  
  - Rollback: revertir módulo y borrar Bolt ledger.
- Paso 4 → 1  
  - Paths: `clients/mt4/master.mq4`, `clients/mt4/slave.mq4`.  
  - Cambios: `IntentQueue` persistente + reintentos/backoff en Master, `PipeDeliveryAck` en Slave con gating handshake/precios.  
  - Rollback: restaurar scripts `.mq4` y borrar `CommonFiles/echo_master_intents_*.jsonl`.

## 2. Cambios de código y artefactos
- Paquetes tocados: `sdk` (proto/domain/métricas), `core/internal/*` (router, delivery_service, repos), `agent/internal/*` (delivery_manager, stream, pipe_manager) y `clients/mt4/{master,slave}.mq4`.
- SDK/Contratos: `sdk/proto/v1/agent.proto` + `sdk/pb/v1` extienden `DeliveryHeartbeat` con `ack_timeout_ms` para propagar el timeout real sin depender del heartbeat.
- Migraciones DB: `deploy/postgres/migrations/i17_delivery_journal.sql` crea `echo.delivery_journal` + `echo.delivery_retry_event` con índices `(status,next_retry_at)` y `(trade_id)`.
- Flags/Config: claves `/echo/core/delivery/*` (ack_timeout, retry_backoff, max_retries, heartbeat) y `/echo/agent/delivery/*` para ledger Bolt. Config se propaga vía `DeliveryHeartbeat`.
- Contratos: `CommandAck` incluye nuevas etapas; `TradeIntentAck` (Agent→Master) y `PipeDeliveryAck` (Slave→Agent) se publican como JSON y son ignorados por clientes legacy (modo compat documentado).
- Core delivery loop: `core/internal/core.go` y `core/internal/config.go` añaden `LoadDeliveryRuntimeConfig` + `startDeliveryHeartbeatLoop`, que refresca las llaves `/echo/core/delivery/*` desde ETCD y reenvía `DeliveryHeartbeat` periódicos con los nuevos valores. `core/internal/delivery_service.go` admite actualización dinámica de `AckTimeout/RetryBackoff`.
- Heartbeats gRPC: `core/internal/config.go` fuerza que `grpc/keepalive/*` adopten el mismo `heartbeat_interval_ms` (1–5 s) leído desde `/echo/core/delivery/heartbeat_interval_ms`, y `agent/internal/config.go` replica ese valor para el cliente gRPC. Esto elimina los defaults legacy 60 s/20 s reportados en H1.
- Entrega lossless en Core: `core/internal/delivery_service.go` encapsula todas las escrituras (`Insert`, `UpdateStatus`, `AssignAgent`, `MarkAcked`) bajo `withJournalRetry` (backoff ≤ `max_retries`) y expone fallos no recuperables al Router. `core/internal/router.go` ahora corta el fanout cuando `ScheduleExecuteOrder`/`ScheduleCloseOrder` agota reintentos, loguea `core_delivery_failed` y emite `echo_core.delivery.retry_total{delivery_stage="journal",result="failed"}` (H2).
- Agent concurrency/metrics: `agent/internal/delivery_manager.go` protege `DeliveryConfig` con RWMutex (sin carreras con el `retryLoop`) e instrumenta `echo_agent.pipe.delivery_latency_ms` alrededor de cada write/ack al EA.
- Master EA: `agent/internal/pipe_manager` mantiene un snapshot (`MasterDeliveryConfig`) y emite mensajes `delivery_config` con la secuencia `/echo/core/delivery/retry_backoff_ms` + `max_retries`; `clients/mt4/master.mq4` sobrescribe `g_IntentRetryBackoffMs` y `g_IntentMaxRetries` en runtime, evitando recompilar cuando ETCD cambia.

## 3. Instrumentación
- Métricas:
  - `echo_core.delivery.retry_total{stage,result,agent_id}` y `pending_age_ms{stage}` (journal/reconciliador).
  - `echo_core.delivery.compat_mode_total` (handshake modo compat) y logs `core.delivery.*`.
  - `echo_agent.pipe.retry_total{agent_id,account_id,delivery_stage,result}` en `DeliveryManager` (cuenta tanto fallas de `WriteMessage` como `PipeDeliveryAck` con RETRY/FAILED).
- `echo_agent.pipe.delivery_latency_ms{agent_id,account_id,result}` en `DeliveryManager` para medir la latencia Agent→EA por intento.
  - `echo_ea.trade_intent.buffer_depth{account_id}` se reporta desde el Agent usando el buffer_depth enviado por el Master; permite alertar backlog del IntentQueue.
  - Master/Slave emiten logs JSON `intent_retry` para troubleshooting del buffer.
- Router publica `core_delivery_failed` + `echo_core.delivery.retry_total{delivery_stage="journal",result="failed"}` cuando `withJournalRetry` agota reintentos al persistir, para que NOC detecte pérdidas por caída de Postgres (H2).
- Spans/Logs: `core/internal/delivery_service` abre `core.delivery.journal` (persistencia) y `core.delivery.retry` (cada envío o replay, con `ack_timeout_ms`/`backoff_ms` de la config vigente). `agent/internal/delivery_manager` emite `agent.pipe.delivery` alrededor de cada `WriteMessage` al pipe; `agent/internal/pipe_manager` marca `ea.intent.buffer` cuando recibe intents, propagando `trade_id`, `command_id`, `attempt` y `buffer_depth`. Master/Slave mantienen logs JSON estructurados conforme a PR-OBS.

## 4. Prueba local
- Pre-requisitos: Postgres (schema `echo`), ETCD con llaves `core/*` y `agent/*`, permisos de escritura en `%PROGRAMDATA%/Echo/agent/acks` y en `CommonFiles` para el IntentQueue MT4.
- Comandos ejecutados:
  - build: `go build ./agent/...`, `go build ./core/...`.
  - unit: `cd agent && go test ./...`; `cd core && go test ./...`.
  - lint: `golangci-lint run` (instalado vía `go install`) **falló** con `goanalysis_metalinter: unsupported version internal/goarch` (la herramienta no soporta Go 1.25 en la devbox). Se documenta y se delega al pipeline oficial.
  - integración: Pipes reales aún pendientes (QA manual).
- Datos: seeds de cuentas existentes, ETCD con `delivery/*`, backoff default y `PROGRAMDATA` limpio antes de pruebas.
- Troubleshooting: revisar tablas `echo.delivery_journal`, `delivery_retry_event`, logs `core.delivery*`, Bolt ledger (`Echo/agent/acks/*.db`) y `CommonFiles/echo_master_intents_*.jsonl`.

## 5. Prueba en CI
- Jobs afectados: build/core, build/agent, unit/core, unit/agent. Se documenta necesidad de habilitar migraciones i17 en pipeline.
- Variables: conexión Postgres + ETCD, ruta de ack ledger (usamos default en Windows runner).
- Artefactos: coverage ≥95 % en rutas tocadas (core/agent) mantenido por suites existentes.

## 6. Casos borde cubiertos
- Master desconectado / Agent caído o primer `PipeWriteLn` fallido → `IntentQueue` siempre agenda `next_retry_ms` y emite `intent_retry`; se reintenta hasta 100 veces antes de descartar.
- Agent pipe write failure → DeliveryManager marca `ACK_STAGE_AGENT_BUFFERED`, incrementa backoff, registra `echo_agent.pipe.retry_total` y sólo falla tras `max_retries`, enviando `CommandAck` con `FAILED`.
- Slave pipe caído / handshake degradado → `PipeDeliveryAck` responde `RETRY` o `FAILED`; Agent no marca `PIPE_DELIVERED` hasta recibir `OK`.
- ExecutionResult faltante → reconciliador mantiene `status=inflight`, reinserta y sólo marca `acked` tras `EA_CONFIRMED`.
- Cambio dinámico de backoff/ack_timeout (`/echo/core/delivery/*`) → el loop `startDeliveryHeartbeatLoop` refresca ETCD y envía `DeliveryHeartbeat` recurrentes; el Agent aplica `ack_timeout_ms`, actualiza ledger/pipe retries y `pipe_manager` reenvía `delivery_config` para que el Master confirme los nuevos valores sin reinicio manual.
- Caída temporal de Postgres durante `delivery_journal.insert`/`update` → `withJournalRetry` insiste con backoff hasta agotar `max_retries`, registrando `echo_core.delivery.retry_total{delivery_stage="journal"}`; si la base sigue caída, el Router detiene el fanout, emite `core_delivery_failed` y requiere intervención manual en lugar de perder órdenes (H2).

## 7. Matriz de contratos
- `pb.AgentHello/CoreHello`: negociación de `protocol_version` y flag `supports_lossless_delivery` mantienen compatibilidad con agentes legacy (Core expone métrica `compat_mode_total`).
- `CommandAck`: `CORE_ACCEPTED` (persistencia), `AGENT_BUFFERED` (pipe pendiente), `PIPE_DELIVERED`, `EA_CONFIRMED`. Resultados `PENDING/OK/FAILED` reflejan cada transición.
- `TradeIntentAck` (Agent→Master) y `PipeDeliveryAck` (Slave→Agent) se representan en JSON (pipes MT4). Clientes antiguos ignoran el mensaje sin crash.

## 8. Política de pruebas — Estado final
- Unit: verde [x] (`go test ./agent/...`, `go test ./core/...`).
- Integración: verde [ ] (pipes reales aún pendientes; QA manual).
- E2E/Smoke/Regresión: a cargo de QA (documentado en RFC/Runbooks).

## 9. Checklist final
- Compila [x]
- Lints [ ] (`golangci-lint run` instalado vía `go install` pero falla con `goanalysis_metalinter` por incompatibilidad con Go 1.25; se documenta en CI y se deja al pipeline).
- Tests unit [x]
- Tests integración [ ]
- Respeta PR-* [x]
- Contratos intactos/BWC [x]

