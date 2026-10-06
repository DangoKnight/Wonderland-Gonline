# Client assets and multiple sessions

## Build and edit

The canonical client build runs the asset compiler before the Go compiler:

```sh
make build-client
./bin/wonderland-client
```

This produces `bin/wonderland-client`, the core pack `bin/client-assets.zip`,
and three companion packs for world, sprites and audio. `_packs.json` inside
the core pack lists their exact filenames. Ship the executable, core pack and
those three files together. The executable discovers the core beside itself
and mounts the whole set directly without unpacking a second asset tree. `-assets /path/client-assets.zip` selects another bundle.
`-assets data` still supports loose editable sources during development.

Edit the PNGs and JSON under `data/`, then run `make build-client` again. The
compiler hashes source content, reuses unchanged compressed entries and rebuilds
changed entries. Terrain and sprite compilation also use content hashes and a
verified output cache. An entirely unchanged build leaves the packs untouched.
Timestamps alone cannot hide edits. Build output is replaced
only after validation succeeds, so a bad edit leaves the previous bundle usable.
It never re-extracts WLRI assets or overwrites the artist's source files.

On systems without Make, the same build is:

```sh
go run ./cmd/client-bundle -source data -output bin/client-assets.zip -contract client/asset-contract.json
cd client
go build -o ../bin/wonderland-client .
```

On Windows, use `../bin/wonderland-client.exe` for the final output. A plain
`go build` only compiles Go; it cannot invoke an asset compiler. Use the build
commands above to include edits in a distribution. `go generate` is not required.

Make variables `CLIENT_DATA`, `CLIENT_BUNDLE`, `CLIENT_OUTPUT` and
`CLIENT_CONTRACT` select alternative source/output paths. Compiled bundles and
runtime profiles under `bin/` are ignored by Git. Track the dimension contract,
JSON metadata and code; keep the existing ignored-media policy for PNG/audio/video.

## Format and compatibility checks

The container format remains standard ZIP64, with independently readable entries.
The compiler now produces runtime data instead of shipping the largest exports:

- `runtime/maps/<map>.bin`: one terrain record per map, with binary walk grids,
  layers, object placements and sound zones.
- `runtime/events/<map>.bin` and `index.bin`: event payloads and a compact map/scene
  index. The client loads only the selected map's event bytes.
- `runtime/objects.bin`: binary WEM object definitions without hexadecimal text.
- `sprites/<archive>/runtime.bin`: a small archive index, original source size,
  PNG sheet references and stable sprite IDs/slots.
- `sprites/<archive>/records/<slot>.bin`: one sprite's frames, animations,
  placement, palette and already matched palette indices. Each record loads on
  demand. Unchanged frames remain recolorable; edited frames retain their colors.
- `pictures/**/*.png.wli` and `media/**/*.png.wli`: binary image descriptors
  preserving logical names, original canvas dimensions and cropped atlas views.
- `runtime/images/{world,core}/<hash>.wlt`: independently compressed image pages.
  Limited-color pages store palette indices and prepared RGB565 palette values.
  Other pages use compact RGB565/residual planes or optimized PNG, whichever is
  smaller. Both encodings preserve the client’s decoded 8-bit RGBA values; transparent pixels with
  nonzero hidden RGB values are retained too.
- Sprite PNG atlases remain unchanged. Editable picture/media PNGs stay in `data/`
  and are omitted from optimized runtime packs. WAV, Ogg and AVI stay whole.
- Smaller JSON definitions and lookup manifests remain compacted and compressed.
  Their existing typed consumers remain compatible.

Terrain/event/sprite records have a `WLRT` magic and explicit schema version followed by Go
`encoding/gob` data. Record entries are DEFLATE-compressed independently. No
native JMA archive or temporary decoded sprite file is needed by a compiled client.
Loose `-assets data` development mode still supports the original editable exports.

