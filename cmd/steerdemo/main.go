// Command steerdemo abre la TUI con datos en memoria (sin AWS) para probar la
// interfaz localmente. Es una utilidad local; no forma parte del binario steer.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/juanMaAV92/steer/internal/config"
	"github.com/juanMaAV92/steer/internal/core"
	"github.com/juanMaAV92/steer/internal/core/coretest"
	"github.com/juanMaAV92/steer/internal/providers"
	"github.com/juanMaAV92/steer/internal/tui"
)

// fakeProvider adapta un core.Deployer en memoria al Provider bundle.
type fakeProvider struct{ dep core.Deployer }

func (p fakeProvider) Deployer() (core.Deployer, error) { return p.dep, nil }

// Registry no está cableado en esta demo: sin bloque [images].
func (p fakeProvider) Registry() (core.Registry, error) { return nil, core.ErrNoImagesConfig }

// Logs no está cableado en esta demo: sin LogSource.
func (p fakeProvider) Logs() (core.LogSource, error) { return nil, core.ErrNoLogSource }

func main() {
	now := time.Now()
	fake := &coretest.FakeDeployer{
		Services: []core.ServiceStatus{
			{Name: "nao-v2-demo-api", Running: 2, Desired: 2, Pending: 0, Status: "ACTIVE", Tag: "v1.4"},
			{Name: "nao-v2-demo-web", Running: 3, Desired: 3, Pending: 0, Status: "ACTIVE", Tag: "v2.0"},
			{Name: "nao-v2-demo-worker", Running: 1, Desired: 2, Pending: 1, Status: "ACTIVE", Tag: "v1.1"},
			{Name: "nao-v2-demo-cron", Running: 0, Desired: 1, Pending: 0, Status: "ACTIVE", Tag: ""},
		},
		DeploymentValue: core.Deployment{Rollout: "COMPLETED", Running: 2, Desired: 2},
		Events: []core.ServiceEvent{
			{ID: "3", At: now, Message: "(service nao-v2-demo-api) has reached a steady state."},
			{ID: "2", At: now.Add(-30 * time.Second), Message: "(service nao-v2-demo-api) registered 2 targets."},
			{ID: "1", At: now.Add(-60 * time.Second), Message: "(service nao-v2-demo-api) has started 2 tasks."},
		},
	}
	cur := config.Context{Name: "demo", Cloud: "aws", Cluster: "demo-cluster", Writable: true}
	factory := providers.ProviderFactory(func(context.Context, config.Context) (providers.Provider, error) {
		return fakeProvider{dep: fake}, nil
	})
	if err := tui.Run(context.Background(), factory, []config.Context{cur}, cur); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
