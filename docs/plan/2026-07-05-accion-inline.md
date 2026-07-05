# Formulario de acción inline — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reemplazar el modal centrado de deploy/scale/rollback por un formulario inline en el panel Details, con foco navegable entre botones, todo clickeable, y click fuera = no-op.

**Architecture:** Un componente `actionForm` (evolución de `action`) se renderiza bajo la fila de botones de Details; captura el teclado con el mismo patrón del filtro `/` y expone su geometría de botones (fila + hit-test por columna) como fuente única para render y mouse. `actionOverlay` desaparece; el picker de contextos sigue siendo el único overlay. `actionConfirmedMsg` se conserva tal cual, así el flujo aguas abajo (startDeployCmd, scale, rollback) no cambia.

**Tech Stack:** Go, Bubble Tea (Elm), lipgloss, testify. Spec: `docs/plan/2026-07-05-accion-inline-diseno.md`.

## Global Constraints

- Comentarios en español; strings de UI en inglés.
- PROHIBIDO cualquier atribución a Claude/IA en commits, comentarios o PRs.
- Branch de trabajo: `feat/accion-inline` (creada en Task 1 desde main).
- Antes de CADA commit: `gofmt -w internal/ && golangci-lint run && go vet ./... && go test ./... -count=1 && go build ./...` — todo verde.
- Tests de click anclados al render: X/Y derivados del texto real de `View()` (columnas en runas, patrón existente con `stripANSI`), nunca coordenadas inventadas.
- Fuente única render/hit-testing: la geometría de botones del formulario sale de `actionForm` (consts + `buttonAt`), no de números mágicos en app.go.
- Click fuera del formulario abierto = no-op (no cierra, no selecciona, no abre picker). Solo `esc` o `[ Cancel ]` cierran.
- El foco inicial del formulario es el botón de confirmar (índice 0).
- Los tests existentes de click que hoy asertan `m.overlay`/`actionOverlay` se adaptan SOLO en su aserción final (overlay → form); su mecánica de anclaje al render no se toca. Cualquier otra debilitación de asserts es un defecto.

---

### Task 1: `render.ButtonsWithFocus` + componente `actionForm`

Componente puro y helper de render, sin cableado en el Model. `action`/`actionOverlay` viejos siguen intactos y compilando; se eliminan en Task 2.

**Files:**
- Modify: `internal/render/buttons.go`
- Modify: `internal/render/buttons_test.go`
- Create: `internal/tui/form.go`
- Create: `internal/tui/form_test.go`
- Modify: `internal/tui/styles.go` (colorSelectionBar pasa a referenciar render)

**Interfaces:**
- Consumes: `render.Accent`, `render.BrandColor` (`internal/render/table.go:24`), `render.LabelAtColumn`/`buttonGap` (`internal/render/buttons.go`), `actionKind`/`actionDeploy`/`actionScale`/`actionRollback` (`internal/tui/app.go:36-42`), `actionConfirmedMsg` (`internal/tui/overlay.go:23`).
- Produces (Task 2 y 3 dependen de esto):
  - `render.SelectionBarColor` (const `"#0c3a44"`) y `render.ButtonsWithFocus(labels []string, focus int) string`.
  - `newActionForm(kind actionKind, service string) *actionForm`
  - métodos: `typeKey(msg tea.KeyMsg)`, `moveFocus(delta int)`, `ready() bool`, `activate() (bool, tea.Msg)`, `activateIndex(idx int) (bool, tea.Msg)`, `labels() []string`, `view() string`, `buttonAt(row, x int) int`
  - consts: `formButtonRow = 3`, `formContentX0 = 2`.

- [ ] **Step 1: Crear la branch**

```bash
git checkout main && git pull && git checkout -b feat/accion-inline
```

- [ ] **Step 2: Test que falla — render.ButtonsWithFocus**

Añadir a `internal/render/buttons_test.go` (el archivo ya importa `require`; añadir `"github.com/charmbracelet/lipgloss"` al import):

