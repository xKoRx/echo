# Slave EA — Guía de Implementación i1

## 📋 Descripción

El Slave EA es el Expert Advisor que se ejecuta en terminales MT4/MT5 slave y ejecuta **ExecuteOrders** recibidas del Agent, reportando resultados con **ExecutionResults**.

## 🎯 Cambios i1 vs i0

### Cambios Obligatorios

1. **Comment con trade_id**: Incluir el `trade_id` en el comment de `OrderSend` para correlación persistente
2. **Timestamps completos**: Reportar t5, t6 y t7 en `ExecutionResult`
3. **CloseOrder con ticket exacto**: Usar el ticket recibido en `CloseOrder` (no buscar por magic+symbol si ticket != 0)

### Cambios Opcionales

- Mejorar precisión de timestamps (usar DLL con `GetTickCount()`)
- Validar formato UUIDv7 de `trade_id` recibido

---

## 🔑 Componentes Clave

### 1. ExecuteOrder con trade_id en Comment

Al recibir un `ExecuteOrder` del Agent via Named Pipe, extraer el `trade_id` e incluirlo en el comment de `OrderSend`:

```mql4
//+------------------------------------------------------------------+
//| Procesa ExecuteOrder recibido del Agent                          |
//+------------------------------------------------------------------+
void ProcessExecuteOrder(string jsonMessage)
{
   // 1. Parse JSON
   CJAVal json;
   if(!json.Deserialize(jsonMessage)) {
       Print("ERROR: JSON inválido en ExecuteOrder");
       return;
   }
   
   // 2. Extraer campos
   string commandID = json["command_id"].ToStr();
   string tradeID = json["trade_id"].ToStr();  // ⚠️ IMPORTANTE
   string symbol = json["symbol"].ToStr();
   int side = (int)json["side"].ToInt();       // 1=BUY, 2=SELL
   double lotSize = json["lot_size"].ToDbl();
   int magicNumber = (int)json["magic_number"].ToInt();
   
   // SL/TP opcionales
   double sl = 0, tp = 0;
   if(json["stop_loss"].m_type != jtNULL) {
       sl = json["stop_loss"].ToDbl();
   }
   if(json["take_profit"].m_type != jtNULL) {
       tp = json["take_profit"].ToDbl();
   }
   
   // 3. Timestamps (i1: t4 = Agent recv, t5 = Slave EA recv)
   ulong t4 = 0, t5 = (ulong)(TimeCurrent() * 1000);
   if(json["timestamps"]["t4_agent_recv_ms"].m_type != jtNULL) {
       t4 = (ulong)json["timestamps"]["t4_agent_recv_ms"].ToInt();
   }
   
   // 4. Ejecutar OrderSend con trade_id en comment
   ulong t6 = (ulong)(TimeCurrent() * 1000);  // t6: antes de OrderSend
   
   int ticket = OrderSend(
       symbol,
       side == 1 ? OP_BUY : OP_SELL,
       lotSize,
       side == 1 ? MarketInfo(symbol, MODE_ASK) : MarketInfo(symbol, MODE_BID),
       3,  // slippage
       sl,
       tp,
       "TID:" + tradeID,  // ⚠️ CRÍTICO: incluir trade_id en comment
       magicNumber,
       0,
       side == 1 ? clrGreen : clrRed
   );
   
   ulong t7 = (ulong)(TimeCurrent() * 1000);  // t7: después de OrderSend
   
   // 5. Construir ExecutionResult
   bool success = (ticket > 0);
   double executedPrice = 0;
   string errorCode = "ERROR_CODE_UNSPECIFIED";
   string errorMessage = "";
   
   if(success) {
       executedPrice = side == 1 ? MarketInfo(symbol, MODE_ASK) : MarketInfo(symbol, MODE_BID);
       errorCode = "NO_ERROR";
   } else {
       int err = GetLastError();
       errorCode = ErrorCodeToString(err);
       errorMessage = ErrorDescription(err);
   }
   
   // 6. Enviar ExecutionResult al Agent
   SendExecutionResult(commandID, tradeID, success, ticket, errorCode, errorMessage,
                       executedPrice, t4, t5, t6, t7);
}
```

