# Revisión RFC 13a — parallel-slave-processing-nfrs — Iter 1

- **Resumen de auditoría**
  - Revisé `echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md` contra los documentos base requeridos y el template oficial.
  - El RFC cubre la intención general del worker pool y añade criterios Given-When-Then, pero mantiene huecos críticos en configuración y resiliencia; lo considero aún en estado **conceptual/observado**.

- **Matriz de conformidad por requisito**

| Requisito | Evidencia | Estado | Dev/QA Ready |
|-----------|-----------|--------|--------------|
| Paralelismo por `trade_id` con orden determinista | [echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#51-visión-general](echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#51-visión-general) | OBS | NO |
| Telemetría específica del router (métricas/logs/spans) | [echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#8-observabilidad-logs-métricas-trazas](echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#8-observabilidad-logs-métricas-trazas) | OK | SI |
| Configuración del pool declarada y gestionada en ETCD | [echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#63-configuración-flags-y-parámetros](echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#63-configuración-flags-y-parámetros) | FALLA | NO |
| Compatibilidad hacia atrás en contratos gRPC/Pipes | [echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#61-mensajes-contratos](echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#61-mensajes-contratos) | OK | SI |
| Criterios QA Given-When-Then documentados | [echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#10-criterios-de-aceptación-given-when-then](echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#10-criterios-de-aceptación-given-when-then) | OK | SI |

- **Cobertura PR-***

| PR-* | Evidencia | Estado | Comentario |
|------|-----------|--------|------------|
| PR-MVP | [RFC-13a#7-principios-de-diseño-y-trade-offs](echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#7-principios-de-diseño-y-trade-offs) | OK | Elimina FF para priorizar entrega directa. |
| PR-ROB | [RFC-13a#53-flujos-principales](echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#53-flujos-principales) | OBS | No define qué ocurre ante colas crecientes o workers colgados. |
| PR-MOD | [RFC-13a#52-componentes-afectados-touchpoints](echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#52-componentes-afectados-touchpoints) | OK | Cambios encapsulados en el router y telemetría. |
| PR-ESC | [RFC-13a#51-visión-general](echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#51-visión-general) | OBS | No hay lineamientos para dimensionar `worker_pool_size`. |
| PR-CLN | [RFC-13a#52-componentes-afectados-touchpoints](echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#52-componentes-afectados-touchpoints) | INFO | No se describen interfaces ni responsabilidades internas del pool. |
| PR-SOLID | [RFC-13a#52-componentes-afectados-touchpoints](echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#52-componentes-afectados-touchpoints) | INFO | Falta detallar interfaces/contratos para invertir dependencias. |
| PR-KISS | [RFC-13a#5-arquitectura-de-solución](echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#5-arquitectura-de-solución) | OK | Mantiene el pipeline existente. |
| PR-OBS | [RFC-13a#8-observabilidad-logs-métricas-trazas](echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#8-observabilidad-logs-métricas-trazas) | OK | Define nombres, labels y spans. |
| PR-BWC | [RFC-13a#61-mensajes-contratos](echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#61-mensajes-contratos) | OK | Contratos proto se mantienen. |
| PR-IDEMP | [RFC-13a#54-reglas-de-negocio-y-casos-borde](echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#54-reglas-de-negocio-y-casos-borde) | OK | Continúa la secuencialidad por `trade_id`. |
| PR-RES | [RFC-13a#53-flujos-principales](echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#53-flujos-principales) | OBS | No existe plan de backpressure ni recuperación parcial de workers. |
| PR-SEC | [RFC-13a#61-mensajes-contratos](echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#61-mensajes-contratos) | INFO | No se mencionan implicancias de seguridad (no cambio declarado). |
| PR-PERF | [RFC-13a#3-objetivos-medibles-definition-of-done](echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#3-objetivos-medibles-definition-of-done) | OBS | No hay meta cuantitativa de latencia o throughput para validar la mejora. |

- **Hallazgos**

#### GAP-DEV-001 — Tipo: GAP-DEV — Severidad: BLOQ — PR-ROB/PR-BWC
- **Evidencia**: [RFC-13a#63-configuración-flags-y-parámetros](echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#63-configuración-flags-y-parámetros) define `worker_pool_size` como constante/flag del binario; los principios obligan a cargar toda configuración desde ETCD [echo/docs/00-contexto-general.md#principios-obligatorios-de-ingeniería](echo/docs/00-contexto-general.md#principios-obligatorios-de-ingeniería) y la arquitectura documenta jerarquía y claves [echo/docs/rfcs/RFC-architecture.md#9-configuración-etcd](echo/docs/rfcs/RFC-architecture.md#9-configuración-etcd).
- **Impacto**: Dev no puede implementar sin violar el estándar único de configuración; tampoco hay ruta ni clave en ETCD para que QA/ops ajusten el fanout. Esto bloquea el diseño y deja la capacidad del pool acoplada al binario, dificultando tunning y rollback.
- **Propuesta de cambio**: Definir claves ETCD bajo `/echo/core/router/worker_pool_size` (y, si aplica, `/echo/core/router/queue_depth_max`), documentar valores por entorno y cargar el valor una sola vez en bootstrap. Describir fallback y validaciones.
- **Trade-offs**: Añadir ETCD implica asegurar watches/caches, pero mantiene gobernanza centralizada y permite ajustar N sin recompilar.

#### GAP-DEV-002 — Tipo: GAP-DEV — Severidad: BLOQ — PR-ROB/PR-RES/PR-PERF
- **Evidencia**: Ni la visión general ni los flujos del router ([RFC-13a#51](echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#51-visión-general), [RFC-13a#53](echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#53-flujos-principales)) especifican tamaño de cola, política de backpressure, ni qué hacer ante saturación; el contexto base exige canales con buffer y control explícito de backpressure [echo/docs/00-contexto-general.md#6-conexiones-inestables-y-backpressure](echo/docs/00-contexto-general.md#6-conexiones-inestables-y-backpressure).
- **Impacto**: Dev no sabe si debe crear colas acotadas, descartar, bloquear o reintentar cuando `worker_id` se atrasa. Sin lineamientos, se puede seguir bloqueando el fanout o agotar memoria, y QA no puede definir pruebas de degradación.
- **Propuesta de cambio**: Documentar parámetros `queue_depth_max`, timeout por worker y estrategia cuando se alcanza el límite (rechazo, prioridad, pausas). Añadir métrica/alerta y runbook asociado.
- **Trade-offs**: Establecer límites puede forzar rechazos anticipados, pero ofrece comportamiento determinista y reduce el riesgo de slippage incontrolado.

#### ARQ-001 — Tipo: ARQ — Severidad: MAY — PR-CLN/PR-SOLID/PR-SEC/PR-PERF
- **Evidencia**: La nueva sección de matriz PR-* solo cubre siete principios ([RFC-13a#9-matriz-pr-](echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#9-matriz-pr-)), omitiendo CLN, SOLID, KISS, SEC y PERF definidos como obligatorios [echo/vibe-coding/prompts/common-principles.md#pr-rob-robustez-tolerancia-a-fallos-timeouts-reintentos-backoff-sin-afectar-integridad-de-datos](echo/vibe-coding/prompts/common-principles.md#pr-rob-robustez-tolerancia-a-fallos-timeouts-reintentos-backoff-sin-afectar-integridad-de-datos).
- **Impacto**: El revisor y el owner no tienen trazabilidad de cómo el diseño respeta limpieza de código, seguridad o performance. Falta de evidencia dificulta asegurar Dev/QA-Ready.
- **Propuesta de cambio**: Completar la matriz con todas las PR-* del documento base, marcando estado/evidencia por cada una (CLN, SOLID, KISS, SEC, PERF, MVP, etc.).
- **Trade-offs**: Más contenido, pero evita debates posteriores y ancla decisiones a principios explícitos.

#### PERF-001 — Tipo: GAP-DEV — Severidad: MAY — PR-PERF
- **Evidencia**: El DoD declara que “no se fija un SLO” para las nuevas métricas ([RFC-13a#3-objetivos-medibles-definition-of-done](echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#3-objetivos-medibles-definition-of-done)), pese a que el contexto exige p95 intra-host <100 ms y control de slippage [echo/docs/00-contexto-general.md#qué-hace-único-a-echo-visión-de-clase-mundial](echo/docs/00-contexto-general.md#qué-hace-único-a-echo-visión-de-clase-mundial).
- **Impacto**: QA no puede validar que el paralelismo realmente mejora la latencia ni detectar regresiones; Dev carece de un objetivo concreto para evaluar la efectividad del pool.
- **Propuesta de cambio**: Definir métricas de aceptación (p.ej., `echo_core_router_dispatch_duration_ms` p95 < 40 ms con N=4 y 8 slaves activos) y umbrales de cola admisibles.
- **Trade-offs**: Obliga a medir y reportar, pero otorga un criterio objetivo para aprobar la iteración.

- **Gaps de implementabilidad para Dev/QA (GAP-DEV)**
  - **GAP-DEV-001** (`§6.3 Configuración, flags y parámetros`): Falta ruta ETCD y contrato para `worker_pool_size`, bloqueando la construcción del bootstrap y la parametrización operativa.
  - **GAP-DEV-002** (`§5.1–5.3 Arquitectura y flujos`): No existe definición de `queue_depth_max`, política de backpressure ni manejo de workers lentos; Dev no sabe cómo implementar las colas ni QA cómo simular saturación.
  - **PERF-001** (`§3 Definition of Done`): La ausencia de metas cuantitativas impide a QA diseñar pruebas que verifiquen la mejora de latencia y throughput.
  - **Conclusión**: No se puede iniciar implementación ni pruebas sin inventar la gobernanza de configuración, límites de cola y criterios de performance.

- **Citas faltantes / Suposiciones**
  - No se detectaron citas faltantes adicionales; todas las afirmaciones revisadas refieren al propio RFC o a docs base.

- **Cambios sugeridos (diff textual conceptual)**
  - En `§6.3`, sustituir la descripción de constante por una tabla ETCD: clave, tipo, default, validaciones y cómo se carga en bootstrap.
  - Añadir en `§5.1/§5.3` un párrafo que detalle `queue_depth_max`, política al alcanzar el límite y relación con métricas/alertas.
  - Completar `§9 Matriz PR-*` con las PR restantes (CLN, SOLID, KISS, SEC, PERF) indicando estado y evidencia.
  - Extender `§3 Definition of Done` con umbrales cuantitativos (p95 de `dispatch_duration`, límite de `queue_depth`, ratio de éxito mínimo) y ligarlos a los criterios de aceptación.

- **Evaluación de riesgos**
  - Sin gobernanza de configuración, cada build puede tener un `worker_pool_size` distinto sin registro, elevando riesgo de rollbacks fallidos.
  - Colas sin límites claros pueden crecer indefinidamente ante un slave lento, afectando p95 <100 ms y pudiendo causar OOM.
  - La falta de métricas objetivo dificulta detectar si el cambio realmente reduce slippage; existe riesgo de gastar iteraciones sin validar beneficios.

- **Decisión**
  - `decision: RECHAZADO`
  - **Condiciones de cierre**: Resolver GAP-DEV-001, GAP-DEV-002 y PERF-001; completar la matriz PR-* (ARQ-001) antes de nuevo handoff.

- **Refs cargadas**
  - `echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md` — "`---`"
  - `echo/docs/00-contexto-general.md` — "`---`"
  - `echo/docs/01-arquitectura-y-roadmap.md` — "`---`"
  - `echo/docs/rfcs/RFC-architecture.md` — "`---`"
  - `echo/vibe-coding/prompts/common-principles.md` — "`**PR-ROB** Robustez: tolerancia a fallos, timeouts, reintentos, backoff; sin afectar integridad de datos.`"
  - `echo/docs/templates/rfc.md` — "`---`"


