# Onboarding (hito 08) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `steer config init` es un wizard (detecta perfiles AWS, propone contextos desde listas reales, valida y prueba la conexión), la config se gestiona con `add`/`remove`/`list`, y los errores comunes de AWS enseñan su remedio en wizard, CLI y TUI.

**Architecture:** Un paquete `internal/cli/wizard` con el flujo agnóstico (formularios `huh` + funciones puras de propuesta) que consume una interface `Detector`; la implementación AWS vive en `internal/providers/aws` (parser de `~/.aws`, `ListClusters`, smoke test). `config` gana mutación + serialización determinista (`AddContext`/`RemoveContext`/`Write`). Un traductor `providers.Friendly(err)` aplica los mensajes-que-enseñan en el único punto de impresión del CLI (main), en la pantalla de error del TUI y en cada fallo del wizard.

**Tech Stack:** Go, `charmbracelet/huh` (ya en go.mod), aws-sdk-go-v2 (ecs ListClusters).
Spec: `docs/superpowers/specs/2026-07-07-onboarding-design.md`.

## Global Constraints

- Comentarios en español; strings de UI en inglés. PROHIBIDA atribución a Claude/IA (sin trailer Co-Authored-By).
- Branch: `feat/onboarding` (creada en Task 1 desde main).
- Antes de CADA commit: `gofmt -w internal/ cmd/ && golangci-lint run && go vet ./... && go test ./... -count=1 && go build ./...` — todo verde.
- Toda la LÓGICA del wizard es testeable sin TTY: propuestas y mutación de config = funciones puras con unit tests; el `Detector` se inyecta (fake en tests); la capa `huh` queda delgada y sin tests (documentado).
- `Write` es determinista (contextos ordenados alfabéticamente, default_context primero) y preserva los contextos existentes en add/remove; los comentarios manuales del usuario NO se preservan (aviso "rewrote <path>" en el output).
- Los mensajes amables siguen el formato `<qué pasó> — try: <remedio>` + línea tenue con el error original truncado a 120 chars.
- `config init --example` conserva el dump estático EXACTO actual (`exampleConfig` de config_cmd.go).
- Permisos de escritura del toml: 0600; directorio global `~/.config/steer` se crea con 0755 si falta.
- Cancelar el wizard (esc/ctrl+c en huh → error) no escribe nada; en modo add, el toml original queda intacto.

---

### Task 1: config — mutación y serialización determinista

**Files:**
- Create: `internal/config/write.go`
- Create: `internal/config/write_test.go`
- Modify: `internal/config/resolve.go` (helper GlobalPath)

**Interfaces:**
- Consumes: `Config`/`Context` (`internal/config/config.go`, `context.go`), `candidatePaths` (`resolve.go:11`).
- Produces (T4-T5 dependen):
  - `(c *Config) AddContext(ctx Context) error` (error si el nombre ya existe o Validate del contexto falla; si es el primero, lo fija como default)
  - `(c *Config) RemoveContext(name string) (wasDefault bool, err error)` (error si no existe; si era default: default pasa al primer restante alfabético, o "" si no queda ninguno)
  - `(c *Config) Write(path string) error` (serialización determinista + 0600 + mkdir -p del directorio)
  - `config.GlobalPath() (string, error)` (`~/.config/steer/steer.toml`)

- [ ] **Step 1: Crear la branch**

```bash
git checkout main && git pull && git checkout -b feat/onboarding
```

- [ ] **Step 2: Tests que fallan**

