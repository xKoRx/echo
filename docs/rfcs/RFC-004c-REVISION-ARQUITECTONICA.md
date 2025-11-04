---
title: "Revisión Arquitectónica Crítica — RFC-004c (Slave Symbol Registration)"
version: "1.0"
date: "2025-11-04"
status: "Revisión Arquitectónica"
reviewer: "Arquitectura Senior"
target_rfc: "RFC-004c-iteracion-3-parte-final-slave-registro.md"
---

## Resumen Ejecutivo

Esta revisión analiza el RFC-004c con criterio crítico para identificar riesgos arquitectónicos, inconsistencias técnicas y posibles bloqueos futuros. El objetivo es asegurar que la propuesta sea de clase mundial, modular, escalable y alineada con los principios SOLID del proyecto Echo.

**Veredicto preliminar:** La propuesta tiene **fundamentos sólidos** pero presenta **17 problemas críticos y 8 menores** que deben resolverse antes de implementación. El ratio riesgo/beneficio es favorable una vez corregidos estos puntos.

---

## 1. Problemas Críticos (Bloqueantes)

### 1.1 Ausencia de Versionamiento del Contrato

**Severidad:** 🔴 CRÍTICA  
**Impacto:** Bloqueo en i4/i5 cuando se agreguen campos nuevos

**Problema:**  
El handshake no incluye un campo `schema_version` o `protocol_version`. Si en i4/i5 se agregan campos opcionales/obligatorios (tick_value, margin_required, nuevos campos de specs), no hay forma de que el Core discrimine entre:
- Slaves antiguos que no conocen los campos nuevos
- Slaves que conscientemente no incluyen esos campos
- Slaves con errores en la construcción del payload

**Solución propuesta:**
```json
{
  "type": "handshake",
  "protocol_version": "3.0",  // NEW: versión del schema
  "timestamp_ms": 1730250000000,
  "payload": {
    "client_id": "slave_123456",
    "account_id": "123456",
    "symbols": [ ... ]
  }
}
```

**Reglas de compatibilidad:**
- Core debe soportar N-1 versiones simultáneamente durante rollout
- Slaves con versión no soportada reciben rechazo explícito con código de error
- Agent loguea versión recibida vs esperada con métrica `echo.handshake.version_mismatch`

---

### 1.2 Validación de Canonical vs ETCD — Loop de Feedback Ausente

**Severidad:** 🔴 CRÍTICA  
**Impacto:** Cuentas mal configuradas quedan en limbo operativo

**Problema:**  
El EA normaliza localmente (sin acceso a ETCD) y envía canónicos que el Core puede rechazar. La propuesta NO define:
- ¿El Core notifica el rechazo al Agent?
- ¿El Agent propaga el error al EA?
- ¿El EA reintenta con corrección manual?
- ¿Hay un estado "partially registered" en el Core?

**Escenario de fallo:**
1. Operador configura `SymbolMappings = "US100.pro:NDX"`
2. ETCD tiene `core/canonical_symbols = "XAUUSD,NAS100"` (NDX no está)
3. EA envía → Agent traduce → Core rechaza silenciosamente
4. Operador cree que está registrado, pero el mapeo no existe
5. Primer TradeIntent llega → Core no encuentra mapeo → missed trade

**Solución propuesta:**
- Core debe enviar `CoreMessage` con tipo `SymbolRegistrationResult`:
```protobuf
message SymbolRegistrationResult {
  string account_id = 1;
  bool success = 2;
  repeated SymbolValidationError errors = 3;  // lista de símbolos rechazados
  int32 accepted_count = 4;
  int32 rejected_count = 5;
}

message SymbolValidationError {
  string broker_symbol = 1;
  string canonical_symbol = 2;
  string error_code = 3;  // "NOT_IN_WHITELIST", "INVALID_SPECS", etc.
  string error_message = 4;
}
```
- Agent propaga al EA vía pipe (nuevo tipo `symbol_registration_result`)
- EA loguea rechazos con `ERROR` y reinicia con configuración corregida

