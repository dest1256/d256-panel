package nodelink

import (
	"time"

	"gorm.io/gorm"

	"github.com/mhsanaei/3x-ui/v3/internal/config"
	"github.com/mhsanaei/3x-ui/v3/internal/database"
)

// rawBucketKeep is how long 5-minute chart buckets stay before they are folded into hourly ones.
const rawBucketKeep = 48 * time.Hour

// RunRetention rolls hourly per-destination rows older than the retention window into days, drops
// old days, and folds old 5-minute chart buckets into hours. It is idempotent and safe to run often.
func RunRetention(now time.Time) error {
	return database.GetDB().Transaction(func(tx *gorm.DB) error {
		hourlyCut := now.Add(-time.Duration(config.GetConnRetentionDays()) * 24 * time.Hour).Unix()
		if err := tx.Exec(`
			INSERT INTO conn_stats_daily (node_id, email, inbound_tag, dest, network, day, up, down, conns)
			SELECT node_id, email, inbound_tag, dest, network, (hour / 86400) * 86400, SUM(up), SUM(down), SUM(conns)
			FROM conn_stats_hourly WHERE hour < ?
			GROUP BY node_id, email, inbound_tag, dest, network, (hour / 86400) * 86400
			ON CONFLICT (node_id, email, inbound_tag, dest, network, day) DO UPDATE SET
				up = conn_stats_daily.up + excluded.up,
				down = conn_stats_daily.down + excluded.down,
				conns = conn_stats_daily.conns + excluded.conns`, hourlyCut).Error; err != nil {
			return err
		}
		if err := tx.Exec(`DELETE FROM conn_stats_hourly WHERE hour < ?`, hourlyCut).Error; err != nil {
			return err
		}
		dailyCut := now.Add(-time.Duration(config.GetConnDailyRetentionDays()) * 24 * time.Hour).Unix()
		if err := tx.Exec(`DELETE FROM conn_stats_daily WHERE day < ?`, dailyCut).Error; err != nil {
			return err
		}

		bucketCut := now.Add(-rawBucketKeep).Unix()
		if err := tx.Exec(`
			INSERT INTO traffic_buckets (node_id, kind, name, len, start, up, down)
			SELECT node_id, kind, name, 3600, (start / 3600) * 3600, SUM(up), SUM(down)
			FROM traffic_buckets WHERE len = 300 AND start < ?
			GROUP BY node_id, kind, name, (start / 3600) * 3600
			ON CONFLICT (node_id, kind, name, len, start) DO UPDATE SET
				up = traffic_buckets.up + excluded.up,
				down = traffic_buckets.down + excluded.down`, bucketCut).Error; err != nil {
			return err
		}
		if err := tx.Exec(`DELETE FROM traffic_buckets WHERE len = 300 AND start < ?`, bucketCut).Error; err != nil {
			return err
		}
		keep := now.Add(-time.Duration(config.GetTrafficBucketRetentionDays()) * 24 * time.Hour).Unix()
		return tx.Exec(`DELETE FROM traffic_buckets WHERE len = 3600 AND start < ?`, keep).Error
	})
}
