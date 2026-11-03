package model

// NodeEnrollment is a one-time enrollment token for a node. Only the SHA-256 of the
// secret is stored; UsedAt != 0 means redeemed.
type NodeEnrollment struct {
	Id          int    `gorm:"primaryKey;autoIncrement"`
	NodeId      int    `gorm:"index"`
	TokenSha256 string `gorm:"column:token_sha256"`
	ExpiresAt   int64
	UsedAt      int64
}
