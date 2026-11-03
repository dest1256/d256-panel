package service

import (
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// The reaper is scoped to the node sweep. Orphans from any other cause carry no
// mark and keep their existing manual-cleanup semantics.
func TestReapSyncOrphansIgnoresUnmarkedOrphans(t *testing.T) {
	db := initTrafficTestDB(t)
	clientSvc := &ClientService{}

	const email = "manual@x"
	rec := &model.ClientRecord{Email: email, Enable: true, UUID: "44444444-4444-4444-4444-444444444444"}
	if err := db.Create(rec).Error; err != nil {
		t.Fatalf("create client: %v", err)
	}

	reaped, err := clientSvc.ReapSyncOrphans()
	if err != nil {
		t.Fatalf("reap: %v", err)
	}
	if reaped != 0 {
		t.Fatalf("reaped %d unmarked orphan(s), want 0", reaped)
	}
	var surviving int64
	if err := db.Model(&model.ClientRecord{}).Where("email = ?", email).Count(&surviving).Error; err != nil {
		t.Fatalf("count clients: %v", err)
	}
	if surviving != 1 {
		t.Fatalf("unmarked orphan was reaped: %d rows survive, want 1", surviving)
	}
}
