# Revisión RFC i17 — end-to-end-lossless-retry — Iter 1

- **Resumen de auditoría**
  - Revisé `echo/docs/rfcs/i17/RFC-i17-end-to-end-lossless-retry.md` contra las bases obligatorias (`00-contexto`, `01-arquitectura`, `RFC-architecture`, `common-principles`) y el template oficial.[echo/docs/00-contexto-general.md#echo--contexto-general-del-copiador-de-operaciones][echo/docs/01-arquitectura-y-roadmap.md#próxima-iteración-i17--garantías-end-to-end-de-replicación][echo/docs/rfcs/RFC-architecture.md#rfc-001--arquitectura-de-echo-v1][echo/vibe-coding/prompts/common-principles.md#pr-rob-robustez-tolerancia-a-fallos-timeouts-reintentos-backoff-sin-afectar-integridad-de-datos][echo/docs/templates/rfc.md#1-resumen-ejecutivo]
  - El RFC está detallado en arquitectura y observabilidad, pero mantiene ambigüedades clave (almacenamiento del ack ledger, secuencia completa de backoff, criterios de QA y compatibilidad operacional). El nivel de madurez queda en **Observado**: requiere ajustes antes del handoff Dev/QA.

- **Matriz de conformidad por requisito**

| Requisito (DoD) | Evidencia | Estado | Dev/QA Ready |
| --- | --- | --- | --- |
| Ledger persistente Core↔Agent↔EA | [echo/docs/rfcs/i17/RFC-i17-end-to-end-lossless-retry.md#3-objetivos-medibles-definition-of-done][echo/docs/rfcs/i17/RFC-i17-end-to-end-lossless-retry.md#62-modelo-de-datos-y-esquema] | OBS (falta definir almacenamiento del ack ledger) | NO |
| Retries obligatorios con backoff homogéneo | [echo/docs/rfcs/i17/RFC-i17-end-to-end-lossless-retry.md#3-objetivos-medibles-definition-of-done][echo/docs/rfcs/i17/RFC-i17-end-to-end-lossless-retry.md#63-configuración-flags-y-parámetros] | FALLA (no se especifica la secuencia completa de `retry_backoff_ms` ni su codificación en ETCD) | NO |
| Observabilidad (métricas/logs/spans) | [echo/docs/rfcs/i17/RFC-i17-end-to-end-lossless-retry.md#8-observabilidad-logs-métricas-trazas] | OK | SI |
| Compatibilidad/BWC | [echo/docs/rfcs/i17/RFC-i17-end-to-end-lossless-retry.md#3-objetivos-medibles-definition-of-done][echo/docs/rfcs/i17/RFC-i17-end-to-end-lossless-retry.md#102-backward-compatibility] | FALLA (no permite operación mixta ni plan detallado de negociación de protocolo) | NO |
| Testing Dev & QA | [echo/docs/rfcs/i17/RFC-i17-end-to-end-lossless-retry.md#9-plan-de-pruebas-dev-y-qa] | OBS (faltan criterios de aceptación y Given-When-Then verificables) | NO |

- **Cobertura PR-***

| PR-* | Evidencia | Estado | Comentario |
| --- | --- | --- | --- |
| PR-ROB / PR-RES | [echo/docs/rfcs/i17/RFC-i17-end-to-end-lossless-retry.md#5-arquitectura-de-solución][echo/docs/rfcs/i17/RFC-i17-end-to-end-lossless-retry.md#62-modelo-de-datos-y-esquema] | OBS | El ledger cubre los hops pero falta definir storage y política de reapertura para garantizar resiliencia práctica. |
| PR-MOD | [echo/docs/rfcs/i17/RFC-i17-end-to-end-lossless-retry.md#52-componentes-afectados-touchpoints] | OK | Los componentes afectados están delineados y siguen límites de módulo. |
| PR-BWC | [echo/docs/rfcs/i17/RFC-i17-end-to-end-lossless-retry.md#102-backward-compatibility][echo/vibe-coding/prompts/common-principles.md#pr-bwc-compatibilidad-hacia-atrás-cambios-no-rompen-contratos-públicos-sin-plan] | FALLA | Obliga big bang y declara que versiones legacy solo pueden vivir en lab sin describir negociación ni fallback operativo. |
| PR-IDEMP | [echo/docs/rfcs/i17/RFC-i17-end-to-end-lossless-retry.md#54-reglas-de-negocio-y-casos-borde] | OK | El estado `pending/inflight/acked/failed` evita duplicados apoyándose en UUIDv7. |
| PR-OBS | [echo/docs/rfcs/i17/RFC-i17-end-to-end-lossless-retry.md#8-observabilidad-logs-métricas-trazas] | OK | Se definen métricas, logs (campos obligatorios) y spans alineados al stack. |
| PR-PERF | [echo/docs/rfcs/i17/RFC-i17-end-to-end-lossless-retry.md#71-principios-de-diseño-y-trade-offs] | OBS | Se reconoce el costo en latencia pero no se cuantifica impacto p95/p99 ni estrategia de medición más allá del backlog. |
| PR-SEC | [echo/docs/rfcs/i17/RFC-i17-end-to-end-lossless-retry.md#71-principios-de-diseño-y-trade-offs] | INFO | No hay cambios de superficie, pero tampoco se detalla control de acceso al nuevo ledger o acks. |

- **Hallazgos**
  - **H1**
    - Tipo: GAP-DEV
    - Severidad: BLOQ
    - PR-*: PR-ROB, PR-MOD
    - Evidencia: [echo/docs/rfcs/i17/RFC-i17-end-to-end-lossless-retry.md#62-modelo-de-datos-y-esquema] indica “BoltDB o SQLite embedded” sin decisión; el template exige especificar store, columnas y estrategia.[echo/docs/templates/rfc.md#62-modelo-de-datos-y-esquema]
    - Impacto: Dev no puede implementar `agent_ack_ledger` ni planificar rutas de despliegue/operación si desconoce engine, formato on-disk, rotación y cómo se replica entre memoria y disco; QA tampoco puede preparar escenarios de crash/restart.
    - Propuesta: Documentar una única tecnología (p.ej. BoltDB), path por cuenta, layout (bucket, claves, TTL), política de fsync/flush y cómo se sincroniza con el ledger en memoria.
    - Trade-offs: Elegir BoltDB simplifica despliegue (sin dependencias externas) pero requiere justificar locking y tamaño; SQLite daría SQL rico pero aumenta footprint. El RFC debe dejar fijado el costo elegido.
  - **H2**
    - Tipo: GAP-DEV
    - Severidad: BLOQ
    - PR-*: PR-ROB, PR-RES
    - Evidencia: [echo/docs/rfcs/i17/RFC-i17-end-to-end-lossless-retry.md#63-configuración-flags-y-parámetros] menciona `retry_backoff_ms` como “list: 50,100,200,...” sin definir longitud, progresión ni codificación en ETCD, contrariando la exigencia de detallar flags.[echo/docs/templates/rfc.md#63-configuración-flags-y-parámetros]
    - Impacto: Core, Agent y EAs no pueden compartir exactamente la misma curva de backoff, por lo que los intentos podrían divergir. QA no puede verificar los 100 retries ni calcular tiempos máximos antes del failover.
    - Propuesta: Documentar explícitamente la secuencia completa (p.ej. vector de 10 valores + multiplicador), cómo se serializa (JSON array/string), reglas de extensión para llegar a 100 intentos y límites máximos.
    - Trade-offs: Una secuencia fija simplifica verificación pero reduce flexibilidad; permitir overrides requiere describir validaciones para evitar explosión de latencias.
  - **H3**
    - Tipo: ARQ
    - Severidad: MAY
    - PR-*: PR-BWC
    - Evidencia: La sección de BWC declara que solo `protocol_version>=3` es aceptado en producción y que los agentes viejos serán confinados a laboratorio, sin detallar planes de convivencia ni fallback.[echo/docs/rfcs/i17/RFC-i17-end-to-end-lossless-retry.md#102-backward-compatibility] Esto contradice PR-BWC.[echo/vibe-coding/prompts/common-principles.md#pr-bwc-compatibilidad-hacia-atrás-cambios-no-rompen-contratos-públicos-sin-plan]
    - Impacto: El rollout requiere actualizar simultáneamente SDK, Agent, Core y EAs; cualquier host retrasado rompe el flujo y no existe estrategia para aislarlo. Riesgo alto de downtime durante producción.
    - Propuesta: Documentar handshake de negociación (`protocol_version`) y comportamiento fallback (p.ej. modo journal-off pero con reconcilia manual) hasta completar el upgrade, junto con pasos operativos para detectar clientes legacy.
    - Trade-offs: Mantener fallback aumenta complejidad temporal pero reduce riesgo de corte; exigir upgrade inmediato acelera delivery pero necesita ventana controlada y plan aprobado.
  - **H4**
    - Tipo: GAP-DEV
    - Severidad: MAY
    - PR-*: PR-ROB, PR-OBS
    - Evidencia: El plan de pruebas enumera escenarios E2E pero no define criterios de aceptación ni Given-When-Then, incumpliendo el template que debe guiar a QA para construir tests trazables.[echo/docs/rfcs/i17/RFC-i17-end-to-end-lossless-retry.md#9-plan-de-pruebas-dev-y-qa][echo/docs/templates/rfc.md#9-plan-de-pruebas-dev-y-qa]
    - Impacto: QA no puede derivar pasos concretos (datos iniciales, triggers, verificaciones objetivas) ni mapear DoD a casos; aumenta el riesgo de interpretaciones distintas entre equipos.
    - Propuesta: Añadir para cada E2E un bloque Given-When-Then con inputs concretos (p.ej. `worker_timeout_ms=1ms`, `trade_id=UUIDv7`), condiciones de éxito y métricas esperadas (`pending_age_ms`).
    - Trade-offs: documentar GWT requiere más tiempo ahora pero reduce el costo de debugging y asegura repetibilidad en regression.

- **Gaps de implementabilidad para Dev/QA (GAP-DEV)**
  - `### 6.2 Modelo de datos y esquema`: falta definir engine, layout y política operacional concreta para `agent_ack_ledger`; sin eso Dev no puede codificar persistencia ni QA reproducir crashes.
  - `### 6.3 Configuración, flags y parámetros`: la clave `/echo/core/delivery/retry_backoff_ms` no especifica lista completa ni encoding, bloqueando la implementación uniforme del backoff y los casos de prueba de 100 intentos.
  - `### 9 Plan de pruebas`: no existen criterios de aceptación formales; QA no puede iniciar validaciones sin inventar pasos.
  - **Conclusión**: No es posible arrancar construcción ni pruebas sin completar los puntos anteriores.

- **Citas faltantes / Suposiciones**
  - No se detectaron citas inválidas; todas las afirmaciones relevantes referencian docs del repo.

- **Cambios sugeridos (diff conceptual)**

```diff
+# 6.2 Agent ack ledger
+- Adoptar BoltDB como store embebido único.
+- Describir bucket `acks/<agent_id>/<command_id>` con campos `stage`, `attempt`, `last_error`, `next_retry_at`, `updated_at`.
+- Precisar fsync en cada transición crítica y política de limpieza >24h.
+
+# 6.3 Configuración de backoff
+- Definir `retry_backoff_ms` como JSON array `[50,100,200,400,800,1600,3200,6400,12800,25600]` y
+  aclarar que al rebasar la lista se recicla el último valor hasta completar 100 intentos.
+- Documentar validaciones (solo valores positivos, longitud mínima 5).
+
+# 9 Plan de pruebas
+- Añadir tabla Given-When-Then por cada E2E (inputs, acciones, resultados observables
+  incluyendo métricas `echo_*` y estados del ledger).
```

- **Evaluación de riesgos**
  - **R-BWC**: El rollout big bang sin fallback puede dejar cuentas desconectadas si algún Agent/EAs no se actualiza a tiempo; riesgo alto durante ventanas nocturnas. Añadir detección y aislamiento reduce impacto.
  - **R-Config**: Una definición incompleta del backoff expone a latencias impredecibles y a timeouts antes del ack, degradando PR-ROB.
  - **R-QA**: Sin criterios de aceptación, la verificación manual puede pasar por alto escenarios críticos (p.ej. reconexión tras crash), incrementando probabilidad de regresiones silenciosas.

- **Decisión**
  - `decision: RECHAZADO`
  - Condiciones de cierre: resolver H1 y H2 (GAP-DEV BLOQ) y completar criterios de QA (H4) antes del siguiente ciclo; detallar plan BWC según H3 para reducir severidad.

- **Refs cargadas**
  - `echo/docs/rfcs/i17/RFC-i17-end-to-end-lossless-retry.md` — "---"
  - `echo/docs/00-contexto-general.md` — "---"
  - `echo/docs/01-arquitectura-y-roadmap.md` — "---"
  - `echo/docs/rfcs/RFC-architecture.md` — "---"
  - `echo/vibe-coding/prompts/common-principles.md` — "**PR-ROB** Robustez: tolerancia a fallos, timeouts, reintentos, backoff; sin afectar integridad de datos."
  - `echo/docs/templates/rfc.md` — "---"
