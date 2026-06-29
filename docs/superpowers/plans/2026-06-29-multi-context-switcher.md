# Contextos multi-cuenta + selector en TUI — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reemplazar el modelo `environments`+`naming` por **contextos** auto-descriptivos (cloud/credencial/cluster/writable) y añadir un **selector de contexto dentro de la TUI** que reconstruye la sesión y recarga sin relanzar, implementado sobre AWS (GCP/Azure representables con error amable).

**Architecture:** El modelo de config pasa a `[contexts.*]` + `default_context`. Una `providers.DeployerFactory` construye el `Deployer` de un contexto (AWS real; otros → `ErrProviderNotImplemented`). La TUI guarda la fábrica + la lista de contextos + el contexto actual y, vía un componente `contextPicker`, conmuta reconstruyendo el Deployer. El CLI resuelve un contexto por `--context/-c` (alias `-e`).

**Tech Stack:** Go, Bubble Tea v1.3.6, Lipgloss, BurntSushi/toml, AWS SDK v2, testify.

## Global Constraints

- `internal/core` NO cambia. La lógica ECS de `internal/providers/aws` no cambia (solo se añade construcción de sesión por contexto).
- Reusar `internal/render`; UI strings en inglés, comentarios en español.
- gofmt-clean y `go vet ./...` limpio antes de cada commit.
- `go test ./...` y `go build ./...` deben pasar antes de cada commit.
- NO añadir autoría de Claude a los commits (sin `Co-Authored-By`, sin "Generated with Claude Code").
- Orden aditivo: las Tareas 1-6 añaden el modelo nuevo y migran consumidores manteniendo el build verde; la Tarea 7 elimina el modelo viejo (`Environment`/`Naming`/`LoadConfig` antiguo) una vez nadie lo usa.
- Acento de marca cian (`render.BrandColor`), verde solo para estado (ya vigente en la TUI).

## File Structure

```
internal/config/
  config.go        ← Config gana DefaultContext + Contexts (Environment/Naming se quitan en Tarea 7)
  context.go       ← (NUEVO) tipo Context + métodos ServiceName/Prefix/Validate
  resolve.go       ← Config.Context/DefaultCtx/AllContexts/ResolveContext (Env/Naming se quitan en Tarea 7)
internal/providers/
  factory.go       ← (NUEVO paquete) DeployerFactory, ErrProviderNotImplemented, NewDeployerFactory
internal/providers/aws/
  session.go       ← +LoadConfigForContext (LoadConfig viejo se quita en Tarea 7)
internal/cli/
  context.go       ← AppContext { Ctx, Config, Factory }
  root.go          ← flags --context/-c (+ alias -e), resolución de contexto
  service_cmd.go   ← newDeployerFn(Context) vía Factory; nombres por Context.ServiceName
  tui_cmd.go       ← arma Factory + lista + current, llama tui.Run nueva firma
  config_cmd.go    ← exampleConfig nuevo formato; validate nuevo modelo
internal/tui/
  run.go           ← Run(factory, contexts, current)
  app.go           ← Model con factory/contexts/current; deriva cluster/writable/prefix; switch
  contextpicker.go ← (NUEVO) componente selector
```

---

### Task 1: Modelo de contextos en config (aditivo)

**Files:**
- Create: `internal/config/context.go`
- Modify: `internal/config/config.go`
- Modify: `internal/config/resolve.go`
- Test: `internal/config/context_test.go`

**Interfaces:**
- Consumes: nada nuevo.
- Produces:
  - `type Context struct { Name, Cloud, Profile, AccountID, RoleARN, Region, Project, Subscription, Cluster, ServiceTemplate string; Writable bool }`
  - `func (c Context) ServiceName(short string) string`
  - `func (c Context) Prefix() string`
  - `func (c Context) Validate() error`
  - En `Config`: campos `DefaultContext string` (toml `default_context`) y `Contexts map[string]Context` (toml `contexts`).
  - `func (c *Config) Context(name string) (Context, error)` — con `Name` poblado.
  - `func (c *Config) DefaultCtx() (Context, error)` — `default_context`, o único, o error si ambiguo.
  - `func (c *Config) AllContexts() []Context` — orden alfabético por `Name`.
  - `func (c *Config) ResolveContext(flag string) (Context, error)` — si `flag != ""` usa `Context(flag)`, si no `DefaultCtx()`.

- [ ] **Step 1: Write the failing tests**

```go
// internal/config/context_test.go
package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeTOML(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "steer.toml")
	require.NoError(t, os.WriteFile(p, []byte(body), 0o600))
	return p
}

const sampleContexts = `
default_context = "nao-dev"

[contexts.nao-dev]
cloud            = "aws"
profile          = "dev"
cluster          = "nao-v2-dev-cluster"
service_template = "nao-v2-dev-{name}"
writable         = true

[contexts.nao-prod]
cloud            = "aws"
profile          = "prod"
cluster          = "nao-v2-production-cluster"
service_template = "nao-v2-production-{name}"
writable         = false
`

func TestLoadParsesContexts(t *testing.T) {
	cfg, err := Load(writeTOML(t, sampleContexts))
	require.NoError(t, err)

	dev, err := cfg.Context("nao-dev")
	require.NoError(t, err)
	require.Equal(t, "nao-dev", dev.Name)
	require.Equal(t, "aws", dev.Cloud)
	require.Equal(t, "dev", dev.Profile)
	require.Equal(t, "nao-v2-dev-cluster", dev.Cluster)
	require.True(t, dev.Writable)

	prod, err := cfg.Context("nao-prod")
	require.NoError(t, err)
	require.False(t, prod.Writable)
}

func TestContextUnknown(t *testing.T) {
	cfg, err := Load(writeTOML(t, sampleContexts))
	require.NoError(t, err)
	_, err = cfg.Context("ghost")
	require.ErrorContains(t, err, "ghost")
}

func TestDefaultCtxUsesDefaultContext(t *testing.T) {
	cfg, err := Load(writeTOML(t, sampleContexts))
	require.NoError(t, err)
	d, err := cfg.DefaultCtx()
	require.NoError(t, err)
	require.Equal(t, "nao-dev", d.Name)
}

func TestDefaultCtxSingleContextNoDefault(t *testing.T) {
	cfg, err := Load(writeTOML(t, `
