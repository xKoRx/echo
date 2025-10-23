# ADR-001: Monorepo para V1

## Estado
**Aprobado** - 2025-10-23

## Contexto

Echo V1 tiene múltiples componentes relacionados (core, agent, sdk, clients) que necesitan evolucionar juntos en la fase MVP.

Opciones consideradas:
- **Monorepo**: Un solo repositorio con múltiples módulos Go
- **Multi-repo**: Repositorios separados por componente

## Decisión

Usaremos **monorepo** para V1.

Estructura:
```
echo/
├── core/          (go.mod independiente)
├── agent/         (go.mod independiente)
├── sdk/           (go.mod independiente, publicable)
├── test_e2e/      (go.mod independiente)
└── clients/       (MQL4/MQL5, no Go modules)
```

Usaremos **Go workspaces** (`go.work`) para desarrollo local.

## Consecuencias

### Positivas
- ✅ **Velocidad de desarrollo**: Cambios transversales en un solo commit
- ✅ **Simplicidad CI/CD**: Un solo pipeline
- ✅ **Refactoring fácil**: Cambios breaking se ajustan en el mismo PR
- ✅ **Historial unificado**: Toda la evolución del proyecto en un lugar
- ✅ **Tests E2E naturales**: Todos los módulos disponibles

### Negativas
- ⚠️ **Pipeline más largo**: Si crece el proyecto, CI puede ser lento
- ⚠️ **Versionado conjunto**: Cambio en core implica "nueva versión" global
- ⚠️ **Permisos granulares**: No podemos dar acceso solo a SDK

## Plan de Migración a Multi-repo

Dividiremos cuando:
- SDK necesite versionado independiente público
- ≥2 agents externos con ciclos distintos
- >10 contribuidores externos
- CI tarde >10min

Repos resultantes:
- `echo-core`
- `echo-proto` (shared contracts)
- `echo-agent-mt5`
- `echo-sdk-go`
- `echo-installers`

## Referencias
- [Roadmap V1](../roadmap-copiear-v1.md)
- [Go Workspaces](https://go.dev/blog/get-familiar-with-workspaces)

