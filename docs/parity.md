# Paridad CLI ↔ TUI

**Actualizado:** 2026-07-09

**Principio de producto:** toda capacidad de steer debe estar soportada en **ambos
frentes** — CLI (scriptable, CI) y TUI (interactivo) — sobre el mismo core. Un frente
puede tener ergonomía propia (el filtro `/` del sidebar no necesita equivalente CLI;
`grep` ya existe), pero ninguna *función* debe existir en un solo frente sin que el gap
esté documentado aquí con su plan de cierre.

## Matriz de paridad (estado real del código)

| Función | CLI | TUI | Paridad |
|---|---|---|---|
| Listar servicios + salud | `service status` (alias `ls`), `-w` refresca | Sidebar SERVICES + Details, auto-refresh 15s | ✅ |
| Deploy (validado contra registry) | `service deploy -s -t [-y]`; bloquea tag/repo inexistente; degrada con warning | Formulario inline + tag-picker; bloquea con línea roja; degrada con notice | ✅ |
| Deploy interactivo | Picker de servicio+tag cuando faltan flags | Botones/`d` + tag-picker filtrable | ✅ |
| Watch de rollout + detección de atasco | `deploy -w`: eventos en vivo, corta al 3er fallo de pull con sugerencia de rollback | Pestaña Events en vivo, corta igual y sugiere `R` | ✅ |
| Scale | `service scale -s -c [-y]` | Formulario Scale | ✅ |
| Rollback | `service rollback -s [-y]` | Formulario Rollback (confirm) | ✅ |
| Resize (CPU/memoria) | `service resize -s --cpu --memory [-w]` | Formulario `[ Resize (z) ]` con picker de combos | ✅ |
| Repos de imágenes | `image ls` (repo + último tag + antigüedad) | Sección IMAGES (nombres; el detalle vive en el panel) | ✅ funcional (ver nota 1) |
| Tags de un repo + desplegado | `image tags -r` (`● now`) | Panel TAGS al seleccionar repo (`● now`) | ✅ |
| Selección de contexto | `--context` / `STEER_CONTEXT` / `default_context` | Switcher en vivo (`c` / click en top bar) | ✅ (modelos distintos, misma función) |
| Guard de solo-lectura | `writable=false` bloquea comandos mutantes | Igual: notice y acciones bloqueadas | ✅ |
| Setup de config | `config init\|add\|remove\|list\|validate` | — (sin config, apunta al wizard) | ✅ aceptado (nota 2) |
| Logs del servicio | `service logs -s [-f] [-n]` (tail 1h, merge multi-contenedor) | Pestaña Logs viva (tail + follow 3s) | ✅ |
| Events históricos del servicio | `service events -s` (últimos 20, ascendente) | Pestaña Events poblada en reposo (tick 15s) | ✅ |
| Bases de datos | ❌ | ❌ sección DATABASES stub | ⏳ hito 04 del roadmap |
| `redeploy` / `promote` | ❌ | ❌ | ⏳ hito 02b del roadmap |

## Gaps documentados

1. **`image ls` muestra último tag; el sidebar no.** Asimetría menor deliberada: en el
   TUI el detalle (tags, antigüedad, tamaño) vive en el panel al seleccionar el repo;
   duplicarlo en la fila del sidebar lo saturaría. No requiere acción.
2. **`config init|add|remove|list|validate` es CLI-only por diseño.** Es el paso previo a
   que exista un contexto utilizable: la TUI no puede arrancar sin config válida. El hito
   **08 (onboarding)** cerró esto: `config init` ahora es un wizard interactivo (detecta
   perfiles AWS, valida, smoke test) y, si la TUI arranca sin `steer.toml`, el error
   apunta explícitamente al wizard (`no steer.toml found — try: steer config init`) en
   vez de un mensaje genérico. El wizard embebido en la TUI misma (en vez de solo el
   puntero al comando) queda fuera de alcance — no se identificó necesidad real.
3. **Logs — cerrado por el hito 03b.** El gap doble (pestaña Logs stub + sin comando
   CLI) se cerró con `core.LogSource`, implementado sobre CloudWatch Logs con
   auto-discovery desde la task definition (driver `awslogs`, cero configuración nueva en
   `steer.toml`): CLI `service logs -s [-f] [-n]` (tail de las últimas 100 líneas dentro de
   una ventana de 1h, follow cada 3s, merge multi-contenedor con prefijo `[container]`) y
   pestaña Logs viva en la TUI (mismo tail+follow, auto-scroll inteligente). La ventana de
   1h es una limitación real de `FilterLogEvents` (solo lee hacia delante, no hay "últimas
   N líneas" arbitrario sin ella). Si el driver de logging no es compatible, ambos frentes
   devuelven `core.ErrNoLogSource` con mensaje explicativo en vez de fallar en silencio.
   Spec: `docs/superpowers/specs/2026-07-09-logs-events-design.md`, plan:
   `docs/superpowers/plans/2026-07-09-logs-events.md`.
4. **Events históricos — cerrado por el hito 03b.** Se reutilizó
   `Deployer.ServiceEvents` para ambos frentes: CLI `service events -s` (últimos 20,
   ascendente) y pestaña Events poblada en reposo vía el tick de 15s ya existente. Durante
   un rollout activo, el feed de deploy en vivo conserva la propiedad de la pestaña Events
   (no compite con la carga histórica). Mismo spec y plan que la nota 3.

## Regla para nuevos hitos

Todo spec de capacidad nueva debe incluir explícitamente las dos superficies (comandos
CLI y sección/vista TUI) o justificar en este documento por qué una queda fuera. El hito
images (`docs/superpowers/specs/2026-07-05-images-registry-design.md`) es el ejemplo del
patrón completo: core interface → provider → CLI → TUI en el mismo plan.
