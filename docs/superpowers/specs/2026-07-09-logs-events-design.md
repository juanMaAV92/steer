# Hito 03b: LOGS + EVENTS (paridad CLI ↔ TUI) — Diseño

**Fecha:** 2026-07-09 · **Estado:** aprobado
**Objetivo:** cerrar los dos gaps de paridad doble de `docs/parity.md` (notas 3-4): la
pestaña Logs del panel deja de ser stub y gana su comando CLI (`service logs`), y la
pestaña Events se puebla también en reposo con su comando CLI (`service events`).
Todo de **solo lectura** — no se añade ningún write path.

## Decisiones (votadas)

1. **Alcance de logs: tail + follow.** Últimas N líneas y opcionalmente seguirlas en
   vivo (`-f`). Sin rangos de tiempo (`--since`) ni filtros server-side — para filtrar
   ya existe grep. Es el 90% del caso de uso ("¿qué está pasando en mi servicio?").
2. **Todos los contenedores, mezclados.** Un solo stream ordenado por tiempo; si la
   task tiene más de un contenedor (app + sidecars), cada línea lleva el prefijo del
   contenedor (patrón kubectl/copilot). Con un solo contenedor no hay prefijo.
3. **Pestaña Logs con follow en vivo.** Al entrar a la pestaña carga las últimas líneas
   y sigue llegando lo nuevo mientras esté visible (polling, mismo espíritu que el
   watch de deploy). Al salir de la pestaña o cambiar de servicio/contexto, se detiene.
4. **Auto-discovery del origen de logs.** El provider AWS lee la task definition del
   servicio: el driver `awslogs` declara log group y stream prefix. **Cero config en
   `steer.toml`** — coherente con el target del producto (devs que no dominan cloud) y
   con el hito 08. Driver no soportado (firelens, splunk…) → mensaje claro, no error
   crudo. Un bloque `[contexts.*.logs]` de override queda explícitamente fuera de
   alcance: se puede añadir después sin romper nada.

## Capacidad core (agnóstica)

En `internal/core/core.go`, interface hermana de `Deployer` y `Registry` (denominador
común de CloudWatch Logs / Cloud Logging / Log Analytics):

```go
// LogLine es una línea de log de un servicio.
type LogLine struct {
	At        time.Time
	Container string // vacío si la task tiene un solo contenedor
	Message   string
}

// LogPage es un lote de líneas + cursor opaco para continuar leyendo.
type LogPage struct {
	Lines  []LogLine // orden cronológico ascendente
	Cursor string
}

// LogSource lee logs de un servicio (CloudWatch Logs / Cloud Logging / Log Analytics).
type LogSource interface {
	// TailLogs devuelve las últimas `limit` líneas del servicio.
	TailLogs(ctx context.Context, service string, limit int) (LogPage, error)
	// FollowLogs devuelve las líneas posteriores al cursor.
	FollowLogs(ctx context.Context, service string, cursor string) (LogPage, error)
}

// ErrNoLogSource indica que el servicio no expone logs legibles por steer
// (driver de logs no soportado o sin logConfiguration).
var ErrNoLogSource = errors.New("no log source for this service")
```

- El **cursor es opaco**: lo define cada provider. Evita acoplar el contrato a
  timestamps/IDs de CloudWatch; el follow de CLI y TUI es "llama a `FollowLogs` con el
  último cursor cada N segundos".
- `coretest` gana `FakeLogSource` (páginas y cursores fijados, contadores de llamadas,
  error inyectable) — espejo de `FakeRegistry`.
- **Events no toca el core**: `Deployer.ServiceEvents` ya existe y devuelve lo necesario.

## Provider AWS

- `aws.Provider` gana `Logs() (core.LogSource, error)` memoizado (mismo patrón que
  `Deployer()`/`Registry()`, sobre la sesión cacheada del bundle). Implementación en
  `internal/providers/aws/cwlogs.go`.
- **Discovery** (por servicio): `DescribeServices` → task definition ARN →
  `DescribeTaskDefinition` → `logConfiguration` de cada contenedor. Driver `awslogs` →
  extrae `awslogs-group` y `awslogs-stream-prefix`. El mapping
  servicio→(grupo, prefijo, contenedor) se **cachea por ARN de task def**: se
  re-resuelve solo cuando el servicio apunta a una revisión distinta (los deploys
  registran revisión nueva; el log group casi nunca cambia).
- **Lectura**: `FilterLogEvents` por grupo con
  `logStreamNamePrefix = "{prefix}/{container}/"` — así no se mezclan logs de otros
  servicios que compartan grupo. Con varios contenedores, una consulta por contenedor y
  merge por timestamp; `LogLine.Container` se rellena solo si hay más de uno.
