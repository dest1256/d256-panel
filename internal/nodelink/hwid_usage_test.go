package nodelink

import (
	"testing"
	"time"

	pb "github.com/dest1256/d256-panel/proto/nodelink/v1"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

const (
	phoneHwid  = "phone0000000000000000000000000000000000000000000000000000000001"
	laptopHwid = "laptop000000000000000000000000000000000000000000000000000000002"
	aliceSub   = "sub-alice@example.com"
)

// hwidFixture creates a node with a client whose phone fetched the subscription from 203.0.113.7 and whose
// laptop fetched it from 198.51.100.9.
func hwidFixture(t *testing.T) (nodeID int, emails, tags map[string]struct{}) {
	t.Helper()
	setup(t)
	n1 := newNode(t, "n1")
	ib := addInbound(t, n1, "in-n1", 20443)
	addClient(t, ib, "alice@example.com")
	db := database.GetDB()
	db.Create(&model.ClientHwid{SubID: aliceSub, HwidHash: phoneHwid, FirstSeen: 1, LastSeen: 2000000, DeviceModel: "Pixel 8", DeviceOS: "Android", OsVersion: "14"})
	db.Create(&model.ClientHwid{SubID: aliceSub, HwidHash: laptopHwid, FirstSeen: 1, LastSeen: 1000000, DeviceOS: "Windows"})
	db.Create(&model.ClientHwidIP{SubID: aliceSub, HwidHash: phoneHwid, IP: "203.0.113.7", LastSeen: 2000000})
	db.Create(&model.ClientHwidIP{SubID: aliceSub, HwidHash: laptopHwid, IP: "198.51.100.9", LastSeen: 1000000})
	return n1, map[string]struct{}{"alice@example.com": {}}, map[string]struct{}{"in-n1": {}}
}

func TestUsageIsAttributedToTheDeviceThatUsedTheSourceAddress(t *testing.T) {
	n1, emails, tags := hwidFixture(t)
	now := time.Date(2026, 6, 15, 12, 30, 0, 0, time.UTC)
	hour := now.Truncate(time.Hour).Unix()
	mk := func(ip string, last int64, up uint64) *pb.ConnEntry {
		return &pb.ConnEntry{
			Email: "alice@example.com", InboundTag: "in-n1", Dest: "example.com", Network: "tcp", HourUnix: hour,
			Up: up, Down: up * 10, Conns: 1, SourceIp: ip, LastUnix: last,
		}
	}
	b := &pb.TrafficBatch{EndUnix: now.Unix(), Conns: []*pb.ConnEntry{
		mk("203.0.113.7", hour+100, 1),
		mk("203.0.113.7", hour+400, 2),
		mk("198.51.100.9", hour+50, 4),
		mk("192.0.2.1", hour+60, 8),       // an address no device fetched the subscription from
		mk("not an ip", hour+70, 16),      // garbage: stored without a device or address
		mk("203.0.113.7", 9999999999, 32), // an absurd timestamp is replaced by the start of the hour
	}}
	if err := storeUsage(database.GetDB(), n1, emails, tags, b, now); err != nil {
		t.Fatal(err)
	}
	var rows []model.ConnStatHourly
	database.GetDB().Find(&rows)
	byHwid := map[string]model.ConnStatHourly{}
	for _, r := range rows {
		byHwid[r.Hwid] = r
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %+v", rows)
	}
	// 1 + 2 + 32 bytes; the real latest use (hour+400) stays.
	if phone := byHwid[phoneHwid]; phone.Up != 35 || phone.Conns != 3 || phone.LastIp != "203.0.113.7" || phone.LastSeen != hour+400 {
		t.Fatalf("phone = %+v", phone)
	}
	if l := byHwid[laptopHwid]; l.Up != 4 || l.LastSeen != hour+50 || l.LastIp != "198.51.100.9" {
		t.Fatalf("laptop = %+v", l)
	}
	if u := byHwid[""]; u.Up != 8+16 || u.Conns != 2 {
		t.Fatalf("unattributed = %+v", u)
	}

	// A later batch adds up and moves the last use forward, never backward.
	later := &pb.TrafficBatch{EndUnix: now.Unix(), Conns: []*pb.ConnEntry{mk("203.0.113.7", hour+900, 100), mk("203.0.113.7", hour+10, 1)}}
	if err := storeUsage(database.GetDB(), n1, emails, tags, later, now); err != nil {
		t.Fatal(err)
	}
	var p model.ConnStatHourly
	database.GetDB().Where("hwid = ?", phoneHwid).First(&p)
	if p.Up != 35+101 || p.LastSeen != hour+900 {
		t.Fatalf("after the second batch: %+v", p)
	}
}

func TestDestinationsShowDevicesLastUseAndFilterByDevice(t *testing.T) {
	n1, _, _ := hwidFixture(t)
	db := database.GetDB()
	base := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC).Unix()
	db.Create(&[]model.ConnStatHourly{
		{NodeId: n1, Email: "alice@example.com", InboundTag: "t", Dest: "a.example", Network: "tcp", Hour: base + 3600, Hwid: phoneHwid, Up: 10, Down: 100, Conns: 2, LastSeen: base + 4000},
		{NodeId: n1, Email: "alice@example.com", InboundTag: "t", Dest: "a.example", Network: "tcp", Hour: base + 3600, Hwid: laptopHwid, Up: 1, Down: 5, Conns: 1, LastSeen: base + 5000},
		{NodeId: n1, Email: "alice@example.com", InboundTag: "t", Dest: "b.example", Network: "tcp", Hour: base + 7200, Hwid: "", Up: 3, Down: 3, Conns: 1, LastSeen: base + 8000},
	})
	db.Create(&model.ConnStatDaily{NodeId: n1, Email: "alice@example.com", InboundTag: "t", Dest: "a.example", Network: "tcp", Day: base, Hwid: phoneHwid, Up: 50, Down: 50, Conns: 5, LastSeen: base + 9000})

	q := DestQuery{Email: "alice@example.com", From: base, To: base + 86400, Desc: true, Sort: "last"}
	rows, err := Destinations(q)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Dest != "a.example" || rows[0].LastSeen != base+9000 {
		t.Fatalf("sorted by last use: %+v", rows)
	}
	a := rows[0]
	if a.Up != 61 || a.Down != 155 || a.Conns != 8 || len(a.Devices) != 2 {
		t.Fatalf("a.example = %+v", a)
	}
	if d := a.Devices[0]; d.Hwid != phoneHwid || d.Label != "Pixel 8 · Android 14" || d.Up != 60 || d.LastSeen != base+9000 || d.Short != "phone000" {
		t.Fatalf("top device = %+v", d)
	}
	if d := a.Devices[1]; d.Hwid != laptopHwid || d.Label != "Windows" {
		t.Fatalf("second device = %+v", d)
	}

	q.Hwid = laptopHwid
	if rows, _ = Destinations(q); len(rows) != 1 || rows[0].Up != 1 || len(rows[0].Devices) != 1 {
		t.Fatalf("only the laptop: %+v", rows)
	}
	q.Hwid = NoDevice
	if rows, _ = Destinations(q); len(rows) != 1 || rows[0].Dest != "b.example" {
		t.Fatalf("only unattributed: %+v", rows)
	}
	q.Hwid = "no-such-device"
	if rows, _ = Destinations(q); len(rows) != 0 {
		t.Fatalf("unknown device: %+v", rows)
	}

	devices, err := ClientDevices("alice@example.com")
	if err != nil || len(devices) != 2 || devices[0].Hwid != phoneHwid || devices[0].LastIP != "203.0.113.7" || devices[0].LastSeen != 2000 {
		t.Fatalf("devices = %+v %v", devices, err)
	}
	if none, _ := ClientDevices("nobody@example.com"); len(none) != 0 {
		t.Fatalf("an unknown client has no devices: %+v", none)
	}
}

