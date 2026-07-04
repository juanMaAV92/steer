package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/juanMaAV92/steer/internal/config"
	"github.com/juanMaAV92/steer/internal/core"
	"github.com/juanMaAV92/steer/internal/core/coretest"
	"github.com/juanMaAV92/steer/internal/providers"
	"github.com/stretchr/testify/require"
)

// fakeProvider adapta un core.Deployer fake al Provider bundle.
type fakeProvider struct{ dep core.Deployer }

func (p fakeProvider) Deployer() (core.Deployer, error) { return p.dep, nil }

func withFakeDeployer(t *testing.T, fake core.Deployer) {
	t.Helper()
	prev := newProviderFactoryFn
	newProviderFactoryFn = func() providers.ProviderFactory {
		return func(context.Context, config.Context) (providers.Provider, error) {
			return fakeProvider{dep: fake}, nil
		}
	}
	t.Cleanup(func() { newProviderFactoryFn = prev })
}

// runRootWithFake combina runRoot con un FakeDeployer neutro, para tests que
// no necesitan asertar sobre las llamadas al deployer.
func runRootWithFake(t *testing.T, args ...string) (string, error) {
	t.Helper()
	withFakeDeployer(t, &coretest.FakeDeployer{})
	return runRoot(t, args...)
}

func TestServiceStatusListsServices(t *testing.T) {
	withFakeDeployer(t, &coretest.FakeDeployer{
		Services: []core.ServiceStatus{
			{Name: "catalog", Running: 2, Desired: 2, Status: "ACTIVE", Tag: "v1.0.0"},
			{Name: "billing", Running: 0, Desired: 1, Status: "ACTIVE"},
		},
	})

	out, err := runRoot(t, "service", "status")
	require.NoError(t, err)
	require.Contains(t, out, "SERVICE") // cabecera de la tabla
	require.Contains(t, out, "TAG")
	require.Contains(t, out, "catalog")
	require.Contains(t, out, "billing")
	require.Contains(t, out, "v1.0.0") // columna de tag
}

func TestDeployNonInteractive(t *testing.T) {
	fake := &coretest.FakeDeployer{CurrentTagValue: "v1"}
	withFakeDeployer(t, fake)

	out, err := runRoot(t, "service", "deploy", "-s", "catalog", "-t", "v2", "-y")
	require.NoError(t, err)
	require.Equal(t, []string{"catalog/v2"}, fake.DeployCalls)
	require.Contains(t, out, "v1")       // preview muestra tag actual
	require.Contains(t, out, "v2")       // y el objetivo
	require.Contains(t, out, "rollback") // sugiere rollback
}

func TestDeployRequiresServiceAndTag(t *testing.T) {
	withFakeDeployer(t, &coretest.FakeDeployer{})
	_, err := runRoot(t, "service", "deploy", "-y")
	require.ErrorContains(t, err, "--service")
}

func TestScaleRequiresCount(t *testing.T) {
	// sin --count debe fallar, aunque haya -y
	_, err := runRootWithFake(t, "service", "scale", "-s", "web", "-y")
	require.ErrorContains(t, err, "--count")
}

func TestDeployNonInteractiveRequiresServiceAndTag(t *testing.T) {
	_, err := runRootWithFake(t, "service", "deploy", "-y")
	require.ErrorContains(t, err, "--service")
}

func TestSteerContextEnvVar(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	multi := "[contexts.dev]\ncloud=\"aws\"\nprofile=\"dev\"\ncluster=\"dev-cluster\"\nwritable=true\n" +
		"[contexts.prod]\ncloud=\"aws\"\nprofile=\"prod\"\ncluster=\"prod-cluster\"\nwritable=true\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "steer.toml"), []byte(multi), 0o600))

	t.Setenv("STEER_CONTEXT", "dev")
	// resolver sin --context debe usar STEER_CONTEXT (config de test con contexto "dev")
	out, err := runRootWithFake(t, "service", "status")
	require.NoError(t, err)
	_ = out
}

func TestDeployWatchFollowsRollout(t *testing.T) {
	fake := &coretest.FakeDeployer{
		CurrentTagValue: "v1",
		DeploymentValue: core.Deployment{Rollout: core.RolloutCompleted, Running: 1, Desired: 1},
	}
	withFakeDeployer(t, fake)

	out, err := runRoot(t, "service", "deploy", "-s", "catalog", "-t", "v2", "-y", "-w")
	require.NoError(t, err)
	require.Equal(t, []string{"catalog/v2"}, fake.DeployCalls)
	require.Contains(t, out, "Rollout")
	require.Contains(t, out, "completed")
}

func TestScaleCommand(t *testing.T) {
	fake := &coretest.FakeDeployer{}
	withFakeDeployer(t, fake)
	_, err := runRoot(t, "service", "scale", "-s", "catalog", "-c", "4", "-y")
	require.NoError(t, err)
	require.Equal(t, []string{"catalog/4"}, fake.ScaleCalls)
}

func TestRollbackCommand(t *testing.T) {
	fake := &coretest.FakeDeployer{}
	withFakeDeployer(t, fake)
	_, err := runRoot(t, "service", "rollback", "-s", "catalog", "-y")
	require.NoError(t, err)
	require.Equal(t, []string{"catalog"}, fake.RollbackCalls)
}
