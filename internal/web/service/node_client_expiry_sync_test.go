package service

import (
	"testing"
)

// TestMergeActivationExpiry covers the pure reconciliation rule in isolation.
func TestMergeActivationExpiry(t *testing.T) {
	const (
		dur   = int64(-2592000000) // 30 days as a "start after first connect" duration
		early = int64(1000)        // earliest absolute deadline (first connection)
		late  = int64(2000)        // a later absolute deadline
	)
	cases := []struct {
		name           string
		existing, node int64
		want           int64
	}{
		{"master unset takes node duration", 0, dur, dur},
		{"master unset takes node activation", 0, early, early},
		{"activation adopted over stored duration", dur, early, early},
		{"node still un-activated does not reset deadline", early, dur, early},
		{"node un-activated zero does not reset deadline", early, 0, early},
		{"master absolute ignores later node absolute", early, late, early},
		{"node value equal to master is a no-op", early, early, early},
		{"stale earlier absolute does not clobber later", late, early, late},
		{"both un-activated keep node value", dur, dur, dur},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := mergeActivationExpiry(c.existing, c.node); got != c.want {
				t.Fatalf("mergeActivationExpiry(%d,%d) = %d, want %d", c.existing, c.node, got, c.want)
			}
		})
	}
}

// Relative to the run: the merge rules now compare the master deadline against
// wall-clock now, so fixed timestamps would rot into the wrong side of it.
var ()
