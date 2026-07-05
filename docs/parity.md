# Paridad CLI ↔ TUI

**Actualizado:** 2026-07-05

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
| Repos de imágenes | `image ls` (repo + último tag + antigüedad) | Sección IMAGES (nombres; el detalle vive en el panel) | ✅ funcional (ver nota 1) |
| Tags de un repo + desplegado | `image tags -r` (`● now`) | Panel TAGS al seleccionar repo (`● now`) | ✅ |
| Selección de contexto | `--context` / `STEER_CONTEXT` / `default_context` | Switcher en vivo (`c` / click en top bar) | ✅ (modelos distintos, misma función) |
| Guard de solo-lectura | `writable=false` bloquea comandos mutantes | Igual: notice y acciones bloqueadas | ✅ |
| Setup de config | `config init` / `config validate` | — | ✅ aceptado (nota 2) |
| **Logs del servicio** | ❌ no existe comando | ❌ pestaña Logs es stub | 🔴 GAP doble (nota 3) |
| **Events históricos del servicio** | ❌ solo durante `deploy -w` | ❌ pestaña Events vacía fuera de un deploy | 🟡 GAP doble (nota 4) |
| Bases de datos | ❌ | ❌ sección DATABASES stub | ⏳ hito 04 del roadmap |
| `redeploy` / `promote` | ❌ | ❌ | ⏳ hito 02b del roadmap |

## Gaps documentados

1. **`image ls` muestra último tag; el sidebar no.** Asimetría menor deliberada: en el
   TUI el detalle (tags, antigüedad, tamaño) vive en el panel al seleccionar el repo;
   duplicarlo en la fila del sidebar lo saturaría. No requiere acción.
2. **`config init|validate` es CLI-only por diseño.** Es el paso previo a que exista un
   contexto utilizable: la TUI no puede arrancar sin config válida. El hito **08
   (onboarding)** convertirá `config init` en wizard; si algún día la TUI arranca sin
   config, deberá ofrecer ese wizard embebido — decisión para el diseño del hito 08.
3. **Logs (GAP doble, el mayor).** La pestaña Logs del panel existe desde el rediseño
   pero es un stub ("no log source configured"), y no hay `steer service logs` en CLI.
   Falta la capacidad completa: interface `core.LogSource` (CloudWatch Logs en AWS) +
   `service logs -s [-f]` en CLI + pestaña viva en TUI. **Cierre propuesto: hito propio
   "logs" después de 07/08** (antes de 04 db si el equipo lo pide — es la pestaña que ya
   está a la vista). Registrado en el roadmap.
4. **Events históricos (GAP doble, menor).** ECS siempre tiene eventos del servicio,
   pero hoy solo se muestran durante un rollout: la pestaña Events queda vacía en reposo
   y no existe `service events` en CLI. Cierre barato reutilizando
   `Deployer.ServiceEvents` (ya existe): poblar la pestaña al seleccionar servicio +
   subcomando `service events -s`. **Cierre propuesto: junto al hito de logs** (misma
   zona de UI, mismo patrón de carga).

## Regla para nuevos hitos

Todo spec de capacidad nueva debe incluir explícitamente las dos superficies (comandos
CLI y sección/vista TUI) o justificar en este documento por qué una queda fuera. El hito
images (`docs/superpowers/specs/2026-07-05-images-registry-design.md`) es el ejemplo del
patrón completo: core interface → provider → CLI → TUI en el mismo plan.
