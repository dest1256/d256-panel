package service

import (
	"testing"
)

func TestNormalizeBasePath(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", "/"},
		{"   ", "/"},
		{"/", "/"},
		{"/panel", "/panel/"},
		{"panel", "/panel/"},
		{"panel/", "/panel/"},
		{"/panel/", "/panel/"},
		{"  /panel  ", "/panel/"},
		{"/a/b/c", "/a/b/c/"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got := normalizeBasePath(c.in)
			if got != c.want {
				t.Fatalf("normalizeBasePath(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestNodeMetricKey(t *testing.T) {
	cases := []struct {
		id     int
		metric string
		want   string
	}{
		{1, "cpu", "node:1:cpu"},
		{42, "mem", "node:42:mem"},
		{0, "anything", "node:0:anything"},
	}
	for _, c := range cases {
		got := nodeMetricKey(c.id, c.metric)
		if got != c.want {
			t.Fatalf("nodeMetricKey(%d, %q) = %q, want %q", c.id, c.metric, got, c.want)
		}
	}
}
