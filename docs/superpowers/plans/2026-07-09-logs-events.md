# Hito 03b: LOGS + EVENTS — Plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** cerrar los gaps de paridad de logs y events: interface `core.LogSource` + CloudWatch Logs con auto-discovery desde la task definition, `steer service logs -s [-f] [-n]` y `steer service events -s` en CLI, y pestañas Logs (follow en vivo) y Events (histórico en reposo) vivas en la TUI.

**Architecture:** mismo patrón por capas del hito images: interface agnóstica en `core` → implementación AWS en el bundle `aws.Provider` (memoizada, sesión cacheada) → comandos CLI → TUI. Events no toca el core (`Deployer.ServiceEvents` ya existe). El origen de logs se descubre de la task definition (driver `awslogs`): cero config en `steer.toml`.

**Tech Stack:** Go, aws-sdk-go-v2 (nuevo módulo: `service/cloudwatchlogs`), Cobra, Bubble Tea, testify.

**Spec:** `docs/superpowers/specs/2026-07-09-logs-events-design.md`

## Global Constraints

- Comentarios y docs en español; copy de UI/CLI en inglés (convención del repo).
- Cada commit compila y pasa `go test ./...`; `make lint` (gofmt + golangci-lint) verde.
- Tests con testify `require`, anclados a comportamiento/render, no a internals.
- Mensajes de commit estilo conventional en español (`feat(providers): …`), sin atribución a IA.
- Solo lectura: ningún write path nuevo.
- Constantes fijadas por el spec/diseño: tail CLI default **100** líneas (`-n`), TUI **100** líneas, follow cada **3s**, events CLI últimos **20** ascendente, ventana de tail **1h** (`tailWindow`), `followLookback` **1min**.

---

### Task 1: Core — `LogSource`, `LogLine`, `LogPage`, `ErrNoLogSource` + `FakeLogSource`

**Files:**
- Modify: `internal/core/core.go` (añadir tras el bloque de `Registry`, línea ~113)
- Create: `internal/core/coretest/fake_logsource.go`
- Test: `internal/core/coretest/fake_logsource_test.go`

**Interfaces:**
- Consumes: nada nuevo.
- Produces: `core.LogLine{At time.Time; Container string; Message string}`, `core.LogPage{Lines []LogLine; Cursor string}`, `core.LogSource{TailLogs(ctx, service string, limit int) (LogPage, error); FollowLogs(ctx, service, cursor string) (LogPage, error)}`, `core.ErrNoLogSource`, `coretest.FakeLogSource{Pages []core.LogPage; TailErr, FollowErr error; TailCalls, FollowCalls []string}`.

- [ ] **Step 1: Test que fija la semántica del fake (paging + contadores)**

Crear `internal/core/coretest/fake_logsource_test.go`:

```go
package coretest

import (
	"context"
	"testing"
	"time"

	"github.com/juanMaAV92/steer/internal/core"
	"github.com/stretchr/testify/require"
)

func TestFakeLogSourcePagesEnOrden(t *testing.T) {
	t0 := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)
	f := &FakeLogSource{Pages: []core.LogPage{
		{Lines: []core.LogLine{{At: t0, Message: "hello"}}, Cursor: "c1"},
		{Lines: []core.LogLine{{At: t0.Add(time.Second), Message: "world"}}, Cursor: "c2"},
	}}

	page, err := f.TailLogs(context.Background(), "api", 100)
	require.NoError(t, err)
	require.Equal(t, "hello", page.Lines[0].Message)
	require.Equal(t, "c1", page.Cursor)
	require.Equal(t, []string{"api/100"}, f.TailCalls)

	page, err = f.FollowLogs(context.Background(), "api", "c1")
	require.NoError(t, err)
	require.Equal(t, "world", page.Lines[0].Message)
	require.Equal(t, "c2", page.Cursor)
	require.Equal(t, []string{"api/c1"}, f.FollowCalls)

	// agotadas las páginas, follow devuelve vacío conservando el cursor
	page, err = f.FollowLogs(context.Background(), "api", "c2")
	require.NoError(t, err)
	require.Empty(t, page.Lines)
	require.Equal(t, "c2", page.Cursor)
}
```

- [ ] **Step 2: Verificar que falla**

Run: `go test ./internal/core/coretest/ -run TestFakeLogSourcePagesEnOrden`
Expected: FAIL (compile error: `FakeLogSource` no definido).

- [ ] **Step 3: Tipos en core + fake**

En `internal/core/core.go`, insertar después del bloque `Registry` (tras la línea `HasTag(...)` y su `}` de cierre, antes de `IsProvisioningFailure`):

```go
// ErrNoLogSource indica que el servicio no expone logs legibles por steer
// (driver de logs no soportado o sin logConfiguration). No es un fallo del cloud.
var ErrNoLogSource = errors.New("no log source for this service")

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

// LogSource lee logs de un servicio (CloudWatch Logs / Cloud Logging / Log
// Analytics). El cursor es opaco: lo define cada provider; el contrato solo
// exige que FollowLogs(cursor) no repita ni pierda líneas.
type LogSource interface {
	// TailLogs devuelve las últimas `limit` líneas del servicio.
	TailLogs(ctx context.Context, service string, limit int) (LogPage, error)
	// FollowLogs devuelve las líneas posteriores al cursor.
	FollowLogs(ctx context.Context, service string, cursor string) (LogPage, error)
}
```

Crear `internal/core/coretest/fake_logsource.go`:

```go
package coretest

import (
	"context"
	"fmt"

	"github.com/juanMaAV92/steer/internal/core"
)

// FakeLogSource es un LogSource en memoria para tests: TailLogs devuelve
// Pages[0] y cada FollowLogs consume la página siguiente.
type FakeLogSource struct {
	Pages     []core.LogPage
	TailErr   error
	FollowErr error

	TailCalls   []string // "service/limit", en orden
	FollowCalls []string // "service/cursor", en orden

	next int
}

func (f *FakeLogSource) TailLogs(_ context.Context, service string, limit int) (core.LogPage, error) {
	f.TailCalls = append(f.TailCalls, fmt.Sprintf("%s/%d", service, limit))
	if f.TailErr != nil {
		return core.LogPage{}, f.TailErr
	}
	if len(f.Pages) == 0 {
		return core.LogPage{}, nil
	}
	f.next = 1
	return f.Pages[0], nil
}

func (f *FakeLogSource) FollowLogs(_ context.Context, service, cursor string) (core.LogPage, error) {
	f.FollowCalls = append(f.FollowCalls, service+"/"+cursor)
	if f.FollowErr != nil {
		return core.LogPage{}, f.FollowErr
	}
	if f.next >= len(f.Pages) {
		return core.LogPage{Cursor: cursor}, nil
	}
	p := f.Pages[f.next]
	f.next++
	return p, nil
}

var _ core.LogSource = (*FakeLogSource)(nil)
```

- [ ] **Step 4: Verificar que pasa**

Run: `go test ./internal/core/... `
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/core/core.go internal/core/coretest/fake_logsource.go internal/core/coretest/fake_logsource_test.go
git commit -m "feat(core): interface LogSource (tail+follow con cursor opaco) y FakeLogSource"
```

---

### Task 2: Provider AWS — `CWLogSource` (discovery + tail + follow)

**Files:**
- Create: `internal/providers/aws/logs.go`
- Test: `internal/providers/aws/logs_test.go`
- Modify: `go.mod`/`go.sum` (nuevo módulo SDK)

**Interfaces:**
- Consumes: `core.LogSource`, `core.LogLine`, `core.LogPage`, `core.ErrNoLogSource` (Task 1).
- Produces: `aws.NewLogSource(cfg awssdk.Config, cluster string) *CWLogSource` (satisface `core.LogSource`); constructor inyectable `newLogSource(e logsECSAPI, c cwlAPI, cluster string) *CWLogSource` para tests.

**Notas de diseño que el implementador debe respetar:**
- `FilterLogEvents` solo lee hacia delante: "últimas N líneas" se materializa como "últimas N de la última hora" (`tailWindow`). El copy de UI lo refleja ("no logs in the last hour").
- Cursor JSON por contenedor `{ts, ids}`: `ts` = timestamp (ms) del último evento visto; `ids` = IDs de evento en ese mismo milisegundo (dedup del borde, porque `StartTime` es inclusivo).
- Contenedor silencioso: el cursor avanza a `now-followLookback` para no reescanear la misma ventana en cada poll. Eventos que lleguen tarde con timestamp anterior se pierden (limitación asumida, misma clase de trade-off que `aws logs tail`).

- [ ] **Step 1: Añadir la dependencia**

Run: `go get github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs && go mod tidy`
Expected: go.mod gana `github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs`.

- [ ] **Step 2: Tests que fijan discovery, merge, dedup y errores**

Crear `internal/providers/aws/logs_test.go`:

```go
package aws

import (
	"context"
	"errors"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	cwl "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	cwltypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/juanMaAV92/steer/internal/core"
	"github.com/stretchr/testify/require"
)

// fakeLogsECS implementa logsECSAPI con una task def fija.
type fakeLogsECS struct {
	tdArn      string
	containers []ecstypes.ContainerDefinition

	describeTaskDefCalls int
}

func (f *fakeLogsECS) DescribeServices(_ context.Context, _ *ecs.DescribeServicesInput, _ ...func(*ecs.Options)) (*ecs.DescribeServicesOutput, error) {
	return &ecs.DescribeServicesOutput{Services: []ecstypes.Service{{TaskDefinition: awssdk.String(f.tdArn)}}}, nil
}

