# Master EA — Guía de Implementación i1

## 📋 Descripción

El Master EA es el Expert Advisor que se ejecuta en terminales MT4/MT5 master y genera **TradeIntents** que serán copiados a slaves.

## 🎯 Cambios i1 vs i0

### Cambios Obligatorios

1. **UUIDv7 Generation**: Generar `trade_id` como UUIDv7 (RFC 9562) en lugar de UUIDv4
2. **Comment con trade_id**: Incluir el `trade_id` en el comment de `OrderSend` para fallback de correlación

### Cambios Opcionales

- Mejorar timestamps t0 para mayor precisión
- Validar formato UUIDv7 antes de enviar

---

## 🔑 Componentes Clave

### 1. UUIDv7 Generation (i1)

UUIDv7 es ordenable por tiempo y mejora la correlación en BD.

**Formato**:
```
xxxxxxxx-xxxx-7xxx-yxxx-xxxxxxxxxxxx
```

Donde:
- Primeros 48 bits: timestamp Unix ms
- Version nibble: `7` (posición 14)
- Variant nibble: `8`, `9`, `A` o `B` (posición 19)

**Implementación MQL4** (helper function):

```mql4
//+------------------------------------------------------------------+
//| Genera UUID v7 (ordenable por tiempo)                            |
//| RFC 9562: https://datatracker.ietf.org/doc/html/rfc9562         |
//+------------------------------------------------------------------+
string GenerateUUIDv7()
{
   // 1. Obtener timestamp actual en milisegundos
   ulong timestampMs = (ulong)(TimeCurrent() * 1000 + TimeLocal() % 1000);
   
   // 2. Generar bytes random (10 bytes)
   uchar random[10];
   for(int i = 0; i < 10; i++) {
       random[i] = (uchar)MathRand() % 256;
   }
   
   // 3. Construir UUID
   // Formato: tttttttt-tttt-7xxx-yxxx-xxxxxxxxxxxx
   
   // Timestamp en los primeros 48 bits (6 bytes)
   ulong ts_high = (timestampMs >> 16) & 0xFFFFFFFF;  // 32 bits altos
   ushort ts_low = (ushort)((timestampMs & 0xFFFF));  // 16 bits bajos
   
   // Construir partes del UUID
   string part1 = StringFormat("%08X", ts_high);
   string part2 = StringFormat("%04X", ts_low);
   
   // part3: 4 hex digits con version 7
   ushort part3_val = (random[0] << 8) | random[1];
   part3_val = (part3_val & 0x0FFF) | 0x7000;  // Set version = 7
   string part3 = StringFormat("%04X", part3_val);
   
   // part4: 4 hex digits con variant bits
   ushort part4_val = (random[2] << 8) | random[3];
   part4_val = (part4_val & 0x3FFF) | 0x8000;  // Set variant = 10xx
   string part4 = StringFormat("%04X", part4_val);
   
   // part5: 12 hex digits (6 bytes restantes)
   string part5 = StringFormat("%02X%02X%02X%02X%02X%02X",
                                 random[4], random[5], random[6],
                                 random[7], random[8], random[9]);
   
   // 4. Combinar con guiones
   return part1 + "-" + part2 + "-" + part3 + "-" + part4 + "-" + part5;
}
```

**Nota crítica**: MT4 no tiene microsegundos nativos, así que usamos `TimeCurrent()` en segundos + `TimeLocal() % 1000` como aproximación de milisegundos. Para mayor precisión, considerar usar DLL externa con `GetTickCount()`.

---

### 2. TradeIntent con trade_id

Al emitir un TradeIntent, usar el UUID v7 generado:

```mql4
//+------------------------------------------------------------------+
//| Emite TradeIntent al Agent via Named Pipe                        |
//+------------------------------------------------------------------+
void SendTradeIntent(string symbol, int side, double lotSize, double price,
                     int magicNumber, int ticket,
                     double stopLoss = 0, double takeProfit = 0)
{
   // 1. Generar trade_id UUIDv7
   string tradeID = GenerateUUIDv7();
   
   // 2. Timestamp actual en ms
   ulong timestampMs = (ulong)(TimeCurrent() * 1000);
   
   // 3. Construir JSON del TradeIntent
   string json = "{";
   json += "\"trade_id\":\"" + tradeID + "\",";
   json += "\"timestamp_ms\":" + IntegerToString(timestampMs) + ",";
   json += "\"client_id\":\"" + IntegerToString(AccountNumber()) + "\",";
   json += "\"symbol\":\"" + symbol + "\",";
   json += "\"side\":" + IntegerToString(side) + ",";  // 1=BUY, 2=SELL
   json += "\"lot_size\":" + DoubleToString(lotSize, 2) + ",";
   json += "\"price\":" + DoubleToString(price, 5) + ",";
   json += "\"magic_number\":" + IntegerToString(magicNumber) + ",";
   json += "\"ticket\":" + IntegerToString(ticket);
   
   // SL/TP opcionales (i1)
   if(stopLoss > 0) {
       json += ",\"stop_loss\":" + DoubleToString(stopLoss, 5);
   }
   if(takeProfit > 0) {
       json += ",\"take_profit\":" + DoubleToString(takeProfit, 5);
   }
   
   // Timestamps (i1: t0 = Master EA genera intent)
   json += ",\"timestamps\":{";
   json += "\"t0_master_ea_ms\":" + IntegerToString(timestampMs);
   json += "}";
   
   json += "}";
   
   // 4. Enviar al Agent via Named Pipe
   WriteToPipe(json);
   
   // 5. Log para debugging
   Print("TradeIntent sent: trade_id=", tradeID, ", ticket=", ticket, ", symbol=", symbol);
}
```

---

### 3. Comment con trade_id (Fallback de Correlación)

