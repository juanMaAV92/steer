package aws

import (
	"context"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	ecstypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/stretchr/testify/require"
)

// fakeECS implementa ecsAPI con respuestas y registro de llamadas.
type fakeECS struct {
	listOut     *ecs.ListServicesOutput
	describeOut *ecs.DescribeServicesOutput
	taskDefOut  *ecs.DescribeTaskDefinitionOutput
	registerOut *ecs.RegisterTaskDefinitionOutput
	listTDOut   *ecs.ListTaskDefinitionsOutput

	registerIn *ecs.RegisterTaskDefinitionInput
	updateIn   *ecs.UpdateServiceInput
}

func (f *fakeECS) ListServices(_ context.Context, _ *ecs.ListServicesInput, _ ...func(*ecs.Options)) (*ecs.ListServicesOutput, error) {
	return f.listOut, nil
}
func (f *fakeECS) DescribeServices(_ context.Context, _ *ecs.DescribeServicesInput, _ ...func(*ecs.Options)) (*ecs.DescribeServicesOutput, error) {
	return f.describeOut, nil
}
func (f *fakeECS) DescribeTaskDefinition(_ context.Context, _ *ecs.DescribeTaskDefinitionInput, _ ...func(*ecs.Options)) (*ecs.DescribeTaskDefinitionOutput, error) {
	return f.taskDefOut, nil
}
func (f *fakeECS) RegisterTaskDefinition(_ context.Context, in *ecs.RegisterTaskDefinitionInput, _ ...func(*ecs.Options)) (*ecs.RegisterTaskDefinitionOutput, error) {
	f.registerIn = in
	return f.registerOut, nil
}
func (f *fakeECS) UpdateService(_ context.Context, in *ecs.UpdateServiceInput, _ ...func(*ecs.Options)) (*ecs.UpdateServiceOutput, error) {
	f.updateIn = in
	return &ecs.UpdateServiceOutput{}, nil
}
func (f *fakeECS) ListTaskDefinitions(_ context.Context, _ *ecs.ListTaskDefinitionsInput, _ ...func(*ecs.Options)) (*ecs.ListTaskDefinitionsOutput, error) {
	return f.listTDOut, nil
}

func TestListServices(t *testing.T) {
	f := &fakeECS{
		listOut: &ecs.ListServicesOutput{ServiceArns: []string{"arn:svc/catalog"}},
		describeOut: &ecs.DescribeServicesOutput{Services: []ecstypes.Service{{
			ServiceName:  awssdk.String("catalog"),
			RunningCount: 2,
			DesiredCount: 3,
		}}},
	}
	d := newDeployer(f)

	got, err := d.ListServices(context.Background(), "stg-cluster")
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "catalog", got[0].Name)
	require.Equal(t, 2, got[0].Running)
	require.Equal(t, 3, got[0].Desired)
}

func TestTagFromImage(t *testing.T) {
	require.Equal(t, "v1.2.3", tagFromImage("123.dkr.ecr.us-east-1.amazonaws.com/catalog:v1.2.3"))
	require.Equal(t, "", tagFromImage("no-tag-image"))
}

func TestCurrentTag(t *testing.T) {
	f := &fakeECS{
		describeOut: &ecs.DescribeServicesOutput{Services: []ecstypes.Service{{
			TaskDefinition: awssdk.String("arn:td/catalog:5"),
		}}},
		taskDefOut: &ecs.DescribeTaskDefinitionOutput{TaskDefinition: &ecstypes.TaskDefinition{
			ContainerDefinitions: []ecstypes.ContainerDefinition{{
				Image: awssdk.String("123.dkr.ecr.us-east-1.amazonaws.com/catalog:v1.2.3"),
			}},
		}},
	}
	d := newDeployer(f)

	tag, err := d.CurrentTag(context.Background(), "stg-cluster", "catalog")
	require.NoError(t, err)
	require.Equal(t, "v1.2.3", tag)
}
