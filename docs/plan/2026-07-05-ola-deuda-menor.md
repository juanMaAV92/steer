# Ola de deuda menor — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Drenar los minors diferidos de los últimos 4 hitos: 3 ajustes de comportamiento votados, 5 correcciones mecánicas y 7 tests faltantes.

**Architecture:** Sin piezas nuevas: un sentinel (`core.ErrRepoNotFound`), un verdict más en la validación de deploy, un helper `closeForm()` en el Model, y el resto son ediciones puntuales y tests sobre los patrones ya existentes.

**Tech Stack:** Go, Bubble Tea, Cobra, testify.
Spec: `docs/superpowers/specs/2026-07-05-ola-deuda-menor-design.md`.

## Global Constraints

- Comentarios en español; strings de UI en inglés.
- PROHIBIDO cualquier atribución a Claude/IA en commits, comentarios o PRs (sin trailer Co-Authored-By).
- Branch de trabajo: `feat/ola-deuda-menor` (creada en Task 1 desde main).
- Antes de CADA commit: `gofmt -w internal/ && golangci-lint run && go vet ./... && go test ./... -count=1 && go build ./...` — todo verde.
- Los tests de click existentes pasan sin modificar (solo cambian 3 comentarios "modal" en app_test.go, que no tocan código de test).
- `closeForm()` restaura TAGS SOLO en cierres sin ejecución (esc/Cancel); los cierres con acción confirmada no restauran.
- Cualquier error del registry distinto de `ErrRepoNotFound` sigue degradando a skipped (estricta + degradable intacta).

---

### Task 1: Comportamiento — repo inexistente bloquea, esc restaura TAGS, reload conserva repos

**Files:**
- Modify: `internal/core/core.go` (sentinel)
- Modify: `internal/providers/aws/registry.go` (HasTag mapea RepositoryNotFoundException)
- Modify: `internal/providers/aws/registry_test.go`
- Modify: `internal/tui/messages.go` (verdict tagRepoNotFound)
- Modify: `internal/tui/app.go` (validateTagCmd, caso tagValidatedMsg, closeForm, reposMsg)
- Modify: `internal/tui/app_test.go`
- Modify: `internal/cli/service_cmd.go` (rama ErrRepoNotFound)
- Modify: `internal/cli/service_cmd_test.go`

**Interfaces:**
- Consumes: `Registry.HasTag`, `tagValidatedMsg{service, tag, repo, verdict}`, `sidebar.cursorEntry()`/`entryRepo`/`lastSelected`, `reposMsg`.
- Produces: `core.ErrRepoNotFound`; verdict `tagRepoNotFound`; `Model.closeForm()` (T2 no depende, pero lo usa el mismo archivo).

- [ ] **Step 1: Crear la branch**

```bash
git checkout main && git pull && git checkout -b feat/ola-deuda-menor
```

- [ ] **Step 2: Tests que fallan**

Añadir a `internal/providers/aws/registry_test.go`:

```go
func TestHasTagRepoNotFoundIsSentinel(t *testing.T) {
	api := &fakeECR{imagesErr: &ecrtypes.RepositoryNotFoundException{}}
	r := newRegistry(api, "")
	ok, err := r.HasTag(context.Background(), "nope-repo", "v1")
	require.False(t, ok)
	require.ErrorIs(t, err, core.ErrRepoNotFound)
}
```

(registry_test.go gana el import `"github.com/juanMaAV92/steer/internal/core"`.)

Añadir a `internal/tui/app_test.go`:

