package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/juanMaAV92/steer/internal/config"
	"github.com/juanMaAV92/steer/internal/core"
	"github.com/juanMaAV92/steer/internal/core/coretest"
	"github.com/juanMaAV92/steer/internal/providers"
	"github.com/stretchr/testify/require"
)

// fakeProvider adapta fakes de core al Provider bundle.
type fakeProvider struct {
	dep core.Deployer
	reg core.Registry // nil → capacidad deshabilitada
}

func (p fakeProvider) Deployer() (core.Deployer, error) { return p.dep, nil }

func (p fakeProvider) Registry() (core.Registry, error) {
	if p.reg == nil {
		return nil, core.ErrNoImagesConfig
	}
	return p.reg, nil
}

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

// TestExecuteContextCancelsWatchPromptly verifica que la cancelación llega
// hasta el comando vía ExecuteContext (main.go pasa un ctx sensible a señales):
// con un ctx ya cancelado, `deploy -w` debe abortar de inmediato con
// context.Canceled en vez de quedarse en el loop de watch.
func TestExecuteContextCancelsWatchPromptly(t *testing.T) {
	fake := &coretest.FakeDeployer{
		CurrentTagValue: "v1",
		DeploymentValue: core.Deployment{Rollout: core.RolloutInProgress},
	}
	withFakeDeployer(t, fake)

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "steer.toml"), []byte(minimalToml), 0o600))
	t.Chdir(dir)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := runRootCtx(t, ctx, "service", "deploy", "-s", "catalog", "-t", "v2", "-y", "-w")
	require.ErrorIs(t, err, context.Canceled)
}

func TestWatchRolloutStopsOnCancel(t *testing.T) {
	fake := &coretest.FakeDeployer{DeploymentValue: core.Deployment{Rollout: core.RolloutInProgress}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var buf bytes.Buffer
	err := watchRollout(ctx, &buf, fake, "svc", "svc", 1)
	require.ErrorIs(t, err, context.Canceled)
}

// stuckDeployer entrega eventos de fallo de pull a partir de la 2ª consulta
// (la 1ª es el baseline del watch) y un rollout que nunca progresa.
type stuckDeployer struct {
	coretest.FakeDeployer
	calls int
}

func (d *stuckDeployer) ServiceEvents(_ context.Context, _ string) ([]core.ServiceEvent, error) {
	d.calls++
	ev := func(i int) core.ServiceEvent {
		return core.ServiceEvent{ID: "ev-" + strconv.Itoa(i), At: time.Now(),
			Message: "CannotPullContainerError: pull image manifest has been retried", IsError: true}
	}
	switch d.calls {
	case 1:
		return nil, nil // baseline del watch
	case 2:
		return []core.ServiceEvent{ev(1), ev(0)}, nil // 2 fallos: NO corta
	default:
		return []core.ServiceEvent{ev(2), ev(1), ev(0)}, nil // 3º: corta
	}
}

func TestWatchRolloutDetectsStuck(t *testing.T) {
	dep := &stuckDeployer{FakeDeployer: coretest.FakeDeployer{
		DeploymentValue: core.Deployment{Rollout: core.RolloutInProgress, Desired: 1},
	}}
	var buf bytes.Buffer
	err := watchRollout(context.Background(), &buf, dep, "nao-v2-dev-api", "api", 0)
	require.ErrorContains(t, err, "stuck")
	require.Contains(t, buf.String(), "steer service rollback -s api")
	require.GreaterOrEqual(t, dep.calls, 3, "con 2 fallos el watch debe seguir; corta al 3º")
}

func TestDeployBlocksUnknownTag(t *testing.T) {
	fake := &coretest.FakeDeployer{CurrentTagValue: "v1"}
	reg := &coretest.FakeRegistry{Tags: map[string][]core.ImageTag{"catalog": sampleTags()}}
	prev := newProviderFactoryFn
	newProviderFactoryFn = func() providers.ProviderFactory {
		return func(context.Context, config.Context) (providers.Provider, error) {
			return fakeProvider{dep: fake, reg: reg}, nil
		}
	}
	t.Cleanup(func() { newProviderFactoryFn = prev })
	_, err := runRoot(t, "service", "deploy", "-s", "catalog", "-t", "nope", "-y")
	require.ErrorContains(t, err, `tag "nope" not found in repository "catalog"`)
	require.Equal(t, []string{"catalog/nope"}, reg.HasTagCalls)
	require.Empty(t, fake.DeployCalls, "la validación debe bloquear antes de llamar a Deploy")
}

func TestDeployDegradesOnRegistryError(t *testing.T) {
	fake := &coretest.FakeDeployer{CurrentTagValue: "v1"}
	reg := &coretest.FakeRegistry{HasTagErr: errors.New("throttled")}
	prev := newProviderFactoryFn
	newProviderFactoryFn = func() providers.ProviderFactory {
		return func(context.Context, config.Context) (providers.Provider, error) {
			return fakeProvider{dep: fake, reg: reg}, nil
		}
	}
	t.Cleanup(func() { newProviderFactoryFn = prev })
	out, err := runRoot(t, "service", "deploy", "-s", "catalog", "-t", "v2", "-y")
	require.NoError(t, err)
	require.Equal(t, []string{"catalog/v2"}, fake.DeployCalls) // desplegó igual
	_ = out                                                    // el warning va a stderr; el flujo estándar no cambia
}

func TestDeployWithoutImagesConfigStillDeploys(t *testing.T) {
	fake := &coretest.FakeDeployer{CurrentTagValue: "v1"}
	withFakeDeployer(t, fake) // fakeProvider.reg nil → ErrNoImagesConfig
	_, err := runRoot(t, "service", "deploy", "-s", "catalog", "-t", "v2", "-y")
	require.NoError(t, err)
	require.Equal(t, []string{"catalog/v2"}, fake.DeployCalls)
}

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
	require.ErrorContains(t, err, "512 MB") // enseña las válidas del tier
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

func TestDeployBlocksWhenRepoMissingCLI(t *testing.T) {
	fake := &coretest.FakeDeployer{CurrentTagValue: "v1"}
	reg := &coretest.FakeRegistry{HasTagErr: core.ErrRepoNotFound}
	prev := newProviderFactoryFn
	newProviderFactoryFn = func() providers.ProviderFactory {
		return func(context.Context, config.Context) (providers.Provider, error) {
			return fakeProvider{dep: fake, reg: reg}, nil
		}
	}
	t.Cleanup(func() { newProviderFactoryFn = prev })
	_, err := runRoot(t, "service", "deploy", "-s", "catalog", "-t", "v2", "-y")
	require.ErrorContains(t, err, `repository "catalog" not found`)
	require.Empty(t, fake.DeployCalls)
}
