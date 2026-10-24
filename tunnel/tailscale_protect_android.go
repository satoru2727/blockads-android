//go:build tailscale && android

package tunnel

import "tailscale.com/net/netns"

func setTailscaleProtectFunc(protectFn func(int) error) {
	netns.SetAndroidProtectFunc(protectFn)
}
