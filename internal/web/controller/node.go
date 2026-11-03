package controller

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/nodelink"
	"github.com/mhsanaei/3x-ui/v3/internal/web/service"
	"github.com/mhsanaei/3x-ui/v3/internal/web/session"
)

type NodeController struct {
	nodeService service.NodeService
}

func NewNodeController(g *gin.RouterGroup) *NodeController {
	a := &NodeController{}
	a.initRouter(g)
	return a
}

func (a *NodeController) initRouter(g *gin.RouterGroup) {
	g.GET("/list", a.list)
	g.GET("/get/:id", a.get)
	g.GET("/live", a.live)
	g.GET("/history/:id/:metric/:bucket", a.history)

	g.POST("/create", a.create)
	g.POST("/update/:id", a.update)
	g.POST("/enrollToken/:id", a.enrollToken)
	g.POST("/importXui/:id", a.importXui)
	g.POST("/del/:id", a.del)
	g.POST("/setEnable/:id", a.setEnable)
}

func nodeID(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		jsonMsg(c, I18nWeb(c, "get"), fmt.Errorf("invalid node id"))
		return 0, false
	}
	return id, true
}

func (a *NodeController) list(c *gin.Context) {
	nodes, err := a.nodeService.GetNodeTreeView()
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.nodes.toasts.list"), err)
		return
	}
	jsonObj(c, nodes, nil)
}

func (a *NodeController) get(c *gin.Context) {
	id, ok := nodeID(c)
	if !ok {
		return
	}
	n, err := a.nodeService.GetViewById(id)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.nodes.toasts.obtain"), err)
		return
	}
	jsonObj(c, n, nil)
}

type nodeCreateRequest struct {
	Name    string `json:"name" form:"name"`
	Remark  string `json:"remark" form:"remark"`
	Address string `json:"address" form:"address"`
}

type NodeEnrollResponse struct {
	NodeID int    `json:"nodeId" example:"1"`
	Token  string `json:"token" example:"1.k3b1..."`
}

// create registers a pending node and returns its one-time enrollment token (shown once).
func (a *NodeController) create(c *gin.Context) {
	svc, _ := nodelink.Default()
	if svc == nil {
		jsonMsg(c, I18nWeb(c, "pages.nodes.toasts.add"), fmt.Errorf("node link is not running"))
		return
	}
	var req nodeCreateRequest
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, I18nWeb(c, "pages.nodes.toasts.add"), err)
		return
	}
	n, token, err := svc.CreateNode(req.Name, req.Remark, req.Address)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.nodes.toasts.add"), err)
		return
	}
	jsonMsgObj(c, I18nWeb(c, "pages.nodes.toasts.add"), NodeEnrollResponse{NodeID: n.Id, Token: token}, nil)
}

// update edits the descriptive fields; the node link itself is configured by enrollment only.
func (a *NodeController) update(c *gin.Context) {
	id, ok := nodeID(c)
	if !ok {
		return
	}
	var req nodeCreateRequest
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, I18nWeb(c, "pages.nodes.toasts.update"), err)
		return
	}
	req.Name, req.Address = strings.TrimSpace(req.Name), strings.TrimSpace(req.Address)
	if req.Name == "" || len(req.Name) > 64 || len(req.Remark) > 256 || !nodelink.ValidAddress(req.Address) {
		jsonMsg(c, I18nWeb(c, "pages.nodes.toasts.update"), fmt.Errorf("invalid node fields"))
		return
	}
	err := a.nodeService.UpdateDescriptive(id, req.Name, req.Remark, req.Address)
	jsonMsg(c, I18nWeb(c, "pages.nodes.toasts.update"), err)
}

// enrollToken revokes the node's certificate and issues a new one-time token (re-enrollment).
func (a *NodeController) enrollToken(c *gin.Context) {
	id, ok := nodeID(c)
	if !ok {
		return
	}
	svc, srv := nodelink.Default()
	if svc == nil {
		jsonMsg(c, I18nWeb(c, "pages.nodes.toasts.update"), fmt.Errorf("node link is not running"))
		return
	}
	token, err := svc.RenewEnrollment(id)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.nodes.toasts.update"), err)
		return
	}
	srv.Hub().Disconnect(id)
	jsonMsgObj(c, I18nWeb(c, "pages.nodes.toasts.update"), NodeEnrollResponse{NodeID: id, Token: token}, nil)
}

func (a *NodeController) del(c *gin.Context) {
	id, ok := nodeID(c)
	if !ok {
		return
	}
	if err := a.nodeService.Delete(id); err != nil {
		jsonMsg(c, I18nWeb(c, "pages.nodes.toasts.delete"), err)
		return
	}
	if svc, srv := nodelink.Default(); svc != nil {
		svc.DeleteEnrollments(id)
		srv.Hub().Forget(id)
	}
	jsonMsg(c, I18nWeb(c, "pages.nodes.toasts.delete"), nil)
}

func (a *NodeController) setEnable(c *gin.Context) {
	id, ok := nodeID(c)
	if !ok {
		return
	}
	body := struct {
		Enable bool `json:"enable" form:"enable"`
	}{}
	if err := c.ShouldBind(&body); err != nil {
		jsonMsg(c, I18nWeb(c, "pages.nodes.toasts.update"), err)
		return
	}
	if err := a.nodeService.SetEnable(id, body.Enable); err != nil {
		jsonMsg(c, I18nWeb(c, "pages.nodes.toasts.update"), err)
		return
	}
	if !body.Enable {
		if _, srv := nodelink.Default(); srv != nil {
			srv.Hub().Disconnect(id)
		}
	}
	jsonMsg(c, I18nWeb(c, "pages.nodes.toasts.update"), nil)
}

