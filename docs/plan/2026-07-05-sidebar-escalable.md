# Sidebar escalable — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implementar el diseño `2026-07-05-sidebar-escalable-diseno.md`: sidebar con secciones colapsables (cursor navega headers, todo clickeable), filtro `/` substring en vivo, y scroll con indicadores `↑/↓ N more` + rueda del mouse.

**Architecture:** El sidebar se reconstruye alrededor de UNA fuente de verdad: `rows(focused)` devuelve las filas renderizadas con su entrada asociada (header/servicio/decorativa) — render y hit-testing indexan la misma lista y no pueden divergir. T1 introduce ese modelo + colapso + cursor-sobre-entradas + selección-por-nombre. T2 añade el modo filtro (captura de teclado a nivel Model, como los overlays). T3 añade la ventana de scroll con indicadores y la rueda. Cada tarea deja la suite verde; los tests anclados al render son el guard.

**Tech Stack:** Go 1.26, Bubble Tea, Lipgloss, testify.

## Global Constraints

- Comportamiento fuera del sidebar intacto: overlays, deploy en vivo, switch de contexto, panel, teclas globales. `d/s/R` siguen actuando sobre el **servicio seleccionado**.
- Pasar el cursor por un header NO cambia la selección (el panel no parpadea).
- Comentarios en español, UI strings en inglés; sin autoría de Claude en commits.
- Antes de cada commit: `gofmt -w internal/ && golangci-lint run && go vet ./... && go test ./... -count=1 && go build ./...` — todo verde.
- Los tests anclados al render se recalibran derivando del render — no se debilitan aserciones (cambiarlas a afirmar por NOMBRE de servicio seleccionado es FORTALECER, no debilitar).
- Fuera de alcance: fuzzy, persistencia de colapso, filas reales de IMAGES/DATABASES, tag-picker.

## File Structure

```
internal/tui/sidebar.go        ← núcleo nuevo: entries/rows, colapso, cursor/selección, filtro, scroll
internal/tui/sidebar_test.go   ← tests del modelo
internal/tui/keys.go           ← + Space (T1), Filter `/` (T2)
internal/tui/app.go            ← enter/space en header; mouse por entrada; routing del filtro; rueda en sidebar
internal/tui/app_test.go       ← clicks anclados al render actualizados (afirman por nombre)
```

---

### Task 1: Modelo de entradas — colapsables, cursor sobre headers, selección por nombre

**Files:**
- Modify: `internal/tui/sidebar.go` (reescritura del núcleo), `internal/tui/sidebar_test.go`, `internal/tui/keys.go` (+`Space`), `internal/tui/app.go` (handleKey sidebar + handleMouse sidebar), `internal/tui/app_test.go`

**Interfaces:**
- Produces (API interna del sidebar — reemplaza `HitAtRow`/`selectIndex`/`sidebarHit`):

```go
type entryKind int

const (
	entryHeader entryKind = iota
	entryService
)

// sidebarEntry es una entrada navegable: header de sección o servicio.
type sidebarEntry struct {
	Kind    entryKind
	Section sidebarSection
	Index   int // índice del servicio visible dentro de su sección (solo entryService)
}

// sidebarRow es una fila renderizada; Entry nil = decorativa (blanco/stub/indicador).
type sidebarRow struct {
	Line  string
	Entry *sidebarEntry
}

func (s sidebar) rows(focused bool) []sidebarRow      // fuente única render+hits
func (s sidebar) view(focused bool) string            // join de rows().Line
func (s sidebar) EntryAtRow(row int) (sidebarEntry, bool)
func (s *sidebar) moveDown() / moveUp()               // sobre entradas navegables
func (s sidebar) cursorEntry() (sidebarEntry, bool)
func (s *sidebar) toggle(sec sidebarSection)
func (s *sidebar) selectEntry(e sidebarEntry)         // click: mueve cursor; si es servicio, selecciona
func (s sidebar) selected() (core.ServiceStatus, bool) // por selectedName
```