func (f *fakeLogsECS) DescribeTaskDefinition(_ context.Context, _ *ecs.DescribeTaskDefinitionInput, _ ...func(*ecs.Options)) (*ecs.DescribeTaskDefinitionOutput, error) {
	f.describeTaskDefCalls++
	return &ecs.DescribeTaskDefinitionOutput{TaskDefinition: &ecstypes.TaskDefinition{ContainerDefinitions: f.containers}}, nil
}

// fakeCWL devuelve eventos canned por (grupo, prefijo de stream).
type fakeCWL struct {
	events map[string][]cwltypes.FilteredLogEvent // clave: group + "|" + streamPrefix
}

func (f *fakeCWL) FilterLogEvents(_ context.Context, in *cwl.FilterLogEventsInput, _ ...func(*cwl.Options)) (*cwl.FilterLogEventsOutput, error) {
	key := awssdk.ToString(in.LogGroupName) + "|" + awssdk.ToString(in.LogStreamNamePrefix)
	var out []cwltypes.FilteredLogEvent
	for _, e := range f.events[key] {
		if awssdk.ToInt64(e.Timestamp) >= awssdk.ToInt64(in.StartTime) {
			out = append(out, e)
		}
	}
	return &cwl.FilterLogEventsOutput{Events: out}, nil
}

func awslogsContainer(name, group, prefix string) ecstypes.ContainerDefinition {
	return ecstypes.ContainerDefinition{
		Name: awssdk.String(name),
		LogConfiguration: &ecstypes.LogConfiguration{
			LogDriver: ecstypes.LogDriverAwslogs,
			Options:   map[string]string{"awslogs-group": group, "awslogs-stream-prefix": prefix},
		},
	}
}

func event(id string, ts int64, msg string) cwltypes.FilteredLogEvent {
	return cwltypes.FilteredLogEvent{EventId: awssdk.String(id), Timestamp: awssdk.Int64(ts), Message: awssdk.String(msg)}
}

func TestTailLogsMergesContainersAscendente(t *testing.T) {
	now := time.Now().UnixMilli()
	e := &fakeLogsECS{tdArn: "td:1", containers: []ecstypes.ContainerDefinition{
		awslogsContainer("app", "/ecs/app", "ecs"),
		awslogsContainer("envoy", "/ecs/envoy", "ecs"),
	}}
	c := &fakeCWL{events: map[string][]cwltypes.FilteredLogEvent{
		"/ecs/app|ecs/app/":     {event("a1", now-2000, "app line")},
		"/ecs/envoy|ecs/envoy/": {event("b1", now-1000, "envoy line")},
	}}
	src := newLogSource(e, c, "cluster")

	page, err := src.TailLogs(context.Background(), "svc", 100)
	require.NoError(t, err)
	require.Len(t, page.Lines, 2)
	require.Equal(t, "app line", page.Lines[0].Message)   // más antiguo primero
	require.Equal(t, "envoy line", page.Lines[1].Message) // más nuevo después
	require.Equal(t, "app", page.Lines[0].Container)      // multi-contenedor → nombre
	require.NotEmpty(t, page.Cursor)
}

func TestTailLogsUnContenedorSinNombre(t *testing.T) {
	now := time.Now().UnixMilli()
	e := &fakeLogsECS{tdArn: "td:1", containers: []ecstypes.ContainerDefinition{
		awslogsContainer("app", "/ecs/app", "ecs"),
	}}
	c := &fakeCWL{events: map[string][]cwltypes.FilteredLogEvent{
		"/ecs/app|ecs/app/": {event("a1", now-1000, "solo")},
	}}
	src := newLogSource(e, c, "cluster")

	page, err := src.TailLogs(context.Background(), "svc", 100)
	require.NoError(t, err)
	require.Equal(t, "", page.Lines[0].Container) // un contenedor → sin prefijo
}

func TestTailLogsRespetaLimit(t *testing.T) {
	now := time.Now().UnixMilli()
	evs := []cwltypes.FilteredLogEvent{
		event("a1", now-3000, "uno"), event("a2", now-2000, "dos"), event("a3", now-1000, "tres"),
	}
	e := &fakeLogsECS{tdArn: "td:1", containers: []ecstypes.ContainerDefinition{awslogsContainer("app", "/ecs/app", "ecs")}}
	c := &fakeCWL{events: map[string][]cwltypes.FilteredLogEvent{"/ecs/app|ecs/app/": evs}}
	src := newLogSource(e, c, "cluster")

	page, err := src.TailLogs(context.Background(), "svc", 2)
	require.NoError(t, err)
	require.Len(t, page.Lines, 2)
	require.Equal(t, "dos", page.Lines[0].Message) // conserva las ÚLTIMAS 2
	require.Equal(t, "tres", page.Lines[1].Message)
}

func TestFollowLogsDedupEnElBorde(t *testing.T) {
	now := time.Now().UnixMilli()
	e := &fakeLogsECS{tdArn: "td:1", containers: []ecstypes.ContainerDefinition{awslogsContainer("app", "/ecs/app", "ecs")}}
	c := &fakeCWL{events: map[string][]cwltypes.FilteredLogEvent{
		"/ecs/app|ecs/app/": {event("a1", now-1000, "visto"), event("a2", now-1000, "nuevo mismo ms"), event("a3", now, "posterior")},
	}}
	src := newLogSource(e, c, "cluster")

	cursor := encodeCursor(cwlCursor{"app": {Ts: now - 1000, IDs: []string{"a1"}}})
	page, err := src.FollowLogs(context.Background(), "svc", cursor)
	require.NoError(t, err)
	require.Len(t, page.Lines, 2) // a1 deduplicado; a2 (mismo ms) y a3 entran
	require.Equal(t, "nuevo mismo ms", page.Lines[0].Message)
	require.Equal(t, "posterior", page.Lines[1].Message)
}

func TestDriverNoSoportadoDevuelveErrNoLogSource(t *testing.T) {
	e := &fakeLogsECS{tdArn: "td:1", containers: []ecstypes.ContainerDefinition{{
		Name: awssdk.String("app"),
		LogConfiguration: &ecstypes.LogConfiguration{
			LogDriver: ecstypes.LogDriverAwsfirelens,
			Options:   map[string]string{},
		},
	}}}
	src := newLogSource(e, &fakeCWL{}, "cluster")

	_, err := src.TailLogs(context.Background(), "svc", 100)
	require.ErrorIs(t, err, core.ErrNoLogSource)
	require.Contains(t, err.Error(), "awsfirelens")
}

func TestSinLogConfigurationDevuelveErrNoLogSource(t *testing.T) {
	e := &fakeLogsECS{tdArn: "td:1", containers: []ecstypes.ContainerDefinition{{Name: awssdk.String("app")}}}
	src := newLogSource(e, &fakeCWL{}, "cluster")

	_, err := src.TailLogs(context.Background(), "svc", 100)
	require.ErrorIs(t, err, core.ErrNoLogSource)
}

func TestDiscoveryCacheadoPorRevision(t *testing.T) {
	e := &fakeLogsECS{tdArn: "td:1", containers: []ecstypes.ContainerDefinition{awslogsContainer("app", "/ecs/app", "ecs")}}
	src := newLogSource(e, &fakeCWL{}, "cluster")

	_, err := src.TailLogs(context.Background(), "svc", 10)
	require.NoError(t, err)
	_, err = src.TailLogs(context.Background(), "svc", 10)
	require.NoError(t, err)
	require.Equal(t, 1, e.describeTaskDefCalls) // misma revisión → cache

	e.tdArn = "td:2" // deploy: revisión nueva → re-descubre
	_, err = src.TailLogs(context.Background(), "svc", 10)
	require.NoError(t, err)
	require.Equal(t, 2, e.describeTaskDefCalls)
}

func TestErroresDeAPIsPropagan(t *testing.T) {
	e := &fakeLogsECS{tdArn: "td:1", containers: []ecstypes.ContainerDefinition{awslogsContainer("app", "/ecs/app", "ecs")}}
	src := newLogSource(e, &fakeCWLErr{}, "cluster")
	_, err := src.TailLogs(context.Background(), "svc", 10)
	require.Error(t, err)
	require.NotErrorIs(t, err, core.ErrNoLogSource) // error del cloud, no "sin logs"
}

type fakeCWLErr struct{}

func (fakeCWLErr) FilterLogEvents(_ context.Context, _ *cwl.FilterLogEventsInput, _ ...func(*cwl.Options)) (*cwl.FilterLogEventsOutput, error) {
	return nil, errors.New("throttled")
}
```

- [ ] **Step 3: Verificar que fallan**

Run: `go test ./internal/providers/aws/ -run 'TestTailLogs|TestFollowLogs|TestDriver|TestSinLog|TestDiscovery|TestErroresDeAPIs'`
Expected: FAIL (compile error: `newLogSource` no definido).

- [ ] **Step 4: Implementación**

Crear `internal/providers/aws/logs.go`:

```go
package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	cwl "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/juanMaAV92/steer/internal/core"
)

// tailWindow acota cuánto pasado escanea TailLogs: FilterLogEvents solo lee
// hacia delante, así que "las últimas N líneas" se materializa como "las
// últimas N líneas de la última hora".
const tailWindow = time.Hour

// followLookback acota el rescan de un contenedor silencioso: si un poll no
// trae eventos, el cursor avanza hasta now-followLookback para no reescanear
// la misma ventana para siempre. Eventos que lleguen tarde con timestamp
// anterior se pierden (trade-off asumido, misma clase que aws logs tail).
const followLookback = time.Minute

