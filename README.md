# Echo - Trading Copier

![Version](https://img.shields.io/badge/version-0.0.1-blue)
![Go](https://img.shields.io/badge/go-1.25-00ADD8)
![License](https://img.shields.io/badge/license-Proprietary-red)

**Echo** es un sistema de copiado de operaciones de trading algorítmico de baja latencia, diseñado para replicar operaciones desde cuentas **master** (MT4/MT5) hacia múltiples cuentas **slave** con tolerancias configurables, Money Management centralizado y observabilidad completa.

## 🎯 Objetivo

Sincronizar operaciones de trading entre cuentas manteniendo:
- **Latencia < 1s** extremo a extremo (objetivo < 100ms intra-host)
- **MagicNumber idéntico** para trazabilidad
- **Tolerancias configurables** (spread, slippage, ventanas de no-ejecución)
- **Observabilidad total** (métricas, logs, trazas)

## 📚 Documentación

- [PRD V1](docs/PRD-copiador-V1.md) - Product Requirements Document
- [Contexto Técnico](docs/trade-copier-context.md) - Problemas y estrategias
- [Roadmap](docs/roadmap-copiear-v1.md) - Plan de iteraciones
- [RFC-001: Arquitectura](docs/RFC-001-architecture.md) - Decisiones arquitectónicas

## 🏗️ Arquitectura

```
Master EA (MT4/MT5)
    ↓ Named Pipes
Agent (Go, Windows Service)
    ↓ gRPC Bidi
Core (Go, Orquestador)
    ↓ gRPC Bidi
Agent (Go, Windows Service)
    ↓ Named Pipes
Slave EA (MT4/MT5)
```

### Componentes

- **core/**: Servicio orquestador central (gRPC, motor de copia, MM, políticas)
- **agent/**: Bridge entre IPC (Named Pipes) y gRPC
- **sdk/**: Librería transversal (proto, telemetry, IPC, domain)
- **clients/**: Conectores (MT4, MT5, Ninja, etc.)
- **test_e2e/**: Tests end-to-end cross-component

## 🚀 Quick Start

### Prerrequisitos

- Go 1.25+
- PostgreSQL 16+
- etcd v3
- Buf CLI (para proto generation)

### Instalación

```bash
# Clonar
git clone https://github.com/xKoRx/echo.git
cd echo

# Instalar herramientas
make install-tools

# Generar proto
make proto

# Compilar
make build

# Ejecutar core
./bin/echo-core

# Ejecutar agent (en otra terminal)
./bin/echo-agent
```

## 📦 Estructura del Proyecto

```
echo/
├── core/          # Servicio orquestador
├── agent/         # Bridge gRPC ↔ IPC
├── sdk/           # Librería transversal
│   ├── proto/     # Contratos gRPC
│   ├── pb/        # Código generado
│   ├── telemetry/ # Observabilidad
│   └── ipc/       # Named Pipes helpers
├── clients/       # Conectores (MT4, MT5, etc.)
├── test_e2e/      # Tests E2E
├── docs/          # Documentación y RFCs
└── tools/         # Herramientas de desarrollo
```

## 🧪 Testing

```bash
# Tests unitarios
make test

# Tests E2E
make test-e2e

# Linters
make lint

# Coverage
go test -coverprofile=coverage.out ./...
```

## 🛠️ Desarrollo

El proyecto usa **Go workspaces** para facilitar el desarrollo local:

```bash
# go.work permite trabajar con múltiples módulos
cd core && go test ./...
cd agent && go test ./...
cd sdk && go test ./...
```

### Workflow

1. Crear feature branch: `git checkout -b feature/nombre`
2. Hacer cambios y tests
3. Ejecutar linters: `make lint`
4. Commit: `git commit -m "feat: descripción"`
5. Push y PR

## 📋 Roadmap

- [x] **Iteración 0**: POC 48h (1 master → 1 slave, market only)
- [ ] **Iteración 1**: Persistencia robusta
- [ ] **Iteración 2**: SL catastrófico + filtros
- [ ] **Iteración 3**: Mapeo símbolos
- [ ] **Iteración 4**: Sizing configurable
- [ ] **Iteración 5**: Multi-slave
- [ ] **Iteración 6**: SL/TP con offset
- [ ] **Iteración 7**: Empaquetado V1

Ver [roadmap completo](docs/roadmap-copiear-v1.md)

## 🤝 Contribuir

Este es un proyecto privado. Contactar al equipo para acceso.

## 📄 Licencia

Proprietary - Aranea Labs © 2025

## 📞 Contacto

- Equipo: Aranea Labs - Trading Copier Team
- Documentación: `/docs`
