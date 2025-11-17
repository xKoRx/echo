---
title: "Echo — Arquitectura y Roadmap Evolutivo"
version: "1.2"
date: "2025-11-07"
status: "Base"
owner: "Equipo Echo"
---

## Objetivo

Establecer el roadmap oficial de Echo V1 y servir como índice operativo de las iteraciones. Toda la arquitectura detallada (componentes, principios, funcionalidades y estados) vive en `docs/rfcs/RFC-architecture.md`.

## Arquitectura (referencia)

La descripción completa del producto, responsabilidades por componente, contratos y estado de cada funcionalidad está documentada en **`docs/rfcs/RFC-architecture.md`**. Este RFC se considera la fuente única de verdad para arquitectura y se mantiene con estados (`✅`, `🚧`, `⏳`, `🕸️`, `❌`).

## Roadmap Evolutivo (post-i1)

| Iteración | Objetivo | Alcance principal | Estado |
|-----------|----------|-------------------|--------|
| i2 | Routing selectivo por ownership | Registrar cuentas en Agent y enrutar `ExecuteOrder`/`CloseOrder` solo al Agent propietario. | ✅ |
| i2b | Estabilidad post-routing | Manejo de desconexiones de EA, timeouts en canales, reducción de latencia. | ✅ |
| i3 | Catálogo canónico y snapshots | `canonical_symbol ⇄ broker_symbol`, reportes 250 ms, persistencia en Postgres. | ✅ |
| i4 | Especificaciones de broker & guardián de volumen | Caché/persistencia de specs, clamps previos a `ExecuteOrder`, políticas `FIXED_LOT` base. | ✅ |
| i5 | Versionado de handshake y feedback | `protocol_version`, `SymbolRegistrationResult` Core→Agent→EA, validaciones tempranas y tooling CLI. | ✅ |
| i6 | Sizing con riesgo fijo (Modo A) | Cálculo con distancia a SL y tick value, uso de políticas `FIXED_RISK`. | ✅ |
| i6b | Hardening multi-activo | Garantizar que master/slave usen precios y quotes por símbolo antes de ejecutar/cerrar órdenes. | ✅ |
| i7 | Filtros de spread y desvío | [Deprecado] Aplicar tolerancias por cuenta×símbolo antes de abrir. | ❌ |
| i8a | SL/TP con offset | Offsets configurables en apertura; fallback reintenta `ExecuteOrder` con offsets 0 (sin `ModifyOrder`, reservado para i8b). | ✅ |
| i8b | StopLevel-aware + modificación post-fill | Validar StopLevel y enviar `ModifyOrder` tras fill cuando aplique. | 🚧 |
| i9 | Ventanas de no-ejecución | [V2] Calendarios que bloquean nuevas operaciones. | ⏳ |
| i10 | SL catastrófico | Protección independiente del master, cierre forzado y telemetría. | ❌ |
| i11 | Espera de mejora (time-boxed) | Buscar mejor precio durante un intervalo breve sin incrementar latencia. | ❌ |
| i12 | Normalización de `error_code` | Diccionario único para logs, métricas y BD. | ❌ |
| i13a | Concurrencia por `trade_id` | [V1] Worker pool con orden garantizado y baja latencia. | ⏳ |
| i13b | Backpressure y límites de cola | [V1] Buffers configurables, métricas de cola, rechazos controlados. | ⏳ |
| i14 | Telemetría avanzada | [V1] Dashboards de funneles, histogramas de latencia, métricas de slippage/spread. | ⏳ |
| i15 | Paquetización y operación | [V1] CLI/scripts, health checks, runbooks y automatización básica. | ❌ |
| i16 | Políticas operativas de trading | [V2] Límites globales (drawdown diario/total, apalancamiento, sizing máximo). | ⏳ |
| i17 | Garantías de replicación determinista | [V1] End-to-end delivery con reintentos, quorum de acks y reconciliación automática de operaciones para evitar pérdidas. | ⏳ |
| TBD | Event store Mongo | Almacenamiento append-only para auditoría y análisis. | ⏳ |
| TBD | SymbolMappings en Master | Master EA consume catálogo canónico y publica símbolos normalizados. | ⏳ |
| TBD | Pipe Start | Los Agentes deben abrir los pipe solo cuando corresponda, o sea, solo cuando el cliente lo solicite, validando la existencia de configuración con core previamente. | ⏳ |

## Estado actual

- ✅ i0 — Flujo mínimo market-only con lot fijo y telemetría base.
- ✅ i1 — Persistencia, dedupe y keepalive/heartbeats.
- ✅ i3 — Catálogo de símbolos y reportes de estado (ver RFC-004c).
- ✅ i4 — Guardián de especificaciones y políticas `FIXED_LOT` centralizadas.
- ✅ i5 — Handshake v2 completo (EAs actualizados, feedback consumido, CLI de re-evaluación operativa).
- ✅ i6 — Motor FixedRisk con cálculo por riesgo monetario, cache de cuentas, métricas y seeds de configuración.
- ✅ i6b — Hardening multi-activo en EAs (quotes y ejecuciones contundentemente por símbolo).
- ✅ i8a — Offsets SL/TP aplicados en Core con métricas `stop_offset_*` y fallback sin offsets ante `INVALID_STOPS`.

## Referencias

- `docs/rfcs/RFC-architecture.md`
- `docs/rfcs/RFC-004-iteracion-4-especificaciones-broker.md`
- `docs/rfcs/RFC-004c-iteracion-3-parte-final-slave-registro.md`
- `docs/rfcs/RFC-003-iteration-1-implementation.md`

## Próxima iteración: i17 — Garantías end-to-end de replicación

- **Puntos críticos identificados**:
  - Desconexión EA↔Agent en el instante de enviar `SymbolRegistrationResult` → Agent memoriza estado `UNSPECIFIED` y bloquea órdenes posteriores.
  - Gaps entre `TradeIntent` y `ExecuteOrder` por reintentos/timeout del stream gRPC.
  - Falta de confirmaciones en el pipe EA ↔ Agent; si el Named Pipe cae tras escribir el JSON, el EA podría no consumirlo.
  - Reconexiones simultáneas Master/Slaves sin rehidratar cache → router en `UNSPECIFIED`.
- **Propuesta**:
  - Implementar **journal de comandos** en Core: cada `ExecuteOrder`/`CloseOrder` se marca `pending` y exige ack del Agent.
  - En Agent, mantener un **ack ledger** y reintentar escritura al Named Pipe hasta confirmación del EA (pong correlacionado o heartbeat extendido).
  - Incorporar **reconciliador de órdenes**: master reporta `TradeIntent` y slaves reportan `ExecutionResult`; un cron verifica faltantes y reinyecta órdenes.
  - Telemetría dedicada (`echo.replicator.*`) + alertas cuando la ventana entre intent y ejecución supere N ms.
  - CLI de auditoría (`echo-core-cli replicator reconcile`) para reemitir operaciones específicas.
