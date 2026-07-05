# Capacidad IMAGES (registry) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** La sección IMAGES del TUI muestra repos y tags reales del registry del contexto (ECR en AWS), con marcador del tag desplegado, tag-picker en el formulario de deploy y comandos `steer image ls`/`steer image tags`.

**Architecture:** Nueva interfaz agnóstica `core.Registry` (repos + tags, solo lectura) implementada por un `ECRRegistry` sobre la sesión cacheada del Provider bundle (`Provider.Registry()`, la costura comentada en factory.go). Config anidada por capacidad: `[contexts.<n>.images]` con `repo_template`; el vínculo repo↔servicio sale del `{name}` corto compartido. El TUI reusa el sidebar escalable (nueva `entryRepo`) y el formulario inline (lista de tags filtrable bajo el input).

**Tech Stack:** Go, aws-sdk-go-v2 (nuevo módulo `service/ecr`), Bubble Tea, Cobra, testify.
Spec: `docs/superpowers/specs/2026-07-05-images-registry-design.md`.

## Global Constraints

- Comentarios en español; strings de UI en inglés.
- PROHIBIDO cualquier atribución a Claude/IA en commits, comentarios o PRs.
- Branch de trabajo: `feat/images-registry` (creada en Task 1 desde main).
- Antes de CADA commit: `gofmt -w internal/ && golangci-lint run && go vet ./... && go test ./... -count=1 && go build ./...` — todo verde.
- Repos ordenados **alfanuméricamente** (case-insensitive, por nombre de display); tags por **PushedAt descendente** (empate: tag ascendente); tope `maxTags = 50`.
- `ListTags` devuelve SOLO imágenes de contenedor con tag desplegables: fuera manifiestos sin tag, attestations/SBOM y firmas cosign (tags con sufijo `.sig`/`.att`).
- `core.ErrNoImagesConfig` = contexto sin bloque `[images]`: hint en TUI, mensaje accionable en CLI, NUNCA un error rojo.
- El registry no bloquea services: sus errores se muestran en su sección/panel; el deploy degrada a input libre.
- Tests de TUI anclados al render (X/Y derivados de `View()` real, patrón `findInView`/`stripANSI`); los tests de click existentes pasan sin modificar.
- `time.Now()` solo en producción; los helpers de formato reciben `now` como parámetro para testear determinista.

---

### Task 1: Contratos — core.Registry, FakeRegistry, config anidada, helpers de formato

**Files:**
- Modify: `internal/core/core.go`
- Create: `internal/core/coretest/fake_registry.go`
- Modify: `internal/config/context.go`
- Modify: `internal/config/context_test.go`
- Create: `internal/render/human.go`
- Create: `internal/render/human_test.go`
- Modify: `steer.example.toml` (migrar del esquema legacy al de contexts + bloque images)

**Interfaces:**
- Consumes: nada nuevo.
- Produces (todo lo demás depende de esto):
  - `core.Repository{Name string}`, `core.ImageTag{Tag, Digest string; SizeBytes int64; PushedAt time.Time}`
  - `core.Registry interface { ListRepositories(ctx) ([]Repository, error); ListTags(ctx, repo string) ([]ImageTag, error) }`
  - `core.ErrNoImagesConfig` (sentinel)
  - `coretest.FakeRegistry{Repos, Tags map[string][]core.ImageTag, ReposErr, TagsErr, ListTagsCalls}`
  - `config.ImagesConfig{RepoTemplate string}`; `Context.Images *ImagesConfig`; `Context.RepoName(short) string`; `Context.RepoPrefix() string`
  - `render.Age(t, now time.Time) string`; `render.Size(b int64) string`; `render.ShortDigest(d string) string`

- [ ] **Step 1: Crear la branch**

```bash
git checkout main && git pull && git checkout -b feat/images-registry
```

- [ ] **Step 2: Tests que fallan — config y helpers**

Añadir a `internal/config/context_test.go`:

```go
func TestRepoNameAndPrefix(t *testing.T) {
	c := Context{Images: &ImagesConfig{RepoTemplate: "nao-v2-{name}"}}
	require.Equal(t, "nao-v2-api", c.RepoName("api"))
	require.Equal(t, "nao-v2-", c.RepoPrefix())
	// sin bloque images: nombre tal cual, prefijo vacío
	var plain Context
	require.Equal(t, "api", plain.RepoName("api"))
	require.Equal(t, "", plain.RepoPrefix())
}

func TestValidateImagesBlock(t *testing.T) {
	base := Context{Name: "dev", Cloud: "aws", Profile: "p", Cluster: "c"}
	ok := base
	ok.Images = &ImagesConfig{RepoTemplate: "x-{name}"}
	require.NoError(t, ok.Validate())
	bad := base
	bad.Images = &ImagesConfig{RepoTemplate: "sin-placeholder"}
	require.ErrorContains(t, bad.Validate(), "{name}")
	empty := base
	empty.Images = &ImagesConfig{}
	require.ErrorContains(t, empty.Validate(), "repo_template")
}
```

Crear `internal/render/human_test.go`:

```go
package render

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAge(t *testing.T) {
	now := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	require.Equal(t, "just now", Age(now.Add(-30*time.Second), now))
	require.Equal(t, "5m ago", Age(now.Add(-5*time.Minute), now))
	require.Equal(t, "2h ago", Age(now.Add(-2*time.Hour), now))
	require.Equal(t, "3d ago", Age(now.Add(-72*time.Hour), now))
}

func TestSize(t *testing.T) {
	require.Equal(t, "142 MB", Size(142*1024*1024))
	require.Equal(t, "1.5 GB", Size(1536*1024*1024))
	require.Equal(t, "0 MB", Size(1024))
}

func TestShortDigest(t *testing.T) {
	require.Equal(t, "abcdef123456", ShortDigest("sha256:abcdef123456789..."))
	require.Equal(t, "corto", ShortDigest("corto"))
}
```

- [ ] **Step 3: Verificar que fallan**

Run: `go test ./internal/config/ ./internal/render/ -run 'TestRepoName|TestValidateImages|TestAge|TestSize|TestShortDigest' -v`
Expected: FAIL con "undefined: ImagesConfig" / "undefined: Age"

- [ ] **Step 4: Implementar contratos**

En `internal/core/core.go` (añadir `"errors"` al import):

```go
// ErrNoImagesConfig indica que el contexto no tiene bloque [images]; la
// capacidad está deshabilitada, no es un fallo del cloud.
var ErrNoImagesConfig = errors.New("images not configured for this context")

// Repository es un repositorio de imágenes del registry del contexto.
type Repository struct {
	Name string // nombre real (con prefijo); la UI lo acorta con RepoPrefix
}

// ImageTag es una imagen de contenedor etiquetada y desplegable.
type ImageTag struct {
	Tag       string
	Digest    string // completo; la UI lo acorta con render.ShortDigest
	SizeBytes int64
	PushedAt  time.Time
}

// Registry lista repositorios e imágenes del registry del contexto
// (ECR / Artifact Registry / ACR). Solo lectura.
type Registry interface {
	// ListRepositories devuelve los repos del prefijo del contexto, alfanuméricos.
	ListRepositories(ctx context.Context) ([]Repository, error)
	// ListTags devuelve solo imágenes con tag desplegables (sin manifiestos
	// colgantes, attestations ni firmas), más recientes primero, tope 50.
	ListTags(ctx context.Context, repo string) ([]ImageTag, error)
}
```

Crear `internal/core/coretest/fake_registry.go`:

```go
package coretest

import (
	"context"

	"github.com/juanMaAV92/steer/internal/core"
)

// FakeRegistry es un Registry en memoria para tests.
type FakeRegistry struct {
	Repos    []core.Repository
	Tags     map[string][]core.ImageTag
	ReposErr error
	TagsErr  error

	ListTagsCalls []string // repos consultados, en orden
}

func (f *FakeRegistry) ListRepositories(_ context.Context) ([]core.Repository, error) {
	return f.Repos, f.ReposErr
}

func (f *FakeRegistry) ListTags(_ context.Context, repo string) ([]core.ImageTag, error) {
	f.ListTagsCalls = append(f.ListTagsCalls, repo)
	if f.TagsErr != nil {
		return nil, f.TagsErr
	}
	return f.Tags[repo], nil
}
```

En `internal/config/context.go`:

```go
// ImagesConfig es el bloque [contexts.<n>.images]: la capacidad de registry.
type ImagesConfig struct {
	RepoTemplate string `toml:"repo_template"`
}
```