// cwlAPI es el subconjunto del cliente CloudWatch Logs que usa el log source.
// El *cloudwatchlogs.Client del SDK lo satisface; los tests inyectan un fake.
type cwlAPI interface {
	FilterLogEvents(ctx context.Context, in *cwl.FilterLogEventsInput, optFns ...func(*cwl.Options)) (*cwl.FilterLogEventsOutput, error)
}

// logsECSAPI es el subconjunto del cliente ECS que usa el discovery de logs.
type logsECSAPI interface {
	DescribeServices(ctx context.Context, in *ecs.DescribeServicesInput, optFns ...func(*ecs.Options)) (*ecs.DescribeServicesOutput, error)
	DescribeTaskDefinition(ctx context.Context, in *ecs.DescribeTaskDefinitionInput, optFns ...func(*ecs.Options)) (*ecs.DescribeTaskDefinitionOutput, error)
}

// logTarget es un destino de lectura descubierto: un contenedor con driver awslogs.
type logTarget struct {
	Container string
	Group     string
	StreamPfx string // "{awslogs-stream-prefix}/{container}/"; "" si el driver no fija prefijo
}

// targetsEntry cachea los targets de un servicio junto a la revisión que los produjo.
type targetsEntry struct {
	tdArn   string
	targets []logTarget
}

// CWLogSource implementa core.LogSource sobre CloudWatch Logs. El origen se
// auto-descubre de la task definition del servicio (driver awslogs): cero config.
type CWLogSource struct {
	ecs     logsECSAPI
	cwl     cwlAPI
	cluster string

	mu    sync.Mutex
	cache map[string]targetsEntry // servicio → targets de su task def actual
}

// NewLogSource crea un CWLogSource desde una aws.Config.
func NewLogSource(cfg awssdk.Config, cluster string) *CWLogSource {
	return newLogSource(ecs.NewFromConfig(cfg), cwl.NewFromConfig(cfg), cluster)
}

// newLogSource es el constructor inyectable usado por los tests.
func newLogSource(e logsECSAPI, c cwlAPI, cluster string) *CWLogSource {
	return &CWLogSource{ecs: e, cwl: c, cluster: cluster, cache: map[string]targetsEntry{}}
}

// discover resuelve los targets de logs del servicio desde su task definition,
// cacheado por ARN de revisión (un deploy registra revisión nueva → re-resuelve;
// el log group casi nunca cambia).
func (s *CWLogSource) discover(ctx context.Context, service string) ([]logTarget, error) {
	desc, err := s.ecs.DescribeServices(ctx, &ecs.DescribeServicesInput{
		Cluster:  awssdk.String(s.cluster),
		Services: []string{service},
	})
	if err != nil {
		return nil, err
	}
	if len(desc.Services) == 0 {
		return nil, fmt.Errorf("service %q not found in cluster %q", service, s.cluster)
	}
	tdArn := awssdk.ToString(desc.Services[0].TaskDefinition)

	s.mu.Lock()
	entry, ok := s.cache[service]
	s.mu.Unlock()
	if ok && entry.tdArn == tdArn {
		return entry.targets, nil
	}

	td, err := s.ecs.DescribeTaskDefinition(ctx, &ecs.DescribeTaskDefinitionInput{
		TaskDefinition: awssdk.String(tdArn),
	})
	if err != nil {
		return nil, err
	}
	var targets []logTarget
	otherDriver := ""
	for _, cd := range td.TaskDefinition.ContainerDefinitions {
		lc := cd.LogConfiguration
		if lc == nil {
			continue
		}
		if lc.LogDriver != ecstypes.LogDriverAwslogs {
			otherDriver = string(lc.LogDriver)
			continue
		}
		name := awssdk.ToString(cd.Name)
		pfx := ""
		if p := lc.Options["awslogs-stream-prefix"]; p != "" {
			pfx = p + "/" + name + "/"
		}
		targets = append(targets, logTarget{Container: name, Group: lc.Options["awslogs-group"], StreamPfx: pfx})
	}
	if len(targets) == 0 {
		if otherDriver != "" {
			return nil, fmt.Errorf("%w: containers log via %q, not awslogs — steer can't read them yet", core.ErrNoLogSource, otherDriver)
		}
		return nil, fmt.Errorf("%w: task definition has no log configuration", core.ErrNoLogSource)
	}
	s.mu.Lock()
	s.cache[service] = targetsEntry{tdArn: tdArn, targets: targets}
	s.mu.Unlock()
	return targets, nil
}

// containerPos es la posición de lectura de un contenedor: último timestamp
// visto (ms) y los IDs de evento en ese milisegundo (dedup del borde inclusivo).
type containerPos struct {
	Ts  int64    `json:"ts"`
	IDs []string `json:"ids"`
}

// cwlCursor es el cursor opaco del contrato: posición por contenedor, en JSON.
type cwlCursor map[string]containerPos

func encodeCursor(c cwlCursor) string {
	b, _ := json.Marshal(c)
	return string(b)
}

func decodeCursor(s string) cwlCursor {
	c := cwlCursor{}
	if s != "" {
		_ = json.Unmarshal([]byte(s), &c)
	}
	return c
}

// cwlEvent es un evento crudo de CloudWatch antes de convertirse en LogLine.
type cwlEvent struct {
	id  string
	ts  int64
	msg string
}

// collect lee todos los eventos de un target desde start (ms, inclusivo),
// paginando hasta el final; si keep > 0 conserva solo los últimos keep
// (FilterLogEvents entrega en orden cronológico, así que basta recortar).
func (s *CWLogSource) collect(ctx context.Context, t logTarget, start int64, keep int) ([]cwlEvent, error) {
	in := &cwl.FilterLogEventsInput{
		LogGroupName: awssdk.String(t.Group),
		StartTime:    awssdk.Int64(start),
	}
	if t.StreamPfx != "" {
		in.LogStreamNamePrefix = awssdk.String(t.StreamPfx)
	}
	var out []cwlEvent
	for {
		resp, err := s.cwl.FilterLogEvents(ctx, in)
		if err != nil {
			return nil, err
		}
		for _, e := range resp.Events {
			out = append(out, cwlEvent{
				id:  awssdk.ToString(e.EventId),
				ts:  awssdk.ToInt64(e.Timestamp),
				msg: awssdk.ToString(e.Message),
			})
		}
		if keep > 0 && len(out) > keep {
			out = out[len(out)-keep:]
		}
		if awssdk.ToString(resp.NextToken) == "" {
			return out, nil
		}
		in.NextToken = resp.NextToken
	}
}

// TailLogs devuelve las últimas limit líneas del servicio dentro de tailWindow.
func (s *CWLogSource) TailLogs(ctx context.Context, service string, limit int) (core.LogPage, error) {
	targets, err := s.discover(ctx, service)
	if err != nil {
		return core.LogPage{}, err
	}
	start := time.Now().Add(-tailWindow).UnixMilli()
	return s.read(ctx, targets, func(logTarget) (int64, []string) { return start, nil }, limit)
}

// FollowLogs devuelve las líneas posteriores al cursor.
func (s *CWLogSource) FollowLogs(ctx context.Context, service, cursor string) (core.LogPage, error) {
	targets, err := s.discover(ctx, service)
	if err != nil {
		return core.LogPage{}, err
	}
	pos := decodeCursor(cursor)
	fallback := time.Now().Add(-tailWindow).UnixMilli()
	return s.read(ctx, targets, func(t logTarget) (int64, []string) {
		if p, ok := pos[t.Container]; ok {
			return p.Ts, p.IDs
		}
		return fallback, nil
	}, 0)
}

// read lee todos los targets desde su posición, mezcla por timestamp ascendente
// y produce la página con el cursor avanzado. Con limit > 0 recorta al final.
func (s *CWLogSource) read(ctx context.Context, targets []logTarget, posFor func(logTarget) (int64, []string), limit int) (core.LogPage, error) {
	named := len(targets) > 1
	next := cwlCursor{}
	var lines []core.LogLine
	for _, t := range targets {
		start, seen := posFor(t)
		evs, err := s.collect(ctx, t, start, limit)
		if err != nil {
			return core.LogPage{}, err
		}
		skip := map[string]bool{}
		for _, id := range seen {
			skip[id] = true
		}
		pos := containerPos{Ts: start, IDs: seen}
		for _, e := range evs {
			if e.ts == start && skip[e.id] {
				continue // dedup del borde: StartTime es inclusivo
			}
			if e.ts > pos.Ts {
				pos = containerPos{Ts: e.ts}
			}
			pos.IDs = append(pos.IDs, e.id)
			container := ""
			if named {
				container = t.Container
			}
			lines = append(lines, core.LogLine{At: time.UnixMilli(e.ts), Container: container, Message: e.msg})
		}
		// contenedor silencioso: avanzar acota el rescan del próximo poll
		if lo := time.Now().Add(-followLookback).UnixMilli(); len(evs) == 0 && lo > pos.Ts {
			pos = containerPos{Ts: lo}
		}
		next[t.Container] = pos
	}
	sort.SliceStable(lines, func(i, j int) bool { return lines[i].At.Before(lines[j].At) })
	if limit > 0 && len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	return core.LogPage{Lines: lines, Cursor: encodeCursor(next)}, nil
}

var _ core.LogSource = (*CWLogSource)(nil)
```

Además, en `internal/providers/aws/errors.go:29`, ampliar el remedio de access-denied para cubrir la capacidad nueva (el test existente solo asegura el prefijo "access denied", no cambia):

```go
	}, "access denied — try: ask whoever manages AWS to grant your role ECS/ECR/CloudWatch Logs read permissions"},
