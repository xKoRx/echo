# Revisión RFC i8(a+b) — sl-tp-offset-stop-level — Iter 2

## Resumen de auditoría
Se verificó la actualización del RFC y el REPLY contra las bases de Echo y la revisión previa. Se enfocó la auditoría en validar la compatibilidad con el SLO p95 ≤100 ms intra-host, la coherencia del plan de reintentos del Agent y la definición del contrato compartido `AdjustableStops`.

## Matriz de conformidad
| Requisito | Evidencia | Estado |
|---|---|---|
| SLO intra-host p95 ≤100 ms sin contradicciones en ejecución | [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#componentes-y-responsabilidades] · [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#riesgos-limites-slos-y-capacidad-kpis--umbrales] · [echo/docs/rfcs/RFC-architecture.md#1-resumen-ejecutivo] | FALLA |
| Retries/backoff consistentes con PR-ROB y presupuesto de latencia | [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#componentes-y-responsabilidades] · [echo/docs/00-contexto-general.md#6-conexiones-inestables-y-backpressure] | FALLA |
| Contrato `AdjustableStops` definido con esquema proto y tipos claros | [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#interfaces-publicas-io-errores-contratos] · [echo/docs/rfcs/RFC-architecture.md#8-1-common-proto] | OBS |

## Cobertura PR-*
| PR-* | Evidencia | Estado | Comentario |
|---|---|---|---|
| PR-PERF | [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#componentes-y-responsabilidades] · [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#riesgos-limites-slos-y-capacidad-kpis--umbrales] · [echo/docs/rfcs/RFC-architecture.md#1-resumen-ejecutivo] | FALLA | El plan de reintentos 50 ms/100 ms/200 ms supera el presupuesto de 20 ms reservado en el SLO de 100 ms. |
| PR-ROB | [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#componentes-y-responsabilidades] · [echo/docs/00-contexto-general.md#6-conexiones-inestables-y-backpressure] | FALLA | La política de retries definida no puede ejecutarse antes del abandono por presupuesto de latencia, dejando el flujo sin estrategia viable. |
| PR-BWC | [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#interfaces-publicas-io-errores-contratos] · [echo/docs/rfcs/RFC-architecture.md#8-1-common-proto] | OBS | Se listan campos de `AdjustableStops`, pero falta el esquema proto con tipos opcionales/requeridos para Core/Agent/EA. |

## Hallazgos
### H4 — Política de retries incompatible con el SLO
- Severidad: **BLOQ**
- PR-*: PR-PERF, PR-ROB
- Evidencia: [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#componentes-y-responsabilidades] · [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#riesgos-limites-slos-y-capacidad-kpis--umbrales] · [echo/docs/rfcs/RFC-architecture.md#1-resumen-ejecutivo]
- Impacto: El SLO mantiene p95 ≤100 ms con ≤20 ms para ajustes, pero la cola define retries a 50 ms, 100 ms y 200 ms (+ jitter) antes de abortar. El flujo nunca cumpliría el presupuesto, por lo que la lógica queda en estado indefinido (abortos inmediatos o SLO roto).
- Propuesta: Alinear parámetros. Opciones: (a) reducir los retries a ≤10 ms/20 ms y reservar presupuesto acorde, o (b) re-clasificar el SLO separando operaciones post-fill con métricas dedicadas, documentando la tolerancia explícita.
- Trade-offs: Ajustar a 10 ms/20 ms reduce tiempo para recuperarse, mientras que separar métricas implica instrumentación adicional. Ambas mantienen trazabilidad y evitan incumplir el SLO oficial.

### H5 — Contrato `AdjustableStops` sin esquema proto
- Severidad: **MAY**
- PR-*: PR-BWC, PR-MOD
- Evidencia: [echo/docs/rfcs/i8(a+b)/RFC-i8(a+b)-sl-tp-offset-stop-level.md#interfaces-publicas-io-errores-contratos] · [echo/docs/rfcs/RFC-architecture.md#8-1-common-proto]
- Impacto: Aunque se listan campos, no se especifican tipos ni si son opcionales/obligatorios, a diferencia del resto de contratos definidos en `common.proto`. Core, Agent y EA podrían implementar formatos distintos, comprometiendo compatibilidad.
- Propuesta: Incluir el snippet proto completo (`message AdjustableStops { ... }`) con tipos (`int32`, `bool`, `string`, etc.) y marcar opcionales/requeridos. Documentar cómo se serializa en el SDK Go/MQL.
- Trade-offs: Añadir el esquema incrementa el trabajo de documentación pero garantiza implementaciones consistentes y facilita upgrades del SDK.

## Citas faltantes / Suposiciones
- Ninguna.

## Cambios sugeridos (diff textual)
```diff
- **Agent**: añade un `AdjustmentQueue` idempotente [...] con política de 3 reintentos exponenciales (50 ms, 100 ms, 200 ms + jitter ≤20 ms) y abortos controlados si excede el presupuesto de latencia.
+ **Agent**: añade un `AdjustmentQueue` idempotente [...] con política de reintentos ≤10 ms, ≤20 ms (jitter ≤5 ms) y límite duro de 20 ms para preservar el SLO intra-host.
```

```diff
- `sdk/domain/AdjustableStops`: estructura declarativa compartida con campos `sl_offset_points`, `tp_offset_points`, `stop_level_breach` y `reason`, respaldada por `message AdjustableStops` en `common.proto` para mantener compatibilidad Core↔Agent↔EA.
+ `sdk/domain/AdjustableStops`: estructura declarativa compartida respaldada por `message AdjustableStops` en `common.proto`:
+ ```protobuf
+ message AdjustableStops {
+   sint32 sl_offset_points = 1;
+   sint32 tp_offset_points = 2;
+   bool stop_level_breach = 3;
+   string reason = 4;
+ }
+ ```
```

## Evaluación de riesgos
Mientras la política de reintentos permanezca desalineada, el sistema corre riesgo de violar el SLO o abortar ajustes cruciales, afectando sincronización maestro→slave y aumentando missed trades. La falta de esquema proto detallado puede provocar incompatibilidades en despliegues coordinados.

## Decisión
- Estado: **Rechazado** hasta ajustar los parámetros de retries/SLO y documentar el esquema proto de `AdjustableStops`.
- Condiciones de cierre: aplicar correcciones propuestas o justificar alternativa equivalente que mantenga PR-PERF/PR-ROB y PR-BWC.

## Refs cargadas
- `echo/docs/00-contexto-general.md` — `"---"`
- `echo/docs/01-arquitectura-y-roadmap.md` — `"---"`
- `echo/docs/rfcs/RFC-architecture.md` — `"---"`
- `echo/vibe-coding/prompts/common-principles.md` — `"**PR-ROB** Robustez: tolerancia a fallos, timeouts, reintentos, backoff; sin afectar integridad de datos."`

