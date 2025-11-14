# Revisión RFC i8(a+b) — sl-tp-offset-stop-level — Iter 3

## Resumen de auditoría
Se validó la versión actualizada del RFC y la respuesta del autor respecto a la revisión iter 2. El análisis cubre SLO intra-host ≤100 ms, política de retries del `AdjustmentQueue`, contrato `AdjustableStops` y consistencia con las bases de Echo (`00-contexto-general`, `01-arquitectura-y-roadmap`, `RFC-architecture`) y principios PR-*.

## Matriz de conformidad
| Requisito | Evidencia | Estado |
|---|---|---|
| SLO intra-host p95 ≤100 ms preservado en post-modificaciones | [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#componentes-y-responsabilidades] · [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#riesgos-limites-slos-y-capacidad-kpis--umbrales] · [echo/docs/rfcs/RFC-architecture.md#1-resumen-ejecutivo] | OK |
| Política de retries/backoff alineada al presupuesto de 20 ms | [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#componentes-y-responsabilidades] · [echo/docs/00-contexto-general.md#6-conexiones-inestables-y-backpressure] | OK |
| Contrato `AdjustableStops` definido y compatible con SDK/EAs | [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#interfaces-publicas-io-errores-contratos] · [echo/docs/rfcs/RFC-architecture.md#8-1-common-proto] | OK |

## Cobertura PR-*
| PR-* | Evidencia | Estado | Comentario |
|---|---|---|---|
| PR-PERF | [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#componentes-y-responsabilidades] · [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#riesgos-limites-slos-y-capacidad-kpis--umbrales] | OK | Límite duro de 20 ms mantiene el SLO de 100 ms descrito en arquitectura. |
| PR-ROB | [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#componentes-y-responsabilidades] · [echo/docs/00-contexto-general.md#6-conexiones-inestables-y-backpressure] | OK | Retries acotados y telemetría asociada al agotamiento del presupuesto. |
| PR-BWC | [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#interfaces-publicas-io-errores-contratos] · [echo/docs/rfcs/RFC-architecture.md#8-1-common-proto] | OK | Se documenta el proto y la versión mínima de handshake. |

## Hallazgos
- Sin hallazgos nuevos; las observaciones de la iteración previa quedaron resueltas.

## Citas faltantes / Suposiciones
- Ninguna.

## NEED-INFO
1. **Fuente y formato de offsets SL/TP** — [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#componentes-y-responsabilidades] no indica de dónde obtiene `RiskPolicyService` los valores (`etcd`, Postgres, otro) ni el formato exacto; se requiere para validar integraciones (PR-MOD, PR-BWC).  
2. **Criterios determinísticos del `StopLevelGuard`** — [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#componentes-y-responsabilidades] y [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#interfaces-publicas-io-errores-contratos] describen las salidas pero no el pseudocódigo o umbrales para emitir `ACCEPT_WITH_OFFSETS`, `ACCEPT_WITH_POST_MODIFY` o `REJECT_WITH_REASON`; necesario para garantizar consistencia con PR-IDEMP y PR-ROB.  
3. **Diseño operativo del `AdjustmentQueue`** — [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#componentes-y-responsabilidades] limita retries a 20 ms pero no define tamaño de cola, número preciso de intentos ni jitter máximo (≤5 ms solicitado) para cumplir PR-PERF y PR-ROB.

## Cambios sugeridos (diff textual)
N/A.

## Evaluación de riesgos
Sin nuevos riesgos detectados tras la corrección de la política de retries y el contrato proto. Las solicitudes de información anterior deben resolverse para asegurar configuraciones coherentes y operación determinística.

## Decisión
- Estado: **Observado** hasta recibir las aclaraciones solicitadas en NEED-INFO.
- Condiciones de cierre: responder los tres puntos de NEED-INFO con detalle operativo.

## Refs cargadas
- `echo/docs/00-contexto-general.md` — `"---"`
- `echo/docs/01-arquitectura-y-roadmap.md` — `"---"`
- `echo/docs/rfcs/RFC-architecture.md` — `"---"`
- `echo/vibe-coding/prompts/common-principles.md` — `"**PR-ROB** Robustez: tolerancia a fallos, timeouts, reintentos, backoff; sin afectar integridad de datos."`

