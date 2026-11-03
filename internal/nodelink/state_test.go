package nodelink

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	pb "github.com/dest1256/d256-panel/proto/nodelink/v1"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

func addInbound(t *testing.T, nodeID int, tag string, port int) *model.Inbound {
	t.Helper()
	ib := &model.Inbound{
		Tag: tag, Port: port, Protocol: model.VLESS, Enable: true, NodeID: &nodeID,
		Settings:       `{"clients":[],"decryption":"none"}`,
		StreamSettings: `{"network":"tcp","security":"none"}`,
		Sniffing:       `{"enabled":false}`,
	}
	if err := database.GetDB().Create(ib).Error; err != nil {
		t.Fatal(err)
	}
	return ib
}

func addClient(t *testing.T, ib *model.Inbound, email string) {
	t.Helper()
	db := database.GetDB()
	c := &model.ClientRecord{Email: email, UUID: "6b0b0e3e-41b2-4f1c-9a16-0d5c1c1f3a11", Enable: true, SubID: "sub-" + email}
	if err := db.Create(c).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ClientInbound{ClientId: c.Id, InboundId: ib.Id}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&xray.ClientTraffic{Email: email, InboundId: ib.Id, Enable: true}).Error; err != nil {
		t.Fatal(err)
	}
}

func openStream(t *testing.T, svc *Service, srv *Server, nodeID int, epoch uint64) (pb.NodeLink_ConnectClient, context.CancelFunc) {
	t.Helper()
	cert := enrolled(t, svc, nodeID)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	st, err := dial(t, srv.Addr().String(), svc.CA(), cert).cli.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	h := hello(1)
	h.GetHello().JournalEpoch = epoch
	if err := st.Send(h); err != nil {
		t.Fatal(err)
	}
	if m, err := st.Recv(); err != nil || !m.GetHelloAck().GetAccepted() {
		t.Fatalf("hello ack: %v %v", m, err)
	}
	return st, cancel
}

func TestDesiredStateIsPerNodeAndVersioned(t *testing.T) {
	svc := setup(t)
	n1, n2 := newNode(t, "n1"), newNode(t, "n2")
	ib1 := addInbound(t, n1, "in-n1", 20443)
	addInbound(t, n2, "in-n2", 20444)
	srv := startServer(t, svc)
	st, cancel := openStream(t, svc, srv, n1, 1)
	defer cancel()

	isDesired := func(m *pb.PanelToNode) bool { return m.GetDesiredState() != nil }
	m, err := recvUntil(st, isDesired)
	if err != nil {
		t.Fatal(err)
	}
	ds := m.GetDesiredState()
	if ds.Version != 1 {
		t.Fatalf("first version = %d", ds.Version)
	}
	var cfg map[string]any
	if err := json.Unmarshal(ds.XrayConfigJson, &cfg); err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg["api"]; ok {
		t.Fatal("api section must not reach the node")
	}
	raw := string(ds.XrayConfigJson)
	if !strings.Contains(raw, "in-n1") || strings.Contains(raw, "in-n2") {
		t.Fatalf("config must contain only this node's inbounds: %s", raw)
	}
	if strings.Contains(raw, `"tag":"api"`) {
		t.Fatal("api inbound must be stripped")
	}

	// A new client changes the rendered config: the version grows and the new state is pushed.
	addClient(t, ib1, "alice@example.com")
	m, err = recvUntil(st, isDesired)
	if err != nil {
		t.Fatal(err)
	}
	if m.GetDesiredState().Version != 2 || !strings.Contains(string(m.GetDesiredState().XrayConfigJson), "alice@example.com") {
		t.Fatalf("second state: v%d", m.GetDesiredState().Version)
	}

	_ = st.Send(&pb.NodeToPanel{Msg: &pb.NodeToPanel_StateAck{StateAck: &pb.StateAck{Version: 2, Ok: true}}})
	waitNode(t, n1, func(n model.Node) bool { return n.AppliedVersion == 2 && n.ApplyError == "" })
	_ = st.Send(&pb.NodeToPanel{Msg: &pb.NodeToPanel_StateAck{StateAck: &pb.StateAck{Version: 3, Ok: false, Error: "boom"}}})
	waitNode(t, n1, func(n model.Node) bool { return n.AppliedVersion == 2 && n.ApplyError == "boom" })
}