- Campos nuevos en `sidebar`: `collapsed map[sidebarSection]bool` (default: IMAGES y DATABASES colapsadas), `selectedName string` (nombre REAL del servicio seleccionado), `cursor int` pasa a indexar entradas navegables.
- Estado inicial tras `setServices`: cursor sobre el primer servicio (índice navegable 1), selección = primer servicio (o se conserva por nombre si sobrevive al reload).
- Render: la fila bajo el **cursor** lleva la barra de fondo (headers incluidos); headers muestran `▾`/`▸` junto al título y el count a la derecha; secciones colapsadas ocultan su contenido (incl. "coming soon").
- `keys.go`: `Space key.Binding` (`" "`). En `handleKey` (foco sidebar): `enter`/`space` con cursor en header → `toggle`.
- `handleMouse` zona sidebar: `EntryAtRow` → header=toggle / servicio=selectEntry (+focus sidebar).

- [ ] **Step 1: Write the failing tests (reemplaza los del modelo viejo)**

```go
// internal/tui/sidebar_test.go — reemplaza TestHitAtRow/TestSidebarNavigationClamps/
// TestSidebarSelectIndex/TestSidebarSetServicesReclampsCursor por estos:

// Con IMAGES/DATABASES colapsadas (default), las entradas navegables son:
// [0]=header SERVICES, [1..4]=servicios, [5]=header IMAGES, [6]=header DATABASES.
func TestSidebarNavEntriesAndInitialState(t *testing.T) {
	s := newSidebar()
	s.setServices(sampleServices())
	e, ok := s.cursorEntry()
	require.True(t, ok)
	require.Equal(t, entryService, e.Kind) // cursor inicial: primer servicio
	sel, ok := s.selected()
	require.True(t, ok)
	require.Equal(t, "api", sel.Name) // selección inicial: primer servicio
}

func TestSidebarCursorOverHeadersKeepsSelection(t *testing.T) {
	s := newSidebar()
	s.setServices(sampleServices())
	for range 4 { // baja hasta salir de los servicios
		s.moveDown()
	}
	e, _ := s.cursorEntry()
	require.Equal(t, entryHeader, e.Kind) // header IMAGES
	require.Equal(t, sectionImages, e.Section)
	sel, ok := s.selected()
	require.True(t, ok)
	require.Equal(t, "worker", sel.Name) // la selección quedó en el último servicio pisado
}

func TestSidebarToggleCollapsesServices(t *testing.T) {
	s := newSidebar()
	s.setServices(sampleServices())
	s.toggle(sectionServices)
	out := stripANSI(s.view(true))
	require.Contains(t, out, "▸")
	require.NotContains(t, out, "api") // sección colapsada oculta items
	// navegación: ya solo hay 3 headers
	s.moveDown()
	e, _ := s.cursorEntry()
	require.Equal(t, entryHeader, e.Kind)
}

func TestSidebarCollapsedByDefaultHidesStubs(t *testing.T) {
	s := newSidebar()
	s.setServices(sampleServices())
	out := stripANSI(s.view(true))
	require.NotContains(t, out, "coming soon") // IMAGES/DATABASES colapsadas
	s.toggle(sectionImages)
	require.Contains(t, stripANSI(s.view(true)), "coming soon")
}

func TestEntryAtRowMatchesRows(t *testing.T) {
	s := newSidebar()
	s.setServices(sampleServices())
	rows := s.rows(true)
	for i, r := range rows {
		e, ok := s.EntryAtRow(i)
		if r.Entry == nil {
			require.False(t, ok, "row %d", i)
		} else {
			require.True(t, ok, "row %d", i)
			require.Equal(t, *r.Entry, e)
		}
	}
	_, ok := s.EntryAtRow(-1)
	require.False(t, ok)
	_, ok = s.EntryAtRow(len(rows) + 5)
	require.False(t, ok)
}

func TestSidebarSelectionSurvivesReload(t *testing.T) {
	s := newSidebar()
	s.setServices(sampleServices())
	s.moveDown() // cursor a "cron" (2º servicio ordenado) — lo selecciona
	sel, _ := s.selected()
	require.Equal(t, "cron", sel.Name)
	s.setServices(sampleServices()) // reload: la selección persiste por nombre
	sel, ok := s.selected()
	require.True(t, ok)
	require.Equal(t, "cron", sel.Name)
}
```

En `app_test.go`, `TestMouseClickSelectsSidebarService` se FORTALECE: tras el click afirma
`sel, ok := m.sidebar.selected(); require.True(t, ok); require.Equal(t, "web", sel.Name)`
(en vez del índice de cursor). Añade además:

