# Deploy seguro — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** El deploy valida el tag contra el registry antes de tocar ECS (estricto si no existe, degradable si el registry falla) y el watch detecta rollouts atascados por fallos de pull en vez de poll-ear para siempre.

**Architecture:** `core.Registry` gana `HasTag` (consulta puntual, no depende del tope de 50 del picker) y core un helper `IsProvisioningFailure` para clasificar eventos de fallo de aprovisionamiento. El formulario de deploy del TUI se queda abierto "validating tag…" hasta el veredicto (ok/notFound/skipped); el CLI valida síncrono antes del preview. El watch (TUI y CLI `-w`) cuenta eventos de fallo y al 3º marca STUCK, detiene el poll y sugiere rollback.

**Tech Stack:** Go, aws-sdk-go-v2/service/ecr (ya en go.mod), Bubble Tea, Cobra, testify.
Spec: `docs/superpowers/specs/2026-07-05-deploy-seguro-design.md`.

## Global Constraints

- Comentarios en español; strings de UI en inglés.
- PROHIBIDO cualquier atribución a Claude/IA en commits, comentarios o PRs.
- Branch de trabajo: `feat/deploy-seguro` (creada en Task 1 desde main).
- Antes de CADA commit: `gofmt -w internal/ && golangci-lint run && go vet ./... && go test ./... -count=1 && go build ./...` — todo verde.
- Validación **estricta + degradable**: tag inexistente bloquea SIEMPRE; error del registry o contexto sin `[images]` degrada con aviso y el deploy continúa. La validación nunca es punto único de fallo.
- Umbral de atasco: **3** eventos de fallo de aprovisionamiento en el rollout actual.
- Tests de TUI anclados al render (patrón `findInView`/`stripANSI`); los tests de click existentes pasan sin modificar.
- Scale y rollback NO cambian (no hay tag que validar).

---

### Task 1: `Registry.HasTag` + `core.IsProvisioningFailure` (contrato, fake y ECR)

**Files:**
- Modify: `internal/core/core.go` (HasTag en la interface + IsProvisioningFailure)
- Create: `internal/core/core_provisioning_test.go`
- Modify: `internal/core/coretest/fake_registry.go`
- Modify: `internal/providers/aws/registry.go`
- Modify: `internal/providers/aws/registry_test.go`

**Interfaces:**
- Consumes: `ecrAPI.DescribeImages` (ya en la interface — no crece), `ecrtypes.ImageNotFoundException`.
- Produces (T2-T4 dependen):
  - `Registry` gana `HasTag(ctx context.Context, repo, tag string) (bool, error)`
  - `core.IsProvisioningFailure(msg string) bool`
  - `FakeRegistry` gana `HasTagErr error` y `HasTagCalls []string` ("repo/tag"); `HasTag` busca en `Tags[repo]`.

- [ ] **Step 1: Crear la branch**

```bash
git checkout main && git pull && git checkout -b feat/deploy-seguro
```

- [ ] **Step 2: Tests que fallan**

Crear `internal/core/core_provisioning_test.go`:

```go
package core

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsProvisioningFailure(t *testing.T) {
	require.True(t, IsProvisioningFailure("(service x) was unable to place a task. Reason: CannotPullContainerError: pull image manifest has been retried"))
	require.True(t, IsProvisioningFailure("CANNOTPULLCONTAINERERROR: image not found")) // case-insensitive
	require.True(t, IsProvisioningFailure("(service x) WAS UNABLE TO PLACE A TASK"))
	require.False(t, IsProvisioningFailure("(service x) has started 1 tasks"))
	require.False(t, IsProvisioningFailure("(service x) has reached a steady state."))
	require.False(t, IsProvisioningFailure(""))
}
```

Añadir a `internal/providers/aws/registry_test.go` (el `fakeECR` existente gana un modo de error y captura del input):

