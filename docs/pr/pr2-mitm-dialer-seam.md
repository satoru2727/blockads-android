# PR 2 — refactor(tunnel): route MITM upstreams through the engine dialer

**Branch:** `pr/2-mitm-dialer-seam` (stacked on PR 1)
**Size:** 10 files, +426 / −29

## What it does

The HTTPS-filtering handlers used to dial upstream sockets themselves. They now go through a single `Engine.DialUpstream(ip, port)` seam that:

1. refuses to dial the proxy server itself,
2. prefers the active `OutboundAdapter` that advertises `DialContext`, gated by the adapter's optional `Matches(addr)` allowlist,
3. falls back to the direct/proxy/Shadowsocks clients, and
4. returns `mitm.ErrNoOutbound` so callers can fall back to the protected direct dialer without treating "no adapter" as an error.

Behaviour is unchanged when no adapter implements `DialContext`: the fallback chain ends at the same direct dialer as before.

## Why it is a separate PR

It is a pure seam refactor with no user-visible change, so it can be reviewed and merged independently of the feature flag in PR 3. It also makes the "which egress did this connection use" question answerable in one place.

## Verification

- `go build ./...`, `go vet ./...`, `go test ./...` pass.
- Existing relay/handler tests updated for the injected dialer; no assertions relaxed.
- Not verified: on-device MITM flows (no device).

## Review focus

1. `ErrNoOutbound` semantics — make sure every caller keeps its previous fallback behaviour.
2. Whether `Matches` belongs on the adapter or should be a router-level policy.
3. Log lines must not include decrypted content (they only log IP:port and adapter name).