Crear `internal/config/write_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func devCtx(name string) Context {
	return Context{Name: name, Cloud: "aws", Profile: name, Cluster: name + "-cluster",
		ServiceTemplate: name + "-{name}", Writable: true}
}

func TestAddContextSetsFirstAsDefault(t *testing.T) {
	c := &Config{Contexts: map[string]Context{}}
	require.NoError(t, c.AddContext(devCtx("dev")))
	require.Equal(t, "dev", c.DefaultContext)
	require.NoError(t, c.AddContext(devCtx("stg")))
	require.Equal(t, "dev", c.DefaultContext) // el default no cambia al agregar más
	// duplicado y contexto inválido fallan
	require.ErrorContains(t, c.AddContext(devCtx("dev")), "already exists")
	bad := devCtx("x")
	bad.Cluster = ""
	require.ErrorContains(t, c.AddContext(bad), "missing cluster")
}

func TestRemoveContextReassignsDefault(t *testing.T) {
	c := &Config{Contexts: map[string]Context{}}
	require.NoError(t, c.AddContext(devCtx("dev")))
	require.NoError(t, c.AddContext(devCtx("prod")))
	wasDefault, err := c.RemoveContext("dev")
	require.NoError(t, err)
	require.True(t, wasDefault)
	require.Equal(t, "prod", c.DefaultContext) // reasignado al primero alfabético
	_, err = c.RemoveContext("nope")
	require.ErrorContains(t, err, "not found")
	wasDefault, err = c.RemoveContext("prod")
	require.NoError(t, err)
	require.True(t, wasDefault)
	require.Empty(t, c.DefaultContext) // no queda ninguno
}

func TestWriteRoundTripPreservesEverything(t *testing.T) {
	c := &Config{Contexts: map[string]Context{}}
	dev := devCtx("dev")
	dev.Images = &ImagesConfig{RepoTemplate: "shared-{name}"}
	prod := devCtx("prod")
	prod.Writable = false
	prod.RoleARN = "arn:aws:iam::1:role/deployer"
	prod.Region = "us-east-1"
	require.NoError(t, c.AddContext(dev))
	require.NoError(t, c.AddContext(prod))

	path := filepath.Join(t.TempDir(), "sub", "steer.toml")
	require.NoError(t, c.Write(path))

	// permisos y determinismo
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	raw1, _ := os.ReadFile(path)
	require.NoError(t, c.Write(path))
	raw2, _ := os.ReadFile(path)
	require.Equal(t, string(raw1), string(raw2), "la serialización debe ser determinista")
	require.True(t, strings.Index(string(raw1), "[contexts.dev]") <
		strings.Index(string(raw1), "[contexts.prod]"), "contextos en orden alfabético")

	// round-trip por el loader real
	loaded, err := Load(path)
	require.NoError(t, err)
	require.NoError(t, loaded.Validate())
	require.Equal(t, "dev", loaded.DefaultContext)
	lDev, _ := loaded.Context("dev")
	require.Equal(t, "shared-{name}", lDev.Images.RepoTemplate)
	lProd, _ := loaded.Context("prod")
	require.False(t, lProd.Writable)
	require.Equal(t, "arn:aws:iam::1:role/deployer", lProd.RoleARN)
	require.Equal(t, "us-east-1", lProd.Region)
}

func TestGlobalPath(t *testing.T) {
	p, err := GlobalPath()
	require.NoError(t, err)
	require.True(t, strings.HasSuffix(p, filepath.Join(".config", "steer", "steer.toml")))
}
```

- [ ] **Step 3: Verificar que fallan**

Run: `go test ./internal/config/ -run 'TestAddContext|TestRemoveContext|TestWriteRound|TestGlobalPath' -v`
Expected: FAIL con "c.AddContext undefined"

- [ ] **Step 4: Implementar `internal/config/write.go`**

```go
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// AddContext valida y agrega un contexto; el primero se vuelve default_context.
func (c *Config) AddContext(ctx Context) error {
	if err := ctx.Validate(); err != nil {
		return err
	}
	if c.Contexts == nil {
		c.Contexts = map[string]Context{}
	}
	if _, ok := c.Contexts[ctx.Name]; ok {
		return fmt.Errorf("context %q already exists", ctx.Name)
	}
	name := ctx.Name
	ctx.Name = "" // la clave del mapa es la fuente del nombre
	c.Contexts[name] = ctx
	if c.DefaultContext == "" {
		c.DefaultContext = name
	}
	return nil
}

// RemoveContext elimina un contexto; si era el default, reasigna al primero
// alfabético restante (o lo deja vacío si no queda ninguno).
func (c *Config) RemoveContext(name string) (wasDefault bool, err error) {
	if _, ok := c.Contexts[name]; !ok {
		return false, fmt.Errorf("context %q not found", name)
	}
	delete(c.Contexts, name)
	if c.DefaultContext != name {
		return false, nil
	}
	c.DefaultContext = ""
	names := make([]string, 0, len(c.Contexts))
	for n := range c.Contexts {
		names = append(names, n)
	}
	sort.Strings(names)
	if len(names) > 0 {
		c.DefaultContext = names[0]
	}
	return true, nil
}

// GlobalPath es la ruta global de config (~/.config/steer/steer.toml).
func GlobalPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "steer", "steer.toml"), nil
}

// Write serializa la config de forma determinista (default primero, contextos
// alfabéticos) con permisos 0600, creando el directorio si falta. Los comentarios
// de un archivo previo NO se preservan.
func (c *Config) Write(path string) error {
	var b strings.Builder
	if c.DefaultContext != "" {
		fmt.Fprintf(&b, "default_context = %s\n\n", strconv.Quote(c.DefaultContext))
	}
	for i, ctx := range c.AllContexts() {
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "[contexts.%s]\n", ctx.Name)
		writeKV(&b, "cloud", ctx.Cloud)
		writeKV(&b, "profile", ctx.Profile)
		writeKV(&b, "account_id", ctx.AccountID)
		writeKV(&b, "role_arn", ctx.RoleARN)
		writeKV(&b, "region", ctx.Region)
		writeKV(&b, "project", ctx.Project)
		writeKV(&b, "subscription", ctx.Subscription)
		writeKV(&b, "cluster", ctx.Cluster)
		writeKV(&b, "service_template", ctx.ServiceTemplate)
		fmt.Fprintf(&b, "writable = %t\n", ctx.Writable)
		if ctx.Images != nil {
			fmt.Fprintf(&b, "\n  [contexts.%s.images]\n", ctx.Name)
			fmt.Fprintf(&b, "  repo_template = %s\n", strconv.Quote(ctx.Images.RepoTemplate))
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(b.String()), 0o600)
}

// writeKV emite `clave = "valor"` omitiendo los vacíos (campos opcionales).
func writeKV(b *strings.Builder, key, val string) {
	if val == "" {
		return
	}
	fmt.Fprintf(b, "%s = %s\n", key, strconv.Quote(val))
}
```

