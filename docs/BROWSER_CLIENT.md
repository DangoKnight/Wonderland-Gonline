# Browser client and WebSocket gateway

Imported from [PR #11](https://github.com/DangoKnight/Wonderland-Gonline/pull/11).
The browser entry point is experimental: cross-compilation and simulated-browser
transport tests pass; a live browser login has not been validated here. The
desktop client remains the normal supported launch path.

## Build

From `client/`, in PowerShell:

```powershell
$env:GOOS = 'js'
$env:GOARCH = 'wasm'
$env:CGO_ENABLED = '0'
go build -o ../bin/wonderland-client.wasm ./web
Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED
```

From the repository root, build/run the transport separately:

The browser gateway is optional and requires `-enable`. Without that switch,
it exits before opening any HTTP or WebSocket endpoints. Starting the normal
game server never starts this gateway; administration remains independent.

```powershell
go build -o bin/wsgateway.exe ./cmd/wsgateway
.\bin\wsgateway.exe -enable -listen 127.0.0.1:8090 -target 127.0.0.1:6414
```

The gateway exposes `/healthz` and `/ws?target=host:port`. Its target policy
permits `127.0.0.1`/`localhost` on ports 6000–6999. Non-loopback listening requires
`-allow-remote`. `-web <directory>` optionally serves a browser host directory at
`/`; TCP connections to the game server originate from the gateway.

## Host-page contract

The PR supplies the Go entry point and transport, not a complete download/cache
host page. A host must load the matching Go toolchain's `wasm_exec.js`, instantiate
the module and provide these globals before starting it:

- `window.wloPacks`: an object mapping each core/companion ZIP filename to its
  Blob. Include all packs required by the compiled assets.
- `window.wloReadBlob(name, offset, length)`: returns a Promise resolving to an
  ArrayBuffer for the requested byte range.

The reader uses a bounded 96 MiB block cache instead of copying whole packs into
Go memory. The optional browser image-decoder hook remains disabled; standard
decoding is used. Pending Promise callbacks and closed socket callbacks are
released. Socket receive and outgoing queues are bounded.

Build the assets using the same source/client-language choices as the desktop
client; see [asset import](ASSET_IMPORT.md) and
[asset distribution](CLIENT_ARCHITECTURE.md). Browser character layouts derive
from the compiled sprite metadata, as on desktop. Compiled terrain records do
not require the loose terrain JSON export. Neighbor prefetch remains disabled.

## Integration verification

The import also fixes guidepost/fixed-art conversation facing and map-local
hidden-NPC ownership. Map acknowledgment retains hides applied by the destination
snapshot while clearing stale click IDs from a different map. Native layout
backgrounds retain their normal selection, with a fallback when only the right
panel exists. Desktop image decoding remains streaming when no browser hook is
installed.

Focused tests cover gateway framing/relay, byte/ReaderAt ZIP mounts, desktop
decoding, map visibility, facing exceptions, skin fallback and compiled terrain.
WebAssembly transport tests run under Node with a simulated WebSocket and verify
binary I/O, callback cleanup and overflow handling. No manual legacy-client or
live browser acceptance is claimed. Local import logs are in
`var/pr-import-*.log`.

[PR #12](https://github.com/DangoKnight/Wonderland-Gonline/pull/12) separately
imports the admin-console layout. It retains Gonline branding, existing tool
views and configured service-port status; formatting, HTML lint and admin API
tests pass. These imports do not change persistent database schemas.
