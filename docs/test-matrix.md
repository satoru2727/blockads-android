# WireGuard / HTTPS / Tailscale test matrix

## Environment constraints

- Android SDK is unavailable; APK builds, Gradle tests, Robolectric execution, and on-device verification were not run in this workspace.
- Toolchain: Go 1.27.0 via `mise exec go --` in `tunnel/` for the final Phase A/Phase B-documentation verification. The project's mise-selected toolchain changed during the work; preserve the toolchain required by the repository when this branch is reviewed upstream.
- `internal/dns.TestDoTErrors` and `internal/dns.TestDoQErrors` fail in this environment because names intended to be unresolvable receive wildcard/private DNS results. The same failures are present in the pre-change baseline and are treated as environment-only exclusions; upstream CI remains the authority.
- No real WireGuard peer or Android device is available.
- No Tailscale dependency or authenticated tailnet is available; Phase B is source/document comparison only.

## Automated Go verification

| Check | Command | Result | Notes |
|---|---|---|---|
| Build | `mise exec go -- go build ./...` | Pass | Go 1.27.0, uncached final run |
| Vet | `mise exec go -- go vet ./...` | Pass | Go 1.27.0, uncached final run |
| Tests | `mise exec go -- go test -count=1 ./...` | Pass except two documented environment tests | 16 packages pass after the final documentation commits; `tunnel/internal/dns` fails only `TestDoQErrors` and `TestDoTErrors` |

Final test failures (environment-only):

```text
--- FAIL: TestDoQErrors (5.00s)
    doq_test.go:224: unresolvable err = DoQ connection: DoQ dial: timeout: no recent network activity
--- FAIL: TestDoTErrors (0.00s)
    transport_test.go:291: unresolvable err = DoT dial: dial tcp 10.77.0.20:853: connect: connection refused
```

Both intentionally-unresolvable test names are answered by this workspace's wildcard/private DNS path. They match the pre-change baseline, so no code change is inferred from these two failures.
The final rerun was executed at branch HEAD `a65d197`; the failure text above is from that run.

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

Design comparison: `docs/tailscale-comparison.md`.

| Test | Expected | Status |
|---|---|---|
| Dependency/build spike | `tsnet` builds behind isolated flag | Verified locally: tagless dep graph has 0 tsnet packages; `go build -tags tailscale ./...` passes. Binary 4.2 MB → 18.9 MB (+14.7 MB) for the Go package |
| Config validation | JSON, absolute state dir, CIDR normalization | Verified by unit tests (`tailscale_config_test.go`) |
| CIDR split tunnel | Only allowlisted destinations use Tailscale | Verified by unit tests (`Matches` boundaries); engine seam routes through `DialUpstream` |
| Private state storage | App-private storage only | Verified: `EnsureStateDir` creates `0700` (unit test) |
| Login/auth flow | Official browser-based auth; no token logging | Not run — no Android device; auth-key path only, real login unverified |
| Tailnet TCP dial | One service reachable through tailnet | Not run — needs `TS_AUTHKEY` + `TS_TEST_TARGET` (test skips without them) |
| Subnet Router TCP | Private LAN address reachable | Not run |
| Subnet Router UDP | Private LAN UDP service reachable | Not run |
| DNS coexistence | BlockAds DNS remains authoritative | Not run |
| MagicDNS/Split DNS | No loop or leak | Not run |
| HTTPS + Tailscale | MITM policy unchanged | Not run |
| Logout/expiry/roam | Safe stop/reconnect | Not run |

## Android build verification (2026-10-25)

Local verification on an Ubuntu VM (JDK 21, Android SDK 36, NDK r27c, Go 1.25, `-Ptunnel.source=prebuilt` with a locally rebuilt `tunnel.aar`):

| Check | Command | Result |
|---|---|---|
| Kotlin compile | `:app:compileFdroidDebugKotlin` | Pass (found and fixed 1 real error: wrong `KeyboardOptions` import) |
| Unit tests | `:app:testFdroidDebugUnitTest` | 865 tests, 5 failures, 68 skipped — all 5 failures are pre-existing/environmental (see below) |
| APK packaging | `:app:assembleFdroidDebug` | Pass — `app-fdroid-universal-debug.apk` (49 MB) |
| Go tests (tagless) | `go test ./...` | Pass except `TestDoTErrors`/`TestDoQErrors` (lab DNS wildcard) |
| Go tests (tagged) | `go test -tags tailscale ./...` | Same result; Tailscale unit tests pass, real-tailnet test skips |

Known pre-existing failures in this environment (not caused by these changes, they fail without any local diff):

| Test | Cause |
|---|---|
| `DnsStampParserTest` (4 cases) | Plain JVM test calls `android.util.Base64` because `Build.VERSION.SDK_INT == 0` in the mockable jar |
| `DnsProviderViewModelTest` (1 case) | Same stamp-parsing dependency |

Budget note for builders: Gradle (`-Xmx900m`), Kotlin (`-Xmx700m`), and the test JVM (`maxHeapSize = 768m`) fit a 4 GB machine only when `/tmp` is not a nearly full tmpfs; a 4 GB swap file avoids host-level OOM kills.
