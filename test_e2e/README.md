# Test E2E - Tests End-to-End

Tests de **integración completa** que validan el flujo completo:

```
Master EA → Agent → Core → Agent → Slave EA
```

## Estructura

```
test_e2e/
├── scenarios/
│   ├── poc_single_copy_test.go       # POC: 1 master → 1 slave
│   ├── multi_slave_test.go           # Multi-slave
│   └── failure_scenarios_test.go     # Errores y reintentos
├── fixtures/
│   ├── test_data.json                # Datos de prueba
│   └── mock_trades.json
├── mocks/
│   └── generated_mocks.go            # Mocks de interfaces
└── README.md
```

## Scope

Los tests E2E pueden:

- Importar `core`, `agent`, `sdk`
- Levantar servicios en memoria
- Simular EAs con mocks
- Validar latencias, métricas, logs

## Ejecución

```bash
cd test_e2e
go test -v ./scenarios/

# Con timeout extendido
go test -v -timeout 5m ./scenarios/

# Específico
go test -v -run TestPOC_SingleCopy
```

## Ejemplo

```go
func TestPOC_SingleCopy(t *testing.T) {
    ctx := context.Background()
    
    // Arrange: Levantar core y agent
    core := startTestCore(t, ctx)
    defer core.Shutdown(ctx)
    
    agent := startTestAgent(t, ctx, core.Address())
    defer agent.Shutdown(ctx)
    
    // Act: Simular TradeIntent desde master
    intent := &echov1.TradeIntent{
        TradeId: "test-001",
        Symbol: "XAUUSD",
        Side: echov1.OrderSide_ORDER_SIDE_BUY,
        LotSize: 0.10,
    }
    
    err := agent.SendIntent(ctx, intent)
    require.NoError(t, err)
    
    // Assert: Verificar que llegó ExecuteOrder al slave
    cmd := waitForCommand(t, agent, 1*time.Second)
    assert.Equal(t, "test-001", cmd.TradeId)
    assert.Equal(t, "XAUUSD", cmd.Symbol)
}
```

## Fixtures

```json
// fixtures/test_data.json
{
  "master_account": {
    "account_id": "MT4-MASTER-001",
    "balance": 10000,
    "equity": 10500
  },
  "slave_accounts": [
    {
      "account_id": "MT5-SLAVE-001",
      "balance": 5000
    }
  ],
  "test_trades": [
    {
      "symbol": "XAUUSD",
      "side": "BUY",
      "lot_size": 0.10,
      "price": 1950.25
    }
  ]
}
```

## Mocks

Usamos `testify/mock` para interfaces:

```go
type MockAgent struct {
    mock.Mock
}

func (m *MockAgent) SendIntent(ctx context.Context, intent *echov1.TradeIntent) error {
    args := m.Called(ctx, intent)
    return args.Error(0)
}
```

## Cobertura

Objetivo: **≥85%** en flujos críticos

```bash
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## CI/CD

Los tests E2E se ejecutan en CI:

```yaml
# .github/workflows/test.yml
- name: Run E2E tests
  run: |
    cd test_e2e
    go test -v -timeout 5m ./...
```

## Próximos Pasos (Iteración 0)

- [ ] Test POC happy path
- [ ] Validar latencia < 120ms
- [ ] Test deduplicación
- [ ] Test reintentos básicos