```go
// Click en el header de IMAGES la expande (todo es clickeable).
func TestClickHeaderTogglesSection(t *testing.T) {
	m := newTestModel(sampleServices())
	out := m.View()
	clickY := -1
	for y, line := range strings.Split(out, "\n") {
		if strings.Contains(stripANSI(line), "IMAGES (ECR)") {
			clickY = y
			break
		}
	}
	require.GreaterOrEqual(t, clickY, 0)
	click := tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 3, Y: clickY}
	m = mustUpdate(t, m, click)
	require.Contains(t, stripANSI(m.View()), "coming soon") // se expandió
}

// enter/space con el cursor en un header lo togglea.
func TestEnterOnHeaderToggles(t *testing.T) {
	m := newTestModel(sampleServices())
	for range 5 { // header SERVICES→…→header IMAGES
		m = mustUpdate(t, m, keyMsg("j"))
	}
	m = mustUpdate(t, m, keyMsg("enter"))
	require.Contains(t, stripANSI(m.View()), "coming soon")
}
```

(Ajusta el conteo de `j` si el punto de partida difiere — el cursor inicial está en el
primer servicio, índice navegable 1; verifica con `cursorEntry`.)

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/tui/ -run 'TestSidebar|TestEntryAtRow|TestClickHeader|TestEnterOnHeader'`
Expected: FAIL (tipos/métodos no definidos).

- [ ] **Step 3: Implement — núcleo nuevo de sidebar.go**

Reemplaza el struct y los métodos de navegación/hit (conserva `setServices`' ordenamiento,
`sectionHeader` y `serviceRow` con ajustes):

```go
// sidebar es la columna izquierda: secciones colapsables con entradas navegables.
type sidebar struct {
	services      []core.ServiceStatus
	collapsed     map[sidebarSection]bool
	cursor        int    // índice sobre las entradas navegables (headers + servicios)
	selectedName  string // nombre REAL del servicio seleccionado (persiste por nombre)
	width, height int
	prefix        string
}

func newSidebar() sidebar {
	return sidebar{collapsed: map[sidebarSection]bool{
		sectionImages:    true,
		sectionDatabases: true,
	}}
}
```

`setServices` (tras ordenar, reemplaza el clamp del cursor):

```go
	s.services = sorted
	// la selección persiste por nombre; si desapareció (o no había), primer servicio
	if _, ok := s.selected(); !ok && len(s.services) > 0 {
		s.selectedName = s.services[0].Name
	}
	// cursor inicial/re-clamp: sobre el primer servicio si existe
	nav := s.navEntries()
	if s.cursor >= len(nav) {
		s.cursor = max(0, len(nav)-1)
	}
	if len(s.services) > 0 && s.cursor == 0 {
		s.cursor = 1 // [0] es el header SERVICES
	}
```

Modelo de filas y navegación (fuente única — una sola pasada construye filas y pinta el cursor):

```go
// rows es la fuente única: cada fila renderizada con su entrada (nil = decorativa).
func (s sidebar) rows(focused bool) []sidebarRow {
	var out []sidebarRow
	nav := 0

	appendHeader := func(sec sidebarSection, title, count string) {
		mark := " ▾"
		if s.collapsed[sec] {
			mark = " ▸"
		}
		var line string
		if nav == s.cursor {
			// header bajo el cursor: barra de fondo a lo ancho
			line = s.barLine(title + mark + "  " + count)
		} else {
			line = s.sectionHeader(title+mark, count, focused && sec == sectionServices)
		}
		out = append(out, sidebarRow{Line: line, Entry: &sidebarEntry{Kind: entryHeader, Section: sec}})
		nav++
	}
	appendBlank := func() { out = append(out, sidebarRow{Line: ""}) }

	// SERVICES
	appendHeader(sectionServices, "SERVICES", "("+strconv.Itoa(len(s.services))+")")
	appendBlank()
	if !s.collapsed[sectionServices] {
		for i, svc := range s.services {
			under := nav == s.cursor
			line := s.serviceRow(svc, under)
			out = append(out, sidebarRow{Line: line, Entry: &sidebarEntry{Kind: entryService, Section: sectionServices, Index: i}})
			nav++
		}
		appendBlank()
	}
	// IMAGES
	appendHeader(sectionImages, "IMAGES (ECR)", "···")
	if !s.collapsed[sectionImages] {
		out = append(out, sidebarRow{Line: render.Dim("  coming soon")})
	}
	appendBlank()
	// DATABASES
	appendHeader(sectionDatabases, "DATABASES", "···")
	if !s.collapsed[sectionDatabases] {
		out = append(out, sidebarRow{Line: render.Dim("  coming soon")})
	}
	return out
}

