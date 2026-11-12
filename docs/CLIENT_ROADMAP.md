# Client development roadmap

The normal client is `client/wlo`, built on Ebitengine. Use
[CLIENT.md](CLIENT.md) for implemented behavior, [CLIENT_ARCHITECTURE.md](CLIENT_ARCHITECTURE.md)
for rendering/assets/session ownership, and [TESTING.md](TESTING.md) for checks.
The server has a separate [migration queue](PORTING.md).

This roadmap describes feature work, not a function-count completion estimate.
The generated [aLogin catalog](ALOGIN_CATALOG.md) and `client-inventory.json`
preserve the indexed executable's addresses and research findings. Address
citations and old subsystem labels do not prove present-day gameplay parity.

## Implemented foundations

These systems have working client paths; each still needs acceptance for its
unverified cases:

- Login, character creation/selection, editable character sprites and portraits.
- Map rendering, collision/pathing, roaming actors, peer movement, NPC dialogue,
  choices, movies, loading screens and localized area titles.
- Inventory/equipment, tooltips, quantity editing, stat allocation, Potential
  Pills, compounding, pet controls, vehicles and collection controls.
- Native battle packets and optional Gonline round chains; attack/skill/item
  targeting, capture, pet switching, AUTO/previous actions, effects, voices,
  damage numbers, escape and post-battle notices.
- Social/mail, item trades, teams, invitations, followers and teammate vitals.
- NPC shops, treatment, spawn records, pet hotel and Props Keeper storage.
- Tent exteriors/interiors, entry/exit, guest eviction, furniture placement and
  movement. See [tent coverage and remaining controls](TENTS_CLIENT.md).
- Mall/catalog/cart, bag contents and equipment previews, forging and supported
  paid minigames. See [mall coverage](LEGACY_MALL_RESEARCH.md).
- Notebook, quest log, monster book and star-book presentation. See
  [notebook scope](NOTEBOOK.md).
- Settings for window modes, frame limits, output scaling and live English/
  Traditional Chinese text. Multiple sessions share rendering assets while
  keeping connections and gameplay state independent.

For individual battle limitations and evidence, use
[battle client parity](BATTLE_CLIENT_PARITY.md). Supported minigame kinds and
remaining machines are listed in [MINIGAMES.md](MINIGAMES.md). Do not infer a
working interface solely from a server handler or an exported native form.

## Remaining work

### Gameplay and presentation parity

Compare native captures with current code for remaining motion/camera phases,
spell paths, spectator/outcome artwork, overworld casting, movie/weather modes
and unusual map/object rendering. Preserve general native rules instead of
adding map- or client-specific exceptions. Resolve unknown asset roles before
removing their retained exports.

### Incomplete feature forms

Housing/furniture, manufacturing, pet/furniture trading, stalls/vendors, guild
management, dungeon objectives/rewards, title entitlement and remaining growth
or service forms need further client work. Existing server definitions do not
establish complete UI support. World PK and traditional PK require their own
packet/acceptance checks; their references are in
[World PK](WORLD_PK_REFERENCE.md) and [traditional PK](TRADITIONAL_PK_REFERENCE.md).

### Settings and release work

