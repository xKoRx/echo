---
title: "Revisión Final RFC-004c — Enfoque en Requerimientos Reales de i3"
version: "1.0"
date: "2025-11-04"
status: "Análisis crítico alineado a scope"
reviewer: "Arquitectura Senior"
target_rfc: "RFC-004c-iteracion-3-parte-final-slave-registro.md"
---

## Resumen Ejecutivo

Esta revisión analiza el RFC-004c actualizado con enfoque estricto en **cumplir los requerimientos de la iteración 3** sin agregar funcionalidad fuera de alcance. El análisis identifica **5 problemas reales**, **3 sobre-implementaciones** y **2 faltantes críticos** respecto al scope original.

**Veredicto:** El RFC tiene **exceso de funcionalidad** (versionamiento, feedback loop, modo degradado) que NO está en los requerimientos de i3, y le **faltan componentes críticos** (reporting de precios 250ms, reconexión, limpieza de buffers) que SÍ están definidos en RFC-001.

---

## 1. Requerimientos Oficiales de la Iteración 3

### Según `01-arquitectura-y-roadmap.md` (líneas 157-162):
```
Iteración 3 — Catálogo canónico de símbolos y mapeo por cuenta
- Objetivo: estandarizar canonical_symbol ⇄ broker_symbol por cuenta.
- Alcance: catálogo canónico en Core; agent/EA reportan mapeo del broker al conectar; validaciones previas al envío.
- Exclusiones: sizing y políticas.
- Criterios de salida: 0 errores por símbolo desconocido; mapeo persistido y trazable.
```

### Según RFC-001 (líneas 437-445):
```
Iteración 3 (2 días)
- Mapeo símbolos (canonical ⇄ broker)
- Specs de broker (min_lot, stop_level, etc.)
- Las EAs deben informar las especificaciones de símbolos del broker
- Reporting de precios cada 250ms (Slaves reportan Bid/Ask)
- Reconexión automática ea-agent y agent-core
- Limpiar los buffers de operaciones luego de que cierre una operación
- Validación de StopLevel en Core
```

---

## 2. Análisis: Qué está FUERA de Scope pero está en el RFC

### 2.1 ❌ Versionamiento del Protocolo (`protocol_version=3.0`)

**En el RFC-004c:** Líneas 49, 108-116, 118  
**Requerimiento de i3:** NO existe

**Análisis:**
- El versionamiento es una mejora arquitectónica válida, PERO no está en los requerimientos de i3
- Agregar esto aumenta complejidad de implementación sin estar en el scope
- Puede implementarse en i4+ cuando realmente se necesite evolucionar el schema

**Recomendación:** **REMOVER** de i3. Si se necesita en el futuro, agregarlo cuando corresponda.

---

### 2.2 ❌ Feedback Loop Completo (Core→Agent→EA)

**En el RFC-004c:** Líneas 50, 53, 126-145, 235-247  
**Requerimiento de i3:** NO existe

**Análisis:**
- RFC-004 original NO menciona feedback activo al EA
- El criterio de salida de i3 es "0 errores por símbolo desconocido" desde la perspectiva del Core, no del EA
- El feedback operativo puede lograrse mediante:
  - Logs del Core (si rechaza símbolos)
  - Métricas en Grafana
  - Estado en PostgreSQL consultable

**Recomendación:** **SIMPLIFICAR**. El feedback puede ser pasivo (logs + métricas) en i3. Si se requiere activo, agregarlo en i4+ cuando se implemente UI/CLI de configuración.

---

### 2.3 ❌ Modo Degradado Complejo con Hash

**En el RFC-004c:** Líneas 99-104, 162-223  
**Requerimiento de i3:** NO existe

**Análisis:**
- El modo degradado con `gLastSymbolsHash` y reintentos en `OnTimer()` es sobre-ingeniería para i3
- El requerimiento simplemente dice: "EA reporta mapeo al conectar"
- Si el parsing falla en `OnInit()`, el EA simplemente NO reporta nada y el Core opera en modo `warn`

**Recomendación:** **SIMPLIFICAR**. En i3:
```mql
int OnInit()
{
   if(!ParseSymbolMappings(SymbolMappings, gSymbolMappings))
   {
      Log("ERROR", "SymbolMappings inválido");
      // NO enviar handshake con símbolos
      return(INIT_SUCCEEDED); // Permitir que el EA arranque sin símbolos
   }
   
   SendHandshake(); // Enviar una sola vez al inicio
   return(INIT_SUCCEEDED);
}
```
No es necesario hash, ni timer, ni reintentos complejos en i3.