// barLine pinta una línea con la barra de fondo del cursor a lo ancho del sidebar.
func (s sidebar) barLine(text string) string {
	bg := lipgloss.Color(colorSelectionBar)
	w := s.width
	if w < 1 {
		w = lipgloss.Width(text) + 2
	}
	return lipgloss.NewStyle().Background(bg).Width(w).Render(" " + text)
}

func (s sidebar) view(focused bool) string {
	var b strings.Builder
	for _, r := range s.rows(focused) {
		b.WriteString(r.Line + "\n")
	}
	return b.String()
}

// navEntries devuelve las entradas navegables en orden (headers + servicios visibles).
func (s sidebar) navEntries() []sidebarEntry {
	var out []sidebarEntry
	for _, r := range s.rows(false) {
		if r.Entry != nil {
			out = append(out, *r.Entry)
		}
	}
	return out
}

func (s sidebar) cursorEntry() (sidebarEntry, bool) {
	nav := s.navEntries()
	if s.cursor < 0 || s.cursor >= len(nav) {
		return sidebarEntry{}, false
	}
	return nav[s.cursor], true
}

func (s *sidebar) moveDown() { s.moveCursor(1) }
func (s *sidebar) moveUp()   { s.moveCursor(-1) }

// moveCursor desplaza el cursor sobre las entradas navegables; al pisar un servicio,
// lo selecciona (pasar por headers no toca la selección).
func (s *sidebar) moveCursor(delta int) {
	nav := s.navEntries()
	if len(nav) == 0 {
		return
	}
	s.cursor = min(max(s.cursor+delta, 0), len(nav)-1)
	if e := nav[s.cursor]; e.Kind == entryService {
		s.selectedName = s.services[e.Index].Name
	}
}

// toggle colapsa/expande una sección conservando el cursor sobre su header.
func (s *sidebar) toggle(sec sidebarSection) {
	s.collapsed[sec] = !s.collapsed[sec]
	// re-ubicar el cursor sobre el header de la sección toggleada
	for i, e := range s.navEntries() {
		if e.Kind == entryHeader && e.Section == sec {
			s.cursor = i
			return
		}
	}
}

// selectEntry ubica el cursor en la entrada e (click); si es servicio, lo selecciona.
func (s *sidebar) selectEntry(target sidebarEntry) {
	for i, e := range s.navEntries() {
		if e == target {
			s.cursor = i
			if e.Kind == entryService {
				s.selectedName = s.services[e.Index].Name
			}
			return
		}
	}
}

// EntryAtRow mapea una fila renderizada a su entrada (ok=false si es decorativa).
func (s sidebar) EntryAtRow(row int) (sidebarEntry, bool) {
	rows := s.rows(false)
	if row < 0 || row >= len(rows) || rows[row].Entry == nil {
		return sidebarEntry{}, false
	}
	return *rows[row].Entry, true
}

func (s sidebar) selected() (core.ServiceStatus, bool) {
	for _, svc := range s.services {
		if svc.Name == s.selectedName {
			return svc, true
		}
	}
	return core.ServiceStatus{}, false
}
```

Notas:
- `serviceRow(svc, under)` — el parámetro pasa a significar "bajo el cursor" (la barra);
  renombra el parámetro `selected` → `underCursor` para honestidad.
- Elimina `selectIndex`, `HitAtRow` y el tipo `sidebarHit` (sustituidos por `EntryAtRow` y
  `sidebarEntry`). Elimina el esqueleto orientativo si quedó algo.
- Los tipos `entryKind`/`sidebarEntry`/`sidebarRow` van arriba del struct.

`keys.go`: añade `Space key.Binding` al struct y `Space: key.NewBinding(key.WithKeys(" "))`.

`app.go` — rama sidebar de `handleKey`:

```go
	// foco en sidebar
	switch {
	case key.Matches(msg, m.keys.Down):
		m.sidebar.moveDown()
	case key.Matches(msg, m.keys.Up):
		m.sidebar.moveUp()
	case key.Matches(msg, m.keys.Enter), key.Matches(msg, m.keys.Space):
		if e, ok := m.sidebar.cursorEntry(); ok && e.Kind == entryHeader {
			m.sidebar.toggle(e.Section)
		}
	}
	return m, nil