The compiler's runtime dependency selection is in
`internal/clientbundle/runtime.go` (`runtimeSource` and `packGroup`). Full ground,
event, WEM and sprite exports, server-only definitions and the currently unused
`data/audio/` extraction tree are excluded. Keep all editable/decompiled sources
under `data/`; compilation changes only disposable build output.
PNG and media dependencies are retained conservatively because native code forms
many image/sound names dynamically.

The four deployment files are the core plus content-addressed
`client-assets-world-<hash>.zip`, `client-assets-sprites-<hash>.zip` and
`client-assets-audio-<hash>.zip`. A new core is published only after companions
exist, leaving the previous set usable after a failed build. Keep an old set's
companion files while its clients are running. `_bundle.json` in each pack records
runtime entry hashes; `_packs.json` in the core selects its companions.
`*.build.zip` files and `.client-runtime-*` directories are compiler caches;
exclude them from distribution. Removing caches forces recompilation. Old
unreferenced companion files can be deleted after clients using them have exited.

`-optimized=false` on `cmd/client-bundle` builds the earlier single export bundle
for compatibility/debugging. Normal builds use optimized packs.

PNG preserves alpha and allows artists to use ordinary image editors. The bundle
keeps lossless source colors without relying on a GPU-specific compression format.
Both renderers preserve native RGB565 color quantization. The GPU rasterizer
applies that quantization in shaders rather than rebuilding a CPU framebuffer.
UI pictures retain the native color key and opaque partial-alpha behavior;
edited sprite PNGs retain straight-alpha blending. See the
[PNG specification](https://www.w3.org/TR/png/) and
[Go ZIP documentation](https://pkg.go.dev/archive/zip).

`client/asset-contract.json` records original PNG page dimensions. Replacing an
image is allowed when its dimensions match; colors and alpha may change freely.
Missing contracted images and resized images fail compilation with their paths
and expected dimensions. New images get dimension entries after a successful
build. The initial contract was captured from this project's current exported
assets. Changing a contract is an intentional format/layout change requiring
review, rather than a way to bypass an incompatible artwork edit.

The compiler also checks:

- JSON syntax, manifest PNG references and atlas rectangles;
- editable sprite versions, stable/unique archive indexes, frame rectangles,
  canvas/anchor bounds and animation frame references;
- that referenced sprite rectangles fit the actual PNG page;
- unsafe reference paths and symlinks;
- changes to source content while an entry is being rebuilt.

These are structural checks, not proof that a gameplay edit is semantically
correct. Keep resource IDs and native action indexes compatible with their
consumers. Client content edits do not modify the server's authoritative assets
database or grant learned skills/items to characters. Typed runtime loaders
continue to validate the data they consume.

## Measurements and remaining opportunities

On the current WLRI data, the previous single bundle was approximately 4.5 GiB.
The optimized deployment set totals approximately 3.0 GiB. Cache/build files are
excluded from these totals. These are local measurements, not format guarantees.

The focused first-terrain benchmark for Ship Deck measured 108–127 ms and about
154 MB of allocations when parsing the editable ground export. The first compiled
lookup measured about 6 ms and 2.7 MB, including ZIP directory initialization;
subsequent fresh record lookups measured about 0.14 ms and 50 KB. These numbers
measure terrain retrieval, not total startup, world rendering or peak resident RAM.
Reproduce with:

```sh
cd client
WONDERLAND_TEST_CLIENT_BUNDLE=../bin/client-assets.zip go test ./wlo/world -run '^$' -bench BenchmarkFirstTerrainRecord -benchtime=1x -count=3
```

The next optimizations should follow profiling: compile remaining small definition
JSON into consumer-specific tables, split world/sprite packs further for smaller
patches, or use independently compressed blocks when read/seek costs justify them.
Patch override precedence is not implemented. GPU rendering uses cached RGBA8
textures and the existing lossless pages. Block-compressed GPU textures remain
future work and need separate checks for editable alpha and native color fidelity.

The source/runtime separation follows the general approach described in
[Godot's import process](https://docs.godotengine.org/en/stable/tutorials/assets_pipeline/import_process.html).
ZIP is also supported by [Godot resource packs](https://docs.godotengine.org/en/stable/tutorials/export/exporting_pcks.html).

## Session workspace

The normal game opens a 1020 × 600 platform window: the original 800 × 600
game viewport on the left and a collapsible 220-pixel session panel on the right.
The panel never covers or resizes the game UI. Four session cards and the **+** card fit together without scrolling. The panel has no title or footer bar.
The **+** card is the last entry in the list and scrolls with it. Click **+** to
open an instance, or a preview card to switch. The active card has a gold border;
cards highlight on hover. When the list overflows, its right-side scrollbar
supports dragging the thumb and paging by clicking the track. Mouse-wheel and
track input ease between pixel offsets, including fractional wheel movement;
partially visible cards are clipped to the list and remain correctly clickable.
Scrollbar gestures stay captured until release and never reach the game.
Card titles default to **Session <number>** and stay independent of character
names. Click a title to edit it; typing replaces the selected text. Enter or
clicking outside saves, Escape cancels, and a blank title restores the default.
Titles accept up to 40 characters and persist in `workspace.json`. While editing,
keyboard input stays in the title editor.
Each card has an **X**; closing requires confirmation inside that card. Cancel
leaves it connected.
Removing the final session closes the application. Settings Exit/Leave closes
that session, while ordinary Log Out returns that session to server selection.

The arrow at the upper-right collapses or expands the panel over a 200 ms slide.
The window keeps its size: when collapsed, the active 800 × 600 game viewport
centers horizontally while the list slides out to the right. The arrow remains
visible as the expand button. The panel never overlaps the game. Mouse input is
translated to the shifted viewport and pauses during the slide; keyboard input,
networking and simulation continue.

Each new session starts at server selection with its own connection, login,
character selection, UI, chat, inventory, skill progress, world actors, timers,
automation and render buffer. Only the selected session receives platform input.
Switching cancels held mouse/key gestures so they cannot affect another session.
The panel consumes its own input even when a game's modal window is open.

Every session processes sockets, world movement and Remote automation on each
60 Hz game tick, including while the platform window is unfocused. Inactive previews normally redraw at 4 Hz. Movies and minigames
retain their normal frame processing because their current implementations
combine updates and rendering. The active session alone plays music, effects
and ambience. Inactive sessions remain connected and can run configured Remote
actions; switching is not a pause or logout.

The first session keeps the existing writable profile location. Additional
sessions use numbered profile directories with separate account lists, server
preferences, local settings and per-character hotbars. Bundle distributions keep
profiles beside the bundle under `profiles/`. Server-list overrides use
`-serverini /path/SERVER.INI`; installation configuration stays outside the bundle.

### Workspace profile and application identity

The client is named **Wonderland Gonline**; the server and its administration
pages are **Wonderland Gonline Server**. The window icon uses Xaolan's original
24 × 24 portrait (sprite family `007`, ID `7148`) through the shared sprite
loader. Editing that portrait and rebuilding assets updates the window icon too.
The Go modules are `wonderland-gonline` and `wonderland-gonline/client`.
References to WLRI, aLogin and sibling directory paths keep their original names.

`workspace.json` remembers the open session list, stable session IDs, active
session, custom titles, collapsed panel state, last selected servers and associated account names.
For the compiled client it lives in `bin/profiles/workspace.json`; loose-data
runs use `var/client/workspace.json`. Embedding callers that supply `app.Options.SettingsPath` store it
beside that settings file. Account and server fields are explicitly `null`
when no association exists. Passwords and live connections are never stored.

On restart, each saved server is connected using the current server list and
its account login screen opens with the saved name filled in. No login is
submitted automatically. Sessions without a server start at server selection;
servers removed from the list also fall back to selection. Failed connections
retain the server/account choice for another restart. Account names are recorded
only after successful authentication; explicit Log Out/back from character
selection clears that account association. Closing the window preserves it.

Changes are saved atomically with owner-only file permissions; unchanged state
causes no file writes. An invalid or unsupported profile is reported in the log
and remains untouched. Move it aside to start a fresh workspace. Closing the final
session leaves one fresh login session for the next launch. A profile supports up
to 256 sessions and 1 MiB of metadata. Each restored session retains its own
writable directory and shares immutable artwork with the others.

## Picture compilation and tiled drawing

The image compiler is `internal/clientbundle/images.go`; codecs and shared cache
are in `internal/clientimage`. It keeps source PNGs and manifests intact. Source
image dimensions and manifest rectangles are validated before publishing packs.
`make build-client` compiles same-size edits automatically. A large image has its
own dependency entry; changing it does not recompile its neighbouring maps. Small
images compile together by source directory because they share pages.

Empty RGBA borders are removed from stored pages, while descriptors retain the
original canvas size and placement offsets. Small images up to 128 × 128 pixels
pack into 256 × 256 pages. Identical small artwork in a page shares a rectangle;
identical complete tiles/images share a content-addressed page across files.
Large canvases split into independently readable tiles of at most 256 × 256.
Existing extracted atlases also become tiled pages, so requesting one icon does
not decode its entire original atlas. Source JSON still resolves logical names,
patch precedence, animation strips and atlas crop rectangles.

World backgrounds retain lightweight image views rather than full pixel buffers.
`surface.DrawRect` requests only the source rectangle intersecting the viewport.
UI images use prepared native color-key pixels where available; tint and RGB555
paths retain the original lossless RGBA conversion. PNG-encoded detailed pages
prepare their native pixel buffers once when decoded. Cursors and bitmap fonts
use the same source-compatible reader. Runtime pages never fall back to an
external installation or a developer's source directory.

The shared page cache uses least-recently-used eviction with a 64 MiB budget for
retained pixel/index buffers. Opaque true-color pages retain RGB565 plus exact
low-bit residuals; palette pages retain indices, colors and a native surface.
They do not retain separate expanded RGBA and keyed surfaces. Drawing reconstructs
only requested regions. Multiple views/sessions share decoded pages. Small UI
surfaces registered by the picture database remain shared cached resources.
The editable-PNG atlas fallback has a separate 64 MiB LRU budget; pages exceeding
that budget can be read without remaining cached. The sprite manager keeps its
existing independent 256 MiB page-cache budget.

The first image build performs decoding and compression for the whole collection.
Later builds reuse verified records. Cached pages can be recompressed losslessly
when the codec improves, without decoding whole source maps again. Published
bundles remain intact if a source edit fails validation. The compiler removes
stale generated records from its working cache; previously published pack sets
remain separate files until explicitly removed.

## Ownership and memory

`app.Resources` belongs to the workspace, not to a character. Sessions share:

- the decoded picture database and lazy UI/item/skill image cache;
- the bitmap font, bounded glyph-cell cache and cursor artwork (cursor selection stays independent);
- sprite archives, palette data and the sprite manager's bounded page cache;
- item/skill/formula definitions and scene/NPC/talk metadata;
- immutable terrain, map image descriptors and byte-bounded decoded image pages;
- one audio context and a common decoded sound-effect cache;
- one GPU rendering device, shader and bounded asset texture cache.

A world receives a separate scene descriptor and actor lists; large immutable
pixel buffers and image pages are shared. The scene cache retains four recent maps, and active
worlds keep their referenced images independently of eviction. The sprite CPU
page cache retains its existing 256 MiB bound. GPU sessions share index textures and palette textures; recoloring runs in the
shader. Each session owns its render target and optional blend scratch image. Audio music players, inventories, UI widgets,
input state and network queues never belong in a shared asset cache.

All workspace/UI/state mutation runs sequentially on the Ebitengine game thread.
Socket workers post events to their owning client; the sprite manager protects
its asynchronous native palette decoder with a lock. A removed session closes
its sockets, stops automation/audio and rejects late socket dial results. Removing
one session must not close another session's shared sprites. Workspace shutdown
closes shared resources after all sessions stop.

## Measured image results

On the current asset collection, the world pack decreased from **1737.49 MiB** to
**1694.78 MiB**. The optimized core pack is **28.42 MiB**. Audio remains unchanged;
sprite PNGs are unchanged. Total size of the core and its three current companions
is **3057.06 MiB**. Old published packs and build caches are not part of that
runtime distribution size.

On an Intel Core i5-13600K, `BenchmarkWorldBackgroundDrawing` for Ship Deck at
camera `(700, 500)` measured **0.774 ms** for the editable-image path and
**0.030 ms** for compiled tile drawing. Both loops allocated zero bytes per draw.
This is a warm-cache, background-only comparison, not an overall FPS measurement.
Direct copies from prepared tile rows avoid a temporary viewport buffer. Run:

```sh
cd client
WONDERLAND_TEST_CLIENT_BUNDLE=../bin/client-assets.zip go test ./wlo/world -run '^$' -bench BenchmarkWorldBackgroundDrawing -benchtime=1s
```

## Verification

Focused tests cover image/JSON edits, transparency, failed-build atomicity,
compression/reuse, per-image invalidation, lossless page formats, native color keys,
trim/atlas offsets, viewport clipping, LRU byte budgets, ZIP64 offsets, safe mounted paths, independent session state,
shared buffers, background packet processing without redraw, sidebar input and
socket disposal. With a real bundle:

```sh
cd client
WONDERLAND_TEST_CLIENT_BUNDLE=../bin/client-assets.zip go test ./wlo/app -run TestCompiledBundleLoginAndSharedWorld
```

For live acceptance, log in as two different accounts, switch between their
previews, and check movement/chat/Remote continue in the background. Remove one
session, verify the observer sees it disconnect, and continue using the remaining
session. Modify one same-size PNG and a JSON value, rebuild, and inspect the change;
resize that PNG and verify the build fails while the previous bundle remains usable.

## GPU rendering

Normal launches use `-renderer auto`: `client/wlo/render.Device` replaces the
window and every session's CPU canvas with a persistent Ebitengine GPU target.
Maps, players/NPCs, bitmap text, inventory/skills/settings/chat UI, weather,
lighting, movie zoom/fades, minigames and workspace previews use GPU draws.
Render targets have no CPU `Pix` buffer. Session previews sample those targets
directly; the final workspace is also composed on the GPU. The existing networking,
simulation and active/background redraw schedule is unchanged.

`-renderer gpu` requires both renderer shaders to compile. `-renderer cpu` uses
software RGB565 drawing and incremental framebuffer uploads. Auto falls back to
CPU rendering on shader compilation failure; graphics-device/window errors remain
errors in every mode. CPU mode still uses Ebitengine to display the frame and
requires a display. Startup logs `renderer: gpu` or `renderer: cpu`.
Offline `-snapshot` renders through CPU surfaces and keeps the existing PNG output.
No source assets or compiled bundle format change is required for GPU rendering.

### Pixel compatibility

`surface.Backend` owns the target. Existing surface calls dispatch ordered GPU
operations instead of modifying CPU pixels. GPU output is RGBA8, quantized to
the same 5/6/5 channel values after every draw. Native color keys, clipped source
rectangles, nearest-neighbor scaling and partial-alpha sprite behavior are preserved.
Ordinary native UI/text blits use Ebitengine's built-in image path and can batch
on its internal atlas. Other operations use `render/raster.kage`:

- integer sample coordinates reproduce software nearest-neighbor selection;
- indexed sprite textures sample shared palette textures, including opaque black;
- PNG sprite blending uses the original 255-scale integer rounding;
- alpha fills use native 256-scale channel arithmetic;
- additive lighting uses the original 32-scale contribution and saturation.

Gauge clipping submits row geometry rather than rebuilding a masked pixel buffer.
The CPU still calculates animation, placement, text layout and gauge boundaries.
Asset decoding, native pixel preparation and cached glyph construction also remain
CPU work. They are asset preparation, rather than per-frame canvas rasterization.
Effects that read the destination copy their affected rectangle to GPU scratch
before the shader draw. Opaque/cutout sprites avoid that copy. Movie zoom and
connection-loss snapshots use GPU-to-GPU surfaces; normal rendering performs no
CPU readback. Explicit `Surface.RGBA()` captures do read back a GPU target.

### Memory and lifecycle

One device belongs to the window and is shared across sessions. Its texture LRU
retains at most 128 MiB of asset pixel payload, excluding driver/atlas overhead
and render targets. It caches source pictures, sprite frames, indices, palettes
and visible compiled pages. Compiled map pages upload only on a texture miss;
loose large images use fixed 256 × 256 source tiles. Camera movement does not
create viewport-specific textures. Missing offscreen pages stay unopened.
A single asset exceeding the cache budget fails with an explicit error.

Each session retains its own 800 × 600 target and, when needed, same-sized scratch
image (about 1.83 MiB each in RGBA8). The window owns its 1020 × 600 composition
target. Frozen scenes retain an additional target until reset. Removing a session
releases its targets without closing shared textures. Closing the workspace
releases the remaining targets and asset textures. `Presenter.Stats()` reports
cache payload, texture/target counts, asset uploads and explicit readbacks.
Performance depends on the graphics driver; there is no measured whole-game FPS
claim for this change.

The CPU compatibility presenter still tracks changed 32 × 32 tiles, coalesces
adjacent tiles and uses a full upload for a densely changed frame. Unchanged CPU
frames upload nothing. The legacy GPU framebuffer-conversion helper remains for
embedding/canvas compatibility; normal GPU play bypasses framebuffer uploads.

### Renderer development and verification

Create temporary targets through `Surface.NewCompatible`; release them with
`Close`. Use `Clone` for freezes and surface draw calls for composition. Never
read/write `Pix` on a GPU target or create a CPU shadow canvas for normal draws.
Only immutable CPU asset construction and CPU compatibility routines may write
pixel arrays directly. Increment a source surface's `Revision` after a direct
pixel edit; normal CPU drawing methods increment it automatically. Texture/glyph
caches and render calls belong to the game thread; socket events stay queued.

Graphics tests require an available display/context:

```sh
cd client
WONDERLAND_TEST_GPU=1 go test ./wlo/render -count=1
WONDERLAND_TEST_GPU=1 \
  WONDERLAND_TEST_CLIENT_BUNDLE=/absolute/path/to/bin/client-assets.zip \
  go test ./wlo/app -run '^TestGPUClientParity$' -count=1
```

The app graphics test runs in a subprocess because Ebitengine owns
process-global graphics state and shuts down its native backend when `RunGame`
returns. This keeps graphics teardown separate from later CPU/UI tests in a full
suite; the subprocess uses the same test binary, including race instrumentation.

The renderer test compares all 65,536 native colors, all alpha levels, clipped
blits, scaling, indexed palettes, lighting, all gauge percentages, sparse compiled
canvases, loose image tiles, cache reuse/eviction and GPU-to-GPU copies against the
CPU renderer through real GPU readbacks. The app test compares entire login,
Ship Deck, inventory, skills, settings and two-session frames, collapsed previews
session removal and clipped, partially scrolled session cards. It also checks that normal composition performs no readbacks
and that removal releases the session target. `GPU_DIFF_DIR` optionally saves
CPU/GPU images when the app comparison fails. Run focused CPU regressions and
race checks on these modules when changing shared caches or lifetime behavior.

## Shoreline water vehicles

Mouse and arrow-key walking retain the requested target while approaching shore.
Native travel classes follow `FUN_00154a64` / `FUN_001549e8`. Shore transitions
follow `FUN_0041897c`, including its -2..+3 cell scan; Go also rejects a crossing
through an obstacle. Empty, locked and wrecked vehicles are excluded.

Boarding sends AC15:14 `[slot:u8, item:u16]`. Only a matching owner AC15:18
`[slot:u8, owner:u32, item:u16, x:u32, y:u32]` advances the transition. Following
`FUN_0044a6dc`, the avatar relocates to the saved water cell and sends AC15:7
`[slot:u8, item:u16]`. AC15:10 `[slot:u8, owner:u32, item:u16]` enables water
pathing and resumes walking. Foreign, malformed and repeated placement receipts
cannot board or replay the confirmation. A missing receipt expires after five
seconds and restores the prior terrain position if the transition was not confirmed.

Mounted paths admit water terrain 2 and 8 and stop before land. A land click
relocates to a nearby clear land cell before requesting AC15:10
`[slot:u8, item:u16]`, as `FUN_0041897c` / `FUN_0035671c` do. Go reports the land
position with AC6:2 first so the server saves the landing location in the dismount
transaction without charging an extra movement step or triggering encounters.
Riding clears on AC15:11/break and the remaining land route resumes. Inventory
consumption stays authoritative on the server, including disposable rafts.

Mount/break/dismount replies update peers and artwork. Known native water families
and capsules are supported. Additional vehicle classes, companion riding and full
native passenger presentation remain pending.

### Seated poses and raft recovery

The default Alt+1 shortcut applies the native held sit pose (actions 16/17,
`FUN_0027f248`, table `0x4bd7bc`, `FUN_00430f90`). It sends AC32:2 and renders
locally; peers receive the server broadcast. Text input, battle/event holds and
mounted vehicles suppress this shortcut. Water riders use directional seated
frames 46..53 (`FUN_00445950`); the vehicle retains its walking/standing direction.
Dismounting restores ordinary body rendering. Raft composition also applies the
native body/direction rider tables (`FUN_00154100`, X at `0x4baafc`, Y at
`0x4babb0`) and the vehicle canvas correction of +68 Y (`FUN_00154d20`).
Weapons use their own +68 Y canvas correction (`FUN_002fe8e8`); mounted seated
poses hide hand weapons except item type 6 (`FUN_00433318`). These corrections
belong to the renderer; exported PNG anchors remain unchanged.

AC15:15 plays the vehicle destruction strip (`FUN_00449f80`): Robinson's raft
uses `images/48010_B`, three vertical frames at 70 ms per frame, centered at the
vehicle's last water position. Missing vehicle artwork falls back to
`images/48005_B`. The client sends the six-byte AC15:13 owner acknowledgment.
Effects expire after one playback, remain specific to their session/map and use
the existing GPU-compatible picture drawing. A preceding AC7 recovery retains
the old water point only for this brief effect; the rescued player stays ashore.

Robinson's disposable raft breaks on Starter Beach (map 11016) at the legacy
movement trigger X >= 280, Y >= 950, or through its ordinary durability/landing
rules. The server now finds the nearest interior walkable land point from the
SQL terrain catalog and commits that position together with raft removal. It
sends authoritative AC7 placement before the deletion/break/dismount sequence.
The Go client applies its own AC7 correction, stops walking and clears pending
shore travel. Already-walkable positions remain unchanged. An available terrain
with no safe land leaves the raft intact. Native AC15:13 acknowledges a dismount
with either the compatibility two-byte packet or the six-byte owner-ID packet;
both are accepted without replaying consumption.


### NPC conversation facing

Dialogue and NPC questions follow `FUN_00304fd0` -> `FUN_00307ae8` ->
`FUN_00432638`. Talk mode 1 turns an eligible NPC toward the local player;
talk mode 2 preserves the NPC's direction. Question prompts turn eligible NPCs
in both acknowledgment modes. The player also faces the NPC. Turning uses the
native eight-direction slope thresholds already implemented by `world.Facing`.

The native exceptions are template kind 6 (props such as chests) and actors whose
pose group is neither walking nor standing (for example, sitting or lying).
The exception is specifically kind 6, rather than all kinds that use fixed prop
frames. Changes are session-local presentation state and send no new packets.
Original NPC actions are saved once per event and restored on AC20:8, matching
`FUN_00307bc4`; subsequent dialogue lines do not replace the saved original.

### Compound window

The Alchemy toolbar button or **Ctrl+C** opens `TRe_CompoundForm` in
`client/wlo/inventory/compound.go`. Its 299 × 467 background, 5 × 10 bag,
five ordered ingredient boxes, close controls and Synthesis button come from
WLRI `FUN_0021b240`; `Screenshot_20261006_123850` is the visual reference.
The cauldron uses the native 23-frame `Compounding` strip at 200 ms per frame.
Submission starts a full sequence; its later frames show the result effect.
At frame 14, a confirmed result flies from the native launch anchor to the
server-selected bag cell at 120 pixels per second along the dominant axis.
The receipt updates inventory immediately; only its compound-window icon is
hidden until landing. Closing/resetting the window or replacing the item
cancels the visual without changing inventory. A missing result reply times out.
Bag and ingredient hover details reuse the inventory item breakdown.

Left-click a bag item or drag it onto an ingredient box to select it;
double-click also fills the first empty box. Selected bag anchors have a pink
background. Right-click the selected bag item or an ingredient box to deselect
it; double-click an ingredient also removes it. The first
occupied box is the base item. Entries refer to bag anchors and consume one
unit each. Selecting an ingredient does not change the bag. Locked items and
an active vehicle cannot be selected; replaced or depleted items invalidate
their selections. Loading, scripted events and modal windows block synthesis.

The server accepts two to five total ingredients, including Alchemy Books, with
at least two non-book materials. With exactly one learned alchemy skill, the client uses it automatically and
hides tier controls. Multiple learned skills offer the native Junior checkbox
or Superior tier selector, limited to learned choices, using AC23:14/87/101; Server validates that tier against SQL skills. Alchemy
uses Formula.dat advancement thresholds and its native 30-level cap, with
per-grade SQL EXP converted to cumulative login/AC8:1 stat 111 counters.
The Go skill UI converts those counters back into per-grade proficiency.
The rank/base algorithm is documented in
[COMPOUNDING.md](COMPOUNDING.md). Compound tooltips include native material-base
names. The choice defaults to the first available tier, remains selected when reopening the window,
and resets on character handoff. Unlearned tiers and changing tier during
synthesis are blocked. No overworld synthesis packet is sent: native AC23:122
is a fishing-stop command incorrectly labelled as synthesis in Legacy.

AC23:9 removes consumed units; AC23:8 installs the fresh result and metadata;
AC23:13 announces the result. Duplicate submission is
blocked while waiting. After five seconds without a result the controls unlock
and display a notice, without automatically resubmitting. Recheck inventory
before retrying, since the server may reject a recipe or lack output space.
AC23:122 is accepted as a fishing-stop receipt. Closing the form before
submission retains the recipe; accepted submissions clear its ingredient
selection locally without consuming inventory.
Character changes and disconnects reset it. Each session owns its selections,
request state and animation clock. Ingredients can be selected and another
attempt submitted while the previous cauldron/result animation is playing.
After a successful new submission, the previous result is immediately revealed
in its authoritative bag position and the new cauldron animation starts.
Only an outstanding server request blocks submission; animation playback does
not. A failed send preserves the previous animation and current recipe.

### Login clipboard

Username and password fields support Ctrl+V from the desktop system clipboard.
`app.loginClipboardText` converts text to the same Big5 encoding as keyboard
input; `seui.Editor` retains the existing 10-byte limit, account normalization,
caret insertion and password masking. Clipboard contents are neither logged nor
cleared, allowing reuse across sessions. The local Ebitengine fork exposes
`ClipboardText` on the window thread through GLFW (Linux/BSD/macOS) and native
Unicode clipboard reads on Windows. Unavailable clipboard reads leave the field
unchanged.
