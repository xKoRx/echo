# Revisión RFC i8(a+b) — sl-tp-offset-stop-level — Iter 6

## Resumen de auditoría
Se verificó la última actualización del RFC y la respuesta a H7/NI-5. El análisis confirma la eliminación de metadata no documentada, el uso de `ERROR_CODE_TIMEOUT`, y la decisión de mantener la orquestación en `core/internal/router.go` hasta una refactorización futura. Se revisaron las secciones modificadas contra las bases de Echo y los principios PR-*.

## Matriz de conformidad
| Requisito | Evidencia | Estado |
|---|---|---|
| Gestión de retries/timeout sin ampliar el proto existente | [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#componentes-y-responsabilidades] · [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#scheduler-del-adjustmentqueue] · [echo/docs/rfcs/RFC-architecture.md#8-2-trade-proto] | OK |
| Claridad sobre ubicación de `StopLevelGuard` vs orquestador actual | [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#componentes-y-responsabilidades] · [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#integracion-con-el-orquestador-existente] | OK |

## Cobertura PR-*
| PR-* | Evidencia | Estado | Comentario |
|---|---|---|---|
| PR-BWC | [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#componentes-y-responsabilidades] · [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#scheduler-del-adjustmentqueue] · [echo/docs/rfcs/RFC-architecture.md#8-2-trade-proto] | OK | Se reutiliza `ERROR_CODE_TIMEOUT` sin añadir campos ad hoc. |
| PR-MOD | [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#componentes-y-responsabilidades] · [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#integracion-con-el-orquestador-existente] | OK | Se documenta la inserción del guardián en `router` y la deuda técnica futura. |

## Hallazgos
- Ninguno; todas las observaciones previas fueron atendidas.

## Citas faltantes / Suposiciones
- Ninguna.

## Necesidad de información adicional
- Ninguna; NI-5 recibida y resuelta.

## Evaluación de riesgos
Con el uso consistente de `ERROR_CODE_TIMEOUT` y la aclaración sobre el router actual, el plan mantiene compatibilidad y permite instrumentar métricas de timeout sin alterar el proto. La refactorización a un nuevo orquestador queda como trabajo futuro documentado.

## Decisión
- Estado: **Aprobado** (sin pendientes).
- Condiciones: ninguna.

## Refs cargadas
- `echo/docs/00-contexto-general.md` — `"---"`
- `echo/docs/01-arquitectura-y-roadmap.md` — `"---"`
- `echo/docs/rfcs/RFC-architecture.md` — `"---"`
- `echo/vibe-coding/prompts/common-principles.md` — `"**PR-ROB** Robustez: tolerancia a fallos, timeouts, reintentos, backoff; sin afectar integridad de datos."`

