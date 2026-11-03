package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func TestNodeCredentialsNeverMarshal(t *testing.T) {
	raw, err := json.Marshal(&model.Node{
		Id:       7,
		Name:     "node",
		ApiToken: "plain-secret-token",
	})
	if err != nil {
		t.Fatalf("marshal node: %v", err)
	}
	out := string(raw)
	if strings.Contains(out, "plain-secret-token") || strings.Contains(out, "apiToken") {
		t.Fatalf("model.Node JSON leaked api token field: %s", out)
	}
}
