# Refactor de TUI (pre-⌘k) — Diseño

**Fecha:** 2026-07-04 · **Estado:** aprobado (bloque 1 de la revisión de arquitectura + cosméticos)
**Alcance:** solo refactor — cero features nuevas. Prepara la TUI para la paleta ⌘k y las
secciones IMAGES/DATABASES sin reescrituras.

## Decisiones

1. **Abstracción `overlay`** (reemplaza los estados de foco `focusAction`/`focusContextPicker`):

```go
// overlay es una capa modal que captura teclado y mouse mientras está activa.
type overlay interface {
	// Update procesa un evento. done=true cierra el overlay; result (opcional)
	// es un tea.Msg tipado que el Model ejecuta (p.ej. contextChosenMsg).
	Update(msg tea.Msg) (done bool, result tea.Msg)
	// View renderiza el overlay para el área del cuerpo (width×height).
	View(width, height int) string
}
```

   - `Model` gana `overlay overlay` (nil = ninguno) y pierde los campos `picker`/`action`
     y los valores de foco `focusAction`/`focusContextPicker` (el enum queda
     `focusSidebar | focusPanel`).
   - Routing único en `Update`: ctrl+c siempre sale; si `m.overlay != nil`, teclado y mouse
     van al overlay; `done` lo cierra; `result` se despacha en `handleOverlayResult`.
   - Resultados tipados: `contextChosenMsg{ctx config.Context}` (picker) y
     `actionConfirmedMsg{kind, service, input}` (modal). `applyContextSwitch` pasa a recibir
     el contexto elegido; `runActionCmd` se parametriza (ya no lee `m.action`).
   - Los structs existentes `contextPicker` y `action` se conservan como núcleo de los dos
     implementadores (`pickerOverlay`, `actionOverlay`) — render y lógica de input intactos.
   - La paleta ⌘k futura = tercer implementador, sin tocar el enum ni el routing.

2. **Sidebar consciente de secciones**: `HitAtRow(row int) (sidebarHit, bool)` con
   `sidebarHit{Section, Index}` y constantes `sectionServices/sectionImages/sectionDatabases`.
   El mapeo replica la estructura de `view()` (mismo patrón que `indexAtLine` del picker).
   Solo `sectionServices` es accionable hoy; cursores por sección se añadirán cuando las
   secciones tengan filas reales (YAGNI). `handleMouse` usa `HitAtRow` en vez de la
   aritmética con `serviceRowCount`.

3. **Hit-testing unificado**: `render.LabelAtColumn(labels []string, pad, gap, x int) int`
   como primitiva única (ancho por **runas**). `ButtonAtColumn = LabelAtColumn(labels, 4,
   2, x)`; `Tabs.TabAtColumn` delega en ella (de paso corrige el `len()` por bytes latente).
   El picker sigue con `indexAtLine` (dominio distinto: filas, no columnas).

4. **Geometría derivada, no mágica**: `panel.DetailsButtonLine = 7` (línea de la fila de
   botones dentro del output de `DetailsView`), con un test que la valida contra el render
   real. `app.go` computa `detailsButtonRowY = topBarHeight + borderTop + 1(tabs) +
   1(blanco) + panel.DetailsButtonLine`.

5. **keyMap completo**: `Left` (`left/h`), `Right` (`right/l`), `Context` (`c`) entran al
   keyMap; `handleKey` usa `key.Matches` — se eliminan los `msg.String()` crudos.

6. **`providers.IsImplemented(cloud string) bool`**: fuente única de "¿este cloud tiene
   provider?"; la fábrica la usa y el `contextpicker` deja de hardcodear `!= "aws"`.

7. **Cosméticos del ledger**: `ctx config.Context` → `cfgCtx` en `aws/provider.go`; límite
   de palabra en la detección de esquema legacy; assert duplicado en `rollout_test.go`;
   fijar el string del error en `TestRunActionCmdRejectsDeploy`; guard de `deploy -y` antes
   de `RequireWritable`.

## Invariantes

- Comportamiento observable idéntico (teclas, clicks, flujos de deploy/switch); los tests
  anclados al render existentes deben seguir pasando sin debilitarse.
- `render` sigue hoja; `core` stdlib-only.
- Fuera de alcance: paleta ⌘k, filas reales en IMAGES/DATABASES, config anidada.