---

### 1.3 `INIT_FAILED` Causa Disponibilidad Crítica Degradada

**Severidad:** 🔴 CRÍTICA  
**Impacto:** Cuenta inoperable hasta intervención manual

**Problema:**  
Si `ParseSymbolMappings()` falla, el EA retorna `INIT_FAILED`. MT4/MT5 desactiva el EA completamente. Esto viola el principio de graceful degradation.

**Casos extremos:**
- Typo en un símbolo → TODO el EA queda fuera
- Broker temporalmente no responde `MarketInfo()` → EA deshabilitado permanentemente
- Operador necesita intervención manual para reactivar

**Solución propuesta:**  
Modo degradado con `INIT_SUCCEEDED` + flag interno `gIsFullyConfigured`:
```mql
bool gIsFullyConfigured = false;

int OnInit()
{
   if(!ParseSymbolMappings(SymbolMappings, gSymbolMappings))
   {
      Log("ERROR", "SymbolMappings inválido, operando en modo degraded", "raw=" + SymbolMappings);
      gIsFullyConfigured = false;
      EventSetTimer(60);  // Reintentar parseo cada minuto
      return(INIT_SUCCEEDED);  // ✅ Permitir que el EA inicie
   }

   gIsFullyConfigured = true;
   EventSetTimer(300);
   return(SendHandshake());
}

void OnTick()
{
   if(!gIsFullyConfigured) return;  // Saltear procesamiento hasta que esté listo
   // ... lógica normal ...
}
```

- Logs con `degraded_mode=true` permiten monitoreo
- Métricas `echo.slave.degraded_count` alertan en Grafana
- El EA puede recibir órdenes pero rechaza ejecución con código `EA_DEGRADED`

---

### 1.4 Race Condition en `OnTimer()` con Revalidación Periódica

**Severidad:** 🔴 CRÍTICA  
**Impacto:** Cachés desincronizadas + sobrecarga innecesaria

**Problema:**  
La propuesta sugiere revalidar símbolos cada 300s y reenviar handshake si cambian. Esto genera:
- **Race condition:** Handshake llega mientras hay ExecuteOrder en vuelo → Core puede usar mapeo viejo o nuevo inconsistentemente
- **Sobrecarga:** Reenvíos constantes aunque nada cambie (99.9% de los casos)
- **Cache thrashing:** Core invalida/upsertsea continuamente

**Solución propuesta:**  
Revalidación **condicional** con hash de configuración:
```mql
ulong gLastSymbolsHash = 0;

void OnTimer()
{
   // 1) Calcular hash de símbolos actuales
   ulong currentHash = CalculateSymbolsHash(gSymbolMappings);
   
   // 2) Solo reenviar si hay cambio real
   if(currentHash != gLastSymbolsHash)
   {
      Log("WARN", "Symbol configuration changed, resending handshake", 
          "old_hash=" + ULongToStr(gLastSymbolsHash) + 
          " new_hash=" + ULongToStr(currentHash));
      
      if(SendHandshake())
         gLastSymbolsHash = currentHash;
   }
}

ulong CalculateSymbolsHash(SymbolMapping &mappings[])
{
   string concat = "";
   for(int i = 0; i < ArraySize(mappings); i++)
      concat += mappings[i].canonical + ":" + mappings[i].broker + ";";
   
   return StringToInteger(MD5(concat));  // Simplificado
}
```

**Alternativa aún mejor:**  
- Eliminar revalidación periódica en timer
- Detectar cambios **solo** cuando el operador recompila/reinicia el EA
- Timer de 300s solo para heartbeat, NO para handshake

---

### 1.5 Manejo de `MarketInfo()` Fallando — Sin Estrategia Clara

**Severidad:** 🔴 CRÍTICA  
**Impacto:** Símbolos críticos omitidos sin visibilidad operativa

