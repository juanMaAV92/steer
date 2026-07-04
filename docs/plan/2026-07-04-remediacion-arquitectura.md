# Remediación de arquitectura — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ejecutar las olas 1 (higiene) y 2 (costuras) de la revisión de arquitectura del 2026-07-04: eliminar deuda puntual (estado de deploy disperso, strings mágicos de rollout, rama muerta, flags peligrosos, CI sin lint) y generalizar las costuras (interface `Deployer` sin ECS-ismos, `Provider` bundle con sesión cacheada y `ctx` enhebrado, validación global de config) **antes** de construir la capacidad `registry`.

**Architecture:** La ola 1 son cambios locales sin efecto en firmas públicas. La ola 2 cambia dos costuras: (a) `core.Deployer` deja de recibir `cluster` en cada método (se liga al construir el provider) y `Rollout` pasa a un tipo con constantes; (b) `DeployerFactory` se reemplaza por `ProviderFactory(ctx, config.Context) (Provider, error)` — un bundle por contexto que cachea la `aws.Config` y entrega capacidades memoizadas, listo para `Registry()` en el siguiente hito.

**Tech Stack:** Go 1.26, Cobra, Bubble Tea, AWS SDK v2, testify, golangci-lint.

## Global Constraints

- La lógica ECS existente NO cambia de comportamiento (solo firmas/plumbing); el flujo de deploy en vivo (pestaña Events) y el teclado de la TUI no cambian.
- Comentarios en español, UI strings en inglés; cian de marca (`render.BrandColor`), verde solo para estado.
- `render` sigue siendo hoja (no importa paquetes de steer); `core` solo stdlib.
- gofmt-clean, `go vet ./...` limpio, `go test ./... -count=1` y `go build ./...` verdes antes de cada commit.
- NO añadir autoría de Claude a los commits (sin `Co-Authored-By`, sin "Generated with Claude Code").
- Orden: Tareas 1–5 (higiene) son independientes entre sí; Tareas 6→7→8 son secuenciales; 9–10 independientes. Cada tarea deja el build verde.
- **Fuera de alcance** (planes posteriores): config anidada por capacidad (`[contexts.dev.registry]`) → plan de registry; abstracción `overlay`/sidebar por secciones/`detailsButtonRowY` derivada → plan de TUI previo a la paleta ⌘k.

## File Structure

```
internal/core/core.go            ← RolloutState + constantes; Deployer sin cluster; ServiceEvent.IsError
internal/core/coretest/fake.go   ← FakeDeployer sin cluster en llamadas
internal/render/rollout.go       ← (NUEVO) render.Rollout(s string)
internal/providers/factory.go    ← Provider + ProviderFactory (reemplaza DeployerFactory)
internal/providers/aws/provider.go ← (NUEVO) bundle AWS con aws.Config cacheada
internal/providers/aws/ecs.go    ← cluster ligado al constructor; mapa RolloutState; IsError
internal/config/config.go        ← Config.Validate()
internal/cli/root.go             ← --env deprecated; STEER_CONTEXT; Validate global
internal/cli/context.go          ← AppContext con Provider; helper Deployer()
internal/cli/service_cmd.go      ← sin newDeployerFn global; count obligatorio; ctx en watch
internal/tui/app.go              ← deployState struct; sin rama muerta; provider en Model; ctx
internal/tui/commands.go         ← ctx como parámetro; constantes rollout
internal/tui/run.go              ← Run(ctx, factory, contexts, current)
.golangci.yml                    ← (NUEVO) linters mínimos
.github/workflows/ci.yml         ← lint + gofmt gate
Makefile                         ← target lint
```

---

## OLA 1 — Higiene

### Task 1: RolloutState tipado + render.Rollout compartido

**Files:**
- Modify: `internal/core/core.go`
- Create: `internal/render/rollout.go`
- Modify: `internal/tui/commands.go`, `internal/tui/app.go` (borra `rolloutColored`), `internal/cli/service_cmd.go` (borra `rolloutColor`)
- Test: `internal/render/rollout_test.go`, `internal/core/core_test.go`

**Interfaces:**
- Produces:
  - `type RolloutState string` en core, con `const ( RolloutInProgress RolloutState = "IN_PROGRESS"; RolloutCompleted RolloutState = "COMPLETED"; RolloutFailed RolloutState = "FAILED" )`.
  - `Deployment.Rollout` pasa de `string` a `RolloutState`.
  - `func render.Rollout(s string) string` — colorea COMPLETED verde / FAILED rojo / resto cian. Único sitio con el switch de colores.

- [ ] **Step 1: Write the failing tests**

```go
// internal/render/rollout_test.go
package render

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRolloutContainsState(t *testing.T) {
	for _, s := range []string{"COMPLETED", "FAILED", "IN_PROGRESS"} {
		require.Contains(t, Rollout(s), s)
	}
	require.True(t, strings.Contains(Rollout("COMPLETED"), "COMPLETED"))
}
```

