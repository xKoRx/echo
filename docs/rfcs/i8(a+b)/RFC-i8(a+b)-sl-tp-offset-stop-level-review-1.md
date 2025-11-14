# Revisión RFC i8(a+b) — sl-tp-offset-stop-level — Iter 1

## Resumen de auditoría
Se revisó el RFC contra los lineamientos base de Echo y los principios PR-*. El foco estuvo en validar SLOs, contratos compartidos y planes de resiliencia/observabilidad entre Core, Agent y EA. Se contrastaron las afirmaciones con `docs/00-contexto-general.md`, `docs/01-arquitectura-y-roadmap.md`, `docs/rfcs/RFC-architecture.md` y `vibe-coding/prompts/common-principles.md`.

## Matriz de conformidad
| Requisito | Evidencia | Estado |
|---|---|---|
| Mantener SLO intra-host p95 <100 ms | [echo/docs/rfcs/RFC-architecture.md#1-resumen-ejecutivo] · [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#riesgos-limites-slos-y-capacidad-kpis--umbrales] | FALLA |
| Contrato compartido documentado en proto/SDK para offsets | [echo/docs/rfcs/RFC-architecture.md#81-commonproto] · [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#interfaces-publicas-io-errores-contratos] | FALLA |
| Reintentos/backoff explícitos para `ModifyStops` | [echo/docs/00-contexto-general.md#6-conexiones-inestables-y-backpressure] · [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#componentes-y-responsabilidades] | OBS |
| Observabilidad alineada a `sdk/telemetry` (logs/metrics/spans) | [echo/docs/00-contexto-general.md#principios-obligatorios-de-ingenieria] · [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#observabilidad-logs-estructurados-metricas-spans] | OK |

## Cobertura PR-*
| PR-* | Evidencia | Estado | Comentario |
|---|---|---|---|
| PR-PERF | [echo/docs/rfcs/RFC-architecture.md#1-resumen-ejecutivo] · [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#riesgos-limites-slos-y-capacidad-kpis--umbrales] | FALLA | El RFC eleva el p95 total a 180 ms, rompiendo el objetivo intra-host de 100 ms. |
| PR-BWC | [echo/docs/rfcs/RFC-architecture.md#81-commonproto] · [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#interfaces-publicas-io-errores-contratos] | FALLA | No existe especificación del nuevo mensaje/estructura `AdjustableStops`, generando ruptura de contrato compartido. |
| PR-MOD | [echo/docs/rfcs/RFC-architecture.md#4-3-core] · [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#componentes-y-responsabilidades] | OBS | El rol de `StopLevelGuard` está alineado, pero falta detallar cómo se integra sin duplicar lógica en servicios existentes. |
| PR-ROB | [echo/docs/00-contexto-general.md#6-conexiones-inestables-y-backpressure] · [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#componentes-y-responsabilidades] | OBS | No se especifica la estrategia de reintentos/backoff para comandos `ModifyStops`. |
| PR-OBS | [echo/docs/00-contexto-general.md#principios-obligatorios-de-ingenieria] · [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#observabilidad-logs-estructurados-metricas-spans] | OK | Cumple con logs JSON, métricas y spans en contexto OTEL. |

## Hallazgos
### H1 — SLO degradado
- Severidad: **BLOQ**
- PR-*: PR-PERF
- Evidencia: [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#riesgos-limites-slos-y-capacidad-kpis--umbrales] · [echo/docs/rfcs/RFC-architecture.md#1-resumen-ejecutivo]
- Impacto: Rompe el objetivo intra-host <100 ms definido para Echo V1, comprometiendo consistencia operacional y acuerdos con traders.
- Propuesta: Mantener el target total p95 ≤100 ms, asignando un presupuesto explícito (≤20 ms) para post-modificaciones y optimizando la cola `AdjustmentQueue`.
- Trade-offs: Requiere mayor afinamiento en Core/Agent para no introducir latencia adicional, pero preserva el SLO contractual sin aumentar riesgo de slippage.

### H2 — Contrato compartido sin definición
- Severidad: **BLOQ**
- PR-*: PR-BWC, PR-MOD
- Evidencia: [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#interfaces-publicas-io-errores-contratos] · [echo/docs/rfcs/RFC-architecture.md#81-commonproto]
- Impacto: `AdjustableStops` no existe en `common.proto`/SDK, impidiendo coordinar Core↔Agent↔EA y rompiendo compatibilidad binaria.
- Propuesta: Especificar el mensaje en `common.proto` (campos `sl_offset_points`, `tp_offset_points`, `reason`, etc.) y documentar la versión mínima del SDK.
- Trade-offs: Añadir el mensaje implica migración coordinada, pero mantiene contratos explícitos y evita divergencias entre módulos.

### H3 — Estrategia de reintentos insuficiente
- Severidad: **MAY**
- PR-*: PR-ROB
- Evidencia: [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#componentes-y-responsabilidades] · [echo/docs/00-contexto-general.md#6-conexiones-inestables-y-backpressure]
- Impacto: Sin política definida de retry/backoff para `ModifyStops`, persiste el riesgo de saturar Agent o quedarse sin sincronía ante fallas transitorias del broker.
- Propuesta: Documentar en el RFC el número de reintentos, temporizadores y backoff/jitter para la cola de ajustes, con métricas asociadas.
- Trade-offs: Exponer estos parámetros aumenta la complejidad de configuración, pero aporta resiliencia y diagnósticos más fiables.

## Citas faltantes / Suposiciones
- Ninguna.

## Cambios sugeridos (diff textual)
```diff
- SLO p95 ≤80 ms adicionales por operación (target total p95 <180 ms) con monitoreo continuo.
+ SLO intra-host p95 ≤100 ms en total, reservando ≤20 ms adicionales para ajustes y con monitoreo continuo.
```

```diff
- `sdk/domain/AdjustableStops`: estructura declarativa compartida que encapsula offsets, límites y motivos para auditoría.
+ `sdk/domain/AdjustableStops`: estructura declarativa compartida con campos `sl_offset_points`, `tp_offset_points`, `stop_level_breach` y `reason`.
+ `common.proto` incorpora `message AdjustableStops` y versiones mínimas para Core/Agent/EA que publiquen/consuman este contrato.
```

## Evaluación de riesgos
Persisten riesgos de latencia fuera de SLO, ruptura de compatibilidad entre módulos y falta de lineamientos de retry ante rechazos del broker. Esto puede incrementar missed trades y degradar la sincronía maestro→slave, afectando KPIs de i8(a+b).

## Decisión
- Estado: **Rechazado** hasta cerrar los hallazgos BLOQ/MAY.
- Condiciones de cierre: ajustar SLO al objetivo oficial, definir el contrato `AdjustableStops` en el proto/SDK y describir estrategia de retries para `ModifyStops`.

## Refs cargadas
- `echo/docs/00-contexto-general.md` — `"---"`
- `echo/docs/01-arquitectura-y-roadmap.md` — `"---"`
- `echo/docs/rfcs/RFC-architecture.md` — `"---"`
- `echo/vibe-coding/prompts/common-principles.md` — `"**PR-ROB** Robustez: tolerancia a fallos, timeouts, reintentos, backoff; sin afectar integridad de datos."`