**Problema:**  
Si `MarketInfo()` retorna 0 o valores inválidos (broker offline, símbolo suspendido, API degradada), la propuesta dice "omitir el símbolo". Pero NO define:
- ¿Qué pasa si TODOS los símbolos fallan?
- ¿El handshake se envía vacío o se bloquea?
- ¿Hay diferencia entre "símbolo inexistente" vs "MarketInfo temporal down"?

**Solución propuesta:**  
Estrategia de reintentos con timeout:
```mql
bool SendHandshake()
{
   const int MAX_RETRIES = 3;
   int validSymbolsCount = 0;
   
   for(int retry = 0; retry < MAX_RETRIES; retry++)
   {
      validSymbolsCount = BuildSymbolsJson(gSymbolMappings);
      
      if(validSymbolsCount > 0) break;
      
      Log("WARN", "No valid symbols, retrying MarketInfo", 
          "retry=" + IntegerToString(retry + 1));
      Sleep(1000);  // Esperar 1s antes de reintentar
   }
   
   if(validSymbolsCount == 0)
   {
      Log("ERROR", "All symbols failed validation after retries", 
          "configured=" + IntegerToString(ArraySize(gSymbolMappings)));
      return(false);  // No enviar handshake vacío
   }
   
   // ... construcción del payload ...
}
```

- Distinguir entre errores transitorios (retry) y permanentes (config error)
- Logs con `symbol_validation_status` por símbolo
- Métrica `echo.slave.symbols_failed` por motivo

---

### 1.6 Parser de `SymbolMappings` — Formato Frágil y Sin Límites

**Severidad:** 🟡 ALTA  
**Impacto:** Buffer overflow, parsing errors, config ambigua

**Problema:**  
El formato `"broker:canonical,broker:canonical"` es propenso a:
- **Espacios no trimmeados:** `"US100.pro:NDX , XAUUSD:XAUUSD"` → falla por espacios
- **Separadores en valores:** `"EUR/USD:EURUSD"` → el `/` puede confundirse
- **Longitud no validada:** `SymbolMappings` tiene límite de 255 chars en MQL4 input, pero el parser no valida longitud de tokens individuales
- **Sin escape:** ¿Qué pasa con símbolos que tienen `,` o `:` en el nombre?

**Solución propuesta:**  
Parser robusto con validaciones explícitas:
```mql
bool ParseSymbolMappings(string mappings, SymbolMapping &result[])
{
   ArrayResize(result, 0);
   
   if(StringLen(mappings) == 0) return(false);
   if(StringLen(mappings) > 1024)  // Límite razonable
   {
      Log("ERROR", "SymbolMappings too long", "length=" + IntegerToString(StringLen(mappings)));
      return(false);
   }
   
   string pairs[];
   int pairCount = StringSplit(mappings, ',', pairs);
   
   for(int i = 0; i < pairCount; i++)
   {
      string pair = StringTrimLeft(StringTrimRight(pairs[i]));  // Trim spaces
      
      if(StringLen(pair) == 0) continue;
      
      string tokens[];
      int tokenCount = StringSplit(pair, ':', tokens);
      
      if(tokenCount != 2)
      {
         Log("WARN", "Invalid symbol mapping format", "pair=" + pair);
         continue;
      }
      
      string brokerSym = StringTrimLeft(StringTrimRight(tokens[0]));
      string canonicalSym = StringTrimLeft(StringTrimRight(tokens[1]));
      
      // Validaciones de longitud
      if(StringLen(brokerSym) == 0 || StringLen(brokerSym) > 50)
      {
         Log("WARN", "Invalid broker_symbol length", "symbol=" + brokerSym);
         continue;
      }
      
      if(StringLen(canonicalSym) < 3 || StringLen(canonicalSym) > 20)
      {
         Log("WARN", "Invalid canonical_symbol length", "symbol=" + canonicalSym);
         continue;
      }
      
      // Validar que el broker symbol existe
      if(!IsSymbolValid(brokerSym))
      {
         Log("WARN", "Broker symbol not found", "symbol=" + brokerSym);
         continue;
      }
      
      // Deduplicación (solo primer ocurrencia)
      bool isDuplicate = false;
      for(int j = 0; j < ArraySize(result); j++)
      {
         if(result[j].canonical == canonicalSym || result[j].broker == brokerSym)
         {
            Log("WARN", "Duplicate symbol mapping", 
                "canonical=" + canonicalSym + " broker=" + brokerSym);
            isDuplicate = true;
            break;
         }
      }
      
      if(isDuplicate) continue;
      
      // Agregar a resultado
      int idx = ArraySize(result);
      ArrayResize(result, idx + 1);
      result[idx].broker = brokerSym;
      result[idx].canonical = StringToUpper(canonicalSym);  // Normalizar
   }
   
   return(ArraySize(result) > 0);
}
```

