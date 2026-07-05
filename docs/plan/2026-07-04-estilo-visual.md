# Pulido visual del TUI — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implementar el diseño `2026-07-04-estilo-visual-diseno.md`: marco único (reglas + divisor vertical, foco por header), aire con padding, top bar con icono y estado alineado a la derecha, headers de sección estilizados y stubs en inglés — sin cambiar ningún flujo, tecla ni click.

**Architecture:** T1 restyle del top bar + helpers de reglas (`hrule`/`vdivider` en context.go). T2 restyle del sidebar (header con foco, count/`···` a la derecha, línea en blanco tras el header → `HitAtRow` se desplaza una fila). T3 el ensamblaje: `View()`/`layout()` sin cajas, divisor vertical, ajuste de las constantes X del mouse (−1 en el panel) y del offset Y del picker (+1 por la regla), y limpieza de estilos muertos. Los tests anclados al render son el guard en cada paso.

**Tech Stack:** Go 1.26, Bubble Tea, Lipgloss, testify.

## Global Constraints

- Comportamiento observable idéntico salvo lo visual: mismas teclas, mismos clicks, mismos flujos (deploy en vivo, switch, overlays). Los tests anclados al render se recalibran solos (derivan coordenadas del render) — NO se debilitan aserciones.
- `render` sigue hoja; comentarios en español, UI strings en inglés; sin autoría de Claude en commits.
- Antes de cada commit: `gofmt -w internal/ && golangci-lint run && go vet ./... && go test ./... -count=1 && go build ./...` — todo verde.
- Los botones `[ Deploy (d) ]…`, el contenido del help bar y los overlays NO cambian.

## File Structure

```
internal/tui/context.go       ← topBar(width,...) con icono + estado a la derecha; hrule; vdivider
internal/tui/context_test.go
internal/tui/sidebar.go       ← view(focused bool); sectionHeader; "coming soon"; HitAtRow +1
internal/tui/sidebar_test.go
internal/tui/app.go           ← layout()/View() marco único; constantes X del mouse; borderTop=regla
internal/tui/overlay.go       ← offset Y del picker (+ regla)
internal/tui/styles.go        ← borra focusedBorder/blurredBorder; + colorDivider
internal/tui/app_test.go      ← solo donde el render cambió (headers/one-liners)
```

---

### Task 1: Top bar con icono, `·` y estado alineado a la derecha + helpers de reglas

**Files:**
- Modify: `internal/tui/context.go`, `internal/tui/context_test.go`
- Modify: `internal/tui/app.go` (solo el call site de `topBar` en `View()`, pasa `m.width`)

**Interfaces:**
- Produces:
  - `func topBar(width int, cloud, env, cluster string, writable bool) string` — `⛵ steer · aws · env (cluster: X)` a la izquierda + relleno + `writable ●`/`read-only ○` a la derecha; ancho de display exacto `width` (relleno con `lipgloss.Width`, clamp ≥1).
  - `func hrule(width int) string` — regla horizontal tenue (`─` × width, `render.Dim`).
  - `func vdivider(height int) string` — columna de `│` tenues, `height` filas (usada en T3).
  - `const brandIcon = "⛵"`.

- [ ] **Step 1: Write the failing tests**

```go
// reemplaza/añade en internal/tui/context_test.go
func TestTopBarShowsContext(t *testing.T) {
	out := topBar(100, "aws", "staging", "staging-cluster", true)
	require.Contains(t, out, "aws")
	require.Contains(t, out, "staging")
	require.Contains(t, out, "staging-cluster")
	require.Contains(t, strings.ToLower(out), "writable")
	require.Contains(t, out, brandIcon)
}

func TestTopBarReadOnly(t *testing.T) {
	out := topBar(100, "aws", "prod", "prod-cluster", false)
	require.Contains(t, strings.ToLower(out), "read-only")
}

// El estado queda alineado a la derecha: el ancho de display es exactamente width.
func TestTopBarRightAlignsState(t *testing.T) {
	out := topBar(100, "aws", "dev", "c", true)
	require.Equal(t, 100, lipgloss.Width(out))
	require.True(t, strings.HasSuffix(stripANSI(out), "writable ●"))
}

func TestHruleAndVdivider(t *testing.T) {
	require.Equal(t, 10, lipgloss.Width(hrule(10)))
	require.Equal(t, "", hrule(0))
	d := vdivider(3)
	require.Equal(t, 3, strings.Count(d, "│"))
	require.Equal(t, 2, strings.Count(d, "\n"))
}
```

