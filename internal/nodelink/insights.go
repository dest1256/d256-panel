package nodelink

import (
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// Limits on what one query may return.
const (
	MaxDestinationRows = 5000
	maxSeriesPoints    = 2000
)

// DestDevice is the part of a destination's usage that was attributed to one device.
type DestDevice struct {
	Hwid     string `json:"hwid"`  // hash of the device id; "" when the device could not be determined
	Short    string `json:"short"` // first characters of the hash, for display
	Label    string `json:"label"` // device model / OS when the device is known
	Up       int64  `json:"up"`
	Down     int64  `json:"down"`
	LastSeen int64  `json:"lastSeen"`
}

// DestRow is the usage of a client towards one destination over a period.
type DestRow struct {
	Dest     string       `json:"dest"`
	Network  string       `json:"network"`
	NodeID   int          `json:"nodeId"`
	Node     string       `json:"node"`
	Up       int64        `json:"up"`
	Down     int64        `json:"down"`
	Conns    int64        `json:"conns"`
	Total    int64        `json:"total"`
	LastSeen int64        `json:"lastSeen"` // unix seconds of the latest use
	Devices  []DestDevice `json:"devices"`
}

// DestQuery selects destination usage. From/To are unix seconds (To exclusive).
type DestQuery struct {
	Email  string
	From   int64
	To     int64
	NodeID int    // 0 = all nodes
	Hwid   string // "" = every device, NoDevice = only usage that could not be attributed to a device
	Search string
	Sort   string // dest | up | down | total | conns | last
	Desc   bool
	Limit  int
}

// NoDevice selects usage that was not attributed to any device.
const NoDevice = "-"

func nodeNames() map[int]string {
	var nodes []model.Node
	_ = database.GetDB().Select("id", "name").Find(&nodes).Error
	m := make(map[int]string, len(nodes))
	for _, n := range nodes {
		m[n.Id] = n.Name
	}
	return m
}

// NodeName returns the display name of a node ("" if unknown).
func NodeName(id int) string { return nodeNames()[id] }

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

// DeviceInfo describes a device of a client's subscription.
type DeviceInfo struct {
	Hwid      string `json:"hwid"`
	Short     string `json:"short"`
	Label     string `json:"label"`
	FirstSeen int64  `json:"firstSeen"` // unix seconds
	LastSeen  int64  `json:"lastSeen"`
	LastIP    string `json:"lastIp"` // the address it last fetched the subscription from
}

func shortHwid(h string) string {
	if len(h) > 8 {
		return h[:8]
	}
	return h
}

func deviceLabel(m model.ClientHwid) string {
	parts := make([]string, 0, 2)
	if m.DeviceModel != "" {
		parts = append(parts, m.DeviceModel)
	}
	if m.DeviceOS != "" {
		os := m.DeviceOS
		if m.OsVersion != "" {
			os += " " + m.OsVersion
		}
		parts = append(parts, os)
	}
	return strings.Join(parts, " · ")
}

// ClientDevices lists the devices that fetched the subscription of a client (by e-mail).
func ClientDevices(email string) ([]DeviceInfo, error) {
	var sub string
	if err := database.GetDB().Model(&model.ClientRecord{}).Where("email = ?", email).Limit(1).Pluck("sub_id", &sub).Error; err != nil {
		return nil, err
	}
	out := []DeviceInfo{}
	if sub == "" {
		return out, nil
	}
	var rows []model.ClientHwid
	if err := database.GetDB().Where("sub_id = ?", sub).Order("last_seen DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	var ips []model.ClientHwidIP
	_ = database.GetDB().Where("sub_id = ?", sub).Order("last_seen DESC").Find(&ips).Error
	lastIP := map[string]string{}
	for _, ip := range ips {
		if _, ok := lastIP[ip.HwidHash]; !ok {
			lastIP[ip.HwidHash] = ip.IP
		}
	}
	for _, r := range rows {
		out = append(out, DeviceInfo{
			Hwid: r.HwidHash, Short: shortHwid(r.HwidHash), Label: deviceLabel(r),
			FirstSeen: r.FirstSeen / 1000, LastSeen: r.LastSeen / 1000, LastIP: lastIP[r.HwidHash],
		})
	}
	return out, nil
}

// Destinations sums hourly and rolled-up daily rows per destination and attributes the usage to devices.
func Destinations(q DestQuery) ([]DestRow, error) {
	if q.Email == "" {
		return nil, errors.New("email is required")
	}
	if q.To <= q.From {
		return nil, errors.New("empty period")
	}
	if q.Limit <= 0 || q.Limit > MaxDestinationRows {
		q.Limit = MaxDestinationRows
	}
	args := []any{q.Email, q.From, q.To}
	filter := ""
	if q.NodeID > 0 {
		filter += " AND node_id = ?"
		args = append(args, q.NodeID)
	}
	switch {
	case q.Hwid == NoDevice:
		filter += " AND hwid = ''"
	case q.Hwid != "":
		filter += " AND hwid = ?"
		args = append(args, q.Hwid)
	}
	if s := strings.ToLower(strings.TrimSpace(q.Search)); s != "" {
		filter += ` AND dest LIKE ? ESCAPE '\'`
		args = append(args, "%"+escapeLike(s)+"%")
	}
	// Daily rows count when their day starts inside the period; hourly rows when their hour does.
	query := `SELECT dest, network, node_id, hwid, SUM(up) AS up, SUM(down) AS down, SUM(conns) AS conns, MAX(last_seen) AS last_seen FROM (
		SELECT dest, network, node_id, hwid, up, down, conns, last_seen FROM conn_stats_hourly WHERE email = ? AND hour >= ? AND hour < ?` + filter + `
		UNION ALL
		SELECT dest, network, node_id, hwid, up, down, conns, last_seen FROM conn_stats_daily WHERE email = ? AND day >= ? AND day < ?` + filter + `
	) GROUP BY dest, network, node_id, hwid`
	all := append(append([]any{}, args...), args...)
	var raw []struct {
		Dest     string
		Network  string
		NodeID   int
		Hwid     string
		Up       int64
		Down     int64
		Conns    int64
		LastSeen int64
	}
	if err := database.GetDB().Raw(query, all...).Scan(&raw).Error; err != nil {
		return nil, err
	}
	names := nodeNames()
	devices, _ := ClientDevices(q.Email)
	labels := make(map[string]string, len(devices))
	for _, d := range devices {
		labels[d.Hwid] = d.Label
	}

	type rowKey struct {
		dest, network string
		node          int
	}
	index := map[rowKey]int{}
	var rows []DestRow
	for _, r := range raw {
		k := rowKey{r.Dest, r.Network, r.NodeID}
		i, ok := index[k]
		if !ok {
			i = len(rows)
			index[k] = i
			rows = append(rows, DestRow{Dest: r.Dest, Network: r.Network, NodeID: r.NodeID, Node: names[r.NodeID], Devices: []DestDevice{}})
		}
		row := &rows[i]
		row.Up += r.Up
		row.Down += r.Down
		row.Conns += r.Conns
		row.LastSeen = max(row.LastSeen, r.LastSeen)
		row.Devices = append(row.Devices, DestDevice{
			Hwid: r.Hwid, Short: shortHwid(r.Hwid), Label: labels[r.Hwid], Up: r.Up, Down: r.Down, LastSeen: r.LastSeen,
		})
	}
	for i := range rows {
		rows[i].Total = rows[i].Up + rows[i].Down
		sort.Slice(rows[i].Devices, func(a, b int) bool {
			return rows[i].Devices[a].Up+rows[i].Devices[a].Down > rows[i].Devices[b].Up+rows[i].Devices[b].Down
		})
	}
	less := func(a, b DestRow) bool { return a.Total < b.Total }
	switch q.Sort {
	case "dest":
		less = func(a, b DestRow) bool { return a.Dest < b.Dest }
	case "up":
		less = func(a, b DestRow) bool { return a.Up < b.Up }
	case "down":
		less = func(a, b DestRow) bool { return a.Down < b.Down }
	case "conns":
		less = func(a, b DestRow) bool { return a.Conns < b.Conns }
	case "last":
		less = func(a, b DestRow) bool { return a.LastSeen < b.LastSeen }
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if q.Desc {
			return less(rows[j], rows[i])
		}
		return less(rows[i], rows[j])
	})
	if len(rows) > q.Limit {
		rows = rows[:q.Limit]
	}
	return rows, nil
}

// Point is one time bucket of a traffic series.
type Point struct {
	T    int64 `json:"t"`
	Up   int64 `json:"up"`
	Down int64 `json:"down"`
}

// Series is the traffic of one node or one inbound over time.
type Series struct {
	Name   string  `json:"name"`
	NodeID int     `json:"nodeId"`
	Points []Point `json:"points"`
}

// TrafficResult is a client's traffic broken down by node and by inbound.
type TrafficResult struct {
	From     int64    `json:"from"`
	To       int64    `json:"to"`
	Step     int64    `json:"step"`
	Total    Point    `json:"total"`
	Nodes    []Series `json:"nodes"`
	Inbounds []Series `json:"inbounds"`
}

// Period resolves a named period ending at now into [from, to) and a chart step.
func Period(name string, from, to int64, now time.Time) (int64, int64, int64, error) {
	var span time.Duration
	switch name {
	case "1h":
		span = time.Hour
	case "24h", "":
		span = 24 * time.Hour
	case "7d":
		span = 7 * 24 * time.Hour
	case "30d":
		span = 30 * 24 * time.Hour
	case "custom":
		if to <= from {
			return 0, 0, 0, errors.New("invalid custom period")
		}
		span = time.Duration(to-from) * time.Second
		if span > 366*24*time.Hour {
			return 0, 0, 0, errors.New("period too long")
		}
		return from, to, stepFor(span), nil
	default:
		return 0, 0, 0, errors.New("unknown period")
	}
	end := now.Unix()
	return end - int64(span/time.Second), end + 1, stepFor(span), nil
}

func stepFor(span time.Duration) int64 {
	switch {
	case span <= 6*time.Hour:
		return 300
	case span <= 3*24*time.Hour:
		return 3600
	default:
		return 3600 * 6
	}
}

func addPoint(m map[int64]*Point, t, up, down int64) {
	p := m[t]
	if p == nil {
		p = &Point{T: t}
		m[t] = p
	}
	p.Up += up
	p.Down += down
}

// Traffic returns upload/download series for a client. The node split comes from the 5-minute chart
// buckets; the inbound split from the client's per-destination hourly rows (older data is hourly).
func Traffic(email string, from, to, step int64) (*TrafficResult, error) {
	if email == "" {
		return nil, errors.New("email is required")
	}
	if to <= from || step <= 0 || (to-from)/step > maxSeriesPoints {
		return nil, errors.New("invalid range")
	}
	names := nodeNames()
	res := &TrafficResult{From: from, To: to, Step: step, Nodes: []Series{}, Inbounds: []Series{}}

	var buckets []model.TrafficBucket
	if err := database.GetDB().Where("kind = ? AND name = ? AND start >= ? AND start < ?",
		model.TrafficBucketUser, email, from/3600*3600, to).Find(&buckets).Error; err != nil {
		return nil, err
	}
	byNode := map[int]map[int64]*Point{}
	for _, b := range buckets {
		m := byNode[b.NodeId]
		if m == nil {
			m = map[int64]*Point{}
			byNode[b.NodeId] = m
		}
		addPoint(m, b.Start/step*step, b.Up, b.Down)
		res.Total.Up += b.Up
		res.Total.Down += b.Down
	}
	for id, m := range byNode {
		res.Nodes = append(res.Nodes, Series{Name: names[id], NodeID: id, Points: sortedPoints(m)})
	}
	sort.Slice(res.Nodes, func(i, j int) bool { return res.Nodes[i].NodeID < res.Nodes[j].NodeID })

	var hourly []model.ConnStatHourly
	if err := database.GetDB().Where("email = ? AND hour >= ? AND hour < ?", email, from/3600*3600, to).Find(&hourly).Error; err != nil {
		return nil, err
	}
	byTag := map[string]map[int64]*Point{}
	for _, h := range hourly {
		m := byTag[h.InboundTag]
		if m == nil {
			m = map[int64]*Point{}
			byTag[h.InboundTag] = m
		}
		addPoint(m, h.Hour/step*step, h.Up, h.Down)
	}
	for tag, m := range byTag {
		res.Inbounds = append(res.Inbounds, Series{Name: tag, Points: sortedPoints(m)})
	}
	sort.Slice(res.Inbounds, func(i, j int) bool { return res.Inbounds[i].Name < res.Inbounds[j].Name })
	return res, nil
}

func sortedPoints(m map[int64]*Point) []Point {
	out := make([]Point, 0, len(m))
	for _, p := range m {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].T < out[j].T })
	return out
}