[contexts.only]
cloud   = "aws"
profile = "dev"
cluster = "c"
`))
	require.NoError(t, err)
	d, err := cfg.DefaultCtx()
	require.NoError(t, err)
	require.Equal(t, "only", d.Name)
}

func TestDefaultCtxAmbiguous(t *testing.T) {
	cfg, err := Load(writeTOML(t, `
[contexts.a]
cloud = "aws"
profile = "x"
cluster = "ca"
[contexts.b]
cloud = "aws"
profile = "y"
cluster = "cb"
`))
	require.NoError(t, err)
	_, err = cfg.DefaultCtx()
	require.ErrorContains(t, err, "default_context")
}

func TestAllContextsSorted(t *testing.T) {
	cfg, err := Load(writeTOML(t, sampleContexts))
	require.NoError(t, err)
	all := cfg.AllContexts()
	require.Len(t, all, 2)
	require.Equal(t, "nao-dev", all[0].Name)
	require.Equal(t, "nao-prod", all[1].Name)
}

func TestResolveContextFlagOrDefault(t *testing.T) {
	cfg, err := Load(writeTOML(t, sampleContexts))
	require.NoError(t, err)
	byFlag, err := cfg.ResolveContext("nao-prod")
	require.NoError(t, err)
	require.Equal(t, "nao-prod", byFlag.Name)
	byDefault, err := cfg.ResolveContext("")
	require.NoError(t, err)
	require.Equal(t, "nao-dev", byDefault.Name)
}

func TestContextServiceNameAndPrefix(t *testing.T) {
	c := Context{ServiceTemplate: "nao-v2-dev-{name}"}
	require.Equal(t, "nao-v2-dev-audit-ms", c.ServiceName("audit-ms"))
	require.Equal(t, "nao-v2-dev-", c.Prefix())

	bare := Context{}
	require.Equal(t, "audit-ms", bare.ServiceName("audit-ms")) // sin template → sin cambio
	require.Equal(t, "", bare.Prefix())
}

func TestContextValidate(t *testing.T) {
	require.NoError(t, Context{Cloud: "aws", Profile: "dev", Cluster: "c"}.Validate())
	require.Error(t, Context{Cloud: "aws", Cluster: "c"}.Validate())  // aws sin profile
	require.Error(t, Context{Profile: "dev", Cluster: "c"}.Validate()) // sin cloud
	require.Error(t, Context{Cloud: "aws", Profile: "d"}.Validate())   // sin cluster
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/config/ -run 'Context|DefaultCtx|AllContexts|ResolveContext'`
Expected: FAIL (símbolos no definidos).

- [ ] **Step 3: Implement**

```go
// internal/config/context.go
package config

import "strings"

// Context es un destino conmutable: una credencial + un cluster.
type Context struct {
	Name            string `toml:"-"` // = clave del mapa [contexts.<name>]
	Cloud           string `toml:"cloud"`
	Profile         string `toml:"profile"`      // AWS
	AccountID       string `toml:"account_id"`   // AWS (opcional)
	RoleARN         string `toml:"role_arn"`     // AWS (opcional)
	Region          string `toml:"region"`       // AWS/GCP (opcional)
	Project         string `toml:"project"`      // GCP
	Subscription    string `toml:"subscription"` // Azure
	Cluster         string `toml:"cluster"`
	ServiceTemplate string `toml:"service_template"`
	Writable        bool   `toml:"writable"`
}

// ServiceName resuelve un nombre corto al nombre real vía service_template.
// Sin template, devuelve el nombre tal cual.
func (c Context) ServiceName(short string) string {
	if c.ServiceTemplate == "" {
		return short
	}
	return strings.ReplaceAll(c.ServiceTemplate, "{name}", short)
}

// Prefix es el prefijo a ocultar en la lista (= ServiceName("")).
func (c Context) Prefix() string { return c.ServiceName("") }

// Validate comprueba los campos mínimos del contexto.
func (c Context) Validate() error {
	if c.Cloud == "" {
		return fmt.Errorf("context %q: missing cloud", c.Name)
	}
	if c.Cluster == "" {
		return fmt.Errorf("context %q: missing cluster", c.Name)
	}
	if c.Cloud == "aws" && c.Profile == "" {
		return fmt.Errorf("context %q: aws context needs a profile", c.Name)
	}
	return nil
}
```

Añade el import `"fmt"` a `context.go`.

En `internal/config/config.go`, añade a `Config` los campos nuevos (deja `Providers` por ahora):

```go
type Config struct {
	DefaultContext string             `toml:"default_context"`
	Contexts       map[string]Context `toml:"contexts"`
	Providers      Providers          `toml:"providers"` // legacy; se elimina en Tarea 7
}
```

En `internal/config/resolve.go`, añade:

```go
import "sort" // (añadir al bloque de imports existente)

// Context devuelve el contexto con nombre name (con Name poblado) o un error.
func (c *Config) Context(name string) (Context, error) {
	ctx, ok := c.Contexts[name]
	if !ok {
		return Context{}, fmt.Errorf("context %q not found in config", name)
	}
	ctx.Name = name
	return ctx, nil
}

// AllContexts devuelve los contextos ordenados alfabéticamente por nombre.
func (c *Config) AllContexts() []Context {
	names := make([]string, 0, len(c.Contexts))
	for n := range c.Contexts {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]Context, 0, len(names))
	for _, n := range names {
		ctx := c.Contexts[n]
		ctx.Name = n
		out = append(out, ctx)
	}
	return out
}

// DefaultCtx devuelve el contexto por defecto: default_context, o el único, o error.
func (c *Config) DefaultCtx() (Context, error) {
	if c.DefaultContext != "" {
		return c.Context(c.DefaultContext)
	}
	if len(c.Contexts) == 1 {
		for n := range c.Contexts {
			return c.Context(n)
		}
	}
	return Context{}, fmt.Errorf("no default_context set and %d contexts available — pass --context", len(c.Contexts))
}

// ResolveContext resuelve por flag (si no vacío) o por defecto.
func (c *Config) ResolveContext(flag string) (Context, error) {
	if flag != "" {
		return c.Context(flag)
	}
	return c.DefaultCtx()
}
```

- [ ] **Step 4: Run to verify pass**

Run: `gofmt -w internal/config/ && go test ./internal/config/... && go build ./...`
Expected: PASS (los tests viejos de Environment/Naming siguen verdes; solo se añadió).

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat(config): modelo de contextos (aditivo)"
```

---

### Task 2: Fábrica de Deployer (aditivo)

**Files:**
- Create: `internal/providers/factory.go`
- Modify: `internal/providers/aws/session.go`
- Test: `internal/providers/factory_test.go`

**Interfaces:**
- Consumes: `config.Context`, `core.Deployer`, `aws.NewDeployer`.
- Produces:
  - `type DeployerFactory func(ctx config.Context) (core.Deployer, error)` (paquete `providers`)
  - `var ErrProviderNotImplemented = errors.New("provider not implemented")`
  - `func NewDeployerFactory() DeployerFactory`
  - `func aws.LoadConfigForContext(ctx context.Context, c config.Context) (awssdk.Config, error)`

- [ ] **Step 1: Write the failing test**

```go
// internal/providers/factory_test.go
package providers

import (
	"errors"
	"testing"

	"github.com/juanMaAV92/steer/internal/config"
	"github.com/stretchr/testify/require"
)

func TestFactoryUnknownCloud(t *testing.T) {
	f := NewDeployerFactory()
	_, err := f(config.Context{Name: "x", Cloud: "gcp", Cluster: "c"})
	require.ErrorIs(t, err, ErrProviderNotImplemented)
	require.ErrorContains(t, err, "gcp")
}

func TestFactoryAzureUnknownCloud(t *testing.T) {
	f := NewDeployerFactory()
	_, err := f(config.Context{Name: "x", Cloud: "azure", Cluster: "c"})
	require.True(t, errors.Is(err, ErrProviderNotImplemented))
}
```

(El camino AWS real necesita credenciales; no se prueba en unit. La fábrica solo cablea `LoadConfigForContext`+`NewDeployer`.)

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/providers/ -run TestFactory`
Expected: FAIL (paquete `providers` no existe).

- [ ] **Step 3: Implement**

```go
// internal/providers/factory.go
// Package providers cablea la construcción de capacidades por contexto/cloud.
package providers

import (
	"context"
	"errors"
	"fmt"

	"github.com/juanMaAV92/steer/internal/config"
	"github.com/juanMaAV92/steer/internal/core"
	"github.com/juanMaAV92/steer/internal/providers/aws"
)

// ErrProviderNotImplemented indica que el cloud del contexto aún no tiene provider.
var ErrProviderNotImplemented = errors.New("provider not implemented")

// DeployerFactory construye un Deployer para un contexto (o un error).
type DeployerFactory func(ctx config.Context) (core.Deployer, error)

// NewDeployerFactory devuelve la fábrica por defecto (AWS real; otros → error).
func NewDeployerFactory() DeployerFactory {
	return func(c config.Context) (core.Deployer, error) {
		switch c.Cloud {
		case "aws":
			cfg, err := aws.LoadConfigForContext(context.Background(), c)
			if err != nil {
				return nil, err
			}
			return aws.NewDeployer(cfg), nil
		default:
			return nil, fmt.Errorf("%w: %q", ErrProviderNotImplemented, c.Cloud)
		}
	}
}
```

En `internal/providers/aws/session.go`, añade (sin tocar `LoadConfig`/`profileFor` viejos):

```go
// LoadConfigForContext crea una aws.Config para un contexto (profile + region).
func LoadConfigForContext(ctx context.Context, c config.Context) (aws.Config, error) {
	var opts []func(*awscfg.LoadOptions) error
	if c.Profile != "" {
		opts = append(opts, awscfg.WithSharedConfigProfile(c.Profile))
	}
	if c.Region != "" {
		opts = append(opts, awscfg.WithRegion(c.Region))
	}
	return awscfg.LoadDefaultConfig(ctx, opts...)
}
```

- [ ] **Step 4: Run to verify pass**

Run: `gofmt -w internal/providers/ && go test ./internal/providers/... && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/providers/
git commit -m "feat(providers): DeployerFactory por contexto (AWS; otros no implementados)"
```

---

### Task 3: Migrar el CLI a contextos

**Files:**
- Modify: `internal/cli/context.go`
- Modify: `internal/cli/root.go`
- Modify: `internal/cli/service_cmd.go`
- Modify: `internal/cli/context_test.go`
- Modify: `internal/cli/service_cmd_test.go`

