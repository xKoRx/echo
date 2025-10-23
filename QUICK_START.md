# 🚀 Quick Start - Echo

Guía rápida para comenzar con el proyecto echo recién scaffoldeado.

---

## ✅ Pre-requisitos

- Go 1.23+
- Git
- PostgreSQL 16+ (para iteración 1+)
- etcd v3 (para iteración 2+)
- Buf CLI (opcional, para proto generation)

---

## 📦 Instalación Inicial

### 1. Clonar (si aún no lo hiciste)

```bash
cd /home/kor/go/src/github.com/xKoRx/echo
```

### 2. Verificar Go Version

```bash
go version
# Debe ser >= 1.23
```

### 3. Inicializar Workspace

```bash
# El go.work ya está creado, solo verifica
cat go.work

# Output esperado:
# go 1.23
# use (
#   ./core
#   ./agent
#   ./sdk
#   ./test_e2e
# )
```

---

## 🔧 Setup de Herramientas

### Instalar Tools

```bash
make install-tools
```

Esto instala:
- `buf` (proto generation)
- `protoc-gen-go`
- `protoc-gen-go-grpc`
- `golangci-lint`
- `mockery`

### Generar Proto

```bash
make proto
```

Esto genera código Go desde los `.proto` en `sdk/pb/v1/`.

---

## 🏗️ Build

### Compilar Core

```bash
make build-core
# Output: bin/echo-core
```

### Compilar Agent

```bash
make build-agent
# Output: bin/echo-agent
```

### Compilar Todo

```bash
make build
```

---

## 🧪 Tests

### Ejecutar Tests Unitarios

```bash
make test
```

Esto ejecuta tests en:
- `core/`
- `agent/`
- `sdk/`

### Ejecutar Tests E2E

```bash
make test-e2e
```

**Nota**: Por ahora no hay tests implementados (iteración 0).

---

## 🔍 Linting

```bash
make lint
```

---

## 📝 Commit Inicial

### 1. Revisar Archivos

```bash
git status
```

Deberías ver todos los archivos creados en el scaffolding.

### 2. Commit

```bash
git add .

git commit -m "feat: scaffolding inicial completo para echo

- Estructura monorepo con Go workspaces (1.23)
- Módulos independientes: core, agent, sdk, test_e2e
- Contratos proto V1 (common, trade, agent)
- Telemetry limpia desde 0 (sin deuda técnica)
- RFC-001 profesional con arquitectura completa
- ADRs 001-005 (monorepo, gRPC, Named Pipes, Postgres, etcd)
- READMEs en cada módulo
- .cursorrules con reglas específicas del proyecto
- Makefile con tareas comunes

Iteración 0 lista para iniciar desarrollo."
```

### 3. Push a Remote

```bash
# Si aún no tienes remote configurado:
git remote add origin https://github.com/xKoRx/echo.git
# O SSH:
# git remote add origin git@github.com:xKoRx/echo.git

# Push
git branch -M main
git push -u origin main
```

---

## 📚 Documentación Clave

Lee estos documentos para entender el proyecto:

1. **[README.md](README.md)**: Visión general
2. **[RFC-001](docs/RFC-001-architecture.md)**: Arquitectura completa
3. **[ADRs](docs/adr/)**: Decisiones técnicas
4. **[PRD](docs/PRD-copiador-V1.md)**: Requerimientos de producto
5. **[SCAFFOLDING_SUMMARY.md](SCAFFOLDING_SUMMARY.md)**: Resumen ejecutivo

---

## 🎯 Próximos Pasos (Iteración 0 - POC 48h)

### Objetivo

POC funcional: 1 master MT4 → 1 slave MT4, market orders only, símbolo único.

### Tareas

1. **Core**:
   - [ ] Implementar router básico en `core/internal/engine/`
   - [ ] Servidor gRPC bidi en `core/internal/grpc/`
   - [ ] Deduplicación in-memory en `core/internal/state/`
   - [ ] Logger básico con telemetry

2. **Agent**:
   - [ ] Cliente gRPC al core en `agent/internal/grpc/`
   - [ ] Servidor Named Pipes en `agent/internal/ipc/`
   - [ ] Bridge simple en `agent/internal/bridge/`

3. **SDK**:
   - [ ] Helpers Named Pipes en `sdk/ipc/`
   - [ ] Domain entities en `sdk/domain/`

4. **Clients**:
   - [ ] Master EA básico en `clients/mt4/MasterEA.mq4`
   - [ ] Slave EA básico en `clients/mt4/SlaveEA.mq4`
   - [ ] Librería IPC en `clients/mt4/Include/EchoIPC.mqh`

5. **Tests**:
   - [ ] Test E2E del flujo completo en `test_e2e/scenarios/`

### Criterios de Éxito

- ✅ p95 latencia < 120ms intra-host
- ✅ 0 duplicados
- ✅ 10 ejecuciones consecutivas correctas

---

## 🐛 Troubleshooting

### Error: go.work.sum not found

```bash
# Sincronizar workspace
go work sync
```

### Error: buf command not found

```bash
make install-tools
```

### Error: protoc not found

```bash
# Linux
sudo apt install protobuf-compiler

# macOS
brew install protobuf

# Windows
choco install protoc
```

### Error: cannot find module

```bash
# Limpiar y descargar dependencias
make tidy
```

---

## 💡 Tips

1. **Usa el Makefile**: Todas las tareas comunes están ahí
2. **Lee .cursorrules**: Contiene reglas específicas del proyecto
3. **Sigue RFC-001**: Es la fuente de verdad arquitectónica
4. **Telemetry obligatoria**: Logs, métricas, trazas en TODO el código
5. **Tests desde día 1**: No dejes tests para después

---

## 📞 Ayuda

- **Docs**: `/docs`
- **RFCs**: `/docs/RFC-*.md`
- **ADRs**: `/docs/adr/*.md`
- **Issues**: GitHub Issues (cuando esté configurado)

---

**¡Éxito! 🎯**

