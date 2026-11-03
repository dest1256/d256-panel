package nodelink

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	pb "github.com/dest1256/d256-panel/proto/nodelink/v1"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func newKeyAndCSR(t *testing.T) (*ecdsa.PrivateKey, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{}, key)
	if err != nil {
		t.Fatal(err)
	}
	return key, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der})
}

func setup(t *testing.T) *Service {
	t.Helper()
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "x-ui.db"))
	svc := &Service{}
	if err := svc.Init(); err != nil {
		t.Fatal(err)
	}
	return svc
}

func newNode(t *testing.T, name string) int {
	t.Helper()
	n := &model.Node{Name: name, Address: "localhost", Enable: true, Status: "pending"}
	if err := database.GetDB().Create(n).Error; err != nil {
		t.Fatal(err)
	}
	return n.Id
}

func TestSignNodeCSR(t *testing.T) {
	certPEM, keyPEM, err := GenerateCA()
	if err != nil {
		t.Fatal(err)
	}
	ca, err := LoadCA(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	_, csr := newKeyAndCSR(t)
	out, serial, err := ca.SignNodeCSR(csr, 7, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := pem.Decode(out)
	cert, err := x509.ParseCertificate(b.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cert.Verify(x509.VerifyOptions{Roots: ca.Pool(), KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}}); err != nil {
		t.Fatalf("issued cert does not verify: %v", err)
	}
	if id, err := NodeIDFromCert(cert); err != nil || id != 7 {
		t.Fatalf("node id = %d, %v", id, err)
	}
	if SerialHex(cert) != serial {
		t.Fatal("serial mismatch")
	}

	rk, _ := rsa.GenerateKey(rand.Reader, 2048)
	der, _ := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{}, rk)
	if _, _, err := ca.SignNodeCSR(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: der}), 7, time.Hour); err == nil {
		t.Fatal("RSA CSR must be rejected")
	}
	if _, _, err := ca.SignNodeCSR([]byte("junk"), 7, time.Hour); err == nil {
		t.Fatal("junk CSR must be rejected")
	}
}

func TestEnrollmentTokenLifecycle(t *testing.T) {
	svc := setup(t)
	id, other := newNode(t, "a"), newNode(t, "b")
	_, csr := newKeyAndCSR(t)

	tok, err := svc.CreateEnrollment(id)
	if err != nil {
		t.Fatal(err)
	}
	var rows []model.NodeEnrollment
	database.GetDB().Find(&rows)
	for _, r := range rows {
		if strings.Contains(tok, r.TokenSha256) || r.TokenSha256 == tok {
			t.Fatal("token stored in plaintext")
		}
	}

	// A malformed CSR must not burn the token.
	if _, err := svc.Redeem(tok, []byte("junk"), "panel.example.com:443", "v"); err == nil {
		t.Fatal("expected CSR error")
	}
	// Token for another node id, wrong secret and garbage are all refused.
	for _, bad := range []string{"", "x", "99.abc", strings.Replace(tok, ".", "x.", 1), tok + "A"} {
		if _, err := svc.Redeem(bad, csr, "h", "v"); !errors.Is(err, ErrEnrollment) {
			t.Fatalf("token %q: err = %v, want ErrEnrollment", bad, err)
		}
	}
	res, err := svc.Redeem(tok, csr, "panel.example.com:443", "v1")
	if err != nil {
		t.Fatal(err)
	}
	if res.NodeID != id {
		t.Fatalf("node id %d", res.NodeID)
	}
	var n model.Node
	database.GetDB().First(&n, id)
	if n.CertSerial == "" || n.EnrolledAt == 0 || n.NodeVersion != "v1" {
		t.Fatalf("node not updated: %+v", n)
	}
	if _, err := svc.Redeem(tok, csr, "h", "v"); !errors.Is(err, ErrEnrollment) {
		t.Fatalf("token reuse: %v", err)
	}
	var o model.Node
	database.GetDB().First(&o, other)
	if o.CertSerial != "" {
		t.Fatal("other node must be untouched")
	}

	// Expiry.
	tok2, _ := svc.CreateEnrollment(other)
	database.GetDB().Model(&model.NodeEnrollment{}).Where("node_id = ?", other).Update("expires_at", time.Now().Add(-time.Second).Unix())
	if _, err := svc.Redeem(tok2, csr, "h", "v"); !errors.Is(err, ErrEnrollment) {
		t.Fatalf("expired: %v", err)
	}
	// Re-issuing invalidates the previous unused token.
	t3, _ := svc.CreateEnrollment(other)
	t4, _ := svc.CreateEnrollment(other)
	if _, err := svc.Redeem(t3, csr, "h", "v"); !errors.Is(err, ErrEnrollment) {
		t.Fatalf("superseded token accepted: %v", err)
	}
	if _, err := svc.Redeem(t4, csr, "h", "v"); err != nil {
		t.Fatalf("latest token: %v", err)
	}
}

