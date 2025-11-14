# RFC i8(a+b) — sl-tp-offset-stop-level

## Resumen ejecutivo
La iteración i8(a+b) incorpora el copiado de SL/TP con offset configurable y la validación automática de StopLevel para sostener la sincronía maestro→slave sin degradar la latencia operativa definida para Echo V1.[echo/docs/01-arquitectura-y-roadmap.md#roadmap-evolutivo-post-i1][echo/docs/rfcs/RFC-architecture.md#5-2-logica-v1-segun-prd] Esta evolución aborda los rechazos por restricciones del broker descritos como riesgo recurrente del dominio, reduciendo missed trades y manteniendo trazabilidad completa.[echo/docs/00-contexto-general.md#problemas-tipicos-del-dominio-y-patrones-de-solucion]

## Alcance / No alcance
- **Incluido**: cálculo centralizado de offsets SL/TP, verificación de StopLevel pre y post fill, y modificaciones diferidas cuando el broker rechaza los niveles iniciales.[echo/docs/rfcs/RFC-architecture.md#5-2-logica-v1-segun-prd]
- **Incluido**: propagación determinística de parámetros mediante contratos Core↔Agent↔EA manteniendo idempotencia por `command_id`.[echo/docs/rfcs/RFC-architecture.md#4-4-slave-ea]
- **Excluido**: estrategias de espera de mejora o reintentos agresivos; permanecen fuera de alcance de i8 y se mantienen en iteraciones posteriores.[echo/docs/01-arquitectura-y-roadmap.md#roadmap-evolutivo-post-i1]
- **Excluido**: cambios en Money Management o políticas de sizing; se reutiliza el motor `FixedRisk` establecido.[echo/docs/rfcs/RFC-architecture.md#6-funcionalidades-clave-v1]

## Contexto y supuestos
- Los brokers imponen StopLevel variables; si se incumplen, rechazan la orden, generando desincronización que debemos prevenir.[echo/docs/00-contexto-general.md#problemas-tipicos-del-dominio-y-patrones-de-solucion]
- Echo debe conservar latencia intra-host <100 ms p95 y trazabilidad end-to-end en cualquier ajuste sobre órdenes replicadas.[echo/docs/00-contexto-general.md#que-hace-unico-a-echo-vision-de-clase-mundial]
- Core continúa siendo el orquestador de políticas y persistencia, evitando lógica de negocio en los EAs.[echo/docs/rfcs/RFC-architecture.md#3-2-componentes-principales]
- La configuración y telemetría existentes son obligatorias y no deben duplicarse ni divergir entre módulos.[echo/docs/rfcs/RFC-architecture.md#7-5-observabilidad]

## Diseño propuesto

### Componentes y responsabilidades
- **Core**: mientras se planifica la refactorización hacia un paquete dedicado, `StopLevelGuard` se integra en el router existente (`core/internal/router.go`) antes de invocar el `CommandBuilder` actual (`core/internal/commandbuilder.go`), reutilizando hooks y contratos vigentes sin introducir nuevos paquetes en esta iteración.[echo/docs/rfcs/RFC-architecture.md#4-3-core]
- **Agent**: añade un `AdjustmentQueue` idempotente para reenviar comandos de modificación (`ModifyStops`) al EA manteniendo orden por `command_id`, con tamaño de cola de 1 por `trade_id`, hasta 3 intentos (inmediato, ≤10 ms y ≤20 ms con jitter ≤5 ms) y límite duro de 20 ms para preservar el SLO intra-host antes de concluir con éxito o devolver al Core un `ExecutionResult` con `ErrorCode_ERROR_CODE_TIMEOUT`; no se agregan campos a los contratos, y la distinción del escenario de timeout se expone via métricas y spans.[echo/docs/00-contexto-general.md#6-conexiones-inestables-y-backpressure][echo/docs/rfcs/RFC-architecture.md#8-2-trade-proto]
- **Slave EA**: ejecuta órdenes con offsets recibidos y, ante instrucciones de modificación diferida, valida nuevamente `MarketInfo` del símbolo para evitar ejecuciones con quotes estancadas.[echo/docs/rfcs/RFC-architecture.md#4-4-slave-ea]

### Interfaces públicas (I/O, errores, contratos)
- `core/capabilities/StopLevelGuard`: interfaz que recibe `ExecuteOrder` enriquecido y retorna decisión (`ACCEPT_WITH_OFFSETS`, `ACCEPT_WITH_POST_MODIFY`, `REJECT_WITH_REASON`) y atributos de telemetría; errores deben propagar `ErrorCode_ERROR_CODE_INVALID_STOPS`.[echo/docs/rfcs/RFC-architecture.md#8-2-trade-proto]
- `agent/capabilities/ModificationDispatcher`: contrato de envío idempotente al EA con confirmación, reportando `ExecutionResult` y reutilizando `ErrorCode_ERROR_CODE_TIMEOUT` cuando expira el presupuesto de 20 ms, complementado por logs/metrics/spans (`echo_agent_post_modify_timeouts_total`, `result=timeout`) para análisis operativo.[echo/docs/rfcs/RFC-architecture.md#3-2-componentes-principales][echo/docs/00-contexto-general.md#metricas-clave-y-slo]
- `sdk/domain/AdjustableStops`: estructura declarativa compartida respaldada por el siguiente esquema en `common.proto`:
  ```protobuf
  message AdjustableStops {
    sint32 sl_offset_points = 1;
    sint32 tp_offset_points = 2;
    bool stop_level_breach = 3;
    string reason = 4;
  }
  ```
  Este contrato se refleja en el SDK Go y en los EAs (MQL) mediante estructuras homólogas, asegurando compatibilidad Core↔Agent↔EA.[echo/docs/rfcs/RFC-architecture.md#8-1-common-proto]
- Las versiones mínimas de Core/Agent/EA se fijan mediante `protocol_version` ≥8 para garantizar que todos los binarios entiendan `AdjustableStops` antes de habilitar la bandera del feature.[echo/docs/rfcs/RFC-architecture.md#6-funcionalidades-clave-v1]

### Configuración de offsets SL/TP
- Origen de la configuración: claves en ETCD bajo `/echo/core/policies/{account_id}/{symbol}/sl_offset_points` y `/echo/core/policies/{account_id}/{symbol}/tp_offset_points`, con valores `sint32` en puntos consumidos por `RiskPolicyService` al hidratar políticas y con watches para cambios en caliente.[echo/docs/rfcs/RFC-architecture.md#2-principios-arquitectonicos]
- Cuando el valor no existe, se aplica offset `0`, preservando el comportamiento actual y registrando en Postgres los valores efectivos tras cada ejecución para auditoría y resiliencia.[echo/docs/00-contexto-general.md#expectativas-de-calidad]

### Lógica determinística de `StopLevelGuard`
1. Calcula `sl_gap` y `tp_gap` entre precio actual y niveles requeridos; descuenta offsets configurados (`sl_offset_points`, `tp_offset_points`) para obtener distancias efectivas.[echo/docs/rfcs/RFC-architecture.md#5-2-logica-v1-segun-prd]
2. Si `sl_required` o `tp_required` y la distancia efectiva queda ≤`stop_level_points`, retorna `REJECT_WITH_REASON` con `ErrorCode_ERROR_CODE_INVALID_STOPS` y `reason="stop_level_pre_fill"`, dejando rastros en telemetría.[echo/docs/00-contexto-general.md#problemas-tipicos-del-dominio-y-patrones-de-solucion]
3. Si ambas distancias exceden `stop_level_points`, emite `ACCEPT_WITH_OFFSETS`, persiste `stop_level_breach=false` y envía el comando original al Agent.[echo/docs/rfcs/RFC-architecture.md#4-3-core]
4. Si alguna distancia viola StopLevel pero el nivel no es obligatorio, emite `ACCEPT_WITH_POST_MODIFY`, encola `ModifyStops` con offsets originales, marca `stop_level_breach=true` y registra `post_modify_attempts` para auditoría.[echo/docs/rfcs/RFC-architecture.md#4-3-core]

### Scheduler del `AdjustmentQueue`
- Tamaño de cola por `trade_id`: 1 elemento; nuevos `ModifyStops` reemplazan al pendiente manteniendo idempotencia por `command_id` y evitando backpressure excesivo.[echo/docs/rfcs/RFC-architecture.md#4-2-agent]
- Retries: hasta 3 intentos (0 ms, ≤10 ms y ≤20 ms con jitter ≤5 ms) respetando el presupuesto total de 20 ms; al agotarse, el dispatcher reutiliza `ErrorCode_ERROR_CODE_TIMEOUT`, incrementa `post_modify_attempts=3` y deja constancia mediante logs estructurados y métricas (`echo_agent_post_modify_timeouts_total`).[echo/docs/00-contexto-general.md#6-conexiones-inestables-y-backpressure]
- Tiempos límite y métricas: si el EA devuelve `ERROR_CODE_INVALID_STOPS` o se supera el umbral, se incrementa `echo_agent_post_modify_timeouts_total`, se generan spans con `result=timeout` y se informa a Operaciones para intervención manual.[echo/docs/00-contexto-general.md#metricas-clave-y-slo]

### Integración con el orquestador existente
- El router (`core/internal/router.go`) mantiene la secuencia `RiskPolicyService` → `StopLevelGuard` → `CommandBuilder` → `AgentDispatcher`, sin crear nuevos paquetes en i8; un ADR separado definirá la posible migración futura a `core/orchestrator/executeorder` para reducir deuda técnica.[echo/docs/rfcs/RFC-architecture.md#4-3-core]
- `CommandBuilder` actual se amplía para aceptar `AdjustableStops` opcional; en caso de `ACCEPT_WITH_POST_MODIFY`, adjunta los offsets originales en el payload enviado al Agent para programar `ModifyStops` posteriores sin romper el esquema proto de `ExecuteOrder`.[echo/docs/rfcs/RFC-architecture.md#8-2-trade-proto]

### Persistencia / esquema
- `executions` y `trades` incluirán columnas `sl_offset_points`, `tp_offset_points`, `stop_level_breach` (bool) y `post_modify_attempts` (int) para auditoría y análisis de performance.[echo/docs/rfcs/RFC-architecture.md#4-3-core]
- No se alteran claves ni índices existentes; se agregan con valores por defecto y migraciones idempotentes para mantener compatibilidad.[echo/docs/00-contexto-general.md#expectativas-de-calidad]

### Observabilidad (logs estructurados, métricas, spans)
- Logs JSON con campos `{app, iter, comp, op, corr_id, span_id, sev, msg}` y metadata de offsets aplicados, asegurando consistencia con los bundles definidos.[echo/docs/rfcs/RFC-architecture.md#7-5-observabilidad]
- Métricas `echo_core_stop_level_rejections_total`, `echo_core_offset_apply_duration_ms`, `echo_agent_post_modify_retries_total`, `echo_agent_post_modify_timeouts_total` y `echo_slave_modify_success_ratio` con atributos `strategy_id`, `account_id`, `symbol` y `result` para seguimiento de desempeño.[echo/docs/00-contexto-general.md#metricas-clave-y-slo]
- Spans `core.stop_level.guard`, `agent.modify.dispatch`, `ea.modify.apply` con atributos `result`, `error_code`, `offset_mode`; se cierran siempre y reportan errores con `telemetry.RecordError`.[echo/docs/00-contexto-general.md#metricas-clave-y-slo]

## Decisiones (ADR breves) y alternativas descartadas
- Se mantiene la lógica de offsets en Core para garantizar single source of truth, descartando cálculo local en EAs por riesgo de divergencias y falta de configurabilidad central.[echo/docs/rfcs/RFC-architecture.md#3-2-componentes-principales]
- Se decide modificar post-fill mediante comandos explícitos en vez de reemitir la orden completa, para respetar idempotencia y reducir latencia.[echo/docs/rfcs/RFC-architecture.md#5-2-logica-v1-segun-prd]
- Se descarta introducir colas externas; la secuenciación interna del Agent es suficiente y evita nuevos puntos de fallo.[echo/docs/rfcs/RFC-architecture.md#3-arquitectura-de-componentes]

## Riesgos, límites, SLOs y capacidad (KPIs + umbrales)
- Riesgo de latencia adicional por modificaciones; se mantiene el SLO intra-host total p95 ≤100 ms reservando ≤20 ms del presupuesto para ajustes, con alertas cuando `echo_core_offset_apply_duration_ms` exceda ese límite y cuando `echo_agent_post_modify_timeouts_total` indique agotamiento del presupuesto de 20 ms.[echo/docs/rfcs/RFC-architecture.md#1-resumen-ejecutivo]
- Brecha en cobertura de StopLevel por cambios del broker; se programan validaciones periódicas y alarmas cuando `stop_level_breach` supere 0.3% semanal.[echo/docs/00-contexto-general.md#problemas-tipicos-del-dominio-y-patrones-de-solucion]
- Uso de CPU del Agent bajo control: el `AdjustmentQueue` aprovecha workers existentes por `trade_id`, manteniendo escalabilidad horizontal.[echo/docs/rfcs/RFC-architecture.md#3-2-componentes-principales]

## Criterios de aceptación (Given-When-Then)
- **Given** una orden BUY con SL/TP requeridos, **When** el broker acepta los niveles, **Then** la ejecución del slave replica offsets exactos y la telemetría marca `result=success`.[echo/docs/rfcs/RFC-architecture.md#5-2-logica-v1-segun-prd]
- **Given** un StopLevel mayor al offset deseado, **When** el broker rechaza la orden con `INVALID_STOPS`, **Then** Core registra `stop_level_breach=true`, reemite `ModifyStops` post-fill y la orden queda sincronizada.[echo/docs/00-contexto-general.md#problemas-tipicos-del-dominio-y-patrones-de-solucion]
- **Given** un fallback de modificación, **When** se repite tres veces sin confirmación, **Then** se marca `post_modify_attempts=3`, se alerta y se apalanca el plan de rollback manteniendo el presupuesto de backoff descrito.[echo/docs/00-contexto-general.md#6-conexiones-inestables-y-backpressure]

## Plan de rollout, BWC, idempotencia y rollback
- Migraciones y despliegues se realizan en azul/verde: primero Core, luego Agent, finalmente actualización de EA con bandera para aceptar `ModifyStops`; la versión anterior ignora el nuevo mensaje preservando BWC.[echo/docs/rfcs/RFC-architecture.md#6-funcionalidades-clave-v1]
- Idempotencia asegurada por `command_id` y ledger existente; se agregan pruebas de repetición de comandos de modificación.[echo/docs/rfcs/RFC-architecture.md#6-funcionalidades-clave-v1]
- Rollback: desactivar feature flag `enable_stop_level_guard` en Core y revertir migraciones manteniendo columnas como opcionales; Agents vuelven a ignorar `ModifyStops` sin afectar ejecuciones base.[echo/docs/rfcs/RFC-architecture.md#3-arquitectura-de-componentes]
- El rollout se condiciona a `protocol_version` negociada en el handshake, bloqueando la activación cuando algún componente no soporte `AdjustableStops`.[echo/docs/rfcs/RFC-architecture.md#6-funcionalidades-clave-v1]
- La configuración ETCD usa `WithPrefix("/echo/core/policies")` y se valida en pipelines antes de habilitar la bandera en producción para evitar drift entre hosts.[echo/docs/rfcs/RFC-architecture.md#2-principios-arquitectonicos]

## Matriz PR-*
| PR-* | Evidencia | Impacto |
|---|---|---|
| PR-ROB | [echo/docs/00-contexto-general.md#problemas-tipicos-del-dominio-y-patrones-de-solucion] | MAY |
| PR-MOD | [echo/docs/rfcs/RFC-architecture.md#3-2-componentes-principales] | MAY |
| PR-ESC | [echo/docs/rfcs/RFC-architecture.md#3-arquitectura-de-componentes] | MEN |
| PR-CLN | [echo/docs/rfcs/RFC-architecture.md#2-principios-arquitectonicos] | MEN |
| PR-SOLID | [echo/docs/rfcs/RFC-architecture.md#2-principios-arquitectonicos] | MEN |
| PR-KISS | [echo/docs/rfcs/RFC-architecture.md#5-2-logica-v1-segun-prd] | MEN |
| PR-OBS | [echo/docs/rfcs/RFC-architecture.md#7-5-observabilidad] | MAY |
| PR-BWC | [echo/docs/rfcs/RFC-architecture.md#6-funcionalidades-clave-v1] | MAY |
| PR-IDEMP | [echo/docs/rfcs/RFC-architecture.md#6-funcionalidades-clave-v1] | MAY |
| PR-RES | [echo/docs/00-contexto-general.md#que-hace-unico-a-echo-vision-de-clase-mundial] | MEN |
| PR-SEC | [echo/docs/00-contexto-general.md#expectativas-de-calidad] | MEN |
| PR-PERF | [echo/docs/rfcs/RFC-architecture.md#1-resumen-ejecutivo] | MAY |

## Refs cargadas
- echo/docs/00-contexto-general.md — "---"
- echo/docs/01-arquitectura-y-roadmap.md — "---"
- echo/docs/rfcs/RFC-architecture.md — "---"
- echo/vibe-coding/prompts/common-principles.md — "**PR-ROB** Robustez: tolerancia a fallos, timeouts, reintentos, backoff; sin afectar integridad de datos."

## Referencias adicionales
- N/A

## NEED-INFO
- No aplica