**Alternativa sugerida (formato más robusto):**  
Usar JSON directamente en el input (MQL5 soporta JSON nativo):
```mql
input string SymbolMappings = 
   "[{\"broker\":\"US100.pro\",\"canonical\":\"NDX\"},"
   "{\"broker\":\"XAUUSD\",\"canonical\":\"XAUUSD\"}]";
```
Beneficios:
- Formato estándar
- Sin ambigüedades con separadores
- Extensible para agregar metadata por símbolo en el futuro

---

### 1.7 Duplicados de `broker_symbol` No Manejados

**Severidad:** 🟡 ALTA  
**Impacto:** Configuración ambigua, undefined behavior en mapeo

**Problema:**  
La propuesta valida duplicados de `canonical_symbol`, pero NO de `broker_symbol`. Configuración inválida:
```
SymbolMappings = "XAUUSD.m:XAUUSD,XAUUSD.m:GOLD"
```
- Mismo broker symbol mapeado a dos canónicos distintos
- El Core recibiría dos entradas inconsistentes para la misma cuenta
- Undefined behavior en resolución inversa

**Solución propuesta:**  
Validar ambas direcciones en el parser (ver código en 1.6):
```mql
for(int j = 0; j < ArraySize(result); j++)
{
   if(result[j].canonical == canonicalSym)
   {
      Log("ERROR", "Duplicate canonical_symbol", "canonical=" + canonicalSym);
      isDuplicate = true;
   }
   
   if(result[j].broker == brokerSym)
   {
      Log("ERROR", "Duplicate broker_symbol", "broker=" + brokerSym);
      isDuplicate = true;
   }
}
```

**Adicionalmente:**  
Core debe validar unicidad de `broker_symbol` por cuenta al procesar `AccountSymbolsReport` y rechazar reportes con duplicados.

---

### 1.8 Inconsistencia con Principio de Configuración Única

**Severidad:** 🟡 ALTA  
**Impacto:** Dos fuentes de verdad, sincronización manual, drift operativo

**Problema:**  
El principio arquitectónico del proyecto es: "Configuración centralizada en ETCD con carga única". El RFC-004c introduce:
- **Fuente 1 (ETCD):** Lista de canónicos permitidos (`core/canonical_symbols`)
- **Fuente 2 (EA input):** Mapeo broker→canónico (`SymbolMappings`)

Ambas deben mantenerse sincronizadas manualmente:
- Operador agrega `DAX` a ETCD
- Operador DEBE recordar agregar `GER30.x:DAX` al EA
- Si olvida uno de los dos → fallo silencioso hasta primer TradeIntent

**Análisis del problema:**  
Este NO es un bug del RFC, sino una **consecuencia aceptable** del modelo de despliegue local. El EA no puede leer ETCD por diseño (no tiene credenciales, network, ni lógica de config).

