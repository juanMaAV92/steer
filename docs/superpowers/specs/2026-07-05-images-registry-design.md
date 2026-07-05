# Capacidad IMAGES (registry multi-cloud) — Diseño

**Fecha:** 2026-07-05 · **Estado:** aprobado
**Objetivo:** la sección IMAGES del TUI deja de ser stub: repos y tags reales del registry
del contexto (ECR en AWS; homólogos Artifact Registry/ACR después), marcador del tag
desplegado, tag-picker en el formulario de deploy y comandos CLI. Todo de **solo lectura**
en v1 — el único write path sigue siendo el deploy.

## Decisiones (votadas)

1. **Alcance del hito**: repos + tabla de tags en TUI, marcador de tag desplegado,
   tag-picker en el formulario de deploy, y CLI (`steer image ls` / `steer image tags`).
2. **Scoping por prefijo vía template** (config anidada por capacidad):
   `[contexts.<n>.images]` con `repo_template = "nao-v2-{name}"`. IMAGES muestra solo los
   repos que casan con el prefijo, con el prefijo oculto — el patrón de services. El
   vínculo repo↔servicio sale del `{name}` corto compartido entre ambos templates.
3. **Tag-picker: input + lista filtrable.** El input sigue siendo libre; debajo aparecen
   los tags recientes; lo tecleado filtra en vivo; ↑↓/click eligen y rellenan el input;
   enter confirma lo que esté en el input. Sin registry o con error → input solo, como hoy.
4. **Comando `steer image`** (alias `img`), consistente con `steer service`: capacidad,
   no servicio del cloud.
5. **Orden**: repos **alfanuméricos** (como services; el filtro `/` conserva el orden).
   Tags por **fecha de push descendente** (lo relevante al desplegar).
6. **Solo imágenes reales**: `ListTags` devuelve únicamente imágenes de contenedor **con
   tag y desplegables**. Los registros asociados que no son imágenes NO aparecen: en ECR
   se filtran los manifiestos sin `imageTags` (capas colgantes), los artefactos OCI de
   attestation/SBOM (media types de buildkit/in-toto) y las firmas de cosign (tags
   `sha256-*.sig`/`*.att`). Cada provider aplica su filtro; el contrato es de capacidad.

## Capacidad core (agnóstica)

En `internal/core/core.go` (denominador común verificado en ECR, Artifact Registry y ACR):

```go
// Repository es un repositorio de imágenes del registry del contexto.
type Repository struct {
	Name string // nombre real (con prefijo); la UI lo acorta con RepoPrefix
}

// ImageTag es una imagen de contenedor etiquetada y desplegable.
type ImageTag struct {
	Tag       string
	Digest    string // corto para mostrar; completo del provider
	SizeBytes int64
	PushedAt  time.Time
}

// Registry lista repositorios e imágenes del registry del contexto
// (ECR / Artifact Registry / ACR). Solo lectura.
type Registry interface {
	// ListRepositories devuelve los repos que casan con el prefijo del contexto,
	// ordenados alfanuméricamente.
	ListRepositories(ctx context.Context) ([]Repository, error)
	// ListTags devuelve solo imágenes con tag desplegables (sin manifiestos
	// colgantes, attestations ni firmas), más recientes primero, tope maxTags=50.
	ListTags(ctx context.Context, repo string) ([]ImageTag, error)
}
```

`coretest` gana `FakeRegistry` (repos y tags fijados, contadores de llamadas, error
inyectable) — espejo de `FakeDeployer`.

## Provider

- `providers.Provider` gana `Registry() (core.Registry, error)` (la costura comentada en
  `factory.go:24`).
- `aws.Provider` lo implementa con un cliente ECR construido sobre la **sesión cacheada**
  del bundle (mismo patrón que `Deployer()`); el prefijo de repos se fija al construir
  (patrón cluster-al-constructor).
- Filtro ECR de "solo imágenes": descartar entradas sin `imageTags`; descartar
  `artifactMediaType`/`imageManifestMediaType` de attestation (buildkit, in-toto) y
  cualquier tag con sufijo `.sig`/`.att` (convención cosign).
- Contexto sin bloque `[images]` → `Registry()` devuelve `core.ErrNoImagesConfig`
  (sentinel en core), que la UI traduce a hint y el CLI a mensaje accionable.

