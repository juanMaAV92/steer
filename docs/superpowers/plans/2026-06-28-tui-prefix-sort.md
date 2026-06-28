# TUI Prefix Strip & Alpha Sort Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Strip the cluster/env prefix from service display names in the TUI sidebar and Details tab, and sort services alphabetically by stripped name.

**Architecture:** Thread a `prefix string` from `tui_cmd.go` → `Run` → `New` → `sidebar`. The sidebar strips the prefix only when rendering (`view()`) and when sorting (`setServices`). The real `core.ServiceStatus.Name` is never mutated, so all AWS actions (deploy/scale/rollback) keep using the full name. `DetailsView` receives an explicit `displayName string` parameter to show the short name in the Details header.

**Tech Stack:** Go 1.22+, Bubble Tea TUI, `strings.TrimPrefix`, `sort.SliceStable`

## Global Constraints

- Do NOT modify `internal/core` or `internal/providers`
- Do NOT mutate `core.ServiceStatus.Name` — only change rendered strings
- Reuse `internal/render`; UI strings English, comments Spanish
- `gofmt -w internal/tui/ internal/cli/` and `go vet ./...` before committing
- `go test ./...` and `go build ./...` must pass
- NO Claude/AI authorship in commit messages or code comments
- Commit message: `feat(tui): ocultar prefijo del cluster y ordenar servicios alfabéticamente`

---

## File Map

| File | Change |
|------|--------|
| `internal/cli/tui_cmd.go` | Compute prefix via `Naming.Service(env, "")` and pass to `tui.Run` |
| `internal/tui/run.go` | Add `prefix string` parameter to `Run`, pass to `New` |
| `internal/tui/app.go` | Add `prefix string` to `Model` and `New()`; store prefix on sidebar; pass `displayName` to `DetailsView` |
| `internal/tui/sidebar.go` | Add `prefix string` field; sort in `setServices`; strip in `view()` |
| `internal/tui/panel/details.go` | Add `displayName string` parameter; show it as header instead of `s.Name` |
| `internal/tui/sidebar_test.go` | Update existing tests for new sort order; add prefix-strip test |
| `internal/tui/app_test.go` | Pass `prefix ""` to `New` in `newTestModel`; fix `TestMouseClickSelectsSidebarService` cursor |
| `internal/tui/panel/details_test.go` | Update `DetailsView` call to pass `displayName` |
| `cmd/steerdemo/main.go` | Pass prefix to `tui.Run`; update service names to demonstrate stripping |

---

### Task 1: Update `sidebar.go` — add prefix field, sort, and strip

**Files:**
- Modify: `internal/tui/sidebar.go`

**Interfaces:**
- Consumes: `core.ServiceStatus.Name` (full AWS name, never mutated)
- Produces: `sidebar` struct with new field `prefix string`; `setServices` sorts by stripped name; `view()` renders stripped names

- [ ] **Step 1: Read the current file**

```
Read internal/tui/sidebar.go
```
(Already read above — proceed.)

- [ ] **Step 2: Write the updated `sidebar.go`**

Replace the entire file content with:

```go
package tui

import (
	"sort"
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
	prefix        string // prefijo a ocultar en la visualización (ej. "nao-v2-dev-")
}

func newSidebar() sidebar { return sidebar{} }

// setServices guarda los servicios y los ordena alfabéticamente por nombre de visualización.
func (s *sidebar) setServices(svc []core.ServiceStatus) {
	// copiar para no mutar el slice original
	sorted := make([]core.ServiceStatus, len(svc))
	copy(sorted, svc)
	sort.SliceStable(sorted, func(i, j int) bool {
		di := strings.ToLower(strings.TrimPrefix(sorted[i].Name, s.prefix))
		dj := strings.ToLower(strings.TrimPrefix(sorted[j].Name, s.prefix))
		return di < dj
	})
	s.services = sorted
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
		displayName := strings.TrimPrefix(svc.Name, s.prefix)
		b.WriteString(cursor + render.Symbol(render.StatusLevel(svc.Running, svc.Desired)) + " " +
			displayName + "  " + strconv.Itoa(svc.Running) + "/" + strconv.Itoa(svc.Desired) +
			"  " + render.Dim(tag) + "\n")
	}
	b.WriteString("\n" + render.Dim("IMAGES (ECR)") + "\n" + render.Dim("  (próximamente)") + "\n")
	b.WriteString("\n" + render.Dim("DATABASES") + "\n" + render.Dim("  (próximamente)") + "\n")
	return b.String()
}
```

