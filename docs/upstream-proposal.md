# Upstream proposal: WireGuard + HTTPS filtering coexistence

## Summary

BlockAds previously made WireGuard and full HTTPS filtering effectively exclusive: the Android layer chose between the legacy WireGuard engine (`Start`) and the direct full-tunnel/MITM engine (`StartFull`). This branch keeps one Android VPN, moves the configured WireGuard device onto a private gVisor netstack, and sends MITM upstream TCP through that stream transport.

The feature remains behind a default-off experimental setting. It does not alter CA trust, disable TLS verification, or change the existing direct/WireGuard behavior when the setting is disabled.

The result is a deliberately narrow Phase A increment: eligible HTTPS upstream TCP can use WireGuard while DNS and existing packet handlers remain in BlockAds. General non-DNS UDP forwarding, ICMP, full route-level forwarding, and real-device behavior are follow-up work.

## Branch state

Base: `bdc9127fa598494b4370cfd4066de43370bfe8e8`

Commits (oldest first):

| Commit | Subject | PR candidate |
|---|---|---|
| `2491045` | `feat: add wireguard userspace stream transport` | PR 1 |
| `f728652` | `refactor: route mitm upstreams through engine dialer` | PR 2 |
| `9a9ddd5` | `feat: enable wireguard https coexistence` | PR 3 |
| `2fa5297` | `fix: advertise wireguard stream support` | PR 1 |
| `d717495` | `feat: gate wireguard https coexistence` | PR 3 |
| `0355fd7` | `docs: document wireguard https integration` | Documentation |
| `c836221` | `test: cover wireguard udp stream dial` | PR 1 |
| `08cde0d` | `docs: clarify wireguard udp test scope` | Documentation |

The branch intentionally contains eight Phase A commits, not the seven estimated during planning. Documentation-only commits can be retained locally, rebased into their implementation commits, or submitted separately according to maintainer preference.

## Maintainer discussion draft

> ### Proposal: share a WireGuard device with userspace HTTPS filtering
>
> BlockAds can already filter DNS and terminate HTTPS flows in userspace, but engine selection currently forces a choice between the WireGuard packet path and the direct full-tunnel path. I would like to discuss a smaller outbound seam:
>
> 1. Keep Android TUN processing in the existing userspace stack.
> 2. Build the existing WireGuard device against a private gVisor netstack rather than the Android TUN.
> 3. Let MITM upstream TCP use `Engine.DialUpstream`, which prefers the stream-capable active adapter.
> 4. Keep DNS interception, UID allowlists, private-address rules, passthrough, and proxy behavior unchanged.
> 5. Gate the Android behavior behind an experimental, default-off setting while real-peer coverage is completed.
>
> This does not propose moving all arbitrary IP traffic through the private netstack yet. Non-DNS UDP and general router forwarding need a separate stateful design so timeouts, ICMP, IPv6, and fragmentation are not silently changed.

## Suggested PR series

### 1. Add a private WireGuard stream transport

Purpose:

- Add a virtual gVisor netstack and `DialContext` for TCP/UDP IP-literal flows.
- Keep it internal to the tunnel module; expose no new gomobile type.
- Verify stream support and loopback TCP/UDP plumbing.
- Configure the WireGuard device on its private virtual TUN instead of Android TUN.

Review focus:

- Deadlock/lifecycle behavior around WireGuard's virtual TUN.
- Address-family handling and hostname rejection.
- Whether this should remain AmneziaWG-specific or be generalized.

Commits: `2491045`, `2fa5297`, `c836221`.

### 2. Route MITM upstreams through the engine

Purpose:

- Introduce a narrow upstream dial seam and `ErrNoOutbound`.
- Prefer an active stream adapter, then existing proxy clients.
- Fall back to a protected direct dial only when no outbound is configured.
- Preserve fail-closed behavior for real stream errors.

Review focus:

- Fail-closed semantics for real stream errors.
- Proxy-server loop prevention.
- IPv4/IPv6 fallback compatibility.

