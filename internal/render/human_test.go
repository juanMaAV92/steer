package render

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAge(t *testing.T) {
	now := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	require.Equal(t, "just now", Age(now.Add(-30*time.Second), now))
	require.Equal(t, "5m ago", Age(now.Add(-5*time.Minute), now))
	require.Equal(t, "2h ago", Age(now.Add(-2*time.Hour), now))
	require.Equal(t, "3d ago", Age(now.Add(-72*time.Hour), now))
	// fronteras exactas de cada rama
	require.Equal(t, "1m ago", Age(now.Add(-time.Minute), now))
	require.Equal(t, "1h ago", Age(now.Add(-time.Hour), now))
	require.Equal(t, "1d ago", Age(now.Add(-24*time.Hour), now))
}

func TestSize(t *testing.T) {
	require.Equal(t, "142 MB", Size(142*1024*1024))
	require.Equal(t, "1.5 GB", Size(1536*1024*1024))
	require.Equal(t, "0 MB", Size(1024))
}

func TestShortDigest(t *testing.T) {
	require.Equal(t, "abcdef123456", ShortDigest("sha256:abcdef123456789..."))
	require.Equal(t, "corto", ShortDigest("corto"))
}
