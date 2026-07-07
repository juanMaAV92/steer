package aws

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/juanMaAV92/steer/internal/core"
)

// ecsAPI es el subconjunto del cliente ECS que usa el deployer.
// El *ecs.Client del SDK lo satisface; los tests inyectan un fake.
type ecsAPI interface {
	ListServices(ctx context.Context, in *ecs.ListServicesInput, optFns ...func(*ecs.Options)) (*ecs.ListServicesOutput, error)
	DescribeServices(ctx context.Context, in *ecs.DescribeServicesInput, optFns ...func(*ecs.Options)) (*ecs.DescribeServicesOutput, error)
	DescribeTaskDefinition(ctx context.Context, in *ecs.DescribeTaskDefinitionInput, optFns ...func(*ecs.Options)) (*ecs.DescribeTaskDefinitionOutput, error)
	RegisterTaskDefinition(ctx context.Context, in *ecs.RegisterTaskDefinitionInput, optFns ...func(*ecs.Options)) (*ecs.RegisterTaskDefinitionOutput, error)
	UpdateService(ctx context.Context, in *ecs.UpdateServiceInput, optFns ...func(*ecs.Options)) (*ecs.UpdateServiceOutput, error)
	ListTaskDefinitions(ctx context.Context, in *ecs.ListTaskDefinitionsInput, optFns ...func(*ecs.Options)) (*ecs.ListTaskDefinitionsOutput, error)
	ListClusters(ctx context.Context, in *ecs.ListClustersInput, optFns ...func(*ecs.Options)) (*ecs.ListClustersOutput, error)
}

// ECSDeployer implementa core.Deployer sobre AWS ECS.
type ECSDeployer struct {
	api     ecsAPI
	cluster string
}

// NewDeployer crea un ECSDeployer desde una aws.Config.
func NewDeployer(cfg awssdk.Config, cluster string) *ECSDeployer {
	return newDeployer(ecs.NewFromConfig(cfg), cluster)
}

// newDeployer es el constructor inyectable usado por los tests.
func newDeployer(api ecsAPI, cluster string) *ECSDeployer {
	return &ECSDeployer{api: api, cluster: cluster}
}

// ListServices devuelve el estado de los servicios del cluster.
func (d *ECSDeployer) ListServices(ctx context.Context) ([]core.ServiceStatus, error) {
	var arns []string
	var token *string
	for {
		list, err := d.api.ListServices(ctx, &ecs.ListServicesInput{Cluster: awssdk.String(d.cluster), NextToken: token})
		if err != nil {
			return nil, err
		}
		arns = append(arns, list.ServiceArns...)
		if awssdk.ToString(list.NextToken) == "" {
			break
		}
		token = list.NextToken
	}
	if len(arns) == 0 {
		return nil, nil
	}
	var out []core.ServiceStatus
	for _, batch := range chunk(arns, 10) { // ECS DescribeServices: máx 10 por llamada
		desc, err := d.api.DescribeServices(ctx, &ecs.DescribeServicesInput{
			Cluster:  awssdk.String(d.cluster),
			Services: batch,
		})
		if err != nil {
			return nil, err
		}
		for _, s := range desc.Services {
			tag, res := d.taskDefInfo(ctx, awssdk.ToString(s.TaskDefinition))
			out = append(out, core.ServiceStatus{
				Name:      awssdk.ToString(s.ServiceName),
				Running:   int(s.RunningCount),
				Desired:   int(s.DesiredCount),
				Pending:   int(s.PendingCount),
				Status:    awssdk.ToString(s.Status),
				Tag:       tag,
				Resources: res,
			})
		}
	}
	return out, nil
}

// DeploymentStatus devuelve el estado del rollout activo (PRIMARY) de un servicio.
func (d *ECSDeployer) DeploymentStatus(ctx context.Context, service string) (core.Deployment, error) {
	desc, err := d.api.DescribeServices(ctx, &ecs.DescribeServicesInput{
		Cluster:  awssdk.String(d.cluster),
		Services: []string{service},
	})
	if err != nil {
		return core.Deployment{}, err
	}
	if len(desc.Services) == 0 {
		return core.Deployment{}, fmt.Errorf("service %q not found in cluster %q", service, d.cluster)
	}
	s := desc.Services[0]
	for _, dep := range s.Deployments {
		if awssdk.ToString(dep.Status) == "PRIMARY" {
			return core.Deployment{
				Rollout: core.RolloutState(dep.RolloutState),
				Running: int(dep.RunningCount),
				Pending: int(dep.PendingCount),
				Desired: int(s.DesiredCount), // desired autoritativo del servicio
			}, nil
		}
	}
	// Sin deployment PRIMARY: reporta los contadores del servicio.
	return core.Deployment{
		Running: int(s.RunningCount),
		Pending: int(s.PendingCount),
		Desired: int(s.DesiredCount),
	}, nil
}

