# 📡 Echo Trade Copier - Expert Advisors (MT4) — i0

**Versión**: 0.1.0  
**Estado**: ✅ Corregido (Stack Damaged Fix aplicado)  
**Fecha**: 2025-10-26

---

## 📂 Contenido

```
mt4/
├── master.mq4           # Master EA (generador de señales)
├── slave.mq4            # Slave EA (ejecutor de órdenes)
├── JAson.mqh            # Librería JSON para MQL4
├── echo_pipe.cpp        # Código fuente de la DLL (C++)
├── echo_pipe.dll        # DLL compilada (NO en repo, compilar localmente)
├── build_dll.bat        # Script de build (Windows con Visual Studio)
├── build_dll.sh         # Script de build (Linux/WSL con MinGW)
├── FIX-STACK-DAMAGED.md # Documentación del fix aplicado
└── README.md            # Este archivo
```

---

## 🚨 Fix Crítico Aplicado

**Problema Original**: Error "stack damaged, check DLL function call" + "uninit reason 8"

**Causa**: Incompatibilidad de firma DLL → uso de `long` (64-bit) en MQL4 cuando la DLL Win32 retorna `int` (32-bit)

**Solución**: Cambiar todos los `long` a `int` en los `#import` de ambas EAs.

👉 **Ver detalles completos en**: [`FIX-STACK-DAMAGED.md`](FIX-STACK-DAMAGED.md)

---

## 🔧 Setup Rápido

### Paso 1: Compilar la DLL

#### Opción A: Windows con Visual Studio

```cmd
REM Abrir "Developer Command Prompt for VS 2019"
cd path\to\echo\clients\mt4
build_dll.bat
```

#### Opción B: Linux/WSL con MinGW

```bash
# Instalar MinGW (solo primera vez)
sudo apt-get install mingw-w64

# Compilar
cd /path/to/echo/clients/mt4
./build_dll.sh
```

**Output esperado**: `echo_pipe.dll` (32-bit para MT4)

---

### Paso 2: Instalar DLL en MT4

```cmd
REM Opción A: Copiar manualmente
copy echo_pipe.dll "%APPDATA%\MetaQuotes\Terminal\<ID>\MQL4\Libraries\"

REM Opción B: Usar MT4 UI
REM 1. MT4 → File → Open Data Folder
REM 2. Navegar a MQL4\Libraries\
REM 3. Copiar echo_pipe.dll ahí
```

---

### Paso 3: Habilitar DLL en MT4

1. MT4 → **Tools** → **Options**
2. Tab **Expert Advisors**
3. ✅ Marcar **"Allow DLL imports"**
4. ✅ Marcar **"Allow imported functions call"** (si aparece)
5. Click **OK**

---

### Paso 4: Compilar EAs en MetaEditor

1. Abrir MT4
2. Presionar **F4** (abre MetaEditor)
3. Abrir `master.mq4` y `slave.mq4`
4. Compilar cada uno (**F7**)
5. **Verificar**: 0 errores, 0 warnings

---

### Paso 5: Cargar EAs en Charts

#### Master EA

1. Crear chart de **XAUUSD** (cualquier timeframe)
2. Arrastrar **master.mq4** al chart
3. En el popup:
   - `MagicNumber`: 123456 (o tu número único)
   - `EnableDebugLogs`: false
   - `LogToFile`: false (true para debugging)
4. Click **OK**

**Logs esperados** (pestaña "Experts"):
```
[INFO] ... | EA initializing | account=12345
[INFO] ... | EA initialized | tracked_tickets=0
[INFO] ... | Pipe connected | pipe=\\.\pipe\echo_master_12345
[INFO] ... | Handshake sent | account=12345
```

#### Slave EA

1. Crear chart de **XAUUSD** (cualquier timeframe)
2. Arrastrar **slave.mq4** al chart
3. En el popup:
   - `TradeSymbol`: XAUUSD
   - `TimerPeriodMs`: 1000 (polling cada 1 segundo)
   - `EnableDebugLogs`: false
   - `LogToFile`: false
4. Click **OK**

**Logs esperados**:
```
[INFO] ... | EA initialized | timer_period_ms=1000
[INFO] ... | Pipe connected | pipe=\\.\pipe\echo_slave_67890
[INFO] ... | Handshake sent | bytes=142
```

---

## 🎯 Funcionalidad (Iteración 0)

### Master EA

**Responsabilidades**:
- Detectar órdenes ejecutadas (manual o algoritmo)
- Generar `TradeIntent` con UUIDv7 único
- Enviar al Agent vía Named Pipe
- Detectar cierres de posiciones
- Enviar `TradeClose` al Agent

**Limitaciones i0**:
- ✅ Solo símbolo **XAUUSD** (hardcoded)
- ✅ Solo órdenes a **mercado** (OP_BUY, OP_SELL)
- ❌ No SL/TP
- ❌ No pending orders
- ❌ No filtros de horario

