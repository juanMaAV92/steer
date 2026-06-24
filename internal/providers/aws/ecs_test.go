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
	listPages   []*ecs.ListServicesOutput
	listIdx     int
	describeOut *ecs.DescribeServicesOutput
	taskDefOut  *ecs.DescribeTaskDefinitionOutput
	registerOut *ecs.RegisterTaskDefinitionOutput
	listTDOut   *ecs.ListTaskDefinitionsOutput

	registerIn *ecs.RegisterTaskDefinitionInput
	updateIn   *ecs.UpdateServiceInput
}

func (f *fakeECS) ListServices(_ context.Context, _ *ecs.ListServicesInput, _ ...func(*ecs.Options)) (*ecs.ListServicesOutput, error) {
	if f.listIdx >= len(f.listPages) {
		return &ecs.ListServicesOutput{}, nil
	}
	out := f.listPages[f.listIdx]
	f.listIdx++
	return out, nil
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
		listPages: []*ecs.ListServicesOutput{{ServiceArns: []string{"arn:svc/catalog"}}},
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

func TestReplaceTag(t *testing.T) {
	require.Equal(t, "repo:v2", replaceTag("repo:v1", "v2"))
	require.Equal(t, "host/repo:v2", replaceTag("host/repo:v1", "v2"))
	require.Equal(t, "no-tag:v2", replaceTag("no-tag", "v2"))
}

func TestDeployRegistersAndUpdates(t *testing.T) {
	f := &fakeECS{
		describeOut: &ecs.DescribeServicesOutput{Services: []ecstypes.Service{{
			TaskDefinition: awssdk.String("arn:td/catalog:5"),
		}}},
		taskDefOut: &ecs.DescribeTaskDefinitionOutput{TaskDefinition: &ecstypes.TaskDefinition{
			Family: awssdk.String("catalog"),
			ContainerDefinitions: []ecstypes.ContainerDefinition{{
				Name:  awssdk.String("app"),
				Image: awssdk.String("host/catalog:v1"),
			}},
		}},
		registerOut: &ecs.RegisterTaskDefinitionOutput{TaskDefinition: &ecstypes.TaskDefinition{
			TaskDefinitionArn: awssdk.String("arn:td/catalog:6"),
		}},
	}
	d := newDeployer(f)

	err := d.Deploy(context.Background(), "stg-cluster", "catalog", "v2")
	require.NoError(t, err)

	require.NotNil(t, f.registerIn)
	require.Equal(t, "catalog", awssdk.ToString(f.registerIn.Family))
	require.Equal(t, "host/catalog:v2", awssdk.ToString(f.registerIn.ContainerDefinitions[0].Image))

	require.NotNil(t, f.updateIn)
	require.Equal(t, "stg-cluster", awssdk.ToString(f.updateIn.Cluster))
	require.Equal(t, "catalog", awssdk.ToString(f.updateIn.Service))
	require.Equal(t, "arn:td/catalog:6", awssdk.ToString(f.updateIn.TaskDefinition))
}

func TestScaleSetsDesiredCount(t *testing.T) {
	f := &fakeECS{}
	d := newDeployer(f)

	require.NoError(t, d.Scale(context.Background(), "stg-cluster", "catalog", 4))
	require.NotNil(t, f.updateIn)
	require.Equal(t, "catalog", awssdk.ToString(f.updateIn.Service))
	require.Equal(t, int32(4), awssdk.ToInt32(f.updateIn.DesiredCount))
}

func TestRollbackTargetsPreviousRevision(t *testing.T) {
	f := &fakeECS{
		describeOut: &ecs.DescribeServicesOutput{Services: []ecstypes.Service{{
			TaskDefinition: awssdk.String("arn:td/catalog:6"),
		}}},
		taskDefOut: &ecs.DescribeTaskDefinitionOutput{TaskDefinition: &ecstypes.TaskDefinition{
			Family:               awssdk.String("catalog"),
			ContainerDefinitions: []ecstypes.ContainerDefinition{{Image: awssdk.String("host/catalog:v2")}},
		}},
		listTDOut: &ecs.ListTaskDefinitionsOutput{TaskDefinitionArns: []string{
			"arn:td/catalog:6", "arn:td/catalog:5", "arn:td/catalog:4",
		}},
	}
	d := newDeployer(f)

	require.NoError(t, d.Rollback(context.Background(), "stg-cluster", "catalog"))
	require.NotNil(t, f.updateIn)
	require.Equal(t, "arn:td/catalog:5", awssdk.ToString(f.updateIn.TaskDefinition))
}

func TestRollbackNoPreviousRevision(t *testing.T) {
	f := &fakeECS{
		describeOut: &ecs.DescribeServicesOutput{Services: []ecstypes.Service{{
			TaskDefinition: awssdk.String("arn:td/catalog:1"),
		}}},
		taskDefOut: &ecs.DescribeTaskDefinitionOutput{TaskDefinition: &ecstypes.TaskDefinition{
			Family: awssdk.String("catalog"),
		}},
		listTDOut: &ecs.ListTaskDefinitionsOutput{TaskDefinitionArns: []string{"arn:td/catalog:1"}},
	}
	d := newDeployer(f)
	require.Error(t, d.Rollback(context.Background(), "stg-cluster", "catalog"))
}

func TestDeployPreservesRuntimePlatform(t *testing.T) {
	f := &fakeECS{
		describeOut: &ecs.DescribeServicesOutput{Services: []ecstypes.Service{{
			TaskDefinition: awssdk.String("arn:td/catalog:5"),
		}}},
		taskDefOut: &ecs.DescribeTaskDefinitionOutput{TaskDefinition: &ecstypes.TaskDefinition{
			Family:               awssdk.String("catalog"),
			ContainerDefinitions: []ecstypes.ContainerDefinition{{Image: awssdk.String("host/catalog:v1")}},
			RuntimePlatform:      &ecstypes.RuntimePlatform{CpuArchitecture: ecstypes.CPUArchitectureArm64},
		}},
		registerOut: &ecs.RegisterTaskDefinitionOutput{TaskDefinition: &ecstypes.TaskDefinition{
			TaskDefinitionArn: awssdk.String("arn:td/catalog:6"),
		}},
	}
	d := newDeployer(f)
	require.NoError(t, d.Deploy(context.Background(), "stg-cluster", "catalog", "v2"))
	require.NotNil(t, f.registerIn.RuntimePlatform)
	require.Equal(t, ecstypes.CPUArchitectureArm64, f.registerIn.RuntimePlatform.CpuArchitecture)
}

func TestListServicesPaginates(t *testing.T) {
	f := &fakeECS{
		listPages: []*ecs.ListServicesOutput{
			{ServiceArns: []string{"a"}, NextToken: awssdk.String("t1")},
			{ServiceArns: []string{"b"}},
		},
		describeOut: &ecs.DescribeServicesOutput{Services: []ecstypes.Service{
			{ServiceName: awssdk.String("a"), RunningCount: 1, DesiredCount: 1},
		}},
	}
	d := newDeployer(f)
	got, err := d.ListServices(context.Background(), "c")
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(got), 1) // recorrió 2 páginas sin colgarse
	require.Equal(t, 2, f.listIdx)         // consumió ambas páginas
}

