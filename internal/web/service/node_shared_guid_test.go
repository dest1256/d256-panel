package service

import (
	"fmt"
	"testing"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// Cloned node servers ship an identical panelGuid in their copied settings, and
// a node cloned from the master shares the master's own GUID. effectiveNodeGuid
// must keep each physical node in its own attribution bucket by falling back to
// the node-unique id for both collision kinds, while leaving a uniquely-named
// node on its real GUID and never folding transitive (Id 0) nodes.
func TestEffectiveNodeGuid_DisambiguatesAmbiguousGuids(t *testing.T) {
	nodes := []*model.Node{
		{Id: 1, Guid: "dup"},
		{Id: 2, Guid: "dup"},
		{Id: 3, Guid: "uniq"},
		{Id: 4, Guid: ""},
		{Id: 5, Guid: "master"},
		{Id: 0, Guid: "transitive"},
	}
	ambiguous := ambiguousNodeGuids(nodes, "master")

	if _, ok := ambiguous["dup"]; !ok {
		t.Fatalf("dup must be flagged ambiguous, got %v", ambiguous)
	}
	if _, ok := ambiguous["master"]; !ok {
		t.Fatalf("a node sharing the master GUID must be flagged, got %v", ambiguous)
	}
	if _, ok := ambiguous["uniq"]; ok {
		t.Fatalf("uniq must not be flagged, got %v", ambiguous)
	}
	if _, ok := ambiguous["transitive"]; ok {
		t.Fatalf("transitive (Id 0) must not count, got %v", ambiguous)
	}

	cases := map[*model.Node]string{
		nodes[0]: "node:1",
		nodes[1]: "node:2",
		nodes[2]: "uniq",
		nodes[3]: "node:4",
		nodes[4]: "node:5",
		nodes[5]: "transitive",
	}
	for n, want := range cases {
		if got := effectiveNodeGuid(n, ambiguous); got != want {
			t.Errorf("effectiveNodeGuid(Id=%d, Guid=%q) = %q, want %q", n.Id, n.Guid, got, want)
		}
	}
}

// effectiveNodeKey (the no-preloaded-list variant used by the write paths) must
// agree with the slice helper: fall back to the node-unique id when a GUID is
// shared with another node or with the master, else keep the real GUID.
func TestEffectiveNodeKey_FallsBackOnCollision(t *testing.T) {
	setupConflictDB(t)
	db := database.GetDB()
	selfGuid, _ := (&SettingService{}).GetPanelGuid()
	if selfGuid == "" {
		t.Fatal("expected a panel guid")
	}

	mk := func(id int, name, guid string) *model.Node {
		n := &model.Node{Id: id, Name: name, Address: fmt.Sprintf("10.0.0.%d", id), Port: 2053, ApiToken: "t", Guid: guid, Status: "online"}
		if err := db.Create(n).Error; err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		return n
	}
	dupA := mk(1, "a", "shared")
	mk(2, "b", "shared")
	uniq := mk(3, "c", "solo")
	masterClone := mk(4, "d", selfGuid)

	if got := effectiveNodeKey(dupA); got != "node:1" {
		t.Errorf("node-node collision: got %q, want node:1", got)
	}
	if got := effectiveNodeKey(uniq); got != "solo" {
		t.Errorf("unique node: got %q, want solo", got)
	}
	if got := effectiveNodeKey(masterClone); got != "node:4" {
		t.Errorf("master collision: got %q, want node:4", got)
	}
}