```go
// añadir a internal/core/core_test.go
func TestRolloutStateConstants(t *testing.T) {
	require.Equal(t, core.RolloutState("COMPLETED"), core.RolloutCompleted)
	require.Equal(t, core.RolloutState("FAILED"), core.RolloutFailed)
	require.Equal(t, core.RolloutState("IN_PROGRESS"), core.RolloutInProgress)
}
```

(Ajusta imports del test de core a cómo estén los existentes en ese archivo — usa el paquete `core_test` o `core` según el patrón actual.)

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/render/ ./internal/core/ -run 'TestRollout'`
Expected: FAIL (símbolos no definidos).

- [ ] **Step 3: Implement**

En `internal/core/core.go`, añade sobre `Deployment` y cambia el campo:

```go
// RolloutState es el estado del despliegue activo, normalizado entre providers.
type RolloutState string

const (
	RolloutInProgress RolloutState = "IN_PROGRESS"
	RolloutCompleted  RolloutState = "COMPLETED"
	RolloutFailed     RolloutState = "FAILED"
)

// Deployment es el estado del despliegue activo (rollout) de un servicio.
type Deployment struct {
	Rollout RolloutState
	Running int
	Pending int
	Desired int
}
```

```go
// internal/render/rollout.go
package render

// Rollout colorea un estado de rollout: COMPLETED verde, FAILED rojo, resto cian.
// Único punto donde se mapea estado→color (CLI y TUI lo comparten).
func Rollout(s string) string {
	switch s {
	case "COMPLETED":
		return Success(s)
	case "FAILED":
		return Danger(s)
	default:
		return Accent(s)
	}
}
```

Sustituciones (el compilador guía; estas son todas las conocidas):
- `internal/providers/aws/ecs.go` (~línea 97, en `DeploymentStatus`): donde se asigna el rollout crudo, envuélvelo: `Rollout: core.RolloutState(string(...))` (ECS ya devuelve exactamente estos valores).
- `internal/tui/commands.go:50-53`: `done: d.Rollout == core.RolloutCompleted && d.Running >= d.Desired`, `failed: d.Rollout == core.RolloutFailed`. El campo `rollout` de `deployPollMsg` (messages.go) pasa a `core.RolloutState`.
- `internal/tui/app.go`: borra `rolloutColored` (líneas ~573-582) y usa `render.Rollout(string(msg.rollout))` en el `SetStatusLine`.
- `internal/cli/service_cmd.go`: borra `rolloutColor` (~259-268); en `watchRollout` usa `render.Rollout(string(d.Rollout))` y compara `d.Rollout == core.RolloutCompleted` / `== core.RolloutFailed`.
- `internal/core/coretest/fake.go` y tests que construyan `core.Deployment{Rollout: "COMPLETED"}`: pasa a `core.RolloutCompleted`.

- [ ] **Step 4: Run to verify pass**

Run: `gofmt -w internal/ && go vet ./... && go test ./... -count=1 && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add -A internal/
git commit -m "refactor(core): RolloutState tipado y render.Rollout compartido"
```

---

### Task 2: Eliminar la rama muerta de deploy en runActionCmd

**Files:**
- Modify: `internal/tui/app.go` (`runActionCmd`, ~líneas 467-492)
- Test: `internal/tui/app_test.go`

**Interfaces:** ninguna nueva; `runActionCmd` deja de tener caso `actionDeploy`.

- [ ] **Step 1: Write the failing test**

```go
// añadir a internal/tui/app_test.go
// runActionCmd nunca debe ejecutar un deploy: el deploy va por startDeployCmd
// (flujo en vivo). Si llegara un actionDeploy, debe devolver error, no desplegar.
func TestRunActionCmdRejectsDeploy(t *testing.T) {
	fake := &coretest.FakeDeployer{Services: sampleServices()}
	m := newTestModel(sampleServices())
	m.action.open(actionDeploy, "svc")
	m.action.input = "v1"
	cmd := m.runActionCmd()
	msg := cmd().(actionDoneMsg)
	require.Error(t, msg.err)
	require.Empty(t, fake.DeployCalls) // jamás llama a Deploy sin streaming
}
```

Nota: `newTestModel` crea su propio fake interno; para poder afirmar sobre `DeployCalls` construye el modelo a mano como en otros tests (`factory` que devuelve `fake`), siguiendo el patrón de `multiCtxModel`.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/tui/ -run TestRunActionCmdRejectsDeploy`
Expected: FAIL (hoy la rama llama `dep.Deploy` fire-and-forget y devuelve `err: nil`).

- [ ] **Step 3: Implement**

En `runActionCmd`, reemplaza el caso `actionDeploy` completo por:

```go
		case actionDeploy:
			// El deploy SIEMPRE va por startDeployCmd (flujo en vivo con eventos).
			// Esta rama solo es alcanzable si un refactor rompe el guard de Enter.
			return actionDoneMsg{err: fmt.Errorf("internal: deploy must go through startDeployCmd")}
```

Añade `fmt` al import de app.go si no está.

- [ ] **Step 4: Run to verify pass**

