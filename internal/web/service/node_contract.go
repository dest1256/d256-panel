package service

import (
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// NodeView is the browser/API read contract for nodes.
type NodeView struct {
	Id                 int      `json:"id" example:"1"`
	Name               string   `json:"name" example:"edge-1"`
	Remark             string   `json:"remark" example:"Primary edge"`
	Address            string   `json:"address" example:"node.example.com"`
	Enable             bool     `json:"enable" example:"true"`
	Guid               string   `json:"guid" example:"node-guid"`
	Status             string   `json:"status" example:"online"`
	EnrolledAt         int64    `json:"enrolledAt" example:"1700000000"`
	LastHeartbeat      int64    `json:"lastHeartbeat" example:"1700000000"`
	LatencyMs          int      `json:"latencyMs" example:"42"`
	XrayVersion        string   `json:"xrayVersion" example:"26.3.27"`
	PanelVersion       string   `json:"panelVersion" example:"v0.1.0"`
	ProtoVersion       int      `json:"protoVersion" example:"1"`
	CpuPct             float64  `json:"cpuPct" example:"12.5"`
	MemPct             float64  `json:"memPct" example:"45.2"`
	UptimeSecs         uint64   `json:"uptimeSecs" example:"86400"`
	UnavailableMetrics []string `json:"unavailableMetrics,omitempty"`
	NetUp              uint64   `json:"netUp" example:"2097152"`
	NetDown            uint64   `json:"netDown" example:"1048576"`
	LastError          string   `json:"lastError" example:""`
	XrayState          string   `json:"xrayState" example:"running"`
	XrayError          string   `json:"xrayError" example:""`
	DesiredVersion     uint64   `json:"desiredVersion" example:"7"`
	AppliedVersion     uint64   `json:"appliedVersion" example:"7"`
	ApplyError         string   `json:"applyError" example:""`
	InboundCount       int      `json:"inboundCount" example:"3"`
	ClientCount        int      `json:"clientCount" example:"25"`
	OnlineCount        int      `json:"onlineCount" example:"5"`
	ActiveCount        int      `json:"activeCount" example:"20"`
	DisabledCount      int      `json:"disabledCount" example:"2"`
	DepletedCount      int      `json:"depletedCount" example:"1"`
	CreatedAt          int64    `json:"createdAt" example:"1700000000"`
	UpdatedAt          int64    `json:"updatedAt" example:"1700003600"`
}

func toNodeView(n *model.Node) *NodeView {
	if n == nil {
		return nil
	}
	return &NodeView{
		Id:                 n.Id,
		Name:               n.Name,
		Remark:             n.Remark,
		Address:            n.Address,
		Enable:             n.Enable,
		Guid:               n.Guid,
		Status:             n.Status,
		EnrolledAt:         n.EnrolledAt,
		LastHeartbeat:      n.LastHeartbeat,
		LatencyMs:          n.LatencyMs,
		XrayVersion:        n.XrayVersion,
		PanelVersion:       n.PanelVersion,
		ProtoVersion:       n.ProtoVersion,
		CpuPct:             n.CpuPct,
		MemPct:             n.MemPct,
		UptimeSecs:         n.UptimeSecs,
		UnavailableMetrics: n.UnavailableMetrics,
		NetUp:              n.NetUp,
		NetDown:            n.NetDown,
		LastError:          n.LastError,
		XrayState:          n.XrayState,
		XrayError:          n.XrayError,
		DesiredVersion:     n.DesiredVersion,
		AppliedVersion:     n.AppliedVersion,
		ApplyError:         n.ApplyError,
		InboundCount:       n.InboundCount,
		ClientCount:        n.ClientCount,
		OnlineCount:        n.OnlineCount,
		ActiveCount:        n.ActiveCount,
		DisabledCount:      n.DisabledCount,
		DepletedCount:      n.DepletedCount,
		CreatedAt:          n.CreatedAt,
		UpdatedAt:          n.UpdatedAt,
	}
}

func toNodeViews(nodes []*model.Node) []*NodeView {
	views := make([]*NodeView, 0, len(nodes))
	for _, node := range nodes {
		views = append(views, toNodeView(node))
	}
	return views
}

// GetNodeTreeView lists the nodes with their inbound and client counts.
func (s *NodeService) GetNodeTreeView() ([]*NodeView, error) {
	nodes, err := s.GetAll()
	if err != nil {
		return nil, err
	}
	return toNodeViews(nodes), nil
}
