**Objetivo**: orquestar prompts multi-agente y reglas de uso para Echo Copier con foco en robustez, modularidad y trazabilidad.

**Cómo usar**:
1. Copia cada sección a su ruta indicada.
2. En Cursor, activa `/.cursor/rules/echo-prompts.mdc` y mantén los tres docs base adjuntos por defecto.
3. Usa los templates de `/prompts/agent-templates/` en el flujo corto: Arquitecto Autor → Arquitecto Revisor → Dev Autor → Dev Validador → QA Autor → QA Validador → Gatekeeper.
4. Si falta contexto, los agentes deben emitir un bloque **NEED-INFO** y detenerse.

**Notas clave**:
- **Anti-invención**: usa `strict_refs` y `no_fabrication` en los prompts y en las RULES de Cursor.
- **Razonamiento**: no pidas "cadena de pensamiento" en la salida. Exige "Rationale" breve y **matriz de conformidad**. Reduce tokens y evita que el modelo rellene con especulación. 
- **Orden de verdad**: Docs base → RFC aprobado → CI/artefactos. Ante conflicto, solicitar aclaración vía NEED-INFO.
- **KPIs y SLOs**: define KPIs de proceso (métricas operativas) y SLOs (compromisos) en `/observability/` y `/sre/slo.yml`.

