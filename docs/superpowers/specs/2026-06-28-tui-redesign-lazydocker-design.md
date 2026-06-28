# Rediseño TUI — layout multi-panel estilo lazydocker

**Fecha:** 2026-06-28
**Estado:** Diseño aprobado, pendiente de plan de implementación
**Reemplaza:** la TUI secuencial actual (`internal/tui/model.go`, plan `2026-06-15-06-tui-service.md`)

## Motivación

La TUI actual navega entre pantallas secuenciales (lista → detalle → confirmar →
deploy). Es funcional pero se siente como una herramienta de scripting, no como un
producto. La referencia es **lazydocker**: un layout persistente multi-panel donde la
lista, el estado y los logs/eventos se ven al mismo tiempo y el panel derecho reacciona
a la selección sin perder contexto.

Objetivos:

- Layout multi-panel persistente (estilo lazydocker), no navegación por pantallas.
- **Soporte de mouse esencial:** click para seleccionar panel/item/pestaña/acción y
  rueda para scroll en logs/eventos.
- Estructura que **escale al roadmap** (multi-cluster, multi-cloud, y capacidades
  registry/ECR, db, queue… del `2026-06-15-roadmap.md`) sin rediseño.

No-objetivos (YAGNI en esta iteración):

- Paleta de comandos ⌘k (se reserva el hueco; es el plan 06b).
- Selector de contexto *funcional* (conmutar cluster/cloud en vivo). En v1 el top bar
  es informativo; el selector interactivo es trabajo posterior.
- Implementación real de las secciones IMAGES (ECR) / DATABASES (stubs visuales).
- Pestaña Logs con logs reales (stub v1; el `LogSource` aún no se consume aquí).

## Layout

Cuatro zonas:

```
┌─ steer ─ aws · staging (cluster: staging-cluster) ─────────── writable ●┐
├──────────────────────┬──────────────────────────────────────────────────┤
│ SERVICES         (4) │  api                                              │
│ ● api      2/2  v1.4 │  ┌ Details ─ Events ─ Logs ──────────────────────┐│
│ ● web      3/3  v2.0 │  │ running   2/2                                  ││
│ ◐ worker   1/2  v1.1 │  │ pending   0                                    ││
│ ○ cron     0/1  —    │  │ status    ACTIVE                               ││
│                      │  │ tag       v1.4                                 ││
│ IMAGES (ECR)     ··· │  │                                               ││
│   (próximamente)     │  │ [d] deploy  [s] scale  [R] rollback           ││
│                      │  │                                               ││
│ DATABASES        ··· │  └───────────────────────────────────────────────┘│
├──────────────────────┴──────────────────────────────────────────────────┤
│ ↑↓/click select · tab switch panel · ⌘k palette · ? help · q quit        │
└──────────────────────────────────────────────────────────────────────────┘
```

1. **Top bar (contexto)** — `cloud · env (cluster)` + indicador `writable ●` /
   `read-only ○`. Es donde vivirá la conmutación multi-cluster/multi-cloud (futuro). En
   v1 muestra el contexto fijo con el que se lanzó `steer tui`.

2. **Columna izquierda — secciones apiladas por dominio/capacidad** (patrón lazydocker).
   V1: **SERVICES** activa y navegable. **IMAGES (ECR)**, **DATABASES**, etc. aparecen
   como secciones nuevas conforme el core las exponga — cero rediseño. Cada sección
   muestra su título y count. Item de servicio: símbolo de estado + nombre +
   `running/desired` + tag.

3. **Panel derecho — pestañas Details / Events / Logs** sobre el item seleccionado.
   - **Details:** running/desired, pending, status, tag, y la fila de acciones
     `[d] deploy  [s] scale  [R] rollback`.
   - **Events:** eventos ECS (`ServiceEvents`) más el progreso de deploy en vivo
     (steps + eventos + línea de rollout). Scrolleable.
   - **Logs:** stub v1 ("sin fuente de logs configurada").

4. **Bottom bar** — ayuda contextual por foco + hueco reservado para ⌘k (v2).

## Arquitectura (Bubble Tea)

Se compone en vez de un `Model` monolítico. El `model.go` actual (465 líneas) mezcla
estado, comandos, navegación y render de 4 vistas; se parte en componentes de
responsabilidad única, cada uno testeable por separado:

```
internal/tui/
  app.go          ← Model raíz: layout, foco, routing mouse/teclado, top/bottom bar
  context.go      ← top bar: cloud · env · cluster · writable
  sidebar.go      ← columna izquierda: secciones apiladas (Services activa; ECR/DB stubs)
  panel/
    details.go    ← pestaña Details
    events.go     ← pestaña Events (ServiceEvents + deploy progress)
    logs.go       ← pestaña Logs (stub v1)
    tabs.go       ← barra de pestañas + pestaña activa
  action.go       ← input/modal contextual deploy/scale/rollback (overlay)
  keys.go         ← keymap centralizado (bubbles/key) → habilita ? help y rebinds
  styles.go       ← lipgloss: bordes, panel enfocado vs. apagado (reusa internal/render)
```

### Modelo de foco

Un enum `focus` (`focusSidebar` | `focusPanel` | `focusAction`) reemplaza el `viewState`
secuencial actual. El componente enfocado lleva borde resaltado (estilo lazydocker).

