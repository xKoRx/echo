---
title: "Echo — Arquitectura y Roadmap Evolutivo"
version: "1.0"
date: "2025-10-29"
status: "Base"
owner: "Equipo Echo"
---

## Objetivo

Definir la arquitectura oficial de Echo, responsabilidades claras por componente, contratos de integración y un roadmap evolutivo de micro‑iteraciones hacia V1. Las iteraciones 0 y 1 ya están implementadas y se mantienen tal cual.

## Decisiones fundacionales

- Independencia de módulos: sin imports cruzados entre módulos del monorepo.
- DIP/SOLID: programación contra interfaces; composición sobre herencia.
- Configuración centralizada en ETCD con carga única y watches; sin ENV/YAML en runtime (salvo excepciones aprobadas: hostname, HOST_KEY, ENV).
- Observabilidad estándar con un cliente único y bundles de métricas por dominio; atributos en contexto para logs, métricas y spans.
- Identidad e idempotencia con `trade_id` (UUIDv7 estable y ordenable por tiempo) y correlación por tickets en cada slave.
- Orientación SDK‑first: toda lógica reutilizable vive en la SDK (proto, validaciones, transformaciones, gRPC helpers, IPC, telemetry, utils).

## Componentes y responsabilidades

### Master EA (MQL4/MQL5)

- Responsabilidades
  - Detectar fills, modificaciones y cierres locales del master.
  - Emitir `TradeIntent` con metadatos (trade_id, symbol, side, lot del master, price, magic_number, ticket) y reportar cierres.
  - Logs estructurados para diagnóstico.
- No responsabilidades
  - No calcula sizing del slave.
  - No evalúa ventanas ni políticas globales.

### Agent (Go, Windows Service por host)

- Responsabilidades
  - Bridge entre EAs (Named Pipes) y Core (gRPC bidi).
  - Registro de cuentas y ownership por host en handshake; el Core usa esto para routing selectivo (i2+).
  - Routing local por `target_account_id`: envía `ExecuteOrder`/`CloseOrder` al pipe del Slave correcto.
  - Añadir timestamps de hop: `t1` al recibir desde Master EA y `t4` al recibir desde Core.
  - Reportar estado coalescido hacia el Core (~250 ms): equity, balance, margen, posiciones/órdenes, ticks y especificaciones de símbolos por cuenta.
  - Keepalive gRPC y heartbeats de aplicación; reconexión con backoff exponencial en caso de desconexión.
  - Telemetría propia: logs, métricas y trazas del Agent y de los EAs conectados.
- No responsabilidades
  - No decide políticas de negocio (ventanas, tolerancias, Money Management).
  - No transforma reglas de sizing (eso es del Core).
  - No aplica filtros de spread/desvío (eso es del Core en V1).
  - No persiste estado de negocio (solo colas/buffers internos volátiles).

### Core (Go)

- Responsabilidades
  - Orquestación y enrutamiento determinista Master→Slaves (broadcast en i0/i1; selectivo desde i2).
  - Validaciones de forma (símbolos, campos obligatorios) y deduplicación por `trade_id`; correlación por ticket de cada slave.
  - Money Management central: cálculo de `lot_size` para cada slave. En i0/i1 es lot fijo hardcoded (0.10); desde i5 soporta riesgo fijo (modo A) con distancia a SL y tick value, con clamps por `min_lot`, `max_lot` y `lot_step`.
  - Evaluación de políticas: tolerancias (i6), ventanas de no-ejecución (i8), SL catastrófico (i9).
  - Persistencia: PostgreSQL para órdenes, ejecuciones, correlación `trade_id ↔ ticket(s)` y dedupe (desde i1). MongoDB para eventos crudos en append-only (planificado i2+, opcional).
  - Estado réplica por cuenta: posiciones, órdenes, equity, margen, último tick relevante (construido desde reportes del Agent; i2+).
  - Telemetría integral del funnel: latencias por hop y E2E, resultados (éxito/rechazo), causas de rechazo, slippage (cuando aplique).
- No responsabilidades
  - No implementa IPC con EAs (esto es del Agent).
  - No ejecuta órdenes en brokers (esto es del Slave EA vía Agent).

### Slave EA (MQL4/MQL5)

- Responsabilidades
  - Ejecutar `ExecuteOrder`/`CloseOrder` proveniente del Agent (market‑only en V1).
  - Reportar `ExecutionResult` con timestamps completos (también en cierres; el `command_id` identifica la operación).
  - Replicar MagicNumber del master y, cuando aplique, incluir `trade_id` en el comentario de la orden.
- No responsabilidades
  - No decide políticas ni sizing.

### SDK Echo (Go)

