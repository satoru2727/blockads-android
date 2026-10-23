package wireguard

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"os"
	"sync"

	"github.com/amnezia-vpn/amneziawg-go/conn"
	"github.com/amnezia-vpn/amneziawg-go/device"
	"github.com/amnezia-vpn/amneziawg-go/tun"
)

// WgOutbound implements OutboundAdapter for WireGuard.
type WgOutbound struct {
	mu     sync.RWMutex
	dev    *device.Device
	tunDev tun.Device
	chTun  *ChannelTUN
	stream *streamNet

	running bool
}

// NewWgOutbound creates a new WireGuard outbound adapter from a virtual channel TUN
// and a UAPI IPC config string.
func NewWgOutbound(tunDev tun.Device, ipcConfig string, protectFn func(fd int) bool) (*WgOutbound, error) {
	logger := device.NewLogger(device.LogLevelVerbose, "[BlockAds/WG] ")

	var bind conn.Bind
	if protectFn != nil {
		bind = newProtectedBind(protectFn)
	} else {
		bind = conn.NewDefaultBind()
	}

	dev := device.NewDevice(tunDev, bind, logger)

	if err := dev.IpcSet(ipcConfig); err != nil {
		dev.Close()
		return nil, fmt.Errorf("IpcSet failed: %v", err)
	}

	chTun, _ := tunDev.(*ChannelTUN)

	return &WgOutbound{
		dev:    dev,
		tunDev: tunDev,
		chTun:  chTun,
	}, nil
}

// Name returns the adapter name.
func (w *WgOutbound) Name() string {
	return "wireguard"
}

// Start brings the WireGuard device online.
func (w *WgOutbound) Start() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.running {
		return fmt.Errorf("WgOutbound already running")
	}

	if err := w.dev.Up(); err != nil {
		return fmt.Errorf("device.Up failed: %v", err)
	}

	w.running = true
	fmt.Fprintln(os.Stderr, "[BlockAds/Go] WgOutbound: started")

	go func() {
		<-w.dev.Wait()
		fmt.Fprintln(os.Stderr, "[BlockAds/Go] WgOutbound: device closed")
		w.mu.Lock()
		w.running = false
		w.mu.Unlock()
	}()

	return nil
}

// NewStreamOutbound creates a WireGuard device attached to a private userspace
// netstack. DialContext produces TCP/UDP connections that are encrypted by the
// same WireGuard device; no packets reach the Android TUN directly.
func NewStreamOutbound(localAddresses []netip.Addr, ipcConfig string, protectFn func(fd int) bool) (*WgOutbound, error) {
	stream, err := newStreamNet(localAddresses, defaultMTU)
	if err != nil {
		return nil, err
	}
	adapter, err := NewWgOutbound(stream, ipcConfig, protectFn)
	if err != nil {
		stream.Close()
		return nil, err
	}
	adapter.stream = stream
	return adapter, nil
}

// DialContext dials an IP literal through the private WireGuard netstack.
// Hostnames are intentionally rejected so upstream DNS cannot recursively use
// the VPN being built.
func (w *WgOutbound) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.stream == nil || w.dev == nil || !w.running {
		return nil, fmt.Errorf("wireguard stream transport is not running")
	}

	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return nil, fmt.Errorf("wireguard stream dial requires an IP literal: %w", err)
	}
	port, err := net.LookupPort(network, portText)
	if err != nil {
		return nil, err
	}
	addrPort := netip.AddrPortFrom(addr.Unmap(), uint16(port))

	switch network {
	case "tcp", "tcp4", "tcp6":
		if (network == "tcp4" && addr.Is6()) || (network == "tcp6" && addr.Is4()) {
			return nil, fmt.Errorf("%s does not match address %s", network, addr)
		}
		return w.stream.dialTCP(ctx, addrPort)
	case "udp", "udp4", "udp6":
		if (network == "udp4" && addr.Is6()) || (network == "udp6" && addr.Is4()) {
			return nil, fmt.Errorf("%s does not match address %s", network, addr)
		}
		return w.stream.dialUDP(addrPort)
	default:
		return nil, fmt.Errorf("unsupported wireguard stream network %q", network)
	}
}

// Listen starts a TCP listener on the private WireGuard netstack. It is not
// exported across gomobile and exists for engine-level tests and future local
// stream services.
func (w *WgOutbound) Listen(network, address string) (net.Listener, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.stream == nil || w.dev == nil || !w.running {
		return nil, fmt.Errorf("wireguard stream transport is not running")
	}
	if network != "tcp" {
		return nil, fmt.Errorf("unsupported wireguard listen network %q", network)
	}
	host, portText, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return nil, fmt.Errorf("wireguard stream listen requires an IP literal: %w", err)
	}
	port, err := net.LookupPort(network, portText)
	if err != nil {
		return nil, err
	}
	return w.stream.listenTCP(netip.AddrPortFrom(addr.Unmap(), uint16(port)))
}

// Stop shuts down the WireGuard device, including one whose Start failed.
func (w *WgOutbound) Stop() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.dev == nil {
		return
	}

	w.dev.Close()
	w.dev = nil
	w.tunDev = nil
	w.chTun = nil
	w.running = false
	fmt.Fprintln(os.Stderr, "[BlockAds/Go] WgOutbound: stopped")
}

// HandlePacket injects a non-DNS packet into WireGuard for encryption.
func (w *WgOutbound) HandlePacket(packet []byte, length int) {
	if w.chTun != nil {
		w.chTun.Inject(packet[:length])
	}
}

// SupportsStreams returns false — WireGuard is a Layer 3 protocol.
func (w *WgOutbound) SupportsStreams() bool {
	return false
}

// IsRunning returns whether the adapter is active.
func (w *WgOutbound) IsRunning() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.running
}
