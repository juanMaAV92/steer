package wizard

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProposeName(t *testing.T) {
	require.Equal(t, "nao-v2-dev", ProposeName("nao-v2-dev-cluster"))
	require.Equal(t, "prod", ProposeName("prod"))
	require.Equal(t, "my-cluster-x", ProposeName("my-cluster-x")) // solo sufijo exacto
}

func TestProposeServiceTemplate(t *testing.T) {
	require.Equal(t, "nao-v2-dev-{name}", ProposeServiceTemplate("nao-v2-dev-cluster"))
	require.Equal(t, "prod-{name}", ProposeServiceTemplate("prod"))
}

func TestProposeWritable(t *testing.T) {
	require.False(t, ProposeWritable("nao-production"))
	require.False(t, ProposeWritable("PROD-east"))
	require.False(t, ProposeWritable("client-prd"))
	require.True(t, ProposeWritable("dev"))
	require.True(t, ProposeWritable("staging"))
}

func TestProposeRepoTemplate(t *testing.T) {
	// quita el último segmento-de-ambiente del prefijo del service_template
	require.Equal(t, "nao-v2-{name}", ProposeRepoTemplate("nao-v2-dev-{name}"))
	require.Equal(t, "{name}", ProposeRepoTemplate("prod-{name}"))
	require.Equal(t, "{name}", ProposeRepoTemplate("{name}"))
}
