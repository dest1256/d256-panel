package nodelink

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"

	pb "github.com/dest1256/d256-panel/proto/nodelink/v1"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/logger"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

const (
	maxTrafficEntries = 20000
	maxNameLen        = 256
	// maxEntryBytes rejects physically impossible counters (1 PiB in one batch).
	maxEntryBytes = uint64(1) << 50
	// maxEpoch keeps the epoch inside SQLite's signed 64-bit integers.
	maxEpoch = uint64(1) << 62
)

var errStaleJournal = errors.New("traffic batch does not follow the acknowledged position")

type sessState struct {
	lastSentVersion uint64
	// sent keeps the configs not acknowledged yet; acked is what the node is known to run, the base
	// for planning a restart-free update.
	sent         map[uint64][]byte
	ackedVersion uint64
	ackedCfg     []byte
}

func (st *sessState) onAck(version uint64, ok bool) {
	cfg, found := st.sent[version]
	if !ok || !found {
		return
	}
	st.ackedVersion, st.ackedCfg = version, cfg
	for v := range st.sent {
		if v <= version {
			delete(st.sent, v)
		}
	}
}

func toProtoOps(ops []service.NodeHotOp) []*pb.HotOp {
	out := make([]*pb.HotOp, 0, len(ops))
	for _, o := range ops {
		switch o.Kind {
		case "removeUser":
			out = append(out, &pb.HotOp{Op: &pb.HotOp_RemoveUser{RemoveUser: &pb.RemoveUser{InboundTag: o.Tag, Email: o.Email, BestEffort: o.BestEffort}}})
		case "addUser":
			out = append(out, &pb.HotOp{Op: &pb.HotOp_AddUser{AddUser: &pb.AddUser{InboundTag: o.Tag, User: o.UserProto}}})
		case "removeInbound":
			out = append(out, &pb.HotOp{Op: &pb.HotOp_RemoveInbound{RemoveInbound: &pb.RemoveInbound{InboundTag: o.Tag}}})
		case "addInbound":
			out = append(out, &pb.HotOp{Op: &pb.HotOp_AddInbound{AddInbound: &pb.AddInbound{InboundJson: o.InboundJSON}}})
		}
	}
	return out
}

// syncJournalEpoch aligns the stored journal position with the node's sequence space and returns
// the acknowledged sequence the node must continue after.
func (s *Server) syncJournalEpoch(nodeID int, epoch uint64) (uint64, error) {
	var n model.Node
	if err := database.GetDB().Where("id = ?", nodeID).Limit(1).Find(&n).Error; err != nil {
		return 0, err
	}
	if epoch == 0 || epoch >= maxEpoch || epoch != n.JournalEpoch {
		if epoch >= maxEpoch {
			epoch = 0
		}
		err := database.GetDB().Model(&model.Node{}).Where("id = ?", nodeID).
			Updates(map[string]any{"journal_epoch": epoch, "journal_acked_seq": 0}).Error
		return 0, err
	}
	return n.JournalAckedSeq, nil
}

// syncDesired builds the node's config and sends it when it changed (or when force is set, after a
// (re)connect). The version only grows when the rendered config really changes.
func (s *Server) syncDesired(stream pb.NodeLink_ConnectServer, nodeID int, st *sessState, force bool) error {
	cfg, err := s.xray.GetXrayConfigForNode(nodeID)
	if err != nil {
		logger.Warningf("node %d: building config failed, keeping the previous one: %v", nodeID, err)
		return nil
	}
	sum := sha256.Sum256(cfg)
	hash := hex.EncodeToString(sum[:])

	var n model.Node
	if err := database.GetDB().Where("id = ?", nodeID).Limit(1).Find(&n).Error; err != nil || n.Id == 0 {
		return nil
	}
	version := n.DesiredVersion
	if hash != n.DesiredHash {
		version++
		if err := database.GetDB().Model(&model.Node{}).Where("id = ?", nodeID).
			Updates(map[string]any{"desired_version": version, "desired_hash": hash}).Error; err != nil {
			return nil
		}
	}
	if !force && version == st.lastSentVersion {
		return nil
	}
	ds := &pb.DesiredState{Version: version, XrayConfigJson: cfg}
	// A node that is known to run an older state may be able to reach this one without a restart.
	if !force && st.ackedCfg != nil && st.ackedVersion < version {
		if ops, ok := s.xray.PlanNodeHotOps(st.ackedCfg, cfg); ok {
			ds.BaseVersion, ds.HotOpsValid, ds.HotOps = st.ackedVersion, true, toProtoOps(ops)
		}
	}
	if err := stream.Send(&pb.PanelToNode{Msg: &pb.PanelToNode_DesiredState{DesiredState: ds}}); err != nil {
		return err
	}
	if st.sent == nil {
		st.sent = map[uint64][]byte{}
	}
	st.sent[version] = cfg
	st.lastSentVersion = version
	return nil
}

