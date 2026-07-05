# Ola de deuda menor — Diseño

**Fecha:** 2026-07-05 · **Estado:** aprobado
**Objetivo:** drenar los minors diferidos acumulados en los ledgers de los hitos
sidebar-escalable, acción-inline, images-registry y deploy-seguro: 3 ajustes de
comportamiento (votados), 5 correcciones mecánicas y 7 tests faltantes.

## Decisiones (votadas)

Los 3 ajustes de comportamiento entran:

1. **Repo inexistente bloquea el deploy.** Repo-no-existe es una respuesta definitiva
   (la imagen seguro no está), no un fallo del registry: no debe degradar a "skipped".
   Nuevo sentinel `core.ErrRepoNotFound`; `ECRRegistry.HasTag` mapea
   `RepositoryNotFoundException` → `(false, core.ErrRepoNotFound)`. El TUI lo trata como
   veredicto bloqueante con línea roja `repository <repo> not found`; el CLI sale con
   `repository %q not found (check images.repo_template)`. Cualquier OTRO error del
   registry sigue degradando a skipped (filosofía estricta + degradable intacta).
2. **Esc restaura el panel TAGS.** Al cerrar el formulario sin ejecutar (esc o botón
   Cancel), si el cursor del sidebar sigue sobre un repo (`cursorEntry().Kind ==
   entryRepo`), `sidebar.lastSelected` vuelve a `sectionImages`. Helper `closeForm()`
   en el Model para los sitios de cierre sin ejecución; los cierres CON ejecución
   (deploy/scale/rollback confirmados) NO restauran (van a Events/Details).
3. **Reload fallido conserva repos.** En `reposMsg{err}` con repos ya cargados: la lista
   sigue visible (`imagesReady`) y el error va a la barra inferior como notice
   `images refresh failed: <err>`. Sin repos previos: estado `imagesError` en la
   sección, como hoy.

## Correcciones mecánicas

- **tagsRepo huérfano**: al procesar `reposMsg` exitoso, si el repo seleccionado
  desapareció (`selectedRepo()` no ok) y `tagsRepo != ""`, resetear
  `tagsRepo/tags/tagsErr/tagsLoading` — un `tagsMsg` tardío del repo desaparecido ya no
  pasa el guard.
- **Comentarios "modal" obsoletos** → "formulario": `app.go:542`, `app_test.go:539,569,576`.
- **Helper `shortRepo`**: `func (m Model) shortRepo(repo string) string` deduplica el
  `strings.TrimPrefix(repo, m.current.RepoPrefix())` de `panelBody` y `deployedTagFor`.
- **`appendBlank` único** en el bloque IMAGES de `sidebar.rows()` (hoy duplicado en
  if/else).
- **Comentario "defensivo"** en la cláusula `m.form.input != msg.tag` del guard de
  `tagValidatedMsg` (inalcanzable con el teclado congelado; queda por si un futuro
  productor externo emite veredictos).

## Tests faltantes

- **Paginación ECR**: `fakeECR` gana modo paginado (2 páginas con NextToken);
  `ListRepositories` y `ListTags` recorren ambas.
- **Fronteras de `render.Age`**: exactamente 1min → "1m ago", 1h → "1h ago",
  24h → "1d ago".
- **Umbral CLI discriminado**: `stuckDeployer` entrega 2 eventos en una consulta y 1 en
  la siguiente; el watch NO corta con 2 y SÍ con el 3º.
- **Errores del registry en CLI**: `ReposErr` aborta `image ls`; `TagsErr` aborta
  `image tags` (exit con error, mensaje del provider).
- **Click en botones de Details con form abierto**: no-op (no reabre otra acción) —
  cubre el edge razonado-por-trace del hito acción-inline.
- **Loop notFound → retry → ok end-to-end**: confirmar tag malo → línea roja → corregir
  (errMsg se limpia al teclear) → confirmar tag bueno → deploy arranca;
  `HasTagCalls` acumula las 2 consultas.
- **rollout_test.go**: verificar el assert duplicado reportado en la remediación; si
  sigue, dedupe (si ya no existe, no-op documentado en el reporte).

## Explícitamente NO se toca

Colisión de indicadores con `height==1` (terminal degenerada); `scrollBy` re-renderiza
para contar (patrón del paquete); receivers mixtos de `actionForm` y `moveFocus` ±1
(idiomáticos, 2 botones); backspace-tras-pick edita el valor elegido (patrón
autocomplete estándar); generation-id en `formTagsMsg` (carrera teórica);
re-salto de cursor en drain-repopulate del sidebar (edge cosmético).

## Pruebas y disciplina

Cada ajuste de comportamiento lleva su test (render-anchored donde aplique); los tests
de click existentes pasan sin modificar (salvo los 4 comentarios "modal", que son solo
comentarios); gates completos antes de cada commit; sin atribución a IA.
