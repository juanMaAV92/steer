package aws

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/juanMaAV92/steer/internal/cli/wizard"
	"github.com/juanMaAV92/steer/internal/config"
	"github.com/juanMaAV92/steer/internal/core"
)

// Detector descubre perfiles y clusters AWS para el wizard de onboarding.
type Detector struct {
	home string
	// newECS es inyectable en tests: construye el cliente ECS de un perfil/región.
	newECS func(ctx context.Context, profile, region string) (ecsAPI, error)
}

// NewDetector lee el ~/.aws real del usuario.
func NewDetector() *Detector {
	home, _ := os.UserHomeDir()
	return NewDetectorWithHome(home)
}

// NewDetectorWithHome es el constructor inyectable (tests).
func NewDetectorWithHome(home string) *Detector {
	d := &Detector{home: home}
	d.newECS = func(ctx context.Context, profile, region string) (ecsAPI, error) {
		cfg, err := LoadConfigForContext(ctx, config.Context{Profile: profile, Region: region})
		if err != nil {
			return nil, err
		}
		return ecs.NewFromConfig(cfg), nil
	}
	return d
}

// Profiles parsea ~/.aws/config ([profile X] y [default]) y ~/.aws/credentials
// ([X]), deduplicados y en orden alfabético. Sin ~/.aws devuelve lista vacía.
func (d *Detector) Profiles() ([]string, error) {
	set := map[string]bool{}
	parse := func(path, prefix string) error {
		f, err := os.Open(path)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		defer func() { _ = f.Close() }()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if !strings.HasPrefix(line, "[") || !strings.HasSuffix(line, "]") {
				continue
			}
			name := strings.TrimSpace(strings.Trim(line, "[]"))
			name = strings.TrimSpace(strings.TrimPrefix(name, prefix))
			if name != "" {
				set[name] = true
			}
		}
		return sc.Err()
	}
	if err := parse(filepath.Join(d.home, ".aws", "config"), "profile "); err != nil {
		return nil, err
	}
	if err := parse(filepath.Join(d.home, ".aws", "credentials"), ""); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(set))
	for n := range set {
		out = append(out, n)
	}
	sort.Strings(out)
	return out, nil
}

// Clusters lista los clusters ECS visibles con ese perfil/región (nombres, no ARNs).
func (d *Detector) Clusters(ctx context.Context, profile, region string) ([]string, error) {
	api, err := d.newECS(ctx, profile, region)
	if err != nil {
		return nil, err
	}
	var names []string
	var token *string
	for {
		out, err := api.ListClusters(ctx, &ecs.ListClustersInput{NextToken: token})
		if err != nil {
			return nil, err
		}
		for _, arn := range out.ClusterArns {
			if i := strings.LastIndex(arn, "/"); i >= 0 {
				names = append(names, arn[i+1:])
			}
		}
		if awssdk.ToString(out.NextToken) == "" {
			break
		}
		token = out.NextToken
	}
	sort.Strings(names)
	return names, nil
}

// SmokeTest construye el deployer del contexto y cuenta sus servicios.
func (d *Detector) SmokeTest(ctx context.Context, c config.Context) (int, error) {
	p, err := NewProvider(ctx, c)
	if err != nil {
		return 0, err
	}
	var dep core.Deployer
	if dep, err = p.Deployer(); err != nil {
		return 0, err
	}
	svcs, err := dep.ListServices(ctx)
	if err != nil {
		return 0, err
	}
	return len(svcs), nil
}

var _ wizard.Detector = (*Detector)(nil)
