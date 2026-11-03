package service

import (
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

func useOnlineTestProcess(t *testing.T) *xray.Process {
	t.Helper()
	previousProcess, previousResult := xrayState.snapshot()
	process := xray.NewTestProcess(nil, "")
	xrayState.replace(process)
	t.Cleanup(func() {
		xrayState.mu.Lock()
		xrayState.process = previousProcess
		xrayState.result = previousResult
		xrayState.mu.Unlock()
	})
	return process
}
