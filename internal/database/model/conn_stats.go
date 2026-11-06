package model

// ConnStatHourly is the usage of one client towards one destination during one hour, per node, inbound and
// device. Rows older than the retention window are rolled up into ConnStatDaily.
//
// Hwid is the hash of the device the usage is attributed to ("" when the device is unknown). Xray never
// sees a device id: the panel maps the connection's source address to the device that fetched the
// subscription from the same address (see ClientHwidIP), so the attribution is as good as that match.
type ConnStatHourly struct {
	Id         int64  `gorm:"primaryKey;autoIncrement"`
	NodeId     int    `gorm:"uniqueIndex:ux_conn_hourly_h,priority:1"`
	Email      string `gorm:"uniqueIndex:ux_conn_hourly_h,priority:2;index:idx_conn_hourly_email_hour,priority:1;index:idx_conn_hourly_email_dest,priority:1"`
	InboundTag string `gorm:"uniqueIndex:ux_conn_hourly_h,priority:3"`
	Dest       string `gorm:"uniqueIndex:ux_conn_hourly_h,priority:4;index:idx_conn_hourly_email_dest,priority:2"`
	Network    string `gorm:"uniqueIndex:ux_conn_hourly_h,priority:5"`
	Hour       int64  `gorm:"uniqueIndex:ux_conn_hourly_h,priority:6;index:idx_conn_hourly_email_hour,priority:2"`
	Hwid       string `gorm:"uniqueIndex:ux_conn_hourly_h,priority:7;size:64;default:''"`
	Up         int64
	Down       int64
	Conns      int64
	// LastSeen is the last moment (unix seconds) the client used the destination in this row.
	LastSeen int64 `gorm:"default:0"`
	// LastIp is the address of that last use (kept in hourly rows only).
	LastIp string `gorm:"size:64;default:''"`
}

func (ConnStatHourly) TableName() string { return "conn_stats_hourly" }

// ConnStatDaily is ConnStatHourly summed per day.
type ConnStatDaily struct {
	Id         int64  `gorm:"primaryKey;autoIncrement"`
	NodeId     int    `gorm:"uniqueIndex:ux_conn_daily_h,priority:1"`
	Email      string `gorm:"uniqueIndex:ux_conn_daily_h,priority:2;index:idx_conn_daily_email_day,priority:1"`
	InboundTag string `gorm:"uniqueIndex:ux_conn_daily_h,priority:3"`
	Dest       string `gorm:"uniqueIndex:ux_conn_daily_h,priority:4"`
	Network    string `gorm:"uniqueIndex:ux_conn_daily_h,priority:5"`
	Day        int64  `gorm:"uniqueIndex:ux_conn_daily_h,priority:6;index:idx_conn_daily_email_day,priority:2"`
	Hwid       string `gorm:"uniqueIndex:ux_conn_daily_h,priority:7;size:64;default:''"`
	Up         int64
	Down       int64
	Conns      int64
	LastSeen   int64 `gorm:"default:0"`
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

// ClientHwidIP remembers from which public address a device (HWID) fetched its subscription. A VPN
// connection from the same address is attributed to that device. Rows are pruned after ClientHwidIPKeepDays.
type ClientHwidIP struct {
	Id       int64  `gorm:"primaryKey;autoIncrement"`
	SubID    string `gorm:"column:sub_id;not null;uniqueIndex:ux_client_hwid_ip,priority:1;index:idx_client_hwid_ip_lookup,priority:1"`
	HwidHash string `gorm:"column:hwid_hash;size:64;not null;uniqueIndex:ux_client_hwid_ip,priority:2"`
	IP       string `gorm:"column:ip;size:64;not null;uniqueIndex:ux_client_hwid_ip,priority:3;index:idx_client_hwid_ip_lookup,priority:2"`
	LastSeen int64  `gorm:"column:last_seen;not null;index"` // unix milliseconds
}

func (ClientHwidIP) TableName() string { return "client_hwid_ips" }

// ClientHwidIPKeepDays is how long a device's subscription address is remembered.
const ClientHwidIPKeepDays = 60