- [ ] **Step 3: Run `gofmt`**

```bash
gofmt -w /Users/juanmaav92/Documents/juanMa/steer/internal/tui/sidebar.go
```

---

### Task 2: Update `panel/details.go` — add `displayName` parameter

**Files:**
- Modify: `internal/tui/panel/details.go`

**Interfaces:**
- Produces: `DetailsView(s core.ServiceStatus, writable bool, displayName string) string` — shows `displayName` in the bold header instead of `s.Name`. All stats still come from `s`.

- [ ] **Step 1: Write the updated `details.go`**

```go
package panel

import (
	"strconv"
	"strings"

	"github.com/juanMaAV92/steer/internal/core"
	"github.com/juanMaAV92/steer/internal/render"
)

// DetailsView renderiza la pestaña Details con stats y la fila de acciones.
// displayName es el nombre de visualización (sin prefijo de entorno).
func DetailsView(s core.ServiceStatus, writable bool, displayName string) string {
	var b strings.Builder
	b.WriteString(render.Bold(displayName) + "\n\n")
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

- [ ] **Step 2: Run `gofmt`**

```bash
gofmt -w /Users/juanmaav92/Documents/juanMa/steer/internal/tui/panel/details.go
```

---

### Task 3: Update `app.go` — store prefix, pass displayName to DetailsView

**Files:**
- Modify: `internal/tui/app.go`

**Interfaces:**
- Consumes: `sidebar.prefix string` (new field), `panel.DetailsView(s, writable, displayName)`
- Produces: `New(dep, cluster, env string, writable bool, prefix string) Model`

- [ ] **Step 1: Add `prefix` to `Model` struct and `New` function**

In `app.go`, find the `Model` struct definition and add a `prefix string` field after `writable bool`:

```go
// Model es el estado raíz de la TUI (patrón Elm de Bubble Tea).
type Model struct {
	dep      core.Deployer
	cluster  string
	env      string
	writable bool
	prefix   string // prefijo de entorno a ocultar en la visualización
	keys     keyMap
	// ... rest unchanged
```

- [ ] **Step 2: Update `New` signature and body**

```go
func New(dep core.Deployer, cluster, env string, writable bool, prefix string) Model {
	sb := newSidebar()
	sb.prefix = prefix
	return Model{
		dep: dep, cluster: cluster, env: env, writable: writable, prefix: prefix,
		keys: defaultKeys(), sidebar: sb, events: panel.NewEvents(),
		loading: true,
	}
}
```

- [ ] **Step 3: Update `panelBody` to pass displayName to DetailsView**

Find the `panelBody` method and update the `default` case:

```go
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
		displayName := strings.TrimPrefix(s.Name, m.prefix)
		return panel.DetailsView(s, m.writable, displayName)
	}
}
```

Note: `strings` import must be present. Check the import block in `app.go` — it doesn't currently import `strings`. Add it:

```go
import (
	"context"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/juanMaAV92/steer/internal/core"
	"github.com/juanMaAV92/steer/internal/render"
	"github.com/juanMaAV92/steer/internal/tui/panel"
)
```

- [ ] **Step 4: Run `gofmt`**

```bash
gofmt -w /Users/juanmaav92/Documents/juanMa/steer/internal/tui/app.go
```

---

### Task 4: Update `run.go` — add prefix parameter

**Files:**
- Modify: `internal/tui/run.go`

**Interfaces:**
- Produces: `Run(dep core.Deployer, cluster, env string, writable bool, prefix string) error`

- [ ] **Step 1: Write the updated `run.go`**

```go
package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/juanMaAV92/steer/internal/core"
)

