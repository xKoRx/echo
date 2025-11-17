# Revisión RFC 13a — parallel-slave-processing-nfrs — Iter 2

- **Resumen de auditoría**
  - Validé `echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md` + REPLY oficial contra los documentos base obligatorios y el template.
  - El RFC cierra los GAP-DEV previos (config en ETCD, backpressure y metas cuantitativas) y está prácticamente Dev/QA-Ready; solo quedan dos ajustes editoriales para consistencia operativa.

- **Matriz de conformidad por requisito**

| Requisito | Evidencia | Estado | Dev/QA Ready |
|-----------|-----------|--------|--------------|
| Configuración del pool centralizada en ETCD con validaciones y bootstrap inmutable | [echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#63-configuración-flags-y-parámetros](echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#63-configuración-flags-y-parámetros) | OK | SI |
| Backpressure determinista: colas finitas, rechazos y métrica `echo_core_router_rejections_total` | [RFC-13a#51-visión-general](echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#51-visión-general), [RFC-13a#54-reglas-de-negocio-y-casos-borde](echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#54-reglas-de-negocio-y-casos-borde), [RFC-13a#8-observabilidad-logs-métricas-trazas](echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#8-observabilidad-logs-métricas-trazas) | OK | SI |
| Definition of Done con metas cuantitativas (p95, cola ≤ límite, ≥99% éxito) | [RFC-13a#3-objetivos-medibles-definition-of-done](echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#3-objetivos-medibles-definition-of-done) | OK | SI |
| Tabla de touchpoints alineada con la nueva gobernanza de configuración (ETCD) | [RFC-13a#52-componentes-afectados-touchpoints](echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#52-componentes-afectados-touchpoints) | OBS | NO |
| Runbook/operación reflejan los nuevos límites configurables (`queue_depth_max`) | [RFC-13a#12-plan-de-rollout-bwc-y-operación](echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#12-plan-de-rollout-bwc-y-operación) | OBS | NO |

- **Cobertura PR-***

| PR-* | Evidencia | Estado | Comentario |
|------|-----------|--------|------------|
| PR-MVP | [RFC-13a#7-principios-de-diseño-y-trade-offs](echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#7-principios-de-diseño-y-trade-offs) | OK | Mantiene rollout directo sin FF. |
| PR-ROB | [RFC-13a#51-visión-general](echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#51-visión-general) | OK | Aislación por worker + rechazos controlados. |
| PR-MOD | [RFC-13a#52-componentes-afectados-touchpoints](echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#52-componentes-afectados-touchpoints) | OBS | Tabla de impacto contradice la nueva estrategia de configuración. |
| PR-ESC | [RFC-13a#51-visión-general](echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#51-visión-general) | OK | Dimensionamiento vía ETCD. |
| PR-CLN | [RFC-13a#51-visión-general](echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#51-visión-general) | OK | Pasos claros y responsabilidades delimitadas. |
| PR-SOLID | [RFC-13a#63-configuración-flags-y-parámetros](echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#63-configuración-flags-y-parámetros) | OK | `RouterConfig` inyectable. |
| PR-KISS | [RFC-13a#5-arquitectura-de-solución](echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#5-arquitectura-de-solución) | OK | Solo se añade hashing+colas. |
| PR-BWC | [RFC-13a#61-mensajes-contratos](echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#61-mensajes-contratos) | OK | Contratos proto intactos. |
| PR-OBS | [RFC-13a#8-observabilidad-logs-métricas-trazas](echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#8-observabilidad-logs-métricas-trazas) | OK | Métricas, logs y spans listos. |
| PR-IDEMP | [RFC-13a#54-reglas-de-negocio-y-casos-borde](echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#54-reglas-de-negocio-y-casos-borde) | OK | Orden por trade_id garantizado. |
| PR-RES | [RFC-13a#53-flujos-principales](echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#53-flujos-principales) | OK | Reintentos confinados por worker. |
| PR-SEC | [RFC-13a#9-matriz-pr-](echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#9-matriz-pr-) | OK | Sin superficie nueva ni datos sensibles adicionales. |
| PR-PERF | [RFC-13a#3-objetivos-medibles-definition-of-done](echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#3-objetivos-medibles-definition-of-done) | OK | p95 y ratios cuantificados. |

- **Hallazgos**

#### ARQ-002 — Tipo: ARQ — Severidad: MEN — PR-MOD/PR-CLN
- **Evidencia**: La tabla de touchpoints aún dice “`echo/core/internal/config` — Valor fijo `worker_pool_size` (build-time) — No expuesto vía ETCD (MVP)” [echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#52-componentes-afectados-touchpoints](echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#52-componentes-afectados-touchpoints), contradictorio con la nueva sección `§6.3` que mueve toda la configuración a ETCD [echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#63-configuración-flags-y-parámetros].
- **Impacto**: Devs/QA que lean la tabla rápida (usada para estimar impactos) podrían implementar la versión errónea (flags locales), generando deuda o re-trabajo. Rompe trazabilidad PR-MOD al dejar documentación inconsistente.
- **Propuesta**: Actualizar la fila para reflejar “Configuración en ETCD” (p.ej. “Carga inicial de claves `/echo/core/router/*` con validaciones”) y detallar que requiere reinicio para aplicar cambios.
- **Trade-offs**: Solo implica ajustar documentación; mantiene alineación con la realidad del diseño.

#### OPS-001 — Tipo: ARQ — Severidad: MEN — PR-ROB/PR-RES
- **Evidencia**: El runbook operativo mantiene el umbral fijo “`queue_depth` > 10 durante 5 minutos” [echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#12-plan-de-rollout-bwc-y-operación](echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#12-plan-de-rollout-bwc-y-operación), pero los nuevos límites de cola dependen de `queue_depth_max` configurable en ETCD (default 8) [echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#63-configuración-flags-y-parámetros].
- **Impacto**: Operaciones podría ignorar alertas reales cuando el límite es menor (p.ej., 8) o reaccionar tarde si el límite aumenta. Para QA también resulta ambiguo qué condición validar para degradación controlada.
- **Propuesta**: Reescribir el runbook para referenciar explícitamente `queue_depth_max` (p.ej., “alertar cuando `queue_depth` supere el 80 % del valor configurado”) y describir cómo obtener el valor actual desde ETCD/telemetría.
- **Trade-offs**: Ajuste documental mínimo que evita falsos positivos/negativos en operación.

- **Gaps de implementabilidad para Dev/QA (GAP-DEV)**
  - No se detectan GAP-DEV activos. La información sobre configuración, backpressure y objetivos de performance es suficiente para iniciar desarrollo y diseñar pruebas.

- **Citas faltantes / Suposiciones**
  - No se detectaron citas faltantes adicionales; todas las afirmaciones relevantes enlazan a docs oficiales.

- **Cambios sugeridos (diff textual conceptual)**
  - En `§5.2` reemplazar la fila del componente `echo/core/internal/config` por: “Carga y validación de claves `/echo/core/router/{worker_pool_size, queue_depth_max, worker_timeout_ms}` vía ETCD” y ajustar la nota a “Se requiere reinicio para aplicar cambios”.
  - En `§12.4 Operación y soporte` reemplazar “`queue_depth` > 10 durante 5 minutos” por “`queue_depth` > `0.8 * queue_depth_max` durante 5 minutos” (o referenciar la clave ETCD explícitamente).

- **Evaluación de riesgos**
  - La inconsistencia en la tabla puede inducir implementaciones fuera de estándar de configuración.
  - Un runbook con umbral fijo puede retrasar la detección de backpressure real o generar alertas fantasma, afectando PR-ROB/PR-RES durante la operación inicial.

- **Decisión**
  - `decision: OBSERVADO`
  - **Condiciones de cierre**: Actualizar la fila de touchpoints y el runbook operativo para referenciar la nueva configuración en ETCD y los límites derivados de `queue_depth_max`.

- **Refs cargadas**
  - `echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md` — "`---`"
  - `echo/docs/rfcs/13a/REPLY-13a-review-1.md` — "`| Hallazgo | Acción (Cambiar/Justificar) | PR-* | Sección corregida/enlazada |`"
  - `echo/docs/00-contexto-general.md` — "`---`"
  - `echo/docs/01-arquitectura-y-roadmap.md` — "`---`"
  - `echo/docs/rfcs/RFC-architecture.md` — "`---`"
  - `echo/vibe-coding/prompts/common-principles.md` — "`**PR-ROB** Robustez: tolerancia a fallos, timeouts, reintentos, backoff; sin afectar integridad de datos.`"
  - `echo/docs/templates/rfc.md` — "`---`"