**Testing Manual**:
1. Hacer clic en botón **"BUY XAUUSD"** en el chart
2. Verificar logs: `[INFO] ... | TradeIntent sent | trade_id=...`
3. Cerrar posición manualmente
4. Verificar logs: `[INFO] ... | TradeClose sent | close_id=...`

---

### Slave EA

**Responsabilidades**:
- Conectar a Named Pipe del Agent
- Recibir comandos `ExecuteOrder` y `CloseOrder`
- Ejecutar `OrderSend` / `OrderClose`
- Reportar `ExecutionResult` / `CloseResult`
- Registrar timestamps (t5, t6, t7) para métricas

**Limitaciones i0**:
- ✅ Solo símbolo **XAUUSD**
- ✅ Lot size viene del Core (0.10 en i0)
- ❌ No validación de balance/equity
- ❌ No money management
- ❌ No reintentos automáticos

**Testing Manual**:
1. Ejecutar orden desde Master
2. Verificar logs Slave: `[INFO] ... | Command received | type=execute_order`
3. Verificar logs Slave: `[INFO] ... | Order executed | ticket=...`
4. Verificar logs Slave: `[INFO] ... | ExecutionResult sent | success=true`

---

## 📋 Checklist de Validación

### Pre-Testing

- [ ] DLL compilada correctamente (32-bit)
- [ ] DLL copiada a `MT4/MQL4/Libraries/`
- [ ] "Allow DLL imports" habilitado en MT4
- [ ] EAs compiladas sin errores en MetaEditor
- [ ] Agent y Core corriendo (ver logs Go)
- [ ] Named Pipes creados por Agent (ver logs Agent)

### Testing Master EA

- [ ] EA carga sin "stack damaged" ni "uninit reason 8"
- [ ] Handshake enviado correctamente
- [ ] Botón "BUY XAUUSD" visible en el chart
- [ ] Click en botón ejecuta orden en broker
- [ ] TradeIntent enviado (ver logs MT4 + Agent)
- [ ] Cierre manual genera TradeClose

### Testing Slave EA

- [ ] EA carga sin "stack damaged" ni "uninit reason 8"
- [ ] Handshake enviado correctamente
- [ ] Timer OnTimer() ejecuta cada 1s (ver logs)
- [ ] Recibe ExecuteOrder desde Agent
- [ ] Ejecuta OrderSend correctamente
- [ ] Reporta ExecutionResult con timestamps completos (t0-t7)

### Testing E2E

- [ ] Master BUY → Slave BUY ejecutado en < 2s
- [ ] Tickets diferentes (Master ≠ Slave)
- [ ] MagicNumber idéntico en ambos
- [ ] Master CLOSE → Slave CLOSE ejecutado en < 2s
- [ ] 0 duplicados en 10 ejecuciones consecutivas
- [ ] Latencia E2E p95 < 120ms (ver logs Core)

---

## 🐛 Troubleshooting

### Error: "stack damaged"

**Síntoma**: EA no se inicializa, uninit reason 8

**Causa**: Firma de DLL incorrecta (long vs int)

**Solución**: Ver [`FIX-STACK-DAMAGED.md`](FIX-STACK-DAMAGED.md) sección completa.

---

### Error: "cannot load 'echo_pipe.dll'"

**Síntoma**: EA no puede cargar la DLL

**Posibles causas**:
1. DLL no está en `MQL4/Libraries/`
2. DLL está corrupta o mal compilada
3. DLL de 64-bit en MT4 32-bit (o viceversa)
4. Runtime DLLs faltantes (msvcr120.dll, etc.)

**Solución**:
```cmd
REM 1. Verificar que DLL existe
dir "%APPDATA%\MetaQuotes\Terminal\<ID>\MQL4\Libraries\echo_pipe.dll"

REM 2. Verificar arquitectura
dumpbin /headers echo_pipe.dll | findstr "machine"
REM Esperado: machine (x86)

REM 3. Verificar exports
dumpbin /exports echo_pipe.dll
REM Esperado: ConnectPipe, WritePipeW, ReadPipeLine, ClosePipe

REM 4. Si falta runtime, recompilar con static linking
REM    Visual Studio: usar /MT en vez de /MD
REM    MinGW: usar -static-libgcc -static-libstdc++
```

---

### Error: "Pipe connection failed"

**Síntoma**: `[ERROR] Pipe connection failed | pipe=\\.\pipe\echo_master_12345`

**Causa**: Agent no está corriendo o no creó el pipe

**Solución**:
1. Verificar que Agent está corriendo:
   ```bash
   ps aux | grep echo-agent
   # O en Windows:
   tasklist | findstr echo-agent
   ```

2. Verificar logs del Agent:
   ```
   [INFO] Named Pipe created: \\.\pipe\echo_master_12345
   [INFO] Named Pipe created: \\.\pipe\echo_slave_67890
   ```

