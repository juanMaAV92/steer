# UX de acciones: botones + modal — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Convertir las acciones de Details (`deploy/scale/rollback`) en botones clickeables con la tecla indicada, y mover el input/confirmación de la barra inferior a un modal centrado sobre el panel.

**Architecture:** Un helper de botones en `internal/render` (render + hit-testing por columna, compartido por `panel` y `tui`). `panel/details.go` dibuja los botones; `tui/action.go` gana un `modalView(width, height)` centrado con `lipgloss.Place`; `tui/app.go` renderiza el modal en `View()` y enruta el mouse (click en botón de Details → abre acción; click con el modal abierto → cancela).

**Tech Stack:** Go, Bubble Tea v1.3.6, Lipgloss, testify.

## Global Constraints

- `internal/core` y `internal/providers` NO cambian.
- Reusar `internal/render`; UI strings en inglés, comentarios en español.
- gofmt-clean y `go vet ./...` limpio antes de cada commit.
- `go test ./...` y `go build ./...` deben pasar antes de cada commit.
- NO añadir autoría de Claude a los commits (sin `Co-Authored-By`, sin "Generated with Claude Code").
- Acento de marca cian (`render.BrandColor`); verde solo para estado.
- El flujo de deploy en vivo (pestaña Events) NO cambia. El teclado del modal (teclear/enter/esc) NO cambia respecto a hoy.
- Anchos siempre por **runas** (columnas de celda), nunca bytes — etiquetas como `Cancel (esc)` son ASCII, pero `(↵)` es multibyte.

## File Structure

```
internal/render/buttons.go       ← (NUEVO) Buttons(labels) + ButtonAtColumn(labels, x)
internal/render/buttons_test.go
internal/tui/panel/details.go    ← botones en vez de texto; var DetailsActionLabels
internal/tui/panel/details_test.go
internal/tui/action.go           ← modalView(width,height) reemplaza view()
internal/tui/action_test.go
internal/tui/app.go              ← View() modal centrado; handleMouse (botones Details + modal)
internal/tui/app_test.go
```

---

### Task 1: Helper de botones en `internal/render`

**Files:**
- Create: `internal/render/buttons.go`
- Test: `internal/render/buttons_test.go`

**Interfaces:**
- Produces:
  - `func Buttons(labels []string) string` — fila `"[ l0 ]  [ l1 ]  …"`, cada botón con el texto en cian de marca, separados por 2 espacios.
  - `func ButtonAtColumn(labels []string, x int) int` — índice del botón cuya caja `"[ label ]"` cubre la columna `x` (relativa al inicio de la fila), o `-1`. Ancho de cada botón = `utf8.RuneCountInString(label) + 4` (`"[ "` + label + `" ]"`); separador de 2 columnas entre botones.

- [ ] **Step 1: Write the failing test**

```go
// internal/render/buttons_test.go
package render

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestButtonsContainsLabels(t *testing.T) {
	out := Buttons([]string{"Deploy (d)", "Scale (s)"})
	require.Contains(t, out, "Deploy (d)")
	require.Contains(t, out, "Scale (s)")
	require.Contains(t, out, "[")
	require.Contains(t, out, "]")
}

func TestButtonAtColumn(t *testing.T) {
	labels := []string{"Deploy (d)", "Scale (s)", "Rollback (R)"}
	// anchos: "Deploy (d)"=10 -> caja 14 (cols 0..13); sep 2 (14,15);
	// "Scale (s)"=9 -> caja 13 (cols 16..28); sep 2 (29,30);
	// "Rollback (R)"=12 -> caja 16 (cols 31..46)
	require.Equal(t, 0, ButtonAtColumn(labels, 0))
	require.Equal(t, 0, ButtonAtColumn(labels, 13))
	require.Equal(t, -1, ButtonAtColumn(labels, 14)) // separador
	require.Equal(t, 1, ButtonAtColumn(labels, 16))
	require.Equal(t, 1, ButtonAtColumn(labels, 28))
	require.Equal(t, 2, ButtonAtColumn(labels, 31))
	require.Equal(t, 2, ButtonAtColumn(labels, 46))
	require.Equal(t, -1, ButtonAtColumn(labels, 47)) // fuera
	require.Equal(t, -1, ButtonAtColumn(labels, -1))
}

func TestButtonAtColumnMultibyte(t *testing.T) {
	// "Deploy (↵)" tiene una runa multibyte; el ancho debe contarse por runas (10), caja 14.
	labels := []string{"Deploy (↵)", "Cancel (esc)"}
	require.Equal(t, 0, ButtonAtColumn(labels, 0))
	require.Equal(t, 0, ButtonAtColumn(labels, 13))
	require.Equal(t, 1, ButtonAtColumn(labels, 16))
	_ = strings.TrimSpace
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/render/ -run 'TestButton'`
Expected: FAIL (`Buttons`/`ButtonAtColumn` no definidos).

