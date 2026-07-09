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
