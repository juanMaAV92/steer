# Refactor de TUI (overlays, secciones, geometría) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ejecutar el refactor de TUI del diseño `2026-07-04-refactor-tui-diseno.md` — abstracción `overlay`, sidebar por secciones, hit-testing unificado, geometría derivada, keyMap completo, `providers.IsImplemented` — más los cosméticos diferidos del ledger. Cero features nuevas; comportamiento observable idéntico.

**Architecture:** Primero las primitivas independientes (T1–T5: `render.LabelAtColumn`, `panel.DetailsButtonLine`, keyMap, `IsImplemented`, `sidebar.HitAtRow`), luego la migración grande (T6: `overlay` interface que absorbe `focusAction`/`focusContextPicker` con resultados tipados), y al final la ola cosmética (T7). Cada tarea deja la suite verde.

**Tech Stack:** Go 1.26, Bubble Tea, Lipgloss, testify, golangci-lint.

## Global Constraints

- Comportamiento observable idéntico: mismas teclas, mismos clicks, mismos flujos (deploy en vivo, switch de contexto). Los tests anclados al render existentes deben seguir pasando SIN debilitarse (si uno cambia, es para reflejar la nueva forma de consultar estado, no para relajar aserciones).
- `render` sigue siendo hoja (sin imports de steer); `core` stdlib-only.
- Comentarios en español, UI strings en inglés; sin autoría de Claude en commits.
- Antes de cada commit: `gofmt -w internal/ && golangci-lint run && go vet ./... && go test ./... -count=1 && go build ./...` — todo verde.
- Fuera de alcance: paleta ⌘k, filas reales en IMAGES/DATABASES, config anidada.

## File Structure

```
internal/render/buttons.go        ← LabelAtColumn (primitiva); ButtonAtColumn delega
internal/tui/panel/tabs.go        ← TabAtColumn delega en render.LabelAtColumn
internal/tui/panel/details.go     ← + DetailsButtonLine (const exportada, autovalidada)
internal/tui/keys.go              ← + Left, Right, Context
internal/providers/factory.go     ← + IsImplemented; la fábrica lo usa
internal/tui/contextpicker.go     ← usa providers.IsImplemented
internal/tui/sidebar.go           ← + sidebarHit/HitAtRow + constantes de sección
internal/tui/overlay.go           ← (NUEVO) interface overlay + pickerOverlay + actionOverlay
internal/tui/app.go               ← Model.overlay; routing único; handleOverlayResult;
                                     enum de foco reducido; geometría derivada
internal/providers/aws/provider.go ← cfgCtx (cosmético)
internal/config/config.go         ← límite de palabra legacy (cosmético)
internal/render/rollout_test.go   ← assert duplicado (cosmético)
internal/cli/service_cmd.go       ← orden del guard -y (cosmético)
```

---

### Task 1: `render.LabelAtColumn` — primitiva única de hit-testing por columna

**Files:**
- Modify: `internal/render/buttons.go`
- Modify: `internal/tui/panel/tabs.go` (`TabAtColumn`)
- Test: `internal/render/buttons_test.go`, `internal/tui/panel/tabs_test.go`

**Interfaces:**
- Produces: `func LabelAtColumn(labels []string, pad, gap, x int) int` — índice de la etiqueta cuya franja `[col, col+runas(label)+pad)` cubre `x`, separadas por `gap` columnas; `-1` si cae en separador o fuera. `ButtonAtColumn(labels, x) == LabelAtColumn(labels, 4, 2, x)` (compatible). `Tabs.TabAtColumn(x)` delega con `pad=0, gap=2` (y pasa a contar runas, no bytes).

- [ ] **Step 1: Write the failing test**

