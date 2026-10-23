package wireguard

import (
	"context"
	"io"
	"net/netip"
	"testing"
	"time"
)

func TestStreamOutboundDialsListener(t *testing.T) {
	local := netip.MustParseAddr("10.64.0.2")
	outbound, err := NewStreamOutbound([]netip.Addr{local}, testIpc(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(outbound.Stop)
	if err := outbound.Start(); err != nil {
		t.Fatal(err)
	}
	if !outbound.SupportsStreams() {
		t.Fatal("stream outbound does not advertise stream support")
	}

	listener, err := outbound.Listen("tcp", "10.64.0.2:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		server, err := listener.Accept()
		if err != nil {
			return
		}
		defer server.Close()
		buf := make([]byte, 5)
		_, _ = server.Read(buf)
		_, _ = server.Write([]byte("world"))
	}()

	client, err := outbound.DialContext(context.Background(), "tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	deadline := time.Now().Add(3 * time.Second)
	_ = client.SetDeadline(deadline)
	if _, err := client.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 5)
	if _, err := io.ReadFull(client, buf); err != nil {
		t.Fatal(err)
	}
	if string(buf) != "world" {
		t.Fatalf("echo = %q", buf)
	}
}

func TestStreamOutboundSupportsUDPDial(t *testing.T) {
	local := netip.MustParseAddr("10.64.0.2")
	remote := netip.MustParseAddrPort("10.64.0.3:53")
	outbound, err := NewStreamOutbound([]netip.Addr{local}, testIpc(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(outbound.Stop)
	if err := outbound.Start(); err != nil {
		t.Fatal(err)
	}

	conn, err := outbound.DialContext(context.Background(), "udp", remote.String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if conn.RemoteAddr().Network() != "udp" || conn.RemoteAddr().String() != remote.String() {
		t.Fatalf("UDP remote = %v, want %v", conn.RemoteAddr(), remote)
	}
}

func TestStreamOutboundRejectsHostnameBeforeStart(t *testing.T) {
	outbound, err := NewStreamOutbound(nil, testIpc(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(outbound.Stop)
	if _, err := outbound.DialContext(context.Background(), "tcp", "example.com:443"); err == nil {
		t.Fatal("dial accepted a hostname")
	}
}
