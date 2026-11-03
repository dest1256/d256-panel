package service

import (
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func disableInboundRow(t *testing.T, id int) {
	t.Helper()
	if err := database.GetDB().Model(&model.Inbound{}).Where("id = ?", id).Update("enable", false).Error; err != nil {
		t.Fatalf("disable row %d: %v", id, err)
	}
}