```go
// añadir a internal/render/buttons_test.go
func TestLabelAtColumn(t *testing.T) {
	labels := []string{"Details", "Events", "Logs"}
	// pad=0, gap=2: Details(0-6) sep(7-8) Events(9-14) sep(15-16) Logs(17-20)
	require.Equal(t, 0, LabelAtColumn(labels, 0, 2, 0))
	require.Equal(t, 0, LabelAtColumn(labels, 0, 2, 6))
	require.Equal(t, -1, LabelAtColumn(labels, 0, 2, 7))
	require.Equal(t, 1, LabelAtColumn(labels, 0, 2, 9))
	require.Equal(t, 2, LabelAtColumn(labels, 0, 2, 20))
	require.Equal(t, -1, LabelAtColumn(labels, 0, 2, 21))
	require.Equal(t, -1, LabelAtColumn(labels, 0, 2, -1))
	// equivalencia con ButtonAtColumn (pad=4)
	bl := []string{"Deploy (d)", "Scale (s)"}
	for x := -1; x < 35; x++ {
		require.Equal(t, ButtonAtColumn(bl, x), LabelAtColumn(bl, 4, 2, x), "x=%d", x)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/render/ -run TestLabelAtColumn`
Expected: FAIL (`LabelAtColumn` no definida).

- [ ] **Step 3: Implement**

En `internal/render/buttons.go`:

```go
// LabelAtColumn es la primitiva de hit-testing por columna: devuelve el índice de la
// etiqueta cuya franja (ancho en runas + pad) cubre la columna x, con gap columnas
// entre etiquetas; -1 si x cae en un separador o fuera de rango.
func LabelAtColumn(labels []string, pad, gap, x int) int {
	col := 0
	for i, l := range labels {
		w := utf8.RuneCountInString(l) + pad
		if x >= col && x < col+w {
			return i
		}
		col += w + gap
	}
	return -1
}

// ButtonAtColumn devuelve el índice del botón "[ label ]" que cubre la columna x.
func ButtonAtColumn(labels []string, x int) int {
	return LabelAtColumn(labels, 4, buttonGap, x)
}
```

(elimina `boxWidth` si queda sin uso; `Buttons` no cambia).

En `internal/tui/panel/tabs.go`, reemplaza `TabAtColumn`:

```go
// TabAtColumn devuelve la pestaña cuya etiqueta cubre la columna x (relativa al inicio
// del contenido del panel), o -1. Delega en render.LabelAtColumn (ancho por runas).
func (tb Tabs) TabAtColumn(x int) int {
	labels := make([]string, tb.Count())
	for i := range labels {
		labels[i] = Tab(i).String()
	}
	return render.LabelAtColumn(labels, 0, 2, x)
}
```

- [ ] **Step 4: Run to verify pass**

Run: `gofmt -w internal/ && go test ./internal/render/... ./internal/tui/... -count=1 && go build ./...`
Expected: PASS (incl. `TestTabAtColumn` y los tests de click anclados al render, sin cambios).

- [ ] **Step 5: Commit**

```bash
git add internal/render/ internal/tui/panel/
git commit -m "refactor(render): LabelAtColumn como primitiva única de hit-testing"
```

---

### Task 2: `panel.DetailsButtonLine` — geometría derivada del layout

**Files:**
- Modify: `internal/tui/panel/details.go`, `internal/tui/app.go` (~línea 301)
- Test: `internal/tui/panel/details_test.go`

**Interfaces:**
- Produces: `const DetailsButtonLine = 7` en `panel` (línea, 0-based, de la fila de botones dentro del output de `DetailsView`), autovalidada por test. En `app.go` la constante local `detailsButtonRowY = 11` se reemplaza por el cómputo `topBarHeight + borderTop + 1 /*tabs*/ + 1 /*blanco*/ + panel.DetailsButtonLine`.

- [ ] **Step 1: Write the failing test (autovalidación de la constante)**

```go
// añadir a internal/tui/panel/details_test.go
// DetailsButtonLine debe apuntar a la línea real de los botones en el render.
func TestDetailsButtonLineMatchesRender(t *testing.T) {
	s := core.ServiceStatus{Name: "api", Running: 2, Desired: 2, Status: "ACTIVE", Tag: "v1"}
	lines := strings.Split(DetailsView(s, true, "api"), "\n")
	require.Greater(t, len(lines), DetailsButtonLine)
	require.Contains(t, lines[DetailsButtonLine], "Deploy (d)")
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/tui/panel/ -run TestDetailsButtonLine`
Expected: FAIL (constante no definida).

- [ ] **Step 3: Implement**