```go
// TestDeployBlocksWhenRepoMissing: repo inexistente es respuesta definitiva → bloquea.
func TestDeployBlocksWhenRepoMissing(t *testing.T) {
	reg := &coretest.FakeRegistry{HasTagErr: core.ErrRepoNotFound}
	m := newTestModelWithRegistry(servicesNamed("api"), reg)
	updated, cmd := m.Update(keyMsg("d"))
	m = updated.(Model)
	if cmd != nil {
		m = mustUpdate(t, m, cmd().(formTagsMsg))
	}
	for _, r := range "v1" {
		m = mustUpdate(t, m, keyMsg(string(r)))
	}
	updated, cmd = m.Update(keyMsg("enter"))
	m = updated.(Model)
	m = mustUpdate(t, m, cmd().(tagValidatedMsg))
	require.NotNil(t, m.form, "repo inexistente bloquea: el form queda abierto")
	require.Contains(t, stripANSI(m.View()), "repository api not found")
	require.False(t, m.deploy.Active)
}

// TestEscRestoresTagsPanel: cancelar una acción abierta desde un repo vuelve a TAGS.
func TestEscRestoresTagsPanel(t *testing.T) {
	reg := &coretest.FakeRegistry{
		Repos: []core.Repository{{Name: "api"}},
		Tags:  map[string][]core.ImageTag{"api": {{Tag: "v1", PushedAt: time.Now()}}},
	}
	m := newTestModelWithRegistry(servicesNamed("api"), reg)
	m = mustUpdate(t, m, reposMsg{repos: reg.Repos})
	m.sidebar.collapsed[sectionImages] = false
	clickX, clickY := findInView(t, m.View(), "▣ api")
	updated, cmd := m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: clickX, Y: clickY})
	m = updated.(Model)
	m = mustUpdate(t, m, cmd().(tagsMsg))
	require.Equal(t, sectionImages, m.sidebar.lastSelected)
	// abrir deploy (panel salta a Details con el form) y cancelar con esc
	updated, _ = m.Update(keyMsg("d"))
	m = updated.(Model)
	require.NotNil(t, m.form)
	require.Equal(t, sectionServices, m.sidebar.lastSelected)
	m = mustUpdate(t, m, keyMsg("esc"))
	require.Nil(t, m.form)
	require.Equal(t, sectionImages, m.sidebar.lastSelected, "esc restaura TAGS: el cursor sigue en el repo")
	require.Contains(t, stripANSI(m.View()), "TAGS")
}

// TestReloadErrorKeepsRepos: un Refresh fallido no borra la lista ya cargada.
func TestReloadErrorKeepsRepos(t *testing.T) {
	reg := &coretest.FakeRegistry{Repos: []core.Repository{{Name: "api"}}}
	m := newTestModelWithRegistry(servicesNamed("api"), reg)
	m = mustUpdate(t, m, reposMsg{repos: reg.Repos})
	m.sidebar.collapsed[sectionImages] = false
	m = mustUpdate(t, m, reposMsg{err: errors.New("throttled")})
	out := stripANSI(m.View())
	require.Contains(t, out, "▣ api", "los repos cargados siguen visibles")
	require.Contains(t, m.notice, "images refresh failed")
	// sin repos previos, el error sí muestra el estado de sección (comportamiento actual)
	m2 := newTestModelWithRegistry(nil, reg)
	m2 = mustUpdate(t, m2, reposMsg{err: errors.New("boom")})
	m2.sidebar.collapsed[sectionImages] = false
	require.Contains(t, stripANSI(m2.View()), "registry error: boom")
}
```

(app_test.go gana el import `"errors"` si no lo tiene.)

Añadir a `internal/cli/service_cmd_test.go`:

```go
func TestDeployBlocksWhenRepoMissingCLI(t *testing.T) {
	fake := &coretest.FakeDeployer{CurrentTagValue: "v1"}
	reg := &coretest.FakeRegistry{HasTagErr: core.ErrRepoNotFound}
	prev := newProviderFactoryFn
	newProviderFactoryFn = func() providers.ProviderFactory {
		return func(context.Context, config.Context) (providers.Provider, error) {
			return fakeProvider{dep: fake, reg: reg}, nil
		}
	}
	t.Cleanup(func() { newProviderFactoryFn = prev })
	_, err := runRoot(t, "service", "deploy", "-s", "catalog", "-t", "v2", "-y")
	require.ErrorContains(t, err, `repository "catalog" not found`)
	require.Empty(t, fake.DeployCalls)
}
```

- [ ] **Step 3: Verificar que fallan**

Run: `go test ./internal/providers/aws/ ./internal/tui/ ./internal/cli/ -run 'TestHasTagRepo|TestDeployBlocksWhenRepo|TestEscRestores|TestReloadErrorKeeps' -v`
Expected: FAIL — "undefined: core.ErrRepoNotFound" y comportamientos actuales (skipped/Details/lista borrada).

- [ ] **Step 4: Implementar**

`internal/core/core.go` (junto a ErrNoImagesConfig):