3. Verificar con herramienta de pipes (Windows):
   ```cmd
   REM Instalar PipeList de Sysinternals
   pipelist.exe | findstr echo_
   ```

---

### Warning: "Symbol not supported"

**Síntoma**: `[WARN] Symbol not supported | symbol=EURUSD`

**Causa**: En i0, solo está soportado **XAUUSD**

**Solución**: Cargar EAs solo en charts de **XAUUSD**.

---

### Error: OrderSend failed (código 134)

**Síntoma**: `[ERROR] OrderSend failed | error=134`

**Causa**: `ERR_NOT_ENOUGH_MONEY` - balance insuficiente

**Solución**:
1. Verificar balance en cuenta demo
2. Reducir lot size en Master (i0 usa 0.01 en master, 0.10 en slaves)
3. Depositar fondos en cuenta demo

---

### EA pierde conexión con Agent

**Síntoma**: `[ERROR] WritePipe failed | ...` repetido

**Causa**: Agent se detuvo o pipe cerrado

**Solución**:
1. Verificar que Agent está corriendo
2. Ver logs Agent para errores
3. EA intentará reconectar automáticamente cada 5s (3 intentos máximo)
4. Si entra en "degraded mode", reiniciar EA

---

## 📊 Métricas y Observabilidad

### Logs Estructurados

Formato: `[LEVEL] timestamp_ms | event | details`

**Niveles**:
- `DEBUG`: Solo si `EnableDebugLogs=true`
- `INFO`: Eventos normales
- `WARN`: Advertencias (no bloquean ejecución)
- `ERROR`: Errores (requieren atención)

**Eventos clave a buscar**:
- `Pipe connected`: Conexión exitosa al Agent
- `Handshake sent`: Registro exitoso con Agent
- `TradeIntent sent`: Orden enviada al Core
- `Order executed`: OrderSend exitoso
- `ExecutionResult sent`: Resultado enviado al Core

### Log to File

Si `LogToFile=true`, logs se guardan en:
- Master: `MT4/MQL4/Files/echo_master_<account>.log`
- Slave: `MT4/MQL4/Files/echo_slave_<account>.log`

**Útil para**:
- Debugging de errores intermitentes
- Análisis post-mortem
- Auditoría de operaciones

---

## 🔐 Seguridad y Consideraciones

### Cuentas Demo

**IMPORTANTE**: En i0, usar **SOLO cuentas demo**. No usar dinero real.

**Razones**:
- Sin validación de equity/riesgo
- Sin confirmación de usuario
- Sin rollback en caso de error
- Sin persistencia (órdenes en vuelo se pierden si Agent cae)

### MagicNumber Único

Usar un `MagicNumber` **único por estrategia**:
- Permite identificar órdenes del sistema Echo
- Evita conflictos con otras EAs/operaciones manuales
- Facilita debugging y auditoría

**Recomendación**: `123456` para testing, cambiar en producción.

### Named Pipes - Seguridad Local

Named Pipes en i0 **NO tienen autenticación**.

**Implicaciones**:
- Cualquier proceso en el mismo Windows puede conectarse a los pipes
- En i1+: agregar autenticación básica (token en handshake)

**Mitigación i0**:
- Usar solo en host de testing aislado
- No exponer Agent a red externa

---

## 📝 Próximos Pasos (Post-i0)

### Iteración 1 (i1)

- [ ] Multi-símbolo (EURUSD, GBPUSD, etc.)
- [ ] Configuración via etcd (símbolos, lot sizes, slaves por master)
- [ ] Money Management (% de equity, lot size dinámico)
- [ ] Persistencia (Postgres para estado de órdenes)
- [ ] Reintentos con backoff exponencial
- [ ] SL/TP dinámicos
- [ ] Pending orders (OP_BUYLIMIT, OP_SELLLIMIT, etc.)

### Iteración 2 (i2)

- [ ] Multi-broker (diferentes brokers para master y slaves)
- [ ] Symbol mapping (XAUUSD en broker A → GOLD en broker B)
- [ ] Filtros de horario (solo copiar en ciertos horarios)
- [ ] Copy inverso (master BUY → slave SELL)
- [ ] Ratio de lot size configurable por slave

---

## 📚 Referencias

- [RFC-002: Plan de Implementación i0](../../docs/rfcs/RFC-002-iteration-0-implementation.md)
- [FIX-STACK-DAMAGED.md](FIX-STACK-DAMAGED.md) - Documentación del fix crítico
- [MQL4 Documentation](https://docs.mql4.com/)
- [Named Pipes (Windows)](https://docs.microsoft.com/en-us/windows/win32/ipc/named-pipes)

---

**Autor**: Aranea Labs - Echo Trade Copier Team  
**Licencia**: Privado  
**Última actualización**: 2025-10-26

