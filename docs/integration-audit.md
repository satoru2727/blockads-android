# WireGuard / DNS / HTTPS integration audit

This audit records the pre-change data paths and the evidence behind the Phase A implementation. It is scoped to local source inspection and tests; no Android device or real WireGuard peer was available.

## Scope and confidence labels

- **Confirmed** — local source inspection or automated tests establish the behavior.
- **Hypothesis** - source structure suggests the behavior, but a real network/device path has not established it.
- **Unverified** - requires an Android device, real peer, roam, or an external integration to establish.

## Baseline

- Branch: `feat/wireguard-https-coexistence`
- Base commit: `bdc9127fa598494b4370cfd4066de43370bfe8e8`
- Go toolchain: Go 1.25 via `mise exec go --`
- Android SDK is unavailable in this environment, so Gradle/APK and on-device checks are **unverified**.

## Android VPN boundary

### Confirmed

- `AdBlockVpnService` owns the single Android `VpnService`, constructs `GoTunnelAdapter`, and hands a TUN `ParcelFileDescriptor` to `VpnSessionController`.
- `VpnSessionController.start` prepares startup config, configures DNS/filter state, resolves WireGuard configuration, establishes the interface, and calls its `VpnSessionEngine.startTunnel`.
- `VpnEngineCoordinator.startTunnel` derives the effective WireGuard JSON from routing mode or the active proxy profile, then delegates to `GoTunnelAdapter.start`.
- `VpnTunnelBuilder.establish` configures one TUN, app allow/deny lists, MTU, virtual DNS addresses, and WireGuard interface addresses.
- `GoTunnelAdapter.start` wraps the Android `protect(fd)` callback in the gomobile-compatible Go `SocketProtector`. WireGuard's UDP bind is protected through `protectedBind`.
- `GoTunnelAdapter.stop` and `VpnSessionController.stop/restart/tearDownForRestart` stop the Go engine and release session resources.

### Root cause of mutual exclusion

Before this branch, `GoTunnelAdapter.start` selected `engine.startFull` only when `wgConfigJson.isEmpty()` was true. With a WireGuard profile it selected `engine.start`, and its MITM decision did not distinguish direct HTTPS mode from coexistence. UI paths also switched routing away from WireGuard when HTTPS was enabled.

The split was therefore not an Android TUN ownership conflict: both paths could attach to one TUN FD. The conflict was that `Start` used the packet interceptor while `StartFull` owned the TUN with a direct gVisor stack, and upstream MITM dialing had no way to use the WireGuard outbound.

## Legacy WireGuard and DNS flow

### Confirmed

1. Android establishes one TUN and calls `Engine.Start(fd, protector, wgConfigJSON)`.
2. `Engine.startWireGuard` parses/builds WireGuard IPC configuration.
3. Packet mode constructs `wireguard.NewWgOutbound(ChannelTUN, ipc, protectFn)` and registers it with `Router`.
4. `DnsInterceptor.Run` reads the TUN, handles DNS locally, and sends non-DNS IP packets to `Router.RoutePacket`.
5. `Router.RoutePacket` sends packets to the active adapter's `HandlePacket`; the ChannelTUN injects them into WireGuard.
6. WireGuard-protected UDP uses `protectedBind`, avoiding re-entry into the BlockAds VPN TUN.
7. Invalid WireGuard startup fails closed: the engine does not silently fall back to direct forwarding.
8. `TestStartWireGuardConfig`, `TestStartRoutesNonDNSToAdapter`, and lifecycle tests cover config selection, fail-closed behavior, packet routing, and cleanup.

### Sequence

```text
Android apps -> VpnService TUN -> DnsInterceptor
  DNS -> Engine resolver/filter path
  non-DNS IP -> Router -> WgOutbound -> protected UDP -> peer
```

## Direct full-tunnel and MITM flow

### Confirmed

1. Without WireGuard, `GoTunnelAdapter.start` calls `Engine.StartFull`.
2. `StartFull` duplicates the TUN FD, wraps it in `bufferedTun`, and gives it directly to `NewTcpIpStack`.
3. DNS UDP/TCP goes to `Engine.ServeDNS` through the virtual-DNS handlers.
4. TCP is terminated in userspace. Eligible browser flows enter the MITM handler; other flows relay through passthrough/direct handlers.
5. UDP `:443` can be dropped for selected UIDs when HTTP/3 filtering is enabled; other UDP uses protected direct flow relay.
6. `Engine.StartStackMitm` initializes CA/filter state before the stack starts.
7. Automated socketpair/TCP/UDP tests cover direct full-tunnel flow handling, DNS behavior, QUIC suppression, and cleanup.