```go
// ErrRepoNotFound indica que el repositorio no existe en el registry. A diferencia
// de un fallo transitorio, es una respuesta definitiva: el deploy debe bloquearse.
var ErrRepoNotFound = errors.New("repository not found in registry")
```

`internal/providers/aws/registry.go`, en `HasTag`, antes del check de ImageNotFound:

```go
	var repoNotFound *ecrtypes.RepositoryNotFoundException
	if errors.As(err, &repoNotFound) {
		return false, core.ErrRepoNotFound
	}
```

`internal/tui/messages.go` — verdict nuevo:

```go
const (
	tagOK tagVerdict = iota
	tagNotFound
	tagRepoNotFound // el repo no existe: bloquea con mensaje propio
	tagSkipped
)
```

`internal/tui/app.go` — `validateTagCmd`, la rama de error de HasTag:

```go
		ok, err := reg.HasTag(ctx, repo, tag)
		if errors.Is(err, core.ErrRepoNotFound) {
			return tagValidatedMsg{service: service, tag: tag, repo: repo, verdict: tagRepoNotFound}
		}
		if err != nil {
			return tagValidatedMsg{service: service, tag: tag, repo: repo, verdict: tagSkipped}
		}
```

Caso `tagValidatedMsg`, añadir al switch:

```go
		case tagRepoNotFound:
			m.form.validating = false
			m.form.errMsg = "repository " + msg.repo + " not found"
			return m, nil
```

`closeForm` (junto a openActionKind) y sus 4 sitios de uso:

```go
// closeForm cierra el formulario SIN ejecutar acción y devuelve el panel a TAGS
// si el cursor del sidebar sigue sobre un repo (la acción se abrió desde ahí).
// Los cierres con acción confirmada NO pasan por aquí (van a Events/Details).
func (m *Model) closeForm() {
	m.form = nil
	if e, ok := m.sidebar.cursorEntry(); ok && e.Kind == entryRepo {
		m.sidebar.lastSelected = sectionImages
	}
}
```

- `handleFormKey`, bloque de congelación: `m.form = nil` → `m.closeForm()`.
- `handleFormKey`, `case key.Matches(msg, m.keys.Esc)`: ídem.
- `handleFormKey`, caso Enter: `if done { m.form = nil }` →
  `if done { if result == nil { m.closeForm() } else { m.form = nil } }`.
- `clickForm`, tras `activateIndex`: mismo patrón que el Enter.

Caso `reposMsg`, rama de error:

```go
		case msg.err != nil:
			if len(m.sidebar.repos) > 0 {
				// conservar la lista cargada: el fallo transitorio va como notice
				m.sidebar.imagesState = imagesReady
				m.notice = "images refresh failed: " + msg.err.Error()
			} else {
				m.sidebar.imagesState = imagesError
				m.sidebar.imagesErr = msg.err.Error()
			}
```

`internal/cli/service_cmd.go`, en el switch de validación del deploy:

```go
				switch found, herr := reg.HasTag(cmd.Context(), repo, tag); {
				case errors.Is(herr, core.ErrRepoNotFound):
					return fmt.Errorf("repository %q not found (check images.repo_template)", repo)
				case herr != nil:
					_, _ = fmt.Fprintln(cmd.ErrOrStderr(),
						render.Warn("warning: registry check skipped: "+herr.Error()))
				case !found:
					return fmt.Errorf("tag %q not found in repository %q", tag, repo)
				}
```

(service_cmd.go gana el import `"errors"` si no lo tiene.)

- [ ] **Step 5: Verificar que pasan + gates + commit**

```bash
gofmt -w internal/ && golangci-lint run && go vet ./... && go test ./... -count=1 && go build ./...
git add internal/
git commit -m "fix(tui,cli,providers): repo inexistente bloquea; esc restaura TAGS; reload fallido conserva repos"
```

---

### Task 2: Correcciones mecánicas

**Files:**
- Modify: `internal/tui/app.go` (tagsRepo huérfano, shortRepo, comentarios)
- Modify: `internal/tui/app_test.go` (comentarios "modal" + test del huérfano)
- Modify: `internal/tui/sidebar.go` (appendBlank único)

**Interfaces:**
- Consumes: caso `reposMsg` (con la forma que dejó T1), `deployedTagFor`, `panelBody`.
- Produces: `Model.shortRepo(repo string) string`.

- [ ] **Step 1: Test que falla — tagsRepo huérfano**

