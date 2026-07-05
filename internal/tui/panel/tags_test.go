package panel

import (
	"strings"
	"testing"
	"time"

	"github.com/juanMaAV92/steer/internal/core"
	"github.com/stretchr/testify/require"
)

func TestTagsViewRendersRowsWithDeployedMarker(t *testing.T) {
	now := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	tags := []core.ImageTag{
		{Tag: "v2", Digest: "sha256:bbbb222222222", SizeBytes: 100 * 1024 * 1024, PushedAt: now.Add(-time.Hour)},
		{Tag: "v1", Digest: "sha256:aaaa111111111", SizeBytes: 90 * 1024 * 1024, PushedAt: now.Add(-72 * time.Hour)},
	}
	out := TagsView("api", tags, "v1", now)
	require.Contains(t, out, "v2")
	require.Contains(t, out, "1h ago")
	require.Contains(t, out, "100 MB")
	require.Contains(t, out, "bbbb22222222")
	// solo la fila desplegada (v1) lleva el marcador, exactamente una vez
	require.Contains(t, out, "● now")
	require.Equal(t, 1, strings.Count(out, "● now"))
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "● now") {
			require.Contains(t, l, "v1")
		}
	}
}

func TestTagsViewEmptyAndStates(t *testing.T) {
	now := time.Now()
	require.Contains(t, TagsView("api", nil, "", now), "no images yet")
}
