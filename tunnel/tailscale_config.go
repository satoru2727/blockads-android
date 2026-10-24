package tunnel

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
)

const defaultTailscaleHostname = "blockads-android"

// TailscaleConfig contains the app-private settings for the tsnet PoC.
type TailscaleConfig struct {
	StateDir     string   `json:"stateDir"`
	Hostname     string   `json:"hostname"`
	AuthKey      string   `json:"authKey"`
	ControlURL   string   `json:"controlURL"`
	AllowedCIDRs []string `json:"allowedCIDRs"`

	allowedPrefixes []netip.Prefix
}

// ParseTailscaleConfig parses and validates the JSON configuration without
// including secrets in returned errors.
func ParseTailscaleConfig(raw string) (*TailscaleConfig, error) {
	var cfg TailscaleConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return nil, fmt.Errorf("invalid tailscale config JSON: %w", err)
	}
	if cfg.StateDir == "" {
		return nil, fmt.Errorf("tailscale stateDir is required")
	}
	if !filepath.IsAbs(cfg.StateDir) {
		return nil, fmt.Errorf("tailscale stateDir must be an absolute path")
	}
	if cfg.Hostname == "" {
		cfg.Hostname = defaultTailscaleHostname
	}

	cfg.allowedPrefixes = make([]netip.Prefix, 0, len(cfg.AllowedCIDRs))
	for i, rawCIDR := range cfg.AllowedCIDRs {
		prefix, err := netip.ParsePrefix(rawCIDR)
		if err != nil {
			return nil, fmt.Errorf("tailscale allowedCIDRs[%d] is invalid", i)
		}
		prefix = prefix.Masked()
		cfg.AllowedCIDRs[i] = prefix.String()
		cfg.allowedPrefixes = append(cfg.allowedPrefixes, prefix)
	}
	return &cfg, nil
}

// Matches reports whether addr is eligible for the Tailscale outbound.
// An empty allowlist permits every address.
func (c *TailscaleConfig) Matches(addr netip.Addr) bool {
	if c == nil || len(c.allowedPrefixes) == 0 {
		return true
	}
	for _, prefix := range c.allowedPrefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

// EnsureStateDir creates the tsnet state directory and enforces mode 0700.
func (c *TailscaleConfig) EnsureStateDir() error {
	if c == nil || c.StateDir == "" || !filepath.IsAbs(c.StateDir) {
		return fmt.Errorf("tailscale stateDir must be an absolute path")
	}
	if err := os.MkdirAll(c.StateDir, 0700); err != nil {
		return fmt.Errorf("create tailscale state directory: %w", err)
	}
	if err := os.Chmod(c.StateDir, 0700); err != nil {
		return fmt.Errorf("secure tailscale state directory: %w", err)
	}
	info, err := os.Stat(c.StateDir)
	if err != nil {
		return fmt.Errorf("stat tailscale state directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("tailscale state path is not a directory")
	}
	if info.Mode().Perm() != 0700 {
		return fmt.Errorf("tailscale state directory must have mode 0700")
	}
	return nil
}