```go
// imagesErrFn permite inyectar la respuesta de DescribeImages por llamada.
// (añadir campo a fakeECR: `imagesErr error` y `lastImagesInput *ecr.DescribeImagesInput`;
// en DescribeImages: guardar `f.lastImagesInput = in` y si f.imagesErr != nil devolverlo)

func TestHasTagFoundAndNotFound(t *testing.T) {
	api := &fakeECR{images: []ecrtypes.ImageDetail{{ImageTags: []string{"v1"}}}}
	r := newRegistry(api, "")
	ok, err := r.HasTag(context.Background(), "nao-v2-shared-api", "v1")
	require.NoError(t, err)
	require.True(t, ok)
	// la consulta es puntual: DescribeImages recibe el ImageIds con el tag
	require.Len(t, api.lastImagesInput.ImageIds, 1)
	require.Equal(t, "v1", awssdk.ToString(api.lastImagesInput.ImageIds[0].ImageTag))

	api.imagesErr = &ecrtypes.ImageNotFoundException{}
	ok, err = r.HasTag(context.Background(), "nao-v2-shared-api", "nope")
	require.NoError(t, err) // not found NO es error: es la respuesta
	require.False(t, ok)
}

func TestHasTagPropagatesRealErrors(t *testing.T) {
	api := &fakeECR{imagesErr: errors.New("throttled")}
	r := newRegistry(api, "")
	_, err := r.HasTag(context.Background(), "repo", "v1")
	require.ErrorContains(t, err, "throttled")
}
```

(Imports nuevos en registry_test.go: `"errors"`.)

- [ ] **Step 3: Verificar que fallan**

Run: `go test ./internal/core/ ./internal/providers/aws/ -run 'TestIsProvisioning|TestHasTag' -v`
Expected: FAIL con "undefined: IsProvisioningFailure" / "r.HasTag undefined"

- [ ] **Step 4: Implementar**

En `internal/core/core.go`, dentro de la interface `Registry` (tras ListTags):

```go
	// HasTag verifica si el tag existe en el repo. Consulta puntual: no depende
	// del tope de ListTags (valida tags viejos que el picker no muestra).
	HasTag(ctx context.Context, repo, tag string) (bool, error)
```

y el helper (junto a los tipos de eventos):

```go
// IsProvisioningFailure detecta eventos de fallo de aprovisionamiento en el texto
// que reporta el provider. Heurística documentada por provider (ECS hoy): un
// rollout que acumula estos eventos está atascado reintentando (p. ej. un tag
// que no existe) y ECS no lo reporta como FAILED sin circuit breaker.
func IsProvisioningFailure(msg string) bool {
	m := strings.ToLower(msg)
	return strings.Contains(m, "cannotpullcontainererror") ||
		strings.Contains(m, "unable to place a task")
}
```

(`core.go` gana el import `"strings"`.)

En `internal/core/coretest/fake_registry.go`:

```go
	HasTagErr   error
	HasTagCalls []string // "repo/tag", en orden
```

```go
func (f *FakeRegistry) HasTag(_ context.Context, repo, tag string) (bool, error) {
	f.HasTagCalls = append(f.HasTagCalls, repo+"/"+tag)
	if f.HasTagErr != nil {
		return false, f.HasTagErr
	}
	for _, t := range f.Tags[repo] {
		if t.Tag == tag {
			return true, nil
		}
	}
	return false, nil
}
```

En `internal/providers/aws/registry.go` (imports nuevos: `"errors"`, `ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"`):

```go
// HasTag verifica el tag con una consulta puntual; ImageNotFoundException es la
// respuesta "no existe", no un error.
func (r *ECRRegistry) HasTag(ctx context.Context, repo, tag string) (bool, error) {
	_, err := r.api.DescribeImages(ctx, &ecr.DescribeImagesInput{
		RepositoryName: awssdk.String(repo),
		ImageIds:       []ecrtypes.ImageIdentifier{{ImageTag: awssdk.String(tag)}},
	})
	var notFound *ecrtypes.ImageNotFoundException
	if errors.As(err, &notFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
```

Y en `fakeECR` (registry_test.go) los campos/captura descritos en Step 2.

- [ ] **Step 5: Verificar que pasan + gates + commit**

Run: `go test ./... -count=1` → PASS (FakeRegistry y ECRRegistry satisfacen la interface ampliada; ningún otro implementor de `core.Registry` existe).

```bash
gofmt -w internal/ && golangci-lint run && go vet ./... && go test ./... -count=1 && go build ./...
git add internal/core/ internal/providers/aws/
git commit -m "feat(core,providers): Registry.HasTag e IsProvisioningFailure"
```

