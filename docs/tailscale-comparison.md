# Phase B: Tailscale integration comparison

## Scope and evidence

This document compares four Phase B paths for BlockAds. It is a design comparison only: no Tailscale dependency or PoC is included in Phase A.

Evidence was collected on 2026-10-23 from the current `main` branch of `tailscale/tailscale`, the current `main` branch of `tailscale/tailscale-android`, and the sing-box `testing` branch (default branch `testing`). Live upstream code moves quickly; pin an exact version and re-run the security/size review before implementation.

Primary sources:

- Tailscale tsnet docs and API reference: `https://tailscale.com/docs/features/tsnet` and `https://tailscale.com/docs/reference/tsnet-server-api`
- Current tsnet source: `https://github.com/tailscale/tailscale/blob/main/tsnet/tsnet.go`
- Tailscale userspace routing semantics: `https://tailscale.com/docs/reference/kernel-vs-userspace-routers`
- Tailscale Subnet Router, MagicDNS, and Exit Node docs: `https://tailscale.com/docs/features/subnet-routers`, `https://tailscale.com/docs/features/magicdns`, and `https://tailscale.com/docs/features/exit-nodes`
- Official Android integration: `https://github.com/tailscale/tailscale-android/tree/main/libtailscale`
- sing-box endpoint docs and implementation: `https://sing-box.sagernet.org/configuration/endpoint/tailscale/` and `https://github.com/SagerNet/sing-box/blob/testing/protocol/tailscale/endpoint.go`
- Licenses inspected from repository `LICENSE` files / GitHub license metadata.

## Recommendation

Use **`tailscale.com/tsnet`** for the first BlockAds PoC, but do not describe it as a generic Layer-3 router.

The smallest useful increment is:

1. Add `tsnet.Server` behind a default-off experimental build/runtime flag.
2. Route **one explicitly selected Tailnet TCP destination** through `Server.Dial`.
3. Keep all Android TUN, DNS interception, and general IP packet handling in BlockAds.
4. Supply an app-private `ipn.StateStore`; do not use a world-readable default state directory.
5. Keep normal internet traffic direct and disable Tailscale DNS acceptance in the first PoC.
6. Add connected-UDP forwarding and route acceptance only after TCP succeeds.

This fits Phase A's `DialContext`-style outbound seam. `tsnet` is also the highest-level public API and keeps Tailscale responsible for control, authentication, key rotation, peers, DERP, and WireGuard transport. Its cost is a material Go dependency and an application-specific bridge for generic Android flows.

Move down to `wgengine` only if a TCP-first `tsnet` PoC proves that BlockAds needs lower-level route/packet control that `tsnet` cannot expose safely. Do not import sing-box merely to obtain Tailscale: its endpoint is useful reference code, but the dependency duplicates BlockAds' routing/DNS engine and carries a large GPL-v3-or-later and custom-name restrictions surface.

### Phase B PoC result (2026-10-24)

The TCP-first PoC confirms that `tsnet.Server.Dial` fits the existing
`DialContext` outbound seam and can be isolated behind the `tailscale` build
tag. The implementation uses `tailscale.com v1.92.5` because `v1.104.2`
requires a newer gVisor API that is incompatible with the repository's
`tun2socks/v2 v2.6.0`; the tagless build remains the priority. A Linux,
unstripped Go test binary measured 27,681,494 bytes without the tag and
42,295,463 bytes with it, a 14,613,969-byte (+52.8%) increase. The PoC did
not verify real login, Tailnet TCP, Subnet Router, MagicDNS, UDP forwarding,
or Android hardware behavior. See `docs/tailscale-poc.md` for commands,
results, and the next-stage gates.

## What `tsnet` actually provides

Source-backed facts:

- `tsnet.Server` is the public embedded node type. Its exported settings include `Dir`, `Store`, `Hostname`, `Ephemeral`, `AuthKey`, `ControlURL`, `Port`, `AdvertiseTags`, and a custom `tun.Device`.
- `Server.Dial(ctx, "tcp", address)` returns a Go `net.Conn` and implicitly starts the node. It uses Tailscale's userspace stack and can resolve MagicDNS names.
- `Server.Listen("tcp", addr)` accepts incoming Tailnet TCP flows. `Server.ListenPacket("udp", ip:port)` accepts UDP, but the official API requires an explicit local Tailscale IP and port.
- Internally, tsnet constructs Tailscale `wgengine` plus `wgengine/netstack`; its TCP/UDP dial callbacks call gVisor `DialContextTCPWithBind` and `DialContextUDPWithBind`.
- Authentication is either an auth key/OAuth/workload credential or an interactive login URL surfaced through the configured logger/state flow. BlockAds must show that URL without logging unrelated secrets or decrypted traffic.
- By default, state is a file below `Dir`; `Store` allows an app-backed `ipn.StateStore`.

Critical limitation: `Dial`, `Listen`, and `ListenPacket` are flow APIs. They do **not** make BlockAds' Android TUN disappear into a Tailscale interface and do not automatically forward arbitrary IP packets. BlockAds still needs route selection, stateful UDP mapping, ICMP policy, address-family policy, and Android `protect()` recursion control.

## Option 1: Tailscale `tsnet`

### Integration shape

Wrap `tsnet.Server` in an internal Go outbound adapter implementing the existing stream-dial interface. Keep the concrete Tailscale type unexported and behind a build tag so no new gomobile API or mandatory dependency is introduced.

For authentication, persist state through an app-supplied `ipn.StateStore` backed by Android app-private encrypted storage. Expose only non-secret status and an actionable login URL to Kotlin. Never copy auth keys into logs or analytics.

### Capabilities

- **TCP:** strongest first step; `Server.Dial` directly matches the MITM upstream seam.
- **UDP:** source supports connected UDP dialing and `ListenPacket`, but BlockAds must add a stateful destination mapping and idle timeout. The first PoC should not claim arbitrary UDP forwarding.
- **ICMP:** no generic `net.Conn` ICMP dial API. Ping would need separate handling and remains out of scope.
- **Subnet Router:** TCP/UDP to accepted subnet routes is possible after implementing route acceptance and fail-closed routing. Userspace Tailscale reconstructs TCP/UDP rather than preserving Layer-3 packets.
- **MagicDNS/Split DNS:** `Server.Dial` can use MagicDNS names, but BlockAds DNS must remain authoritative. Split DNS needs explicit policy to avoid loops and leaks; do not blindly apply Tailscale DNS configuration in the PoC.
- **Exit Node:** possible later through Tailscale control preferences, but not appropriate until all-egress routing, UDP, IPv6, and fail-closed behavior are solved. Do not start here.
- **Android:** pure Go userspace operation avoids a second `VpnService`; transport sockets still require BlockAds/Android `protect()` and lifecycle wiring. It is not a drop-in Android client replacement.

### Cost and risk

- **API stability:** best of the Tailscale options: documented public `tsnet.Server` API, although the module evolves and must be pinned.
- **Binary size:** material and unmeasured; likely dominated by control plane, crypto, DERP, and gVisor/Tailscale code. Measure per-ABI AAR/APK delta before merging.
- **License:** Tailscale repository reports BSD-3-Clause. Verify the pinned module and transitive notices.
- **Update burden:** moderate: version pinning, control-protocol changes, state migration, Android lifecycle, and security advisories.

## Option 2: Tailscale `wgengine` / userspace networking

### Integration shape

Assemble `wgengine.NewUserspaceEngine`, `ipnlocal.LocalBackend`, `wgengine/netstack`, router/DNS configurators, and Android socket protection directly, similar to the official Android `libtailscale` backend. BlockAds would provide its own TUN or private netstack and feed selected flows/packets into that engine.

### Capabilities