```

`app.go` — zona sidebar de `handleMouse`:

```go
	if msg.X < m.sidebarW {
		row := msg.Y - (topBarHeight + borderTop)
		if e, ok := m.sidebar.EntryAtRow(row); ok {
			switch e.Kind {
			case entryHeader:
				m.sidebar.toggle(e.Section)
			case entryService:
				m.sidebar.selectEntry(e)
			}
			m.focus = focusSidebar
		}
		return nil
	}
```

- [ ] **Step 4: Run to verify pass**

Run: `gofmt -w internal/tui/ && golangci-lint run && go vet ./... && go test ./... -count=1 && go build ./...`
Expected: PASS — incluidos los clicks anclados al render (recalibran solos) y el resto del
paquete. Si `TestSingleColumnDetailsClickNoMisfire` u otros fallan por geometría, revisa
`EntryAtRow` contra `rows()` — nunca ajustes el test.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/
git commit -m "feat(tui): sidebar con secciones colapsables y cursor sobre entradas"
```

---

### Task 2: Filtro `/` substring en vivo

**Files:**
- Modify: `internal/tui/sidebar.go`, `internal/tui/keys.go` (+`Filter`), `internal/tui/app.go` (routing de captura), tests en `sidebar_test.go` + `app_test.go`

**Interfaces:**
- Produces:
  - `sidebar` gana `filterActive bool`, `filterQuery string`; `visibleServices() []core.ServiceStatus` (filtrado case-insensitive por substring del nombre de display) — `rows()`/`navEntries()`/`selectEntry`/`moveCursor` operan sobre los visibles; el `Index` de `sidebarEntry` indexa los VISIBLES.
  - Header con filtro: título `SERVICES /aud▌` mientras se teclea (cursor `▌`), count `(1/16)`.
  - `func (s *sidebar) setFilter(q string)` / `func (s *sidebar) clearFilter()`; si la selección queda filtrada fuera → primer visible.
  - `keys.go`: `Filter key.Binding` (`/`).
  - `Model`: routing de captura — cuando `m.sidebar.filterActive` (sin overlay), las teclas van a `handleFilterKey`: runas/backspace editan y refiltran en vivo; `esc` limpia y desactiva; `enter` fija (desactiva dejando el query). Las teclas globales NO disparan mientras se teclea.

- [ ] **Step 1: Write the failing tests**

```go
// sidebar_test.go
func TestSidebarFilterLive(t *testing.T) {
	s := newSidebar()
	s.setServices(sampleServices())
	s.setFilter("wo")
	require.Len(t, s.visibleServices(), 1)
	sel, ok := s.selected()
	require.True(t, ok)
	require.Equal(t, "worker", sel.Name) // la selección salta al primer visible
	out := stripANSI(s.view(true))
	require.Contains(t, out, "/wo")
	require.Contains(t, out, "(1/4)")
	require.NotContains(t, out, "api")
	s.clearFilter()
	require.Len(t, s.visibleServices(), 4)
}

// app_test.go
func TestFilterModeCapturesGlobalKeys(t *testing.T) {
	m := newTestModel(sampleServices())
	m = mustUpdate(t, m, keyMsg("/")) // activa filtro
	m = mustUpdate(t, m, keyMsg("d")) // debe TECLEAR, no abrir deploy
	require.Nil(t, m.overlay)
	require.Contains(t, stripANSI(m.View()), "/d")
	m = mustUpdate(t, m, keyMsg("esc")) // limpia y sale
	require.NotContains(t, stripANSI(m.View()), "/d")
}

func TestFilterEnterKeepsQuery(t *testing.T) {
	m := newTestModel(sampleServices())
	m = mustUpdate(t, m, keyMsg("/"))
	for _, r := range "web" {
		m = mustUpdate(t, m, keyMsg(string(r)))
	}
	m = mustUpdate(t, m, keyMsg("enter"))
	sel, _ := m.sidebar.selected()
	require.Equal(t, "web", sel.Name)
	// tras enter, las teclas globales vuelven a funcionar
	m = mustUpdate(t, m, keyMsg("d"))
	require.NotNil(t, m.overlay) // abrió el modal de deploy
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/tui/ -run 'TestSidebarFilter|TestFilterMode|TestFilterEnter'`
Expected: FAIL.

