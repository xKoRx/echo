# Revisión RFC i8(a+b) — sl-tp-offset-stop-level — Iter 5

## Resumen de auditoría
Se auditó la versión más reciente del RFC y la respuesta NI-4/ H6. La revisión comparó contratos (`ExecutionResult`, `ErrorCode`), integración de `StopLevelGuard`, routing existente y configuraciones ETCD contra las bases de Echo (00-contexto, 01-roadmap, RFC-architecture) y los principios PR-*.

## Matriz de conformidad
| Requisito | Evidencia | Estado |
|---|---|---|
| Uso de `ERROR_CODE_TIMEOUT` con semántica uniforme para ajustes agotados | [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#componentes-y-responsabilidades] · [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#scheduler-del-adjustmentqueue] · [echo/docs/rfcs/RFC-architecture.md#8-2-trade-proto] | FALLA |

## Cobertura PR-*
| PR-* | Evidencia | Estado | Comentario |
|---|---|---|---|
| PR-BWC | [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#componentes-y-responsabilidades] · [echo/docs/rfcs/RFC-architecture.md#8-2-trade-proto] | FALLA | El RFC agrega metadata `adjustment_timeout=true` a `ExecutionResult` sin definir cómo se serializa en el proto actual. |

## Hallazgos
### H7 — Metadata `adjustment_timeout` sin contrato proto
- Severidad: **MAY**
- PR-*: PR-BWC
- Evidencia: [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#componentes-y-responsabilidades] · [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#interfaces-publicas-i-o-errores-contratos] · [echo/docs/rfcs/RFC-architecture.md#8-2-trade-proto]
- Impacto: `ExecutionResult` no posee campos para flags arbitrarios; sólo expone `command_id`, `success`, `ticket`, `error_code`, `executed_price` y `timestamps`. Sin especificar dónde vive `adjustment_timeout`, Core/Agent/EA pueden implementar extensiones incompatibles, rompiendo contratos y dificultando telemetría uniforme.
- Propuesta: (a) Extender `ExecutionResult` con un campo opcional (e.g., `map<string, string> metadata` o `bool adjustment_timeout`), documentando migración y versionado; o (b) registrar el estado exclusivamente vía telemetría/logs, describiendo cómo se reflejará en BD sin tocar el proto. Cualquiera debe incluir plan BWC y actualización de SDK.
- Trade-offs: Ampliar el proto exige versionar contratos y coordinar despliegues; mover la señal a telemetría evita migración pero requiere fuentes alternativas para dashboards.

## Citas faltantes / Suposiciones
- Ninguna.

## NEED-INFO
1. **Ubicación/contrato `ExecuteOrderOrchestrator` y `CommandBuilder`** — El RFC requiere módulos en `core/orchestrator/executeorder/...`, pero en el código actual la orquestación reside en `core/internal/router.go`. Confirmar si debemos crear y migrar a los nuevos paquetes (indicando dependencias y firmas mínimas) o si se admitirá integrar `StopLevelGuard` en el router existente mientras se planifica la refactorización futura. (Impacta PR-MOD, PR-BWC).  
   - Propuesta A: Crear los paquetes `core/orchestrator/executeorder` y `commandbuilder`, trasladando la lógica con guía de dependencias.  
   - Propuesta B: Mantener la orquestación actual en `router`, incorporando los nuevos pasos sin reestructurar hasta iteración posterior.

## Cambios sugeridos (diff textual)
```diff
- ... devuelve al Core un `ExecutionResult` con `ERROR_CODE_TIMEOUT` y atributos `adjustment_timeout=true`.
+ ... devuelve al Core un `ExecutionResult` con `ERROR_CODE_TIMEOUT`; si se requiere distinguir el caso, extender el proto con un campo `adjustment_timeout` (o registrar en telemetría) y documentar la migración correspondiente.
```

## Evaluación de riesgos
Mientras la metadata de timeout no se formalice, existe riesgo de divergencias entre Core y Agent al interpretar reintentos agotados, comprometiendo tableros operativos y compatibilidad con EAs. La decisión sobre la reestructuración del orquestador es crítica para planificar la implementación sin romper el router vigente.

## Decisión
- Estado: **Observado** (queda H7 abierto + NEED-INFO).
- Condiciones de cierre: definir el contrato para `adjustment_timeout` y responder la NEED-INFO sobre el orquestador/CommandBuilder.

## Refs cargadas
- `echo/docs/00-contexto-general.md` — `"---"`
- `echo/docs/01-arquitectura-y-roadmap.md` — `"---"`
- `echo/docs/rfcs/RFC-architecture.md` — `"---"`
- `echo/vibe-coding/prompts/common-principles.md` — `"**PR-ROB** Robustez: tolerancia a fallos, timeouts, reintentos, backoff; sin afectar integridad de datos."`

