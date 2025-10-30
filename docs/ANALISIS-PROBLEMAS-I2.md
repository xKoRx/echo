# Análisis de Problemas Detectados en Iteración 2

**Fecha:** 2025-10-29  
**Versión:** i2 (Routing Selectivo)  
**Autor:** Análisis Técnico

---

## Resumen Ejecutivo

Se han identificado dos problemas críticos después de la implementación de la iteración 2:

1. **Desconexión de EA no detectada**: El Agent no reconoce cuando un Slave EA o Master EA se desconecta del Named Pipe.
2. **Latencia aumentada**: La latencia E2E aumentó de ~250ms a ~2 segundos, un incremento de ~8x.

---

## Problema 1: Desconexión de EA No Detectada

### Componentes Involucrados

| Componente | Path | Responsabilidad |
|------------|------|-----------------|
| **PipeHandler.Run()** | `agent/internal/pipe_manager.go:227-333` | Loop principal de lectura del pipe |
| **LineReader.ReadLine()** | `sdk/ipc/reader.go:68-94` | Lectura de líneas con timeout |
| **JSONReader.ReadLine()** | `sdk/ipc/reader.go:68-94` | Implementación con scanner |
| **notifyAccountDisconnected()** | `agent/internal/pipe_manager.go:666-693` | Notificación al Core |
| **WindowsPipeServer** | `sdk/ipc/windows_pipe.go:89-113` | Manejo de conexiones Named Pipe |

### Teorías del Problema

#### Teoría 1.1: EOF Tratado como Condición Normal ⚠️ **MÁS PROBABLE**

**Ubicación:** `agent/internal/pipe_manager.go:287-293`

**Descripción:**
El código actual trata EOF como una condición normal de "no hay datos disponibles", ejecutando `continue` en lugar de salir del loop.

**Código problemático:**
```go
if errStr == "i/o timeout" || errStr == "EOF" {
    select {
    case <-h.ctx.Done():
        return nil
    case <-time.After(100 * time.Millisecond):
    }
    continue  // ❌ PROBLEMA: EOF debería salir del loop
}
```

**Consecuencia:**
- El loop nunca termina cuando hay EOF real (desconexión del cliente).
- El código en líneas 326-330 (`notifyAccountDisconnected`) solo se ejecuta si el loop sale, pero nunca sale por EOF.
- La desconexión nunca se reporta al Core.

**Evidencia:**
- EOF es retornado por `scanner.Scan()` cuando el pipe se cierra (línea 82 en `reader.go`).
- El código trata EOF igual que timeout, que es conceptualmente incorrecto.

---

#### Teoría 1.2: Named Pipe No Reporta EOF Inmediatamente

**Ubicación:** `sdk/ipc/windows_pipe.go:89-113` y `sdk/ipc/reader.go:68-94`

**Descripción:**
Los Named Pipes de Windows pueden no reportar EOF inmediatamente cuando el cliente se desconecta. El estado de desconexión solo se detecta cuando el servidor intenta leer después del cierre.

**Mecanismo:**
- `WaitForConnection()` solo espera la conexión inicial (línea 234 en `pipe_manager.go`).
- Si el cliente cierra el pipe sin notificar, `ReadLine()` puede seguir funcionando hasta el siguiente intento de lectura.
- Windows Named Pipes puede mantener el handle abierto aunque el cliente se haya desconectado físicamente.

**Consecuencia:**
- Puede haber un delay entre la desconexión física y la detección del EOF.
- El timeout de 1 segundo puede enmascarar la desconexión real.

---

#### Teoría 1.3: Timeout Enmascara Desconexiones

**Ubicación:** `agent/internal/pipe_manager.go:264` y `sdk/ipc/reader.go:68-94`

**Descripción:**
El timeout de 1 segundo puede ocurrir antes de que se detecte el EOF real cuando un cliente se desconecta.

**Mecanismo:**
- `lr.SetTimeout(1 * time.Second)` se establece en cada iteración (línea 264).
- Si el cliente se desconecta entre lecturas, el siguiente `ReadLine()` esperará hasta el timeout.
- El timeout se captura como "i/o timeout" y se trata igual que EOF (línea 287).