- Responsabilidades
  - Contratos (tipos/mensajes), validaciones de forma y transformaciones Proto↔Dominio↔JSON.
  - Abstracciones de gRPC (cliente/servidor/stream) y de IPC (Named Pipes) reutilizables.
  - Telemetry unificada (cliente, bundles de métricas de dominio, semconv, context helpers).
  - Utilidades (UUID, timestamps, JSON, etc.).

### Infraestructura de soporte

- ETCD v3: configuración central y declarativa con watches (desde i1).
- PostgreSQL 16: estado vivo, correlación `trade_id ↔ ticket(s)`, dedupe y auditoría operacional (desde i1).
- MongoDB 7: eventos crudos (append‑only) para análisis/auditoría futura (planificado i2+, opcional).
- Observabilidad: OTLP (trazas/métricas/logs) hacia Prometheus, Loki y Jaeger; dashboards en Grafana (desde i0).

## Contratos funcionales (mensajes mínimos v1)

- TradeIntent (Master EA → Core vía Agent)
  - Requeridos: `trade_id` (UUIDv7), `timestamp_ms`, `client_id`, `symbol`, `side`, `lot_size` (del master), `price`, `magic_number`, `ticket` (del master), `timestamps` (t0).
  - Opcionales: `stop_loss`, `take_profit`, `comment`, `attempt` (número de reintento, desde i1).

- ExecuteOrder (Core → Slave EA vía Agent)
  - Requeridos: `command_id` (UUIDv7), `trade_id`, `timestamp_ms`, `target_client_id`, `target_account_id`, `symbol`, `side`, `lot_size` (calculado para el slave), `magic_number`, `timestamps` (propaga t0..t3).
  - Opcionales: `stop_loss`, `take_profit`, `comment`.

- ExecutionResult (Slave EA → Core vía Agent)
  - Requeridos: `command_id`, `trade_id`, `success` (bool), `ticket` (si `success=true`, ticket real del broker), `error_code`, `timestamps` completos (t0..t7).
  - Opcionales: `error_message`, `executed_price`, `execution_time_ms`.

- TradeClose (Master EA → Core vía Agent)
  - Requeridos: `trade_id`, `timestamp_ms`, `client_id`, `account_id` (del master), `ticket` (del master), `symbol`, `magic_number`, `close_price`.

- CloseOrder (Core → Slave EA vía Agent)
  - Requeridos: `command_id` (UUIDv7), `trade_id`, `timestamp_ms`, `ticket` (del slave; 0 en i0 si no se conoce, exacto desde i1), `target_client_id`, `target_account_id`, `symbol`, `magic_number`, `timestamps`.
  - Opcionales: `lot_size` (para cierres parciales cuando aplique).

## Flujo de datos (alto nivel con timestamps)

1. Master EA genera `TradeIntent` con `trade_id` (UUIDv7) y `t0` (fill detectado).
2. Agent recibe desde pipe, añade `t1`, convierte a Proto y envía al Core.
3. Core recibe, añade `t2`, valida y deduplica por `trade_id`, calcula `lot_size` por slave y genera `ExecuteOrder`(s). Marca `t3` y envía a Agents.
4. Agent recibe `ExecuteOrder`, añade `t4` y hace routing local por `target_account_id` al pipe del Slave correcto.
5. Slave EA recibe, marca `t5`/`t6`/`t7` (recv/before-send/after-send) y envía `ExecutionResult` con todos los timestamps.
6. Core persiste ejecución, actualiza correlación `trade_id ↔ ticket` y registra latencias por hop y E2E.
7. Cierre: Master EA emite `TradeClose`; Core consulta correlación y envía `CloseOrder` por ticket exacto; Slave EA cierra y envía `ExecutionResult` correspondiente al cierre.

## Calidad, seguridad y SLO

- Latencia objetivo: p95 intra‑host < 100 ms (E2E), sin duplicados.
- Observabilidad obligatoria: logs estructurados, métricas y trazas desde i0. Errores siempre registrados con contexto. Códigos de error normalizados desde i11.
- Seguridad V1: sin mTLS ni KMS en primera fase; diseño listo para activarse en iteraciones posteriores.

## Estado actual

- Iteración 0 (POC): flujo E2E mínimo con lot fijo, market‑only, sin persistencia; telemetría básica.
- Iteración 1: persistencia mínima (correlación y dedupe), keepalive gRPC y heartbeats, normalización de resultados y estabilidad del stream; configuración mínima en ETCD.

## Roadmap evolutivo (micro‑iteraciones post‑i1)

Reglas del roadmap
- Iteraciones pequeñas, con alcance acotado y criterios de salida claros.
- Tocar un tema 1–3 veces como máximo (p. ej., SL/TP en dos pasos); evitar cambios repetidos en cada iteración.
- Cero regresiones: cada paso debe poder desplegarse y operar por sí mismo.

