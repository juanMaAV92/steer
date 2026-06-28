# Rediseño TUI multi-panel (lazydocker) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reescribir `internal/tui/` a un layout multi-panel persistente estilo lazydocker (sidebar de servicios + panel derecho con pestañas Details/Events/Logs), con soporte de mouse, conservando la capa de comandos y `core.Deployer` intactos.

**Architecture:** Se descompone el `Model` monolítico actual (`model.go`, 465 líneas) en componentes de responsabilidad única (sidebar, panel/tabs, context bar, action overlay) coordinados por un `app.go` raíz que reparte layout con `WindowSizeMsg`, gestiona un enum de foco y enruta teclado y mouse por zonas. La capa `tea.Cmd` (load/deploy/poll/ticks) y los tipos de mensaje se extraen sin cambios a `commands.go`/`messages.go` y se reutilizan.

**Tech Stack:** Go, Bubble Tea v1.3.6, Lipgloss v1.1.0, Bubbles (key, viewport), testify.

## Global Constraints

- `internal/core` y `internal/providers` NO se modifican.
- `tui.Run(dep core.Deployer, cluster, env string, writable bool) error` mantiene su firma (la llama `internal/cli/tui_cmd.go`).
- Reusar `internal/render` para colores/símbolos (`render.Symbol`, `render.StatusLevel`, `render.Bold`, `render.Dim`, `render.Accent`, `render.Success`, `render.Danger`, `render.Warn`). No duplicar lógica de color.
- En entorno `writable == false`, las acciones deploy/scale/rollback se bloquean con aviso; nunca se ejecutan.
- TDD: test que falla → implementación mínima → test pasa → commit. Cada componente renderiza a string de forma determinista para poder afirmarse sin terminal real.
- Comentarios y mensajes de UI en el idioma ya usado en el paquete (español en comentarios, inglés en strings de UI), siguiendo el estilo actual.
- Correr `go test ./internal/tui/...` y `go build ./...` antes de cada commit.

## File Structure

```
internal/tui/
  app.go          ← Model raíz: layout, foco, routing mouse/teclado, top/bottom bar
  app_test.go
  messages.go     ← tipos de mensaje (extraídos de model.go)
  commands.go     ← tea.Cmd: load/deploy/poll/ticks (extraídos de model.go)
  keys.go         ← keymap centralizado (bubbles/key)
  keys_test.go
  styles.go       ← lipgloss: bordes foco/blur, anchos, helpers de zona
  sidebar.go      ← columna izquierda: secciones apiladas
  sidebar_test.go
  context.go      ← top bar (contexto) + bottom bar (ayuda)
  context_test.go
  action.go       ← overlay input deploy/scale/rollback
  action_test.go
  panel/
    tabs.go       ← barra de pestañas + pestaña activa
    tabs_test.go
    details.go    ← pestaña Details (+ fila de acciones)
    details_test.go
    events.go     ← pestaña Events (viewport + feed de deploy)
    events_test.go
    logs.go       ← pestaña Logs (stub)
    logs_test.go
```

Se ELIMINAN al final: `internal/tui/model.go` y `internal/tui/model_test.go` (su contenido se reparte/reescribe).

---

### Task 1: Keymap centralizado y estilos base

**Files:**
- Create: `internal/tui/keys.go`
- Create: `internal/tui/styles.go`
- Test: `internal/tui/keys_test.go`

**Interfaces:**
- Produces:
  - `type keyMap struct { Up, Down, Tab, ShiftTab, Enter, Esc, Deploy, Scale, Rollback, Refresh, Help, Quit key.Binding }`
  - `func defaultKeys() keyMap`
  - `func (k keyMap) shortHelp() string` — línea de ayuda para la bottom bar.
  - Estilos lipgloss: `func focusedBorder() lipgloss.Style`, `func blurredBorder() lipgloss.Style`, `const sidebarMinWidth = 24`, `const singleColumnThreshold = 80`.

- [ ] **Step 1: Write the failing test**

```go
// internal/tui/keys_test.go
package tui

import (
	"testing"

	"github.com/charmbracelet/bubbles/key"
	"github.com/stretchr/testify/require"
)

func TestDefaultKeysBound(t *testing.T) {
	k := defaultKeys()
	require.True(t, key.Matches(keyMsg("j"), k.Down))
	require.True(t, key.Matches(keyMsg("k"), k.Up))
	require.True(t, key.Matches(keyMsg("d"), k.Deploy))
	require.True(t, key.Matches(keyMsg("q"), k.Quit))
	require.NotEmpty(t, k.shortHelp())
}

// keyMsg es el helper compartido de tests del paquete tui.
func keyMsg(s string) tea.KeyMsg {
	switch s {
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}
```

Añade el import `tea "github.com/charmbracelet/bubbletea"` al test.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestDefaultKeysBound`
Expected: FAIL (compilación: `defaultKeys`, `keyMap`, `focusedBorder` no definidos). Nota: `model_test.go` aún define su propio `keyMsg`; este test lo redefine — ver Step 3.

- [ ] **Step 3: Resolver colisión del helper `keyMsg`**

`model_test.go` ya define `keyMsg`. Para evitar redefinición, ELIMINA la función `keyMsg` de `internal/tui/model_test.go` (líneas 69-80) — quedará la versión en `keys_test.go`. No toques el resto de `model_test.go`.

- [ ] **Step 4: Write minimal implementation**

```go
// internal/tui/keys.go
package tui

import "github.com/charmbracelet/bubbles/key"

// keyMap centraliza los atajos de la TUI (habilita ? help y rebinds futuros).
type keyMap struct {
	Up, Down, Tab, ShiftTab, Enter, Esc       key.Binding
	Deploy, Scale, Rollback, Refresh, Help, Quit key.Binding
}

func defaultKeys() keyMap {
	return keyMap{
		Up:       key.NewBinding(key.WithKeys("up", "k")),
		Down:     key.NewBinding(key.WithKeys("down", "j")),
		Tab:      key.NewBinding(key.WithKeys("tab")),
		ShiftTab: key.NewBinding(key.WithKeys("shift+tab")),
		Enter:    key.NewBinding(key.WithKeys("enter")),
		Esc:      key.NewBinding(key.WithKeys("esc")),
		Deploy:   key.NewBinding(key.WithKeys("d")),
		Scale:    key.NewBinding(key.WithKeys("s")),
		Rollback: key.NewBinding(key.WithKeys("R")),
		Refresh:  key.NewBinding(key.WithKeys("r")),
		Help:     key.NewBinding(key.WithKeys("?")),
		Quit:     key.NewBinding(key.WithKeys("q", "ctrl+c")),
	}
}

func (k keyMap) shortHelp() string {
	return "↑↓/click select · tab switch panel · d deploy · s scale · R rollback · r refresh · ? help · q quit"
}
```

```go
// internal/tui/styles.go
package tui

import "github.com/charmbracelet/lipgloss"

const (
	sidebarMinWidth       = 24
	singleColumnThreshold = 80
)

// focusedBorder es el borde de un panel con foco (resaltado).
func focusedBorder() lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("12")) // azul
}

