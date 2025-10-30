# Solución a Problemas Detectados en Iteración 2 (i2b)

**Fecha:** 2025-10-29  
**Versión:** i2b (Correcciones de Estabilidad y Latencia)  
**Autor:** Correcciones Técnicas

---

## Resumen Ejecutivo

Se identificaron y corrigieron dos problemas críticos detectados después de la implementación de i2 (Routing Selectivo):

1. **Desconexión de EA no detectada**: Corregido - EOF ahora termina el loop y notifica al Core.
2. **Latencia aumentada (250ms → 2s)**: Corregido - Timeout de 500ms en canales y logging optimizado.

**Impacto esperado:**
- Detección de desconexiones: <1 segundo (vs nunca antes).
- Latencia E2E: restaurada a ~250-500ms (desde ~2s).
- Robustez: sistema no se bloquea si Agents están saturados.

---

## Problema 1: Desconexión de EA No Detectada

### Análisis del Problema

**Causa raíz:** En `agent/internal/pipe_manager.go`, el código trataba EOF (End Of File) como condición normal de "sin datos disponibles", ejecutando `continue` en lugar de salir del loop.

**Código problemático (antes):**
```go
if errStr == "i/o timeout" || errStr == "EOF" {
    select {
    case <-h.ctx.Done():
        return nil
    case <-time.After(100 * time.Millisecond):
    }
    continue  // ❌ EOF nunca sale del loop
}
```

**Consecuencia:**
- Cuando un Slave EA o Master EA cerraba su conexión Named Pipe, se retornaba EOF.
- El loop nunca terminaba porque EOF hacía `continue`.
- La función `notifyAccountDisconnected()` (líneas 328-330) nunca se ejecutaba.
- El Core nunca recibía mensaje `AccountDisconnected`.
- La cuenta permanecía registrada indefinidamente en `AccountRegistry`.

### Solución Implementada

**Archivo:** `agent/internal/pipe_manager.go` (líneas 284-319)

**Cambio:** Separar el tratamiento de EOF y timeout. EOF ahora ejecuta `break` para salir del loop.

```go
if err != nil {
    errStr := err.Error()
    
    // i2b: EOF significa desconexión del cliente - salir del loop
    if errStr == "EOF" {
        h.logInfo("Client disconnected (EOF detected)", map[string]interface{}{
            "pipe_name": h.name,
            "role":      h.role,
        })
        // Salir del loop para ejecutar notifyAccountDisconnected
        break
    }
    
    // Timeout es NORMAL cuando EA no envía nada - continuar esperando
    if errStr == "i/o timeout" {
        select {
        case <-h.ctx.Done():
            return nil
        case <-time.After(100 * time.Millisecond):
        }
        continue
    }
    
    // Otros errores: loggear y continuar (pueden ser transitorios)
    // ...
}
```

**Flujo corregido:**
1. Slave EA cierra Named Pipe.
2. `ReadLine()` retorna `io.EOF`.
3. Código detecta `EOF` y ejecuta `break`.
4. Loop termina, ejecución continúa en líneas 328-330.
5. `notifyAccountDisconnected("client_disconnected")` se ejecuta.
6. Core recibe `AccountDisconnected` y desregistra cuenta de `AccountRegistry`.

### Validación

**Escenarios de prueba:**
1. Cerrar Slave EA manualmente → debe detectar EOF en <1s y notificar al Core.
2. Verificar log "Client disconnected (EOF detected)" en Agent.
3. Verificar log "AccountDisconnected sent to Core (i2)" en Agent.
4. Verificar log "AccountDisconnected received (i2b)" en Core.
5. Verificar que cuenta desaparece de `AccountRegistry.GetStats()`.

---

## Problema 2: Latencia Aumentada (250ms → 2 segundos)

### Análisis del Problema

**Causa raíz:** Combinación de tres factores:

1. **Bloqueo indefinido en canales sin timeout** (más crítico):
   - `sendToAgent()` y `broadcastOrder()` usaban `select` sin timeout.
   - Si `agent.SendCh` estaba lleno (buffer 1000), se bloqueaban indefinidamente.
   - Con 15 slaves y broadcast fallback, podía acumular múltiples bloqueos.