Commit: `f728652`.

### 3. Add default-off Android coexistence gating

Purpose:

- Persist an experimental preference with default `false`.
- Pass it through VPN startup.
- Preserve legacy WireGuard behavior when false.
- Select `Start` plus MITM/TCP stack when true.
- Keep the setting visible as experimental and reversible.

Review focus:

- Preference naming and UI copy.
- Restart behavior when the setting changes.
- Compatibility with proxy and Shadowsocks routing modes.

Commits: `9a9ddd5`, `d717495`.

### 4. Generalize UDP/IP forwarding

Purpose:

- Add a flow-level outbound interface for arbitrary UDP.
- Preserve idle timeout and addressing semantics.
- Route only explicitly selected remote-egress modes; avoid direct fallback.
- Define IPv4/IPv6 and fragmentation behavior before exposing a setting.

Review focus:

- DNS loop prevention and QUIC policy.
- ICMP/IPv6 behavior.
- Real-peer tests and MTU behavior.

### 5. Tailscale research spike

Purpose:

- Evaluate `tsnet` versus lower-level `wgengine` and a sing-box dependency using `docs/tailscale-comparison.md`.
- Start with one authenticated Tailnet TCP dial behind an isolated experimental flag.
- Document binary-size, state-storage, licensing, and maintenance costs.

Review focus:

- Whether maintainers want a Tailscale dependency at all.
- Minimum supported Tailscale version and upgrade policy.
- Authentication UX and private-state guarantees.

## Why not use `StartFull` for everything

`StartFull` owns the Android TUN with a direct gVisor stack. WireGuard's current adapter is a Layer-3 virtual-TUN device. Replacing WireGuard packet forwarding immediately would require translating every packet flow and preserving MTU, UDP, ICMP, route, and lifecycle semantics. A private stream transport limits Phase A to the already-terminated MITM upstream path and keeps the blast radius small.

## Known limitations to state in the PR

- Real-peer and on-device validation is not complete.
- General non-DNS TCP is not yet routed through the WireGuard stream adapter.
- General non-DNS UDP is not yet routed through the WireGuard stream adapter.
- The private adapter has connected-UDP dial plumbing, but no full-stack UDP handler integration or idle-timeout design yet.
- IPv6, MTU/fragmentation, roaming, always-on VPN, lockdown, battery, and memory need device measurements.
- Browser certificate-pinning compatibility is unchanged but not yet device-tested in this mode.
- The feature is intentionally experimental and default-off.
- No Tailscale dependency or PoC is included; Phase B is a comparison only.

## Security review checklist

- [ ] No `InsecureSkipVerify` or global trust-store weakening.
- [ ] No private keys, tokens, auth URLs with credentials, or decrypted bodies in logs.
- [ ] MITM remains limited to explicitly selected apps/sites.
- [ ] Certificate passthrough and private-address rules remain effective.
- [ ] Failed remote egress does not silently become direct egress.
- [ ] Android `protect()` prevents WireGuard/Tailscale transport recursion.
- [ ] Experimental state is reversible and disabled by default.
- [ ] Dependency licenses and binary-size impact are documented.

## Reproduction and verification

From `tunnel/`:

```sh
mise exec go -- go build ./...
mise exec go -- go vet ./...
mise exec go -- go test -count=1 ./...
```

Environment note: `tunnel/internal/dns.TestDoTErrors` and `tunnel/internal/dns.TestDoQErrors` fail in this workspace because the environment resolves intentionally unresolvable names through wildcard/private DNS. The errors match the pre-change baseline. Upstream CI remains authoritative.

Android verification still requires:

1. Build the local tunnel AAR and debug APK.
2. Install the BlockAds CA only in the intended test profile.
3. Connect to a controlled WireGuard peer.
4. Verify blocked/allowed DNS, filtered HTTPS, passthrough, egress IP, IPv4/IPv6, UDP/QUIC, MTU, roam, and always-on behavior.