**Interfaces:**
- Consumes: `config.Context`, `config.Config.ResolveContext`, `providers.NewDeployerFactory`, `providers.DeployerFactory`, `Context.ServiceName`.
- Produces:
  - `type AppContext struct { Ctx config.Context; Config *config.Config; Factory providers.DeployerFactory }`
  - `func (a *AppContext) RequireWritable() error`
  - `func (a *AppContext) IsProduction() bool` → `!a.Ctx.Writable`
  - `newDeployerFn` con firma `func(app *AppContext) (core.Deployer, string, error)` que usa `app.Factory(app.Ctx)` y `app.Ctx.Cluster`.

- [ ] **Step 1: Actualizar tests de cli al nuevo AppContext**

```go
// internal/cli/context_test.go (reemplazar TestIsProduction)
func TestIsProduction(t *testing.T) {
	require.True(t, (&AppContext{Ctx: config.Context{Name: "prod", Writable: false}}).IsProduction())
	require.False(t, (&AppContext{Ctx: config.Context{Name: "stg", Writable: true}}).IsProduction())
}

func TestRequireWritable(t *testing.T) {
	require.NoError(t, (&AppContext{Ctx: config.Context{Writable: true}}).RequireWritable())
	require.Error(t, (&AppContext{Ctx: config.Context{Name: "prod", Writable: false}}).RequireWritable())
}
```

Añade el import `"github.com/juanMaAV92/steer/internal/config"` al test si falta.

En `internal/cli/service_cmd_test.go`, donde se arme un `AppContext` para inyectar el fake `newDeployerFn`, cámbialo para poblar `Ctx`/`Factory` en vez de `EnvName`/`Env`. (Localiza los usos de `AppContext{...EnvName...}` y `app.Config.Providers...`; reemplázalos por `AppContext{Ctx: config.Context{Name:"stg", Cluster:"stg-cluster", ServiceTemplate:"{name}", Writable:true}, Config: cfg, Factory: <fake>}`. El fake `newDeployerFn` se mantiene como seam; si el test lo sustituye por una función, esa función ahora lee `app.Ctx.Cluster`.)

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/cli/ -run 'IsProduction|RequireWritable'`
Expected: FAIL de compilación (AppContext aún tiene EnvName/Env).

- [ ] **Step 3: Implement**

`internal/cli/context.go`:

```go
// Package cli contiene el armazón de la CLI (Cobra) y el contexto de aplicación.
package cli

import (
	"fmt"

	"github.com/juanMaAV92/steer/internal/config"
	"github.com/juanMaAV92/steer/internal/providers"
)

// AppContext es el estado compartido por todos los comandos.
type AppContext struct {
	Ctx     config.Context
	Config  *config.Config
	Factory providers.DeployerFactory
}

// IsProduction indica si el contexto activo es de solo lectura (prod).
func (a *AppContext) IsProduction() bool { return !a.Ctx.Writable }

// RequireWritable falla si el contexto activo es de solo lectura.
func (a *AppContext) RequireWritable() error {
	if !a.Ctx.Writable {
		return fmt.Errorf("context %q is read-only (writable=false)", a.Ctx.Name)
	}
	return nil
}
```

`internal/cli/root.go` — la flag y la resolución:

```go
func NewRootCmd(version string) *cobra.Command {
	var contextName string

	root := &cobra.Command{
		Use:           "steer",
		Short:         "Steer your cloud from the terminal",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVarP(&contextName, "context", "c", "", "target context (default: default_context)")
	root.PersistentFlags().StringVarP(&contextName, "env", "e", "", "alias of --context") // compat

	root.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		factory := providers.NewDeployerFactory()
		if cmd.Parent() != nil && cmd.Parent().Name() == "config" {
			cmd.SetContext(context.WithValue(cmd.Context(), ctxKey{}, &AppContext{Factory: factory}))
			return nil
		}
		path, err := config.Find()
		if err != nil {
			return err
		}
		cfg, err := config.Load(path)
		if err != nil {
			return err
		}
		cur, err := cfg.ResolveContext(contextName)
		if err != nil {
			return err
		}
		if err := cur.Validate(); err != nil {
			return err
		}
		app := &AppContext{Ctx: cur, Config: cfg, Factory: factory}
		cmd.SetContext(context.WithValue(cmd.Context(), ctxKey{}, app))
		return nil
	}
	return root
}
```

Nota: registrar dos flags (`--context` y `--env`) sobre la MISMA variable `contextName` con `StringVarP` da error de banderas duplicadas con el shorthand. En su lugar registra `--context/-c` con `StringVarP` y `--env` SIN shorthand con `StringVar`:

```go
	root.PersistentFlags().StringVarP(&contextName, "context", "c", "", "target context (default: default_context)")
	root.PersistentFlags().StringVar(&contextName, "env", "", "alias of --context")
