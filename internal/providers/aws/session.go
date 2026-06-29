// Package aws implementa las capacidades de core sobre AWS.
package aws

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/juanMaAV92/steer/internal/config"
)

// LoadConfigForContext crea una aws.Config para un contexto (profile + region).
func LoadConfigForContext(ctx context.Context, c config.Context) (aws.Config, error) {
	var opts []func(*awscfg.LoadOptions) error
	if c.Profile != "" {
		opts = append(opts, awscfg.WithSharedConfigProfile(c.Profile))
	}
	if c.Region != "" {
		opts = append(opts, awscfg.WithRegion(c.Region))
	}
	return awscfg.LoadDefaultConfig(ctx, opts...)
}