Run: `gofmt -w internal/tui/ && go test ./internal/tui/... -count=1 && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/
git commit -m "fix(tui): runActionCmd rechaza deploy (solo startDeployCmd despliega)"
```

---

### Task 3: deployState struct con Reset()

**Files:**
- Modify: `internal/tui/app.go` (campos ~71-73 y todos los usos), `internal/tui/app_test.go` (usos de los campos)

**Interfaces:**
- Produces:
  - `type deployState struct { Active, Done bool; Service, LastID string }` con `func (d *deployState) Reset() { *d = deployState{} }`.
  - En `Model`: los 4 campos (`deployActive`, `deployDone`, `deployService`, `deployLastID`) se reemplazan por `deploy deployState`.

- [ ] **Step 1: Write the failing test**

```go
// añadir a internal/tui/app_test.go
func TestDeployStateReset(t *testing.T) {
	d := deployState{Active: true, Done: true, Service: "svc", LastID: "id"}
	d.Reset()
	require.Equal(t, deployState{}, d)
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/tui/ -run TestDeployStateReset`
Expected: FAIL (tipo no definido).

- [ ] **Step 3: Implement**

En `app.go`, define el tipo (junto al Model) y sustituye:

```go
// deployState agrupa el estado del watch de deploy en vivo; Reset lo limpia entero.
type deployState struct {
	Active  bool
	Done    bool
	Service string
	LastID  string
}

func (d *deployState) Reset() { *d = deployState{} }
```

Sitios conocidos (el compilador guía):
- Model: `deploy deployState` (reemplaza los 4 campos).
- Enter-deploy (~340): `m.deploy = deployState{Active: true, Service: svc}`.
- `deployStartedMsg` (~170-174): `m.deploy.Done = true` / `m.deploy.LastID = msg.lastID` / poll usa `m.deploy.Service`, `m.deploy.LastID`.
- `deployPollMsg` done/failed (~193-200): reemplaza los pares `deployActive=false; deployDone=true` por `m.deploy.Active = false; m.deploy.Done = true` (mantén la semántica exacta).
- `deployPollTickMsg` (~206): `if m.deploy.Active && !m.deploy.Done`.
- `applyContextSwitch` (~436-439): las 4 asignaciones se vuelven `m.deploy.Reset()`.
- Tests que toquen los campos (`TestSwitchDuringDeployStopsPollLoop`, deploy-flow): actualiza a `m.deploy.X`.

- [ ] **Step 4: Run to verify pass**

Run: `gofmt -w internal/tui/ && go vet ./internal/tui/... && go test ./internal/tui/... -count=1 && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/
git commit -m "refactor(tui): deployState con Reset único (antes 4 flags dispersos)"
```

---

### Task 4: CLI seguro — --env deprecated, count obligatorio, STEER_CONTEXT, guard de deploy no-interactivo

**Files:**
- Modify: `internal/cli/root.go`, `internal/cli/service_cmd.go`
- Test: `internal/cli/service_cmd_test.go`, `internal/cli/context_test.go` (o el archivo de tests raíz que corresponda)

**Interfaces:**
- Produces:
  - `--env` marcado deprecated (visible con warning, sigue funcionando).
  - Resolución de contexto: flag `--context` > env var `STEER_CONTEXT` > `default_context`.
  - `scale`: `--count` sin default implícito — error si no se pasa.
  - `deploy` no-interactivo (`-y`): error explícito si falta `-s` o `-t`.

- [ ] **Step 1: Write the failing tests**

```go
// añadir a internal/cli/service_cmd_test.go (usa los helpers runRoot/withFakeDeployer existentes)
func TestScaleRequiresCount(t *testing.T) {
	// sin --count debe fallar, aunque haya -y
	_, err := runRootWithFake(t, "service", "scale", "-s", "web", "-y")
	require.ErrorContains(t, err, "--count")
}

func TestDeployNonInteractiveRequiresServiceAndTag(t *testing.T) {
	_, err := runRootWithFake(t, "service", "deploy", "-y")
	require.ErrorContains(t, err, "--service")
}

func TestSteerContextEnvVar(t *testing.T) {
	t.Setenv("STEER_CONTEXT", "dev")
	// resolver sin --context debe usar STEER_CONTEXT (config de test con contexto "dev")
	out, err := runRootWithFake(t, "service", "status")
	require.NoError(t, err)
	_ = out
}
```

Adapta los nombres de helper a los reales del paquete (`runRoot` + `withFakeDeployer` según `config_cmd_test.go`/`service_cmd_test.go`); si no existe un combinado, crea `runRootWithFake` local en el test.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/cli/ -run 'TestScaleRequires|TestDeployNonInteractive|TestSteerContextEnv'`
Expected: FAIL (count tiene default 1; deploy -y cae al form; env var no se lee).

- [ ] **Step 3: Implement**

`root.go`:

```go
	root.PersistentFlags().StringVar(&contextName, "context", "", "target context (default: STEER_CONTEXT or default_context)")
	root.PersistentFlags().StringVar(&contextName, "env", "", "alias of --context")
	_ = root.PersistentFlags().MarkDeprecated("env", "use --context instead")
