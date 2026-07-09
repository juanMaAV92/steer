package coretest

import (
	"context"
	"testing"
	"time"

	"github.com/juanMaAV92/steer/internal/core"
	"github.com/stretchr/testify/require"
)

func TestFakeLogSourcePagesEnOrden(t *testing.T) {
	t0 := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)
	f := &FakeLogSource{Pages: []core.LogPage{
		{Lines: []core.LogLine{{At: t0, Message: "hello"}}, Cursor: "c1"},
		{Lines: []core.LogLine{{At: t0.Add(time.Second), Message: "world"}}, Cursor: "c2"},
	}}

	page, err := f.TailLogs(context.Background(), "api", 100)
	require.NoError(t, err)
	require.Equal(t, "hello", page.Lines[0].Message)
	require.Equal(t, "c1", page.Cursor)
	require.Equal(t, []string{"api/100"}, f.TailCalls)

	page, err = f.FollowLogs(context.Background(), "api", "c1")
	require.NoError(t, err)
	require.Equal(t, "world", page.Lines[0].Message)
	require.Equal(t, "c2", page.Cursor)
	require.Equal(t, []string{"api/c1"}, f.FollowCalls)

	// agotadas las páginas, follow devuelve vacío conservando el cursor
	page, err = f.FollowLogs(context.Background(), "api", "c2")
	require.NoError(t, err)
	require.Empty(t, page.Lines)
	require.Equal(t, "c2", page.Cursor)
}