En `details.go` (junto a `DetailsActionLabels`):

```go
// DetailsButtonLine es la línea (0-based) de la fila de botones dentro del output de
// DetailsView: name(0), blank(1), running(2), pending(3), status(4), tag(5), blank(6), botones(7).
// El test TestDetailsButtonLineMatchesRender la valida contra el render real.
const DetailsButtonLine = 7
```

En `app.go`, reemplaza la constante mágica (~301):

```go
	// fila de botones de Details en pantalla, derivada del layout real del panel
	detailsButtonRowY := topBarHeight + borderTop + 1 /*pestañas*/ + 1 /*línea en blanco*/ + panel.DetailsButtonLine
	if !m.singleColumn && m.current.Writable && m.tabs.Active == panel.TabDetails && msg.Y == detailsButtonRowY {
```

- [ ] **Step 4: Run to verify pass**

Run: `gofmt -w internal/tui/ && go test ./internal/tui/... -count=1 && go build ./...`
Expected: PASS (los tests de click de botones anclados al render siguen verdes — son el guard de que 11 == cómputo).

- [ ] **Step 5: Commit**

```bash
git add internal/tui/
git commit -m "refactor(tui): fila de botones de Details derivada del layout"
```

---

### Task 3: keyMap completo — Left/Right/Context vía key.Matches

**Files:**
- Modify: `internal/tui/keys.go`, `internal/tui/app.go` (casos `msg.String()` en handleKey ~387 y ~406-409)
- Test: `internal/tui/keys_test.go`

**Interfaces:**
- Produces: `keyMap` gana `Left` (`left`,`h`), `Right` (`right`,`l`), `Context` (`c`); `handleKey` usa `key.Matches(msg, m.keys.X)` para abrir el picker y navegar pestañas. Sin cambios de comportamiento.

- [ ] **Step 1: Write the failing test**

```go
// añadir a internal/tui/keys_test.go
func TestNavAndContextKeysBound(t *testing.T) {
	k := defaultKeys()
	require.True(t, key.Matches(keyMsg("l"), k.Right))
	require.True(t, key.Matches(keyMsg("h"), k.Left))
	require.True(t, key.Matches(keyMsg("c"), k.Context))
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/tui/ -run TestNavAndContextKeys`
Expected: FAIL (campos no definidos).

- [ ] **Step 3: Implement**

`keys.go`: añade al struct `Left, Right, Context key.Binding` y en `defaultKeys()`:

```go
		Left:    key.NewBinding(key.WithKeys("left", "h")),
		Right:   key.NewBinding(key.WithKeys("right", "l")),
		Context: key.NewBinding(key.WithKeys("c")),
```

`app.go`: reemplaza `case msg.String() == "c":` por `case key.Matches(msg, m.keys.Context):`, y el switch de pestañas:

```go
		switch {
		case key.Matches(msg, m.keys.Right):
			m.tabs.Next()
		case key.Matches(msg, m.keys.Left):
			m.tabs.Prev()
		}
```

- [ ] **Step 4: Run to verify pass**

Run: `gofmt -w internal/tui/ && go test ./internal/tui/... -count=1 && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/
git commit -m "refactor(tui): Left/Right/Context al keyMap (fuera msg.String crudo)"
```

---

### Task 4: `providers.IsImplemented` — fuente única de clouds soportados

**Files:**
- Modify: `internal/providers/factory.go`, `internal/tui/contextpicker.go` (~106)
- Test: `internal/providers/factory_test.go`

**Interfaces:**
- Produces: `func IsImplemented(cloud string) bool`; la fábrica la usa (`if !IsImplemented(...) → ErrProviderNotImplemented`); el picker marca `(no impl.)` con ella.

- [ ] **Step 1: Write the failing test**

```go
// añadir a internal/providers/factory_test.go
func TestIsImplemented(t *testing.T) {
	require.True(t, IsImplemented("aws"))
	require.False(t, IsImplemented("gcp"))
	require.False(t, IsImplemented("azure"))
	require.False(t, IsImplemented(""))
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/providers/ -run TestIsImplemented`
Expected: FAIL.

- [ ] **Step 3: Implement**

