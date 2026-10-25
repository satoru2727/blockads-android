# PR 1 — feat(tunnel): wireguard userspace stream transport

**Branch:** `pr/1-wireguard-stream-transport` (base: `main` @ `bdc9127`)
**Size:** 5 files, +384 / −4

## What it does

Adds an internal, tagless stream transport for the existing WireGuard device so TCP flows can be dialled *through* WireGuard without touching the Android TUN:

- A private gVisor netstack + virtual TUN hosts the AmneziaWG/WireGuard device instead of the Android TUN fd.
- `DialContext(ctx, network, address)` opens TCP (and connected UDP) flows over that netstack.
- `SupportsStreams()` advertises stream capability on the WireGuard adapter so the engine's existing dial seam can pick it.
- Address-family handling is explicit: IP literals only, hostnames are rejected, IPv6 is supported.

No exported gomobile type is added. No existing routing, DNS, UID allowlist, passthrough, or proxy behaviour changes: when nobody dials, the device behaves exactly as before.

## Why it is safe

- WireGuard traffic is not re-read from the Android TUN, so there is no routing loop; the netstack dialer is the only new consumer.
- The virtual interface has no routes installed in the Android stack — it exists solely for the dialer.

## Verification

- `go build ./...`, `go vet ./...`, `go test ./...` pass (2 pre-existing DNS tests are excluded: they assume the host resolver rejects `*.invalid`, which a wildcard-DNS lab does not).
- New unit tests: stream dial over loopback TCP/UDP, address-family rejection, adapter stream advertisement.
- Not verified: real peer handshake and on-device behaviour (no Android device in the authoring environment).

## Review focus

1. Lifecycle/deadlock around the virtual TUN (`Close` ordering, goroutine leaks on stop).
2. Whether this should stay AmneziaWG-specific or be generalised to `wireguard-go`.
3. Any concern about the extra gVisor stack instance per device (memory).
