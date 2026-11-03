package service

import (
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
)

type clientIpPGStats struct {
	Calls             float64
	Rows              float64
	TotalExecTime     float64
	WalRecords        float64
	WalFpi            float64
	WalBytes          float64
	SharedBlksHit     float64
	SharedBlksRead    float64
	SharedBlksDirtied float64
}

func clientIpPGSnapshot(t *testing.T) clientIpPGStats {
	t.Helper()
	var out clientIpPGStats
	db := database.GetDB()
	if db.Name() != "postgres" {
		return out
	}
	err := db.Raw(`SELECT COALESCE(sum(calls),0) AS calls, COALESCE(sum(rows),0) AS rows,
		COALESCE(sum(total_exec_time),0) AS total_exec_time, COALESCE(sum(wal_records),0) AS wal_records,
		COALESCE(sum(wal_fpi),0) AS wal_fpi, COALESCE(sum(wal_bytes),0) AS wal_bytes,
		COALESCE(sum(shared_blks_hit),0) AS shared_blks_hit, COALESCE(sum(shared_blks_read),0) AS shared_blks_read,
		COALESCE(sum(shared_blks_dirtied),0) AS shared_blks_dirtied
		FROM public.pg_stat_statements WHERE dbid = (SELECT oid FROM pg_database WHERE datname = current_database())
		AND (query ILIKE '%inbound_client_ips%' OR query ILIKE '%node_client_ips%')
		AND query NOT ILIKE '%pg_stat_statements%'`).Scan(&out).Error
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func logClientIpPGDelta(t *testing.T, before, after clientIpPGStats) {
	t.Helper()
	if database.GetDB().Name() != "postgres" {
		return
	}
	t.Logf("PG delta calls=%.0f rows=%.0f total_exec_time_ms=%.3f wal_records=%.0f wal_fpi=%.0f wal_bytes=%.0f shared_blks_hit=%.0f read=%.0f dirtied=%.0f",
		after.Calls-before.Calls, after.Rows-before.Rows, after.TotalExecTime-before.TotalExecTime,
		after.WalRecords-before.WalRecords, after.WalFpi-before.WalFpi, after.WalBytes-before.WalBytes,
		after.SharedBlksHit-before.SharedBlksHit, after.SharedBlksRead-before.SharedBlksRead, after.SharedBlksDirtied-before.SharedBlksDirtied)
}

func logClientIpPGContext(t *testing.T) {
	t.Helper()
	db := database.GetDB()
	if db.Name() != "postgres" {
		return
	}
	var info struct {
		StatsReset string
		Dealloc    int64
	}
	if err := db.Raw("SELECT stats_reset::text, dealloc FROM public.pg_stat_statements_info").Scan(&info).Error; err != nil {
		t.Fatal(err)
	}
	var version string
	if err := db.Raw("SELECT version()").Scan(&version).Error; err != nil {
		t.Fatal(err)
	}
	var settings struct {
		FullPageWrites    string
		CheckpointTimeout string
	}
	if err := db.Raw("SELECT current_setting('full_page_writes') AS full_page_writes, current_setting('checkpoint_timeout') AS checkpoint_timeout").Scan(&settings).Error; err != nil {
		t.Fatal(err)
	}
	t.Logf("PG %s stats_reset=%s dealloc=%d full_page_writes=%s checkpoint_timeout=%s", version, info.StatsReset, info.Dealloc, settings.FullPageWrites, settings.CheckpointTimeout)
}
