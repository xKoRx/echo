# Estructura Completa del Proyecto Echo

Este documento contiene la estructura completa del proyecto Echo, incluyendo todos los archivos y directorios.

**Última actualización**: Generado automáticamente

---

## 📁 Estructura de Directorios y Archivos

```
echo/
├── .cursorrules                                    # Reglas de Cursor
├── build_all.sh                                   # Script de construcción
├── echo-agent                                      # Ejecutable del agente
├── go.work                                         # Archivo de workspace de Go
├── go.work.sum                                     # Checksum del workspace
├── Makefile                                        # Makefile principal
├── PIPE_RECONNECT_FIX_i2b.md                      # Documentación de fix de reconexión
├── QUICK_START.md                                  # Guía de inicio rápido
├── README.md                                       # README principal del proyecto
├── SCAFFOLDING_SUMMARY.md                          # Resumen de scaffolding
│
├── agent/                                          # Módulo del agente
│   ├── bin/
│   │   ├── echo-agent                              # Binario del agente (Linux)
│   │   └── echo-agent.exe                          # Binario del agente (Windows)
│   ├── cmd/
│   │   └── echo-agent/
│   │       └── main.go                             # Punto de entrada del agente
│   ├── internal/
│   │   ├── agent.go                                # Lógica principal del agente
│   │   ├── config.go                               # Configuración del agente
│   │   ├── core_client.go                          # Cliente del core
│   │   ├── pipe_manager.go                         # Gestor de pipes
│   │   ├── pipe_manager_stub.go                    # Stub del gestor de pipes
│   │   ├── stream.go                               # Manejo de streams
│   │   ├── telemetry.go                            # Instrumentación de telemetría
│   │   └── utils.go                                # Utilidades
│   ├── main                                        # Ejecutable alternativo
│   ├── go.mod                                      # Módulo Go del agente
│   ├── go.sum                                      # Checksum de dependencias
│   └── README.md                                   # Documentación del agente
│
├── bin/                                            # Binarios compilados
│   ├── echo-agent.exe                              # Ejecutable del agente
│   ├── echo-core                                   # Ejecutable del core
│   ├── echo-core-cli                               # CLI del core
│   ├── echo_pipe_x64.dll                           # DLL de pipe x64
│   ├── echo_pipe_x86.dll                           # DLL de pipe x86
│   ├── JAson.mqh                                   # Librería JSON para MQL4
│   ├── master.mq4                                  # Expert Advisor Master
│   └── slave.mq4                                   # Expert Advisor Slave
│
├── clients/                                        # Clientes para diferentes plataformas
│   ├── mt4/                                        # Cliente MetaTrader 4
│   │   ├── JAson.mqh                               # Librería JSON
│   │   ├── master.mq4                              # EA Master para MT4
│   │   └── slave.mq4                               # EA Slave para MT4
│   ├── mt5/                                        # Cliente MetaTrader 5 (vacío)
│   ├── ninja/                                      # Cliente NinjaTrader (vacío)
│   └── README.md                                   # Documentación de clientes
│
├── core/                                           # Módulo core del sistema
│   ├── bin/
│   │   └── echo-core                               # Binario del core
│   ├── cmd/
│   │   ├── echo-core/
│   │   │   └── main.go                             # Punto de entrada del core
│   │   └── echo-core-cli/
│   │       └── main.go                             # Punto de entrada del CLI
│   ├── internal/
│   │   ├── account_registry.go                      # Registro de cuentas
│   │   ├── account_state_service.go                 # Servicio de estado de cuentas
│   │   ├── config.go                                # Configuración del core
│   │   ├── core.go                                  # Lógica principal del core
│   │   ├── dedupe.go                                # Lógica de deduplicación
│   │   ├── dedupe_service.go                        # Servicio de deduplicación
│   │   ├── handshake_evaluator.go                   # Evaluador de handshake
│   │   ├── handshake_evaluator_test.go              # Tests del evaluador
│   │   ├── handshake_reconciler.go                  # Reconciliador de handshake
│   │   ├── handshake_registry.go                    # Registro de handshakes
│   │   ├── protocol_validator.go                    # Validador de protocolo
│   │   ├── repository/
│   │   │   ├── correlation.go                       # Correlación de datos
│   │   │   ├── handshake_postgres.go                # Repositorio de handshake (Postgres)
│   │   │   └── postgres.go                          # Cliente Postgres
│   │   ├── riskengine/
│   │   │   ├── fixed_risk_engine.go                 # Motor de riesgo fijo
│   │   │   └── fixed_risk_engine_test.go            # Tests del motor de riesgo
│   │   ├── risk_policy_service.go                   # Servicio de políticas de riesgo
│   │   ├── risk_policy_service_test.go              # Tests del servicio de riesgo
│   │   ├── router.go                                # Router de mensajes
│   │   ├── symbol_quote_service.go                  # Servicio de cotizaciones
│   │   ├── symbol_resolver.go                       # Resolvedor de símbolos
│   │   ├── symbol_spec_service.go                   # Servicio de especificaciones
│   │   ├── symbol_validator.go                      # Validador de símbolos
│   │   └── volumeguard/
│   │       ├── guard.go                             # Guardia de volumen
│   │       └── guard_test.go                        # Tests del guardia
│   ├── pkg/                                        # Paquetes públicos (vacío)
│   ├── go.mod                                      # Módulo Go del core
│   └── go.sum                                      # Checksum de dependencias
│
├── deploy/                                         # Scripts de despliegue
│   └── postgres/
│       ├── migrations/                              # Migraciones de base de datos
│       │   ├── i3_symbol_specs_quotes.sql           # Migración i3: specs y quotes
│       │   ├── i3_symbols.sql                       # Migración i3: símbolos
│       │   ├── i4_risk_policy.sql                   # Migración i4: políticas de riesgo
│       │   ├── i4_symbol_spec_guard.sql             # Migración i4: guard de specs
│       │   ├── i5_handshake.sql                     # Migración i5: handshake
│       │   └── i6_risk_policy_fixed_risk.sql        # Migración i6: riesgo fijo
│       ├── README.md                                # Documentación de Postgres
│       ├── setup.sql                                # Script de configuración
│       └── teardown.sql                             # Script de limpieza
│
├── docs/                                           # Documentación del proyecto
│   ├── 00-contexto-general.md                       # Contexto general
│   ├── 01-arquitectura-y-roadmap.md                 # Arquitectura y roadmap
│   ├── 02-correcciones-arquitecturales.md          # Correcciones arquitecturales
│   ├── 03-respuesta-a-correcciones.md               # Respuesta a correcciones
│   ├── adr/                                        # Architecture Decision Records
│   │   ├── 001-monorepo.md                          # ADR: Monorepo
│   │   ├── 002-grpc-transport.md                    # ADR: Transporte gRPC
│   │   ├── 003-named-pipes-ipc.md                   # ADR: Named Pipes IPC
│   │   ├── 004-postgres-state.md                    # ADR: Estado en Postgres
│   │   ├── 005-etcd-config.md                        # ADR: Configuración ETCD
│   │   └── README.md                                # Índice de ADRs
│   ├── architecture/                                # Documentación de arquitectura (vacío)
│   ├── diagrams/                                    # Diagramas (vacío)
│   ├── ea/                                         # Documentación de Expert Advisors
│   │   ├── MASTER_EA_i1_GUIDE.md                    # Guía del EA Master i1
│   │   └── SLAVE_EA_i1_GUIDE.md                     # Guía del EA Slave i1
│   ├── echo-agent-ea-integration-guide.md           # Guía de integración agente-EA
│   ├── PRD-copiador-V1.md                           # Product Requirements Document
│   ├── reports/                                     # Reportes de implementación
│   │   ├── i3-implementation-gap.md                 # Gap de implementación i3
│   │   └── i5-handshake-regresion.md                # Regresión de handshake i5
│   ├── rfcs/                                       # Request for Comments
│   │   ├── RFC-000-iteration-0-implementation.md    # RFC: Iteración 0
│   │   ├── RFC-001-iteration-1-implementation.md    # RFC: Iteración 1
│   │   ├── RFC-002-routing-selectivo.md              # RFC: Routing selectivo
│   │   ├── RFC-003a-iteracion-3-catalogo-simbolos.md # RFC: Catálogo de símbolos (parte A)
│   │   ├── RFC-003b-iteracion-3-parte-final-slave-registro.md # RFC: Registro slave (parte B)
│   │   ├── RFC-004-iteracion-4-especificaciones-broker.md # RFC: Especificaciones de broker
│   │   ├── RFC-005-iteracion-5-handshake-versionado.md # RFC: Handshake versionado
│   │   ├── RFC-006-iteracion-6-fixed-risk.md        # RFC: Riesgo fijo
│   │   └── RFC-architecture.md                     # RFC: Arquitectura
│   ├── roadmap-copiear-v1.md                        # Roadmap del copiador V1
│   ├── runbooks/                                   # Runbooks operativos
│   │   ├── deploy_v3.md                             # Runbook: Despliegue v3
│   │   ├── escalation_prompt.md                     # Runbook: Escalación
│   │   └── specs.md                                 # Runbook: Especificaciones
│   └── trade-copier-context.md                      # Contexto del copiador de trades
│
├── pipe/                                           # Componente de Named Pipes
│   ├── bin/
│   │   ├── echo_pipe_x64.dll                        # DLL x64 compilada
│   │   ├── echo_pipe_x86.dll                        # DLL x86 compilada
│   │   ├── test_pipe_x64.exe                        # Ejecutable de test x64
│   │   └── test_pipe_x86.exe                        # Ejecutable de test x86
│   ├── BUILD_REPORT.md                              # Reporte de construcción
│   ├── build.sh                                     # Script de construcción
│   ├── CHANGELOG.md                                 # Changelog del componente
│   ├── CMakeLists.txt                               # Archivo CMake
│   ├── COMPONENT_SUMMARY.md                         # Resumen del componente
│   ├── echo_pipe.cpp                                # Código fuente C++ del pipe
│   ├── echo_pipe_x64.def                            # Definición de exportaciones x64
│   ├── echo_pipe_x86.def                            # Definición de exportaciones x86
│   ├── INSTALL.md                                   # Guía de instalación
│   ├── Makefile                                     # Makefile del componente
│   ├── MQL4_USAGE_EXAMPLE.mq4                       # Ejemplo de uso en MQL4
│   ├── QUICK_REFERENCE.md                           # Referencia rápida
│   ├── README.md                                    # Documentación del componente
│   ├── test_pipe.cpp                                # Código de test del pipe
│   ├── toolchain-mingw-x64.cmake                    # Toolchain CMake x64
│   └── toolchain-mingw-x86.cmake                    # Toolchain CMake x86
│
├── scripts/                                        # Scripts auxiliares
│   ├── cleanup_i1_critical_fixes.sql                # Script de limpieza i1
│   └── normalize_uuids.sql                          # Script de normalización de UUIDs
│
├── sdk/                                            # SDK compartido
│   ├── contracts/                                   # Contratos (vacío)
│   ├── domain/                                      # Dominio del negocio
│   │   ├── account_validation.go                    # Validación de cuentas
│   │   ├── doc.go                                   # Documentación del paquete
│   │   ├── errors.go                                # Errores del dominio
│   │   ├── handshake/                               # Lógica de handshake
│   │   │   ├── doc.go                               # Documentación del paquete
│   │   │   ├── evaluation_compare.go                # Comparación de evaluaciones
│   │   │   ├── handshake.go                         # Lógica de handshake
│   │   │   └── handshake_test.go                    # Tests de handshake
│   │   ├── models.go                                # Modelos del dominio
│   │   ├── repository.go                            # Interfaces de repositorio
│   │   ├── risk_calculator.go                       # Calculadora de riesgo
│   │   ├── risk_calculator_test.go                  # Tests de calculadora
│   │   ├── risk_policy.go                           # Políticas de riesgo
│   │   ├── risk_policy_validator.go                 # Validador de políticas
│   │   ├── risk_policy_validator_test.go            # Tests del validador
│   │   ├── trade.go                                 # Modelo de trade
│   │   ├── transformers.go                          # Transformadores de datos
│   │   ├── validation.go                            # Validaciones generales
│   │   ├── volume.go                                # Lógica de volumen
│   │   ├── volume_guard_policy.go                   # Política de guardia de volumen
│   │   └── volume_test.go                           # Tests de volumen
│   ├── etcd/                                       # Cliente ETCD
│   │   ├── cache.go                                 # Caché de configuración
│   │   ├── cache_internal_test.go                   # Tests internos de caché
│   │   ├── cache_test.go                            # Tests de caché
│   │   ├── client.go                                # Cliente ETCD
│   │   ├── client_helpers_test.go                   # Tests de helpers
│   │   ├── client_test.go                           # Tests del cliente
│   │   ├── doc.go                                   # Documentación del paquete
│   │   ├── echo_seed_test.go                        # Tests de seed
│   │   ├── etcd_middleware.go                       # Middleware ETCD
│   │   ├── etcd_middleware_test.go                  # Tests del middleware
│   │   ├── example_test.go                          # Ejemplos de uso
│   │   └── README.md                                # Documentación de ETCD
│   ├── grpc/                                       # Utilidades gRPC
│   │   ├── client.go                                # Cliente gRPC
│   │   ├── doc.go                                   # Documentación del paquete
│   │   ├── interceptors.go                          # Interceptores gRPC
│   │   ├── server.go                                # Servidor gRPC
│   │   └── stream.go                                # Manejo de streams
│   ├── ipc/                                        # Comunicación entre procesos
│   │   ├── doc.go                                   # Documentación del paquete
│   │   ├── pipe.go                                  # Implementación de pipes
│   │   ├── reader.go                                # Lector de pipes
│   │   ├── windows_pipe.go                          # Pipes específicos de Windows
│   │   └── writer.go                                # Escritor de pipes
│   ├── pb/                                         # Código generado de Protocol Buffers
│   │   └── v1/
│   │       ├── agent_grpc.pb.go                     # Código gRPC generado
│   │       ├── agent.pb.go                          # Código de mensajes del agente
│   │       ├── common.pb.go                         # Mensajes comunes
│   │       └── trade.pb.go                          # Mensajes de trades
│   ├── proto/                                      # Definiciones Protocol Buffers
│   │   ├── buf.gen.yaml                             # Configuración de generación
│   │   ├── buf.yaml                                 # Configuración de buf
│   │   ├── generate.sh                              # Script de generación
│   │   └── v1/
│   │       ├── agent.proto                          # Definición del agente
│   │       ├── common.proto                         # Definiciones comunes
│   │       └── trade.proto                          # Definición de trades
│   ├── telemetry/                                  # Telemetría y observabilidad
│   │   ├── client.go                                # Cliente de telemetría
│   │   ├── config.go                                # Configuración de telemetría
│   │   ├── context.go                               # Manejo de contexto
│   │   ├── doc.go                                   # Documentación del paquete
│   │   ├── example_test.go                          # Ejemplos de uso
│   │   ├── logs.go                                  # Logging estructurado
│   │   ├── metricbundle/                           # Bundles de métricas
│   │   │   ├── base.go                              # Base de bundles
│   │   │   ├── candle.go                            # Métricas de velas
│   │   │   ├── doc.go                               # Documentación
│   │   │   ├── document.go                          # Métricas de documentos
│   │   │   ├── echo.go                              # Métricas de Echo
│   │   │   ├── example_test.go                      # Ejemplos
│   │   │   ├── http.go                              # Métricas HTTP
│   │   │   ├── migration.go                         # Métricas de migración
│   │   │   ├── minio.go                             # Métricas de MinIO
│   │   │   ├── postgres.go                          # Métricas de Postgres
│   │   │   ├── signal.go                            # Métricas de señales
│   │   │   ├── sqx.go                               # Métricas de SQX
│   │   │   ├── temporal.go                          # Métricas de Temporal
│   │   │   ├── tick.go                              # Métricas de ticks
│   │   │   └── trade.go                             # Métricas de trades
│   │   ├── metrics.go                               # Métricas generales
│   │   ├── README.md                                # Documentación de telemetría
│   │   ├── semconv/                                # Convenciones semánticas
│   │   │   ├── doc.go                               # Documentación
│   │   │   ├── document.go                          # Convenciones de documentos
│   │   │   ├── echo.go                              # Convenciones de Echo
│   │   │   ├── example_test.go                      # Ejemplos
│   │   │   ├── http.go                              # Convenciones HTTP
│   │   │   ├── logs.go                              # Convenciones de logs
│   │   │   ├── metrics.go                           # Convenciones de métricas
│   │   │   └── sqx.go                               # Convenciones SQX
│   │   └── traces.go                                # Trazas distribuidas
│   ├── utils/                                      # Utilidades generales
│   │   ├── doc.go                                   # Documentación del paquete
│   │   ├── json.go                                  # Utilidades JSON
│   │   ├── timestamp.go                             # Utilidades de timestamps
│   │   └── uuid.go                                  # Utilidades de UUID
│   ├── go.mod                                      # Módulo Go del SDK
│   ├── go.sum                                      # Checksum de dependencias
│   └── README.md                                   # Documentación del SDK
│
├── test_e2e/                                       # Tests end-to-end
│   ├── fixtures/                                    # Fixtures de test (vacío)
│   ├── mocks/                                       # Mocks para tests (vacío)
│   ├── scenarios/                                   # Escenarios de test (vacío)
│   ├── go.mod                                      # Módulo Go de tests
│   └── README.md                                   # Documentación de tests
│
├── tools/                                          # Herramientas auxiliares (vacío)
│
└── vibe-coding/                                    # Sistema de prompts y reglas
    ├── contracts/                                  # Contratos de prompts
    │   └── errors.md                                # Contrato de errores
    ├── docs/                                       # Documentación de vibe-coding
    │   ├── CHANGELOG.md                             # Changelog
    │   ├── NEED-INFO.md                             # Plantilla de información necesaria
    │   ├── qa/
    │   │   └── templates/
    │   │       └── README.md                        # Templates de QA
    │   └── release-notes/
    │       └── TEMPLATE.md                          # Plantilla de release notes
    ├── echo_prompts_pack_v1.md                      # Pack de prompts v1
    ├── observability/                              # Observabilidad
    │   └── metrics.md                               # Documentación de métricas
    ├── prompts/                                    # Prompts del sistema
    │   ├── agent-templates/                        # Templates de agentes
    │   │   ├── arch_author.md                       # Template: Arquitecto Autor
    │   │   ├── arch_reviewer.md                     # Template: Arquitecto Revisor
    │   │   ├── dev_author.md                        # Template: Dev Autor
    │   │   ├── dev_reviewer.md                      # Template: Dev Revisor
    │   │   ├── gatekeeper.md                        # Template: Gatekeeper
    │   │   ├── qa_author.md                         # Template: QA Autor
    │   │   └── qa_reviewer.md                       # Template: QA Revisor
    │   ├── common-principles.md                     # Principios comunes
    │   └── prompt-manifest.json                     # Manifesto de prompts
    ├── README.md                                   # Documentación de vibe-coding
    └── sre/                                       # Site Reliability Engineering
        └── slo.yml                                 # Definición de SLOs
```