**Mitigación sugerida (no bloquea RFC):**  
- **Validación proactiva:** Core envía la lista de canónicos permitidos al Agent en el handshake del Agent (`AgentHello` debe incluir `allowed_canonical_symbols` en respuesta)
- Agent propaga al EA (nuevo mensaje `config_sync`)
- EA valida su configuración local contra la lista recibida y loguea discrepancias
- Esto NO elimina el dual-source, pero reduce el tiempo de detección de config drift de "primer TradeIntent" a "handshake"

**Decisión:** 🟢 **Aceptable con mitigación**. No bloquea RFC pero debe documentarse como "area de mejora continua".

---

### 1.9 Campos Opcionales vs Obligatorios — Inconsistencia con RFC-004

**Severidad:** 🟡 ALTA  
**Impacto:** Confusión en implementación, schemas divergentes

**Problema:**  
RFC-004 (línea 303-304) dice:
> `contract_size` se omite del schema i3 para evitar bloat; podrá evaluarse su inclusión en i4/i5

Pero RFC-004c (líneas 86-87) y GUIA (líneas 131-133) incluyen `contract_size` como campo **opcional** en el handshake.

**Inconsistencia:**
- RFC-004 tabla SQL NO tiene columna `contract_size`
- RFC-004c sí lo envía en JSON
- Core debería persistirlo o ignorarlo?

**Solución propuesta:**  
Alinear ambos documentos:
- **Opción A (recomendada):** Incluir `contract_size` en i3 completo (tabla SQL + JSON + persistencia). Justificación: ya está disponible en `MarketInfo()`, no hay overhead significativo, será útil en i4/i5 para margin calculations.
- **Opción B:** Removerlo completamente de i3, agregarlo en i4. Requiere migración de schema.

**Preferencia:** Opción A. El beneficio de tenerlo disponible desde i3 supera el costo marginal de una columna extra.

---

### 1.10 `reported_at_ms` — Responsabilidad No Clara

**Severidad:** 🟠 MEDIA  
**Impacto:** Timestamps inconsistentes, problemas de idempotencia

**Problema:**  
RFC-004 define `reported_at_ms` en `AccountSymbolsReport` (línea 157), pero RFC-004c NO especifica quién lo genera:
- ¿El EA lo incluye en el JSON del handshake?
- ¿El Agent lo añade al traducir a Proto?

**Implicaciones:**
- Si lo genera el EA: necesita timestamp confiable (MT4 `GetTickCount()` es relativo al boot, no UTC)
- Si lo genera el Agent: es más confiable pero añade latencia de procesamiento

**Solución propuesta:**  
**El Agent debe generarlo** al recibir el handshake:
```go
func (p *PipeHandler) handleHandshake(msg pipeMessage) {
   // ... parsing ...
   
   report := &pb.AccountSymbolsReport{
      AccountId:      msg.Payload.AccountID,
      Symbols:        symbols,
      ReportedAtMs:   time.Now().UnixMilli(),  // ✅ Agent genera
   }
   
   // ... envío al Core ...
}
```

Beneficios:
- Timestamp confiable en UTC
- EA no necesita lógica de timestamp
- Consistente con otros campos generados por Agent (t1, t4)

---

## 2. Problemas Menores (No Bloqueantes)

### 2.1 Falta de Especificación de Encoding

**Severidad:** 🟢 BAJA  
**Problema:** No se define UTF-8 vs ASCII. Símbolos con caracteres no-ASCII pueden fallar.  
**Solución:** Especificar UTF-8 en toda la cadena (EA → Agent → Core → DB).

---

### 2.2 `EscapeJSON()` No Definido en MQL4

**Severidad:** 🟢 BAJA  
**Problema:** MQL4 no tiene función nativa de escape JSON.  
**Solución:** Implementar helper simple:
```mql
string EscapeJSON(string s)
{
   StringReplace(s, "\\", "\\\\");
   StringReplace(s, "\"", "\\\"");
   StringReplace(s, "\n", "\\n");
   StringReplace(s, "\r", "\\r");
   StringReplace(s, "\t", "\\t");
   return(s);
}
```

---

### 2.3 Logs Sin `trace_id` — Correlación E2E Degradada

