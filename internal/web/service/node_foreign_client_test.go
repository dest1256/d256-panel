package service

import (
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"

	"gorm.io/gorm"
)

func readClientUUID(t *testing.T, db *gorm.DB, email string) string {
	t.Helper()
	var row model.ClientRecord
	if err := db.Where("email = ?", email).First(&row).Error; err != nil {
		t.Fatalf("read client %q: %v", email, err)
	}
	return row.UUID
}

func nodeInboundID(t *testing.T, db *gorm.DB, tag string) int {
	t.Helper()
	var ib model.Inbound
	if err := db.Where("tag = ?", tag).First(&ib).Error; err != nil {
		t.Fatalf("read inbound %q: %v", tag, err)
	}
	return ib.Id
}
