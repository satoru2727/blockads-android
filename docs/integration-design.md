# Integration design and Tailscale comparison

## Goals

Phase A enables one Android `VpnService` to keep DNS filtering active while selected HTTPS flows are intercepted and their upstream TCP connections traverse WireGuard. Phase B evaluates how the same outbound abstraction can later support Tailscale without adding a second VPN.

## Recommended Phase A architecture

```text
Android apps
  |
VpnService TUN
  |
Userspace packet/TCP processing
  |- DNS filtering
  |- HTTPS MITM (opt-in UIDs/sites)
  |- Passthrough / non-eligible flows
  |
Outbound abstraction
  |- Direct (protected sockets)
  |- WireGuard packet adapter (legacy)
  |- WireGuard private stream adapter (Phase A)
  `- Future Tailscale outbound
```

The design keeps the existing Android TUN processing intact and adds a second, private netstack between the MITM dialer and WireGuard. This is deliberately smaller than replacing the current full-tunnel or legacy WireGuard forwarding model.

### Why this split

- The Android TUN cannot safely be owned by both `DnsInterceptor` and `StartFull` at the same time.
- MITM already works on terminated TCP flows, so it does not need Layer-3 packets for its own upstream.
- WireGuard accepts a virtual TUN, so a private gVisor stack can present `net.Conn` while the same WireGuard device supplies encryption.
- DNS can remain in BlockAds, preventing the app's DNS filtering from disappearing into a remote WireGuard resolver.
- Existing direct and WireGuard behavior can remain behind the same engine APIs, reducing gomobile churn.

### Startup state machine

1. Resolve routing mode and effective WireGuard JSON.
2. Read the default-off coexistence preference.
3. Direct mode:
   - HTTPS enabled -> TCP stack + MITM + `StartFull`.
   - HTTPS disabled -> existing `StartFull` passthrough.
4. WireGuard mode:
   - Flag off -> legacy packet adapter and MITM off.
   - Flag on -> initialize MITM, enable TCP stack, create stream WireGuard adapter, call `Start`.
5. Proxy/Shadowsocks modes retain their existing engine dialer precedence unless a stream-capable router adapter is active.

Failure behavior:

- Invalid or failed WireGuard startup stops the engine and does not silently leak traffic to direct.
- Stream dial failures fail the requesting MITM connection; they do not fall back to direct unless no stream outbound exists (`ErrNoOutbound`).
- MITM initialization failure is logged and MITM is not treated as active; startup must still fail closed for invalid WireGuard configuration.

## Data-flow decisions

| Flow | Phase A behavior | Rationale |
|---|---|---|
| DNS | Intercepted by BlockAds before outbound selection | Preserves ad blocking and avoids recursive VPN DNS |
| Eligible HTTPS TCP | MITM terminates flow; upstream uses WireGuard stream adapter | Combines filtering with encrypted remote egress |
| Passthrough TCP | Existing full-stack handler policy | Minimal behavioral change and preserves pinned/ineligible flows |
| Non-DNS UDP | Existing direct/relay or drop policy | Needs a stateful router-level mapping before safe WireGuard migration |
| WireGuard transport UDP | Android-protected physical socket | Prevents tunnel recursion |

### Required next hardening

1. Add a UDP `OutboundDialer` path to the full-stack UDP handler.
2. Preserve source/destination identity and idle timeout when relaying through a stream socket.
3. Add a real-peer integration test that proves an encrypted WireGuard handshake and an HTTPS request exits with the peer's address.
4. Measure MTU, memory, CPU, battery, and browser load behavior.

## Tailscale integration options

Phase B is a design and dependency decision, not a completed integration. No Tailscale code or dependency was added on this branch. The detailed source-backed comparison lives in `docs/tailscale-comparison.md`; the summary below preserves the architectural rationale.

### Option 1: `tsnet`

`tsnet` runs a userspace Tailscale node in-process and exposes TCP listeners/dialers. It can use userspace networking and an egress proxy, while control-plane authentication and state remain Tailscale-managed.

Strengths:

- Highest-level and most stable public integration surface.
- Natural fit for `DialContext`-style outbound routing.
- Avoids a second Android VPN and leaves BlockAds as the packet filter.
- Tailscale maintains control protocol, key rotation, authentication, and peer state.

Weaknesses:

- It does not by itself provide a generic Layer-3 outbound for arbitrary Android packets.
- UDP, ICMP, Subnet Router behavior, and route acceptance require a flow-level bridge.
- State and key storage must still be mapped to private Android storage.
- Go dependency and binary size are material but more contained than re-implementing the control plane.

Assessment: best candidate for a TCP-first PoC.

### Option 2: `wgengine` / userspace networking

Tailscale's lower-level engine can operate with userspace networking and exposes richer control over routes, peers, and packet flow.

Strengths:

- More direct access to IP-level packets than `tsnet`.
- Potentially better for Subnet Router and future Exit Node handling.
- Can coexist with BlockAds' existing gVisor experience.

Weaknesses:

- More internal APIs and lifecycle invariants than `tsnet`.
- Higher upgrade and regression burden.
- Rebuilding correct Tailscale control, ACL, MagicDNS, and key behavior would duplicate upstream work.
- Android integration and process lifecycle still need custom adapters.

Assessment: possible after a `tsnet` TCP PoC; unsuitable as the first small upstream change.

### Option 3: sing-box Tailscale endpoint

sing-box has a Tailscale endpoint implementation that can be used as a reference or potentially a dependency.

Strengths:

This can demonstrate a practical userspace Tailscale integration and packet/flow bridge; reviewing it may shorten discovery work. It is also cross-platform and already packaged for Android by sing-box.

Weaknesses:

It adds a large networking dependency and a second engine abstraction, even if only the endpoint is reused.
- Duplicates proxy, routing, DNS, and tunnel functionality already present in BlockAds.
- Increases APK size, build complexity, security-review surface, license review, and upgrade cadence.
- Harder to split into the small upstream PR series requested by maintainers.

Assessment: use as source research first; do not adopt as a dependency without a maintainer discussion and a size/security budget.

### Option 4: standard WireGuard interoperability only

Some Tailnet transport uses WireGuard, but standard WireGuard configuration cannot reproduce Tailscale's identity, control plane, key rotation, ACLs, MagicDNS, or authenticated Subnet Router participation.

Assessment: viable only for a user's manually configured WireGuard network. It is not Tailscale integration and must not be presented as such.

## Tailscale comparison matrix

| Axis | `tsnet` | `wgengine` | sing-box endpoint | Manual WireGuard |
|---|---|---|---|---|
| API stability | High | Medium/low | Depends on sing-box | High |
| Smallest first PoC | TCP dial | IP/route bridge | Dependency-heavy | Existing |
| Arbitrary TCP | Strong | Strong | Strong | Existing |
| Arbitrary UDP | Needs bridge | Potentially strong | Strong | Existing |
| ICMP | Not via `DialContext` | Engine-level possible | Depends | Existing |
| Subnet Router | After route/control handling | After route handling | Present in dependency | Manual only |
| MagicDNS/Split DNS | Requires integration | Requires integration | Dependency behavior | Not Tailscale |
| Exit Node | Future | Future | Dependency behavior | Not equivalent |
| Auth/key lifecycle | Built in | Built in | Built in | Manual |
| Maintainability | Good initially | Higher | Upstream cadence | Best |
| Size impact | Medium | Medium/large | Large | None |

## Recommended Phase B increment

1. Add a private Go `tailscale` integration behind a default-off build flag/experimental setting.
2. Use `tsnet` for login and one TCP `DialContext` to a Tailnet address.
3. Store state only in app-private storage and never log auth URLs beyond the minimum user-facing flow.
4. Keep BlockAds DNS interception; do not proxy DNS through the tailnet during the first PoC.
5. Extend the same outbound abstraction so direct/WireGuard/Tailscale selection is testable without Android.
6. Add a loopback fake for lifecycle tests, then a real Tailnet integration test requiring externally supplied state.
7. Only after TCP works, add UDP/Subnet Router flow forwarding and route-control tests.
8. Treat HTTPS coexistence with Tailscale as a separate acceptance milestone.

## Security controls

- Experimental settings default off.
- No global TLS verification bypass.
- No secret, token, private key, URL query credential, or decrypted content logging.
- App-private state only; use Android Keystore/encrypted storage if Tailscale secrets require local wrapping.
- Fail closed for selected remote-egress modes; never silently replace a failed remote route with direct.
- Preserve certificate pinning/passthrough behavior.
- Explicitly test DNS leak, IPv6 leak, kill switch/lockdown, and tunnel recursion.

## Upstream PR split

1. **WireGuard stream transport** - private userspace netstack and tests.
2. **MITM outbound seam** - `DialUpstream`/`ErrNoOutbound` behavior and direct fallback tests.
3. **Experimental coexistence gating** - default-off preference and Android engine selection.
4. **UDP forwarding** - flow-level stream/adapter routing and real-peer tests.
5. **Tailscale research/PoC** - dependency and design proposal before any large integration.