- [ ] **Step 3: Implement**

```go
// internal/render/buttons.go
package render

import (
	"strings"
	"unicode/utf8"
)

const buttonGap = 2 // columnas entre botones

// boxWidth es el ancho en columnas de la caja "[ label ]".
func boxWidth(label string) int { return utf8.RuneCountInString(label) + 4 }

// Buttons renderiza una fila de botones "[ label ]" en cian de marca, separados por
// buttonGap espacios. Las etiquetas se muestran tal cual (ASCII o con glifos).
func Buttons(labels []string) string {
	parts := make([]string, 0, len(labels))
	for _, l := range labels {
		parts = append(parts, Accent("[ "+l+" ]"))
	}
	return strings.Join(parts, strings.Repeat(" ", buttonGap))
}

// ButtonAtColumn devuelve el índice del botón cuya caja cubre la columna x (relativa al
// inicio de la fila), o -1 si x cae en un separador o fuera de rango.
func ButtonAtColumn(labels []string, x int) int {
	col := 0
	for i, l := range labels {
		w := boxWidth(l)
		if x >= col && x < col+w {
			return i
		}
		col += w + buttonGap
	}
	return -1
}
```

- [ ] **Step 4: Run to verify pass**

Run: `gofmt -w internal/render/ && go test ./internal/render/... && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/render/buttons.go internal/render/buttons_test.go
git commit -m "feat(render): helper de botones (render + hit-testing por columna)"
```

---

### Task 2: Botones en el panel Details

**Files:**
- Modify: `internal/tui/panel/details.go`
- Modify: `internal/tui/panel/details_test.go`

**Interfaces:**
- Consumes: `render.Buttons`.
- Produces:
  - `var DetailsActionLabels = []string{"Deploy (d)", "Scale (s)", "Rollback (R)"}` (exportada; fuente única usada por el render y por el hit-testing en app.go).
  - `DetailsView` renderiza, cuando `writable`, `render.Buttons(DetailsActionLabels)`; en read-only, el aviso actual.

- [ ] **Step 1: Update the tests**

Reemplaza en `details_test.go` la aserción de la fila de acciones por la nueva (los tests existentes que afirmaban `"deploy"`/`"scale"`/`"rollback"` siguen valiendo porque las etiquetas los contienen; añade verificación de la tecla y el formato de botón):

```go
func TestDetailsViewShowsButtonsWithKeys(t *testing.T) {
	s := core.ServiceStatus{Name: "api", Running: 2, Desired: 2, Status: "ACTIVE", Tag: "v1.4"}
	out := DetailsView(s, true, "api")
	require.Contains(t, out, "Deploy (d)")
	require.Contains(t, out, "Scale (s)")
	require.Contains(t, out, "Rollback (R)")
	require.Contains(t, out, "[") // estilo de botón
}

func TestDetailsViewReadOnlyHasNoButtons(t *testing.T) {
	s := core.ServiceStatus{Name: "api", Running: 1, Desired: 1}
	out := DetailsView(s, false, "api")
	require.Contains(t, strings.ToLower(out), "read-only")
	require.NotContains(t, out, "Deploy (d)")
}
```