func TestDeploymentStatusReadsPrimary(t *testing.T) {
	f := &fakeECS{
		describeOut: &ecs.DescribeServicesOutput{Services: []ecstypes.Service{{
			RunningCount: 1,
			DesiredCount: 1,
			Deployments: []ecstypes.Deployment{
				{Status: awssdk.String("ACTIVE"), RolloutState: ecstypes.DeploymentRolloutStateCompleted, RunningCount: 1},
				{Status: awssdk.String("PRIMARY"), RolloutState: ecstypes.DeploymentRolloutStateInProgress, RunningCount: 0, PendingCount: 1, DesiredCount: 1},
			},
		}}},
	}
	d := newDeployer(f)

	dep, err := d.DeploymentStatus(context.Background(), "stg-cluster", "catalog")
	require.NoError(t, err)
	require.Equal(t, "IN_PROGRESS", dep.Rollout)
	require.Equal(t, 0, dep.Running)
	require.Equal(t, 1, dep.Pending)
	require.Equal(t, 1, dep.Desired)
}

func TestListServicesEnrichesPendingStatusTag(t *testing.T) {
	f := &fakeECS{
		listPages: []*ecs.ListServicesOutput{{ServiceArns: []string{"arn:svc/catalog"}}},
		describeOut: &ecs.DescribeServicesOutput{Services: []ecstypes.Service{{
			ServiceName:    awssdk.String("catalog"),
			RunningCount:   1,
			DesiredCount:   2,
			PendingCount:   1,
			Status:         awssdk.String("ACTIVE"),
			TaskDefinition: awssdk.String("arn:td/catalog:5"),
		}}},
		taskDefOut: &ecs.DescribeTaskDefinitionOutput{TaskDefinition: &ecstypes.TaskDefinition{
			ContainerDefinitions: []ecstypes.ContainerDefinition{{
				Image: awssdk.String("host/catalog:v1.2.3"),
			}},
		}},
	}
	d := newDeployer(f)

	got, err := d.ListServices(context.Background(), "stg-cluster")
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, 1, got[0].Pending)
	require.Equal(t, "ACTIVE", got[0].Status)
	require.Equal(t, "v1.2.3", got[0].Tag)
}