Añadir a `internal/tui/app_test.go`:

```go
// TestRepoVanishOnReloadResetsTags: si el repo seleccionado desaparece en un reload,
// el estado de tags se limpia y un tagsMsg tardío del repo desaparecido se ignora.
func TestRepoVanishOnReloadResetsTags(t *testing.T) {
	reg := &coretest.FakeRegistry{
		Repos: []core.Repository{{Name: "api"}},
		Tags:  map[string][]core.ImageTag{"api": {{Tag: "v1", PushedAt: time.Now()}}},
	}
	m := newTestModelWithRegistry(servicesNamed("api"), reg)
	m = mustUpdate(t, m, reposMsg{repos: reg.Repos})
	m.sidebar.collapsed[sectionImages] = false
	clickX, clickY := findInView(t, m.View(), "▣ api")
	updated, cmd := m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: clickX, Y: clickY})
	m = updated.(Model)
	m = mustUpdate(t, m, cmd().(tagsMsg))
	require.Equal(t, "api", m.tagsRepo)
	// reload exitoso SIN el repo → estado de tags limpio
	m = mustUpdate(t, m, reposMsg{repos: nil})
	require.Empty(t, m.tagsRepo)
	require.Nil(t, m.tags)
	// un tagsMsg tardío del repo desaparecido no revive nada
	m = mustUpdate(t, m, tagsMsg{repo: "api", tags: reg.Tags["api"]})
	require.Nil(t, m.tags)
}
```

- [ ] **Step 2: Verificar que falla**

Run: `go test ./internal/tui/ -run TestRepoVanishOnReload -v`
Expected: FAIL — `m.tagsRepo` sigue siendo "api" tras el reload.

- [ ] **Step 3: Implementar los 5 mecánicos**

3a. Caso `reposMsg`, rama `default` (tras `setRepos`):

```go
		default:
			m.sidebar.setRepos(msg.repos)
			// si el repo seleccionado desapareció, limpiar el estado de tags para
			// que una respuesta tardía suya no pase el guard de tagsMsg
			if _, ok := m.sidebar.selectedRepo(); !ok && m.tagsRepo != "" {
				m.tagsRepo, m.tags, m.tagsErr, m.tagsLoading = "", nil, "", false
			}
```

3b. Helper `shortRepo` (junto a `deployedTagFor`) y reemplazo de los 2 usos
(`panelBody` y `deployedTagFor`):

```go
// shortRepo devuelve el nombre de display del repo (sin el prefijo del contexto).
func (m Model) shortRepo(repo string) string {
	return strings.TrimPrefix(repo, m.current.RepoPrefix())
}
```

3c. Comentarios "modal" → formulario:
- `app.go:542`: "provocaría un misfire que abre el modal de acción erróneamente" →
  "provocaría un misfire que abre el formulario de acción erróneamente".
- `app_test.go:539`: "abre el modal de deploy" → "abre el formulario de deploy".
- `app_test.go:569`: "NO debe abrir el modal en modo una columna" → "NO debe abrir el
  formulario en modo una columna".
- `app_test.go:576`: "abre el modal con el actionKind correcto" → "abre el formulario
  con el actionKind correcto".

3d. Comentario en el guard de `tagValidatedMsg` (app.go), sobre la cláusula
`m.form.input != msg.tag`:

```go
		// (input != tag es defensivo: hoy inalcanzable porque el teclado queda
		// congelado durante la validación; protege a futuros productores externos)
```

3e. `sidebar.go`, bloque IMAGES de `rows()`: sacar el `appendBlank()` del if/else a una
sola llamada tras el bloque:

```go
	if !s.collapsed[sectionImages] {
		switch s.imagesState {
		// ... casos sin cambios ...
		}
	}
	appendBlank()
```

(El render resultante es idéntico: ambas ramas terminaban en `appendBlank()`. Los tests
de scroll/click existentes lo verifican al pasar sin cambios.)

- [ ] **Step 4: Verificar + gates + commit**

Run: `go test ./internal/tui/... -count=1` → PASS completo (ningún test anclado cambia).

```bash
gofmt -w internal/ && golangci-lint run && go vet ./... && go test ./... -count=1 && go build ./...
git add internal/tui/
git commit -m "refactor(tui): limpieza menor — tags huérfanos, shortRepo, comentarios al día"
```

