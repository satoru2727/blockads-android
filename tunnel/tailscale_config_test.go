package tunnel

import (
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseTailscaleConfig(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "state")
	cfg, err := ParseTailscaleConfig(`{"stateDir":"` + stateDir + `","authKey":"tskey-secret","allowedCIDRs":["100.64.1.9/24","fd7a:115c:a1e0::/48"]}`)
	if err != nil {
		t.Fatalf("ParseTailscaleConfig failed: %v", err)
	}
	if cfg.Hostname != defaultTailscaleHostname {
		t.Fatalf("Hostname = %q, want %q", cfg.Hostname, defaultTailscaleHostname)
	}
	if got, want := strings.Join(cfg.AllowedCIDRs, ","), "100.64.1.0/24,fd7a:115c:a1e0::/48"; got != want {
		t.Fatalf("AllowedCIDRs = %q, want %q", got, want)
	}
	if cfg.AuthKey != "tskey-secret" {
		t.Fatal("AuthKey was not retained in memory")
	}
}

func TestParseTailscaleConfigRejectsInvalidJSON(t *testing.T) {
	if _, err := ParseTailscaleConfig(`{"stateDir":`); err == nil {
		t.Fatal("invalid JSON was accepted")
	}
}

func TestParseTailscaleConfigRejectsRelativeStateDir(t *testing.T) {
	if _, err := ParseTailscaleConfig(`{"stateDir":"relative"}`); err == nil {
		t.Fatal("relative stateDir was accepted")
	}
}

func TestParseTailscaleConfigRejectsInvalidCIDR(t *testing.T) {
	_, err := ParseTailscaleConfig(`{"stateDir":"/tmp/blockads","allowedCIDRs":["not-a-cidr"]}`)
	if err == nil {
		t.Fatal("invalid CIDR was accepted")
	}
	if strings.Contains(err.Error(), "not-a-cidr") {
		t.Fatalf("CIDR value leaked in error: %v", err)
	}
}

func TestTailscaleConfigMatches(t *testing.T) {
	cfg, err := ParseTailscaleConfig(`{"stateDir":"/tmp/blockads","allowedCIDRs":["100.64.0.0/10","2001:db8:1234::/48"]}`)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		addr string
		want bool
	}{
		{"IPv4 lower boundary", "100.64.0.0", true},
		{"IPv4 upper boundary", "100.127.255.255", true},
		{"IPv4 outside", "100.128.0.0", false},
		{"IPv6 lower boundary", "2001:db8:1234::", true},
		{"IPv6 outside", "2001:db8:1235::1", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cfg.Matches(netip.MustParseAddr(tt.addr)); got != tt.want {
				t.Fatalf("Matches(%s) = %t, want %t", tt.addr, got, tt.want)
			}
		})
	}

	empty, err := ParseTailscaleConfig(`{"stateDir":"/tmp/blockads"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !empty.Matches(netip.MustParseAddr("192.0.2.1")) {
		t.Fatal("empty allowlist did not permit address")
	}
}

func TestTailscaleConfigEnsureStateDir(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "nested", "state")
	cfg, err := ParseTailscaleConfig(`{"stateDir":"` + stateDir + `"}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.EnsureStateDir(); err != nil {
		t.Fatalf("EnsureStateDir failed: %v", err)
	}
	info, err := os.Stat(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0700 {
		t.Fatalf("state directory mode = %o, want 0700", info.Mode().Perm())
	}

	if err := os.Chmod(stateDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := cfg.EnsureStateDir(); err != nil {
		t.Fatalf("EnsureStateDir did not repair mode: %v", err)
	}
	info, err = os.Stat(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0700 {
		t.Fatalf("repaired state directory mode = %o, want 0700", info.Mode().Perm())
	}
}

func TestParseTailscaleConfigDoesNotLeakAuthKey(t *testing.T) {
	secret := "tskey-auth-secret"
	_, err := ParseTailscaleConfig(`{"stateDir":"relative","authKey":"` + secret + `"}`)
	if err == nil {
		t.Fatal("invalid config was accepted")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("AuthKey leaked in error: %v", err)
	}
}