---

### Task 2: TUI — validación del tag en el confirm del deploy

**Files:**
- Modify: `internal/tui/form.go` (estado validating/errMsg + geometría)
- Modify: `internal/tui/form_test.go`
- Modify: `internal/tui/messages.go` (tagValidatedMsg)
- Modify: `internal/tui/app.go` (interceptar confirm de deploy, validateTagCmd, caso tagValidatedMsg, guards en handleFormKey/clickForm)
- Modify: `internal/tui/app_test.go`

**Interfaces:**
- Consumes: `Registry.HasTag`, `core.ErrNoImagesConfig` (T1); `actionForm` actual (`activate`, `buttonRow() = 3 + len(visibleTags())`, `tagAt` con base 3); `applyActionConfirmed`; `newTestModelWithRegistry`, `servicesNamed`.
- Produces: `actionForm.validating bool`, `errMsg string`, `statusRows() int`; `tagValidatedMsg{service, tag, repo string, verdict tagVerdict}` con `tagOK|tagNotFound|tagSkipped`; `Model.validateTagCmd(service, tag string) tea.Cmd`.

- [ ] **Step 1: Tests que fallan (form unit + integración)**

Añadir a `internal/tui/form_test.go`:

```go
func TestFormStatusRowShiftsGeometry(t *testing.T) {
	f := newActionForm(actionDeploy, "api")
	require.Equal(t, 3, f.buttonRow())
	f.validating = true
	require.Equal(t, 1, f.statusRows())
	require.Equal(t, 4, f.buttonRow()) // la línea de estado empuja los botones
	require.Contains(t, stripANSI(f.view()), "validating tag…")
	f.validating = false
	f.errMsg = "tag not found in nao-v2-shared-api"
	require.Equal(t, 4, f.buttonRow())
	require.Contains(t, stripANSI(f.view()), "tag not found in nao-v2-shared-api")
	// con picker: estado + tags desplazan juntos
	f.setTags(pickerTags())
	require.Equal(t, 4+3, f.buttonRow())
	require.Equal(t, 0, f.tagAt(4)) // los tags empiezan tras la línea de estado
	require.Equal(t, -1, f.tagAt(3))
}
```

Añadir a `internal/tui/app_test.go`:

```go
// TestDeployValidatesTagNotFound: confirmar con tag inexistente mantiene el form
// abierto con el error; corregir y reconfirmar con tag válido despliega.
func TestDeployValidatesTagNotFound(t *testing.T) {
	reg := &coretest.FakeRegistry{Tags: map[string][]core.ImageTag{
		"api": {{Tag: "v9.9", PushedAt: time.Now().Add(-time.Hour)}},
	}}
	m := newTestModelWithRegistry(servicesNamed("api"), reg)
	updated, cmd := m.Update(keyMsg("d"))
	m = updated.(Model)
	m = mustUpdate(t, m, cmd().(formTagsMsg))
	for _, r := range "nope" {
		m = mustUpdate(t, m, keyMsg(string(r)))
	}
	// enter dispara la validación: el form queda abierto validando
	updated, cmd = m.Update(keyMsg("enter"))
	m = updated.(Model)
	require.NotNil(t, m.form, "el form no se cierra hasta el veredicto")
	require.True(t, m.form.validating)
	require.NotNil(t, cmd)
	// enter y clicks quedan inertes mientras valida
	updated, c2 := m.Update(keyMsg("enter"))
	m = updated.(Model)
	require.Nil(t, c2)
	require.True(t, m.form.validating)
	// veredicto notFound: form abierto con la línea roja, sin deploy
	m = mustUpdate(t, m, cmd().(tagValidatedMsg))
	require.NotNil(t, m.form)
	require.False(t, m.form.validating)
	require.Contains(t, stripANSI(m.View()), "tag not found in api")
	require.False(t, m.deploy.Active)
	require.Equal(t, []string{"api/nope"}, reg.HasTagCalls)
}

// TestDeployValidatesTagOKStartsDeploy: veredicto ok cierra el form y arranca el flujo.
func TestDeployValidatesTagOKStartsDeploy(t *testing.T) {
	reg := &coretest.FakeRegistry{Tags: map[string][]core.ImageTag{
		"api": {{Tag: "v9.9", PushedAt: time.Now().Add(-time.Hour)}},
	}}
	m := newTestModelWithRegistry(servicesNamed("api"), reg)
	updated, cmd := m.Update(keyMsg("d"))
	m = updated.(Model)
	m = mustUpdate(t, m, cmd().(formTagsMsg))
	m = mustUpdate(t, m, tea.KeyMsg{Type: tea.KeyDown}) // pick v9.9
	updated, cmd = m.Update(keyMsg("enter"))
	m = updated.(Model)
	require.True(t, m.form.validating)
	updated, cmd = m.Update(cmd().(tagValidatedMsg))
	m = updated.(Model)
	require.Nil(t, m.form)
	require.NotNil(t, cmd) // startDeployCmd
	require.Equal(t, panel.TabEvents, m.tabs.Active)
	require.True(t, m.deploy.Active)
}

// TestDeploySkippedWithoutRegistryStillDeploys: sin [images] el check se salta con aviso.
func TestDeploySkippedWithoutRegistryStillDeploys(t *testing.T) {
	m := newTestModel(sampleServices()) // sin registry → Registry() = ErrNoImagesConfig
	m = mustUpdate(t, m, keyMsg("d"))
	for _, r := range "v2" {
		m = mustUpdate(t, m, keyMsg(string(r)))
	}
	updated, cmd := m.Update(keyMsg("enter"))
	m = updated.(Model)
	require.NotNil(t, cmd)
	updated, cmd = m.Update(cmd().(tagValidatedMsg)) // verdict: tagSkipped
	m = updated.(Model)
	require.Nil(t, m.form)
	require.NotNil(t, cmd) // el deploy continúa
	require.True(t, m.deploy.Active)
	require.Contains(t, m.notice, "registry check skipped")
}

// TestStaleTagValidatedIgnored: un veredicto tras esc no revive nada.
func TestStaleTagValidatedIgnored(t *testing.T) {
	m := newTestModel(sampleServices())
	m = mustUpdate(t, m, keyMsg("d"))
	for _, r := range "v2" {
		m = mustUpdate(t, m, keyMsg(string(r)))
	}
	updated, cmd := m.Update(keyMsg("enter"))
	m = updated.(Model)
	msg := cmd().(tagValidatedMsg)
	m = mustUpdate(t, m, keyMsg("esc")) // cancela durante la validación
	require.Nil(t, m.form)
	updated, c2 := m.Update(msg)
	m = updated.(Model)
	require.Nil(t, m.form)
	require.Nil(t, c2)
	require.False(t, m.deploy.Active)
}
```

- [ ] **Step 2: Verificar que fallan**

Run: `go test ./internal/tui/ -run 'TestFormStatusRow|TestDeployValidates|TestDeploySkipped|TestStaleTagValidated' -v`
Expected: FAIL con "f.validating undefined" / "undefined: tagValidatedMsg"

- [ ] **Step 3: Implementar el formulario (`internal/tui/form.go`)**

Campos nuevos en `actionForm`:

```go
	validating bool   // consulta HasTag en vuelo: botones y teclado inertes (esc cancela)
	errMsg     string // veredicto notFound: línea roja bajo el prompt
```

Métodos/ajustes:

```go
// statusRows: fila opcional de estado (validating o error) entre prompt y tags.
func (f actionForm) statusRows() int {
	if f.validating || f.errMsg != "" {
		return 1
	}
	return 0
}
```

- `buttonRow()`: `return 3 + f.statusRows() + len(f.visibleTags())`
- `tagAt(row)`: la base pasa de 3 a `3 + f.statusRows()`:

```go
func (f actionForm) tagAt(row int) int {
	base := 3 + f.statusRows()
	n := len(f.visibleTags())
	if n == 0 || row < base || row >= base+n {
		return -1
	}
	return row - base
}
```

- `view()`: tras `prompt`, insertar la fila de estado antes del picker:

```go
	rows := []string{render.Bold(title), prompt}
	if f.validating {
		rows = append(rows, render.Dim("validating tag…"))
	} else if f.errMsg != "" {
		rows = append(rows, render.Danger(f.errMsg))
	}
```