`factory.go`:

```go
// IsImplemented indica si un cloud tiene provider real. Fuente única: la fábrica
// y la UI (marca "(no impl.)") deben coincidir siempre.
func IsImplemented(cloud string) bool { return cloud == "aws" }
```

Y en `NewProviderFactory`, reemplaza el switch por:

```go
	return func(ctx context.Context, c config.Context) (Provider, error) {
		if !IsImplemented(c.Cloud) {
			return nil, fmt.Errorf("%w: %q", ErrProviderNotImplemented, c.Cloud)
		}
		return aws.NewProvider(ctx, c)
	}
```

`contextpicker.go` (~106): `if c.Cloud != "aws" {` → `if !providers.IsImplemented(c.Cloud) {` (añade el import de `providers`).

- [ ] **Step 4: Run to verify pass**

Run: `gofmt -w internal/ && go test ./... -count=1 && go build ./...`
Expected: PASS (los tests de picker/switch no cambian de comportamiento).

- [ ] **Step 5: Commit**

```bash
git add internal/providers/ internal/tui/
git commit -m "refactor(providers): IsImplemented como fuente única de clouds soportados"
```

---

### Task 5: sidebar por secciones — `HitAtRow`

**Files:**
- Modify: `internal/tui/sidebar.go`, `internal/tui/app.go` (zona de click del sidebar, ~296)
- Test: `internal/tui/sidebar_test.go`

**Interfaces:**
- Produces:

```go
type sidebarSection int

const (
	sectionServices sidebarSection = iota
	sectionImages
	sectionDatabases
)

type sidebarHit struct {
	Section sidebarSection
	Index   int // índice dentro de la sección (solo services tiene filas hoy)
}

// HitAtRow mapea una fila (relativa al inicio del contenido del sidebar, donde la
// fila 0 es el header "SERVICES (n)") a la sección/índice que se renderiza ahí.
// ok=false para headers, líneas en blanco, stubs "(próximamente)" o fuera de rango.
func (s sidebar) HitAtRow(row int) (sidebarHit, bool)
```

  - Replica la estructura de `view()`: fila 0 header SERVICES; filas 1..n los servicios; luego blanco, header IMAGES, stub, blanco, header DATABASES, stub — nada de eso es accionable (ok=false), pero el mapeo ya distingue secciones para el futuro.
  - `serviceRowCount()` se elimina; `handleMouse` usa `HitAtRow` y solo actúa si `hit.Section == sectionServices`.

- [ ] **Step 1: Write the failing test**

```go
// añadir a internal/tui/sidebar_test.go
func TestHitAtRow(t *testing.T) {
	s := newSidebar()
	s.setServices(sampleServices()) // 4 servicios ordenados
	// fila 0 = header SERVICES → no accionable
	_, ok := s.HitAtRow(0)
	require.False(t, ok)
	// filas 1..4 = servicios 0..3
	hit, ok := s.HitAtRow(1)
	require.True(t, ok)
	require.Equal(t, sectionServices, hit.Section)
	require.Equal(t, 0, hit.Index)
	hit, ok = s.HitAtRow(4)
	require.True(t, ok)
	require.Equal(t, 3, hit.Index)
	// fila 5 = línea en blanco antes de IMAGES → no accionable
	_, ok = s.HitAtRow(5)
	require.False(t, ok)
	// headers/stubs de IMAGES/DATABASES → no accionables
	_, ok = s.HitAtRow(6)
	require.False(t, ok)
	// fuera de rango
	_, ok = s.HitAtRow(-1)
	require.False(t, ok)
	_, ok = s.HitAtRow(99)
	require.False(t, ok)
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/tui/ -run TestHitAtRow`
Expected: FAIL.

- [ ] **Step 3: Implement**

En `sidebar.go` (reemplaza `serviceRowCount`):

