---
rfc_id: "RFC-00X"
title: "Iteración X — <título descriptivo>"
version: "1.0"
status: "draft"            # draft | in_review | approved | implemented | deprecated
owner_arch: "<equipo/rol>" # p.ej. "Arquitectura Echo"
owner_dev: "<equipo/rol>"  # p.ej. "Echo Core"
owner_qa: "<equipo/rol>"
date: "YYYY-MM-DD"
iteration: "X"
type: "feature"            # feature | refactor | infra | observability | bugfix
depends_on:
  - "docs/00-contexto-general.md"
  - "docs/01-arquitectura-y-roadmap.md"
  - "docs/rfcs/RFC-architecture.md"
related_rfcs:
  - "docs/rfcs/RFC-00Y-iteracion-Y-algo.md"
tags:
  - "core"
  - "agent"
  - "sdk"
  - "ea"
---

# RFC-00X: Iteración X — <título>

> **Para qué sirve esta sección:** El front matter concentra toda la metadata que usan los agentes, humanos y tooling (búsquedas, dashboards, etc.).

---

## 1. Resumen ejecutivo

> **Para qué sirve:** TL;DR de la iteración. Cualquier persona (dev, QA, PM) debería entender en 1 minuto qué cambia y por qué.

- En una o dos frases: problema actual y objetivo del cambio.
- En una o dos frases: enfoque de solución (qué se introduce / modifica).
- Beneficio esperado (negocio / técnico).
- Principios clave involucrados (PR-ROB, PR-MOD, PR-ESC, etc.).

---

## 2. Contexto y motivación

> **Para qué sirve:** Deja claro el “antes”, el problema real y por qué vale la pena tocar el sistema.

### 2.1 Estado actual

- Cómo funciona hoy la parte afectada (Core, Agent, SDK, EAs, DB, etc.).
- Referencias explícitas a código y docs:
  - `echo/core/...`
  - `echo/agent/...`
  - `sdk/...`
  - Tablas relevantes en PostgreSQL / otros stores.

### 2.2 Problema / gaps

- Limitaciones concretas (funcionales, de performance, de robustez, de observabilidad, etc.).
- Riesgos actuales: qué podría salir mal si seguimos igual.
- Ejemplos reales (logs, trazas, incidentes) si existen.

### 2.3 Objetivo de la iteración

- Qué se quiere lograr con esta iteración X.
- Cómo se alinea con roadmap / arquitectura general.

---

## 3. Objetivos medibles (Definition of Done)

> **Para qué sirve:** Define “hecho” en términos verificables. Dev y QA usan esto como checklist duro.

- Métricas / SLOs específicos:
  - p.ej. “Latencia adicional del cálculo ≤ 2 ms p95 por orden (span `core.risk.calculate`).”
  - p.ej. “Cobertura ≥ 85% en paquete `core/internal/foo` y ≥ 80% en cambios de SDK/Agent.”
  - p.ej. “0 órdenes enviadas sin `StopLoss` válido bajo política X.”
- Criterios de aceptación funcionales:
  - Qué comportamientos deben verse sí o sí (happy path).
  - Qué comportamientos deben ser explícitamente rechazados (bad path).
- Criterios de aceptación no funcionales:
  - Robustez, idempotencia, BWC, seguridad, etc.

---

## 4. Alcance

> **Para qué sirve:** Aclara qué SÍ entra en esta iteración y qué se empuja a futuro. Evita que el scope se descontrole.

### 4.1 Dentro de alcance

- Lista concreta de cosas que se van a implementar:
  - Nuevos modos de operación / políticas.
  - Nuevos contratos / mensajes / campos.
  - Cambios en componentes específicos.

### 4.2 Fuera de alcance (y futuro)

- Lo que explícitamente NO se hará ahora (aunque esté relacionado).
- Ideas para iteraciones futuras (iX+1, iX+2, etc.).

---

## 5. Arquitectura de solución

> **Para qué sirve:** Explica el “cómo” a nivel de sistema. Es la foto grande que guía a devs y QA.

### 5.1 Visión general

- Descripción narrativa de punta a punta:
  - Desde el evento de entrada (EA master / mensaje / API) hasta la acción final (orden enviada, registro en DB, etc.).