(Si existe un test previo como `TestDetailsViewShowsStatsAndActions` que afirme el texto viejo `"[d] deploy"`, actualízalo a las nuevas etiquetas o elimínalo si queda cubierto por los de arriba.)

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/tui/panel/ -run TestDetailsView`
Expected: FAIL (aún se renderiza el texto viejo).

- [ ] **Step 3: Implement**

En `internal/tui/panel/details.go`, añade la variable exportada y reemplaza la fila de acciones:

```go
// DetailsActionLabels son las etiquetas de los botones de acción del panel Details.
// Fuente única: las usa DetailsView para renderizar y app.go para el hit-testing del click.
var DetailsActionLabels = []string{"Deploy (d)", "Scale (s)", "Rollback (R)"}
```

```go
	// ...tras "tag       ..." + "\n\n":
	if writable {
		b.WriteString(render.Buttons(DetailsActionLabels))
	} else {
		b.WriteString(render.Warn("read-only environment — actions disabled"))
	}
```

- [ ] **Step 4: Run to verify pass**

Run: `gofmt -w internal/tui/panel/ && go test ./internal/tui/panel/... && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/panel/details.go internal/tui/panel/details_test.go
git commit -m "feat(tui): botones de acción con tecla indicada en el panel Details"
```

---

### Task 3: Modal centrado en `action.go`

**Files:**
- Modify: `internal/tui/action.go`
- Modify: `internal/tui/action_test.go`

**Interfaces:**
- Consumes: `render.Buttons`, `render.Bold`, `render.Accent`, `render.Dim`, `lipgloss`.
- Produces:
  - `func (a action) modalView(width, height int) string` — caja centrada con `lipgloss.Place` sobre un área `width × height`. Reemplaza a `view()`.
  - (Se elimina `view()`.)

- [ ] **Step 1: Update the tests**

Reemplaza en `action_test.go` cualquier test de `view()` por uno de `modalView`:

```go
func TestActionModalDeploy(t *testing.T) {
	var a action
	a.open(actionDeploy, "api")
	a.typeKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v2")})
	out := a.modalView(80, 24)
	require.Contains(t, out, "Deploy")
	require.Contains(t, out, "api")
	require.Contains(t, out, "v2")
	require.Contains(t, out, "Cancel (esc)")
}

func TestActionModalRollback(t *testing.T) {
	var a action
	a.open(actionRollback, "api")
	out := a.modalView(80, 24)
	require.Contains(t, strings.ToLower(out), "roll back")
	require.Contains(t, out, "Confirm (↵)")
	require.Contains(t, out, "Cancel (esc)")
}
```

Añade los imports `"strings"` y `tea "github.com/charmbracelet/bubbletea"` al test si faltan.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/tui/ -run TestActionModal`
Expected: FAIL (`modalView` no existe).

- [ ] **Step 3: Implement**

En `internal/tui/action.go`, añade el import de `lipgloss` y reemplaza `view()` por `modalView`:

```go
import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/juanMaAV92/steer/internal/render"
)
```

```go
// modalView renderiza el diálogo de acción como una caja centrada en un área width×height.
func (a action) modalView(width, height int) string {
	var title, body, confirm string
	switch a.kind {
	case actionDeploy:
		title = "Deploy " + a.service
		body = "image tag:  " + render.Accent(a.input) + "_"
		confirm = "Deploy (↵)"
	case actionScale:
		title = "Scale " + a.service
		body = "desired count:  " + render.Accent(a.input) + "_"
		confirm = "Scale (↵)"
	case actionRollback:
		title = "Roll back " + a.service + "?"
		body = render.Dim("This reverts to the previous revision.")
		confirm = "Confirm (↵)"
	}
	inner := render.Bold(title) + "\n\n" + body + "\n\n" +
		render.Buttons([]string{confirm, "Cancel (esc)"})
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(render.BrandColor)).
		Padding(1, 2).
		Render(inner)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, box)
}
```

Elimina la función `view()` anterior.

- [ ] **Step 4: Run to verify pass**