```

- [ ] **Step 5: Verificar que pasan**

Run: `go test ./internal/providers/aws/`
Expected: PASS (incluidos los tests preexistentes).

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/providers/aws/logs.go internal/providers/aws/logs_test.go internal/providers/aws/errors.go
git commit -m "feat(providers): CWLogSource — logs de CloudWatch con auto-discovery desde la task definition"
```

---

### Task 3: Costura del provider — `Logs()` en el bundle y en `AppContext`

**Files:**
- Modify: `internal/providers/factory.go` (interface `Provider`, línea ~34)
- Modify: `internal/providers/aws/provider.go`
- Modify: `internal/cli/context.go`
- Modify: `internal/cli/service_cmd_test.go` (fakeProvider, línea ~20)
- Modify: `internal/tui/app_test.go` (fakeProvider, línea ~39)

**Interfaces:**
- Consumes: `aws.NewLogSource` (Task 2), `core.LogSource`/`core.ErrNoLogSource` (Task 1).
- Produces: `providers.Provider.Logs() (core.LogSource, error)`; `(*aws.Provider).Logs()`; `(*cli.AppContext).Logs(ctx context.Context) (core.LogSource, error)`; `fakeProvider{dep, reg, logs}` en ambos paquetes de test (campo `logs core.LogSource`, nil → `core.ErrNoLogSource`).

Nota: añadir el método a la interface rompe los `fakeProvider` de los tests de cli y tui — **todo va en esta task** para que cada commit compile.

- [ ] **Step 1: Ampliar la interface y el bundle AWS**

En `internal/providers/factory.go`, dentro de `type Provider interface` (tras `Registry()`):

```go
	// Logs devuelve la capacidad de lectura de logs del contexto. El origen se
	// descubre por servicio: core.ErrNoLogSource llega por operación, no aquí.
	Logs() (core.LogSource, error)
```

En `internal/providers/aws/provider.go`, añadir campos al struct `Provider` (junto a `regOnce`/`registry`):

```go
	logsOnce sync.Once
	logsrc   core.LogSource
```

y el método (tras `Registry()`):

```go
// Logs devuelve el LogSource CloudWatch del contexto (memoizado). El origen
// por servicio se auto-descubre de la task definition: no requiere config.
func (p *Provider) Logs() (core.LogSource, error) {
	p.logsOnce.Do(func() { p.logsrc = NewLogSource(p.cfg, p.cfgCtx.Cluster) })
	return p.logsrc, nil
}
```

- [ ] **Step 2: `AppContext.Logs` en la CLI**

En `internal/cli/context.go`, tras `Registry()`:

```go
// Logs construye (una vez) el provider y devuelve su LogSource.
func (a *AppContext) Logs(ctx context.Context) (core.LogSource, error) {
	if a.provider == nil {
		p, err := a.Factory(ctx, a.Ctx)
		if err != nil {
			return nil, err
		}
		a.provider = p
	}
	return a.provider.Logs()
}
```

- [ ] **Step 3: Actualizar los fakeProvider de los tests**

En `internal/cli/service_cmd_test.go`, ampliar el struct y añadir el método:

```go
// fakeProvider adapta fakes de core al Provider bundle.
type fakeProvider struct {
	dep  core.Deployer
	reg  core.Registry  // nil → capacidad deshabilitada
	logs core.LogSource // nil → sin log source (tests)
}
```

```go
func (p fakeProvider) Logs() (core.LogSource, error) {
	if p.logs == nil {
		return nil, core.ErrNoLogSource
	}
	return p.logs, nil
}
```

En `internal/tui/app_test.go`, mismo cambio en su `fakeProvider` (campo `logs core.LogSource` + método `Logs()` idéntico), y añadir junto a `fakeFactoryWithRegistry`:

```go
// fakeFactoryWithLogs adapta un core.Deployer y un core.LogSource fake a una
// ProviderFactory (para tests de la pestaña Logs).
func fakeFactoryWithLogs(dep core.Deployer, src core.LogSource) providers.ProviderFactory {
	return func(context.Context, config.Context) (providers.Provider, error) {
		return fakeProvider{dep: dep, logs: src}, nil
	}
}
```

- [ ] **Step 4: Verificar compilación y tests**

Run: `go build ./... && go test ./...`
Expected: PASS (sin tests nuevos: esta task es la costura).

- [ ] **Step 5: Commit**

```bash
git add internal/providers/factory.go internal/providers/aws/provider.go internal/cli/context.go internal/cli/service_cmd_test.go internal/tui/app_test.go
git commit -m "feat(providers,cli): capacidad Logs() en el Provider bundle y AppContext"
```

---

### Task 4: CLI — `steer service logs -s [-f] [-n]`

**Files:**
- Create: `internal/cli/service_logs_cmd.go`
- Test: `internal/cli/service_logs_cmd_test.go`
- Modify: `internal/cli/service_cmd.go:25` (registrar el subcomando)

**Interfaces:**
- Consumes: `AppContext.Logs(ctx)` (Task 3), `core.LogPage`/`core.LogLine` (Task 1), `render.Dim/Accent`, `FromContext`, `app.Ctx.ServiceName`.
- Produces: subcomando `logs` bajo `service`; helper `printLogLine(out io.Writer, l core.LogLine)`.

- [ ] **Step 1: Tests**

Crear `internal/cli/service_logs_cmd_test.go`:

```go
package cli

import (
	"context"
	"testing"
	"time"

	"github.com/juanMaAV92/steer/internal/config"
	"github.com/juanMaAV92/steer/internal/core"
	"github.com/juanMaAV92/steer/internal/core/coretest"
	"github.com/juanMaAV92/steer/internal/providers"
	"github.com/stretchr/testify/require"
)

// withFakeLogSource inyecta una factory cuyo provider expone el LogSource dado
// (nil → core.ErrNoLogSource) sobre un FakeDeployer neutro.
func withFakeLogSource(t *testing.T, src core.LogSource) {
	t.Helper()
	prev := newProviderFactoryFn
	newProviderFactoryFn = func() providers.ProviderFactory {
		return func(context.Context, config.Context) (providers.Provider, error) {
			return fakeProvider{dep: &coretest.FakeDeployer{}, logs: src}, nil
		}
	}
	t.Cleanup(func() { newProviderFactoryFn = prev })
}

func TestServiceLogsImprimeTail(t *testing.T) {
	t0 := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)
	src := &coretest.FakeLogSource{Pages: []core.LogPage{{
		Lines: []core.LogLine{
			{At: t0, Message: "listening on :8080"},
			{At: t0.Add(time.Second), Message: "GET /health 200"},
		},
		Cursor: "c1",
	}}}
	withFakeLogSource(t, src)

	out, err := runRoot(t, "service", "logs", "-s", "api")
	require.NoError(t, err)
	require.Contains(t, out, "listening on :8080")
	require.Contains(t, out, "GET /health 200")
	require.Equal(t, []string{"api/100"}, src.TailCalls) // default -n 100
}

func TestServiceLogsPrefijoDeContenedor(t *testing.T) {
	t0 := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)
	src := &coretest.FakeLogSource{Pages: []core.LogPage{{
		Lines: []core.LogLine{{At: t0, Container: "envoy", Message: "ready"}},
	}}}
	withFakeLogSource(t, src)

	out, err := runRoot(t, "service", "logs", "-s", "api")
	require.NoError(t, err)
	require.Contains(t, out, "[envoy]")
}

func TestServiceLogsRespetaN(t *testing.T) {
	src := &coretest.FakeLogSource{}
	withFakeLogSource(t, src)

	_, err := runRoot(t, "service", "logs", "-s", "api", "-n", "25")
	require.NoError(t, err)
	require.Equal(t, []string{"api/25"}, src.TailCalls)
}

func TestServiceLogsSinLineas(t *testing.T) {
	withFakeLogSource(t, &coretest.FakeLogSource{})
	out, err := runRoot(t, "service", "logs", "-s", "api")
	require.NoError(t, err)
	require.Contains(t, out, "no logs in the last hour")
}

func TestServiceLogsSinLogSource(t *testing.T) {
	withFakeLogSource(t, nil) // fakeProvider.logs nil → core.ErrNoLogSource
	_, err := runRoot(t, "service", "logs", "-s", "api")
	require.ErrorIs(t, err, core.ErrNoLogSource)
}

func TestServiceLogsRequiereService(t *testing.T) {
	withFakeLogSource(t, &coretest.FakeLogSource{})
	_, err := runRoot(t, "service", "logs")
	require.ErrorContains(t, err, "--service")
}
```

- [ ] **Step 2: Verificar que fallan**

Run: `go test ./internal/cli/ -run TestServiceLogs`
Expected: FAIL (`unknown command "logs"` o error de compilación).

- [ ] **Step 3: Implementación**

Crear `internal/cli/service_logs_cmd.go`:

