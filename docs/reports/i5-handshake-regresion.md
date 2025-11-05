# Reporte – Iteración 5 (Handshake v2)

**Fecha:** 2025-11-05  
**Autor:** Equipo Echo  
**Ámbito:** Core / Agent / EAs (Iteración 5 – Handshake versionado)

---

## Contexto de la iniciativa

La iteración 5 incorporó el handshake versionado entre Master/Slave EA → Agent → Core, con feedback estructurado `SymbolRegistrationResult`, bloqueo operativo por estado (`ACCEPTED|WARNING|REJECTED`) y tooling para re-evaluaciones manuales. Los cambios principales viven en:

- `core/internal/handshake_evaluator.go`
- `core/internal/handshake_reconciler.go`
- `agent/internal/pipe_manager.go`
- `clients/mt4/master.mq4`, `clients/mt4/slave.mq4`
- `core/cmd/echo-core-cli`

## Trabajo realizado hasta ahora

- **Core/Agent** persisten y reenvían `SymbolRegistrationResult` con las reglas de evaluación centralizadas.
- **EAs** envían handshake v2, consumen el feedback y bloquean operaciones cuando `status=REJECTED`.
- **CLI (`echo-core-cli handshake evaluate`)** habilita re-evaluaciones manuales con opción `--no-send`.
- **Seeds de ETCD** se alinearon con la nueva configuración de protocolo (sin `blocked_versions`).

## Pendientes detectados en ambiente de pruebas

### 1. Re-evaluaciones infinitas en el Core

**Síntoma:** Al conectar un slave rechazado, el Core genera cientos de logs por segundo (`Handshake evaluado` / `Handshake re-evaluado`).  
**Referencias:** `core/internal/handshake_evaluator.go`, `core/internal/handshake_reconciler.go`

**Causa raíz preliminar:**
- `HandshakeEvaluator.Evaluate` persiste SIEMPRE un nuevo registro (`CreateEvaluation`) antes de que el reconciliador compare la evaluación previa.
- Aunque `evaluationEquivalent(...)` detecte que el contenido no cambió, el insert ya disparó `pg_notify`, generando otro ciclo.
- Cada ciclo actualiza `spec_age_ms` y `evaluated_at`, lo que mantiene vivo el loop.

**Acción recomendada:**
1. Refactorizar `HandshakeEvaluator.Evaluate` para calcular el resultado **antes** de persistir y sólo llamar a `CreateEvaluation` cuando haya cambios relevantes.
2. Alternativamente, permitir a `CreateEvaluation` detectar duplicados (por `evaluation_id` o hash) y evitar el `INSERT + NOTIFY` cuando el estado sea idéntico.
3. Añadir métricas/alertas de protección (`echo.core.handshake.reconcile_skipped_total`) para monitorear loops futuros.

### 2. Flood de `SymbolRegistrationResult` en el Agent

**Síntoma:** El Agent reenvía el mismo `SymbolRegistrationResult` múltiples veces (`SymbolRegistrationResult routed` en bucle).  
**Referencias:** `agent/internal/pipe_manager.go` (`routeSymbolRegistrationResult`).

**Causa raíz preliminar:** El loop descrito en el punto 1 dispara nuevos inserts y, por ende, nuevos mensajes gRPC que el Agent reenvía al pipe. Aunque el EA bloquea el trading, el ruido en logs/telemetría es elevado.

**Acción recomendada:**
- Resolver el loop del Core. Una vez que cesen los inserts, el Agent dejará de recibir duplicados.
- Opcional: agregar cache de `evaluation_id` en el Agent para descartar `SymbolRegistrationResult` repetidos.

## Plan de abordaje propuesto

1. **Core:** Mover la lógica de comparación previa a la persistencia. Evaluar introducir un `EvaluatePreview` que retorne `handshake.Evaluation` sin efectos secundarios.
2. **Agent:** Duplicar protección opcional (cache simple por `account_id → evaluation_id`) para filtrar reenvíos hasta que el Core sea idempotente.
3. **Pruebas:**
   - Unitarias sobre `handshake_reconciler` que simulen notificaciones repetidas sin cambios en la evaluación.
   - Escenario end-to-end con cuenta en estado `REJECTED` para verificar que se emite un único feedback.
4. **Monitoreo:** Incorporar métricas de skip/loops y alarmas en dashboards de Handshake.

## Estado actual

- ✅ Handshake v2, feedback y bloqueos locales están funcionales.  
- ❌ Persisten re-evaluaciones infinitas y ruido operacional cuando una cuenta queda en `REJECTED`.

> Nota: el seed de ETCD ya no contiene `core/protocol/blocked_versions`, por lo que el Core vuelve a levantar correctamente una vez limpia la configuración.

---

**Seguimiento:** Una vez corregido el flujo en Core/Agent, actualizar este reporte (o emitir un addendum) para dejar constancia de la solución y evidencias de las pruebas.