- Si aplica, incluir diagrama (Mermaid):
  - Diagrama de componentes o de secuencia.

### 5.2 Componentes afectados (touchpoints)

> Tabla rápida para saber qué archivos/proyectos tocar.

| Componente           | Tipo de cambio           | BWC (sí/no) | Notas breves |
|----------------------|-------------------------|------------|--------------|
| `echo/core/...`      | Lógica nueva / refactor | sí         | ...          |
| `echo/agent/...`     | Validaciones, metrics   | sí         | ...          |
| `sdk/proto/...`      | Campos nuevos           | sí         | ...          |
| `ea/master/slave`    | Payloads/reporting      | sí/no      | ...          |
| DB `schema`          | Migración / índice      | sí/no      | ...          |

### 5.3 Flujos principales

- Describir paso a paso los flujos clave:
  - Happy path principal.
  - Flujos alternativos y reintentos.
- Para cada flujo, indicar:
  - Entradas (mensajes, eventos, campos).
  - Decisiones importantes (ramas).
  - Salidas.

### 5.4 Reglas de negocio y casos borde

- Reglas de negocio nuevas o modificadas (explícitas, no implícitas).
- Casos borde que el sistema debe manejar:
  - Datos faltantes / inválidos.
  - Timeouts, reintentos, in-flight changes.
  - Ejemplos concretos (con números/códigos reales).

---

## 6. Contratos, datos y persistencia

> **Para qué sirve:** Es la parte que después se transforma en protos, structs, tablas y migraciones. Dev no tiene que adivinar nada.

### 6.1 Mensajes / contratos (SDK, gRPC, EAs, APIs)

- Listar mensajes nuevos y campos agregados:
  - `proto/v1/...`:
    - `message Foo { ... }`
    - Campos nuevos (tipo, semántica, ejemplos).
- Códigos de error nuevos o cambiados:
  - Constante, significado, en qué casos se usa.
- Reglas de compatibilidad:
  - Cómo conviven protocol_version N y N-1.
  - Qué pasa con agentes/EAs viejos.

### 6.2 Modelo de datos y esquema

- Tablas afectadas (PostgreSQL u otros):
  - Nuevas tablas.
  - Nuevas columnas / índices.
  - Restricciones (UNIQUE, FK, CHECK, etc.).
- Describir cada cambio con:
  - Nombre de columna.
  - Tipo.
  - Default / nullability.
  - Semántica (qué representa, quién la escribe, quién la lee).
- Migraciones:
  - Nombre de archivos de migración propuestos.
  - Estrategia de backfill si aplica (online / offline).

### 6.3 Configuración, flags y parámetros

- Nuevos flags / env vars / claves en ETCD:
  - Nombre, tipo, default.
  - Quién lo configura (infra, mesa, dev).
- Feature flags:
  - Nombre del flag.
  - Qué habilita/deshabilita.
  - Estrategia de rollout (por cuenta, por estrategia, global, etc.).

---

## 7. Principios de diseño y trade-offs

> **Para qué sirve:** Conecta la solución con tus principios (PR-ROB, PR-MOD, etc.) y documenta decisiones difíciles.

- Principios fortalecidos:
  - PR-ROB (robustez): cómo se mejora la tolerancia a fallos.
  - PR-MOD (modularidad): nuevas separaciones de responsabilidad.
  - PR-ESC (escalabilidad), PR-OBS (observabilidad), etc.
- Trade-offs aceptados:
  - Qué se sacrificó (simplicidad, tiempo de respuesta, etc.).
  - Por qué se aceptó ese costo.

---

## 8. Observabilidad (logs, métricas, trazas)

> **Para qué sirve:** Define la telemetría desde el diseño. Dev implementa y QA/operaciones saben qué mirar en Grafana/Jaeger.

### 8.1 Métricas

- Métricas nuevas / extendidas:
  - Nombre completo (namespace).
  - Tipo (counter, histogram, gauge).
  - Labels obligatorios.
  - Semántica y ejemplos de uso.
- Ejemplo:
  - `echo.core.risk.expected_loss` (histogram)
    - labels: `risk_currency`, `policy_type`, `account_id`, `strategy_id`.

### 8.2 Logs estructurados