Añadir el campo al struct `Context` (tras `ServiceTemplate`):

```go
	Images *ImagesConfig `toml:"images"` // capacidad registry (opcional)
```

Helpers espejo de ServiceName/Prefix:

```go
// RepoName resuelve un nombre corto al repo real vía images.repo_template.
// Sin bloque images (o sin template), devuelve el nombre tal cual.
func (c Context) RepoName(short string) string {
	if c.Images == nil || c.Images.RepoTemplate == "" {
		return short
	}
	return strings.ReplaceAll(c.Images.RepoTemplate, "{name}", short)
}

// RepoPrefix es el prefijo de repos a ocultar en la lista (= RepoName("")).
func (c Context) RepoPrefix() string {
	if c.Images == nil {
		return ""
	}
	return c.RepoName("")
}
```

En `Validate()` (antes del `return nil`):

```go
	if c.Images != nil {
		if c.Images.RepoTemplate == "" {
			return fmt.Errorf("context %q: images block needs repo_template", c.Name)
		}
		if !strings.Contains(c.Images.RepoTemplate, "{name}") {
			return fmt.Errorf("context %q: images.repo_template must contain {name}", c.Name)
		}
	}
```

Crear `internal/render/human.go`:

```go
package render

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Age formatea la antigüedad de t respecto a now ("2h ago", "3d ago").
func Age(t, now time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + "m ago"
	case d < 24*time.Hour:
		return strconv.Itoa(int(d.Hours())) + "h ago"
	default:
		return strconv.Itoa(int(d.Hours()/24)) + "d ago"
	}
}

// Size formatea bytes como MB/GB legibles (registro de imágenes: MB es la unidad natural).
func Size(b int64) string {
	const mb = 1024 * 1024
	if b >= 1024*mb {
		return fmt.Sprintf("%.1f GB", float64(b)/float64(1024*mb))
	}
	return strconv.FormatInt(b/mb, 10) + " MB"
}

// ShortDigest acorta un digest sha256 a 12 caracteres para mostrar.
func ShortDigest(d string) string {
	d = strings.TrimPrefix(d, "sha256:")
	if len(d) > 12 {
		return d[:12]
	}
	return d
}
```

Reescribir `steer.example.toml` (el actual documenta el esquema legacy `[providers.*]` que `Validate()` ya rechaza):

```toml
# Steer configuration — copy to `steer.toml` (gitignored) and fill in your values.
# Steer looks for `steer.toml` in the current repo or ~/.config/steer/steer.toml.

default_context = "dev"

[contexts.dev]
cloud            = "aws"
profile          = "dev"
cluster          = "myteam-dev"
service_template = "myteam-dev-{name}"
writable         = true

  # Optional: enables the IMAGES section and the deploy tag-picker.
  [contexts.dev.images]
  repo_template = "myteam-{name}"

[contexts.prod]
cloud            = "aws"
profile          = "prod"
role_arn         = "arn:aws:iam::000000000000:role/your-deployer-role"
cluster          = "myteam-prod"
service_template = "myteam-prod-{name}"
writable         = false   # read-only: blocks mutating commands in prod
```

- [ ] **Step 5: Verificar que pasan + gates + commit**

Run: `go test ./internal/... -count=1` → PASS.

```bash
gofmt -w internal/ && golangci-lint run && go vet ./... && go test ./... -count=1 && go build ./...
git add internal/core/ internal/config/ internal/render/ steer.example.toml
git commit -m "feat(core,config): contrato Registry, config anidada images y helpers de formato"
```

---

### Task 2: Provider — ECRRegistry y la costura Provider.Registry()

**Files:**
- Create: `internal/providers/aws/registry.go`
- Create: `internal/providers/aws/registry_test.go`
- Modify: `internal/providers/aws/provider.go`
- Modify: `internal/providers/factory.go:22-25` (interface)
- Modify: `internal/cli/service_cmd_test.go:17-20` (fakeProvider gana Registry)
- Modify: `internal/tui/app_test.go` (el fake provider del TUI gana Registry)

**Interfaces:**
- Consumes: `core.Registry`, `core.ErrNoImagesConfig`, `config.Context.RepoPrefix()` (Task 1); `LoadConfigForContext`/sesión cacheada (`aws/provider.go`).
- Produces: `providers.Provider` gana `Registry() (core.Registry, error)`; `aws.NewRegistry(cfg awssdk.Config, prefix string) *ECRRegistry`; los fakes de tests exponen `Reg core.Registry` (nil → `ErrNoImagesConfig`).

- [ ] **Step 1: Dependencia ECR**

```bash
go get github.com/aws/aws-sdk-go-v2/service/ecr && go mod tidy
```

- [ ] **Step 2: Tests que fallan — filtro solo-imágenes, orden y scoping**

Crear `internal/providers/aws/registry_test.go`:

```go
package aws

import (
	"context"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"
	"github.com/stretchr/testify/require"
)

// fakeECR devuelve páginas fijadas de repos e imágenes.
type fakeECR struct {
	repos  []ecrtypes.Repository
	images []ecrtypes.ImageDetail
}

func (f *fakeECR) DescribeRepositories(_ context.Context, _ *ecr.DescribeRepositoriesInput, _ ...func(*ecr.Options)) (*ecr.DescribeRepositoriesOutput, error) {
	return &ecr.DescribeRepositoriesOutput{Repositories: f.repos}, nil
}

func (f *fakeECR) DescribeImages(_ context.Context, _ *ecr.DescribeImagesInput, _ ...func(*ecr.Options)) (*ecr.DescribeImagesOutput, error) {
	return &ecr.DescribeImagesOutput{ImageDetails: f.images}, nil
}

func TestListRepositoriesFiltersPrefixAndSorts(t *testing.T) {
	api := &fakeECR{repos: []ecrtypes.Repository{
		{RepositoryName: awssdk.String("nao-v2-worker")},
		{RepositoryName: awssdk.String("otro-equipo-api")},
		{RepositoryName: awssdk.String("nao-v2-api")},
	}}
	r := newRegistry(api, "nao-v2-")
	repos, err := r.ListRepositories(context.Background())
	require.NoError(t, err)
	require.Len(t, repos, 2)
	require.Equal(t, "nao-v2-api", repos[0].Name) // alfanumérico
	require.Equal(t, "nao-v2-worker", repos[1].Name)
}

func TestListTagsOnlyDeployableImages(t *testing.T) {
	now := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	img := func(tags []string, artifact string, pushed time.Time) ecrtypes.ImageDetail {
		d := ecrtypes.ImageDetail{
			ImageTags:        tags,
			ImageDigest:      awssdk.String("sha256:aaaa"),
			ImageSizeInBytes: awssdk.Int64(100 * 1024 * 1024),
			ImagePushedAt:    awssdk.Time(pushed),
		}
		if artifact != "" {
			d.ArtifactMediaType = awssdk.String(artifact)
		}
		return d
	}
	api := &fakeECR{images: []ecrtypes.ImageDetail{
		img([]string{"v1"}, "application/vnd.docker.container.image.v1+json", now.Add(-2*time.Hour)),
		img(nil, "", now),                                     // sin tag: fuera
		img([]string{"sha256-abc.sig"}, "", now),              // firma cosign: fuera
		img([]string{"v2"}, "application/vnd.in-toto+json", now), // attestation: fuera
		img([]string{"v3", "latest"}, "application/vnd.oci.image.config.v1+json", now.Add(-time.Hour)),
	}}
	r := newRegistry(api, "")
	tags, err := r.ListTags(context.Background(), "nao-v2-api")
	require.NoError(t, err)
	// v3 y latest (misma imagen, 1h) antes que v1 (2h); nada más
	require.Len(t, tags, 3)
	require.Equal(t, "latest", tags[0].Tag) // empate por fecha → tag ascendente
	require.Equal(t, "v3", tags[1].Tag)
	require.Equal(t, "v1", tags[2].Tag)
	require.Equal(t, int64(100*1024*1024), tags[0].SizeBytes)
}
```

- [ ] **Step 3: Verificar que fallan**

Run: `go test ./internal/providers/aws/ -run 'TestListRepositories|TestListTags' -v`
Expected: FAIL con "undefined: newRegistry"

- [ ] **Step 4: Implementar `internal/providers/aws/registry.go`**

