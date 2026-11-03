package service

import (
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

func TestXrayLifecycleSnapshotDoesNotOverwriteNewerResult(t *testing.T) {
	state := xrayLifecycle{}
	first := xray.NewProcess(&xray.Config{})
	second := xray.NewProcess(&xray.Config{})

	state.replace(first)
	state.storeResult(first, "first result")
	process, result := state.snapshot()
	if process != first || result != "first result" {
		t.Fatalf("snapshot = (%p, %q), want (%p, %q)", process, result, first, "first result")
	}
	state.replace(second)
	state.storeResult(first, "old result")

	process, result = state.snapshot()
	if process != second {
		t.Fatal("snapshot returned the replaced process")
	}
	if result != "" {
		t.Fatalf("snapshot result = %q, want empty", result)
	}
}