Security/account/payment controls and title selection remain limited as listed
under [Settings UI](CLIENT.md#settings-ui). Window resizing, multi-monitor/DPI
behavior, scaling and frame pacing need manual platform checks. Resolution
selection and wheel map zoom are currently disabled; the game remains 800×600
plus the session sidebar. Do not document dormant code as a selectable feature.

Package the executable with the core bundle and the exact companion packs named
by `_packs.json`; omit compiler caches. Test fresh Chinese-only, translated-only
and bilingual imports. Preserve the optional `-legacy` front end and `-workbench`
inspection modes while they remain useful for comparison.

### AFK mode per client instance

- [x] Implement AFK mode exclusively for Gonline Client connected to Gonline
  Server. Negotiate support through the existing connection capability exchange;
  offer the mode only when both endpoints support it.
- [x] Add an AFK toggle for each individual client instance/session. Changing
  one instance must not affect any other instance in the same process.
- [x] Restrict AFK operation to vital communication: connection maintenance,
  required protocol acknowledgements, authoritative player/pet/party state,
  combat and configured automation, inventory/reward changes, map transitions,
  timers and actionable errors. Classify messages explicitly so ignoring visual
  updates cannot stall a battle, dialogue or other active interaction.
- [x] Ignore nonessential information such as unrelated players on the map,
  their movement/appearance and chat. Gonline Server must suppress these updates
  for the individual AFK connection, and Gonline Client must ignore any already
  queued nonessential updates. AFK mode is unavailable on Legacy servers and to
  Legacy clients; their existing communication behavior remains unchanged.
- [x] Replace the rendered game scene entirely with an event-log view. Keep
  player, pet and party stats continuously visible, including current/maximum
  HP and SP, level, experience/progress and other relevant status values. Obtain
  any missing authoritative stats through the vital state-update path.
- [x] Log important gameplay events such as combat outcomes, EXP/level gains,
  item acquisition/use, pet changes, recovery, defeats and connection problems.
  Bound retained log history and avoid unnecessary world rendering, animation,
  audio and asset-loading work while AFK mode is active.
- [x] On leaving AFK mode, resynchronize the current scene and entities before
  restoring the world view. Test independent AFK/normal instances, existing
  automation, combat, travel, disconnect/reconnect and return to normal mode.
- [ ] Complete multi-client live acceptance and measure CPU, asset-loading and
  network reductions. Current coverage uses automated fixtures and rendered UI
  inspection; retained NPC/map state and combat timing still require client work.
- [x] Implement negotiated aggregate communication for
  AFK instances in the same client process. For example, AFK party members
  should share one authorized party-state request/response rather than each
  requesting duplicate self/party snapshots. Prefer server-pushed updates over
  polling. Negotiate aggregation explicitly, distinguish shared party state from
  private owner-only data, and preserve per-session authentication and command
  ownership. Handle subscriber changes, leadership changes, mixed AFK/normal
  sessions, reconnects and stale responses with a safe independent-stream fallback.
  Verified with eight native sessions in two parties: two normal leaders and
  six AFK followers stayed bound to two shared streams for a 60-second automated
  battle run. A matching-state wire fixture uses 209 bytes instead of 620 bytes
  for three subscribers. Battle gameplay packets remain per connection; the
  aggregation covers common AFK party snapshots, not all combat traffic.

### Complete headless API coverage

Implemented version 2: typed commands and queries, per-session queues and
in-flight/history tracking, cancellation/expiry/reset, semantic form controls with
strict payload schemas and stale-state/context fences, gameplay/service adapters,
browser preference storage, and authenticated optional RPC. See
[the API reference](HEADLESS_API.md) for coverage and explicit native limitations.
Local dispatch and observed server state remain distinct: Legacy packets do not
provide universal transaction receipts, so automatic resource retries are unsafe.

Agent/API gameplay requires a server-controlled opt-in. It is denied by default; the
gameplay database setting `agent_api_enabled` authorizes declared API sessions,
and revocation disconnects them with a policy explanation. Ordinary manual
clients do not request this permission. See [server permission](HEADLESS_API.md#server-permission).

The native Go client supports browser-free headless TCP API hosting and a
native TCP spectator. Native spectators reconstruct the normal client scene from
ordered packet/action data and render locally at their configured frame rate;
they do not display streamed browser/JPEG frames. The web host/viewer remain
optional alternatives.
The persistent host supports a detachable browser/native viewer and exclusive
API/human intervention alongside the direct-action coverage below.
See [session-host requirements](HEADLESS_API.md#persistent-session-host-and-detachable-viewer).

- [x] Separate hosted game-session lifetime from API transport and viewer lifetime.
  The host retains the game connection and state when a viewer disconnects.
- [x] Attach a Gonline client to the same hosted session for readonly viewing,
  without a second game login; resynchronize snapshots and ordered updates on
  reconnect, including ongoing battles, dialogue and forms.
- [x] Implement atomic, per-session API/human ownership with generation fencing.
  Reject gameplay commands from the inactive owner while allowing observations;
  clear queued/held/unsent input and fence AUTO/Remote producers on handoff.
- [x] Implement explicit intervention/release and controller-loss recovery.
  Revoke disconnected viewer control, reject stale messages and return control
  without replaying old API commands or interrupting the API/game connection.
- [x] Verify exclusive execution and lifetime isolation with race/integration
  tests and live viewer disconnect/reconnect tests throughout gameplay.

- [x] Extend the optional headless API to cover every implemented client feature
  through structured commands and state queries. Every supported workflow must
  be fully operational without mouse emulation, screen coordinates, synthetic
  keyboard input or screenshot interpretation. Keep coverage aligned with new
  gameplay features; input simulation and JavaScript evaluation may remain
  optional diagnostic tools, but must never be required to complete a workflow.
- [x] Dispatch commands directly through the client's normal application actions
  and validation paths, without synthesizing UI input or bypassing server rules.
  Provide direct movement by world coordinates, entity targeting by stable IDs,
  item use by inventory identity/slot and explicit target, and menu access by
  named module. Support opening, inspecting, selecting, confirming and closing
  menus/forms through the API, including nested dialogs and reward choices.
- [x] Publish a versioned, discoverable API contract describing available commands,
  arguments, state fields, prerequisites and errors. Validate types, ranges and
  session ownership before dispatching commands.
- [x] Support login, server/channel and character selection, character creation,
  logout and reconnect. Handle credentials without including them in logs, state
  snapshots or command history.
- [x] Cover movement, targeting, NPC dialogue and choices, quests, collection,
  inventory/equipment, consumables, Potential Pills, compounding, pets, vehicles,
  tents/furniture, shops, treatment, storage and other implemented NPC services.
- [x] Cover combat actions and target selection, capture, pet changes, AUTO,
  complete Remote configuration, Join Battle, spectating, teams/leadership,
  traditional PK and World PK. Retain normal client/server eligibility checks.
- [x] Cover social/friends, chat, mail, boards, trading, mall/cart, reward choices,
  forging, supported minigames, notebook, settings and session management wherever
  these features are implemented. Report unavailable features explicitly.
- [x] Expose sufficient structured state to operate these features without
  screenshots: available actions, open forms and prompts, selections, inventory,
  skills, player/pet/party stats, timers, combat state and actionable errors.
- [x] Distinguish command acceptance, local execution, server rejection and
  observed completion. Propagate client errors through RPC instead of returning
  success merely because JavaScript evaluation succeeded. Provide bounded event
  history, correlation where supported, and timeout/cancellation semantics;
  never infer authoritative success or retry resource-consuming actions blindly.
- [x] Preserve independent instance ownership, ordered input and gameplay gates.
  Keep headless services behind explicit opt-in and authenticated loopback access;
  normal server startup must not enable them. Administration remains separate.
- [x] Verify command/state coverage with focused tests, malformed-input and
  rejection cases, concurrent independent agents, disconnect/reconnect and live
  end-to-end checks with input-emulation helpers disabled. Any remaining UI-only
  path is incomplete API coverage; document it and unverified behavior explicitly.
- [x] Keep this API separate from AFK mode: headless automation runs the native
  Go client or the optional browser backend, while AFK mode negotiates reduced
  traffic and replaces scene rendering. Verify compatibility if both features
  are used together.

## Development and acceptance

1. Read the matching decompiled functions and retained native captures. Record
   executable hashes and address provenance; addresses vary by client build.
2. Name protocol values in their owning packages, preserve independent raw-byte
   goldens, and keep unresolved meanings explicit.
3. Add focused tests for the affected behavior, integrations and resource
   ownership. UI changes need rendered or live comparison where layout matters.
4. Record remaining live checks separately from automated passes. Update the
   owning feature guide when behavior changes.

Follow [development conventions](DEVELOPMENT.md) and
[test selection](TESTING.md#test-selection-and-correctness). Full validation is
required before a push; it is not the default for each UI edit.
