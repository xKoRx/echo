# Revisión RFC i8(a+b) — sl-tp-offset-stop-level — Iter 4

## Resumen de auditoría
Se evaluó la versión actualizada del RFC y la respuesta del autor respecto a los hallazgos de la iteración 3. Se verificaron los nuevos apartados de configuración ETCD, lógica determinística del `StopLevelGuard` y scheduler del `AdjustmentQueue`, contrastándolos con las bases de Echo y los principios PR-*.

## Matriz de conformidad
| Requisito | Evidencia | Estado |
|---|---|---|
| Offsets SL/TP con origen y formato en ETCD documentados | [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#configuracion-de-offsets-sl-tp] · [echo/docs/rfcs/RFC-architecture.md#2-principios-arquitectonicos] | OK |
| Criterios determinísticos para `StopLevelGuard` (ACCEPT/POST/REJECT) | [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#logica-deterministica-de-stoplevelguard] · [echo/docs/00-contexto-general.md#problemas-tipicos-del-dominio-y-patrones-de-solucion] | OK |
| Contratos de error/resultados documentados para retries del Agent | [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#componentes-y-responsabilidades] · [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#scheduler-del-adjustmentqueue] · [echo/docs/rfcs/RFC-architecture.md#8-2-trade-proto] | FALLA |

## Cobertura PR-*
| PR-* | Evidencia | Estado | Comentario |
|---|---|---|---|
| PR-ROB | [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#scheduler-del-adjustmentqueue] · [echo/docs/00-contexto-general.md#6-conexiones-inestables-y-backpressure] | OK | Retries y métricas alineados con presupuesto de 20 ms. |
| PR-PERF | [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#componentes-y-responsabilidades] · [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#riesgos-limites-slos-y-capacidad-kpis--umbrales] | OK | Mantiene SLO intra-host p95 ≤100 ms. |
| PR-BWC | [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#componentes-y-responsabilidades] · [echo/docs/rfcs/RFC-architecture.md#8-2-trade-proto] | OBS | Se introduce el resultado `AdjustmentTimedOut` sin definir contrato proto/código de error. |

## Hallazgos
### H6 — Resultado `AdjustmentTimedOut` sin contrato
- Severidad: **MAY**
- PR-*: PR-BWC, PR-MOD
- Evidencia: [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#componentes-y-responsabilidades] · [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#scheduler-del-adjustmentqueue] · [echo/docs/rfcs/RFC-architecture.md#8-2-trade-proto]
- Impacto: El RFC indica que el Agent reportará `AdjustmentTimedOut`, pero en los contratos vigentes (`ExecutionResult`, `ErrorCode`) no existe tal enumeración. Sin especificación queda ambiguo si es un nuevo `ErrorCode`, un campo adicional o un evento de telemetría, generando riesgo de incompatibilidad e implementaciones divergentes.
- Propuesta: Definir explícitamente el mecanismo: (a) agregar `ERROR_CODE_ADJUSTMENT_TIMEOUT` al proto con migración coordinada, o (b) reutilizar `ERROR_CODE_TIMEOUT` documentando atributos adicionales. Incluir la actualización correspondiente en SDK/EA.
- Trade-offs: Añadir un nuevo código implica migración pero clarifica el flujo; reutilizar `ERROR_CODE_TIMEOUT` evita cambios en proto pero requiere acordar metadata uniforme.

## Citas faltantes / Suposiciones
- Ninguna.

## NEED-INFO
1. **Ubicación y contrato de `ExecuteOrderOrchestrator` / `CommandBuilder`** — [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#componentes-y-responsabilidades] exige embutir el `StopLevelGuard` antes del `CommandBuilder`, pero la arquitectura actual no documenta dichos componentes. Indicar paquete/archivo o especificar si deben crearse, incluyendo firma y responsabilidades, para acoplar el guardián sin romper la orquestación existente (router, risk engine). (Impacta PR-MOD, PR-BWC).

## Cambios sugeridos (diff textual)
```diff
- ... antes de confirmar éxito o reportar `AdjustmentTimedOut` al Core.
+ ... antes de confirmar éxito o reportar `ExecutionResult` con `ERROR_CODE_TIMEOUT` (o nuevo código acordado) al Core.
```

## Evaluación de riesgos
Persisten riesgos de incompatibilidad si `AdjustmentTimedOut` no se define en los contratos públicos. Además, la ausencia de guía sobre `ExecuteOrderOrchestrator`/`CommandBuilder` dificulta integrar el guardián sin duplicar lógica.

## Decisión
- Estado: **Observado** (queda un hallazgo MAY y una solicitud de información).
- Condiciones de cierre: definir el contrato para `AdjustmentTimedOut` y responder la NEED-INFO indicada.

## Refs cargadas
- `echo/docs/00-contexto-general.md` — `"---"`
- `echo/docs/01-arquitectura-y-roadmap.md` — `"---"`
- `echo/docs/rfcs/RFC-architecture.md` — `"---"`
- `echo/vibe-coding/prompts/common-principles.md` — `"**PR-ROB** Robustez: tolerancia a fallos, timeouts, reintentos, backoff; sin afectar integridad de datos."`

