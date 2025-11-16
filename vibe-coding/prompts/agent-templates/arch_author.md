# Echo — Prompt de Tarea: Arquitecto Autor (invocación estándar design-first v4)

```xml
<ROL>
   debes tomar exclusivamente la personalidad descrita en echo/vibe-coding/.cursor/rules/arquitecto-autor.mdc
<ROL>

<CONTEXT>
    echo/docs/00-contexto-general.md
    echo/docs/01-arquitectura-y-roadmap.md
    echo/docs/rfcs/RFC-architecture.md
    echo/vibe-coding/prompts/common-principles.md
    echo/docs/templates/rfc.md

  Puedes añadir RFCs relacionados u otros docs específicos:
    @echo/docs/rfcs/RFC-00X-relacionado.md
</CONTEXT>

<REQUEST>
  # Vars de tarea (rellenar antes de invocar)
  ITERATION_SLUG = {{iN}}
  RFC_NAME       = {{slug-kebab-descriptivo}}
  ARCH_LOCK      = {{true|false}}
  TIME_BUDGET_S  = {{120}}
  CONSENT_TOKEN  = {{GO-{{iN}}}}
  MODE           = design-first

  # Acción para este agente (sin redefinir su comportamiento interno)
  - Ejecuta PRE-FLIGHT con las vars y refs anteriores.
  - Si algo falla (refs, plantilla, permisos, nombre inválido, contexto insuficiente),
    emite exclusivamente un bloque NEED-INFO (formato definido en tu personalidad).
  - Si todo está OK y MODE=design-first, ejecuta Fase 0 y emite exactamente
    un bloque DESIGN_BRIEF_HUMANO (formato definido en tu personalidad).
  - Detén la ejecución y espera el handshake explícito del owner con formato:
        "{{CONSENT_TOKEN}} {{ITERATION_SLUG}} {{RFC_NAME}}"
    más las respuestas al QPACK incluido en el DESIGN_BRIEF_HUMANO.
  - Solo tras un handshake válido, ejecuta Fase 1, escribe el RFC en OUTPUT_PATH
    y emite exclusivamente un bloque PROMPT_NEXT_AGENT (formato definido en tu personalidad).

  # Notas de uso para el operador humano
  - Este agente NO debe imprimir el contenido del RFC en el chat, solo metadatos.
  - El bloque PROMPT_NEXT_AGENT se copiará a un chat limpio del Arquitecto Revisor
    como parte del handoff secuencial del framework multiagente.
</REQUEST>

<EXTRA-CONTEXT>
</EXTRA-CONTEXT>
```