// NodeLive is the live state of one node as last reported over the link.
type NodeLive struct {
	NodeID        int     `json:"nodeId" example:"1"`
	Online        bool    `json:"online" example:"true"`
	LatencyMs     uint32  `json:"latencyMs" example:"42"`
	LastSeen      int64   `json:"lastSeen" example:"1700000000"`
	NodeVersion   string  `json:"nodeVersion" example:"v0.1.0"`
	XrayVersion   string  `json:"xrayVersion" example:"26.3.27"`
	XrayState     string  `json:"xrayState" example:"not_configured"`
	XrayError     string  `json:"xrayError"`
	CPUPct        float64 `json:"cpuPct" example:"12.5"`
	CPUCores      uint32  `json:"cpuCores" example:"4"`
	CPUMhz        float64 `json:"cpuMhz" example:"2400"`
	MemTotal      uint64  `json:"memTotal"`
	MemUsed       uint64  `json:"memUsed"`
	SwapTotal     uint64  `json:"swapTotal"`
	SwapUsed      uint64  `json:"swapUsed"`
	DiskTotal     uint64  `json:"diskTotal"`
	DiskUsed      uint64  `json:"diskUsed"`
	NetUpBps      uint64  `json:"netUpBps"`
	NetDownBps    uint64  `json:"netDownBps"`
	NetSentTotal  uint64  `json:"netSentTotal"`
	NetRecvTotal  uint64  `json:"netRecvTotal"`
	TCPSockets    uint32  `json:"tcpSockets"`
	UDPSockets    uint32  `json:"udpSockets"`
	UptimeSecs    uint64  `json:"uptimeSecs"`
	JournalBytes  uint64  `json:"journalBytes"`
	OnlineClients uint32  `json:"onlineClients"`
}

// live returns the latest metrics of every node that has connected since the panel started.
func (a *NodeController) live(c *gin.Context) {
	out := []NodeLive{}
	if _, srv := nodelink.Default(); srv != nil {
		for _, sn := range srv.Hub().Snapshots() {
			l := NodeLive{
				NodeID: sn.NodeID, Online: sn.Online, LatencyMs: sn.LatencyMs, LastSeen: sn.LastSeen.Unix(),
				NodeVersion: sn.NodeVersion, XrayVersion: sn.XrayVersion,
			}
			if x := sn.Xray; x != nil {
				l.XrayState, l.XrayError = x.State, x.Error
				if x.Version != "" {
					l.XrayVersion = x.Version
				}
			}
			if m := sn.Metrics; m != nil {
				l.CPUPct, l.CPUCores, l.CPUMhz = m.CpuPct, m.CpuCores, m.CpuMhz
				l.MemTotal, l.MemUsed, l.SwapTotal, l.SwapUsed = m.MemTotal, m.MemUsed, m.SwapTotal, m.SwapUsed
				l.DiskTotal, l.DiskUsed = m.DiskTotal, m.DiskUsed
				l.NetUpBps, l.NetDownBps, l.NetSentTotal, l.NetRecvTotal = m.NetUpBps, m.NetDownBps, m.NetSentTotal, m.NetRecvTotal
				l.TCPSockets, l.UDPSockets, l.UptimeSecs = m.TcpSockets, m.UdpSockets, m.UptimeSecs
				l.JournalBytes, l.OnlineClients = m.JournalBytes, m.OnlineClients
			}
			out = append(out, l)
		}
	}
	slices.SortFunc(out, func(x, y NodeLive) int { return x.NodeID - y.NodeID })
	jsonObj(c, out, nil)
}

func (a *NodeController) history(c *gin.Context) {
	id, ok := nodeID(c)
	if !ok {
		return
	}
	metric := c.Param("metric")
	if !slices.Contains(service.NodeMetricKeys, metric) {
		jsonMsg(c, "invalid metric", fmt.Errorf("unknown metric"))
		return
	}
	bucket, err := strconv.Atoi(c.Param("bucket"))
	if err != nil || bucket <= 0 || !service.IsAllowedHistoryBucket(bucket) {
		jsonMsg(c, "invalid bucket", fmt.Errorf("unsupported bucket"))
		return
	}
	jsonObj(c, a.nodeService.AggregateNodeMetric(id, metric, bucket, 60), nil)
}

// importXui adds the inbounds, clients and traffic of a 3x-ui database (x-ui.db) to this node.
func (a *NodeController) importXui(c *gin.Context) {
	id, ok := nodeID(c)
	if !ok {
		return
	}
	file, _, err := c.Request.FormFile("db")
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.index.readDatabaseError"), err)
		return
	}
	defer file.Close()
	var inbounds service.InboundService
	res, err := inbounds.ImportXUIDB(file, id, session.GetLoginUser(c).Id)
	if err != nil {
		jsonMsg(c, I18nWeb(c, "pages.nodes.toasts.import"), err)
		return
	}
	notifyClientsChanged()
	jsonObj(c, res, nil)
}