(Añade los imports `"strings"` y `"github.com/charmbracelet/lipgloss"` al test si faltan; `stripANSI` ya existe en el paquete.)

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/tui/ -run 'TestTopBar|TestHrule'`
Expected: FAIL (firma nueva / helpers no definidos).

- [ ] **Step 3: Implement**

Reemplaza `topBar` en `internal/tui/context.go` y añade los helpers:

```go
import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/juanMaAV92/steer/internal/render"
)

// brandIcon es el glifo de marca del top bar.
const brandIcon = "⛵"

// topBar renderiza la barra de contexto: icono + wordmark + contexto a la izquierda
// y el estado writable/read-only alineado a la derecha (ancho de display = width).
func topBar(width int, cloud, env, cluster string, writable bool) string {
	state := render.Success("writable ●")
	if !writable {
		state = render.Warn("read-only ○")
	}
	left := brandIcon + " " + render.Brand("steer") + render.Dim(" · "+cloud+" · ") +
		render.Accent(env) + render.Dim(" (cluster: "+cluster+")")
	fill := width - lipgloss.Width(left) - lipgloss.Width(state)
	if fill < 1 {
		fill = 1
	}
	return left + strings.Repeat(" ", fill) + state
}

// hrule dibuja una regla horizontal tenue de ancho width.
func hrule(width int) string {
	if width < 1 {
		return ""
	}
	return render.Dim(strings.Repeat("─", width))
}

// vdivider dibuja el divisor vertical (una columna de │ tenues, height filas).
func vdivider(height int) string {
	col := render.Dim("│")
	rows := make([]string, height)
	for i := range rows {
		rows[i] = col
	}
	return strings.Join(rows, "\n")
}
```

En `internal/tui/app.go` `View()`, actualiza el call site: `top := topBar(m.width, m.current.Cloud, m.current.Name, m.current.Cluster, m.current.Writable)`.

- [ ] **Step 4: Run to verify pass**

Run: `gofmt -w internal/tui/ && golangci-lint run && go test ./internal/tui/... -count=1 && go build ./...`
Expected: PASS (`vdivider` queda sin uso hasta T3 — si el linter `unused` lo marca, añádele un uso trivial en un test como el de arriba, que ya lo cubre).

- [ ] **Step 5: Commit**

```bash
git add internal/tui/
git commit -m "feat(tui): top bar con icono y estado a la derecha; helpers de reglas"
```

---

### Task 2: Sidebar estilizado — header con foco, sufijos a la derecha, "coming soon"

**Files:**
- Modify: `internal/tui/sidebar.go`, `internal/tui/sidebar_test.go`, `internal/tui/app.go` (call sites de `m.sidebar.view(...)`), `internal/tui/app_test.go` (asserts de "próximamente" si existen)

**Interfaces:**
- Produces:
  - `func (s sidebar) view(focused bool) string` — header `SERVICES` en `render.Brand` si `focused`, `render.Dim` si no; count `(n)` alineado a la derecha; **línea en blanco tras el header**; secciones `IMAGES (ECR)`/`DATABASES` con `···` a la derecha y stub `coming soon`.
  - `func (s sidebar) sectionHeader(title, suffix string, focused bool) string`.
  - `HitAtRow`: servicios pasan a filas **2..n+1** (fila 0 header, fila 1 en blanco).

- [ ] **Step 1: Update the failing tests**

En `sidebar_test.go`: actualiza `TestHitAtRow` (servicios en 2..5 con 4 servicios; filas 0,1 y 6+ no accionables) y los asserts de view:

```go
func TestHitAtRow(t *testing.T) {
	s := newSidebar()
	s.setServices(sampleServices())
	for _, row := range []int{-1, 0, 1, 6, 99} {
		_, ok := s.HitAtRow(row)
		require.False(t, ok, "row %d", row)
	}
	hit, ok := s.HitAtRow(2)
	require.True(t, ok)
	require.Equal(t, sectionServices, hit.Section)
	require.Equal(t, 0, hit.Index)
	hit, ok = s.HitAtRow(5)
	require.True(t, ok)
	require.Equal(t, 3, hit.Index)
}

func TestSidebarViewStyledSections(t *testing.T) {
	s := newSidebar()
	s.width = 30
	s.setServices(sampleServices())
	out := stripANSI(s.view(true))
	require.Contains(t, out, "SERVICES")
	require.Contains(t, out, "(4)")
	require.Contains(t, out, "coming soon")
	require.NotContains(t, out, "próximamente")
	require.Contains(t, out, "···")
	// línea en blanco tras el header: la fila 1 está vacía y la 2 tiene el primer servicio
	lines := strings.Split(out, "\n")
	require.Equal(t, "", strings.TrimSpace(lines[1]))
	require.Contains(t, lines[2], "api")
}
```

(`stripANSI` vive en `app_test.go`, mismo paquete. Actualiza cualquier otro test del paquete que afirme "próximamente" o la posición vieja de filas.)

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/tui/ -run 'TestHitAtRow|TestSidebarViewStyled'`
Expected: FAIL.