// blurredBorder es el borde de un panel sin foco (apagado).
func blurredBorder() lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")) // gris
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/tui/ -run TestDefaultKeysBound && go build ./...`
Expected: PASS y build OK.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/keys.go internal/tui/styles.go internal/tui/keys_test.go internal/tui/model_test.go
git commit -m "feat(tui): keymap centralizado y estilos base de paneles

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: Extraer mensajes y comandos de model.go

Refactor puro: mover los tipos de mensaje y las `tea.Cmd` fuera de `model.go` a archivos propios, sin cambiar comportamiento. Mantiene la suite verde.

**Files:**
- Create: `internal/tui/messages.go`
- Create: `internal/tui/commands.go`
- Modify: `internal/tui/model.go` (quitar lo movido)

**Interfaces:**
- Produces (movidos sin cambios desde model.go):
  - Mensajes: `servicesMsg`, `tickMsg`, `actionDoneMsg`, `deployStartedMsg`, `deployPollMsg`, `deployPollTickMsg`.
  - Comandos: `tickCmd() tea.Cmd`, `deployTickCmd() tea.Cmd`, `startDeployCmd(dep core.Deployer, cluster, service, tag string) tea.Cmd`, `deployPollCmd(dep core.Deployer, cluster, service, lastID string) tea.Cmd`.
  - Constante `refreshInterval = 15 * time.Second`.

- [ ] **Step 1: Crear messages.go**

Mueve a `internal/tui/messages.go` (con `package tui` y los imports `"time"`, `"github.com/juanMaAV92/steer/internal/core"`) estos tipos, recortándolos de `model.go`: `servicesMsg`, `tickMsg`, `actionDoneMsg`, `deployStartedMsg`, `deployPollMsg` (usa `core.ServiceEvent`), `deployPollTickMsg`.

```go
// internal/tui/messages.go
package tui

import (
	"time"

	"github.com/juanMaAV92/steer/internal/core"
)

type servicesMsg struct {
	services []core.ServiceStatus
	err      error
}

type tickMsg struct{}

type actionDoneMsg struct {
	msg string
	err error
}

type deployStartedMsg struct {
	steps  []string
	lastID string
	err    error
}

type deployPollMsg struct {
	events                    []core.ServiceEvent
	lastID                    string
	rollout                   string
	running, pending, desired int
	done, failed              bool
	err                       error
}

type deployPollTickMsg struct{}

const refreshInterval = 15 * time.Second

var _ = time.Second // (eliminar si refreshInterval ya usa time)
```

Quita la última línea `var _ = ...` (es solo recordatorio); `refreshInterval` ya referencia `time`.

- [ ] **Step 2: Crear commands.go**

Mueve a `internal/tui/commands.go` las funciones `tickCmd`, `deployTickCmd`, `startDeployCmd`, `deployPollCmd` recortándolas de `model.go` (idénticas).

```go
// internal/tui/commands.go
package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/juanMaAV92/steer/internal/core"
)

func tickCmd() tea.Cmd {
	return tea.Tick(refreshInterval, func(time.Time) tea.Msg { return tickMsg{} })
}

func deployTickCmd() tea.Cmd {
	return tea.Tick(3*time.Second, func(time.Time) tea.Msg { return deployPollTickMsg{} })
}

func startDeployCmd(dep core.Deployer, cluster, service, tag string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		baseline := ""
		if evs, err := dep.ServiceEvents(ctx, cluster, service); err == nil && len(evs) > 0 {
			baseline = evs[0].ID
		}
		var steps []string
		err := dep.Deploy(ctx, cluster, service, tag, func(s string) { steps = append(steps, s) })
		return deployStartedMsg{steps: steps, lastID: baseline, err: err}
	}
}

func deployPollCmd(dep core.Deployer, cluster, service, lastID string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		var fresh []core.ServiceEvent
		newLast := lastID
		if evs, err := dep.ServiceEvents(ctx, cluster, service); err == nil {
			for _, e := range evs {
				if e.ID == lastID {
					break
				}
				fresh = append(fresh, e)
			}
			if len(evs) > 0 {
				newLast = evs[0].ID
			}
		}
		d, err := dep.DeploymentStatus(ctx, cluster, service)
		return deployPollMsg{
			events: fresh, lastID: newLast,
			rollout: d.Rollout, running: d.Running, pending: d.Pending, desired: d.Desired,
			done:   d.Rollout == "COMPLETED" && d.Running >= d.Desired,
			failed: d.Rollout == "FAILED",
			err:    err,
		}
	}
}
```

- [ ] **Step 3: Limpiar model.go**

En `internal/tui/model.go` elimina las definiciones ya movidas (los seis tipos de mensaje, `refreshInterval`, `tickCmd`, `deployTickCmd`, `startDeployCmd`, `deployPollCmd`). Ajusta los imports de `model.go` quitando los que queden sin uso (probablemente ninguno cambia salvo que `time` siga usándose por otros lados — déjalo si compila).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/tui/... && go build ./...`
Expected: PASS (refactor sin cambio de comportamiento; los tests existentes siguen verdes).

- [ ] **Step 5: Commit**

```bash
git add internal/tui/messages.go internal/tui/commands.go internal/tui/model.go
git commit -m "refactor(tui): extraer mensajes y comandos de model.go

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: Componente Sidebar

Columna izquierda con secciones apiladas. V1: sección SERVICES navegable; secciones IMAGES (ECR) y DATABASES como stubs visuales "(próximamente)".

**Files:**
- Create: `internal/tui/sidebar.go`
- Test: `internal/tui/sidebar_test.go`

**Interfaces:**
- Consumes: `core.ServiceStatus`, `render.Symbol`, `render.StatusLevel`, `render.Accent`, `render.Dim`.
- Produces:
  - `type sidebar struct { services []core.ServiceStatus; cursor int; focused bool; width, height int }`
  - `func newSidebar() sidebar`
  - `func (s *sidebar) setServices(svc []core.ServiceStatus)` — fija lista y reajusta cursor (clamp).
  - `func (s *sidebar) moveUp()` / `func (s *sidebar) moveDown()`
  - `func (s sidebar) selected() (core.ServiceStatus, bool)`
  - `func (s *sidebar) selectIndex(i int)` — usado por click (no-op si fuera de rango).
  - `func (s sidebar) serviceRowCount() int` — nº de filas clicables de servicio (para mapeo de mouse).
  - `func (s sidebar) view() string`

- [ ] **Step 1: Write the failing test**

```go
// internal/tui/sidebar_test.go
package tui

import (
	"strings"
	"testing"

	"github.com/juanMaAV92/steer/internal/core"
	"github.com/stretchr/testify/require"
)

func sampleServices() []core.ServiceStatus {
	return []core.ServiceStatus{
		{Name: "api", Running: 2, Desired: 2, Tag: "v1.4"},
		{Name: "web", Running: 3, Desired: 3, Tag: "v2.0"},
		{Name: "worker", Running: 1, Desired: 2, Tag: "v1.1"},
	}
}

func TestSidebarNavigationClamps(t *testing.T) {
	s := newSidebar()
	s.setServices(sampleServices())
	require.Equal(t, 0, s.cursor)
	s.moveUp() // clamp en 0
	require.Equal(t, 0, s.cursor)
	s.moveDown()
	s.moveDown()
	s.moveDown() // clamp en 2
	require.Equal(t, 2, s.cursor)
	sel, ok := s.selected()
	require.True(t, ok)
	require.Equal(t, "worker", sel.Name)
}

func TestSidebarSelectIndex(t *testing.T) {
	s := newSidebar()
	s.setServices(sampleServices())
	s.selectIndex(1)
	sel, _ := s.selected()
	require.Equal(t, "web", sel.Name)
	s.selectIndex(99) // fuera de rango: no-op
	sel, _ = s.selected()
	require.Equal(t, "web", sel.Name)
}

func TestSidebarSetServicesReclampsCursor(t *testing.T) {
	s := newSidebar()
	s.setServices(sampleServices())
	s.selectIndex(2)
	s.setServices([]core.ServiceStatus{{Name: "api"}}) // lista se encoge
	require.Equal(t, 0, s.cursor)
}

