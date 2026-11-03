package service

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func mtgConfigPath(t *testing.T, inboundId int) string {
	t.Helper()
	return filepath.Join(os.Getenv("XUI_BIN_FOLDER"), "mtproto", fmt.Sprintf("mtg-%d.toml", inboundId))
}

func readMtgConfig(t *testing.T, inboundId int) string {
	t.Helper()
	data, err := os.ReadFile(mtgConfigPath(t, inboundId))
	if err != nil {
		t.Fatalf("read mtg config: %v", err)
	}
	return string(data)
}