type testPeer struct {
	conn *grpc.ClientConn
	cli  pb.NodeLinkClient
}

func dial(t *testing.T, addr string, ca *CA, cert *tls.Certificate) *testPeer {
	t.Helper()
	tc := &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: ca.Pool(), ServerName: "localhost"}
	if cert != nil {
		tc.Certificates = []tls.Certificate{*cert}
	}
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(credentials.NewTLS(tc)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return &testPeer{conn: conn, cli: pb.NewNodeLinkClient(conn)}
}

func enrolled(t *testing.T, svc *Service, id int) *tls.Certificate {
	t.Helper()
	key, csr := newKeyAndCSR(t)
	tok, err := svc.CreateEnrollment(id)
	if err != nil {
		t.Fatal(err)
	}
	res, err := svc.Redeem(tok, csr, "localhost:443", "test")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := pem.Decode(res.CertPEM)
	return &tls.Certificate{Certificate: [][]byte{b.Bytes}, PrivateKey: key}
}

func startServer(t *testing.T, svc *Service) *Server {
	t.Helper()
	srv := NewServer(svc)
	if err := srv.Start("127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Stop)
	return srv
}

func hello(version uint32) *pb.NodeToPanel {
	return &pb.NodeToPanel{Msg: &pb.NodeToPanel_Hello{Hello: &pb.Hello{ProtocolVersion: version, NodeVersion: "t1", XrayVersion: "x1"}}}
}

func TestLinkSession(t *testing.T) {
	svc := setup(t)
	id := newNode(t, "n1")
	cert := enrolled(t, svc, id)
	srv := startServer(t, svc)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	st, err := dial(t, srv.Addr().String(), svc.CA(), cert).cli.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Send(hello(1)); err != nil {
		t.Fatal(err)
	}
	ack, err := st.Recv()
	if err != nil || !ack.GetHelloAck().GetAccepted() {
		t.Fatalf("hello ack: %v %v", ack, err)
	}
	now := time.Now().UnixNano()
	_ = st.Send(&pb.NodeToPanel{Msg: &pb.NodeToPanel_Heartbeat{Heartbeat: &pb.Heartbeat{UnixNanos: now, LastRttMs: 12}}})
	echo, err := recvUntil(st, func(m *pb.PanelToNode) bool { return m.GetHeartbeat() != nil })
	if err != nil || echo.GetHeartbeat().GetUnixNanos() != now {
		t.Fatalf("heartbeat echo: %v %v", echo, err)
	}
	_ = st.Send(&pb.NodeToPanel{Msg: &pb.NodeToPanel_SystemMetrics{SystemMetrics: &pb.SystemMetrics{
		CpuPct: 42.5, MemTotal: 1000, MemUsed: 250, UptimeSecs: 99, NetUpBps: 5, NetDownBps: 6, CpuCores: 4,
	}}})
	deadline := time.Now().Add(5 * time.Second)
	for {
		var n model.Node
		database.GetDB().First(&n, id)
		if n.Status == "online" && n.CpuPct == 42.5 && n.MemPct == 25 && n.NodeVersion == "t1" && n.XrayVersion == "x1" && n.LatencyMs == 12 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("node row not updated: %+v", n)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if snaps := srv.Hub().Snapshots(); len(snaps) != 1 || !snaps[0].Online || snaps[0].Metrics.CpuCores != 4 {
		t.Fatalf("snapshots: %+v", snaps)
	}

	_ = st.CloseSend()
	for {
		var n model.Node
		database.GetDB().First(&n, id)
		if n.Status == "offline" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("node did not go offline")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestLinkRejects(t *testing.T) {
	svc := setup(t)
	id := newNode(t, "n1")
	cert := enrolled(t, svc, id)
	srv := startServer(t, svc)
	addr := srv.Addr().String()

	connectFails := func(name string, p *testPeer, send *pb.NodeToPanel) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		st, err := p.cli.Connect(ctx)
		if err == nil {
			if send != nil {
				_ = st.Send(send)
			}
			if m, rerr := st.Recv(); rerr == nil && m.GetHelloAck().GetAccepted() {
				t.Fatalf("%s: connection was accepted", name)
			}
		}
	}

	// No client certificate.
	connectFails("no cert", dial(t, addr, svc.CA(), nil), hello(1))

	// Certificate from a foreign CA claiming a valid node identity.
	fc, fk, _ := GenerateCA()
	foreign, _ := LoadCA(fc, fk)
	key, csr := newKeyAndCSR(t)
	fpem, _, err := foreign.SignNodeCSR(csr, id, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := pem.Decode(fpem)
	connectFails("foreign CA", dial(t, addr, svc.CA(), &tls.Certificate{Certificate: [][]byte{b.Bytes}, PrivateKey: key}), hello(1))

	// Valid cert, but the first message is not Hello.
	connectFails("no hello", dial(t, addr, svc.CA(), cert), &pb.NodeToPanel{Msg: &pb.NodeToPanel_Heartbeat{Heartbeat: &pb.Heartbeat{}}})

	// Protocol below the minimum.
	connectFails("old protocol", dial(t, addr, svc.CA(), cert), hello(0))

	// Disabled node.
	database.GetDB().Model(&model.Node{}).Where("id = ?", id).Update("enable", false)
	connectFails("disabled", dial(t, addr, svc.CA(), cert), hello(1))
	database.GetDB().Model(&model.Node{}).Where("id = ?", id).Update("enable", true)

	// Revoked certificate (serial cleared) and a certificate superseded by re-enrollment.
	if err := svc.Revoke(id); err != nil {
		t.Fatal(err)
	}
	connectFails("revoked", dial(t, addr, svc.CA(), cert), hello(1))
	cert2 := enrolled(t, svc, id)
	connectFails("superseded", dial(t, addr, svc.CA(), cert), hello(1))

	// The fresh certificate works.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	st, err := dial(t, addr, svc.CA(), cert2).cli.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_ = st.Send(hello(1))
	if m, err := st.Recv(); err != nil || !m.GetHelloAck().GetAccepted() {
		t.Fatalf("fresh cert rejected: %v %v", m, err)
	}
}

func TestLinkRejectsOversizedMessage(t *testing.T) {
	svc := setup(t)
	id := newNode(t, "n1")
	cert := enrolled(t, svc, id)
	srv := startServer(t, svc)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	st, err := dial(t, srv.Addr().String(), svc.CA(), cert).cli.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_ = st.Send(hello(1))
	if _, err := st.Recv(); err != nil {
		t.Fatal(err)
	}
	big := &pb.NodeToPanel{Msg: &pb.NodeToPanel_XrayStatus{XrayStatus: &pb.XrayStatus{Error: strings.Repeat("A", maxRecvMsgSize+1024)}}}
	_ = st.Send(big)
	if _, err := recvUntil(st, func(*pb.PanelToNode) bool { return false }); err == nil {
		t.Fatal("oversized message must terminate the stream")
	}
}

// recvUntil reads until a message satisfies pred (the panel also sends journal acks and desired state).
func recvUntil(st pb.NodeLink_ConnectClient, pred func(*pb.PanelToNode) bool) (*pb.PanelToNode, error) {
	for {
		m, err := st.Recv()
		if err != nil || pred(m) {
			return m, err
		}
	}
}