---

### 2. ExecutionResult con Timestamps Completos (t5, t6, t7)

**Timestamps i1** (RFC-003 sección 2):

- **t0**: Master EA genera intent
- **t1**: Agent recibe de pipe (master)
- **t2**: Core recibe de stream
- **t3**: Core envía ExecuteOrder
- **t4**: Agent recibe ExecuteOrder
- **t5**: Slave EA recibe comando (⚠️ SLAVE)
- **t6**: Slave EA llama OrderSend (⚠️ SLAVE)
- **t7**: Slave EA recibe ticket/fill (⚠️ SLAVE)

```mql4
//+------------------------------------------------------------------+
//| Envía ExecutionResult al Agent                                   |
//+------------------------------------------------------------------+
void SendExecutionResult(string commandID, string tradeID, bool success,
                         int ticket, string errorCode, string errorMessage,
                         double executedPrice,
                         ulong t4, ulong t5, ulong t6, ulong t7)
{
   // Construir JSON
   string json = "{";
   json += "\"command_id\":\"" + commandID + "\",";
   json += "\"trade_id\":\"" + tradeID + "\",";
   json += "\"success\":" + (success ? "true" : "false") + ",";
   json += "\"ticket\":" + IntegerToString(ticket) + ",";
   json += "\"error_code\":\"" + errorCode + "\",";
   
   if(errorMessage != "") {
       json += "\"error_message\":\"" + errorMessage + "\",";
   }
   
   if(success && executedPrice > 0) {
       json += "\"executed_price\":" + DoubleToString(executedPrice, 5) + ",";
   }
   
   // ⚠️ CRÍTICO i1: Timestamps completos (t5, t6, t7)
   json += "\"timestamps\":{";
   if(t4 > 0) json += "\"t4_agent_recv_ms\":" + IntegerToString(t4) + ",";
   json += "\"t5_slave_ea_recv_ms\":" + IntegerToString(t5) + ",";
   json += "\"t6_order_send_ms\":" + IntegerToString(t6) + ",";
   json += "\"t7_order_filled_ms\":" + IntegerToString(t7);
   json += "}";
   
   json += "}";
   
   // Enviar al Agent via Named Pipe
   WriteToPipe(json);
   
   Print("ExecutionResult sent: command_id=", commandID, ", ticket=", ticket, ", success=", success);
}
```

---

### 3. CloseOrder con Ticket Exacto (i1)

**CAMBIO CRÍTICO i1**: El `CloseOrder` ahora incluye el `ticket` exacto del slave (no 0). Si `ticket != 0`, usar ese ticket directamente. Si `ticket == 0`, hacer fallback a búsqueda por `magic_number + symbol` (comportamiento i0).

