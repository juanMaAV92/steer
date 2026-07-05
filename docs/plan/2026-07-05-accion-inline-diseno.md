# Formulario de acción inline (deploy · scale · rollback) — Diseño

**Fecha:** 2026-07-05 · **Estado:** aprobado
**Objetivo:** reemplazar el modal centrado de acciones por un formulario inline dentro del
panel de detalles. Motivación: el modal actual se cierra con cualquier click (incluso sobre
sus propios botones) sin que el usuario note qué pasó, sus botones no son clickeables, y
tapa el contexto (tag actual, running count) que se necesita para decidir.

## Decisiones (votadas)

1. **Ubicación: inline en el panel de detalles**, bajo la fila de botones de acción. Los
   datos actuales del servicio (Tag, Running, Status) quedan visibles arriba — no se
   duplican dentro del formulario. Al no ser overlay, el problema del "click fuera cierra
   en silencio" desaparece por construcción.
2. **Click fuera del formulario: no-op.** El formulario solo se cierra con `esc` o el
   botón `[ Cancel ]`. Imposible perder el input tecleado por un click accidental. El
   picker de contextos sigue siendo overlay y conserva su comportamiento.
3. **Foco navegable entre botones**: tab/←→/shift-tab mueve el foco entre confirmar y
   cancelar; el botón enfocado se resalta con la barra cian (`colorSelectionBar`, la misma
   de la selección del sidebar); `enter` activa el botón enfocado; `esc` cancela siempre.
   El foco inicia en el botón de confirmar. Click sobre cualquiera de los dos botones lo
   activa directamente.

## Modelo de interacción

- `d`/`s`/`R` (o click en `[ Deploy ]`/`[ Scale ]`/`[ Rollback ]`) abre el formulario del
  kind correspondiente para el servicio seleccionado.
- **Modo captura** (mismo patrón del filtro `/`): con el formulario abierto, las runas y
  backspace editan el input; las teclas globales (`d`, `q`, `c`, `/`…) NO disparan;
  tab/←→ mueven foco; enter activa; esc cancela.
- **Mouse con formulario abierto**: click en botón del formulario → activa; click en
  cualquier otra zona (sidebar, tabs, top bar, botones de acción) → no-op. La rueda sigue
  funcionando (scroll de sidebar/events es inofensivo y de solo lectura).
- **Rollback** usa el mismo formulario sin campo de input: texto explicativo +
  `[ Confirm ] [ Cancel ]`.
- Confirmar emite `actionConfirmedMsg{kind, service, input}` — el flujo aguas abajo
  (startDeployCmd, scale, rollback) no cambia.

## Arquitectura

- **`actionForm`** (evolución de `action`, deja de ser overlay): estado `kind, service,
  input, focus (0=confirmar, 1=cancelar)`. Expone:
  - `rows(width) []formRow` — filas renderizadas con la geometría de cada botón
    (fila + rango de columnas), **fuente única** para render y hit-testing, misma técnica
    del sidebar: no pueden divergir por construcción.
  - `typeKey`, `moveFocus(delta)`, `activate() (done bool, result tea.Msg)`, `ready()`.
- **`actionOverlay` se elimina.** El picker (`pickerOverlay`) sigue siendo el único
  overlay. `actionConfirmedMsg` se conserva tal cual.
- **Render**: el panel de detalles dibuja el formulario bajo la fila de botones cuando
  está activo (caja con borde redondeado y título del kind). Las pestañas son excluyentes
  (no hay zona de events debajo de Details); en alturas degeneradas el formulario se
  recorta con el resto del panel, igual que hoy.
- **Ruteo en `app.go`** (orden): ctrl+c → overlay (picker) → **captura del formulario**
  → captura del filtro → teclas globales. En mouse: si el formulario está abierto, solo
  se atienden clicks dentro de su geometría (vía `rows`) y la rueda; el resto no-op.
- Layout single-column: el formulario vive en la misma zona de detalles (mitad superior).

## Fuera de alcance

Autocompletado/tag-picker en el input de deploy (hito registry); validación semántica del
tag o del count más allá de `ready()` (no vacío); animaciones de expansión; recordar el
último valor tecleado entre aperturas.

## Pruebas

- Unit (`actionForm`): foco con wrap (tab/←→/shift-tab), typeKey (runas/backspace,
  rollback ignora), `ready()` por kind, `activate()` en confirmar vs cancelar.
- Ancladas al render (coordenadas derivadas del `View()` real): click en el botón
  confirmar del formulario emite la acción; click en `[ Cancel ]` cierra; click en el
  sidebar/tabs con formulario abierto no hace nada; teclas globales capturadas mientras
  el formulario está abierto; esc cierra sin emitir.
- Las 9 pruebas de click existentes pasan sin modificar (los clicks que ABREN el
  formulario desde los botones de acción se conservan; cambia lo que pasa después).
- Comportamiento intacto: picker de contextos (overlay), filtro `/`, deploy en curso.