func (s *Server) recordStateAck(nodeID int, a *pb.StateAck) {
	updates := map[string]any{"apply_error": truncate(a.Error, 512)}
	if a.Ok {
		updates["applied_version"] = a.Version
		updates["apply_error"] = ""
	}
	database.GetDB().Model(&model.Node{}).Where("id = ?", nodeID).Updates(updates)
}

// allowedNames returns the user emails and inbound tags a node is entitled to report on.
func allowedNames(nodeID int) (emails, tags map[string]struct{}, err error) {
	db := database.GetDB()
	var tagList []string
	if err = db.Model(&model.Inbound{}).Where("node_id = ?", nodeID).Pluck("tag", &tagList).Error; err != nil {
		return nil, nil, err
	}
	var emailList []string
	err = db.Table("clients").
		Joins("JOIN client_inbounds ci ON ci.client_id = clients.id").
		Joins("JOIN inbounds ib ON ib.id = ci.inbound_id").
		Where("ib.node_id = ?", nodeID).Distinct().Pluck("clients.email", &emailList).Error
	if err != nil {
		return nil, nil, err
	}
	emails = make(map[string]struct{}, len(emailList))
	for _, e := range emailList {
		emails[e] = struct{}{}
	}
	tags = make(map[string]struct{}, len(tagList))
	for _, t := range tagList {
		tags[t] = struct{}{}
	}
	return emails, tags, nil
}

// handleTraffic applies one journal batch exactly once: the batch must continue right after the
// acknowledged sequence, and the new position is committed in the same transaction as the usage.
func (s *Server) handleTraffic(stream pb.NodeLink_ConnectServer, nodeID int, b *pb.TrafficBatch) error {
	if b.FromSeq == 0 || b.ToSeq < b.FromSeq || len(b.Entries) > maxTrafficEntries {
		return status.Error(codes.InvalidArgument, "malformed traffic batch")
	}
	var n model.Node
	if err := database.GetDB().Where("id = ?", nodeID).Limit(1).Find(&n).Error; err != nil || n.Id == 0 {
		return status.Error(codes.Internal, "lookup failed")
	}
	sendAck := func(seq uint64) error {
		return stream.Send(&pb.PanelToNode{Msg: &pb.PanelToNode_JournalAck{JournalAck: &pb.JournalAck{AckedSeq: seq}}})
	}
	if b.FromSeq != n.JournalAckedSeq+1 {
		// Duplicate (already counted) or a gap: tell the node where to continue.
		return sendAck(n.JournalAckedSeq)
	}

	emails, tags, err := allowedNames(nodeID)
	if err != nil {
		return status.Error(codes.Internal, "lookup failed")
	}
	var inbound []*xray.Traffic
	var clients []*xray.ClientTraffic
	for _, e := range b.Entries {
		if len(e.Name) > maxNameLen || e.Up > maxEntryBytes || e.Down > maxEntryBytes || (e.Up == 0 && e.Down == 0) {
			continue
		}
		if e.Inbound {
			if _, ok := tags[e.Name]; ok {
				inbound = append(inbound, &xray.Traffic{IsInbound: true, Tag: e.Name, Up: int64(e.Up), Down: int64(e.Down)})
			}
		} else if _, ok := emails[e.Name]; ok {
			clients = append(clients, &xray.ClientTraffic{Email: e.Name, Up: int64(e.Up), Down: int64(e.Down)})
		}
	}
	_, _, err = s.inbound.AddTrafficTx(inbound, clients, func(tx *gorm.DB) error {
		res := tx.Model(&model.Node{}).Where("id = ? AND journal_acked_seq = ?", nodeID, b.FromSeq-1).
			Update("journal_acked_seq", b.ToSeq)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return errStaleJournal
		}
		return nil
	})
	switch {
	case errors.Is(err, errStaleJournal):
		return sendAck(n.JournalAckedSeq)
	case err != nil:
		logger.Warningf("node %d: applying traffic failed, the node will retry: %v", nodeID, err)
		return nil
	}
	return sendAck(b.ToSeq)
}