// Run abre la TUI a pantalla completa con soporte de mouse hasta que el usuario sale.
func Run(dep core.Deployer, cluster, env string, writable bool, prefix string) error {
	p := tea.NewProgram(
		New(dep, cluster, env, writable, prefix),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	_, err := p.Run()
	return err
}
```

- [ ] **Step 2: Run `gofmt`**

```bash
gofmt -w /Users/juanmaav92/Documents/juanMa/steer/internal/tui/run.go
```

---

### Task 5: Update `tui_cmd.go` — compute prefix and pass to Run

**Files:**
- Modify: `internal/cli/tui_cmd.go`

**Interfaces:**
- Consumes: `app.Config.Providers.AWS.Naming.Service(app.EnvName, "")` → `prefix string`
- Produces: calls `tui.Run(dep, cluster, app.EnvName, app.Env.Writable, prefix)`

- [ ] **Step 1: Write the updated `tui_cmd.go`**

```go
package cli

import (
	"github.com/juanMaAV92/steer/internal/tui"
	"github.com/spf13/cobra"
)

// NewTuiCmd construye `steer tui`.
func NewTuiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Open the interactive dashboard",
		RunE: func(cmd *cobra.Command, _ []string) error {
			app := FromContext(cmd.Context())
			dep, cluster, err := newDeployerFn(app)
			if err != nil {
				return err
			}
			// prefijo del servicio para ocultar en la visualización (ej. "nao-v2-dev-")
			prefix := app.Config.Providers.AWS.Naming.Service(app.EnvName, "")
			return tui.Run(dep, cluster, app.EnvName, app.Env.Writable, prefix)
		},
	}
}
```

- [ ] **Step 2: Run `gofmt`**

```bash
gofmt -w /Users/juanmaav92/Documents/juanMa/steer/internal/cli/tui_cmd.go
```

---

### Task 6: Update `cmd/steerdemo/main.go` — compile fix + showcase prefix stripping

**Files:**
- Modify: `cmd/steerdemo/main.go`

**Interfaces:**
- Consumes: `tui.Run` with new 5th arg `prefix string`

- [ ] **Step 1: Write the updated `main.go`**

Use full prefixed names (`nao-v2-demo-api`, etc.) and pass `prefix="nao-v2-demo-"` so the demo showcases stripping:

```go
// Command steerdemo abre la TUI con datos en memoria (sin AWS) para probar la
// interfaz localmente. Es una utilidad local; no forma parte del binario steer.
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/juanMaAV92/steer/internal/core"
	"github.com/juanMaAV92/steer/internal/core/coretest"
	"github.com/juanMaAV92/steer/internal/tui"
)

func main() {
	now := time.Now()
	fake := &coretest.FakeDeployer{
		Services: []core.ServiceStatus{
			{Name: "nao-v2-demo-api", Running: 2, Desired: 2, Pending: 0, Status: "ACTIVE", Tag: "v1.4"},
			{Name: "nao-v2-demo-web", Running: 3, Desired: 3, Pending: 0, Status: "ACTIVE", Tag: "v2.0"},
			{Name: "nao-v2-demo-worker", Running: 1, Desired: 2, Pending: 1, Status: "ACTIVE", Tag: "v1.1"},
			{Name: "nao-v2-demo-cron", Running: 0, Desired: 1, Pending: 0, Status: "ACTIVE", Tag: ""},
		},
		DeploymentValue: core.Deployment{Rollout: "COMPLETED", Running: 2, Desired: 2},
		Events: []core.ServiceEvent{
			{ID: "3", At: now, Message: "(service nao-v2-demo-api) has reached a steady state."},
			{ID: "2", At: now.Add(-30 * time.Second), Message: "(service nao-v2-demo-api) registered 2 targets."},
			{ID: "1", At: now.Add(-60 * time.Second), Message: "(service nao-v2-demo-api) has started 2 tasks."},
		},
	}
	if err := tui.Run(fake, "demo-cluster", "demo", true, "nao-v2-demo-"); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
```

---

### Task 7: Update `sidebar_test.go` — fix sort order + add prefix-strip test

**Files:**
- Modify: `internal/tui/sidebar_test.go`

**Context:** `sampleServices()` currently returns `[api, web, worker]`. The spec says to add `cron` to the sample set (matching the current `app_test.go` usage). After alpha sort with no prefix, the order is `api, cron, web, worker` (indices 0, 1, 2, 3).

Changes needed:
1. `sampleServices()`: add `cron` (index 3 in unsorted input → index 1 after sort)
2. `TestSidebarNavigationClamps`: 3× moveDown lands at index 3 (`worker`), not index 2
3. `TestSidebarSelectIndex`: index 1 now maps to `cron` (not `web`)
4. `TestSidebarSetServicesReclampsCursor`: `selectIndex(2)` then shrink — still correct (just needs to pass)
5. `TestSidebarViewListsServicesAndSections`: add `cron` check
6. NEW `TestSidebarPrefixStrip`: prefix stripping test
7. NEW `TestSidebarSortOrder`: explicit sort-order test

- [ ] **Step 1: Write the updated `sidebar_test.go`**

```go
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
		{Name: "cron", Running: 0, Desired: 1, Tag: ""},
	}
}