- **TCP/UDP:** source exposes userspace netstack TCP/UDP dial functions; official Android uses `wgengine` and `netstack` for a full client.
- **ICMP:** lower-level packet access makes more possible, but Android userspace behavior must still be explicitly designed. Tailscale's userspace router documentation supports reconstructed ping, not arbitrary ICMP.
- **Subnet Router/Exit Node:** closest surface for accepting routes and implementing all-egress policy, but BlockAds must preserve the semantics of Tailscale's local backend and Android router.
- **MagicDNS/Split DNS:** access to Tailscale DNS manager/state, at the cost of integrating it without breaking BlockAds DNS and leak prevention.
- **Authentication/key state:** handled by Tailscale libraries, but BlockAds must implement the same secure state-store and UI integration.

### Cost and risk

- **API stability:** lower than `tsnet`: `wgengine`, `tsd.System`, `netstack.Impl`, and local backend wiring are internal-leaning integration surfaces even where exported.
- **Android suitability:** proven by Tailscale Android, but the official app also has substantial VpnService, multi-TUN, protect/bind-to-network, notification, policy, and storage glue.
- **Binary size:** similar or larger than `tsnet` once the required control/local-backend subsystems are linked.
- **License:** BSD-3-Clause in the inspected repository.
- **Update burden:** high: small upstream refactor can force BlockAds changes and security re-review.

Assessment: retain as a later option for generic route handling, not the first small upstream contribution.

## Option 3: sing-box Tailscale Endpoint

### Integration shape

sing-box exposes a configured Tailscale endpoint with state directory, auth key, hostname, ephemeral mode, accepted routes, exit node, advertised routes/tags, system TUN mode, UDP timeout, and interactive authentication. Its implementation is a large integration layer over a sagernet-maintained Tailscale fork and exposes TCP, UDP, and ICMP adapter networks.

### Capabilities

- **TCP/UDP:** implementation has flow dialers and packet listeners through its userspace stack.
- **ICMP:** advertises ICMP in its adapter network set, but BlockAds would still need Android packet semantics and acceptance tests; do not infer complete transparent ICMP support from the type list.
- **Subnet Router/Exit Node:** configuration already includes `accept_routes`, `advertise_routes`, and exit-node selection.
- **MagicDNS/Split DNS:** endpoint participates in sing-box DNS routing and exposes preferred-domain/address logic; adopting it would conflict/duplicate BlockAds DNS policy.
- **Authentication/key state:** supports auth key and interactive URL, with its own state directory and Android app integration.

### Cost and risk

- **API stability:** stable as a sing-box configuration/endpoint, but coupling BlockAds to sing-box release cadence and a sagernet Tailscale fork is substantial.
- **Android suitability:** sing-box is Android-capable, but the endpoint's system-interface mode conflicts with BlockAds owning the sole VpnService. Reusing only its internal endpoint code is not a stable API promise.
- **Binary size:** expected largest; it imports a complete proxy/routing platform, Tailscale fork, TUN stack, DNS machinery, and auxiliary features.
- **License:** inspected sing-box license is GPL-v3-or-later plus a name/association restriction. This is not legal advice, but it makes extraction or static reuse materially different from the BSD Tailscale path.
- **Update burden:** high and includes two upstream projects/forks.

Assessment: excellent reference implementation, poor first dependency for a small BlockAds upstream PR.

## Option 4: standard WireGuard instead of Tailscale

This is already available for ordinary WireGuard profiles, but it is **not Tailscale integration**. Standard WireGuard configuration does not reproduce Tailscale identity, coordination, ACLs, key rotation/device approval, DERP relay policy, MagicDNS, or authenticated Subnet Router participation.

It remains the best answer when a user controls both endpoints and only needs a static encrypted route. It should not be labeled as Tailscale support or used to claim feature parity.

## Comparison matrix

