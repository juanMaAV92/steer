# Pulido visual del TUI (estilo mockup) — Diseño

**Fecha:** 2026-07-04 · **Estado:** aprobado
**Objetivo:** acercar el render al mockup de referencia — más estilizado y moderno — sin
perder sencillez ni cambiar ningún flujo/tecla/click.

## Decisiones (votadas)

1. **Marco único** (adiós a las dos cajas con borde):
   - Fila 0: top bar. Fila 1: **regla horizontal** tenue (`─`). Cuerpo. Regla horizontal
     tenue. Última fila: help bar.
   - **Divisor vertical** tenue (`│`, gris 240) entre sidebar y panel — siempre igual, no
     cambia con el foco.
   - **Foco**: el header `SERVICES` se pinta **cian de marca** cuando el foco está en el
     sidebar y dim cuando no. En el panel, la pestaña activa ya se subraya en cian.
     Regla mental: "SERVICES apagado ⇒ el foco está en el panel".
   - Modo una-columna: regla horizontal entre las dos zonas apiladas.

2. **Compacto con padding**: una fila por servicio (como hoy). El aire viene de:
   padding izquierdo de 1 col en ambos bloques (compensa la columna que ocupaba el borde,
   así las X del contenido del sidebar no cambian), línea en blanco entre el header
   SERVICES y la primera fila, y el espaciado de secciones existente.

3. **Icono en el top bar**: `⛵ steer · aws · nao-dev (cluster: …)` a la izquierda y
   `writable ●`/`read-only ○` **alineado a la derecha** (relleno calculado con
   `lipgloss.Width`, robusto al ancho del emoji). Separadores `·` (fuera el `—`).

4. **Headers de sección**: `SERVICES` + count `(n)` alineado a la derecha del sidebar;
   `IMAGES (ECR)` y `DATABASES` con `···` a la derecha; los stubs pasan de
   "(próximamente)" a **"coming soon"** (corrige la convención de UI en inglés).

## Sin cambios

Botones `[ Deploy (d) ]…`, contenido de pestañas, overlays (picker/modal), flujos de
deploy/switch, teclas, y el contenido del help bar (sin `⌘k` hasta que exista la paleta).

## Geometría (impacto en mouse)

- **Vertical: sin cambios** — la regla horizontal ocupa la misma fila (Y=1) que ocupaba el
  borde superior de las cajas; el contenido sigue empezando en Y=2.
- **Sidebar X: sin cambios** — el `PaddingLeft(1)` del bloque compensa la columna del
  borde eliminado (contenido sigue en X=1+).
- **Sidebar filas**: la línea en blanco tras el header desplaza los servicios una fila
  (`HitAtRow`: servicios en filas 2..n+1). El test de click anclado al render recalibra
  solo; `HitAtRow`/`TestHitAtRow` se actualizan explícitamente.
- **Panel X: −1** — antes el contenido empezaba en `sidebarW+3` (dos bordes + inner);
  ahora divisor en `X=sidebarW` y contenido en `sidebarW+2`. Las dos constantes de
  producción (`panelContentX0` para pestañas y botones de Details) se actualizan; los
  tests anclados al render son el guard.

## Invariantes

- Comportamiento observable idéntico salvo lo puramente visual.
- Tests anclados al render no se debilitan (derivan coordenadas del render real).
- `render` hoja; comentarios español, UI strings inglés.
