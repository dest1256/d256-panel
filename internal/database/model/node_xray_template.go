package model

// NodeXrayTemplate overrides the global Xray template (routing, outbounds, DNS, policy) for one node.
// A node without a row uses the global template.
type NodeXrayTemplate struct {
	NodeId    int    `gorm:"primaryKey;autoIncrement:false"`
	Template  string `gorm:"type:text"`
	UpdatedAt int64
}
