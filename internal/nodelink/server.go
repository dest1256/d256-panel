package nodelink

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	pb "github.com/dest1256/d256-panel/proto/nodelink/v1"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
)

const (
	// MinProtocolVersion is the lowest node protocol the panel accepts.
	MinProtocolVersion = 1
	// maxRecvMsgSize bounds any single node message (journal batches come later and stay below this).
	maxRecvMsgSize = 4 << 20

	helloTimeout = 10 * time.Second
	// idleTimeout: a node that sends nothing (heartbeats are every 5s) for this long is dropped.
	idleTimeout     = 20 * time.Second
	revalidateEvery = 30 * time.Second
	// pushEvery is how often the desired state is recomputed; a changed config is sent at once.
	pushEvery = 2 * time.Second
)

// Snapshot is the latest live state of a connected (or recently seen) node.
type Snapshot struct {
	NodeID    int
	Metrics   *pb.SystemMetrics
	Xray      *pb.XrayStatus
	LatencyMs uint32
	// NodeVersion and XrayVersion come from Hello.
	NodeVersion string
	XrayVersion string
	LastSeen    time.Time
	Online      bool
}

// Hub tracks live node sessions and their latest metrics.
type Hub struct {
	mu       sync.RWMutex
	sessions map[int]*session
	snaps    map[int]*Snapshot
}

type session struct {
	cancel context.CancelFunc
}

func newHub() *Hub { return &Hub{sessions: map[int]*session{}, snaps: map[int]*Snapshot{}} }

// attach registers a session, cancelling any previous one for the same node.
func (h *Hub) attach(id int, s *session) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if old := h.sessions[id]; old != nil {
		old.cancel()
	}
	h.sessions[id] = s
	sn := h.snaps[id]
	if sn == nil {
		sn = &Snapshot{NodeID: id}
		h.snaps[id] = sn
	}
	sn.Online = true
	sn.LastSeen = time.Now()
}

// detach removes the session if it is still the current one and reports whether it was.
func (h *Hub) detach(id int, s *session) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.sessions[id] != s {
		return false
	}
	delete(h.sessions, id)
	if sn := h.snaps[id]; sn != nil {
		sn.Online = false
	}
	return true
}

func (h *Hub) update(id int, fn func(*Snapshot)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if sn := h.snaps[id]; sn != nil {
		fn(sn)
		sn.LastSeen = time.Now()
	}
}

// Snapshots returns copies of all known node snapshots.
func (h *Hub) Snapshots() []Snapshot {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]Snapshot, 0, len(h.snaps))
	for _, sn := range h.snaps {
		out = append(out, *sn)
	}
	return out
}

// OnlineByNode returns, per node id, the users that currently have a connection.
func (h *Hub) OnlineByNode() map[int][]string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make(map[int][]string, len(h.snaps))
	for id, sn := range h.snaps {
		if sn.Online && sn.Xray != nil && len(sn.Xray.OnlineUsers) > 0 {
			out[id] = append([]string(nil), sn.Xray.OnlineUsers...)
		}
	}
	return out
}

// Disconnect drops the live session of a node (used after revoke/delete).
func (h *Hub) Disconnect(id int) {
	h.mu.RLock()
	s := h.sessions[id]
	h.mu.RUnlock()
	if s != nil {
		s.cancel()
	}
}

// Forget drops the cached snapshot of a deleted node.
func (h *Hub) Forget(id int) {
	h.Disconnect(id)
	h.mu.Lock()
	delete(h.snaps, id)
	h.mu.Unlock()
}

// Server is the node gRPC endpoint.
type Server struct {
	pb.UnimplementedNodeLinkServer
	svc     *Service
	hub     *Hub
	node    service.NodeService
	xray    service.XrayService
	inbound service.InboundService
	gs      *grpc.Server
	lis     net.Listener
}

// NewServer prepares the server; call Start to listen.
func NewServer(svc *Service) *Server {
	return &Server{svc: svc, hub: newHub()}
}

// Hub exposes live node state to the web layer.
func (s *Server) Hub() *Hub { return s.hub }

