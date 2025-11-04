# Escalamiento Iteración i3 – Síntesis para AI Externa

## Referencias Clave
- `docs/rfcs/RFC-004c-iteracion-3-parte-final-slave-registro.md`
- `docs/rfcs/RFC-004-iteracion-3-catalogo-simbolos.md`
- `docs/runbooks/deploy_v3.md`
- Código relevante:
  - `clients/mt4/master.mq4`
  - `clients/mt4/slave.mq4`
  - `agent/internal/pipe_manager.go`
  - `core/internal/router.go`
  - `deploy/postgres/migrations/i3_symbol_specs_quotes.sql`

## Estado Actual de la Iteración
- SDK/Proto actualizado con `SymbolSpecReport` y `SymbolQuoteSnapshot`.
- Core integra `SymbolSpecService` y `SymbolQuoteService`, ajuste de SL/TP según quotes.
- Agent traduce `handshake`, `symbol_spec_report`, `quote_snapshot` y reenvía al Core.
- Slave EA envía mappings, specs y snapshots cada 250 ms.
- Master EA modificado para incluir `stop_loss`/`take_profit` en `trade_intent`.
- Script `build_all.sh` genera todos los artefactos (`echo-core`, `echo-agent.exe`, DLLs, Master/Slave).

## Problemas Pendientes
1. **Slaves ejecutan órdenes sin SL/TP** a pesar de que el Master y el Core deberían transmitirlos/ajustarlos.
2. **Tabla `echo.account_symbol_spec` permanece vacía** aun después de enviar `symbol_spec_report` desde los Slaves con whitelist `core/canonical_symbols = "XAUUSD,DAX"`.

## Prompt para la IA Externa
```
Contexto: Estamos en la iteración i3 del proyecto Echo Trade Copier. Revisa los documentos RFC-004c (iteración 3 parte final) y RFC-004 (catálogo de símbolos por cuenta), además del runbook deploy_v3. El código relevante está en master.mq4, slave.mq4, agent/internal/pipe_manager.go, core/internal/router.go y la migración deploy/postgres/migrations/i3_symbol_specs_quotes.sql.

Estado actual:
- SDK y Core soportan SymbolSpecReport + SymbolQuoteSnapshot.
- Agent procesa handshake, symbol_spec_report y quote_snapshot.
- Slave envía mappings, specs y snapshots cada 250 ms.
- Master supuestamente manda SL/TP en trade_intent.
- build_all.sh genera artefactos.

Problemas:
1. Slave replica órdenes sin stop loss ni take profit.
2. Tabla echo.account_symbol_spec no obtiene registros a pesar de enviar symbol_spec_report y tener core/canonical_symbols = "XAUUSD,DAX".

Necesito que traces la causa raíz y propongas la solución completa para ambos problemas, validando la data que fluye entre Master → Agent → Core → Slave.
```

## Acción Final Requerida
- Ejecutar `./build_all.sh` al cierre de la investigación para asegurar que todos los binarios y scripts queden compilables y consistentes.
