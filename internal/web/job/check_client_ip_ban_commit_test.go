package job

import (
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"

	"gorm.io/gorm"
)

// installClientIpCommitFailure fails the transaction at COMMIT, not at a
// statement, so every write inside it succeeds before the rollback.
func installClientIpCommitFailure(t *testing.T) {
	t.Helper()
	db := database.GetDB()
	switch db.Name() {
	case "sqlite":
		sqlDB, err := db.DB()
		if err != nil {
			t.Fatalf("get sql DB: %v", err)
		}
		// foreign_keys is per connection, so the injection only holds while the
		// pool cannot hand the scan a fresh one with the pragma back at OFF.
		sqlDB.SetMaxOpenConns(1)
		for _, statement := range []string{
			"DROP TRIGGER IF EXISTS commitfail_on_ips",
			"DROP TABLE IF EXISTS commitfail_child",
			"DROP TABLE IF EXISTS commitfail_parent",
			"PRAGMA foreign_keys = ON",
			"CREATE TABLE commitfail_parent (id INTEGER PRIMARY KEY)",
			"CREATE TABLE commitfail_child (parent_id INTEGER, FOREIGN KEY(parent_id) REFERENCES commitfail_parent(id) DEFERRABLE INITIALLY DEFERRED)",
			"CREATE TRIGGER commitfail_on_ips AFTER UPDATE OF ips ON inbound_client_ips BEGIN INSERT INTO commitfail_child(parent_id) VALUES (999); END",
		} {
			if err := db.Exec(statement).Error; err != nil {
				t.Fatalf("install SQLite commit-failure injection %q: %v", statement, err)
			}
		}
		t.Cleanup(func() {
			_ = db.Exec("DROP TRIGGER IF EXISTS commitfail_on_ips").Error
			_ = db.Exec("DROP TABLE IF EXISTS commitfail_child").Error
			_ = db.Exec("DROP TABLE IF EXISTS commitfail_parent").Error
			_ = db.Exec("PRAGMA foreign_keys = OFF").Error
		})
	case "postgres":
		for _, statement := range []string{
			"DROP TABLE IF EXISTS commitfail_child",
			"DROP TABLE IF EXISTS commitfail_parent",
			"CREATE TABLE commitfail_parent (id bigint PRIMARY KEY)",
			"CREATE TABLE commitfail_child (id bigint PRIMARY KEY, parent_id bigint REFERENCES commitfail_parent(id) DEFERRABLE INITIALLY DEFERRED)",
		} {
			if err := db.Exec(statement).Error; err != nil {
				t.Fatalf("install PostgreSQL commit-failure injection %q: %v", statement, err)
			}
		}
		const callbackName = "test:client_ip_commit_failure"
		if err := db.Callback().Update().After("gorm:update").Register(callbackName, func(tx *gorm.DB) {
			stmt := tx.Statement
			if stmt == nil || stmt.Schema == nil || stmt.Schema.Table != "inbound_client_ips" {
				return
			}
			result := tx.Session(&gorm.Session{NewDB: true}).Exec("INSERT INTO commitfail_child (id, parent_id) VALUES (1, 999)")
			if result.Error != nil {
				_ = tx.AddError(result.Error)
			}
		}); err != nil {
			t.Fatalf("register PostgreSQL commit-failure callback: %v", err)
		}
		t.Cleanup(func() {
			_ = db.Callback().Update().Remove(callbackName)
			_ = db.Exec("DROP TABLE IF EXISTS commitfail_child").Error
			_ = db.Exec("DROP TABLE IF EXISTS commitfail_parent").Error
		})
	default:
		t.Fatalf("unsupported test database dialect %q", db.Name())
	}
}

type failingWriter struct{ err error }

func (f failingWriter) Write([]byte) (int, error) { return 0, f.err }