- Actualizar el comentario de geometría del archivo: «borde(0), título(1), prompt(2),
  estado(3 si hay), tags(…), botones, borde».
- `typeKey`: al editar, limpiar el error de un intento previo: añadir `f.errMsg = ""`
  en ambos casos (Backspace y Runes).

- [ ] **Step 4: Cablear en `app.go` + messages.go**

En `messages.go`:

```go
// tagVerdict es el resultado de validar un tag contra el registry.
type tagVerdict int

const (
	tagOK tagVerdict = iota
	tagNotFound
	tagSkipped // sin [images] o registry con error: se despliega sin verificar
)

type tagValidatedMsg struct {
	service string
	tag     string
	repo    string
	verdict tagVerdict
}
```

En `app.go` — el comando:

```go
// validateTagCmd consulta HasTag para el repo hermano del servicio. Estricta +
// degradable: notFound bloquea; error del registry o sin [images] → skipped.
func (m Model) validateTagCmd(service, tag string) tea.Cmd {
	provider := m.provider
	ctx := m.runCtx
	short := strings.TrimPrefix(service, m.current.Prefix())
	repo := m.current.RepoName(short)
	return func() tea.Msg {
		reg, err := provider.Registry()
		if err != nil {
			return tagValidatedMsg{service: service, tag: tag, repo: repo, verdict: tagSkipped}
		}
		ok, err := reg.HasTag(ctx, repo, tag)
		if err != nil {
			return tagValidatedMsg{service: service, tag: tag, repo: repo, verdict: tagSkipped}
		}
		if !ok {
			return tagValidatedMsg{service: service, tag: tag, repo: repo, verdict: tagNotFound}
		}
		return tagValidatedMsg{service: service, tag: tag, repo: repo, verdict: tagOK}
	}
}
```

Caso en `Update`:

```go
	case tagValidatedMsg:
		// guard de obsolescencia: el form debe seguir abierto validando ESTE tag
		if m.form == nil || m.form.kind != actionDeploy || !m.form.validating ||
			m.form.service != msg.service || m.form.input != msg.tag {
			return m, nil
		}
		switch msg.verdict {
		case tagNotFound:
			m.form.validating = false
			m.form.errMsg = "tag not found in " + msg.repo
			return m, nil
		case tagSkipped:
			m.notice = "registry check skipped — deploying unverified tag"
		}
		m.form = nil
		cmd := m.applyActionConfirmed(actionConfirmedMsg{kind: actionDeploy, service: msg.service, input: msg.tag})
		return m, cmd
```

`handleFormKey` — congelar durante la validación (al inicio del método) e interceptar
el confirm de deploy (en el caso Enter):

```go
	if m.form.validating {
		if key.Matches(msg, m.keys.Esc) {
			m.form = nil // esc sigue cancelando; el veredicto llegará obsoleto
		}
		return m, nil
	}
```

```go
	case key.Matches(msg, m.keys.Enter):
		done, result := m.form.activate()
		if r, ok := result.(actionConfirmedMsg); ok && r.kind == actionDeploy {
			// deploy no arranca directo: primero se valida el tag (el form queda abierto)
			m.form.validating = true
			m.form.errMsg = ""
			return m, m.validateTagCmd(r.service, r.input)
		}
		if done {
			m.form = nil
		}
		if result != nil {
			return m.handleOverlayResult(result)
		}
```

`clickForm` — mismo patrón: al inicio, `if m.form.validating { return nil }`; y en el
camino del botón:

```go
	done, result := m.form.activateIndex(idx)
	if r, ok := result.(actionConfirmedMsg); ok && r.kind == actionDeploy {
		m.form.validating = true
		m.form.errMsg = ""
		return m.validateTagCmd(r.service, r.input)
	}
	if done {
		m.form = nil
	}
	if r, ok := result.(actionConfirmedMsg); ok {
		return m.applyActionConfirmed(r)
	}
	return nil
```

- [ ] **Step 5: Verificar todo + gates + commit**

Run: `go test ./internal/tui/... -count=1` → PASS completo (los tests de click del form
existentes no cambian: sin validating/errMsg la geometría es idéntica).