```go
// HitAtRow replica la estructura de view(): header SERVICES (fila 0), un servicio por
// fila, y después solo headers/stubs no accionables. Cuando IMAGES/DATABASES tengan
// filas reales, este mapeo crece con ellas (mismo patrón que contextPicker.indexAtLine).
func (s sidebar) HitAtRow(row int) (sidebarHit, bool) {
	if row < 1 { // fila 0: header "SERVICES (n)"
		return sidebarHit{}, false
	}
	if idx := row - 1; idx < len(s.services) {
		return sidebarHit{Section: sectionServices, Index: idx}, true
	}
	// blanco, IMAGES header/stub, blanco, DATABASES header/stub: no accionables
	return sidebarHit{}, false
}
```

En `app.go`, la zona de click del sidebar pasa de la aritmética con `serviceRowCount` a:

```go
	if msg.X <= m.sidebarW {
		row := msg.Y - (topBarHeight + borderTop)
		if hit, ok := m.sidebar.HitAtRow(row); ok && hit.Section == sectionServices {
			m.sidebar.selectIndex(hit.Index)
			m.focus = focusSidebar
		}
		return nil
	}
```

> Nota: hoy el cómputo es `row := msg.Y - (topBarHeight + borderTop + sidebarHeader)` y
> compara contra filas de servicio directamente. Con `HitAtRow` el header es la fila 0 del
> contenido, así que `sidebarHeader` sale de la resta (lo absorbe el mapeo). El test
> anclado al render `TestMouseClickSelectsSidebarService` es el guard: debe seguir verde.
> Si `sidebarHeader` queda sin uso en app.go, elimínala del bloque de constantes.

- [ ] **Step 4: Run to verify pass**

Run: `gofmt -w internal/tui/ && go test ./internal/tui/... -count=1 && go build ./...`
Expected: PASS, incluido `TestMouseClickSelectsSidebarService` sin cambios.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/
git commit -m "refactor(tui): sidebar con hit-testing por secciones (HitAtRow)"
```

---

### Task 6: abstracción `overlay` (absorbe focusAction y focusContextPicker)

La tarea grande. Los structs `contextPicker` y `action` se conservan como núcleo; se
envuelven en dos implementadores de `overlay` y el Model pasa a tener un solo campo
`overlay` con routing único y **resultados tipados**.

**Files:**
- Create: `internal/tui/overlay.go`
- Modify: `internal/tui/app.go` (Model, Update, handleKey, handleMouse, View, applyContextSwitch, runActionCmd, openAction/openActionKind)
- Modify: `internal/tui/app_test.go` (sweep de aserciones de foco/picker/action)
- Test: `internal/tui/overlay_test.go`

**Interfaces:**
- Produces:

```go
// internal/tui/overlay.go
// overlay es una capa modal que captura teclado y mouse mientras está activa.
type overlay interface {
	Update(msg tea.Msg) (done bool, result tea.Msg)
	View(width, height int) string
}

// resultados tipados que el Model ejecuta al cerrarse un overlay
type contextChosenMsg struct{ ctx config.Context }
type actionConfirmedMsg struct {
	kind    actionKind
	service string
	input   string
}

func newPickerOverlay(keys keyMap, contexts []config.Context, current string) *pickerOverlay
func newActionOverlay(keys keyMap, kind actionKind, service string) *actionOverlay
```

- En `Model`: `overlay overlay` reemplaza a `picker contextPicker` y `action action`; el enum `focus` queda `focusSidebar | focusPanel` (se BORRAN `focusAction` y `focusContextPicker`).
- `applyContextSwitch(sel config.Context)` recibe el contexto elegido (ya no lee el picker).
- `runActionCmd(kind actionKind, service, input string) tea.Cmd` parametrizado (ya no lee `m.action`).

- [ ] **Step 1: Write the failing tests**

```go
// internal/tui/overlay_test.go
package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/juanMaAV92/steer/internal/config"
	"github.com/stretchr/testify/require"
)

func TestPickerOverlayEnterEmitsChosenContext(t *testing.T) {
	o := newPickerOverlay(defaultKeys(), samplePickerContexts(), "nao-dev")
	// bajar a nao-prod y confirmar
	done, res := o.Update(keyMsg("j"))
	require.False(t, done)
	require.Nil(t, res)
	done, res = o.Update(keyMsg("enter"))
	require.True(t, done)
	chosen, ok := res.(contextChosenMsg)
	require.True(t, ok)
	require.Equal(t, "nao-prod", chosen.ctx.Name)
}

