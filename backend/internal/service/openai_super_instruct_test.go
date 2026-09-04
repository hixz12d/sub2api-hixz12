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

func TestAccountSuperInstructProfile(t *testing.T) {
	t.Parallel()

	require.Equal(t, SuperInstructProfileDefault, (*Account)(nil).SuperInstructProfile())
	require.Equal(t, SuperInstructProfileDefault, (&Account{}).SuperInstructProfile())
	require.Equal(t, SuperInstructProfileDefault, (&Account{Extra: map[string]any{SuperInstructProfileExtraKey: "DEFAULT"}}).SuperInstructProfile())
	require.Equal(t, SuperInstructProfileDefault, (&Account{Extra: map[string]any{SuperInstructProfileExtraKey: "super-instruct"}}).SuperInstructProfile())
	require.Equal(t, SuperInstructProfilePortableKit, (&Account{Extra: map[string]any{SuperInstructProfileExtraKey: "portable-kit"}}).SuperInstructProfile())
	require.Equal(t, SuperInstructProfileDefault, (&Account{Extra: map[string]any{SuperInstructProfileExtraKey: "../etc/passwd"}}).SuperInstructProfile())
	require.Equal(t, SuperInstructProfileDefault, (&Account{Extra: map[string]any{SuperInstructProfileExtraKey: "bad name"}}).SuperInstructProfile())
}

func TestResolveAccountSuperInstructBridgePath(t *testing.T) {
	t.Parallel()

	def := `/app/data/super-instruct/bridge.md`
	require.Equal(t, def, resolveAccountSuperInstructBridgePath(nil, def))
	require.Equal(t, def, resolveAccountSuperInstructBridgePath(&Account{Extra: map[string]any{
		SuperInstructProfileExtraKey: "default",
	}}, def))

	acc := &Account{Extra: map[string]any{SuperInstructProfileExtraKey: "portable-kit"}}
	got := resolveAccountSuperInstructBridgePath(acc, def)
	require.Equal(t, filepath.Join(`/app/data/super-instruct`, "profiles", "portable-kit.md"), got)
}

func TestApplyAccountSuperInstructBridge_PrependAndReplace(t *testing.T) {
	t.Parallel()

	bridge := "[Super-Instruct // test]\nBRIDGE"
	kit := "[Portable-Kit // test]\nKIT"
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

	// kit marker also blocks re-inject
	bodyKit := map[string]any{"instructions": kit + "\n\nclient"}
	require.False(t, applyAccountSuperInstructBridge(bodyKit, accOn, bridge))

	body2 := map[string]any{"instructions": "client-system"}
	require.True(t, applyAccountSuperInstructBridge(body2, accReplace, bridge))
	require.Equal(t, bridge, body2["instructions"])

	body3 := map[string]any{}
	require.True(t, applyAccountSuperInstructBridge(body3, accOn, kit))
	require.Equal(t, kit, body3["instructions"])

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

	// missing file keeps the last good content for the same path
	require.NoError(t, os.Remove(path))
	time.Sleep(superInstructBridgeReloadMinInterval + 50*time.Millisecond)
	require.Equal(t, "v2-bridge", loadSuperInstructBridgeText(path))

	// second path is independent
	path2 := filepath.Join(dir, "profiles", "portable-kit.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(path2), 0o755))
	require.NoError(t, os.WriteFile(path2, []byte("kit-v1"), 0o644))
	require.Equal(t, "kit-v1", loadSuperInstructBridgeText(path2))
	require.Equal(t, "v2-bridge", loadSuperInstructBridgeText(path))
}

func TestApplyAccountSuperInstructBridgeFromConfig_Profile(t *testing.T) {
	resetSuperInstructBridgeCacheForTest()
	t.Cleanup(resetSuperInstructBridgeCacheForTest)

	dir := t.TempDir()
	defPath := filepath.Join(dir, "bridge.md")
	kitPath := filepath.Join(dir, "profiles", "portable-kit.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(kitPath), 0o755))
	require.NoError(t, os.WriteFile(defPath, []byte("[Super-Instruct // d]\nSI"), 0o644))
	require.NoError(t, os.WriteFile(kitPath, []byte("[Portable-Kit // k]\nKIT"), 0o644))

	accSI := &Account{Extra: map[string]any{SuperInstructExtraKey: true}}
	body := map[string]any{}
	require.True(t, applyAccountSuperInstructBridgeFromConfig(body, accSI, defPath))
	require.Contains(t, body["instructions"], "SI")

	accKit := &Account{Extra: map[string]any{
		SuperInstructExtraKey:        true,
		SuperInstructProfileExtraKey: "portable-kit",
	}}
	body2 := map[string]any{}
	require.True(t, applyAccountSuperInstructBridgeFromConfig(body2, accKit, defPath))
	require.Contains(t, body2["instructions"], "KIT")
	require.NotContains(t, body2["instructions"], "\nSI")
}