// Start listens on addr with mandatory client certificates issued by the panel CA.
func (s *Server) Start(addr string) error {
	ca := s.svc.CA()
	if ca == nil {
		return errors.New("nodelink: service not initialised")
	}
	tc := &tls.Config{
		MinVersion:     tls.VersionTLS13,
		ClientAuth:     tls.RequireAndVerifyClientCert,
		ClientCAs:      ca.Pool(),
		GetCertificate: s.svc.getServerCert,
		NextProtos:     []string{"h2"},
	}
	lis, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", addr)
	if err != nil {
		return err
	}
	s.lis = lis
	s.gs = grpc.NewServer(
		grpc.Creds(credentials.NewTLS(tc)),
		grpc.MaxRecvMsgSize(maxRecvMsgSize),
		grpc.WaitForHandlers(true),
		grpc.MaxConcurrentStreams(8),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{MinTime: 5 * time.Second, PermitWithoutStream: true}),
		grpc.KeepaliveParams(keepalive.ServerParameters{Time: 30 * time.Second, Timeout: 10 * time.Second}),
	)
	pb.RegisterNodeLinkServer(s.gs, s)
	go func() {
		if err := s.gs.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			logger.Warning("node link server stopped:", err)
		}
	}()
	logger.Info("node link (gRPC mTLS) listening on", lis.Addr())
	return nil
}

// Addr is the bound listener address (useful with port 0 in tests).
func (s *Server) Addr() net.Addr {
	if s.lis == nil {
		return nil
	}
	return s.lis.Addr()
}

// Stop terminates all sessions.
func (s *Server) Stop() {
	if s.gs != nil {
		s.gs.Stop()
	}
}

// authenticate resolves the node from the verified client certificate and checks that it is the
// one certificate currently bound to an enabled node.
func (s *Server) authenticate(ctx context.Context) (*model.Node, error) {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "no peer")
	}
	ti, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok || len(ti.State.PeerCertificates) == 0 {
		return nil, status.Error(codes.Unauthenticated, "client certificate required")
	}
	leaf := ti.State.PeerCertificates[0]
	id, err := NodeIDFromCert(leaf)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "no node identity")
	}
	var n model.Node
	if err := database.GetDB().Where("id = ?", id).Limit(1).Find(&n).Error; err != nil {
		return nil, status.Error(codes.Internal, "lookup failed")
	}
	if n.Id == 0 || !n.Enable || n.CertSerial == "" || n.CertSerial != SerialHex(leaf) {
		return nil, status.Error(codes.PermissionDenied, "certificate not accepted")
	}
	return &n, nil
}

func (s *Server) stillValid(id int, serial string) bool {
	var n model.Node
	if err := database.GetDB().Where("id = ?", id).Limit(1).Find(&n).Error; err != nil {
		return true // transient DB error: keep the session
	}
	return n.Id != 0 && n.Enable && n.CertSerial == serial
}

type recvResult struct {
	msg *pb.NodeToPanel
	err error
}

