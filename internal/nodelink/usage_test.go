package nodelink

import (
	"strings"
	"testing"
	"time"

	pb "github.com/dest1256/d256-panel/proto/nodelink/v1"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func connBatch(from, to uint64, now time.Time, conns ...*pb.ConnEntry) *pb.NodeToPanel {
	return &pb.NodeToPanel{Msg: &pb.NodeToPanel_Traffic{Traffic: &pb.TrafficBatch{
		FromSeq: from, ToSeq: to, EndUnix: now.Unix(),
		Entries: []*pb.TrafficEntry{userEntry("alice@example.com", 100, 200), {Name: "in-n1", Inbound: true, Up: 100, Down: 200}},
		Conns:   conns,
	}}}
}

func conn(dest string, hour int64, up, down uint64, n uint32) *pb.ConnEntry {
	return &pb.ConnEntry{
		Email: "alice@example.com", InboundTag: "in-n1", Dest: dest, Network: "tcp",
		HourUnix: hour, Up: up, Down: down, Conns: n,
	}
}

func hourlyRows(t *testing.T) []model.ConnStatHourly {
	t.Helper()
	var rows []model.ConnStatHourly
	if err := database.GetDB().Order("dest").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestPerDestinationUsageIsStoredOnceAndValidated(t *testing.T) {
	svc := setup(t)
	n1, n2 := newNode(t, "n1"), newNode(t, "n2")
	ib1 := addInbound(t, n1, "in-n1", 20443)
	ib2 := addInbound(t, n2, "in-n2", 20444)
	addClient(t, ib1, "alice@example.com")
	addClient(t, ib2, "bob@example.com")
	srv := startServer(t, svc)
	st, cancel := openStream(t, svc, srv, n1, 11)
	defer cancel()
	ackOf(t, st)

	now := time.Now()
	hour := now.Truncate(time.Hour).Unix()
	bad := conn("evil\nhost", hour, 1, 1, 1)
	foreign := &pb.ConnEntry{Email: "bob@example.com", InboundTag: "in-n1", Dest: "bob.example", Network: "tcp", HourUnix: hour, Up: 1, Down: 1}
	wrongTag := &pb.ConnEntry{Email: "alice@example.com", InboundTag: "in-n2", Dest: "tag.example", Network: "tcp", HourUnix: hour, Up: 1, Down: 1}
	wrongNet := &pb.ConnEntry{Email: "alice@example.com", InboundTag: "in-n1", Dest: "net.example", Network: "sctp", HourUnix: hour, Up: 1, Down: 1}
	ancient := conn("old.example", now.Add(-800*24*time.Hour).Unix(), 1, 1, 1)
	long := conn(strings.Repeat("a", 300), hour, 1, 1, 1)

	_ = st.Send(connBatch(1, 1, now,
		conn("Example.COM", hour, 1000, 5000, 2),
		conn("example.com", hour, 10, 20, 1), // folds into the same row
		conn("1.1.1.1", hour, 7, 8, 1),
		bad, foreign, wrongTag, wrongNet, ancient, long))
	if got := ackOf(t, st); got != 1 {
		t.Fatalf("ack = %d", got)
	}
	rows := hourlyRows(t)
	if len(rows) != 2 {
		t.Fatalf("rows = %+v", rows)
	}
	byDest := map[string]model.ConnStatHourly{}
	for _, r := range rows {
		byDest[r.Dest] = r
	}
	ex := byDest["example.com"]
	if ex.NodeId != n1 || ex.Up != 1010 || ex.Down != 5020 || ex.Conns != 3 || ex.Hour != hour {
		t.Fatalf("example.com = %+v", ex)
	}
	if byDest["1.1.1.1"].Up != 7 {
		t.Fatalf("ip row = %+v", byDest["1.1.1.1"])
	}

	// A resend of the same batch must not be booked twice.
	_ = st.Send(connBatch(1, 1, now, conn("example.com", hour, 1000, 5000, 2)))
	if got := ackOf(t, st); got != 1 {
		t.Fatalf("ack after duplicate = %d", got)
	}
	if got := hourlyRows(t); len(got) != 2 || got[0].Up+got[1].Up != 1010+7 {
		t.Fatalf("duplicate was counted: %+v", got)
	}

	// The next batch adds to the same hour and bucket.
	_ = st.Send(connBatch(2, 2, now, conn("example.com", hour, 1, 2, 1)))
	if got := ackOf(t, st); got != 2 {
		t.Fatalf("ack = %d", got)
	}
	var ex2 model.ConnStatHourly
	database.GetDB().Where("dest = ?", "example.com").First(&ex2)
	if ex2.Up != 1011 || ex2.Conns != 4 {
		t.Fatalf("after second batch: %+v", ex2)
	}

	var buckets []model.TrafficBucket
	database.GetDB().Where("kind = ?", model.TrafficBucketUser).Find(&buckets)
	if len(buckets) != 1 || buckets[0].NodeId != n1 || buckets[0].Up != 200 || buckets[0].Down != 400 || buckets[0].Len != 300 {
		t.Fatalf("buckets = %+v", buckets)
	}
}

func TestRetentionRollsHourlyIntoDailyAndDropsOld(t *testing.T) {
	setup(t)
	db := database.GetDB()
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	day := func(d int) int64 { return now.Add(-time.Duration(d) * 24 * time.Hour).Truncate(24 * time.Hour).Unix() }
	mk := func(hour int64, up int64) model.ConnStatHourly {
		return model.ConnStatHourly{NodeId: 1, Email: "a@x", InboundTag: "t", Dest: "d.example", Network: "tcp", Hour: hour, Up: up, Down: up, Conns: 1}
	}
	db.Create(&[]model.ConnStatHourly{
		mk(day(40)+3600, 10), mk(day(40)+7200, 20), // old, same day
		mk(day(1)+3600, 5), // recent: stays hourly
	})
	// A rolled-up day already exists for the old day and must be added to, not replaced.
	db.Create(&model.ConnStatDaily{NodeId: 1, Email: "a@x", InboundTag: "t", Dest: "d.example", Network: "tcp", Day: day(40), Up: 100, Down: 100, Conns: 5})
	db.Create(&model.ConnStatDaily{NodeId: 1, Email: "a@x", InboundTag: "t", Dest: "d.example", Network: "tcp", Day: day(500), Up: 1})

	// Chart buckets: raw ones older than 48h fold into hours; very old hours are dropped.
	old := now.Add(-72 * time.Hour).Truncate(time.Hour).Unix()
	db.Create(&[]model.TrafficBucket{
		{NodeId: 1, Kind: 0, Name: "a@x", Len: 300, Start: old, Up: 1, Down: 2},
		{NodeId: 1, Kind: 0, Name: "a@x", Len: 300, Start: old + 300, Up: 10, Down: 20},
		{NodeId: 1, Kind: 0, Name: "a@x", Len: 300, Start: now.Add(-time.Hour).Unix() / 300 * 300, Up: 7, Down: 7},
		{NodeId: 1, Kind: 0, Name: "a@x", Len: 3600, Start: now.Add(-200*24*time.Hour).Unix() / 3600 * 3600, Up: 9, Down: 9},
	})

	for i := 0; i < 2; i++ { // idempotent
		if err := RunRetention(now); err != nil {
			t.Fatal(err)
		}
	}

	var hourly []model.ConnStatHourly
	db.Find(&hourly)
	if len(hourly) != 1 || hourly[0].Up != 5 {
		t.Fatalf("hourly = %+v", hourly)
	}
	var daily []model.ConnStatDaily
	db.Order("day").Find(&daily)
	if len(daily) != 1 || daily[0].Day != day(40) || daily[0].Up != 130 || daily[0].Down != 130 || daily[0].Conns != 7 {
		t.Fatalf("daily = %+v", daily)
	}
	var buckets []model.TrafficBucket
	db.Order("len, start").Find(&buckets)
	if len(buckets) != 2 {
		t.Fatalf("buckets = %+v", buckets)
	}
	if buckets[0].Len != 300 || buckets[0].Up != 7 {
		t.Fatalf("recent raw bucket = %+v", buckets[0])
	}
	if buckets[1].Len != 3600 || buckets[1].Start != old || buckets[1].Up != 11 || buckets[1].Down != 22 {
		t.Fatalf("rolled bucket = %+v", buckets[1])
	}
}

func TestDestinationsQueryUnionsHourlyAndDaily(t *testing.T) {
	setup(t)
	db := database.GetDB()
	db.Create(&model.Node{Id: 1, Name: "n1", Address: "n1.example"})
	db.Create(&model.Node{Id: 2, Name: "n2", Address: "n2.example"})
	base := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC).Unix()
	rows := []model.ConnStatHourly{
		{NodeId: 1, Email: "a@x", InboundTag: "t", Dest: "big.example", Network: "tcp", Hour: base + 3600, Up: 100, Down: 900, Conns: 2},
		{NodeId: 2, Email: "a@x", InboundTag: "t", Dest: "big.example", Network: "tcp", Hour: base + 3600, Up: 1, Down: 1, Conns: 1},
		{NodeId: 1, Email: "a@x", InboundTag: "t", Dest: "small_1.example", Network: "tcp", Hour: base + 7200, Up: 1, Down: 2, Conns: 1},
		{NodeId: 1, Email: "other@x", InboundTag: "t", Dest: "big.example", Network: "tcp", Hour: base + 3600, Up: 999, Down: 999, Conns: 9},
		{NodeId: 1, Email: "a@x", InboundTag: "t", Dest: "outside.example", Network: "tcp", Hour: base - 7200, Up: 5, Down: 5, Conns: 1},
	}
	db.Create(&rows)
	db.Create(&model.ConnStatDaily{NodeId: 1, Email: "a@x", InboundTag: "t", Dest: "big.example", Network: "tcp", Day: base, Up: 50, Down: 50, Conns: 3})

	q := DestQuery{Email: "a@x", From: base, To: base + 86400, Desc: true}
	got, err := Destinations(q)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Dest != "big.example" || got[0].NodeID != 1 || got[0].Up != 150 || got[0].Down != 950 || got[0].Conns != 5 || got[0].Node != "n1" || got[0].Total != 1100 {
		t.Fatalf("got %+v", got)
	}

	q.NodeID = 2
	if got, _ = Destinations(q); len(got) != 1 || got[0].Node != "n2" {
		t.Fatalf("node filter: %+v", got)
	}
	q.NodeID = 0

	// "_" in a search is a literal, not a wildcard.
	q.Search = "small_1"
	if got, _ = Destinations(q); len(got) != 1 || got[0].Dest != "small_1.example" {
		t.Fatalf("search: %+v", got)
	}
	q.Search = "smallx1"
	if got, _ = Destinations(q); len(got) != 0 {
		t.Fatalf("wildcard leaked: %+v", got)
	}
	q.Search = ""

	q.Sort, q.Desc, q.Limit = "dest", false, 1
	if got, _ = Destinations(q); len(got) != 1 || got[0].Dest != "big.example" {
		t.Fatalf("sort/limit: %+v", got)
	}

	if _, err := Destinations(DestQuery{Email: "", From: 0, To: 1}); err == nil {
		t.Fatal("email is required")
	}
	if _, err := Destinations(DestQuery{Email: "a@x", From: 5, To: 5}); err == nil {
		t.Fatal("empty period must fail")
	}
}

func TestTrafficSeriesSplitByNodeAndInbound(t *testing.T) {
	setup(t)
	db := database.GetDB()
	db.Create(&model.Node{Id: 1, Name: "n1", Address: "n1.example"})
	base := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC).Unix()
	db.Create(&[]model.TrafficBucket{
		{NodeId: 1, Kind: 0, Name: "a@x", Len: 300, Start: base, Up: 1, Down: 2},
		{NodeId: 1, Kind: 0, Name: "a@x", Len: 300, Start: base + 300, Up: 3, Down: 4},
		{NodeId: 1, Kind: 0, Name: "other@x", Len: 300, Start: base, Up: 100, Down: 100},
	})
	db.Create(&[]model.ConnStatHourly{
		{NodeId: 1, Email: "a@x", InboundTag: "t1", Dest: "x", Network: "tcp", Hour: base, Up: 4, Down: 6},
		{NodeId: 1, Email: "a@x", InboundTag: "t2", Dest: "y", Network: "tcp", Hour: base, Up: 1, Down: 1},
	})

	res, err := Traffic("a@x", base, base+3600, 3600)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total.Up != 4 || res.Total.Down != 6 || len(res.Nodes) != 1 || res.Nodes[0].Name != "n1" {
		t.Fatalf("totals/nodes: %+v", res)
	}
	if p := res.Nodes[0].Points; len(p) != 1 || p[0].T != base || p[0].Up != 4 || p[0].Down != 6 {
		t.Fatalf("node points: %+v", p)
	}
	if len(res.Inbounds) != 2 || res.Inbounds[0].Name != "t1" || res.Inbounds[0].Points[0].Down != 6 {
		t.Fatalf("inbounds: %+v", res.Inbounds)
	}

	if _, _, _, err := Period("custom", 10, 5, time.Now()); err == nil {
		t.Fatal("inverted custom period must fail")
	}
	if _, _, _, err := Period("forever", 0, 0, time.Now()); err == nil {
		t.Fatal("unknown period must fail")
	}
	if _, err := Traffic("a@x", 0, 365*86400, 300); err == nil {
		t.Fatal("too many points must fail")
	}
}
