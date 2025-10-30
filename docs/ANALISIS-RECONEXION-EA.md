# Análisis y Diseño: Reconexión Automática de EAs

**Fecha:** 2025-10-29  
**Versión:** i2b (Mejora de Reconexión)  
**Problema:** Cuando una EA se desconecta y vuelve a conectar, el Agent no la detecta.

---

## Problema Identificado

### Síntoma
- La EA se desconecta correctamente (se detecta EOF y se envía `AccountDisconnected`).
- Cuando la EA se vuelve a conectar, el Agent no la detecta.
- El pipe queda "huérfano" esperando una conexión que nunca se procesa.

### Causa Raíz

**Ubicación:** `agent/internal/pipe_manager.go:227-333`

**Flujo Actual (PROBLEMÁTICO):**
```
1. createPipe() crea handler y lanza goroutine
2. Handler.Run() se ejecuta:
   a. WaitForConnection()  ← SE EJECUTA UNA SOLA VEZ
   b. notifyAccountConnected()
   c. Loop de lectura (while true)
   d. Si EOF → break (sale del loop)
   e. notifyAccountDisconnected()
   f. return nil  ← TERMINA LA GOROUTINE
3. ❌ NUNCA vuelve a llamar WaitForConnection()
```

**Problema:**
- `WaitForConnection()` solo se llama una vez al inicio.
- Cuando la conexión se pierde, el método `Run()` termina completamente.
- La goroutine termina y nunca vuelve a esperar una nueva conexión.
- El pipe server sigue existiendo pero nadie está escuchando nuevas conexiones.

---

## Componentes Involucrados

| Componente | Path | Responsabilidad Actual | Cambio Necesario |
|------------|------|------------------------|------------------|
| **PipeHandler.Run()** | `agent/internal/pipe_manager.go:227-333` | Loop de conexión única | Envolver en loop externo de reconexión |
| **WindowsPipeServer** | `sdk/ipc/windows_pipe.go:42-112` | Accept de conexiones | Sin cambios (ya soporta múltiples Accept) |
| **WindowsPipeServer.listener** | `sdk/ipc/windows_pipe.go:69` | Named Pipe listener | Debe poder Accept múltiples veces |

---

## Análisis del Comportamiento Actual

### WindowsPipeServer - ¿Soporta Múltiples Conexiones?

**Código Actual:** `sdk/ipc/windows_pipe.go:89-112`

```go
func (s *WindowsPipeServer) WaitForConnection(ctx context.Context) error {
    // ...
    conn, err := s.listener.Accept()  // ← ¿Puede Accept múltiples veces?
    // ...
    s.currentConn = conn  // ← Guarda solo la conexión actual
}
```

**Pregunta crítica:** ¿El `listener` (Windows Named Pipe) puede hacer `Accept()` múltiples veces después de que una conexión se cierra?

**Respuesta:** 
- **SÍ**, los Named Pipes de Windows en modo servidor pueden aceptar múltiples conexiones secuenciales.
- Cada llamada a `Accept()` espera una nueva conexión del cliente.
- Cuando una conexión se cierra, el servidor puede llamar `Accept()` nuevamente para esperar la siguiente.

**Problema identificado:**
- `currentConn` se sobrescribe en cada `Accept()`, pero si la conexión anterior sigue abierta, puede haber conflictos.
- Necesitamos asegurar que la conexión anterior esté cerrada antes de aceptar una nueva.

---

## Diseño de Solución: Loop de Reconexión

### Arquitectura Propuesta

```
PipeHandler.Run():
┌─────────────────────────────────────────┐
│ LOOP EXTERNO (Reconexión)               │
│  ┌───────────────────────────────────┐ │
│  │ WaitForConnection()                │ │
│  │ notifyAccountConnected()           │ │
│  │                                     │ │
│  │ LOOP INTERNO (Lectura)             │ │
│  │  ┌──────────────────────────────┐  │ │
│  │  │ ReadLine()                    │  │ │
│  │  │ Process message               │  │ │
│  │  │ Si EOF → break (sale del      │  │ │
│  │  │          loop interno)        │  │ │
│  │  └──────────────────────────────┘  │ │
│  │                                     │ │
│  │ notifyAccountDisconnected()        │ │
│  │ Cerrar conexión actual              │ │
│  │ Esperar breve delay (backoff)       │ │
│  │ ← Volver al inicio del loop externo│ │
│  └───────────────────────────────────┘ │
└─────────────────────────────────────────┘
```

### Cambios Requeridos

#### 1. Modificar `PipeHandler.Run()` - Loop de Reconexión

**Ubicación:** `agent/internal/pipe_manager.go:227-333`

**Cambio:**
- Envolver todo el código actual en un loop externo `for { ... }`.
- Después de `notifyAccountDisconnected()`, cerrar la conexión actual.
- Esperar un breve delay (backoff exponencial opcional).
- Volver a llamar `WaitForConnection()`.