**Consecuencia:**
- Las desconexiones pueden tardar hasta 1 segundo en detectarse (si acaso).
- Si hay múltiples timeouts consecutivos, podría ser evidencia de desconexión pero no se detecta como tal.

---

#### Teoría 1.4: Scanner Puede Ocultar Errores de Conexión

**Ubicación:** `sdk/ipc/reader.go:68-94` y `agent/internal/pipe_manager.go:263`

**Descripción:**
Se crea un nuevo `LineReader` (y por ende un nuevo scanner) en cada iteración del loop. Si el scanner queda en un estado inconsistente, puede no reflejar correctamente el estado real de la conexión.

**Mecanismo:**
- Línea 263: `lr := ipc.NewLineReader(h.server)` se ejecuta en cada iteración.
- El scanner interno puede tener estado que no se resetea correctamente.
- Si hay un error de conexión pero el scanner no lo detecta, el loop continúa indefinidamente.

**Consecuencia:**
- Posible estado inconsistente entre iteraciones.
- El estado real del pipe puede no reflejarse en el scanner.

---

#### Teoría 1.5: Falta Verificación Activa del Estado del Pipe

**Ubicación:** `agent/internal/pipe_manager.go:227-333`

**Descripción:**
El código solo hace lectura reactiva (espera datos), pero no verifica activamente el estado del pipe para detectar desconexiones.

**Mecanismo:**
- No hay heartbeat o ping al pipe para verificar que sigue conectado.
- Solo se detecta desconexión cuando se intenta leer y falla.
- Si el cliente cierra sin escribir, puede pasar mucho tiempo hasta detectar la desconexión.

**Consecuencia:**
- Desconexiones silenciosas pueden pasar desapercibidas.
- Dependencia total de la detección reactiva vía EOF/timeout.

---

### Diagnóstico del Problema 1

**Causa Principal Identificada:**  
**Teoría 1.1** es la más probable. El código trata EOF como condición normal y nunca sale del loop, por lo que `notifyAccountDisconnected()` nunca se ejecuta.

**Cómo Verificar:**
1. Añadir logs cuando se detecta EOF para ver si realmente ocurre.
2. Verificar si el loop de `Run()` alguna vez sale (log al final del método).
3. Monitorear si `AccountDisconnected` alguna vez se envía al Core.

---

## Problema 2: Latencia Aumentada (250ms → 2 segundos)

### Componentes Involucrados

| Componente | Path | Responsabilidad |
|------------|------|-----------------|
| **Router.handleTradeIntent()** | `core/internal/router.go:186-365` | Procesamiento de TradeIntent y routing |
| **AccountRegistry.GetOwner()** | `core/internal/account_registry.go:148-160` | Lookup de owner por account_id |
| **Router.sendToAgent()** | `core/internal/router.go:901-925` | Envío selectivo a Agent |
| **Router.broadcastOrder()** | `core/internal/router.go:927-958` | Fallback broadcast a todos los Agents |
| **Router.recordRoutingMetric()** | `core/internal/router.go:960-974` | Registro de métricas |
| **AgentConnection.SendCh** | `core/internal/core.go:310` | Canal de envío a Agent (buffer 1000) |
| **PipeHandler.Run()** | `agent/internal/pipe_manager.go:227-333` | Loop de lectura con nuevo LineReader |

### Teorías del Problema

#### Teoría 2.1: Lookup Adicional en Cada Orden (IMPACTO BAJO)

**Ubicación:** `core/internal/router.go:298` y `core/internal/account_registry.go:151-159`

**Descripción:**
Se añadió un lookup en el registry por cada orden: `r.core.accountRegistry.GetOwner(targetAccountID)`.

**Análisis:**
- Operación O(1) con RLock (lectura).
- Lock compartido con mínima contención.
- Overhead estimado: <1ms por lookup.

**Conclusión:**  
❌ **No explica el aumento de latencia.** El lookup es demasiado rápido para causar 1.75 segundos de delay.

---

#### Teoría 2.2: Fallback a Broadcast Cuando No Hay Owner ⚠️ **MUY PROBABLE**

**Ubicación:** `core/internal/router.go:332-342` y `core/internal/router.go:927-958`