- Nuevos campos en logs:
  - Claves JSON, tipo y semántica.
- Momentos clave donde se loguea:
  - Decisiones, rechazos, errores no fatales, etc.
- Reglas de PII / sensibilidad si aplica.

### 8.3 Trazas y spans

- Spans nuevos o modificados:
  - Nombre del span.
  - Atributos importantes (semconv internos).
- Relación con métricas (link entre spans y métricas).

---

## 9. Plan de pruebas (Dev y QA)

> **Para qué sirve:** Es la guía directa para qué probar. Desde aquí el Dev extrae tests unitarios/integración y QA arma E2E, smoke, regresión.

### 9.1 Casos de uso E2E

- Tabla de escenarios de negocio:

| ID Escenario | Descripción breve                      | Precondiciones                 | Resultado esperado           |
|--------------|----------------------------------------|--------------------------------|------------------------------|
| E2E-01       | ...                                    | Cuenta X con política Y       | Orden aceptada con Z        |
| E2E-02       | ...                                    | Falta dato crítico            | Rechazo con error ABC       |

### 9.2 Pruebas del Dev (unit / integración)

- Áreas mínimas que deben tener tests unitarios:
  - Paquetes / funciones clave.
- Tests de integración:
  - Qué caminos deben estar cubiertos.
  - Dependencias a mockear / fakear.

### 9.3 Pruebas de QA (E2E, smoke, regresión, no funcionales)

- E2E detallados que QA debe implementar.
- Smoke tests para validar despliegues.
- Pruebas de regresión: qué flujos críticos no pueden romperse.
- Pruebas no funcionales si aplica:
  - Carga, estrés, resiliencia.

### 9.4 Datos de prueba

- Fixtures / seeds necesarios.
- Cuentas, estrategias, símbolos, estados del sistema requeridos.

---

## 10. Plan de rollout, BWC y operación

> **Para qué sirve:** Define cómo se despliega y qué hacer si sale mal. Evita improvisar en producción.

### 10.1 Estrategia de despliegue

- Orden de despliegue por componentes:
  - SDK → Agent → Core → EAs, etc.
- Entornos:
  - Dev / staging / prod.
- Dependencias externas (brokers, colas, etc.).

### 10.2 Backward compatibility (BWC)

- Cómo se garantiza que agentes / EAs / versiones viejas sigan funcionando.
- Ventana de compatibilidad (cuánto tiempo convivirán N y N-1).

### 10.3 Rollback y mitigación

- Qué se hace si hay que revertir:
  - Cómo deshabilitar feature flags.
  - Cómo revertir migraciones (si aplica).
- Riesgos de datos y cómo mitigarlos.

### 10.4 Operación y soporte

- Nuevas alarmas / alertas recomendadas.
- Dashboards que deberían existir / actualizarse.
- Runbook breve para incidentes relacionados con este RFC.

---

## 11. Riesgos, supuestos y preguntas abiertas

> **Para qué sirve:** Expone los puntos débiles y cosas que dependen de terceros o decisiones futuras.

### 11.1 Riesgos

- Riesgos técnicos identificados.
- Impacto + probabilidad + mitigaciones.

### 11.2 Supuestos

- Cosas que se dan por hechas (p.ej. “todos los agentes tendrán handshake v2 antes de iX+1”).
- Qué pasa si un supuesto no se cumple.

### 11.3 Preguntas abiertas / NEED-INFO

- Preguntas que deben resolverse antes de aprobar / implementar.
- Dueños de cada respuesta.

---

## 12. Trabajo futuro (iteraciones siguientes)

> **Para qué sirve:** Estacionamiento controlado de ideas que no van en esta iteración, pero no queremos perder.

- Ideas / mejoras que se proponen para iX+1, iX+2...
- Dependencias con otros RFCs futuros.

---

## 13. Referencias

> **Para qué sirve:** Lista de verdad oficial para que los agentes (y humanos) no inventen contexto.

- Docs base:
  - `echo/docs/00-contexto-general.md`
  - `echo/docs/01-arquitectura-y-roadmap.md`
  - `echo/docs/rfcs/RFC-architecture.md`
- RFCs relacionados:
  - ...
- Otros recursos relevantes:
  - Enlaces a dashboards / runbooks / PRs importantes.