Run: `gofmt -w internal/tui/ && go test ./internal/tui/ -run TestAction && go build ./...`
Expected: PASS. (Si `app.go` aún llama a `m.action.view()`, fallará el build — se arregla en la Task 4. Para mantener esta task aislada, en este paso cambia esa única línea en `app.go` a `m.action.modalView(m.width, m.bodyH)` provisionalmente; la Task 4 reescribe el render del modal correctamente. Alternativamente, ejecuta Task 3 y Task 4 juntas si el build no pasa aislado.)

- [ ] **Step 5: Commit**

```bash
git add internal/tui/action.go internal/tui/action_test.go internal/tui/app.go
git commit -m "feat(tui): render del modal de acción centrado (reemplaza barra inferior)"
```

---

### Task 4: Wiring en `app.go` — modal centrado y mouse

**Files:**
- Modify: `internal/tui/app.go`
- Modify: `internal/tui/app_test.go`

**Interfaces:**
- Consumes: `m.action.modalView`, `render.ButtonAtColumn`, `panel.DetailsActionLabels`, `panel.TabDetails`.
- Produces: `View()` dibuja el modal centrado cuando `m.focus == focusAction`; `handleMouse` abre la acción al click en la fila de botones de Details y cancela el modal con cualquier click.

- [ ] **Step 1: Write the failing tests**

```go
// añadir a internal/tui/app_test.go
import "unicode/utf8" // si falta

// Click en el botón [ Deploy (d) ] de Details abre el modal de deploy. Anclado al render.
func TestClickDetailsDeployButton(t *testing.T) {
	m := newTestModel(sampleServices()) // foco sidebar, tab Details, writable
	out := m.View()
	clickX, clickY := -1, -1
	for y, line := range strings.Split(out, "\n") {
		clean := stripANSI(line)
		if i := strings.Index(clean, "Deploy (d)"); i >= 0 {
			clickX = utf8.RuneCountInString(clean[:i]) + 1 // dentro de "[ Deploy..."
			clickY = y
			break
		}
	}
	require.GreaterOrEqual(t, clickX, 0, "no se encontró el botón Deploy en el render")
	click := tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: clickX, Y: clickY}
	m = mustUpdate(t, m, click)
	require.Equal(t, focusAction, m.focus)
	require.Equal(t, actionDeploy, m.action.kind)
}

// Con el modal abierto, cualquier click lo cancela.
func TestClickCancelsActionModal(t *testing.T) {
	m := newTestModel(sampleServices())
	m = mustUpdate(t, m, keyMsg("d")) // abre deploy
	require.Equal(t, focusAction, m.focus)
	click := tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 1, Y: 1}
	m = mustUpdate(t, m, click)
	require.NotEqual(t, focusAction, m.focus) // cerrado
	require.False(t, m.action.active)
}

// Read-only: click en la fila de botones no abre acción.
func TestReadOnlyDetailsButtonsNoOp(t *testing.T) {
	fake := &coretest.FakeDeployer{Services: sampleServices()}
	factory := func(_ config.Context) (core.Deployer, error) { return fake, nil }
	cur := config.Context{Name: "prod", Cloud: "aws", Cluster: "c", Writable: false}
	m := New(factory, []config.Context{cur}, cur)
	m.sidebar.setServices(sampleServices())
	m, _ = applySize(m, 120, 40)
	// click donde estaría la fila de botones; en read-only no hay botones → no-op
	click := tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: m.sidebarW + 5, Y: 11}
	m = mustUpdate(t, m, click)
	require.NotEqual(t, focusAction, m.focus)
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/tui/ -run 'TestClickDetails|TestClickCancelsAction|TestReadOnlyDetailsButtons'`
Expected: FAIL (el click no abre acción / el modal no se cancela con click).

- [ ] **Step 3: Implement**

(3a) `View()` — sustituye la rama de la barra inferior por el modal centrado. Localiza:

```go
	bottom := bottomBar(m.keys.shortHelp(), m.notice, m.status)
	if m.focus == focusAction {
		bottom = m.action.view()
	}
	return top + "\n" + body + "\n" + bottom
```