func TestPickerOverlayEscCloses(t *testing.T) {
	o := newPickerOverlay(defaultKeys(), samplePickerContexts(), "nao-dev")
	done, res := o.Update(keyMsg("esc"))
	require.True(t, done)
	require.Nil(t, res)
}

func TestActionOverlayTypingAndConfirm(t *testing.T) {
	o := newActionOverlay(defaultKeys(), actionDeploy, "api")
	for _, r := range "v2" {
		done, _ := o.Update(keyMsg(string(r)))
		require.False(t, done)
	}
	done, res := o.Update(keyMsg("enter"))
	require.True(t, done)
	conf, ok := res.(actionConfirmedMsg)
	require.True(t, ok)
	require.Equal(t, actionDeploy, conf.kind)
	require.Equal(t, "api", conf.service)
	require.Equal(t, "v2", conf.input)
}

func TestActionOverlayEnterWithoutInputStaysOpen(t *testing.T) {
	o := newActionOverlay(defaultKeys(), actionDeploy, "api")
	done, res := o.Update(keyMsg("enter")) // input vacío → no listo
	require.False(t, done)
	require.Nil(t, res)
}

func TestActionOverlayClickCancels(t *testing.T) {
	o := newActionOverlay(defaultKeys(), actionScale, "api")
	click := tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 1, Y: 1}
	done, res := o.Update(click)
	require.True(t, done)
	require.Nil(t, res)
}

func TestOverlayViewsRender(t *testing.T) {
	p := newPickerOverlay(defaultKeys(), samplePickerContexts(), "nao-dev")
	require.Contains(t, p.View(80, 24), "Switch context")
	a := newActionOverlay(defaultKeys(), actionRollback, "api")
	require.Contains(t, strings.ToLower(a.View(80, 24)), "roll back")
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/tui/ -run 'TestPickerOverlay|TestActionOverlay|TestOverlayViews'`
Expected: FAIL (tipos no definidos).

- [ ] **Step 3: Implement overlay.go**

```go
// internal/tui/overlay.go
package tui

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/juanMaAV92/steer/internal/config"
)

// overlay es una capa modal que captura teclado y mouse mientras está activa.
// done=true cierra el overlay; result (opcional) es un tea.Msg tipado que el
// Model ejecuta (contextChosenMsg, actionConfirmedMsg). ctrl+c NO llega aquí:
// el Model lo intercepta antes (quit global).
type overlay interface {
	Update(msg tea.Msg) (done bool, result tea.Msg)
	View(width, height int) string
}

// contextChosenMsg: el usuario eligió un contexto en el picker.
type contextChosenMsg struct{ ctx config.Context }

// actionConfirmedMsg: el usuario confirmó una acción en el modal.
type actionConfirmedMsg struct {
	kind    actionKind
	service string
	input   string
}

// ---- pickerOverlay: envuelve contextPicker ----

type pickerOverlay struct {
	keys   keyMap
	picker contextPicker
}

func newPickerOverlay(keys keyMap, contexts []config.Context, current string) *pickerOverlay {
	return &pickerOverlay{keys: keys, picker: newContextPicker(contexts, current)}
}

func (o *pickerOverlay) Update(msg tea.Msg) (bool, tea.Msg) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, o.keys.Esc):
			return true, nil
		case key.Matches(msg, o.keys.Enter):
			if sel, ok := o.picker.selected(); ok {
				return true, contextChosenMsg{ctx: sel}
			}
			return true, nil
		case key.Matches(msg, o.keys.Down):
			o.picker.moveDown()
		case key.Matches(msg, o.keys.Up):
			o.picker.moveUp()
		}
		return false, nil
	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			// la línea 0 del overlay es el título del picker (el Model resta el top bar)
			if idx, ok := o.picker.indexAtLine(msg.Y - topBarHeight); ok {
				o.picker.selectIndex(idx)
				if sel, ok := o.picker.selected(); ok {
					return true, contextChosenMsg{ctx: sel}
				}
			}
		}
		return false, nil
	}
	return false, nil
}

func (o *pickerOverlay) View(width, height int) string { return o.picker.view() }

