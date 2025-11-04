# Guía: Activar Nuevos Símbolos (Ejemplo: DAX)

Esta guía explica cómo activar nuevos activos en Echo Trade Copier usando el sistema de catálogo canónico y mapeo por cuenta de la Iteración 3.

## 📋 Pasos para Activar DAX

### 1. Configurar el Catálogo Canónico en ETCD

El Core lee el catálogo canónico desde ETCD. Debes agregar DAX a la lista.

**ETCD Key:** `core/canonical_symbols`  
**Formato:** Lista separada por comas (CSV)

#### Ejemplo con `etcdctl`:

```bash
# Conectar a ETCD
export ETCDCTL_API=3
export ETCDCTL_ENDPOINTS=http://localhost:2379

# Leer valor actual
etcdctl get core/canonical_symbols

# Agregar DAX (ejemplo: si ya tienes XAUUSD)
etcdctl put core/canonical_symbols "XAUUSD,DAX"

# O si quieres usar un nombre más descriptivo
etcdctl put core/canonical_symbols "XAUUSD,GER30"  # GER30 es común para DAX
```

#### Opciones de Nombres Canónicos para DAX:

- `DAX` - Nombre corto común
- `GER30` - Índice alemán (30 acciones)
- `DE30` - Alternativa común

**⚠️ Importante:** El nombre canónico debe ser consistente. Si eliges `GER30`, todos los brokers deben reportar ese mismo nombre canónico para sus variantes de DAX.

#### Política de Símbolos Desconocidos:

**ETCD Key:** `core/symbols/unknown_action`  
**Valores:** `warn` (por defecto) o `reject`

```bash
# Durante rollout inicial: usar "warn" para permitir símbolos no mapeados
etcdctl put core/symbols/unknown_action "warn"

# Después de validar todo: cambiar a "reject" para rechazar símbolos no mapeados
etcdctl put core/symbols/unknown_action "reject"
```

---

### 2. Modificar el Slave EA para Reportar Símbolos

El Slave EA debe enviar los símbolos disponibles en el handshake. Actualmente el EA solo envía metadata básica.

#### Modificar `SendHandshake()` en `slave.mq4`:

**Ubicación:** Función `SendHandshake()` alrededor de la línea 289

**Código Actual:**
```mql4
void SendHandshake()
{
   string payload =
      "{"
      +"\"type\":\"handshake\","
      +"\"timestamp_ms\":" + ULongToStr(GetTickCount()) + ","
      +"\"payload\":{"
      +"\"client_id\":\"slave_"+ IntegerToString(AccountNumber()) + "\","
      +"\"account_id\":\""+ IntegerToString(AccountNumber()) + "\""
      +"}"
      +"}";
   PipeWriteLn(payload);
   Log("INFO","Handshake sent","account="+IntegerToString(AccountNumber()));
}
```

