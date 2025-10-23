# Roadmap micro-iterativo V1 “copiear”

Objetivo:
- POC funcional en 48 h (Thu–Fri, America/Santiago).
- Beta estable en 7–10 días.
- Cambios pequeños, validación E2E continua.

## Principios
- Cortes verticales: core + agente + EAs en cada iteración.
- Contratos versionados: `v0` mínimo, compatibles hacia adelante.
- Feature flags desde el día 1: `dry_run`, `use_symbol_map`, `catastrophic_sl`, `multi_slave`, `copy_sl_tp`.
- Idempotencia obligatoria: `trade_id` = UUIDv7 + store de Acks en core.
- Trazabilidad mínima: logs JSON + 3 métricas (latencia p50/p95, tasa de copia, rechazos).

## Pistas técnicas
- IPC EA↔agente: Named Pipes (MT4/MT5 friendly).
- agente↔core: gRPC bidi localhost.
- Config v0: YAML local. etcd después.
- Persistencia v0: in-memory + WAL local en agente (append-only) para replays cortos.
- SO objetivo de POC: Windows (servicio para agente).

---

## Iteración 0 — POC 48 h
**Scope**
- Símbolo único (ej. XAUUSD).
- 1 master MT4 → 1 slave MT4 en el mismo host.
- Solo órdenes a mercado (BUY/SELL, CLOSE). Sin SL/TP.
- Sizing fijo hardcodeado (ej. 0.10 lot).
- Contratos `v0`: `TradeIntent`, `CopyCmd`, `Ack`, `Heartbeat`.
- Core mínimo: router + dedupe en memoria.
- Agente: servicio Windows; canal NP + gRPC.
- EAs: Master emite intent; Slave ejecuta y reporta fill.

**Criterios de salida**
- p95 intra-host < 120 ms.
- 0 duplicados.
- 10 ejecuciones consecutivas correctas en demo local.

**Pros**: impacto inmediato, E2E real. **Contras**: sin SL/TP ni persistencia robusta.

---

## Iteración 1 — POC+ robusto (72 h)
**Scope**
- Confirmar SELL, BUY, CLOSE.
- Persistencia mínima: core guarda `intents` y `acks` en SQLite local (o Postgres si disponible).
- Idempotencia reforzada en core y agente.
- Health/Heartbeats y `retry` simple (`offquotes`, `busy`, `timeout` con backoff).
- Métricas: latencia, copias ok, rechazos, reintentos.

**Criterios**
- Reintentos < 3% en demo controlado.
- Recupera tras reinicio del agente sin perder órdenes pendientes.

**Pros**: confiabilidad básica. **Contras**: aún mono-host y sin SL/TP.

---

## Iteración 2 — SL catastrófico + filtros mínimos (2–3 días)
**Scope**
- SL catastrófico opcional por cuenta desde core (`catastrophic_sl=true`, distancia en pips).
- Filtros: `max_spread`, `max_age_ms` del intent, `max_slippage`.
- “Late copy drop”: no copiar si supera `max_age_ms`.
- Logging de motivos de rechazo estandarizado.

**Criterios**
- Sin rechazos silenciosos. Todos los drops con razón.
- SL catastrófico aplicado post-fill sin brecha.

**Pros**: protección real para demo externa. **Contras**: más ramas de error.

---

## Iteración 3 — Mapeo de símbolos y specs de broker (2 días)
**Scope**
- Tabla `canonical ⇄ broker_symbol` en core.
- Handshake Slave→agente→core con `min_lot`, `lot_step`, `stop_level`, `tick_value`, `digits`.
- Validación previa al envío y normalización de lotes.

**Criterios**
- Copia estable en al menos 2 símbolos.
- 0 rechazos por `volume invalid` y `stop level` en pruebas.

**Pros**: listo para brokers reales. **Contras**: mantener catálogo.

---

## Iteración 4 — Sizing configurable (1–2 días)
**Scope**
- Modos: `fixed`, `multiplier`, `risk_percent` (solo market, sin cálculo de SL de señal).
- Parámetros por cuenta y por símbolo.
- Guardas de límites: `min/max_lot`, redondeo a `lot_step`.

**Criterios**
- Error absoluto de sizing < `lot_step`.
- Cambios de sizing en caliente vía YAML reload.

**Pros**: usabilidad real. **Contras**: edge cases por `lot_step`.

---

## Iteración 5 — Multi-slave local y backpressure (2 días)
**Scope**
- 1 master → N slaves en el mismo host.
- Paralelismo sin head-of-line blocking. Cola por slave.
- Backpressure: si un slave está lento, no frena a los demás.

**Criterios**
- p95 por slave estable al escalar a N=10.
- 0 pérdidas bajo estrés moderado.

**Pros**: escala mínima. **Contras**: depuración más compleja.

---

## Iteración 6 — SL/TP de señal con offset y StopLevel-aware (2 días)
**Scope**
- Dos modos por cuenta:
  - Copiar SL/TP del master con offset en pips y ajuste a `stop_level`.
  - Sin SL/TP, cierre solo por señales del master.
- Reintentos de `OrderModify` post-fill si `stop_level` rechaza.

**Criterios**
- Ratio de éxito en `modify` > 99% con ajustes.
- Telemetría de ajustes aplicada.

**Pros**: paridad mayor con master. **Contras**: más latencia post-fill.

---

## Iteración 7 — Harden + empaquetado V1 (2 días)
**Scope**
- CLI mínima, servicio del agente instalable, EAs compilados.
- Panel Grafana básico (latencia, tasa copia, drop reasons).
- Manual corto y ejemplos de YAML.

**Criterios**
- Instalación limpia en máquina nueva en < 15 min.
- Demo pública sin intervención manual.

**Pros**: listo para real. **Contras**: trabajo no-funcional.

---

## Orden y dependencias
- Siempre vertical: contratos `v0` → core mínimo → agente mínimo → EAs mínimos → E2E.
- Persistencia ligera antes de filtros. Filtros antes de mapeo. Mapeo antes de sizing.
- Sizing antes de multi-slave. Multi-slave antes de SL/TP de señal.

## Estrategia de ramas
- `main`: estable y demo-ready.
- `feat/*`: una historia por iteración.
- CI gates: build, linters, tests de contratos, smoke E2E local.

## Riesgos y mitigación
- **StopLevel y rechazos**: posponer SL/TP de señal a Iteración 6; SL catastrófico desde Iteración 2.
- **Inestabilidad Windows/NP**: watchdog y reconexión; logs con causa raíz.
- **Cambios de contrato**: `v0` congelado por 3 iteraciones; campos nuevos como opcionales.
- **Latencia**: medir desde Iteración 0; límites claros por iteración.

## Alternativa “POC turbo”
- Core + agente en un solo binario para la demo. API interna equivalente.
- Pros: menos procesos y redes. Contras: menor desacople. Separar en Iteración 1.

