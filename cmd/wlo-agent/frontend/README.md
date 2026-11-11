# Agent host frontend

Vue 3, Tailwind CSS 4 and Vite, using the same dependencies and Vue formatting
conventions as the admin frontend. Go still owns the canvas, automation API,
authenticated RPC, asset allowlist and game WebSocket gateway.

From the repository root:

```sh
npm ci
npm run agent-host:test
npm run format:check
npm run lint
npm run agent-host:build
go test ./cmd/wlo-agent
go build -o bin/wlo-agent ./cmd/wlo-agent
```

Commit the frontend sources and the complete generated `../host` directory
together. Do not edit generated HTML, JavaScript or CSS. A normal Go build embeds
that checked-in bundle and needs no Node.js, npm service or CDN at runtime.
`public/wasm_exec.js` is the upstream Go browser runtime, copied byte-for-byte
into the bundle; update it with the supported Go toolchain rather than formatting
or modifying it by hand.

The compiled UI uses `/ui/` so `/assets/` remains reserved for game packs. The
host retains the `window.wloPacks` and `window.wloReadBlob` contracts and fetches
only verified HTTP byte ranges. Its status disappears once `window.wlo` reports
that the client is running; startup and runtime failures remain visible.

For development, explicitly start the Go agent with `-enable` and copy the **assets server** URL printed
in its log (not the authenticated RPC listener). Then, in a second terminal:

```powershell
$env:WLO_AGENT_HOST_URL = 'http://127.0.0.1:PORT'
npm run agent-host:dev
```

The loopback Vite server proxies pack, WASM and game WebSocket requests to that
host, using its origin. Open the development URL printed by Vite. Reload the page
after changing startup/bridge code so the previous Go WASM runtime is replaced.
Production agents continue to use the compiled embedded UI.