```go
func TestButtonsWithFocusHighlightsAndKeepsWidth(t *testing.T) {
	labels := []string{"Deploy (↵)", "Cancel (esc)"}
	plain := Buttons(labels)
	focused := ButtonsWithFocus(labels, 0)
	require.Contains(t, focused, "Deploy (↵)")
	// el resaltado no puede alterar la geometría: mismo ancho visible
	require.Equal(t, lipgloss.Width(plain), lipgloss.Width(focused))
	// focus fuera de rango = idéntico a Buttons
	require.Equal(t, plain, ButtonsWithFocus(labels, -1))
}
```

- [ ] **Step 3: Verificar que falla**

Run: `go test ./internal/render/ -run TestButtonsWithFocus -v`
Expected: FAIL con "undefined: ButtonsWithFocus"

- [ ] **Step 4: Implementar en `internal/render/buttons.go`**

Añadir el import de lipgloss y reescribir la sección de Buttons:

```go
// SelectionBarColor es el fondo de la barra de selección (cursor del sidebar,
// botón enfocado del formulario de acción).
const SelectionBarColor = "#0c3a44"

// focusedButton pinta la caja del botón enfocado con la barra de selección.
var focusedButton = lipgloss.NewStyle().
	Foreground(lipgloss.Color(BrandColor)).
	Background(lipgloss.Color(SelectionBarColor)).
	Bold(true)

// Buttons renderiza una fila de botones "[ label ]" en cian de marca, separados por
// buttonGap espacios. Las etiquetas se muestran tal cual (ASCII o con glifos).
func Buttons(labels []string) string { return ButtonsWithFocus(labels, -1) }

// ButtonsWithFocus renderiza la fila como Buttons resaltando labels[focus] con la
// barra de selección; el ancho de cada caja no cambia con el foco, así el
// hit-testing de ButtonAtColumn vale para ambas variantes. focus fuera de rango
// no resalta ninguno.
func ButtonsWithFocus(labels []string, focus int) string {
	parts := make([]string, 0, len(labels))
	for i, l := range labels {
		box := "[ " + l + " ]"
		if i == focus {
			parts = append(parts, focusedButton.Render(box))
		} else {
			parts = append(parts, Accent(box))
		}
	}
	return strings.Join(parts, strings.Repeat(" ", buttonGap))
}
```

En `internal/tui/styles.go`, quitar el hex duplicado y referenciar render (añadir import):

```go
package tui

import "github.com/juanMaAV92/steer/internal/render"

const (
	sidebarMinWidth       = 24
	singleColumnThreshold = 80
	colorSelectionBar     = render.SelectionBarColor // cian oscuro — barra de selección
)
```

- [ ] **Step 5: Verificar que pasa**

Run: `go test ./internal/render/ ./internal/tui/ -count=1`
Expected: PASS (todo lo existente sigue verde)

- [ ] **Step 6: Tests que fallan — actionForm**

Crear `internal/tui/form_test.go`:

```go
// internal/tui/form_test.go
package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

func TestFormDeployTypingAndReady(t *testing.T) {
	f := newActionForm(actionDeploy, "api")
	require.False(t, f.ready()) // input vacío
	for _, r := range "v2" {
		f.typeKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(string(r))})
	}
	require.Equal(t, "v2", f.input)
	require.True(t, f.ready())
	f.typeKey(tea.KeyMsg{Type: tea.KeyBackspace})
	require.Equal(t, "v", f.input)
}

func TestFormRollbackAlwaysReadyIgnoresTyping(t *testing.T) {
	f := newActionForm(actionRollback, "api")
	require.True(t, f.ready())
	f.typeKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	require.Empty(t, f.input) // rollback no acepta input
}

func TestFormMoveFocusWraps(t *testing.T) {
	f := newActionForm(actionDeploy, "api")
	require.Equal(t, 0, f.focus) // inicia en confirmar
	f.moveFocus(1)
	require.Equal(t, 1, f.focus)
	f.moveFocus(1)
	require.Equal(t, 0, f.focus) // wrap adelante
	f.moveFocus(-1)
	require.Equal(t, 1, f.focus) // wrap atrás
}

func TestFormActivateConfirmAndCancel(t *testing.T) {
	f := newActionForm(actionDeploy, "api")
	done, res := f.activate() // confirmar sin input → no listo, sigue abierto
	require.False(t, done)
	require.Nil(t, res)
	f.typeKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v2")})
	done, res = f.activate()
	require.True(t, done)
	conf, ok := res.(actionConfirmedMsg)
	require.True(t, ok)
	require.Equal(t, actionDeploy, conf.kind)
	require.Equal(t, "api", conf.service)
	require.Equal(t, "v2", conf.input)

	// Cancel siempre cierra sin resultado, aun sin input
	f2 := newActionForm(actionScale, "api")
	done, res = f2.activateIndex(1)
	require.True(t, done)
	require.Nil(t, res)
}

func TestFormViewAndButtonGeometry(t *testing.T) {
	f := newActionForm(actionRollback, "api")
	out := f.view()
	require.Contains(t, out, "Roll back?")
	require.Contains(t, out, "Confirm (↵)")
	require.Contains(t, out, "Cancel (esc)")
	// formButtonRow declara la fila real de los botones dentro del view
	lines := strings.Split(out, "\n")
	require.Contains(t, stripANSI(lines[formButtonRow]), "Confirm")
	// hit-testing: la primera columna del contenido cae en el botón 0
	require.Equal(t, 0, f.buttonAt(formButtonRow, formContentX0))
	require.Equal(t, -1, f.buttonAt(formButtonRow-1, formContentX0)) // otra fila
	require.Equal(t, -1, f.buttonAt(formButtonRow, 0))               // borde de la caja
}
```

- [ ] **Step 7: Verificar que fallan**

Run: `go test ./internal/tui/ -run TestForm -v`
Expected: FAIL con "undefined: newActionForm"

- [ ] **Step 8: Implementar `internal/tui/form.go`**

```go
// internal/tui/form.go
package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/juanMaAV92/steer/internal/render"
)

// Geometría del formulario inline, fuente única para render y hit-testing:
// borde(0), título(1), prompt(2), botones(3), borde(4).
const (
	formButtonRow = 3 // fila 0-based de los botones dentro del view del formulario
	formContentX0 = 2 // columnas a la izquierda del contenido: borde(1) + padding(1)
)

// actionForm es el formulario inline de deploy/scale/rollback que se dibuja
// dentro del panel Details, bajo la fila de botones de acción. Reemplaza al
// modal centrado: no es overlay, el click fuera no lo cierra.
type actionForm struct {
	kind    actionKind
	service string
	input   string
	focus   int // 0 = confirmar, 1 = cancelar
}

func newActionForm(kind actionKind, service string) *actionForm {
	return &actionForm{kind: kind, service: service}
}

func (f *actionForm) typeKey(msg tea.KeyMsg) {
	if f.kind == actionRollback {
		return
	}
	switch msg.Type {
	case tea.KeyBackspace:
		if n := len(f.input); n > 0 {
			f.input = f.input[:n-1]
		}
	case tea.KeyRunes:
		f.input += string(msg.Runes)
	}
}

// moveFocus mueve el foco entre confirmar(0) y cancelar(1), con wrap.
func (f *actionForm) moveFocus(delta int) {
	f.focus = (f.focus + delta%2 + 2) % 2
}

func (f actionForm) ready() bool {
	return f.kind == actionRollback || f.input != ""
}

// labels devuelve las etiquetas de los botones (fuente única con el render).
func (f actionForm) labels() []string {
	switch f.kind {
	case actionScale:
		return []string{"Scale (↵)", "Cancel (esc)"}
	case actionRollback:
		return []string{"Confirm (↵)", "Cancel (esc)"}
	default:
		return []string{"Deploy (↵)", "Cancel (esc)"}
	}
}

// activate ejecuta el botón enfocado: cancelar cierra sin resultado; confirmar
// emite actionConfirmedMsg solo si el formulario está listo (si no, sigue abierto).
func (f actionForm) activate() (bool, tea.Msg) {
	if f.focus == 1 {
		return true, nil
	}
	if !f.ready() {
		return false, nil
	}
	return true, actionConfirmedMsg{kind: f.kind, service: f.service, input: f.input}
}

// activateIndex enfoca el botón idx y lo ejecuta (ruta del click).
func (f *actionForm) activateIndex(idx int) (bool, tea.Msg) {
	f.focus = idx
	return f.activate()
}

// buttonAt devuelve el índice del botón bajo la coordenada (row, x) local al
// view del formulario, o -1 si no cae en ninguno.
func (f actionForm) buttonAt(row, x int) int {
	if row != formButtonRow {
		return -1
	}
	return render.ButtonAtColumn(f.labels(), x-formContentX0)
}

// view renderiza la caja del formulario: título, prompt y botones con foco.
func (f actionForm) view() string {
	var title, prompt string
	switch f.kind {
	case actionDeploy:
		title = "Deploy"
		prompt = "image tag:  " + render.Accent(f.input) + "_"
	case actionScale:
		title = "Scale"
		prompt = "desired count:  " + render.Accent(f.input) + "_"
	case actionRollback:
		title = "Roll back?"
		prompt = render.Dim("This reverts to the previous revision.")
	}
	inner := render.Bold(title) + "\n" + prompt + "\n" +
		render.ButtonsWithFocus(f.labels(), f.focus)
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(render.BrandColor)).
		Padding(0, 1).
		Render(inner)
}
```