2. **Procesamiento secuencial amplificaba delays**:
   - Loop `for _, order := range orders` procesaba 15 órdenes secuencialmente.
   - Cada bloqueo de 100-200ms se acumulaba: 15 × 150ms = 2.25s.

3. **Logging excesivo en hot path**:
   - Cada envío generaba log Info con 7 atributos.
   - Con broadcast a 3 Agents: 15 órdenes × 3 Agents = 45 logs.
   - Overhead estimado: ~100-200ms acumulado.

**Código problemático (antes):**
```go
select {
case agent.SendCh <- msg:  // ❌ SIN TIMEOUT
    r.core.telemetry.Info(ctx, "ExecuteOrder sent...", /* 7 atributos */)
    return true
case <-ctx.Done():
    // Solo cancela si contexto termina (raro)
}
```

### Solución Implementada

**Archivos modificados:**
- `core/internal/router.go`: Añadido import `time` y timeout en 3 funciones.

#### Cambio 1: Import de time

```go
import (
    "context"
    "fmt"
    "strings"
    "sync"
    "time"  // i2b: NEW - para timeouts en canales
    // ...
)
```

#### Cambio 2: sendToAgent con timeout (líneas 901-936)

```go
func (r *Router) sendToAgent(ctx context.Context, agent *AgentConnection, msg *pb.CoreMessage, order *pb.ExecuteOrder) bool {
    // i2b: Timeout de 500ms para evitar bloqueos indefinidos
    timeout := time.NewTimer(500 * time.Millisecond)
    defer timeout.Stop()

    select {
    case agent.SendCh <- msg:
        // ✅ Envío exitoso - logging reducido para hot path
        r.core.telemetry.Debug(ctx, "ExecuteOrder sent to Agent (selective i2b)",
            attribute.String("agent_id", agent.AgentID),
            attribute.String("command_id", order.CommandId),
            attribute.String("trade_id", order.TradeId),
            attribute.String("target_account_id", order.TargetAccountId),
        )
        return true

    case <-timeout.C:
        // ⚠️ Canal lleno o Agent lento - registrar warning y fallar
        r.core.telemetry.Warn(ctx, "Timeout sending to Agent, channel may be full (i2b)",
            attribute.String("agent_id", agent.AgentID),
            attribute.String("command_id", order.CommandId),
            attribute.String("target_account_id", order.TargetAccountId),
        )
        return false

    case <-ctx.Done():
        r.core.telemetry.Error(ctx, "Context cancelled while sending ExecuteOrder", ctx.Err())
        return false
    }
}
```

**Mejoras clave:**
- Timeout 500ms: si canal no acepta mensaje en 500ms, falla y continúa.
- Logging a Debug: reduce overhead en caso exitoso (mayoría de las veces).
- Atributos reducidos: solo 4 atributos clave en vez de 7.

#### Cambio 3: broadcastOrder con timeout (líneas 938-990)

```go
func (r *Router) broadcastOrder(ctx context.Context, msg *pb.CoreMessage, order *pb.ExecuteOrder) int {
    agents := r.core.GetAgents()
    if len(agents) == 0 {
        r.core.telemetry.Warn(ctx, "No agents connected, broadcast failed (i2b)")
        return 0
    }

    sentCount := 0
    timeoutCount := 0
    
    for _, agent := range agents {
        // i2b: Timeout de 500ms por Agent para evitar bloqueos acumulados
        timeout := time.NewTimer(500 * time.Millisecond)
        
        select {
        case agent.SendCh <- msg:
            sentCount++
            timeout.Stop()
            // i2b: Logging reducido - solo debug level en hot path

        case <-timeout.C:
            // ⚠️ Timeout en este Agent - continuar con los demás
            timeoutCount++
            r.core.telemetry.Warn(ctx, "Timeout broadcasting to Agent (i2b)",
                attribute.String("agent_id", agent.AgentID),
                attribute.String("command_id", order.CommandId),
            )

        case <-ctx.Done():
            timeout.Stop()
            // Continuar con otros agents
        }
    }
    
    // i2b: Log consolidado DESPUÉS del broadcast (reducir logging en hot path)
    if sentCount > 0 || timeoutCount > 0 {
        r.core.telemetry.Info(ctx, "Broadcast completed (fallback i2b)",
            attribute.String("command_id", order.CommandId),
            attribute.String("trade_id", order.TradeId),
            attribute.String("target_account_id", order.TargetAccountId),
            attribute.Int("sent_count", sentCount),
            attribute.Int("timeout_count", timeoutCount),
        )
    }

    return sentCount
}
```

