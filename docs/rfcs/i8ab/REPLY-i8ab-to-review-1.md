| Hallazgo | Acción (Cambiar/Justificar) | PR-* | Sección corregida/enlazada |
|----------|----------------------------|------|---------------------------|
| H1 — Fallback ausente ante `INVALID_STOPS` | Cambiar: se documentó degradado determinista (reintento sin offset, `ModifyOrder` opcional, métricas/spans y KPI ≥95 % éxito). | PR-ROB, PR-RES | `§3`, `§5.3`, `§5.4`, `§8.1`, `§8.3`, `§9.1–9.4`, `§14` |
| H2 — Métricas con alta cardinalidad | Cambiar: métricas usan labels agregados (`segment`, `type`, `result`) y los IDs quedan en logs/spans conforme a la guía de observabilidad. | PR-OBS | `§8.1` |