- [ ] **Step 9: Verificar que pasan**

Run: `go test ./internal/tui/ -run TestForm -v`
Expected: PASS (5 tests)

- [ ] **Step 10: Verificación completa y commit**

```bash
gofmt -w internal/ && golangci-lint run && go vet ./... && go test ./... -count=1 && go build ./...
git add internal/render/buttons.go internal/render/buttons_test.go internal/tui/form.go internal/tui/form_test.go internal/tui/styles.go
git commit -m "feat(render,tui): actionForm con foco navegable y ButtonsWithFocus"
```

---

### Task 2: Cableado por teclado — el formulario reemplaza al modal

El Model gana `form *actionForm`; captura de teclado antes del filtro; `d/s/R` abren el formulario en la pestaña Details; `actionOverlay` y `action` se eliminan.

**Files:**
- Modify: `internal/tui/app.go` (Model, Update, handleFormKey nuevo, openAction, openActionKind, panelBody, handleOverlayResult)
- Modify: `internal/tui/overlay.go` (eliminar actionOverlay; mover actionConfirmedMsg a form.go)
- Modify: `internal/tui/form.go` (recibe actionConfirmedMsg)
- Delete: `internal/tui/action.go`, `internal/tui/action_test.go`
- Modify: `internal/tui/overlay_test.go` (eliminar tests de actionOverlay)
- Modify: `internal/tui/app_test.go` (adaptar aserciones overlay → form; tests nuevos de captura)

**Interfaces:**
- Consumes: `newActionForm`, `activate()`, `moveFocus(delta)`, `typeKey(msg)`, `view()` (Task 1); `m.keys.Esc/Enter/Tab/ShiftTab/Left/Right` (`internal/tui/keys.go`, ya existen).
- Produces (Task 3 depende): `Model.form *actionForm`; `applyActionConfirmed(r actionConfirmedMsg) tea.Cmd` (pointer receiver, ejecuta la acción confirmada); `handleFormKey`.

- [ ] **Step 1: Tests que fallan — captura y apertura**

Añadir a `internal/tui/app_test.go`:

```go
// TestFormOpensInDetailsTabAndCapturesKeys: 'd' abre el formulario inline en la
// pestaña Details y el teclado queda capturado (las teclas globales no disparan).
func TestFormOpensInDetailsTabAndCapturesKeys(t *testing.T) {
	m := newTestModel(sampleServices())
	m.tabs.Active = panel.TabEvents // desde otra pestaña
	m = mustUpdate(t, m, keyMsg("d"))
	require.NotNil(t, m.form)
	require.Equal(t, actionDeploy, m.form.kind)
	require.Equal(t, panel.TabDetails, m.tabs.Active)
	// "q" no cierra la app: se teclea en el input
	updated, cmd := m.Update(keyMsg("q"))
	m = updated.(Model)
	require.Nil(t, cmd)
	require.Equal(t, "q", m.form.input)
}

// TestFormTabMovesFocusEnterActivates: tab enfoca Cancel; enter sobre Cancel cierra sin acción.
func TestFormTabMovesFocusEnterActivates(t *testing.T) {
	m := newTestModel(sampleServices())
	m = mustUpdate(t, m, keyMsg("d"))
	m = mustUpdate(t, m, tea.KeyMsg{Type: tea.KeyTab})
	require.NotNil(t, m.form)
	require.Equal(t, 1, m.form.focus)
	updated, cmd := m.Update(keyMsg("enter"))
	m = updated.(Model)
	require.Nil(t, m.form)
	require.Nil(t, cmd)
}

// TestFormEscCloses: esc cierra el formulario sin emitir acción.
func TestFormEscCloses(t *testing.T) {
	m := newTestModel(sampleServices())
	m = mustUpdate(t, m, keyMsg("s"))
	require.NotNil(t, m.form)
	m = mustUpdate(t, m, keyMsg("esc"))
	require.Nil(t, m.form)
}

// TestFormRendersInsideDetailsPanel: el formulario se dibuja bajo los botones de Details.
func TestFormRendersInsideDetailsPanel(t *testing.T) {
	m := newTestModel(sampleServices())
	m = mustUpdate(t, m, keyMsg("d"))
	out := stripANSI(m.View())
	require.Contains(t, out, "image tag:")
	require.Contains(t, out, "Cancel (esc)")
	require.Contains(t, out, "Deploy (d)") // los botones de Details siguen visibles
}
```

- [ ] **Step 2: Verificar que fallan**

Run: `go test ./internal/tui/ -run 'TestFormOpens|TestFormTab|TestFormEsc|TestFormRenders' -v`
Expected: FAIL con "m.form undefined"

- [ ] **Step 3: Implementar el cableado en `internal/tui/app.go`**

3a. Campo en Model (junto a `overlay overlay`):

```go
	overlay overlay
	form    *actionForm
```

3b. En `Update`, caso `tea.KeyMsg` — la captura del formulario va tras el overlay y antes del filtro:

```go
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
		if m.overlay != nil {
			return m.routeOverlay(msg)
		}
		if m.form != nil {
			return m.handleFormKey(msg)
		}
		if m.sidebar.filterActive {
			return m.handleFilterKey(msg)
		}
		return m.handleKey(msg)
```

3c. Nuevo handler (junto a handleFilterKey):

```go
// handleFormKey captura el teclado mientras el formulario de acción está abierto:
// esc cancela, enter activa el botón enfocado, tab/←/→ mueven el foco y el resto
// se teclea en el input. Las teclas globales NO disparan (modo captura).
func (m Model) handleFormKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.keys.Esc):
		m.form = nil
	case key.Matches(msg, m.keys.Enter):
		done, result := m.form.activate()
		if done {
			m.form = nil
		}
		if result != nil {
			return m.handleOverlayResult(result)
		}
	case key.Matches(msg, m.keys.Tab), key.Matches(msg, m.keys.Right):
		m.form.moveFocus(1)
	case key.Matches(msg, m.keys.ShiftTab), key.Matches(msg, m.keys.Left):
		m.form.moveFocus(-1)
	default:
		m.form.typeKey(msg)
	}
	return m, nil
}
```

3d. Extraer `applyActionConfirmed` y reusar en `handleOverlayResult` (reemplaza el case actionConfirmedMsg completo):

```go
// handleOverlayResult ejecuta la elección hecha en un overlay o en el formulario
// inline. NOTA: el overlay/form ya fue cerrado por el caller antes de llegar aquí.
func (m Model) handleOverlayResult(res tea.Msg) (tea.Model, tea.Cmd) {
	switch r := res.(type) {
	case contextChosenMsg:
		return m.applyContextSwitch(r.ctx)
	case actionConfirmedMsg:
		cmd := m.applyActionConfirmed(r)
		return m, cmd
	}
	return m, nil
}

// applyActionConfirmed ejecuta una acción confirmada (desde teclado o click).
// El deploy va por el flujo en vivo (Events + poll); scale/rollback por runActionCmd.
func (m *Model) applyActionConfirmed(r actionConfirmedMsg) tea.Cmd {
	if r.kind == actionDeploy {
		m.focus = focusPanel
		m.tabs.Active = panel.TabEvents
		m.events.Reset()
		m.deploy = deployState{Active: true, Service: r.service}
		return startDeployCmd(m.runCtx, m.dep, r.service, r.input)
	}
	return m.runActionCmd(r.kind, r.service, r.input)
}
```