- [ ] **Step 3: Implement**

En `sidebar.go`:

```go
// sectionHeader alinea el título a la izquierda y el sufijo a la derecha del ancho del
// sidebar. El título se ilumina en cian de marca cuando la sección tiene el foco.
func (s sidebar) sectionHeader(title, suffix string, focused bool) string {
	fill := s.width - lipgloss.Width(title) - lipgloss.Width(suffix)
	if fill < 1 {
		fill = 1
	}
	t := render.Dim(title)
	if focused {
		t = render.Brand(title)
	}
	return t + strings.Repeat(" ", fill) + render.Dim(suffix)
}

func (s sidebar) view(focused bool) string {
	var b strings.Builder
	b.WriteString(s.sectionHeader("SERVICES", "("+strconv.Itoa(len(s.services))+")", focused) + "\n\n")
	for i, svc := range s.services {
		b.WriteString(s.serviceRow(svc, i == s.cursor) + "\n")
	}
	b.WriteString("\n" + s.sectionHeader("IMAGES (ECR)", "···", false) + "\n" + render.Dim("  coming soon") + "\n")
	b.WriteString("\n" + s.sectionHeader("DATABASES", "···", false) + "\n" + render.Dim("  coming soon") + "\n")
	return b.String()
}
```

(añade el import de `lipgloss`). `HitAtRow` se desplaza:

```go
// HitAtRow replica la estructura de view(): fila 0 header, fila 1 en blanco,
// filas 2..n+1 los servicios; el resto no es accionable.
func (s sidebar) HitAtRow(row int) (sidebarHit, bool) {
	if row < 2 {
		return sidebarHit{}, false
	}
	if idx := row - 2; idx < len(s.services) {
		return sidebarHit{Section: sectionServices, Index: idx}, true
	}
	return sidebarHit{}, false
}
```

En `app.go`, los dos call sites de `m.sidebar.view()` (dos columnas y una columna) pasan a `m.sidebar.view(m.focus == focusSidebar)`.

- [ ] **Step 4: Run to verify pass**

Run: `gofmt -w internal/tui/ && golangci-lint run && go test ./internal/tui/... -count=1 && go build ./...`
Expected: PASS — `TestMouseClickSelectsSidebarService` recalibra solo (deriva la Y del render y `HitAtRow` ya coincide).

- [ ] **Step 5: Commit**

```bash
git add internal/tui/
git commit -m "feat(tui): sidebar estilizado (foco en header, sufijos a la derecha, coming soon)"
```

---

### Task 3: Marco único — View/layout sin cajas, divisor vertical y geometría del mouse

**Files:**
- Modify: `internal/tui/app.go` (`layout()`, `View()`, `handleMouse` constantes X), `internal/tui/overlay.go` (offset Y del picker), `internal/tui/styles.go` (limpieza), `internal/tui/app_test.go` (solo si algún assert dependía de los bordes `╭`)

**Interfaces:**
- Consumes: `topBar(width,...)`, `hrule`, `vdivider` (T1); `sidebar.view(focused)` (T2).
- Produces: layout final — fila 0 top bar, fila 1 regla, cuerpo (`bodyH` filas: sidebar con `PaddingLeft(1)` | divisor en `X=sidebarW` | panel con `PaddingLeft(1)`), regla, help. Panel content X0 = `sidebarW+2` (antes `+3`). El overlay se dibuja tras la regla (su Y arranca en 2, no en 1).

- [ ] **Step 1: layout()**

Reemplaza el cuerpo de `layout()`:

```go
// layout reparte el espacio: [top bar][regla][cuerpo bodyH][regla][help].
// Si el ancho < singleColumnThreshold, colapsa a una sola columna apilada.
func (m *Model) layout() {
	m.singleColumn = m.width < singleColumnThreshold
	m.bodyH = m.height - 4 // top bar + regla + regla + help
	if m.bodyH < 3 {
		m.bodyH = 3
	}
	if m.singleColumn {
		m.sidebarW = m.width
		m.panelW = m.width
		if m.sidebarW < 10 {
			m.sidebarW, m.panelW = 10, 10
		}
		m.sidebar.width = m.sidebarW - 1 // PaddingLeft(1) del bloque
		m.events.SetSize(m.panelW-2, m.bodyH/2-2)
		return
	}
	m.sidebarW = m.width * 30 / 100
	if m.sidebarW < sidebarMinWidth {
		m.sidebarW = sidebarMinWidth
	}
	m.panelW = m.width - m.sidebarW - 1 // columna del divisor
	if m.panelW < 10 {
		m.panelW = 10
	}
	m.sidebar.width = m.sidebarW - 1
	m.events.SetSize(m.panelW-2, m.bodyH-2) // - pestañas - línea en blanco
}
```

