package nodelink

import (
	"strings"
	"time"
	"unicode"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	pb "github.com/dest1256/d256-panel/proto/nodelink/v1"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

const (
	maxConnEntries = 20000
	maxDestLen     = 253
	maxConnCount   = uint32(1) << 30
	bucketSeconds  = 300
	// maxBatchAge bounds how old a batch's time span may be: a node can be offline for a long time, but
	// not forever.
	maxBatchAge = 400 * 24 * time.Hour
)

// sanitizeDest keeps a destination safe to store and show: no control characters, bounded length.
func sanitizeDest(d string) (string, bool) {
	d = strings.TrimSpace(d)
	if d == "" || len(d) > maxDestLen {
		return "", false
	}
	for _, r := range d {
		if unicode.IsControl(r) {
			return "", false
		}
	}
	return strings.ToLower(d), true
}

// spanEnd picks the instant a batch's usage is attributed to, ignoring clocks that are absurd.
func spanEnd(b *pb.TrafficBatch, now time.Time) int64 {
	end := b.EndUnix
	if end <= 0 || end > now.Add(2*time.Hour).Unix() || end < now.Add(-maxBatchAge).Unix() {
		return now.Unix()
	}
	return end
}

func addAssign(table string) clause.Set {
	return clause.Assignments(map[string]any{
		"up":    gorm.Expr(table + ".up + excluded.up"),
		"down":  gorm.Expr(table + ".down + excluded.down"),
		"conns": gorm.Expr(table + ".conns + excluded.conns"),
	})
}

// storeUsage books the per-time-bucket usage (for charts) and the per-destination usage of a batch.
// It runs inside the transaction that also advances the journal position, so a retried batch is
// never booked twice. Entries naming clients or inbounds the node does not host are skipped.
func storeUsage(tx *gorm.DB, nodeID int, emails, tags map[string]struct{}, b *pb.TrafficBatch, now time.Time) error {
	bucket := spanEnd(b, now) / bucketSeconds * bucketSeconds

	var buckets []model.TrafficBucket
	for _, e := range b.Entries {
		if len(e.Name) > maxNameLen || e.Up > maxEntryBytes || e.Down > maxEntryBytes || (e.Up == 0 && e.Down == 0) {
			continue
		}
		kind := model.TrafficBucketUser
		known := false
		if e.Inbound {
			kind = model.TrafficBucketInbound
			_, known = tags[e.Name]
		} else {
			_, known = emails[e.Name]
		}
		if !known {
			continue
		}
		buckets = append(buckets, model.TrafficBucket{
			NodeId: nodeID, Kind: kind, Name: e.Name, Len: bucketSeconds, Start: bucket,
			Up: int64(e.Up), Down: int64(e.Down),
		})
	}
	if len(buckets) > 0 {
		err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "node_id"}, {Name: "kind"}, {Name: "name"}, {Name: "len"}, {Name: "start"}},
			DoUpdates: clause.Assignments(map[string]any{
				"up":   gorm.Expr("traffic_buckets.up + excluded.up"),
				"down": gorm.Expr("traffic_buckets.down + excluded.down"),
			}),
		}).CreateInBatches(buckets, 400).Error
		if err != nil {
			return err
		}
	}

	oldest := now.Add(-maxBatchAge).Unix()
	var hourly []model.ConnStatHourly
	for _, c := range b.Conns {
		dest, ok := sanitizeDest(c.Dest)
		if !ok || c.Up > maxEntryBytes || c.Down > maxEntryBytes || c.Conns > maxConnCount {
			continue
		}
		if c.Network != "tcp" && c.Network != "udp" {
			continue
		}
		if _, ok := emails[c.Email]; !ok {
			continue
		}
		if _, ok := tags[c.InboundTag]; !ok {
			continue
		}
		if c.HourUnix < oldest || c.HourUnix > now.Add(2*time.Hour).Unix() {
			continue
		}
		hourly = append(hourly, model.ConnStatHourly{
			NodeId: nodeID, Email: c.Email, InboundTag: c.InboundTag, Dest: dest, Network: c.Network,
			Hour: c.HourUnix / 3600 * 3600, Up: int64(c.Up), Down: int64(c.Down), Conns: int64(c.Conns),
		})
	}
	if len(hourly) == 0 {
		return nil
	}
	// One statement must not name the same key twice, so fold duplicates first.
	type key struct {
		email, inbound, dest, network string
		hour                          int64
	}
	folded := make(map[key]*model.ConnStatHourly, len(hourly))
	var order []key
	for i := range hourly {
		h := &hourly[i]
		k := key{h.Email, h.InboundTag, h.Dest, h.Network, h.Hour}
		if cur, ok := folded[k]; ok {
			cur.Up += h.Up
			cur.Down += h.Down
			cur.Conns += h.Conns
			continue
		}
		folded[k] = h
		order = append(order, k)
	}
	rows := make([]model.ConnStatHourly, 0, len(order))
	for _, k := range order {
		rows = append(rows, *folded[k])
	}
	return tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "node_id"}, {Name: "email"}, {Name: "inbound_tag"}, {Name: "dest"}, {Name: "network"}, {Name: "hour"},
		},
		DoUpdates: addAssign("conn_stats_hourly"),
	}).CreateInBatches(rows, 200).Error
}
