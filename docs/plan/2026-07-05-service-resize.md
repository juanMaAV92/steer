# Service resize (CPU/memoria) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Actualizar CPU/memoria de un servicio desde TUI (picker de dos pasos con combos válidos) y CLI (`service resize` con validación que enseña), reutilizando el flujo vivo del deploy (rollout + watch + atasco + rollback).

**Architecture:** `Deployer` gana `Resize` y `ResourceOptions`; en ECS, `Resize` comparte con `Deploy` un helper extraído `registerRevision` (clonar task def + registrar + apuntar servicio). `ServiceStatus` gana `Resources` leyendo Cpu/Memory de la misma DescribeTaskDefinition que ya trae el tag. El TUI añade el kind `actionResize` al formulario inline con dos filas-picker; el CLI, el subcomando con parsing de unidades humanas.

**Tech Stack:** Go, aws-sdk-go-v2/ecs, Bubble Tea, Cobra, testify.
Spec: `docs/superpowers/specs/2026-07-05-service-resize-design.md`.

## Global Constraints

- Comentarios en español; strings de UI en inglés.
- PROHIBIDO cualquier atribución a Claude/IA en commits, comentarios o PRs (sin trailer Co-Authored-By).
- Branch de trabajo: `feat/service-resize` (creada en Task 1 desde main).
- Antes de CADA commit: `gofmt -w internal/ && golangci-lint run && go vet ./... && go test ./... -count=1 && go build ./...` — todo verde.
- Unidades agnósticas: mili-vCPU (1000 = 1 vCPU) y MiB. Mapeo ECS exacto: `unidades = milli*1024/1000`, `milli = unidades*1000/1024` (250↔256, 500↔512, 1000↔1024, 2000↔2048, 4000↔4096).
- Tabla Fargate v1 (5 tiers): 250m→{512,1024,2048} · 500m→{1024..4096 paso 1024} · 1000m→{2048..8192} · 2000m→{4096..16384} · 4000m→{8192..30720}.
- Servicio sin recursos de task (EC2): error `task-level resources not set — EC2 launch type not supported yet`; nunca tocar el cloud.
- El clone de task def preserva TODO salvo Cpu/Memory (test crítico); el refactor de Deploy no cambia su comportamiento (sus tests pasan sin modificar).
- Tests de TUI anclados al render; los tests de click existentes pasan sin modificar salvo los que el corrimiento de `DetailsButtonLine` (7→8, fila nueva de recursos) obligue — y esos derivan coordenadas del render, así que NO deberían cambiar; solo `DetailsButtonLine` y su test de validación se actualizan.

---

### Task 1: Contratos — core.Resources/ResourceOption, Deployer ampliado, helpers de display, fake

**Files:**
- Modify: `internal/core/core.go`
- Modify: `internal/core/coretest/fake.go`
- Modify: `internal/render/human.go`
- Modify: `internal/render/human_test.go`

**Interfaces:**
- Produces (T2-T4 dependen):
  - `core.Resources{CPUMilli, MemoryMiB int}`; `core.ResourceOption{CPUMilli int; MemoryMiB []int}`
  - `Deployer` gana `Resize(ctx, service string, res Resources, log StepLogger) error` y `ResourceOptions() []ResourceOption`
  - `ServiceStatus` gana `Resources Resources` (cero = desconocido)
  - `render.CPULabel(milli int) string` ("0.25 vCPU", "1 vCPU"); `render.MemLabel(mib int) string` ("512 MB", "0.5 GB"→ver código, "2 GB")
  - `coretest.FakeDeployer`: `ResourcesValue core.Resources`, `ResizeErr error`, `ResizeCalls []string` ("service/milli/mib"), `ResourceOptions()` con tabla fija de test.

- [ ] **Step 1: Crear la branch**

```bash
git checkout main && git pull && git checkout -b feat/service-resize
```

- [ ] **Step 2: Tests que fallan — labels**

Añadir a `internal/render/human_test.go`:

```go
func TestCPULabel(t *testing.T) {
	require.Equal(t, "0.25 vCPU", CPULabel(250))
	require.Equal(t, "0.5 vCPU", CPULabel(500))
	require.Equal(t, "1 vCPU", CPULabel(1000))
	require.Equal(t, "4 vCPU", CPULabel(4000))
}

func TestMemLabel(t *testing.T) {
	require.Equal(t, "512 MB", MemLabel(512))
	require.Equal(t, "1 GB", MemLabel(1024))
	require.Equal(t, "1.5 GB", MemLabel(1536))
	require.Equal(t, "30 GB", MemLabel(30720))
}
```

- [ ] **Step 3: Verificar que fallan**

Run: `go test ./internal/render/ -run 'TestCPULabel|TestMemLabel' -v`
Expected: FAIL con "undefined: CPULabel"

- [ ] **Step 4: Implementar contratos**

En `internal/core/core.go` (junto a ServiceStatus):

```go
// Resources son los recursos de cómputo de un servicio, en unidades agnósticas.
type Resources struct {
	CPUMilli  int // mili-vCPU: 1000 = 1 vCPU
	MemoryMiB int
}

// ResourceOption es un tier de CPU con sus memorias válidas, ordenadas ascendente.
type ResourceOption struct {
	CPUMilli  int
	MemoryMiB []int
}
```

`ServiceStatus` gana el campo:

```go
	Resources Resources // recursos de la task; cero = desconocido (p. ej. EC2)
```

`Deployer` gana (tras Rollback):

```go
	// Resize registra una nueva revisión con los recursos y actualiza el servicio
	// (rollout). El Rollback existente revierte también los recursos.
	Resize(ctx context.Context, service string, res Resources, log StepLogger) error
	// ResourceOptions devuelve la tabla de combos válidos del provider, por CPU
	// ascendente. Estática: no consulta el cloud.
	ResourceOptions() []ResourceOption
```