// TestSidebarNavigationClamps verifica que el cursor no excede los límites.
// Orden alfabético post-sort: api(0), cron(1), web(2), worker(3).
func TestSidebarNavigationClamps(t *testing.T) {
	s := newSidebar()
	s.setServices(sampleServices())
	require.Equal(t, 0, s.cursor)
	s.moveUp() // clamp en 0
	require.Equal(t, 0, s.cursor)
	s.moveDown()
	s.moveDown()
	s.moveDown()
	s.moveDown() // clamp en 3 (worker es el último)
	require.Equal(t, 3, s.cursor)
	sel, ok := s.selected()
	require.True(t, ok)
	require.Equal(t, "worker", sel.Name)
}

// TestSidebarSelectIndex verifica selección directa por índice.
// Orden: api(0), cron(1), web(2), worker(3).
func TestSidebarSelectIndex(t *testing.T) {
	s := newSidebar()
	s.setServices(sampleServices())
	s.selectIndex(1)
	sel, _ := s.selected()
	require.Equal(t, "cron", sel.Name) // índice 1 = cron tras el sort
	s.selectIndex(99)                  // fuera de rango: no-op
	sel, _ = s.selected()
	require.Equal(t, "cron", sel.Name)
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
	require.Contains(t, out, "cron")
	require.Contains(t, out, "IMAGES")
	require.Contains(t, strings.ToLower(out), "próximamente")
}

// TestSidebarSortOrder verifica que los servicios se muestran en orden alfabético.
func TestSidebarSortOrder(t *testing.T) {
	s := newSidebar()
	// entregar fuera de orden: worker, api, cron, web
	s.setServices([]core.ServiceStatus{
		{Name: "worker"},
		{Name: "api"},
		{Name: "cron"},
		{Name: "web"},
	})
	require.Equal(t, "api", s.services[0].Name)
	require.Equal(t, "cron", s.services[1].Name)
	require.Equal(t, "web", s.services[2].Name)
	require.Equal(t, "worker", s.services[3].Name)
}

// TestSidebarPrefixStrip verifica que el prefijo se oculta en la visualización
// pero el Name real permanece intacto para las acciones.
func TestSidebarPrefixStrip(t *testing.T) {
	s := newSidebar()
	s.prefix = "nao-v2-dev-"
	s.setServices([]core.ServiceStatus{
		{Name: "nao-v2-dev-audit-ms", Running: 1, Desired: 1, Tag: "v3"},
		{Name: "nao-v2-dev-billing", Running: 2, Desired: 2, Tag: "v1"},
	})

	out := s.view()

	// los nombres cortos deben aparecer
	require.Contains(t, out, "audit-ms")
	require.Contains(t, out, "billing")

	// los nombres completos NO deben aparecer en la visualización
	require.NotContains(t, out, "nao-v2-dev-audit-ms")
	require.NotContains(t, out, "nao-v2-dev-billing")

	// el Name real (con prefijo) sigue intacto en el slice
	sel, ok := s.selected()
	require.True(t, ok)
	require.Equal(t, "nao-v2-dev-audit-ms", sel.Name) // primer servicio alfabéticamente: audit-ms
}
```

- [ ] **Step 2: Run the tests to see them fail (expected at this point)**

```bash
cd /Users/juanmaav92/Documents/juanMa/steer && go test ./internal/tui/ -run "TestSidebarNavigationClamps|TestSidebarSelectIndex|TestSidebarSortOrder|TestSidebarPrefixStrip" -v 2>&1 | tail -20
```

Expected: some pass, some fail — proceed after confirming tests exist and compile.

---

### Task 8: Update `panel/details_test.go` — pass displayName arg

**Files:**
- Modify: `internal/tui/panel/details_test.go`

- [ ] **Step 1: Write the updated `details_test.go`**

```go
package panel