```go
package cli

import (
	"fmt"
	"io"
	"time"

	"github.com/juanMaAV92/steer/internal/core"
	"github.com/juanMaAV92/steer/internal/render"
	"github.com/spf13/cobra"
)

func newServiceLogsCmd() *cobra.Command {
	var service string
	var follow bool
	var lines, interval int
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Show recent service logs (all containers, merged)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if service == "" {
				return fmt.Errorf("--service is required")
			}
			app := FromContext(cmd.Context())
			src, err := app.Logs(cmd.Context())
			if err != nil {
				return err
			}
			realName := app.Ctx.ServiceName(service)
			out := cmd.OutOrStdout()

			page, err := src.TailLogs(cmd.Context(), realName, lines)
			if err != nil {
				return err
			}
			if len(page.Lines) == 0 {
				_, _ = fmt.Fprintln(out, render.Dim("no logs in the last hour"))
			}
			for _, l := range page.Lines {
				printLogLine(out, l)
			}
			if !follow {
				return nil
			}
			cursor := page.Cursor
			_, _ = fmt.Fprintln(out, render.Dim("following logs (Ctrl+C to stop)..."))
			for {
				select {
				case <-cmd.Context().Done():
					return cmd.Context().Err()
				case <-time.After(time.Duration(interval) * time.Second):
				}
				page, err := src.FollowLogs(cmd.Context(), realName, cursor)
				if err != nil {
					return err
				}
				for _, l := range page.Lines {
					printLogLine(out, l)
				}
				cursor = page.Cursor
			}
		},
	}
	cmd.Flags().StringVarP(&service, "service", "s", "", "service short name")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "keep streaming new lines")
	cmd.Flags().IntVarP(&lines, "lines", "n", 100, "how many recent lines to show")
	cmd.Flags().IntVar(&interval, "interval", 3, "poll interval in seconds for --follow")
	return cmd
}

// printLogLine imprime una línea de log: HH:MM:SS  [container]  mensaje
// (el contenedor solo aparece cuando la task tiene más de uno).
func printLogLine(out io.Writer, l core.LogLine) {
	ts := render.Dim(l.At.Format("15:04:05"))
	if l.Container != "" {
		_, _ = fmt.Fprintf(out, "%s  %s  %s\n", ts, render.Accent("["+l.Container+"]"), l.Message)
		return
	}
	_, _ = fmt.Fprintf(out, "%s  %s\n", ts, l.Message)
}
```

En `internal/cli/service_cmd.go:25`, registrar el subcomando:

```go
	cmd.AddCommand(newServiceStatusCmd(), newServiceDeployCmd(), newServiceScaleCmd(), newServiceRollbackCmd(), newServiceResizeCmd(), newServiceLogsCmd())
```

- [ ] **Step 4: Verificar que pasan**

Run: `go test ./internal/cli/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/service_logs_cmd.go internal/cli/service_logs_cmd_test.go internal/cli/service_cmd.go
git commit -m "feat(cli): steer service logs — tail y follow de logs del servicio"
```

---

### Task 5: CLI — `steer service events -s`

**Files:**
- Create: `internal/cli/service_events_cmd.go`
- Test: `internal/cli/service_events_cmd_test.go`
- Modify: `internal/cli/service_cmd.go:25` (registrar el subcomando)

**Interfaces:**
- Consumes: `AppContext.Deployer(ctx)`, `core.ServiceEvent` (existentes), `render.Table/Dim/Danger`.
- Produces: subcomando `events` bajo `service`. Constante `maxEventsShown = 20`.

- [ ] **Step 1: Tests**

Crear `internal/cli/service_events_cmd_test.go`:

```go
package cli

import (
	"strings"
	"testing"
	"time"

	"github.com/juanMaAV92/steer/internal/core"
	"github.com/juanMaAV92/steer/internal/core/coretest"
	"github.com/stretchr/testify/require"
)

func TestServiceEventsMuestraAscendente(t *testing.T) {
	t0 := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)
	// el deployer entrega más recientes primero (contrato de ServiceEvents)
	withFakeDeployer(t, &coretest.FakeDeployer{Events: []core.ServiceEvent{
		{ID: "2", At: t0.Add(time.Minute), Message: "reached steady state"},
		{ID: "1", At: t0, Message: "started 2 tasks", IsError: false},
	}})

	out, err := runRoot(t, "service", "events", "-s", "api")
	require.NoError(t, err)
	require.Contains(t, out, "TIME")
	require.Contains(t, out, "MESSAGE")
	// ascendente: lo más reciente al final, junto al prompt
	require.Less(t, strings.Index(out, "started 2 tasks"), strings.Index(out, "reached steady state"))
}

func TestServiceEventsRecortaAVeinte(t *testing.T) {
	t0 := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)
	evs := make([]core.ServiceEvent, 25)
	for i := range evs {
		// más recientes primero: el índice 0 es el más nuevo
		evs[i] = core.ServiceEvent{ID: string(rune('a' + i)), At: t0.Add(-time.Duration(i) * time.Minute), Message: "event-" + string(rune('a'+i))}
	}
	withFakeDeployer(t, &coretest.FakeDeployer{Events: evs})

	out, err := runRoot(t, "service", "events", "-s", "api")
	require.NoError(t, err)
	require.Contains(t, out, "event-a")     // el más reciente entra
	require.NotContains(t, out, "event-y")  // el 25º (más viejo) queda fuera
}

func TestServiceEventsVacio(t *testing.T) {
	withFakeDeployer(t, &coretest.FakeDeployer{})
	out, err := runRoot(t, "service", "events", "-s", "api")
	require.NoError(t, err)
	require.Contains(t, out, "no events")
}

func TestServiceEventsRequiereService(t *testing.T) {
	_, err := runRootWithFake(t, "service", "events")
	require.ErrorContains(t, err, "--service")
}
```

- [ ] **Step 2: Verificar que fallan**

Run: `go test ./internal/cli/ -run TestServiceEvents`
Expected: FAIL (`unknown command "events"`).

- [ ] **Step 3: Implementación**

Crear `internal/cli/service_events_cmd.go`:

```go
package cli

import (
	"fmt"

	"github.com/juanMaAV92/steer/internal/render"
	"github.com/spf13/cobra"
)

// maxEventsShown acota la tabla de events: es una vista de un vistazo,
// no una herramienta de arqueología.
const maxEventsShown = 20

func newServiceEventsCmd() *cobra.Command {
	var service string
	cmd := &cobra.Command{
		Use:   "events",
		Short: "Show recent service events",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if service == "" {
				return fmt.Errorf("--service is required")
			}
			app := FromContext(cmd.Context())
			dep, err := app.Deployer(cmd.Context())
			if err != nil {
				return err
			}
			realName := app.Ctx.ServiceName(service)
			evs, err := dep.ServiceEvents(cmd.Context(), realName)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(evs) == 0 {
				_, _ = fmt.Fprintln(out, render.Dim("no events"))
				return nil
			}
			if len(evs) > maxEventsShown {
				evs = evs[:maxEventsShown] // ServiceEvents entrega más recientes primero
			}
			headers := []string{"TIME", "MESSAGE"}
			rows := make([][]string, 0, len(evs))
			for i := len(evs) - 1; i >= 0; i-- { // ascendente: lo más nuevo abajo
				e := evs[i]
				msg := e.Message
				if e.IsError {
					msg = render.Danger(msg)
				}
				rows = append(rows, []string{render.Dim(e.At.Format("Jan 02 15:04:05")), msg})
			}
			_, _ = fmt.Fprint(out, render.Table(headers, rows))
			return nil
		},
	}
	cmd.Flags().StringVarP(&service, "service", "s", "", "service short name")
	return cmd
}
```

En `internal/cli/service_cmd.go:25`, añadir `newServiceEventsCmd()` al `AddCommand` (queda con los 7 subcomandos).

- [ ] **Step 4: Verificar que pasan**

Run: `go test ./internal/cli/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/service_events_cmd.go internal/cli/service_events_cmd_test.go internal/cli/service_cmd.go
git commit -m "feat(cli): steer service events — tabla de eventos recientes del servicio"
```

---

### Task 6: TUI — componente `panel.Logs` (viewport con auto-scroll inteligente)

**Files:**
- Modify: `internal/tui/panel/logs.go` (añadir el struct; **mantener** `LogsView()` hasta la Task 7 para no romper `app.go:1004`)
- Test: `internal/tui/panel/logs_test.go` (reescribir)

**Interfaces:**
- Consumes: `viewport` de bubbles, `render.Dim` (patrón de `panel/events.go`).
- Produces: `panel.Logs` con `NewLogs() Logs`, `SetSize(w, h int)`, `SetLines(lines []string)`, `AppendLines(lines []string)`, `Reset()`, `Update(msg tea.Msg) tea.Cmd`, `View() string`.

- [ ] **Step 1: Tests**

Reescribir `internal/tui/panel/logs_test.go` (conservando `TestLogsViewStub` hasta la Task 7):

```go
package panel

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLogsViewStub(t *testing.T) {
	require.Contains(t, strings.ToLower(LogsView()), "logs")
}

func TestLogsVacioMuestraPlaceholder(t *testing.T) {
	l := NewLogs()
	l.SetSize(40, 5)
	require.Contains(t, l.View(), "no logs yet")
}

func TestLogsSetLinesBajaAlFondo(t *testing.T) {
	l := NewLogs()
	l.SetSize(40, 3)
	l.SetLines([]string{"l1", "l2", "l3", "l4", "l5"})
	require.Contains(t, l.View(), "l5") // el fondo (lo más nuevo) queda visible
}

func TestLogsAppendMantieneElFondo(t *testing.T) {
	l := NewLogs()
	l.SetSize(40, 3)
	l.SetLines([]string{"l1", "l2", "l3", "l4", "l5"})
	l.AppendLines([]string{"l6"})
	require.Contains(t, l.View(), "l6") // estaba al fondo → sigue al fondo
}

func TestLogsAppendNoRobaElScroll(t *testing.T) {
	l := NewLogs()
	l.SetSize(40, 3)
	l.SetLines([]string{"l1", "l2", "l3", "l4", "l5"})
	l.vp.GotoTop() // el usuario subió a leer historia
	l.AppendLines([]string{"l6"})
	require.Contains(t, l.View(), "l1")    // la posición no cambia
	require.NotContains(t, l.View(), "l6") // lo nuevo no arrastra la vista
}

func TestLogsReset(t *testing.T) {
	l := NewLogs()
	l.SetSize(40, 3)
	l.SetLines([]string{"l1"})
	l.Reset()
	require.Contains(t, l.View(), "no logs yet")
}
```