---

## 3. Análisis: Qué FALTA pero debería estar en i3

### 3.1 🔴 Reporting de Precios cada 250ms

**Requerimiento RFC-001:** "Slaves reportan Bid/Ask actual cada 250ms (coalesced)"  
**En RFC-004c:** NO existe

**Análisis:**
- Este es un requerimiento EXPLÍCITO de i3 en RFC-001
- Es crítico para que el Core tenga precios actualizados por cuenta
- Se usa para cálculo de sizing óptimo y timing de entrada

**Solución:**
- Agent debe tener un `StateSnapshot` periódico (250ms) por cuenta
- Slave EA debe reportar en `OnTick()` coalescido:
```mql
void OnTick()
{
   static datetime lastReport = 0;
   datetime now = TimeCurrent();
   
   if(now - lastReport < 0.25) return; // Coalesce 250ms
   lastReport = now;
   
   string prices = 
      "{"
      +"\"type\":\"state_snapshot\","
      +"\"timestamp_ms\":" + ULongToStr(GetTickCount()) + ","
      +"\"payload\":{"
      +"\"account_id\":\"" + IntegerToString(AccountNumber()) + "\","
      +"\"bid\":" + DoubleToString(MarketInfo(Symbol(), MODE_BID), Digits) + ","
      +"\"ask\":" + DoubleToString(MarketInfo(Symbol(), MODE_ASK), Digits) + ","
      +"\"symbol\":\"" + Symbol() + "\""
      +"}"
      +"}";
   
   PipeWriteLn(prices);
}
```

**Impacto:** CRÍTICO. Sin esto, el Core no tiene visibilidad de precios actuales por cuenta.

---

### 3.2 🔴 Reconexión Automática EA↔Agent y Agent↔Core

**Requerimiento RFC-001:** "Reconexión automática ea-agent y agent-core"  
**En RFC-004c:** NO existe

**Análisis:**
- RFC-001 dice explícitamente: "Las EAs una vez que pierden la conexión no se vuelven a comunicar nunca más con el agente. Validar esto mismo con core-agent"
- Este es un bug conocido que DEBE arreglarse en i3

**Solución para EA:**
```mql
bool gPipeConnected = false;

void OnTimer()
{
   if(!gPipeConnected)
   {
      if(ReconnectPipe())
      {
         gPipeConnected = true;
         SendHandshake(); // Reenviar handshake después de reconectar
      }
   }
}
```

**Solución para Agent:** Ya existe lógica de reconexión gRPC en Agent, solo falta validar que funcione correctamente.

**Impacto:** CRÍTICO. Sin reconexión, cualquier desconexión temporal deja cuentas inoperables.

---

### 3.3 🟡 Limpieza de Buffers al Cerrar Operación

**Requerimiento RFC-001:** "Limpiar los buffers de operaciones luego de que cierre una operación en EA, agent y core"  
**En RFC-004c:** NO existe

**Análisis:**
- Este es un leak de memoria conocido que crece con cada operación
- Es menos crítico que los anteriores pero debería estar en i3

**Impacto:** MEDIO. Puede causar degradación de performance en operación prolongada.

---

### 3.4 🟡 Validación de StopLevel en Core

**Requerimiento RFC-001:** "Validación de StopLevel en Core: Validar que SL/TP cumplen stop_level antes de enviar"  
**En RFC-004c:** NO existe

**Análisis:**
- RFC-004c menciona que se reporta `stop_level` (línea 95), pero NO hay lógica en el Core para validarlo
- Según RFC-001, el Core debe validar ANTES de enviar `ExecuteOrder`

**Impacto:** MEDIO. Sin esto, pueden enviarse órdenes que serán rechazadas por el broker.

---

## 4. Problemas Reales del RFC (alineados a scope de i3)

### 4.1 🔴 Parser sin Validación de Duplicados en Implementación

**Problema:** Líneas 85-90 dicen "deduplicación bidireccional", pero el pseudocódigo NO lo implementa