- [ ] **Step 2: View()**

Reemplaza el ensamblaje (mantiene la rama de error y la de overlay, ahora con reglas):

```go
func (m Model) View() string {
	if m.err != nil {
		return render.Danger("error: "+m.err.Error()) + "\n" + render.Dim("press q to quit")
	}
	top := topBar(m.width, m.current.Cloud, m.current.Name, m.current.Cluster, m.current.Writable)
	rule := hrule(m.width)

	if m.overlay != nil {
		return top + "\n" + rule + "\n" + m.overlay.View(m.width, m.bodyH) + "\n" +
			rule + "\n" + bottomBar(m.keys.shortHelp(), m.notice, m.status)
	}

	block := func(w int) lipgloss.Style {
		return lipgloss.NewStyle().Width(w).Height(m.bodyH).PaddingLeft(1)
	}
	panelBody := m.tabs.View() + "\n\n" + m.panelBody()
	var body string
	if m.singleColumn {
		side := block(m.sidebarW).Height(m.bodyH / 2).Render(m.sidebar.view(m.focus == focusSidebar))
		pan := block(m.panelW).Height(m.bodyH - m.bodyH/2 - 1).Render(panelBody)
		body = lipgloss.JoinVertical(lipgloss.Left, side, rule, pan)
	} else {
		side := block(m.sidebarW).Render(m.sidebar.view(m.focus == focusSidebar))
		pan := block(m.panelW).Render(panelBody)
		body = lipgloss.JoinHorizontal(lipgloss.Top, side, vdivider(m.bodyH), pan)
	}
	bottom := bottomBar(m.keys.shortHelp(), m.notice, m.status)
	return top + "\n" + rule + "\n" + body + "\n" + rule + "\n" + bottom
}
```

- [ ] **Step 3: Geometría del mouse y del overlay**

En `app.go`:
- El comentario/uso de `borderTop` pasa a significar "fila de la regla horizontal" (mismo valor 1; actualiza el comentario del bloque de constantes).
- Zona del sidebar: `if msg.X < m.sidebarW {` (el divisor en `X == sidebarW` cae al panel, donde no acierta nada).
- Los dos `panelContentX0`/offsets del panel: `m.sidebarW + 3` → `m.sidebarW + 2` (pestañas y botones de Details).

En `overlay.go` (`pickerOverlay.Update`, rama mouse): el picker ahora se dibuja tras la regla — el offset pasa de `msg.Y - topBarHeight` a `msg.Y - (topBarHeight + borderTop)` (comenta: top bar + regla).

- [ ] **Step 4: Limpieza de estilos**

En `styles.go`: elimina `focusedBorder()`, `blurredBorder()`, `colorFocusBorder` y `colorBlurredBorder` (ya sin usos); el import de `render` puede quedar sin uso — elimínalo si el compilador lo pide. Conserva `sidebarMinWidth`, `singleColumnThreshold`, `colorSelectionBar`.

- [ ] **Step 5: Run to verify pass (los tests anclados al render son el guard)**

Run: `gofmt -w internal/tui/ && golangci-lint run && go vet ./... && go test ./... -count=1 && go build ./...`
Expected: PASS. En particular deben pasar SIN debilitarse: `TestMouseClickSelectsSidebarService`, `TestClickPanelTabSwitches`, `TestClickDetailsDeployButton`, `TestClickDetailsScaleAndRollbackButtons`, `TestClickPickerRowSwitchesContext`, `TestClickCancelsActionModal`, `TestSingleColumnDetailsClickNoMisfire`, `TestClickTopBarOpensContextPicker`. Si alguno falla, la geometría de producción está desalineada con el render — corrige las constantes, no el test.

- [ ] **Step 6: Smoke visual**

Crea un test temporal que imprima `m.View()` con datos fake a 100×28 (patrón de smokes previos), inspecciona que el layout coincida con el diseño (reglas, divisor, headers, coming soon, estado a la derecha) y bórralo antes del commit.

- [ ] **Step 7: Commit**

```bash
git add internal/tui/
git commit -m "feat(tui): marco único con reglas y divisor vertical (adiós cajas)"
```

---

## Verificación final (no es una tarea)

- Suite completa `-count=1 -race` + lint + build.
- Smoke manual: `go run ./cmd/steerdemo` — comprobar el layout del mockup, foco (SERVICES cian ↔ panel), clicks en todas las zonas, picker y modal, y modo una-columna (terminal angosta).
- Actualizar `docs/plan/2026-07-04-revision-arquitectura.md` no aplica (esto es visual, no de la revisión).