En `internal/render/human.go`:

```go
// CPULabel formatea mili-vCPU en unidades humanas ("0.25 vCPU", "1 vCPU").
func CPULabel(milli int) string {
	return strconv.FormatFloat(float64(milli)/1000, 'f', -1, 64) + " vCPU"
}

// MemLabel formatea MiB en la unidad natural ("512 MB", "1.5 GB", "2 GB").
func MemLabel(mib int) string {
	if mib < 1024 {
		return strconv.Itoa(mib) + " MB"
	}
	return strconv.FormatFloat(float64(mib)/1024, 'f', -1, 64) + " GB"
}
```

En `internal/core/coretest/fake.go`:

```go
	ResourcesValue core.Resources // devuelto en ServiceStatus.Resources vía Services
	ResizeErr      error
	ResizeCalls    []string // "service/cpuMilli/memMiB"
```

```go
func (f *FakeDeployer) Resize(_ context.Context, service string, res core.Resources, log core.StepLogger) error {
	if log != nil {
		log("resizing")
	}
	f.ResizeCalls = append(f.ResizeCalls,
		fmt.Sprintf("%s/%d/%d", service, res.CPUMilli, res.MemoryMiB))
	return f.ResizeErr
}

// ResourceOptions: tabla pequeña y fija para tests (2 tiers).
func (f *FakeDeployer) ResourceOptions() []core.ResourceOption {
	return []core.ResourceOption{
		{CPUMilli: 250, MemoryMiB: []int{512, 1024, 2048}},
		{CPUMilli: 500, MemoryMiB: []int{1024, 2048, 3072, 4096}},
	}
}
```

(fake.go gana el import `"fmt"`.)

- [ ] **Step 5: Verificar que pasan + gates + commit**

Run: `go test ./... -count=1` → PASS. NOTA: la interface crece — el compilador señalará
cualquier otro implementor de `core.Deployer`; hoy solo existen `ECSDeployer` (T2 lo
implementa; para que T1 compile, añadir en `internal/providers/aws/ecs.go` stubs
mínimos que T2 reemplaza NO es aceptable — en su lugar, T1 añade la implementación
REAL de `ResourceOptions` (la tabla, trivial) y un `Resize` que devuelve
`fmt.Errorf("not implemented")`; T2 lo completa) y `FakeDeployer` (arriba). Si algún
test de cli/tui usa un deployer propio embebiendo FakeDeployer, hereda los métodos.
Grep `core.Deployer` por otros implementors directos (p. ej. `cmd/steerdemo`) y darles
los dos métodos con el mismo patrón mínimo si el compilador los señala.

En `internal/providers/aws/ecs.go` (T1 deja esto compilando; T2 lo completa):

```go
// fargateOptions: tiers clásicos de Fargate (los de 8/16 vCPU quedan fuera de v1).
var fargateOptions = []core.ResourceOption{
	{CPUMilli: 250, MemoryMiB: []int{512, 1024, 2048}},
	{CPUMilli: 500, MemoryMiB: memRange(1024, 4096)},
	{CPUMilli: 1000, MemoryMiB: memRange(2048, 8192)},
	{CPUMilli: 2000, MemoryMiB: memRange(4096, 16384)},
	{CPUMilli: 4000, MemoryMiB: memRange(8192, 30720)},
}

// memRange genera memorias válidas de from a to en pasos de 1 GiB.
func memRange(from, to int) []int {
	var out []int
	for m := from; m <= to; m += 1024 {
		out = append(out, m)
	}
	return out
}

// ResourceOptions devuelve la tabla Fargate.
func (d *ECSDeployer) ResourceOptions() []core.ResourceOption { return fargateOptions }

// Resize se completa en la siguiente task del plan.
func (d *ECSDeployer) Resize(context.Context, string, core.Resources, core.StepLogger) error {
	return fmt.Errorf("resize: not implemented yet")
}
```

```bash
gofmt -w internal/ && golangci-lint run && go vet ./... && go test ./... -count=1 && go build ./...
git add internal/core/ internal/render/ internal/providers/aws/ecs.go
git commit -m "feat(core,render): contrato Resize/ResourceOptions y labels de recursos"
```

---

### Task 2: ECS — Resize real, registerRevision compartido y Resources en ServiceStatus

**Files:**
- Modify: `internal/providers/aws/ecs.go`
- Modify: `internal/providers/aws/ecs_test.go`

**Interfaces:**
- Consumes: `core.Resources/ResourceOption`, `fargateOptions` (T1); `currentTaskDef`, `Deploy` (`ecs.go:256-305`), `tagForTaskDef` (`ecs.go:139-152`), `ListServices` (`ecs.go:42-80`).
- Produces: `registerRevision(ctx, service, log, mutate)` (privado); `Resize` real; `ServiceStatus.Resources` poblado por `ListServices`.

- [ ] **Step 1: Tests que fallan**

Añadir a `internal/providers/aws/ecs_test.go` (usa el fake ECS del archivo; revisar su
forma actual — captura `RegisterTaskDefinitionInput` o añadir el campo de captura si
no existe, siguiendo el patrón de los tests de Deploy existentes):

