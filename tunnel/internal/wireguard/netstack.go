package wireguard

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"os"
	"sync"

	"github.com/amnezia-vpn/amneziawg-go/tun"
	"gvisor.dev/gvisor/pkg/buffer"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/link/channel"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv6"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
	"gvisor.dev/gvisor/pkg/tcpip/transport/udp"
)

// streamNet is a private netstack used as WireGuard's TUN device and as the
// caller-facing TCP/UDP dialer. It deliberately omits DNS and hostname lookup:
// BlockAds resolves upstreams before dialing so those lookups cannot loop back
// through the VPN.
type streamNet struct {
	endpoint  *channel.Endpoint
	stack     *stack.Stack
	events    chan tun.Event
	incoming  chan *buffer.View
	mtu       int
	closeOnce sync.Once
	closeMu   sync.Mutex
	closed    bool
	done      chan struct{}
}

func newStreamNet(addresses []netip.Addr, mtu int) (*streamNet, error) {
	s := &streamNet{
		endpoint: channel.New(1024, uint32(mtu), "wireguard-stream"),
		stack: stack.New(stack.Options{
			NetworkProtocols:   []stack.NetworkProtocolFactory{ipv4.NewProtocol, ipv6.NewProtocol},
			TransportProtocols: []stack.TransportProtocolFactory{tcp.NewProtocol, udp.NewProtocol},
			HandleLocal:        true,
		}),
		events:   make(chan tun.Event, 1),
		incoming: make(chan *buffer.View, 256),
		mtu:      mtu,
		done:     make(chan struct{}),
	}
	s.endpoint.AddNotify(&streamNotifier{s: s})
	if err := s.stack.CreateNIC(1, s.endpoint); err != nil {
		return nil, fmt.Errorf("create stream NIC: %s", err)
	}

	var hasV4, hasV6 bool
	for _, address := range addresses {
		address = address.Unmap()
		protocol := ipv6.ProtocolNumber
		if address.Is4() {
			protocol = ipv4.ProtocolNumber
			hasV4 = true
		} else {
			hasV6 = true
		}
		protocolAddress := tcpip.ProtocolAddress{
			Protocol:          protocol,
			AddressWithPrefix: tcpip.AddrFromSlice(address.AsSlice()).WithPrefix(),
		}
		if err := s.stack.AddProtocolAddress(1, protocolAddress, stack.AddressProperties{}); err != nil {
			return nil, fmt.Errorf("add stream address %s: %s", address, err)
		}
	}
	if hasV4 {
		s.stack.AddRoute(tcpip.Route{Destination: header.IPv4EmptySubnet, NIC: 1})
	}
	if hasV6 {
		s.stack.AddRoute(tcpip.Route{Destination: header.IPv6EmptySubnet, NIC: 1})
	}
	s.events <- tun.EventUp
	return s, nil
}

func (s *streamNet) Read(bufs [][]byte, sizes []int, offset int) (int, error) {
	select {
	case view := <-s.incoming:
		n, err := view.Read(bufs[0][offset:])
		view.Release()
		if err != nil {
			return 0, err
		}
		sizes[0] = n
		return 1, nil
	case <-s.done:
		return 0, net.ErrClosed
	}
}

func (s *streamNet) Write(bufs [][]byte, offset int) (int, error) {
	select {
	case <-s.done:
		return 0, net.ErrClosed
	default:
	}
	for _, buf := range bufs {
		packet := buf[offset:]
		if len(packet) == 0 {
			continue
		}
		protocol := ipv6.ProtocolNumber
		if packet[0]>>4 == 4 {
			protocol = ipv4.ProtocolNumber
		}
		pkt := stack.NewPacketBuffer(stack.PacketBufferOptions{
			Payload: buffer.MakeWithData(append([]byte(nil), packet...)),
		})
		s.endpoint.InjectInbound(protocol, pkt)
	}
	return len(bufs), nil
}

func (s *streamNet) Close() error {
	s.closeOnce.Do(func() {
		s.closeMu.Lock()
		s.closed = true
		close(s.done)
		s.stack.RemoveNIC(1)
		s.endpoint.Close()
		close(s.events)
		s.closeMu.Unlock()
	})
	return nil
}

func (s *streamNet) MTU() (int, error)        { return s.mtu, nil }
func (s *streamNet) Name() (string, error)    { return "wireguard-stream", nil }
func (s *streamNet) Events() <-chan tun.Event { return s.events }
func (s *streamNet) File() *os.File           { return nil }
func (s *streamNet) BatchSize() int           { return 1 }

func (s *streamNet) dialTCP(ctx context.Context, addr netip.AddrPort) (net.Conn, error) {
	fullAddr, protocol := streamFullAddr(addr)
	return gonet.DialContextTCP(ctx, s.stack, fullAddr, protocol)
}

func (s *streamNet) dialUDP(addr netip.AddrPort) (net.Conn, error) {
	fullAddr, protocol := streamFullAddr(addr)
	return gonet.DialUDP(s.stack, nil, &fullAddr, protocol)
}

func (s *streamNet) listenTCP(addr netip.AddrPort) (net.Listener, error) {
	fullAddr, protocol := streamFullAddr(addr)
	return gonet.ListenTCP(s.stack, fullAddr, protocol)
}

func streamFullAddr(addr netip.AddrPort) (tcpip.FullAddress, tcpip.NetworkProtocolNumber) {
	address := addr.Addr().Unmap()
	protocol := ipv6.ProtocolNumber
	if address.Is4() {
		protocol = ipv4.ProtocolNumber
	}
	return tcpip.FullAddress{
		NIC:  1,
		Addr: tcpip.AddrFromSlice(address.AsSlice()),
		Port: addr.Port(),
	}, protocol
}

type streamNotifier struct{ s *streamNet }

func (n *streamNotifier) WriteNotify() {
	s := n.s
	pkt := s.endpoint.Read()
	if pkt == nil {
		return
	}
	view := pkt.ToView()
	pkt.DecRef()
	if view == nil {
		return
	}
	select {
	case <-s.done:
		view.Release()
	case s.incoming <- view:
	default:
		view.Release()
	}
}

var _ tun.Device = (*streamNet)(nil)
