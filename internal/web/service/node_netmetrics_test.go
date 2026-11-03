package service

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func TestNodeMetricKeysIncludesNet(t *testing.T) {
	for _, k := range []string{"netUp", "netDown"} {
		if !slices.Contains(NodeMetricKeys, k) {
			t.Fatalf("NodeMetricKeys must include %q so the history endpoint accepts it", k)
		}
	}
}

func TestNodeAvailabilityColumnKeepsLegacyRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	dbtest.InitDB(t, path)
	n := &model.Node{Name: "legacy", CpuPct: 0, UptimeSecs: 0}
	if err := database.GetDB().Create(n).Error; err != nil {
		t.Fatal(err)
	}
	// Recreate the pre-change schema, then let the normal startup migration run.
	if err := database.GetDB().Migrator().DropColumn(&model.Node{}, "UnavailableMetrics"); err != nil {
		t.Fatal(err)
	}
	if err := database.CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := database.InitDB(path); err != nil {
		t.Fatal(err)
	}
	var stored model.Node
	if err := database.GetDB().First(&stored, n.Id).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Name != "legacy" || stored.CpuPct != 0 || stored.UptimeSecs != 0 || len(stored.UnavailableMetrics) != 0 {
		t.Fatalf("legacy row changed: id=%d availability=%v", stored.Id, stored.UnavailableMetrics)
	}
}