- [ ] **Step 2: Verificar que fallan**

Run: `go test ./internal/tui/panel/ -run TestLogs`
Expected: FAIL (`NewLogs` no definido).

- [ ] **Step 3: Implementación**

En `internal/tui/panel/logs.go`, añadir debajo del stub `LogsView()` existente:

```go
// Logs es la pestaña de logs del servicio: viewport scrolleable alimentado por
// tail + follow (espejo estructural de Events).
type Logs struct {
	vp    viewport.Model
	lines []string
}

func NewLogs() Logs { return Logs{vp: viewport.New(0, 0)} }

func (l *Logs) SetSize(w, h int) {
	l.vp.Width = w
	l.vp.Height = h
	l.sync(false)
}

// SetLines reemplaza el contenido (tail inicial) y baja al fondo.
func (l *Logs) SetLines(lines []string) {
	l.lines = lines
	l.sync(true)
}

// AppendLines añade líneas del follow; solo baja al fondo si ya estabas al
// fondo (no roba la posición a quien subió a leer historia).
func (l *Logs) AppendLines(lines []string) {
	atBottom := l.vp.AtBottom()
	l.lines = append(l.lines, lines...)
	l.sync(atBottom)
}

func (l *Logs) Reset() {
	l.lines = nil
	l.sync(false)
}

func (l *Logs) sync(gotoBottom bool) {
	body := strings.Join(l.lines, "\n")
	if body == "" {
		body = render.Dim("no logs yet")
	}
	l.vp.SetContent(body)
	if gotoBottom {
		l.vp.GotoBottom()
	}
}

// Update delega scroll (rueda/teclas) al viewport interno.
func (l *Logs) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	l.vp, cmd = l.vp.Update(msg)
	return cmd
}

func (l Logs) View() string { return l.vp.View() }
```

Actualizar los imports del fichero:

```go
import (
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/juanMaAV92/steer/internal/render"
)
```

- [ ] **Step 4: Verificar que pasan**

Run: `go test ./internal/tui/panel/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/panel/logs.go internal/tui/panel/logs_test.go
git commit -m "feat(tui): componente panel.Logs — viewport con auto-scroll que respeta la lectura"
```

---

### Task 7: TUI — pestaña Logs viva (tail + follow, generación anti-stale)

**Files:**
- Modify: `internal/tui/app.go` (Model, New, layout, Update, handleKey, handleMouse, applyContextSwitch, panelBody; borrar la llamada a `panel.LogsView()`)
- Modify: `internal/tui/messages.go` (mensajes nuevos)
- Modify: `internal/tui/commands.go` (`logsTickCmd`)
- Modify: `internal/tui/panel/logs.go` (borrar el stub `LogsView()`)
- Modify: `internal/tui/panel/logs_test.go` (borrar `TestLogsViewStub`)
- Test: `internal/tui/app_test.go` (helper + tests nuevos)

**Interfaces:**
- Consumes: `panel.Logs` (Task 6), `provider.Logs()` (Task 3), `core.ErrNoLogSource`, `fakeFactoryWithLogs` (Task 3).
- Produces: campos de Model `logs panel.Logs; logsService string; logsCursor string; logsLoading bool; logsErr error; logsGen int`; `syncLogs() tea.Cmd`; `tailLogsCmd/followLogsCmd`; msgs `logsPageMsg{gen int; initial bool; page core.LogPage; err error}` y `logsTickMsg{gen int}`; helper `logLineView(l core.LogLine) string`; const `tuiLogLines = 100`.

**Semántica de `logsGen`:** cada vez que `syncLogs` cambia de servicio (o resetea), incrementa la generación. Los mensajes de páginas y ticks llevan la generación con la que nacieron; si al llegar no coincide con la actual, se descartan. Así un follow abandonado (cambio de pestaña/servicio/contexto y vuelta rápida) no duplica el loop de polling ni pinta contenido obsoleto.

- [ ] **Step 1: Tests**

Añadir a `internal/tui/app_test.go`:

```go
// newTestModelWithLogs es como newTestModel pero con un LogSource fake.
func newTestModelWithLogs(services []core.ServiceStatus, src core.LogSource) Model {
	fake := &coretest.FakeDeployer{Services: services}
	factory := fakeFactoryWithLogs(fake, src)
	cur := config.Context{Name: "stg", Cloud: "aws", Cluster: "stg-cluster", Writable: true}
	m := New(context.Background(), factory, []config.Context{cur}, cur)
	m.sidebar.setServices(services)
	m, _ = applySize(m, 120, 40)
	return m
}

func TestLogsTabCargaYSigue(t *testing.T) {
	t0 := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)
	src := &coretest.FakeLogSource{Pages: []core.LogPage{
		{Lines: []core.LogLine{{At: t0, Message: "hello"}}, Cursor: "c1"},
		{Lines: []core.LogLine{{At: t0.Add(time.Second), Message: "world"}}, Cursor: "c2"},
	}}
	m := newTestModelWithLogs(servicesNamed("api"), src)
	m.tabs.Set(panel.TabLogs)

	cmd := m.syncLogs()
	require.NotNil(t, cmd)
	require.Contains(t, stripANSI(m.View()), "loading logs")

	mm, _ := m.Update(cmd()) // logsPageMsg inicial
	m = mm.(Model)
	require.Contains(t, stripANSI(m.View()), "hello")

	// tick de follow → segunda página
	mm, followCmd := m.Update(logsTickMsg{gen: m.logsGen})
	m = mm.(Model)
	require.NotNil(t, followCmd)
	mm, _ = m.Update(followCmd())
	m = mm.(Model)
	require.Contains(t, stripANSI(m.View()), "world")
	require.Equal(t, "c2", m.logsCursor)
}

func TestLogsTabSinLogSourceMuestraHint(t *testing.T) {
	m := newTestModel(servicesNamed("api")) // provider sin logs → ErrNoLogSource
	m.tabs.Set(panel.TabLogs)
	cmd := m.syncLogs()
	mm, _ := m.Update(cmd())
	m = mm.(Model)
	require.Contains(t, stripANSI(m.View()), "no log source")
}

func TestLogsSeReseteanAlCambiarDeServicio(t *testing.T) {
	src := &coretest.FakeLogSource{Pages: []core.LogPage{
		{Lines: []core.LogLine{{At: time.Now(), Message: "hello"}}, Cursor: "c1"},
	}}
	m := newTestModelWithLogs(servicesNamed("api", "web"), src)
	m.tabs.Set(panel.TabLogs)
	cmd := m.syncLogs()
	mm, _ := m.Update(cmd())
	m = mm.(Model)
	gen := m.logsGen

	m.sidebar.moveDown() // selección → web
	cmd = m.syncLogs()
	require.NotNil(t, cmd)
	require.Greater(t, m.logsGen, gen) // generación nueva: lo viejo se descarta
	require.Contains(t, stripANSI(m.View()), "loading logs")
}

func TestLogsTickObsoletoNoDisparaFollow(t *testing.T) {
	src := &coretest.FakeLogSource{}
	m := newTestModelWithLogs(servicesNamed("api"), src)
	m.tabs.Set(panel.TabLogs)
	_ = m.syncLogs()

	_, cmd := m.Update(logsTickMsg{gen: m.logsGen - 1}) // tick de una sesión anterior
	require.Nil(t, cmd)
}
```

Imports nuevos que puede necesitar el fichero de test: `time`, `"github.com/juanMaAV92/steer/internal/tui/panel"` (ya está importado para otros tests; verificar).

- [ ] **Step 2: Verificar que fallan**

Run: `go test ./internal/tui/ -run TestLogs`
Expected: FAIL (compile error: `syncLogs`/`logsTickMsg` no definidos).

- [ ] **Step 3: Mensajes y tick**

En `internal/tui/messages.go`, añadir:

```go
// logsPageMsg trae una página de logs (tail inicial o follow). gen ata la
// respuesta a la sesión de follow que la pidió: si no coincide, se descarta.
type logsPageMsg struct {
	gen     int
	initial bool
	page    core.LogPage
	err     error
}

// logsTickMsg dispara el siguiente poll del follow de la sesión gen.
type logsTickMsg struct{ gen int }
```

En `internal/tui/commands.go`, añadir:

```go
func logsTickCmd(gen int) tea.Cmd {
	return tea.Tick(3*time.Second, func(time.Time) tea.Msg { return logsTickMsg{gen: gen} })
}
```

- [ ] **Step 4: Model, sync y comandos en app.go**

En `internal/tui/app.go`:

1. Campos nuevos en `Model` (tras el bloque `tagsRepo…tagsErr`):

```go
	logs        panel.Logs
	logsService string // servicio cuyo tail/follow está en pantalla ("" = inactivo)
	logsCursor  string
	logsLoading bool
	logsErr     error
	logsGen     int // generación del follow: invalida páginas/ticks obsoletos
```

2. En `New()`, junto a `events: panel.NewEvents()`: `logs: panel.NewLogs(),`

3. Constante junto a las de geometría:

```go
// tuiLogLines es el tail inicial de la pestaña Logs (mismo default que la CLI).
const tuiLogLines = 100
```