```mql4
//+------------------------------------------------------------------+
//| Procesa CloseOrder recibido del Agent                            |
//+------------------------------------------------------------------+
void ProcessCloseOrder(string jsonMessage)
{
   // 1. Parse JSON
   CJAVal json;
   if(!json.Deserialize(jsonMessage)) {
       Print("ERROR: JSON inválido en CloseOrder");
       return;
   }
   
   // 2. Extraer campos
   string commandID = json["command_id"].ToStr();
   string tradeID = json["trade_id"].ToStr();
   int ticket = (int)json["ticket"].ToInt();           // ⚠️ i1: puede ser != 0
   string symbol = json["symbol"].ToStr();
   int magicNumber = (int)json["magic_number"].ToInt();
   
   Print("CloseOrder received: command_id=", commandID, ", trade_id=", tradeID,
         ", ticket=", ticket, ", symbol=", symbol, ", magic=", magicNumber);
   
   // 3. Resolver ticket
   int ticketToClose = 0;
   
   if(ticket != 0) {
       // i1: Ticket exacto proporcionado por Core
       ticketToClose = ticket;
       Print("i1: Usando ticket exacto del Core: ", ticket);
   } else {
       // i0: Fallback - buscar por magic_number + symbol
       Print("i1: Ticket=0, buscando por magic+symbol (fallback i0)");
       ticketToClose = FindTicketByMagicAndSymbol(magicNumber, symbol);
   }
   
   if(ticketToClose == 0) {
       Print("ERROR: No se encontró ticket para cerrar");
       SendCloseResult(commandID, tradeID, false, 0, "NOT_FOUND", "Ticket not found");
       return;
   }
   
   // 4. Cerrar orden
   bool closed = OrderClose(ticketToClose, OrderLots(), OrderClosePrice(), 3, clrRed);
   
   // 5. Enviar CloseResult
   if(closed) {
       SendCloseResult(commandID, tradeID, true, ticketToClose, "NO_ERROR", "");
   } else {
       int err = GetLastError();
       SendCloseResult(commandID, tradeID, false, ticketToClose,
                       ErrorCodeToString(err), ErrorDescription(err));
   }
}

//+------------------------------------------------------------------+
//| Busca ticket por magic number y símbolo (fallback i0)            |
//+------------------------------------------------------------------+
int FindTicketByMagicAndSymbol(int magicNumber, string symbol)
{
   for(int i = OrdersTotal() - 1; i >= 0; i--) {
       if(!OrderSelect(i, SELECT_BY_POS, MODE_TRADES)) continue;
       
       if(OrderMagicNumber() == magicNumber && OrderSymbol() == symbol) {
           return OrderTicket();
       }
   }
   
   return 0;  // No encontrado
}

//+------------------------------------------------------------------+
//| Envía CloseResult al Agent                                       |
//+------------------------------------------------------------------+
void SendCloseResult(string commandID, string tradeID, bool success,
                     int ticket, string errorCode, string errorMessage)
{
   // CloseResult se reporta como ExecutionResult en i0/i1
   // TODO i2: considerar mensaje específico CloseResult
   
   double closePrice = 0;
   if(success && OrderSelect(ticket, SELECT_BY_TICKET)) {
       closePrice = OrderClosePrice();
   }
   
   ulong timestampMs = (ulong)(TimeCurrent() * 1000);
   
   string json = "{";
   json += "\"command_id\":\"" + commandID + "\",";
   json += "\"trade_id\":\"" + tradeID + "\",";
   json += "\"success\":" + (success ? "true" : "false") + ",";
   json += "\"ticket\":" + IntegerToString(ticket) + ",";
   json += "\"error_code\":\"" + errorCode + "\"";
   
   if(errorMessage != "") {
       json += ",\"error_message\":\"" + errorMessage + "\"";
   }
   
   if(success && closePrice > 0) {
       json += ",\"executed_price\":" + DoubleToString(closePrice, 5);
   }
   
   json += "}";
   
   WriteToPipe(json);
   
   Print("CloseResult sent: ticket=", ticket, ", success=", success);
}
```

---

## 📐 Estructura Recomendada del Slave EA