func TestSidebarViewListsServicesAndSections(t *testing.T) {
	s := newSidebar()
	s.width, s.height = 30, 20
	s.setServices(sampleServices())
	out := s.view()
	require.Contains(t, out, "SERVICES")
	require.Contains(t, out, "api")
	require.Contains(t, out, "worker")
	require.Contains(t, out, "IMAGES")
	require.Contains(t, strings.ToLower(out), "próximamente")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestSidebar`
Expected: FAIL (`newSidebar` etc. no definidos).

- [ ] **Step 3: Write minimal implementation**

```go
// internal/tui/sidebar.go
package tui

import (
	"strconv"
	"strings"

	"github.com/juanMaAV92/steer/internal/core"
	"github.com/juanMaAV92/steer/internal/render"
)

// sidebar es la columna izquierda con secciones apiladas por dominio.
type sidebar struct {
	services      []core.ServiceStatus
	cursor        int
	focused       bool
	width, height int
}

func newSidebar() sidebar { return sidebar{} }

func (s *sidebar) setServices(svc []core.ServiceStatus) {
	s.services = svc
	if s.cursor >= len(s.services) {
		s.cursor = max(0, len(s.services)-1)
	}
}

func (s *sidebar) moveDown() {
	if s.cursor < len(s.services)-1 {
		s.cursor++
	}
}

func (s *sidebar) moveUp() {
	if s.cursor > 0 {
		s.cursor--
	}
}

func (s *sidebar) selectIndex(i int) {
	if i >= 0 && i < len(s.services) {
		s.cursor = i
	}
}

func (s sidebar) selected() (core.ServiceStatus, bool) {
	if s.cursor < 0 || s.cursor >= len(s.services) {
		return core.ServiceStatus{}, false
	}
	return s.services[s.cursor], true
}

// serviceRowCount es el nº de filas de servicio clicables (mapeo de mouse).
func (s sidebar) serviceRowCount() int { return len(s.services) }

func (s sidebar) view() string {
	var b strings.Builder
	b.WriteString(render.Bold("SERVICES") + render.Dim("  ("+strconv.Itoa(len(s.services))+")") + "\n")
	for i, svc := range s.services {
		cursor := "  "
		if i == s.cursor {
			cursor = render.Accent("> ")
		}
		tag := svc.Tag
		if tag == "" {
			tag = "—"
		}
		b.WriteString(cursor + render.Symbol(render.StatusLevel(svc.Running, svc.Desired)) + " " +
			svc.Name + "  " + strconv.Itoa(svc.Running) + "/" + strconv.Itoa(svc.Desired) +
			"  " + render.Dim(tag) + "\n")
	}
	b.WriteString("\n" + render.Dim("IMAGES (ECR)") + "\n" + render.Dim("  (próximamente)") + "\n")
	b.WriteString("\n" + render.Dim("DATABASES") + "\n" + render.Dim("  (próximamente)") + "\n")
	return b.String()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/tui/ -run TestSidebar && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/sidebar.go internal/tui/sidebar_test.go
git commit -m "feat(tui): componente sidebar con secciones apiladas

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: Panel — barra de pestañas y Details

Subpaquete `internal/tui/panel`. Pestañas Details/Events/Logs + render de la pestaña Details con la fila de acciones.

**Files:**
- Create: `internal/tui/panel/tabs.go`
- Create: `internal/tui/panel/details.go`
- Test: `internal/tui/panel/tabs_test.go`
- Test: `internal/tui/panel/details_test.go`

**Interfaces:**
- Consumes: `core.ServiceStatus`, `render` helpers.
- Produces:
  - `type Tab int` con `const ( TabDetails Tab = iota; TabEvents; TabLogs )`
  - `func (t Tab) String() string` → "Details"/"Events"/"Logs".
  - `type Tabs struct { Active Tab }`
  - `func (tb *Tabs) Next()` / `func (tb *Tabs) Set(t Tab)`
  - `func (tb Tabs) View() string` — barra "Details ─ Events ─ Logs" con la activa resaltada.
  - `func (tb Tabs) Count() int` → 3 (para mapeo de clicks).
  - `func DetailsView(s core.ServiceStatus, writable bool) string` — stats + fila de acciones (las acciones aparecen tenues/bloqueadas si `!writable`).

- [ ] **Step 1: Write the failing tests**

```go
// internal/tui/panel/tabs_test.go
package panel

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTabsNextCycles(t *testing.T) {
	tb := Tabs{}
	require.Equal(t, TabDetails, tb.Active)
	tb.Next()
	require.Equal(t, TabEvents, tb.Active)
	tb.Next()
	require.Equal(t, TabLogs, tb.Active)
	tb.Next() // vuelve al inicio
	require.Equal(t, TabDetails, tb.Active)
}

func TestTabsViewShowsAllTabs(t *testing.T) {
	tb := Tabs{Active: TabEvents}
	out := tb.View()
	require.Contains(t, out, "Details")
	require.Contains(t, out, "Events")
	require.Contains(t, out, "Logs")
	require.Equal(t, 3, tb.Count())
}
```

```go
// internal/tui/panel/details_test.go
package panel

import (
	"strings"
	"testing"

	"github.com/juanMaAV92/steer/internal/core"
	"github.com/stretchr/testify/require"
)

func TestDetailsViewShowsStatsAndActions(t *testing.T) {
	s := core.ServiceStatus{Name: "api", Running: 2, Desired: 2, Pending: 0, Status: "ACTIVE", Tag: "v1.4"}
	out := DetailsView(s, true)
	require.Contains(t, out, "2/2")
	require.Contains(t, out, "ACTIVE")
	require.Contains(t, out, "v1.4")
	require.Contains(t, strings.ToLower(out), "deploy")
	require.Contains(t, strings.ToLower(out), "scale")
	require.Contains(t, strings.ToLower(out), "rollback")
}

func TestDetailsViewReadOnlyHint(t *testing.T) {
	s := core.ServiceStatus{Name: "api", Running: 1, Desired: 1}
	out := DetailsView(s, false)
	require.Contains(t, strings.ToLower(out), "read-only")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/panel/ -run 'TestTabs|TestDetails'`
Expected: FAIL (paquete `panel` no existe).

- [ ] **Step 3: Write minimal implementation**

```go
// internal/tui/panel/tabs.go
package panel

import (
	"strings"

	"github.com/juanMaAV92/steer/internal/render"
)

// Tab identifica una pestaña del panel derecho.
type Tab int

const (
	TabDetails Tab = iota
	TabEvents
	TabLogs
)

func (t Tab) String() string {
	switch t {
	case TabEvents:
		return "Events"
	case TabLogs:
		return "Logs"
	default:
		return "Details"
	}
}

// Tabs es el estado de la barra de pestañas.
type Tabs struct{ Active Tab }

func (tb *Tabs) Next()      { tb.Active = (tb.Active + 1) % Tab(tb.Count()) }
func (tb *Tabs) Set(t Tab)  { tb.Active = t }
func (tb Tabs) Count() int  { return 3 }

func (tb Tabs) View() string {
	parts := make([]string, 0, tb.Count())
	for i := 0; i < tb.Count(); i++ {
		label := Tab(i).String()
		if Tab(i) == tb.Active {
			label = render.Accent(label)
		} else {
			label = render.Dim(label)
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, render.Dim(" ─ "))
}
```

```go
// internal/tui/panel/details.go
package panel

import (
	"strconv"
	"strings"

	"github.com/juanMaAV92/steer/internal/core"
	"github.com/juanMaAV92/steer/internal/render"
)

// DetailsView renderiza la pestaña Details con stats y la fila de acciones.
func DetailsView(s core.ServiceStatus, writable bool) string {
	var b strings.Builder
	b.WriteString(render.Bold(s.Name) + "\n\n")
	b.WriteString("running   " + strconv.Itoa(s.Running) + "/" + strconv.Itoa(s.Desired) + "\n")
	b.WriteString("pending   " + strconv.Itoa(s.Pending) + "\n")
	b.WriteString("status    " + s.Status + "\n")
	tag := s.Tag
	if tag == "" {
		tag = "—"
	}
	b.WriteString("tag       " + render.Accent(tag) + "\n\n")
	if writable {
		b.WriteString(render.Dim("[d] deploy  [s] scale  [R] rollback"))
	} else {
		b.WriteString(render.Warn("read-only environment — actions disabled"))
	}
	return b.String()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/tui/panel/ && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/panel/tabs.go internal/tui/panel/details.go internal/tui/panel/tabs_test.go internal/tui/panel/details_test.go
git commit -m "feat(tui): panel tabs y pestaña Details con fila de acciones

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: Panel — Events (viewport) y Logs (stub)

**Files:**
- Create: `internal/tui/panel/events.go`
- Create: `internal/tui/panel/logs.go`
- Test: `internal/tui/panel/events_test.go`
- Test: `internal/tui/panel/logs_test.go`

**Interfaces:**
- Consumes: `core.ServiceEvent`, `bubbles/viewport`, `render`.
- Produces:
  - `type Events struct { vp viewport.Model; lines []string; statusLine string }`
  - `func NewEvents() Events`
  - `func (e *Events) SetSize(w, h int)`
  - `func (e *Events) AppendLine(line string)` — añade una línea y reescribe el viewport (auto-scroll al fondo).
  - `func (e *Events) SetStatusLine(s string)`
  - `func (e *Events) Reset()` — limpia líneas y status (al iniciar un deploy nuevo).
  - `func (e *Events) Update(msg tea.Msg) tea.Cmd` — delega scroll de rueda/teclas al viewport.
  - `func (e Events) View() string`
  - `func LogsView() string` — stub: "logs no disponibles todavía".

- [ ] **Step 1: Write the failing tests**

```go
// internal/tui/panel/events_test.go
package panel

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEventsAppendAndView(t *testing.T) {
	e := NewEvents()
	e.SetSize(40, 10)
	e.AppendLine("[12:00:00] task started")
	e.AppendLine("[12:00:01] task running")
	e.SetStatusLine("Rollout: IN_PROGRESS")
	out := e.View()
	require.Contains(t, out, "task started")
	require.Contains(t, out, "task running")
	require.Contains(t, out, "Rollout: IN_PROGRESS")
}

func TestEventsReset(t *testing.T) {
	e := NewEvents()
	e.SetSize(40, 10)
	e.AppendLine("old line")
	e.Reset()
	require.NotContains(t, e.View(), "old line")
}
```

```go
// internal/tui/panel/logs_test.go
package panel

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLogsViewStub(t *testing.T) {
	require.Contains(t, strings.ToLower(LogsView()), "logs")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/panel/ -run 'TestEvents|TestLogs'`
Expected: FAIL (`NewEvents`, `LogsView` no definidos).

- [ ] **Step 3: Write minimal implementation**

```go
// internal/tui/panel/events.go
package panel

import (
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/juanMaAV92/steer/internal/render"
)

// Events es la pestaña de eventos ECS + feed de progreso de deploy (scrolleable).
type Events struct {
	vp         viewport.Model
	lines      []string
	statusLine string
}

func NewEvents() Events { return Events{vp: viewport.New(0, 0)} }

func (e *Events) SetSize(w, h int) {
	e.vp.Width = w
	e.vp.Height = h
	e.sync()
}

func (e *Events) AppendLine(line string) {
	e.lines = append(e.lines, line)
	e.sync()
	e.vp.GotoBottom()
}

func (e *Events) SetStatusLine(s string) {
	e.statusLine = s
	e.sync()
}

func (e *Events) Reset() {
	e.lines = nil
	e.statusLine = ""
	e.sync()
}

func (e *Events) sync() {
	body := strings.Join(e.lines, "\n")
	if e.statusLine != "" {
		body += "\n\n" + e.statusLine
	}
	if body == "" {
		body = render.Dim("no events yet")
	}
	e.vp.SetContent(body)
}

// Update delega scroll (rueda/teclas) al viewport interno.
func (e *Events) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	e.vp, cmd = e.vp.Update(msg)
	return cmd
}

func (e Events) View() string { return e.vp.View() }
```

```go
// internal/tui/panel/logs.go
package panel

import "github.com/juanMaAV92/steer/internal/render"

// LogsView es un stub: la fuente de logs (LogSource) aún no se consume aquí.
func LogsView() string {
	return render.Dim("Logs no disponibles todavía — sin fuente de logs configurada.")
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/tui/panel/ && go build ./...`
Expected: PASS. Si `bubbles/viewport` no resuelve, corre `go mod tidy` (mueve bubbles a dependencia directa) y reintenta.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/panel/events.go internal/tui/panel/logs.go internal/tui/panel/events_test.go internal/tui/panel/logs_test.go go.mod go.sum
git commit -m "feat(tui): pestaña Events con viewport y stub de Logs

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 6: Top bar (contexto) y bottom bar (ayuda)

**Files:**
- Create: `internal/tui/context.go`
- Test: `internal/tui/context_test.go`

**Interfaces:**
- Consumes: `render` helpers, `keyMap.shortHelp`.
- Produces:
  - `func topBar(cloud, env, cluster string, writable bool) string` — `steer — aws · staging (cluster: …) — writable ●`/`read-only ○`.
  - `func bottomBar(help, notice, status string) string` — ayuda + (si hay) aviso/estado.

- [ ] **Step 1: Write the failing test**

```go
// internal/tui/context_test.go
package tui

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTopBarShowsContext(t *testing.T) {
	out := topBar("aws", "staging", "staging-cluster", true)
	require.Contains(t, out, "aws")
	require.Contains(t, out, "staging")
	require.Contains(t, out, "staging-cluster")
	require.Contains(t, strings.ToLower(out), "writable")
}

func TestTopBarReadOnly(t *testing.T) {
	out := topBar("aws", "prod", "prod-cluster", false)
	require.Contains(t, strings.ToLower(out), "read-only")
}

func TestBottomBarShowsNoticeOverHelp(t *testing.T) {
	out := bottomBar("help text", "blocked!", "")
	require.Contains(t, out, "blocked!")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run 'TestTopBar|TestBottomBar'`
Expected: FAIL (`topBar`/`bottomBar` no definidos).

- [ ] **Step 3: Write minimal implementation**

```go
// internal/tui/context.go
package tui

import "github.com/juanMaAV92/steer/internal/render"

// topBar renderiza la barra de contexto (cloud · env · cluster · writable).
func topBar(cloud, env, cluster string, writable bool) string {
	state := render.Success("writable ●")
	if !writable {
		state = render.Warn("read-only ○")
	}
	return render.Bold("steer") + render.Dim(" — "+cloud+" · "+env+
		" (cluster: "+cluster+") — ") + state
}

// bottomBar muestra ayuda y, si hay, un aviso o estado que tiene prioridad visual.
func bottomBar(help, notice, status string) string {
	switch {
	case notice != "":
		return render.Warn(notice)
	case status != "":
		return render.Success(status)
	default:
		return render.Dim(help)
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/tui/ -run 'TestTopBar|TestBottomBar' && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/context.go internal/tui/context_test.go
git commit -m "feat(tui): top bar de contexto y bottom bar de ayuda

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 7: Overlay de acción (deploy/scale/rollback)

Input contextual que reemplaza la `viewConfirm` actual. Maneja el texto del tag/count y la confirmación de rollback.

**Files:**
- Create: `internal/tui/action.go`
- Test: `internal/tui/action_test.go`

**Interfaces:**
- Consumes: `render`, `tea.KeyMsg`.
- Produces:
  - `type actionKind int` con `const ( actionRollback actionKind = iota; actionDeploy; actionScale )`
  - `type action struct { kind actionKind; service, input string; active bool }`
  - `func (a *action) open(kind actionKind, service string)` — activa y resetea input.
  - `func (a *action) close()`
  - `func (a *action) typeKey(msg tea.KeyMsg)` — añade runas / backspace (ignora en rollback).
  - `func (a action) ready() bool` — rollback siempre listo; deploy/scale requieren input no vacío.
  - `func (a action) view() string` — texto del overlay según kind.

- [ ] **Step 1: Write the failing test**

```go
// internal/tui/action_test.go
package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

func TestActionDeployTypingAndReady(t *testing.T) {
	var a action
	a.open(actionDeploy, "api")
	require.True(t, a.active)
	require.False(t, a.ready()) // input vacío
	for _, r := range "v2" {
		a.typeKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(string(r))})
	}
	require.Equal(t, "v2", a.input)
	require.True(t, a.ready())
	a.typeKey(tea.KeyMsg{Type: tea.KeyBackspace})
	require.Equal(t, "v", a.input)
}

func TestActionRollbackAlwaysReadyIgnoresTyping(t *testing.T) {
	var a action
	a.open(actionRollback, "api")
	require.True(t, a.ready())
	a.typeKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	require.Empty(t, a.input) // rollback no acepta input
}

func TestActionCloseDeactivates(t *testing.T) {
	var a action
	a.open(actionScale, "api")
	a.close()
	require.False(t, a.active)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestAction`
Expected: FAIL (`action` no definido). Nota: si `model.go` aún define `actionKind`/`actionRollback`, habrá colisión — se resuelve en Task 8 al eliminar model.go. Por ahora, si colisiona, comenta temporalmente no; en su lugar, ejecuta el test con model.go presente fallando por redefinición y procede a Task 8 que elimina model.go. **Para evitar fricción: implementa Step 3 y luego ve directo a Task 8** (ambos commits pueden quedar juntos si el build no pasa aislado). Alternativamente, en Step 3 NO redefinas `actionKind`/`actionRollback`/`actionDeploy`/`actionScale` (ya existen en model.go) y solo agrega `action`.

Para mantener cada task verde, sigue esta variante: en Step 3 define únicamente lo que NO existe ya en `model.go`. `model.go` ya declara `actionKind`, `actionRollback`, `actionDeploy`, `actionScale` y un `pendingAction`. Reusa esos identificadores; crea solo el nuevo tipo `action` y sus métodos.

- [ ] **Step 3: Write minimal implementation**

```go
// internal/tui/action.go
package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/juanMaAV92/steer/internal/render"
)

// action es el overlay de input para deploy/scale/rollback.
// (actionKind y sus constantes ya existen en model.go y se reutilizan;
// se eliminará el viejo pendingAction al borrar model.go en la Task 8.)
type action struct {
	kind    actionKind
	service string
	input   string
	active  bool
}

func (a *action) open(kind actionKind, service string) {
	a.kind = kind
	a.service = service
	a.input = ""
	a.active = true
}

func (a *action) close() { a.active = false }

func (a *action) typeKey(msg tea.KeyMsg) {
	if a.kind == actionRollback {
		return
	}
	switch msg.Type {
	case tea.KeyBackspace:
		if n := len(a.input); n > 0 {
			a.input = a.input[:n-1]
		}
	case tea.KeyRunes:
		a.input += string(msg.Runes)
	}
}

func (a action) ready() bool {
	return a.kind == actionRollback || a.input != ""
}

func (a action) view() string {
	switch a.kind {
	case actionRollback:
		return "Roll back " + render.Bold(a.service) + " to previous revision?\n" +
			render.Dim("enter to confirm · esc to cancel")
	case actionDeploy:
		return "Deploy " + render.Bold(a.service) + " — image tag: " + render.Accent(a.input) + "\n" +
			render.Dim("type the tag · enter to deploy · esc to cancel")
	case actionScale:
		return "Scale " + render.Bold(a.service) + " — desired count: " + render.Accent(a.input) + "\n" +
			render.Dim("type a number · enter to scale · esc to cancel")
	}
	return ""
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/tui/ -run TestAction && go build ./...`
Expected: PASS (reusando `actionKind`/constantes de model.go; `action` es nuevo).

- [ ] **Step 5: Commit**

```bash
git add internal/tui/action.go internal/tui/action_test.go
git commit -m "feat(tui): overlay de acción deploy/scale/rollback

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 8: App root — layout, foco y routing de teclado (reemplaza model.go)

Crea el nuevo `Model` raíz en `app.go`, ensamblando los componentes, con layout por `WindowSizeMsg`, enum de foco y routing de teclado. ELIMINA `model.go` y reescribe `model_test.go` como `app_test.go`.

**Files:**
- Create: `internal/tui/app.go`
- Create: `internal/tui/app_test.go`
- Delete: `internal/tui/model.go`
- Delete: `internal/tui/model_test.go`

**Interfaces:**
- Consumes: `sidebar`, `panel.Tabs`, `panel.Events`, `panel.DetailsView`, `panel.LogsView`, `action`, `topBar`, `bottomBar`, `defaultKeys`, comandos de `commands.go`, mensajes de `messages.go`, estilos de `styles.go`.
- Produces:
  - `type focus int` con `const ( focusSidebar focus = iota; focusPanel; focusAction )`
  - `type Model struct { ... }` (campos abajo)
  - `func New(dep core.Deployer, cluster, env string, writable bool) Model`
  - `func (m Model) Init() tea.Cmd`
  - `func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd)`
  - `func (m Model) View() string`
  - helpers: `func (m Model) loadServicesCmd() tea.Cmd`, `func (m *Model) runActionCmd() tea.Cmd`, `func (m *Model) layout()` (reparte tamaños).

- [ ] **Step 1: Eliminar model.go y model_test.go**

```bash
git rm internal/tui/model.go internal/tui/model_test.go
```

(Los tipos `actionKind`, `actionRollback`, `actionDeploy`, `actionScale` que vivían en model.go se REDEFINEN ahora en app.go — ver Step 3. `selected`, `View`, `Update`, `New` se reescriben.)

- [ ] **Step 2: Write the failing tests**

```go
// internal/tui/app_test.go
package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/juanMaAV92/steer/internal/core"
	"github.com/juanMaAV92/steer/internal/core/coretest"
	"github.com/stretchr/testify/require"
)

func newTestModel(services []core.ServiceStatus) Model {
	m := New(&coretest.FakeDeployer{Services: services}, "stg-cluster", "stg", true)
	m.sidebar.setServices(services)
	m, _ = applySize(m, 120, 40)
	return m
}

func applySize(m Model, w, h int) (Model, tea.Cmd) {
	updated, cmd := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return updated.(Model), cmd
}

func mustUpdate(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	updated, _ := m.Update(msg)
	return updated.(Model)
}

func TestServicesMsgPopulatesSidebar(t *testing.T) {
	m := newTestModel(nil)
	m = mustUpdate(t, m, servicesMsg{services: sampleServices()})
	require.Len(t, m.sidebar.services, 3)
	require.False(t, m.loading)
}

func TestSidebarKeyboardNavigation(t *testing.T) {
	m := newTestModel(sampleServices())
	m = mustUpdate(t, m, keyMsg("j"))
	require.Equal(t, 1, m.sidebar.cursor)
	m = mustUpdate(t, m, keyMsg("k"))
	require.Equal(t, 0, m.sidebar.cursor)
}

func TestTabMovesFocusToPanel(t *testing.T) {
	m := newTestModel(sampleServices())
	require.Equal(t, focusSidebar, m.focus)
	m = mustUpdate(t, m, keyMsg("tab"))
	require.Equal(t, focusPanel, m.focus)
}

func TestQuitKeys(t *testing.T) {
	m := newTestModel(nil)
	for _, key := range []string{"q", "ctrl+c"} {
		_, cmd := m.Update(keyMsg(key))
		require.NotNil(t, cmd, "expected quit cmd for %q", key)
	}
}

func TestReadOnlyBlocksActions(t *testing.T) {
	ro := New(&coretest.FakeDeployer{Services: sampleServices()}, "prod-cluster", "production", false)
	ro.sidebar.setServices(sampleServices())
	ro, _ = applySize(ro, 120, 40)
	for _, key := range []string{"d", "s", "R"} {
		m := mustUpdate(t, ro, keyMsg(key))
		require.NotEqual(t, focusAction, m.focus, "key %q must not open action overlay in read-only", key)
		require.NotEmpty(t, m.notice)
	}
}

func TestViewRendersWithoutPanic(t *testing.T) {
	m := newTestModel(sampleServices())
	require.NotEmpty(t, m.View())
}
```

- [ ] **Step 3: Write minimal implementation**

```go
// internal/tui/app.go
package tui

import (
	"context"
	"strconv"

	"github.com/charmbracelet/lipgloss"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/juanMaAV92/steer/internal/core"
	"github.com/juanMaAV92/steer/internal/render"
	"github.com/juanMaAV92/steer/internal/tui/panel"
)

type focus int

const (
	focusSidebar focus = iota
	focusPanel
	focusAction
)

type actionKind int

const (
	actionRollback actionKind = iota
	actionDeploy
	actionScale
)

// Model es el estado raíz de la TUI (patrón Elm de Bubble Tea).
type Model struct {
	dep      core.Deployer
	cluster  string
	env      string
	writable bool
	keys     keyMap

	sidebar sidebar
	tabs    panel.Tabs
	events  panel.Events
	action  action

	focus   focus
	loading bool
	notice  string
	status  string
	err     error

	width, height            int
	sidebarW, panelW, bodyH  int
	singleColumn             bool
	deployActive, deployDone bool
	deployLastID             string
	deployService            string
}

func New(dep core.Deployer, cluster, env string, writable bool) Model {
	return Model{
		dep: dep, cluster: cluster, env: env, writable: writable,
		keys: defaultKeys(), sidebar: newSidebar(), events: panel.NewEvents(),
		loading: true,
	}
}

func (m Model) Init() tea.Cmd { return tea.Batch(m.loadServicesCmd(), tickCmd()) }

func (m Model) loadServicesCmd() tea.Cmd {
	return func() tea.Msg {
		s, err := m.dep.ListServices(context.Background(), m.cluster)
		return servicesMsg{services: s, err: err}
	}
}

// layout reparte el espacio disponible entre sidebar y panel.
// Si el ancho < singleColumnThreshold, colapsa a una sola columna apilada.
func (m *Model) layout() {
	m.singleColumn = m.width < singleColumnThreshold
	m.bodyH = m.height - 4 // top bar (3) + bottom (1)
	if m.bodyH < 3 {
		m.bodyH = 3
	}
	if m.singleColumn {
		m.sidebarW = m.width - 4
		m.panelW = m.width - 4
		if m.sidebarW < 10 {
			m.sidebarW, m.panelW = 10, 10
		}
		m.events.SetSize(m.panelW-2, m.bodyH/2-3)
		return
	}
	m.sidebarW = m.width * 30 / 100
	if m.sidebarW < sidebarMinWidth {
		m.sidebarW = sidebarMinWidth
	}
	m.panelW = m.width - m.sidebarW - 4 // bordes
	if m.panelW < 10 {
		m.panelW = 10
	}
	m.events.SetSize(m.panelW-2, m.bodyH-3) // -tabs -bordes
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()
		return m, nil

	case servicesMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.sidebar.setServices(msg.services)
		return m, nil

	case tickMsg:
		return m, tea.Batch(m.loadServicesCmd(), tickCmd())

	case actionDoneMsg:
		m.focus = focusSidebar
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.status = msg.msg
		}
		return m, m.loadServicesCmd()

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// foco en overlay de acción: captura el input
	if m.focus == focusAction {
		switch {
		case key.Matches(msg, m.keys.Quit) && msg.Type == tea.KeyCtrlC:
			return m, tea.Quit
		case key.Matches(msg, m.keys.Esc):
			m.action.close()
			m.focus = focusSidebar
			return m, nil
		case key.Matches(msg, m.keys.Enter):
			if !m.action.ready() {
				return m, nil
			}
			return m, m.runActionCmd()
		default:
			m.action.typeKey(msg)
			return m, nil
		}
	}

	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, m.keys.Tab):
		if m.focus == focusSidebar {
			m.focus = focusPanel
		} else {
			m.focus = focusSidebar
		}
		return m, nil
	case key.Matches(msg, m.keys.Refresh):
		m.loading = true
		return m, m.loadServicesCmd()
	case key.Matches(msg, m.keys.Deploy), key.Matches(msg, m.keys.Scale), key.Matches(msg, m.keys.Rollback):
		return m.openAction(msg)
	}

	if m.focus == focusPanel {
		switch {
		case key.Matches(msg, m.keys.Down), key.Matches(msg, m.keys.Up):
			cmd := m.events.Update(msg)
			return m, cmd
		case msg.Type == tea.KeyTab: // ya cubierto arriba
		}
		// permitir cambiar pestaña con left/right
		switch msg.String() {
		case "right", "l":
			m.tabs.Next()
		}
		return m, nil
	}

	// foco en sidebar
	switch {
	case key.Matches(msg, m.keys.Down):
		m.sidebar.moveDown()
	case key.Matches(msg, m.keys.Up):
		m.sidebar.moveUp()
	}
	return m, nil
}

func (m Model) openAction(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if !m.writable {
		m.notice = "read-only environment (writable=false) — action blocked"
		return m, nil
	}
	s, ok := m.sidebar.selected()
	if !ok {
		return m, nil
	}
	switch {
	case key.Matches(msg, m.keys.Deploy):
		m.action.open(actionDeploy, s.Name)
	case key.Matches(msg, m.keys.Scale):
		m.action.open(actionScale, s.Name)
	case key.Matches(msg, m.keys.Rollback):
		m.action.open(actionRollback, s.Name)
	}
	m.notice = ""
	m.focus = focusAction
	return m, nil
}

func (m *Model) runActionCmd() tea.Cmd {
	a := m.action
	dep, cluster := m.dep, m.cluster
	m.action.close()
	m.focus = focusSidebar
	return func() tea.Msg {
		ctx := context.Background()
		switch a.kind {
		case actionRollback:
			return actionDoneMsg{msg: "rolled back " + a.service, err: dep.Rollback(ctx, cluster, a.service)}
		case actionDeploy:
			return actionDoneMsg{msg: "deployed " + a.service + " -> " + a.input,
				err: dep.Deploy(ctx, cluster, a.service, a.input, nil)}
		case actionScale:
			n, convErr := strconv.Atoi(a.input)
			if convErr != nil {
				return actionDoneMsg{err: convErr}
			}
			return actionDoneMsg{msg: "scaled " + a.service + " to " + a.input,
				err: dep.Scale(ctx, cluster, a.service, n)}
		}
		return actionDoneMsg{}
	}
}

func (m Model) View() string {
	if m.err != nil {
		return render.Danger("error: "+m.err.Error()) + "\n" + render.Dim("press q to quit")
	}
	top := topBar("aws", m.env, m.cluster, m.writable)

	sideStyle := blurredBorder()
	panelStyle := blurredBorder()
	if m.focus == focusSidebar {
		sideStyle = focusedBorder()
	} else if m.focus == focusPanel {
		panelStyle = focusedBorder()
	}
	panelBody := m.tabs.View() + "\n\n" + m.panelBody()
	var body string
	if m.singleColumn {
		// apilado: sidebar arriba, panel abajo (cada uno la mitad del alto)
		side := sideStyle.Width(m.sidebarW).Height(m.bodyH/2).Render(m.sidebar.view())
		pan := panelStyle.Width(m.panelW).Height(m.bodyH/2).Render(panelBody)
		body = lipgloss.JoinVertical(lipgloss.Left, side, pan)
	} else {
		side := sideStyle.Width(m.sidebarW).Height(m.bodyH).Render(m.sidebar.view())
		pan := panelStyle.Width(m.panelW).Height(m.bodyH).Render(panelBody)
		body = lipgloss.JoinHorizontal(lipgloss.Top, side, pan)
	}

	bottom := bottomBar(m.keys.shortHelp(), m.notice, m.status)
	if m.focus == focusAction {
		bottom = m.action.view()
	}
	return top + "\n" + body + "\n" + bottom
}

func (m Model) panelBody() string {
	s, ok := m.sidebar.selected()
	if !ok {
		return render.Dim("no service selected")
	}
	switch m.tabs.Active {
	case panel.TabEvents:
		return m.events.View()
	case panel.TabLogs:
		return panel.LogsView()
	default:
		return panel.DetailsView(s, m.writable)
	}
}
```

Añade el import `"github.com/charmbracelet/bubbles/key"` a `app.go`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/tui/... && go build ./...`
Expected: PASS. Si hay imports sin uso (p.ej. el `case msg.Type == tea.KeyTab` vacío), límpialos hasta compilar.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/app.go internal/tui/app_test.go
git rm --cached internal/tui/model.go internal/tui/model_test.go 2>/dev/null; true
git add -A internal/tui/
git commit -m "feat(tui): app root multi-panel con foco y routing de teclado

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 9: Routing de mouse por zonas

Click en items del sidebar, pestañas y acciones; rueda → scroll en el panel.

**Files:**
- Modify: `internal/tui/app.go`
- Test: `internal/tui/app_test.go`

**Interfaces:**
- Consumes: `tea.MouseMsg`, geometría de `layout()`, `sidebar.selectIndex`, `sidebar.serviceRowCount`, `tabs.Set`/`tabs.Count`, `events.Update`.
- Produces: manejo de `tea.MouseMsg` dentro de `Model.Update`; helper `func (m *Model) handleMouse(msg tea.MouseMsg) tea.Cmd`.

> **Verificar la API de mouse primero:** los nombres de campos/constantes de `tea.MouseMsg`
> (`Type` vs `Action`/`Button`, `MouseLeft` vs `MouseButtonLeft`, `MouseWheelDown`) varían
> entre versiones de Bubble Tea. Antes de escribir el test, corre
> `go doc github.com/charmbracelet/bubbletea.MouseMsg` y ajusta los constructores del test
> y los `switch` de `handleMouse` a lo que exponga la v1.3.6 instalada.

- [ ] **Step 1: Write the failing test**

```go
// añadir a internal/tui/app_test.go
func TestMouseClickSelectsSidebarService(t *testing.T) {
	m := newTestModel(sampleServices())
	// fila 0 = header SERVICES (y=4 con top bar de 3 + borde 1); fila del 2º servicio:
	click := tea.MouseMsg{Type: tea.MouseLeft, Action: tea.MouseActionPress, X: 3, Y: sidebarServiceRowY(1)}
	m = mustUpdate(t, m, click)
	require.Equal(t, 1, m.sidebar.cursor)
	require.Equal(t, focusSidebar, m.focus)
}

func TestMouseWheelScrollsPanelWhenFocused(t *testing.T) {
	m := newTestModel(sampleServices())
	m.focus = focusPanel
	wheel := tea.MouseMsg{Type: tea.MouseWheelDown, Action: tea.MouseActionPress}
	// no debe panic ni cambiar de servicio
	m = mustUpdate(t, m, wheel)
	require.Equal(t, focusPanel, m.focus)
}
```

`sidebarServiceRowY(i)` es un helper de test que reproduce el mapeo de filas: `return topBarHeight + borderTop + headerRows + i`. Defínelo en el test con las mismas constantes que use `handleMouse` (exponlas en app.go como `const topBarHeight = 3` y documenta el offset). Ajusta el cálculo para que coincida con tu render real.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestMouse`
Expected: FAIL (no se maneja `tea.MouseMsg`).

- [ ] **Step 3: Write minimal implementation**

Añade a `Model.Update` un case `tea.MouseMsg` y el helper. El mapeo de zonas usa los tamaños calculados en `layout()`:

```go
// dentro de Model.Update, añade un case:
	case tea.MouseMsg:
		return m, m.handleMouse(msg)
```

```go
// app.go — constantes de geometría y handler
const (
	topBarHeight   = 3 // top bar con borde
	borderTop      = 1 // borde superior del panel/sidebar
	sidebarHeader  = 1 // línea "SERVICES (n)"
)

func (m *Model) handleMouse(msg tea.MouseMsg) tea.Cmd {
	// rueda: scrollea el panel si está enfocado (o al pasar sobre él)
	if msg.Type == tea.MouseWheelUp || msg.Type == tea.MouseWheelDown {
		if msg.X > m.sidebarW {
			return m.events.Update(msg)
		}
		return nil
	}
	if msg.Action != tea.MouseActionPress || msg.Type != tea.MouseLeft {
		return nil
	}
	// click en la zona del sidebar
	if msg.X <= m.sidebarW {
		row := msg.Y - (topBarHeight + borderTop + sidebarHeader)
		if row >= 0 && row < m.sidebar.serviceRowCount() {
			m.sidebar.selectIndex(row)
			m.focus = focusSidebar
		}
		return nil
	}
	// click en la zona del panel: primera fila útil = pestañas
	panelRow := msg.Y - (topBarHeight + borderTop)
	if panelRow == 0 {
		// reparte el ancho del panel entre las pestañas
		seg := m.panelW / m.tabs.Count()
		if seg < 1 {
			seg = 1
		}
		idx := (msg.X - m.sidebarW) / seg
		if idx >= 0 && idx < m.tabs.Count() {
			m.tabs.Set(panel.Tab(idx))
		}
	}
	m.focus = focusPanel
	return nil
}
```

Ajusta los offsets hasta que `TestMouseClickSelectsSidebarService` pase (el render real puede diferir en 1 fila según bordes; calibra `sidebarHeader`/`borderTop`).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/tui/... && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/app.go internal/tui/app_test.go
git commit -m "feat(tui): routing de mouse (click sidebar/pestañas, rueda scroll)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 10: Flujo de deploy en vivo dentro de la pestaña Events

Conecta `startDeployCmd`/`deployPollCmd` al panel Events en lugar de una pantalla separada. Al ejecutar un deploy, salta a la pestaña Events y alimenta el feed.

**Files:**
- Modify: `internal/tui/app.go`
- Test: `internal/tui/app_test.go`

**Interfaces:**
- Consumes: `deployStartedMsg`, `deployPollMsg`, `deployPollTickMsg`, `startDeployCmd`, `deployPollCmd`, `deployTickCmd`, `events.Reset/AppendLine/SetStatusLine`.
- Produces: en `runActionCmd`, la rama `actionDeploy` ahora devuelve `startDeployCmd` y pone `m.deployActive=true`, `m.tabs.Active=panel.TabEvents`; nuevos cases en `Update` para los tres mensajes de deploy.

- [ ] **Step 1: Write the failing test**

```go
// añadir a internal/tui/app_test.go
func TestDeployFlowFeedsEventsPanel(t *testing.T) {
	fake := &coretest.FakeDeployer{
		Services:        sampleServices(),
		DeploymentValue: core.Deployment{Rollout: "COMPLETED", Running: 2, Desired: 2},
	}
	m := New(fake, "stg-cluster", "stg", true)
	m.sidebar.setServices(fake.Services)
	m, _ = applySize(m, 120, 40)

	// abrir deploy del 1er servicio (api) y teclear tag
	m = mustUpdate(t, m, keyMsg("d"))
	require.Equal(t, focusAction, m.focus)
	for _, r := range "v2" {
		m = mustUpdate(t, m, keyMsg(string(r)))
	}
	// enter ejecuta: devuelve startDeployCmd y salta a Events
	updated, cmd := m.Update(keyMsg("enter"))
	m = updated.(Model)
	require.Equal(t, panel.TabEvents, m.tabs.Active)
	require.NotNil(t, cmd)

	started := cmd().(deployStartedMsg)
	require.NoError(t, started.err)
	require.Equal(t, []string{"stg-cluster/api/v2"}, fake.DeployCalls)

	updated, cmd = m.Update(started)
	m = updated.(Model)
	require.NotNil(t, cmd) // primer poll

	poll := cmd().(deployPollMsg)
	updated, _ = m.Update(poll)
	m = updated.(Model)
	require.True(t, m.deployDone)
	require.Contains(t, m.events.View(), "completed")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestDeployFlow`
Expected: FAIL (deploy no alimenta Events; `deployStartedMsg` no se maneja en app).

- [ ] **Step 3: Write minimal implementation**

En `runActionCmd`, cambia la rama `actionDeploy` para iniciar el flujo en vivo:

```go
// reemplaza la rama actionDeploy dentro de runActionCmd:
		case actionDeploy:
			// el flujo en vivo se arranca fuera (ver openDeploy); aquí no debería entrar
			return actionDoneMsg{} // (deploy va por startDeployCmd, no por runActionCmd)
```

Mejor: separa el deploy del resto. En `handleKey`, cuando `focus==focusAction` y `Enter` con `action.kind==actionDeploy`, arranca el flujo:

```go
		case key.Matches(msg, m.keys.Enter):
			if !m.action.ready() {
				return m, nil
			}
			if m.action.kind == actionDeploy {
				svc, tag := m.action.service, m.action.input
				m.action.close()
				m.focus = focusPanel
				m.tabs.Active = panel.TabEvents
				m.events.Reset()
				m.deployActive, m.deployDone = true, false
				return m, startDeployCmd(m.dep, m.cluster, svc, tag)
			}
			return m, m.runActionCmd()
```

Y añade los cases de deploy a `Update`:

```go
	case deployStartedMsg:
		for _, s := range msg.steps {
			m.events.AppendLine(render.Dim("[*] " + s))
		}
		if msg.err != nil {
			m.events.AppendLine(render.Danger("error: " + msg.err.Error()))
			m.deployDone = true
			return m, nil
		}
		m.deployLastID = msg.lastID
		return m, deployPollCmd(m.dep, m.cluster, m.action.service, m.deployLastID)

	case deployPollMsg:
		if msg.err != nil {
			m.events.AppendLine(render.Danger("error: " + msg.err.Error()))
			m.deployDone = true
			return m, m.loadServicesCmd()
		}
		for i := len(msg.events) - 1; i >= 0; i-- {
			e := msg.events[i]
			m.events.AppendLine(render.Dim("[" + e.At.Format("15:04:05") + "] " + e.Message))
		}
		m.deployLastID = msg.lastID
		m.events.SetStatusLine("Rollout: " + rolloutColored(msg.rollout) +
			" | Running: " + strconv.Itoa(msg.running) +
			" | Pending: " + strconv.Itoa(msg.pending) +
			" | Desired: " + strconv.Itoa(msg.desired))
		if msg.done {
			m.events.AppendLine(render.Success("✓ deployment completed"))
			m.deployDone = true
			return m, m.loadServicesCmd()
		}
		if msg.failed {
			m.events.AppendLine(render.Danger("✗ deployment failed"))
			m.deployDone = true
			return m, m.loadServicesCmd()
		}
		return m, deployTickCmd()

	case deployPollTickMsg:
		if m.deployActive && !m.deployDone {
			return m, deployPollCmd(m.dep, m.cluster, m.action.service, m.deployLastID)
		}
		return m, nil
```

Nota: `m.action.service` se usa en el poll; como `action.close()` resetea `active` pero NO borra `service`, sigue disponible. Si lo borra, guarda `m.deployService string` al arrancar y úsalo en los polls. Añade ese campo si es necesario para que el test pase.

Añade `rolloutColored` a app.go (venía de model.go):

```go
func rolloutColored(state string) string {
	switch state {
	case "COMPLETED":
		return render.Success(state)
	case "FAILED":
		return render.Danger(state)
	default:
		return render.Accent(state)
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/tui/... && go build ./...`
Expected: PASS. Si el test falla porque `action.service` se perdió, añade `deployService` al `Model` y úsalo en los polls.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/app.go internal/tui/app_test.go
git commit -m "feat(tui): deploy en vivo dentro de la pestaña Events

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

### Task 11: Activar mouse en run.go y verificación final

**Files:**
- Modify: `internal/tui/run.go`
- Test: `internal/cli/tui_cmd_test.go` (verificar que sigue verde)

**Interfaces:**
- Consumes: `tea.WithMouseCellMotion`, `tea.WithAltScreen`.
- Produces: `Run` arranca el `Program` con mouse activado.

- [ ] **Step 1: Modify run.go**

```go
// internal/tui/run.go
package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/juanMaAV92/steer/internal/core"
)

// Run abre la TUI a pantalla completa con soporte de mouse hasta que el usuario sale.
func Run(dep core.Deployer, cluster, env string, writable bool) error {
	p := tea.NewProgram(
		New(dep, cluster, env, writable),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	_, err := p.Run()
	return err
}
```

- [ ] **Step 2: Run the full suite and build**

Run: `go test ./... && go build ./...`
Expected: PASS en todo el módulo (incl. `internal/cli/tui_cmd_test.go`).

- [ ] **Step 3: Smoke manual (opcional pero recomendado)**

Run: `go run ./cmd/steer tui -e staging` (o el binario equivalente del proyecto). Verifica visualmente: dos paneles con bordes, foco conmuta con `tab`, click selecciona servicio, rueda scrollea Events, `d` abre overlay de deploy, `q` sale. Documenta cualquier desajuste de offset de mouse y corrígelo en `handleMouse`.

- [ ] **Step 4: Commit**

```bash
git add internal/tui/run.go
git commit -m "feat(tui): activar soporte de mouse en el programa

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Notas de ejecución

- El orden importa: Task 2 (extraer comandos/mensajes) debe ir antes de Task 8 (que borra model.go) para no perder esas definiciones.
- Tasks 7 y 8 comparten los identificadores `actionKind`/constantes: en Task 7 se reutilizan los de model.go; en Task 8, al borrar model.go, se redefinen en app.go. Si al ejecutar Task 7 de forma aislada el build se queja, es esperado que quede verde recién tras Task 8 — ejecútalas en secuencia y valida el build al cerrar Task 8.
- Calibración de mouse (Task 9/11): los offsets de fila dependen del render real de lipgloss (bordes). El smoke manual es la forma fiable de afinarlos.
