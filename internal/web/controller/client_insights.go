package controller

import (
	"encoding/csv"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mhsanaei/3x-ui/v3/internal/nodelink"
)

// ClientInsightsController serves what the client info dialog shows: live connections, devices,
// per-destination usage and traffic charts.
type ClientInsightsController struct{}

func NewClientInsightsController(g *gin.RouterGroup) *ClientInsightsController {
	a := &ClientInsightsController{}
	g.GET("/connections", a.connections)
	g.GET("/devices", a.devices)
	g.GET("/destinations", a.destinations)
	g.GET("/traffic", a.traffic)
	return a
}

// ActiveConnView is one live connection of a client.
type ActiveConnView struct {
	NodeID     int    `json:"nodeId"`
	Node       string `json:"node"`
	InboundTag string `json:"inbound"`
	SourceIP   string `json:"sourceIp"`
	Dest       string `json:"dest"`
	Port       uint32 `json:"port"`
	Network    string `json:"network"`
	StartUnix  int64  `json:"start"`
	Up         uint64 `json:"up"`
	Down       uint64 `json:"down"`
}

// DeviceView is a source address a client connected from recently.
type DeviceView struct {
	NodeID       int    `json:"nodeId"`
	Node         string `json:"node"`
	IP           string `json:"ip"`
	LastSeenUnix int64  `json:"lastSeen"`
}

func emailParam(c *gin.Context) (string, bool) {
	e := strings.TrimSpace(c.Query("email"))
	if e == "" || len(e) > 256 {
		jsonMsg(c, "email", errors.New("email is required"))
		return "", false
	}
	return e, true
}

func hubOrNil() *nodelink.Hub {
	if _, srv := nodelink.Default(); srv != nil {
		return srv.Hub()
	}
	return nil
}

func (a *ClientInsightsController) connections(c *gin.Context) {
	email, ok := emailParam(c)
	if !ok {
		return
	}
	out := []ActiveConnView{}
	if h := hubOrNil(); h != nil {
		names := map[int]string{}
		for _, ac := range h.ActiveFor(email) {
			if _, seen := names[ac.NodeID]; !seen {
				names[ac.NodeID] = nodelink.NodeName(ac.NodeID)
			}
			x := ac.Conn
			out = append(out, ActiveConnView{
				NodeID: ac.NodeID, Node: names[ac.NodeID], InboundTag: x.InboundTag, SourceIP: x.SourceIp,
				Dest: x.Dest, Port: x.Port, Network: x.Network, StartUnix: x.StartUnix, Up: x.Up, Down: x.Down,
			})
		}
	}
	jsonObj(c, out, nil)
}

func (a *ClientInsightsController) devices(c *gin.Context) {
	email, ok := emailParam(c)
	if !ok {
		return
	}
	out := []DeviceView{}
	if h := hubOrNil(); h != nil {
		for _, d := range h.IPsFor(email) {
			out = append(out, DeviceView{NodeID: d.NodeID, Node: nodelink.NodeName(d.NodeID), IP: d.IP.Ip, LastSeenUnix: d.IP.LastSeenUnix})
		}
	}
	jsonObj(c, out, nil)
}

func queryInt(c *gin.Context, name string) int64 {
	v, _ := strconv.ParseInt(c.Query(name), 10, 64)
	return v
}

func (a *ClientInsightsController) destinations(c *gin.Context) {
	email, ok := emailParam(c)
	if !ok {
		return
	}
	from, to, _, err := nodelink.Period(c.DefaultQuery("period", "24h"), queryInt(c, "from"), queryInt(c, "to"), time.Now())
	if err != nil {
		jsonMsg(c, "period", err)
		return
	}
	rows, err := nodelink.Destinations(nodelink.DestQuery{
		Email: email, From: from, To: to, NodeID: int(queryInt(c, "nodeId")), Search: c.Query("q"),
		Sort: c.Query("sort"), Desc: c.Query("order") != "asc", Limit: int(queryInt(c, "limit")),
	})
	if err != nil {
		jsonMsg(c, "destinations", err)
		return
	}
	if c.Query("format") != "csv" {
		jsonObj(c, rows, nil)
		return
	}
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="destinations.csv"`)
	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{"destination", "network", "node", "upload_bytes", "download_bytes", "total_bytes", "connections"})
	for _, r := range rows {
		_ = w.Write([]string{
			csvSafe(r.Dest), r.Network, csvSafe(r.Node),
			strconv.FormatInt(r.Up, 10), strconv.FormatInt(r.Down, 10), strconv.FormatInt(r.Total, 10), strconv.FormatInt(r.Conns, 10),
		})
	}
	w.Flush()
}

// csvSafe stops spreadsheet programs from treating a value as a formula.
func csvSafe(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

func (a *ClientInsightsController) traffic(c *gin.Context) {
	email, ok := emailParam(c)
	if !ok {
		return
	}
	from, to, step, err := nodelink.Period(c.DefaultQuery("period", "24h"), queryInt(c, "from"), queryInt(c, "to"), time.Now())
	if err != nil {
		jsonMsg(c, "period", err)
		return
	}
	res, err := nodelink.Traffic(email, from, to, step)
	jsonObj(c, res, err)
}