4. En `layout()`, junto a cada `m.events.SetSize(...)` (dos sitios: single column y dos columnas), añadir la línea equivalente:

```go
	m.logs.SetSize(m.panelW-2, m.bodyH/2-2) // rama singleColumn
```
```go
	m.logs.SetSize(m.panelW-2, m.bodyH-2) // rama dos columnas
```

5. Comandos y sync (junto a `syncRepoTags`):

```go
// tailLogsCmd pide el tail inicial de logs del servicio para la sesión gen.
func (m Model) tailLogsCmd(service string, gen int) tea.Cmd {
	provider := m.provider
	ctx := m.runCtx
	return func() tea.Msg {
		src, err := provider.Logs()
		if err != nil {
			return logsPageMsg{gen: gen, initial: true, err: err}
		}
		page, err := src.TailLogs(ctx, service, tuiLogLines)
		return logsPageMsg{gen: gen, initial: true, page: page, err: err}
	}
}

// followLogsCmd pide las líneas posteriores al cursor para la sesión gen.
func (m Model) followLogsCmd(service, cursor string, gen int) tea.Cmd {
	provider := m.provider
	ctx := m.runCtx
	return func() tea.Msg {
		src, err := provider.Logs()
		if err != nil {
			return logsPageMsg{gen: gen, err: err}
		}
		page, err := src.FollowLogs(ctx, service, cursor)
		return logsPageMsg{gen: gen, page: page, err: err}
	}
}

// syncLogs arranca/detiene el tail+follow según pestaña y selección. Cambiar
// de servicio, pestaña o contexto resetea el contenido y sube la generación
// (las respuestas y ticks de la sesión anterior se descartan al llegar).
func (m *Model) syncLogs() tea.Cmd {
	sel := ""
	if m.tabs.Active == panel.TabLogs && m.sidebar.lastSelected != sectionImages {
		if s, ok := m.sidebar.selected(); ok {
			sel = s.Name
		}
	}
	if sel == m.logsService {
		return nil
	}
	m.logsGen++
	m.logs.Reset()
	m.logsService, m.logsCursor, m.logsErr = sel, "", nil
	if sel == "" {
		m.logsLoading = false
		return nil
	}
	m.logsLoading = true
	return m.tailLogsCmd(sel, m.logsGen)
}

// logLineView formatea una línea de log para la pestaña Logs.
func logLineView(l core.LogLine) string {
	line := render.Dim(l.At.Format("15:04:05")) + "  "
	if l.Container != "" {
		line += render.Accent("["+l.Container+"]") + "  "
	}
	return line + l.Message
}
```

6. Casos nuevos en `Update()` (junto a `tagsMsg`):

```go
	case logsPageMsg:
		if msg.gen != m.logsGen {
			return m, nil // respuesta de una sesión de follow abandonada
		}
		m.logsLoading = false
		if msg.err != nil {
			m.logsErr = msg.err
			return m, nil
		}
		m.logsCursor = msg.page.Cursor
		lines := make([]string, 0, len(msg.page.Lines))
		for _, l := range msg.page.Lines {
			lines = append(lines, logLineView(l))
		}
		if msg.initial {
			m.logs.SetLines(lines)
		} else if len(lines) > 0 {
			m.logs.AppendLines(lines)
		}
		return m, logsTickCmd(m.logsGen)

	case logsTickMsg:
		if msg.gen != m.logsGen || m.logsService == "" || m.logsErr != nil {
			return m, nil // la sesión murió: el loop se corta aquí
		}
		return m, m.followLogsCmd(m.logsService, m.logsCursor, m.logsGen)
```

7. Routing de scroll por pestaña. En `handleKey`, rama `focusPanel` Up/Down:

```go
		case key.Matches(msg, m.keys.Down), key.Matches(msg, m.keys.Up):
			if m.tabs.Active == panel.TabLogs {
				cmd := m.logs.Update(msg)
				return m, cmd
			}
			cmd := m.events.Update(msg)
			return m, cmd
```

y el cambio de pestaña Right/Left pasa a devolver el sync:

```go
		switch {
		case key.Matches(msg, m.keys.Right):
			m.tabs.Next()
		case key.Matches(msg, m.keys.Left):
			m.tabs.Prev()
		}
		return m, m.syncLogs()
```

En `handleMouse`, la rueda sobre el panel:

```go
		if m.tabs.Active == panel.TabLogs {
			return m.logs.Update(msg)
		}
		return m.events.Update(msg)
```

el click en sidebar (rama `entryService, entryRepo`) pasa de `return m.syncRepoTags()` a:

```go
			return tea.Batch(m.syncRepoTags(), m.syncLogs())
```

y el final de `handleMouse` (tras el hit-testing de pestañas) pasa de `return nil` a:

```go
	m.focus = focusPanel
	return m.syncLogs()
```

En `handleKey`, el `return m, m.syncRepoTags()` final (rama sidebar) pasa a:

```go
	return m, tea.Batch(m.syncRepoTags(), m.syncLogs())
```

8. En `applyContextSwitch`, junto al reset de tags (línea ~764):

```go
	m.logs.Reset()
	m.logsService, m.logsCursor, m.logsErr, m.logsLoading = "", "", nil, false
	m.logsGen++
```

9. En `panelBody()`, reemplazar el caso `TabLogs`:

```go
	case panel.TabLogs:
		switch {
		case m.logsLoading:
			return render.Dim("loading logs…")
		case m.logsErr != nil:
			if errors.Is(m.logsErr, core.ErrNoLogSource) {
				return render.Dim(m.logsErr.Error())
			}
			return render.Danger("logs error: " + providers.Friendly(m.logsErr))
		default:
			return m.logs.View()
		}
```

10. Borrar `LogsView()` de `internal/tui/panel/logs.go` y `TestLogsViewStub` de `internal/tui/panel/logs_test.go` (ya no hay llamadores).

- [ ] **Step 5: Verificar que pasan**

Run: `go test ./internal/tui/... && go build ./...`
Expected: PASS (incluidos los tests preexistentes de tabs, clicks y deploy).

- [ ] **Step 6: Commit**

```bash
git add internal/tui/app.go internal/tui/messages.go internal/tui/commands.go internal/tui/panel/logs.go internal/tui/panel/logs_test.go internal/tui/app_test.go
git commit -m "feat(tui): pestaña Logs viva — tail + follow con generación anti-stale"
```

---

### Task 8: TUI — pestaña Events con histórico en reposo

**Files:**
- Modify: `internal/tui/app.go` (Model, Update, syncEvents, tick, applyContextSwitch, applyActionConfirmed, panelBody, helper `eventLine`)
- Modify: `internal/tui/messages.go` (`serviceEventsMsg`)
- Test: `internal/tui/app_test.go`

**Interfaces:**
- Consumes: `Deployer.ServiceEvents` (existente), `panel.Events` (existente), `syncLogs` ya cableado en los mismos puntos (Task 7).
- Produces: campos de Model `eventsService string; eventsLastID string; eventsErr string`; `syncEvents() tea.Cmd`; `loadServiceEventsCmd(service string) tea.Cmd`; msg `serviceEventsMsg{service string; events []core.ServiceEvent; err error}`; helper `eventLine(e core.ServiceEvent) string`.

**Regla de propiedad de la pestaña** (del spec): el feed de deploy en vivo es dueño mientras `deploy.Active`, y su resultado sigue en pantalla mientras `deploy.Done` y la selección siga siendo ese servicio. En cualquier otro caso, la pestaña muestra el histórico del servicio seleccionado, refrescado por el tick de 15s.

- [ ] **Step 1: Tests**

Añadir a `internal/tui/app_test.go`:

```go
func TestEventsTabMuestraHistorico(t *testing.T) {
	t0 := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)
	fake := &coretest.FakeDeployer{
		Services: servicesNamed("api"),
		Events: []core.ServiceEvent{ // más recientes primero (contrato)
			{ID: "2", At: t0.Add(time.Minute), Message: "reached steady state"},
			{ID: "1", At: t0, Message: "started 2 tasks"},
		},
	}
	m := newTestModelWithDeployer(t, fake)
	m.tabs.Set(panel.TabEvents)

	cmd := m.syncEvents()
	require.NotNil(t, cmd)
	mm, _ := m.Update(cmd())
	m = mm.(Model)
	view := stripANSI(m.View())
	require.Contains(t, view, "started 2 tasks")
	require.Contains(t, view, "reached steady state")
	// ascendente: lo más nuevo al fondo
	require.Less(t, strings.Index(view, "started 2 tasks"), strings.Index(view, "reached steady state"))
}

func TestEventsHistoricoNoInterrumpeElDeploy(t *testing.T) {
	m := newTestModel(servicesNamed("api"))
	m.tabs.Set(panel.TabEvents)
	m.deploy = deployState{Active: true, Service: "api"}
	require.Nil(t, m.syncEvents()) // el feed en vivo es dueño de la pestaña
}

func TestEventsHistoricoSinNovedadesNoRepinta(t *testing.T) {
	t0 := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)
	fake := &coretest.FakeDeployer{
		Services: servicesNamed("api"),
		Events:   []core.ServiceEvent{{ID: "1", At: t0, Message: "steady"}},
	}
	m := newTestModelWithDeployer(t, fake)
	m.tabs.Set(panel.TabEvents)
	mm, _ := m.Update(m.syncEvents()())
	m = mm.(Model)
	require.Equal(t, "1", m.eventsLastID)

	// segunda respuesta idéntica (refresh del tick): no resetea el viewport
	mm, _ = m.Update(serviceEventsMsg{service: "api", events: fake.Events})
	m = mm.(Model)
	require.Equal(t, "1", m.eventsLastID)
	require.Contains(t, stripANSI(m.View()), "steady")
}

func TestLoadServiceEventsCmdProduceMsg(t *testing.T) {
	t0 := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)
	fake := &coretest.FakeDeployer{
		Services: servicesNamed("api"),
		Events:   []core.ServiceEvent{{ID: "1", At: t0, Message: "steady"}},
	}
	m := newTestModelWithDeployer(t, fake)

	msg := m.loadServiceEventsCmd("api")()
	evMsg, ok := msg.(serviceEventsMsg)
	require.True(t, ok)
	require.Equal(t, "api", evMsg.service)
	require.Len(t, evMsg.events, 1)
}
```

