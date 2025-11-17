---
rfc_id: "RFC-i17"
title: "Iteración i17 — End-to-end lossless retry"
version: "1.0"
status: "draft"
owner_arch: "Arquitectura Echo"
owner_dev: "Echo Core"
owner_qa: "Echo QA"
date: "2025-11-17"
iteration: "i17"
type: "infra"
depends_on:
  - "docs/00-contexto-general.md"
  - "docs/01-arquitectura-y-roadmap.md"
  - "docs/rfcs/RFC-architecture.md"
  - "docs/rfcs/13a/DT-13a-parallel-slave-processing.md"
related_rfcs:
  - "docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md"
tags:
  - "core"
  - "agent"
  - "sdk"
  - "ea"
---

# RFC-i17: Iteración i17 — End-to-end lossless retry

> **Mandatos i17**
> - Iteración 100 % no funcional: solo se endurece la entrega; no se cambian políticas ni capacidades de trading.
> - Prioridad absoluta a consistencia: se acepta degradar latencia y throughput si eso evita pérdida de mensajes.
> - Sin feature flags, rollout gradual, CLI nuevos ni pipelines adicionales; todo se entrega en modo MVP directo con observabilidad completa.
> - Retries obligatorios en todos los hops (master→agent→core→agent→slave) con hasta 100 intentos y backoff.
> - Histéresis y tooling CLI se mueve a i18 (V2); i17 elimina cualquier requisito de ventana >5 s/80 %.
> - Heartbeats gRPC se fijan en 1–5 s y ETCD queda restringido a parámetros globales; toda configuración negocio (p.ej. offsets, políticas) permanece en PostgreSQL.
> [echo/docs/01-arquitectura-y-roadmap.md#proxima-iteracion-i17--garantias-end-to-end-de-replicacion][echo/docs/01-arquitectura-y-roadmap.md#roadmap-evolutivo-post-i1][echo/docs/rfcs/RFC-architecture.md#72-comunicacion][echo/docs/rfcs/RFC-architecture.md#9-configuracion-etcd]

---

## 1. Resumen ejecutivo

- El router paralelo de i13a puede perder órdenes cuando `sendToAgent` excede `worker_timeout_ms` o cuando el pipe Agent↔EA cae, porque no existe requeue ni ack formal, dejando trades sin replicar en el Core ni en los slaves.[echo/docs/rfcs/13a/DT-13a-parallel-slave-processing.md#2-falta-de-retries-tras-worker_timeout_ms]
- i17 introduce un delivery ledger end-to-end: journal en Core, ack ledger en Agent, buffer en Master EA y watchdog en Slave EA. Cada hop mantiene estado `pending→inflight→acked/failed`, reintenta hasta 100 veces con backoff y dispara reconciliación automática antes de admitir pérdida.[echo/docs/01-arquitectura-y-roadmap.md#proxima-iteracion-i17--garantias-end-to-end-de-replicacion][echo/docs/rfcs/RFC-architecture.md#51-flujo-de-datos]
- Beneficio: cero omisiones de órdenes, correlación trazable y capacidad de operar sin histéresis mientras medimos backlog y envejecimiento, alineado a PR-ROB, PR-IDEMP y PR-MVP.[echo/vibe-coding/prompts/common-principles.md#pr-rob-robustez-tolerancia-a-fallos-timeouts-reintentos-backoff-sin-afectar-integridad-de-datos][echo/vibe-coding/prompts/common-principles.md#pr-idemp-idempotencia-reintentos-seguros-en-operaciones-con-side-effects][echo/vibe-coding/prompts/common-principles.md#pr-mvp-modo-mvp-esto-quiere-decir-que-por-sobre-todas-las-cosas-se-deben-obviar-temas-de-seguridad-slo-rollout-controlados-feature-flags-etc-prima-por-sobre-todas-las-cosas-la-velocidad]

---

## 2. Contexto y motivación

### 2.1 Estado actual

- Master EA envía `TradeIntent` vía Named Pipe y asume éxito inmediato; no retiene el comando y, si el Agent está desconectado, la orden se pierde sin backoff ni persistencia.[echo/docs/rfcs/RFC-architecture.md#51-flujo-de-datos]
- Core registra intents y crea `ExecuteOrder`, pero `sendToAgent` solo realiza un intento por worker y descarta al expirar `worker_timeout_ms`, generando huecos silenciosos.[echo/docs/rfcs/13a/DT-13a-parallel-slave-processing.md#2-falta-de-retries-tras-worker_timeout_ms]
- Agent reenvía comandos al Named Pipe sin confirmar recepción real; el EA Slave únicamente responde con `ExecutionResult` tras ejecutar, por lo que un pipe caído antes de la ejecución no deja rastro operacional.[echo/docs/00-contexto-general.md#problemas-tipicos-del-dominio-y-patrones-de-solucion]

### 2.2 Problema / gaps

- No existe ledger persistente que permita reintentar ni reconciliar; Core y Agent dependen de colas en memoria que se pierden ante timeout o reinicio.[echo/docs/rfcs/13a/DT-13a-parallel-slave-processing.md#2-falta-de-retries-tras-worker_timeout_ms]
- La ausencia de ack por hop impide saber si una orden quedó en Agent, en Named Pipe o en EA, dificultando QA y operación.[echo/docs/01-arquitectura-y-roadmap.md#proxima-iteracion-i17--garantias-end-to-end-de-replicacion]
- Configuración de delivery reparte toggles entre ETCD y Postgres sin regla clara, generando riesgo de modificar políticas en tiempo de ejecución.[echo/docs/rfcs/RFC-architecture.md#9-configuracion-etcd]

### 2.3 Objetivo de la iteración

- Garantizar replicación determinista end-to-end mediante retry + reconciliación sin introducir histéresis ni cambios funcionales, tal como exige el roadmap i17.[echo/docs/01-arquitectura-y-roadmap.md#roadmap-evolutivo-post-i1]
- Formalizar separación de configuraciones (ETCD = infraestructura global, Postgres = negocio) y documentar heartbeats seguros para mantener acks activos.[echo/docs/rfcs/RFC-architecture.md#72-comunicacion][echo/docs/rfcs/RFC-architecture.md#9-configuracion-etcd]

---

## 3. Objetivos medibles (Definition of Done)

1. **Ledger persistente**: toda orden `ExecuteOrder`/`CloseOrder` permanece en `delivery_journal` hasta recibir `CommandAck` de tipo `EA_CONFIRMED`. Cero filas quedan con `status=pending` más de `ack_timeout_ms` sin que el reconciliador intente reenvío.[echo/docs/01-arquitectura-y-roadmap.md#proxima-iteracion-i17--garantias-end-to-end-de-replicacion]
2. **Retries obligatorios**: cada hop (master buffer, Core→Agent, Agent→EA) ejecuta hasta 100 intentos con backoff exponencial antes de etiquetar `failed`, registrando log JSON, span y métrica `echo_core.delivery.retry_total`/`echo_agent.delivery.retry_total`.[echo/docs/01-arquitectura-y-roadmap.md#proxima-iteracion-i17--garantias-end-to-end-de-replicacion]
3. **Observabilidad**: nuevas métricas (`echo_core.delivery.pending_age_ms`, `echo_agent.pipe.delivery_latency_ms`, `echo_ea.trade_intent.buffer_depth`) tienen dashboards y alertas cuando `pending_age_ms > ack_timeout_ms`.[echo/docs/rfcs/RFC-architecture.md#11-observabilidad-y-calidad]
4. **Compatibilidad**: agentes y EAs legacy siguen operando mientras actualizamos en sitio; `protocol_version` negocia la presencia de `CommandAck` sin detener versiones anteriores.[echo/docs/rfcs/RFC-architecture.md#43-core]
5. **Testing**: suites unitarias para `DeliveryService`, `AgentAckLedger` y colas de EA verifican retries/backoff; QA ejecuta E2E manuales sin necesidad de pipelines CI nuevos, en línea con PR-MVP.[echo/vibe-coding/prompts/common-principles.md#pr-mvp-modo-mvp-esto-quiere-decir-que-por-sobre-todas-las-cosas-se-deben-obviar-temas-de-seguridad-slo-rollout-controlados-feature-flags-etc-prima-por-sobre-todas-las-cosas-la-velocidad]

---

## 4. Alcance

### 4.1 Dentro de alcance

- Journal y reconciliador en Core (Go) + migraciones Postgres (`delivery_journal`, `delivery_retry_event`).
- Ack ledger y rewriter en Agent (Go) + buffers persistentes en Named Pipes Master/Slave.
- Nuevos mensajes proto (`CommandAck`, `DeliveryHeartbeat`), ampliación de `AgentMessage`/`CoreMessage` envelopes y eventos JSON en pipes EA.
- Configuración ETCD bajo `/echo/core/delivery/*` para `ack_timeout_ms`, `retry_backoff_ms`, `heartbeat_interval_ms` (solo valores globales técnicos).
- Observabilidad completa (logs JSON, métricas OTEL, spans) siguiendo bundles existentes.
- Ajuste de documentación en `docs/rfcs/RFC-architecture.md` y `docs/01-arquitectura-y-roadmap.md` para reflejar reglas de configuración e histéresis V2.[echo/docs/01-arquitectura-y-roadmap.md#proxima-iteracion-i17--garantias-end-to-end-de-replicacion][echo/docs/rfcs/RFC-architecture.md#9-configuracion-etcd]

### 4.2 Fuera de alcance (y futuro)

- Histéresis y thresholds dinámicos → i18 V2.[echo/docs/01-arquitectura-y-roadmap.md#roadmap-evolutivo-post-i1]
- Nuevos modos de Money Management o tolerancias de mercado (no cambia la lógica de trading).[echo/docs/rfcs/RFC-architecture.md#52-logica-v1]
- Herramientas CLI de reconciliación manual (diferidas a i18).[echo/docs/01-arquitectura-y-roadmap.md#proxima-iteracion-i17--garantias-end-to-end-de-replicacion]
- Cambios en pipelines CI/CD o en la estrategia de rollout (> sigue siendo big bang MVP).[echo/vibe-coding/prompts/common-principles.md#pr-mvp-modo-mvp-esto-quiere-decir-que-por-sobre-todas-las-cosas-se-deben-obviar-temas-de-seguridad-slo-rollout-controlados-feature-flags-etc-prima-por-sobre-todas-las-cosas-la-velocidad]

---

## 5. Arquitectura de solución

### 5.1 Visión general

1. **Master EA buffer**: al detectar `TradeIntent`, se inserta en `IntentQueue` (memoria + archivo circular). El Agent envía `TradeIntentAck` (Pipe) para liberar la entrada; mientras no llegue, el EA reenvía cada `master_retry_backoff_ms` hasta 100 intentos, generando log `intent_retry`.
2. **Agent ↔ Core journal**: Core persiste `delivery_journal` antes de publicar `ExecuteOrder`. Agent confirma recepción (`CommandAck{stage=CORE_ACCEPTED}`); si Core no recibe ack antes de `ack_timeout_ms`, se reintenta gRPC send.
3. **Agent pipe delivery**: tras persistir en su `ack_ledger`, el Agent escribe el JSON del comando y espera `PipeDeliveryAck` del EA. Si el pipe falla, repite envío según backoff y actualiza métricas.
4. **Slave execution**: EA toma el comando, responde `PipeDeliveryAck` inmediato y luego `ExecutionResult`. Core marca `stage=EA_CONFIRMED` al recibir ambos.
5. **Reconciliador**: proceso cada `reconciler_interval_ms` busca entradas `pending` o `failed`, evalúa `next_retry_at` y reinyecta mensajes respetando orden por `trade_id`.
6. **Observabilidad y ops**: spans `core.delivery.retry`, `agent.pipe.delivery`, `ea.intent.buffer` conectan contadores y ayudan a QA.

### 5.2 Componentes afectados (touchpoints)

| Componente | Tipo de cambio | BWC | Notas breves |
|------------|----------------|-----|--------------|
| `core/internal/router`, `core/internal/delivery` | Nuevo `DeliveryService`, integración con worker pool y reconciliador | Sí | Se envuelven envíos existente; no se altera lógica de riesgo/trading. |
| `core/persist/migrations` | Nuevas tablas/índices | Sí | Migraciones forward-only para journal. |
| `agent/internal/stream`, `agent/internal/pipes` | Ledger y espera activa de acks | Sí | Controla reintentos sin bloquear watchers. |
| `sdk/proto/v1/agent.proto` & `trade.proto` | Nuevos mensajes y enums | Sí | `protocol_version` fuerza fallback noop. |
| `clients/master/slave` (EAs) | Buffers y nuevos mensajes JSON | Sí | Cambios additivos; se ignoran si no están presentes. |
| Docs (`docs/rfcs/RFC-architecture.md`, `docs/01-arquitectura-y-roadmap.md`) | Actualización de reglas | N/A | Fuente única para futuras iteraciones. |

### 5.3 Flujos principales

- **F1 – Apertura con éxito**: Master encola intent → Agent ack → Core journal → Agent ack stage CORE_ACCEPTED → Agent escribe pipe → Slave ack pipe → ExecutionResult → Core marca EA_CONFIRMED → reconciliador ignora (estado final).
- **F2 – Core timeout**: Core no recibe ack → `DeliveryService` incrementa `retry_attempt`, reenvía gRPC, mide latencia y, tras 100 intentos, marca `failed` + log + métrica; QA revisa en dashboard.
- **F3 – Pipe caído**: Agent detecta error de escritura → reabre pipe, reenvía comando siguiendo backoff; `PipeDeliveryAck` restablece ledger.
- **F4 – CloseOrder pendiente**: Reconciliador detecta `CloseOrder` sin `EA_CONFIRMED`, reinyecta y bloquea nuevos close para el mismo `trade_id` hasta que ack llegue, evitando inconsistencias.[echo/docs/01-arquitectura-y-roadmap.md#proxima-iteracion-i17--garantias-end-to-end-de-replicacion]

### 5.4 Reglas de negocio y casos borde

- Ningún mensaje se descarta: al agotar 100 intentos se mantiene en `failed` y se levanta alerta; el operador decide manualmente.
- `omit with comment` (p. ej. reglas de política) se mantiene, pero ahora registra log `omit_event`, span `core.delivery.omit` y métrica `echo_core.delivery.omit_total` para trazabilidad.[echo/docs/01-arquitectura-y-roadmap.md#proxima-iteracion-i17--garantias-end-to-end-de-replicacion]
- BWC: si Agent o EA legacy no soportan nuevos acks, el Core detecta `protocol_version<3` y opera en modo compatibilidad (sin journal) únicamente en entornos de prueba; en producción i17 se exige versión nueva antes del despliegue (documentado en plan de rollout).
- Heartbeats gRPC nunca bajan de 1 s para evitar derribar clientes.[echo/docs/rfcs/RFC-architecture.md#72-comunicacion]

---

## 6. Contratos, datos y persistencia

### 6.1 Mensajes / contratos

1. **Proto `agent.proto`**
   - `message AgentHello` (nuevo) extenderá el stream bidi como primer payload enviado por el Agent:
     - `uint32 protocol_version = 1;`
     - `bool supports_lossless_delivery = 2;`
     - `string agent_id = 3;`
   - El Core responde con `CoreHello` (nuevo) que incluye `uint32 required_protocol_version = 1; bool lossless_required = 2;`.
   - `CommandAck` mantiene `command_id`, `stage`, `result`, `attempt`, `observed_at`, `error_code`.
   - `AckStage` define: `ACK_STAGE_UNSPECIFIED`, `CORE_ACCEPTED`, `AGENT_BUFFERED`, `PIPE_DELIVERED`, `EA_CONFIRMED`.
   - `AckResult` define: `ACK_RESULT_PENDING`, `ACK_RESULT_OK`, `ACK_RESULT_FAILED`.
   - Flujo: si `supports_lossless_delivery=false`, el Core marca al Agent en modo compatibilidad, limita los spans a `CORE_ACCEPTED` y emite la métrica `compat_mode_total`. Cuando el Agent actualiza y envía `true`, el Core reactiva journal completo.[echo/docs/rfcs/RFC-architecture.md#83-agentproto]

2. **Proto `trade.proto` / Pipes**
   - `TradeIntentAck` (Master↔Agent) retorna `trade_id`, `command_id`, `attempt`, `result` en JSON.
   - `PipeDeliveryAck` (EA→Agent) reporta `command_id`, `target_account_id`, `result` al encolar antes de ejecutar.

3. **Backoff config exposure**
   - `DeliveryHeartbeat` (Core→Agent) añade `heartbeat_interval_ms` para sincronizar la cadencia sin editar watchdog existentes.

### 6.2 Modelo de datos y esquema

1. `delivery_journal`
   - `command_id UUID PK` — corrige `pb.ExecuteOrder.CommandId`.
   - `trade_id UUID NOT NULL`.
   - `stage SMALLINT NOT NULL` (`0=pending` ... `4=ea_confirmed`).
   - `status TEXT NOT NULL` (`pending|inflight|acked|failed`).
   - `attempt INT NOT NULL DEFAULT 0` (0–100).
   - `last_error TEXT NULL`.
   - `next_retry_at TIMESTAMPTZ NOT NULL`.
   - `created_at`, `updated_at`.
   - Indexes: `(status, next_retry_at)`, `(trade_id)` para reconciliador y consultas de QA.

2. `delivery_retry_event`
   - Historial append-only para auditoría; columnas `command_id`, `attempt`, `stage`, `result`, `error`, `duration_ms`, `created_at`.

3. `agent_ack_ledger`
   - **Store único:** BoltDB embebido por Agent en `%PROGRAMDATA%/Echo/agent/acks/<agent_id>.db`, con un bucket `acks` y claves `command_id` (UUIDv7). No se admite SQLite para evitar bifurcaciones, manteniendo la política “configuración y estado local fuera de Postgres” documentada para componentes Windows.[echo/docs/rfcs/RFC-architecture.md#73-persistencia]
   - **Payload:** cada valor es JSON compacto `{"stage":int,"attempt":int,"last_error":string,"next_retry_at":int64,"updated_at":int64}`. Los campos `next_retry_at`/`updated_at` usan epoch ms para facilitar auditorías cruzadas con OTEL.
   - **Sincronización memoria↔disco:** todas las transiciones de estado (`pending→inflight`, `inflight→acked`, `acked→failed`) aplican `batch` BoltDB y `fsync` inmediato antes de liberar el canal gRPC; la cache en memoria se actualiza con el mismo objeto para lecturas de hot path.
   - **Retención y limpieza:** un job del Agent elimina entradas `ACK_RESULT_OK` con `updated_at > 24h` y colapsa archivos cuando superan 256 MB; comandos `failed` permanecen hasta intervención manual documentada en el runbook.

4. **Master EA IntentQueue**
   - Archivo circular (por cuenta) con campos `trade_id`, `payload`, `attempt`, `next_retry_at`.
   - Máximo 1 000 intents en buffer; al exceder, se bloquea la generación de nuevos intents y se registra `intent_buffer_full`.

5. **No cambios** en tablas de negocio (políticas, offsets) según mandato no funcional.[echo/docs/01-arquitectura-y-roadmap.md#proxima-iteracion-i17--garantias-end-to-end-de-replicacion]

### 6.3 Configuración, flags y parámetros

- `/echo/core/delivery/ack_timeout_ms` (int, default 150) — tiempo máximo entre etapa y ack antes de reintentar.[echo/docs/rfcs/RFC-architecture.md#9-configuracion-etcd]
- `/echo/core/delivery/retry_backoff_ms` (JSON array) — secuencia global usada por Core, Agent y Master/Slave EAs. Default: `[50,100,200,400,800,1600,3200,6400,12800,25600]` (ms). El algoritmo toma los primeros 10 intentos de la lista y, a partir del 11, reutiliza el último valor (`25600`) hasta completar 100 retries. Validaciones: longitud mínima 5, todos los valores >0 y ordenados ascendentemente. El valor se serializa como string JSON (ej. `"[50,100,...]"`) para garantizar lectura homogénea desde el cliente ETCD.[echo/docs/templates/rfc.md#63-configuración-flags-y-parámetros]
- `master_retry_backoff_ms` — los Master EA consumen la misma secuencia anterior vía configuración enviada por el Agent durante el handshake (`AgentHello` propaga el array). No existe clave separada para evitar divergencias; cualquier cambio en `/echo/core/delivery/retry_backoff_ms` se replica automáticamente a los EAs a través de `DeliveryHeartbeat`. Documentar en runbook que los Master deben reiniciarse tras modificar la secuencia para tomar el nuevo valor.
- `/echo/core/delivery/max_retries` (int, default 100) — límite duro ordenado; no puede sobre-escribirse por cuenta.
- `/echo/core/delivery/reconciler_interval_ms` (int, default 500) — frecuencia del job.
- `/echo/core/delivery/heartbeat_interval_ms` (int, default 1000) — alinea con límites 1–5 s.
- Todo parámetro sigue la regla “infra global en ETCD, negocio en Postgres”. Configuraciones específicas por cuenta (offsets, ventanas) continúan en `account_strategy_risk_policy` u otras tablas existentes.[echo/docs/rfcs/RFC-architecture.md#9-configuracion-etcd]

---

## 7. Principios de diseño y trade-offs

- **PR-ROB / PR-RES**: ledger + retries eliminan fallos silenciosos al precio de mayor latencia cuando existen reenvíos.[echo/vibe-coding/prompts/common-principles.md#pr-rob-robustez-tolerancia-a-fallos-timeouts-reintentos-backoff-sin-afectar-integridad-de-datos]
- **PR-IDEMP**: estados `pending/inflight/acked` garantizan que reintentos no duplican ejecuciones, apoyados en UUIDv7 y dedupe existente.[echo/vibe-coding/prompts/common-principles.md#pr-idemp-idempotencia-reintentos-seguros-en-operaciones-con-side-effects]
- **PR-MVP**: se elimina histéresis y toda complejidad opcional para acelerar entrega; latencia puede aumentar pero es aceptado.[echo/vibe-coding/prompts/common-principles.md#pr-mvp-modo-mvp-esto-quiere-decir-que-por-sobre-todas-las-cosas-se-deben-obviar-temas-de-seguridad-slo-rollout-controlados-feature-flags-etc-prima-por-sobre-todas-las-cosas-la-velocidad]
- **Trade-off**: sin CLI ni rollout gradual, cualquier bug implica revertir binario completo; se considera aceptable dado que el cambio es no funcional y contamos con journaling para investigar rápido.[echo/docs/01-arquitectura-y-roadmap.md#proxima-iteracion-i17--garantias-end-to-end-de-replicacion]

---

## 8. Observabilidad (logs, métricas, trazas)

### 8.1 Métricas

| Métrica | Tipo | Labels | Semántica |
|---------|------|--------|-----------|
| `echo_core.delivery.retry_total` | Counter | `stage`, `result`, `agent_id` | Reintentos realizados por Core. |
| `echo_core.delivery.pending_age_ms` | Histogram | `stage`, `agent_id` | Edad de comandos pendientes. |
| `echo_agent.pipe.retry_total` | Counter | `account_id`, `agent_id`, `stage` | Retries al escribir pipe. |
| `echo_agent.delivery.heartbeat_interval_ms` | Gauge | `agent_id` | Intervalo real de heartbeat. |
| `echo_ea.trade_intent.buffer_depth` | Gauge | `account_id` | Intents encolados en Master EA. |
| `echo_ea.pipe.delivery_latency_ms` | Histogram | `account_id`, `symbol` | Latencia entre recibir comando y enviar ack. |
| `echo_core.delivery.compat_mode_total` | Counter | `agent_id`, `protocol_version` | Conexiones activas en modo compatibilidad (agentes que aún no soportan `supports_lossless_delivery`). |

### 8.2 Logs estructurados

- Campos obligatorios: `delivery_stage`, `command_id`, `attempt`, `result`, `next_retry_at`, además de `app`, `feature=replay`, `event=retry` heredados del contexto común.[echo/docs/rfcs/RFC-architecture.md#11-observabilidad-y-calidad]
- Eventos clave:
  1. `intent_buffer_retry` (master) cuando se reenvía `TradeIntent`.
  2. `core_delivery_failed` cuando se agotan 100 intentos.
  3. `agent_pipe_retry` ante error Named Pipe.
  4. `omit_event` cuando se decide no reenviar por política documentada.

### 8.3 Trazas y spans

- Spans nuevos: `core.delivery.journal`, `core.delivery.retry`, `agent.pipe.delivery`, `ea.intent.buffer`.
- Atributos: `command_id`, `trade_id`, `stage`, `attempt`, `ack_timeout_ms`, `backoff_ms`.
- `CommandAck` enlaza spans mediante atributos `link.command_id` para reconstruir hops en Jaeger.

---

## 9. Plan de pruebas (Dev y QA)

### 9.1 Casos de uso E2E

| ID | Descripción | Precondiciones | Resultado |
|----|-------------|----------------|-----------|
| E2E-01 | Master pierde conexión al enviar intent | Agent desconectado 2 s | EA reintenta hasta reconexión y registra ack completo. |
| E2E-02 | Agent timeout al enviar Core→Agent | `worker_timeout_ms` forzado a 1 ms | Core reintenta 100 veces y alerta tras agotar límite. |
| E2E-03 | Pipe Slave cae durante envío | Named Pipe cerrado antes de ack | Agent reabre pipe y reenvía hasta recibir `PipeDeliveryAck`. |
| E2E-04 | CloseOrder pendiente | `ExecutionResult` omitido | Reconciliador detecta y reinyecta sin duplicar. |

**Criterios Given-When-Then (QA):**

| ID | Given | When | Then |
|----|-------|------|------|
| GWT-01 (E2E-01) | `TradeIntent` con `trade_id=T1`, buffer master con 0 elementos, Agent desconectado y `retry_backoff_ms` default | El Master envía `TradeIntent` y transcurren 2 s sin ack | `intent_buffer_depth` llega a 1, log `intent_buffer_retry` muestra `attempt=3`, `agent_ack_ledger` registra `stage=ACK_STAGE_CORE_ACCEPTED` tras la reconexión y métrica `echo_core.delivery.retry_total{stage="CORE_ACCEPTED"}` incrementa en ≥1. |
| GWT-02 (E2E-02) | Core con `worker_timeout_ms=1ms`, `max_retries=100`, Agent disponible | El Core envía `ExecuteOrder` y el Agent simula backpressure bloqueando `SendCh` >1 ms | `delivery_journal` entry mantiene `status=inflight` hasta intento 100, span `core.delivery.retry` se crea por cada reintento, log `core_delivery_failed` aparece al agotar 100 intentos y `pending_age_ms` excede `ack_timeout_ms` generando alerta WARN. |
| GWT-03 (E2E-03) | Agent con pipe `slave-A` abierto, `ack_timeout_ms=150ms` | El Agent escribe comando y el pipe se cierra antes de enviar `PipeDeliveryAck` | `agent_pipe_retry_total{account_id="slave-A"}` incrementa, BoltDB almacena `stage=ACK_STAGE_PIPE_DELIVERED` después del reenvío exitoso, y `PipeDeliveryAck` llega en <5 s. |
| GWT-04 (E2E-04) | `CloseOrder` pendiente en `delivery_journal` (`stage=CORE_ACCEPTED`), reconciliador corriendo cada 500 ms | Se omite a propósito `ExecutionResult` del slave | Reconciliador registra evento en `delivery_retry_event`, reinyecta close y, tras recibir `EA_CONFIRMED`, la entrada pasa a `status=acked`; métrica `echo_core.delivery.retry_total{stage="EA_CONFIRMED"}` incrementa. |

Estos criterios sirven como aceptación objetiva para QA y se deben documentar en los runbooks de pruebas.[echo/docs/templates/rfc.md#9-plan-de-pruebas-dev-y-qa]

### 9.2 Pruebas del Dev

- Unit tests table-driven para `DeliveryService` (journal transitions), `AckStage` parser y `Reconciler` (tiempos, backoff).
- Tests de integración Core↔Agent usando fakes gRPC para validar 100 retries y métricas registradas.
- Tests del Agent para `PipeWriter` y ledger persistente, con pipes simulados que fallan intermitentemente.

### 9.3 Pruebas de QA

- Escenarios manuales y smoke en entorno local: 3 masters, 6 slaves, desconectando componentes para verificar cero pérdidas.
- No se agregan pipelines CI; QA ejecuta regresión manual enfocada en entrega y reporta usando dashboards `echo.delivery.*`.[echo/docs/01-arquitectura-y-roadmap.md#proxima-iteracion-i17--garantias-end-to-end-de-replicacion]

### 9.4 Datos de prueba

- Seeds con `trade_id` determinísticos, `account_id` configurados y colas iniciales vacías.
- Scripts para forzar fallos (simular desconexión gRPC, cerrar Named Pipe) y validar registros en `delivery_journal`.

---

## 10. Plan de rollout, BWC y operación

### 10.1 Estrategia de despliegue

1. Actualizar SDK proto → Agent → Core → EAs (Master y Slave) en una misma ventana.
2. No hay feature flags ni rollout gradual; se despliega binario completo y se valida con smoke tests locales.[echo/vibe-coding/prompts/common-principles.md#pr-mvp-modo-mvp-esto-quiere-decir-que-por-sobre-todas-las-cosas-se-deben-obviar-temas-de-seguridad-slo-rollout-controlados-feature-flags-etc-prima-por-sobre-todas-las-cosas-la-velocidad]
3. Configurar claves ETCD globales antes del despliegue; reiniciar servicios para tomar valores (config carga única).[echo/docs/rfcs/RFC-architecture.md#9-configuracion-etcd]

### 10.2 Backward compatibility

- **Negociación de protocolo:** el handshake gRPC expone `protocol_version` y un flag `supports_lossless_delivery`. Core publica `required_version=3`; si el Agent reporta `<3`, el Core habilita **modo compatibilidad** (journal activo solo en Core, ack stages truncados a `CORE_ACCEPTED`) y emite alerta `compat_mode_active`. Esto permite convivir con agentes viejos durante la ventana de despliegue.[echo/vibe-coding/prompts/common-principles.md#pr-bwc-compatibilidad-hacia-atrás-cambios-no-rompen-contratos-públicos-sin-plan]
- **Detección y migración:** métrica `echo_core.delivery.compat_mode_total{agent_id}` expone hosts pendientes. Operaciones deben actualizarlos dentro de 24 h; pasado ese plazo el Core empieza a rechazar conexiones legacy para evitar inconsistencias.
- **EA legacy:** si el Slave EA no envía `PipeDeliveryAck`, el Agent mantiene ledger hasta `ExecutionResult`. Se documenta en runbook que el modo compatibilidad no garantiza cero pérdida, por lo que debe utilizarse exclusivamente durante la transición.
- `CommandAck`/`DeliveryHeartbeat` siguen siendo campos opcionales: versiones antiguas los ignoran sin panic, y el Core limita spans/métricas a las etapas soportadas hasta que detecta upgrade completo.

### 10.3 Rollback y mitigación

- Única opción: revertir a binario previo (Core, Agent, EAs) y truncar tablas `delivery_journal`/`delivery_retry_event` tras tomar snapshot.
- Sin toggle de emergencia; la mitigación primaria es monitorear `pending_age_ms` y usar reintentos manuales.

### 10.4 Operación y soporte

- Dashboards `Delivery Overview`: backlog, retries, age, ack latency.
- Alertas:
  - `pending_age_ms > ack_timeout_ms * 3` (WARN).
  - `delivery_failed_total > 0` en 5 min (CRIT).
  - `intent_buffer_depth > 800` (WARN) o >950 (CRIT).
- Runbook: verificar salud de ETCD, gRPC, Named Pipes; usar reconciliador manual (`corectl delivery requeue <command_id>`) si se habilita en i18 (no en esta iteración).

---

## 11. Riesgos, supuestos y preguntas abiertas

### 11.1 Riesgos

| ID | Riesgo | Impacto | Mitigación |
|----|--------|---------|------------|
| R1 | Exceso de retries aumenta latencia | MAY | Observabilidad y límites de 100 intentos; priorizamos consistencia según PR-MVP. |
| R2 | Ledger ocupa almacenamiento | MEN | Tabla sólo guarda comandos vivos; job limpia `acked` >24h. |
| R3 | Agentes legacy sin actualizar causan pérdidas | BLOQ | Política: no desplegar en hosts sin versión nueva; doc en rollout. |

### 11.2 Supuestos

- Todos los hosts cuentan con ETCD accesible y Postgres disponible para migraciones.[echo/docs/rfcs/RFC-architecture.md#73-persistencia]
- Operaciones acepta trade-off latencia vs consistencia durante V1.[echo/docs/01-arquitectura-y-roadmap.md#proxima-iteracion-i17--garantias-end-to-end-de-replicacion]

### 11.3 Preguntas abiertas / NEED-INFO

- Sin preguntas pendientes tras el handshake del owner.

---

## 12. Trabajo futuro (iteraciones siguientes)

- i18: histéresis >5 s/80 %, afinamiento de backpressure y CLI/manual tooling.[echo/docs/01-arquitectura-y-roadmap.md#roadmap-evolutivo-post-i1]
- i19+: telemetría avanzada (dashboards funneles) y event store Mongo usando los datos del journal como fuente primaria.[echo/docs/rfcs/RFC-architecture.md#11-observabilidad-y-calidad]

---

## 13. Referencias

1. `echo/docs/00-contexto-general.md`
2. `echo/docs/01-arquitectura-y-roadmap.md`
3. `echo/docs/rfcs/RFC-architecture.md`
4. `echo/docs/rfcs/13a/DT-13a-parallel-slave-processing.md`
5. `echo/vibe-coding/prompts/common-principles.md`
