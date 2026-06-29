# UX de acciones: botones clickeables + modal centrado

**Fecha:** 2026-06-29
**Estado:** Diseño aprobado, pendiente de plan de implementación

## Motivación

Hoy las acciones del panel Details se muestran como texto (`[d] deploy  [s] scale  [R] rollback`)
que **no es clickeable**, y al disparar una acción el input/confirmación **reemplaza la barra
inferior** de la terminal — lejos del panel donde el usuario actuó, lo que rompe la UX. Esta
feature convierte las acciones en **botones clickeables** (con la tecla indicada) y mueve el
input a un **modal centrado** sobre el panel.

## Botones de acción (panel Details)

La fila de acciones pasa de texto plano a tres botones:

```
[ Deploy (d) ]   [ Scale (s) ]   [ Rollback (R) ]
```

- Cada botón se renderiza enmarcado/resaltado en cian de marca (`render.BrandColor`); la
  **tecla** va entre paréntesis con el glifo en cian, mostrando la `R` en mayúscula tal como
  se presiona.
- **Clickeable**: un click sobre un botón abre su acción (= abre el modal).
- **Teclado**: `d`/`s`/`R` siguen abriendo la acción correspondiente (sin cambio).
- En contexto **read-only** (`writable == false`) los botones se muestran apagados
  (`render.Dim`) y no responden; en su lugar, el aviso "read-only environment — actions
  disabled" como hoy.
- Hit-testing por **posición de etiqueta** (mismo enfoque que `panel.Tabs.TabAtColumn`):
  un helper calcula el rango de columnas de cada botón en la fila.

## Modal centrado de acción

Al disparar una acción (click o tecla) se entra a un nuevo estado de foco
`focusActionModal` que reemplaza al actual `focusAction` (que pintaba en la barra inferior).
El modal se dibuja como una caja redondeada **centrada sobre el cuerpo** con `lipgloss.Place`:

```
        ╭─ Deploy accreditation-ms ───────────────╮
        │                                         │
        │   image tag:  v1.5_                      │
        │                                         │
        │      [ Deploy (↵) ]   [ Cancel (esc) ]  │
        ╰─────────────────────────────────────────╯
```

- **Deploy / Scale**: título (`Deploy <service>` / `Scale <service>`), un campo de texto
  (tag / número) que refleja lo que se teclea, y dos botones `[ Deploy (↵) ]` / `[ Cancel (esc) ]`
  (para scale, `[ Scale (↵) ]`).
- **Rollback**: solo confirmación (`Roll back <service> to previous revision?`) + botones
  `[ Confirm (↵) ]` / `[ Cancel (esc) ]`.
- **Teclado** (sin cambio respecto a hoy): teclear alimenta el campo (no aplica a rollback),
  `enter` confirma si el input es válido, `esc` cancela.
- **Mouse**:
  - Click en el botón de confirmar (`Deploy`/`Scale`/`Confirm`) → ejecuta (igual que enter).
  - Click en `[ Cancel ]` → cierra sin ejecutar.
  - Click **fuera del modal** → cancela (cierra).
  - El modal **captura todos los eventos de mouse** mientras está abierto (no se filtran al
    sidebar/panel por debajo), igual que el `contextPicker`.
- Al confirmar un **deploy**, el flujo en vivo sigue alimentando la pestaña **Events** tal
  como ya funciona hoy (no cambia).

## Arquitectura

- **`action.go`**: el tipo `action` gana el render del modal — una caja (`lipgloss` rounded
  border) con título, campo/confirmación y la fila de botones, centrada con `lipgloss.Place`
  sobre el área del cuerpo (recibe ancho/alto disponibles). Mantiene `open`/`close`/`typeKey`/
  `ready` actuales; se reemplaza `view()` (barra inferior) por `modalView(width, height int) string`.
- **Helper de botones** (`buttonbar` o similar, reutilizable): dada una lista de etiquetas y
  un offset de inicio, renderiza la fila de botones y expone `buttonAtColumn(x) int` para el
  hit-testing — usado tanto por la fila de Details como por los botones del modal. (Se modela
  igual que `panel.Tabs.TabAtColumn`.)
- **`panel/details.go`** (`DetailsView`): renderiza los botones `[ Deploy (d) ]` … usando el
  helper, en vez del texto actual; expone la geometría (offset/anchos) para que `app.go` mapee
  clicks. Como `DetailsView` es una función pura que devuelve string, el hit-testing de la fila
  de Details vive en `app.go` reusando el helper con las mismas etiquetas/offset.
- **`app.go`**:
  - Nuevo `focusActionModal` (reemplaza el uso de `focusAction` para el render inferior; el
    nombre del estado puede conservarse o renombrarse — decisión menor del plan).
  - `View()`: cuando el foco es el modal, dibuja el cuerpo normal y superpone el modal
    centrado (`lipgloss.Place` sobre el cuerpo, o render del cuerpo + overlay).
  - `handleKey`: la rama del modal mantiene el comportamiento actual (teclear/enter/esc).
  - `handleMouse`: (a) en el panel Details, click en la fila de botones → abre la acción
    (reusando el helper de botones + la geometría del panel, `panelContentX0 = sidebarW + 3`);
    (b) cuando el foco es el modal, enruta el click a los botones del modal o cancela si es
    fuera, y traga el resto.

## Manejo de errores / casos borde

- Read-only: los botones de Details no abren acción; clic = no-op (el aviso ya existe).
- Deploy/scale con input vacío: `enter`/click en confirmar no hace nada hasta que haya input
  (comportamiento actual de `ready()`).
- Scale con número inválido: igual que hoy (`actionDoneMsg` con error → se muestra).
- Terminal angosta: el modal usa un ancho acotado (`min(ancho_cuerpo - margen, ~50)`) y se
  centra; si el cuerpo es muy pequeño, se degrada a ancho del cuerpo.

## Alcance

**Dentro:**
- Botones clickeables en Details con tecla indicada (`(d)`/`(s)`/`(R)`).
- Modal centrado para deploy/scale/rollback (input + botones, teclado + mouse).
- Helper de hit-testing de botones reutilizable.

**Fuera (sin cambios):**
- Flujo de deploy en vivo (pestaña Events).
- `internal/core` y `internal/providers`.
- El selector de contexto (su overlay ya existe; este modal sigue el mismo patrón pero es
  independiente).

## Pruebas

- **Helper de botones**: `buttonAtColumn` devuelve el índice correcto por rangos y `-1` en
  separadores/fuera.
- **Details**: `DetailsView` contiene los tres botones con su tecla; en read-only muestra el
  aviso y no botones activos.
- **app (anclado al render)**:
  - Click en `[ Deploy (d) ]` de Details abre el modal (`focus == focusActionModal`, kind deploy).
  - En el modal: click en `[ Cancel ]` cierra (vuelve a sidebar/panel, sin ejecutar); click en
    `[ Deploy ]` con input válido ejecuta (devuelve cmd); click fuera del modal cancela.
  - Teclado en el modal sigue funcionando (teclear tag → `ready()`; enter ejecuta; esc cancela).
  - El modal traga clicks que caen fuera de sus botones pero dentro del overlay (no muta el
    sidebar/panel por debajo).
- `go test ./...`, `go build ./...`, `go vet ./...` en verde.