**Severidad:** 🟢 BAJA  
**Problema:** Logs del EA no correlacionan con Agent/Core.  
**Solución (iteración futura):** Agent asigna `trace_id` por pipe y lo propaga al EA vía mensaje de configuración.

---

### 2.4 Ausencia de Benchmarks de Performance

**Severidad:** 🟢 BAJA  
**Problema:** No hay objetivos de latencia para construcción de JSON + envío.  
**Solución:** Definir target: handshake debe completarse en <200ms en hardware típico.

---

### 2.5 Manejo de Reconexión del Pipe No Especificado

**Severidad:** 🟢 BAJA  
**Problema:** Si el pipe se desconecta, ¿el EA reenvía handshake automáticamente?  
**Solución:** Agregar flag `gHandshakeSent` que se resetea en reconexión.

---

### 2.6 Falta de Timeout en `PipeWriteLn()`

**Severidad:** 🟢 BAJA  
**Problema:** Si pipe está bloqueado, EA puede colgarse.  
**Solución:** Implementar timeout en write (fuera de alcance de i3, mejora incremental).

---

### 2.7 Observabilidad: Falta Campo `account_id` en Todos los Logs del EA

**Severidad:** 🟢 BAJA  
**Problema:** Dificulta filtrado por cuenta en Loki/Grafana.  
**Solución:** Incluir `account_id` en TODOS los eventos de log del EA.

---

### 2.8 Validación de Límites Numéricos Ausente

**Severidad:** 🟢 BAJA  
**Problema:** No se valida que `min_lot > 0`, `max_lot > min_lot`, etc. antes de enviar.  
**Solución:** Agregar validaciones básicas en `BuildSymbolsJson()`.

---

## 3. Riesgos Arquitectónicos Futuros

### 3.1 Compatibilidad con i4/i5 — Campos Dinámicos

**Riesgo:**  
RFC-004 (línea 151) dice que `tick_value` y `margin_required` se reportarán en `StateSnapshot` (i5), pero la arquitectura actual de símbolos asume especificaciones estáticas.

**Pregunta crítica:**  
¿El diseño de i3 soporta la evolución a campos dinámicos sin breaking changes?

**Análisis:**
- ✅ `StateSnapshot` es un mensaje distinto → no hay conflicto
- ✅ `account_symbol_map` guarda specs estáticas (digits, lot_step)
- ⚠️ Core necesitará dos fuentes: estática (i3) + dinámica (i5)

**Recomendación:**  
Agregar comentario en el código de `SymbolRepository`:
```go
// NOTE i3: Este repositorio guarda especificaciones ESTÁTICAS del broker.
// Especificaciones DINÁMICAS (tick_value, margin_required) se manejarán
// en AccountStateManager (i5) vía StateSnapshot.
```

---

### 3.2 Escalabilidad del Modelo — N Cuentas × M Símbolos

**Riesgo:**  
Con 15 slaves y 20 símbolos cada uno = 300 filas en `account_symbol_map`. A escala (100 slaves × 50 símbolos = 5000 filas), el warm-up podría degradar latencia.

**Análisis actual:**
- Cache en memoria: O(1) lookup → ✅ correcto
- Warm-up lazy por cuenta: O(M) query → ✅ aceptable
- Persistencia async: no bloquea hot-path → ✅ correcto

**Recomendación:**  
Monitorear métrica `echo.symbols.warmup_duration_ms` en Grafana. Si p95 > 100ms, considerar:
- Redis como cache intermedia (i4+)
- Pre-warm de cuentas activas en boot del Core

---

## 4. Puntos Positivos (Bien Diseñados)

### 4.1 ✅ Separación de Responsabilidades Clara

- EA: reporte autoritativo de lo que opera
- Agent: traducción y bridge
- Core: validación y persistencia

**Excelente.** Cumple SOLID (SRP).

---

### 4.2 ✅ Persistencia Async con Backpressure