**Solución:**
```mql
bool ParseSymbolMappings(string mappings, SymbolMapping &result[])
{
   ArrayResize(result, 0);
   string pairs[];
   int pairCount = StringSplit(mappings, ',', pairs);
   
   for(int i = 0; i < pairCount; i++)
   {
      string tokens[];
      if(StringSplit(pairs[i], ':', tokens) != 2) continue;
      
      string broker = StringTrimLeft(StringTrimRight(tokens[0]));
      string canonical = StringToUpper(StringTrimLeft(StringTrimRight(tokens[1])));
      
      // Validar duplicados
      for(int j = 0; j < ArraySize(result); j++)
      {
         if(result[j].broker == broker)
         {
            Log("ERROR", "Duplicate broker_symbol", "symbol=" + broker);
            return(false); // Parser falla completamente si hay duplicados
         }
         if(result[j].canonical == canonical)
         {
            Log("ERROR", "Duplicate canonical_symbol", "symbol=" + canonical);
            return(false);
         }
      }
      
      int idx = ArraySize(result);
      ArrayResize(result, idx + 1);
      result[idx].broker = broker;
      result[idx].canonical = canonical;
   }
   
   return(ArraySize(result) > 0);
}
```

---

### 4.2 🔴 Reintentos de `MarketInfo()` sin Lógica de Backoff

**Problema:** Línea 94 dice "hasta tres reintentos (sleep 1 s)", pero no especifica si es backoff fijo o exponencial

**Solución:** Para i3, backoff fijo de 1s es suficiente:
```mql
string BuildSymbolsJson(SymbolMapping &mappings[])
{
   const int MAX_RETRIES = 3;
   string json = "";
   
   for(int i = 0; i < ArraySize(mappings); i++)
   {
      string sym = mappings[i].broker;
      bool success = false;
      
      for(int retry = 0; retry < MAX_RETRIES && !success; retry++)
      {
         double point = MarketInfo(sym, MODE_POINT);
         if(point > 0)
         {
            success = true;
            // ... construir JSON ...
         }
         else
         {
            Sleep(1000); // 1 segundo fijo
         }
      }
      
      if(!success)
      {
         Log("WARN", "MarketInfo failed after retries", "symbol=" + sym);
         continue; // Omitir este símbolo
      }
   }
   
   return(json);
}
```

---

### 4.3 🟡 Persistencia de `contract_size` pero sin Migración de Esquema

**Problema:** Línea 123 dice "nueva columna NUMERIC(18,8)", pero no se especifica la migración SQL completa

**Solución:**
```sql
-- Migración i3: Agregar contract_size
ALTER TABLE echo.account_symbol_map 
ADD COLUMN IF NOT EXISTS contract_size NUMERIC(18,8) DEFAULT NULL;

-- Constraint de unicidad por broker_symbol
ALTER TABLE echo.account_symbol_map 
DROP CONSTRAINT IF EXISTS account_symbol_map_pkey;

ALTER TABLE echo.account_symbol_map 
ADD CONSTRAINT account_symbol_map_pkey 
PRIMARY KEY (account_id, broker_symbol);

-- Índice para búsqueda por canónico
CREATE INDEX IF NOT EXISTS idx_account_symbol_map_canonical 
ON echo.account_symbol_map(account_id, canonical_symbol);
```

**Nota:** El constraint debe ser por `broker_symbol`, NO por `canonical_symbol`, ya que un broker puede mapear múltiples símbolos al mismo canónico en diferentes cuentas.

---

### 4.4 🟡 Agent debe Traducir `handshake` a `AccountSymbolsReport` pero no está el Código

**Problema:** RFC-004c dice que Agent traduce, pero no muestra el código

**Solución (pseudo-Go):**
```go
func (p *PipeHandler) handleHandshake(msg pipeMessage) {
    // ... validar protocol_version ...
    
    symbols := make([]*pb.SymbolMapping, 0, len(msg.Payload.Symbols))
    
    for _, s := range msg.Payload.Symbols {
        symbols = append(symbols, &pb.SymbolMapping{
            CanonicalSymbol: s.Canonical,
            BrokerSymbol:    s.Broker,
            Digits:          int32(s.Digits),
            Point:           s.Point,
            TickSize:        s.TickSize,
            MinLot:          s.MinLot,
            MaxLot:          s.MaxLot,
            LotStep:         s.LotStep,
            StopLevel:       int32(s.StopLevel),
            ContractSize:    s.ContractSize, // Puede ser nil
        })
    }
    
    report := &pb.AccountSymbolsReport{
        AccountId:     msg.Payload.AccountID,
        Symbols:       symbols,
        ReportedAtMs:  time.Now().UnixMilli(),
    }
    
    // Enviar al Core vía gRPC
    p.sendToCore(&pb.AgentMessage{
        AgentId:     p.agentID,
        TimestampMs: time.Now().UnixMilli(),
        Payload:     &pb.AgentMessage_AccountSymbolsReport{AccountSymbolsReport: report},
    })
}
```

---

### 4.5 🟢 Logs JSON bien Especificados

**Evaluación:** Líneas 149, 262 definen correctamente los logs estructurados.

