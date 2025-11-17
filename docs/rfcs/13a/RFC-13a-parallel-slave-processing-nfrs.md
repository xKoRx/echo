---
rfc_id: "RFC-13a"
title: "Iteración 13a — Paralelismo seguro de órdenes por slave"
version: "1.0"
status: "draft"
owner_arch: "Arquitectura Echo"
owner_dev: "Echo Core"
owner_qa: "QA Echo"
date: "2025-11-16"
iteration: "13a"
type: "refactor"
depends_on:
  - "echo/docs/00-contexto-general.md"
  - "echo/docs/01-arquitectura-y-roadmap.md"
  - "echo/docs/rfcs/RFC-architecture.md"
  - "echo/vibe-coding/prompts/common-principles.md"
related_rfcs:
  - "echo/docs/rfcs/RFC-002-routing-selectivo.md"
  - "echo/docs/rfcs/RFC-006-iteracion-6-fixed-risk.md"
tags:
  - "core"
  - "agent"
  - "sdk"
---

# RFC-13a: Iteración 13a — Paralelismo seguro de órdenes por slave

---

## 1. Resumen ejecutivo

- El Core continúa procesando órdenes de los slaves secuencialmente, generando colas y slippage innecesario frente a la meta de fanout paralelo ya identificada como patrón crítico del dominio [echo/docs/00-contexto-general.md#problemas-típicos-del-dominio-y-patrones-de-solución].
- Esta iteración introduce un worker pool ordenado por `trade_id` dentro del Core para ejecutar comandos hacia múltiples slaves en paralelo sin tocar contratos ni servicios de negocio [echo/docs/rfcs/RFC-architecture.md#4.3-core].
- El beneficio inmediato es reducir latencia y riesgo de desincronización intra-host, directamente alineado con la visión de baja latencia y trazabilidad de Echo [echo/docs/00-contexto-general.md#qué-hace-único-a-echo-visión-de-clase-mundial].
- Principios dominantes: PR-MVP (priorizar funcionamiento sin FF/rollout), PR-ROB, PR-ESC y PR-BWC [echo/vibe-coding/prompts/common-principles.md#pr-mvp-modo-mvp-esto-quiere-decir-que-por-sobre-todas-las-cosas-se-deben-obviar-temas-de-seguridad-slo-rollout-controlados-feature-flags-etc-prima-por-sobre-todas-las-cosas-la-velocidad].

---

## 2. Contexto y motivación

### 2.1 Estado actual

- El Core dispone de un router secuencial que toma intents deduplicados, aplica políticas (`RiskPolicyService`, `SymbolResolver`, etc.) y publica `ExecuteOrder` hacia el Agent siguiendo estricto orden FIFO [echo/docs/rfcs/RFC-architecture.md#4.3-core].
- La cadena Master→Core→Agent→Slave mantiene correlaciones vía UUIDv7 y tablas en PostgreSQL (`trades`, `executions`, `dedupe`) sin paralelismo interno por `trade_id` [echo/docs/rfcs/RFC-architecture.md#5.1-flujo-de-datos].
- El roadmap oficial marca la iteración i13a como el punto donde se habilita “worker pool ordenado sin bloqueos” [echo/docs/01-arquitectura-y-roadmap.md#roadmap-evolutivo-post-i1].

### 2.2 Problema / gaps

- La ejecución secuencial introduce colas en escenarios con múltiples slaves activos, lo que aumenta latencia intra-host y slippage, contraviniendo el patrón de fanout paralelo recomendado para mitigar divergencias de precio [echo/docs/00-contexto-general.md#problemas-típicos-del-dominio-y-patrones-de-solución].
- Sin paralelismo, un fallo puntual en un slave bloquea órdenes posteriores aunque pertenezcan a `trade_id` distintos, comprometiendo la resiliencia y el throughput objetivo <100 ms intra-host [echo/docs/00-contexto-general.md#qué-hace-único-a-echo-visión-de-clase-mundial].
- La ausencia de telemetría específica de colas impide a operaciones observar degradaciones antes de que impacten en trading, lo que viola PR-OBS [echo/docs/rfcs/RFC-architecture.md#7.5-observabilidad].

### 2.3 Objetivo de la iteración

- Habilitar ejecución paralela controlada por `trade_id` para alcanzar el hito de i13a sin alterar contratos externos [echo/docs/01-arquitectura-y-roadmap.md#roadmap-evolutivo-post-i1].
- Mantener PR-MVP: sin feature flags, sin rollout gradual y sin nuevos SLO, priorizando la salida funcional rápida acordada para V1 [echo/vibe-coding/prompts/common-principles.md#pr-mvp-modo-mvp-esto-quiere-decir-que-por-sobre-todas-las-cosas-se-deben-obviar-temas-de-seguridad-slo-rollout-controlados-feature-flags-etc-prima-por-sobre-todas-las-cosas-la-velocidad].

---

## 3. Objetivos medibles (Definition of Done)

- El Core debe despachar órdenes en paralelo siempre que los `trade_id` difieran, manteniendo orden determinista por `trade_id` y sin introducir duplicaciones (ver validaciones de dedupe vigentes) [echo/docs/rfcs/RFC-architecture.md#5.1-flujo-de-datos].
- Métrica `echo_core_router_dispatch_duration_ms` (histogram) debe registrar p95 ≤ 40 ms con `worker_pool_size=4` y ≥8 slaves activos en staging; este umbral deja margen para alcanzar el objetivo intra-host <100 ms descrito en la visión del copiador [echo/docs/00-contexto-general.md#qué-hace-único-a-echo-visión-de-clase-mundial].
- `echo_core_router_queue_depth` (gauge) no debe superar `queue_depth_max` (valor fijado en ETCD) durante más de 5 segundos; al superarse, el router debe emitir rechazos controlados y alertar a operaciones [echo/docs/00-contexto-general.md#6-conexiones-inestables-y-backpressure].
- Ratio de despachos exitosos `echo_core_router_dispatch_total{result="success"}` ≥ 99% por ventana de 5 minutos; los errores deben mapearse a códigos existentes para mantener compatibilidad y trazabilidad [echo/docs/00-contexto-general.md#principios-obligatorios-de-ingeniería].
- No se permite degradar idempotencia ni contratos proto; ejecutar pruebas de regresión básicas para asegurar correlaciones `trade_id ↔ ticket` intactas [echo/docs/00-contexto-general.md#principios-obligatorios-de-ingeniería].
- QA valida al menos dos escenarios: (a) órdenes simultáneas hacia distintos slaves completan sin bloqueo cruzado; (b) un slave defectuoso no bloquea otros trades.

---

## 4. Alcance

### 4.1 Dentro de alcance

- Diseño e implementación del worker pool por `trade_id` en `echo/core/internal/router` (nombre actual) manteniendo compatibilidad con servicios existentes [echo/docs/rfcs/RFC-architecture.md#4.3-core].
- Nueva telemetría de colas, spans y logs para el router.
- Documentación de operación y pruebas mínimas necesarias para validar paralelismo.

### 4.2 Fuera de alcance (y futuro)

- Cambios en protocolos gRPC, proto o Named Pipes (se mantienen los mensajes actuales) [echo/docs/rfcs/RFC-architecture.md#8.2-tradeproto].
- Ajustes de sizing, políticas de riesgo o recalculo de lotes (cubierto por i6) [echo/docs/rfcs/RFC-architecture.md#4.3-core].
- Rollouts controlados, feature flags, degradaciones progresivas o nuevos SLO (expresamente excluidos por PR-MVP) [echo/vibe-coding/prompts/common-principles.md#pr-mvp-modo-mvp-esto-quiere-decir-que-por-sobre-todas-las-cosas-se-deben-obviar-temas-de-seguridad-slo-rollout-controlados-feature-flags-etc-prima-por-sobre-todas-las-cosas-la-velocidad].

---

## 5. Arquitectura de solución

### 5.1 Visión general

1. El Core recibe `TradeIntent` ya deduplicado, lo valida y construye `ExecuteOrder` como hoy [echo/docs/rfcs/RFC-architecture.md#5.1-flujo-de-datos].
2. En vez de encolar todos los comandos en un solo canal secuencial, se calcula `worker_index = hash(trade_id) % worker_pool_size` y se envía a la cola del worker correspondiente; `worker_pool_size` proviene de ETCD (ver §6.3).
3. Cada cola está acotada por `queue_depth_max`; si un enqueue excede ese límite, el router rechaza la orden con `ERROR_CODE_BROKER_BUSY`, incrementa `echo_core_router_rejections_total` y registra el evento para que el master decida reintentar [echo/docs/00-contexto-general.md#6-conexiones-inestables-y-backpressure].
4. Cada worker procesa su cola local preservando el orden de llegada para ese `trade_id`, asegurando que comandos del mismo trade no se intercalen; diferentes workers ejecutan en paralelo.
5. Los workers aplican `worker_timeout_ms` (configurada en ETCD) para cada interacción con Agent; al excederse, colocan el comando en retry según la lógica existente y reportan la saturación.
6. Telemetría: cada worker emite spans `core.router.worker`, métricas de profundidad/tiempo y rejections para correlacionar carga vs. capacidad.

### 5.2 Componentes afectados (touchpoints)

| Componente | Tipo de cambio | BWC (sí/no) | Notas breves |
|------------|----------------|-------------|--------------|
| `echo/core/internal/router` | Refactor para crear worker pool | sí | Nuevo orquestador sin tocar API externa |
| `echo/core/internal/telemetry` | Nuevas métricas/spans | sí | Reusa cliente existente |
| `echo/core/internal/config` | Carga inicial de `/echo/core/router/{worker_pool_size,queue_depth_max,worker_timeout_ms}` desde ETCD | sí | Bootstrap inmutable; requiere reinicio para aplicar cambios |
| `echo/agent/...` | Sin cambios | sí | Solo recibe órdenes más rápido |
| DB `public.*` | Sin cambios | sí | Persistencia actual se reutiliza |

### 5.3 Flujos principales

- **Flujo happy path**: Master emite dos `TradeIntent` con `trade_id` distintos → ambos caen en workers distintos → cada worker genera `ExecuteOrder` y lo envía al Agent → Agent los propaga a los slaves correspondientes. Orden relativo por `trade_id` se preserva en cada cola [echo/docs/rfcs/RFC-architecture.md#5.1-flujo-de-datos].
- **Slave lento**: si un worker detecta timeout/retry al interactuar con el Agent, solo su cola se ve afectada; otras colas continúan procesando. El worker actualiza `queue_depth` y, si alcanza `queue_depth_max`, inicia rechazo controlado hasta que la profundidad caiga por debajo del 80 % del límite.
- **Backpressure en Core**: al recibir más intents de los que puede procesar (todas las colas llenas), el Core responde `worker_pool_backpressure=true` en logs y expone `echo_core_router_rejections_total`. Operaciones pueden ajustar `worker_pool_size` o `queue_depth_max` desde ETCD y reiniciar el Core para aplicar cambios [echo/docs/rfcs/RFC-architecture.md#9-configuración-etcd].
- **Reintento/Retry**: los workers reusan la lógica de reintentos existente, respetando `worker_timeout_ms`; los errores siguen el mismo contrato que el router secuencial.

### 5.4 Reglas de negocio y casos borde

- No se pueden ejecutar dos comandos del mismo `trade_id` simultáneamente; la cola del worker debe esperar a que finalice el comando previo antes de despachar el siguiente.
- Si un worker falla, el proceso debe reiniciarse completo (no hay restart individual), pero la deduplicación en Postgres impide duplicar operaciones al retomar [echo/docs/rfcs/RFC-architecture.md#5.1-flujo-de-datos].
- Cuando `queue_depth_max` se alcanza, el router devuelve rechazo determinista (`ERROR_CODE_BROKER_BUSY`) y añade atributos `backpressure=true`, `worker_id`, `queue_depth` en el log para facilitar diagnóstico; QA debe incluir este caso en pruebas de estrés [echo/docs/00-contexto-general.md#6-conexiones-inestables-y-backpressure].
- Cualquier orden que exceda los límites actuales (p.ej., falta de política) se rechaza igual que hoy; el worker no reinterpreta reglas de negocio.

---

## 6. Contratos, datos y persistencia

### 6.1 Mensajes / contratos

- No se introducen mensajes nuevos; `TradeIntent`, `ExecuteOrder` y `ExecutionResult` permanecen idénticos, cumpliendo el mandato de BWC del roadmap i13a [echo/docs/rfcs/RFC-architecture.md#8.2-tradeproto].
- Sin nuevos códigos de error.

### 6.2 Modelo de datos y esquema

- No se modifican tablas ni índices. El worker pool solo opera en memoria apoyado sobre los registros actuales (`trades`, `executions`, `dedupe`) [echo/docs/rfcs/RFC-architecture.md#4.3-core].

### 6.3 Configuración, flags y parámetros

Toda configuración del router vive en ETCD bajo `/echo/core/router/`, respetando la jerarquía documentada para el Core [echo/docs/rfcs/RFC-architecture.md#9-configuración-etcd]. Se cargan una sola vez durante el bootstrap del Core; al cambiar valores se requiere reinicio (modo MVP sin toggles en runtime).

| Clave ETCD | Tipo / default (dev) | Validaciones | Uso |
|------------|----------------------|--------------|-----|
| `/echo/core/router/worker_pool_size` | entero, default 4 | `>=2`, `<=32`; debe ser potencia de dos para hashing uniforme | Tamaño del pool. `hash(trade_id) % worker_pool_size` define el worker; cambiarlo altera distribución y requiere limpiar colas antes del reinicio. |
| `/echo/core/router/queue_depth_max` | entero, default 8 | `>=4`, `<=128` | Profundidad máxima por worker. Cuando `queue_depth` alcanza el límite se activan rechazos controlados (`ERROR_CODE_BROKER_BUSY`). |
| `/echo/core/router/worker_timeout_ms` | entero, default 50 | `>=20`, `<=200` | Timeout de interacción con Agent antes de aplicar retry/backoff. |

Carga y validaciones:
- El bootstrap (`core/cmd/echo-core/main.go`) crea un cliente ETCD, lee las claves y aborta si faltan (manteniendo el principio de configuración única) [echo/docs/00-contexto-general.md#principios-obligatorios-de-ingeniería].
- Se almacenan en un struct inmutable (`RouterConfig`) inyectado al paquete router. No se admiten feature flags ni toggles de runtime; cualquier ajuste se hace editando ETCD y reiniciando el proceso.
- Los valores se exponen en métricas (`router_config_worker_pool_size`, etc.) para trazabilidad operativa, pero no se permiten sobreescrituras locales.

---

## 7. Principios de diseño y trade-offs

- **PR-MVP**: se prioriza liberar el paralelismo sin esperar guardrails de rollout; los riesgos se gestionan mediante pruebas y reversión manual (no flags) [echo/vibe-coding/prompts/common-principles.md#pr-mvp-modo-mvp-esto-quiere-decir-que-por-sobre-todas-las-cosas-se-deben-obviar-temas-de-seguridad-slo-rollout-controlados-feature-flags-etc-prima-por-sobre-todas-las-cosas-la-velocidad].
- **PR-ROB & PR-RES**: la aislación por worker evita que un slave defectuoso bloquee todo el sistema, reduciendo el riesgo de colas globales [echo/docs/00-contexto-general.md#problemas-típicos-del-dominio-y-patrones-de-solución].
- **PR-ESC**: el pool permite escalar throughput linealmente aumentando N (build-time) sin reescribir servicios [echo/docs/01-arquitectura-y-roadmap.md#roadmap-evolutivo-post-i1].
- **Trade-off**: al no contar con FF ni rollout gradual, cualquier bug requiere revertir el despliegue completo; se acepta el costo para honrar PR-MVP.

---

## 8. Observabilidad (logs, métricas, trazas)

### 8.1 Métricas

- `echo_core_router_queue_depth` (gauge): profundidad actual por worker. Labels: `worker_id`, `component="core.router"`.
- `echo_core_router_dispatch_total` (counter): cantidad de órdenes despachadas por worker. Labels: `worker_id`, `result` (`success`, `error`).
- `echo_core_router_dispatch_duration_ms` (histogram con buckets ms): tiempo desde enqueue hasta envío al Agent.
- `echo_core_router_rejections_total` (counter): rechazos por backpressure cuando `queue_depth_max` se alcanza. Labels: `worker_id`, `reason="queue_full"`.

### 8.2 Logs estructurados

- Nuevos eventos `core.router.enqueue` y `core.router.dispatch` con campos JSON `{app="echo-core", comp="router", trade_id, worker_id, queue_depth, result}` heredando atributos comunes [echo/docs/00-contexto-general.md#principios-obligatorios-de-ingeniería].
- Para auditoría se exige loggear asignación worker→trade (`worker_id`) dado que el owner lo solicitó explícitamente.

### 8.3 Trazas y spans

- Span `core.router.schedule` alrededor del cálculo de hash y encolado.
- Span `core.router.worker` por cada comando procesado, con atributos `trade_id`, `worker_id`, `account_id`, `strategy_id`, `result` [echo/docs/rfcs/RFC-architecture.md#7.5-observabilidad].
- Relacionar los spans con las métricas usando los mismos labels para facilitar correlación en Jaeger/Grafana.

---

## 9. Matriz PR-*

| Principio | Estado | Evidencia | Justificación |
|-----------|--------|-----------|---------------|
| PR-MVP | OK | [echo/vibe-coding/prompts/common-principles.md#pr-mvp-modo-mvp-esto-quiere-decir-que-por-sobre-todas-las-cosas-se-deben-obviar-temas-de-seguridad-slo-rollout-controlados-feature-flags-etc-prima-por-sobre-todas-las-cosas-la-velocidad] | Sin FF ni rollout gradual; cambios se liberan directo con foco en velocidad. |
| PR-ROB | OK | [echo/docs/00-contexto-general.md#problemas-típicos-del-dominio-y-patrones-de-solución] | Workers aislados evitan que un slave defectuoso bloquee todo el fanout. |
| PR-MOD | OK | [echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#52-componentes-afectados-touchpoints](echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#52-componentes-afectados-touchpoints) | Impacto confinado al router y telemetría; agentes/EAs no cambian. |
| PR-ESC | OK | [echo/docs/01-arquitectura-y-roadmap.md#roadmap-evolutivo-post-i1] | Ajustando `worker_pool_size` por ETCD se escala throughput sin tocar contratos. |
| PR-CLN | OK | [echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#51-visión-general](echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#51-visión-general) | Responsabilidades del router están documentadas con pasos y reglas explícitas. |
| PR-SOLID | OK | [echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#63-configuración-flags-y-parámetros](echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#63-configuración-flags-y-parámetros) | `RouterConfig` se inyecta como dependencia, permitiendo probar el router contra interfaces. |
| PR-KISS | OK | [echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#5-arquitectura-de-solución](echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#5-arquitectura-de-solución) | Se reutiliza el pipeline actual; solo se añade hashing + colas finitas. |
| PR-BWC | OK | [echo/docs/rfcs/RFC-architecture.md#8.2-tradeproto] | Contratos gRPC/Named Pipe permanecen idénticos y la dedupe sigue vigente. |
| PR-OBS | OK | [echo/docs/rfcs/RFC-architecture.md#7.5-observabilidad] | Métricas, spans y logs específicos del router permiten diagnosticar colas. |
| PR-IDEMP | OK | [echo/docs/rfcs/RFC-architecture.md#5.1-flujo-de-datos] | Orden por `trade_id` y dedupe en Postgres garantizan reintentos seguros. |
| PR-RES | OK | [echo/docs/00-contexto-general.md#principios-obligatorios-de-ingeniería] | Retries permanecen locales al worker afectado, manteniendo resiliencia del resto. |
| PR-SEC | OK | [echo/docs/00-contexto-general.md#principios-obligatorios-de-ingeniería] | No se agregan superficies nuevas; se conservan logs JSON sin PII adicional. |
| PR-PERF | OK | [echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#3-objetivos-medibles-definition-of-done](echo/docs/rfcs/13a/RFC-13a-parallel-slave-processing-nfrs.md#3-objetivos-medibles-definition-of-done) | Se fijan metas de p95 y límites de cola alineados al objetivo <100 ms intra-host. |

---

## 10. Criterios de aceptación (Given-When-Then)

- **CA-01 — Paralelismo exitoso (happy path)**  
  Given el Core está procesando dos `TradeIntent` con `trade_id` distintos y policies válidas,  
  When ambos ingresan al router,  
  Then cada uno se asigna a workers distintos y los `ExecuteOrder` llegan al Agent sin bloquearse entre sí [echo/docs/rfcs/RFC-architecture.md#5.1-flujo-de-datos].

- **CA-02 — Aislamiento ante error de slave**  
  Given un worker experimenta timeout al enviar `ExecuteOrder` hacia cierto Agent,  
  When otro worker procesa un trade distinto,  
  Then el segundo worker completa su envío sin esperar al primero y el error se limita al trade original [echo/docs/00-contexto-general.md#problemas-típicos-del-dominio-y-patrones-de-solución].

- **CA-03 — Compatibilidad hacia atrás**  
  Given un Agent legado en handshake v2 recibe comandos del Core con worker pool habilitado,  
  When procesa órdenes provenientes del nuevo router,  
  Then mantiene el mismo contrato `ExecuteOrder`/`ExecutionResult` y correlación `trade_id ↔ ticket` sin requerir actualización adicional [echo/docs/rfcs/RFC-architecture.md#8.2-tradeproto].

---

## 11. Plan de pruebas (Dev y QA)

### 9.1 Casos de uso E2E

| ID | Descripción | Precondiciones | Resultado esperado |
|----|-------------|----------------|--------------------|
| E2E-01 | Dos trades paralelos con slaves distintos | Cuentas registradas y policies válidas | Ambos comandos enviados sin bloqueo cruzado |
| E2E-02 | Slave defectuoso + slave sano | Un slave simula timeout | Solo el worker afectado genera reintentos; el otro completa |

### 9.2 Pruebas del Dev

- Tests unitarios table-driven para el hashing de `trade_id` → `worker_id` y para garantizar orden estable en la cola.
- Tests de integración para verificar que el worker pool invoca servicios existentes (`RiskPolicyService`, `SymbolResolver`) sin cambios.

### 9.3 Pruebas de QA

- QA ejecuta regresión básica sobre flujos market-only con múltiples slaves; no se requieren tests de estrés extendidos por decisión del owner (modo MVP).

### 9.4 Datos de prueba

- Fixtures: cuentas con políticas `FIXED_LOT` y `FIXED_RISK`, dos slaves por master, catálogo de símbolos cargado [echo/docs/rfcs/RFC-architecture.md#4.3-core].

---

## 12. Plan de rollout, BWC y operación

### 10.1 Estrategia de despliegue

- Despliegue directo (sin FF) del binario Core con worker pool habilitado; Agent y EAs no requieren cambios [echo/docs/rfcs/RFC-architecture.md#4.2-agent].
- Secuencia: actualizar Core en staging → smoke → producción el mismo día.

### 10.2 Backward compatibility

- BWC garantizada porque los contratos gRPC/Named Pipe no cambian y la deduplicación se mantiene idéntica [echo/docs/rfcs/RFC-architecture.md#5.1-flujo-de-datos].

### 10.3 Rollback y mitigación

- Única opción: revertir al binario previo del Core; no existen toggles ni FF y esto debe quedar explícito (PR-MVP).
- Riesgo: órdenes encoladas en el worker pool se pierden al reiniciar; mitigación: antes del rollback se drena cada worker esperando que sus colas queden vacías.

### 10.4 Operación y soporte

- Actualizar dashboards para incluir métricas nuevas.
- Runbook: “si `queue_depth` supera el 80 % de `queue_depth_max` (valor leíble desde `/echo/core/router/queue_depth_max` en ETCD o métrica `echo_core_router_queue_depth`) durante 5 minutos, revisar logs de `core.router.worker`, evaluar aumento temporal de `worker_pool_size` o drenado del worker afectado y reiniciar el Core solo si se identifica worker colgado”.

---

## 13. Riesgos, supuestos y preguntas abiertas

### 11.1 Riesgos

- R1 (MAY): bug en hashing provoca hot-spot en un worker → Mitigación: testear bucketización con cargas sintéticas previas al release.
- R2 (MEN): sin FF no existe forma de degradar a modo secuencial sin redeploy → Mitigación: mantener binario previo listo para rollback inmediato.

### 11.2 Supuestos

- Todos los agentes/hosts ya están en handshake v2 y toleran paralelo (estado `✅` en arquitectura base) [echo/docs/rfcs/RFC-architecture.md#4.2-agent].
- Los slaves siguen siendo hedged y respalda la correlación trade_id→ticket; si cambia el modelo de cuentas, se requiere nuevo RFC [echo/docs/00-contexto-general.md#qué-es-un-copiador-de-operaciones].

### 11.3 Preguntas abiertas / NEED-INFO

- Ninguna; QPACK resuelto por el owner.

---

## 14. Trabajo futuro (iteraciones siguientes)

- i13b: límites explícitos de backpressure y métricas de cola (complemento natural de este RFC) [echo/docs/01-arquitectura-y-roadmap.md#roadmap-evolutivo-post-i1].
- i14: dashboards avanzados para slippage/latencia, aprovechando las métricas introducidas aquí [echo/docs/01-arquitectura-y-roadmap.md#roadmap-evolutivo-post-i1].

---

## 15. Referencias

- echo/docs/00-contexto-general.md — "---"
- echo/docs/01-arquitectura-y-roadmap.md — "---"
- echo/docs/rfcs/RFC-architecture.md — "---"
- echo/vibe-coding/prompts/common-principles.md — "**PR-ROB** Robustez: tolerancia a fallos, timeouts, reintentos, backoff; sin afectar integridad de datos."
- echo/docs/rfcs/RFC-002-routing-selectivo.md — "---"
- echo/docs/rfcs/RFC-006-iteracion-6-fixed-risk.md — "---"