func TestRetentionKeepsDevicesAndLastUseAndForgetsOldAddresses(t *testing.T) {
	n1, _, _ := hwidFixture(t)
	db := database.GetDB()
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	day := now.Add(-40 * 24 * time.Hour).Truncate(24 * time.Hour).Unix()
	db.Create(&[]model.ConnStatHourly{
		{NodeId: n1, Email: "alice@example.com", InboundTag: "t", Dest: "d.example", Network: "tcp", Hour: day + 3600, Hwid: phoneHwid, Up: 1, Down: 1, Conns: 1, LastSeen: day + 3700, LastIp: "203.0.113.7"},
		{NodeId: n1, Email: "alice@example.com", InboundTag: "t", Dest: "d.example", Network: "tcp", Hour: day + 7200, Hwid: phoneHwid, Up: 2, Down: 2, Conns: 1, LastSeen: day + 7300},
		{NodeId: n1, Email: "alice@example.com", InboundTag: "t", Dest: "d.example", Network: "tcp", Hour: day + 7200, Hwid: laptopHwid, Up: 4, Down: 4, Conns: 1, LastSeen: day + 7100},
	})
	db.Model(&model.ClientHwidIP{}).Where("ip = ?", "203.0.113.7").Update("last_seen", now.Add(-time.Hour).UnixMilli())
	db.Create(&model.ClientHwidIP{SubID: aliceSub, HwidHash: phoneHwid, IP: "192.0.2.50", LastSeen: now.Add(-90 * 24 * time.Hour).UnixMilli()})

	if err := RunRetention(now); err != nil {
		t.Fatal(err)
	}
	var daily []model.ConnStatDaily
	db.Find(&daily)
	if len(daily) != 2 {
		t.Fatalf("daily = %+v", daily)
	}
	byHwid := map[string]model.ConnStatDaily{}
	for _, d := range daily {
		byHwid[d.Hwid] = d
	}
	if p := byHwid[phoneHwid]; p.Up != 3 || p.Conns != 2 || p.LastSeen != day+7300 {
		t.Fatalf("phone day = %+v", p)
	}
	if l := byHwid[laptopHwid]; l.Up != 4 || l.LastSeen != day+7100 {
		t.Fatalf("laptop day = %+v", l)
	}
	var stale, fresh int64
	db.Model(&model.ClientHwidIP{}).Where("ip = ?", "192.0.2.50").Count(&stale)
	db.Model(&model.ClientHwidIP{}).Where("ip = ?", "203.0.113.7").Count(&fresh)
	if stale != 0 || fresh != 1 {
		t.Fatalf("addresses: stale %d, fresh %d", stale, fresh)
	}
}
