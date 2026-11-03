package service

import (
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func rawStoredToken(t *testing.T, id int) string {
	t.Helper()
	var n model.Node
	if err := database.GetDB().Model(model.Node{}).Where("id = ?", id).First(&n).Error; err != nil {
		t.Fatalf("raw load: %v", err)
	}
	return n.ApiToken
}