**Estructura:**
```go
func (h *PipeHandler) Run() error {
    reconnectDelay := 100 * time.Millisecond  // Delay inicial
    maxReconnectDelay := 5 * time.Second      // Delay máximo
    
    for {
        // 1. Esperar conexión
        if err := h.server.WaitForConnection(h.ctx); err != nil {
            // Si contexto cancelado, salir completamente
            if h.ctx.Err() != nil {
                return nil
            }
            // Error en Accept - esperar y reintentar
            select {
            case <-h.ctx.Done():
                return nil
            case <-time.After(reconnectDelay):
                reconnectDelay = min(reconnectDelay*2, maxReconnectDelay)
                continue
            }
        }
        
        // 2. Notificar conexión
        h.notifyAccountConnected()
        
        // 3. Loop de lectura (código actual)
        for {
            // ... código actual de lectura ...
            if errStr == "EOF" {
                break  // Sale del loop interno
            }
        }
        
        // 4. Notificar desconexión
        h.notifyAccountDisconnected("client_disconnected")
        
        // 5. Cerrar conexión actual
        if err := h.server.Close(); err != nil {
            h.logError("Failed to close pipe connection", err, nil)
        }
        
        // 6. Reset delay y volver al inicio del loop externo
        reconnectDelay = 100 * time.Millisecond
    }
}
```

#### 2. WindowsPipeServer - Soporte para Múltiples Conexiones

**Ubicación:** `sdk/ipc/windows_pipe.go:42-112`

**Problema Potencial:**
- `currentConn` puede seguir abierta cuando se llama `Accept()` nuevamente.
- Necesitamos asegurar que se cierre antes de aceptar nueva conexión.

**Cambio Propuesto:**
```go
func (s *WindowsPipeServer) WaitForConnection(ctx context.Context) error {
    // Cerrar conexión anterior si existe
    if s.currentConn != nil {
        s.currentConn.Close()
        s.currentConn = nil
    }
    
    // ... resto del código actual ...
}
```

**Alternativa (mejor):**
- El `PipeHandler` debe cerrar la conexión explícitamente antes de llamar `WaitForConnection()` nuevamente.
- O `WaitForConnection()` puede verificar y cerrar la conexión anterior automáticamente.

#### 3. Manejo de Backoff Exponencial

**Propósito:** Evitar reconexiones agresivas que puedan saturar el sistema.

**Estrategia:**
- Delay inicial: 100ms
- Multiplicador: x2 en cada fallo
- Delay máximo: 5 segundos
- Reset a delay inicial cuando hay éxito

**Código:**
```go
reconnectDelay := 100 * time.Millisecond
maxReconnectDelay := 5 * time.Second

// En caso de error en WaitForConnection:
reconnectDelay = min(reconnectDelay*2, maxReconnectDelay)
select {
case <-time.After(reconnectDelay):
    continue
case <-h.ctx.Done():
    return nil
}

// En caso de conexión exitosa:
reconnectDelay = 100 * time.Millisecond  // Reset
```

---

## Consideraciones de Diseño

### 1. Cierre de Conexión Anterior

**Problema:** Si no cerramos la conexión anterior antes de aceptar una nueva, puede haber:
- Handles de archivo abiertos (resource leak)
- Confusión sobre qué conexión está activa
- Errores en `ReadLine()` si intenta leer de conexión cerrada

**Solución:**
- Cerrar `currentConn` explícitamente antes de llamar `WaitForConnection()` nuevamente.
- O hacer que `WaitForConnection()` cierre automáticamente la conexión anterior.

### 2. Estado del Pipe Server

**Pregunta:** ¿El `listener` puede seguir aceptando conexiones después de cerrar `currentConn`?

**Respuesta:** 
- **SÍ**, el `listener` (creado una vez en `NewWindowsPipeServer`) puede aceptar múltiples conexiones.
- Cada `Accept()` retorna una nueva conexión.
- Cerrar `currentConn` no afecta al `listener`.

### 3. Notificaciones al Core

**Comportamiento Esperado:**
- Cada vez que se conecta una EA → `AccountConnected`.
- Cada vez que se desconecta → `AccountDisconnected`.
- Si la misma EA se reconecta rápidamente → `AccountDisconnected` seguido de `AccountConnected`.

**Consideración:**
- El Core debe manejar reconexiones rápidas correctamente.
- El `AccountRegistry` ya tiene lógica para re-registro (última escritura gana).

### 4. Contexto y Cancelación

**Importante:**
- Si `h.ctx.Done()` se cancela, salir completamente del loop externo.
- No intentar reconectar si el contexto está cancelado.
- Limpiar recursos apropiadamente.

### 5. Logging y Observabilidad

**Añadir logs para:**
- Intento de reconexión (con delay actual).
- Éxito de reconexión.
- Fallos en reconexión.
- Métricas de número de reconexiones por pipe.

---

## Flujo Completo Propuesto

### Secuencia Normal (Primera Conexión)
```
1. EA se conecta
2. WaitForConnection() → éxito
3. notifyAccountConnected() → Core registra cuenta
4. Loop de lectura activo
5. EA envía mensajes → procesados
```

