# Code Review i8ab — sl-tp-offset-strategy
## Metadatos
- RFC: `echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md`
- Implementación: `echo/echo/docs/rfcs/i8ab/IMPLEMENTATION.md`
- Revisor: Echo_Revisor_Codigo_v1
- Dictamen: Cambios mayores
- Fecha: 2025-11-15

## 1. Resumen Ejecutivo
- El pipeline aún ignora offsets negativos de gran magnitud: cuando `distance_pips + offset <= 0` se desecha el ajuste en lugar de clamplearlo al StopLevel, por lo que las estrategias que acercan SL/TP siguen operando con la distancia original (CR-i8ab-01).
- Se agregaron pruebas para helpers, pero siguen faltando casos que validen clamps, telemetría y el fallback attempt1/attempt2 exigido en el plan; el bug anterior pasó sin cobertura (CR-i8ab-02).
- Migración, métricas `echo_core.*`, el span/log `core.stop_offset.fallback` y los buckets solicitados ya están en su sitio; los builds locales y `go test ./sdk/... ./core/... ./agent/...` pasan, aunque `golangci-lint` continúa sin ejecutarse por ausencia de la herramienta en el entorno.

## 2. Matriz de Hallazgos
| ID | Archivo / Recurso | Ítem / Área | Severidad | Evidencia | Sugerencia |
|----|-------------------|-------------|-----------|-----------|------------|
| CR-i8ab-01 | `core/internal/router.go` | Contratos / Lógica offsets | BLOQUEANTE | El RFC define `final_distance_pips = distance_pips + sl_offset_pips` y luego clampa el resultado a `minDistance` antes de calcular el precio ([echo/docs/rfcs/i8ab/RFC-i8ab-sl-tp-offset-strategy.md#53-flujos-principales]). En `computeStopOffsetTargets` cuando la suma es ≤0 simplemente se asigna 0 y se omite el offset (líneas 1623-1644), y `adjustStopsAndTargets` sólo aplica clamps cuando `stats.TargetSLDistancePips > 0` (líneas 881-907), dejando el SL/TP en el valor original del master. | Mantener siempre la distancia resultante: después de sumar el offset, usar `finalDistance = max(minDistance/pip_size, abs(distance+offset))` y marcar `SLResult/TPResult = "clamped"` cuando se eleve. Garantizar que `stats.Target*DistancePips` nunca sea 0 si existe SL/TP y recalcular el precio con el `entryPrice` del slave. Agregar pruebas que cubran BUY/SELL con offsets que intentan colocarse por debajo del StopLevel. |
| CR-i8ab-02 | `core/internal/router_offsets_test.go` | Testing / Cobertura | MAYOR | El plan e Implementation.md piden “casos table-driven para offsets/fallback” (p.ej. §1 Paso 4–6) y el RFC §9.2 agrega escenarios E2E. El archivo de tests sólo cubre helpers y dos casos de `computeStopOffsetTargets` (líneas 12-160); no hay pruebas para clamps efectivos, métricas/logs ni el flujo attempt1/attempt2. Por eso CR-i8ab-01 no se detectó. | Añadir suites que verifiquen: (a) offsets positivos/negativos con distintas combinaciones BUY/SELL y StopLevel; (b) generación de métricas/logs `stop_offset_*`; (c) fallback completo midiendo `stage`/`result` y los `command_id` originales vs retry. Considerar un test de integración del router con quotes simuladas. |

### Fragmentos de evidencia
- **E1** `core/internal/router.go`
```1623:1644:core/internal/router.go
	if intent.StopLoss != nil && *intent.StopLoss != 0 {
		masterDistance := computeStopDistance(intent.Side, intent.Price, *intent.StopLoss)
		if masterDistance > 0 {
			stats.MasterSLDistancePips = masterDistance / stats.PipSize
			stats.TargetSLDistancePips = stats.MasterSLDistancePips + float64(policy.StopLossOffsetPips)
			if stats.TargetSLDistancePips > 0 {
				...
				stats.SLResult = "applied"
			} else {
				stats.TargetSLDistancePips = 0
			}
		}
	}
```
- **E2** `core/internal/router.go`
```881:907:core/internal/router.go
	targetSLDistance := 0.0
	if stats != nil && stats.TargetSLDistancePips > 0 && pipSize > 0 {
		targetSLDistance = stats.TargetSLDistancePips * pipSize
	} else if order.StopLoss != nil {
		targetSLDistance = quoteDistanceToStop(intent.Side, entryPrice, order.GetStopLoss())
	}
	if targetSLDistance > 0 {
		newSL := computeStopPrice(intent.Side, entryPrice, targetSLDistance)
		if minDistance > 0 && targetSLDistance < minDistance {
			newSL = computeStopPrice(intent.Side, entryPrice, minDistance)
			outcome.StopLossClamped = true
```
- **E3** `core/internal/router_offsets_test.go`
```50:160:core/internal/router_offsets_test.go
func TestForceMinDistanceStops(t *testing.T) { ... }
func TestClassifyStopOffsetResult(t *testing.T) { ... }
func TestComputeStopOffsetTargets_BuyOffsets(t *testing.T) { ... }
func TestComputeStopOffsetTargets_InvalidNegativeOffset(t *testing.T) { ... }
```

## 3. Contratos vs RFC
- CR-i8ab-01 viola §5.3–5.4: offsets negativos deberían acercar SL/TP hasta el StopLevel mínimo, no ignorarse. El fallback documentado depende de que el primer intento respete la distancia solicitada.
- El resto de contratos (migración y métricas) se alinean ahora con el RFC; `deploy/postgres/migrations/i8ab_risk_policy_offsets.sql` es la fuente única y `postgresRiskPolicyRepo` detecta las nuevas columnas.

## 4. Concurrencia, Errores y Límites
### 4.1 Concurrencia
- No se observaron nuevas race conditions; `commandContextMu` y la propagación de spans mantienen el comportamiento previo.
### 4.2 Manejo de Errores
- El fallback ahora registra attempt1/attempt2 correctamente, pero debido a CR-i8ab-01 puede activarse innecesariamente, inflando `stop_offset_fallback_total`.
### 4.3 Límites y Edge Cases
- Falta cubrir los casos donde `distance + offset <= 0`; al no clamplearse, las estrategias que buscan “apretar” stops nunca aplican y pueden generar retries superfluos.

## 5. Observabilidad y Performance
- Métricas, logs JSON y spans (`core.stop_offset.compute`/`fallback`) cumplen las guías; también se añadieron los buckets 0/1/2/5/10/20/50/100 pips.
- Mientras el bug BLOQUEANTE permanezca, la telemetría reportará “skipped” aunque exista configuración, reduciendo su utilidad para medir el KPI de rechazos.

## 6. Dictamen Final y Checklist
- **Dictamen global:** Cambios mayores (offsets aún fallan + cobertura insuficiente).
- **Checklist:**
  - RFC cubierto sin desviaciones: **FALLA** (CR-i8ab-01).
  - Build compilable: **OK** (`go test ./sdk/... ./core/... ./agent/...`; lint sigue bloqueado por falta de `golangci-lint` en el entorno).
  - Tests clave verdes: **OBS** (sin lint ni suites de integración específicas).
  - Telemetría mínima requerida presente: **OK** (instrumentación “stop_offset_*” conforme).
  - Riesgos críticos abordados: **FALLA** (offsets que acercan SL/TP no se aplican y el fallback se activa de más).