```go
func TestResizeRegistersRevisionPreservingEverything(t *testing.T) {
	// task def actual con Cpu/Memory y campos que DEBEN sobrevivir al clone
	api := newFakeECSWithTaskDef(ecstypes.TaskDefinition{
		Family: awssdk.String("nao-api"),
		Cpu:    awssdk.String("256"), Memory: awssdk.String("512"),
		ContainerDefinitions: []ecstypes.ContainerDefinition{
			{Image: awssdk.String("repo/api:v1"), Name: awssdk.String("app")},
		},
		ExecutionRoleArn:        awssdk.String("arn:exec"),
		TaskRoleArn:             awssdk.String("arn:task"),
		NetworkMode:             ecstypes.NetworkModeAwsvpc,
		RequiresCompatibilities: []ecstypes.Compatibility{ecstypes.CompatibilityFargate},
	})
	d := newDeployer(api, "cluster")
	var steps []string
	err := d.Resize(context.Background(), "svc", core.Resources{CPUMilli: 500, MemoryMiB: 1024},
		func(s string) { steps = append(steps, s) })
	require.NoError(t, err)
	in := api.lastRegisterInput
	require.Equal(t, "512", awssdk.ToString(in.Cpu))     // 500m → 512 unidades
	require.Equal(t, "1024", awssdk.ToString(in.Memory)) // MiB directo
	// el clone preserva todo lo demás
	require.Equal(t, "repo/api:v1", awssdk.ToString(in.ContainerDefinitions[0].Image))
	require.Equal(t, "arn:exec", awssdk.ToString(in.ExecutionRoleArn))
	require.Equal(t, "arn:task", awssdk.ToString(in.TaskRoleArn))
	require.Equal(t, ecstypes.NetworkModeAwsvpc, in.NetworkMode)
	require.NotEmpty(t, steps)
	require.True(t, api.updateServiceCalled) // el servicio apunta a la nueva revisión
}

func TestResizeRejectsInvalidComboAndEC2(t *testing.T) {
	api := newFakeECSWithTaskDef(ecstypes.TaskDefinition{
		Family: awssdk.String("nao-api"),
		Cpu:    awssdk.String("256"), Memory: awssdk.String("512"),
		ContainerDefinitions: []ecstypes.ContainerDefinition{{Image: awssdk.String("repo/api:v1")}},
	})
	d := newDeployer(api, "cluster")
	// combo inválido: 0.25 vCPU no soporta 8GB
	err := d.Resize(context.Background(), "svc", core.Resources{CPUMilli: 250, MemoryMiB: 8192}, nil)
	require.ErrorContains(t, err, "invalid cpu/memory combination")
	// EC2: task def sin recursos de task
	api2 := newFakeECSWithTaskDef(ecstypes.TaskDefinition{
		Family:               awssdk.String("legacy"),
		ContainerDefinitions: []ecstypes.ContainerDefinition{{Image: awssdk.String("repo/x:v1")}},
	})
	d2 := newDeployer(api2, "cluster")
	err = d2.Resize(context.Background(), "svc", core.Resources{CPUMilli: 250, MemoryMiB: 512}, nil)
	require.ErrorContains(t, err, "EC2 launch type not supported")
}

func TestListServicesIncludesResources(t *testing.T) {
	// el fake de ListServices/DescribeServices/DescribeTaskDefinition existente,
	// con Cpu "256" y Memory "512" en la task def
	// (adaptar al helper del archivo; asertar:)
	// require.Equal(t, core.Resources{CPUMilli: 250, MemoryMiB: 512}, out[0].Resources)
}
```

(`newFakeECSWithTaskDef` es un helper nuevo del test: un fake `ecsAPI` cuyo
DescribeServices devuelve un servicio con TaskDefinition arn fijo, DescribeTaskDefinition
devuelve la td dada, y captura `lastRegisterInput`/`updateServiceCalled`. Si el fake
existente del archivo ya cubre esto, extenderlo en vez de duplicar. El tercer test se
escribe COMPLETO adaptando el fixture existente de ListServices — sin pseudocódigo.)

- [ ] **Step 2: Verificar que fallan**

Run: `go test ./internal/providers/aws/ -run TestResize -v`
Expected: FAIL — "resize: not implemented yet"

- [ ] **Step 3: Implementar**

3a. Extraer el helper del cuerpo de `Deploy` (el bloque RegisterTaskDefinition +
UpdateService de `ecs.go:275-304`):

```go
// registerRevision clona la task def actual (preservando todos sus campos),
// aplica mutate sobre el input y apunta el servicio a la nueva revisión.
// Compartido por Deploy (cambia imagen) y Resize (cambia cpu/memoria).
func (d *ECSDeployer) registerRevision(ctx context.Context, service string, td *ecstypes.TaskDefinition,
	log core.StepLogger, mutate func(*ecs.RegisterTaskDefinitionInput)) error {
	step := func(msg string) {
		if log != nil {
			log(msg)
		}
	}
	containers := make([]ecstypes.ContainerDefinition, len(td.ContainerDefinitions))
	copy(containers, td.ContainerDefinitions)
	in := &ecs.RegisterTaskDefinitionInput{
		Family:                  td.Family,
		ContainerDefinitions:    containers,
		Cpu:                     td.Cpu,
		Memory:                  td.Memory,
		NetworkMode:             td.NetworkMode,
		ExecutionRoleArn:        td.ExecutionRoleArn,
		TaskRoleArn:             td.TaskRoleArn,
		RequiresCompatibilities: td.RequiresCompatibilities,
		Volumes:                 td.Volumes,
		PlacementConstraints:    td.PlacementConstraints,
		RuntimePlatform:         td.RuntimePlatform,
		EphemeralStorage:        td.EphemeralStorage,
		ProxyConfiguration:      td.ProxyConfiguration,
		PidMode:                 td.PidMode,
		IpcMode:                 td.IpcMode,
	}
	mutate(in)
	step("registering new task definition revision")
	reg, err := d.api.RegisterTaskDefinition(ctx, in)
	if err != nil {
		return err
	}
	step(fmt.Sprintf("registered %s:%d", awssdk.ToString(reg.TaskDefinition.Family), reg.TaskDefinition.Revision))
	step("updating service")
	_, err = d.api.UpdateService(ctx, &ecs.UpdateServiceInput{
		Cluster:        awssdk.String(d.cluster),
		Service:        awssdk.String(service),
		TaskDefinition: reg.TaskDefinition.TaskDefinitionArn,
	})
	return err
}
```

