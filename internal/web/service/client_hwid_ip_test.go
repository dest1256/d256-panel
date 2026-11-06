package service

import (
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

func TestRecordHwidIPRemembersTheAddressOfADevice(t *testing.T) {
	setupBulkDB(t)
	var svc ClientService
	const hwid = "device-id-12345"

	svc.RecordHwidIP("sub1", hwid, "203.0.113.7")
	svc.RecordHwidIP("sub1", hwid, "203.0.113.7") // the same address again: one row, newer time
	svc.RecordHwidIP("sub1", hwid, "2001:db8::1")
	svc.RecordHwidIP("sub2", hwid, "203.0.113.7") // another subscription is a separate record

	var rows []model.ClientHwidIP
	database.GetDB().Order("sub_id, ip").Find(&rows)
	if len(rows) != 3 {
		t.Fatalf("rows = %+v", rows)
	}
	if rows[0].HwidHash != hashHwid(hwid) || rows[0].HwidHash == hwid {
		t.Fatalf("only the hash of the device id may be stored: %+v", rows[0])
	}
	if rows[0].LastSeen == 0 {
		t.Fatal("the time must be recorded")
	}
}

func TestRecordHwidIPIgnoresUnusableInput(t *testing.T) {
	setupBulkDB(t)
	var svc ClientService
	svc.RecordHwidIP("", "device-id-12345", "203.0.113.7")        // no subscription
	svc.RecordHwidIP("sub1", "", "203.0.113.7")                   // the client sent no device id
	svc.RecordHwidIP("sub1", "abc", "203.0.113.7")                // too short to be a device id
	svc.RecordHwidIP("sub1", "device-id-12345", "not an address") // not an address
	svc.RecordHwidIP("sub1", "device-id-12345", "")

	var n int64
	database.GetDB().Model(&model.ClientHwidIP{}).Count(&n)
	if n != 0 {
		t.Fatalf("%d rows were stored for unusable input", n)
	}
}