| Axis | `tsnet` | `wgengine` | sing-box endpoint | Standard WireGuard |
|---|---|---|---|---|
| API stability | High, documented public package | Medium/low, internal-leaning subsystem APIs | Stable config surface; internal fork coupling high | High for existing BlockAds path |
| Android suitability | Good for in-process userspace flow bridge; custom protect/lifecycle | Proven but needs substantial official-style glue | Proven platform, but conflicts/duplicates BlockAds engine | Already integrated |
| TCP | Strong via `Dial` | Strong via userspace stack | Strong in endpoint | Existing |
| UDP | Supported as flows; bridge required | Supported as flows/packets; bridge required | Supported with endpoint UDP timeout | Existing |
| ICMP | No generic Dial API; ping only as future special case | More possible, still explicit design required | Advertised but unverified for BlockAds | Existing |
| Subnet Router | After route acceptance + TCP/UDP bridge | Stronger route-level control | Configuration already present | Static routes only |
| MagicDNS / Split DNS | Names usable; BlockAds integration required | Direct manager access, high integration cost | Present but duplicates/conflicts with BlockAds DNS | Not Tailscale |
| Exit Node | Future all-egress phase | Future all-egress phase | Present in dependency | Not equivalent |
| Authentication | Auth key or interactive URL; public API | Same primitives, more wiring | Built into endpoint | Static private key |
| Key/state storage | `ipn.StateStore`; use app-private encrypted store | Same; implement store glue | Endpoint state directory; Android handling needed | Existing profile path |
| Binary size | Medium/unmeasured | Medium-large/unmeasured | Large/unmeasured | None |
| License | BSD-3-Clause (pinned-version review required) | BSD-3-Clause (pinned-version review required) | GPL-v3-or-later + name restriction | Existing |
| Update burden | Moderate | High | High, includes fork | Low |

## Milestones

### B0: dependency and security spike (no user feature)

- Pin Tailscale and build an unused, tag-isolated adapter.
- Record AAR/APK size across all Android ABIs.
- Audit transitive licenses and telemetry/logging defaults.
- Implement no-op lifecycle tests.

### B1: authenticated TCP PoC

- Login through the official interactive URL or a user-entered auth key.
- Store all state in Android app-private encrypted storage.
- Dial one Tailnet TCP service through the outbound seam.
- Keep internet traffic and DNS direct/BlockAds-controlled.
- Verify transport sockets are protected from recursive VPN capture.

### B2: Subnet Router TCP

- Enable only explicitly configured accepted routes.
- Resolve destination IPs through BlockAds policy.
- Fail closed when the Tailscale outbound is unavailable.
- Verify no DNS leak and no direct fallback.

### B3: UDP and route hardening

- Add connected-UDP mapping, source/destination identity, and idle timeout.
- Test QUIC, IPv4/IPv6, MTU/fragmentation, roaming, logout, and key expiry.
- Determine ping-only ICMP policy or explicitly reject ICMP.

### B4+: MagicDNS/Split DNS and Exit Node

- Integrate selected Tailscale DNS domains without losing ad blocking.
- Add leak/loop tests and explicit user-visible policy.
- Consider Exit Node only after all-egress routing and kill-switch behavior are stable.

## Decision gates

- No Tailscale dependency enters `main` without maintainer approval and a measured size budget.
- No generic forwarding claim until B2/B3 pass real-device tests.
- No Tailscale DNS application until DNS loop/leak tests pass.
- No Exit Node until fail-closed and IPv6 leak tests pass.
- No logs may contain private keys, auth credentials, auth-key values, or decrypted request/response content.

## Source notes

- The Tailscale userspace router documentation states that userspace mode reconstructs TCP/UDP, supports only reconstructed ICMP ping, and does not support other IP protocols.
- Official tsnet docs state that tsnet uses a userspace TCP/IP stack and that `Dial`/`Listen` implicitly start the server.
- Official Tailscale Android code uses `wgengine.NewUserspaceEngine`, `wgengine/netstack`, Android `VpnService.protect`/bind-to-network hooks, and an encrypted Android state store. It is evidence that lower-level integration works, not that it is small.
- sing-box's endpoint code uses a sagernet-maintained Tailscale fork and implements TCP/UDP flow/packet dialing plus route/DNS integration. It is a useful blueprint for later milestones, not a low-cost dependency.