(Nota: `cmd := ...; return m, cmd` en dos líneas es deliberado — evita el orden de
evaluación indefinido entre leer `m` y mutarla en la misma sentencia return.)

3e. `openAction` y `openActionKind` abren el formulario (ya no overlay) y fuerzan la pestaña Details:

```go
func (m Model) openAction(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if !m.current.Writable {
		m.notice = "read-only environment (writable=false) — action blocked"
		return m, nil
	}
	s, ok := m.sidebar.selected()
	if !ok {
		return m, nil
	}
	switch {
	case key.Matches(msg, m.keys.Deploy):
		m.form = newActionForm(actionDeploy, s.Name)
	case key.Matches(msg, m.keys.Scale):
		m.form = newActionForm(actionScale, s.Name)
	case key.Matches(msg, m.keys.Rollback):
		m.form = newActionForm(actionRollback, s.Name)
	}
	m.tabs.Active = panel.TabDetails // el formulario vive en Details
	m.focus = focusPanel
	m.notice = ""
	return m, nil
}

// openActionKind abre el formulario para una acción (click en botones de Details).
func (m *Model) openActionKind(kind actionKind) {
	s, ok := m.sidebar.selected()
	if !ok {
		return
	}
	m.form = newActionForm(kind, s.Name)
	m.tabs.Active = panel.TabDetails
	m.focus = focusPanel
	m.notice = ""
}
```

3f. `panelBody` dibuja el formulario bajo Details (case default):

```go
	default:
		displayName := strings.TrimPrefix(s.Name, m.current.Prefix())
		body := panel.DetailsView(s, m.current.Writable, displayName)
		if m.form != nil {
			body += "\n" + m.form.view()
		}
		return body
```

- [ ] **Step 4: Eliminar el modal viejo**

- Borrar `internal/tui/action.go` y `internal/tui/action_test.go` (`git rm`).
- En `internal/tui/overlay.go`: borrar el bloque completo de `actionOverlay` (tipo, constructor, Update, View — líneas 75-113) y la declaración de `actionConfirmedMsg`; actualizar el comentario del interface overlay que menciona `actionConfirmedMsg`.
- En `internal/tui/form.go`: añadir la declaración movida:

```go
// actionConfirmedMsg: el usuario confirmó una acción en el formulario inline.
type actionConfirmedMsg struct {
	kind    actionKind
	service string
	input   string
}
```

- En `internal/tui/overlay_test.go`: borrar `TestActionOverlayTypingAndConfirm`, `TestActionOverlayEnterWithoutInputStaysOpen`, `TestActionOverlayClickCancels`, y en `TestOverlayViewsRender` las 2 líneas del actionOverlay (queda solo el picker; quitar el import de `strings` si queda sin uso).

- [ ] **Step 5: Adaptar aserciones existentes (overlay → form) en `internal/tui/app_test.go`**

Solo cambia la aserción final; el anclaje al render de cada test queda intacto:

- `TestDeployFlowFeedsEventsPanel` (línea ~372): `require.IsType(t, &actionOverlay{}, m.overlay)` → `require.NotNil(t, m.form)`.
- `TestClickDetailsDeployButton` (~414-415): las 2 aserciones finales →
  `require.NotNil(t, m.form)` y `require.Equal(t, actionDeploy, m.form.kind)`.
- `TestClickDetailsScaleAndRollbackButtons` (~470-471): ídem con `tc.kind`.
- `TestSingleColumnDetailsClickNoMisfire` (~440): `require.Nil(t, m.overlay)` → `require.Nil(t, m.form)`.
- `TestReadOnlyDetailsButtonsNoOp` (~487): `require.Nil(t, m.overlay)` → añadir también `require.Nil(t, m.form)`.
- `TestClickCancelsActionModal` (~419-426): BORRARLO — ese comportamiento se elimina por diseño; su reemplazo (`TestClickOutsideFormIsNoop`) llega en Task 3.