- [ ] **Step 5: Verificar + gates + commit**

```bash
gofmt -w internal/ && golangci-lint run && go vet ./... && go test ./... -count=1 && go build ./...
git add internal/config/
git commit -m "feat(config): AddContext/RemoveContext y serialización determinista de steer.toml"
```

---

### Task 2: Errores que enseñan — `providers.Friendly`

**Files:**
- Create: `internal/providers/aws/errors.go`
- Create: `internal/providers/aws/errors_test.go`
- Modify: `internal/providers/factory.go` (fachada `providers.Friendly`)
- Modify: `cmd/steer/main.go` (punto de impresión del CLI)
- Modify: `internal/tui/app.go` (pantalla de error del TUI)
- Modify: `internal/tui/app_test.go`

**Interfaces:**
- Consumes: errores del aws-sdk-go-v2 (`ecstypes.ClusterNotFoundException`, mensajes de credenciales/SSO/red por substring — patrón `IsProvisioningFailure`).
- Produces (T3-T4 dependen): `providers.Friendly(err error) string` — si hay mapeo: `"<qué pasó> — try: <remedio>\n  (<error original truncado a 120>)"`; sin mapeo: `err.Error()` tal cual. `aws.FriendlyError(err) (string, bool)` interno.

- [ ] **Step 1: Tests que fallan**

Crear `internal/providers/aws/errors_test.go`:

```go
package aws

import (
	"errors"
	"testing"

	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/stretchr/testify/require"
)

func TestFriendlyErrorMappings(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string // substring del mensaje amable
	}{
		{"sso expirado", errors.New("failed to refresh cached credentials, the SSO session has expired or is invalid"),
			"AWS session expired — try: aws sso login --profile <your-profile>"},
		{"sin perfil", errors.New(`failed to get shared config profile, devx`),
			"try: aws configure --profile"},
		{"sin credenciales", errors.New("failed to retrieve credentials, no EC2 IMDS role found"),
			"no AWS credentials found"},
		{"access denied", errors.New("api error AccessDeniedException: User is not authorized to perform: ecs:ListServices"),
			"access denied"},
		{"cluster no existe", &ecstypes.ClusterNotFoundException{},
			"cluster not found in this account/region"},
		{"timeout", errors.New("dial tcp: lookup ecs.us-east-1.amazonaws.com: i/o timeout"),
			"could not reach AWS"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msg, ok := FriendlyError(tc.err)
			require.True(t, ok, "debe mapear: %v", tc.err)
			require.Contains(t, msg, tc.want)
			require.Contains(t, msg, "— try:", "formato qué-pasó — try: remedio")
		})
	}
}

func TestFriendlyErrorFallback(t *testing.T) {
	_, ok := FriendlyError(errors.New("algo totalmente distinto"))
	require.False(t, ok)
}

func TestFriendlyErrorTruncatesOriginal(t *testing.T) {
	long := errors.New("failed to refresh cached credentials, the SSO session has expired " +
		string(make([]byte, 300)))
	msg, ok := FriendlyError(long)
	require.True(t, ok)
	require.LessOrEqual(t, len(msg), 400, "el original va truncado a ~120 chars")
}
```

- [ ] **Step 2: Verificar que fallan**

Run: `go test ./internal/providers/aws/ -run TestFriendly -v`
Expected: FAIL con "undefined: FriendlyError"

- [ ] **Step 3: Implementar**

`internal/providers/aws/errors.go`:

```go
package aws

import (
	"errors"
	"strings"

	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
)

// friendlyRule mapea una firma de error AWS a un mensaje que enseña el remedio.
type friendlyRule struct {
	match func(err error, lower string) bool
	msg   string
}

// Detección mixta: tipos del SDK cuando existen (errors.As) y substrings
// documentados cuando el SDK solo da texto (mismo criterio que IsProvisioningFailure).
var friendlyRules = []friendlyRule{
	{func(_ error, l string) bool {
		return strings.Contains(l, "sso session has expired") || strings.Contains(l, "sso session is invalid") ||
			strings.Contains(l, "token has expired")
	}, "AWS session expired — try: aws sso login --profile <your-profile>"},
	{func(_ error, l string) bool { return strings.Contains(l, "failed to get shared config profile") },
		"AWS profile not found — try: aws configure --profile <name> (or check ~/.aws/config)"},
	{func(_ error, l string) bool { return strings.Contains(l, "failed to retrieve credentials") },
		"no AWS credentials found — try: aws configure, or aws sso login if your team uses SSO"},
	{func(_ error, l string) bool {
		return strings.Contains(l, "accessdenied") || strings.Contains(l, "not authorized to perform")
	}, "access denied — try: ask whoever manages AWS to grant your role ECS/ECR read permissions"},
	{func(err error, _ string) bool {
		var cnf *ecstypes.ClusterNotFoundException
		return errors.As(err, &cnf)
	}, "cluster not found in this account/region — try: check the cluster name in steer.toml and the profile's region"},
	{func(_ error, l string) bool {
		return strings.Contains(l, "i/o timeout") || strings.Contains(l, "no such host") ||
			strings.Contains(l, "connection refused")
	}, "could not reach AWS — try: check your network/VPN and retry"},
}

// FriendlyError traduce errores comunes de AWS a mensajes accionables.
// ok=false si no hay mapeo (el llamador muestra el error tal cual).
func FriendlyError(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	lower := strings.ToLower(err.Error())
	for _, r := range friendlyRules {
		if r.match(err, lower) {
			orig := err.Error()
			if len(orig) > 120 {
				orig = orig[:117] + "..."
			}
			return r.msg + "\n  (" + orig + ")", true
		}
	}
	return "", false
}
```

`internal/providers/factory.go` — fachada agnóstica (junto a IsImplemented):

```go
// Friendly traduce errores comunes del cloud a mensajes que enseñan el remedio;
// sin mapeo devuelve err.Error() tal cual. Fachada por-provider (AWS hoy).
func Friendly(err error) string {
	if msg, ok := aws.FriendlyError(err); ok {
		return msg
	}
	return err.Error()
}
```

`cmd/steer/main.go` — el punto único de impresión:

```go
		fmt.Fprintln(os.Stderr, "error:", cli.FriendlyError(err))
```

con `func FriendlyError(err error) string { return providers.Friendly(err) }` exportado
en `internal/cli/context.go` (main no debe importar providers directo — respeta la
capa; una línea).

`internal/tui/app.go` — pantalla de error (`View`, primer if):

```go
	if m.err != nil {
		return render.Danger("error: "+providers.Friendly(m.err)) + "\n" + render.Dim("press q to quit")
	}
```

- [ ] **Step 4: Test TUI + gates + commit**

Añadir a `internal/tui/app_test.go`:

```go
// TestErrorScreenTeaches: el error de SSO vencido se muestra con su remedio.
func TestErrorScreenTeaches(t *testing.T) {
	m := newTestModel(sampleServices())
	m.err = errors.New("failed to refresh cached credentials, the SSO session has expired")
	out := stripANSI(m.View())
	require.Contains(t, out, "aws sso login")
}
```

```bash
gofmt -w internal/ cmd/ && golangci-lint run && go vet ./... && go test ./... -count=1 && go build ./...
git add internal/providers/ internal/cli/context.go internal/tui/ cmd/steer/main.go
git commit -m "feat(providers,cli,tui): errores de AWS que enseñan el remedio"
```

---

### Task 3: Detector AWS — perfiles, clusters en vivo y smoke test

**Files:**
- Create: `internal/cli/wizard/detector.go` (solo la interface + fake compartido de test)
- Create: `internal/providers/aws/detector.go`
- Create: `internal/providers/aws/detector_test.go`
- Modify: `internal/providers/aws/ecs.go` (ecsAPI gana ListClusters)

**Interfaces:**
- Consumes: `LoadConfigForContext` (`aws/session.go`), `ecsAPI`, `providers.NewProviderFactory` (para el smoke test), `config.Context`.
- Produces (T4 depende):

```go
// package wizard
type Detector interface {
	Profiles() ([]string, error)
	Clusters(ctx context.Context, profile, region string) ([]string, error)
	// SmokeTest construye el provider del contexto y cuenta sus servicios.
	SmokeTest(ctx context.Context, c config.Context) (int, error)
}
```
  - `aws.NewDetector() *Detector` (implementación; lee `~/.aws` real) y
    `aws.NewDetectorWithHome(home string)` (inyectable para tests).

- [ ] **Step 1: Tests que fallan**

Crear `internal/providers/aws/detector_test.go`:

```go
package aws

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/stretchr/testify/require"
)

func writeAWSFixtures(t *testing.T) string {
	home := t.TempDir()
	dir := filepath.Join(home, ".aws")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "config"), []byte(`[default]
region = us-east-1

[profile dev]
sso_session = corp

[profile staging]
region = us-west-2
`), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "credentials"), []byte(`[legacy]
aws_access_key_id = AKIA...

