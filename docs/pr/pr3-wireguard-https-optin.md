# PR 3 — feat(app): opt-in WireGuard + HTTPS filtering coexistence

**Branch:** `pr/3-wireguard-https-optin` (stacked on PR 1 and PR 2)
**Size:** 33 files, +1243 / −76 (docs included)

## What it does

Until now `GoTunnelAdapter.start()` silently disabled the userspace MITM stack whenever a WireGuard config was present:

```kotlin
val mitmMode = httpsFilteringEnabled && certDir.isNotEmpty() && wgConfigJson.isEmpty()
```

and the engine picked one of two engines (`start()` for WireGuard, `startFull()` for direct + MITM). This PR removes that implicit exclusivity behind an **experimental, default-off** setting:

- New preference `wireguard_https_coexistence` (default `false`); the old behaviour is bit-for-bit preserved when it is off.
- When on, the engine starts the full-tunnel stack (DNS filtering, per-app firewall, MITM for selected browsers) and registers the WireGuard adapter from PR 1, so eligible HTTPS upstream TCP leaves through the WireGuard peer while everything else keeps the existing path.
- The UI shows the state and a one-line explanation that this is experimental.
- Settings backup/restore includes the flag.

## What it deliberately does not do

- No change to CA handling, no TLS verification bypass, no certificate pinning defeat: pinned or unsupported connections still fall back to the existing bypass path.
- No general non-DNS UDP or ICMP forwarding through WireGuard yet — that needs a stateful design (timeouts, fragmentation, IPv6) and is tracked as follow-up.
- No telemetry.

## Verification

- Go: `go build`, `go vet`, `go test` pass (tagless and `-tags tailscale`).
- Android, verified locally with JDK 21 / SDK 36 / NDK r27c and a locally rebuilt `tunnel.aar`: `:app:compileFdroidDebugKotlin` and `:app:assembleFdroidDebug` succeed; `:app:testFdroidDebugUnitTest` runs 865 tests with 5 failures, all pre-existing and unrelated (`DnsStampParserTest` calls `android.util.Base64` under the mockable jar).
- New tests: engine-selection matrix (WireGuard only / HTTPS only / both, flag on and off), settings round-trip, fixture updates for the new preference.
- Not verified: real device, real WireGuard handshake, real browser HTTPS egress, battery/memory impact.

## Review focus

1. Is an opt-in flag the right shape, or would the maintainers prefer this to always be on once proven?
2. The engine-selection condition — any hidden assumption that WireGuard mode owns the TUN exclusively?
3. Which follow-ups should be in scope (UDP/QUIC through WireGuard, always-on VPN interplay, MTU)?