```mql4
//+------------------------------------------------------------------+
//| Slave EA i1                                                       |
//+------------------------------------------------------------------+
#property copyright "Echo Trading Copier"
#property version   "1.00"
#property strict

// Imports
#include <JAson.mqh>  // JSON parser

// Named Pipe DLL
#import "echo_pipe_x86.dll"
   int OpenEchoPipe(string pipeName);
   string ReadFromEchoPipe(int handle);
   int WriteToEchoPipe(int handle, string message);
   int CloseEchoPipe(int handle);
#import

// Globals
int g_pipeHandle = -1;
string g_pipeName = "\\\\.\\pipe\\echo_slave_pipe";

//+------------------------------------------------------------------+
//| Expert initialization function                                    |
//+------------------------------------------------------------------+
int OnInit()
{
   // Abrir pipe al Agent
   g_pipeHandle = OpenEchoPipe(g_pipeName);
   if(g_pipeHandle < 0) {
       Print("ERROR: No se pudo conectar al Agent via Named Pipe");
       return INIT_FAILED;
   }
   
   Print("Slave EA i1 iniciado. Conectado al Agent.");
   return INIT_SUCCEEDED;
}

//+------------------------------------------------------------------+
//| Expert deinitialization function                                  |
//+------------------------------------------------------------------+
void OnDeinit(const int reason)
{
   if(g_pipeHandle >= 0) {
       CloseEchoPipe(g_pipeHandle);
   }
   Print("Slave EA i1 detenido.");
}

//+------------------------------------------------------------------+
//| Expert tick function                                              |
//+------------------------------------------------------------------+
void OnTick()
{
   // Leer comandos del Agent (non-blocking)
   if(g_pipeHandle >= 0) {
       string message = ReadFromEchoPipe(g_pipeHandle);
       if(StringLen(message) > 0) {
           ProcessCommand(message);
       }
   }
}

//+------------------------------------------------------------------+
//| Procesa comando recibido del Agent                               |
//+------------------------------------------------------------------+
void ProcessCommand(string jsonMessage)
{
   // Parse JSON para determinar tipo de comando
   CJAVal json;
   if(!json.Deserialize(jsonMessage)) {
       Print("ERROR: JSON inválido");
       return;
   }
   
   // Detectar tipo de comando por presencia de campos
   if(json["command_id"].m_type != jtNULL && json["side"].m_type != jtNULL) {
       // Es un ExecuteOrder
       ProcessExecuteOrder(jsonMessage);
   }
   else if(json["command_id"].m_type != jtNULL && json["ticket"].m_type != jtNULL) {
       // Es un CloseOrder
       ProcessCloseOrder(jsonMessage);
   }
   else {
       Print("WARN: Comando desconocido");
   }
}

// ... (funciones ProcessExecuteOrder, ProcessCloseOrder, etc. arriba)
```

---

## ✅ Checklist de Implementación i1

Slave EA:

- [ ] Incluir `trade_id` en comment de `OrderSend` con formato `"TID:<uuid>"`
- [ ] Reportar timestamps completos: t5 (recv), t6 (pre-send), t7 (post-send)
- [ ] Usar ticket exacto de `CloseOrder` si `ticket != 0` (i1)
- [ ] Implementar fallback a búsqueda por magic+symbol si `ticket == 0`
- [ ] Enviar `ExecutionResult` con todos los campos obligatorios
- [ ] Testing: verificar correlación correcta trade_id ↔ ticket en BD
- [ ] Testing: cierre exitoso con ticket exacto (sin colisiones)

---

## 🐛 Troubleshooting

### Comment truncado en BD
- MT4/MT5 limita comment a 31 caracteres
- UUIDv7 completo = 36 caracteres con guiones
- Solución: usar solo últimos 8-12 caracteres o formato corto

### Ticket no encontrado en CloseOrder
- Verificar que ExecutionResult se haya persistido correctamente
- Revisar BD: `SELECT * FROM echo.executions WHERE trade_id = '...'`
- Si ticket=0, verificar fallback por magic+symbol

### Timestamps negativos
- MT4 usa `GetTickCount()` que puede overflow cada 49 días
- Solución: usar timestamps relativos o DLL con timestamp absoluto

### CloseOrder cierra orden incorrecta
- i0: puede cerrar cualquier orden con mismo magic+symbol
- i1: debe usar ticket exacto del BD
- Verificar que Core resuelva ticket correctamente con CorrelationService

---

## 📚 Referencias

- RFC-003 Iteración 1: `/echo/docs/rfcs/RFC-003-iteration-1-implementation.md`
- RFC-002 (Timestamps): `/echo/docs/rfcs/RFC-002-iteration-0-implementation.md`
- Named Pipes IPC: `/echo/pipe/README.md`
- JAson MQL4: https://github.com/EarnForex/JAson

