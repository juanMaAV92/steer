# Sidebar escalable (scroll · colapsables · filtro) — Diseño

**Fecha:** 2026-07-05 · **Estado:** aprobado
**Objetivo:** que el sidebar aguante listas grandes (16 servicios hoy; decenas de repos ECR
mañana) sin degradar la UX: secciones colapsables, filtro en vivo y scroll con indicadores.
Todo navegable por teclado y **todo clickeable** (headers e items).

## Decisiones (votadas)

1. **Filtro `/` substring en vivo** (no fuzzy — evolucionable después): `/` abre un
   mini-input en el header (`SERVICES /aud▌ (1/16)`); cada tecla filtra case-insensitive;
   `esc` limpia y sale; `enter` fija el filtro y devuelve el control a la lista. Mientras
   se teclea, las teclas globales (`d`, `q`, `c`…) NO disparan (estado de captura).
2. **Cursor navega headers e items** (modelo lazygit): `j/k` recorre headers y servicios
   (salta blancos/stubs); con el cursor en un header, `enter`/`space` togglea ▸/▾; click
   en header togglea; click en item selecciona.
3. **Scroll con contadores**: ventana que sigue al cursor; líneas tenues `↑ N more` /
   `↓ N more` en los bordes cuando hay contenido fuera de vista; la **rueda sobre el
   sidebar scrollea el sidebar** (antes solo el panel).

## Modelo

- **Filas como fuente única**: `sidebar.rows(focused)` devuelve la lista de filas
  renderizadas con su entrada asociada (`header | service | nil` para blancos/stubs/
  indicadores). `view()` une las líneas; el hit-testing indexa la misma lista — render y
  mouse no pueden divergir por construcción.
- **Colapso**: `collapsed map[sidebarSection]bool`, en memoria (no persiste). Default:
  `SERVICES ▾` expandida; `IMAGES ▸`/`DATABASES ▸` colapsadas (ocultan "coming soon").
  El header muestra ▸/▾ y el count a la derecha (`(16)`, o `(3/16)` con filtro activo).
- **Cursor vs selección**: el cursor es un índice sobre las entradas navegables; la
  **selección** es el último servicio sobre el que pasó el cursor (o click). Pasar por un
  header NO cambia la selección → el panel derecho no parpadea. `d/s/R` siguen actuando
  sobre el servicio seleccionado.
- **Filtro**: se aplica a los items de la sección (hoy solo SERVICES tiene). Si el
  servicio seleccionado queda fuera del filtro, la selección pasa al primer visible.
- **Scroll**: offset sobre las filas renderizadas; ventana de alto `bodyH`; cuando se
  recorta arriba/abajo, la primera/última fila visible se reemplaza por el indicador
  `↑ N more`/`↓ N more` (no navegable). El cursor siempre queda dentro de la ventana.
  Rueda del mouse sobre el sidebar: ±3 filas.

## Integración (app.go)

- keyMap gana `Filter` (`/`) y `Space`; el modo filtro se rutea ANTES del switch global
  (mismo patrón de captura que los overlays; los overlays tienen precedencia).
- `handleMouse` zona sidebar: mapea la fila visible (con scroll/indicadores) a su entrada —
  header → toggle; service → seleccionar; nil → no-op. Rueda en zona sidebar → scroll.
- `enter`/`space` con foco en sidebar y cursor sobre header → toggle.
- Preparado para registry: cuando IMAGES tenga repos reales, serán items de su sección —
  cero rediseño (la selección de repo alimentará el panel derecho).

## Fuera de alcance

Fuzzy matching; persistencia del colapso entre sesiones; filas reales de IMAGES/DATABASES
(hito registry); tag-picker del modal de deploy (hito registry); filtro por sección
múltiple (un solo filtro activo).

## Pruebas

- Unit: `rows()` (estructura con colapso/filtro/scroll), toggle, semántica cursor/selección
  (pasar por header no cambia selección), filtro (vivo, esc, enter, captura de teclas),
  ventana de scroll (indicadores, cursor-follow, límites).
- Anclados al render: click en header togglea; click en item con scroll desplazado
  selecciona el correcto; los 8 clicks existentes siguen pasando.
- Comportamiento intacto fuera del sidebar: overlays, deploy, switch, panel.
