package aws

import (
	"context"
	"fmt"
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
}

// ECSDeployer implementa core.Deployer sobre AWS ECS.
type ECSDeployer struct {
	api ecsAPI
}

// NewDeployer crea un ECSDeployer desde una aws.Config.
func NewDeployer(cfg awssdk.Config) *ECSDeployer {
	return &ECSDeployer{api: ecs.NewFromConfig(cfg)}
}

// newDeployer es el constructor inyectable usado por los tests.
func newDeployer(api ecsAPI) *ECSDeployer {
	return &ECSDeployer{api: api}
}

// ListServices devuelve el estado de los servicios del cluster.
func (d *ECSDeployer) ListServices(ctx context.Context, cluster string) ([]core.ServiceStatus, error) {
	var arns []string
	var token *string
	for {
		list, err := d.api.ListServices(ctx, &ecs.ListServicesInput{Cluster: awssdk.String(cluster), NextToken: token})
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
			Cluster:  awssdk.String(cluster),
			Services: batch,
		})
		if err != nil {
			return nil, err
		}
		for _, s := range desc.Services {
			out = append(out, core.ServiceStatus{
				Name:    awssdk.ToString(s.ServiceName),
				Running: int(s.RunningCount),
				Desired: int(s.DesiredCount),
			})
		}
	}
	return out, nil
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
func (d *ECSDeployer) currentTaskDef(ctx context.Context, cluster, service string) (*ecstypes.TaskDefinition, error) {
	desc, err := d.api.DescribeServices(ctx, &ecs.DescribeServicesInput{
		Cluster:  awssdk.String(cluster),
		Services: []string{service},
	})
	if err != nil {
		return nil, err
	}
	if len(desc.Services) == 0 {
		return nil, fmt.Errorf("service %q not found in cluster %q", service, cluster)
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
func (d *ECSDeployer) CurrentTag(ctx context.Context, cluster, service string) (string, error) {
	td, err := d.currentTaskDef(ctx, cluster, service)
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
func (d *ECSDeployer) Scale(ctx context.Context, cluster, service string, count int) error {
	_, err := d.api.UpdateService(ctx, &ecs.UpdateServiceInput{
		Cluster:      awssdk.String(cluster),
		Service:      awssdk.String(service),
		DesiredCount: awssdk.Int32(int32(count)),
	})
	return err
}

// Rollback apunta el servicio a la revisión de task def inmediatamente anterior.
func (d *ECSDeployer) Rollback(ctx context.Context, cluster, service string) error {
	td, err := d.currentTaskDef(ctx, cluster, service)
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
		Cluster:        awssdk.String(cluster),
		Service:        awssdk.String(service),
		TaskDefinition: awssdk.String(prev),
	})
	return err
}

// Deploy registra una nueva task def con la imagen re-tageada y apunta el servicio a ella.
func (d *ECSDeployer) Deploy(ctx context.Context, cluster, service, tag string) error {
	td, err := d.currentTaskDef(ctx, cluster, service)
	if err != nil {
		return err
	}
	if len(td.ContainerDefinitions) == 0 {
		return fmt.Errorf("task definition for %q has no containers", service)
	}
	containers := make([]ecstypes.ContainerDefinition, len(td.ContainerDefinitions))
	copy(containers, td.ContainerDefinitions)
	containers[0].Image = awssdk.String(replaceTag(awssdk.ToString(containers[0].Image), tag))

	reg, err := d.api.RegisterTaskDefinition(ctx, &ecs.RegisterTaskDefinitionInput{
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
		InferenceAccelerators:   td.InferenceAccelerators,
		PidMode:                 td.PidMode,
		IpcMode:                 td.IpcMode,
	})
	if err != nil {
		return err
	}

	_, err = d.api.UpdateService(ctx, &ecs.UpdateServiceInput{
		Cluster:        awssdk.String(cluster),
		Service:        awssdk.String(service),
		TaskDefinition: reg.TaskDefinition.TaskDefinitionArn,
	})
	return err
}
