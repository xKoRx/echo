---
title: "Echo — Arquitectura y Roadmap Evolutivo"
version: "1.1"
date: "2025-11-04"
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
| i4 | Especificaciones de broker & guardián de volumen | Caché/persistencia de specs, clamps previos a `ExecuteOrder`, políticas `FIXED_LOT` base. | 🚧 |
| i5 | Versionado de handshake y feedback | `protocol_version`, `SymbolRegistrationResult` Core→Agent→EA, validaciones tempranas. | ⏳ |
| i6 | Sizing con riesgo fijo (Modo A) | Cálculo con distancia a SL y tick value, uso de políticas `FIXED_RISK`. | ⏳ |
| i7 | Filtros de spread y desvío | Aplicar tolerancias por cuenta×símbolo antes de abrir. | ⏳ |
| i8a | SL/TP con offset | Offsets configurables en apertura, fallback si broker rechaza. | ⏳ |
| i8b | StopLevel-aware + modificación post-fill | Validar StopLevel y enviar `ModifyOrder` tras fill cuando aplique. | ⏳ |
| i9 | Ventanas de no-ejecución | Calendarios que bloquean nuevas aperturas sin afectar cierres. | ⏳ |
| i10 | SL catastrófico | Protección independiente del master, cierre forzado y telemetría. | ⏳ |
| i11 | Espera de mejora (time-boxed) | Buscar mejor precio durante un intervalo breve sin incrementar latencia. | ⏳ |
| i12 | Normalización de `error_code` | Diccionario único para logs, métricas y BD. | ⏳ |
| i13a | Concurrencia por `trade_id` | Worker pool con orden garantizado y baja latencia. | ⏳ |
| i13b | Backpressure y límites de cola | Buffers configurables, métricas de cola, rechazos controlados. | ⏳ |
| i14 | Telemetría avanzada | Dashboards de funneles, histogramas de latencia, métricas de slippage/spread. | ⏳ |
| i15 | Paquetización y operación | CLI/scripts, health checks, runbooks y automatización básica. | ⏳ |
| i16 | Políticas operativas de trading | Límites globales (drawdown diario/total, apalancamiento, sizing máximo). | ⏳ |
| TBD | Event store Mongo | Almacenamiento append-only para auditoría y análisis. | ⏳ |

## Estado actual

- ✅ i0 — Flujo mínimo market-only con lot fijo y telemetría base.
- ✅ i1 — Persistencia, dedupe y keepalive/heartbeats.
- ✅ i3 — Catálogo de símbolos y reportes de estado (ver RFC-004c).
- 🚧 i4 — Guardián de especificaciones y políticas `FIXED_LOT` centralizadas.

## Referencias

- `docs/rfcs/RFC-architecture.md`
- `docs/rfcs/RFC-004-iteracion-4-especificaciones-broker.md`
- `docs/rfcs/RFC-004c-iteracion-3-parte-final-slave-registro.md`
- `docs/rfcs/RFC-003-iteration-1-implementation.md`