import (
	"strings"
	"testing"

	"github.com/juanMaAV92/steer/internal/core"
	"github.com/stretchr/testify/require"
)

func TestDetailsViewShowsStatsAndActions(t *testing.T) {
	s := core.ServiceStatus{Name: "nao-v2-dev-api", Running: 2, Desired: 2, Pending: 0, Status: "ACTIVE", Tag: "v1.4"}
	out := DetailsView(s, true, "api")
	require.Contains(t, out, "2/2")
	require.Contains(t, out, "ACTIVE")
	require.Contains(t, out, "v1.4")
	require.Contains(t, strings.ToLower(out), "deploy")
	require.Contains(t, strings.ToLower(out), "scale")
	require.Contains(t, strings.ToLower(out), "rollback")
	// el nombre visible es el corto
	require.Contains(t, out, "api")
}

func TestDetailsViewReadOnlyHint(t *testing.T) {
	s := core.ServiceStatus{Name: "nao-v2-dev-api", Running: 1, Desired: 1}
	out := DetailsView(s, false, "api")
	require.Contains(t, strings.ToLower(out), "read-only")
}

// TestDetailsViewDisplayName verifica que el displayName se muestra en lugar del Name completo.
func TestDetailsViewDisplayName(t *testing.T) {
	s := core.ServiceStatus{Name: "nao-v2-dev-audit-ms", Running: 2, Desired: 2, Tag: "v1"}
	out := DetailsView(s, true, "audit-ms")
	require.Contains(t, out, "audit-ms")
	require.NotContains(t, out, "nao-v2-dev-audit-ms")
}
```

---

### Task 9: Update `app_test.go` — fix `New` calls + mouse-click cursor

**Files:**
- Modify: `internal/tui/app_test.go`

**Context:**
- `newTestModel` calls `New(...)` — must now pass `prefix ""` as 5th arg
- `TestReadOnlyBlocksActions` calls `New(...)` directly — same fix
- `TestDeployFlowFeedsEventsPanel` calls `New(...)` directly — same fix
- `TestMouseClickSelectsSidebarService`: uses `sampleServices()[1].Name` = `"web"`. After alpha sort with `""` prefix, order is `api(0), cron(1), web(2), worker(3)`. So "web" is at sorted index 2. The test already derives `targetY` from the render — it will still find "web" on the correct line. The final assertion `require.Equal(t, 1, m.sidebar.cursor)` must change to `require.Equal(t, 2, m.sidebar.cursor)`.

- [ ] **Step 1: Update `newTestModel`**

Find:
```go
func newTestModel(services []core.ServiceStatus) Model {
	m := New(&coretest.FakeDeployer{Services: services}, "stg-cluster", "stg", true)
```
Replace with:
```go
func newTestModel(services []core.ServiceStatus) Model {
	m := New(&coretest.FakeDeployer{Services: services}, "stg-cluster", "stg", true, "")
```

- [ ] **Step 2: Update `TestReadOnlyBlocksActions`**

Find:
```go
ro := New(&coretest.FakeDeployer{Services: sampleServices()}, "prod-cluster", "production", false)
```
Replace with:
```go
ro := New(&coretest.FakeDeployer{Services: sampleServices()}, "prod-cluster", "production", false, "")
```

- [ ] **Step 3: Update `TestDeployFlowFeedsEventsPanel`**

Find:
```go
m := New(fake, "stg-cluster", "stg", true)
```
Replace with:
```go
m := New(fake, "stg-cluster", "stg", true, "")
```

- [ ] **Step 4: Update `TestMouseClickSelectsSidebarService` cursor assertion**

Find:
```go
require.Equal(t, 1, m.sidebar.cursor)
```
Replace with:
```go
require.Equal(t, 2, m.sidebar.cursor)
```

- [ ] **Step 5: Run `gofmt`**

```bash
gofmt -w /Users/juanmaav92/Documents/juanMa/steer/internal/tui/app_test.go
```

---

### Task 10: Run all tests and build, then commit

**Files:** none (verification + commit)

- [ ] **Step 1: Run `gofmt` on all changed dirs**

```bash
gofmt -w /Users/juanmaav92/Documents/juanMa/steer/internal/tui/ /Users/juanmaav92/Documents/juanMa/steer/internal/cli/
```

- [ ] **Step 2: Run `go vet`**

```bash
cd /Users/juanmaav92/Documents/juanMa/steer && go vet ./...
```

Expected: no output (no errors).

- [ ] **Step 3: Run all tests**

```bash
cd /Users/juanmaav92/Documents/juanMa/steer && go test ./...
```

Expected: `ok` for all packages.

- [ ] **Step 4: Run build**

```bash
cd /Users/juanmaav92/Documents/juanMa/steer && go build ./...
```

Expected: no output (success).

- [ ] **Step 5: Write the report**

Write a brief report to `/Users/juanmaav92/Documents/juanMa/steer/.superpowers/sdd/enh-prefix-sort-report.md` documenting what changed, how the prefix is threaded, which tests changed and why, and evidence that `go test`/`build` pass.

- [ ] **Step 6: Stage and commit**

```bash
cd /Users/juanmaav92/Documents/juanMa/steer && git add \
  internal/tui/sidebar.go \
  internal/tui/sidebar_test.go \
  internal/tui/app.go \
  internal/tui/app_test.go \
  internal/tui/run.go \
  internal/tui/panel/details.go \
  internal/tui/panel/details_test.go \
  internal/cli/tui_cmd.go \
  cmd/steerdemo/main.go \
  .superpowers/sdd/enh-prefix-sort-report.md
```

```bash
cd /Users/juanmaav92/Documents/juanMa/steer && git commit -m "feat(tui): ocultar prefijo del cluster y ordenar servicios alfabéticamente"
```

---

## Self-Review Checklist

**Spec coverage:**
- [x] Display-only prefix strip in sidebar `view()` — Task 1
- [x] Display-only prefix strip in Details header — Tasks 2 + 3
- [x] `core.ServiceStatus.Name` never mutated — prefix only used in render calls
- [x] Alpha sort in `setServices` by stripped name, case-insensitive — Task 1
- [x] `prefix` threaded: `tui_cmd.go` → `Run` → `New` → `sidebar` — Tasks 3–5
- [x] `TestSidebarNavigationClamps` updated (4× moveDown, cursor=3, worker) — Task 7
- [x] `TestSidebarSelectIndex` updated (index 1 = cron) — Task 7
- [x] `TestSidebarSetServicesReclampsCursor` still valid — Task 7
- [x] New prefix-strip test — Task 7
- [x] New sort-order test — Task 7
- [x] `app_test.go` `New` calls updated — Task 9
- [x] `TestMouseClickSelectsSidebarService` cursor updated to 2 — Task 9
- [x] `panel/details_test.go` `DetailsView` calls updated — Task 8
- [x] `steerdemo/main.go` compiles + showcases stripping — Task 6
- [x] `gofmt` + `go vet` + `go test` + `go build` — Task 10
- [x] Report written — Task 10

**Type consistency:**
- `DetailsView(s core.ServiceStatus, writable bool, displayName string)` — used consistently in Task 2 (definition), Task 3 (call site in `panelBody`), Task 8 (test), and Task 9 (no direct call — via `panelBody`).
- `New(dep, cluster, env string, writable bool, prefix string)` — defined in Task 3, called in Task 4 (`run.go`), Task 5 (`tui_cmd.go`), Task 9 (tests).
- `Run(dep, cluster, env string, writable bool, prefix string)` — defined in Task 4, called in Task 5 and Task 6.
- `sidebar.prefix string` — set in Task 1 (struct + `setServices` + `view`), initialized via `New` in Task 3.

**No placeholders:** verified — all steps contain actual code.
