package tunnel

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestSetTailscaleConfig(t *testing.T) {
	e := NewEngine()
	stateDir := filepath.Join(t.TempDir(), "state")
	raw := `{"stateDir":"` + stateDir + `","hostname":"poc-node","allowedCIDRs":["100.64.0.0/10"]}`
	if got := e.SetTailscaleConfig(raw); got != "" {
		t.Fatalf("SetTailscaleConfig returned %q", got)
	}

	e.mu.Lock()
	cfg := e.tailscaleConfig
	e.mu.Unlock()
	if cfg == nil || cfg.Hostname != "poc-node" {
		t.Fatalf("stored config = %#v", cfg)
	}

	if got := e.SetTailscaleConfig(""); got != "" {
		t.Fatalf("clearing config returned %q", got)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.tailscaleConfig != nil {
		t.Fatal("empty config did not clear Tailscale settings")
	}
}

func TestSetTailscaleConfigRejectsWithoutReplacing(t *testing.T) {
	e := NewEngine()
	stateDir := filepath.Join(t.TempDir(), "state")
	if got := e.SetTailscaleConfig(`{"stateDir":"` + stateDir + `","authKey":"keep-me"}`); got != "" {
		t.Fatalf("valid config returned %q", got)
	}
	if got := e.SetTailscaleConfig(`{"stateDir":"relative","authKey":"do-not-leak"}`); got == "" {
		t.Fatal("invalid config was accepted")
	} else if strings.Contains(got, "do-not-leak") {
		t.Fatalf("AuthKey leaked in error: %q", got)
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	if e.tailscaleConfig == nil || e.tailscaleConfig.AuthKey != "keep-me" {
		t.Fatal("invalid config replaced the previous valid config")
	}
}