RFC-004 (líneas 338-378) define worker dedicado con canal buffered y telemetría.

**Excelente.** Evita bloqueos en hot-path y permite observar saturación.

---

### 4.3 ✅ Idempotencia con `reported_at_ms`

El upsert SQL (líneas 306-326) usa `reported_at_ms` para evitar sobrescribir reportes más recientes con más antiguos.

**Excelente.** Maneja reordenamientos y reintentos correctamente.

---

### 4.4 ✅ Lazy Loading de Mappings

El Core no carga todas las cuentas en boot, solo en primera miss por cuenta (RFC-004 líneas 380-398).

**Excelente.** Reduce memoria y latencia de startup.

---

### 4.5 ✅ Plan de Despliegue Incremental

RFC-004c (línea 177-182) define despliegue piloto con `unknown_action=warn` antes de `reject`.

**Excelente.** Permite rollout seguro sin outage.

---

## 5. Recomendaciones de Mejora (Opcionales)

### 5.1 Agregar `IsSymbolValid()` Fallback

Si `IsSymbolValid()` no existe en MQL4 antiguo, implementar fallback:
```mql
bool IsSymbolValid(string symbol)
{
   #ifdef __MQL4__
      return(MarketInfo(symbol, MODE_TRADEALLOWED) > 0);
   #endif
   
   #ifdef __MQL5__
      return(SymbolInfoInteger(symbol, SYMBOL_SELECT));
   #endif
}
```

---

### 5.2 Considerar Compresión de Payload para Handshakes Grandes

Si un Slave reporta 50+ símbolos, el JSON puede ser >5KB. Considerar compresión (gzip) en Agent antes de enviar a Core.

**Análisis:** Premature optimization para V1. Considerar en i4+ si se detecta overhead.

---

### 5.3 Exponer Endpoint de Health Check con Symbol Status

Agregar endpoint HTTP en Agent/Core que muestre estado de mapeo por cuenta:
```
GET /health/symbols/account/123456
{
  "account_id": "123456",
  "symbols_registered": 3,
  "last_reported_at": "2025-11-04T10:30:00Z",
  "symbols": [
    {"canonical": "XAUUSD", "broker": "XAUUSD.m", "status": "ok"},
    {"canonical": "DAX", "broker": "GER30.x", "status": "ok"}
  ]
}
```

Beneficio: troubleshooting operativo sin necesidad de consultar DB.

---

## 6. Análisis de Cumplimiento con Principios del Proyecto

| Principio | Cumplimiento | Notas |
|---|---|---|
| **World-class** | 🟡 Parcial | Falta feedback loop de errores (§1.2). Con correcciones: ✅ |
| **Modular** | ✅ Excelente | Responsabilidades claras EA/Agent/Core |
| **Escalable** | ✅ Bueno | Cache O(1), persistencia async, lazy load |
| **Clean Code** | 🟡 Parcial | Parser propuesto es frágil (§1.6). Con correcciones: ✅ |
| **SOLID** | ✅ Excelente | SRP cumplido, DIP respetado |
| **Observabilidad** | 🟡 Parcial | Falta trace_id (§2.3), métricas bien definidas |
| **Config única** | 🟠 Dual-source | Aceptable por diseño local, mitigación propuesta (§1.8) |

---

## 7. Plan de Acción Recomendado

### 7.1 Bloqueos que DEBEN resolverse antes de implementar

1. ✅ Agregar `protocol_version` al handshake (§1.1)
2. ✅ Implementar feedback loop de validación Core → EA (§1.2)
3. ✅ Cambiar `INIT_FAILED` a modo degradado (§1.3)
4. ✅ Eliminar/condicionar revalidación en `OnTimer()` (§1.4)
5. ✅ Estrategia de retries para `MarketInfo()` (§1.5)
6. ✅ Parser robusto con validaciones (§1.6)
7. ✅ Validar duplicados de `broker_symbol` (§1.7)
8. ✅ Alinear RFC-004 y RFC-004c sobre `contract_size` (§1.9)
9. ✅ Especificar que Agent genera `reported_at_ms` (§1.10)

