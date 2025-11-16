# Revisión RFC i8ab — sl-tp-offset-strategy — Iter 3

- **Resumen de auditoría**
  - Se validó `echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md` (sha256 `b4923d0a…`) contra los documentos base, concentrándonos en el fallback i8a y la observabilidad agregada.[echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md#5-arquitectura-de-solucion]
  - El documento ahora detalla el degradado completo ante `INVALID_STOPS` y define explícitamente el origen del label `segment` para todas las métricas, quedando listo para handoff Dev/QA.[echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md#81-metricas]

- **Matriz de conformidad por requisito**

| Requisito | Evidencia | Estado | Dev/QA Ready |
|-----------|-----------|--------|--------------|
| Fallback determinista con métricas/spans y casos E2E | §5.3 describe reintento con offsets 0, `ModifyOrder` opcional, spans `core.stop_offset.fallback`, métricas `stop_offset_fallback_total` y caso E2E-06.[echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md#53-flujos-principales][echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md#91-casos-e2e] | OK | SI |
| Métricas con baja cardinalidad y semántica definida | §8.1 fija `segment=global|tier_1|tier_2|tier_3`, proveniente de `config.risk_tier` (Postgres), con fallback `global` y propagación a atributos comunes.[echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md#81-metricas] | OK | SI |

- **Cobertura PR-***

| PR-* | Evidencia | Estado | Comentario |
|------|-----------|--------|------------|
| PR-ROB | Fallback i8a evita rechazos permanentes y garantiza KPI ≥95 % éxito.[echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md#53-flujos-principales] | OK | Define límites y alertas. |
| PR-RES | Reintentos controlados + clamps mantienen servicio aun con brokers estrictos.[echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md#54-reglas-de-negocio-y-casos-borde] | OK | Degradación determinista. |
| PR-OBS | Métricas/logs/spans comparten `segment` bien definido y sin cardinalidad explosiva.[echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md#81-metricas] | OK | Telemetría implementable. |
| PR-MOD | Cambios siguen confinados a RiskPolicyService + router + SDK (sin nuevos módulos).[echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md#52-componentes-afectados] | OK | Responsabilidades claras. |

- **Hallazgos**
  - Ninguno. El hallazgo previo (semántica de `segment`) quedó resuelto.

- **Gaps de implementabilidad para Dev/QA (GAP-DEV)**
  - No se identifican GAP-DEV. Dev y QA pueden implementar/pruebar usando solo el RFC + docs base.

- **Citas faltantes / Suposiciones**
  - Sin citas faltantes.

- **Cambios sugeridos (diff textual conceptual)**
  - No se requieren cambios adicionales.

- **Evaluación de riesgos**
  - Riesgos previstos (offsets extremos, migración, clamps) ya cuentan con mitigaciones documentadas; no aparecen riesgos nuevos.

- **Decisión**
  - `decision: APROBADO`
  - `condiciones de cierre:` Ninguna pendiente; listo para handoff Dev/QA.

- **Refs cargadas**
  - echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md — "---"
  - echo/docs/00-contexto-general.md — "---"
  - echo/docs/01-arquitectura-y-roadmap.md — "---"
  - echo/docs/rfcs/RFC-architecture.md — "---"
  - echo/vibe-coding/prompts/common-principles.md — "**PR-ROB** Robustez: tolerancia a fallos, timeouts, reintentos, backoff; sin afectar integridad de datos."
  - echo/docs/templates/rfc.md — "---"
  - echo/docs/03-respuesta-a-correcciones.md — "---"
