# 🔥 Corrección Urgente - i2b Ping/Pong

**Fecha:** 2025-10-29 21:32  
**Prioridad:** CRÍTICA  
**Estado:** ✅ CORREGIDO

---

## 🐛 Problemas Identificados

### Problema 1: Agent sin handler de ping (CRÍTICO)
**Síntoma:**
```json
{"level":"WARN","msg":"Unknown message type from slave","type":"ping"}
```

**Causa:**  
El binario `echo-agent-windows-amd64.exe` usado era **anterior** a la implementación del handler de ping/pong.

**Solución:**  
✅ **Recompilado** `echo-agent-windows-amd64.exe` con el handler actualizado.

---

### Problema 2: Timestamps negativos en Slave EA (CRÍTICO)
**Síntoma:**
```json
"timestamp_ms":-2124526328
```

**Causa:**  
Slave EA hacía `IntegerToString((int)GetTickCount())` causando **overflow** al convertir `ulong` (unsigned 32-bit) a `int` (signed 32-bit).

**Solución:**  
✅ Añadida función `ULongToStr()` en Slave EA  
✅ Reemplazados **todos** los `IntegerToString((int)GetTickCount())` con `ULongToStr(GetTickCount())`

---

## 📋 Archivos Actualizados

### 1. Agent (Go)
- **Archivo:** `/home/kor/go/src/github.com/xKoRx/echo/bin/echo-agent-windows-amd64.exe`
- **Tamaño:** 19 MB
- **Cambios:** Handler de ping/pong compilado correctamente

### 2. Slave EA (MQL4)
- **Archivo:** `/home/kor/go/src/github.com/xKoRx/echo/bin/slave.mq4`
- **Tamaño:** 49 KB
- **Cambios:**
  - Añadida función `ULongToStr()`
  - Timestamps corregidos en:
    - `SendHandshake()`
    - `SendPing()`
    - `TryHandlePong()`
    - `SendExecutionResult()`
    - `SendCloseResult()`
    - `OnTimer()` (watchdog)

### 3. Master EA (MQL4)
- **Archivo:** `/home/kor/go/src/github.com/xKoRx/echo/bin/master.mq4`
- **Tamaño:** 50 KB
- **Estado:** ✅ Sin cambios (ya tenía `ULongToStr()`)

---

## 🚀 Pasos de Despliegue (URGENTE)

### Paso 1: Detener Agent Actual
```powershell
# En Windows, cerrar el Agent actual (Ctrl+C o Task Manager)
```

### Paso 2: Reemplazar Agent
```powershell
# Copiar el nuevo binario desde echo/bin/
copy \\path\to\echo\bin\echo-agent-windows-amd64.exe C:\path\to\agent\

# Renombrar si es necesario
ren echo-agent-windows-amd64.exe echo-agent.exe
```

### Paso 3: Iniciar Nuevo Agent
```powershell
.\echo-agent.exe
```

### Paso 4: Recompilar Slave EA
1. Abrir **MetaEditor** (MT4/MT5)
2. Abrir `slave.mq4` desde `echo/bin/`
3. Asegurar que `JAson.mqh` esté en `Include/`
4. Compilar (F7)
5. Verificar: **0 errors, 0 warnings**

### Paso 5: Remover EAs Viejos de MT4/MT5
```
1. En MT4/MT5, remover todos los Slave EAs de los gráficos
2. Esperar 5 segundos (para que se desconecten)
```

### Paso 6: Adjuntar Nuevos EAs
```
1. Adjuntar Slave EA actualizado a gráficos
2. Verificar "Allow DLL imports" habilitado
3. Verificar conexión en logs del Agent
```

---

## ✅ Verificación de Corrección

### Agent debe mostrar:
```json
{"level":"INFO","msg":"Handshake received","role":"slave"}
{"level":"INFO","msg":"Pong sent","ping_id":"<UUID>","echo_ms":<POSITIVE_NUMBER>}
```