- [ ] **Step 3: Implement**

`sidebar.go`:

```go
// visibleServices aplica el filtro substring (case-insensitive) sobre el nombre de display.
func (s sidebar) visibleServices() []core.ServiceStatus {
	if s.filterQuery == "" {
		return s.services
	}
	q := strings.ToLower(s.filterQuery)
	var out []core.ServiceStatus
	for _, svc := range s.services {
		if strings.Contains(strings.ToLower(strings.TrimPrefix(svc.Name, s.prefix)), q) {
			out = append(out, svc)
		}
	}
	return out
}

func (s *sidebar) setFilter(q string) {
	s.filterQuery = q
	vis := s.visibleServices()
	// si la selección quedó fuera del filtro, saltar al primer visible
	stillVisible := false
	for _, svc := range vis {
		if svc.Name == s.selectedName {
			stillVisible = true
			break
		}
	}
	if !stillVisible {
		s.selectedName = ""
		if len(vis) > 0 {
			s.selectedName = vis[0].Name
		}
	}
	// re-clamp del cursor sobre las nuevas entradas
	if nav := s.navEntries(); s.cursor >= len(nav) {
		s.cursor = max(0, len(nav)-1)
	}
}

func (s *sidebar) clearFilter() {
	s.filterActive = false
	s.setFilter("")
}
```

En `rows()`: itera `s.visibleServices()` en lugar de `s.services`; el header SERVICES usa:

```go
	count := "(" + strconv.Itoa(len(s.visibleServices())) + "/" + strconv.Itoa(len(s.services)) + ")"
	if s.filterQuery == "" && !s.filterActive {
		count = "(" + strconv.Itoa(len(s.services)) + ")"
	}
	title := "SERVICES"
	if s.filterActive {
		title = "SERVICES /" + s.filterQuery + "▌"
	} else if s.filterQuery != "" {
		title = "SERVICES /" + s.filterQuery
	}
```

`moveCursor`/`selectEntry`: donde asignaban `s.services[e.Index].Name` pasan a
`s.visibleServices()[e.Index].Name`.

`keys.go`: `Filter: key.NewBinding(key.WithKeys("/"))`.

`app.go` — en `Update`, tras el routing de overlays y ANTES de `handleKey`:

```go
		if m.sidebar.filterActive {
			return m.handleFilterKey(msg)
		}
		return m.handleKey(msg)
```

```go
// handleFilterKey edita el filtro del sidebar en vivo (captura el teclado).
func (m Model) handleFilterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.sidebar.clearFilter()
	case tea.KeyEnter:
		m.sidebar.filterActive = false // fija el query
	case tea.KeyBackspace:
		if q := m.sidebar.filterQuery; q != "" {
			m.sidebar.setFilter(q[:len(q)-1])
		}
	case tea.KeyRunes:
		m.sidebar.setFilter(m.sidebar.filterQuery + string(msg.Runes))
	}
	return m, nil
}
```

Y en `handleKey`, en el switch global (tras `Context`): `case key.Matches(msg, m.keys.Filter): if m.focus == focusSidebar { m.sidebar.filterActive = true }; return m, nil`.

- [ ] **Step 4: Run to verify pass**

Run: `gofmt -w internal/tui/ && golangci-lint run && go vet ./... && go test ./... -count=1 && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/
git commit -m "feat(tui): filtro / substring en vivo en el sidebar"
```

---

### Task 3: Scroll con indicadores + rueda en el sidebar

**Files:**
- Modify: `internal/tui/sidebar.go`, `internal/tui/app.go` (layout pasa height; rueda; mapeo de click por ventana), tests

