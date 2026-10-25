# Phase B: Tailscale `tsnet` PoC

Date: 2026-10-24

## Scope

This PoC adds an experimental, TCP-first Tailscale outbound without replacing
BlockAds' Android TUN, DNS filtering, or full-network gVisor stack. The
integration is disabled unless the `tailscale` Go build tag is enabled and a
Tailscale configuration is supplied through `Engine.SetTailscaleConfig`.

The implementation uses `tailscale.com/tsnet` pinned to `v1.92.5`. The newer
`v1.104.2` release was evaluated, but it requires a newer gVisor API that is
incompatible with the repository's `tun2socks/v2 v2.6.0` dependency. Keeping
the existing gVisor replace and the tagless build working is the current
priority; the pinned version must be revisited before production use.

## Implementation

- `TailscaleConfig` parses JSON, requires an absolute `StateDir`, defaults
  `Hostname` to `blockads-android`, normalizes and validates `AllowedCIDRs`,
  and never includes `AuthKey` in validation errors.
- `EnsureStateDir` creates or repairs the state directory with mode `0700`.
- `tailscaleOutbound` implements the existing `OutboundAdapter` contract plus
  the internal `DialContext` seam used by `Engine.DialUpstream`.
- `StartFull` creates and registers the Tailscale outbound only when a
  configuration exists. Without it, the existing direct/proxy behavior is
  unchanged.
- CIDR-restricted destinations use Tailscale. Destinations outside the
  allowlist return `ErrNoOutbound` and retain the existing protected direct
  path. A failed Tailscale dial for an allowed destination does not silently
  fall back to direct.
- Android builds connect the existing `SocketProtector` to
  `netns.SetAndroidProtectFunc`; non-Android tagged builds use a no-op hook so
  Linux verification remains possible.
- Non-`tailscale` builds expose a stub constructor error explaining how to
  enable support.
- The real Tailnet TCP test runs only when both `TS_AUTHKEY` and
  `TS_TEST_TARGET` are set.

The auth key is held in memory for tsnet initialization but is not logged or
included in returned configuration errors. The PoC uses the tsnet file state
store below the caller-provided app-private directory; encrypted Android
state storage is not implemented yet.

## Android wiring

- `TailscalePreferences` stores the experimental settings in the existing
  DataStore: enabled, auth key, hostname, allowed CIDRs, and optional control
  URL.
- The default is disabled, with hostname `blockads-android` and an empty
  allowlist. The HTTPS Filtering screen exposes only the minimal experimental
  controls: enable switch, auth key, hostname, and comma-separated allowed
  CIDRs. There is no Tailscale login UI; authentication is auth-key-only.
- `TailscaleConfigJson` creates the Go configuration with
  `context.filesDir/tailscale` as the absolute state directory. The auth key
  is stored in the app-private DataStore and is never included in the input
  object's `toString` or Android log messages.
- `VpnSessionController` reaches `VpnEngineCoordinator`, which builds the JSON
  snapshot and passes it to `GoTunnelAdapter` only for the full-tunnel start
  path. An empty JSON preserves the existing startup behavior.

The Android wiring was not compiled in this environment because the Android
SDK and generated tunnel AAR bindings are unavailable. APK/AAR size,
device startup, and real Android auth-key login remain unverified.

## Build and test

Run from `tunnel/`:

```sh
go build ./...
go vet ./...
go test ./... -skip 'TestDoTErrors|TestDoQErrors'

go build -tags tailscale ./...
go vet -tags tailscale ./...
go test -tags tailscale ./... -skip 'TestDoTErrors|TestDoQErrors'
```

The two skipped DNS tests depend on wildcard DNS behavior in this environment:
`internal/dns/TestDoTErrors` and `internal/dns/TestDoQErrors`.

Both tagged and tagless builds, vet runs, configuration tests, API golden
tests, and full test suites passed with those two known environment-dependent
tests skipped. The real Tailnet test was skipped because no test credentials
were supplied.

## Binary size measurement

These are Linux, unstripped Go test binaries, not Android AAR/APK or
ABI-specific release measurements:

| Build | Command | Size |
|---|---|---:|
| Tagless | `go test -c -o /tmp/blockads-tunnel-default.test .` | 27,681,494 bytes |
| `tailscale` | `go test -tags tailscale -c -o /tmp/blockads-tunnel-tailscale.test .` | 42,295,463 bytes |
| Difference | tagged minus tagless | 14,613,969 bytes (+52.8%) |

An Android AAR/APK measurement across all supported ABIs is still required
before merging the dependency into a production build.

## Not verified

- Real Tailscale login, auth-key rotation, logout, or machine approval.
- TCP connectivity to an actual Tailnet service.
- Subnet Router route acceptance and fail-closed behavior on a real tailnet.
- MagicDNS, split DNS, exit nodes, or Tailscale DNS configuration.
- UDP forwarding through Tailscale; full-stack UDP remains on the existing
  protected direct path.
- Android `VpnService.protect()` behavior and an Android device build.
- Android AAR/APK size, startup latency, memory, battery, and ABI coverage.
- Encrypted app-backed `ipn.StateStore`.

## Next stage

1. Add an Android-authenticated state store and user-visible login/status flow.
2. Run the TCP PoC on a physical Android device against a controlled Tailnet.
3. Add explicit Subnet Router route acceptance and route-selection tests.
4. Design connected-UDP mapping, idle timeout, IPv6, MTU, and leak tests before
   claiming broader Tailscale forwarding.
5. Re-measure AAR/APK size and re-evaluate the pinned Tailscale/gVisor versions.

## Building an APK that actually contains Tailscale

The upstream-facing default AAR is built **without** the `tailscale` tag, so a
plain `go build`/`gomobile bind` keeps tsnet out of the artifact and
`SetTailscaleConfig` reports
`tailscale support is not enabled; rebuild with -tags tailscale`.

This fork's own releases are different: `release.yml` and `branch-build.yml`
default to `tunnel_tags=tailscale`, so every published fork APK contains the
tsnet outbound (measured: 23 MB → 32 MB per ABI, `libgojni.so` 26.6 MB).
To build by hand:

```bash
./gradlew -Ptunnel.source=local -Ptunnel.tags=tailscale assembleFdroidDebug
```

The same switch is exposed on the fork's `Branch build` workflow
(`workflow_dispatch` → `tunnel_tags: tailscale`), which uploads
`blockads-debug-<run>-tailscale`. CI lints and unit-tests the tagged build too.

Measured on this branch (Android/arm64 only, `-ldflags="-s -w"`):

| Artifact | Size |
|---|---|
| `tunnel.aar` (default) | 6.6 MB |
| `tunnel.aar` (`-tags tailscale`) | 10.0 MB |

The Android build with the tag is verified to compile; a real Tailnet login,
Subnet Router reachability and MagicDNS remain unverified (no credentials or
device in the authoring environment).