// ---- actionOverlay: envuelve action ----

type actionOverlay struct {
	keys keyMap
	act  action
}

func newActionOverlay(keys keyMap, kind actionKind, service string) *actionOverlay {
	o := &actionOverlay{keys: keys}
	o.act.open(kind, service)
	return o
}

func (o *actionOverlay) Update(msg tea.Msg) (bool, tea.Msg) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, o.keys.Esc):
			return true, nil
		case key.Matches(msg, o.keys.Enter):
			if !o.act.ready() {
				return false, nil
			}
			return true, actionConfirmedMsg{kind: o.act.kind, service: o.act.service, input: o.act.input}
		default:
			o.act.typeKey(msg)
			return false, nil
		}
	case tea.MouseMsg:
		// cualquier click cancela el modal (comportamiento actual)
		if msg.Action == tea.MouseActionPress {
			return true, nil
		}
		return false, nil
	}
	return false, nil
}

func (o *actionOverlay) View(width, height int) string { return o.act.modalView(width, height) }
```

- [ ] **Step 4: Rewire app.go**

(4a) Model y enum:
- Borra `focusAction` y `focusContextPicker` del enum (queda `focusSidebar, focusPanel`).
- En `Model`: reemplaza `picker contextPicker` y `action action` por `overlay overlay`.

(4b) Routing único en `Update` (reemplaza los cases `tea.KeyMsg`/`tea.MouseMsg` actuales, incluida la rama `handlePickerMouse` que se ELIMINA):

```go
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
		if m.overlay != nil {
			return m.routeOverlay(msg)
		}
		return m.handleKey(msg)

	case tea.MouseMsg:
		if m.overlay != nil {
			return m.routeOverlay(msg)
		}
		return m, m.handleMouse(msg)
```

```go
// routeOverlay entrega el evento al overlay activo y ejecuta su resultado.
func (m Model) routeOverlay(msg tea.Msg) (tea.Model, tea.Cmd) {
	done, result := m.overlay.Update(msg)
	if done {
		m.overlay = nil
		m.focus = focusSidebar
	}
	if result != nil {
		return m.handleOverlayResult(result)
	}
	return m, nil
}

// handleOverlayResult ejecuta la elección hecha en un overlay.
// NOTA: m.overlay ya fue puesto a nil por routeOverlay antes de llegar aquí.
func (m Model) handleOverlayResult(res tea.Msg) (tea.Model, tea.Cmd) {
	switch r := res.(type) {
	case contextChosenMsg:
		return m.applyContextSwitch(r.ctx)
	case actionConfirmedMsg:
		if r.kind == actionDeploy {
			// flujo de deploy en vivo (idéntico al actual del handler de Enter)
			m.focus = focusPanel
			m.tabs.Active = panel.TabEvents
			m.events.Reset()
			m.deploy = deployState{Active: true, Service: r.service}
			return m, startDeployCmd(m.runCtx, m.dep, r.service, r.input)
		}
		return m, m.runActionCmd(r.kind, r.service, r.input)
	}
	return m, nil
}
```

(4c) `applyContextSwitch(sel config.Context)` — quita la lectura del picker (`m.picker.selected()`) y el no-op de mismo-contexto usa `sel.Name == m.current.Name`. El resto (factory con `m.runCtx`, notice en error, reset de sidebar/deploy, `loadServicesCmd`) queda igual.

(4d) `runActionCmd(kind actionKind, service, input string) tea.Cmd` — parametrizado; borra `m.action.close()` / `m.focus = focusSidebar` internos (el overlay ya se cerró); el cuerpo del closure es el mismo (rollback/scale; deploy sigue devolviendo el error defensivo).

(4e) Aperturas:
- Tecla `c` y click en top bar: `m.overlay = newPickerOverlay(m.keys, m.contexts, m.current.Name); m.notice = ""` (sin tocar `m.focus`).
- `openAction`/`openActionKind`: `m.overlay = newActionOverlay(m.keys, kind, s.Name)` (mismos guards de writable/selected).
- En `handleKey`, BORRA los bloques `if m.focus == focusContextPicker {...}` y `if m.focus == focusAction {...}` (el routing ya pasó por el overlay). En `handleMouse`, BORRA el guard de `focusAction` (mismo motivo).

(4f) `View()` — las dos ramas de overlay se unifican:

```go
	if m.overlay != nil {
		return top + "\n" + m.overlay.View(m.width, m.bodyH) + "\n" +
			bottomBar(m.keys.shortHelp(), m.notice, m.status)
	}