**Código Modificado (con símbolos):**
```mql4
void SendHandshake()
{
   // Construir array de símbolos disponibles
   string symbolsJson = "";
   int symbolCount = 0;
   
   // Símbolos a reportar (ejemplo: XAUUSD y DAX)
   string symbolsToReport[] = {"XAUUSD", "GER30"};  // Ajustar según tu broker
   
   for(int i = 0; i < ArraySize(symbolsToReport); i++)
   {
      string sym = symbolsToReport[i];
      
      // Verificar que el símbolo existe en el broker
      if(!IsSymbolValid(sym))
      {
         Log("WARN", "Symbol not available", "symbol=" + sym);
         continue;
      }
      
      // Obtener especificaciones del símbolo
      double point = MarketInfo(sym, MODE_POINT);
      int digits = (int)MarketInfo(sym, MODE_DIGITS);
      double tickSize = MarketInfo(sym, MODE_TICKSIZE);
      double minLot = MarketInfo(sym, MODE_MINLOT);
      double maxLot = MarketInfo(sym, MODE_MAXLOT);
      double lotStep = MarketInfo(sym, MODE_LOTSTEP);
      int stopLevel = (int)MarketInfo(sym, MODE_STOPLEVEL);
      double contractSize = MarketInfo(sym, MODE_LOTSIZE);
      
      // Normalizar canonical_symbol (quitar sufijos/prefijos del broker)
      string canonicalSymbol = sym;
      // Ejemplo: si broker tiene "GER30.a" o "GER30.x", usar "GER30"
      // Ajustar según las convenciones de tu broker
      if(StringFind(sym, ".") > 0)
         canonicalSymbol = StringSubstr(sym, 0, StringFind(sym, "."));
      
      if(symbolCount > 0) symbolsJson += ",";
      symbolsJson += "{"
         +"\"canonical_symbol\":\"" + canonicalSymbol + "\","
         +"\"broker_symbol\":\"" + EscapeJSON(sym) + "\","
         +"\"digits\":" + IntegerToString(digits) + ","
         +"\"point\":" + DoubleToString(point, 10) + ","
         +"\"tick_size\":" + DoubleToString(tickSize, 10) + ","
         +"\"min_lot\":" + DoubleToString(minLot, 10) + ","
         +"\"max_lot\":" + DoubleToString(maxLot, 10) + ","
         +"\"lot_step\":" + DoubleToString(lotStep, 10) + ","
         +"\"stop_level\":" + IntegerToString(stopLevel);
      
      // ContractSize es opcional
      if(contractSize > 0)
         symbolsJson += ",\"contract_size\":" + DoubleToString(contractSize, 10);
      
      symbolsJson += "}";
      symbolCount++;
   }
   
   string payload =
      "{"
      +"\"type\":\"handshake\","
      +"\"timestamp_ms\":" + ULongToStr(GetTickCount()) + ","
      +"\"payload\":{"
      +"\"client_id\":\"slave_"+ IntegerToString(AccountNumber()) + "\","
      +"\"account_id\":\""+ IntegerToString(AccountNumber()) + "\","
      +"\"symbols\":[" + symbolsJson + "]"
      +"}"
      +"}";
   PipeWriteLn(payload);
   Log("INFO","Handshake sent","account="+IntegerToString(AccountNumber()) + " symbols=" + IntegerToString(symbolCount));
}
```

#### Versión Dinámica (Reportar Todos los Símbolos Disponibles):

Si prefieres reportar automáticamente todos los símbolos disponibles en el Market Watch:

```mql4
void SendHandshake()
{
   string symbolsJson = "";
   int symbolCount = 0;
   
   // Iterar sobre símbolos en Market Watch
   for(int i = 0; i < SymbolsTotal(true); i++)
   {
      string sym = SymbolName(i, true);
      
      // Filtrar solo símbolos de interés (opcional)
      // if(StringFind(sym, "XAU") < 0 && StringFind(sym, "GER") < 0) continue;
      
      if(!IsSymbolValid(sym)) continue;
      
      double point = MarketInfo(sym, MODE_POINT);
      int digits = (int)MarketInfo(sym, MODE_DIGITS);
      double tickSize = MarketInfo(sym, MODE_TICKSIZE);
      double minLot = MarketInfo(sym, MODE_MINLOT);
      double maxLot = MarketInfo(sym, MODE_MAXLOT);
      double lotStep = MarketInfo(sym, MODE_LOTSTEP);
      int stopLevel = (int)MarketInfo(sym, MODE_STOPLEVEL);
      double contractSize = MarketInfo(sym, MODE_LOTSIZE);
      
      // Normalizar canonical_symbol
      string canonicalSymbol = sym;
      // Ejemplo: remover sufijos comunes del broker
      StringReplace(canonicalSymbol, ".m", "");  // Micro lot
      StringReplace(canonicalSymbol, ".a", "");  // Alternativo
      StringReplace(canonicalSymbol, ".x", "");   // Ejecución
      
      if(symbolCount > 0) symbolsJson += ",";
      symbolsJson += "{"
         +"\"canonical_symbol\":\"" + canonicalSymbol + "\","
         +"\"broker_symbol\":\"" + EscapeJSON(sym) + "\","
         +"\"digits\":" + IntegerToString(digits) + ","
         +"\"point\":" + DoubleToString(point, 10) + ","
         +"\"tick_size\":" + DoubleToString(tickSize, 10) + ","
         +"\"min_lot\":" + DoubleToString(minLot, 10) + ","
         +"\"max_lot\":" + DoubleToString(maxLot, 10) + ","
         +"\"lot_step\":" + DoubleToString(lotStep, 10) + ","
         +"\"stop_level\":" + IntegerToString(stopLevel);
      
      if(contractSize > 0)
         symbolsJson += ",\"contract_size\":" + DoubleToString(contractSize, 10);
      
      symbolsJson += "}";
      symbolCount++;
   }
   
   string payload =
      "{"
      +"\"type\":\"handshake\","
      +"\"timestamp_ms\":" + ULongToStr(GetTickCount()) + ","
      +"\"payload\":{"
      +"\"client_id\":\"slave_"+ IntegerToString(AccountNumber()) + "\","
      +"\"account_id\":\""+ IntegerToString(AccountNumber()) + "\", {
      +"\"symbols\":[" + symbolsJson + "]"
      +"}"
      +"}";
   PipeWriteLn(payload);
   Log("INFO","Handshake sent","account="+IntegerToString(AccountNumber()) + " symbols=" + IntegerToString(symbolCount));
}
```

