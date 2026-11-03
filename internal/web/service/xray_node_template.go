package service

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
)

// GetXrayConfigTemplateForNode returns the node's own template when it has one, else the global one.
// override reports which of the two was returned.
func (s *SettingService) GetXrayConfigTemplateForNode(nodeID int) (template string, override bool, err error) {
	var row model.NodeXrayTemplate
	res := database.GetDB().Where("node_id = ?", nodeID).Limit(1).Find(&row)
	if res.Error != nil {
		return "", false, res.Error
	}
	if res.RowsAffected > 0 && row.Template != "" {
		return row.Template, true, nil
	}
	template, err = s.GetXrayConfigTemplate()
	return template, false, err
}

// SaveNodeXraySetting validates the template like the global one and stores it as the node's override.
func (s *XraySettingService) SaveNodeXraySetting(nodeID int, template string) error {
	var count int64
	if err := database.GetDB().Model(&model.Node{}).Where("id = ?", nodeID).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return common.NewError("node not found")
	}
	prepared, err := s.prepareXraySetting(template)
	if err != nil {
		return err
	}
	return database.GetDB().Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "node_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"template", "updated_at"}),
	}).Create(&model.NodeXrayTemplate{NodeId: nodeID, Template: prepared, UpdatedAt: time.Now().Unix()}).Error
}

// DeleteNodeXraySetting drops the node's override so it falls back to the global template.
func (s *XraySettingService) DeleteNodeXraySetting(nodeID int) error {
	return database.GetDB().Where("node_id = ?", nodeID).Delete(&model.NodeXrayTemplate{}).Error
}

// deleteNodeTemplateTx removes a deleted node's override.
func deleteNodeTemplateTx(tx *gorm.DB, nodeID int) error {
	return tx.Where("node_id = ?", nodeID).Delete(&model.NodeXrayTemplate{}).Error
}