**Mejoras clave:**
- Timeout 500ms por Agent: evita bloqueos acumulados.
- Contador de timeouts: monitoreo de Agents saturados.
- Logging consolidado: UN log Info al final con resumen, en lugar de N logs por Agent.
- `timeout.Stop()`: libera recursos del timer cuando no es necesario.

#### Cambio 4: broadcastCloseOrder con timeout (líneas 1008-1056)

**Mismas mejoras que broadcastOrder**, aplicadas a cierres de órdenes para consistencia.

### Análisis de Impacto

**Antes (i2 con problemas):**
- 15 órdenes × 3 Agents = hasta 45 envíos en broadcast fallback.
- Si 1 Agent tiene canal lleno: bloqueo indefinido.
- Procesamiento secuencial: 15 × bloqueo = latencia acumulada.
- Logging: 45 logs Info con serialización JSON.
- **Latencia observada: ~2 segundos**.

**Después (i2b corregido):**
- Timeout 500ms máximo por envío.
- Si canal lleno: warning y continuar con siguiente Agent.
- Procesamiento secuencial continúa pero sin bloqueos indefinidos.
- Logging: Debug en caso exitoso (no serializa atributos pesados), Info consolidado al final.
- **Latencia esperada: ~250-500ms** (restaurada a niveles i0/i1).

**Cálculo conservador (15 slaves, 3 Agents, todos con timeout):**
- Peor caso: 15 órdenes × 3 Agents × 500ms timeout = teóricamente 22.5s.
- **Realidad:** timeout solo ocurre si canal lleno, que es excepcional.
- En operación normal: canal no lleno → envío instantáneo (<1ms).
- **Latencia real esperada: 250-500ms en >95% de los casos.**

---

## Validación y Monitoreo

### Métricas Clave

1. **Desconexiones detectadas:**
   - Buscar logs "Client disconnected (EOF detected)" en Agent.
   - Buscar logs "AccountDisconnected received (i2b)" en Core.
   - Verificar que `echo.account_registry.total_accounts` disminuye cuando EA se desconecta.

2. **Timeouts en canales:**
   - Buscar logs "Timeout sending to Agent, channel may be full (i2b)".
   - Buscar logs "Timeout broadcasting to Agent (i2b)".
   - Si aparecen frecuentemente: Agent está saturado, considerar optimizaciones adicionales.

3. **Latencia E2E:**
   - Métrica: `echo.latency.e2e` (p50, p95, p99).
   - Objetivo: p95 < 500ms (restaurado desde ~2s).
   - Si p95 sigue alto: investigar otros cuellos de botella.

4. **Broadcast vs Selective:**
   - Métrica: `echo.routing.mode{mode="selective"}` vs `{mode="fallback_broadcast"}`.
   - Objetivo: >95% selectivo después de que todas las cuentas se registren.
   - Si broadcast alto: problema de registro de cuentas (timing o configuración).

### Dashboards Grafana (añadir paneles)

**Panel 1: Timeouts en Canales (NEW i2b)**
- Query: `sum(rate(echo_routing_timeout_total[5m])) by (agent_id)`
- Visualización: Time series.
- Alerta: si rate > 0.1 (más de 1 timeout cada 10s), Agent saturado.

**Panel 2: Latencia E2E (actualizado)**
- Query: `histogram_quantile(0.95, rate(echo_latency_e2e_bucket[5m]))`
- Visualización: Gauge con threshold en 500ms.
- Comparar i1 vs i2 vs i2b.

**Panel 3: Desconexiones de Cuentas (NEW i2b)**
- Query: `increase(echo_account_disconnected_total[5m])`
- Visualización: Bar chart.
- Muestra frecuencia de desconexiones de EAs.

---

## Checklist de Validación i2b

