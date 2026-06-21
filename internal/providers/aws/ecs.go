package aws

import (
	"context"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
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
	list, err := d.api.ListServices(ctx, &ecs.ListServicesInput{Cluster: awssdk.String(cluster)})
	if err != nil {
		return nil, err
	}
	if len(list.ServiceArns) == 0 {
		return nil, nil
	}
	var out []core.ServiceStatus
	for _, batch := range chunk(list.ServiceArns, 10) { // ECS DescribeServices: máx 10 por llamada
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

// compile-time check parcial (se completa en tasks siguientes).
var _ interface {
	ListServices(context.Context, string) ([]core.ServiceStatus, error)
} = (*ECSDeployer)(nil)
