# Architecture Decision Records (ADRs)

Este directorio contiene todas las **decisiones arquitectónicas** tomadas en el proyecto Echo.

## Formato

Cada ADR sigue la estructura:

- **Título**: Número + descripción breve
- **Estado**: Draft / Aprobado / Superseded / Deprecated
- **Contexto**: Problema y opciones consideradas
- **Decisión**: Qué se decidió y por qué
- **Consecuencias**: Positivas y negativas
- **Referencias**: Links a docs relacionados

## Lista de ADRs

| ID | Título | Estado | Fecha |
|----|--------|--------|-------|
| [001](001-monorepo.md) | Monorepo para V1 | Aprobado | 2025-10-23 |
| [002](002-grpc-transport.md) | gRPC Bidi-Streaming Core ↔ Agent | Aprobado | 2025-10-23 |
| [003](003-named-pipes-ipc.md) | Named Pipes para IPC Agent ↔ EAs | Aprobado | 2025-10-23 |
| [004](004-postgres-state.md) | PostgreSQL para Estado | Aprobado | 2025-10-23 |
| [005](005-etcd-config.md) | etcd para Configuración Live | Aprobado | 2025-10-23 |

## Proceso

### Crear nuevo ADR

1. Copiar template (si existe) o usar estructura estándar
2. Numerar secuencialmente (006, 007, etc.)
3. Escribir en estado **Draft**
4. Discutir con el equipo
5. Aprobar y cambiar estado a **Aprobado**
6. Referenciar en código si aplica

### Modificar ADR Existente

- **No modificar ADRs aprobados**
- Si necesitas cambiar una decisión, crea un nuevo ADR que **superseda** al anterior

### Ejemplo de Supersesión

```markdown
# ADR-006: MongoDB para Eventos (Supersede ADR-004)

## Estado
**Aprobado** - 2025-11-01

Supersede: [ADR-004](004-postgres-state.md) parcialmente (eventos)

## Contexto
Postgres no escala bien para eventos append-only de alta frecuencia...
```

## Referencias

- [ADR Template (Michael Nygard)](https://github.com/joelparkerhenderson/architecture-decision-record)
- [RFC-001: Arquitectura](../RFC-001-architecture.md)

