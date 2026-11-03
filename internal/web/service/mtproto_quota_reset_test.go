package service

import (
	"slices"
	"strings"
	"testing"
)

func quotaResets(t *testing.T, logPath string) []string {
	t.Helper()
	var out []string
	for _, line := range fakeMtgLog(t, logPath) {
		if name, ok := strings.CutPrefix(line, "reset:"); ok {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out
}
