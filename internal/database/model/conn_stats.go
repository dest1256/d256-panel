package model

// ConnStatHourly is the usage of one client towards one destination during one hour, per node and inbound.
// Rows older than the retention window are rolled up into ConnStatDaily.
type ConnStatHourly struct {
	Id         int64  `gorm:"primaryKey;autoIncrement"`
	NodeId     int    `gorm:"uniqueIndex:ux_conn_hourly,priority:1"`
	Email      string `gorm:"uniqueIndex:ux_conn_hourly,priority:2;index:idx_conn_hourly_email_hour,priority:1;index:idx_conn_hourly_email_dest,priority:1"`
	InboundTag string `gorm:"uniqueIndex:ux_conn_hourly,priority:3"`
	Dest       string `gorm:"uniqueIndex:ux_conn_hourly,priority:4;index:idx_conn_hourly_email_dest,priority:2"`
	Network    string `gorm:"uniqueIndex:ux_conn_hourly,priority:5"`
	Hour       int64  `gorm:"uniqueIndex:ux_conn_hourly,priority:6;index:idx_conn_hourly_email_hour,priority:2"`
	Up         int64
	Down       int64
	Conns      int64
}

func (ConnStatHourly) TableName() string { return "conn_stats_hourly" }

// ConnStatDaily is ConnStatHourly summed per day.
type ConnStatDaily struct {
	Id         int64  `gorm:"primaryKey;autoIncrement"`
	NodeId     int    `gorm:"uniqueIndex:ux_conn_daily,priority:1"`
	Email      string `gorm:"uniqueIndex:ux_conn_daily,priority:2;index:idx_conn_daily_email_day,priority:1"`
	InboundTag string `gorm:"uniqueIndex:ux_conn_daily,priority:3"`
	Dest       string `gorm:"uniqueIndex:ux_conn_daily,priority:4"`
	Network    string `gorm:"uniqueIndex:ux_conn_daily,priority:5"`
	Day        int64  `gorm:"uniqueIndex:ux_conn_daily,priority:6;index:idx_conn_daily_email_day,priority:2"`
	Up         int64
	Down       int64
	Conns      int64
}

func (ConnStatDaily) TableName() string { return "conn_stats_daily" }

// Traffic bucket kinds.
const (
	TrafficBucketUser    = 0
	TrafficBucketInbound = 1
)

// TrafficBucket is usage over a time bucket for the traffic charts. Len is the bucket length in seconds:
// 300 for recent data, 3600 once rolled up.
type TrafficBucket struct {
	Id     int64  `gorm:"primaryKey;autoIncrement"`
	NodeId int    `gorm:"uniqueIndex:ux_traffic_bucket,priority:1"`
	Kind   int    `gorm:"uniqueIndex:ux_traffic_bucket,priority:2"`
	Name   string `gorm:"uniqueIndex:ux_traffic_bucket,priority:3;index:idx_traffic_bucket_name,priority:1"` // client email or inbound tag
	Len    int    `gorm:"uniqueIndex:ux_traffic_bucket,priority:4"`
	Start  int64  `gorm:"uniqueIndex:ux_traffic_bucket,priority:5;index:idx_traffic_bucket_name,priority:2"`
	Up     int64
	Down   int64
}

func (TrafficBucket) TableName() string { return "traffic_buckets" }