func waitNode(t *testing.T, id int, cond func(model.Node) bool) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for {
		var n model.Node
		database.GetDB().First(&n, id)
		if cond(n) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("condition not reached: %+v", n)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func batch(from, to uint64, entries ...*pb.TrafficEntry) *pb.NodeToPanel {
	return &pb.NodeToPanel{Msg: &pb.NodeToPanel_Traffic{Traffic: &pb.TrafficBatch{FromSeq: from, ToSeq: to, Entries: entries}}}
}

func userEntry(name string, up, down uint64) *pb.TrafficEntry {
	return &pb.TrafficEntry{Name: name, Up: up, Down: down}
}

func clientBytes(t *testing.T, email string) (up, down int64) {
	t.Helper()
	var ct xray.ClientTraffic
	database.GetDB().Where("email = ?", email).First(&ct)
	return ct.Up, ct.Down
}

func ackOf(t *testing.T, st pb.NodeLink_ConnectClient) uint64 {
	t.Helper()
	m, err := recvUntil(st, func(m *pb.PanelToNode) bool { return m.GetJournalAck() != nil })
	if err != nil {
		t.Fatal(err)
	}
	return m.GetJournalAck().AckedSeq
}

func TestTrafficBatchesAreExactlyOnce(t *testing.T) {
	svc := setup(t)
	n1, n2 := newNode(t, "n1"), newNode(t, "n2")
	ib1 := addInbound(t, n1, "in-n1", 20443)
	ib2 := addInbound(t, n2, "in-n2", 20444)
	addClient(t, ib1, "alice@example.com")
	addClient(t, ib2, "bob@example.com")
	srv := startServer(t, svc)
	st, cancel := openStream(t, svc, srv, n1, 7)
	defer cancel()
	if got := ackOf(t, st); got != 0 {
		t.Fatalf("initial ack = %d", got)
	}

	// Foreign and unknown names are dropped, the batch is still acknowledged.
	_ = st.Send(batch(1, 1, userEntry("alice@example.com", 10, 20), userEntry("bob@example.com", 99, 99), userEntry("ghost", 5, 5)))
	if got := ackOf(t, st); got != 1 {
		t.Fatalf("ack after first batch = %d", got)
	}
	if up, down := clientBytes(t, "alice@example.com"); up != 10 || down != 20 {
		t.Fatalf("alice = %d/%d", up, down)
	}
	if up, down := clientBytes(t, "bob@example.com"); up != 0 || down != 0 {
		t.Fatalf("a node must not be able to bill another node's client: %d/%d", up, down)
	}

	// Resend of the same batch and a gap are not applied; the panel states the position.
	_ = st.Send(batch(1, 1, userEntry("alice@example.com", 10, 20)))
	if got := ackOf(t, st); got != 1 {
		t.Fatalf("ack after duplicate = %d", got)
	}
	_ = st.Send(batch(5, 5, userEntry("alice@example.com", 1000, 1000)))
	if got := ackOf(t, st); got != 1 {
		t.Fatalf("ack after gap = %d", got)
	}
	if up, down := clientBytes(t, "alice@example.com"); up != 10 || down != 20 {
		t.Fatalf("duplicate or gap was counted: %d/%d", up, down)
	}

	// The next contiguous batch (a range, as a compacted journal sends) is applied.
	_ = st.Send(batch(2, 4, userEntry("alice@example.com", 1, 2)))
	if got := ackOf(t, st); got != 4 {
		t.Fatalf("ack after range = %d", got)
	}
	if up, down := clientBytes(t, "alice@example.com"); up != 11 || down != 22 {
		t.Fatalf("alice = %d/%d", up, down)
	}

	// Malformed batches terminate the stream.
	_ = st.Send(batch(0, 0))
	if _, err := recvUntil(st, func(*pb.PanelToNode) bool { return false }); err == nil {
		t.Fatal("malformed batch must end the stream")
	}
}

func TestJournalEpochResetsPosition(t *testing.T) {
	svc := setup(t)
	n1 := newNode(t, "n1")
	ib := addInbound(t, n1, "in-n1", 20443)
	addClient(t, ib, "alice@example.com")
	srv := startServer(t, svc)

	st, cancel := openStream(t, svc, srv, n1, 11)
	ackOf(t, st)
	_ = st.Send(batch(1, 3, userEntry("alice@example.com", 1, 1)))
	if got := ackOf(t, st); got != 3 {
		t.Fatalf("ack = %d", got)
	}
	cancel()
	waitNode(t, n1, func(n model.Node) bool { return n.Status == "offline" })

	// Same epoch (a node with a persisted journal): the position is kept.
	st2, cancel2 := openStream(t, svc, srv, n1, 11)
	if got := ackOf(t, st2); got != 3 {
		t.Fatalf("same epoch ack = %d", got)
	}
	cancel2()
	waitNode(t, n1, func(n model.Node) bool { return n.Status == "offline" })

	// A fresh epoch (state lost): the panel starts the node's sequence space over.
	st3, cancel3 := openStream(t, svc, srv, n1, 12)
	defer cancel3()
	if got := ackOf(t, st3); got != 0 {
		t.Fatalf("new epoch ack = %d", got)
	}
	_ = st3.Send(batch(1, 1, userEntry("alice@example.com", 5, 5)))
	if got := ackOf(t, st3); got != 1 {
		t.Fatalf("ack = %d", got)
	}
}