---

### 3. Formato JSON del Handshake con Símbolos

El Agent espera recibir el handshake en este formato:

```json
{
  "type": "handshake",
  "timestamp_ms": 1234567890,
  "payload": {
    "client_id": "slave_12345",
    "account_id": "12345",
    "symbols": [
      {
        "canonical_symbol": "XAUUSD",
        "broker_symbol": "XAUUSD.m",
        "digits": 2,
        "point": 0.01,
        "tick_size": 0.01,
        "min_lot": 0.01,
        "max_lot": 100.0,
        "lot_step": 0.01,
        "stop_level": 0,
        "contract_size": 100.0
      },
      {
        "canonical_symbol": "GER30",
        "broker_symbol": "GER30.x",
        "digits": 1,
        "point": 0.1,
        "tick_size": 0.1,
        "min_lot": 0.01,
        "max_lot": 50.0,
        "lot_step": 0.01,
        "stop_level": 5,
        "contract_size": 1.0
      }
    ]
  }
}
```

**Campos Requeridos:**
- `canonical_symbol`: Nombre canónico (debe coincidir con ETCD)
- `broker_symbol`: Nombre exacto del símbolo en el broker
- `digits`, `point`, `tick_size`, `min_lot`, `max_lot`, `lot_step`, `stop_level`

**Campos Opcionales:**
- `contract_size`: Tamaño del contrato (si aplica)

---

### 4. Normalización del Nombre Canónico

El Core normaliza automáticamente los símbolos usando estas reglas:

1. Convertir a mayúsculas
2. Remover espacios
3. Remover caracteres especiales comunes (`-`, `_`, `.`)

**Ejemplos:**
- `"dax"` → `"DAX"`
- `"ger-30"` → `"GER30"`
- `"DE30.x"` → `"DE30"`

**⚠️ Importante:** El nombre canónico que reportes debe normalizarse al mismo valor que pusiste en ETCD.

---

### 5. Verificar que Funciona

#### 5.1 Verificar Configuración ETCD

```bash
# Ver catálogo canónico
etcdctl get core/canonical_symbols
# Debe incluir: XAUUSD,DAX (o GER30)

# Ver política de símbolos desconocidos
etcdctl get core/symbols/unknown_action
# Debe ser: warn o reject
```

#### 5.2 Verificar Logs del Agent

El Agent debe reportar los símbolos al Core:

```
[INFO] AccountSymbolsReport sent to Core (i3) | account_id=12345 symbols_count=2
```

#### 5.3 Verificar Logs del Core

El Core debe aceptar y persistir los mapeos:

```
[INFO] Symbol mappings upserted (i3) | account_id=12345 mappings_count=2
[INFO] Account mappings loaded from PostgreSQL (lazy load) | account_id=12345 mappings_count=2
```

#### 5.4 Verificar Base de Datos

```sql
-- Ver mapeos por cuenta
SELECT account_id, canonical_symbol, broker_symbol, digits, point
FROM echo.account_symbol_map
WHERE account_id = '12345';

-- Debe mostrar:
-- 12345 | XAUUSD | XAUUSD.m | 2 | 0.01
-- 12345 | GER30  | GER30.x  | 1 | 0.1
```