**Descripción:**
Si la cuenta no está registrada en el registry, el código hace fallback a `broadcastOrder()`, que envía a TODOS los Agents.

**Mecanismo:**
- Si `found == false` (línea 332), se ejecuta `broadcastOrder()`.
- `broadcastOrder()` itera sobre todos los Agents (línea 938).
- El `select` en línea 940 puede bloquearse si `agent.SendCh` está lleno.

**Problema Crítico:**
```go
select {
case agent.SendCh <- msg:  // ❌ SIN TIMEOUT - puede bloquear indefinidamente
    sentCount++
case <-ctx.Done():
    // Solo se ejecuta si contexto se cancela
}
```

**Consecuencia:**
- Si las cuentas no están registradas al inicio, TODAS las órdenes hacen broadcast.
- Si algún canal `SendCh` está lleno, el código se bloquea esperando espacio.
- Delay acumulado: 1-2 segundos si hay múltiples Agents con canales llenos.

**Evidencia:**
- El código i0/i1 hacía broadcast siempre, pero probablemente los canales estaban vacíos.
- Ahora puede haber más contención si hay retrasos en el procesamiento.

---

#### Teoría 2.3: Procesamiento Secuencial Amplifica Delays ⚠️ **MUY PROBABLE**

**Ubicación:** `core/internal/router.go:286` y `core/internal/router.go:186-365`

**Descripción:**
El código procesa las órdenes secuencialmente en un loop `for _, order := range orders`. Si una orden tiene delay, todas las siguientes esperan.

**Mecanismo:**
```go
for _, order := range orders {  // ❌ SECUENCIAL
    // ... lookup ...
    if found {
        r.sendToAgent(...)  // Puede bloquear
    } else {
        r.broadcastOrder(...)  // Puede bloquear más
    }
}
```

**Análisis:**
- Con 15 slaves configurados, se crean 15 `ExecuteOrder`.
- Si cada broadcast toma 100-200ms (por bloqueo en canales), el total es: 15 × 150ms = 2.25 segundos.
- El procesamiento secuencial acumula todos los delays.

**Consecuencia:**
- Latencia acumulada en lugar de paralela.
- Cada orden espera a que la anterior termine completamente.

---

#### Teoría 2.4: Logging Excesivo en Hot Path ⚠️ **PROBABLE**

**Ubicación:** 
- `core/internal/router.go:907-914` (sendToAgent)
- `core/internal/router.go:942-947` (broadcastOrder)
- `core/internal/router.go:302-308` (métricas de lookup)

**Descripción:**
Cada orden genera múltiples logs estructurados dentro del hot path del routing.

**Mecanismo:**
- `sendToAgent()` llama a `telemetry.Info()` con 7 atributos (línea 907).
- `broadcastOrder()` llama a `telemetry.Info()` por cada Agent (línea 942).
- `RecordAccountLookup()` se llama siempre (línea 302 o 306).
- `recordRoutingMetric()` se llama siempre (línea 318, 329, 340).

**Análisis:**
- Con 15 órdenes y broadcast a 3 Agents: 15 × 3 × 1 log = 45 logs por TradeIntent.
- Cada log requiere serialización JSON y posible I/O.
- Overhead estimado: 2-5ms por log × 45 = 90-225ms acumulado.

**Consecuencia:**
- Logging bloqueante añade overhead significativo.
- Más notorio si el backend de logging (OTLP/Loki) está lento.

---

#### Teoría 2.5: Registro de Métricas Sincrónico

**Ubicación:**
- `core/internal/router.go:302-308` (RecordAccountLookup)
- `core/internal/router.go:960-974` (RecordRoutingMode)
- `sdk/telemetry/metricbundle/echo.go:291-299` y `304-311`

**Descripción:**
Las métricas se registran de forma sincrónica con múltiples atributos.

**Mecanismo:**
- `RecordAccountLookup()` se llama siempre (líneas 302 o 306).
- `RecordRoutingMode()` se llama siempre (líneas 318, 329, 340).
- Cada métrica puede tener overhead de serialización de atributos.

**Análisis:**
- 2-3 llamadas a métricas por orden.
- Overhead estimado: 0.5-1ms por llamada × 2-3 = 1-3ms por orden.
- Con 15 órdenes: 15-45ms acumulado.

