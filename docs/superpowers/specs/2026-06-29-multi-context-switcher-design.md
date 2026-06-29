# Contextos multi-cuenta/multi-cloud + selector en la TUI

**Fecha:** 2026-06-29
**Estado:** Diseño aprobado, pendiente de plan de implementación

## Motivación

Hoy un "entorno" es una entrada `[providers.aws.environments.<name>]` con un `profile` de
AWS; el cluster se deriva del nombre con `cluster_template`, y se elige con el flag
`-e/--env` **al lanzar**. Dentro de la TUI no se puede cambiar de entorno, y el modelo solo
representa AWS. Los usuarios manejan varias cuentas (los 4 ambientes de NAO viven en
perfiles distintos) y a futuro varios clouds.

Esta feature introduce un modelo de **contextos** auto-descriptivos y un **selector dentro
de la TUI** para conmutar sin relanzar. El modelo admite `cloud = aws|gcp|azure`, pero solo
**AWS se implementa ahora**; GCP/Azure quedan representables y visibles en el selector con
un error amable hasta que existan sus providers.

## Modelo de configuración (`steer.toml`)

Reemplaza por completo `[providers.aws.environments.*]` y `[providers.aws.naming]`.

```toml
default_context = "nao-dev"          # cuál abre `steer tui` sin --context

[contexts.nao-dev]
cloud            = "aws"
profile          = "dev"
cluster          = "nao-v2-dev-cluster"
service_template = "nao-v2-dev-{name}"   # resuelve nombres cortos + define el prefijo a ocultar
writable         = true

[contexts.nao-production]
cloud            = "aws"
profile          = "prod"
cluster          = "nao-v2-production-cluster"
service_template = "nao-v2-production-{name}"
writable         = false

[contexts.acme-staging]
cloud   = "gcp"            # representable; su provider aún no existe
project = "acme-stg"
region  = "us-central1"
cluster = "acme-stg-run"
writable = true
```

Reglas:

- Cada contexto es **explícito y auto-descriptivo** (sin plantillas de cuenta).
- Campos comunes: `cloud`, `cluster`, `writable`, `service_template` (opcional).
- Auth por cloud: AWS usa `profile` (+ opcional `account_id`, `role_arn`, `region`);
  GCP usa `project`+`region`; Azure usa `subscription` (se modelan los campos, pero solo
  AWS los consume hoy).
- `service_template` cumple dos roles, igual que el `service_template` actual: resolver
  nombres cortos (`audit-ms` → `nao-v2-dev-audit-ms`) y, vía `Service(ctx, "")`, definir el
  **prefijo a ocultar** en la lista de la TUI. Si se omite, no hay resolución de cortos ni
  recorte de prefijo (el nombre real se muestra tal cual).
- `default_context` (a nivel raíz) indica qué contexto abre `steer tui`/los comandos sin
  `--context`. Si falta y hay un solo contexto, ese es el default; si hay varios y no se da
  `default_context` ni `--context`, es un error de config explicado.

## Modelo de dominio (`internal/config`)

```go
// Context es un destino conmutable: una credencial + un cluster.
type Context struct {
    Name            string
    Cloud           string // "aws" | "gcp" | "azure"
    Profile         string // AWS
    AccountID       string // AWS (opcional)
    RoleARN         string // AWS (opcional)
    Region          string // AWS/GCP (opcional)
    Project         string // GCP
    Subscription    string // Azure
    Cluster         string
    ServiceTemplate string
    Writable        bool
}

// Contexts es la colección cargada de steer.toml.
type Contexts struct { /* mapa + orden + default */ }

func (c Contexts) Get(name string) (Context, error) // error si no existe
func (c Contexts) Default() (Context, error)         // default_context, o único, o error
func (c Contexts) All() []Context                    // orden estable (alfabético por Name)
func (ctx Context) ServiceName(short string) string  // resuelve nombre real (service_template)
func (ctx Context) Prefix() string                   // prefijo a ocultar (= ServiceName(""))
```

`config.Load` parsea `[contexts.*]` + `default_context`. Validación: cada contexto exige
`cloud` y `cluster`; AWS exige `profile`; `default_context` (si está) debe existir.

## Fábrica de Deployer

El cambio arquitectónico clave: hoy la TUI recibe **un** `core.Deployer` ya construido. Para
conmutar necesita una **fábrica** que, dado un contexto, construya el Deployer.

```go
// internal/providers
var ErrProviderNotImplemented = errors.New("provider not implemented")

// DeployerFactory construye un Deployer para un contexto (o un error).
type DeployerFactory func(ctx config.Context) (core.Deployer, error)
```

- Para `cloud == "aws"`: crea la sesión AWS con `profile`/`region`/`role_arn` del contexto y
  devuelve el `ECSDeployer` existente.
- Para `cloud != "aws"`: devuelve `ErrProviderNotImplemented` (envuelto con el nombre del
  cloud para el mensaje).
- En tests se sustituye por un seam fake (igual que `newDeployerFn` hoy).

## TUI: selector de contexto

- **Estado nuevo en el Model:** la fábrica `DeployerFactory`, la colección `Contexts`, y el
  `current config.Context`. El `cluster`/`writable`/`prefix` actuales se derivan del
  contexto actual en vez de pasarse sueltos.