#### 5.5 Probar Trading con DAX

Enviar un `TradeIntent` desde el Master EA con `symbol="GER30"` (o el nombre canónico que elegiste). El Core debe:

1. Validar que `GER30` está en el catálogo canónico ✓
2. Resolver `GER30` → `GER30.x` (o el símbolo del broker) ✓
3. Enviar `ExecuteOrder` con `symbol="GER30.x"` al Slave ✓

---

### 6. Troubleshooting

#### Problema: "Symbol not in whitelist"

**Causa:** El símbolo canónico no está en ETCD o la normalización no coincide.

**Solución:**
1. Verificar que el símbolo está en `core/canonical_symbols`
2. Verificar que el nombre canónico reportado se normaliza correctamente
3. Ver logs del Core para ver el símbolo normalizado recibido

#### Problema: "Symbol mapping not found"

**Causa:** El Slave EA no reportó el símbolo en el handshake o el mapeo no se persistió.

**Solución:**
1. Verificar que el Slave EA envía `symbols` en el handshake
2. Verificar logs del Agent: debe mostrar "AccountSymbolsReport sent"
3. Verificar base de datos: debe haber registro en `account_symbol_map`
4. Reiniciar el Slave EA para que vuelva a enviar el handshake

#### Problema: "Unknown symbol, warn mode" (con `unknown_action=warn`)

**Causa:** El símbolo se está usando pero no está mapeado.

**Solución:**
1. Verificar que el Slave EA reportó el símbolo
2. Verificar que el nombre canónico coincide (después de normalización)
3. Si el símbolo es válido pero no mapeado, agregarlo al handshake del Slave EA

---

### 7. Ejemplo Completo: Activar DAX en un Broker Específico

**Escenario:** Broker usa `GER30.x` para DAX, quieres usar `GER30` como canónico.

#### Paso 1: ETCD
```bash
etcdctl put core/canonical_symbols "XAUUSD,GER30"
etcdctl put core/symbols/unknown_action "warn"
```

#### Paso 2: Modificar Slave EA
```mql4
// En SendHandshake(), agregar:
string symbolsToReport[] = {"XAUUSD", "GER30.x"};  // Broker tiene GER30.x

// En el loop de construcción:
string canonicalSymbol = sym;
if(StringFind(sym, ".x") > 0)
   canonicalSymbol = StringSubstr(sym, 0, StringFind(sym, ".x"));
// Resultado: canonicalSymbol = "GER30", broker_symbol = "GER30.x"
```

#### Paso 3: Master EA envía TradeIntent
```mql4
// En Master EA, enviar:
SendTradeIntent(ticket, "GER30", ...);  // Usar nombre canónico
```

#### Paso 4: Core traduce automáticamente
- Core recibe: `symbol="GER30"`
- Core valida: `GER30` está en catálogo ✓
- Core resuelve: `GER30` → `GER30.x` (desde caché)
- Core envía: `ExecuteOrder` con `symbol="GER30.x"` al Slave

---

## 📝 Resumen de Checklist

- [ ] Agregar símbolo canónico a `core/canonical_symbols` en ETCD
- [ ] Configurar `core/symbols/unknown_action` (warn durante rollout, reject después)
- [ ] Modificar `SendHandshake()` en Slave EA para incluir `symbols`
- [ ] Normalizar nombres canónicos según convenciones del broker
- [ ] Recompilar y desplegar Slave EA modificado
- [ ] Verificar logs del Agent: debe mostrar "AccountSymbolsReport sent"
- [ ] Verificar logs del Core: debe mostrar "Symbol mappings upserted"
- [ ] Verificar base de datos: debe haber registros en `account_symbol_map`
- [ ] Probar TradeIntent con el nuevo símbolo desde Master EA
- [ ] Verificar que ExecuteOrder llega con el símbolo correcto del broker

---

## 🔗 Referencias

- RFC-004: Catálogo canónico de símbolos (Iteración 3)
- `core/internal/symbol_validator.go`: Validación de símbolos canónicos
- `core/internal/symbol_resolver.go`: Resolución de mapeos por cuenta
- `agent/internal/pipe_manager.go`: Procesamiento de handshake con símbolos

