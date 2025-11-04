# Runbook de Despliegue – Echo v3

## 1. Preparación
- Verificar acceso a los entornos (PostgreSQL, ETCD, hosts del Core, Agents y estaciones MT4).
- Confirmar variables en ETCD:
  - `core/canonical_symbols` actualizado con la whitelist.
  - `core/symbols/unknown_action` en `warn` para rollout seguro.
- Descargar artefactos generados por `./build_all.sh`:
  - `bin/echo-core`
  - `bin/echo-agent.exe`
  - `bin/echo_pipe_x86.dll` y `bin/echo_pipe_x64.dll`
  - `bin/master.mq4`, `bin/slave.mq4`, `bin/JAson.mqh`

## 2. Migraciones de Base de Datos
Ejecutar en PostgreSQL (entorno por entorno) el script `deploy/postgres/migrations/i3_symbol_specs_quotes.sql`:
```sql
psql $ECHO_DB_URL -f deploy/postgres/migrations/i3_symbol_specs_quotes.sql
```
Esto crea/actualiza:
- `echo.account_symbol_spec`
- `echo.symbol_quote_latest`

## 3. Despliegue del Core (Linux x86_64)
1. Copiar `bin/echo-core` al host del Core.
2. Detener servicio actual: `systemctl stop echo-core` (o equivalente).
3. Reemplazar binario y ajustar permisos (`chmod +x`).
4. Iniciar servicio: `systemctl start echo-core`.
5. Validar logs (`journalctl -u echo-core`) y métricas `echo.symbol.*`.

## 4. Despliegue de Agents (Windows x86_64)
1. Copiar `bin/echo-agent.exe` a cada host Agent.
2. Detener servicio/Task Scheduler actual.
3. Reemplazar binario y reiniciar el servicio del Agent.
4. Validar logs: buscar `SymbolSpecReport forwarded`.

## 5. Actualización de DLL y EAs MT4
1. Copiar `bin/echo_pipe_x86.dll` y `bin/echo_pipe_x64.dll` a `MQL4\Libraries` y `MQL5\Libraries` según plataforma.
2. Para cada cuenta MT4:
   - Abrir MetaEditor y reemplazar `master.mq4`, `slave.mq4`, `JAson.mqh` en `MQL4\Experts` / `MQL4\Include`.
   - Compilar `master.mq4` y `slave.mq4`.
   - Configurar el nuevo input `SymbolMappings` en el Slave.
3. Reiniciar los EAs y verificar log MT4: `SymbolSpecReport forwarded to Core` y `QuoteSnapshot forwarded to Core`.

## 6. Monitoreo de Rollout
- Core en modo `warn`: validar métricas
  - `echo.symbol.spec_reported`
  - `echo.symbol.quote_received`
  - `echo.symbol.lookup`
- Revisar logs del Core buscando `SymbolSpecReport processed` y ajustes de `StopLoss`/`TakeProfit`.
- Asegurar que no existan entradas `symbol mapping missing`.

## 7. Conmutar a Modo Estricto
Una vez todas las cuentas reporten specs y quotes (24h sin incidentes):
1. Cambiar en ETCD: `core/symbols/unknown_action = reject`.
2. Confirmar que órdenes con símbolos desconocidos sean rechazadas correctamente.

## 8. Post-Deploy
- Documentar evidencias (capturas de métricas, logs clave).
- Informar al equipo de operaciones y trading.
- Agendar revisión a las 24h para confirmar estabilidad.
