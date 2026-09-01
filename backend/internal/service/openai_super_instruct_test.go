package service

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAccountIsSuperInstructEnabled(t *testing.T) {
	t.Parallel()

	require.False(t, (*Account)(nil).IsSuperInstructEnabled())
	require.False(t, (&Account{}).IsSuperInstructEnabled())
	require.False(t, (&Account{Extra: map[string]any{SuperInstructExtraKey: "true"}}).IsSuperInstructEnabled())
	require.False(t, (&Account{Extra: map[string]any{SuperInstructExtraKey: false}}).IsSuperInstructEnabled())
	require.True(t, (&Account{Extra: map[string]any{SuperInstructExtraKey: true}}).IsSuperInstructEnabled())
}

func TestAccountSuperInstructMode(t *testing.T) {
	t.Parallel()

	require.Equal(t, SuperInstructModePrepend, (*Account)(nil).SuperInstructMode())
	require.Equal(t, SuperInstructModePrepend, (&Account{}).SuperInstructMode())
	require.Equal(t, SuperInstructModePrepend, (&Account{Extra: map[string]any{SuperInstructModeExtraKey: "PREPEND"}}).SuperInstructMode())
	require.Equal(t, SuperInstructModeReplace, (&Account{Extra: map[string]any{SuperInstructModeExtraKey: "replace"}}).SuperInstructMode())
	require.Equal(t, SuperInstructModePrepend, (&Account{Extra: map[string]any{SuperInstructModeExtraKey: "weird"}}).SuperInstructMode())
}

func TestApplyAccountSuperInstructBridge_PrependAndReplace(t *testing.T) {
	t.Parallel()

	bridge := "[Super-Instruct // test]\nBRIDGE"
	accOff := &Account{Extra: map[string]any{}}
	accOn := &Account{Extra: map[string]any{SuperInstructExtraKey: true}}
	accReplace := &Account{Extra: map[string]any{
		SuperInstructExtraKey:     true,
		SuperInstructModeExtraKey: SuperInstructModeReplace,
	}}

	body := map[string]any{"instructions": "client-system"}
	require.False(t, applyAccountSuperInstructBridge(body, accOff, bridge))
	require.Equal(t, "client-system", body["instructions"])

	require.True(t, applyAccountSuperInstructBridge(body, accOn, bridge))
	require.Equal(t, bridge+"\n\nclient-system", body["instructions"])

	// second apply is a no-op (marker present)
	require.False(t, applyAccountSuperInstructBridge(body, accOn, bridge))

	body2 := map[string]any{"instructions": "client-system"}
	require.True(t, applyAccountSuperInstructBridge(body2, accReplace, bridge))
	require.Equal(t, bridge, body2["instructions"])

	body3 := map[string]any{}
	require.True(t, applyAccountSuperInstructBridge(body3, accOn, bridge))
	require.Equal(t, bridge, body3["instructions"])

	require.False(t, applyAccountSuperInstructBridge(map[string]any{}, accOn, "   "))
}

func TestLoadSuperInstructBridgeText_HotReload(t *testing.T) {
	resetSuperInstructBridgeCacheForTest()
	t.Cleanup(resetSuperInstructBridgeCacheForTest)

	dir := t.TempDir()
	path := filepath.Join(dir, "bridge.md")
	require.NoError(t, os.WriteFile(path, []byte("v1-bridge"), 0o644))

	require.Equal(t, "v1-bridge", loadSuperInstructBridgeText(path))

	// within min interval: keep cached even if file changes
	require.NoError(t, os.WriteFile(path, []byte("v2-bridge"), 0o644))
	require.Equal(t, "v1-bridge", loadSuperInstructBridgeText(path))

	// after interval + mtime change: reload
	time.Sleep(superInstructBridgeReloadMinInterval + 50*time.Millisecond)
	// ensure mtime advances on coarse FS
	require.NoError(t, os.Chtimes(path, time.Now(), time.Now()))
	require.Equal(t, "v2-bridge", loadSuperInstructBridgeText(path))

	// missing file keeps last good
	time.Sleep(superInstructBridgeReloadMinInterval + 50*time.Millisecond)
	require.Equal(t, "v2-bridge", loadSuperInstructBridgeText(filepath.Join(dir, "missing.md")))
}