**Interfaces:**
- Produces:
  - `sidebar.scroll int`; `func (s sidebar) visibleRows(focused bool) []sidebarRow` — ventana de `s.height` filas sobre `rows()`; si hay recorte arriba/abajo, la primera/última fila visible se sustituye por el indicador `↑ N more`/`↓ N more` (`Entry` nil, `render.Dim`).
  - `view()` une `visibleRows`; `EntryAtVisibleRow(row int)` mapea la fila EN PANTALLA a su entrada (consciente de scroll+indicadores) — reemplaza a `EntryAtRow` en el mouse.
  - `func (s *sidebar) ensureCursorVisible()` — llamado tras cada moveCursor/toggle/filtro.
  - `func (s *sidebar) scrollBy(delta int)` — clamp [0, max]; rueda del mouse: ±3.
  - `layout()` fija `m.sidebar.height = m.bodyH` (dos ramas).
  - `handleMouse`: rueda con `msg.X < m.sidebarW` → `m.sidebar.scrollBy(±3)`; clicks usan `EntryAtVisibleRow`.

- [ ] **Step 1: Write the failing tests**

```go
// sidebar_test.go
func manyServices(n int) []core.ServiceStatus {
	out := make([]core.ServiceStatus, n)
	for i := range out {
		out[i] = core.ServiceStatus{Name: fmt.Sprintf("svc-%02d", i), Running: 1, Desired: 1}
	}
	return out
}

func TestSidebarScrollWindowWithIndicators(t *testing.T) {
	s := newSidebar()
	s.height = 10
	s.setServices(manyServices(30))
	rows := s.visibleRows(true)
	require.Len(t, rows, 10)
	last := stripANSI(rows[len(rows)-1].Line)
	require.Contains(t, last, "more") // recorte abajo
	require.Contains(t, last, "↓")
	// scrollear al fondo produce indicador arriba y no abajo
	s.scrollBy(1000)
	rows = s.visibleRows(true)
	require.Contains(t, stripANSI(rows[0].Line), "↑")
	require.NotContains(t, stripANSI(rows[len(rows)-1].Line), "more")
}

func TestSidebarCursorFollow(t *testing.T) {
	s := newSidebar()
	s.height = 8
	s.setServices(manyServices(30))
	for range 20 {
		s.moveDown()
	}
	// el cursor (servicio 20) debe estar dentro de la ventana visible
	found := false
	for _, r := range s.visibleRows(true) {
		if r.Entry != nil && r.Entry.Kind == entryService && strings.Contains(stripANSI(r.Line), "svc-20") {
			found = true
		}
	}
	require.True(t, found)
}

func TestEntryAtVisibleRowWithScroll(t *testing.T) {
	s := newSidebar()
	s.height = 8
	s.setServices(manyServices(30))
	s.scrollBy(5)
	rows := s.visibleRows(false)
	for i, r := range rows {
		e, ok := s.EntryAtVisibleRow(i)
		if r.Entry == nil {
			require.False(t, ok, "row %d", i)
		} else {
			require.True(t, ok, "row %d", i)
			require.Equal(t, *r.Entry, e)
		}
	}
}
```

En `app_test.go` (anclado al render, con scroll real):

```go
func TestClickServiceWithScrolledSidebar(t *testing.T) {
	m := newTestModel(manyServices(40))
	// scrollear el sidebar con la rueda
	for range 4 {
		wheel := tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown, X: 3, Y: 5}
		m = mustUpdate(t, m, wheel)
	}
	out := m.View()
	targetY, targetName := -1, ""
	for y, line := range strings.Split(out, "\n") {
		clean := stripANSI(line)
		if strings.Contains(clean, "svc-2") && targetY == -1 { // primer svc-2x visible
			targetY, targetName = y, strings.Fields(clean)[1] // "● svc-2X ..."
			break
		}
	}
	require.GreaterOrEqual(t, targetY, 0)
	click := tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: 3, Y: targetY}
	m = mustUpdate(t, m, click)
	sel, ok := m.sidebar.selected()
	require.True(t, ok)
	require.Equal(t, targetName, sel.Name)
}
```