```

Y en `PersistentPreRunE`, antes de `ResolveContext`:

```go
		if contextName == "" {
			contextName = os.Getenv("STEER_CONTEXT")
		}
```

(añade `"os"` al import).

`service_cmd.go` — scale: cambia el flag y valida presencia explícita:

```go
	cmd.Flags().IntVarP(&count, "count", "c", 0, "desired task count (required)")
```

```go
		RunE: func(cmd *cobra.Command, _ []string) error {
			if service == "" {
				return fmt.Errorf("--service is required")
			}
			if !cmd.Flags().Changed("count") {
				return fmt.Errorf("--count is required (refusing to default to 1)")
			}
			// ... resto igual
```

`service_cmd.go` — deploy: al inicio del RunE, antes del picker:

```go
			if yes && (service == "" || tag == "") {
				return fmt.Errorf("non-interactive deploy (-y) requires --service and --tag")
			}
```

Si el test viejo `TestDeployRequiresServiceAndTag` dependía del fallo del form de huh en no-TTY, actualízalo para esperar este error explícito.

- [ ] **Step 4: Run to verify pass**

Run: `gofmt -w internal/cli/ && go vet ./internal/cli/... && go test ./internal/cli/... -count=1 && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/
git commit -m "feat(cli): --env deprecated, STEER_CONTEXT, count obligatorio y guard de deploy -y"
```

---

### Task 5: Lint en CI (golangci-lint + gofmt gate)

**Files:**
- Create: `.golangci.yml`
- Modify: `.github/workflows/ci.yml`, `Makefile`
- (y los fixes que el lint destape)

**Interfaces:** ninguna; gate de calidad.

- [ ] **Step 1: Config del linter**

```yaml
# .golangci.yml
version: "2"
linters:
  default: none
  enable:
    - errcheck
    - govet
    - staticcheck
    - unused
    - ineffassign
formatters:
  enable:
    - gofmt
```

(Si la versión instalada de golangci-lint usa el esquema v1, adapta: `linters: disable-all: true / enable: [...]` — verifica con `golangci-lint --version`.)

- [ ] **Step 2: CI y Makefile**

`.github/workflows/ci.yml` — añade tras el setup-go:

```yaml
      - name: gofmt
        run: test -z "$(gofmt -l .)" || (gofmt -l . && exit 1)
      - name: golangci-lint
        uses: golangci/golangci-lint-action@v8
        with: { version: latest }
```

`Makefile`:

```makefile
.PHONY: build test tidy lint

lint:
	gofmt -l . && test -z "$$(gofmt -l .)"
	go vet ./...
	golangci-lint run
```

- [ ] **Step 3: Correr el lint localmente y arreglar hallazgos**

Run: `golangci-lint run ./... 2>&1 | head -40` (instálalo si falta: `brew install golangci-lint`).
Arregla cada hallazgo real (sobre todo `errcheck`: errores ignorados). Si un hallazgo es intencional (p.ej. `_ = f.Close()` en defer), usa el patrón explícito `_ =`, no `//nolint` salvo justificación en comentario.

- [ ] **Step 4: Run to verify pass**

Run: `golangci-lint run && go test ./... -count=1 && go build ./...`
Expected: limpio y verde.

- [ ] **Step 5: Commit**

```bash
git add .golangci.yml .github/workflows/ci.yml Makefile internal/ cmd/
git commit -m "ci: golangci-lint y gate de gofmt; fixes de errcheck"
```

---

## OLA 2 — Costuras

### Task 6: Des-fugar Deployer — cluster ligado al constructor

**Files:**
- Modify: `internal/core/core.go`, `internal/core/coretest/fake.go`, `internal/providers/aws/ecs.go`, `internal/providers/factory.go`, `internal/cli/service_cmd.go`, `internal/cli/tui_cmd.go`, `internal/tui/{app.go,commands.go}` + tests afectados en cli/tui/aws

**Interfaces:**
- Produces (firma nueva de `core.Deployer` — SIN `cluster`):

```go
type Deployer interface {
	ListServices(ctx context.Context) ([]ServiceStatus, error)
	CurrentTag(ctx context.Context, service string) (string, error)
	Deploy(ctx context.Context, service, tag string, log StepLogger) error
	Scale(ctx context.Context, service string, count int) error
	Rollback(ctx context.Context, service string) error
	DeploymentStatus(ctx context.Context, service string) (Deployment, error)
	ServiceEvents(ctx context.Context, service string) ([]ServiceEvent, error)
}
```

- `aws.NewDeployer(cfg awssdk.Config, cluster string) *ECSDeployer` (y `newDeployer(api ecsAPI, cluster string)`) — el `ECSDeployer` guarda `cluster` y lo usa internamente.
- `coretest.FakeDeployer` registra llamadas SOLO con `service` (p.ej. `DeployCalls: ["svc/v2"]`); si algún test necesita el cluster, se afirma vía el contexto usado para construir, no vía la llamada.
- `providers.DeployerFactory` pasa el `c.Cluster` al constructor. `newDeployerFn` (CLI) pasa a devolver `(core.Deployer, error)` — sin `cluster`.

- [ ] **Step 1: Actualizar la interface y el fake (tests primero)**

Cambia `core.Deployer` a la firma de arriba. Actualiza `coretest/fake.go` quitando el parámetro `cluster` de cada método y ajustando los registros de llamadas (`cluster+"/"+service+...` → `service+...`). Actualiza el test de conformance si existe.

- [ ] **Step 2: Run — el build enumera cada sitio**

Run: `go build ./... 2>&1 | head -30`
Expected: FAIL en ecs.go, factory.go, cli, tui — esa lista es tu checklist.

- [ ] **Step 3: Migrar provider, factory, CLI y TUI**

`ecs.go`:

```go
type ECSDeployer struct {
	api     ecsAPI
	cluster string
}

func NewDeployer(cfg awssdk.Config, cluster string) *ECSDeployer {
	return newDeployer(ecs.NewFromConfig(cfg), cluster)
}

func newDeployer(api ecsAPI, cluster string) *ECSDeployer {
	return &ECSDeployer{api: api, cluster: cluster}
}
```

En cada método, elimina el parámetro y usa `d.cluster` en las llamadas al SDK. Los tests de `ecs` que pasaban cluster por llamada pasan a construir `newDeployer(fakeAPI, "stg-cluster")`.

`factory.go`: `return aws.NewDeployer(cfg, c.Cluster), nil`.

`internal/cli/service_cmd.go`: `newDeployerFn` devuelve `(core.Deployer, error)`; borra la variable `cluster` en cada RunE (las llamadas ya no la pasan). `internal/cli/tui_cmd.go` no cambia (pasa la factory).

`internal/tui`: `startDeployCmd`/`deployPollCmd` pierden el parámetro `cluster`; `loadServicesCmd` llama `m.dep.ListServices(ctx)`. Los call sites en `app.go` dejan de pasar `m.current.Cluster` (el cluster sigue mostrándose en el top bar desde `m.current.Cluster` — solo se elimina del plumbing de llamadas).

- [ ] **Step 4: Run to verify pass**

Run: `gofmt -w internal/ && go vet ./... && go test ./... -count=1 && go build ./...`
Expected: PASS — misma semántica, sin `cluster` en las firmas.

- [ ] **Step 5: Commit**

```bash
git add -A internal/
git commit -m "refactor(core): ligar cluster al constructor del provider (interface agnóstica)"
```

---

### Task 7: Provider bundle con sesión cacheada + ctx en la fábrica

**Files:**
- Modify: `internal/providers/factory.go`
- Create: `internal/providers/aws/provider.go`
- Modify: `internal/cli/context.go`, `internal/cli/root.go`, `internal/cli/service_cmd.go`, `internal/cli/tui_cmd.go`, `internal/tui/{run.go,app.go}`, `cmd/steerdemo/main.go` + tests
- Test: `internal/providers/factory_test.go`

**Interfaces:**
- Produces:

```go
// internal/providers/factory.go
// Provider agrupa las capacidades de un contexto; cachea la sesión del cloud.
type Provider interface {
	Deployer() (core.Deployer, error)
	// Registry() (core.Registry, error)  ← se añade en el hito registry
}

// ProviderFactory construye el bundle de un contexto. ctx permite cancelar la
// carga de sesión (SSO, red).
type ProviderFactory func(ctx context.Context, c config.Context) (Provider, error)

func NewProviderFactory() ProviderFactory
```

- AWS impl (`internal/providers/aws/provider.go`): carga `aws.Config` UNA vez (con el `ctx` recibido) y memoiza el `Deployer`. Cloud no-aws → `ErrProviderNotImplemented` (se conserva el sentinel y su semántica en la TUI).
- `DeployerFactory` y `NewDeployerFactory` se ELIMINAN.
- `cli.AppContext`: campo `Factory providers.ProviderFactory`; nuevo helper que reemplaza al global `newDeployerFn`:

```go
// Deployer construye (una vez) el provider del contexto activo y devuelve su Deployer.
func (a *AppContext) Deployer(ctx context.Context) (core.Deployer, error)
```

  Los tests del CLI inyectan un fake asignando `app.Factory` (se elimina el var global `newDeployerFn` y su `withFakeDeployer` pasa a inyectar factory).
- `tui.Run(ctx context.Context, factory providers.ProviderFactory, contexts []config.Context, current config.Context) error`; el `Model` guarda `ctx`, `factory` y `provider` — `New` construye el provider inicial con la factory y obtiene `dep` de él; `applyContextSwitch` construye el provider del contexto elegido.

- [ ] **Step 1: Write the failing tests**

```go
// internal/providers/factory_test.go — reemplaza los tests de DeployerFactory
func TestProviderFactoryUnknownCloud(t *testing.T) {
	f := NewProviderFactory()
	_, err := f(context.Background(), config.Context{Name: "x", Cloud: "gcp", Cluster: "c"})
	require.ErrorIs(t, err, ErrProviderNotImplemented)
	require.ErrorContains(t, err, "gcp")
}

func TestProviderFactoryRespectsContextCancel(t *testing.T) {
	f := NewProviderFactory()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := f(ctx, config.Context{Name: "x", Cloud: "aws", Profile: "p", Cluster: "c"})
	require.Error(t, err) // la carga de sesión debe respetar el ctx cancelado
}
```

(El segundo test depende de que `LoadDefaultConfig` respete el ctx; si en la práctica no falla con ctx cancelado antes de tocar red, valida al inicio de la factory: `if err := ctx.Err(); err != nil { return nil, err }` — y el test queda determinista.)

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/providers/ -run TestProviderFactory`
Expected: FAIL (símbolos no definidos).

- [ ] **Step 3: Implement**

`internal/providers/aws/provider.go`:

```go
// Package aws: bundle de capacidades AWS por contexto, con sesión cacheada.
package aws

import (
	"context"
	"sync"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/juanMaAV92/steer/internal/config"
	"github.com/juanMaAV92/steer/internal/core"
)

// Provider agrupa las capacidades AWS de un contexto. La aws.Config se carga
// una sola vez; las capacidades se memoizan.
type Provider struct {
	cfg awssdk.Config
	ctx config.Context

	once     sync.Once
	deployer core.Deployer
}

// NewProvider carga la sesión AWS del contexto (cancelable vía ctx).
func NewProvider(ctx context.Context, c config.Context) (*Provider, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cfg, err := LoadConfigForContext(ctx, c)
	if err != nil {
		return nil, err
	}
	return &Provider{cfg: cfg, ctx: c}, nil
}

// Deployer devuelve el Deployer ECS del contexto (memoizado).
func (p *Provider) Deployer() (core.Deployer, error) {
	p.once.Do(func() { p.deployer = NewDeployer(p.cfg, p.ctx.Cluster) })
	return p.deployer, nil
}
```

`internal/providers/factory.go` (reemplaza el contenido de DeployerFactory):

```go
// Provider agrupa las capacidades de un contexto; cachea la sesión del cloud.
type Provider interface {
	Deployer() (core.Deployer, error)
}

// ProviderFactory construye el bundle de un contexto (ctx cancela la carga de sesión).
type ProviderFactory func(ctx context.Context, c config.Context) (Provider, error)

// NewProviderFactory devuelve la fábrica por defecto (AWS real; otros → error).
func NewProviderFactory() ProviderFactory {
	return func(ctx context.Context, c config.Context) (Provider, error) {
		switch c.Cloud {
		case "aws":
			return aws.NewProvider(ctx, c)
		default:
			return nil, fmt.Errorf("%w: %q", ErrProviderNotImplemented, c.Cloud)
		}
	}
}
```

`cli/context.go`:

```go
type AppContext struct {
	Ctx     config.Context
	Config  *config.Config
	Factory providers.ProviderFactory

	provider providers.Provider // memoizado por comando
}

// Deployer construye (una vez) el provider del contexto activo y devuelve su Deployer.
func (a *AppContext) Deployer(ctx context.Context) (core.Deployer, error) {
	if a.provider == nil {
		p, err := a.Factory(ctx, a.Ctx)
		if err != nil {
			return nil, err
		}
		a.provider = p
	}
	return a.provider.Deployer()
}
```

`cli/service_cmd.go`: elimina `var newDeployerFn`; cada RunE usa `dep, err := app.Deployer(cmd.Context())`. Los tests reemplazan `withFakeDeployer` por inyección de factory:

```go
func fakeFactory(fake core.Deployer) providers.ProviderFactory {
	return func(context.Context, config.Context) (providers.Provider, error) {
		return fakeProvider{dep: fake}, nil
	}
}

type fakeProvider struct{ dep core.Deployer }

func (p fakeProvider) Deployer() (core.Deployer, error) { return p.dep, nil }
```

(el hook de test asigna `app.Factory = fakeFactory(fake)`; si los tests montan el root con `PersistentPreRunE` real, expón un seam mínimo: variable de paquete `newProviderFactoryFn = providers.NewProviderFactory` usada en root.go y reasignada en tests — mismo patrón que el viejo `newDeployerFn` pero ÚNICO para todas las capacidades).

`tui/run.go` y `app.go`: `Run(ctx, factory, contexts, current)`; `Model` guarda `runCtx context.Context`, `factory providers.ProviderFactory`, `provider providers.Provider`; `New(ctx, factory, ...)` construye provider+dep (con `depErr` como hoy); `applyContextSwitch` usa `m.factory(m.runCtx, sel)` y `provider.Deployer()`. `cli/tui_cmd.go` pasa `cmd.Context()` y `app.Factory`. `cmd/steerdemo/main.go` se adapta con un fake provider.

- [ ] **Step 4: Run to verify pass**

Run: `gofmt -w internal/ cmd/ && go vet ./... && go test ./... -count=1 && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add -A internal/ cmd/
git commit -m "refactor(providers): Provider bundle con sesión cacheada y ctx (reemplaza DeployerFactory)"
```

---

### Task 8: Cancelación — ctx en comandos de la TUI y watch del CLI

**Files:**
- Modify: `internal/tui/commands.go`, `internal/tui/app.go`, `internal/cli/service_cmd.go`
- Test: `internal/tui/app_test.go` (compila con firmas nuevas), `internal/cli/service_cmd_test.go`

**Interfaces:**
- Produces: `startDeployCmd(ctx, dep, service, tag)`, `deployPollCmd(ctx, dep, service, lastID)`, `loadServicesCmd` usa `m.runCtx`; `watchRollout` y el loop de `status -w` seleccionan sobre `ctx.Done()`.

- [ ] **Step 1: TUI — reemplazar context.Background()**

En `commands.go`, añade `ctx context.Context` como primer parámetro de `startDeployCmd` y `deployPollCmd` y elimina los `context.Background()` internos. En `app.go`, `loadServicesCmd` usa `m.runCtx` y los call sites pasan `m.runCtx`.

- [ ] **Step 2: CLI — watch cancelable**

En `watchRollout` (y el loop de `status --watch`), reemplaza `time.Sleep(...)` por:

```go
		select {
		case <-ctx.Done():
			fmt.Fprintln(out)
			return ctx.Err()
		case <-time.After(time.Duration(interval) * time.Second):
		}
```

(`ctx` es el `cmd.Context()` que ya llega a esas funciones; si no llega, pásalo).

- [ ] **Step 3: Test de cancelación del watch**

```go
// internal/cli/service_cmd_test.go
func TestWatchRolloutStopsOnCancel(t *testing.T) {
	fake := &coretest.FakeDeployer{DeploymentValue: core.Deployment{Rollout: core.RolloutInProgress}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var buf bytes.Buffer
	err := watchRollout(ctx, &buf, fake, "svc", 1)
	require.ErrorIs(t, err, context.Canceled)
}
```

(ajusta la firma real de `watchRollout` — el orden/número de parámetros según quede tras la Task 6).

- [ ] **Step 4: Run to verify pass**

Run: `gofmt -w internal/ && go vet ./... && go test ./... -count=1 && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/
git commit -m "fix: cancelación real — ctx en comandos TUI y watch del CLI"
```

---

### Task 9: Config.Validate() global (default_context, esquema legacy, contextos)

**Files:**
- Modify: `internal/config/config.go`, `internal/cli/root.go`, `internal/cli/config_cmd.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces: `func (c *Config) Validate() error` — falla si: (a) `default_context` apunta a un contexto inexistente; (b) no hay contextos Y el TOML crudo contiene el esquema legacy `[providers...]` (error con guía de migración); (c) algún contexto individual no pasa su `Validate()`. `Load` NO valida (comportamiento igual); `root.go` y `config validate` llaman `cfg.Validate()`.

- [ ] **Step 1: Write the failing tests**

```go
// internal/config/config_test.go
func TestValidateDefaultContextMustExist(t *testing.T) {
	cfg, err := Load(writeTOML(t, `
default_context = "ghost"
[contexts.dev]
cloud = "aws"
profile = "p"
cluster = "c"
`))
	require.NoError(t, err)
	require.ErrorContains(t, cfg.Validate(), "ghost")
}

func TestValidateDetectsLegacySchema(t *testing.T) {
	cfg, err := Load(writeTOML(t, `
[providers.aws.environments.dev]
profile = "dev"
`))
	require.NoError(t, err)
	err = cfg.Validate()
	require.ErrorContains(t, err, "contexts")
}

func TestValidateOK(t *testing.T) {
	cfg, err := Load(writeTOML(t, sampleContexts))
	require.NoError(t, err)
	require.NoError(t, cfg.Validate())
}
```

Nota de implementación para (b): el struct `Config` ya no decodifica `providers`, así que para detectar el esquema legacy `Load` debe conservar una señal. Opción simple: en `Load`, tras decodificar, usa `toml.MetaData.Undecoded()` (la API de BurntSushi devuelve las claves no decodificadas) y guarda en un campo no exportado `hasLegacyProviders bool` si alguna clave empieza por `providers.`. `Validate` lo consulta.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/config/ -run TestValidate`
Expected: FAIL.

- [ ] **Step 3: Implement**

```go
// en config.go
// Load conserva metadatos para detectar el esquema legacy.
func Load(path string) (*Config, error) {
	var cfg Config
	md, err := toml.DecodeFile(path, &cfg)
	if err != nil {
		return nil, err
	}
	for _, k := range md.Undecoded() {
		if strings.HasPrefix(k.String(), "providers") {
			cfg.hasLegacyProviders = true
			break
		}
	}
	return &cfg, nil
}

// Validate comprueba invariantes globales del steer.toml.
func (c *Config) Validate() error {
	if len(c.Contexts) == 0 {
		if c.hasLegacyProviders {
			return fmt.Errorf("steer.toml uses the legacy [providers.*] schema; migrate to [contexts.*] (see 'steer config init' for the new format)")
		}
		return fmt.Errorf("steer.toml has no contexts; run 'steer config init'")
	}
	if c.DefaultContext != "" {
		if _, err := c.Context(c.DefaultContext); err != nil {
			return fmt.Errorf("default_context %q not found in contexts", c.DefaultContext)
		}
	}
	for _, ctx := range c.AllContexts() {
		if err := ctx.Validate(); err != nil {
			return err
		}
	}
	return nil
}
```

(añade el campo `hasLegacyProviders bool` sin tag toml al struct `Config`, y los imports `strings`/`fmt`).

`root.go` (`PersistentPreRunE`): tras `config.Load`, llama `if err := cfg.Validate(); err != nil { return err }` (y el `cur.Validate()` individual posterior se vuelve redundante — elimínalo). `config_cmd.go` `validate`: usa `cfg.Validate()` en lugar del loop manual, conservando el output `ok: <path> (<n> contexts)`.

- [ ] **Step 4: Run to verify pass**

Run: `gofmt -w internal/ && go vet ./... && go test ./... -count=1 && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config/ internal/cli/
git commit -m "feat(config): Validate global (default_context, esquema legacy, contextos)"
```

---

### Task 10: ServiceEvent.IsError — sacar el conocimiento ECS de la capa de render

**Files:**
- Modify: `internal/core/core.go`, `internal/providers/aws/ecs.go`, `internal/cli/service_cmd.go` (`printEvent`), `internal/tui/app.go` (coloreo de eventos en Events)
- Test: `internal/providers/aws/ecs_test.go` (o donde estén los tests del provider), `internal/cli/service_cmd_test.go`

**Interfaces:**
- Produces: `core.ServiceEvent` gana `IsError bool`; el provider ECS lo marca (`"unable to place"`, `"ResourceInitializationError"`); CLI y TUI colorean por `e.IsError`, sin strings de ECS.

- [ ] **Step 1: Write the failing test**

```go
// internal/providers/aws/ecs_test.go — mismo patrón que TestServiceEventsNewestFirst
func TestServiceEventsMarksErrors(t *testing.T) {
	f := &fakeECS{
		describeOut: &ecs.DescribeServicesOutput{Services: []ecstypes.Service{{
			Events: []ecstypes.ServiceEvent{
				{Id: awssdk.String("e2"), Message: awssdk.String("service was unable to place a task")},
				{Id: awssdk.String("e1"), Message: awssdk.String("reached a steady state")},
			},
		}}},
	}
	d := newDeployer(f, "stg-cluster") // firma post-Task 6

	evs, err := d.ServiceEvents(context.Background(), "catalog")
	require.NoError(t, err)
	require.Len(t, evs, 2)
	require.True(t, evs[0].IsError)
	require.False(t, evs[1].IsError)
}
```

(Si esta tarea se ejecutara antes de la Task 6, ajusta las firmas — pero el plan las ordena 6→10, así que usa las firmas nuevas.)

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/providers/aws/ -run TestServiceEventsMarksErrors`
Expected: FAIL (campo no existe).

- [ ] **Step 3: Implement**

`core.go`: añade `IsError bool` a `ServiceEvent`. `ecs.go` (`ServiceEvents`): al construir cada evento:

```go
		msg := awssdk.ToString(e.Message)
		ev := core.ServiceEvent{
			ID: awssdk.ToString(e.Id), At: awssdk.ToTime(e.CreatedAt), Message: msg,
			IsError: strings.Contains(msg, "unable to place") || strings.Contains(msg, "ResourceInitializationError"),
		}
```

(adapta a los nombres reales del código actual). `printEvent` (CLI) pasa a:

```go
func printEvent(out io.Writer, e core.ServiceEvent) {
	line := fmt.Sprintf("[%s] %s", e.At.Format("15:04:05"), e.Message)
	if e.IsError {
		fmt.Fprintln(out, render.Danger(line))
		return
	}
	fmt.Fprintln(out, render.Dim(line))
}
```

TUI (`deployPollMsg` en app.go): al hacer `AppendLine` de eventos, usa `render.Danger` si `e.IsError`, `render.Dim` si no (hoy siempre Dim — esto además unifica el criterio con el CLI).

- [ ] **Step 4: Run to verify pass**

Run: `gofmt -w internal/ && go vet ./... && go test ./... -count=1 && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/
git commit -m "feat(core): ServiceEvent.IsError — el provider clasifica, CLI/TUI solo colorean"
```

---

## Verificación final (no es una tarea)

- `go test ./... -count=1 -race && go build ./... && golangci-lint run` — todo verde.
- Smoke manual: `go run ./cmd/steerdemo` (nav + deploy fake) y `go run ./cmd/steer tui` contra AWS real: cambiar contexto (`c`), deploy en vivo, `q` durante un poll (debe salir limpio — cancelación).
- Actualizar `docs/superpowers/specs/2026-06-30-registry-ecr-design.md`: reemplazar `RegistryFactory` por `Provider.Registry()` sobre el bundle nuevo (el spec de registry se revisa en su propio hito).