**Aprobado:** Este punto SÍ cumple con los requerimientos del proyecto.

---

## 5. Propuesta de Ajustes para i3 Realista

### 5.1 Remover Funcionalidad Fuera de Scope

- ❌ Remover `protocol_version` (no está en requerimientos de i3)
- ❌ Remover feedback loop `SymbolRegistrationResult` (no está en requerimientos de i3)
- ❌ Simplificar modo degradado (sin hash, sin timer de revalidación)

### 5.2 Agregar Funcionalidad Faltante

- ✅ Agregar reporting de precios cada 250ms (`StateSnapshot`)
- ✅ Agregar reconexión automática EA↔Agent
- ✅ Agregar limpieza de buffers al cerrar operaciones
- ✅ Agregar validación de StopLevel en Core (opcional, puede ser i4)

### 5.3 Corregir Implementación

- ✅ Corregir parser para validar duplicados bidireccionales
- ✅ Especificar lógica de reintentos de `MarketInfo()`
- ✅ Definir migración SQL completa con constraint correcto
- ✅ Agregar código de traducción en Agent

---

## 6. Alcance Ajustado de i3 (Realista)

### Slave EA:
1. Input `SymbolMappings` con parser robusto (trim, validaciones, duplicados)
2. Handshake simple con `symbols[]` (SIN versionamiento)
3. Reporting de precios cada 250ms en `OnTick()`
4. Reconexión automática de pipe

### Agent:
1. Traducir `handshake` → `AccountSymbolsReport`
2. Generar `reported_at_ms`
3. Coalesce de precios cada 250ms
4. Reconexión automática gRPC (ya existe, validar)

### Core:
1. Validar símbolos contra ETCD
2. Persistir mapeo con `contract_size` en PostgreSQL
3. Validar unicidad por `account_id + broker_symbol`
4. Logs/métricas de validación
5. **(Opcional)** Validación de StopLevel antes de enviar órdenes

---

## 7. Criterios de Aceptación (Ajustados al Scope Real)

- ✅ Slave EA reporta `symbols[]` en handshake al conectar
- ✅ Core valida contra ETCD y persiste en PostgreSQL
- ✅ Core traduce `canonical → broker_symbol` antes de enviar órdenes
- ✅ Métricas de lookup (hit/miss) activas
- ✅ Slave EA reporta Bid/Ask cada 250ms
- ✅ Reconexión automática funciona en EA y Agent
- ✅ 0 errores por símbolo desconocido cuando `unknown_action=reject`
- ✅ Mapeo persistido y trazable (consultas SQL funcionan)

---

## 8. Estimación de Esfuerzo

**RFC-004c actual:** ~5 días (por sobre-ingeniería)  
**RFC-004c ajustado:** ~2 días (según roadmap original)

### Desglose:
- Slave EA (parser + handshake + prices): 4 horas
- Agent (traducción + coalesce): 3 horas
- Core (validación + persistencia): 4 horas
- Migraciones SQL: 1 hora
- Reconexión (EA + Agent): 3 horas
- Testing manual: 3 horas

**Total:** ~18 horas = 2 días con margen

---

## 9. Conclusión

### Problemas Identificados:

1. **Sobre-ingeniería:** Versionamiento, feedback loop y modo degradado complejo NO están en los requerimientos de i3
2. **Funcionalidad faltante:** Reporting de precios 250ms y reconexión SÍ están en RFC-001 pero NO en RFC-004c
3. **Implementación incompleta:** Parser sin duplicados, migración SQL sin constraint correcto

### Recomendación Final:

🟡 **SIMPLIFICAR RFC-004c** para cumplir con el scope real de i3:
- Remover funcionalidad fuera de scope (versionamiento, feedback, hash)
- Agregar funcionalidad faltante (precios 250ms, reconexión)
- Corregir implementación (parser, SQL, traducción Agent)

Con estos ajustes, i3 queda **limpia, completa y en 2 días** según el roadmap original.

---

## 10. Actualización de RFC-001 con Mejoras Propuestas

Las siguientes mejoras SALEN de i3 y se agregan a iteraciones futuras:

### Para Iteración 4:
- Versionamiento del protocolo (`protocol_version`)
- Feedback loop activo Core→Agent→EA

### Para Iteración 7 (CLI/Panel):
- Modo degradado con monitoreo gráfico
- Hash de configuración para detección de cambios

---

**Responsable de aplicar ajustes:** Equipo de implementación  
**Próximo paso:** Actualizar RFC-004c con scope ajustado y validar con stakeholders

---

*Fin del documento de revisión final.*

