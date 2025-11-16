**PR-ROB** Robustez: tolerancia a fallos, timeouts, reintentos, backoff; sin afectar integridad de datos.  
**PR-MOD** Modularidad: componentes con responsabilidades claras y bajo acoplamiento.  
**PR-ESC** Escalabilidad: linealidad o sublinealidad con carga; horizontabilidad explícita.  
**PR-CLN** Clean code: legibilidad, naming consistente, deuda técnica explícita.  
**PR-SOLID** SOLID: interfaces mínimas, inversión de dependencias donde agregue valor.  
**PR-KISS** KISS: evitar complejidad innecesaria; elegir la opción más simple que cumple.  
**PR-OBS** Observabilidad: logs estructurados, métricas, spans con semántica consistente.  
**PR-BWC** Compatibilidad hacia atrás: cambios no rompen contratos públicos sin plan.  
**PR-IDEMP** Idempotencia: reintentos seguros en operaciones con side-effects.  
**PR-RES** Resiliencia: manejo de fallos parciales y degradación controlada.  
**PR-SEC** Seguridad: mínimo privilegio, manejo de secretos, validación de entrada.  
**PR-PERF** Performance: latencias objetivo, GC/allocs, uso de CPU/IO, perfiles.
**PR-MVP** MODO MVP: ESTO QUIERE DECIR QUE POR SOBRE TODAS LAS COSAS SE DEBEN OBVIAR TEMAS DE SEGURIDAD, SLO, ROLLOUT CONTROLADOS, FEATURE FLAGS, ETC. PRIMA POR SOBRE TODAS LAS COSAS LA VELOCIDAD.

**Severidad para hallazgos**:  
- **BLOQ**: bloquea avance o viola PR-* crítico.  
- **MAY**: impacto relevante pero con workaround.  
- **MEN**: mejora menor o estilo; no bloquea.  
- **INFO**: observación o nota futura.