```

(4g) Sweep de tests en `app_test.go` (el compilador enumera): las aserciones cambian de
`require.Equal(t, focusContextPicker, m.focus)` → `require.NotNil(t, m.overlay)` (y
`require.IsType(t, &pickerOverlay{}, m.overlay)` donde importe el tipo); de
`require.NotEqual(t, focusAction, m.focus)` → `require.Nil(t, m.overlay)`; los accesos a
`m.picker.*` en tests de switch se sustituyen navegando el overlay
(`m.overlay.(*pickerOverlay).picker.selectIndex(i)`) o preferiblemente por interacción de
teclado/click como ya hacen los tests anclados al render. `m.action.open(...)` en
`TestRunActionCmdRejectsDeploy` pasa a llamar `m.runActionCmd(actionDeploy, "svc", "v1")`
directamente. NO debilites aserciones: los tests de click (picker row, modal cancel,
botones de Details) deben seguir verificando el mismo comportamiento observable.

- [ ] **Step 5: Run to verify pass**

Run: `gofmt -w internal/tui/ && golangci-lint run && go vet ./... && go test ./... -count=1 && go build ./...`
Expected: PASS — misma UX, un solo mecanismo de overlay.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/
git commit -m "refactor(tui): abstracción overlay con resultados tipados (picker y modal)"
```

---

### Task 7: Ola cosmética (ledger)

**Files:**
- Modify: `internal/providers/aws/provider.go`, `internal/config/config.go`, `internal/render/rollout_test.go`, `internal/tui/app_test.go`, `internal/cli/service_cmd.go`

**Interfaces:** ninguna nueva.

- [ ] **Step 1: Aplicar los cinco fixes**

1. `aws/provider.go`: renombra el campo `ctx config.Context` a `cfgCtx` (y sus usos `p.ctx.` → `p.cfgCtx.`).
2. `config/config.go` (detección legacy): límite de palabra —

```go
		ks := k.String()
		if ks == "providers" || strings.HasPrefix(ks, "providers.") {
			cfg.hasLegacyProviders = true
			break
		}
```

3. `render/rollout_test.go`: elimina la línea redundante `require.True(t, strings.Contains(Rollout("COMPLETED"), "COMPLETED"))` (y el import `strings` si queda sin uso).
4. `app_test.go` `TestRunActionCmdRejectsDeploy`: cambia `require.Error(t, msg.err)` por `require.ErrorContains(t, msg.err, "deploy must go through startDeployCmd")`.
5. `cli/service_cmd.go` (deploy RunE): mueve el guard `if yes && (service == "" || tag == "")` ANTES de `app.RequireWritable()` (primera comprobación del RunE), para que el usuario en read-only con flags faltantes vea el error de flags.

- [ ] **Step 2: Run to verify pass**

Run: `gofmt -w internal/ && golangci-lint run && go vet ./... && go test ./... -count=1 && go build ./...`
Expected: PASS. Si el test del guard `-y` afirmaba el orden anterior, ajústalo al nuevo orden.

- [ ] **Step 3: Commit**

```bash
git add internal/
git commit -m "chore: ola cosmética del ledger (cfgCtx, límite legacy, asserts, orden guard -y)"
```

---

## Verificación final (no es una tarea)

- `go test ./... -count=1 -race && golangci-lint run && go build ./...` — verde.
- Smoke manual: `go run ./cmd/steerdemo` — abrir picker (`c` y click en top bar), click en fila del picker, `d` + tag + enter (deploy en vivo en Events), click cancela el modal, click en botones de Details, pestañas con `h/l` y click, Ctrl+C sale limpio.
- Actualizar el estado en `docs/plan/2026-07-04-revision-arquitectura.md` (punto 5 → hecho).