### EA debe mostrar:
```
[INFO] 2170628046 | Handshake sent | account=2089126186
[DEBUG] 2170633050 | Ping sent | id=<UUID>
[DEBUG] 2170636053 | Pong received | id=<UUID>,rtt_ms=3
```

### ❌ NO debe aparecer:
- `"Unknown message type from slave"`
- `"timestamp_ms":-<negative_number>`
- `"Pipe reconnection failed"` (ciclo infinito)

---

## 🔍 Cambios Técnicos Detallados

### ULongToStr() en Slave EA
```mql4
string ULongToStr(ulong v)
{
   if(v == 0) return "0";
   string s = "";
   while(v > 0)
   {
      int d = (int)(v % 10);
      s = (string)CharToString('0' + d) + s;
      v /= 10;
   }
   return s;
}
```

### Antes (INCORRECTO):
```mql4
string json = "{\"type\":\"ping\",\"timestamp_ms\":"+IntegerToString((int)ts)+"}";
// GetTickCount() = 4,170,440,968 (ulong)
// (int)ts = -124,526,328 (overflow negativo) ❌
```

### Después (CORRECTO):
```mql4
string json = "{\"type\":\"ping\",\"timestamp_ms\":"+ULongToStr(ts)+"}";
// GetTickCount() = 4,170,440,968 (ulong)
// ULongToStr(ts) = "4170440968" ✅
```

---

## 📊 Archivos en echo/bin/

```bash
$ ls -lh /home/kor/go/src/github.com/xKoRx/echo/bin/

-rwxrwxr-x  19M  echo-agent-windows-amd64.exe  # ✅ ACTUALIZADO
-rwxrwxr-x  85K  echo_pipe_x86.dll             # ✅ OK (sin cambios)
-rw-rw-r--  50K  master.mq4                    # ✅ OK (sin cambios)
-rw-rw-r--  49K  slave.mq4                     # ✅ ACTUALIZADO
```

---

## 🎯 Resultado Esperado

### Conexión exitosa:
```
EA → Handshake → Agent ✓
EA → Ping (id=ABC, ts=4170440968) → Agent ✓
Agent → Pong (id=ABC, echo_ms=4170440968) → EA ✓
EA: "Pong received, RTT=3ms" ✓
```

### Sin desconexiones:
- ✅ Ping/Pong cada 5 segundos
- ✅ Sin timeouts
- ✅ Sin reconexiones innecesarias

---

## 🔧 Si Siguen Problemas

### Verificar versión del Agent:
```powershell
# En logs del Agent, debe aparecer:
{"level":"INFO","msg":"Pong sent",...}
```

Si sigue diciendo "Unknown message type", **el Agent NO se actualizó**.

### Verificar timestamps en EA:
```
# En logs de MT4/MT5:
[DEBUG] 2170628046 | Ping sent | ...
```

Si aparece un número **negativo**, el EA NO se recompiló.

### Limpiar cache de MT4/MT5:
```
1. Cerrar MT4/MT5
2. Borrar: C:\Users\<user>\AppData\Roaming\MetaQuotes\Terminal\<ID>\MQL4\Experts\*.ex4
3. Abrir MT4/MT5
4. Recompilar EAs
```

---

## 📝 Checklist de Despliegue

- [ ] Agent detenido
- [ ] Nuevo binario `echo-agent-windows-amd64.exe` copiado
- [ ] Agent iniciado
- [ ] Logs del Agent muestran handler de ping activo
- [ ] `slave.mq4` actualizado en MetaEditor
- [ ] Slave EA recompilado (0 errors)
- [ ] EAs viejos removidos de gráficos
- [ ] Nuevos EAs adjuntados
- [ ] Logs muestran "Pong sent" y "Pong received"
- [ ] No hay timestamps negativos
- [ ] No hay mensajes "Unknown message type"
- [ ] No hay reconexiones en loop

---

**Fin del documento - Corrección aplicada** ✅

**Próximo paso:** Desplegar y verificar funcionamiento estable por 5 minutos.