(Ajusta la extracción del nombre a la estructura real de la fila; el objetivo es: el
click sobre la fila visible selecciona EXACTAMENTE el servicio que se ve ahí, con la
ventana desplazada.)

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/tui/ -run 'TestSidebarScroll|TestSidebarCursorFollow|TestEntryAtVisibleRow|TestClickServiceWithScrolled'`
Expected: FAIL.

- [ ] **Step 3: Implement**

`sidebar.go`:

```go
// visibleRows devuelve la ventana de s.height filas sobre rows(), con indicadores
// ↑/↓ N more cuando hay recorte (filas decorativas, no navegables).
func (s sidebar) visibleRows(focused bool) []sidebarRow {
	all := s.rows(focused)
	if s.height <= 0 || len(all) <= s.height {
		return all
	}
	scroll := min(max(s.scroll, 0), len(all)-s.height)
	win := make([]sidebarRow, s.height)
	copy(win, all[scroll:scroll+s.height])
	if scroll > 0 {
		win[0] = sidebarRow{Line: render.Dim("  ↑ " + strconv.Itoa(scroll) + " more")}
	}
	if hidden := len(all) - (scroll + s.height); hidden > 0 {
		win[len(win)-1] = sidebarRow{Line: render.Dim("  ↓ " + strconv.Itoa(hidden) + " more")}
	}
	return win
}

func (s *sidebar) scrollBy(delta int) {
	all := len(s.rows(false))
	maxScroll := max(0, all-s.height)
	s.scroll = min(max(s.scroll+delta, 0), maxScroll)
}

// ensureCursorVisible ajusta el scroll para que la fila del cursor quede en la ventana
// (dejando sitio a los indicadores).
func (s *sidebar) ensureCursorVisible() {
	if s.height <= 0 {
		return
	}
	row := s.cursorRow()
	if row < 0 {
		return
	}
	if row < s.scroll+1 { // +1: fila del indicador ↑
		s.scroll = max(0, row-1)
	}
	if row > s.scroll+s.height-2 { // -2: fila del indicador ↓
		s.scroll = row - s.height + 2
	}
	s.scrollBy(0) // clamp final
}

// cursorRow devuelve la fila (en rows completas) de la entrada bajo el cursor.
func (s sidebar) cursorRow() int {
	nav := 0
	for i, r := range s.rows(false) {
		if r.Entry != nil {
			if nav == s.cursor {
				return i
			}
			nav++
		}
	}
	return -1
}

// EntryAtVisibleRow mapea una fila EN PANTALLA (ventana con indicadores) a su entrada.
func (s sidebar) EntryAtVisibleRow(row int) (sidebarEntry, bool) {
	rows := s.visibleRows(false)
	if row < 0 || row >= len(rows) || rows[row].Entry == nil {
		return sidebarEntry{}, false
	}
	return *rows[row].Entry, true
}
```

- `view()` pasa a unir `visibleRows(focused)`.
- Llama `s.ensureCursorVisible()` al final de `moveCursor`, `toggle`, `setFilter` y
  `selectEntry`.
- `EntryAtRow` se elimina (su test de T1 se adapta a `EntryAtVisibleRow` con `height=0`
  → sin ventana, misma semántica).

`app.go`:
- `layout()`: en ambas ramas, `m.sidebar.height = m.bodyH` (una columna: `m.bodyH / 2`).
- `handleMouse` rueda: reemplaza la rama actual por:

```go
	if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown {
		delta := 3
		if msg.Button == tea.MouseButtonWheelUp {
			delta = -3
		}
		if msg.X < m.sidebarW {
			m.sidebar.scrollBy(delta)
			return nil
		}
		return m.events.Update(msg)
	}
```

- Click en sidebar: `EntryAtRow` → `EntryAtVisibleRow` (misma estructura del switch).

- [ ] **Step 4: Run to verify pass**

Run: `gofmt -w internal/tui/ && golangci-lint run && go vet ./... && go test ./... -count=1 && go build ./...`
Expected: PASS — todos los anclados al render, incluido el nuevo click-con-scroll.

- [ ] **Step 5: Smoke visual (temporal, borrar antes del commit)**

Render a 100×20 con `manyServices(40)`: verifica indicadores `↓ N more`, cursor-follow al
bajar, colapso de SERVICES dejando ver IMAGES/DATABASES, y filtro `/svc-3` con `(N/40)`.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/
git commit -m "feat(tui): scroll del sidebar con indicadores y rueda del mouse"
```

---

## Verificación final (no es una tarea)

- Suite completa `-count=1 -race` + lint + build.
- Smoke manual: `go run ./cmd/steerdemo` y `go run ./cmd/steer tui` con los 16 servicios:
  j/k por headers e items, enter/space/click togglea, `/` filtra en vivo (y `d` no dispara
  mientras tecleas), rueda scrollea el sidebar, click con scroll selecciona el correcto,
  overlays y deploy intactos.