[dev]
aws_access_key_id = AKIA...
`), 0o600))
	return home
}

func TestDetectorProfilesParsesAndDedups(t *testing.T) {
	d := NewDetectorWithHome(writeAWSFixtures(t))
	profiles, err := d.Profiles()
	require.NoError(t, err)
	// default + dev + staging (config) + legacy (credentials); dev deduplicado; orden alfabético
	require.Equal(t, []string{"default", "dev", "legacy", "staging"}, profiles)
}

func TestDetectorProfilesNoAWSDir(t *testing.T) {
	d := NewDetectorWithHome(t.TempDir())
	profiles, err := d.Profiles()
	require.NoError(t, err) // sin ~/.aws no es error: lista vacía (el wizard enseña)
	require.Empty(t, profiles)
}

func TestDetectorClustersListsNames(t *testing.T) {
	api := &fakeECS{clusterArns: []string{
		"arn:aws:ecs:us-east-1:1:cluster/nao-v2-dev-cluster",
		"arn:aws:ecs:us-east-1:1:cluster/otro",
	}}
	d := NewDetectorWithHome(t.TempDir())
	d.newECS = func(context.Context, string, string) (ecsAPI, error) { return api, nil }
	clusters, err := d.Clusters(context.Background(), "dev", "")
	require.NoError(t, err)
	require.Equal(t, []string{"nao-v2-dev-cluster", "otro"}, clusters)
}
```

(El `fakeECS` del paquete gana el campo `clusterArns` y el método `ListClusters` que
los devuelve; seguir el patrón de los fakes existentes en ecs_test.go/registry_test.go.
Usar `awssdk` donde haga falta para punteros.)

- [ ] **Step 2: Verificar que fallan**

Run: `go test ./internal/providers/aws/ -run TestDetector -v`
Expected: FAIL con "undefined: NewDetectorWithHome"

- [ ] **Step 3: Implementar**

3a. `ecsAPI` (ecs.go) gana:

```go
	ListClusters(ctx context.Context, in *ecs.ListClustersInput, optFns ...func(*ecs.Options)) (*ecs.ListClustersOutput, error)
```

(los fakes de ecs_test.go/registry_test.go ganan un stub que devuelve
`&ecs.ListClustersOutput{}` — o el campo `clusterArns` donde el test lo use).

3b. `internal/providers/aws/detector.go`:

```go
package aws

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/juanMaAV92/steer/internal/config"
	"github.com/juanMaAV92/steer/internal/core"
)

// Detector descubre perfiles y clusters AWS para el wizard de onboarding.
type Detector struct {
	home string
	// newECS es inyectable en tests: construye el cliente ECS de un perfil/región.
	newECS func(ctx context.Context, profile, region string) (ecsAPI, error)
}

// NewDetector lee el ~/.aws real del usuario.
func NewDetector() *Detector {
	home, _ := os.UserHomeDir()
	return NewDetectorWithHome(home)
}

// NewDetectorWithHome es el constructor inyectable (tests).
func NewDetectorWithHome(home string) *Detector {
	d := &Detector{home: home}
	d.newECS = func(ctx context.Context, profile, region string) (ecsAPI, error) {
		cfg, err := LoadConfigForContext(ctx, config.Context{Profile: profile, Region: region})
		if err != nil {
			return nil, err
		}
		return ecs.NewFromConfig(cfg), nil
	}
	return d
}

// Profiles parsea ~/.aws/config ([profile X] y [default]) y ~/.aws/credentials
// ([X]), deduplicados y en orden alfabético. Sin ~/.aws devuelve lista vacía.
func (d *Detector) Profiles() ([]string, error) {
	set := map[string]bool{}
	parse := func(path, prefix string) error {
		f, err := os.Open(path)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if !strings.HasPrefix(line, "[") || !strings.HasSuffix(line, "]") {
				continue
			}
			name := strings.TrimSpace(strings.Trim(line, "[]"))
			name = strings.TrimSpace(strings.TrimPrefix(name, prefix))
			if name != "" {
				set[name] = true
			}
		}
		return sc.Err()
	}
	if err := parse(filepath.Join(d.home, ".aws", "config"), "profile "); err != nil {
		return nil, err
	}
	if err := parse(filepath.Join(d.home, ".aws", "credentials"), ""); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(set))
	for n := range set {
		out = append(out, n)
	}
	sort.Strings(out)
	return out, nil
}

// Clusters lista los clusters ECS visibles con ese perfil/región (nombres, no ARNs).
func (d *Detector) Clusters(ctx context.Context, profile, region string) ([]string, error) {
	api, err := d.newECS(ctx, profile, region)
	if err != nil {
		return nil, err
	}
	var names []string
	var token *string
	for {
		out, err := api.ListClusters(ctx, &ecs.ListClustersInput{NextToken: token})
		if err != nil {
			return nil, err
		}
		for _, arn := range out.ClusterArns {
			if i := strings.LastIndex(arn, "/"); i >= 0 {
				names = append(names, arn[i+1:])
			}
		}
		if awssdk.ToString(out.NextToken) == "" {
			break
		}
		token = out.NextToken
	}
	sort.Strings(names)
	return names, nil
}

// SmokeTest construye el deployer del contexto y cuenta sus servicios.
func (d *Detector) SmokeTest(ctx context.Context, c config.Context) (int, error) {
	p, err := NewProvider(ctx, c)
	if err != nil {
		return 0, err
	}
	var dep core.Deployer
	if dep, err = p.Deployer(); err != nil {
		return 0, err
	}
	svcs, err := dep.ListServices(ctx)
	if err != nil {
		return 0, err
	}
	return len(svcs), nil
}
```

