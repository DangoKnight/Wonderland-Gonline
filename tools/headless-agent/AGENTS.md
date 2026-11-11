# Headless agent instructions

This directory drives the real Wonderland client headlessly. Read
`docs/HEADLESS_AGENT_GUIDE.md` first; it covers login, combat, collisions,
warps, shops and the full `window.wlo` surface.

## Launch

```bash
# 1. WASM client (from the repository root)
cd client
GOOS=js GOARCH=wasm CGO_ENABLED=0 go build -o ../bin/wonderland-client.wasm ./web
cd ..

# 2. Orchestrator
go build -o bin/wlo-agent ./cmd/wlo-agent

# Set WLO_AGENT_TOKEN to the same random 32+ byte secret for host and scripts.
# 3. Run (JSON-RPC: ws://127.0.0.1:9000/rpc)
./bin/wlo-agent -enable \
  -chrome /path/to/chrome \
  -assets ./bin \
  -game 127.0.0.1:6414
```

The agent serves the host page, the WASM binary, the asset packs and the
`/ws` game gateway on one loopback port. The core bundle manifest selects
its exact companion packs, including audio; stale bundles are ignored.

## Files and what belongs in git

- `examples/*.example.py` are tracked templates. Copy them to `session/`:
  `examples/wlo.example.py` -> `session/wlo.py`,
  `examples/agent_world.example.py` -> `session/agent_world.py`, and so on.
- `session/` is gitignored. It holds the working per-session controllers,
  `__pycache__` and `bugs.log`. Do not commit it.
- Keep findings in `session/bugs.log`; it is local and gitignored.
- Reusable tools and durable findings can be committed **only with user
  approval**. Prototype instrumentation goes to a separate
  `experiment/debug-tools-*` branch.

## Operating rules

- Decide from `state`, `grid`, `talk` and `battle`; use screenshots as
  evidence, not as the primary navigation source.
- Heal before a fight (Spicy Hot Pot, slot 3). After the fight check
  `battle.result`; on `lose`, heal and retreat.
- Stop when `state.map.id` changes. Re-read map/camera/areas before the next
  click; never keep walking old coordinates.
- Talk menus: if a click does not advance, use `talk.answers` and
  `answer(index)`.
- Keep the local bug log in the form
  `timestamp | severity | area | symptom | repro | evidence`.
- When stuck, add a tool in layers: client method -> `window.wlo` ->
  Python helper -> JSON-RPC. Prefer a small reusable helper over retrying
  coordinates.
- Ask before committing or pushing. Never read or print secrets.
