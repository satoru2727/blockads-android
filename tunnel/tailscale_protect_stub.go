//go:build tailscale && !android

package tunnel

func setTailscaleProtectFunc(func(int) error) {}
