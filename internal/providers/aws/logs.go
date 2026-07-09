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