### Secuencia de Desconexión/Reconexión
```
1. EA se desconecta (cierra pipe)
2. ReadLine() detecta EOF
3. Sale del loop interno
4. notifyAccountDisconnected() → Core desregistra cuenta
5. Cerrar currentConn
6. Esperar delay (100ms inicial)
7. WaitForConnection() → espera nueva conexión
8. EA se reconecta
9. WaitForConnection() → éxito
10. notifyAccountConnected() → Core registra cuenta nuevamente
11. Loop de lectura activo
```

### Secuencia con Errores
```
1. WaitForConnection() falla (error en Accept)
2. Verificar si ctx.Done() → si sí, salir
3. Esperar delay con backoff exponencial
4. Volver a intentar WaitForConnection()
5. Si éxito → continuar normalmente
6. Si falla repetidamente → seguir reintentando con backoff
```

---

## Cambios Específicos Requeridos

### Archivo 1: `agent/internal/pipe_manager.go`

**Método:** `PipeHandler.Run()`

**Cambios:**
1. Envolver código actual en loop externo `for { ... }`.
2. Añadir lógica de backoff exponencial.
3. Cerrar conexión antes de reconectar.
4. Reset de delay en éxito.
5. Logging de reconexiones.

**Líneas afectadas:** 227-333

### Archivo 2: `sdk/ipc/windows_pipe.go` (Opcional)

**Método:** `WindowsPipeServer.WaitForConnection()`

**Cambio opcional:**
- Cerrar `currentConn` si existe antes de aceptar nueva conexión.
- Esto hace el código más robusto pero no es estrictamente necesario si el handler cierra la conexión.

**Líneas afectadas:** 89-112

---

## Métricas Recomendadas (Futuro)

Para monitoreo de reconexiones:

1. **`echo.pipe.reconnection.attempts`** (counter)
   - Labels: `pipe_name`, `role`, `account_id`
   - Incrementar en cada intento de reconexión

2. **`echo.pipe.reconnection.success`** (counter)
   - Labels: `pipe_name`, `role`, `account_id`
   - Incrementar cuando reconexión exitosa

3. **`echo.pipe.reconnection.delay_ms`** (histogram)
   - Labels: `pipe_name`, `role`
   - Registrar delay usado en cada reconexión

4. **`echo.pipe.connection.duration_seconds`** (histogram)
   - Labels: `pipe_name`, `role`
   - Duración de cada conexión antes de desconexión

---

## Consideraciones de Rendimiento

### Impacto del Loop de Reconexión

**Overhead:**
- Loop externo: mínimo (solo cuando no hay conexión).
- `WaitForConnection()`: bloqueante pero en goroutine separada.
- Backoff: evita CPU spinning.

**Conclusión:**  
✅ **Impacto mínimo.** El overhead solo ocurre cuando no hay conexión activa.

### Impacto en Latencia

**Reconexión rápida:**
- Delay mínimo: 100ms
- Si EA se reconecta inmediatamente: 100ms de delay adicional.
- Aceptable para operación resiliente.

**Reconexión con errores:**
- Backoff exponencial puede llegar a 5 segundos.
- Aceptable para evitar saturación del sistema.

---

## Pruebas Recomendadas

### Escenario 1: Reconexión Simple
1. EA conectada y enviando mensajes.
2. Cerrar EA manualmente.
3. Verificar `AccountDisconnected` enviado.
4. Reconectar EA.
5. Verificar `AccountConnected` enviado.
6. Verificar que mensajes se procesan correctamente.

### Escenario 2: Reconexión Rápida
1. EA conectada.
2. Cerrar y reconectar rápidamente (<100ms).
3. Verificar que ambas notificaciones ocurren.
4. Verificar que no hay pérdida de mensajes.

### Escenario 3: Múltiples Reconexiones
1. Conectar/desconectar EA múltiples veces.
2. Verificar que cada conexión se detecta.
3. Verificar que no hay memory leaks.
4. Verificar que el delay de backoff funciona.

### Escenario 4: Desconexión durante Lectura
1. EA conectada y enviando mensajes continuamente.
2. Desconectar abruptamente durante envío.
3. Verificar que EOF se detecta correctamente.
4. Verificar que reconexión funciona.

---

## Resumen de Diseño

### Cambio Principal
- **Envolver `Run()` en loop externo** que reintenta `WaitForConnection()` después de cada desconexión.

### Componentes Modificados
1. `agent/internal/pipe_manager.go:227-333` - Añadir loop de reconexión
2. `sdk/ipc/windows_pipe.go:89-112` - Opcional: cerrar conexión anterior automáticamente

### Comportamiento Nuevo
- ✅ Reconexión automática después de desconexión
- ✅ Backoff exponencial para evitar saturación
- ✅ Notificaciones correctas al Core en cada conexión/desconexión
- ✅ Sin cambios en protocolo o interfaces externas

### Compatibilidad
- ✅ Sin breaking changes
- ✅ Comportamiento transparente para Core
- ✅ Compatible con EAs existentes

---

**Fin del Diseño**