---

## 📊 Estadísticas del Proyecto

### Por Tipo de Archivo

- **Go (.go)**: ~150 archivos
- **Markdown (.md)**: ~50 archivos
- **SQL (.sql)**: ~10 archivos
- **Protocol Buffers (.proto)**: 3 archivos
- **C++ (.cpp)**: 2 archivos
- **MQL4 (.mq4)**: 5 archivos
- **MQL Header (.mqh)**: 2 archivos
- **CMake (.cmake)**: 2 archivos
- **Shell Scripts (.sh)**: 3 archivos
- **Makefiles**: 2 archivos
- **Configuración (YAML, JSON)**: 5 archivos
- **Binarios**: ~10 archivos

### Por Módulo

1. **agent/**: Módulo del agente (cliente)
2. **core/**: Módulo core del sistema (servidor)
3. **sdk/**: SDK compartido con utilidades comunes
4. **pipe/**: Componente de Named Pipes (C++)
5. **test_e2e/**: Tests end-to-end
6. **vibe-coding/**: Sistema de prompts y reglas para desarrollo
7. **docs/**: Documentación completa del proyecto
8. **deploy/**: Scripts de despliegue y migraciones
9. **clients/**: Clientes para diferentes plataformas de trading

---

## 🔍 Descripción de Componentes Principales

### Agent (`agent/`)
Cliente que se ejecuta en la máquina del trader y se comunica con el core mediante gRPC y Named Pipes.

### Core (`core/`)
Servidor central que gestiona el routing de trades, validaciones, políticas de riesgo y estado del sistema.

### SDK (`sdk/`)
Biblioteca compartida que contiene:
- **domain/**: Modelos y lógica de negocio
- **etcd/**: Cliente de configuración
- **grpc/**: Utilidades gRPC
- **ipc/**: Comunicación entre procesos (Named Pipes)
- **telemetry/**: Observabilidad (logs, métricas, trazas)
- **utils/**: Utilidades generales

### Pipe (`pipe/`)
Componente C++ que implementa Named Pipes para comunicación entre el agente y los Expert Advisors de MetaTrader.

### Vibe-Coding (`vibe-coding/`)
Sistema de prompts y reglas para orquestar el desarrollo multi-agente con Cursor, incluyendo templates para arquitectos, desarrolladores y QA.

### Docs (`docs/`)
Documentación completa incluyendo:
- **ADRs**: Decisiones arquitecturales
- **RFCs**: Especificaciones de iteraciones
- **Runbooks**: Guías operativas
- **Guías**: Documentación de uso

---

## 📝 Notas

- Los directorios marcados como "(vacío)" contienen la estructura pero no tienen archivos actualmente.
- Los binarios compilados están en `bin/` y en los respectivos `bin/` de cada módulo.
- Las migraciones de base de datos están organizadas por iteración (i3, i4, i5, i6).
- El proyecto usa Go workspaces (`go.work`) para manejar múltiples módulos.

---

**Generado automáticamente** - Para actualizar este documento, ejecuta el script de generación correspondiente.

