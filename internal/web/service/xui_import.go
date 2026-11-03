package service

import (
	"fmt"
	"mime/multipart"
	"os"
	"path/filepath"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
	"github.com/mhsanaei/3x-ui/v3/internal/xray"
)

// XUIImportSkipped is an inbound that was not imported, with the reason.
type XUIImportSkipped struct {
	Remark string `json:"remark"`
	Port   int    `json:"port"`
	Reason string `json:"reason"`
}

// XUIImportResult summarises an x-ui.db import.
type XUIImportResult struct {
	Inbounds int                `json:"inbounds"`
	Clients  int                `json:"clients"`
	Skipped  []XUIImportSkipped `json:"skipped"`
}

// importableProtocols are the protocols the d256 panel can host on a node. MTProto, AmneziaWG, TUIC and
// the like exist in newer 3x-ui databases but are not part of this product.
var importableProtocols = map[model.Protocol]bool{
	model.VMESS: true, model.VLESS: true, model.Trojan: true, model.Shadowsocks: true,
	model.Hysteria: true, model.HTTP: true, model.Mixed: true, model.WireGuard: true, model.Tunnel: true,
}

// ImportXUIDB reads a database file made by 3x-ui (x-ui.db) and creates its inbounds, clients and
// traffic counters on the given node. The panel's own database is only added to, never replaced; the
// source file is opened read-only on a private copy. Inbounds that cannot be imported (unsupported
// protocol, duplicate client e-mail, port clash) are reported and skipped; the rest still import.
func (s *InboundService) ImportXUIDB(file multipart.File, nodeID int, userID int) (*XUIImportResult, error) {
	var node model.Node
	if err := database.GetDB().First(&node, nodeID).Error; err != nil {
		return nil, common.NewErrorf("Node %d not found", nodeID)
	}

	dir, err := os.MkdirTemp("", "d256-xui-import-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "x-ui.db")
	if err := saveUploadedFile(file, path); err != nil {
		return nil, common.NewErrorf("Error saving the uploaded file: %v", err)
	}
	if err := database.ValidateSQLiteDB(path); err != nil {
		return nil, common.NewErrorf("Not a valid SQLite database: %v", err)
	}

	src, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		return nil, common.NewErrorf("Cannot open the database: %v", err)
	}
	if sqlDB, e := src.DB(); e == nil {
		defer sqlDB.Close()
	}
	if !src.Migrator().HasTable("inbounds") {
		return nil, common.NewError("The file has no inbounds table: this is not an x-ui.db")
	}

	var inbounds []model.Inbound
	if err := src.Order("id").Find(&inbounds).Error; err != nil {
		return nil, common.NewErrorf("Cannot read inbounds: %v", err)
	}
	traffic := map[string]xray.ClientTraffic{}
	if src.Migrator().HasTable("client_traffics") {
		var rows []xray.ClientTraffic
		if err := src.Find(&rows).Error; err != nil {
			return nil, common.NewErrorf("Cannot read client traffic: %v", err)
		}
		for _, r := range rows {
			traffic[r.Email] = r
		}
	}

	res := &XUIImportResult{Skipped: []XUIImportSkipped{}}
	for i := range inbounds {
		in := inbounds[i]
		skip := func(reason string) {
			res.Skipped = append(res.Skipped, XUIImportSkipped{Remark: in.Remark, Port: in.Port, Reason: reason})
		}
		if !importableProtocols[in.Protocol] {
			skip(fmt.Sprintf("protocol %q is not supported", in.Protocol))
			continue
		}
		var clash int64
		database.GetDB().Model(&model.Inbound{}).
			Where("node_id = ? AND port = ? AND (listen = ? OR listen = '' OR ? = '')", nodeID, in.Port, in.Listen, in.Listen).
			Count(&clash)
		if clash > 0 {
			skip("port is already used by another inbound on this node")
			continue
		}
		id := nodeID
		in.NodeID, in.UserId, in.Tag, in.ClientStats = &id, userID, "", nil
		in.OriginNodeGuid = ""

		added, _, err := s.AddInbound(&in)
		if err != nil {
			skip(err.Error())
			continue
		}
		res.Inbounds++

		var emails []string
		database.GetDB().Model(&xray.ClientTraffic{}).Where("inbound_id = ?", added.Id).Pluck("email", &emails)
		for _, email := range emails {
			t, ok := traffic[email]
			if !ok {
				continue
			}
			// The source counters are authoritative for usage and limits; ids and inbound links are ours.
			database.GetDB().Model(&xray.ClientTraffic{}).Where("email = ?", email).Updates(map[string]any{
				"up": t.Up, "down": t.Down, "total": t.Total, "expiry_time": t.ExpiryTime, "enable": t.Enable,
				"reset": t.Reset, "last_online": t.LastOnline,
			})
		}
		res.Clients += len(emails)
	}
	InvalidateSubData()
	return res, nil
}