**Conclusión:**  
⚠️ **Contribuye pero no es la causa principal.** Puede añadir 50-100ms totales.

---

#### Teoría 2.6: Creación de Nuevo LineReader en Cada Iteración

**Ubicación:** `agent/internal/pipe_manager.go:263-264`

**Descripción:**
Se crea un nuevo `LineReader` (y scanner interno) en cada iteración del loop de lectura.

**Código:**
```go
for {
    // ...
    lr := ipc.NewLineReader(h.server)  // ❌ Nuevo reader cada vez
    lr.SetTimeout(1 * time.Second)
    line, err := lr.ReadLine()
    // ...
}
```

**Análisis:**
- Cada `NewLineReader()` puede crear un nuevo scanner.
- Cada `SetTimeout()` establece deadline en el pipe.
- Overhead estimado: <1ms por iteración.

**Conclusión:**  
❌ **Impacto mínimo.** No explica el aumento de latencia.

---

#### Teoría 2.7: Posible Deadlock o Contención en AccountRegistry

**Ubicación:** `core/internal/account_registry.go:51-52`, `100-101`, `129-130`, `152-153`

**Descripción:**
Uso de RWMutex para proteger el registry. Si hay escrituras frecuentes (conexiones/desconexiones), puede haber contención.

**Análisis:**
- `GetOwner()` usa RLock (lectura) - líneas 152-153.
- `RegisterAccount()` y `UnregisterAccount()` usan Lock (escritura) - líneas 51, 100, 129.
- Lecturas concurrentes no se bloquean entre sí (RWMutex).
- Escrituras bloquean todas las lecturas.

**Conclusión:**  
❌ **Poco probable.** Los lookups son mayormente lecturas, y las escrituras son poco frecuentes (solo en conexión/desconexión).

---

#### Teoría 2.8: Canal Sin Buffer o Bloqueo en Escritura al Agent

**Ubicación:** 
- `core/internal/core.go:310` (buffer 1000)
- `core/internal/router.go:905-924` (sendToAgent)
- `core/internal/router.go:940-947` (broadcastOrder)

**Descripción:**
El canal `agent.SendCh` tiene buffer de 1000, pero si el Agent procesa lento, el buffer puede llenarse y bloquear.

**Código Problemático:**
```go
select {
case agent.SendCh <- msg:  // ❌ SIN TIMEOUT
    // éxito
case <-ctx.Done():
    // solo si contexto cancelado
}
```

**Mecanismo:**
- Si `SendCh` está lleno (1000 mensajes), el `select` bloquea esperando espacio.
- El contexto solo se cancela en shutdown, no tiene timeout.
- Puede bloquearse indefinidamente si el Agent está procesando lento.

**Consecuencia:**
- Bloqueo potencial de múltiples segundos si el Agent está saturado.
- Especialmente crítico en `broadcastOrder()` que envía a todos los Agents.

---

#### Teoría 2.9: Broadcast Cuando Owner No Disponible Inmediatamente

**Ubicación:** `core/internal/router.go:332-342` y `core/internal/router.go:320-331`

**Descripción:**
Si el lookup falla (cuenta no registrada o Agent desconectado), se hace broadcast, que tiene más overhead que envío selectivo.

**Mecanismo:**
- Lookup falla → `found == false` (línea 332).
- Se ejecuta `broadcastOrder()` que itera TODOS los Agents (línea 938).
- Cada Agent puede tener su propio delay si su canal está lleno.

**Consecuencia:**
- Si las cuentas no están registradas al inicio, TODAS las órdenes hacen broadcast.
- Overhead de iterar todos los Agents vs enviar a uno solo.
- Delay acumulado si múltiples Agents tienen canales llenos.

---

#### Teoría 2.10: Contexto Compartido con Deadline

**Ubicación:** `core/internal/router.go:286` y contexto pasado a `handleTradeIntent()`

**Descripción:**
El contexto usado puede tener deadline o cancelación que retrase el procesamiento.

**Análisis:**
- El contexto viene de `agentCtx` creado en `StreamBidi` (línea 306 en `core.go`).
- No se observa deadline explícito en el código.
- El contexto solo se cancela cuando el Agent se desconecta.

