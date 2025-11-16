# Echo — Prompt de Tarea: Arquitecto Revisor (Dev/QA-Ready) v3

<ROL>
   debes tomar exclusivamente la personalidad descrita en echo/vibe-coding/.cursor/rules/arquitecto-revisor.mdc
<ROL>

<CONTEXT>
  # Docs base obligatorias (solo lectura)
  echo/docs/00-contexto-general.md
  echo/docs/01-arquitectura-y-roadmap.md
  echo/docs/rfcs/RFC-architecture.md
  echo/vibe-coding/prompts/common-principles.md

  # RFC a auditar (ruta absoluta)
  echo/docs/rfcs/{{ITERATION_SLUG}}/RFC-{{ITERATION_SLUG}}-{{RFC_NAME}}.md

  # Opcionales: RFCs relacionados u otros docs específicos de la iteración
  # echo/docs/rfcs/RFC-00X-relacionado.md
  # ...

  # Handoff del Arquitecto Autor:
  # Pega íntegro aquí el bloque PROMPT_NEXT_AGENT que emitió el Arquitecto Autor
  # para este RFC (incluyendo rfc_path, sha256, line_count y refs base).
  {{PROMPT_NEXT_AGENT_AUTOR}}
</CONTEXT>

<REQUEST>
  # Vars de tarea (instanciar antes de ejecutar)
  ITERATION_SLUG = {{iN}}
  RFC_NAME       = {{slug-kebab}}
  REVIEW_ITER    = {{1}}          # Aumenta en cada ciclo de review
  TIME_BUDGET_S  = {{120}}

  # Derivadas (no modificar salvo que cambie el layout del repo)
  RFC_PATH    = echo/docs/rfcs/{{ITERATION_SLUG}}/RFC-{{ITERATION_SLUG}}-{{RFC_NAME}}.md
  OUTPUT_PATH = echo/docs/rfcs/{{ITERATION_SLUG}}/RFC-{{ITERATION_SLUG}}-{{RFC_NAME}}-review-{{REVIEW_ITER}}.md
  REPO_ROOT   = echo/

  # Tarea
  1) Ejecuta el **PRE-FLIGHT** definido en tu personalidad sobre RFC_PATH y las refs base.
  2) Si PRE-FLIGHT pasa, realiza la revisión completa Dev/QA-Ready y escribe el archivo
     de revisión en OUTPUT_PATH siguiendo exactamente la estructura estándar descrita en
     [TAREA_CONTENIDO_ARCHIVO_REVISION] de tu personalidad.
  3) Calcula sha256 y line_count del archivo de revisión.
  4) Si todo es correcto, responde en este chat únicamente con el bloque `PROMPT_NEXT_AGENT`
     definido en tu personalidad, rellenando:
       - review_path
       - review_iter
       - review_sha256
       - review_line_count
       - decisión y conteos de severidades.
  5) Si falla cualquier paso de PRE-FLIGHT o no puedes completar la revisión sin inventar
     información crítica, responde únicamente con un bloque `NEED-INFO` conforme a tu
     personalidad, indicando motivo, detalle y cómo resolverlo.
</REQUEST>
