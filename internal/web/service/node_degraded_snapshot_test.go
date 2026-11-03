package service

import (
	"testing"

	"gorm.io/gorm"
)

// linkCount returns how many client_inbounds links a client currently has,
// across every inbound — the value ReapSyncOrphans checks before deleting.
func linkCount(t *testing.T, db *gorm.DB, email string) int64 {
	t.Helper()
	var n int64
	if err := db.Table("client_inbounds").
		Joins("JOIN clients ON clients.id = client_inbounds.client_id").
		Where("clients.email = ?", email).
		Count(&n).Error; err != nil {
		t.Fatalf("count links for %q: %v", email, err)
	}
	return n
}
