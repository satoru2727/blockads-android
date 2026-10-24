//go:build tailscale

package tunnel

import (
	"context"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTailscaleRealTailnetTCP(t *testing.T) {
	authKey := os.Getenv("TS_AUTHKEY")
	target := os.Getenv("TS_TEST_TARGET")
	if authKey == "" || target == "" {
		t.Skip("set TS_AUTHKEY and TS_TEST_TARGET to run the real Tailnet TCP test")
	}

	cfg, err := ParseTailscaleConfig(`{"stateDir":"` + filepath.Join(t.TempDir(), "state") + `","authKey":"` + authKey + `"}`)
	if err != nil {
		t.Fatal("test config could not be parsed")
	}
	outbound, err := newTailscaleOutbound(cfg, nil)
	if err != nil {
		t.Fatal("test outbound could not be created")
	}
	if err := outbound.Start(); err != nil {
		t.Fatal("tsnet could not start")
	}
	t.Cleanup(outbound.Stop)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	conn, err := outbound.DialContext(ctx, "tcp", target)
	if err != nil {
		t.Fatal("Tailnet TCP dial failed")
	}
	_ = conn.Close()
}

func TestTailscaleOutboundContracts(t *testing.T) {
	var _ OutboundAdapter = (*tailscaleOutbound)(nil)
	var _ interface {
		DialContext(context.Context, string, string) (net.Conn, error)
	} = (*tailscaleOutbound)(nil)
}

func TestTailscaleOutboundMatchesWithoutStarting(t *testing.T) {
	cfg, err := ParseTailscaleConfig(`{"stateDir":"/tmp/blockads","allowedCIDRs":["100.64.0.0/10"]}`)
	if err != nil {
		t.Fatal(err)
	}
	outbound, err := newTailscaleOutbound(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !outbound.Matches(netip.MustParseAddr("100.64.0.1")) {
		t.Fatal("allowed address did not match")
	}
	if outbound.Matches(netip.MustParseAddr("192.0.2.1")) {
		t.Fatal("disallowed address matched")
	}
}
