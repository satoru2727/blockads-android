//go:build !tailscale

package tunnel

import (
	"strings"
	"testing"
)

func TestTailscaleOutboundStub(t *testing.T) {
	cfg, err := ParseTailscaleConfig(`{"stateDir":"/tmp/blockads"}`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = newTailscaleOutbound(cfg, nil)
	if err == nil || !strings.Contains(err.Error(), "rebuild with -tags tailscale") {
		t.Fatalf("stub error = %v", err)
	}
}