- [ ] **Step 6: Verificar que todo pasa**

Run: `go test ./internal/tui/... -count=1 -v -run 'TestForm|TestDeploy|TestClick|TestSingle|TestReadOnly'`
Expected: PASS completo

- [ ] **Step 7: Verificación completa y commit**

```bash
gofmt -w internal/ && golangci-lint run && go vet ./... && go test ./... -count=1 && go build ./...
git add -A internal/tui/
git commit -m "feat(tui): formulario de acción inline en Details (reemplaza modal centrado)"
```

---

### Task 3: Mouse — botones del formulario clickeables, click fuera no-op

Con el formulario abierto: click en sus botones los activa; click en cualquier otra zona no hace nada; la rueda sigue scrolleando sidebar/events.

**Files:**
- Modify: `internal/tui/app.go` (handleMouse + clickForm nuevo)
- Modify: `internal/tui/app_test.go` (helper findInView + 5 tests nuevos)

**Interfaces:**
- Consumes: `Model.form`, `applyActionConfirmed` (Task 2); `actionForm.buttonAt(row, x)`, `activateIndex(idx)`, `formButtonRow` (Task 1); `panel.DetailsButtonLine` (`internal/tui/panel/details.go:18`); `topBarHeight`, `borderTop` (`internal/tui/app.go:31-34`).
- Produces: nada aguas abajo (última task).

- [ ] **Step 1: Helper de anclaje + tests que fallan**

Añadir a `internal/tui/app_test.go`:

```go
// findInView localiza needle en el render y devuelve la coordenada de click
// (columna en runas + 1 para caer dentro de la caja, y la fila). Falla si no aparece.
func findInView(t *testing.T, view, needle string) (x, y int) {
	t.Helper()
	for row, line := range strings.Split(view, "\n") {
		clean := stripANSI(line)
		if i := strings.Index(clean, needle); i >= 0 {
			return utf8.RuneCountInString(clean[:i]) + 1, row
		}
	}
	t.Fatalf("no se encontró %q en el render", needle)
	return -1, -1
}

// TestClickFormConfirmButton: click en [ Deploy (↵) ] del formulario confirma y
// arranca el flujo de deploy en vivo. Anclado al render.
func TestClickFormConfirmButton(t *testing.T) {
	m := newTestModel(sampleServices())
	m = mustUpdate(t, m, keyMsg("d"))
	for _, r := range "v2" {
		m = mustUpdate(t, m, keyMsg(string(r)))
	}
	clickX, clickY := findInView(t, m.View(), "Deploy (↵)")
	click := tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: clickX, Y: clickY}
	updated, cmd := m.Update(click)
	m = updated.(Model)
	require.Nil(t, m.form)
	require.NotNil(t, cmd, "el click en confirmar debe devolver startDeployCmd")
	require.Equal(t, panel.TabEvents, m.tabs.Active)
	require.True(t, m.deploy.Active)
}

// TestClickFormCancelButton: click en [ Cancel (esc) ] cierra sin emitir acción.
func TestClickFormCancelButton(t *testing.T) {
	m := newTestModel(sampleServices())
	m = mustUpdate(t, m, keyMsg("d"))
	clickX, clickY := findInView(t, m.View(), "Cancel (esc)")
	updated, cmd := m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: clickX, Y: clickY})
	m = updated.(Model)
	require.Nil(t, m.form)
	require.Nil(t, cmd)
	require.False(t, m.deploy.Active)
}

// TestClickOutsideFormIsNoop: con el formulario abierto, el click fuera no cierra,
// no cambia la selección y no abre el picker (reemplaza a TestClickCancelsActionModal).
func TestClickOutsideFormIsNoop(t *testing.T) {
	m := newTestModel(sampleServices())
	m = mustUpdate(t, m, keyMsg("d"))
	require.NotNil(t, m.form)
	before, _ := m.sidebar.selected()
	for _, click := range []tea.MouseMsg{
		{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 1, Y: 5}, // sidebar
		{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 1, Y: 0}, // top bar
	} {
		m = mustUpdate(t, m, click)
		require.NotNil(t, m.form, "el formulario no debe cerrarse con click fuera")
		require.Nil(t, m.overlay, "el click en la top bar no debe abrir el picker")
	}
	after, _ := m.sidebar.selected()
	require.Equal(t, before.Name, after.Name, "la selección no debe cambiar")
}

// TestWheelStillWorksWithFormOpen: la rueda no cierra ni activa nada con el formulario abierto.
func TestWheelStillWorksWithFormOpen(t *testing.T) {
	m := newTestModel(sampleServices())
	m = mustUpdate(t, m, keyMsg("d"))
	wheel := tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown, X: m.sidebarW + 5, Y: 10}
	m = mustUpdate(t, m, wheel)
	require.NotNil(t, m.form)
}

// TestFormSingleColumnClickNoop: en una columna la geometría de botones no aplica;
// ningún click activa ni cierra el formulario (teclado sigue funcionando).
func TestFormSingleColumnClickNoop(t *testing.T) {
	m := newTestModel(sampleServices())
	m, _ = applySize(m, 79, 40)
	require.True(t, m.singleColumn)
	m = mustUpdate(t, m, keyMsg("d"))
	require.NotNil(t, m.form)
	updated, cmd := m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 40, Y: 25})
	m = updated.(Model)
	require.NotNil(t, m.form)
	require.Nil(t, cmd)
}
```

