# Code Review i8ab — sl-tp-offset-strategy
## Metadatos
- RFC: `echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md`
- Implementación: `echo/echo/docs/rfcs/i8ab/IMPLEMENTATION.md`
- Revisor: Echo_Revisor_Codigo_v1
- Dictamen: Listo
- Fecha: 2025-11-16

## 1. Resumen Ejecutivo
- `computeStopOffsetTargets` ahora clampa cualquier distancia que resulte ≤0 al StopLevel (o a 1 pip si no hay dato) y marca `SLResult/TPResult` como `clamped` (ver `core/internal/router.go` líneas 1623-1687). `adjustStopsAndTargets` consume esas distancias para recalcular los precios finales sobre la cotización del slave, por lo que los offsets aplican incluso cuando se acercan al mínimo permitido (`core/internal/router.go` 884-919).
- El fallback registra métricas/logs por `attempt1|attempt2` y conserva el `command_id` original y el del retry mediante `recordStopOffsetFallback` (`core/internal/router.go` 2093-2120). Además, `retryExecuteOrderWithoutOffsets` emite el span `core.stop_offset.fallback` y persiste la metadata en `CommandContext`.
- Las nuevas pruebas en `core/internal/router_offsets_test.go` ejercitan offsets positivos/negativos, clamps por StopLevel y la actualización del contexto de fallback (`core/internal/router_offsets_test.go` 93-179). Con ellas se captura el bug original.
- CI local: `go test ./sdk/... ./core/...` y `go test ./agent/...` en verde. `golangci-lint` sigue sin ejecutarse porque la herramienta no está instalada en el entorno reportado.

## 2. Matriz de Hallazgos
| ID | Archivo / Recurso | Ítem / Área | Severidad | Evidencia | Sugerencia |
|----|-------------------|-------------|-----------|-----------|------------|
| — | — | — | — | No se identificaron hallazgos pendientes. | — |

## 3. Contratos vs RFC
- Offsets en pips se aplican antes de emitir `ExecuteOrder` y sólo se degradan cuando StopLevel lo exige, cumpliendo §5.3 del RFC. No se observan regresiones de contratos gRPC ni en la estructura de políticas.

## 4. Concurrencia, Errores y Límites
### 4.1 Concurrencia
- El acceso a `CommandContext` sigue protegido por `commandContextMu`; las nuevas propiedades sólo se usan dentro de las secciones críticas.
### 4.2 Manejo de Errores
- El fallback diferencia correctamente `attempt1`/`attempt2` con métricas/logs y siempre libera el contexto cuando no quedan retries pendientes.
### 4.3 Límites y Edge Cases
- Se cubren offsets negativos que pretendan colocar SL/TP por debajo del StopLevel y se asegura un mínimo de 1 pip cuando no se cuenta con datos del broker.

## 5. Observabilidad y Performance
- `stop_offset_applied/distance/edge_rejections/fallback` usan el prefijo `echo_core.*` y reusan los buckets RFC. Los logs `stop_offset_fallback` incluyen `original_command_id` y `retry_command_id`, facilitando la correlación.
- El span `core.stop_offset.fallback` cubre el retry completo, por lo que la latencia adicional queda instrumentada. El costo computacional del nuevo cálculo se mantiene constante (operaciones aritméticas y sin I/O extra).

## 6. Dictamen Final y Checklist
- Dictamen: **Listo**
- Checklist:
  - RFC cubierto sin desviaciones: ✅
  - Build compilable: ✅ (`go test ./sdk/... ./core/...`, `go test ./agent/...`)
  - Tests clave verdes: ✅ (lint pendiente sólo por falta de `golangci-lint` en el entorno)
  - Telemetría mínima requerida presente: ✅
  - Riesgos críticos abordados: ✅