y reemplázala por: render del modal centrado (overlay que reemplaza el cuerpo, como el picker):

```go
	if m.focus == focusAction {
		return top + "\n" + m.action.modalView(m.width, m.bodyH) + "\n" +
			bottomBar(m.keys.shortHelp(), m.notice, m.status)
	}
	bottom := bottomBar(m.keys.shortHelp(), m.notice, m.status)
	return top + "\n" + body + "\n" + bottom
```

(Coloca el bloque `if m.focus == focusAction` justo después de construir `body`, antes del `return` normal. El `body` no se usa en la rama del modal — está bien, Go no se queja porque `body` ya se usó al construirse… si el compilador marca `body` sin uso en esa ruta, mueve la construcción de `body` dentro del `else`.)

(3b) `handleMouse` — añade, ANTES de la lógica de rueda/zonas, la captura del modal; y dentro de la zona del panel, el click en la fila de botones de Details. Reemplaza el cuerpo de `handleMouse` para que empiece así:

```go
func (m *Model) handleMouse(msg tea.MouseMsg) tea.Cmd {
	// con el modal de acción abierto, cualquier click lo cancela (captura el mouse)
	if m.focus == focusAction {
		if msg.Action == tea.MouseActionPress {
			m.action.close()
			m.focus = focusSidebar
		}
		return nil
	}
	// ... (resto existente: rueda, filtro press/left, top bar, sidebar, panel) ...
```

Y dentro de la zona del panel, cuando es la fila de botones de Details, abre la acción. Localiza el bloque de la zona del panel (tras `// click en la zona del panel`) y, antes del cálculo de pestañas, añade:

```go
	// click en la fila de botones de acción del panel Details
	const detailsButtonRowY = 11 // topBar(1)+borde(1)+tabs(1)+blanco(1)+details: name,blank,4 stats,blank,botones
	if m.current.Writable && m.tabs.Active == panel.TabDetails && msg.Y == detailsButtonRowY {
		localX := msg.X - (m.sidebarW + 3)
		if idx := render.ButtonAtColumn(panel.DetailsActionLabels, localX); idx >= 0 {
			m.openActionKind(actionKindFor(idx))
			return nil
		}
	}
```

Añade los helpers en `app.go`:

```go
// actionKindFor mapea el índice del botón de Details a su actionKind.
func actionKindFor(idx int) actionKind {
	switch idx {
	case 1:
		return actionScale
	case 2:
		return actionRollback
	default:
		return actionDeploy
	}
}

// openActionKind abre el modal para una acción (usado por el click en botones de Details).
func (m *Model) openActionKind(kind actionKind) {
	s, ok := m.sidebar.selected()
	if !ok {
		return
	}
	m.action.open(kind, s.Name)
	m.notice = ""
	m.focus = focusAction
}
```

> **Calibración:** `detailsButtonRowY = 11` es la fila esperada con el layout actual de
> `DetailsView`. El test `TestClickDetailsDeployButton` está anclado al render (deriva la Y
> real del texto "Deploy (d)"). Si falla por la Y, ajusta la constante al valor que reporte
> el render y vuelve a correr (RED→GREEN). Documenta el valor final en el reporte.

Asegúrate de que `render` y `panel` están importados en `app.go` (lo están).

- [ ] **Step 4: Run to verify pass**

Run: `gofmt -w internal/tui/ && go vet ./internal/tui/... && go test ./internal/tui/... && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/app.go internal/tui/app_test.go
git commit -m "feat(tui): modal de acción centrado y click en botones de Details"
```

---

## Nota de verificación final (no es una tarea)

Smoke manual recomendado: `go run ./cmd/steerdemo` (1 contexto) o `go run ./cmd/steer tui`.
Verificar: click en `[ Deploy (d) ]` abre el modal centrado; teclear tag + enter despliega;
esc o click fuera cancela; `d`/`s`/`R` siguen funcionando; en un contexto read-only los
botones aparecen apagados y no responden.