- `tab` / `shift+tab` y click mueven el foco entre sidebar y panel.
- Flechas / `j` / `k` navegan **dentro** del foco (items del sidebar, o scroll del panel).
- `focusAction` es transitorio: se entra al pulsar `d`/`s`/`R` (overlay de input) y se
  sale con `enter` (ejecuta) o `esc` (cancela).

### Mouse

Se activa con `tea.WithMouseCellMotion()` en el `Program`. `app.go` recibe `tea.MouseMsg`,
y usando las dimensiones que ya conoce por `WindowSizeMsg` calcula la zona del click y
enruta:

- Click en item del sidebar → selecciona ese servicio + foco al sidebar.
- Click en pestaña → cambia la pestaña activa + foco al panel.
- Click en `[d]/[s]/[R]` → entra a `focusAction` con esa acción.
- Rueda sobre el panel → scroll (vía `bubbles/viewport`).

### Datos y comandos

La capa `tea.Cmd` existente se conserva intacta: `loadServicesCmd`, `startDeployCmd`,
`deployPollCmd`, `tickCmd`/`deployTickCmd`. **El `core.Deployer` no se modifica.** Lo que
cambia es *dónde* se renderiza el resultado: el progreso de deploy alimenta la pestaña
**Events** del panel en vez de una pantalla `viewDeploy` separada. El auto-refresh
(15 s) y el poll de deploy (3 s) siguen igual.

### Scroll / viewport

Las pestañas **Events** y **Logs** usan `bubbles/viewport` para contenido scrolleable
(rueda + `j`/`k`/`pgup`/`pgdn` cuando el panel tiene foco). **Details** es contenido fijo
sin viewport.

### Layout responsivo

`WindowSizeMsg` reparte el espacio:

- Anchos: sidebar ~30 % (con mínimo ~24 columnas), panel ~70 %.
- Altos: top bar (3 líneas con borde), bottom bar (1 línea), resto al cuerpo.
- Fallback: si el ancho total < umbral (~80 columnas), **colapsa a una sola columna
  apilada**: sidebar arriba, panel abajo. Comportamiento explícito, nunca un layout roto.

### Acciones y read-only

`d`/`s`/`R` (o sus clicks) abren `action.go` como overlay de input contextual sobre el
panel. En entorno `writable == false` se bloquean y muestran un aviso en la bottom bar,
igual que hoy. La confirmación de deploy/scale exige input no vacío; rollback solo
confirma. Tras ejecutar, se refresca la lista.

## Testing

TDD por componente, siguiendo el patrón de `internal/tui/model_test.go` actual:

- `sidebar_test.go` — render de secciones/items, navegación, selección por índice.
- `panel/*_test.go` — render de cada pestaña, cambio de pestaña, scroll.
- `app_test.go` — routing de foco (tab/shift+tab), routing de mouse por zona,
  integración del flujo deploy (steps → Events), bloqueo read-only.
- Reusar el `FakeDeployer` existente para los comandos.

Cada componente debe renderizar a string de forma determinista para poder afirmarse en
tests sin terminal real.

## Migración

- Se reescribe `internal/tui/`. `model.go` se descompone; su lógica de comandos se
  preserva moviéndola a los componentes correspondientes.
- `internal/cli/tui_cmd.go` cambia solo para activar el mouse
  (`tea.WithMouseCellMotion()`) al construir el `Program`; la firma de `New(...)` puede
  ajustarse pero mantiene los mismos parámetros de contexto (dep, cluster, env, writable).
- `internal/core` y `internal/providers` **no se tocan**.

## Decisiones resueltas

- **Fallback angosto:** colapsar a una sola columna apilada (sidebar arriba, panel abajo).
- **Fila de acciones:** vive dentro de la pestaña Details (`[d] deploy  [s] scale  [R] rollback`).
- **Pestaña Logs:** visible en v1 como placeholder ("logs no disponibles todavía"),
  lista para conectarse cuando el core exponga un `LogSource`.
- **Identidad de color:** el acento de marca de steer es **cian de Go** (`#00ADD8`,
  `render.BrandColor`), usado en wordmark, pestaña activa, tags, valores destacados,
  teclas de acción `[d]/[s]/[R]`, cursor/selección y borde del panel con foco. El
  **verde/amarillo/rojo se reservan exclusivamente para estado de salud** (puntos `●`,
  `ACTIVE`, `writable ●`, éxito/fallo). La fila seleccionada del sidebar lleva una barra
  de fondo cian oscuro conservando el color del punto de estado.
- **Lista de servicios:** se oculta el prefijo de cluster/entorno (`nao-v2-{env}-`) en la
  visualización (el nombre real se conserva para las acciones) y se ordena alfabéticamente.

## Decisiones diferidas (futuro, no en este plan)

- Atajo y forma del selector de contexto interactivo (multi-cluster/multi-cloud); en v1
  solo se reserva el espacio en el top bar.
- **Click sobre la fila de acciones `[d]/[s]/[R]`** del panel Details: en v1 las acciones
  se disparan por teclado (`d`/`s`/`R`); el click directo sobre esos botones queda
  diferido (la geometría de filas del Details es frágil de mapear). El mouse en v1 cubre
  click en servicios del sidebar, click en pestañas y rueda para scroll.
