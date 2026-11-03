package job

import (
	"encoding/json"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func inboundClientEnables(t *testing.T, inboundID int) map[string]bool {
	t.Helper()
	var ib model.Inbound
	if err := database.GetDB().First(&ib, inboundID).Error; err != nil {
		t.Fatalf("reload inbound: %v", err)
	}
	var settings struct {
		Clients []model.Client `json:"clients"`
	}
	if err := json.Unmarshal([]byte(ib.Settings), &settings); err != nil {
		t.Fatalf("parse settings: %v", err)
	}
	out := make(map[string]bool, len(settings.Clients))
	for _, c := range settings.Clients {
		out[c.Email] = c.Enable
	}
	return out
}
