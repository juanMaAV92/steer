// Package aws: bundle de capacidades AWS por contexto, con sesión cacheada.
package aws

import (
	"context"
	"sync"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/juanMaAV92/steer/internal/config"
	"github.com/juanMaAV92/steer/internal/core"
)

// Provider agrupa las capacidades AWS de un contexto. La aws.Config se carga
// una sola vez; las capacidades se memoizan.
type Provider struct {
	cfg    awssdk.Config
	cfgCtx config.Context

	once     sync.Once
	deployer core.Deployer

	regOnce  sync.Once
	registry core.Registry
}

// NewProvider carga la sesión AWS del contexto (cancelable vía ctx).
func NewProvider(ctx context.Context, c config.Context) (*Provider, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cfg, err := LoadConfigForContext(ctx, c)
	if err != nil {
		return nil, err
	}
	return &Provider{cfg: cfg, cfgCtx: c}, nil
}

// Deployer devuelve el Deployer ECS del contexto (memoizado).
func (p *Provider) Deployer() (core.Deployer, error) {
	p.once.Do(func() { p.deployer = NewDeployer(p.cfg, p.cfgCtx.Cluster) })
	return p.deployer, nil
}

// Registry devuelve el Registry ECR del contexto (memoizado).
// Sin bloque [images] en el contexto, la capacidad está deshabilitada.
func (p *Provider) Registry() (core.Registry, error) {
	if p.cfgCtx.Images == nil {
		return nil, core.ErrNoImagesConfig
	}
	p.regOnce.Do(func() { p.registry = NewRegistry(p.cfg, p.cfgCtx.RepoPrefix()) })
	return p.registry, nil
}
