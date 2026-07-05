# Deploy seguro (validación de tag + detección de atasco) — Diseño

**Fecha:** 2026-07-05 · **Estado:** aprobado
**Objetivo:** cerrar los dos huecos que expuso desplegar un tag inexistente (`dev`): el
deploy registró la task definition a ciegas y ECS quedó reintentando el pull para siempre
mientras el watch de steer poll-eaba mudo e indefinidamente. Con el registry recién
construido, steer puede validar ANTES y detectar el atasco DESPUÉS.

## Decisiones (votadas)

1. **Validación estricta + degradable.** Tag inexistente → bloquea siempre (TUI y CLI).
   Consulta al registry fallida (red, permisos) o contexto sin `[images]` → el deploy
   continúa con un aviso. La validación es una ayuda, no un punto único de fallo: un ECR
   intermitente nunca rompe CI ni deja sin desplegar.
2. **Atasco por eventos: 3 fallos de aprovisionamiento → STUCK.** Sin timeout global
   (falso positivo con imágenes grandes; queda como paracaídas futuro). El umbral 3
   tolera un fallo transitorio de throttling.

## Capacidad — `Registry.HasTag`

`core.Registry` gana:

```go
// HasTag verifica si el tag existe en el repo. Consulta puntual: no depende del
// tope de ListTags (valida tags viejos que el picker no muestra).
HasTag(ctx context.Context, repo, tag string) (bool, error)
```

- **ECR**: `DescribeImages` con `ImageIds: [{ImageTag: tag}]`. `ImageNotFoundException`
  → `false, nil`. Otro error → `false, err` (dispara la degradación en el llamador).
- **FakeRegistry**: busca en su mapa `Tags[repo]`; `HasTagErr` inyectable;
  `HasTagCalls []string` ("repo/tag") para asertar.

## Heurística de atasco — `core.IsProvisioningFailure`

```go
// IsProvisioningFailure detecta eventos de fallo de aprovisionamiento en el texto
// que reporta el provider (heurística documentada: ECS hoy; otros clouds añadirán
// sus patrones). Casan: "CannotPullContainerError", "was unable to place a task".
func IsProvisioningFailure(msg string) bool
```

Case-insensitive por robustez. Vive en core porque el contador es del watch (TUI/CLI),
no del provider; los patrones son por-provider y se documentan como tal.

## TUI — validación en el confirm del deploy

Confirmar un deploy (enter o click en `[ Deploy (↵) ]`) ya NO arranca el deploy directo:

- El formulario **permanece abierto** en estado "validating tag…" (línea tenue bajo el
  prompt; botones inertes mientras valida) y se dispara `validateTagCmd(service, tag)`.
- `tagValidatedMsg{service, tag, verdict}` con tres veredictos:
  - **ok** → cierra el form y ejecuta `applyActionConfirmed` (flujo de siempre:
    Events + poll).
  - **notFound** → el form sigue abierto con línea roja `tag not found in <repo>`;
    el usuario corrige el input y reintenta (o esc).
  - **skipped** (ErrNoImagesConfig o error del registry) → cierra el form, ejecuta el
    deploy Y muestra notice de aviso: `registry check skipped — deploying unverified tag`.
- Guards de respuesta obsoleta (form cerrado / kind≠deploy / service o tag distinto):
  se ignora, mismo patrón de `formTagsMsg`.
- Scale y rollback no cambian (no hay tag que validar).

## CLI — misma regla, síncrona

`steer service deploy`: antes del preview, si el contexto puede dar registry →
`HasTag(RepoName(short), tag)`:
- `false, nil` → error y exit: `tag "dev" not found in repository "nao-v2-shared-audit-ms"`.
- error / `ErrNoImagesConfig` → warning en stderr (`warning: registry check skipped: <razón>`)
  y el flujo continúa como hoy.

## Watch — detección de atasco (TUI y CLI `-w`)

- `deployState` gana `PullErrors int`; se resetea con `Reset()` y al arrancar cada deploy.
- En cada `deployPollMsg`, los eventos nuevos que casen `core.IsProvisioningFailure`
  incrementan el contador. Al llegar a **3** en el rollout actual:
  - TUI: `deploy.Active=false, Done=true` (el poll se detiene), línea roja en Events:
    `✗ deployment stuck: image pull failing — roll back with R`, y el status line lo
    refleja. `R` sigue operativo (el form de rollback funciona normal).
  - CLI `-w`: imprime el error, sugiere `steer service rollback -s <svc>` y sale con
    código ≠ 0.
- El rollout que ECS sí reporta como `FAILED` sigue su camino actual (sin cambios).

## Errores

- La validación nunca sustituye al error real de ECS: si algo pasa el check (carrera
  con un untag) el watch atrapa el atasco igual.
- `notFound` no toca el estado del deploy (nada arrancó); `skipped` deja rastro visible
  (notice/stderr) de que se desplegó sin verificar.

## Fuera de alcance

`--force`/override; timeout global de watch; habilitar el circuit breaker de ECS
(recomendado, pero es infraestructura del usuario); validación en `scale`/`rollback`;
patrones de otros clouds en `IsProvisioningFailure` (llegan con sus providers).

## Pruebas

- Core: `IsProvisioningFailure` (patrones, case-insensitive, negativos); fake `HasTag`.
- Provider AWS: `HasTag` con `ImageNotFoundException` → false sin error; error genérico
  → propaga; llamada con el repo/tag correctos.
- TUI: los tres veredictos (ok arranca deploy; notFound mantiene el form con la línea
  roja — anclado al render; skipped despliega con notice); respuesta obsoleta ignorada;
  atasco: 3 eventos de pull → poll detenido + mensaje rojo + R operativo; 2 eventos → sigue.
- CLI: deploy bloqueado con tag inexistente (mensaje + exit error); degradado con
  registry caído (warning + despliega); `-w` sale con error y sugiere rollback al atasco.
