# WireGuard / HTTPS / Tailscale test matrix

## Environment constraints

- Android SDK is unavailable; APK builds, Gradle tests, Robolectric execution, and on-device verification were not run in this workspace.
- Toolchain: Go 1.25 via `mise exec go --` in `tunnel/`.
- `internal/dns.TestDoTErrors` and `internal/dns.TestDoQErrors` fail in this environment because names intended to be unresolvable receive wildcard/private DNS results. The same failures are present in the pre-change baseline and are treated as environment-only exclusions; upstream CI remains the authority.
- No real WireGuard peer or Android device is available.

## Automated Go verification

| Check | Command | Result | Notes |
|---|---|---|---|
| Build | `go build ./...` | Pass | Run with Go 1.25/mise |
| Vet | `go vet ./...` | Pass | Run with Go 1.25/mise |
| Tests | `go test ./...` | Pass except baseline DNS tests | Only `TestDoTErrors` and `TestDoQErrors` fail; both match the documented environment issue |

## Unit and integration coverage

| Area | Evidence | Expected result | Status |
|---|---|---|---|
| WireGuard TCP stream plumbing | `internal/wireguard/stream_test.go` | TCP dials a listener through private netstack | Confirmed |
| WireGuard UDP stream dial | `internal/wireguard/stream_test.go` | UDP returns a connected stream-transport socket | Confirmed |
| IP-literal guard | stream tests | Hostnames are rejected before start | Confirmed |
| Stream support discovery | engine lifecycle test | Stream adapter reports `SupportsStreams`; packet adapter does not | Confirmed |
| MITM fallback semantics | `internal/mitm` tests | `ErrNoOutbound` falls back; real stream errors fail | Confirmed |
| Legacy WireGuard routing | `TestStartRoutesNonDNSToAdapter` | Non-DNS packets reach router adapter | Confirmed |
| Invalid WireGuard config | `TestStartWireGuardConfig` | Engine fails closed | Confirmed |
| Direct DNS-only mode | lifecycle tests | Existing DNS behavior retained | Confirmed |
| Direct TCP/UDP stack | lifecycle/full-tunnel tests | Existing relay behavior retained | Confirmed |
| MITM eligibility | MITM/full-tunnel tests | Existing UID/site/pass-through gates retained | Confirmed |
| Kotlin startup gating | adapter/coordinator tests | Flag off keeps WG MITM off; flag on starts WG + MITM | Source-level tests added; Gradle not runnable |
| Preference default | preferences/source tests | Experimental flag defaults false | Confirmed |

## Phase A acceptance matrix

| Scenario | Expected | Result |
|---|---|---|
| Direct + DNS | Existing DNS filtering | Go path confirmed |
| Direct + DNS + HTTPS | Existing `StartFull` + MITM | Go path confirmed |
| WireGuard + DNS | Existing WireGuard adapter | Go path confirmed |
| WG + DNS + HTTPS, flag off | Legacy behavior; MITM off | Unit coverage added; Android run unverified |
| WG + DNS + HTTPS, flag on | MITM upstream TCP via WG stream transport | Local transport tests only |
| Non-DNS TCP via WG in coexistence | All non-DNS TCP uses WireGuard egress | Not implemented/verified |
| Non-DNS UDP via WG in coexistence | UDP uses WireGuard egress | Not implemented/verified |
| DNS block while WG active | BlockAds handles DNS first | Local path confirmed |
| Invalid WG config | Engine stops, no direct fallback | Automated test confirmed |
| MITM stream dial failure | Request fails; no direct fallback | Unit semantics confirmed |
| Certificate passthrough | Existing bypass retained | Source inspection only |

## Android and real-network tests still required

| Test | Method | Acceptance |
|---|---|---|
| APK build | `./gradlew assembleDebug` with local tunnel AAR | Build succeeds |
| Kotlin unit suite | `./gradlew testDebugUnitTest` | No regressions |
| Real WG handshake | Device + known peer | Handshake and data without direct leak |
| Filtered HTTPS via WG | Installed CA + selected browser + peer egress check | Response filtered and egress IP is peer |
| HTTPS passthrough | Pinned app/site | No interception or pin failure |
| DNS filtering | Allowed, blocked, custom, and split-DNS domains | BlockAds result and no DNS leak |
| UDP/QUIC | QUIC allowed and HTTP/3-filtering modes | Existing policy preserved; WG UDP limitation visible |
| IPv4/IPv6 | Dual-stack peer and sites | No family fallback leak |
| MTU/fragmentation | 1280/1350 MTU and large transfers | Stable throughput |
| Network roam | Wi-Fi/mobile switch | Stops/reconnects without duplicate sessions |
| Always-on/lockdown | OS settings | No direct fallback while remote route is selected |
| Battery/memory | Browser load benchmark | No unacceptable regression |

## Phase B Tailscale matrix

| Test | Expected | Status |
|---|---|---|
| Dependency/build spike | `tsnet` builds behind isolated flag | Not run |
| Login/auth flow | Official browser-based auth; no token logging | Not run |
| Private state storage | App-private storage only | Not run |
| Tailnet TCP dial | One service reachable through tailnet | Not run |
| Subnet Router TCP | Private LAN address reachable | Not run |
| Subnet Router UDP | Private LAN UDP service reachable | Not run |
| DNS coexistence | BlockAds DNS remains authoritative | Not run |
| MagicDNS/Split DNS | No loop or leak | Not run |
| HTTPS + Tailscale | MITM policy unchanged | Not run |
| Logout/expiry/roam | Safe stop/reconnect | Not run |