## Config anidada por capacidad

```toml
[contexts.dev]
cloud    = "aws"
profile  = "dev"
cluster  = "nao-v2-dev"
service_template = "nao-v2-dev-{name}"
writable = true

  [contexts.dev.images]
  repo_template = "nao-v2-{name}"
```

- `config.Context` gana `Images *ImagesConfig` (`toml:"images"`), con
  `ImagesConfig{RepoTemplate string}`.
- Helpers espejo: `Context.RepoName(short)` y `Context.RepoPrefix()` (mismas semánticas
  que `ServiceName`/`Prefix`).
- `Validate()`: si el bloque `[images]` existe, `repo_template` es obligatorio y debe
  contener `{name}`. Sin bloque, la capacidad queda deshabilitada sin error.
- `steer.example.toml` documenta el bloque.

## TUI

- **Sidebar**: IMAGES se llena con los repos (nombre corto, alfanumérico). Colapso,
  filtro `/`, scroll y clicks ya funcionan — el sidebar escalable no cambia. El count del
  header refleja los repos. Sin config `[images]`: la sección muestra una línea tenue
  "configure images in steer.toml". Error del registry: línea tenue de error en la
  sección, sin romper SERVICES.
- **Carga**: async al entrar al contexto (mismo ciclo que `loadServicesCmd`); los repos
  no se re-piden en cada tick (cambian poco) — se cargan al conmutar contexto y con `r`.
- **Panel**: al seleccionar un repo, el panel derecho muestra la tabla de tags:
  `TAG · AGE · SIZE · DIGEST`, más reciente primero, con `● now` en el tag que corre en
  el servicio hermano (comparado contra `ServiceStatus.Tag` ya cargado). Estados:
  "loading tags…" / error / "no images yet". La selección repo vs servicio decide qué
  muestra el panel (pestañas Details/Events/Logs siguen siendo de servicios).
- **Tag-picker en deploy**: al abrir el formulario de deploy se disparan los tags del
  repo hermano en background. La lista (hasta 5 visibles, scroll implícito al filtrar)
  aparece entre el input y los botones; teclear filtra por substring; ↑↓ mueven la
  selección de la lista y rellenan el input; click elige y rellena; enter confirma el
  input (elegido o tecleado). Registry ausente/error/carga lenta → el formulario funciona
  exactamente como hoy (input libre); la lista aparece si llega. El contrato de foco de
  botones (tab/←→) y click-fuera-no-op no cambia.

## CLI

- `steer image ls` — tabla `REPO · LATEST TAG · PUSHED` (repos alfanuméricos, último tag
  de cada uno).
- `steer image tags -r <short>` — tabla `TAG · AGE · SIZE · DIGEST · DEPLOYED` del repo.
- Alias `img`. Respetan `--context`/`STEER_CONTEXT`. Render con `internal/render/table.go`.
- Sin `[images]` en el contexto: mensaje accionable con el snippet de config sugerido.

## Errores

- El registry NUNCA bloquea el flujo de services: sus errores se muestran en su sección/
  panel y el deploy sigue funcionando con input libre.
- `ErrNoImagesConfig` distingue "no configurado" (hint) de un error real del cloud
  (mensaje del provider).

## Fuera de alcance

Borrar/untag imágenes; paginación más allá del tope de 50 tags; métricas de vulnerabilidad
(scan findings); pull-through/replicación; homólogos GCP/Azure (la interfaz los admite;
`IsImplemented` sigue siendo aws); refresco periódico de tags por tick.

## Pruebas

- Core/coretest: `FakeRegistry`; contrato de orden (repos alfanumérico, tags por fecha).
- Provider AWS: filtro de solo-imágenes (sin tags, attestations, `.sig`) con respuestas
  ECR fixture; scoping por prefijo; `ErrNoImagesConfig` sin bloque images.
- Config: `Validate` del bloque images (template sin `{name}` falla); `RepoName`/`RepoPrefix`.
- TUI ancladas al render: repos en IMAGES (orden y prefijo oculto), click en repo muestra
  tabla, `● now` en el tag correcto, picker filtra/elige/rellena, deploy degrada a input
  libre sin registry. Los tests de click existentes intactos.
- CLI: `image ls`/`image tags` con `runRootWithFake` (fake registry), incluido el caso
  sin config.