```bash
gofmt -w internal/ && golangci-lint run && go vet ./... && go test ./... -count=1 && go build ./...
git add internal/tui/
git commit -m "feat(tui): validación del tag contra el registry antes del deploy"
```

---

### Task 3: CLI — validación síncrona antes del preview

**Files:**
- Modify: `internal/cli/service_cmd.go:146-148` (antes de `CurrentTag`)
- Modify: `internal/cli/service_cmd_test.go`

**Interfaces:**
- Consumes: `AppContext.Registry(ctx)` (existente), `Registry.HasTag` (T1), `Context.RepoName`.
- Produces: nada aguas abajo.

- [ ] **Step 1: Tests que fallan**

Añadir a `internal/cli/service_cmd_test.go` (usa `withFakeRegistry` de image_cmd_test.go
y `sampleTags()`):

```go
func TestDeployBlocksUnknownTag(t *testing.T) {
	reg := &coretest.FakeRegistry{Tags: map[string][]core.ImageTag{"catalog": sampleTags()}}
	withFakeRegistry(t, reg)
	_, err := runRoot(t, "service", "deploy", "-s", "catalog", "-t", "nope", "-y")
	require.ErrorContains(t, err, `tag "nope" not found in repository "catalog"`)
	require.Equal(t, []string{"catalog/nope"}, reg.HasTagCalls)
}

func TestDeployDegradesOnRegistryError(t *testing.T) {
	fake := &coretest.FakeDeployer{CurrentTagValue: "v1"}
	reg := &coretest.FakeRegistry{HasTagErr: errors.New("throttled")}
	prev := newProviderFactoryFn
	newProviderFactoryFn = func() providers.ProviderFactory {
		return func(context.Context, config.Context) (providers.Provider, error) {
			return fakeProvider{dep: fake, reg: reg}, nil
		}
	}
	t.Cleanup(func() { newProviderFactoryFn = prev })
	out, err := runRoot(t, "service", "deploy", "-s", "catalog", "-t", "v2", "-y")
	require.NoError(t, err)
	require.Equal(t, []string{"catalog/v2"}, fake.DeployCalls) // desplegó igual
	_ = out // el warning va a stderr; el flujo estándar no cambia
}

func TestDeployWithoutImagesConfigStillDeploys(t *testing.T) {
	fake := &coretest.FakeDeployer{CurrentTagValue: "v1"}
	withFakeDeployer(t, fake) // fakeProvider.reg nil → ErrNoImagesConfig
	_, err := runRoot(t, "service", "deploy", "-s", "catalog", "-t", "v2", "-y")
	require.NoError(t, err)
	require.Equal(t, []string{"catalog/v2"}, fake.DeployCalls)
}
```

(Imports nuevos si faltan: `errors`.)

- [ ] **Step 2: Verificar que fallan**

Run: `go test ./internal/cli/ -run 'TestDeployBlocks|TestDeployDegrades|TestDeployWithoutImages' -v`
Expected: TestDeployBlocksUnknownTag FAIL (hoy despliega "nope" sin chistar); los otros
dos pueden pasar ya — se quedan como contrato de regresión.

- [ ] **Step 3: Implementar**

En `newServiceDeployCmd`, tras `realName := app.Ctx.ServiceName(service)` y antes de
`dep.CurrentTag(...)`:

```go
			// validación del tag contra el registry: estricta si no existe,
			// degradable si el registry no está disponible (nunca bloquea CI).
			if reg, rerr := app.Registry(cmd.Context()); rerr == nil {
				repo := app.Ctx.RepoName(service)
				switch found, herr := reg.HasTag(cmd.Context(), repo, tag); {
				case herr != nil:
					_, _ = fmt.Fprintln(cmd.ErrOrStderr(),
						render.Warn("warning: registry check skipped: "+herr.Error()))
				case !found:
					return fmt.Errorf("tag %q not found in repository %q", tag, repo)
				}
			} else {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(),
					render.Warn("warning: registry check skipped (images not configured or registry unavailable)"))
			}
```

- [ ] **Step 4: Verificar que pasan + gates + commit**