`Deploy` queda: leer td → validar containers → `registerRevision(..., func(in) {
in.ContainerDefinitions[0].Image = awssdk.String(replaceTag(...)) })` con su step
"reading current task definition" antes. SUS TESTS EXISTENTES PASAN SIN CAMBIOS.

3b. `Resize` real (reemplaza el stub de T1):

```go
// Resize registra una nueva revisión con los recursos dados y actualiza el servicio.
func (d *ECSDeployer) Resize(ctx context.Context, service string, res core.Resources, log core.StepLogger) error {
	if !validResources(res) {
		return fmt.Errorf("invalid cpu/memory combination: %dm / %d MiB", res.CPUMilli, res.MemoryMiB)
	}
	step := func(msg string) {
		if log != nil {
			log(msg)
		}
	}
	step("reading current task definition")
	td, err := d.currentTaskDef(ctx, service)
	if err != nil {
		return err
	}
	if awssdk.ToString(td.Cpu) == "" || awssdk.ToString(td.Memory) == "" {
		return fmt.Errorf("task-level resources not set — EC2 launch type not supported yet")
	}
	return d.registerRevision(ctx, service, td, log, func(in *ecs.RegisterTaskDefinitionInput) {
		in.Cpu = awssdk.String(strconv.Itoa(res.CPUMilli * 1024 / 1000))
		in.Memory = awssdk.String(strconv.Itoa(res.MemoryMiB))
	})
}

// validResources comprueba el combo contra la tabla Fargate.
func validResources(res core.Resources) bool {
	for _, opt := range fargateOptions {
		if opt.CPUMilli != res.CPUMilli {
			continue
		}
		for _, m := range opt.MemoryMiB {
			if m == res.MemoryMiB {
				return true
			}
		}
	}
	return false
}
```

3c. Resources en ListServices: generalizar `tagForTaskDef` a

```go
// taskDefInfo lee tag de imagen y recursos de una task def; ceros si no se puede.
func (d *ECSDeployer) taskDefInfo(ctx context.Context, tdArn string) (tag string, res core.Resources) {
	if tdArn == "" {
		return "", core.Resources{}
	}
	out, err := d.api.DescribeTaskDefinition(ctx, &ecs.DescribeTaskDefinitionInput{
		TaskDefinition: awssdk.String(tdArn),
	})
	if err != nil || out == nil || out.TaskDefinition == nil {
		return "", core.Resources{}
	}
	td := out.TaskDefinition
	if len(td.ContainerDefinitions) > 0 {
		tag = tagFromImage(awssdk.ToString(td.ContainerDefinitions[0].Image))
	}
	if cpuUnits, err := strconv.Atoi(awssdk.ToString(td.Cpu)); err == nil {
		res.CPUMilli = cpuUnits * 1000 / 1024
	}
	if mib, err := strconv.Atoi(awssdk.ToString(td.Memory)); err == nil {
		res.MemoryMiB = mib
	}
	return tag, res
}
```

y en `ListServices`, el campo del status: `Tag` y `Resources` salen de una sola llamada
(`tag, res := d.taskDefInfo(...)`). Eliminar `tagForTaskDef` (o dejarlo como wrapper si
otro sitio lo usa — grep primero). `ecs.go` gana el import `"strconv"` si falta.

- [ ] **Step 4: Verificar que pasan + gates + commit**

```bash
gofmt -w internal/ && golangci-lint run && go vet ./... && go test ./... -count=1 && go build ./...
git add internal/providers/aws/
git commit -m "feat(providers): Resize ECS con registerRevision compartido y Resources en el status"
```

---

### Task 3: TUI — fila de recursos, botón Resize y formulario de dos pickers

**Files:**
- Modify: `internal/tui/panel/details.go` (fila resources; `DetailsButtonLine` 7→8; label nuevo)
- Modify: `internal/tui/panel/details_test.go`
- Modify: `internal/tui/keys.go` (binding Resize = "z")
- Modify: `internal/tui/app.go` (actionResize, openAction, applyActionConfirmed, startResizeCmd)
- Modify: `internal/tui/form.go` (estado resize + view + hit-testing)
- Modify: `internal/tui/form_test.go`
- Modify: `internal/tui/app_test.go`
- Modify: `internal/tui/commands.go` (startResizeCmd)

**Interfaces:**
- Consumes: `Deployer.Resize/ResourceOptions`, `render.CPULabel/MemLabel`, `ServiceStatus.Resources` (T1-T2); `actionForm` (buttonRow/statusRows/labels/activate), `applyActionConfirmed`, `deployStartedMsg`/flujo vivo.
- Produces: `actionResize actionKind`; `actionConfirmedMsg` gana `resources core.Resources`; `newResizeForm(service string, opts []core.ResourceOption, current core.Resources) *actionForm`; `form.resField/cpuIdx/memIdx`, `resizeRowAt(row, x) (field, idx int)`; `startResizeCmd(ctx, dep, service, res)`.

- [ ] **Step 1: Tests que fallan (form unit)**

Añadir a `internal/tui/form_test.go`:

```go
func resizeOpts() []core.ResourceOption {
	return []core.ResourceOption{
		{CPUMilli: 250, MemoryMiB: []int{512, 1024, 2048}},
		{CPUMilli: 500, MemoryMiB: []int{1024, 2048, 3072, 4096}},
	}
}

func TestResizeFormPreselectsCurrentAndNavigates(t *testing.T) {
	f := newResizeForm("api", resizeOpts(), core.Resources{CPUMilli: 500, MemoryMiB: 2048})
	require.Equal(t, 1, f.cpuIdx) // preseleccionado en el actual
	require.Equal(t, 1, f.memIdx)
	// ←→ cambia el valor del campo activo (cpu)
	f.moveResValue(1)
	require.Equal(t, 0, f.cpuIdx) // wrap: 500→250
	// al cambiar el tier, la memoria salta a la válida más cercana (2048 existe en 250)
	require.Equal(t, 2048, f.selectedResources().MemoryMiB)
	// ↑↓ cambia de campo: cpu(0) → mem(1) → botones(2)
	f.moveResField(1)
	require.Equal(t, 1, f.resField)
	f.moveResValue(1)
	require.Equal(t, core.Resources{CPUMilli: 250, MemoryMiB: 512}, f.selectedResources()) // wrap 2048→512
}

func TestResizeFormNearestMemoryOnTierChange(t *testing.T) {
	f := newResizeForm("api", resizeOpts(), core.Resources{CPUMilli: 500, MemoryMiB: 4096})
	f.resField = 0
	f.moveResValue(-1) // 500 → 250: 4096 no existe; la más cercana es 2048
	require.Equal(t, core.Resources{CPUMilli: 250, MemoryMiB: 2048}, f.selectedResources())
}

func TestResizeFormGeometryAndActivate(t *testing.T) {
	f := newResizeForm("api", resizeOpts(), core.Resources{CPUMilli: 250, MemoryMiB: 512})
	require.Equal(t, 4, f.buttonRow()) // borde(0) título(1) cpu(2) mem(3) botones(4)
	out := stripANSI(f.view())
	require.Contains(t, out, "0.25 vCPU")
	require.Contains(t, out, "512 MB")
	require.Contains(t, out, "● now")
	// activate emite el combo elegido
	f.resField = 2 // botones, foco en confirmar
	done, msg := f.activate()
	require.True(t, done)
	conf := msg.(actionConfirmedMsg)
	require.Equal(t, actionResize, conf.kind)
	require.Equal(t, core.Resources{CPUMilli: 250, MemoryMiB: 512}, conf.resources)
}
```

- [ ] **Step 2: Verificar que fallan**

Run: `go test ./internal/tui/ -run TestResizeForm -v`
Expected: FAIL con "undefined: newResizeForm"

- [ ] **Step 3: Implementar el formulario (`internal/tui/form.go`)**

Nuevo kind en app.go: `actionResize` (tras actionScale). `actionConfirmedMsg` gana
`resources core.Resources`. Campos nuevos de `actionForm`:

```go
	resOpts  []core.ResourceOption // solo kind resize
	current  core.Resources        // combo actual del servicio (marca ● now)
	resField int                   // 0=cpu, 1=memoria, 2=botones
	cpuIdx   int
	memIdx   int
```

```go
// newResizeForm crea el formulario de resize preseleccionado en el combo actual.
func newResizeForm(service string, opts []core.ResourceOption, current core.Resources) *actionForm {
	f := &actionForm{kind: actionResize, service: service, resOpts: opts,
		current: current, resField: 0, pick: -1}
	for i, o := range opts {
		if o.CPUMilli == current.CPUMilli {
			f.cpuIdx = i
		}
	}
	f.memIdx = nearestIdx(opts[f.cpuIdx].MemoryMiB, current.MemoryMiB)
	return f
}

// nearestIdx devuelve el índice del valor más cercano a target.
func nearestIdx(vals []int, target int) int {
	best, bestDiff := 0, int(^uint(0)>>1)
	for i, v := range vals {
		diff := v - target
		if diff < 0 {
			diff = -diff
		}
		if diff < bestDiff {
			best, bestDiff = i, diff
		}
	}
	return best
}

// selectedResources devuelve el combo elegido en los pickers.
func (f actionForm) selectedResources() core.Resources {
	opt := f.resOpts[f.cpuIdx]
	return core.Resources{CPUMilli: opt.CPUMilli, MemoryMiB: opt.MemoryMiB[f.memIdx]}
}

// moveResField cambia el campo activo (cpu → memoria → botones), con wrap.
func (f *actionForm) moveResField(delta int) {
	f.resField = (f.resField + delta%3 + 3) % 3
}

// moveResValue cambia el valor del campo activo con wrap; al cambiar el tier de
// CPU la memoria salta a la válida más cercana del nuevo tier.
func (f *actionForm) moveResValue(delta int) {
	switch f.resField {
	case 0:
		prevMem := f.resOpts[f.cpuIdx].MemoryMiB[f.memIdx]
		n := len(f.resOpts)
		f.cpuIdx = (f.cpuIdx + delta%n + n) % n
		f.memIdx = nearestIdx(f.resOpts[f.cpuIdx].MemoryMiB, prevMem)
	case 1:
		mems := f.resOpts[f.cpuIdx].MemoryMiB
		n := len(mems)
		f.memIdx = (f.memIdx + delta%n + n) % n
	case 2:
		f.moveFocus(delta)
	}
}
```

- `ready()`: `case actionResize: return true` (siempre hay combo válido elegido).
- `activate()`: para resize, el confirm emite
  `actionConfirmedMsg{kind: actionResize, service: f.service, resources: f.selectedResources()}`
  (mismo patrón de foco confirmar/cancelar).
- `buttonRow()`: `if f.kind == actionResize { return 4 }` antes del cálculo actual.
- `view()` para resize: título "Resize", dos filas de valores con la misma técnica del
  picker de tags — cada fila: prefijo alineado (`"cpu:    "` / `"memory: "`, ambos 8
  runas) + valores unidos por `" · "`; el valor SELECCIONADO con fondo
  `render.SelectionBarColor`; la fila del campo ACTIVO con el prefijo en
  `render.Brand`; sufijo `render.Success(" ● now")` en la fila cuya selección coincide
  con `f.current`. Botones con `ButtonsWithFocus` (labels `{"Resize (↵)", "Cancel (esc)"}` —
  añadir el caso a `labels()`).
- Hit-testing de valores:

```go
// resizeValueAt mapea (row, x) local del view a (campo, índice de valor); -1 si nada.
// Filas: cpu=2, memoria=3; los valores usan LabelAtColumn con gap 3 (" · ") y pad 0,
// tras el prefijo de 8 runas.
func (f actionForm) resizeValueAt(row, x int) (field, idx int) {
	if f.kind != actionResize {
		return -1, -1
	}
	var labels []string
	switch row {
	case 2:
		field = 0
		for _, o := range f.resOpts {
			labels = append(labels, render.CPULabel(o.CPUMilli))
		}
	case 3:
		field = 1
		for _, m := range f.resOpts[f.cpuIdx].MemoryMiB {
			labels = append(labels, render.MemLabel(m))
		}
	default:
		return -1, -1
	}
	idx = render.LabelAtColumn(labels, 0, 3, x-formContentX0-8)
	if idx < 0 {
		return -1, -1
	}
	return field, idx
}
```

(el view debe renderizar los valores EXACTAMENTE como `strings.Join(labels, " · ")`
con estilos que no cambian anchos — fuente única con LabelAtColumn.)

- [ ] **Step 4: Cablear en app.go/keys.go/commands.go/details**

- `keys.go`: `Resize: key.NewBinding(key.WithKeys("z"))`.
- `panel/details.go`: `DetailsActionLabels` gana `"Resize (z)"`; tras la fila del tag,
  nueva fila `resources` (`render.Dim("cpu/mem   —")` si `s.Resources` es cero, si no
  `"cpu/mem   " + render.Accent(render.CPULabel(...)+" · "+render.MemLabel(...))`);
  `DetailsButtonLine = 8` y su comentario; actualizar `TestDetailsButtonLineMatchesRender`
  y el test de labels si asertan el slice.
- `app.go`:
  - `actionKindFor`: índice 3 → `actionResize`.
  - `openAction`: caso Resize — requiere `s.Resources` conocido:

```go
	case key.Matches(msg, m.keys.Resize):
		if s.Resources == (core.Resources{}) {
			m.notice = "task-level resources not set — resize unavailable"
			return m, nil
		}
		m.form = newResizeForm(s.Name, m.dep.ResourceOptions(), s.Resources)
```

  (mismo bloque en `openActionKind`; ambos ya fuerzan Details/lastSelected).
  - `handleFormKey`: para `kind == actionResize`, ↑↓ (por `msg.Type`) → `moveResField(±1)`;
    ←→ → `moveResValue(∓1/±1)`; tab → `moveResField(1)`; enter → activate (solo confirma
    desde la fila de botones: si `resField != 2`, enter mueve a botones en su lugar).
  - `clickForm`: antes de `buttonAt`, `if fld, idx := m.form.resizeValueAt(row, x); fld >= 0 {`
    aplica la selección (cpu: set cpuIdx + nearest mem; mem: set memIdx; y
    `resField = fld`) `}`.
  - `applyActionConfirmed`: caso `actionResize` — flujo vivo idéntico al deploy:

```go
	if r.kind == actionDeploy || r.kind == actionResize {
		m.focus = focusPanel
		m.tabs.Active = panel.TabEvents
		m.events.Reset()
		m.deploy = deployState{Active: true, Service: r.service}
		if r.kind == actionResize {
			return startResizeCmd(m.runCtx, m.dep, r.service, r.resources)
		}
		return startDeployCmd(m.runCtx, m.dep, r.service, r.input)
	}
```

  - `commands.go`: `startResizeCmd` — espejo de `startDeployCmd` con `dep.Resize`.
  - El confirm de resize NO pasa por la validación de tag (gate `r.kind == actionDeploy`
    en handleFormKey/clickForm queda intacto; resize activa directo).

- [ ] **Step 5: Tests de integración (app_test.go)**

```go
// TestResizeFlowStartsLiveRollout: z abre el form preseleccionado; confirmar entra a Events.
func TestResizeFlowStartsLiveRollout(t *testing.T) {
	fake := &coretest.FakeDeployer{Services: []core.ServiceStatus{
		{Name: "api", Running: 1, Desired: 1, Resources: core.Resources{CPUMilli: 250, MemoryMiB: 512}},
	}}
	m := newTestModelWithDeployer(t, fake) // helper: newTestModel pero con fake inyectado; crear si no existe reutilizando el cuerpo de newTestModel
	m = mustUpdate(t, m, keyMsg("z"))
	require.NotNil(t, m.form)
	require.Equal(t, actionResize, m.form.kind)
	// ←→ sube el tier de cpu; enter desde botones confirma
	m = mustUpdate(t, m, tea.KeyMsg{Type: tea.KeyRight})
	m.form.resField = 2
	updated, cmd := m.Update(keyMsg("enter"))
	m = updated.(Model)
	require.Nil(t, m.form)
	require.NotNil(t, cmd)
	require.Equal(t, panel.TabEvents, m.tabs.Active)
	require.True(t, m.deploy.Active)
	msg := cmd().(deployStartedMsg)
	require.NoError(t, msg.err)
	require.Len(t, fake.ResizeCalls, 1)
}

// TestResizeUnavailableWithoutResources: sin recursos conocidos, z no abre el form.
func TestResizeUnavailableWithoutResources(t *testing.T) {
	m := newTestModel(sampleServices()) // sampleServices sin Resources
	m = mustUpdate(t, m, keyMsg("z"))
	require.Nil(t, m.form)
	require.Contains(t, m.notice, "resize unavailable")
}

// TestClickResizeValue: click en un valor del picker lo selecciona (anclado al render).
func TestClickResizeValue(t *testing.T) {
	fake := &coretest.FakeDeployer{Services: []core.ServiceStatus{
		{Name: "api", Running: 1, Desired: 1, Resources: core.Resources{CPUMilli: 250, MemoryMiB: 512}},
	}}
	m := newTestModelWithDeployer(t, fake)
	m = mustUpdate(t, m, keyMsg("z"))
	clickX, clickY := findInView(t, m.View(), "0.5 vCPU")
	m = mustUpdate(t, m, tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: clickX, Y: clickY})
	require.NotNil(t, m.form)
	require.Equal(t, 500, m.form.selectedResources().CPUMilli)
}
```

