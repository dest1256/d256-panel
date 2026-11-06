package database

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// A panel that already stores per-destination usage (before devices were attributed) must keep its rows,
// gain the new columns, and drop the old unique indexes that would forbid two devices on one destination.
func TestConnStatMigrationKeepsRowsAndAllowsPerDeviceRows(t *testing.T) {
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "x-ui.db")
	legacy, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE conn_stats_hourly (id INTEGER PRIMARY KEY AUTOINCREMENT, node_id INTEGER, email TEXT, inbound_tag TEXT, dest TEXT, network TEXT, hour INTEGER, up INTEGER, down INTEGER, conns INTEGER)`,
		`CREATE UNIQUE INDEX ux_conn_hourly ON conn_stats_hourly (node_id, email, inbound_tag, dest, network, hour)`,
		`INSERT INTO conn_stats_hourly (node_id, email, inbound_tag, dest, network, hour, up, down, conns) VALUES (1, 'a@x', 't', 'old.example', 'tcp', 3600, 5, 6, 2)`,
		`CREATE TABLE conn_stats_daily (id INTEGER PRIMARY KEY AUTOINCREMENT, node_id INTEGER, email TEXT, inbound_tag TEXT, dest TEXT, network TEXT, day INTEGER, up INTEGER, down INTEGER, conns INTEGER)`,
		`CREATE UNIQUE INDEX ux_conn_daily ON conn_stats_daily (node_id, email, inbound_tag, dest, network, day)`,
	} {
		if _, err := legacy.Exec(statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	t.Setenv("XUI_DB_FOLDER", dbDir)
	t.Setenv("XUI_DB_TYPE", "sqlite")
	if err := InitDB(dbPath); err != nil {
		t.Fatalf("InitDB with the old schema: %v", err)
	}
	defer CloseDB()

	var old model.ConnStatHourly
	if err := GetDB().Where("dest = ?", "old.example").First(&old).Error; err != nil {
		t.Fatal(err)
	}
	if old.Up != 5 || old.Down != 6 || old.Hwid != "" || old.LastSeen != 0 {
		t.Fatalf("legacy row = %+v", old)
	}
	for _, index := range []string{"ux_conn_hourly", "ux_conn_daily"} {
		var n int64
		GetDB().Raw("SELECT count(*) FROM sqlite_master WHERE type = 'index' AND name = ?", index).Scan(&n)
		if n != 0 {
			t.Fatalf("the old unique index %s must be dropped", index)
		}
	}
	// Two devices on the same destination in the same hour are two rows now.
	for _, hwid := range []string{"aaa", "bbb"} {
		row := model.ConnStatHourly{NodeId: 1, Email: "a@x", InboundTag: "t", Dest: "old.example", Network: "tcp", Hour: 3600, Hwid: hwid, Up: 1}
		if err := GetDB().Create(&row).Error; err != nil {
			t.Fatalf("device %s: %v", hwid, err)
		}
	}
	// The same device twice is still one row.
	dup := model.ConnStatHourly{NodeId: 1, Email: "a@x", InboundTag: "t", Dest: "old.example", Network: "tcp", Hour: 3600, Hwid: "aaa"}
	if err := GetDB().Create(&dup).Error; err == nil {
		t.Fatal("the unique key must still reject a duplicate device row")
	}

	// Opening it again is a no-op.
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(dbPath); err != nil {
		t.Fatalf("second InitDB: %v", err)
	}
}