- **Cursor**: detalle interno del provider (última marca temporal + dedup de IDs de
  evento en el borde del timestamp). El contrato solo exige que `FollowLogs(cursor)` no
  repita ni pierda líneas.
- Ningún contenedor con driver `awslogs` → `ErrNoLogSource` envuelto con el driver
  encontrado (p. ej. "logs use the firelens driver — steer can't read them yet").
- Errores de permisos IAM (`logs:FilterLogEvents`) → mensaje que enseña el remedio,
  patrón del hito 08 (`internal/providers/aws/errors.go`).

## CLI

Dos subcomandos nuevos bajo `steer service`; respetan `--context`/`STEER_CONTEXT`.

**`steer service logs -s <servicio> [-f] [-n N]`**
- Sin `-f`: imprime las últimas N líneas (default 100) y termina.
- Formato: `HH:MM:SS  mensaje`; con más de un contenedor, `HH:MM:SS  [envoy]  mensaje`.
- Con `-f`: tras el tail inicial, `FollowLogs` cada 3s hasta Ctrl-C (mismo espíritu que
  `deploy -w`).
- `ErrNoLogSource` → mensaje explicativo; errores IAM → remedio accionable.

**`steer service events -s <servicio>`**
- Tabla `TIME · MESSAGE` con los últimos 20 eventos en orden cronológico ascendente (lo
  más reciente abajo, junto al prompt). Eventos con `IsError` en rojo.
- Sin flags en v1: es una vista de un vistazo, no una herramienta de arqueología.

## TUI

**Pestaña Events (cierre barato).** Al seleccionar un servicio (o entrar a la pestaña),
se cargan los `ServiceEvents` async y se pueblan como `HH:MM:SS mensaje` (errores en
rojo); se refrescan con el tick de 15s existente mientras la pestaña esté visible.
**Durante un rollout el comportamiento actual no cambia**: el feed en vivo del deploy es
dueño de la pestaña; al terminar y cambiar de selección se vuelve al modo histórico.
`panel.Events` ya es un viewport genérico — solo cambia quién lo alimenta.

**Pestaña Logs (deja de ser stub).** Nueva `panel.Logs` con viewport (patrón
`panel.Events`):
- Al entrar a la pestaña con un servicio seleccionado: "loading logs…" →
  `TailLogs(100)` → follow con tick de 3s (patrón `deployTickCmd`) mientras la pestaña
  esté visible y el servicio no cambie.
- Cambiar de servicio, pestaña o contexto detiene el follow y resetea el contenido.
- **Auto-scroll inteligente**: las líneas nuevas hacen `GotoBottom` solo si ya estabas
  abajo; si el usuario scrolleó arriba para leer, no se le roba la posición.
- Estados: `ErrNoLogSource` → línea tenue explicativa · error del cloud → mensaje
  accionable · sin líneas → "no logs yet".

## Errores

- Logs y events **nunca bloquean** SERVICES ni el resto del panel (regla del hito
  images): sus errores se muestran en su pestaña/comando.
- `ErrNoLogSource` distingue "este servicio no expone logs legibles" (hint explicativo)
  de un error real del cloud (mensaje del provider con remedio).

## Fuera de alcance

`--since` y filtros de patrón (grep ya existe); override de config `[contexts.*.logs]`;
streaming real con `StartLiveTail` (polling es suficiente y más simple); logs de tasks
paradas (execution failures); selector de contenedor (`--container`); providers
GCP/Azure (la interface los admite; `IsImplemented` sigue siendo aws).

## Pruebas

- **Core/coretest**: `FakeLogSource` con páginas/cursores fijados y error inyectable;
  contrato de orden ascendente.
- **Provider AWS** (fixtures de task def): awslogs simple; multi-contenedor (merge por
  timestamp + prefijo de contenedor); driver no soportado → `ErrNoLogSource`; sin
  `logConfiguration`; dedup del cursor en el borde del timestamp; scoping por
  `logStreamNamePrefix`; cache de discovery invalidada al cambiar la revisión.
- **CLI**: `service logs` (incluye `-n` y caso `ErrNoLogSource`) y `service events` con
  `runRootWithFake`.
- **TUI** (tests anclados al render): pestaña Logs muestra loading/líneas/error; follow
  añade líneas; reset al cambiar servicio; Events histórico se puebla al seleccionar; el
  feed de deploy queda intacto (los tests actuales de deploy no se tocan).

## Al cerrar el hito

Actualizar `docs/parity.md` (notas 3-4 → ✅ en la matriz) y el roadmap
(`docs/superpowers/plans/2026-06-15-roadmap.md`), como marca la convención.
