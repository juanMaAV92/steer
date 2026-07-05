# Service resize (CPU/memoria) — Diseño

**Fecha:** 2026-07-05 · **Estado:** aprobado
**Objetivo:** actualizar CPU y memoria de un servicio desde CLI y TUI (hito 02c). En los
tres clouds objetivo el cambio de recursos es "nueva revisión + rollout" — el mismo
mecanismo del deploy — así que resize reutiliza watch, detección de atasco y rollback.

## Decisiones (votadas)

1. **Nivel task / Fargate en v1.** CPU y memoria a nivel de task definition — el modelo
   que mapea 1:1 a Container Apps y Cloud Run. Un servicio sin `cpu`/`memory` de task
   (EC2 launch type) recibe un error claro: `task-level resources not set — EC2 launch
   type not supported yet`. El nivel por-contenedor queda fuera de alcance.
2. **Picker de dos pasos en el TUI.** Primero el tier de CPU, y la lista de memorias se
   recalcula mostrando SOLO las válidas para ese tier: los combos inválidos no existen
   en la UI (guardrails > potencia). En CLI, inputs libres con validación que enseña.

## Capacidad core (agnóstica)

```go
// Resources son los recursos de cómputo de un servicio, en unidades agnósticas.
type Resources struct {
	CPUMilli  int // mili-vCPU: 1000 = 1 vCPU
	MemoryMiB int
}

// ResourceOption es un tier de CPU con sus memorias válidas.
type ResourceOption struct {
	CPUMilli  int
	MemoryMiB []int
}
```

`Deployer` gana:

```go
// Resize registra una nueva revisión con los recursos y actualiza el servicio
// (rollout). El rollback existente revierte también los recursos.
Resize(ctx context.Context, service string, res Resources, log StepLogger) error
// ResourceOptions devuelve la tabla de combos válidos del provider, ordenada
// por CPU ascendente. Estática (no consulta el cloud).
ResourceOptions() []ResourceOption
```

`ServiceStatus` gana `Resources Resources` (cero = desconocido/EC2). Coste cero de
llamadas: `ListServices` ya describe la task definition por servicio para el tag; se
leen `Cpu`/`Memory` del mismo response.

**Unidades:** el mapeo ECS↔mili-vCPU es exacto en ambas direcciones para los tiers
Fargate (256 unidades = 250m, 512 = 500m, 1024 = 1000m, 2048 = 2000m, 4096 = 4000m).
Display humano: `0.25 vCPU`, `1 GB` (helpers en `render`: `CPU(milli)`, `Mem(mib)`).

`coretest.FakeDeployer` gana `ResourcesValue`, `ResizeCalls []string`
("service/cpuMilli/memMiB"), `ResizeErr`, y una tabla de opciones fija.

## Provider ECS

- **Tabla Fargate v1** (los 5 tiers clásicos; 8/16 vCPU de plataformas nuevas quedan
  documentados como fuera de alcance):
  - 250m → 512, 1024, 2048 MiB
  - 500m → 1024–4096 MiB (pasos de 1024)
  - 1000m → 2048–8192 MiB (pasos de 1024)
  - 2000m → 4096–16384 MiB (pasos de 1024)
  - 4000m → 8192–30720 MiB (pasos de 1024)
- **`Resize`** comparte con `Deploy` el helper de clonar-y-registrar la task definition
  (extraerlo si hoy está inline): `currentTaskDef` → copiar TODO (containers, roles,
  volúmenes, requires…) → `Cpu`/`Memory` nuevos → `RegisterTaskDefinition` →
  `UpdateService`. El test crítico verifica que el clone preserva los demás campos.
- Task def sin `Cpu` o `Memory` → el error de EC2 de la decisión 1.
- `Resize` valida contra `ResourceOptions()` antes de tocar el cloud (defensa del
  provider; CLI y TUI validan antes para la UX).

## TUI

- **Details** muestra la fila de recursos (`cpu 0.5 vCPU · mem 1 GB`; "—" si
  desconocido) y la fila de acciones gana `[ Resize (z) ]` (`z`; `r` es Refresh).
- **Formulario inline** kind `actionResize`, con la geometría fuente-única del form:
  - Dos campos-picker horizontales: `cpu:` (tiers) y `memory:` (válidas del tier).
  - **↑↓ cambia de campo** (cpu → memory → botones y vuelta), **←→ cambia el valor**
    del campo activo (con wrap) o mueve el foco entre botones cuando el campo activo
    es la fila de botones. **Click directo** en cualquier valor lo selecciona (misma
    técnica de hit-testing por etiqueta del resto de la UI).
  - Al cambiar el tier de CPU se recalculan las memorias; si la seleccionada no es
    válida en el nuevo tier, salta a la válida más cercana.
  - `● now` marca los valores actuales del servicio; el formulario abre preseleccionado
    en el combo actual.
  - Sin recursos conocidos (EC2/desconocido): el form no abre; notice con el error.
- **Confirmar** emite `actionConfirmedMsg{kind: actionResize, resources}` y entra al
  flujo vivo del deploy: pestaña Events, poll, detección de atasco (si no hay capacidad,
  "unable to place a task" ya se atrapa), rollback con `R` revierte.
- Guard writable, click-fuera-no-op, esc restaura TAGS, captura de teclado: heredados.

## CLI

```
steer service resize -s api --cpu 0.5 --memory 2GB [-y] [-w]
```

- `--cpu`: vCPU decimal (`0.5`, `1`) o mili (`500m`). `--memory`: MiB (`2048`) o
  human (`2GB`, `512MB`).
- Validación con error que enseña: `cpu 0.5 vCPU supports 1GB-4GB; got 8GB` (lista
  derivada de `ResourceOptions`, no hardcodeada en el mensaje).
- Preview `cpu 0.25→0.5 vCPU · mem 1→2 GB` + confirmación (salvo `-y`); guard writable.
- `-w` reutiliza `watchRollout` tal cual (atasco incluido).
- `service status` muestra columnas CPU/MEM.

## Errores

- Combo inválido: bloquea con el mensaje que enseña (CLI) / no es representable (TUI).
- EC2/sin recursos: error claro en ambos frentes, sin tocar el cloud.
- Rollout atascado por falta de capacidad: lo cubre la detección existente.

## Fuera de alcance

Recursos por contenedor (EC2); tiers Fargate de 8/16 vCPU; autoscaling/políticas;
ephemeral storage; persistir historial de resizes.

## Pruebas

- Core/render: parsing de unidades CLI (casos válidos e inválidos), display humano,
  mapeo mili↔unidades ECS exacto.
- Provider: tabla de combos; clone preserva containers/roles/volúmenes y solo cambia
  cpu/memoria; EC2 sin recursos → error; validación del provider.
- TUI ancladas al render: navegación ↑↓/←→ de los pickers, recálculo de memorias al
  cambiar tier, `● now`, click en valor, confirmar → Events + poll, read-only bloquea.
  Los tests de click existentes pasan sin modificar.
- CLI: resize feliz con preview, combo inválido con mensaje que enseña, `-y`/`-w`,
  EC2 error, status con columnas nuevas.
- Paridad: ambas superficies en este hito (regla de `docs/parity.md`).