Iteración 2 — Routing selectivo por ownership de cuentas
- Objetivo: eliminar broadcast; enrutar `ExecuteOrder`/`CloseOrder` solo al Agent propietario de cada cuenta slave.
- Alcance: registro de ownership en el handshake del Agent; tabla/registro en Core; envío dirigido.
- Exclusiones: políticas, sizing y SL/TP.
- Criterios de salida: 100% de órdenes solo al Agent correcto; métricas de routing activas.

Iteración 2b — Correcciones de estabilidad y latencia post-i2
- Objetivo: corregir problemas críticos detectados en i2: desconexión de EA no detectada y latencia aumentada.
- Alcance:
  - **Problema 1 - Desconexión no detectada**: EOF ahora termina el loop de lectura en PipeHandler (antes hacía `continue`), permitiendo ejecutar `notifyAccountDisconnected` al Core. Timeout sigue siendo tratado como condición normal de "sin datos".
  - **Problema 2 - Latencia aumentada**: Añadido timeout de 500ms en todos los envíos a canales de Agents (`sendToAgent`, `broadcastOrder`, `broadcastCloseOrder`) para evitar bloqueos indefinidos si canal está lleno. Logging movido a nivel Debug en hot path y consolidado post-envío para reducir overhead. Con 15 slaves y 3 Agents, reducción de latencia estimada de ~2s a <500ms.
- Cambios técnicos:
  - `agent/internal/pipe_manager.go`: EOF ejecuta `break` en lugar de `continue` (líneas 287-294).
  - `core/internal/router.go`: Timeout 500ms con `time.NewTimer()` en `select` de envíos (sendToAgent, broadcastOrder, broadcastCloseOrder). Logging reducido a Debug durante envío exitoso, Info consolidado post-broadcast.
- Criterios de salida:
  - Desconexiones de EAs detectadas y notificadas al Core en <1 segundo.
  - Latencia E2E restaurada a niveles de i0/i1 (~250-500ms) bajo condiciones normales.
  - Timeouts en canales registrados como Warning sin bloquear procesamiento de otras órdenes.
  - Métricas `timeout_count` activas en broadcasts para monitoreo de saturación de Agents.

Iteración 3 — Catálogo canónico de símbolos y mapeo por cuenta
- Objetivo: estandarizar `canonical_symbol ⇄ broker_symbol` por cuenta.
- Alcance: catálogo canónico en Core; agent/EA reportan mapeo del broker al conectar; validaciones previas al envío.
- Exclusiones: sizing y políticas.
- Criterios de salida: 0 errores por símbolo desconocido; mapeo persistido y trazable.

Iteración 4b — Especificaciones de broker (min_lot, lot_step, stop_level)
- Objetivo: almacenar y validar specs por símbolo/cuenta.
- Alcance: reporte de specs desde los slaves; validación previa a `ExecuteOrder` (apertura); clamps de volumen por lot_step.
- Exclusiones: SL/TP y modificación post‑fill.
- Criterios de salida: 0 rechazos por volumen inválido en apertura.

Iteración 5 — Sizing con riesgo fijo (modo A)
- Objetivo: pasar de lot fijo a riesgo fijo cuando exista SL efectivo o recomendado.
- Alcance: cálculo con distancia a SL y tick value; fallback seguro a lot fijo si faltan datos; guardas min/max lot.
- Exclusiones: offset de SL/TP y StopLevel.
- Criterios de salida: sizing correcto en pruebas de mesa; métricas de sizing y clamps activas.

Iteración 6 — Filtros de spread y desvío (market entry)
- Objetivo: aplicar tolerancias de spread/desvío en apertura.
- Alcance: políticas por cuenta×símbolo; ejecución si dentro de límites; si no, rechazo registrado (omit with comment).
- Exclusiones: espera de mejora.
- Criterios de salida: métricas de rechazos por motivo; latencia estable.

Iteración 7a — SL/TP con offset (sin StopLevel)
- Objetivo: habilitar SL/TP opcionales con offset configurable en apertura.
- Alcance: si el master trae SL/TP, aplicar offset (+/− pips) y enviar en `ExecuteOrder`. Si el broker rechaza, registrar y continuar sin SL/TP.
- Exclusiones: validación de StopLevel, modificación post‑fill, trailing.
- Criterios de salida: métricas de inserciones vs rechazos por broker; 0 bloqueos en flujo normal.

Iteración 7b — StopLevel‑aware y modificación post‑fill
- Objetivo: validar StopLevel antes de enviar; si no cumple, insertar sin SL/TP y modificar inmediatamente tras el fill.
- Alcance: consultar StopLevel reportado por el Slave (i4); si distancia < StopLevel, abrir sin SL/TP y enviar `ModifyOrder` tras `ExecutionResult` exitoso.
- Exclusiones: trailing, reglas dinámicas.
- Criterios de salida: % de inserciones en apertura vs post‑fill; 0 rechazos por StopLevel en flujo normal.