- [ ] **Step 2: Verificar que fallan**

Run: `go test ./internal/tui/ -run 'TestClickForm|TestClickOutside|TestWheelStill|TestFormSingle' -v`
Expected: FAIL — el click fuera hoy selecciona/abre picker y los botones del formulario no responden.

- [ ] **Step 3: Implementar en `internal/tui/app.go`**

En `handleMouse`, tras el filtro de click izquierdo y ANTES del check de top bar:

```go
	// solo procesar clicks izquierdos
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return nil
	}
	// con el formulario abierto, el mouse solo interactúa con sus botones:
	// click fuera = no-op (solo esc o Cancel cierran)
	if m.form != nil {
		return m.clickForm(msg)
	}
```

Y el método nuevo (junto a openActionKind):

```go
// clickForm resuelve un click con el formulario abierto: activa el botón bajo el
// cursor o no hace nada. La geometría (como la de los botones de Details) solo es
// válida en dos columnas; en una columna el formulario se opera por teclado.
func (m *Model) clickForm(msg tea.MouseMsg) tea.Cmd {
	if m.singleColumn {
		return nil
	}
	// fila superior del formulario: cuerpo del panel (pestañas + línea en blanco)
	// + las filas de DetailsView (0..DetailsButtonLine) → el form empieza después.
	formY0 := topBarHeight + borderTop + 2 + panel.DetailsButtonLine + 1
	row := msg.Y - formY0
	x := msg.X - (m.sidebarW + 2) // contenido del panel: divisor + PaddingLeft
	idx := m.form.buttonAt(row, x)
	if idx < 0 {
		return nil
	}
	done, result := m.form.activateIndex(idx)
	if done {
		m.form = nil
	}
	if r, ok := result.(actionConfirmedMsg); ok {
		return m.applyActionConfirmed(r)
	}
	return nil
}
```

- [ ] **Step 4: Verificar que pasan (nuevos y los 8 anclados existentes)**

Run: `go test ./internal/tui/... -count=1`
Expected: PASS completo

- [ ] **Step 5: Smoke visual temporal (borrar antes del commit)**

Crear `internal/tui/smoke_form_test.go` con un render 100×30: abrir deploy con `d`, teclear `v1.4.2`, imprimir `t.Log("\n" + m.View())` y verificar a ojo en el output del test que la caja del formulario aparece bajo los botones con el foco en Deploy. Correr con `go test ./internal/tui/ -run TestSmokeForm -v`, copiar el extracto al reporte y **borrar el archivo**.

- [ ] **Step 6: Verificación completa y commit**

```bash
rm -f internal/tui/smoke_form_test.go
gofmt -w internal/ && golangci-lint run && go vet ./... && go test ./... -count=1 && go build ./...
git add internal/tui/app.go internal/tui/app_test.go
git commit -m "feat(tui): clicks en el formulario inline; click fuera no-op"
```