3c. `internal/cli/wizard/detector.go` — la interface (T4 la consume):

```go
// Package wizard implementa el flujo de onboarding (config init/add) sobre un
// Detector por-provider: el flujo es agnóstico de cloud.
package wizard

import (
	"context"

	"github.com/juanMaAV92/steer/internal/config"
)

// Detector descubre credenciales y destinos en un cloud concreto (AWS hoy;
// GCP/Azure implementarán el suyo cuando lleguen sus providers).
type Detector interface {
	Profiles() ([]string, error)
	Clusters(ctx context.Context, profile, region string) ([]string, error)
	// SmokeTest construye el provider del contexto y devuelve cuántos servicios ve.
	SmokeTest(ctx context.Context, c config.Context) (int, error)
}
```

Verificación de conformidad: `var _ wizard.Detector = (*aws.Detector)(nil)` en
detector.go de aws (import del paquete wizard — si genera ciclo, poner la aserción en
detector_test.go del paquete wizard en T4; comprobar y documentar en el reporte).

- [ ] **Step 4: Verificar + gates + commit**

```bash
gofmt -w internal/ && golangci-lint run && go vet ./... && go test ./... -count=1 && go build ./...
git add internal/providers/aws/ internal/cli/wizard/
git commit -m "feat(providers): Detector AWS — perfiles de ~/.aws, ListClusters y smoke test"
```

---

### Task 4: Wizard — propuestas puras, flujo huh y `config init`/`add`

**Files:**
- Create: `internal/cli/wizard/propose.go`
- Create: `internal/cli/wizard/propose_test.go`
- Create: `internal/cli/wizard/wizard.go` (flujo huh — capa delgada)
- Modify: `internal/cli/config_cmd.go` (init reescrito + add + --example)
- Modify: `internal/cli/config_cmd_test.go`

**Interfaces:**
- Consumes: `wizard.Detector` (T3), `config.AddContext/Write/GlobalPath/Find/Load` (T1), `providers.Friendly` (T2), `huh` (patrón de `interactive.go`).
- Produces:
  - Propuestas puras: `ProposeName(cluster) string`, `ProposeServiceTemplate(cluster) string`, `ProposeWritable(name) bool`, `ProposeRepoTemplate(serviceTemplate) string`
  - `wizard.Run(ctx, det Detector, existing *config.Config) (*config.Config, string /*path*/, error)` — flujo completo (nil existing = desde cero; con existing = modo add, conserva lo previo)
  - CLI: `steer config init` (wizard; con config existente ofrece add/recrear; `--example` = dump estático actual), `steer config add`.

- [ ] **Step 1: Tests que fallan — propuestas puras**

Crear `internal/cli/wizard/propose_test.go`:

```go
package wizard

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProposeName(t *testing.T) {
	require.Equal(t, "nao-v2-dev", ProposeName("nao-v2-dev-cluster"))
	require.Equal(t, "prod", ProposeName("prod"))
	require.Equal(t, "my-cluster-x", ProposeName("my-cluster-x")) // solo sufijo exacto
}

func TestProposeServiceTemplate(t *testing.T) {
	require.Equal(t, "nao-v2-dev-{name}", ProposeServiceTemplate("nao-v2-dev-cluster"))
	require.Equal(t, "prod-{name}", ProposeServiceTemplate("prod"))
}

func TestProposeWritable(t *testing.T) {
	require.False(t, ProposeWritable("nao-production"))
	require.False(t, ProposeWritable("PROD-east"))
	require.False(t, ProposeWritable("client-prd"))
	require.True(t, ProposeWritable("dev"))
	require.True(t, ProposeWritable("staging"))
}

func TestProposeRepoTemplate(t *testing.T) {
	// quita el último segmento-de-ambiente del prefijo del service_template
	require.Equal(t, "nao-v2-{name}", ProposeRepoTemplate("nao-v2-dev-{name}"))
	require.Equal(t, "{name}", ProposeRepoTemplate("prod-{name}"))
	require.Equal(t, "{name}", ProposeRepoTemplate("{name}"))
}
```

- [ ] **Step 2: Verificar que fallan**

Run: `go test ./internal/cli/wizard/ -v`
Expected: FAIL con "undefined: ProposeName"

