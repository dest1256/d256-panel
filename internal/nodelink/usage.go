package nodelink

import (
	"net"
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

// cleanIP returns the canonical text of a client address, or "" if it is not an IP address.
func cleanIP(s string) string {
	if ip := net.ParseIP(strings.TrimSpace(s)); ip != nil {
		return ip.String()
	}
	return ""
}

// spanEnd picks the instant a batch's usage is attributed to, ignoring clocks that are absurd.
func spanEnd(b *pb.TrafficBatch, now time.Time) int64 {
	end := b.EndUnix
	if end <= 0 || end > now.Add(2*time.Hour).Unix() || end < now.Add(-maxBatchAge).Unix() {
		return now.Unix()
	}
	return end
}

// hourlyAssign adds usage into an existing row and keeps the latest use (and the address of that use).
func hourlyAssign() clause.Set {
	const t = "conn_stats_hourly"
	return clause.Assignments(map[string]any{
		"up":        gorm.Expr(t + ".up + excluded.up"),
		"down":      gorm.Expr(t + ".down + excluded.down"),
		"conns":     gorm.Expr(t + ".conns + excluded.conns"),
		"last_ip":   gorm.Expr("CASE WHEN excluded.last_seen >= " + t + ".last_seen THEN excluded.last_ip ELSE " + t + ".last_ip END"),
		"last_seen": gorm.Expr("CASE WHEN excluded.last_seen > " + t + ".last_seen THEN excluded.last_seen ELSE " + t + ".last_seen END"),
	})
}

// hwidResolver maps (client, source address) to the device that fetched the client's subscription from
// that address most recently.
type hwidResolver struct {
	subByEmail map[string]string
	hwidByIP   map[string]map[string]string // sub id -> ip -> hwid hash
}

func newHwidResolver(tx *gorm.DB, emails map[string]struct{}) *hwidResolver {
	r := &hwidResolver{subByEmail: map[string]string{}, hwidByIP: map[string]map[string]string{}}
	if len(emails) == 0 {
		return r
	}
	list := make([]string, 0, len(emails))
	for e := range emails {
		list = append(list, e)
	}
	var clients []struct {
		Email string
		SubID string `gorm:"column:sub_id"`
	}
	if err := tx.Table("clients").Select("email", "sub_id").Where("email IN ? AND sub_id <> ''", list).Scan(&clients).Error; err != nil {
		return r
	}
	subs := make([]string, 0, len(clients))
	seen := map[string]bool{}
	for _, c := range clients {
		r.subByEmail[c.Email] = c.SubID
		if !seen[c.SubID] {
			seen[c.SubID] = true
			subs = append(subs, c.SubID)
		}
	}
	if len(subs) == 0 {
		return r
	}
	var rows []model.ClientHwidIP
	if err := tx.Where("sub_id IN ?", subs).Order("last_seen DESC").Find(&rows).Error; err != nil {
		return r
	}
	for _, row := range rows {
		m := r.hwidByIP[row.SubID]
		if m == nil {
			m = map[string]string{}
			r.hwidByIP[row.SubID] = m
		}
		if _, ok := m[row.IP]; !ok { // newest first: keep the latest device for an address
			m[row.IP] = row.HwidHash
		}
	}
	return r
}

func (r *hwidResolver) resolve(email, ip string) string {
	if ip == "" {
		return ""
	}
	return r.hwidByIP[r.subByEmail[email]][ip]
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
	newest := now.Add(2 * time.Hour).Unix()
	type valid struct {
		c    *pb.ConnEntry
		dest string
		ip   string
		last int64
	}
	var ok []valid
	used := map[string]struct{}{}
	for _, c := range b.Conns {
		dest, good := sanitizeDest(c.Dest)
		if !good || c.Up > maxEntryBytes || c.Down > maxEntryBytes || c.Conns > maxConnCount {
			continue
		}
		if c.Network != "tcp" && c.Network != "udp" {
			continue
		}
		if _, known := emails[c.Email]; !known {
			continue
		}
		if _, known := tags[c.InboundTag]; !known {
			continue
		}
		if c.HourUnix < oldest || c.HourUnix > newest {
			continue
		}
		last := c.LastUnix
		if last < c.HourUnix || last > newest {
			last = c.HourUnix // a missing or absurd timestamp: the start of the hour is the best known bound
		}
		ok = append(ok, valid{c: c, dest: dest, ip: cleanIP(c.SourceIp), last: last})
		used[c.Email] = struct{}{}
	}
	if len(ok) == 0 {
		return nil
	}
	resolver := newHwidResolver(tx, used)

	// One statement must not name the same key twice, so fold duplicates first.
	type key struct {
		email, inbound, dest, network, hwid string
		hour                                int64
	}
	folded := make(map[key]*model.ConnStatHourly, len(ok))
	var order []key
	for _, v := range ok {
		c := v.c
		hwid := resolver.resolve(c.Email, v.ip)
		k := key{c.Email, c.InboundTag, v.dest, c.Network, hwid, c.HourUnix / 3600 * 3600}
		if cur, seen := folded[k]; seen {
			cur.Up += int64(c.Up)
			cur.Down += int64(c.Down)
			cur.Conns += int64(c.Conns)
			if v.last >= cur.LastSeen {
				cur.LastSeen, cur.LastIp = v.last, v.ip
			}
			continue
		}
		folded[k] = &model.ConnStatHourly{
			NodeId: nodeID, Email: c.Email, InboundTag: c.InboundTag, Dest: v.dest, Network: c.Network,
			Hour: k.hour, Hwid: hwid, Up: int64(c.Up), Down: int64(c.Down), Conns: int64(c.Conns),
			LastSeen: v.last, LastIp: v.ip,
		}
		order = append(order, k)
	}
	rows := make([]model.ConnStatHourly, 0, len(order))
	for _, k := range order {
		rows = append(rows, *folded[k])
	}
	return tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "node_id"}, {Name: "email"}, {Name: "inbound_tag"}, {Name: "dest"}, {Name: "network"}, {Name: "hour"}, {Name: "hwid"},
		},
		DoUpdates: hourlyAssign(),
	}).CreateInBatches(rows, 200).Error
}
