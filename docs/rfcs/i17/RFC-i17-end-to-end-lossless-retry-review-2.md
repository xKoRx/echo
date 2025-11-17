# Revisión RFC i17 — end-to-end-lossless-retry — Iter 2

- **Resumen de auditoría**
  - Revisé `echo/docs/rfcs/i17/RFC-i17-end-to-end-lossless-retry.md` y el `REPLY` asociado contra las bases obligatorias (`00-contexto`, `01-arquitectura`, `RFC-architecture`, `common-principles`) y la plantilla oficial.[echo/docs/00-contexto-general.md#echo--contexto-general-del-copiador-de-operaciones][echo/docs/01-arquitectura-y-roadmap.md#proxima-iteracion-i17--garantias-end-to-end-de-replicacion][echo/docs/rfcs/RFC-architecture.md#rfc-001--arquitectura-de-echo-v1][echo/vibe-coding/prompts/common-principles.md#pr-rob-robustez-tolerancia-a-fallos-timeouts-reintentos-backoff-sin-afectar-integridad-de-datos][echo/docs/templates/rfc.md#1-resumen-ejecutivo]
  - El RFC incorpora los cambios solicitados (BoltDB definido, backoff explícito, GWT y plan BWC), pero aún deja huecos contractuales para la negociación `supports_lossless_delivery` y para la configuración del buffer Master. Nivel de madurez: **Observado** (Bloq pendientes por contratos).

- **Matriz de conformidad por requisito**

| Requisito (DoD) | Evidencia | Estado | Dev/QA Ready |
| --- | --- | --- | --- |
| Ledger persistente Core↔Agent↔EA | [echo/docs/rfcs/i17/RFC-i17-end-to-end-lossless-retry.md#62-modelo-de-datos-y-esquema] | OK | SI |
| Retries obligatorios con backoff homogéneo | [echo/docs/rfcs/i17/RFC-i17-end-to-end-lossless-retry.md#3-objetivos-medibles-definition-of-done][echo/docs/rfcs/i17/RFC-i17-end-to-end-lossless-retry.md#63-configuración-flags-y-parámetros] | OK | SI |
| Observabilidad (métricas/logs/spans) | [echo/docs/rfcs/i17/RFC-i17-end-to-end-lossless-retry.md#8-observabilidad-logs-métricas-trazas] | OK | SI |
| Compatibilidad y modo mixto | [echo/docs/rfcs/i17/RFC-i17-end-to-end-lossless-retry.md#102-backward-compatibility] | OBS (no se especifica contrato del flag `supports_lossless_delivery`) | NO |
| Testing Dev & QA (Given-When-Then) | [echo/docs/rfcs/i17/RFC-i17-end-to-end-lossless-retry.md#91-casos-de-uso-e2e] | OK | SI |

- **Cobertura PR-***

| PR-* | Evidencia | Estado | Comentario |
| --- | --- | --- | --- |
| PR-ROB / PR-RES | [echo/docs/rfcs/i17/RFC-i17-end-to-end-lossless-retry.md#5-arquitectura-de-solución][echo/docs/rfcs/i17/RFC-i17-end-to-end-lossless-retry.md#62-modelo-de-datos-y-esquema] | OBS | Falta definir `master_retry_backoff_ms`, por lo que el hop Master→Agent sigue sin parámetros reproducibles. |
| PR-MOD | [echo/docs/rfcs/i17/RFC-i17-end-to-end-lossless-retry.md#52-componentes-afectados-touchpoints] | OK | Límites de paquete claros. |
| PR-BWC | [echo/docs/rfcs/i17/RFC-i17-end-to-end-lossless-retry.md#102-backward-compatibility] | OBS | Handshake cita `supports_lossless_delivery` pero no se define en contratos proto. |
| PR-OBS | [echo/docs/rfcs/i17/RFC-i17-end-to-end-lossless-retry.md#8-observabilidad-logs-métricas-trazas] | OBS | La métrica `echo_core.delivery.compat_mode_total` mencionada en BWC no figura con tipo/labels en la tabla de métricas. |
| PR-IDEMP | [echo/docs/rfcs/i17/RFC-i17-end-to-end-lossless-retry.md#54-reglas-de-negocio-y-casos-borde] | OK | Estados `pending/inflight/acked/failed` siguen siendo deterministas. |
| PR-PERF | [echo/docs/rfcs/i17/RFC-i17-end-to-end-lossless-retry.md#71-principios-de-diseño-y-trade-offs] | OK | Se documenta el trade-off de latencia al priorizar consistencia. |

- **Hallazgos**
  - **H5**
    - Tipo: GAP-DEV
    - Severidad: BLOQ
    - PR-*: PR-BWC, PR-MOD
    - Evidencia: [echo/docs/rfcs/i17/RFC-i17-end-to-end-lossless-retry.md#102-backward-compatibility] introduce el flag `supports_lossless_delivery` en el handshake gRPC y la métrica `compat_mode_total`, pero `§6.1 Mensajes / contratos` no define campos o mensajes donde viaja ese flag ni enumera la métrica requerida.[echo/docs/rfcs/i17/RFC-i17-end-to-end-lossless-retry.md#61-mensajes--contratos][echo/docs/templates/rfc.md#61-mensajes--contratos-sdk-grpc-eas-apis]
    - Impacto: Dev no puede implementar la negociación (¿nuevo campo en `DeliveryHeartbeat`, `AgentHello`, `CommandAck`?) ni QA validar la transición, por lo que el modo compat queda en el aire y viola PR-BWC/Dev-Ready.
    - Propuesta: Extender `agent.proto`/`core.proto` con el mensaje exacto (nombre del RPC, campo `supports_lossless_delivery bool`, versión mínima, comportamiento cuando es `false`) y registrar la métrica `echo_core.delivery.compat_mode_total` en Observabilidad con tipo/labels.
    - Trade-offs: Formalizar el contrato puede requerir regenerar protos pero destraba implementación y evita regresiones durante upgrade.
  - **H6**
    - Tipo: GAP-DEV
    - Severidad: MAY
    - PR-*: PR-ROB, PR-RES
    - Evidencia: En [§5.1] se define que el Master reintenta cada `master_retry_backoff_ms`, pero `§6.3 Configuración` no especifica ese parámetro ni si reutiliza `/echo/core/delivery/retry_backoff_ms`.[echo/docs/rfcs/i17/RFC-i17-end-to-end-lossless-retry.md#51-visión-general][echo/docs/rfcs/i17/RFC-i17-end-to-end-lossless-retry.md#63-configuración-flags-y-parámetros]
    - Impacto: QA y Dev no pueden alinear la cadencia del buffer Master ni garantizar los 100 intentos requeridos; se pierde trazabilidad en el primer hop (Master→Agent), debilitando PR-ROB.
    - Propuesta: Documentar explícitamente `master_retry_backoff_ms` (clave ETCD o parámetro local), su default, validaciones y relación con `retry_backoff_ms`. Si es el mismo vector, aclararlo y describir cómo el EA accede al valor.
    - Trade-offs: Reutilizar el vector existente simplifica configuración pero debe declararse para evitar duplicidad; un parámetro separado otorga flexibilidad a costa de otra clave en ETCD/Postgres.
  - **H7**
    - Tipo: OBS
    - Severidad: MEN
    - PR-*: PR-OBS
    - Evidencia: `§10.2` introduce la métrica `echo_core.delivery.compat_mode_total{agent_id}`, pero `§8.1 Métricas` no la lista con tipo ni labels, incumpliendo la plantilla de observabilidad.[echo/docs/rfcs/i17/RFC-i17-end-to-end-lossless-retry.md#81-métricas][echo/docs/rfcs/i17/RFC-i17-end-to-end-lossless-retry.md#102-backward-compatibility]
    - Impacto: Sin especificación formal, Dev/QA no sabrán cómo instrumentar ni consultar la métrica que habilita el monitoreo del rollout. Riesgo menor pero afecta trazabilidad del modo compat.
    - Propuesta: Añadir la métrica a la tabla (tipo counter, labels `agent_id`, `protocol_version`, semántica “número de conexiones legacy activas”).
    - Trade-offs: Solo implica ampliar la tabla; sin ello la documentación queda inconsistente.

- **Gaps de implementabilidad para Dev/QA (GAP-DEV)**
  - `§10.2 Backward compatibility`: Falta especificar en `proto`/contratos el campo `supports_lossless_delivery`, su tipo y mensajes implicados, bloqueando el handshake requerido para la convivencia (H5).
  - `§5.1 / §6.3`: El parámetro `master_retry_backoff_ms` no tiene clave/configuración definida ni relación explícita con el vector global, impidiendo que Dev/QA reproduzcan los 100 retries en el hop Master→Agent (H6).
  - **Conclusión**: Mientras el contrato de negociación y la configuración del primer hop sigan sin definir, no se puede dar handoff a Dev/QA.

- **Citas faltantes / Suposiciones**
  - No se detectaron citas inválidas adicionales; los huecos se deben a detalles no documentados, no a referencias faltantes.

- **Cambios sugeridos (diff textual conceptual)**

```diff
+# 6.1 Mensajes / contratos
+- Extender `agent.proto` con `message AgentHello { uint32 protocol_version = 1; bool supports_lossless_delivery = 2; }` y documentar el flujo Core↔Agent.
+- Detallar qué acciones toma el Core al recibir `supports_lossless_delivery=false`.
+
+# 6.3 Configuración
+- Añadir clave `master_retry_backoff_ms` (o aclarar explícitamente que el Master usa `/echo/core/delivery/retry_backoff_ms`).
+- Incluir default, validaciones y forma en que el EA accede al valor.
+
+# 8.1 Métricas
+- Agregar fila para `echo_core.delivery.compat_mode_total` (counter, labels `agent_id`, `protocol_version`, semántica "conexiones legacy aún activas").
```

- **Evaluación de riesgos**
  - **R-BWC**: Sin contrato formal del handshake, los agentes legacy podrían comportarse de forma inconsistent e incluso bloquear despliegues; riesgo alto durante la ventana de upgrade.
  - **R-ROB**: No definir la cadencia `master_retry_backoff_ms` deja el primer hop sin garantías de 100 retries, reintroduciendo el riesgo de pérdida de intents.
  - **R-OBS**: Falta de especificación de la métrica `compat_mode_total` impide detectar cuánto dura el modo compat y dificulta rollback controlado.

- **Decisión**
  - `decision: OBSERVADO`
  - Condiciones de cierre: Documentar el contrato `supports_lossless_delivery` (mensajes, campos, manejo) y la configuración `master_retry_backoff_ms`; actualizar observabilidad con la métrica de compatibilidad.

- **Refs cargadas**
  - `echo/docs/rfcs/i17/RFC-i17-end-to-end-lossless-retry.md` — "---"
  - `echo/docs/rfcs/i17/REPLY-i17-to-review-1.md` — "# REPLY i17 → Review 1"
  - `echo/docs/00-contexto-general.md` — "---"
  - `echo/docs/01-arquitectura-y-roadmap.md` — "---"
  - `echo/docs/rfcs/RFC-architecture.md` — "---"
  - `echo/vibe-coding/prompts/common-principles.md` — "**PR-ROB** Robustez: tolerancia a fallos, timeouts, reintentos, backoff; sin afectar integridad de datos."
  - `echo/docs/templates/rfc.md` — "---"