(Nota: los 8+ tests de click existentes derivan coordenadas del render; la fila nueva
de Details los desplaza pero se auto-ajustan. `TestSingleColumnDetailsClickNoMisfire` y
`TestReadOnlyDetailsButtonsNoOp` usan Y fija 11 — verificar si la fila nueva los rompe
y ajustar SOLO la coordenada comentando el porqué, sin tocar su semántica.)

- [ ] **Step 6: Verificar + gates + commit**

```bash
gofmt -w internal/ && golangci-lint run && go vet ./... && go test ./... -count=1 && go build ./...
git add internal/tui/
git commit -m "feat(tui): resize con picker de dos pasos y flujo vivo de rollout"
```

---

### Task 4: CLI — `service resize` + columnas en status + docs de cierre

**Files:**
- Modify: `internal/cli/service_cmd.go` (subcomando resize + columnas CPU/MEM en status)
- Modify: `internal/cli/service_cmd_test.go`
- Create: `internal/cli/units.go` (parsing)
- Create: `internal/cli/units_test.go`
- Modify: `docs/parity.md` (fila resize ✅), `docs/superpowers/plans/2026-06-15-roadmap.md` (02c ✅), `README.md` (mención en highlights/quick start)

**Interfaces:**
- Consumes: `Deployer.Resize/ResourceOptions`, `render.CPULabel/MemLabel`, `watchRollout(ctx,out,dep,service,short,interval)`.
- Produces: `parseCPU(s string) (int, error)` (mili), `parseMemory(s string) (int, error)` (MiB).

- [ ] **Step 1: Tests que fallan — parsing**

Crear `internal/cli/units_test.go`:

```go
package cli

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseCPU(t *testing.T) {
	for in, want := range map[string]int{"0.25": 250, "0.5": 500, "1": 1000, "500m": 500, "2": 2000} {
		got, err := parseCPU(in)
		require.NoError(t, err, in)
		require.Equal(t, want, got, in)
	}
	_, err := parseCPU("abc")
	require.Error(t, err)
}

func TestParseMemory(t *testing.T) {
	for in, want := range map[string]int{"512": 512, "2048": 2048, "2GB": 2048, "0.5GB": 512, "512MB": 512, "2gb": 2048} {
		got, err := parseMemory(in)
		require.NoError(t, err, in)
		require.Equal(t, want, got, in)
	}
	_, err := parseMemory("mucho")
	require.Error(t, err)
}
```

Y a `service_cmd_test.go`:

```go
func TestResizeHappyPathWithPreview(t *testing.T) {
	fake := &coretest.FakeDeployer{Services: []core.ServiceStatus{
		{Name: "catalog", Resources: core.Resources{CPUMilli: 250, MemoryMiB: 512}},
	}}
	withFakeDeployer(t, fake)
	out, err := runRoot(t, "service", "resize", "-s", "catalog", "--cpu", "0.5", "--memory", "2GB", "-y")
	require.NoError(t, err)
	require.Equal(t, []string{"catalog/500/2048"}, fake.ResizeCalls)
	require.Contains(t, out, "0.25 vCPU") // preview: actual
	require.Contains(t, out, "0.5 vCPU")  // preview: objetivo
	require.Contains(t, out, "rollback")  // sugiere rollback
}

func TestResizeRejectsInvalidComboTeaching(t *testing.T) {
	withFakeDeployer(t, &coretest.FakeDeployer{})
	_, err := runRoot(t, "service", "resize", "-s", "catalog", "--cpu", "0.25", "--memory", "8GB", "-y")
	require.ErrorContains(t, err, "cpu 0.25 vCPU supports")
	require.ErrorContains(t, err, "512 MB")  // enseña las válidas del tier
	require.ErrorContains(t, err, "2 GB")
}

func TestResizeRejectsUnknownTier(t *testing.T) {
	withFakeDeployer(t, &coretest.FakeDeployer{})
	_, err := runRoot(t, "service", "resize", "-s", "catalog", "--cpu", "3", "--memory", "4GB", "-y")
	require.ErrorContains(t, err, "valid cpu tiers")
}

func TestStatusShowsResources(t *testing.T) {
	withFakeDeployer(t, &coretest.FakeDeployer{Services: []core.ServiceStatus{
		{Name: "catalog", Running: 1, Desired: 1, Resources: core.Resources{CPUMilli: 500, MemoryMiB: 1024}},
	}})
	out, err := runRoot(t, "service", "status")
	require.NoError(t, err)
	require.Contains(t, out, "CPU")
	require.Contains(t, out, "0.5")
	require.Contains(t, out, "1 GB")
}
```

- [ ] **Step 2: Verificar que fallan**

Run: `go test ./internal/cli/ -run 'TestParse|TestResize|TestStatusShows' -v`
Expected: FAIL — "undefined: parseCPU" / unknown command "resize"

- [ ] **Step 3: Implementar**

Crear `internal/cli/units.go`:

```go
package cli

import (
	"fmt"
	"strconv"
	"strings"
)

// parseCPU acepta vCPU decimal ("0.5", "1") o mili ("500m") y devuelve mili-vCPU.
func parseCPU(s string) (int, error) {
	s = strings.TrimSpace(s)
	if m, ok := strings.CutSuffix(s, "m"); ok {
		n, err := strconv.Atoi(m)
		if err != nil {
			return 0, fmt.Errorf("invalid cpu %q (use 0.5, 1 or 500m)", s)
		}
		return n, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid cpu %q (use 0.5, 1 or 500m)", s)
	}
	return int(f * 1000), nil
}

// parseMemory acepta MiB ("2048") o humano ("2GB", "512MB", case-insensitive)
// y devuelve MiB.
func parseMemory(s string) (int, error) {
	t := strings.ToUpper(strings.TrimSpace(s))
	mult := 1.0
	switch {
	case strings.HasSuffix(t, "GB"):
		t, mult = strings.TrimSuffix(t, "GB"), 1024
	case strings.HasSuffix(t, "MB"):
		t = strings.TrimSuffix(t, "MB")
	}
	f, err := strconv.ParseFloat(t, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid memory %q (use 2048, 2GB or 512MB)", s)
	}
	return int(f * mult), nil
}
```

En `service_cmd.go`, registrar `newServiceResizeCmd()` en `NewServiceCmd` y:

```go
func newServiceResizeCmd() *cobra.Command {
	var service, cpu, memory string
	var yes, watch bool
	var interval int
	cmd := &cobra.Command{
		Use:   "resize",
		Short: "Update the CPU/memory of a service (new revision + rollout)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if service == "" || cpu == "" || memory == "" {
				return fmt.Errorf("--service, --cpu and --memory are required")
			}
			app := FromContext(cmd.Context())
			if err := app.RequireWritable(); err != nil {
				return err
			}
			cpuMilli, err := parseCPU(cpu)
			if err != nil {
				return err
			}
			memMiB, err := parseMemory(memory)
			if err != nil {
				return err
			}
			dep, err := app.Deployer(cmd.Context())
			if err != nil {
				return err
			}
			// validación que enseña, derivada de la tabla del provider
			opts := dep.ResourceOptions()
			var tier *core.ResourceOption
			for i := range opts {
				if opts[i].CPUMilli == cpuMilli {
					tier = &opts[i]
					break
				}
			}
			if tier == nil {
				var tiers []string
				for _, o := range opts {
					tiers = append(tiers, render.CPULabel(o.CPUMilli))
				}
				return fmt.Errorf("valid cpu tiers: %s", strings.Join(tiers, ", "))
			}
			validMem := false
			for _, m := range tier.MemoryMiB {
				if m == memMiB {
					validMem = true
					break
				}
			}
			if !validMem {
				var mems []string
				for _, m := range tier.MemoryMiB {
					mems = append(mems, render.MemLabel(m))
				}
				return fmt.Errorf("cpu %s supports: %s; got %s",
					render.CPULabel(cpuMilli), strings.Join(mems, ", "), render.MemLabel(memMiB))
			}
			realName := app.Ctx.ServiceName(service)
			// recursos actuales para el preview (mejor esfuerzo)
			currentLabel := "unknown"
			if svcs, err := dep.ListServices(cmd.Context()); err == nil {
				for _, s := range svcs {
					if s.Name == realName && s.Resources != (core.Resources{}) {
						currentLabel = render.CPULabel(s.Resources.CPUMilli) + " · " + render.MemLabel(s.Resources.MemoryMiB)
					}
				}
			}
			out := cmd.OutOrStdout()
			_, _ = fmt.Fprintf(out, "%s (%s):\n  %s: %s %s %s\n",
				render.Bold("Resize preview"), app.Ctx.Name, render.Bold(service),
				render.Dim(currentLabel), render.Dim("->"),
				render.Accent(render.CPULabel(cpuMilli)+" · "+render.MemLabel(memMiB)))
			if !yes {
				_, _ = fmt.Fprint(out, "Apply? [y/N]: ")
				if !confirm(cmd.InOrStdin()) {
					_, _ = fmt.Fprintln(out, render.Dim("aborted"))
					return nil
				}
			}
			if err := dep.Resize(cmd.Context(), realName, core.Resources{CPUMilli: cpuMilli, MemoryMiB: memMiB},
				func(s string) { _, _ = fmt.Fprintln(out, render.Dim("[*] "+s)) }); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(out, "%s %s\n%s\n",
				render.Success("✓ resized"), render.Bold(service),
				render.Dim(fmt.Sprintf("rollback with: steer --context %s service rollback -s %s", app.Ctx.Name, service)))
			if watch {
				return watchRollout(cmd.Context(), out, dep, realName, service, interval)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&service, "service", "s", "", "service short name")
	cmd.Flags().StringVar(&cpu, "cpu", "", "target cpu (0.5, 1 or 500m)")
	cmd.Flags().StringVar(&memory, "memory", "", "target memory (2048, 2GB or 512MB)")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation")
	cmd.Flags().BoolVarP(&watch, "watch", "w", false, "follow the rollout until it completes")
	cmd.Flags().IntVar(&interval, "interval", 3, "poll interval in seconds for --watch")
	return cmd
}
```

(`service_cmd.go` ya importa core/render/strings.) En `serviceStatusTable`, añadir
columnas `CPU` y `MEM` tras `TAG`: `render.CPULabel(...)`/`render.MemLabel(...)` o
"—" si `Resources` es cero.

- [ ] **Step 4: Docs de cierre (mismo commit)**

- `docs/parity.md`: fila nueva en la matriz: `| Resize (CPU/memoria) | service resize -s --cpu --memory [-w] | Formulario [ Resize (z) ] con picker de combos | ✅ |`.
- Roadmap: 02c pasa a `✅ hecho (plan 2026-07-05-service-resize.md)` y sale de "Pendiente".
- `README.md`: añadir `steer service resize -s my-svc --cpu 0.5 --memory 2GB` al quick
  start y mencionar el picker de combos en el highlight de guardrails.

- [ ] **Step 5: Verificar todo + gates + commit**

```bash
gofmt -w internal/ && golangci-lint run && go vet ./... && go test ./... -count=1 && go build ./...
git add internal/cli/ docs/ README.md
git commit -m "feat(cli): service resize con validación que enseña y columnas de recursos en status"
```