```go
package aws

import (
	"context"
	"sort"
	"strings"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/juanMaAV92/steer/internal/core"
)

// maxTags limita cuántas imágenes devuelve ListTags (las más recientes).
const maxTags = 50

// ecrAPI es el subconjunto del cliente ECR que usa el registry.
// El *ecr.Client del SDK lo satisface; los tests inyectan un fake.
type ecrAPI interface {
	DescribeRepositories(ctx context.Context, in *ecr.DescribeRepositoriesInput, optFns ...func(*ecr.Options)) (*ecr.DescribeRepositoriesOutput, error)
	DescribeImages(ctx context.Context, in *ecr.DescribeImagesInput, optFns ...func(*ecr.Options)) (*ecr.DescribeImagesOutput, error)
}

// ECRRegistry implementa core.Registry sobre AWS ECR.
type ECRRegistry struct {
	api    ecrAPI
	prefix string // prefijo de repos del contexto (config.Context.RepoPrefix)
}

// NewRegistry crea un ECRRegistry desde una aws.Config.
func NewRegistry(cfg awssdk.Config, prefix string) *ECRRegistry {
	return newRegistry(ecr.NewFromConfig(cfg), prefix)
}

// newRegistry es el constructor inyectable usado por los tests.
func newRegistry(api ecrAPI, prefix string) *ECRRegistry {
	return &ECRRegistry{api: api, prefix: prefix}
}

// ListRepositories devuelve los repos que casan con el prefijo, alfanuméricos.
func (r *ECRRegistry) ListRepositories(ctx context.Context) ([]core.Repository, error) {
	var out []core.Repository
	var token *string
	for {
		resp, err := r.api.DescribeRepositories(ctx, &ecr.DescribeRepositoriesInput{NextToken: token})
		if err != nil {
			return nil, err
		}
		for _, repo := range resp.Repositories {
			name := awssdk.ToString(repo.RepositoryName)
			if r.prefix == "" || strings.HasPrefix(name, r.prefix) {
				out = append(out, core.Repository{Name: name})
			}
		}
		if awssdk.ToString(resp.NextToken) == "" {
			break
		}
		token = resp.NextToken
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// deployableArtifact acepta solo media types de imagen de contenedor real
// (vacío = manifiestos docker clásicos sin artifact type).
func deployableArtifact(mediaType string) bool {
	switch mediaType {
	case "", "application/vnd.docker.container.image.v1+json", "application/vnd.oci.image.config.v1+json":
		return true
	}
	return false
}

// signatureTag detecta tags de firma/attestation por convención cosign.
func signatureTag(tag string) bool {
	return strings.HasSuffix(tag, ".sig") || strings.HasSuffix(tag, ".att")
}

// ListTags devuelve solo imágenes con tag desplegables, más recientes primero.
func (r *ECRRegistry) ListTags(ctx context.Context, repo string) ([]core.ImageTag, error) {
	var out []core.ImageTag
	var token *string
	for {
		resp, err := r.api.DescribeImages(ctx, &ecr.DescribeImagesInput{
			RepositoryName: awssdk.String(repo),
			NextToken:      token,
		})
		if err != nil {
			return nil, err
		}
		for _, img := range resp.ImageDetails {
			if len(img.ImageTags) == 0 || !deployableArtifact(awssdk.ToString(img.ArtifactMediaType)) {
				continue // manifiesto colgante o attestation/SBOM: no es imagen desplegable
			}
			for _, tag := range img.ImageTags {
				if signatureTag(tag) {
					continue
				}
				out = append(out, core.ImageTag{
					Tag:       tag,
					Digest:    awssdk.ToString(img.ImageDigest),
					SizeBytes: awssdk.ToInt64(img.ImageSizeInBytes),
					PushedAt:  awssdk.ToTime(img.ImagePushedAt),
				})
			}
		}
		if awssdk.ToString(resp.NextToken) == "" {
			break
		}
		token = resp.NextToken
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].PushedAt.Equal(out[j].PushedAt) {
			return out[i].PushedAt.After(out[j].PushedAt)
		}
		return out[i].Tag < out[j].Tag
	})
	if len(out) > maxTags {
		out = out[:maxTags]
	}
	return out, nil
}
```

- [ ] **Step 5: Costura del bundle**

En `internal/providers/factory.go`, la interface queda:

```go
// Provider agrupa las capacidades de un contexto; cachea la sesión del cloud.
type Provider interface {
	Deployer() (core.Deployer, error)
	// Registry devuelve la capacidad de imágenes; core.ErrNoImagesConfig si el
	// contexto no tiene bloque [images].
	Registry() (core.Registry, error)
}
```

En `internal/providers/aws/provider.go`, añadir al struct `Provider`:

```go
	regOnce  sync.Once
	registry core.Registry
```

y el método:

```go
// Registry devuelve el Registry ECR del contexto (memoizado).
// Sin bloque [images] en el contexto, la capacidad está deshabilitada.
func (p *Provider) Registry() (core.Registry, error) {
	if p.cfgCtx.Images == nil {
		return nil, core.ErrNoImagesConfig
	}
	p.regOnce.Do(func() { p.registry = NewRegistry(p.cfg, p.cfgCtx.RepoPrefix()) })
	return p.registry, nil
}
```

- [ ] **Step 6: Actualizar los fakes que implementan Provider**

La interface creció: todo fake que implemente `providers.Provider` necesita `Registry()`.

En `internal/cli/service_cmd_test.go` (fakeProvider, línea ~18):

```go
// fakeProvider adapta fakes de core al Provider bundle.
type fakeProvider struct {
	dep core.Deployer
	reg core.Registry // nil → capacidad deshabilitada
}

func (p fakeProvider) Deployer() (core.Deployer, error) { return p.dep, nil }

func (p fakeProvider) Registry() (core.Registry, error) {
	if p.reg == nil {
		return nil, core.ErrNoImagesConfig
	}
	return p.reg, nil
}
```

En `internal/tui/app_test.go`, localizar el fake provider del helper `fakeFactory` (grep `Deployer() (core.Deployer, error)`) y aplicar el mismo cambio: campo `reg core.Registry` + método `Registry()` idéntico. Añadir además un helper `fakeFactoryWithRegistry(dep core.Deployer, reg core.Registry) providers.ProviderFactory` espejo de `fakeFactory` para los tests de Tasks 4-6.

- [ ] **Step 7: Verificar que pasan + gates + commit**

Run: `go test ./... -count=1` → PASS (los tests de cli/tui compilan con el fake extendido).

```bash
gofmt -w internal/ && golangci-lint run && go vet ./... && go test ./... -count=1 && go build ./...
git add go.mod go.sum internal/providers/ internal/cli/service_cmd_test.go internal/tui/app_test.go
git commit -m "feat(providers): ECRRegistry y Provider.Registry() con filtro de solo imágenes"
```

---

### Task 3: CLI — `steer image ls` y `steer image tags`

**Files:**
- Create: `internal/cli/image_cmd.go`
- Create: `internal/cli/image_cmd_test.go`
- Modify: `internal/cli/context.go` (AppContext.Registry)
- Modify: donde se registran los subcomandos (grep `AddCommand` en `cmd/steer/` e `internal/cli/`; registrar `newImageCmd()` junto a service)

**Interfaces:**
- Consumes: `core.Registry`/`ErrNoImagesConfig`/`FakeRegistry` (T1), `Provider.Registry()` y `fakeProvider.reg` (T2), `render.Table`/`Age`/`Size`/`ShortDigest`, `AppContext` (`internal/cli/context.go`).
- Produces: `steer image ls`, `steer image tags -r <short>` (alias `img`).

- [ ] **Step 1: Tests que fallan**

Crear `internal/cli/image_cmd_test.go`:

```go
package cli

import (
	"context"
	"testing"
	"time"

	"github.com/juanMaAV92/steer/internal/config"
	"github.com/juanMaAV92/steer/internal/core"
	"github.com/juanMaAV92/steer/internal/core/coretest"
	"github.com/juanMaAV92/steer/internal/providers"
	"github.com/stretchr/testify/require"
)

func withFakeRegistry(t *testing.T, reg core.Registry) {
	t.Helper()
	prev := newProviderFactoryFn
	newProviderFactoryFn = func() providers.ProviderFactory {
		return func(context.Context, config.Context) (providers.Provider, error) {
			return fakeProvider{dep: &coretest.FakeDeployer{CurrentTagValue: "v1"}, reg: reg}, nil
		}
	}
	t.Cleanup(func() { newProviderFactoryFn = prev })
}

func sampleTags() []core.ImageTag {
	base := time.Now().Add(-2 * time.Hour)
	return []core.ImageTag{
		{Tag: "v2", Digest: "sha256:bbbb222222222", SizeBytes: 100 * 1024 * 1024, PushedAt: base.Add(time.Hour)},
		{Tag: "v1", Digest: "sha256:aaaa111111111", SizeBytes: 90 * 1024 * 1024, PushedAt: base},
	}
}

func TestImageLsListsRepos(t *testing.T) {
	withFakeRegistry(t, &coretest.FakeRegistry{
		Repos: []core.Repository{{Name: "nao-api"}, {Name: "nao-worker"}},
		Tags:  map[string][]core.ImageTag{"nao-api": sampleTags()},
	})
	out, err := runRoot(t, "image", "ls")
	require.NoError(t, err)
	require.Contains(t, out, "REPO")
	require.Contains(t, out, "nao-api")
	require.Contains(t, out, "nao-worker")
	require.Contains(t, out, "v2") // último tag del repo
}

func TestImageTagsListsTagsWithDeployedMarker(t *testing.T) {
	reg := &coretest.FakeRegistry{Tags: map[string][]core.ImageTag{"api": sampleTags()}}
	withFakeRegistry(t, reg)
	out, err := runRoot(t, "image", "tags", "-r", "api")
	require.NoError(t, err)
	require.Contains(t, out, "TAG")
	require.Contains(t, out, "v2")
	require.Contains(t, out, "bbbb22222222") // digest corto
	require.Contains(t, out, "● now")        // v1 == CurrentTagValue del fake deployer
	require.Equal(t, []string{"api"}, reg.ListTagsCalls)
}

func TestImageTagsRequiresRepo(t *testing.T) {
	withFakeRegistry(t, &coretest.FakeRegistry{})
	_, err := runRoot(t, "image", "tags")
	require.ErrorContains(t, err, "--repo")
}

func TestImageWithoutConfigShowsHint(t *testing.T) {
	withFakeRegistry(t, nil) // fakeProvider.reg nil → ErrNoImagesConfig
	_, err := runRoot(t, "image", "ls")
	require.ErrorContains(t, err, "repo_template")
}
```

(Nota: el steer.toml de los tests lo fabrica `runRoot`; los nombres de repos llegan tal
cual del fake — el CLI muestra el nombre corto con `RepoPrefix` del contexto de test, que
es vacío, así que se ven completos. El test de marker usa `CurrentTagValue: "v1"`.)

- [ ] **Step 2: Verificar que fallan**

Run: `go test ./internal/cli/ -run TestImage -v`
Expected: FAIL — "unknown command \"image\""

- [ ] **Step 3: Implementar**

En `internal/cli/context.go`, añadir:

```go
// Registry construye (una vez) el provider y devuelve su Registry.
// core.ErrNoImagesConfig se traduce a un mensaje accionable con el snippet TOML.
func (a *AppContext) Registry(ctx context.Context) (core.Registry, error) {
	if a.provider == nil {
		p, err := a.Factory(ctx, a.Ctx)
		if err != nil {
			return nil, err
		}
		a.provider = p
	}
	reg, err := a.provider.Registry()
	if errors.Is(err, core.ErrNoImagesConfig) {
		return nil, fmt.Errorf("context %q has no images config; add to steer.toml:\n\n  [contexts.%s.images]\n  repo_template = \"myteam-{name}\"", a.Ctx.Name, a.Ctx.Name)
	}
	return reg, err
}
```

Crear `internal/cli/image_cmd.go`:

```go
package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/juanMaAV92/steer/internal/render"
	"github.com/spf13/cobra"
)

// newImageCmd agrupa los subcomandos de la capacidad de imágenes (registry).
func newImageCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "image",
		Aliases: []string{"img"},
		Short:   "Browse container images in the context registry",
	}
	cmd.AddCommand(newImageLsCmd(), newImageTagsCmd())
	return cmd
}

func newImageLsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "List repositories with their latest tag",
		RunE: func(cmd *cobra.Command, _ []string) error {
			app := FromContext(cmd.Context())
			reg, err := app.Registry(cmd.Context())
			if err != nil {
				return err
			}
			repos, err := reg.ListRepositories(cmd.Context())
			if err != nil {
				return err
			}
			now := time.Now()
			rows := make([][]string, 0, len(repos))
			for _, r := range repos {
				latest, pushed := "—", "—"
				if tags, err := reg.ListTags(cmd.Context(), r.Name); err == nil && len(tags) > 0 {
					latest = tags[0].Tag
					pushed = render.Age(tags[0].PushedAt, now)
				}
				short := strings.TrimPrefix(r.Name, app.Ctx.RepoPrefix())
				rows = append(rows, []string{short, latest, pushed})
			}
			fmt.Fprintln(cmd.OutOrStdout(), render.Table([]string{"REPO", "LATEST TAG", "PUSHED"}, rows))
			return nil
		},
	}
}

func newImageTagsCmd() *cobra.Command {
	var repo string
	cmd := &cobra.Command{
		Use:   "tags",
		Short: "List deployable image tags of a repository",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if repo == "" {
				return fmt.Errorf("--repo is required (short name, e.g. -r api)")
			}
			app := FromContext(cmd.Context())
			reg, err := app.Registry(cmd.Context())
			if err != nil {
				return err
			}
			tags, err := reg.ListTags(cmd.Context(), app.Ctx.RepoName(repo))
			if err != nil {
				return err
			}
			// tag desplegado del servicio hermano ({name} compartido); ausente = sin marca
			deployed := ""
			if dep, err := app.Deployer(cmd.Context()); err == nil {
				if cur, err := dep.CurrentTag(cmd.Context(), app.Ctx.ServiceName(repo)); err == nil {
					deployed = cur
				}
			}
			now := time.Now()
			rows := make([][]string, 0, len(tags))
			for _, t := range tags {
				mark := ""
				if deployed != "" && t.Tag == deployed {
					mark = "● now"
				}
				rows = append(rows, []string{t.Tag, render.Age(t.PushedAt, now),
					render.Size(t.SizeBytes), render.ShortDigest(t.Digest), mark})
			}
			fmt.Fprintln(cmd.OutOrStdout(), render.Table([]string{"TAG", "AGE", "SIZE", "DIGEST", "DEPLOYED"}, rows))
			return nil
		},
	}
	cmd.Flags().StringVarP(&repo, "repo", "r", "", "repository short name")
	return cmd
}
```

Registrarlo donde se registran los demás (grep `AddCommand` — el sitio que añade
`newServiceCmd()`): `root.AddCommand(newImageCmd())` con el mismo patrón.

- [ ] **Step 4: Verificar que pasan + gates + commit**

Run: `go test ./internal/cli/ -run TestImage -v` → PASS; luego suite completa.

```bash
gofmt -w internal/ && golangci-lint run && go vet ./... && go test ./... -count=1 && go build ./...
git add internal/cli/
git commit -m "feat(cli): steer image ls/tags con marcador de tag desplegado"
```

---

### Task 4: TUI — repos reales en la sección IMAGES del sidebar

**Files:**
- Modify: `internal/tui/sidebar.go`
- Modify: `internal/tui/sidebar_test.go`
- Modify: `internal/tui/messages.go` (reposMsg)
- Modify: `internal/tui/app.go` (loadReposCmd, Init, applyContextSwitch, Refresh, servicesMsg case intacto)
- Modify: `internal/tui/app_test.go`

**Interfaces:**
- Consumes: `core.Repository`, `Provider.Registry()`, `fakeFactoryWithRegistry` (T2).
- Produces (T5/T6 dependen): `sidebar.setRepos([]core.Repository)`, `sidebar.imagesState` (`imagesDisabled|imagesLoading|imagesReady|imagesError`), `sidebar.imagesErr string`, `entryRepo` (nuevo entryKind, Index sobre `visibleRepos()`), `sidebar.lastSelected sidebarSection`, `sidebar.selectedRepo() (string, bool)`, `sidebar.repoPrefix string`; `Model.loadReposCmd() tea.Cmd`; `reposMsg{repos []core.Repository, disabled bool, err error}`.

- [ ] **Step 1: Tests que fallan (sidebar unit)**

Añadir a `internal/tui/sidebar_test.go`:

```go
func sampleRepos() []core.Repository {
	return []core.Repository{{Name: "nao-v2-worker"}, {Name: "nao-v2-api"}}
}

func TestSetReposSortsAndNavigates(t *testing.T) {
	s := newSidebar()
	s.height = 30
	s.repoPrefix = "nao-v2-"
	s.setServices(sampleServices())
	s.setRepos(sampleRepos())
	s.collapsed[sectionImages] = false
	// el primer repo visible es "api" (alfanumérico por nombre de display)
	var repoEntries []sidebarEntry
	for _, e := range s.navEntries() {
		if e.Kind == entryRepo {
			repoEntries = append(repoEntries, e)
		}
	}
	require.Len(t, repoEntries, 2)
	require.Equal(t, "nao-v2-api", s.visibleRepos()[repoEntries[0].Index].Name)
}

func TestRepoSelectionSetsLastSelected(t *testing.T) {
	s := newSidebar()
	s.height = 30
	s.setServices(sampleServices())
	s.setRepos(sampleRepos())
	s.collapsed[sectionImages] = false
	// navegar hasta el primer repo
	for {
		e, ok := s.cursorEntry()
		require.True(t, ok)
		if e.Kind == entryRepo {
			break
		}
		s.moveDown()
	}
	repo, ok := s.selectedRepo()
	require.True(t, ok)
	require.Equal(t, "nao-v2-api", repo)
	require.Equal(t, sectionImages, s.lastSelected)
	// volver a un servicio devuelve lastSelected a services
	for {
		e, ok := s.cursorEntry()
		require.True(t, ok)
		if e.Kind == entryService {
			break
		}
		s.moveUp()
	}
	require.Equal(t, sectionServices, s.lastSelected)
}

func TestFilterAppliesToReposToo(t *testing.T) {
	s := newSidebar()
	s.height = 30
	s.repoPrefix = "nao-v2-"
	s.setServices(sampleServices())
	s.setRepos(sampleRepos())
	s.collapsed[sectionImages] = false
	s.setFilter("work")
	require.Len(t, s.visibleRepos(), 1)
	require.Equal(t, "nao-v2-worker", s.visibleRepos()[0].Name)
}

func TestImagesStatesRender(t *testing.T) {
	s := newSidebar()
	s.width = 40
	s.collapsed[sectionImages] = false
	join := func() string {
		var b strings.Builder
		for _, r := range s.rows(false) {
			b.WriteString(stripANSI(r.Line) + "\n")
		}
		return b.String()
	}
	s.imagesState = imagesDisabled
	require.Contains(t, join(), "configure images in steer.toml")
	s.imagesState = imagesLoading
	require.Contains(t, join(), "loading")
	s.imagesState = imagesError
	s.imagesErr = "boom"
	require.Contains(t, join(), "boom")
	s.imagesState = imagesReady
	require.Contains(t, join(), "no repositories") // ready sin repos
}
```

- [ ] **Step 2: Verificar que fallan**

Run: `go test ./internal/tui/ -run 'TestSetRepos|TestRepoSelection|TestFilterAppliesToRepos|TestImagesStates' -v`
Expected: FAIL con "undefined: entryRepo" / "s.setRepos undefined"

- [ ] **Step 3: Implementar en `internal/tui/sidebar.go`**

3a. Tipos y estado:

```go
const (
	entryHeader entryKind = iota
	entryService
	entryRepo
)

// imagesState refleja el ciclo de vida de la sección IMAGES.
type imagesState int

const (
	imagesDisabled imagesState = iota // sin bloque [images] en el contexto
	imagesLoading
	imagesReady
	imagesError
)
```

Campos nuevos en `sidebar`:

```go
	repos        []core.Repository
	repoPrefix   string // prefijo de repos a ocultar (config RepoPrefix)
	selectedRepoName string
	lastSelected sidebarSection // qué sección alimenta el panel derecho
	imagesState  imagesState
	imagesErr    string
```

(`newSidebar` no cambia: `imagesDisabled` es el cero value y las secciones IMAGES/
DATABASES siguen colapsadas por defecto.)

3b. setRepos + visibleRepos (espejo de services):

```go
// setRepos guarda los repos ordenados alfanuméricamente por nombre de display.
func (s *sidebar) setRepos(repos []core.Repository) {
	sorted := make([]core.Repository, len(repos))
	copy(sorted, repos)
	sort.SliceStable(sorted, func(i, j int) bool {
		di := strings.ToLower(strings.TrimPrefix(sorted[i].Name, s.repoPrefix))
		dj := strings.ToLower(strings.TrimPrefix(sorted[j].Name, s.repoPrefix))
		return di < dj
	})
	s.repos = sorted
	s.imagesState = imagesReady
	// la selección de repo persiste por nombre; si desapareció, se limpia
	if _, ok := s.selectedRepo(); !ok {
		s.selectedRepoName = ""
	}
	if s.cursor >= len(s.navEntries()) {
		s.cursor = max(0, len(s.navEntries())-1)
	}
}

// visibleRepos aplica el mismo filtro substring que los servicios.
func (s sidebar) visibleRepos() []core.Repository {
	if s.filterQuery == "" {
		return s.repos
	}
	q := strings.ToLower(s.filterQuery)
	var out []core.Repository
	for _, r := range s.repos {
		if strings.Contains(strings.ToLower(strings.TrimPrefix(r.Name, s.repoPrefix)), q) {
			out = append(out, r)
		}
	}
	return out
}

// selectedRepo devuelve el repo seleccionado (nombre real) si sigue existiendo.
func (s sidebar) selectedRepo() (string, bool) {
	for _, r := range s.repos {
		if r.Name == s.selectedRepoName {
			return r.Name, true
		}
	}
	return "", false
}
```

3c. `rows()`: reemplazar el bloque IMAGES actual (líneas 191-196) por:

```go
	// IMAGES
	visRepos := s.visibleRepos()
	repoCount := "(" + strconv.Itoa(len(visRepos)) + "/" + strconv.Itoa(len(s.repos)) + ")"
	if s.filterQuery == "" && !s.filterActive {
		repoCount = "(" + strconv.Itoa(len(s.repos)) + ")"
	}
	if s.imagesState != imagesReady {
		repoCount = "···"
	}
	appendHeader(sectionImages, "IMAGES", repoCount)
	if !s.collapsed[sectionImages] {
		switch s.imagesState {
		case imagesDisabled:
			out = append(out, sidebarRow{Line: render.Dim("  configure images in steer.toml")})
		case imagesLoading:
			out = append(out, sidebarRow{Line: render.Dim("  loading…")})
		case imagesError:
			out = append(out, sidebarRow{Line: render.Dim("  registry error: " + s.imagesErr)})
		case imagesReady:
			if len(visRepos) == 0 {
				out = append(out, sidebarRow{Line: render.Dim("  no repositories")})
			}
			for i, r := range visRepos {
				under := nav == s.cursor
				out = append(out, sidebarRow{Line: s.repoRow(r, under),
					Entry: &sidebarEntry{Kind: entryRepo, Section: sectionImages, Index: i}})
				nav++
			}
		}
		appendBlank()
	} else {
		appendBlank()
	}
```

y el render de fila:

```go
// repoRow renderiza una fila de repo; bajo el cursor lleva la barra de selección.
func (s sidebar) repoRow(r core.Repository, underCursor bool) string {
	name := strings.TrimPrefix(r.Name, s.repoPrefix)
	if !underCursor {
		return "  " + render.Dim("▣") + " " + name
	}
	return s.barLine("▣ " + name)
}
```

3d. Selección: en `moveCursor` y `selectEntry`, ampliar el manejo de kinds:

```go
	switch e := nav[s.cursor]; e.Kind {
	case entryService:
		s.selectedName = s.visibleServices()[e.Index].Name
		s.lastSelected = sectionServices
	case entryRepo:
		s.selectedRepoName = s.visibleRepos()[e.Index].Name
		s.lastSelected = sectionImages
	}
```

(en `selectEntry` es el mismo switch sobre la entrada encontrada). En `setServices`,
cuando fija la selección inicial de servicio, dejar `s.lastSelected = sectionServices`
solo si no hay ya un repo seleccionado con `lastSelected == sectionImages`.
En `setFilter`, el resync de cursor existente sigue anclado a la selección de servicio;
tras él, si `lastSelected == sectionImages`, no tocar (el cursor de repos se resuelve
por clamp).

- [ ] **Step 4: Cablear la carga en `internal/tui/app.go` + messages.go**

En `messages.go`:

```go
type reposMsg struct {
	repos    []core.Repository
	disabled bool // contexto sin bloque [images]
	err      error
}
```

En `app.go`:

```go
// loadReposCmd pide los repos del registry; los repos no se refrescan por tick
// (cambian poco): solo al entrar al contexto y con Refresh.
func (m Model) loadReposCmd() tea.Cmd {
	provider := m.provider
	ctx := m.runCtx
	return func() tea.Msg {
		reg, err := provider.Registry()
		if errors.Is(err, core.ErrNoImagesConfig) {
			return reposMsg{disabled: true}
		}
		if err != nil {
			return reposMsg{err: err}
		}
		repos, err := reg.ListRepositories(ctx)
		return reposMsg{repos: repos, err: err}
	}
}
```

Caso en `Update`:

```go
	case reposMsg:
		switch {
		case msg.disabled:
			m.sidebar.imagesState = imagesDisabled
		case msg.err != nil:
			m.sidebar.imagesState = imagesError
			m.sidebar.imagesErr = msg.err.Error()
		default:
			m.sidebar.setRepos(msg.repos)
		}
		return m, nil
```

- `New()`: tras construir el sidebar, `m.sidebar.repoPrefix = current.RepoPrefix()` y,
  si `err == nil`, `m.sidebar.imagesState = imagesLoading` cuando `current.Images != nil`.
- `Init()`: `tea.Batch(m.loadServicesCmd(), m.loadReposCmd(), tickCmd())`.
- `applyContextSwitch`: tras `m.sidebar.prefix = sel.Prefix()`, añadir
  `m.sidebar.repoPrefix = sel.RepoPrefix()`, poner `imagesLoading` si `sel.Images != nil`,
  y devolver `tea.Batch(m.loadServicesCmd(), m.loadReposCmd())`.
- Tecla `Refresh`: devolver `tea.Batch(m.loadServicesCmd(), m.loadReposCmd())`
  (y poner `imagesLoading` si aplica).
- El caso `tickMsg` NO cambia (solo services).

- [ ] **Step 5: Test de integración anclado al render**

Añadir a `internal/tui/app_test.go` (usa `fakeFactoryWithRegistry` de T2):

```go
// TestSidebarShowsReposFromRegistry: los repos llegan por reposMsg y se ven en IMAGES.
func TestSidebarShowsReposFromRegistry(t *testing.T) {
	reg := &coretest.FakeRegistry{Repos: []core.Repository{{Name: "api"}, {Name: "worker"}}}
	m := newTestModelWithRegistry(sampleServices(), reg) // helper: como newTestModel pero con registry y [images] en el contexto
	m = mustUpdate(t, m, reposMsg{repos: reg.Repos})
	m.sidebar.collapsed[sectionImages] = false
	out := stripANSI(m.View())
	require.Contains(t, out, "IMAGES")
	require.Contains(t, out, "api")
	require.Contains(t, out, "worker")
	require.NotContains(t, out, "coming soon")
}
```

`newTestModelWithRegistry` se define junto a `newTestModel` reutilizando su cuerpo, con
`fakeFactoryWithRegistry` y `current.Images = &config.ImagesConfig{RepoTemplate: "{name}"}`.
Los tests de click existentes NO se tocan (IMAGES sigue colapsada por defecto y sin
registry el estado es `imagesDisabled`).

- [ ] **Step 6: Verificar todo + gates + commit**

Run: `go test ./internal/tui/... -count=1` → PASS completo (incluidos los anclados previos).

```bash
gofmt -w internal/ && golangci-lint run && go vet ./... && go test ./... -count=1 && go build ./...
git add internal/tui/
git commit -m "feat(tui): sección IMAGES con repos reales del registry"
```

---

### Task 5: TUI — tabla de tags en el panel al seleccionar un repo

**Files:**
- Create: `internal/tui/panel/tags.go`
- Create: `internal/tui/panel/tags_test.go`
- Modify: `internal/tui/messages.go` (tagsMsg)
- Modify: `internal/tui/app.go` (estado de tags, syncRepoTags, View/panelBody)
- Modify: `internal/tui/app_test.go`

**Interfaces:**
- Consumes: `sidebar.selectedRepo()`, `sidebar.lastSelected` (T4); `render.Age/Size/ShortDigest` (T1).
- Produces (T6 no depende de esto): `panel.TagsView(repo string, tags []core.ImageTag, deployed string, now time.Time) string`; `Model.tagsRepo/tags/tagsLoading/tagsErr`; `tagsMsg{repo string, tags []core.ImageTag, err error}`.

- [ ] **Step 1: Tests que fallan (panel unit)**

Crear `internal/tui/panel/tags_test.go`:

```go
package panel

import (
	"strings"
	"testing"
	"time"

	"github.com/juanMaAV92/steer/internal/core"
	"github.com/stretchr/testify/require"
)

func TestTagsViewRendersRowsWithDeployedMarker(t *testing.T) {
	now := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	tags := []core.ImageTag{
		{Tag: "v2", Digest: "sha256:bbbb222222222", SizeBytes: 100 * 1024 * 1024, PushedAt: now.Add(-time.Hour)},
		{Tag: "v1", Digest: "sha256:aaaa111111111", SizeBytes: 90 * 1024 * 1024, PushedAt: now.Add(-72 * time.Hour)},
	}
	out := TagsView("api", tags, "v1", now)
	require.Contains(t, out, "v2")
	require.Contains(t, out, "1h ago")
	require.Contains(t, out, "100 MB")
	require.Contains(t, out, "bbbb22222222")
	// solo la fila desplegada (v1) lleva el marcador, exactamente una vez
	require.Contains(t, out, "● now")
	require.Equal(t, 1, strings.Count(out, "● now"))
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "● now") {
			require.Contains(t, l, "v1")
		}
	}
}

func TestTagsViewEmptyAndStates(t *testing.T) {
	now := time.Now()
	require.Contains(t, TagsView("api", nil, "", now), "no images yet")
}
```

- [ ] **Step 2: Verificar que fallan**

Run: `go test ./internal/tui/panel/ -run TestTagsView -v`
Expected: FAIL con "undefined: TagsView"

- [ ] **Step 3: Implementar `internal/tui/panel/tags.go`**

```go
package panel

import (
	"strings"
	"time"

	"github.com/juanMaAV92/steer/internal/core"
	"github.com/juanMaAV92/steer/internal/render"
)

// TagsView renderiza la lista de tags de un repo: TAG · AGE · SIZE · DIGEST,
// con "● now" en el tag desplegado por el servicio hermano.
func TagsView(repo string, tags []core.ImageTag, deployed string, now time.Time) string {
	var b strings.Builder
	b.WriteString(render.Bold(repo) + "\n\n")
	if len(tags) == 0 {
		b.WriteString(render.Dim("no images yet"))
		return b.String()
	}
	// ancho de la columna TAG para alinear el resto
	w := 0
	for _, t := range tags {
		if len(t.Tag) > w {
			w = len(t.Tag)
		}
	}
	for _, t := range tags {
		pad := strings.Repeat(" ", w-len(t.Tag)+2)
		line := render.Accent(t.Tag) + pad +
			render.Dim(render.Age(t.PushedAt, now)) + "  " +
			render.Dim(render.Size(t.SizeBytes)) + "  " +
			render.Dim(render.ShortDigest(t.Digest))
		if deployed != "" && t.Tag == deployed {
			line += "  " + render.Success("● now")
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}
```

- [ ] **Step 4: Cablear en el Model**

En `messages.go`:

```go
type tagsMsg struct {
	repo string
	tags []core.ImageTag
	err  error
}
```

En `app.go` — campos del Model:

```go
	tagsRepo    string // repo cuyo listado de tags está cargado/cargando
	tags        []core.ImageTag
	tagsLoading bool
	tagsErr     string
```

Comando + sincronización:

```go
// loadTagsCmd pide los tags de un repo (nombre real).
func (m Model) loadTagsCmd(repo string) tea.Cmd {
	provider := m.provider
	ctx := m.runCtx
	return func() tea.Msg {
		reg, err := provider.Registry()
		if err != nil {
			return tagsMsg{repo: repo, err: err}
		}
		tags, err := reg.ListTags(ctx, repo)
		return tagsMsg{repo: repo, tags: tags, err: err}
	}
}

// syncRepoTags dispara la carga de tags si la selección de repo cambió.
// Llamar tras cualquier mutación del sidebar (teclas y clicks).
func (m *Model) syncRepoTags() tea.Cmd {
	repo, ok := m.sidebar.selectedRepo()
	if !ok || repo == m.tagsRepo {
		return nil
	}
	m.tagsRepo = repo
	m.tags = nil
	m.tagsErr = ""
	m.tagsLoading = true
	return m.loadTagsCmd(repo)
}
```

Caso en `Update`:

```go
	case tagsMsg:
		if msg.repo != m.tagsRepo {
			return m, nil // respuesta obsoleta de un repo ya deseleccionado
		}
		m.tagsLoading = false
		if msg.err != nil {
			m.tagsErr = msg.err.Error()
			return m, nil
		}
		m.tags = msg.tags
		return m, nil
```

Llamar `syncRepoTags` donde muta la selección: al final del branch de sidebar en
`handleKey` (`return m, m.syncRepoTags()` tras moveDown/moveUp/toggle) y en
`handleMouse` tras `m.sidebar.selectEntry(e)` (acumular el cmd y devolverlo).
En `applyContextSwitch`, resetear: `m.tagsRepo, m.tags, m.tagsErr, m.tagsLoading = "", nil, "", false`.

Render — en `View()`, la cabecera del panel depende de qué alimenta el panel:

```go
	header := m.tabs.View()
	if m.sidebar.lastSelected == sectionImages {
		header = render.Brand("TAGS")
	}
	panelBody := header + "\n\n" + m.panelBody()
```

y en `panelBody()`, antes del switch de tabs:

```go
	if m.sidebar.lastSelected == sectionImages {
		repo, ok := m.sidebar.selectedRepo()
		if !ok {
			return render.Dim("no repository selected")
		}
		switch {
		case m.tagsLoading:
			return render.Dim("loading tags…")
		case m.tagsErr != "":
			return render.Danger("registry error: " + m.tagsErr)
		default:
			short := strings.TrimPrefix(repo, m.current.RepoPrefix())
			return panel.TagsView(short, m.tags, m.deployedTagFor(repo), time.Now())
		}
	}
```

con el helper del vínculo repo↔servicio:

```go
// deployedTagFor devuelve el tag que corre en el servicio hermano del repo
// (mismo {name} corto en ambos templates); vacío si no hay hermano.
func (m Model) deployedTagFor(repo string) string {
	short := strings.TrimPrefix(repo, m.current.RepoPrefix())
	for _, s := range m.sidebar.services {
		if strings.TrimPrefix(s.Name, m.current.Prefix()) == short {
			return s.Tag
		}
	}
	return ""
}
```

- [ ] **Step 5: Test de integración anclado al render**

Añadir a `internal/tui/app_test.go`:

```go
// TestSelectRepoShowsTagsPanel: seleccionar un repo carga y muestra su tabla de tags
// con el marcador del tag desplegado por el servicio hermano.
func TestSelectRepoShowsTagsPanel(t *testing.T) {
	reg := &coretest.FakeRegistry{
		Repos: []core.Repository{{Name: "api"}},
		Tags: map[string][]core.ImageTag{"api": {
			{Tag: "v2", Digest: "sha256:bbbb222222222", SizeBytes: 100 * 1024 * 1024, PushedAt: time.Now().Add(-time.Hour)},
			{Tag: "v1.0.0", Digest: "sha256:aaaa111111111", SizeBytes: 90 * 1024 * 1024, PushedAt: time.Now().Add(-48 * time.Hour)},
		}},
	}
	m := newTestModelWithRegistry(sampleServices(), reg) // servicio "api" con Tag "v1.0.0" en sampleServices
	m = mustUpdate(t, m, reposMsg{repos: reg.Repos})
	m.sidebar.collapsed[sectionImages] = false
	// click en el repo (anclado al render)
	clickX, clickY := findInView(t, m.View(), "▣ api")
	updated, cmd := m.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: clickX, Y: clickY})
	m = updated.(Model)
	require.NotNil(t, cmd, "seleccionar repo debe disparar la carga de tags")
	m = mustUpdate(t, m, cmd().(tagsMsg))
	out := stripANSI(m.View())
	require.Contains(t, out, "TAGS")
	require.Contains(t, out, "v2")
	require.Contains(t, out, "● now") // v1.0.0 coincide con el tag del servicio api
}
```

(Si `sampleServices()` no incluye un servicio "api" con Tag "v1.0.0", ajustar el fixture
del test creando los servicios inline — sin tocar `sampleServices()` global.)

- [ ] **Step 6: Verificar todo + gates + commit**

```bash
gofmt -w internal/ && golangci-lint run && go vet ./... && go test ./... -count=1 && go build ./...
git add internal/tui/
git commit -m "feat(tui): panel de tags al seleccionar un repo, con marcador del desplegado"
```

---

### Task 6: TUI — tag-picker en el formulario de deploy

**Files:**
- Modify: `internal/tui/form.go`
- Modify: `internal/tui/form_test.go`
- Modify: `internal/tui/messages.go` (formTagsMsg)
- Modify: `internal/tui/app.go` (openAction/openActionKind disparan la carga; handleFormKey ↑↓; clickForm filas de tags)
- Modify: `internal/tui/app_test.go`

**Interfaces:**
- Consumes: `actionForm` (geometría actual: borde/título/prompt/botones/borde), `clickForm`/`handleFormKey`, `core.ImageTag`, `Registry()`.
- Produces: `actionForm.setTags([]core.ImageTag)`, `visibleTags() []core.ImageTag` (filtradas por input, máx 5), `movePick(delta)`, `tagAt(row) int`, `buttonRow() int` (reemplaza la const `formButtonRow`); `formTagsMsg{service string, tags []core.ImageTag}`.

- [ ] **Step 1: Tests que fallan (form unit)**

Añadir a `internal/tui/form_test.go`:

```go
func pickerTags() []core.ImageTag {
	now := time.Now()
	return []core.ImageTag{
		{Tag: "v1.4.2", PushedAt: now.Add(-2 * time.Hour)},
		{Tag: "v1.4.1", PushedAt: now.Add(-72 * time.Hour)},
		{Tag: "v1.3.9", PushedAt: now.Add(-200 * time.Hour)},
	}
}

func TestFormTagsFilterByInput(t *testing.T) {
	f := newActionForm(actionDeploy, "api")
	f.setTags(pickerTags())
	require.Len(t, f.visibleTags(), 3)
	f.typeKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v1.4")})
	require.Len(t, f.visibleTags(), 2)
	f.typeKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("zzz")})
	require.Empty(t, f.visibleTags())
}

func TestFormMovePickFillsInput(t *testing.T) {
	f := newActionForm(actionDeploy, "api")
	f.setTags(pickerTags())
	f.movePick(1)
	require.Equal(t, "v1.4.2", f.input) // primer tag (más reciente)
	f.movePick(1)
	require.Equal(t, "v1.4.1", f.input)
	f.movePick(-1)
	require.Equal(t, "v1.4.2", f.input)
	// teclear resetea el pick y vuelve a filtrar sobre lo tecleado
	f.typeKey(tea.KeyMsg{Type: tea.KeyBackspace})
	require.Equal(t, -1, f.pick)
}

func TestFormGeometryShiftsWithTags(t *testing.T) {
	f := newActionForm(actionDeploy, "api")
	require.Equal(t, 3, f.buttonRow()) // sin tags: geometría de siempre
	f.setTags(pickerTags())
	require.Equal(t, 3+3, f.buttonRow()) // 3 filas de tags entre prompt y botones
	require.Equal(t, 0, f.tagAt(3))      // primera fila de tag
	require.Equal(t, 2, f.tagAt(5))
	require.Equal(t, -1, f.tagAt(6)) // fila de botones, no tag
	require.Equal(t, 0, f.buttonAt(f.buttonRow(), formContentX0))
	// rollback/scale no muestran picker
	s := newActionForm(actionScale, "api")
	s.setTags(pickerTags())
	require.Equal(t, 3, s.buttonRow())
}
```

- [ ] **Step 2: Verificar que fallan**

Run: `go test ./internal/tui/ -run 'TestFormTags|TestFormMovePick|TestFormGeometry' -v`
Expected: FAIL con "f.setTags undefined"

- [ ] **Step 3: Implementar en `internal/tui/form.go`**

Campos nuevos en `actionForm`:

```go
	tags []core.ImageTag // tags del repo hermano (solo kind deploy); nil = sin picker
	pick int             // índice en visibleTags() rellenado por ↑↓; -1 = tecleando
```

`newActionForm` inicializa `pick: -1`. Métodos:

```go
// setTags habilita el picker (solo tiene efecto visual en deploy).
func (f *actionForm) setTags(tags []core.ImageTag) { f.tags = tags }

// visibleTags filtra los tags por el input actual (substring, máx 5 visibles).
func (f actionForm) visibleTags() []core.ImageTag {
	if f.kind != actionDeploy || len(f.tags) == 0 {
		return nil
	}
	q := strings.ToLower(f.input)
	var out []core.ImageTag
	for _, t := range f.tags {
		if q == "" || strings.Contains(strings.ToLower(t.Tag), q) {
			out = append(out, t)
		}
		if len(out) == 5 {
			break
		}
	}
	return out
}

// movePick mueve la selección del picker y rellena el input con el tag elegido.
func (f *actionForm) movePick(delta int) {
	vis := f.visibleTags()
	if len(vis) == 0 {
		return
	}
	// el primer ↓ entra en la lista; después se desplaza con clamp
	f.pick = min(max(f.pick+delta, 0), len(vis)-1)
	f.input = vis[f.pick].Tag
}

// buttonRow es la fila de los botones dentro del view: la geometría base
// (borde, título, prompt) más las filas visibles del picker.
func (f actionForm) buttonRow() int { return 3 + len(f.visibleTags()) }

// tagAt devuelve el índice del tag en la fila row del view, o -1.
func (f actionForm) tagAt(row int) int {
	n := len(f.visibleTags())
	if n == 0 || row < 3 || row >= 3+n {
		return -1
	}
	return row - 3
}
```

Ajustes a lo existente:
- `typeKey`: al editar (runas o backspace), añadir `f.pick = -1` al final de ambos casos.
- `buttonAt(row, x)`: `if row != f.buttonRow() { return -1 }` (la const `formButtonRow`
  se elimina; `formContentX0` se queda). Actualizar el comentario de geometría del
  archivo: «borde(0), título(1), prompt(2), tags(3..3+n-1), botones(3+n), borde».
- `view()`: entre `prompt` y los botones, insertar las filas del picker:

```go
	rows := []string{render.Bold(title), prompt}
	if vis := f.visibleTags(); len(vis) > 0 {
		now := time.Now()
		for i, t := range vis {
			line := "  " + t.Tag + "  " + render.Age(t.PushedAt, now)
			if i == f.pick {
				line = lipgloss.NewStyle().Background(lipgloss.Color(render.SelectionBarColor)).Render(line)
			} else {
				line = render.Dim(line)
			}
			rows = append(rows, line)
		}
	}
	rows = append(rows, render.ButtonsWithFocus(f.labels(), f.focus))
	inner := strings.Join(rows, "\n")
```

- Los usos de `formButtonRow` en `form_test.go` de T1 se actualizan a `f.buttonRow()`
  (mismo valor 3 sin tags — el test existente `TestFormViewAndButtonGeometry` sigue
  válido cambiando la const por el método).

- [ ] **Step 4: Cablear en `app.go`**

En `messages.go`:

```go
type formTagsMsg struct {
	service string
	tags    []core.ImageTag
}
```

Comando (errores → silencio: el formulario degrada a input libre):

```go
// loadFormTagsCmd pide los tags del repo hermano del servicio para el picker.
// Cualquier error (sin config, cloud caído) degrada en silencio a input libre.
func (m Model) loadFormTagsCmd(service string) tea.Cmd {
	provider := m.provider
	ctx := m.runCtx
	short := strings.TrimPrefix(service, m.current.Prefix())
	repo := m.current.RepoName(short)
	return func() tea.Msg {
		reg, err := provider.Registry()
		if err != nil {
			return formTagsMsg{service: service}
		}
		tags, err := reg.ListTags(ctx, repo)
		if err != nil {
			return formTagsMsg{service: service}
		}
		return formTagsMsg{service: service, tags: tags}
	}
}
```

Caso en `Update`:

```go
	case formTagsMsg:
		if m.form != nil && m.form.kind == actionDeploy && m.form.service == msg.service {
			m.form.setTags(msg.tags)
		}
		return m, nil
```

- `openAction`: el caso Deploy devuelve el cmd: tras asignar `m.form`, terminar con
  `if key.Matches(msg, m.keys.Deploy) { return m, m.loadFormTagsCmd(s.Name) }` antes del
  `return m, nil` final. `openActionKind` cambia de firma a
  `func (m *Model) openActionKind(kind actionKind) tea.Cmd` y devuelve
  `m.loadFormTagsCmd(s.Name)` cuando `kind == actionDeploy` (nil en el resto); su caller
  en `handleMouse` devuelve ese cmd.
- `handleFormKey`: ANTES del caso Tab/Right, añadir (¡por `msg.Type`, NO por
  `key.Matches`! — `m.keys.Down` incluye la letra `j` y capturarla impediría teclear
  tags que la contengan; es el mismo patrón de `handleFilterKey`):

```go
	case msg.Type == tea.KeyDown:
		m.form.movePick(1)
	case msg.Type == tea.KeyUp:
		m.form.movePick(-1)
```

(el switch de `handleFormKey` es `switch { case ... }`, así que mezclar condiciones
booleanas con `key.Matches` es válido; con lista vacía son no-op.)
- `clickForm`: tras calcular `row`/`x`, primero el picker:

```go
	if idx := m.form.tagAt(row); idx >= 0 {
		m.form.pick = idx
		m.form.input = m.form.visibleTags()[idx].Tag
		return nil
	}
```

- [ ] **Step 5: Tests de integración**

Añadir a `internal/tui/app_test.go`:

```go
// TestDeployFormShowsAndPicksTags: abrir deploy carga tags; ↓ rellena el input; enter despliega el elegido.
func TestDeployFormShowsAndPicksTags(t *testing.T) {
	reg := &coretest.FakeRegistry{Tags: map[string][]core.ImageTag{
		"api": {{Tag: "v9.9", PushedAt: time.Now().Add(-time.Hour)}},
	}}
	m := newTestModelWithRegistry(servicesNamed("api"), reg)
	updated, cmd := m.Update(keyMsg("d"))
	m = updated.(Model)
	require.NotNil(t, cmd, "abrir deploy debe disparar la carga de tags")
	m = mustUpdate(t, m, cmd().(formTagsMsg))
	require.Contains(t, stripANSI(m.View()), "v9.9")
	m = mustUpdate(t, m, tea.KeyMsg{Type: tea.KeyDown})
	require.Equal(t, "v9.9", m.form.input)
	updated, cmd = m.Update(keyMsg("enter"))
	m = updated.(Model)
	require.Nil(t, m.form)
	require.NotNil(t, cmd) // startDeployCmd con el tag elegido
}

// TestDeployFormDegradesWithoutRegistry: sin [images], el formulario es el de siempre.
func TestDeployFormDegradesWithoutRegistry(t *testing.T) {
	m := newTestModel(sampleServices()) // sin registry
	updated, cmd := m.Update(keyMsg("d"))
	m = updated.(Model)
	require.NotNil(t, m.form)
	if cmd != nil {
		m = mustUpdate(t, m, cmd().(formTagsMsg)) // llega vacío
	}
	require.Equal(t, 3, m.form.buttonRow()) // geometría sin picker
	// el flujo teclear+enter sigue intacto
	for _, r := range "v2" {
		m = mustUpdate(t, m, keyMsg(string(r)))
	}
	updated, cmd = m.Update(keyMsg("enter"))
	m = updated.(Model)
	require.Nil(t, m.form)
	require.NotNil(t, cmd)
}

// TestClickFormTagRowFillsInput: click en una fila del picker rellena el input (anclado al render).
func TestClickFormTagRowFillsInput(t *testing.T) {
	reg := &coretest.FakeRegistry{Tags: map[string][]core.ImageTag{
		"api": {{Tag: "v9.9", PushedAt: time.Now().Add(-time.Hour)}},
	}}
	m := newTestModelWithRegistry(servicesNamed("api"), reg)
	updated, cmd := m.Update(keyMsg("d"))
	m = updated.(Model)
	m = mustUpdate(t, m, cmd().(formTagsMsg))
	clickX, clickY := findInView(t, m.View(), "v9.9")
	m = mustUpdate(t, m, tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: clickX, Y: clickY})
	require.NotNil(t, m.form, "el click en un tag no cierra el formulario")
	require.Equal(t, "v9.9", m.form.input)
}
```

`servicesNamed(names ...string)` es un helper local que crea `[]core.ServiceStatus`
mínimos con esos nombres (para que el repo hermano sea "api" con template `{name}`).
Los tests de click del formulario existentes (`TestClickFormConfirmButton`, etc.) usan
`newTestModel` sin registry → sin tags → la geometría no cambia → pasan sin modificar.

- [ ] **Step 6: Verificar todo + gates + commit**

```bash
gofmt -w internal/ && golangci-lint run && go vet ./... && go test ./... -count=1 && go build ./...
git add internal/tui/
git commit -m "feat(tui): tag-picker filtrable en el formulario de deploy"
```