// Connect is the bidirectional node stream.
func (s *Server) Connect(stream pb.NodeLink_ConnectServer) error {
	node, err := s.authenticate(stream.Context())
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()

	msgs := make(chan recvResult)
	go func() {
		for {
			m, err := stream.Recv()
			select {
			case msgs <- recvResult{m, err}:
			case <-ctx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()

	// First message must be Hello, promptly.
	var first recvResult
	select {
	case first = <-msgs:
	case <-time.After(helloTimeout):
		return status.Error(codes.DeadlineExceeded, "hello timeout")
	case <-ctx.Done():
		return ctx.Err()
	}
	hello := first.msg.GetHello()
	if first.err != nil || hello == nil {
		return status.Error(codes.InvalidArgument, "hello expected")
	}
	if hello.ProtocolVersion < MinProtocolVersion {
		_ = stream.Send(&pb.PanelToNode{Msg: &pb.PanelToNode_HelloAck{HelloAck: &pb.HelloAck{
			Accepted: false, MinProtocolVersion: MinProtocolVersion,
			Reason: "node protocol is too old; upgrade d256-node",
		}}})
		return status.Error(codes.FailedPrecondition, "protocol too old")
	}
	if err := stream.Send(&pb.PanelToNode{Msg: &pb.PanelToNode_HelloAck{HelloAck: &pb.HelloAck{
		Accepted: true, MinProtocolVersion: MinProtocolVersion,
	}}}); err != nil {
		return err
	}
	acked, err := s.syncJournalEpoch(node.Id, hello.JournalEpoch)
	if err != nil {
		return status.Error(codes.Internal, "journal position")
	}
	if err := stream.Send(&pb.PanelToNode{Msg: &pb.PanelToNode_JournalAck{JournalAck: &pb.JournalAck{AckedSeq: acked}}}); err != nil {
		return err
	}
	database.GetDB().Model(&model.Node{}).Where("id = ?", node.Id).Updates(map[string]any{
		"proto_version": int(hello.ProtocolVersion),
		"node_version":  truncate(hello.NodeVersion, 64),
		"xray_version":  truncate(hello.XrayVersion, 64),
	})

	sess := &session{cancel: cancel}
	s.hub.attach(node.Id, sess)
	s.hub.update(node.Id, func(sn *Snapshot) {
		sn.NodeVersion, sn.XrayVersion = truncate(hello.NodeVersion, 64), truncate(hello.XrayVersion, 64)
		sn.Metrics, sn.Xray = nil, nil
	})
	s.markStatus(node.Id, "online", "")
	logger.Infof("node %d (%s) connected", node.Id, node.Name)
	defer func() {
		if s.hub.detach(node.Id, sess) {
			s.markStatus(node.Id, "offline", "")
			logger.Infof("node %d (%s) disconnected", node.Id, node.Name)
		}
	}()

	st := &sessState{}
	if err := s.syncDesired(stream, node.Id, st, true); err != nil {
		return err
	}
	if err := s.syncLimits(stream, node.Id, st, true); err != nil {
		return err
	}
	push := time.NewTicker(pushEvery)
	defer push.Stop()
	idle := time.NewTimer(idleTimeout)
	defer idle.Stop()
	recheck := time.NewTicker(revalidateEvery)
	defer recheck.Stop()
	for {
		select {
		case r := <-msgs:
			if r.err != nil {
				return nil
			}
			if !idle.Stop() {
				select {
				case <-idle.C:
				default:
				}
			}
			idle.Reset(idleTimeout)
			if err := s.handle(stream, node, st, r.msg); err != nil {
				return err
			}
		case <-push.C:
			if err := s.syncDesired(stream, node.Id, st, false); err != nil {
				return err
			}
			if err := s.syncLimits(stream, node.Id, st, false); err != nil {
				return err
			}
		case <-idle.C:
			return status.Error(codes.DeadlineExceeded, "node is silent")
		case <-recheck.C:
			if !s.stillValid(node.Id, node.CertSerial) {
				return status.Error(codes.PermissionDenied, "node removed or revoked")
			}
		case <-ctx.Done():
			return nil
		}
	}
}

func (s *Server) markStatus(id int, st, lastErr string) {
	database.GetDB().Model(&model.Node{}).Where("id = ?", id).Updates(map[string]any{
		"status": st, "last_error": lastErr,
	})
}

func (s *Server) handle(stream pb.NodeLink_ConnectServer, node *model.Node, st *sessState, m *pb.NodeToPanel) error {
	switch x := m.Msg.(type) {
	case *pb.NodeToPanel_Heartbeat:
		s.hub.update(node.Id, func(sn *Snapshot) { sn.LatencyMs = x.Heartbeat.LastRttMs })
		return stream.Send(&pb.PanelToNode{Msg: &pb.PanelToNode_Heartbeat{Heartbeat: x.Heartbeat}})
	case *pb.NodeToPanel_SystemMetrics:
		s.recordMetrics(node.Id, x.SystemMetrics)
	case *pb.NodeToPanel_XrayStatus:
		s.hub.update(node.Id, func(sn *Snapshot) { sn.Xray = x.XrayStatus })
	case *pb.NodeToPanel_StateAck:
		s.recordStateAck(node.Id, x.StateAck)
		st.onAck(x.StateAck.Version, x.StateAck.Ok)
	case *pb.NodeToPanel_Traffic:
		return s.handleTraffic(stream, node.Id, x.Traffic)
	case *pb.NodeToPanel_Hello:
		return status.Error(codes.InvalidArgument, "unexpected hello")
	default:
		// Unknown message from a newer node: ignore (forward compatible).
	}
	return nil
}

func (s *Server) recordMetrics(id int, sm *pb.SystemMetrics) {
	var lat uint32
	var xs *pb.XrayStatus
	var nodeVer, xrayVer string
	s.hub.update(id, func(sn *Snapshot) {
		sn.Metrics = sm
		lat, xs, nodeVer, xrayVer = sn.LatencyMs, sn.Xray, sn.NodeVersion, sn.XrayVersion
	})
	memPct := 0.0
	if sm.MemTotal > 0 {
		memPct = float64(sm.MemUsed) / float64(sm.MemTotal) * 100
	}
	p := service.HeartbeatPatch{
		Status:             "online",
		LastHeartbeat:      time.Now().Unix(),
		LatencyMs:          int(lat),
		CpuPct:             sm.CpuPct,
		MemPct:             memPct,
		UptimeSecs:         sm.UptimeSecs,
		NetUp:              sm.NetUpBps,
		NetDown:            sm.NetDownBps,
		PanelVersion:       nodeVer,
		XrayVersion:        xrayVer,
		UnavailableMetrics: []string{},
	}
	if xs != nil {
		p.XrayState, p.XrayError = xs.State, xs.Error
		if xs.Version != "" {
			p.XrayVersion = xs.Version
		}
	}
	if err := s.node.UpdateHeartbeat(id, p); err != nil {
		logger.Warning("node metrics update failed:", err)
	}
}
