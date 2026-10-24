//go:build tailscale

package tunnel

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"sync"

	"tailscale.com/tsnet"
)

// tailscaleOutbound bridges the full-stack TCP dial seam to a tsnet node.
type tailscaleOutbound struct {
	mu      sync.RWMutex
	config  *TailscaleConfig
	server  *tsnet.Server
	running bool
	protect func(fd int) bool
}

func newTailscaleOutbound(config *TailscaleConfig, protectFn func(fd int) bool) (*tailscaleOutbound, error) {
	if config == nil {
		return nil, errors.New("tailscale config is nil")
	}
	return &tailscaleOutbound{
		config:  config,
		protect: protectFn,
	}, nil
}

func (t *tailscaleOutbound) Name() string {
	return "tailscale"
}

func (t *tailscaleOutbound) Start() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.running {
		return errors.New("tailscale outbound already running")
	}
	if err := t.config.EnsureStateDir(); err != nil {
		return err
	}

	setTailscaleProtectFunc(t.protectFunc())
	server := &tsnet.Server{
		Dir:        t.config.StateDir,
		Hostname:   t.config.Hostname,
		AuthKey:    t.config.AuthKey,
		ControlURL: t.config.ControlURL,
		UserLogf: func(format string, args ...any) {
			logf("[Tailscale] "+format, args...)
		},
	}
	if err := server.Start(); err != nil {
		setTailscaleProtectFunc(nil)
		_ = server.Close()
		return sanitizeTailscaleError(err, t.config.AuthKey)
	}

	t.server = server
	t.running = true
	logf("Tailscale outbound started (hostname=%s)", t.config.Hostname)
	return nil
}

func (t *tailscaleOutbound) Stop() {
	t.mu.Lock()
	server := t.server
	t.server = nil
	t.running = false
	t.mu.Unlock()

	if server != nil {
		if err := server.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			logf("Tailscale outbound close failed: %v", sanitizeTailscaleError(err, t.config.AuthKey))
		}
	}
	setTailscaleProtectFunc(nil)
	logf("Tailscale outbound stopped")
}

func (t *tailscaleOutbound) HandlePacket([]byte, int) {}

func (t *tailscaleOutbound) SupportsStreams() bool {
	return true
}

// Matches reports whether the destination is allowed by the configured CIDRs.
func (t *tailscaleOutbound) Matches(addr netip.Addr) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.config.Matches(addr)
}

func (t *tailscaleOutbound) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	t.mu.RLock()
	server := t.server
	running := t.running
	t.mu.RUnlock()
	if !running || server == nil {
		return nil, errors.New("tailscale outbound is not running")
	}
	return server.Dial(ctx, network, address)
}

func (t *tailscaleOutbound) protectFunc() func(int) error {
	if t.protect == nil {
		return nil
	}
	return func(fd int) error {
		if t.protect(fd) {
			return nil
		}
		return errors.New("socket protection failed")
	}
}

func sanitizeTailscaleError(err error, authKey string) error {
	if err == nil || authKey == "" {
		return err
	}
	return fmt.Errorf("%s", strings.ReplaceAll(err.Error(), authKey, "[redacted]"))
}

var _ OutboundAdapter = (*tailscaleOutbound)(nil)
var _ interface {
	DialContext(context.Context, string, string) (net.Conn, error)
} = (*tailscaleOutbound)(nil)
