<ROL>
   debes tomar exclusivamente la personalidad descrita en vibe-coding/.cursor/rules/revisor-codigo.mdc
<ROL>

<CONTEXT>
  # Docs base de Echo (mínimo estos tres)
  # Docs base obligatorias (solo lectura)
  docs/00-contexto-general.md
  docs/01-arquitectura-y-roadmap.md
  docs/rfcs/RFC-architecture.md
  vibe-coding/prompts/common-principles.md

  # RFC y artefactos de la iteración
  @{{RFC_PATH}}                  # RFC aprobado de la iteración
  @{{IMPLEMENTATION_PATH}}       # IMPLEMENTATION.md del Dev

  # Otros docs relevantes (opcional, según feature)
  # @docs/telemetry/*.md
  # @docs/rfcs/RFC-00X-relacionado.md
  {{PROMPT_NEXT_AGENT}}
</CONTEXT>

<REQUEST>
  # Vars de tarea (instanciar antes de invocar)
  ITERATION_SLUG   = {{ITERATION_SLUG}}          # p.ej. i8(a+b)
  RFC_NAME         = {{RFC_NAME}}                # slug kebab del RFC
  RFC_PATH         = docs/rfcs/{{ITERATION_SLUG}}/RFC-{{ITERATION_SLUG}}-{{RFC_NAME}}.md
  IMPLEMENTATION_PATH = docs/rfcs/{{ITERATION_SLUG}}/IMPLEMENTATION.md
  CR_PATH          = docs/rfcs/{{ITERATION_SLUG}}/CR-{{ITERATION_SLUG}}-{{RFC_NAME}}.md
  REVIEW_ITER      = {{REVIEW_ITER}}             # default: 1
  MAX_DOC_LINES    = 250

  # Objetivo
  # Validar que la implementación de la iteración {{ITERATION_SLUG}} para {{RFC_NAME}}
  # cumple el RFC aprobado y los estándares técnicos de Echo, dejando un informe
  # CR.md accionable para Dev y QA.

  # Tarea del agente (Revisor de Código)
  1) Asume la personalidad completa definida en @revisor-codigo.mdc.

  2) Lee el RFC en {{RFC_PATH}}, la implementación en {{IMPLEMENTATION_PATH}}.
     Si alguno de estos artefactos críticos falta o está irremediablemente
     incompleto, emite SOLO NEED-INFO según tu personalidad.

  3) Verifica, basándote en el diff y CI:
       - Que todas las apps, bins y paquetes afectados son compilables.
       - Que los tests relevantes (unitarios, integración, linters) pasan o,
         si fallan, qué falla y por qué es relevante.
       - Que no existan desviaciones respecto de lo especificado en el RFC
         (scope, contratos, flags, bots, telemetría, etc.).

  4) Evalúa la implementación según estas dimensiones:
       - Contratos vs RFC (firmas, tipos, parámetros, BWC, idempotencia).
       - Concurrencia, manejo de context y cancelación.
       - Manejo de errores y límites (timeouts, retries, tamaños, edge cases).
       - Observabilidad: logs estructurados, métricas, spans y etiquetas.
       - Impacto razonable en performance según los principios de Echo.

  5) Genera o actualiza CR.md en {{CR_PATH}} siguiendo EXACTAMENTE el
     FORMATO_CR_MD definido en tu personalidad:
       - Incluye matriz de hallazgos con severidad y evidencia.
       - Incluye sección específica de contratos vs RFC.
       - Incluye sección de observabilidad (métricas/spans) y performance.
       - Mantén el archivo dentro de ~250 líneas totales.

  6) Decide un dictamen global:
       - Listo
       - Cambios menores
       - Cambios mayores
     según las reglas de severidad de tu personalidad.

  7) Salida en chat:
       - Si la revisión es viable: escribe CR.md en {{CR_PATH}} y emite SOLO
         el bloque PROMPT_NEXT_AGENT descrito en tu personalidad, dirigido
         al Dev Autor y con el dictamen final.
       - Si la revisión NO es viable (falta contexto crítico, archivos
         fuera de límite razonable, etc.): emite SOLO el bloque NEED-INFO.

  8) No imprimas CR.md completo en el chat ni ningún otro contenido extra.
     El chat debe contener exclusivamente PROMPT_NEXT_AGENT O NEED-INFO.
</REQUEST>