---

### Task 3: Tests faltantes

**Files:**
- Modify: `internal/providers/aws/registry_test.go` (paginación)
- Modify: `internal/render/human_test.go` (fronteras Age)
- Modify: `internal/cli/service_cmd_test.go` (umbral discriminado)
- Modify: `internal/cli/image_cmd_test.go` (ReposErr/TagsErr)
- Modify: `internal/tui/app_test.go` (click Details con form abierto; e2e notFound→ok)

**Interfaces:**
- Consumes: todo existente; `fakeECR` gana modo paginado; `stuckDeployer` cambia su entrega de eventos.
- Produces: nada.

- [ ] **Step 1: Paginación ECR**

En `internal/providers/aws/registry_test.go`, dar a `fakeECR` un modo paginado:

```go
	pageSize int // >0: pagina las respuestas con NextToken (índice como token)
```

En `DescribeRepositories` y `DescribeImages`, cuando `pageSize > 0`:

```go
func paginate[T any](items []T, token *string, pageSize int) (page []T, next *string) {
	start := 0
	if token != nil {
		start, _ = strconv.Atoi(*token)
	}
	end := min(start+pageSize, len(items))
	page = items[start:end]
	if end < len(items) {
		next = awssdk.String(strconv.Itoa(end))
	}
	return page, next
}
```

(usar el helper en ambos métodos; import `"strconv"`). Tests:

```go
func TestListRepositoriesPaginates(t *testing.T) {
	api := &fakeECR{pageSize: 2, repos: []ecrtypes.Repository{
		{RepositoryName: awssdk.String("a")},
		{RepositoryName: awssdk.String("b")},
		{RepositoryName: awssdk.String("c")},
	}}
	repos, err := newRegistry(api, "").ListRepositories(context.Background())
	require.NoError(t, err)
	require.Len(t, repos, 3, "debe recorrer todas las páginas")
}

func TestListTagsPaginates(t *testing.T) {
	now := time.Now()
	img := func(tag string, d time.Duration) ecrtypes.ImageDetail {
		return ecrtypes.ImageDetail{ImageTags: []string{tag},
			ImageDigest: awssdk.String("sha256:x"), ImageSizeInBytes: awssdk.Int64(1),
			ImagePushedAt: awssdk.Time(now.Add(-d))}
	}
	api := &fakeECR{pageSize: 2, images: []ecrtypes.ImageDetail{
		img("v1", 3*time.Hour), img("v2", 2*time.Hour), img("v3", time.Hour),
	}}
	tags, err := newRegistry(api, "").ListTags(context.Background(), "repo")
	require.NoError(t, err)
	require.Len(t, tags, 3)
	require.Equal(t, "v3", tags[0].Tag) // el orden global sobrevive a la paginación
}
```

Verificar RED lógico: si `ListRepositories` no siguiera `NextToken`, devolvería 2 —
comprobar mutando mentalmente; los tests deben pasar directo (la paginación ya está
implementada; esto es cobertura). Si alguno FALLA, es un bug real: investigar antes
de seguir.

- [ ] **Step 2: Fronteras de Age**

En `internal/render/human_test.go`, añadir a `TestAge`:

```go
	// fronteras exactas de cada rama
	require.Equal(t, "1m ago", Age(now.Add(-time.Minute), now))
	require.Equal(t, "1h ago", Age(now.Add(-time.Hour), now))
	require.Equal(t, "1d ago", Age(now.Add(-24*time.Hour), now))
```

- [ ] **Step 3: Umbral CLI discriminado**

En `internal/cli/service_cmd_test.go`, reescribir la entrega de `stuckDeployer` para
que el 3er fallo llegue en una consulta posterior (más nuevos primero, dedup por ID):

```go
func (d *stuckDeployer) ServiceEvents(_ context.Context, _ string) ([]core.ServiceEvent, error) {
	d.calls++
	ev := func(i int) core.ServiceEvent {
		return core.ServiceEvent{ID: "ev-" + strconv.Itoa(i), At: time.Now(),
			Message: "CannotPullContainerError: pull image manifest has been retried", IsError: true}
	}
	switch {
	case d.calls == 1:
		return nil, nil // baseline del watch
	case d.calls == 2:
		return []core.ServiceEvent{ev(1), ev(0)}, nil // 2 fallos: NO corta
	default:
		return []core.ServiceEvent{ev(2), ev(1), ev(0)}, nil // 3º: corta
	}
}
```

