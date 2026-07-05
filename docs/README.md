# Documentación de steer — índice y convenciones

**Actualizado:** 2026-07-05

## Mapa

| Documento | Qué es |
|---|---|
| [`design.md`](design.md) | Visión y diseño V1 (documento vivo; secciones realineadas al código el 2026-07-05) |
| [`parity.md`](parity.md) | **Matriz de paridad CLI↔TUI** y gaps documentados con plan de cierre |
| [`superpowers/plans/2026-06-15-roadmap.md`](superpowers/plans/2026-06-15-roadmap.md) | Roadmap por hitos: estado real, adiciones y qué sigue |
| `superpowers/specs/*-design.md` | Specs (diseños aprobados) por hito |
| `plan/*.md` | Planes de implementación ejecutables (y algunos diseños tempranos `*-diseno.md`) |
| `superpowers/plans/2026-06-*.md` | Planes de los hitos fundacionales (01, 02, 06) |

## Convenciones (a partir de 2026-07-05)

- **Specs (diseños)** → `docs/superpowers/specs/YYYY-MM-DD-<tema>-design.md`.
- **Planes de implementación** → `docs/plan/YYYY-MM-DD-<tema>.md`.
- Nota histórica: algunos hitos de 2026-07-04/05 dejaron su diseño como
  `docs/plan/*-diseno.md` en vez de en specs/. Se quedan donde están (los enlaces desde
  el roadmap los cubren); los hitos nuevos siguen la convención de arriba.
- **Paridad**: todo spec de capacidad debe cubrir CLI y TUI, o registrar el gap en
  [`parity.md`](parity.md) con su justificación y plan de cierre.
- El roadmap se actualiza al cerrar cada hito (estado + adiciones), y el README de la
  raíz refleja solo el estado grueso.