Iteración 8 — Ventanas de no‑ejecución (entradas)
- Objetivo: bloquear nuevas aperturas en ventanas definidas, sin bloquear cierres.
- Alcance: calendario por cuenta/símbolo con buffers pre/post; cancelación de pendientes heredadas donde aplique; sin re‑inserción automática.
- Exclusiones: órdenes pendientes (fuera de V1).
- Criterios de salida: métrica de bloqueos por ventana y ausencia de aperturas en intervalos bloqueados.

Iteración 9 — SL catastrófico por cuenta/estrategia
- Objetivo: protección de contingencia independiente del master.
- Alcance: política por cuenta/estrategia; cierre forzado cuando se alcance; registro y observabilidad.
- Exclusiones: trailing o reglas dinámicas.
- Criterios de salida: activación y logging confiables en escenarios controlados.

Iteración 10 — Espera de mejora opcional (time‑boxed)
- Objetivo: reducir slippage cuando el precio va a favor en compra/venta.
- Alcance: esperar mejora hasta objetivo o timeout; límites estrictos para no aumentar latencia E2E.
- Exclusiones: price‑chase.
- Criterios de salida: reducción neta de slippage en entornos con spread/latencia típicos.

Iteración 11 — Normalización de códigos de error y resultados
- Objetivo: mapa único de `error_code` para logs, métricas y persistencia.
- Alcance: diccionario central, validaciones en SDK; persistencia y dashboards alineados.
- Exclusiones: cambios funcionales de ejecución.
- Criterios de salida: 100% de `error_code` normalizados en registros.

Iteración 12a — Serialización por `trade_id` con worker pool
- Objetivo: mantener orden de procesamiento por `trade_id` sin bloquear otros trades.
- Alcance: worker pool con N goroutines; mensajes con mismo `trade_id` se procesan secuencialmente, distintos en paralelo. Canales hash por `trade_id`.
- Exclusiones: límites de cola.
- Criterios de salida: sin reordenamientos ni duplicados; p95 estable bajo carga.

Iteración 12b — Backpressure y límites de cola
- Objetivo: evitar saturación y OOM bajo ráfagas extremas.
- Alcance: canales con buffer configurable; métricas de cola (profundidad, tiempo de espera); rechazo/alerta si buffer lleno.
- Exclusiones: paralelismo agresivo global.
- Criterios de salida: métricas de backpressure activas; sistema estable bajo ráfagas.

Iteración 13 — Telemetría avanzada (slippage, spread, funneles)
- Objetivo: completar bundle de métricas de dominio y trazas con atributos clave.
- Alcance: histogramas por hop; contadores de funnel; atributos comunes/evento/métrica por contexto.
- Exclusiones: nuevas reglas de negocio.
- Criterios de salida: dashboards operativos y SLO observables.

Iteración 14 — Paquetización y operación (CLI/scripts)
- Objetivo: facilitar despliegue y operación local.
- Alcance: CLI y scripts para arrancar Core/Agent; health y comandos básicos; guías de operación.
- Exclusiones: orquestadores externos.
- Criterios de salida: puesta en marcha reproducible en un host limpio.

Nota: Si alguna iteración depende de datos o contratos ausentes en la SDK, primero se incorpora en la SDK y luego se consume en Core/Agent. No se duplican lógicas entre aplicaciones.

## Criterios de diseño por iteración

- Cada iteración debe:
  - Definir un objetivo único y medible.
  - Limitar cambios a un área (p. ej., SL/TP en no más de 2–3 iteraciones).
  - Mantener compatibilidad operativa con lo ya desplegado.
  - Incluir telemetría mínima para validar impacto.

## Resumen ejecutivo

La arquitectura de Echo separa claramente responsabilidades entre EAs, Agent, Core y SDK, con contratos mínimos y observabilidad unificada. El roadmap posterior a i1 avanza en pasos pequeños y seguros hasta completar V1: routing selectivo, catálogo y specs, sizing por riesgo fijo, filtros y ventanas, SL/TP con offset y StopLevel‑aware, y robustecimiento operacional con métricas y concurrencia segura.

# **IMPORTANTE**: Tests
NO SE DEBEN CONSIDERAR TESTS DENTRO DE LAS ITERACIONES SINO HASTA RECIÉN TERMINAR Y PROBAR V1
AL FINALIZAR V1 SE CREARÁ UNA ETAPA DE CREACIÓN DE TESTS Y REFACTOR COMPLETO, PERO SOLO DESPUÉS DE TENER V1 FUNCIONANDO Y EN PRODUCCIÓN