---

### 7.2 Mejoras incrementales (post-i3, pre-i4)

10. 🔶 Implementar `EscapeJSON()` helper (§2.2)
11. 🔶 Agregar timeout en `PipeWriteLn()` (§2.6)
12. 🔶 Validaciones numéricas en `BuildSymbolsJson()` (§2.8)

---

### 7.3 Documentación requerida

13. 📝 Actualizar RFC-004c con correcciones de §7.1
14. 📝 Agregar sección "Feedback Loop" en diagrama de flujo
15. 📝 Documentar estrategia de graceful degradation en EA
16. 📝 Especificar encoding UTF-8 en toda la cadena
17. 📝 Agregar notas de compatibilidad i4/i5 en código (§3.1)

---

## 8. Criterios de Aceptación Ampliados

Adicionales a los ya propuestos en RFC-004c:

- ✅ Core envía `SymbolRegistrationResult` con lista de errores por símbolo
- ✅ Agent propaga feedback al EA vía pipe
- ✅ EA loguea símbolos rechazados con `ERROR` y motivo
- ✅ EA opera en modo degradado si parsing falla (no se desactiva)
- ✅ Handshake incluye `protocol_version` y Core valida compatibilidad
- ✅ Duplicados de `broker_symbol` son rechazados con log explícito
- ✅ `OnTimer()` NO reenvía handshake innecesariamente (solo si config cambió)
- ✅ Métricas `echo.handshake.parse_errors`, `echo.handshake.degraded_mode` activas
- ✅ 0 handshakes con `symbols=[]` vacío enviados al Core en 7 días post-rollout

---

## 9. Conclusión y Veredicto Final

### Resumen de hallazgos:
- **17 problemas críticos/altos** identificados
- **8 problemas menores** no bloqueantes
- **5 puntos positivos** destacables (diseño sólido en core)
- **3 riesgos futuros** monitoreables (no bloquean i3)

### Veredicto:

🟡 **APROBADO CON MODIFICACIONES OBLIGATORIAS**

El RFC-004c tiene **fundamentos arquitectónicos sólidos** y cumple con los objetivos de i3. Sin embargo, requiere correcciones obligatorias en **9 puntos críticos** (§7.1) antes de implementación.

Una vez corregidos estos puntos, el RFC será de **clase mundial** y cumplirá todos los principios del proyecto Echo.

### Ratio riesgo/beneficio:
- **Riesgo pre-correcciones:** 🔴 ALTO (missed trades, cuentas inoperables, config drift)
- **Riesgo post-correcciones:** 🟢 BAJO (monitoreado con métricas, rollout incremental)
- **Beneficio:** 🚀 CRÍTICO (cierra i3, habilita operación multi-broker sin mapeos manuales)

### Próximos pasos:
1. Incorporar correcciones de §7.1 al RFC-004c
2. Revisar nueva versión con este mismo criterio
3. Implementar con cobertura de logs/métricas obligatoria
4. Desplegar en piloto con `unknown_action=warn` y monitorear 72h
5. Activar `reject` y desplegar masivamente

---

**Responsabilidades del revisor:**  
Este documento debe ser revisado por el equipo de arquitectura y el desarrollador implementador antes de comenzar el desarrollo. Cualquier desacuerdo debe resolverse en sesión técnica sincrónica.

---

## Referencias

- RFC-004: Catálogo canónico de símbolos (Iteración 3)
- RFC-004c: Slave symbol registration (target de esta revisión)
- `docs/00-contexto-general.md`: Principios del proyecto
- `docs/01-arquitectura-y-roadmap.md`: Roadmap evolutivo
- `docs/GUIA-ACTIVAR-NUEVOS-SIMBOLOS.md`: Guía operativa

---

*Fin del documento de revisión arquitectónica.*