```

`internal/cli/service_cmd.go` — `newDeployerFn` y la resolución de nombres:

```go
var newDeployerFn = func(app *AppContext) (core.Deployer, string, error) {
	dep, err := app.Factory(app.Ctx)
	if err != nil {
		return nil, "", err
	}
	return dep, app.Ctx.Cluster, nil
}
```

Sustituye los `app.Config.Providers.AWS.Naming.Service(app.EnvName, service)` por `app.Ctx.ServiceName(service)` (3 sitios: deploy, scale, rollback) y los `app.EnvName` restantes en mensajes por `app.Ctx.Name`. El import de `aws` y `context` en `service_cmd.go` puede quedar sin uso tras quitar `aws.LoadConfig`/`context.Background()` de `newDeployerFn`; elimínalos si el compilador lo pide.

- [ ] **Step 4: Run to verify pass**

Run: `gofmt -w internal/cli/ && go vet ./internal/cli/... && go test ./internal/cli/... && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/context.go internal/cli/root.go internal/cli/service_cmd.go internal/cli/context_test.go internal/cli/service_cmd_test.go
git commit -m "feat(cli): resolver contexto con --context (alias -e) vía DeployerFactory"
```

---

### Task 4: Migrar la TUI al modelo de contexto (sin selector aún)

**Files:**
- Modify: `internal/tui/run.go`
- Modify: `internal/tui/app.go`
- Modify: `internal/cli/tui_cmd.go`
- Modify: `internal/tui/app_test.go`

**Interfaces:**
- Consumes: `providers.DeployerFactory`, `config.Context`, `config.Context.Cluster/Writable/Prefix`.
- Produces:
  - `func Run(factory providers.DeployerFactory, contexts []config.Context, current config.Context) error`
  - `func New(factory providers.DeployerFactory, contexts []config.Context, current config.Context) Model`
  - `Model` con campos `factory providers.DeployerFactory`, `contexts []config.Context`, `current config.Context`; los campos `cluster/env/writable/prefix` se derivan de `current`.

- [ ] **Step 1: Actualizar app_test.go a la nueva firma**

El helper `newTestModel` y los `New(...)` cambian. Reemplaza el helper:

```go
func newTestModel(services []core.ServiceStatus) Model {
	fake := &coretest.FakeDeployer{Services: services}
	factory := func(_ config.Context) (core.Deployer, error) { return fake, nil }
	cur := config.Context{Name: "stg", Cloud: "aws", Cluster: "stg-cluster", Writable: true}
	m := New(factory, []config.Context{cur}, cur)
	m.sidebar.setServices(services)
	m, _ = applySize(m, 120, 40)
	return m
}
```

Añade el import `"github.com/juanMaAV92/steer/internal/config"`. En el test read-only (`TestReadOnlyBlocksActions`), construye el modelo con `cur.Writable=false` en vez de `New(..., "production", false)`:

```go
func TestReadOnlyBlocksActions(t *testing.T) {
	fake := &coretest.FakeDeployer{Services: sampleServices()}
	factory := func(_ config.Context) (core.Deployer, error) { return fake, nil }
	cur := config.Context{Name: "production", Cloud: "aws", Cluster: "prod-cluster", Writable: false}
	ro := New(factory, []config.Context{cur}, cur)
	ro.sidebar.setServices(sampleServices())
	ro, _ = applySize(ro, 120, 40)
	for _, key := range []string{"d", "s", "R"} {
		m := mustUpdate(t, ro, keyMsg(key))
		require.NotEqual(t, focusAction, m.focus)
		require.NotEmpty(t, m.notice)
	}
}
```

Cualquier otro `New(...)` en `app_test.go` (p.ej. en `TestDeployFlowFeedsEventsPanel`) se actualiza igual: crear `factory` que devuelve el fake y un `cur` con `Cluster:"stg-cluster"`, `New(factory, []config.Context{cur}, cur)`.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/tui/ -run TestReadOnlyBlocksActions`
Expected: FAIL de compilación (firma de `New` distinta).

- [ ] **Step 3: Implement**

`internal/tui/run.go`:

```go
package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/juanMaAV92/steer/internal/config"
	"github.com/juanMaAV92/steer/internal/providers"
)

// Run abre la TUI a pantalla completa con soporte de mouse hasta que el usuario sale.
func Run(factory providers.DeployerFactory, contexts []config.Context, current config.Context) error {
	p := tea.NewProgram(
		New(factory, contexts, current),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	_, err := p.Run()
	return err
}
```

`internal/tui/app.go` — `New`, campos del Model, y derivación. Reemplaza la firma de `New` y los campos `cluster/env/writable/prefix` por `current` + la fábrica. El Model construye su primer Deployer con la fábrica:

```go
// (en el struct Model) reemplaza dep/cluster/env/writable/prefix por:
	factory  providers.DeployerFactory
	contexts []config.Context
	current  config.Context
	dep      core.Deployer
	depErr   error

func New(factory providers.DeployerFactory, contexts []config.Context, current config.Context) Model {
	dep, err := factory(current)
	m := Model{
		factory: factory, contexts: contexts, current: current,
		dep: dep, depErr: err,
		keys: defaultKeys(), sidebar: newSidebar(), events: panel.NewEvents(),
		loading: err == nil,
	}
	m.sidebar.prefix = current.Prefix()
	if err != nil {
		m.err = err
	}
	return m
}
```

Sustituye cada uso de los antiguos `m.cluster`, `m.env`, `m.writable`, y el prefijo, por derivaciones de `m.current`:
- `m.cluster` → `m.current.Cluster`
- `m.writable` → `m.current.Writable`
- `m.env` (en `topBar`) → `m.current.Name`; y `topBar("aws", ...)` usa `m.current.Cloud`.
- el prefijo del sidebar ya se fija en `New`/al conmutar (Tarea 6); en Details usa `m.current.Prefix()` para el displayName (hoy se pasa `strings.TrimPrefix(name, prefix)`; usa `m.current.Prefix()` como prefijo).
- `Init()` debe disparar `loadServicesCmd()` solo si `m.depErr == nil`.
- `loadServicesCmd` usa `m.dep` y `m.current.Cluster`.

`internal/cli/tui_cmd.go`:

```go
func NewTuiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Open the interactive dashboard",
		RunE: func(cmd *cobra.Command, _ []string) error {
			app := FromContext(cmd.Context())
			return tui.Run(app.Factory, app.Config.AllContexts(), app.Ctx)
		},
	}
}
```

- [ ] **Step 4: Run to verify pass**