### Sequence

```text
Android apps -> VpnService TUN -> gVisor stack
  DNS -> Engine.ServeDNS
  eligible TCP -> MITM filter -> upstream dialer
  other TCP -> protected direct relay
  UDP -> DNS / drop / protected direct relay
```

## MITM eligibility and upstream behavior

### Confirmed

- `internal/mitm` gates interception by MITM state, UID allowlist, DoT/virtual DNS/DoH address rules, private-IP policy, ports, SNI/Host, blocklists, and passthrough suffixes.
- TLS trust is not globally disabled; upstream trust uses the bundled/system root pool.
- `mitm.dialUpstream` first asks the engine's `DialUpstream`; only `mitm.ErrNoOutbound` falls back to protected direct dialing. Other stream errors fail the connection.
- `Engine.DialUpstream` avoids loops to an active proxy server, tries a stream-capable active router adapter, then proxy/Shadowsocks clients, and otherwise returns `mitm.ErrNoOutbound`.
- Handler/relay unit tests cover fallback semantics and IPv4/IPv6 dial behavior.

### Hypothesis

- Existing certificate passthrough, private-address bypass, and UID gating preserve the intended security posture when the upstream is WireGuard rather than direct. Source inspection supports this, but browser pinned-TLS behavior needs on-device confirmation.

### Unverified

- Real-browser behavior with user/system CA stores, all TLS versions, HTTP/2, pinned applications, and decrypted-content filtering performance.

## Phase A stream transport

### Confirmed

- `wireguard.NewStreamOutbound` creates a private gVisor netstack attached to the same WireGuard device. Its traffic does not traverse the Android TUN.
- `WgOutbound.DialContext` supports TCP and UDP IP-literal dials, rejects hostnames to avoid recursive VPN/DNS use, and enforces address-family requests.
- `SupportsStreams` is true only for the stream adapter. This prevents the legacy packet adapter from being mistaken for a stream-capable outbound.
- `Engine.startWireGuard(..., streamMode=true)` selects the private stream adapter while the Android TUN continues through the parallel userspace stack/DNS interceptor.
- `Engine.DialUpstream` dials TCP through that stream adapter before falling back to proxy/direct outbound selection.
- Local loopback tests verify TCP transport, UDP dial construction, startup state, address parsing, and stream support. Engine tests verify stream mode adapter selection and cleanup.

### Phase A sequence

```text
Android apps -> VpnService TUN -> userspace TCP/IP stack
  DNS -> BlockAds DNS filter
  eligible TCP -> MITM -> Engine.DialUpstream
      -> private gVisor netstack -> WireGuard -> protected UDP -> peer
  non-eligible TCP/UDP -> current full-stack handler policy
```

### Unverified

- A handshake and filtered HTTPS request through a real remote WireGuard peer.
- End-to-end DNS + HTTPS filtering on an Android device.
- Roaming, always-on/lockdown, battery/memory under browser load, pinned TLS, and IPv6 to real peers.
- Whether every non-DNS UDP flow can use the stream adapter without changing tun2socks flow semantics; current direct mode's UDP handlers remain the PoC limitation.

## Routing and limitations

### Confirmed

- The experimental preference defaults to false and is now read for every VPN start.
- Legacy behavior is preserved: WireGuard + HTTPS with the flag off starts WireGuard without MITM.
- With the flag on, `engine.startStackMitm`, `setUseTcpStack(true)`, and `engine.start` are used together.
- Direct, DNS-only, WireGuard-only, direct+HTTPS, proxy, and Shadowsocks startup paths remain represented by existing unit tests.

### Hypothesis

- Keeping non-DNS traffic on the established direct handlers while routing MITM upstream TCP through WireGuard is a safer first increment than replacing all packet forwarding at once.

### Unverified

- Full non-DNS TCP/UDP egress through WireGuard in the coexistence configuration. Until a router-level UDP stream policy is implemented and tested, this PoC is TCP/MITM-upstream coexistence rather than a complete all-protocol WireGuard forwarder.

## Security observations

- No `InsecureSkipVerify` was added.
- WireGuard private keys and peer endpoints are passed as configuration, not logged by the new code.
- MITM logs destinations and errors, not plaintext bodies or decrypted content.
- The feature is default-off and does not alter certificate trust or passthrough policy.
- The Android `protect()` path remains required to prevent encrypted WireGuard UDP transport from looping back through the BlockAds TUN; real-device behavior is **unverified**.