**CRÍTICO para i1**: Al abrir una orden en el slave vía `OrderSend`, incluir el `trade_id` en el comment para permitir correlación si falla el tracking normal.

**Ejemplo de integración**:

```mql4
//+------------------------------------------------------------------+
//| OnTick handler con generación de trade_id                        |
//+------------------------------------------------------------------+
void OnTick()
{
   // Lógica de estrategia...
   
   if(SignalToBuy()) {
       // 1. Generar trade_id UUIDv7 ANTES de OrderSend
       string tradeID = GenerateUUIDv7();
       
       // 2. Abrir orden en Master con trade_id en comment
       int ticket = OrderSend(
           Symbol(),
           OP_BUY,
           0.10,
           Ask,
           3,
           0,  // SL (opcional)
           0,  // TP (opcional)
           "TID:" + tradeID,  // ⚠️ IMPORTANTE: Incluir trade_id en comment
           MagicNumber,
           0,
           clrGreen
       );
       
       if(ticket > 0) {
           // 3. Enviar TradeIntent con el MISMO trade_id
           SendTradeIntent(Symbol(), 1, 0.10, Ask, MagicNumber, ticket, 0, 0);
       }
   }
}
```

**Formato recomendado del comment**: `"TID:<uuid>"` o solo `"<uuid>"`.

---

## 📐 Estructura Recomendada del Master EA

```mql4
//+------------------------------------------------------------------+
//| Master EA i1                                                      |
//+------------------------------------------------------------------+
#property copyright "Echo Trading Copier"
#property version   "1.00"
#property strict

// Imports
#include <JAson.mqh>  // JSON parser

// Named Pipe DLL
#import "echo_pipe_x86.dll"
   int OpenEchoPipe(string pipeName);
   int WriteToEchoPipe(int handle, string message);
   int CloseEchoPipe(int handle);
#import

// Globals
int g_pipeHandle = -1;
string g_pipeName = "\\\\.\\pipe\\echo_master_pipe";

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
   
   Print("Master EA i1 iniciado. Conectado al Agent.");
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
   Print("Master EA i1 detenido.");
}

//+------------------------------------------------------------------+
//| Expert tick function                                              |
//+------------------------------------------------------------------+
void OnTick()
{
   // Tu estrategia aquí
   // Al detectar señal, generar trade_id v7 y enviar TradeIntent
}

//+------------------------------------------------------------------+
//| Genera UUID v7                                                    |
//+------------------------------------------------------------------+
string GenerateUUIDv7()
{
   // Implementación arriba
}

//+------------------------------------------------------------------+
//| Envía TradeIntent al Agent                                        |
//+------------------------------------------------------------------+
void SendTradeIntent(...)
{
   // Implementación arriba
}

//+------------------------------------------------------------------+
//| Envía TradeClose al Agent                                         |
//+------------------------------------------------------------------+
void SendTradeClose(string tradeID, int ticket, double closePrice)
{
   ulong timestampMs = (ulong)(TimeCurrent() * 1000);
   
   string json = "{";
   json += "\"trade_id\":\"" + tradeID + "\",";
   json += "\"timestamp_ms\":" + IntegerToString(timestampMs) + ",";
   json += "\"client_id\":\"" + IntegerToString(AccountNumber()) + "\",";
   json += "\"account_id\":\"" + IntegerToString(AccountNumber()) + "\",";
   json += "\"ticket\":" + IntegerToString(ticket) + ",";
   json += "\"symbol\":\"" + Symbol() + "\",";
   json += "\"magic_number\":" + IntegerToString(MagicNumber) + ",";
   json += "\"close_price\":" + DoubleToString(closePrice, 5);
   json += "}";
   
   WriteToPipe(json);
   
   Print("TradeClose sent: trade_id=", tradeID, ", ticket=", ticket);
}

//+------------------------------------------------------------------+
//| Write to Named Pipe                                               |
//+------------------------------------------------------------------+
void WriteToPipe(string message)
{
   if(g_pipeHandle < 0) {
       Print("ERROR: Pipe no conectado");
       return;
   }
   
   int result = WriteToEchoPipe(g_pipeHandle, message + "\n");
   if(result < 0) {
       Print("ERROR: Fallo al escribir en pipe");
   }
}
```

---

## ✅ Checklist de Implementación i1

Master EA:

- [ ] Implementar `GenerateUUIDv7()` con formato RFC 9562
- [ ] Validar que version nibble = `7` y variant nibble = `8/9/A/B`
- [ ] Incluir `trade_id` en comment de `OrderSend` con formato `"TID:<uuid>"`
- [ ] Enviar `TradeIntent` con timestamps completos (t0)
- [ ] Enviar `TradeClose` con mismo `trade_id` usado en apertura
- [ ] Testing: verificar que UUIDs generados sean únicos y ordenables por tiempo
- [ ] Testing: correlación exitosa en BD usando comment como fallback

---

## 🐛 Troubleshooting

### UUID v7 no válido
- Verificar que version nibble sea `7` (posición 14 del string)
- Verificar que variant nibble sea `8`, `9`, `A` o `B` (posición 19)

### Comment truncado
- MT4/MT5 limita comment a 31 caracteres
- UUIDv7 completo = 36 caracteres
- Solución: usar formato corto sin guiones o solo últimos 8 caracteres

### Timestamp impreciso
- MT4 no tiene sub-segundo nativo
- Considerar DLL con `GetTickCount()` para milisegundos reales

---

## 📚 Referencias

- RFC 9562 (UUIDv7): https://datatracker.ietf.org/doc/html/rfc9562
- RFC-003 Iteración 1: `/echo/docs/rfcs/RFC-003-iteration-1-implementation.md`
- Named Pipes IPC: `/echo/pipe/README.md`