Run: `gofmt -w internal/tui/ internal/cli/ && go vet ./internal/tui/... && go test ./internal/tui/... && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/run.go internal/tui/app.go internal/tui/app_test.go internal/cli/tui_cmd.go
git commit -m "feat(tui): Model basado en contexto (fábrica + contexto actual)"
```

---

### Task 5: Componente contextPicker

**Files:**
- Create: `internal/tui/contextpicker.go`
- Test: `internal/tui/contextpicker_test.go`

**Interfaces:**
- Consumes: `config.Context`, `render` helpers.
- Produces:
  - `type contextPicker struct { contexts []config.Context; cursor int }`
  - `func newContextPicker(contexts []config.Context, current string) contextPicker` — cursor en el contexto actual.
  - `func (p *contextPicker) moveUp()` / `func (p *contextPicker) moveDown()`
  - `func (p contextPicker) selected() (config.Context, bool)`
  - `func (p *contextPicker) selectIndex(i int)`
  - `func (p contextPicker) rowCount() int`
  - `func (p contextPicker) view() string` — agrupado por `cloud`, marca `(no impl.)` si `cloud != "aws"`, resalta el cursor y muestra `writable/read-only`.

- [ ] **Step 1: Write the failing tests**

```go
// internal/tui/contextpicker_test.go
package tui

import (
	"strings"
	"testing"

	"github.com/juanMaAV92/steer/internal/config"
	"github.com/stretchr/testify/require"
)

func samplePickerContexts() []config.Context {
	return []config.Context{
		{Name: "nao-dev", Cloud: "aws", Cluster: "c1", Writable: true},
		{Name: "nao-prod", Cloud: "aws", Cluster: "c2", Writable: false},
		{Name: "acme-staging", Cloud: "gcp", Cluster: "c3", Writable: true},
	}
}

func TestPickerStartsAtCurrent(t *testing.T) {
	p := newContextPicker(samplePickerContexts(), "nao-prod")
	sel, ok := p.selected()
	require.True(t, ok)
	require.Equal(t, "nao-prod", sel.Name)
}

func TestPickerNavigationClamps(t *testing.T) {
	p := newContextPicker(samplePickerContexts(), "nao-dev")
	p.moveUp() // clamp en 0
	require.Equal(t, 0, p.cursor)
	p.moveDown()
	p.moveDown()
	p.moveDown() // clamp en 2
	require.Equal(t, 2, p.cursor)
}

func TestPickerViewGroupsByCloudAndMarksNotImpl(t *testing.T) {
	p := newContextPicker(samplePickerContexts(), "nao-dev")
	out := p.view()
	require.Contains(t, out, "AWS")
	require.Contains(t, out, "GCP")
	require.Contains(t, out, "nao-dev")
	require.Contains(t, out, "acme-staging")
	require.Contains(t, strings.ToLower(out), "read-only") // nao-prod
	require.Contains(t, strings.ToLower(out), "no impl")    // gcp
}

func TestPickerSelectIndex(t *testing.T) {
	p := newContextPicker(samplePickerContexts(), "nao-dev")
	p.selectIndex(2)
	sel, _ := p.selected()
	require.Equal(t, "acme-staging", sel.Name)
	p.selectIndex(99) // fuera de rango: no-op
	sel, _ = p.selected()
	require.Equal(t, "acme-staging", sel.Name)
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/tui/ -run TestPicker`
Expected: FAIL (`newContextPicker` no definido).

- [ ] **Step 3: Implement**

```go
// internal/tui/contextpicker.go
package tui

import (
	"sort"
	"strings"

	"github.com/juanMaAV92/steer/internal/config"
	"github.com/juanMaAV92/steer/internal/render"
)

// contextPicker es el overlay para conmutar de contexto.
type contextPicker struct {
	contexts []config.Context // en orden de presentación (agrupado por cloud)
	cursor   int
}

// newContextPicker ordena los contextos por (cloud, nombre) y posiciona el cursor
// en el contexto actual.
func newContextPicker(contexts []config.Context, current string) contextPicker {
	ordered := make([]config.Context, len(contexts))
	copy(ordered, contexts)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Cloud != ordered[j].Cloud {
			return ordered[i].Cloud < ordered[j].Cloud
		}
		return ordered[i].Name < ordered[j].Name
	})
	cur := 0
	for i, c := range ordered {
		if c.Name == current {
			cur = i
			break
		}
	}
	return contextPicker{contexts: ordered, cursor: cur}
}

func (p *contextPicker) moveDown() {
	if p.cursor < len(p.contexts)-1 {
		p.cursor++
	}
}

func (p *contextPicker) moveUp() {
	if p.cursor > 0 {
		p.cursor--
	}
}

func (p *contextPicker) selectIndex(i int) {
	if i >= 0 && i < len(p.contexts) {
		p.cursor = i
	}
}

func (p contextPicker) selected() (config.Context, bool) {
	if p.cursor < 0 || p.cursor >= len(p.contexts) {
		return config.Context{}, false
	}
	return p.contexts[p.cursor], true
}

func (p contextPicker) rowCount() int { return len(p.contexts) }

func (p contextPicker) view() string {
	var b strings.Builder
	b.WriteString(render.Bold("Switch context") + "\n")
	lastCloud := ""
	for i, c := range p.contexts {
		if c.Cloud != lastCloud {
			b.WriteString(render.Dim(strings.ToUpper(c.Cloud)) + "\n")
			lastCloud = c.Cloud
		}
		cursor := "  "
		if i == p.cursor {
			cursor = render.Accent("> ")
		}
		state := render.Success("writable")
		if !c.Writable {
			state = render.Warn("read-only")
		}
		name := c.Name
		if i == p.cursor {
			name = render.Accent(name)
		}
		extra := ""
		if c.Cloud != "aws" {
			extra = render.Dim("  (no impl.)")
		}
		b.WriteString(cursor + name + "  " + state + extra + "\n")
	}
	b.WriteString(render.Dim("\n↑↓/click select · enter switch · esc cancel"))
	return b.String()
}
```

