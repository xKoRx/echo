# Echo — Prompt del Agente: Arquitecto Autor (v3-lean-human, solo Tarea)

```md
<CONTEXT>
<!--
Pega aquí SOLO las refs adjuntas necesarias para esta iteración.
Ejemplo típico:
  @echo/docs/00-contexto-general.md
  @echo/docs/01-arquitectura-y-roadmap.md
  @echo/docs/rfcs/RFC-architecture.md
  @echo/vibe-coding/prompts/common-principles.md
  @echo/docs/rfcs/RFC-00X-relacionado.md   # opcional
-->
</CONTEXT>

<REQUEST>
@echo/vibe-coding/rules/arquitecto-autor.mdc

[VARS_DE_TAREA]
ITERATION_SLUG={{iN}}
RFC_NAME={{slug-kebab-descriptivo}}
ARCH_LOCK={{true|false}}
TIME_BUDGET_S={{120}}
CONSENT_TOKEN={{GO}}
MODE={{auto|design-first}}   # default: design-first

[TAREA]
Ejecutar el flujo estándar definido en la rule del Arquitecto Autor:
- PRE-FLIGHT sobre todas las refs adjuntas.
- FASE 0 — DESIGN CHECKPOINT (solo si MODE=design-first).
- FASE 1 — RFC EN DISCO (solo tras handshake válido con CONSENT_TOKEN).

[OUTPUT_ESPERADO]
- Si MODE=design-first y aún NO hay handshake válido:
  Tu salida debe ser **exactamente un** bloque ```text con DESIGN_BRIEF_HUMANO
  siguiendo la plantilla definida en tu rule (≤60 líneas).
- Si ya recibiste un handshake válido con:
  "{{CONSENT_TOKEN}} {{ITERATION_SLUG}} {{RFC_NAME}}" + respuestas QPACK:
  1) Escribe el RFC completo en {{OUTPUT_PATH}} usando la plantilla estándar.
  2) Calcula sha256 y line_count del archivo.
  3) Devuelve **exactamente un** bloque ```text con PROMPT_NEXT_AGENT,
     incluyendo:
       - path: {{OUTPUT_PATH}}
       - sha256: {{FILE_SHA256}}
       - line_count: {{FILE_LINE_COUNT}}
- Si falla cualquier precondición (refs faltantes, citas inválidas, permisos de escritura):
  Devuelve **exactamente un** bloque ```text con NEED-INFO,
  usando la plantilla de error definida en tu rule.

[NOTAS_TÁCTICAS]
- No imprimas el contenido del RFC en el chat en ningún caso.
- No generes código de implementación; solo contratos, interfaces y observabilidad.
- Toda afirmación de contexto debe ir anclada a [echo/...#seccion], según tu rule.
</REQUEST>
```