```bash
gofmt -w internal/ && golangci-lint run && go vet ./... && go test ./... -count=1 && go build ./...
git add internal/cli/
git commit -m "feat(cli): deploy valida el tag contra el registry antes del preview"
```

---

### Task 4: Watch — detección de rollout atascado (TUI y CLI `-w`)

**Files:**
- Modify: `internal/tui/app.go` (deployState + caso deployPollMsg)
- Modify: `internal/tui/app_test.go`
- Modify: `internal/cli/service_cmd.go` (watchRollout)
- Modify: `internal/cli/service_cmd_test.go`

**Interfaces:**
- Consumes: `core.IsProvisioningFailure` (T1); `deployState` (`internal/tui/app.go:45-52`); `watchRollout(ctx, out, dep, service, interval)` (`internal/cli/service_cmd.go:192`).
- Produces: `deployState.PullErrors int`; `watchRollout` gana el parámetro `short string` (para la sugerencia de rollback).

- [ ] **Step 1: Tests que fallan (TUI)**

Añadir a `internal/tui/app_test.go`:

```go
// stuckEvents fabrica n eventos de fallo de aprovisionamiento con IDs únicos.
func stuckEvents(n int) []core.ServiceEvent {
	out := make([]core.ServiceEvent, n)
	for i := range out {
		out[i] = core.ServiceEvent{
			ID:      "ev-" + strconv.Itoa(i),
			At:      time.Now(),
			Message: "(service x) was unable to place a task. Reason: CannotPullContainerError",
			IsError: true,
		}
	}
	return out
}

// TestWatchStuckAfterThreePullErrors: al 3er fallo el poll se detiene y avisa.
func TestWatchStuckAfterThreePullErrors(t *testing.T) {
	m := newTestModel(sampleServices())
	m.deploy = deployState{Active: true, Service: "api"}
	poll := func(evs []core.ServiceEvent) deployPollMsg {
		return deployPollMsg{events: evs, rollout: core.RolloutInProgress, desired: 1}
	}
	// 2 fallos: sigue poll-eando
	updated, cmd := m.Update(poll(stuckEvents(2)))
	m = updated.(Model)
	require.True(t, m.deploy.Active)
	require.Equal(t, 2, m.deploy.PullErrors)
	require.NotNil(t, cmd) // deployTickCmd
	// 3er fallo: STUCK — poll detenido, mensaje visible, R sigue vivo
	updated, cmd = m.Update(poll(stuckEvents(1)))
	m = updated.(Model)
	require.False(t, m.deploy.Active)
	require.True(t, m.deploy.Done)
	require.NotNil(t, cmd) // loadServicesCmd (refresca la lista, como done/failed)
	require.Contains(t, stripANSI(m.events.View()), "deployment stuck")
	// el tick huérfano no reprograma nada
	_, c2 := m.Update(deployPollTickMsg{})
	require.Nil(t, c2)
}

// TestWatchEventCounterIgnoresNormalEvents: eventos sanos no suman.
func TestWatchEventCounterIgnoresNormalEvents(t *testing.T) {
	m := newTestModel(sampleServices())
	m.deploy = deployState{Active: true, Service: "api"}
	evs := []core.ServiceEvent{{ID: "a", At: time.Now(), Message: "(service x) has started 1 tasks"}}
	updated, _ := m.Update(deployPollMsg{events: evs, rollout: core.RolloutInProgress})
	m = updated.(Model)
	require.Equal(t, 0, m.deploy.PullErrors)
	require.True(t, m.deploy.Active)
}
```

Y a `internal/cli/service_cmd_test.go`:

```go
// stuckDeployer entrega eventos de fallo de pull a partir de la 2ª consulta
// (la 1ª es el baseline del watch) y un rollout que nunca progresa.
type stuckDeployer struct {
	coretest.FakeDeployer
	calls int
}

func (d *stuckDeployer) ServiceEvents(_ context.Context, _ string) ([]core.ServiceEvent, error) {
	d.calls++
	if d.calls == 1 {
		return nil, nil
	}
	evs := make([]core.ServiceEvent, 3)
	for i := range evs {
		evs[i] = core.ServiceEvent{ID: "ev-" + strconv.Itoa(i), At: time.Now(),
			Message: "CannotPullContainerError: pull image manifest has been retried", IsError: true}
	}
	return evs, nil
}

func TestWatchRolloutDetectsStuck(t *testing.T) {
	dep := &stuckDeployer{FakeDeployer: coretest.FakeDeployer{
		DeploymentValue: core.Deployment{Rollout: core.RolloutInProgress, Desired: 1},
	}}
	var buf bytes.Buffer
	err := watchRollout(context.Background(), &buf, dep, "nao-v2-dev-api", "api", 0)
	require.ErrorContains(t, err, "stuck")
	require.Contains(t, buf.String(), "steer service rollback -s api")
}
```

- [ ] **Step 2: Verificar que fallan**

Run: `go test ./internal/tui/ ./internal/cli/ -run 'TestWatch' -v`
Expected: FAIL — `m.deploy.PullErrors` undefined; `watchRollout` no acepta `short` ni
detecta el atasco (loop infinito evitado: el test aún no compila).

- [ ] **Step 3: Implementar — TUI**

En `deployState` (`app.go`):

```go
// deployState agrupa el estado del watch de deploy en vivo; Reset lo limpia entero.
type deployState struct {
	Active     bool
	Done       bool
	Service    string
	LastID     string
	PullErrors int // eventos de fallo de aprovisionamiento en el rollout actual
}
```

En el caso `deployPollMsg`, dentro del loop de eventos existente (que ya hace
`AppendLine`), contar los fallos; y tras `SetStatusLine(...)`, ANTES del check de
`msg.done`, cortar al llegar al umbral:

```go
		for i := len(msg.events) - 1; i >= 0; i-- {
			e := msg.events[i]
			// ... AppendLine existente sin cambios ...
			if core.IsProvisioningFailure(e.Message) {
				m.deploy.PullErrors++
			}
		}
```

```go
		// 3 fallos de aprovisionamiento = rollout atascado (ECS reintenta para
		// siempre sin circuit breaker y nunca reporta FAILED): cortar el poll.
		if m.deploy.PullErrors >= 3 {
			m.events.AppendLine(render.Danger("✗ deployment stuck: image pull failing — roll back with R"))
			m.deploy.Active = false
			m.deploy.Done = true
			return m, m.loadServicesCmd()
		}
```

(`applyActionConfirmed` ya resetea el contador: crea `deployState{...}` fresco.)

- [ ] **Step 4: Implementar — CLI**

`watchRollout` gana el nombre corto para la sugerencia y el contador:

```go
// watchRollout sigue el rollout: streaming de eventos + línea de status, hasta
// COMPLETED/FAILED o hasta detectar un atasco (3 fallos de aprovisionamiento).
func watchRollout(ctx context.Context, out io.Writer, dep core.Deployer, service, short string, interval int) error {
```

- Declarar `pullErrors := 0` junto a `statusShown`.
- En el loop de eventos frescos, tras `printEvent(out, fresh[i])`:

```go
				if core.IsProvisioningFailure(fresh[i].Message) {
					pullErrors++
				}
```

- Tras imprimir los eventos (antes de `DeploymentStatus`):

```go
		if pullErrors >= 3 {
			_, _ = fmt.Fprintln(out)
			_, _ = fmt.Fprintln(out, render.Danger("✗ deployment stuck: image pull failing"))
			_, _ = fmt.Fprintln(out, render.Dim("roll back with: steer service rollback -s "+short))
			return fmt.Errorf("deployment stuck for %q: image pull keeps failing", service)
		}
```

- Actualizar el caller (`newServiceDeployCmd`): `return watchRollout(cmd.Context(), out, dep, realName, service, interval)`.
- `service_cmd.go` gana el import `"github.com/juanMaAV92/steer/internal/core"` si no lo tiene (ya lo tiene).

- [ ] **Step 5: Verificar todo + gates + commit**

Run: `go test ./... -count=1` → PASS completo.

```bash
gofmt -w internal/ && golangci-lint run && go vet ./... && go test ./... -count=1 && go build ./...
git add internal/tui/ internal/cli/
git commit -m "feat(tui,cli): watch detecta rollout atascado por fallos de pull y sugiere rollback"
```