- [ ] **Step 4: Run to verify pass**

Run: `gofmt -w internal/tui/ && go test ./internal/tui/ -run TestPicker && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/contextpicker.go internal/tui/contextpicker_test.go
git commit -m "feat(tui): componente contextPicker (agrupado por cloud)"
```

---

### Task 6: Conmutación de contexto en la TUI

**Files:**
- Modify: `internal/tui/app.go`
- Test: `internal/tui/app_test.go`

**Interfaces:**
- Consumes: `contextPicker`, `m.factory`, `providers.ErrProviderNotImplemented`.
- Produces:
  - nuevo estado de foco `focusContextPicker`
  - campo `picker contextPicker` en `Model`
  - tecla `c` abre el picker; `enter` conmuta; `esc` cancela
  - al conmutar a AWS OK: reemplaza `m.dep`/`m.current`, fija `m.sidebar.prefix`, recarga servicios, vuelve a `focusSidebar`
  - a cloud no implementado o fallo de fábrica: `m.notice` con el error, sin cambiar

- [ ] **Step 1: Write the failing tests**

```go
// añadir a internal/tui/app_test.go
import "github.com/juanMaAV92/steer/internal/providers" // si falta

func multiCtxModel(t *testing.T) Model {
	t.Helper()
	fake := &coretest.FakeDeployer{Services: sampleServices()}
	factory := func(c config.Context) (core.Deployer, error) {
		if c.Cloud != "aws" {
			return nil, providers.ErrProviderNotImplemented
		}
		return fake, nil
	}
	ctxs := []config.Context{
		{Name: "nao-dev", Cloud: "aws", Cluster: "c1", Writable: true},
		{Name: "nao-prod", Cloud: "aws", Cluster: "c2", Writable: false},
		{Name: "acme-staging", Cloud: "gcp", Cluster: "c3", Writable: true},
	}
	m := New(factory, ctxs, ctxs[0])
	m.sidebar.setServices(sampleServices())
	m, _ = applySize(m, 120, 40)
	return m
}

func TestOpenContextPicker(t *testing.T) {
	m := multiCtxModel(t)
	m = mustUpdate(t, m, keyMsg("c"))
	require.Equal(t, focusContextPicker, m.focus)
}

func TestSwitchToWritableContextReloads(t *testing.T) {
	m := multiCtxModel(t)
	m = mustUpdate(t, m, keyMsg("c"))
	m.picker.selectIndex(1) // nao-prod (read-only)
	updated, cmd := m.Update(keyMsg("enter"))
	m = updated.(Model)
	require.Equal(t, "nao-prod", m.current.Name)
	require.False(t, m.current.Writable)
	require.Equal(t, focusSidebar, m.focus)
	require.NotNil(t, cmd) // recarga
}

func TestSwitchToNotImplementedShowsNotice(t *testing.T) {
	m := multiCtxModel(t)
	prev := m.current.Name
	m = mustUpdate(t, m, keyMsg("c"))
	// localizar acme-staging (gcp) por nombre
	for i, c := range m.picker.contexts {
		if c.Name == "acme-staging" {
			m.picker.selectIndex(i)
		}
	}
	m = mustUpdate(t, m, keyMsg("enter"))
	require.Equal(t, prev, m.current.Name) // no cambió
	require.NotEmpty(t, m.notice)
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/tui/ -run 'ContextPicker|Switch'`
Expected: FAIL (`focusContextPicker` y `m.picker` no existen).

- [ ] **Step 3: Implement**

En `internal/tui/app.go`:

1) Añade el estado de foco y el campo:

```go
const (
	focusSidebar focus = iota
	focusPanel
	focusAction
	focusContextPicker
)
// en Model:
	picker contextPicker
```

2) En `handleKey`, antes del switch global, captura el picker (igual que `focusAction`):

```go
	if m.focus == focusContextPicker {
		switch {
		case msg.Type == tea.KeyCtrlC:
			return m, tea.Quit
		case key.Matches(msg, m.keys.Esc):
			m.focus = focusSidebar
			return m, nil
		case key.Matches(msg, m.keys.Enter):
			return m.applyContextSwitch()
		case key.Matches(msg, m.keys.Down):
			m.picker.moveDown()
		case key.Matches(msg, m.keys.Up):
			m.picker.moveUp()
		}
		return m, nil
	}
```

3) Añade la tecla `c` para abrir el picker (en la zona global, junto a Refresh):

```go
	case msg.String() == "c":
		m.picker = newContextPicker(m.contexts, m.current.Name)
		m.notice = ""
		m.focus = focusContextPicker
		return m, nil
```

4) El handler de conmutación:

```go
func (m Model) applyContextSwitch() (tea.Model, tea.Cmd) {
	sel, ok := m.picker.selected()
	if !ok {
		m.focus = focusSidebar
		return m, nil
	}
	if sel.Name == m.current.Name {
		m.focus = focusSidebar
		return m, nil
	}
	dep, err := m.factory(sel)
	if err != nil {
		if errors.Is(err, providers.ErrProviderNotImplemented) {
			m.notice = "provider " + strconv.Quote(sel.Cloud) + " not implemented yet"
		} else {
			m.notice = "switch failed: " + err.Error()
		}
		m.focus = focusSidebar
		return m, nil // conserva el contexto previo
	}
	m.dep = dep
	m.current = sel
	m.sidebar = newSidebar()
	m.sidebar.prefix = sel.Prefix()
	m.sidebar.width = m.sidebarW
	m.loading = true
	m.notice = ""
	m.status = ""
	m.focus = focusSidebar
	return m, m.loadServicesCmd()
}
```