- **Top bar:** muestra `cloud · <context-name>` + `writable/read-only` (ya existe; ahora se
  alimenta del contexto actual).
- **Abrir selector:** tecla `c` o click en el top bar → `focusContextPicker` (nuevo estado
  de foco, overlay).
- **Componente `contextpicker`** (nuevo, testeable solo): renderiza los contextos
  **agrupados por `cloud`** (secciones AWS/GCP/Azure), cada uno con su `writable/read-only`
  y marca `(no impl.)` si su cloud no es AWS. Navegación `↑↓`/click, `enter` selecciona,
  `esc` cancela. El contexto actual aparece resaltado.
- **Conmutar:** al elegir un contexto:
  - Si su cloud no está implementado → mensaje amable en la bottom bar, no cambia.
  - Si es AWS → llama a la fábrica; si falla (p.ej. SSO expirado) → muestra el error y se
    mantiene en el contexto previo; si tiene éxito → reemplaza `dep`/`current`, recarga
    servicios (`loadServicesCmd`), reinicia el sidebar y vuelve a `focusSidebar`.
- **Mouse:** click en una fila del picker selecciona; rueda no aplica (lista corta). Se
  reutiliza el patrón de zonas; el picker es un overlay que captura el input mientras está
  activo.

## CLI

- Se añade `--context/-c <name>`; `-e/--env` se mantiene como **alias compatible** que mapea
  al mismo contexto.
- Sin `--context`: usa `default_context` (o el único; o error si ambiguo).
- La resolución de nombres cortos (`-s audit-ms`) pasa a leerse del contexto activo
  (`Context.ServiceName`), no de un `naming` global.
- `steer config init` genera un `steer.toml` de ejemplo con el formato `[contexts.*]`.
- `steer config validate` valida el nuevo modelo.

## Wiring TUI/CLI

- `tui.Run` cambia de `Run(dep, cluster, env, writable, prefix)` a
  `Run(factory providers.DeployerFactory, contexts config.Contexts, current config.Context)`.
  La TUI construye el primer Deployer con la fábrica a partir de `current`.
- `internal/cli/tui_cmd.go` resuelve el contexto (`--context`/default), arma la fábrica y la
  pasa a `tui.Run`.
- `internal/cli/service_cmd.go` y demás: `newDeployerFn` se reescribe para tomar un
  `config.Context` y usar la fábrica; la resolución de nombres usa `Context.ServiceName`.

## Manejo de errores

- Config inválida (contexto sin `cloud`/`cluster`, AWS sin `profile`, `default_context`
  inexistente, modelo viejo detectado) → error claro al cargar, con qué arreglar.
- Cloud no implementado al conmutar → aviso en bottom bar (`provider "gcp" not implemented yet`),
  sin cambiar de contexto.
- Fallo de auth al construir el Deployer (SSO expirado, perfil inexistente) → error visible,
  se conserva el contexto anterior.
- `steer tui` con config ambigua (varios contextos, sin default ni `--context`) → error que
  lista los contextos disponibles.

## Alcance

**Dentro (implementable ahora, AWS):**

- Modelo `[contexts.*]` + `default_context`, loader y validación.
- `DeployerFactory` (AWS real; no-AWS → `ErrProviderNotImplemented`).
- Selector de contexto en la TUI (tecla `c` + click, overlay, recarga al conmutar).
- CLI `--context/-c` (+ alias `-e`) y resolución de nombres por contexto.
- `config init/validate` actualizados al nuevo formato.

**Fuera (futuro):**

- Providers reales GCP (Cloud Run) y Azure (Container Apps) y su auth.
- `group` por contexto para sub-agrupar dentro de un cloud en el picker.
- Selector de contexto desde la paleta ⌘k (plan 06b).

## Pruebas

- **config:** carga de `[contexts.*]`, `default_context` (presente/único/ambiguo),
  validación de campos requeridos, `ServiceName`/`Prefix` por contexto.
- **fábrica:** AWS construye Deployer; no-AWS devuelve `ErrProviderNotImplemented`; seam
  fake para inyección.
- **contextpicker:** render con grupos por cloud, marca de no-implementado, navegación,
  selección, resaltado del actual.
- **TUI switch:** abrir picker (`c`), elegir AWS → recarga + cambia writable/prefix; elegir
  no-impl → aviso sin cambio; fallo de fábrica → conserva contexto previo.
- **CLI:** `--context` resuelve; `-e` alias; default/ambiguo; `ServiceName` desde contexto.

## Migración

- Se reescribe `internal/config` (modelo + loader + validación) reemplazando
  `Environment`/`Naming` por `Context`/`Contexts`.
- `internal/cli` (root flags, service_cmd, tui_cmd, config_cmd) se adapta al nuevo modelo.
- `internal/tui` (`Run`/`New`, Model, nuevo `contextpicker`, foco) se adapta.
- `internal/core` y `internal/providers/aws` (la lógica ECS) **no cambian**; solo se añade
  la construcción de sesión por contexto en la fábrica.
- El `steer.toml` local del repo se actualiza al nuevo formato.
