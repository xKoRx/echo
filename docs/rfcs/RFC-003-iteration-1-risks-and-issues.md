---
title: "RFC-003 — Iteración 1: Problemas, Riesgos y Decisiones Críticas"
version: "1.0"
date: "2025-10-28"
status: "Proposed"
authors: ["Aranea Labs - Trading Copier Team"]
depends_on: ["RFC-001", "RFC-002", "RFC-003-iteration-1-implementation"]
---

# Iteración 1 — Problemas, Riesgos y Decisiones Críticas

Rol: Arquitectura senior, postura crítica. Objetivo: detectar bloqueantes tempranos y evitar deuda que comprometa la profesionalización del copiador.

---

## 1. Inconsistencias y brechas actuales

- Versión de contratos: coexistencia de `proto v0` en documentos antiguos y `pb/v1` decidido. Riesgo de drift entre Agent/Core/EAs. Acción: unificar en i1 y añadir validadores de versión en arranque.
- Telemetry duplicada: referencias a `echo/sdk/telemetry` y a `sdk/shared/telemetry`. Acción: un cliente único (shared). Mantener adaptador temporal sólo si es imprescindible.
- Config hardcoded en i0: endpoints, toggles y límites en código. Acción: mover a ETCD en i1 (mínimo viable) con cliente oficial.
- Dedupe volátil: mapa in-memory en i0. Reinicios generan duplicados potenciales. Acción: dedupe persistente con TTL y estados terminales.
- Correlación de cierres por `magic_number`: ambigua en presencia de múltiples tickets (hedged, reaperturas). Acción: cierre por `ticket` exacto usando `executions` persistido.
- `FlushFileBuffers` en DLL IPC: potenciales bloqueos y jitter. Acción: deshabilitar por defecto; exponer flag sólo para tests de benchmark.
- KeepAlive gRPC agresivo: valores por defecto pueden gatillar "too many pings". Acción: parámetros conservadores y prueba de resiliencia documentada.

---

## 2. Riesgos técnicos (priorizados)

1) Duplicado de órdenes post-reinicio
- Causa: pérdida del estado en Core/Agent con dedupe in-memory.
- Impacto: doble ejecución en slaves; inconsistencia contable.
- Mitigación i1: tabla `dedupe` persistente + actualización atómica a `executions`.

2) Cierre incorrecto (ticket equivocado o múltiple)
- Causa: correlación por `magic_number` en lugar de ticket.
- Impacto: cierre de posición equivocada; pérdida.
- Mitigación i1: `trade_id → [tickets]` por slave en `executions`; `CloseOrder.ticket` obligatorio.

3) Deriva de versiones de proto
- Causa: mezcla `v0/v1` entre componentes y documentación.
- Impacto: incompatibilidad sutil y fallas de parsing.
- Mitigación i1: única versión `pb/v1`, validación de versión en handshake/arranque.

4) Saturación de métricas (alta cardinalidad)
- Causa: uso de IDs dinámicos en labels.
- Impacto: costos y pobre rendimiento en Prometheus.
- Mitigación i1: lista blanca de atributos; `trade_id` sólo en logs/traces, no en labels.

5) Bloqueos por IPC y latencia espuria
- Causa: `FlushFileBuffers` forzado; lecturas byte-a-byte.
- Impacto: picos de latencia; deadlocks en escenarios límite.
- Mitigación i1: `FlushFileBuffers=false` por defecto; buffering line-delimited en SDK.

6) KeepAlive mal configurado
- Causa: pings muy frecuentes en cliente.
- Impacto: desconexiones; caída de stream.
- Mitigación i1: `time>=60s`, `timeout=20s`, `MinTime>=10s` + pruebas prolongadas.

7) Timestamps heterogéneos (GetTickCount vs Unix ms)
- Causa: fuentes diferentes por plataforma.
- Impacto: métricas E2E con offsets falsos.
- Mitigación i1: tratar latencias como relativas; documentar reinicios; normalizar en i2 si es necesario.

8) Falta de MM en i1 (lote fijo)
- Causa: alcance i1 deliberado.
- Impacto: riesgo operacional en cuentas heterogéneas.
- Mitigación: advertir explícito en docs; priorizar MM en i4 según roadmap.

---

## 3. Decisiones técnicas recomendadas (i1)

- Unificar proto en `echo.v1` y `sdk/pb/v1`. Añadir chequeo de versión mínima en arranque del Agent/Core.
- Implementar migraciones SQL versionadas y validadas en arranque (fail-fast).
- ETCD como única fuente de configuración permitida (excepto `hostname`, `HOST_KEY`, `ENV`).
- Telemetry: único cliente `sdk/shared/telemetry`; `EchoMetrics` como bundle de dominio sobre ese cliente.
- Correlación de cierres por `ticket` obligatorio; fallback por `comment` con `trade_id` si falta.

---

## 4. Pruebas obligatorias de resiliencia (i1)

- Reinicio del Core en medio de 10 ejecuciones: 0 duplicados; dedupe persistente funciona.
- Caída y reconexión del Agent: reestablecer stream < 10s; siguientes órdenes OK.
- KeepAlive prolongado: 90 minutos con tráfico; sin desconexiones; sin "too many pings".
- Stress IPC: ráfaga de 50 intents; sin deadlocks; p95 < 150 ms.

---

## 5. Problemas abiertos que requieren definición en i2

- Concurrencia por `trade_id` y particionamiento por cuenta/símbolo.
- Inclusión de `slippage` y `spread` del slave en `ExecutionResult` para métricas operativas.
- Estrategia de almacenamiento de timestamps (JSONB vs columnas) para consultas en Prometheus/SQL.
- Mapeo de símbolos (canonical ↔ broker) y validaciones de StopLevel.

---

## 6. Checklist de salida i1 (bloqueante)

- [ ] Migraciones aplicadas y validadas en CI/CD.
- [ ] `pb/v1` en todos los binarios y repos; `v0` removido.
- [ ] ETCD operativo con claves mínimas y watches activos.
- [ ] `FlushFileBuffers=false` por defecto en IPC.
- [ ] Métricas de latencia y resultado visibles en Grafana.
- [ ] Pruebas de resiliencia ejecutadas con evidencia (logs/JSON/capturas).

---

Fin — Problemas y Riesgos i1