Añade los imports `"errors"` y `"github.com/juanMaAV92/steer/internal/providers"` a `app.go` (y `strconv` ya está).

5) En `View()`, cuando `m.focus == focusContextPicker`, renderiza el picker como overlay (reemplaza el cuerpo, igual que el action overlay usa la bottom bar). Mínimo: mostrar `m.picker.view()` en lugar del body:

```go
	if m.focus == focusContextPicker {
		return top + "\n" + m.picker.view() + "\n" + bottomBar(m.keys.shortHelp(), m.notice, m.status)
	}
```

(Coloca este retorno temprano en `View()`, tras calcular `top`.)

6) Añade `c context` a la cadena de ayuda en `keys.go` `shortHelp()` (p.ej. `· c context`).

- [ ] **Step 4: Run to verify pass**

Run: `gofmt -w internal/tui/ && go vet ./internal/tui/... && go test ./internal/tui/... && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/app.go internal/tui/app_test.go internal/tui/keys.go
git commit -m "feat(tui): selector de contexto (tecla c) con recarga al conmutar"
```

---

### Task 7: Limpieza del modelo viejo + config init/validate

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/resolve.go`
- Modify: `internal/config/config_test.go`
- Modify: `internal/config/resolve_test.go`
- Modify: `internal/providers/aws/session.go`
- Modify: `internal/providers/aws/session_test.go`
- Modify: `internal/cli/config_cmd.go`
- Modify: `internal/cli/config_cmd_test.go`

**Interfaces:**
- Produces: el modelo `Environment`/`Naming`/`Providers`/`LoadConfig`(viejo)/`profileFor`/`Config.Env`/`Naming.Cluster`/`Naming.Service` quedan ELIMINADOS; `config init`/`validate` operan sobre `[contexts.*]`.

- [ ] **Step 1: Actualizar tests al nuevo formato**

`internal/cli/config_cmd_test.go`: si afirma sobre `environments`, cámbialo para esperar el nuevo `exampleConfig` (que contiene `[contexts.` y `default_context`). `internal/config/config_test.go`: elimina `TestLoadParsesEnvironments` (cubierto por `TestLoadParsesContexts` de la Tarea 1). `internal/config/resolve_test.go`: elimina `TestEnvReturnsEnvironment`, `TestEnvUnknown`, `TestNamingDefaults`, `TestNamingTemplates`, `TestServiceTemplateWithEnv` (cubiertos por los tests de contexto). `internal/providers/aws/session_test.go`: elimina `TestProfileForEnv`/`TestProfileForEnvEmpty`.

- [ ] **Step 2: Run to verify state**

Run: `go build ./...`
Expected: aún compila (todavía no se ha borrado el código viejo; este paso confirma el punto de partida).

- [ ] **Step 3: Eliminar el modelo viejo**

`internal/config/config.go`: quita de `Config` el campo `Providers` y borra los tipos `Providers`, `AWS`, `Environment`, `Naming`.
`internal/config/resolve.go`: borra `func (c *Config) Env`, `func (n Naming) Cluster`, `func (n Naming) Service`. Conserva `candidatePaths`/`Find` y los métodos de contexto de la Tarea 1.
`internal/providers/aws/session.go`: borra `profileFor` y `LoadConfig(env config.Environment)` (queda `LoadConfigForContext`).

`internal/cli/config_cmd.go` — nuevo `exampleConfig` y `validate`:

```go
const exampleConfig = `default_context = "dev"

[contexts.dev]
cloud            = "aws"
profile          = "dev"
cluster          = "dev-cluster"
service_template = "{name}"
writable         = true

[contexts.prod]
cloud            = "aws"
profile          = "prod"
cluster          = "prod-cluster"
service_template = "{name}"
writable         = false
`
```

```go
func newConfigValidateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "validate",
		Short: "Validate the discovered steer.toml",
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := config.Find()
			if err != nil {
				return err
			}
			cfg, err := config.Load(path)
			if err != nil {
				return err
			}
			all := cfg.AllContexts()
			if len(all) == 0 {
				return fmt.Errorf("config %s has no contexts", path)
			}
			for _, c := range all {
				if err := c.Validate(); err != nil {
					return err
				}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "ok: %s (%d contexts)\n", path, len(all))
			return nil
		},
	}
}
```

- [ ] **Step 4: Run to verify pass**

Run: `gofmt -w internal/ && go vet ./... && go test ./... && go build ./...`
Expected: PASS en todo el módulo.

- [ ] **Step 5: Commit**

```bash
git add internal/config/ internal/providers/aws/session.go internal/providers/aws/session_test.go internal/cli/config_cmd.go internal/cli/config_cmd_test.go
git commit -m "refactor: eliminar modelo environments/naming; config init/validate por contextos"
```

---

## Nota de integración (no es una tarea)

`steer.toml` está **gitignored** (config local). Tras la Tarea 7, el `steer.toml` local del repo
sigue en formato viejo y `steer` fallará al cargarlo. Reescríbelo manualmente al nuevo formato
`[contexts.*]` (un contexto por cada uno de los 4 ambientes NAO, con su `profile`, `cluster`
real y `service_template = "nao-v2-<env>-{name}"`), o regéneralo con `steer config init` y
ajústalo. Verifícalo con `steer config validate` y `go run ./cmd/steerdemo` (que no depende de
config).