y en `TestWatchRolloutDetectsStuck`, añadir al final:

```go
	require.GreaterOrEqual(t, dep.calls, 3, "con 2 fallos el watch debe seguir; corta al 3º")
```

- [ ] **Step 4: Errores del registry en CLI**

Añadir a `internal/cli/image_cmd_test.go`:

```go
func TestImageLsAbortsOnRegistryError(t *testing.T) {
	withFakeRegistry(t, &coretest.FakeRegistry{ReposErr: errors.New("ecr down")})
	_, err := runRoot(t, "image", "ls")
	require.ErrorContains(t, err, "ecr down")
}

func TestImageTagsAbortsOnRegistryError(t *testing.T) {
	withFakeRegistry(t, &coretest.FakeRegistry{TagsErr: errors.New("ecr down")})
	_, err := runRoot(t, "image", "tags", "-r", "api")
	require.ErrorContains(t, err, "ecr down")
}
```

(import `"errors"` si falta.)

- [ ] **Step 5: TUI — click en Details con form abierto + e2e notFound→ok**

Añadir a `internal/tui/app_test.go`:

```go
// TestClickDetailsButtonsWithFormOpenIsNoop: con el form abierto, el click en la fila
// de botones de Details (visible encima) no reabre otra acción.
func TestClickDetailsButtonsWithFormOpenIsNoop(t *testing.T) {
	m := newTestModel(sampleServices())
	m = mustUpdate(t, m, keyMsg("d"))
	require.Equal(t, actionDeploy, m.form.kind)
	clickX, clickY := findInView(t, m.View(), "Scale (s)")
	m = mustUpdate(t, m, tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: clickX, Y: clickY})
	require.NotNil(t, m.form)
	require.Equal(t, actionDeploy, m.form.kind, "el click no debe cambiar la acción abierta")
}

// TestDeployNotFoundThenRetryOK: e2e del loop corregir-y-reintentar.
func TestDeployNotFoundThenRetryOK(t *testing.T) {
	reg := &coretest.FakeRegistry{Tags: map[string][]core.ImageTag{
		"api": {{Tag: "v9.9", PushedAt: time.Now().Add(-time.Hour)}},
	}}
	m := newTestModelWithRegistry(servicesNamed("api"), reg)
	updated, cmd := m.Update(keyMsg("d"))
	m = updated.(Model)
	m = mustUpdate(t, m, cmd().(formTagsMsg))
	for _, r := range "bad" {
		m = mustUpdate(t, m, keyMsg(string(r)))
	}
	updated, cmd = m.Update(keyMsg("enter"))
	m = updated.(Model)
	m = mustUpdate(t, m, cmd().(tagValidatedMsg)) // notFound
	require.Contains(t, stripANSI(m.View()), "tag not found")
	// corregir: teclear limpia el error; borrar "bad" y poner el tag bueno
	for range 3 {
		m = mustUpdate(t, m, tea.KeyMsg{Type: tea.KeyBackspace})
	}
	require.Empty(t, m.form.errMsg, "editar limpia la línea de error")
	for _, r := range "v9.9" {
		m = mustUpdate(t, m, keyMsg(string(r)))
	}
	updated, cmd = m.Update(keyMsg("enter"))
	m = updated.(Model)
	updated, cmd = m.Update(cmd().(tagValidatedMsg)) // ok
	m = updated.(Model)
	require.Nil(t, m.form)
	require.True(t, m.deploy.Active)
	require.Equal(t, []string{"api/bad", "api/v9.9"}, reg.HasTagCalls)
}
```

- [ ] **Step 6: rollout_test.go — verificación no-op**

El assert duplicado reportado en la remediación YA no existe (`TestRolloutContainsState`
es un loop único y limpio). No hay cambio; dejar constancia en el reporte de la task.

- [ ] **Step 7: Verificar todo + gates + commit**

Run: `go test ./... -count=1` → PASS completo.

```bash
gofmt -w internal/ && golangci-lint run && go vet ./... && go test ./... -count=1 && go build ./...
git add internal/
git commit -m "test: cobertura de paginación ECR, fronteras de Age, umbral de atasco y errores del registry"
```
