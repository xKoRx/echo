# echo/vibe-coding/prompts/dev-senior-go.request.md

<CONTEXT>
  @echo/vibe-coding/personas/dev-senior-go.mdc

  # Docs base de Echo (mínimo estos tres)
  @echo/docs/00-contexto-general.md
  @echo/docs/01-arquitectura-y-roadmap.md
  @echo/docs/rfcs/RFC-architecture.md

  # RFC de la iteración (fuente de verdad funcional)
  @{{RFC_PATH}}
</CONTEXT>

<REQUEST>
  # Vars de tarea (instanciar antes de invocar)
  ITERATION_SLUG      = {{ITERATION_SLUG}}      # p.ej. i8(a+b)
  RFC_NAME            = {{RFC_NAME}}            # slug kebab del RFC
  RFC_PATH            = echo/docs/rfcs/{{ITERATION_SLUG}}/RFC-{{ITERATION_SLUG}}-{{RFC_NAME}}.md

  IMPLEMENTATION_PATH = echo/docs/rfcs/{{ITERATION_SLUG}}/IMPLEMENTATION.md
  DIFF_PATH           = echo/docs/rfcs/{{ITERATION_SLUG}}/DIFF-{{ITERATION_SLUG}}-{{RFC_NAME}}.txt
  CI_REPORT_PATH      = echo/docs/rfcs/{{ITERATION_SLUG}}/CI-{{ITERATION_SLUG}}-{{RFC_NAME}}.md
  CR_PATH             = echo/docs/rfcs/{{ITERATION_SLUG}}/CR-{{ITERATION_SLUG}}-{{RFC_NAME}}.md

  MAX_DOC_LINES       = 250                     # límite “duro” por documento texto de la iteración

  # Objetivo
  # Implementar TODO lo definido en el RFC aprobado en {{RFC_PATH}} (ni más ni menos),
  # dejando:
  #   - IMPLEMENTATION.md
  #   - DIFF-*.txt
  #   - CI-*.md
  # listos y dentro del límite MAX_DOC_LINES, de modo que el Revisor de Código
  # pueda ejecutar su flujo sin pedir contexto extra.

  # Tarea del agente (Dev Senior Go)
  1) Asume la personalidad completa definida en @dev-senior-go.mdc.

  2) Lee el RFC en {{RFC_PATH}} y los docs base del CONTEXT.
     - Si el RFC no está en estado realmente “Dev-Ready” o falta información crítica
       para implementar de forma segura, emite SOLO NEED-INFO (formato más abajo)
       y NO sigas implementando.

  3) Implementa estrictamente TODO lo definido en el RFC:
       - Código Go y wiring necesario (pkg/internal/cmd) en echo/.
       - Flags y configuración (respetando convenciones y compatibilidad).
       - Migraciones de DB idempotentes y con rollback razonable si aplica.
       - Instrumentación completa: métricas, spans y logs según PR-OBS.
       - Ajustes en tests unitarios e integración existentes afectados.
     Sin “mejoras” fuera de alcance ni refactors grandes que el RFC no pida.

  4) Mantén IMPLEMENTATION.md en {{IMPLEMENTATION_PATH}} siguiendo la plantilla
     definida en tu personalidad:
       - Debe ser la guía única de implementación para Dev y QA.
       - Debe quedar dentro de MAX_DOC_LINES (~250 líneas).

  5) Ejecuta build, lint, tests unitarios e integración relevantes.
     - Resume el resultado en {{CI_REPORT_PATH}}:
         - Estado de cada job (ok/fail) y causa breve en caso de fallo.
         - Sin pegar logs enormes; solo síntesis accionable.
         - Mantén este archivo también dentro de MAX_DOC_LINES.

  6) Genera un diff textual de la iteración en {{DIFF_PATH}}:
       - Resume cambios relevantes: paths, firmas, contratos, flags, migraciones.
       - Agrupa por paquete/módulo.
       - Mantén el diff textual dentro de MAX_DOC_LINES (no es un volcado raw
         infinito del VCS, es un resumen legible para el revisor).

  7) Señal de bloqueo — si en cualquier punto detectas:
       - Ambigüedad crítica en el RFC,
       - Conflicto irresoluble con arquitectura base,
       - O imposibilidad de mantener compatibilidad según principios de Echo,
     NO sigas. Emite SOLO NEED-INFO (formato más abajo).

  8) Salida en chat (MUY IMPORTANTE):
       - NO imprimas IMPLEMENTATION.md, NI DIFF, NI CI completo.
       - Si la implementación es viable: emite SOLO un bloque PROMPT_NEXT_AGENT
         dirigido al Revisor de Código, con TODO lo que requiere su prompt.
       - Si la implementación NO es viable: emite SOLO el bloque NEED-INFO.

  # Formato de salida — PROMPT_NEXT_AGENT hacia Revisor de Código

  # Usa exactamente este formato cuando la implementación esté lista; rellena los
  # campos con los valores efectivos de la iteración:

  <<<PROMPT_NEXT_AGENT_START
  [Para: Revisor de Código]

  # Identidad de la iteración
  ITERATION_SLUG: {{ITERATION_SLUG}}
  RFC_NAME: {{RFC_NAME}}

  # Artefactos críticos (coinciden con los vars del revisor)
  RFC_PATH: {{RFC_PATH}}
  IMPLEMENTATION_PATH: {{IMPLEMENTATION_PATH}}
  DIFF_PATH: {{DIFF_PATH}}
  CI_REPORT_PATH: {{CI_REPORT_PATH}}
  CR_PATH: {{CR_PATH}}

  REVIEW_ITER: 1
  MAX_DOC_LINES: {{MAX_DOC_LINES}}

  # Estado técnico de la implementación (síntesis para guiar la revisión)
  Estado_build_y_tests:
    - build: ok|fail — <motivo si fail>
    - lint: ok|fail — <motivo si fail>
    - unit: ok|fail — <motivo si fail>
    - integración: ok|fail — <motivo si fail>

  Notas_para_revisor:
    - Riesgos_conocidos: <lista breve o “ninguno relevante”>
    - Supuestos_no_explícitos_en_RFC: <si existen, y deben estar documentados también en IMPLEMENTATION.md>
    - Puntos_de_foco_sugeridos:
      - contratos vs RFC (firmas, tipos, BWC, idempotencia)
      - concurrencia/context/cancelación (si aplica)
      - observabilidad (métricas, spans, logs)
      - performance (si hay paths sensibles)

  Instrucciones:
    1) Ejecuta tu flujo usando echo/vibe-coding/prompts/revisor-codigo.request.md
       con las vars de ruta y configuración indicadas arriba.
    2) Valida exclusivamente contra el RFC aprobado y los estándares técnicos de Echo.
  <<<PROMPT_NEXT_AGENT_END

  # En caso de bloqueo — NEED-INFO (misma señal que define la personalidad)

  # Si no puedes implementar de forma segura por falta de información o conflicto grave,
  # la salida debe ser SOLO:

  NEED-INFO: [RFC {{ITERATION_SLUG}} — {{RFC_NAME}}]
  Falta: <dato exacto del RFC o doc base que no está o es ambiguo>
  Impacto: <por qué bloquea implementación segura o rompe compatibilidad/BWC>
  Propuesta: <preguntas concretas o alternativas para Arquitecto/PM>
</REQUEST>
