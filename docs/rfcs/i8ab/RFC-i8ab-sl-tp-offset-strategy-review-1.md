# Revisión RFC i8ab — sl-tp-offset-strategy — Iter 1

- **Resumen de auditoría**
  - Alcance: auditoría completa del RFC contra `echo/docs/00-contexto-general.md`, `echo/docs/01-arquitectura-y-roadmap.md`, `echo/docs/rfcs/RFC-architecture.md`, `echo/vibe-coding/prompts/common-principles.md` y la plantilla oficial.
  - Madurez: el documento está al nivel **DEV/QA-READY**; describe persistencia, flujo en router, degradaciones, observabilidad y pruebas sin dejar huecos para el equipo de implementación o QA.

- **Matriz de conformidad por requisito**

| Requisito | Evidencia | Estado | Dev/QA ready |
|-----------|-----------|--------|--------------|
| Offsets configurables por cuenta×estrategia en Postgres y dominio (`RiskPolicy`, repositorio y service`) | [echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md#41-dentro-de-alcance][echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md#61-mensajes--contratos][echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md#62-modelo-de-datos-y-migracion] | OK | SI |
| Router aplica offsets en pips→precio, respeta BUY/SELL y clamps por StopLevel antes de `ExecuteOrder` | [echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md#51-vision-general][echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md#53-flujos-principales][echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md#54-reglas-de-negocio-y-casos-borde] | OK | SI |
| Fallback determinista ante `INVALID_STOPS`, incluyendo reintento con offsets 0 y `ModifyOrder` opcional + KPI <0.3% | [echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md#53-flujos-principales][echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md#3-objetivos-medibles-definition-of-done][echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md#91-casos-e2e] | OK | SI |
| Observabilidad de offsets (metrics, logs JSON y spans correlacionados con `segment`) | [echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md#8-observabilidad-logs-metricas-trazas] | OK | SI |
| Plan de pruebas completo (E2E, unitarios, QA, datos) para offsets, clamps y fallback | [echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md#9-plan-de-pruebas-dev-y-qa] | OK | SI |
| Rollout/BWC/rollback documentados (migración, defaults 0, operación) | [echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md#10-plan-de-rollout-bwc-y-operacion] | OK | SI |

- **Cobertura PR-***

| PR-* | Evidencia | Estado | Comentario |
|------|-----------|--------|------------|
| PR-ROB | [echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md#53-flujos-principales][echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md#54-reglas-de-negocio-y-casos-borde] | OK | Clamps + fallback documentan degradación controlada. |
| PR-MOD | [echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md#41-dentro-de-alcance] | OK | Mantiene `account_strategy_risk_policy` como única fuente. |
| PR-ESC | [echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md#51-vision-general] | OK | Lógica encapsulada en router/servicio sin dependencias nuevas. |
| PR-CLN | [echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md#41-dentro-de-alcance] | OK | Evita configuraciones duplicadas y describe funciones acotadas. |
| PR-SOLID | [echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md#61-mensajes--contratos] | OK | Extiende structs sin romper interfaces públicas. |
| PR-KISS | [echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md#42-fuera-de-alcance] | OK | No introduce flags ni rutas paralelas; offsets se aplican en un único punto. |
| PR-OBS | [echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md#8-observabilidad-logs-metricas-trazas] | OK | Define métricas, logs y spans con atributos comunes. |
| PR-BWC | [echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md#62-modelo-de-datos-y-migracion][echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md#10-plan-de-rollout-bwc-y-operacion] | OK | Defaults 0 y orden de despliegue (migration→Core) mantienen compatibilidad. |
| PR-IDEMP | [echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md#53-flujos-principales] | OK | Reintentos mantienen `trade_id` y dedupe por `command_id`. |
| PR-RES | [echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md#53-flujos-principales] | OK | Fallback `execute→modify` y alertas cubren fallos parciales. |
| PR-SEC | [echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md#62-modelo-de-datos-y-migracion] | OK | No expone nuevos datos sensibles; reutiliza Postgres existente. |
| PR-PERF | [echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md#3-objetivos-medibles-definition-of-done] | OK | Define techo <2 ms adicional y métricas para vigilar p95. |

- **Hallazgos**

No se identificaron hallazgos; todos los requisitos trazados están cubiertos con evidencia suficiente.

- **Gaps de implementabilidad para Dev/QA (GAP-DEV)**

Sin GAP-DEV: el RFC define contratos, migración, flujo del router, degradaciones y criterios de prueba completos. Implementación y QA pueden iniciar sin inventar detalles adicionales.

- **Citas faltantes / Suposiciones**

No se detectaron afirmaciones sin cita o basadas en suposiciones no documentadas.

- **Cambios sugeridos (diff textual conceptual)**

No aplica.

- **Evaluación de riesgos**

Los riesgos R1–R3 del propio RFC (offset negativo extremo, símbolos exóticos, migración omitida) están descritos con impacto y mitigación ([echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md#111-riesgos]). No se identificaron riesgos adicionales durante la auditoría.

- **Decisión**

`decision: APROBADO` — No hay condiciones abiertas; continuar con handoff a Gatekeeper para formalizar DEV/QA-READY.

- **Refs cargadas**
  - echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md — "---"
  - echo/docs/00-contexto-general.md — "---"
  - echo/docs/01-arquitectura-y-roadmap.md — "---"
  - echo/docs/rfcs/RFC-architecture.md — "---"
  - echo/vibe-coding/prompts/common-principles.md — "**PR-ROB** Robustez: tolerancia a fallos, timeouts, reintentos, backoff; sin afectar integridad de datos."
  - echo/docs/templates/rfc.md — "---"
  - echo/deploy/postgres/migrations/i4_risk_policy.sql — "BEGIN;"
  - echo/deploy/postgres/migrations/i6_risk_policy_fixed_risk.sql — "-- Iteración 6: soporte para políticas FIXED_RISK"