**Conclusión:**  
❌ **Poco probable.** No hay evidencia de deadline en el contexto del routing.

---

### Diagnóstico del Problema 2

**Causa Principal Identificada:**  
**Combinación de Teorías 2.2, 2.3 y 2.4:**

1. **Teoría 2.2 (Fallback Broadcast):** Si las cuentas no están registradas, todas las órdenes hacen broadcast.
2. **Teoría 2.3 (Procesamiento Secuencial):** El procesamiento secuencial amplifica los delays.
3. **Teoría 2.4 (Logging Excesivo):** El logging añade overhead adicional.

**Escenario Probable:**
1. Las cuentas no están registradas cuando llegan las primeras órdenes (timing issue).
2. Todas las órdenes hacen fallback a broadcast.
3. `broadcastOrder()` se bloquea en canales llenos (sin timeout).
4. El procesamiento secuencial acumula todos los delays.
5. El logging excesivo añade overhead adicional.

**Contribución Estimada:**
- Broadcast bloqueante: ~1-2 segundos (si canales llenos)
- Logging excesivo: ~100-200ms acumulado
- Procesamiento secuencial: amplifica los delays (multiplicador)

**Total estimado:** ~1.75-2 segundos (coincide con observación)

---

## Cómo Verificar el Diagnóstico

### Para Problema 1 (Desconexión):

1. **Añadir logs cuando se detecta EOF:**
   ```go
   if errStr == "EOF" {
       h.logInfo("EOF detected - client disconnected", ...)
       // Salir del loop aquí
   }
   ```

2. **Verificar si `notifyAccountDisconnected` se ejecuta:**
   - Buscar logs de "AccountDisconnected sent to Core" en el Agent.
   - Verificar si el Core recibe mensajes `AccountDisconnected`.

3. **Monitorear estado del loop:**
   - Añadir log al final de `Run()` para ver si alguna vez sale.

### Para Problema 2 (Latencia):

1. **Verificar registro de cuentas:**
   - Revisar métricas `echo.routing.account_lookup` con `result=miss`.
   - Si siempre es `miss`, el problema está en el registro de cuentas, no en el routing.

2. **Verificar estado de canales:**
   - Monitorear profundidad de `agent.SendCh` (capacidad 1000).
   - Si se llena frecuentemente, los bloqueos explican la latencia.

3. **Medir latencia por componente:**
   - Timestamp antes/después de `GetOwner()`.
   - Timestamp antes/después de `sendToAgent()` / `broadcastOrder()`.
   - Timestamp antes/después de logging.

4. **Verificar timing de registro:**
   - Verificar si `AccountConnected` se envía antes de que lleguen las primeras órdenes.
   - Si hay race condition, las primeras órdenes siempre harán broadcast.

---

## Archivos Modificados en i2 (Referencia)

Para contexto, estos son los archivos modificados en la iteración 2:

```
sdk/proto/v1/agent.proto                          # Añadidos AccountConnected/Disconnected
sdk/domain/account_validation.go                  # Validaciones nuevas
core/internal/account_registry.go                 # Nuevo componente
core/internal/core.go                              # Integración del registry
core/internal/router.go                            # Routing selectivo
agent/internal/pipe_manager.go                     # Detección conexión/desconexión
agent/internal/stream.go                           # AgentHello sin owned_accounts
agent/internal/agent.go                            # Pass agentID a PipeManager
sdk/telemetry/metricbundle/echo.go                 # Métricas de routing
```

---

## Próximos Pasos Recomendados

1. **Problema 1 (Desconexión):**
   - Tratar EOF como condición de salida del loop, no como condición normal.
   - Añadir verificación explícita del estado del pipe periódicamente.
   - Considerar heartbeat desde el cliente EA.

2. **Problema 2 (Latencia):**
   - Añadir timeout en `select` de canales (usar `time.After()`).
   - Verificar que las cuentas estén registradas antes de enrutar órdenes.
   - Mover logging fuera del hot path (usar goroutine o batching).
   - Considerar procesamiento paralelo de órdenes (con cuidado de race conditions).
   - Optimizar registro de métricas (batching o async).

---

**Fin del Análisis**