- [ ] **Step 3: Implementar `internal/cli/wizard/propose.go`**

```go
package wizard

import "strings"

// ProposeName deduce el nombre del contexto desde el cluster ("-cluster" fuera).
func ProposeName(cluster string) string {
	return strings.TrimSuffix(cluster, "-cluster")
}

// ProposeServiceTemplate propone "<nombre>-{name}" (patrón observado: los servicios
// llevan el prefijo del ambiente).
func ProposeServiceTemplate(cluster string) string {
	return ProposeName(cluster) + "-{name}"
}

// ProposeWritable default false si el nombre huele a producción.
func ProposeWritable(name string) bool {
	l := strings.ToLower(name)
	for _, marker := range []string{"prod", "prd"} {
		if strings.Contains(l, marker) {
			return false
		}
	}
	return true
}

// ProposeRepoTemplate quita el último segmento del prefijo del service_template
// (los registries suelen ser compartidos entre ambientes: nao-v2-dev-{name} →
// nao-v2-{name}); sin segmentos suficientes devuelve "{name}".
func ProposeRepoTemplate(serviceTemplate string) string {
	prefix := strings.TrimSuffix(serviceTemplate, "{name}")
	prefix = strings.TrimSuffix(prefix, "-")
	if prefix == "" {
		return "{name}"
	}
	parts := strings.Split(prefix, "-")
	if len(parts) <= 1 {
		return "{name}"
	}
	return strings.Join(parts[:len(parts)-1], "-") + "-{name}"
}
```

- [ ] **Step 4: Flujo huh (`internal/cli/wizard/wizard.go`) — capa delgada, sin tests unitarios (documentado)**

Estructura (usar `huh.NewForm` por paso, como `interactive.go`; TODOS los strings de UI
en inglés; en cada punto donde un Detector falla, mostrar `providers.Friendly(err)` y
degradar a input manual):

```go
// Run ejecuta el wizard. existing=nil → config nueva; con existing → modo add
// (conserva contextos previos). Devuelve la config final y la ruta destino elegida.
func Run(ctx context.Context, det Detector, existing *config.Config) (*config.Config, string, error)
```

Pasos (por contexto, en loop "Add another context?"):
1. Cloud: `huh.NewSelect` con `aws` habilitado; `gcp`/`azure` como opciones
   deshabilitadas con sufijo "(not implemented yet)" — fuente `providers.IsImplemented`.
2. Profile: select con `det.Profiles()`; lista vacía → nota que enseña
   ("No AWS profiles found — install the AWS CLI and run: aws configure") + input manual.
3. Region: input opcional (placeholder "leave empty to use the profile's default").
4. Cluster: `det.Clusters(ctx, profile, region)` → select; error → `providers.Friendly`
   + input manual.
5. Confirmar propuestas (inputs precargados editables): name=ProposeName,
   service_template=ProposeServiceTemplate, writable=ProposeWritable (confirm).
6. Images: `huh.NewConfirm("Configure the image registry (enables the deploy
   tag-picker)?")` → input repo_template precargado con ProposeRepoTemplate.
7. `config.AddContext` (error → re-preguntar nombre si es duplicado).
8. Loop; al salir: select de default_context entre los nombres; select de ubicación
   (Global `config.GlobalPath()` [default] / This repo `./steer.toml`) — en modo add
   sobre config existente, la ubicación es la del archivo original (`config.Find()`),
   sin pregunta.
9. El CALLER escribe (Write) — Run no toca disco (testeable); el caller también corre
   el smoke test del default (`det.SmokeTest`) e imprime:
   `✓ connected — N services in <cluster>. Try: steer tui` o el error amable
   (la config ya quedó escrita; se dice explícitamente).

- [ ] **Step 5: CLI — init/add reescritos + tests**

`config_cmd.go`:
- `newConfigInitCmd`: flag `--example` (comportamiento actual EXACTO con
  `exampleConfig`). Sin flag: si `config.Find()` NO encuentra config → wizard desde
  cero. Si encuentra → mostrar resumen (`path`, contextos) y `huh.NewSelect`:
  "Add a context" / "Recreate from scratch" (con `huh.NewConfirm` destructivo) /
  "Cancel".
- `newConfigAddCmd`: requiere config existente (error que enseña si no:
  "no steer.toml found — try: steer config init"); wizard en modo add.
- Registrar `add` en `NewConfigCmd` (junto a `remove` y `list` de T5 — aquí solo add).

Tests en `config_cmd_test.go` (los flujos huh no se testean; sí los no-interactivos):

```go
func TestConfigInitExampleKeepsLegacyBehavior(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	out, err := runRootCtx(t, "config", "init", "--example")
	require.NoError(t, err)
	require.Contains(t, out, "created steer.toml")
	raw, _ := os.ReadFile(filepath.Join(dir, "steer.toml"))
	require.Contains(t, string(raw), "[contexts.dev]")
	// segunda vez falla como siempre
	_, err = runRootCtx(t, "config", "init", "--example")
	require.ErrorContains(t, err, "already exists")
}

func TestConfigAddWithoutConfigTeaches(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("HOME", t.TempDir()) // sin global tampoco
	_, err := runRootCtx(t, "config", "add")
	require.ErrorContains(t, err, "steer config init")
}
```