Nota: NO ejecutar el `tea.Cmd` que devuelve el caso `tickMsg` en tests — el batch incluye `tickCmd()` (un `tea.Tick` que duerme 15s). El refresh por tick queda cubierto indirectamente: el guard del batch es trivial y `loadServiceEventsCmd` se testea arriba de forma directa.

- [ ] **Step 2: Verificar que fallan**

Run: `go test ./internal/tui/ -run TestEvents`
Expected: FAIL (`syncEvents` no definido).

- [ ] **Step 3: Implementación**

En `internal/tui/messages.go`:

```go
// serviceEventsMsg trae el histórico de eventos del servicio (pestaña Events
// en reposo; el feed de deploy en vivo no pasa por aquí).
type serviceEventsMsg struct {
	service string
	events  []core.ServiceEvent
	err     error
}
```

En `internal/tui/app.go`:

1. Campos de Model (junto a los de logs):

```go
	eventsService string // servicio cuyo histórico está en la pestaña Events
	eventsLastID  string // ID del evento más reciente pintado (dedup del refresh)
	eventsErr     string
```

2. Helper y comando (junto a `syncLogs`):

```go
// eventLine formatea un evento del servicio para el feed del panel.
func eventLine(e core.ServiceEvent) string {
	line := "[" + e.At.Format("15:04:05") + "] " + e.Message
	if e.IsError {
		return render.Danger(line)
	}
	return render.Dim(line)
}

// loadServiceEventsCmd pide el histórico de eventos del servicio.
func (m Model) loadServiceEventsCmd(service string) tea.Cmd {
	dep := m.dep
	ctx := m.runCtx
	return func() tea.Msg {
		evs, err := dep.ServiceEvents(ctx, service)
		return serviceEventsMsg{service: service, events: evs, err: err}
	}
}

// syncEvents dispara la carga del histórico si la pestaña Events está visible
// para un servicio distinto del cargado. El feed de deploy tiene prioridad:
// activo, o terminado con su servicio aún seleccionado, no se toca.
func (m *Model) syncEvents() tea.Cmd {
	if m.tabs.Active != panel.TabEvents || m.sidebar.lastSelected == sectionImages {
		return nil
	}
	s, ok := m.sidebar.selected()
	if !ok {
		return nil
	}
	if m.deploy.Active {
		return nil
	}
	if m.deploy.Done && m.deploy.Service == s.Name {
		return nil // el resultado del deploy sigue en pantalla
	}
	if m.eventsService == s.Name {
		return nil
	}
	if m.deploy.Done {
		m.deploy.Reset() // feed de otro servicio: se abandona
	}
	m.eventsService = s.Name
	m.eventsLastID = ""
	m.eventsErr = ""
	m.events.Reset()
	return m.loadServiceEventsCmd(s.Name)
}
```

3. Caso en `Update()`:

```go
	case serviceEventsMsg:
		if msg.service != m.eventsService || m.deploy.Active {
			return m, nil // obsoleto, o el feed de deploy tomó la pestaña
		}
		if msg.err != nil {
			m.eventsErr = msg.err.Error()
			return m, nil
		}
		m.eventsErr = ""
		if len(msg.events) > 0 && msg.events[0].ID == m.eventsLastID {
			return m, nil // sin novedades: no repintar (conserva el scroll)
		}
		m.eventsLastID = ""
		if len(msg.events) > 0 {
			m.eventsLastID = msg.events[0].ID
		}
		m.events.Reset()
		for i := len(msg.events) - 1; i >= 0; i-- { // ascendente: lo nuevo al fondo
			m.events.AppendLine(eventLine(msg.events[i]))
		}
		return m, nil
```

4. Refresh por tick — reemplazar el caso `tickMsg`:

```go
	case tickMsg:
		cmds := []tea.Cmd{m.loadServicesCmd(), tickCmd()}
		if m.tabs.Active == panel.TabEvents && m.eventsService != "" &&
			!m.deploy.Active && !m.deploy.Done {
			cmds = append(cmds, m.loadServiceEventsCmd(m.eventsService))
		}
		return m, tea.Batch(cmds...)
```

5. Cablear `syncEvents` en los mismos puntos donde la Task 7 puso `syncLogs` (los cuatro): Right/Left de `handleKey` → `return m, tea.Batch(m.syncEvents(), m.syncLogs())`; rama sidebar de `handleKey` → `return m, tea.Batch(m.syncRepoTags(), m.syncEvents(), m.syncLogs())`; click en entry de `handleMouse` → `return tea.Batch(m.syncRepoTags(), m.syncEvents(), m.syncLogs())`; final de `handleMouse` → `return tea.Batch(m.syncEvents(), m.syncLogs())`.

6. En `applyContextSwitch` (junto al reset de logs):

```go
	m.eventsService, m.eventsLastID, m.eventsErr = "", "", ""
```

7. En `applyActionConfirmed`, tras `m.events.Reset()` (línea ~472): el deploy toma la pestaña, el histórico se invalida:

```go
		m.eventsService, m.eventsLastID, m.eventsErr = "", "", ""
```

8. En `panelBody()`, caso `TabEvents`:

```go
	case panel.TabEvents:
		if m.eventsErr != "" && !m.deploy.Active && !m.deploy.Done {
			return render.Danger("events error: " + m.eventsErr)
		}
		return m.events.View()
```

9. Refactor pequeño: en el handler de `deployPollMsg`, reemplazar el formateo inline del evento (líneas ~370-377) por `m.events.AppendLine(eventLine(e))` conservando el conteo de `IsProvisioningFailure`.

- [ ] **Step 4: Verificar que pasan**

Run: `go test ./internal/tui/... && go test ./...`
Expected: PASS (los tests de deploy existentes intactos: el feed en vivo no cambió).

- [ ] **Step 5: Commit**

```bash
git add internal/tui/app.go internal/tui/messages.go internal/tui/app_test.go
git commit -m "feat(tui): pestaña Events con histórico en reposo — el feed de deploy conserva la prioridad"
```

---

### Task 9: Docs — README, paridad y roadmap

**Files:**
- Modify: `README.md` (quick start ~línea 128 y roadmap ~línea 149)
- Modify: `docs/parity.md` (matriz + notas 3-4 + fecha)
- Modify: `docs/superpowers/plans/2026-06-15-roadmap.md` (fila 03b + secciones de estado)

- [ ] **Step 1: README**

En el bloque de quick start (tras `steer service rollback -s my-svc`, línea ~132), añadir:

```
steer service logs -s my-svc -f
steer service events -s my-svc
```

En la línea del roadmap (~149) `7. service logs/events, db, promote, ⌘k palette, minor capabilities`: quitar `service logs/events` de pendientes y reflejarlo como hecho en la lista de hitos completados del README (seguir el estilo de las entradas existentes; leer el bloque completo antes de editar).

- [ ] **Step 2: parity.md**

- Actualizar `**Actualizado:** 2026-07-07` a la fecha del cierre.
- Filas de la matriz:

```markdown
| Logs del servicio | `service logs -s [-f] [-n]` (tail 1h, merge multi-contenedor) | Pestaña Logs viva (tail + follow 3s) | ✅ |
| Events históricos del servicio | `service events -s` (últimos 20, ascendente) | Pestaña Events poblada en reposo (tick 15s) | ✅ |
```

- Reescribir las notas 3 y 4 en estilo cerrado (como la nota 2), explicando: hito 03b los cerró; `core.LogSource` con auto-discovery desde la task definition (driver awslogs, cero config); "últimas N líneas" acotado a la última hora por la semántica forward-only de FilterLogEvents; el feed de deploy conserva la propiedad de la pestaña Events durante un rollout; driver no soportado → `ErrNoLogSource` con mensaje explicativo.

- [ ] **Step 3: roadmap**

En `docs/superpowers/plans/2026-06-15-roadmap.md`:

- Añadir la fila a la tabla:

```markdown
| 03b | **Logs + events** | `core.LogSource` (CloudWatch, auto-discovery de la task def), `service logs -s [-f]` y `service events -s`, pestañas Logs (follow) y Events (histórico) vivas. Cierra los gaps de paridad doble (parity.md notas 3-4). | 02, 03 | ✅ hecho (plan `2026-07-09-logs-events.md`) |
```

- En **Estado y por dónde vamos**: añadir 03b a la lista de mergeado; quitar "03b logs/events" de **Pendiente**; actualizar **Próximo recomendado** a `04 db` (o `02b`, a decidir al cerrar).

- [ ] **Step 4: Verificación final del hito**

Run: `go build ./... && go test ./... && make lint`
Expected: todo verde.

- [ ] **Step 5: Commit**

```bash
git add README.md docs/parity.md docs/superpowers/plans/2026-06-15-roadmap.md
git commit -m "docs: cierre del hito 03b — paridad de logs/events en README, parity y roadmap"
```
