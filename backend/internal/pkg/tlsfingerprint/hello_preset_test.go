package tlsfingerprint

import "testing"

func TestBuiltinProfiles(t *testing.T) {
	chrome := BuiltinChromeAutoProfile()
	if !chrome.UsesChromeAuto() {
		t.Fatal("Chrome Auto profile must use HelloChrome_Auto")
	}
	if chrome.CacheID() != HelloPresetChromeAuto {
		t.Fatalf("CacheID = %q", chrome.CacheID())
	}

	node := BuiltinNodeDefaultProfile()
	if node.UsesChromeAuto() {
		t.Fatal("Node default must not use Chrome Auto")
	}
	if node.CacheID() != "Built-in Default (Node.js 24.x)" {
		t.Fatalf("CacheID = %q", node.CacheID())
	}
}