(Adaptar `runRootCtx` al helper real del archivo; `t.Chdir` requiere Go 1.24+ — ya se
usa 1.26. Si `runRootCtx` no aísla HOME, usar `t.Setenv("HOME", ...)` como arriba.)

- [ ] **Step 6: Verificar + gates + commit**

```bash
gofmt -w internal/ && golangci-lint run && go vet ./... && go test ./... -count=1 && go build ./...
git add internal/cli/
git commit -m "feat(cli): wizard de onboarding en config init/add con detección en vivo"
```

---

### Task 5: `config remove`/`list`, hint del TUI sin config y docs de cierre

**Files:**
- Modify: `internal/cli/config_cmd.go` (remove + list)
- Modify: `internal/cli/config_cmd_test.go`
- Modify: `internal/cli/tui_cmd.go` (sin config → mensaje que apunta al wizard)
- Modify: `docs/parity.md` (nota 2 actualizada), `docs/superpowers/plans/2026-06-15-roadmap.md` (08 ✅), `README.md` (Quick start: paso 1 = steer config init)

**Interfaces:**
- Consumes: `config.RemoveContext/Write/Find/Load` (T1), `confirm` helper (service_cmd.go).
- Produces: `steer config remove <name>`, `steer config list`.

- [ ] **Step 1: Tests que fallan**

```go
func writeTestConfig(t *testing.T, dir string) string {
	path := filepath.Join(dir, "steer.toml")
	require.NoError(t, os.WriteFile(path, []byte(`default_context = "dev"

[contexts.dev]
cloud = "aws"
profile = "dev"
cluster = "dev-cluster"
writable = true

[contexts.prod]
cloud = "aws"
profile = "prod"
cluster = "prod-cluster"
writable = false
`), 0o600))
	return path
}

func TestConfigListShowsContexts(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	writeTestConfig(t, dir)
	out, err := runRootCtx(t, "config", "list")
	require.NoError(t, err)
	require.Contains(t, out, "dev")
	require.Contains(t, out, "prod")
	require.Contains(t, out, "default") // marca cuál es el default
	require.Contains(t, out, "read-only")
}

func TestConfigRemoveDeletesAndReassigns(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	path := writeTestConfig(t, dir)
	out, err := runRootCtx(t, "config", "remove", "dev", "-y")
	require.NoError(t, err)
	require.Contains(t, out, "removed")
	require.Contains(t, out, "default_context is now") // avisa la reasignación
	cfg, err := config.Load(path)
	require.NoError(t, err)
	require.Equal(t, "prod", cfg.DefaultContext)
	_, err = runRootCtx(t, "config", "remove", "nope", "-y")
	require.ErrorContains(t, err, "not found")
}
```

(`remove` gana `-y/--yes` para saltarse la confirmación — mismo patrón de scale/deploy;
sin `-y` usa el helper `confirm` existente.)

- [ ] **Step 2: Verificar que fallan**

Run: `go test ./internal/cli/ -run TestConfigList\|TestConfigRemove -v`
Expected: FAIL — unknown command

- [ ] **Step 3: Implementar**

- `newConfigListCmd`: carga config (`Find`+`Load`), tabla `render.Table` con
  `NAME · CLOUD · CLUSTER · MODE · DEFAULT` (MODE = "writable"/"read-only"; DEFAULT =
  "●" en la fila del default).
- `newConfigRemoveCmd`: args exactos 1; confirmación (`Are you sure? This removes
  "<name>" from <path> [y/N]`) salvo `-y`; `cfg.RemoveContext` + `cfg.Write(path)`;
  si `wasDefault`, imprimir `default_context is now "<nuevo>"` (o aviso si quedó vacía).
- `tui_cmd.go`: donde hoy falla al no encontrar config, envolver:
  `no steer.toml found — try: steer config init (interactive setup)`.
- Docs: parity.md nota 2 → añadir "el TUI sin config apunta al wizard (`steer config
  init`)"; roadmap 08 → ✅ (tabla + Estado, próximo recomendado pasa a 03b logs/events
  o 04 db); README Quick start paso 1: `steer config init   # interactive setup — detects
  your AWS profiles` (reemplaza el cp del example), dejando `--example` mencionado para
  el camino manual.

- [ ] **Step 4: Verificar + gates + commit**

```bash
gofmt -w internal/ && golangci-lint run && go vet ./... && go test ./... -count=1 && go build ./...
git add internal/cli/ docs/ README.md
git commit -m "feat(cli): config remove/list, hint del TUI sin config y docs del hito 08"
```
