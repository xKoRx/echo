# Clients - Conectores de Trading

Los **clients** son los conectores que se ejecutan en las plataformas de trading (MT4, MT5, NinjaTrader, etc.) y se comunican con el **agent** local via IPC.

## Tipos de Clientes

### MT4/MT5 (Expert Advisors - MQL)

```
clients/
├── mt4/
│   ├── MasterEA.mq4        # EA para cuentas master
│   ├── SlaveEA.mq4         # EA para cuentas slave
│   ├── Include/
│   │   └── EchoIPC.mqh     # Librería IPC Named Pipes
│   └── README.md
├── mt5/
│   ├── MasterEA.mq5
│   ├── SlaveEA.mq5
│   ├── Include/
│   │   └── EchoIPC.mqh
│   └── README.md
```

### Otros (Futuro)

```
├── ninja/                  # NinjaTrader C#
├── ctrader/                # cTrader C#
├── go/                     # Cliente custom Go
└── python/                 # Cliente custom Python
```

## Responsabilidades

### Master EA

- Detectar trades abiertos en la cuenta master
- Enviar `TradeIntent` al agent vía Named Pipe
- Reportar cierres y modificaciones
- Enviar specs de símbolo al conectar

### Slave EA

- Recibir comandos (`ExecuteOrder`, `CloseOrder`, `ModifyOrder`) del agent
- Ejecutar órdenes en el terminal MT4/MT5
- Reportar resultados (`ExecutionResult`)
- Enviar estado de cuenta periódicamente

## Protocolo IPC

Los EAs se comunican con el agent vía **Named Pipes** usando JSON:

```json
// Master → Agent
{
  "type": "trade_intent",
  "trade_id": "uuid-v7",
  "timestamp_ms": 1234567890,
  "symbol": "XAUUSD",
  "side": "BUY",
  "lot_size": 0.10,
  "price": 1950.25,
  "magic_number": 12345,
  "ticket": 987654
}
```

```json
// Agent → Slave
{
  "type": "execute_order",
  "command_id": "uuid",
  "trade_id": "uuid-v7",
  "symbol": "XAUUSD",
  "side": "BUY",
  "lot_size": 0.05,
  "magic_number": 12345
}
```

```json
// Slave → Agent
{
  "type": "execution_result",
  "command_id": "uuid",
  "success": true,
  "ticket": 123456,
  "executed_price": 1950.30
}
```

## Instalación

### MT4/MT5

1. Copiar EAs a `<Terminal>/MQL4/Experts/` o `<Terminal>/MQL5/Experts/`
2. Copiar librería `EchoIPC.mqh` a `<Terminal>/MQL4/Include/` o `<Terminal>/MQL5/Include/`
3. Compilar en MetaEditor
4. Arrastrar EA al gráfico

### Configuración

Parámetros del EA:

```
AgentPipeName = "\\.\pipe\echo_master_001"  // Para Master
AgentPipeName = "\\.\pipe\echo_slave_001"    // Para Slave
MagicNumber = 12345
```

## Desarrollo

### MT4

```mql4
// MasterEA.mq4
#include <EchoIPC.mqh>

void OnTick() {
  // Detectar nuevo trade
  if (OrdersTotal() > prevOrders) {
    SendTradeIntent(...);
  }
}
```

### MT5

```mql5
// MasterEA.mq5
#include <EchoIPC.mqh>

void OnTrade() {
  // Detectar trade en historial
  SendTradeIntent(...);
}
```

## Tests

Los EAs se prueban en:

- **Demo MT4/MT5** (manual)
- **Strategy Tester** (backtesting)
- **test_e2e/** (integración con agent simulado)

## Próximos Pasos (Iteración 0)

- [ ] Implementar MasterEA.mq4 básico
- [ ] Implementar SlaveEA.mq4 básico
- [ ] Librería EchoIPC.mqh con Named Pipes
- [ ] Probar en demo local

