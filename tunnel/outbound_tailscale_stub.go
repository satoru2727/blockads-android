//go:build !tailscale

package tunnel

import (
	"errors"
)

func newTailscaleOutbound(*TailscaleConfig, func(int) bool) (OutboundAdapter, error) {
	return nil, errors.New("tailscale support is not enabled; rebuild with -tags tailscale")
}
