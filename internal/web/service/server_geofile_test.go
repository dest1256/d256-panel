package service

import (
	"strings"
	"testing"
)

// Loyalsoldier and runetfreedom write "<hash>  geoip.dat"; chocolate4u writes
// "<hash>  release/geoip.dat", the path from its own build.
func TestParseGeofileDigest(t *testing.T) {
	const digest = "0d5d2ba0c5a5c58027fd1347a6afd57c9470799b6bb3cbc274fd4657ed8de382"

	for _, tc := range []struct {
		name  string
		sums  string
		asset string
		want  string
	}{
		{"bare-name", digest + "  geoip.dat\n", "geoip.dat", digest},
		{"build-path", digest + "  release/geoip.dat\n", "geoip.dat", digest},
		{"binary-mode-marker", digest + "  *geoip.dat\n", "geoip.dat", digest},
		{"uppercase-digest", strings.ToUpper(digest) + "  geoip.dat\n", "geoip.dat", digest},
		{"picks-matching-line", "aaaa  geosite.dat\n" + digest + "  geoip.dat\n", "geoip.dat", digest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseGeofileDigest([]byte(tc.sums), tc.asset)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if got != tc.want {
				t.Fatalf("digest = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseGeofileDigest_Errors(t *testing.T) {
	const digest = "0d5d2ba0c5a5c58027fd1347a6afd57c9470799b6bb3cbc274fd4657ed8de382"

	for _, tc := range []struct {
		name    string
		sums    string
		asset   string
		wantErr string
	}{
		// Accepting this would verify geoip.dat against geosite.dat's digest.
		{"names-another-asset", digest + "  geosite.dat\n", "geoip.dat", "no SHA-256 entry for geoip.dat"},
		{"empty", "", "geoip.dat", "no SHA-256 entry for geoip.dat"},
		{"malformed-short", "deadbeef  geoip.dat\n", "geoip.dat", "malformed SHA-256 entry for geoip.dat"},
		{"not-hex", strings.Repeat("z", 64) + "  geoip.dat\n", "geoip.dat", "malformed SHA-256 entry for geoip.dat"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseGeofileDigest([]byte(tc.sums), tc.asset)
			if err == nil {
				t.Fatalf("%s: expected an error", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %q, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestGeofileTagFromLocation(t *testing.T) {
	got, err := geofileTagFromLocation("https://github.com/o/r/releases/download/202609022346/geoip.dat")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got != "202609022346" {
		t.Fatalf("tag = %q, want 202609022346", got)
	}
	for _, bad := range []string{
		"https://github.com/o/r/releases/latest/download/geoip.dat",
		"https://github.com/o/r/releases/download/202609022346",
		"",
	} {
		if _, err := geofileTagFromLocation(bad); err == nil {
			t.Fatalf("expected an error for %q", bad)
		}
	}
}

func TestStandardGeodataSources(t *testing.T) {
	want := []GeodataSource{
		{URL: "https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geoip.dat", File: "geoip.dat"},
		{URL: "https://github.com/chocolate4u/Iran-v2ray-rules/releases/latest/download/geoip.dat", File: "geoip_IR.dat"},
		{URL: "https://github.com/runetfreedom/russia-v2ray-rules-dat/releases/latest/download/geoip.dat", File: "geoip_RU.dat"},
		{URL: "https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geosite.dat", File: "geosite.dat"},
		{URL: "https://github.com/chocolate4u/Iran-v2ray-rules/releases/latest/download/geosite.dat", File: "geosite_IR.dat"},
		{URL: "https://github.com/runetfreedom/russia-v2ray-rules-dat/releases/latest/download/geosite.dat", File: "geosite_RU.dat"},
	}

	got := StandardGeodataSources()
	if len(got) != len(want) {
		t.Fatalf("sources = %d entries, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("source %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}