- [ ] **Funcional - Desconexión:**
  - [ ] Cerrar Slave EA → EOF detectado en <1s.
  - [ ] `AccountDisconnected` enviado al Core.
  - [ ] Cuenta desregistrada de `AccountRegistry`.
  - [ ] Próxima orden para esa cuenta hace fallback a broadcast.

- [ ] **Funcional - Timeout:**
  - [ ] Si canal `SendCh` lleno → timeout 500ms y warning en logs.
  - [ ] Sistema continúa procesando otras órdenes sin bloquear.
  - [ ] Métrica `timeout_count` incrementa en broadcast.

- [ ] **Performance:**
  - [ ] Latencia E2E p95 < 500ms (restaurada desde ~2s).
  - [ ] Sin bloqueos indefinidos en envíos a canales.
  - [ ] Logging reducido: Debug en hot path, Info consolidado.

- [ ] **Observabilidad:**
  - [ ] Logs "EOF detected" visibles cuando EA se desconecta.
  - [ ] Logs "Timeout sending/broadcasting" visibles si canal lleno.
  - [ ] Métrica `timeout_count` disponible en broadcasts.
  - [ ] Dashboard Grafana actualizado con paneles nuevos.

- [ ] **Calidad:**
  - [ ] Linters limpios (`go vet`, `staticcheck`).
  - [ ] Sin race conditions en operación manual.
  - [ ] Documentación actualizada (este documento + roadmap).

---

## Archivos Modificados

```
agent/internal/pipe_manager.go         # EOF ejecuta break (líneas 284-319)
core/internal/router.go                # Timeout 500ms + logging optimizado (3 funciones)
docs/01-arquitectura-y-roadmap.md      # Iteración 2b documentada (líneas 143-155)
docs/SOLUCION-PROBLEMAS-I2B.md         # Este documento (nuevo)
```

---

## Próximos Pasos

### Validación Inmediata (post-merge)

1. Desplegar en staging con carga real (3 Agents, 15 slaves).
2. Observar latencia E2E durante 1 hora con Grafana.
3. Forzar desconexiones de EAs y verificar notificación al Core.
4. Forzar saturación de Agent (retrasar consumo) y verificar timeouts.

### Optimizaciones Futuras (opcional, post-i2b)

Si timeouts persisten o latencia sigue alta:

1. **Paralelización del routing (i12a):**
   - Procesar órdenes en paralelo con goroutines (cuidado con race conditions).
   - Mantener serialización por `trade_id` para orden correcto.

2. **Backpressure en Agent (i12b):**
   - Monitorear profundidad de `SendCh` con métricas.
   - Rechazar mensajes si cola excede threshold (ej. 800/1000).
   - Alertar si Agent no puede consumir a tiempo.

3. **Async logging:**
   - Mover logging a goroutine separada con canal buffered.
   - Batch de logs antes de serializar a OTLP/Loki.

4. **Reconexión automática de EA (fuera de V1):**
   - Si EA se desconecta, intentar reconexión con backoff.
   - Útil para estabilidad en producción.

---

## Conclusiones

### Resumen de Correcciones

| Problema | Causa | Solución | Impacto |
|----------|-------|----------|---------|
| EOF no detectado | `continue` en lugar de `break` | `break` al detectar EOF | Desconexiones detectadas <1s |
| Latencia 250ms→2s | Bloqueo en canales sin timeout | Timeout 500ms + logging optimizado | Latencia restaurada a ~250-500ms |

### Lecciones Aprendidas

1. **EOF es diferente de timeout:** EOF indica cierre de conexión, timeout es "sin datos disponibles".
2. **Canales con buffer no son mágicos:** Si productor es más rápido que consumidor, se llenan y bloquean.
3. **Logging en hot path es costoso:** Serialización JSON + I/O en cada envío añade latencia acumulada.
4. **Timeouts son críticos para robustez:** Sin timeout, un componente lento bloquea todo el sistema.

### Estado de i2b

- ✅ **Implementado**: EOF termina loop, timeout en canales, logging optimizado.
- ✅ **Documentado**: Roadmap actualizado, este documento creado.
- ⏳ **Pendiente validación**: Despliegue en staging y observación de métricas.
- ⏳ **Pendiente merge**: Linters limpios, esperando validación operativa.

---

**Fin del Documento - Solución i2b**