// ServiceEvents devuelve los eventos del servicio (ECS los entrega más recientes primero).
func (d *ECSDeployer) ServiceEvents(ctx context.Context, service string) ([]core.ServiceEvent, error) {
	desc, err := d.api.DescribeServices(ctx, &ecs.DescribeServicesInput{
		Cluster:  awssdk.String(d.cluster),
		Services: []string{service},
	})
	if err != nil {
		return nil, err
	}
	if len(desc.Services) == 0 {
		return nil, fmt.Errorf("service %q not found in cluster %q", service, d.cluster)
	}
	var out []core.ServiceEvent
	for _, e := range desc.Services[0].Events {
		msg := awssdk.ToString(e.Message)
		out = append(out, core.ServiceEvent{
			ID:      awssdk.ToString(e.Id),
			At:      awssdk.ToTime(e.CreatedAt),
			Message: msg,
			IsError: strings.Contains(msg, "unable to place") || strings.Contains(msg, "ResourceInitializationError"),
		})
	}
	return out, nil
}

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

func chunk(xs []string, n int) [][]string {
	var batches [][]string
	for i := 0; i < len(xs); i += n {
		end := i + n
		if end > len(xs) {
			end = len(xs)
		}
		batches = append(batches, xs[i:end])
	}
	return batches
}

var _ core.Deployer = (*ECSDeployer)(nil)

// currentTaskDef obtiene la task definition activa de un servicio.
func (d *ECSDeployer) currentTaskDef(ctx context.Context, service string) (*ecstypes.TaskDefinition, error) {
	desc, err := d.api.DescribeServices(ctx, &ecs.DescribeServicesInput{
		Cluster:  awssdk.String(d.cluster),
		Services: []string{service},
	})
	if err != nil {
		return nil, err
	}
	if len(desc.Services) == 0 {
		return nil, fmt.Errorf("service %q not found in cluster %q", service, d.cluster)
	}
	tdArn := awssdk.ToString(desc.Services[0].TaskDefinition)
	td, err := d.api.DescribeTaskDefinition(ctx, &ecs.DescribeTaskDefinitionInput{
		TaskDefinition: awssdk.String(tdArn),
	})
	if err != nil {
		return nil, err
	}
	return td.TaskDefinition, nil
}

// tagFromImage extrae el tag de una imagen "repo:tag" (cadena vacía si no hay tag).
func tagFromImage(image string) string {
	i := strings.LastIndex(image, ":")
	if i < 0 {
		return ""
	}
	return image[i+1:]
}

// CurrentTag devuelve el tag de imagen del primer contenedor del servicio.
func (d *ECSDeployer) CurrentTag(ctx context.Context, service string) (string, error) {
	td, err := d.currentTaskDef(ctx, service)
	if err != nil {
		return "", err
	}
	if len(td.ContainerDefinitions) == 0 {
		return "", fmt.Errorf("task definition for %q has no containers", service)
	}
	return tagFromImage(awssdk.ToString(td.ContainerDefinitions[0].Image)), nil
}

// replaceTag sustituye el tag de una imagen "repo:tag" por newTag.
func replaceTag(image, newTag string) string {
	i := strings.LastIndex(image, ":")
	if i < 0 {
		return image + ":" + newTag
	}
	return image[:i+1] + newTag
}

// Scale ajusta el desired count del servicio.
func (d *ECSDeployer) Scale(ctx context.Context, service string, count int) error {
	_, err := d.api.UpdateService(ctx, &ecs.UpdateServiceInput{
		Cluster:      awssdk.String(d.cluster),
		Service:      awssdk.String(service),
		DesiredCount: awssdk.Int32(int32(count)),
	})
	return err
}

// Rollback apunta el servicio a la revisión de task def inmediatamente anterior.
func (d *ECSDeployer) Rollback(ctx context.Context, service string) error {
	td, err := d.currentTaskDef(ctx, service)
	if err != nil {
		return err
	}
	family := awssdk.ToString(td.Family)
	list, err := d.api.ListTaskDefinitions(ctx, &ecs.ListTaskDefinitionsInput{
		FamilyPrefix: awssdk.String(family),
		Sort:         ecstypes.SortOrderDesc,
	})
	if err != nil {
		return err
	}
	if len(list.TaskDefinitionArns) < 2 {
		return fmt.Errorf("no previous revision to roll back to for %q", service)
	}
	prev := list.TaskDefinitionArns[1]
	_, err = d.api.UpdateService(ctx, &ecs.UpdateServiceInput{
		Cluster:        awssdk.String(d.cluster),
		Service:        awssdk.String(service),
		TaskDefinition: awssdk.String(prev),
	})
	return err
}

// Deploy registra una nueva task def con la imagen re-tageada y apunta el servicio a ella.
func (d *ECSDeployer) Deploy(ctx context.Context, service, tag string, log core.StepLogger) error {
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
	if len(td.ContainerDefinitions) == 0 {
		return fmt.Errorf("task definition for %q has no containers", service)
	}
	return d.registerRevision(ctx, service, td, log, func(in *ecs.RegisterTaskDefinitionInput) {
		in.ContainerDefinitions[0].Image = awssdk.String(replaceTag(awssdk.ToString(in.ContainerDefinitions[0].Image), tag))
	})
}

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
