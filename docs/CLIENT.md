# Native client replacement

The Go/Ebitengine client recreates the original `aLogin.exe` front end and world
from the decompile (`client/wlo`). Implemented flows include login/creation,
world events and NPC movement, battle scenes and direct targeting, inventory,
rank/base compounding, Skills and Teams, pet riding/following, Social/mail,
item trades, the scene minimap and several local/paid minigames. Rendering and
gameplay parity remain partial; subsystem details below identify the gaps. `-legacy`
runs the earlier reference front end (`client/ui`), and `-workbench` opens artwork
inspection. These modes do not represent complete original-game parity.

Start with [GETTING_STARTED.md](GETTING_STARTED.md) for WLRI extraction, server
setup and compiling/running both programs. Use [CONFIGURATION.md](CONFIGURATION.md)
for connection settings and client overrides.

For measured runtime costs and optimization priorities, see the
[client performance investigation](CLIENT_PERFORMANCE.md). Full test-suite
timings are separate from interactive frame-time measurements.

Verified [IGG English text overrides](IGG_TRANSLATIONS.md) are baked into compiled
assets for matching NPCs, items, skills, scenes and dialogue. Verified IGG text
takes first priority; existing name corrections supply fallbacks. Unmatched modern content keeps its current
translation. Recompile assets after editing text sources and restart to remount
rebuilt bundles. Selecting an installed language in Settings takes effect live.
For loose exports, `go run ./cmd/asset-build -data data -text-only` generates
pretranslated documents under `data/runtime/text`. The game reads final text and
does not carry or search the override tables.

The workspace's **Settings** button or **F10** opens window-wide settings,
including display controls and language selection.

### AFK mode

On a supporting Gonline server, each session preview in the right sidebar has an
**AFK** toggle beside its title. It changes only that session, without switching
the selected preview. Close interactive forms before enabling it. AFK
replaces that session's world view with player, pet and party HP/SP, level and EXP
progress, gold/timer information and a timestamped event log. Configure Remote or
AUTO beforehand; AFK does not enable automation or change gameplay rules itself.
Other sessions remain independent. Legacy servers do not offer this mode.

Combat, collection, travel, recovery, inventory/rewards and protocol processing
continue. Ordinary chat and unrelated player presentation are suppressed on the
server and ignored if already queued at the client. World/UI drawing, minimap
refresh, speculative world prefetch and ambient audio are skipped; battle timing
metadata and map/NPC state remain available to automation. This is reduced-work
operation, not a separate headless engine or a zero-asset client.

The log keeps at most 256 entries; use the mouse wheel to review it. **Return to
game** or **Esc** requests a peer refresh and waits for local scene preparation
before restoring the view. Unexpected dialogues or other interactive forms
automatically request this return rather than silently answering or stalling.
Disconnects disable AFK; a fresh connection must negotiate it again.
AFK members of the same party in one client workspace automatically share one
common stats stream when supported by the server. Private position/resting-pet
updates remain separate. Carrier departure reassigns the stream; a lone AFK
member falls back to independent updates. Separate client processes do not share
streams.
Supporting servers send compact changed-field updates after the first AFK
snapshot. Unchanged names, maximum stats and other metadata are not repeated;
full snapshots remain available for stream initialization and roster changes.

The protocol is documented in [connection modes](CONNECTION_PROTOCOL.md#afk-mode).
Independent-session, Remote/battle, loading, filtering, privacy, malformed-packet
and refresh-fence tests cover the implementation. Eight-session automated battle
runs verify shared streams and network reductions; see
[performance measurements](CLIENT_PERFORMANCE.md). Broader manual acceptance
remains tracked in the [roadmap](CLIENT_ROADMAP.md#afk-mode-per-client-instance).

### Session debug log

The window settings (F10) carry a **Debug log** row: a switch and a line-cap
button that cycles the offered caps (`client/debuglog`, `client/wlo/app/display.go`,
`client/wlo/app/workspace.go`). The rule belongs to the window, not to a single
session, so it is stored with the other display settings in `display.json` and
applies to every open session at once; sessions added later inherit it.

Each session records into its own file, distinguished by the window suffix:

```
var/client/user/logs/session-YYYYMMDD-HHMMSS-screen1.txt      session 1
var/client/user/session-2/user/logs/session-…-screen2.txt     session 2
```

A transcript records packets in both directions, with command bytes and payloads
in hexadecimal. Credential-bearing login, character creation/deletion and bank
command families retain only their headers and lengths; their payloads are redacted
in both directions, including malformed variants. Other packet payloads can still
contain personal information, including chat, so review logs before sharing them.

Session-owned diagnostics (dialogue, movies, minigames and connection errors) go
to the owning transcript and the normal logger. Process-wide renderer/workspace
messages keep their normal destination. Recording never redirects the global logger,
and disabling or closing one session does not affect another session's recording.

The cap keeps the last N lines, rewritten to the file in batches; **0 =
unlimited**, and an unlimited session can fill the disk, which the row's value
makes explicit. The cap list is `0, 1000, 5000, 20000, 100000`; a hand-edited
`debug_log_max_lines` is clamped to `0…1000000`. Changing the cap while
recording is on starts a new transcript, closing a session flushes its file, and
quitting flushes the rest.
The first implementation supports windowed mode, Ebitengine fullscreen, and a
desktop-sized borderless window. Windowed mode supports edge resizing and the
normal maximize/restore controls. Resolution picking is temporarily disabled:
the internal framebuffer is fixed at 1020x600 (the original 800x600 game plus
220 pixels for session management). Previously saved render resolutions are
retained but ignored while disabled. The read-only resolution field shows this
fixed size. The output scales into the physical window with a static aspect
ratio and letterboxing. Existing bitmap assets retain their native detail.
Pointer input follows the same transform and ignores the borders.
Fullscreen modes occupy the current monitor at desktop resolution; exclusive
fullscreen and monitor video-mode switching are not implemented. See
[Ebitengine's fullscreen contract](https://pkg.go.dev/github.com/hajimehoshi/ebiten/v2#SetFullscreen).
Frame limits are VSync, 30, 60, 90, 120, 144, 165, 240 FPS or unlimited.
Mode, frame-limit and scaling-filter controls open dropdown lists. Click an entry, or
use Up/Down and Enter; Escape or an outside click dismisses the open list.
These control presentation; simulation and input updates remain at 60 TPS.
**Apply** previews changes for 15 seconds. **Keep** saves them to `display.json`
beside `workspace.json`; **Revert**, Escape, or timeout restores the previous
settings. Display changes apply to all sessions and do not require a reconnect.
Scaling choices are Native (nearest), Bilinear, Bicubic (Catmull-Rom), Edge-aware
(Scale2x neighbourhood rules followed by linear scaling), and Edge-aware + Sharpen
(a mild unsharp pass). The filter applies to the composed output, including UI
and text. Native remains the default. Filters sample decoded RGBA, never packed
RGB565 words, and clamp source edges to avoid black fringes. Edge-aware modes
compute their neighbourhood pass at 2x native resolution rather than repeating
it at every output pixel. Intermediate targets stay on the GPU and are reused;
no client assets need to be re-extracted. See the original
[Scale2x algorithm](https://www.scale2x.it/algorithm).
While resolution picking is disabled, filtering runs in Ebitengine's final-screen
pass, where the fixed native framebuffer is enlarged into the OS window. Thus
bilinear/bicubic options remain effective when resizing or maximizing; they are
not applied prematurely to an unchanged 1x intermediate image.
Windows resize/maximize/restore and independent framebuffer resolution have a
native graphics-context test. CPU/GPU pixel tests cover scaled presentation and
letterboxing; persistence, rollback, pointer mapping and panel rendering also
have automated coverage. Multi-monitor/DPI behavior and frame pacing still need
manual testing. No client asset rebuild or database migration is required.

Wheel zoom is temporarily disabled (`worldZoomEnabled` in `world_zoom.go`), and
the client resets any previous map-zoom state to the standard 1x viewport.
UI wheel scrolling continues normally. The dormant 0.75x–1.25x map-camera
implementation remains available for later re-enabling; its limit/picking
tests are skipped while disabled, and a dedicated reset test covers 1x.

Map changes show the existing `Anim_load0` illustration with short fades and
a minimum visible duration of about 800 ms. Slow map readiness holds the
illustration longer. Networking and the native map-loaded acknowledgement remain
immediate; this is presentation timing, with no blocking sleep. Map clicks,
gameplay controls and automation wait until the reveal finishes, and area titles
start afterward. Same-map repositioning does not trigger it. Story cinematics
take precedence so their opening is never hidden behind the loading screen.
The session sidebar remains available. No asset rebuild is required.

The loading illustration has a bottom progress bar and uses the mushroom cursor.
Completion waits for the destination's rendered assets, pending editable sprite
palette decoding and the native AC5:4 map-ready reply. Failed optional palette
archives settle using their existing painted-frame fallback. The bar represents
preparation stages rather than a byte-download percentage and never completes
solely because the minimum display time elapsed. The initial Gonline connection
hello negotiates complete-map-snapshot readiness: updated servers send AC5:4 only
after peers, vehicles, props, tents and other snapshot state have been sent.
Legacy connections retain their original packet order and AC5:4 readiness behavior.

Eligible area changes display the localized scene name at the center of the game view.
The Legacy title-group identity comes from SceneData.dat offset 94
(`FUN_003075f0`); its `t10002` Kelan artwork contains seven animation frames.
Gonline renders a continuous 2200 ms ripple effect instead. Ripple travel runs
for 2120 ms, so the last ring disappears just before the title. Text reveals
behind the outer ring with opacity increasing through the intermediate rings
to full opacity at the innermost ring, and fades over the final
500 ms. The text gently waves
horizontally throughout the effect, and the thicker ripples dissolve towards an elliptical
boundary instead of clipping against the title canvas. It waits for map readiness
and visible gameplay, respects shared area groups, and does not intercept input.
Scenes with title group zero do not trigger the overlay. Returning from an
interior (SceneData.dat disk offset 23, category 3) to a village (category 2)
also suppresses it, matching Legacy. Kelan/North Island transitions show titles;
Civilian House visits and Cabin/Ship Deck transitions do not.
The small title canvas uses alpha-blended GPU-compatible drawing without scene
readback. No asset rebuild or database migration is needed for this overlay.

## Direct decoded assets

The game client can run directly against repository `data/`:

```sh
cd client
go run . -assets ../data
```

Explicit `-assets ../data` reads loose exports. Without an override, discovery
prefers a compiled bundle beside the executable or under `bin/` before falling
back to `data/` or `../data`. Rebuild packs or select loose data to see artwork edits.
The UI uses `media/menu` PNG skins/backgrounds, `media/font/TATPC1_TWN` PNG
font atlases and `media/sound` Ogg Vorbis files. Native WAV references resolve to
the standardized OGG outputs; older WAV-only extractions also work. Maps use `pictures/` and decoded
JSON. Picture lookups resolve logical paths through `pictures/manifest.json`
and crop atlas rectangles; patch priority remains intact. Sprites load editable
`sprites/` atlases without decoded JMA files. Saved accounts/settings go to
`var/client/user`, outside the derived assets. Use `-serverini` for an authored
server list. Regenerate and verify media with the procedures in
[data/README.md](../data/README.md). Packaging uses the same decompiled tree; native inspection modes use explicit
sibling source paths.

## Run

The client is an isolated module in `client/`; the server module does not acquire Ebitengine dependencies. Requires Go 1.25 or later.

All shared command blocks below work in Linux Bash and Windows PowerShell. Start from the repository root unless stated otherwise. Linux paths and archive filenames are case-sensitive: preserve the package's original names, including `Ground.MMG`, `map.JMG` and `item.BMg`.

Windows requires a working graphics driver and desktop session. Ebitengine 2.10 desktop builds use pure Go; the separate server still needs a C compiler for SQLite. Linux requires an X11 desktop (or XWayland) and working OpenGL drivers. On Debian/Ubuntu with Mesa:

```bash
sudo apt install libx11-6 libgl1 libglx-mesa0
sudo apt install libxcursor1 libxi6 libxinerama1 libxrandr2 libxrender1 libxext6
```

Keep an existing proprietary graphics-driver setup. Other distributions and optional audio dependencies are covered in the [official Ebitengine installation instructions](https://ebitengine.org/en/documents/install.html). A server-only headless Linux machine needs a display environment to run the interactive client or rendered snapshots; downloading/staging/verifying assets through `cmd/client-assets` does not require a display.

From the repository root, with the server running:

```text
cd client
go run .
```

The client reads `SERVER.INI` from the asset directory (`data/SERVER.INI` when using the repository data root), as the original reads it beside `aLogin.exe`; `-serverini <path>` selects another file, and `-legacy` reads it from the working directory. Without one it lists a single local server, `01[Local]1` / `Local1*127.0.0.1`. To open the artwork workbench instead:

```text
cd client
go run . -client ../../Wonderland-Client -archive map.JMG -name 11016.jpg
```

Builds discover `client-assets.zip` beside the executable. Source runs can
use `-assets ../data` for loose editable assets; `-assets <bundle.zip>` selects
a compiled bundle. The collapsible panel beside the game manages independent sessions sharing
decoded assets. See [Client architecture](CLIENT_ARCHITECTURE.md). The workbench requires `-client`
pointing to an original installation; its location is unrestricted.

## Compile the client

From the repository root:

```sh
make build-client
./bin/wonderland-client
```

GPU rendering is enabled by default (`-renderer auto`). Require it with
`./bin/wonderland-client -renderer gpu`; select software drawing with
`-renderer cpu`. Maps, sprites, text, UI, effects and session previews render to
GPU targets in GPU mode. CPU snapshots remain available. See
[renderer details and graphics tests](CLIENT_ARCHITECTURE.md#gpu-rendering).


PowerShell:

```powershell
go run ./cmd/client-bundle -source data -output bin/client-assets.zip -contract client/asset-contract.json
Push-Location client
go build -o ../bin/wonderland-client.exe .
Pop-Location
.\bin\wonderland-client.exe
```

The server must be running for login. Use the registration endpoint to create a
Go account; the original private-server database is not an account source.
The root `go build` and `go test` commands do not include this nested module.

## Asset layout and distribution

Repository `data/` is the decompiled asset tree used by the normal client.
It includes JSON data, PNG picture and sprite atlases, exported media and audio.
Development mode reads these exports; distribution builds compile heavy exports
into binary runtime records and split packs. Missing assets do not trigger native
Item.Dat, Formula.Dat or font fallbacks.

Development runs read repository `data/` directly, without links or duplicate
asset directories. From `client/`, run `go run . -assets ../data`. Saved user
state lives in `var/client/user`, outside the asset tree.

For a distribution, build the assets and executable together:

```sh
make build-client
mkdir -p var/client-distribution
cp bin/wonderland-client var/client-distribution/
python3 - <<'PYASSETS'
import json, pathlib, shutil, zipfile
source = pathlib.Path("bin")
with zipfile.ZipFile(source / "client-assets.zip") as pack:
    names = ["client-assets.zip"] + json.loads(pack.read("_packs.json"))["packs"]
for name in names:
    shutil.copy2(source / name, pathlib.Path("var/client-distribution") / name)
PYASSETS
./var/client-distribution/wonderland-client
```

On Windows, copy the `.exe`, core pack and its three listed companions together. Add `SERVER.INI` beside them
for a remote server, or pass `-serverini <path>`. No native asset directory is
included. Profiles remain outside the bundle. See
[Client architecture](CLIENT_ARCHITECTURE.md) for editing constraints and format.
`cmd/asset-build` still verifies/copies loose exports for development and inspection.

The old front end (`-legacy`) and native artwork workbench are inspection modes.
They require `-client` pointing explicitly to the authoritative sibling client
installation. They do not use or recreate `client/assets/legacy`. Offline
`cmd/client-assets` remains available for native archive inspection, but its
output defaults to `var/client-original-assets` and is separate from the normal
client package. Optional `cmd/sprite-build` packs similarly default to
`var/client-spritepacks`; use `-sprites` explicitly to inspect those packs.

## Client port from the reference decompile

The client is being ported again from the decompiled client. The source of
truth is the full decompile of the WLRI build
(`var/decompiled/aLogin_ca19ee087b60_full.c`): it is the client actually run,
and it is more complete than the sibling reference decompile, which lacks
VMT-only methods and stops at 0x4ac2d8 (no main form). The reference serves as
a cross-check. Captures may confirm a result but do not define behaviour. New code
lives in `client/wlo/`, one package per original unit, and every ported
function cites its reference address. The earlier `client/ui` screens remain
until the port replaces them.

The reference decompile is a slightly different build from any available
executable. It contains code only, and Ghidra missed most methods reached
through VMTs. Two tools fill the gaps from the WLRI build (`ca19ee087b60`),
which is the same program linked in a different order:

```bash
# Delphi metadata: units, form designs (DFM), and every class VMT.
python3 tools/decompile/delphi_meta.py aLogin.exe var/decompiled/meta
# Decompile the WLRI build, creating functions at every VMT slot.
analyzeHeadless var/ghidra/proj aLogin -import aLogin.exe -scriptPath tools/ghidra \
  -postScript CreateFromList.java var/decompiled/meta/entry_points.txt \
  -postScript CreateMissing.java -postScript DumpAll.java var/decompiled/aLogin_ca19ee087b60_full.c
# Align, annotate and split the reference decompile by unit.
cd tools/decompile && python3 reorganize.py REFERENCE.c ../../var/decompiled/aLogin_ca19ee087b60_full.c \
  aLogin.exe ../../var/decompiled/meta ../../var/decompiled/reference
```

`entry_points.txt` lists every VMT slot target and published method; write it
from `classes.json`. The reorganised output has one file per unit, with each
function annotated with its counterpart in the WLRI build, the class virtual
slots it implements and its published name. 8,010 reference functions match
exactly and 966 more by position. Functions the reference lacks (2,702) are
included from the WLRI build and marked as supplements. Matched functions
differ only where the builds differ, for example record fields moved by 0x88
bytes.

### Traced structure (WLRI build addresses)

Startup (`TForm1.FormCreate` 0x493fb0; the loader `FUN_0049bfdc`, first read from disassembly with `tools/decompile/strace.py`, now decompiled with `DecompileAt`'s raised limits):

- DXDraw display 800×600 at 16 bits; window title "WLO Rhodes Island"; `ClientSocket1` port 6414 (from the form design).
- Picture database created, then `menu\skins\default\` registered lazily (tint 0), then the BMg/JMg archives and `.bls` files.
- `Skins.Flst` holds triples (name, colour, path); the white skin (colour 2113 = `0x0841`) is selected (0x343e80): `menu\Skins\white\` is registered with immediate loading, and the colour goes to the form manager (0x46762c). Because registration keeps the first entry, white only adds names default lacks.
- Forms are created in order and registered with the form manager (0x466f64), which applies the skin colour to each form.

Frame (`DXTimer1Timer` 0x4a1d60), login phase:

1. Login background object (0x40c3c8): `pic\LogPic1.jpg` through the canvas, then `Icon_LoginLogo_1` at x = 800 − width.
2. Login timeout: after 30 s without a reply, "Can't reach login server" and back to the server list.
3. `Web01`, three frames, at (0, 555); hover changes the frame.
4. Form manager: per-form tick (0x46753c), then each form's update/draw in list order, stay-on-top forms last, then the hovered control's hint (0x467148).
5. Flip.

Forms:

- Server list (paint 0x3ff65c) draws `Form_ServerList_1.jpg` at (246, 130).
- Login (paint 0x40063c) draws `Form_IdPassword_1.jpg` at (267, 135) and `Icon_RecordAccount` at (327, 304).
- `TSe_Form`'s update (0x468a58) keeps forms on screen. A form whose bottom passes 565 is moved to top = 575 − height. The login form is 800×600, so its top becomes −25, and every login control is drawn 25 px above its constructor position.
- Bringing a form forward (0x467058) moves it to the end of the draw list.

Input: the window's mouse handlers (0x496784 move, 0x4968f8 up, 0x496ca4 down, 0x497c64 double-click) pass to the UI handler (0x40f5d0 down, 0x40f70c move, 0x40f4f4 double-click), which calls the hovered control's slots: +0x38 down, +0x30 click, +0x3c, +0x48 move, +0x2c double-click.

Ported so far:

- **`client/wlo/surface`:** the 16-bit DelphiX surface. Colour-keyed blits clipped to source and destination.
- **`client/wlo/picdb`:** `TSe_CachePicDB`. Directory registration where the first registration of a name wins, case-insensitive names, and deferred loading. The bitmap conversion follows `FUN_0047b274`:
    - pixels with every channel below 8 get blue 8;
    - pure green becomes the key 0;
    - every other pixel except exactly (0,0,8) gets a saturating per-image tint, then is packed as RGB565.
- **`client/wlo/text`:** the bitmap text renderer (`FUN_00489754`), styles 0–2.
- **`client/wlo/seui`:** the se_UI framework: `TSe_GrBasic`, `TSe_Component`, the form manager and input handler, `TSe_FixedButton` (including its centred icon, `FUN_0046e478`), `TSe_panel`, `TSe_Form`, `TSe_ScrollButton`, `TSe_SelectText` (now with `Insert`/`IndexOf`) and `TSe_Editor` (`editor.go`, ported from the disassembly: character filter, double-byte input, caret scrolling, password stars, 300 ms caret).

#### Decompile tooling notes

- `unit_dump.py` used to drop every line mentioning a stack temporary, hiding real calls. It now drops only declarations and Ghidra's return-address and exception-frame bookkeeping. Ports written before this fix were rechecked; only the button icon draw had been missed.
- Ghidra had missed 451 functions with ordinary prologues (for example the server list's component setup at `0x3fecf4`). They were decompiled with `DecompileAt` and merged into the full decompile in address order (backup `*.c.bak`). The list is `var/decompiled/meta/missing_prologues.txt`.
- For virtual calls, Ghidra lists stack arguments last-declared first. The Go ports keep Ghidra's order, so for `Init` it is (name, left, clipH, clipW, clipY, clipX, useClip, height, width, top), and for `SetBounds` it is (left, top, height, width).

### Login screens (ported: `client/wlo/seui/combo.go`, `client/wlo/login`, `client/wlo/app`)

**Combo box** (`TCJ_ComboBox`, constructor `FUN_0047af90`; `TAccountList` adds slot +0x94):

- It is an editor plus:
    - a list `+0x24c` named "TextList";
    - a drop button `+0x250`;
    - a scroll bar `+0x254`;
    - at most 5 items (`+0x248`);
    - the back image `Icon_AccountBack` (`+0x258`).
- Setup is `FUN_0047b1dc(button, x, y offsets, scroll offsets, thumb, rail)`:
    - Button (`FUN_0047b32c`, default `Btn_ArrowDn_2`, frame height = image height ÷ 3): at `Left+Width+off` and `Top+off`.
    - Scroll (`FUN_0047b438`, thumb `bar_H4` w ÷ 3, rail `Rail_H5`, size 0x11×0x3e, inset 3, caps 7/0x10).
    - List (`FUN_0047b5e0`): at `(1, Height+4)` relative to the combo, height = scroll height, width = `Width−2`, colour 0x841, no icons, no hover, highlight 0xffff.
    - These positions subtract `Abs()` while the form's Top is still 0, so they become relative offsets. Compute them at construction.
- Toggle (`FUN_0047b814`, the button's click):
    - only when the list has items;
    - list and scroll visibility flip, and the top resets;
    - height = rows×20+2, or 0x3e with 4 or more rows; the scroll is hidden below 4 rows.
- Paint (`FUN_0047b6dc`):
    - editor, then, while the list is open, close it if none of button, list or scroll has focus;
    - `Icon_AccountBack` at absolute `(Left, Top−Height)` (a navy canvas fill if the image is missing);
    - then the list.
- Choosing an item (`FUN_0047b868`): `SetText`, close, focus the combo.
- Adding (`FUN_0047b8d8`): a new name drops the last item at 5 entries and inserts at 0. An existing name is removed from the strings only and reinserted at 0.
- `TAccountList` accounts file `user\AccountList.dat` (one per line):
    - load `FUN_00402d48`;
    - save `FUN_00402c30`;
    - right-click deletes an item (`FUN_00402b5c`); an empty list deletes the file.

**Server list form** (`TSe_SelectServer`, constructor 0x3fe778 → setup `FUN_003fecf4`; the form itself is 0×0 at 0,0):

- Background `Menu\Skins\White\Form_ServerList_1.jpg` (`+0x154`), drawn at (0xf6,0x82) by paint `0x3ff65c`.
- An image-less panel `+0x13c`.
- Address list `+0x138` (TStringList).
- Region list `+0x168` and server list `+0x130`, both at (0x10d,0xd2) and (0x19d,0xd2), 0xdc×0x78, with highlight 0xf5bf89, colour 0x841, not embossed, no focus box, selection shown.
    - Region `OnSelect` → `FUN_003fe8e8`; server `OnDblClick` (`+0xf0`) → connect `FUN_003ff4f4`.
- Scroll `+0x134`: thumb `bar_H4` (w 9, h 0x15), track at (0x188,0xe5), 0xb×0xb4, caps 7/0xd, attached to the server list.
- Buttons, sharing the tag handler `0x3fe880`: Up `Btn_ArrowUp_1` (0x188,0xd2) tag 3 → list +0x88; Down `Btn_ArrowDn_1` (0x188,0x19b) tag 4 → +0x8c; `btn_Leave_1` (0x1ab,0x1ba) tag 1 → exit; `Btn_Next` (0x143,0x1ba) tag 2 → connect the selected server.
- 100 name lists `+0x180+i*4` and 100 address lists `+0x310+i*4`.

Show (`0x3fde2c`), unless the global state +0x23d is 1 or 2, reads SERVER.INI:

- A line whose third character is `[` is a region line `NN[Name]…` (NN = 1..99).
- Name = `Copy(line,4,Pos(']')−4)`.
- An optional `<B|G|R>` after `]` gives the colour 0x1f, 0x540 or 0xf800 and shifts the flag position by 3.
- The trailing flag is `StrToInt` of the rest; it is 0 on error or when the line ends with `]`.
- Following lines up to the next region are `name*address` (no `*` gives "No Name").
- Region 95 is stored separately (globals +0x260/+0x264).
- A single name table (`PTR_DAT_004c9dc8`) holds region names at index r and server names at `flag*100+i+1`. That index is the server's status ID.
- Afterwards, `FUN_003ff16c` restores `TopLine` from `user\save.dat`; hide (`0x3fe848`) saves it (`FUN_003ff348`) and clears both selections.

Region select (`FUN_003fe8e8`):

- Fills the server list (with colour tags) and the addresses.
- Sizes the status byte array `+0x150`.
- Maps each server to its status ID by name.
- Queries status from a random server (`FUN_003ff82c`): ClientSocket3, port 0x1910 (6416), with `sound\wav0151.wav`.
- Applies known statuses (`FUN_003ff2f4`, values above 3 become 0).

Update (`0x3ff6d0`) draws a signal light per visible row after the children (`FUN_003ff704`):

- source (0, status×18, 18×18) of `icon_ServerSignal`;
- at (0x202, 0xd0+row×20);
- hovering it shows a tooltip (`FUN_00477864`, text table `PTR_PTR_004c9858`, colours 0xff0000/0xf98b3d/0xffffff).

Status socket read (`ClientSocket3Read` 0x4a6940):

- Buffers across reads.
- Every 3 bytes is a record: u16 LE ID, then the signal.
- IDs of 10000 or more are ignored, which skips the header.

Connect (`FUN_003ff4f4`):

- ClientSocket1 to the server's address, port 6414.
- Sets the connecting flag.
- Hides the server list.

**Login socket** (`ClientSocket1`):

- On connect it sends nothing; the frame code's send function `FUN_002c2394(net, action, sub, …)` sends action 0 (`"\x00"`).
- Received data is XOR-decoded, reframed on magic `F4 44` with a u16 length, and queued (`CY_AddRevQueue`).
- The dispatcher `FUN_002dde1c` switches on the action, then the sub. Packet 1/9 (`FUN_004a7028`):
    - stores a word, a flag and the remaining text;
    - hides the server list;
    - shows the login form.
- Disconnect hides the login forms and shows "Connection lost" in the message form (`PTR_DAT_004ca12c`).
- Error does the same unless the server list is visible.

Login packet 63/4 (send case at 0x2d8e37), in order:

1. 63, 4.
2. u16 LE version `0x4BB`.
3. Account: `Trim`, at most 10 bytes, length-prefixed.
4. Password, length-prefixed.
5. Token length, then a random key byte, then the token: `IntToStr(size of Data\Item.Dat)` XOR key, empty if the file is missing.

Afterwards the password field is set to "12345678" and then cleared.

**Login form** (`TSe_IDPassored`, constructor `FUN_003ffbc0`; Init 800×600 at 0,0, clamped to top −25):

- Background `Form_IdPassword_1.jpg` (`+0x140`) at absolute (0x10b,0x87); `Icon_RecordAccount` at absolute (0x147,0x130).
- Account `+0x148`: a `TAccountList` named "IDField" (uppercases a–z), `Panel26` at (0x16d,0x110), 0x68×0xc.
    - Margins 1, ink 0x1f, paste allowed, max 10.
    - Enter/Tab → `FUN_004001f0`: validate, then focus the password, or show "帳號輸入不正確" for 3 s.
    - Drop-down setup `FUN_0047b1dc(…,"Btn_ArrowDn_1",0,1,0,0,0,−3)`.
- Password `+0x158`: an editor at (0x16d,0x130), 0x66×0xc, password mode; Enter/Tab clicks Login.
- Buttons, all 0x8f×0x14 at x 0x148:
    - `Btn_Login_L` y 0x168 → `FUN_00400420`;
    - `Btn_Prev_L` y 0x186 → `FUN_00400170` (hide, close the socket, show the server list, clear the password);
    - `Btn_applyID_1` y 0x1a4 → web page (do nothing);
    - `Btn_About_L` y 0x1c2 → about (do nothing).
- Checkbox `btn_UnCheck_1`/`btn_Check_1` (0x148,0x14a) toggles Remember (`+0x150`). Remember adds the account to the list after the login reply (dispatcher 0x2ed846).
- Login (`FUN_00400420`):
    - if the account starts with "WR", letters 'O' after the prefix become '0';
    - validate with `FUN_004a91d4` (failure shows "Incorrect name"; an empty password shows "Pwd is empty");
    - send 63/4, hide, and record the time for the 30 s timeout.
- Validator `FUN_004a91d4`:
    - trimmed length at least 3;
    - the first two characters, upper-cased, must be two non-blank characters;
    - with the "WR" prefix, the rest must be numeric and in 1..4,500,000 (`FUN_004852f8` maps 4,500,001..8,999,999 down);
    - otherwise length at most 10.
- Show (`0x4006d4`): focus the account field, clear the password, load the account list. Hide clears the password.

**Frame** (login phase of `DXTimer1Timer`, 0x4a1fd0):

1. Clear the back buffer and the hovered control.
2. Draw the background object (`0x40c3c8`): `pic\LogPic1.jpg` at (0,0) through the canvas, then `Icon_LoginLogo_1` at x = 800 − width.
3. Login timeout: 30000 ms after the 63/4 send, the notice "Can't reach login server" (2 s, notice object `PTR_DAT_004c9950`), then the account form's Previous.
4. `Web01` (3 frames stacked vertically, at (0,555); frame 1 under the pointer in x 0x30–0xec, y 0x240–0x24d; frame 2 while pressed there). This client has no picture of that name, so the original draws nothing; neither does the port.
5. The manager's tick and draw.

**Packets in the login phase** (dispatcher `FUN_002dde1c`, action then sub):

- 1/9: server description (above).
- 1/6 (0x2deded): the account form returns with both fields cleared and "Password wrong" for 2 s. The login timer keeps running.
- 63/1 (0x2ed7ec): character list. Character selection (`FUN_00402468`) shows its form and stops the login timer, and a remembered account is added and saved. Only the timer and the account list are ported.
- 63/2: goes to `FUN_0015db98` (not traced).
- 0/n (0x2de0b0): the disconnect reason. Texts for subs 1–38 and 40–82, "Connection lost" otherwise. Codes in the bit set at 0x2f01e4 (or game states 5–16) read "text:code", others "D/c:code". The text replaces "Connection lost" in the disconnect message.

**Sound**: `FUN_00404fa4` plays `sound\wav0151.wav` when a socket opens (Mode 0 only).

**Tooltip** (`FUN_00477864`): a box ending at (x, y), 8 px per character plus 6 wide and 20 px per 40 characters plus 4 high, kept on screen. It is drawn as an alpha-200 fill (signal tips `$F98B3D`), a 1-px pen (`$FF0000`), and style-2 text, white on 0x841. The light names are Offline, Good, Active and Crowded.

#### Verification

Login automation, live-server opt-ins and reference-frame evidence are described
in [login checks](TESTING.md#login-checks).

#### Approximations still in the port

- **Notices**: the objects at `PTR_DAT_004c9e40`/`PTR_DAT_004c9950` are not traced. Notices are drawn centred at y 0x1e0.
- **Message form**: `PTR_DAT_004ca12c` and the return path `FUN_0049a2e8` are not traced. A disconnect or connection error shows its text as a notice, stops the login timer and reopens the server list.
- **Alpha blending**: DelphiX `FillRectAlpha` is approximated with an 8-bit blend of expanded pixels.
- **Code page conversion**: the Windows-locale branches (`FUN_00486a84`/`FUN_00486a70`) for code-page conversion and the "TH"/"TF" account dialog are not taken.
- **Unnamed object reset**: the object at `PTR_DAT_004ca958` that is reset when connecting is not identified.
- **Send queue**: action 0 is queued until the socket connects. The original's send thread (`+0x42c`) is not traced.
- **Input mapping**: double-click uses the Windows default 500 ms. Key repeat is 500 ms then every 2 frames. Non-ASCII typed characters are converted to Big5 and delivered as two characters.

### Character selection (ported: `client/wlo/login/selectchar.go`, `status.go`; `client/wlo/seui/sebutton.go`)

**Form** (`TSe_SelectCharacter`, constructor 0x400804, paint `FUN_004017e8`, show `FUN_00402a90`):

- **Panels**: left at (0x28,0x87) and right at (0x1a4,0x87). Each draws `Form_SelectRole_Empty_*` or `Form_SelectRole_Info_*.jpg` through the canvas. `_L` is used only when the content level is 0. The content level (`FUN_004a3b50`) is 1–5 from the size of `jma\001.jma`; this install's file makes it 5.
- **Header icon**: `Icon_SelectRole_1` at (0x11a,0x50).
- **Empty slot**: `Icon_Empty` at (0xa6/0x222, 0x128).
- **Buttons** (`TSe_Button`, below):
    - Enter: `Btn_Login` at (0x137/0x2b3, 0x1b3), tags 1/3.
    - Create or delete: at (0x137/0x2b3, 0x1ca), tags 2/4, showing `Btn_CreateCharacter` or `Btn_DeleteCharacter_1`.
    - Previous: `Btn_Prev_1` at (0x172, 500).
    - Turn: arrows tagged 1–4 cycle the slot's direction through 8–15 (`0x402920`).
- **Element icons** (`TRe_ElementIconPanel`): `Icon_Element_<n>_1` at ((i−1)·0x17c+0x107, 0x9a), with hints None/Eart/Watr/Fire/Wind.

**An occupied slot shows**:

- the name at (+0x37, +0x6c), width 100, white style 2;
- Lv digits at (+0x37, +0x87);
- `Icon_Career_2` at (+0xac, +0x2d) when there is a job, and the job icon at (+0xe3, +0x30) with its name in a tooltip;
- HP, SP, EXP and gold at x +0xd9 and y +0x48, +0x57, +0x68, +0x79.

These are drawn by `TSe_MainStatus`'s drawers (`FUN_00261730`, `FUN_00262128`, `FUN_00262370`, `FUN_00261e7c`, `FUN_002625c0`, `FUN_002618bc`):

- **Digit sheets**: `Num_White3_1` is 10 rows; `Num_MainInfo` is height ÷ 11 rows, with '%' at row 10 and '/' at row 11.
- **EXP bar**: `Panel38` filled to the level progress inside `Panel37`.
- **Level curve** (`FUN_0036e78c`): the EXP to reach level L is Trunc(L^p + q).
    - Normally p and q come from `Data\Formula.Dat`, a single 407-byte record of version 2: a double at 0xf1 (3.1) and an int32 at 0x169 (5).
    - For reborn characters p = 3.3 and q = 50, or q = (L−150)^4.9 above level 150.
- **Sprites**: each slot also draws a body and a portrait sprite (THuman). They are behind the `RoleView` interface and not drawn yet.

**Roster record** (63/1, `FUN_00402468`): after the subcommand, each record is

1. slot;
2. name length n, then the name;
3. level, element;
4. six u32 values;
5. body and head (one byte each, plus a high byte);
6. colour1, colour2;
7. reborn and job (one byte each);
8. six u16 equipment IDs.

That is n + 0x36 bytes in total.

The drawers show HP as the record's _second_ u32, then '/', then the first, and SP the same way with the third and fourth.

**Buttons:**

- **Enter** sends 63/2 + slot (`FUN_00402274`). First `FUN_00401fd0` looks for `voice\<role>_nm01.wav` (`_nm02` or `_nm03` on a 1–55 % roll) and, only if the file exists, plays it and enters when it ends. No install has these files (the `voice` folders hold `am_`, `aw_`, `km_`, `kw_` … clips only), so the original always enters at once, as the port does.
- **Create** on an empty slot sends 63/2 + slot; the server answers 1/3, which opens character creation (below).
- **Delete** on an occupied slot hides the selection and opens the password dialog in mode 2 (`Form_InputPw_2`, OK at 0x402a0c, Cancel at `FUN_00402a78`). OK checks the fields like creation does, then sends **35/2**: slot, password and secret code, each string with a length byte (send case 0x2d1a8e). The server's 35/2 reply (0x2eab55): 1 "Delete success" (the slot is cleared), 2 and 3 "Wrong Del Pwd", 4 "Delete failed", 5 "Delete failed, error code: " and the next byte. The [live deletion check](TESTING.md#login-checks) deletes slot 1 on an explicitly selected test account.
- **Previous** sends 63/0 and reopens the account form.

**`TSe_Button`** (`FUN_00467834`): a nine-slice panel whose image row is its own state (+0x118). It has a pressed flag and an optional icon of one or three rows. It is distinct from `TSe_FixedButton`.

**Reference**: full-window screenshots of the original in `client/reference/screenshots/`: `Login/` holds the login phase (server list, account form, a forced error message, character selection and the creation steps) and `In-Game/` the first in-game screens (ship deck, cabin, inventory, chat, a teleport event and the mole minigame). The folder is not tracked. They guide layout only; the decompile decides. See [character presentation checks](TESTING.md#character-presentation-checks) for the selection capture comparison.

#### Server compatibility corrections

The Go server now writes rebirth/job bytes before exactly six equipment IDs,
matching `FUN_00402468`, and sends maximum HP/SP before current HP/SP so the
selection drawer displays current/max. Saved rebirth and class/job values are
transmitted and decoded, including job icons and reborn EXP progression.
These corrections deliberately differ from the legacy C#
serializer's misplaced equipment and reversed health pairs. Original-client
packet captures remain pending.

AC63:3, sent when creation is cancelled on its first step, releases the
creation-name reservation and the chosen slot; the account stays logged in and
there is no reply. Bare AC63:0 from Previous releases the authenticated account and any creation-name
reservation without closing the connection or sending an acknowledgment. The
client can authenticate again on that connection. AC63:2 still selects a slot.
Native AC63:4's optional XOR-encoded item-file size hint is fully consumed before
authentication; truncated or extra fields are rejected. The hint is not used as
an authentication factor.

### Character sprites and native format reference

`THuman` (VMT 0x40fa0c, constructor `FUN_00410e04`) draws bodies (`FUN_00412c50`) and portraits (`FUN_002586c8`) from `jma\*.jma` with `.Jxa` indexes (archive unit 0x2fdc40–0x302cf8). Known so far:

- **`.jma`**: a 14-byte header ("ja", entry count at +10), then 28-byte directory entries: a short-string name of 20 bytes (e.g. `1001.jmp`), u32 size, u32 offset.
- **`.jmp`**: a 14-byte "jm" header (frame count at +10), then 40-byte frame records. Each record holds:
    - a 12-byte name such as `00-1.bmp`;
    - canvas width and height as u16 at +0x0c and +0x0e (256×256 in the files checked);
    - anchor x and y on the canvas as u16 at +0x10 and +0x12;
    - u32 pixel byte count (+0x14), pixel offset from the sprite start (+0x18), width (+0x1c) and height (+0x20);
    - one more u32 (+0x24) that the decoder does not use.
- **Palette**: 1 KB (256 BGRA entries) directly after the frame records.
- **Pixels**: 8-bit palette indexes, uncompressed, rows top-down, each row padded to 4 bytes. Index 0 is transparent.
- **Decoder**: `internal/clientassets/jma.go`, with golden and native tests. The weapon archive `001w.jma` decodes correctly (frame `1001.jmp/00-1` is a sword).
- **Client palette conversion** (`FUN_00301118`), entries 1–255 to RGB565:
    - black becomes green 4, so it isn't confused with the key;
    - pure green and pure blue become the key 0.
- **Colour customization** (`FUN_0030105c`): 16-entry palette ranges are shifted by (value − 4)·25 per channel, clamped to 5..250. The values come from the 45-byte colour block (`FUN_00444e28`); see **Colours** below.
- **`.Jxa`** ("jx" header, sprite count at +10, e.g. 704 in `001.Jxa` for sprites 1000–1703): an 80-byte record per sprite of per-action frame counts. A frame's record index is the sum of the earlier counts plus the frame (`FUN_002fe570`).
- **Body draw** (`FUN_00412c50`, 1,116 lines, over a dozen archives chosen by the character's state): the sprite is `1000 + id`, with the direction at +0x121. The draw itself is `FUN_002fe8e8`:
    - placement from the canvas centre and the anchor;
    - optional flip and mirror;
    - a paletted blit (`FUN_00104bb0`).
- **Not traced**: the path character selection takes through `FUN_00412c50`, and the portrait (`FUN_002586c8`).

#### Player sprites (ported: `client/wlo/role`)

**Archives** (`archives.go`, `lookup.go`, both generated from the disassembly):
- Startup opens 166 jma archives (`FUN_002fdc40` call sites). The table at 0x4aa094 sets each patch archive's first sprite ID (+0x18).
- A sprite's entry is id − first, or id % 1000 when first is 0 (`FUN_00302934`). Its 80-byte Jxa record gives the frame count of each action; the frame record is the earlier actions' counts plus the frame (`FUN_002fe570`).
- `FUN_0045ac44` lists, for the equipment and body-type-0 families, the patches checked in order; the last whose first ID is at most the ID wins.
  - Weapon families start with an ID shift of −1. The `…w_01` and `…w_a01` patches compare against id − 1.
  - A match on `…w1`, `…w1_b01`, `…w1_c01`, `…w2` or `…w3` clears the shift.
  - The WLRI build names patch archives (`002e_1`, `002w_01`, …) that neither its own install nor ours contains; those installs carry the patch sprites in the base archive (for example `002e` holds 2300–2359). Taken literally, the original would draw nothing for them. The port falls back to the base archive when the chosen patch isn't installed, so capes and wings appear. Weapon 2700 still draws nothing: it resolves to the missing `002w_01`, and `002w` has no such sprite.
- Families not in that table (base bodies, `…h` heads, `…f` portraits) are drawn from their own archive.

**Layered body** (`human.go`, `FUN_00412c50` → `FUN_00433318`) for body types 1–4, family `00N` with N = type+1:
1. The base body, sprite 1000·N.
2. Seven layer positions in `FUN_004465f0`'s table order (`order.go`). The default table (`DAT_004c9530`) draws slot 6, 5, 4, 2, head, slot 1, slot 3. The table depends on:
   - **the body sprite's class** (the costume's sprite when slot 6 holds a costume, otherwise the armour's), from a compare tree evaluated into `classes.go`: class 1 or a slot-6 hand byte (disk 0x19f) of 1–2 uses the two-hand tables (`DAT_004c9550`, `DAT_004c9570` for the actions in the bit set at `DAT_004478f4`); class 2 or hand 3 uses `FUN_004478fc`'s tables (`DAT_004c9590`/`95b0`, by pose and attack frame); class 3 or hand 4 uses `DAT_004c95d0`/`95f0`; a few sprites always use `DAT_004c9550`;
   - **the slot-2 item's flag byte** (disk 0x198): 5–7 (body types 1 and 3), 9–11 (2 and 4) and 13–15 select the three variants 8 bytes further on; any other non-zero value gives slot 0 at every position.
   - **Costumes** (slot 6, `FUN_00432c9c`): a second tree gives a category (1, 2 or 6, or 6 from a hand byte of 1–4). With a category, layer 6 draws by the item's type: 12 and 16 (capes, wings) the costume's sprite from the body-armour archive, 13 (hats) from the hat archive. Slot-6 item 0x5400 hides every other layer.
   - The extra armour sprites of `FUN_00448c8c` and `FUN_00448a8c` need the human's state (+0xb1) to be 1 or 2; the selection's humans keep 0, so they aren't drawn there.
   - **Head:** `00Nh`, sprite head + 1000·N + 100. It is hidden when an equipped item's flags (disk offset 0x198) have bit 0x10 or 0x40.
   - **Equipment:** an item's sprite for body type t is `Sprites[t−1]` (1000 means none). Archives by equipment slot: 1 `c`, 2 `e`, 3 `w`, 4 `a`, 5 `s`.
3. Items come from the client's `data\Item.Dat` through the existing decoder (`internal/assets.ParseNativeItems`, documented in `docs/ITEM_FORMAT.md`). The roster's six equipment IDs are placed by each item's equipment slot.
4. Frames advance every 100 ms and wrap at the base sprite's count for the action (`FUN_004122ec`).
5. Placement: the canvas is centred on x; the top is y − (canvas height − 32) + anchor y. Ghidra lost the operands of the vertical term's 64-bit helpers, and this reading aligns every layer.
6. Palette (`FUN_00301118`): index 0 is transparent; black becomes green 4; pure green and pure blue become the key.

**Portrait** (`FUN_002586c8`): `00Nf`, sprite head + 1000·N + 601, action 2, with the blink state as the frame.

**Colours** (`colors.go`):
- **Colour block:** `FUN_004013d0` fills a 45-digit block (+0xd5) with 4s, then writes the roster's colour1 as group 1 and colour2 as group 5 (`FUN_00444e28`). The portrait object copies the block. Each value is read as nine decimal digits in three groups of three:
    - group 1: digits 0–8;
    - group 5: digits 9–11, 39–41 and 42–44.
- **Shift:** every layer, including the head, equipment and portrait, is drawn through `FUN_002fe8e8`. For a player outside battle, it shifts 13 palette ranges by (digit − 4)·25 per channel (`FUN_0030105c`). Ranges 0x10–0xcf use digits 0–35 in order as red, green, blue triples; range 0xd0–0xdf uses digits 39–41. Every channel in a shifted range clamps to 5..250, even at the neutral 4.
- **Exceptions:** sprites 6027 and 9000 always draw neutral (`FUN_002fe888`). The 0xe0 shift is skipped, because the selection screen passes 0 for it.
- **Drawing:** the shifted palette then goes through `FUN_00301118`'s rules into RGB565.
- **Palette indices:** recolouring needs them. Built packs keep them; for the editable export read directly, the sprite manager derives them from each archive's `sprites.json` (see [Sprite packs](SPRITE_PACKS.md)). Frames edited away from their original pixels draw as painted.

Character layer/palette regression checks and capture results are in
[character presentation checks](TESTING.md#character-presentation-checks).

**Not ported yet**:
- The visibility check `FUN_00442c58`.
- Body type 0 and native passengers. Item-vehicle and pet-riding composition
  are implemented; see [riding](CLIENT_ARCHITECTURE.md).

Sprites load through the sprite manager (`client/wlo/sprites`) from the editable data root. Optional packs built with `cmd/sprite-build` are selected explicitly with `-sprites`. The client no longer reads `jma/`. See [Sprite packs](SPRITE_PACKS.md).

### Character creation (ported: `client/wlo/login/createrole.go`, `inputpw.go`, `rolepanel.go`; `client/wlo/role/roleimage.go`)

`TRE_CreateCharacter` (VMT 0x212fcc, constructor `FUN_00215d38`) is a `TSe_FixedForm` (`client/wlo/seui/fixedform.go`). Packet 1/3 (case 0x2ded1a) hides the selection and opens step 1; the byte after the subcommand says the account already has a secret code.

**Steps** (`FUN_00219bfc`; backgrounds `Menu\Skins\White\Form_CreateRole_N.jpg`):

1. **Portraits and name.** Fourteen `Btn_RolePic_N` pictures form a carousel; the arrows animate (`FUN_0046bd0c`) and scroll it towards the stops at `DAT_004bd460`, by `ceil(ms × 0.02)` pixels per frame since the scroll began. Only the hovered picture uses its crisp row. A click chooses the body type and head (`FUN_0021838c`) and dresses the preview in the starter outfit (`PTR_DAT_004ca774`, `FUN_00215c24`). Next checks the name locally (`FUN_004a6d00`, three Big5 word tables in `badwords_table.go`), then sends 9/2 + name; 9/3 answers 0 (next step), 1 "Name already in use" or 2 "Illegal name, try another".
2. **Points.** Five points over STR, CON, INT, WIS and AGI, with the body's bonuses (`PTR_DAT_004c9f28`) shown as "+ N" and hints over the labels (`FUN_00477a70`).
3. **Element.** Four 17×17 tabs (so their labels are clipped, as in the original) and `ElementInfo_N`. Entering this step selects element 1 and, coming back from step 4, resets the colours.
4. **Colours.** Hair, skin, clothing and eye parts edit three digits each (0–9) of the 45-digit colour block (see **Colours** above); Default resets them. The preview is undressed.
5. **Password dialog** (`TRe_InputPwAndDarkPw`, `Form_InputPw_1`), only without a secret code: a password and a secret code, each typed twice, 6–10 characters and different (`FUN_0021a60c`).

OK on step 4 (accounts with a code) or the dialog's OK sends **9/1**: body, 0, head, look, colour1 and colour2 (u32, the digits 0–8 and 9–11, 39–44), element, the points as STR, AGI, WIS, INT, CON, and without a code the two length-prefixed strings. Previous on step 1 sends **63/3** and returns to the selection.

**Preview** (`TJK_RoleImage`, `FUN_002566bc`): base archives only (no patch lookup), the layer order from the slot-2 item's flag byte (tables at `DAT_004bd79d`), the head drawn with the hat or before the weapon, and the portrait (`FUN_002586c8`) with its blink timer (6–8 s, 50 ms).

Creation snapshot, packet and live-account checks are in
[character creation checks](TESTING.md#character-creation-checks).

**Not ported**: stopping voices after 9/1 (`FUN_00405138(0)`).

### World view (implemented core: `client/wlo/world`, `client/wlo/app/world.go`)

**AC3** (receive case 0x2dfdb3) is the player's own character and opens the world. It is read field by field (`FUN_00113b14`) in the layout the server sends: ID, body type, map, X, Y, a byte, head and look, the two colour values, the worn items, a word and two bytes, then the name. The original reads more trailing fields; our server doesn't send them.

**Scene** (`scene.go`, loader `FUN_003f830c`): `Data\Ground.MMG` is a named archive (`clientassets.ReadNamedArchive`, `FUN_0010f154`: payloads, then 29-byte index rows and a count). The record `<map>.map` begins with the scene size, its background layers and a 20-pixel walk grid (`FUN_004121a8`). The client reads it from the export (`ground_data.json`, its `terrain`). Each layer is `<resource>.jpg` from `pic\map_d01.JMg`, `map_c01.JMg` or `map.JMG`, read from their PNG exports (`pictures/map_d01`, `map_c01`, `map`). Map 10017 is the 2432×1792 Ship Deck with one layer, 10001.

The final `optional_grid` is also retained in compiled map records. Mode 1 uses
native isometric floor/wall resources from `images2.BMg` (`FUN_003fab58`,
`FUN_003faa10`, `FUN_003fb2b4`), including Chicken Farm's cliffs. Wall pictures
are depth-sorted using the authored signed 32-bit depth rather than their image
top. This is independent of the WEM object list and applies across maps. Other
grid modes retain their raw export data until their rendering semantics are ported.
Rebuild previously compiled client bundles to retain this formerly discarded grid.

Scenery object placements are signed 16-bit coordinates (`FUN_003f830c` passes
both fields through `short` before `FUN_003f9310`). The export and compiled records
retain their native unsigned bit patterns; the scene loader sign-extends them
before placement and depth sorting. This allows trees extending above/left of
the map to remain visible. Chicken Farm's northern trees include (811, -91) and
(988, -70); treating their Y values as 65445/65466 previously hid them off-screen.
The correction applies to all maps and works with existing compiled bundles;
it requires no extraction, asset rebuild or runtime-format change.

**Names** (`names.go`): scene names come from the `SceneData.dat` export (`scene_data.json`), and each map's scene from the `eve.Emg` export (`eve_data.json`); map 10017 is scene 10001, "Ship Deck".

**Drawing**: the camera keeps the player at the screen centre (the scene is drawn from X − 400, Y − 300; this matches `In-Game/Ship_Deck.png` exactly) but stays on the map (`FUN_003f94e8`, the map's +0xc/+0x10): within 400 pixels of the left edge (300 of the top) it is 0; the horizontal camera clamps continuously at the map's width − 800 − 20 (the original's 0x14) and at the bottom edge at its height − 600 (0 when the map is narrower or shorter than the screen). Before this the port drew black past the map's edges (`Chat_02/Go_Beach_Camera_Black_Edge.png`); at Dango Beach X:1082 Y:1195 it now frames the scene as the original's `Chat_02/Beach_Camera_Clamped.png` does. The player's name follows the player instead of a fixed row. The player is drawn by the selection's layered renderer, with the name centred above (yellow) and the location line `<name> <scene> X:<x> Y:<y>` at the bottom right; both positions are measured from the capture. `world_snapshot_test.go` (opt-in) renders the capture's position.

**NPCs** (`npcs.go`, `client/wlo/role/npc.go`): the map loader (`FUN_003030ec`) creates one character object per `eve.Emg` NPC record (parsed with `assets.ParseEVEMap` from `eve_data.json`'s `decoded_hex`; the record conversion is `FUN_00484a50`) and applies its `Npc.dat` template (`FUN_004265a4`):
- the look is the record's word at disk offset 14, modulo 1000; an NPC keeps body type 0, so the draw (`FUN_00433318`) is one sprite of the `001` family, 1000 + look;
- the four colour values at disk offsets 18–30 fill colour groups 1–4 over a neutral block;
- the record's facing r (1–8) becomes (9 − r) mod 8, and `FUN_00430474(…, 8)` stands the NPC: action 8 + facing (facing 6 is action 11, towards the camera);
- flag bit 0 shows the NPC at load; otherwise it starts hidden.

AC22:4 (`FUN_0038cf30`, 14-byte entries: click ID, state, X, Y, kind, duration, action) moves NPCs and shows (kind 1) or hides (kind 2) them. For props (Npc.dat kind 6, 9 or 10, `FUN_00482424`) the state's low byte fixes the drawn frame (+0x11f; 0xff animates), so a chest stays closed (0) or open (1); other NPCs animate their action. NPCs and the player are drawn back to front by Y; an NPC adds its eve record's depth (+0x1180, copied from the record's +0x6b4, the last trailing int), so the coconut up a palm on Newbie Island (depth 150) sorts in front of the baked fruit. Floor item coordinates are icon centres (`FUN_003d78e4`, `FUN_003d7b80`), with both drawing and click bounds subtracting half the canvas size on each axis; this avoids the former 16-pixel upward displacement of normal 32-pixel icons. They join this same depth-sorted pass at their Y plus the signed offset in the final four bytes of each AC23:4 record. The server preserves the native EVE trailer on map entry and respawn; player drops use zero. This keeps elevated items such as Beach coconuts in front of the appropriate background palms while zero-depth items remain behind native foreground objects (including palm leaf picture 11016004). There are no item-specific masks or overrides; native source pictures and depth lines remain unchanged. Floor-item hover uses opaque source pixels and the same depth order as drawing, with native additive light level 15 and an outlined yellow item name near the pointer. Pickup owns the click: it stops existing movement, announces the current position before the native pickup request, and prevents held-ground input from restarting the route. Distant pickup arrival runs during simulation, including background sessions. The name overlay (`FUN_004147f8`) skips kinds 6, 8 and 9. Templates on the tall name base (Npc.dat +0x57 = 1) draw their sprite 0x44 lower, 0x20 more on the doubled scale (`FUN_002fe8e8`); their name and shadow stay at the feet (Burke the tiger). The talk window's body mode draws through the same placement, so the drop applies there too (`In-Game/Burke_Talk.png`). Verified against `In-Game/Newbie_Island_Props.png` and `Newbie_Island_Coconut_Hover.png`. On Ship Deck the eleven NPCs stand where the capture shows them, and their colours match it exactly.

**AC12** (case 0x2e20cf: ID, map, X, Y, portal) places the player; the map reloads when it changes, then the client acknowledges with 12/1 as `FUN_002f2fd0` does. The server waits for that acknowledgment before publishing the character and accepting actions.

**Names and shadows**: an NPC's name (its template's name, from the object's +9) is drawn by the NPC class's overlay method (`0x4147f8`, called from the depth-sorted pass `FUN_004219d8`): white (0xffff), outlined (style 2), centred on its X, at Y − h − 0x19 (or − 10 when h ≥ 0x32). The name height h (+0x114, `FUN_004265a4`) is 0x6a minus the anchor Y of the sprite's first frame (0xa6 when the template's byte at disk offset 87 is 1), doubled when the byte at offset 56 is 1; ID-specific heights on the 0xa6 base (`FUN_0018b544` and others) are not ported. The ground shadow (`FUN_0030120c`) depends on the template's byte at offset 74: 0 draws the (49,88)–(80,105) cut of the `Shadow` picture at (x − 15, y − 4), 1 the whole `Monster_Shadow` at (x − 47, y − 18), others none; both pictures come from `pictures/images` and are registered in the picture database at startup (`picdb.Add`). On Ship Deck names and shadows land on the capture's pixels. Players and other players get the same `Shadow` cut (`FUN_0030120c` for a role whose +0xb1 is 1 or 2); the shadow switch (+0x1efd) and the map checks (`FUN_00334120`) are not ported.

**Not yet**: wandering NPCs (walk behaviours other than 1 stay at their record position), the scene's objects and animated WEM resources, other players, the HUD (status, toolbar, F1–F8 slots, chat bar), chat lines, walking, the camera at scene edges, and the origin offset for scenes smaller than the screen (`FUN_003f830c` centres them in 20-pixel steps).

### Scene objects (ported: `client/wlo/world/objects.go`)

The background pictures already contain railings, masts and other scenery, but the original repaints those pieces as objects so characters can pass behind them. The map record's object list (resource, X, Y after the walk grid, read by `FUN_003f830c`) creates `TGroundObj`s described by their `Wem.MMG` records (`FUN_003f5f1c`: resource, a dword, depth line in grid cells, frames, interval in ms, width, height, translucency, kind). Each paints its picture (green key transparent; frames stacked vertically) with the top-left corner at its position (`FUN_003f65cc`). The map paint (`FUN_0041e0d8`) orders them by kind (+0xd6):

- kinds 0–2 right after the background;
- kind 3 in the depth-sorted list with the characters (`FUN_0041d13c`), keyed by Y plus the depth line, after characters with the same key;
- kinds 4–5 after every character. The deck's railings are kind 5 (42 of the deck's 72 objects), so they cross the characters' legs as in the captures.

`TestShipDeckObjects` checks the deck's counts. Not ported: the cull test (`FUN_003f5d70`), translucent objects (+0xd4 other than 0xff, or +0xe0 from the picture's first pixel), and the resources 11011–11015 and 11031 that the loader forces to kind 3. Compiled picture pages use a shared 64 MiB LRU cache; map backgrounds decode only visible tiles. The editable PNG atlas fallback also uses a separate 64 MiB LRU cache.

Scenery shadow smoothing (`world/scenery_shadows.go`) reconstructs complete
2-by-2 checkerboards of transparent key pixels and matching dark pixels as
half-opacity shading. This reduces tree-shadow shimmer during fractional camera
motion without filtering opaque foliage or characters. Detection is shared per
unique scenery picture when loading a scene, respects animation frame boundaries,
and retains the authored source. Both editable and bundled pictures use the same
CPU/GPU alpha path; no extraction or bundle rebuild is needed. Tests cover camera
phase stability, artwork preservation and Chicken Farm's actual scenery.

### Status panel (ported: `client/wlo/hud`, `client/wlo/world/stats.go`)

The top-left panel is `TSe_MainStatus` (constructor `FUN_0025ff84`, painter `FUN_00260fac`), a fixed form shown when the world opens. From its corner at (2, 10) it draws `Main_Status_1`, the HP and SP gauges, the face, the level, the job icon, HP, SP, EXP and gold; the element icon (`Icon_Element_<e>_1` at +0x71, +7) and `Btn_Allrecovery_1` (0x27, 0x6a; hint "Restore HP/SP using Potions and Bottles", no action yet) are children. The value drawers are the ones the character selection already borrowed (`login.Status`).

- **Gauges** (`FUN_00463d98`, `picdb.DrawGauge`): `Main_Hp` and `Main_Sp` are cut along a line from (0x2e, 0) to (Round(p × 0.82) + 9, 24 − Round(p × 0.24)), so the orb fills along its arc; pictures wider than 100 pixels start at 0x3a and 0x15. The fill is the share of the maximum, at least 1 while anything is left.
- **Face**: the role's face sprite in action 0 (the selection uses the boxed action 2), blinking for 50 ms every 6–8 s.
- **Values** (`world.Stats`): 5/3 (`FUN_004381c4`) fills element, HP, SP, the attributes (read as STR, CON, INT, WIS, AGI; this server writes CON, INT, STR, AGI, WIS, and the 8/1 updates that follow set each by ID), level, EXP, the stored maxima and, after the skills, rebirth and job. 8/1 (`FUN_00416ebc`) sets one stat by ID; the bonus stats 0xcf and 0xd0 (and WIS) recompute the maxima with `Formula.Dat` (`FUN_0036e674`, `FUN_0036e704`): level^k × attribute × k + level × k + attribute × k, rounded, plus a base and the bonuses, with level counting 100 per rebirth. The constants match the server's (HP 1, 2, 0.35, 2, 180; SP 1, 3.2, 0.3, 2, 94). 26/4 sets the gold.

On Ship Deck with the capture's values (level 1, 181/181, 100/100) the panel matches the capture apart from the character's own face and element.

The active pet's compact HUD uses the native `007` portrait, white level digits,
HP/SP bars and experience bar/percentage beside the player portrait
(`FUN_0029d3a4`, `FUN_0029dbf0`). It reads the current party roster and active-pet
selection, independently of Inventory's local selection. Native pet stat packets
update it immediately; switching the pet to Rest or removing it hides the panel. Hovering the bars
shows the corresponding numeric values. Portrait anchors, template colors and
the original `Word_*`/`Pet_*` skin artwork are preserved.

**Not yet**: the panel's slide-in (`FUN_00260b90`), the restore button's action, the discussion button's mode, VIP numbers and the save-lock icon.

### Button bars (ported: `client/wlo/hud/buttons.go`)

- **Toolbar** (`TRE_FuncBtnForm`, constructor `FUN_00295fb8`, paint `FUN_00296744`): `Btn_BackGround_1` at (0x1b0, 0), then World PK (`Btn_PlayerBattle`), PK, Jump In, Spectate, Join Team, Make Friends, Trade (`Btn_1`–`Btn_6`) every 0x25 pixels from 0x1ae; Guild Invite (`Btn_7`) is hidden and Events and Lucky Draw (`Btn_8`, `Btn_9`) take the slots from it; Item Mall (`Btn_711`) sits at 0x2fb. Lucky Draw opens its native window, and its rows 2–3 animate every 300 ms while the server reports available rewards and remaining draws. Events and the gold-dependent hint remain unported.
- **Menu** (`TRE_MainBtnForm`, constructor `FUN_002950b8`, paint `FUN_002956ac`): `Btn_BackGround_2` at (0x213, 0x233), then Inventory, Skills, Alchemy, Team, Social, Options (`Btn_11`–`Btn_16`) at their own corners; Pickup Furn (`Btn_BringUpFur_1`, 0x2fb, 0x1fb) stays hidden.

Each button is its picture's width by a third of its height (three state rows). The bars match `Ship_Deck.png`; the buttons have no actions yet.

### Hot-key bar (ported: `client/wlo/hud/hotkey.go`)

`TSe_HotKeyForm` (constructor `FUN_00266098`, layout `FUN_0028f55c`, paint `FUN_00296870`) is the F1–F8 bar: `Form_HotKey_V` (0x38 × 0x112) at the right edge, vertically centred, with eight 0x18 slots at (0x17, 0x1d + 0x1b·i), Toggle View (`Btn_Minimize_1`, 0x25, 0), page arrows (`Btn_ArrowL_3`/`Btn_ArrowR_3` at y 0xf8) that cycle pages 1–3 (`FUN_0028f2d8`, `FUN_0028f2b4`), and the page number `HotKeyPage_<n>` at (0x17, 0xf5). It can be dragged and stays within 800 × 0x226 (`FUN_002968e0`). It matches the capture. Slot icons, per-character bindings, three pages, horizontal/vertical layouts and F1–F8/click activation are implemented. The fully minimised native layout remains separate work; see [hotbar controls](#hotbar-and-logout-confirmation).

### Chat bar (ported: `client/wlo/hud/inputbar.go`)

`TSe_InputBar` (constructor `FUN_0026348c`, paint `FUN_00268194`) is `main_input_1` (0x209 × 0x19) at (0, 0x23f): the mail icon (`Anim_GotMailS_1`'s first frame) at (4, 2), the channel button (0x1f, 2), the whisper field (0x49, 0x58 wide, 15 characters), the message field (0xa2, 0x148 wide, 60 characters, hint "Press Ctrl+V to paste") and Chat Emotes (`Btn_expression_1`, 0x1ec).

- **Channels** (+0x140): 1 World, 2 Local (the start, and again on each world entry, `FUN_00268980`), 3 Whisper, 4 Team, 5 Guild (6 Ally and 7 GM exist but have no button). The channel button shows the channel's name, `btn_channel_<n>_1` (40 × 60, three states; `FUN_00267db0`), with the hint "Switch Channel".
- **Switching**: the paint (`FUN_00268194`, rectangles by `FUN_00020d08` = Bounds) opens the list while the pointer is over the button (0x1c, 0, 0x28 × 0x15 in the bar) and keeps it open while the pointer is over the list (the 0x7f pixels above): `icon_Channelframe_1` with a button for each other channel (`btn_channel_<n>_1`, 0x14 apart from 0x17 above the bar); a click on one picks it (`FUN_00267c5c`). The button's click (`FUN_00269b30`, World → Local → Whisper → Team) only acts while the list is closed, so with the pointer on the button a click changes nothing; its other handler (`FUN_0026527c`) toggles the list. Choosing Team or Guild without a team or guild adds "(System):You are not in a Team" or "(System):You are not in a Guild" and returns to Local, as `Team_Guild_Chat.png` shows. `TestChannelButtonOpensList`.
- **Whisper field**: read-only outside Whisper, and emptied (with the target, +0x18c) when leaving it. It has no picture and keeps its pixel hit test, so it never takes a press itself: the bar's press (`FUN_002695d8`) inside (0x46, 0x240, 0x56 × 0x16) selects Whisper, which gives the field the keyboard (`TestWhisperAreaPress`); a press on the mail icon opens the implemented mailbox. Enter in it (`FUN_00268a30`) looks the name up among the known players (letter case ignored): an empty field says "No target"; an unknown name says "<<No such person online>>" and empties the field; a known one becomes the target (`FUN_00269068`), adding "Whisp to <<name>>", joining the recent whisperers and moving the keyboard to the message field. Losing focus also validates a nonempty recipient: an unknown name clears the field and target and shows "<<No such person online>>" once; valid names resolve without taking focus away from the clicked control. Empty blur is silent (`Chat_03/Chat_Whisper.png`, `TestChatWhisperBlurValidation`).
- **Recent whisperers** (+0x174 in the panel +0x178, `FUN_00268cdc`): another player's whisper puts its speaker before the last name; a chosen target moves to the end; at most ten. On the Whisper channel the list opens while the pointer is over the whisper field (0x46, 1, 0x56 × 0x16) or the list, 0x14 a row above the bar; a pick (`FUN_00268c38`) fills the field and runs its Enter. The panel is created without a picture, so only the names show. `TestWhisperers`.
- **Known players** (`PTR_DAT_004c9788`): every AC4 record (`FUN_00429a38`) is kept, whatever its map, until the player leaves (AC12 to map 0), and names whisper targets and chat speakers beyond the map's players. Chat portraits use the cached appearance across map changes, hide on logout and return on a fresh appearance or online presence update. Gonline Server remembers which character appearances each connection has received and sends those observers native logout/reconnect notifications across maps. It does not announce unknown characters; separate private instances receive presence only, avoiding an unintended map spawn.
- **Speaker icons as targets** (`FUN_00496ca4`, the main form's press): the list's paint records the speaker whose icon is under the pointer (+0x500, from the sprite's own pixel test); a press then, with the channel list closed, whispers to that speaker without the "Whisp to" line (+0x1b4) and takes the press. `TestSpeakerPortraitPress`.
- **Clicking the message field**: the field has no picture, and the constructor clears its pixel hit test (+0xa8), so it is hit by its rectangle; the port had missed this, and clicks fell through to the bar, so the field could not take the keyboard (`TestMessageFieldClick`). The whisper field keeps the flag, as in the original, and gets the keyboard when Whisper is selected.
- **History**: the last ten messages sent are recalled with Up and Down in the message field (`FUN_00265178`, `FUN_002651f8`).

The Chat Emotes button opens the native `panel_expression_1` picker: 30 icons in
three rows plus the 31st at the bottom right. `FUN_0026a008` inserts the selected
two-byte code at the message caret, reserves both bytes before insertion and
keeps the picker open. The codes come from the binary table at `004bdc18`;
for example `:D`, `XD` and `-P`. They travel as ordinary chat text. The chat log
renders them using `icon_expre_1..31`, retaining Big5 character/code boundaries
when wrapping. The picker animates every 500 ms; log icons advance every seventh
30 ms game frame. The message editor previews the same animated icons while retaining their text
codes for sending (`FUN_00269734`, `Chat_03/Chat_Emoji_Preview.png`). Caret
movement, deletion and horizontal scrolling keep emoticon codes whole
(`FUN_0045cbbc`). The picker tooltips retain the codes shown in
`Chat_Tooltip_Emoji.png` and `Chat_Tooltip_Emoji_Go.png`.
`TestChatEmoticonEditorPreview` and `TestChatEmoticonEditingAndHints` cover
rendering, editing and hints. The mail icon opens the mailbox and blinks for
unread letters; the battle/viewer/auto-play buttons remain pending.

### Chat (ported: `client/wlo/hud/chatlog.go`, `client/wlo/app/chat.go`)

- **Log window**: `TTalkMsgForm` (constructor `FUN_0048bbf4`), 0x208 × 100 at (0, 0x1db), in the transparent mode of the capture: `panel_TalkForm2_1` as the strip at the left (stretched from 86 to 100 pixels through its scroll track, with the `bar_H4` thumb filling the track), Chatbox Lock (7, −8), the scroll arrows and Chat Box Switch. Lines go into the message list (`TSe_CharMsg`, 0x32, 0xe, 405 × 60), rows 0x14 apart, newest at the bottom, wrapped to 50 characters without splitting double-byte characters, at most 100 complete messages.
- **Navigation**: arrows, the draggable thumb and mouse wheel scroll the retained messages. Lock holds the current view as other players speak; an outgoing message releases it and follows the newest rows (`FUN_0048c9d4`). The switch cycles opaque, transparent and ticker-only modes (`FUN_0048d2e4`). Transparent text lets clicks through to the map; opaque message rows and speaker portraits can select whisper targets.
- **Resize and move**: the opaque window's native handle changes width/height with even dimensions and a minimum 224 × 72 (`FUN_0048e004`). The HUD bottom is aligned at y 575, matching `Chat_03/Chat_Background.png`; the binary's y-566 resize clamp displaced the port by nine pixels. All modes retain the same size and position (`Chat_Resizing.png`). Unlocked background mode can be dragged with pointer capture across child controls, bounded to the screen and y 578 (`FUN_0048c32c`, `Chat_Move.png`). Lock prevents dragging. The last 100 complete messages are retained and rewrapped with Big5/emoticon boundaries intact. A locked view follows its message and byte offset when wrapping changes. `TestChatNativeResizeCapture` and `TestChatResizeRewrapAndLockedAnchor` cover input and layout.
- **Background and ticker**: opaque mode tiles the native colour-keyed `32X32Grid` within the stretched frame (`FUN_0048d410`). The ticker follows the window, sits 20 pixels above its bottom and fits its width without splitting Big5 characters. `TestChatReferenceGeometryAndDrag` checks the background, shared geometry and drag/lock capture.
- **Lines** (`FUN_0048ac28(form, speaker, text, colour, channel)`): channel 1 reads "(World)" + name + ":" + text, and likewise 2 "(Local)", 3 "(Whisp)", 4 "(GM)", 5 "(Team)", 6 "(Guild)" and 7 "(Ally)". Channel 0 is the text alone with a speaker and "(SystemPromp):" + text without one, and also joins the ticker. Channel 10 is the client's own messages as given, 11 "(System):" + text and 12 "(Bouquet):" + text.
- **Speaker icons** (`FUN_004905e8`): a row of channels 1–7 whose speaker is the player or a player on the map shows the speaker's face sprite (`<family>f`, 601 + head, as the portraits) in action 4 at its second frame (+0x11e = 1: the head turned three-quarters; the first frame faces front), at the list's left − 10 and the row's top + 10. `Chat_Icons_02.png` (the original) and `Chat_Icons.png` (the port, before the fix) show the difference; `TestChatSpeakerIcon` checks the action and frame.
- **Colours**: channel → entry of the table `FUN_003beb14` fills (0x72a360): system 7 (0xf800 red), World 1, Local 9 (0xff80), Whisper 10 (0xfc00 orange), GM 10, Team 6, Guild 4, Ally 5, channel 10 7 (red); the table is fe31, ffb3, c6f3, 8653, 6e7e, 8e27, ffff, f800, f4a3, ff80, fc00. Every sent and received line first copies the player's `ChannelColor1`…`5` setting (Local, Whisper, Team, Guild, World; read from the character's `user\save` file by `FUN_00284434`) into its channel's slot (+0x4e0 + channel × 2); without a save the defaults are 9, 10, 6, 4 and 1 (0x284d05). The Channels settings window now persists local palette choices; reading legacy save files is not ported. `Chat_02/Connection_Lost.png` shows whispers in orange (the port had them peach before, `Chat_02/Go_Whisper_Colors_Tooltip.png`).
- **Ticker** (`FUN_0048d688`): channel-0 lines also queue for a ticker drawn at (0x32, 0x22b). It starts as 60 spaces; every tick its first character is dropped and, while it is shorter than 0x36, the message's next character is appended, so a message types in from the right. Each message plays three times. The tick is 100 ms, which puts the welcome message where `Ship_Deck.png` caught it 33 ticks in.
- **Welcome**: on the first world entry (`FUN_00492ac4`) the log empties and "Welcome to [<server>] Server" is added on channel 0 with the player as speaker (so no prefix and no icon), with the server's name from 1/9 ("Wonderland Gonline Server" from this repository's server).
- **Receiving** (the cases at 0x2df0fb…0x2dfa42, table at 0x2df0b7): 2/n for n up to 7 carries the speaker's ID and the text (cut to 60 characters except 2/0 and 2/4) and is shown on channel n with the speaker's name from the map's players; 2/5 needs a known speaker. 2/16 (case 0x2dfd44) shows its text on the notice board for 2 seconds. (2/3 is Whisper, not a notice, as the port had it before.)
- **Sending** (`FUN_002641c4`, Enter in the message field): by channel,
  - World: needs a radio in the special slot (equipment slot 6: Radio Set 34076, Loudspeaker 34132 or Transceiver 39070, `FUN_00452fc4` → `FUN_003d1018`), else "(System):World Channel requires Radio Set" (`World_Chat_02.png`); with one, at least 15 SP ("(Alert):Not enough SP to use Radio Setq"). Sends 2/1 and adds the line at once.
  - Local: sends 2/2 and adds the line at once (`Local_Chat_02.png`).
  - Whisper: an empty whisper field says "No target" (`Whisper_Chat_02.png`); no target says "No target" too; a target not among the players says "Player is offline". Sends 2/3 with the target's ID before the text; the line comes back from the server. On map 0x29cd it is "Temporary unavailable" on the notice board.
  - Team and Guild: send 2/5 and 2/6 (lines come back from the server); without membership they say "(System):Not in Team Chat anymore" or "… Guild …" and return to Local.
  - "/msg <name> <text>" (`FUN_002687f0`) switches to Whisper and, for a player on the map, takes them as the target, adding "(GM):Whisper <name>" (the original's channel-4 call) and sending the rest. The whisper field stays as it was, so with it empty the send still says "No target", as in the original.
  - A refused message keeps its text; a sent one empties the field and joins the history.
- **Tests**: `TestChatChannels` and `TestChatWhisperAndWorld` replay the Chat captures; `TestChatPrefixes` covers the channels.

Server side (`internal/server/chat.go`, `chat_channels.go`): 2/1 goes to every player in the world except the sender (the sender's client logs it); 2/3 [target][text] goes to the target and back to the sender as 2/3 [sender][text]; 2/5 goes to the party, the sender included; 2/6 reaches online SQL guild members, including the sender; the guild-management UI remains pending. The `/world`, `/team` and `/whisper` commands send the same packets, `/world` echoed to the sender. GM broadcasts are 2/4, which the client shows as "(GM):" lines.

This build never shows chat speech bubbles: `FUN_00428ed0` jumps past its own body (`jmp 0x428f6c` at 0x428f06), and nothing else sets the bubble text (+0x2318) that the role's bubble drawer reads.

Not ported in the Go client: GM ban controls, the radio's 5-second charge, the Loudspeaker's World-line variant, alternate backgrounds, links/VIP marks and the announcement banner (`FUN_0048d9b8`). Server administration already supports account/IP bans. The Team window is covered in [CLIENT_ARCHITECTURE.md](CLIENT_ARCHITECTURE.md); the guild interface remains pending.

### Walking (ported: `client/wlo/world/walk.go`, `client/wlo/app/world.go`)

A left click that no control takes walks the player (`FUN_0043bc70`):
- **Grid**: the scene's 20-pixel walk grid; a cell is walkable when `cell & 0x17` is 0 (0x13 from 0x4c94c8, plus 4). On Ship Deck 0 is the deck and 1 everything else.
- **Path** (`FUN_0041a9b8`): only within 41 cells of the player. A blocked target backs off toward the player; a straight line of free cells gives one waypoint, otherwise a search of the 82 × 82 cells around the player gives the turning points. Waypoints are cell × 20 + (2, 15). The original's own search order (`FUN_0041af9c`, `FUN_0041a1e8`), its target adjustment (`FUN_00419ccc`, `FUN_0041a348`) and the other maps' pathfinder (`FUN_003cbb2c`, for maps 0xf80d–0xffdc) are approximated by a breadth-first search.
- **Movement** (`FUN_004126b8`): 0.16 pixels per millisecond (+0x1ee0, set by `FUN_00410e04`), waypoint by waypoint; the camera keeps the player centred.
- **Facing** (`FUN_0041218c`): eight directions, 0 up then anticlockwise (1 up-left … 7 up-right); a diagonal when |dy| / (0.001 + |dx|) is between 0.25 and 2. Walking uses action = direction, standing 8 + direction.
- **Holding**: arrow keys walk 0x50 pixels from the player on each held axis, re-planned every 400 ms (`FUN_004a4248`, skipped while a text field has focus). Holding the left button after a ground click re-aims the walk at the pointer on the same 400 ms cadence; the original's mouse-hold code was not found, so this follows the key walk.
- **6/1** (send case at 0x2c33cc, sent per leg by `FUN_0041897c`): the facing and the waypoint, then 8 bytes of an anti-cheat checksum from the role's timing table (+0x3ae0). This server reads only the first five; the client sends zeros for the rest.

Other players' 6/1 movement and stop poses are handled. Remaining movement work includes the camera at scene edges and native stall/ride restrictions. NPC click and door/area handling are described below.

**Walk marker** (`FUN_0049bbf4`, `client/wlo/world/marker.go`): a mouse walk, a click or a held button's re-aim (0x4a1d60), shows the marker at the walk's destination (+0x84/+0x88: the route's last waypoint, or the player's position when no route is active) and restarts it. Every 120 ms (0x78) it steps through the skin's `Arrow2`, `Arrow3` and `Arrow4` (64 × 64, centred on the destination), drawn after the map, then hides; a step within 120 ms of the previous one keeps the old picture until the interval has passed. Arrow-key walks show none. A click on unwalkable ground moves the target back along the line toward the player to the first walkable cell (`FUN_0041a348`), so the marker lands on the edge of the walkable area. Once the player reaches that edge, repeated clicks and held-button re-aims keep the marker at their feet (`Waypoint_Legacy.png`, corrected from `Waypoint_Go.png`). `TestWalkMarker` and `TestWalkMarkerBlockedArrival` cover this; the marker matches `Walk_Marker.png`'s.

**Animation speed** (`FUN_004122ec` → `FUN_00411f54`): a role's frame advances every 100 ms, or every 230 ms (0xe6) while it is at rest (+0x123, set when a walk ends and on most standing paths). The port uses 230 ms for the standing actions (8–15) of the player, other players and NPCs, and 100 ms for walking; the login previews keep 100 ms.

### Other players (ported: `client/wlo/world/peers.go`)

- **AC4** (receive case 0x2e0878, record `FUN_00429a38`): another player, kept when its map is ours: ID, body, element, level, map, X, Y, a byte, head (a word), the two colour values, the worn items, a dword, three bytes, the name (then the nickname and trailing bytes). It is dressed like the player (layered body, colours, worn items) and drawn in the depth-sorted pass, with its name in cyan (0x7ff, the overlay's ink for other players) 100 pixels above its feet, as far as the player's own.
- **6/1** for another ID (case 0x2e0f02): it walks to the point along a planned path (`FUN_00418968`, `FUN_0041b218`) at the player's speed; the player's own echo is ignored, as the original looks the ID up among other players only.
- **AC7** places it; **AC12** for its ID removes it unless the map is ours (this server sends map 0 when a player leaves).
- **Chat**: 2/2 lines take the speaker's name from the map's players.

`TestPeers` drives movement and presence with the server's packet builders. Ship Deck, Cabin and starter Wilson/Robinson island scenes are private: the server suppresses other players there. Use a later public map for observer tests.

AC5:0 replaces another player's worn item list, including an empty snapshot;
AC5:8 refreshes its sprite. AC10:1/5 updates nicknames/names, and AC10:2/3 stores
presence metadata without despawning the character. Names and nicknames respect
the separate Info Visibility settings. AC32:2 applies batched ID/action records
and stops movement; movement replies with standing/held actions place the peer
without starting another walk. AC32:1 starts an independent `E<code>` expression
animation, resets it when repeated and expires it after four passes at 200 ms
per frame. It leaves the held pose unchanged. Malformed record batches do not
partially update the scene. See `TestNativePresentationAndAllocation` and
`client/wlo/world/presentation_test.go`.

Expressions and held poses can be selected in the Social window's Emote tab;
its Friend tab displays presence. Player target selection supports Join Team,
Make Friends and Trade Items. Riding and following pets are covered in
[CLIENT_ARCHITECTURE.md](CLIENT_ARCHITECTURE.md). Titles, passengers, additional
vehicle classes and transforms remain pending.

### Talking to NPCs (ported: `client/wlo/app/events.go`, `client/wlo/hud/talk.go`)

- **Clicking an NPC** (`FUN_00303b54`): the shown NPC under the pointer (the front one by feet Y; the hit box is its current frame) gets **20/1** with its click ID as a word. Within 0xa9 pixels on each axis it is sent at once; otherwise a two-second “Too far” notice appears without walking or sending a click.
- **Event frames 20/1..6** (receive case 0x2e3dc0 → interpreter `FUN_00304fd0`): after the command byte, `[sub, byte, word, step, kind, subject, actor u16, mode, value u32, text u16, branch]`, matching the server's `wire.EventFrame`. Kind 1 is a line of dialogue: subject 3 is a map NPC (the actor), subject 7 the player. The text is the talk ID's `Talk.dat` record (`data/talk_data.json`).
- **Acknowledging**: a step the client has finished (+0x7108) is answered with **20/6** by the frame loop (`FUN_00307300`). A dialogue line is finished when clicked; 20/7 finishes the step as well; 20/10 (and the unnamed 11 and 13–17, receive cases at 0x2e3dc0) finishes the step too, so server-driven sequences such as the beach rescue advance at once; **20/8** ends the event and releases the server's hold (player +0x2392 = 0, receive case 8), which is how a door event's 6/2 hold ends after its teleport. While an event runs or the server holds the player (**6/2** with 1), clicks, held buttons and arrow keys do not walk.
- **Talk window** (`TSe_TalkMsgFormPlus`, `PTR_DAT_004c9950`, constructor `FUN_0034bfa4`, face mode `FUN_0034d990`): `panel22` at the top centre, y 0x19, fitted to its content by `FUN_0034ba20`: the longest line × 8 + 110 wide and the lines × 18 + 66 tall; a face adds 0x78 and makes it at least 0x9c tall, a body (body mode, below) adds its frame width + 0x1e and makes it at least its height + 0x46 tall; then 5 more (the Visitor's line gives 0x21e × 0xa1, the cat's "Meow~" about 0xdc × 0x6e, both as captured); yellow lines centred and wrapped at word boundaries; an NPC's face (`Npc.dat` +0x58, a `008` sprite in action 0 with the NPC's colours, blinking every 6–8 s) with its feet at (right − 0x49, top + 0x46) and its name in white 0x24 below; the player's portrait (action 2) at the left instead; and `Icon_Nextone_1`'s two frames swapped every 250 ms at the bottom centre. `TestWorldSnapshot` renders `In-Game/Ship_Deck_Talking.png`'s line and the cat's (`Cat_Dialogue.png`): the text, speaker, name, frame and icon align with both captures. - **The player's lines** (`FUN_0046fe50`'s `Form_Talk` branch, +0x2a8): a wide form instead, `Form_Talk_1` (the blue skin, +0x38c = 1; `Form_Talk_2` is the pale one) at (0x23, 0x195), 0x300 × 0xa9, not fitted to the text; lines of 0x2d characters (+0x294, 0x28 in the normal window), left-aligned at 209 and 0x2e from its top; the next icon at its centre. The face sprite's large art (action 3) stands at x = half the **body** sprite's frame width in action 3 (`FUN_00437cd4` measures the body) + 0xa7, its feet 0x1e above the form's bottom (`FUN_0034d990`), without a name. In the game modes 1, 3 and 4 (`PTR_DAT_004ca2f8` +0x1d; the HUD is hidden then) the form sits 0x19 lower and the art 0x19 lower again (`Talk.Lowered`). `TestPlayerTalkSnapshot` renders `In-Game/Player_Dialogue.png` (body 4's art 5607, talk 20306): frame, text and art align with the capture. Not ported: the original's check that the worn look matches its table (`PTR_DAT_004ca774`) before choosing the wide form — the port uses it whenever the art exists — and the modes that hide the HUD.

- **Speakers** (`FUN_0034c814`): an NPC whose template has a face sprite, and the player, use face mode. An NPC without one (the Persian Cat) uses body mode (`FUN_0034cb7c`): its whole sprite, standing facing down-left (action 0xb), with its feet at (right − width/2 − 0x1e, top + height + 0x14), its name 0x10 below; `Npc.dat` bytes 86 and 87 lower the sprite and the name.
- **Markup** (`FUN_00478140`, scanner `FUN_00477e2c`, styles `FUN_00470f48`/`FUN_00471028`): an opener `#X` counts only when `/#X` follows; both are removed. `#R` is red (0xf800), `#W` white, `#B` bold, otherwise yellow; `#n` inserts the player's name, `#e` a line break, `#g` the castle owner ("No guild own Castle"); `#F1`–`#F3` (face expressions) and `#P` are removed; `#s` carries a seven-character sound name and plays `sound\<name>.wav` — the Persian Cat's `#swav1541/#sMeow~`. Sounds play when the line shows, since the typing that reaches them is not ported.
- **Voices** (`FUN_0034d3e8`): the original also looks up a recorded line, `<talk ID>[_10|_20|_30<nn>].ogg`, in `Data\odd.dat` and `odd_d01.dat` (exported to `data/audio`). Gonline selects the native speaker suffix and decodes recordings off the UI thread through one session worker with a latest-only pending queue. Dismissal/replacement cancels pending playback, and session shutdown stops the worker. The shared PCM cache holds its lock only for lookup/publication, so a recording cannot stall cached sound effects while decoding. Player portrait/large-art blinks draw the eyelid patch over the base art, preserving the character's full silhouette.

- **Opening and closing:** Gonline deliberately improves the dialogue presentation while retaining the native protocol and fitted layouts. Text appears immediately. The panel grows by eight pixels per edge over 100 ms, using elapsed time and easing on both dimensions together. Logical dismissal is immediate. The complete message, including its text and portraits, closes over 50 ms on a transparent render target without receiving input. Hidden geometry remains available for up to 80 ms so a following line can take over smoothly; the new line cancels the preceding closing image. Repeated server close packets do not restart the timer. `TestTalkAnimation` and `TestTalkTransitionCanBeInterrupted` cover these rules for plain, portrait and wide player dialogues; `TestBreillatPlayerToNPCDialogue` replays the captured player-to-Breillat handoff. The Legacy reference (`FUN_00472664`, `FUN_00472868`) instead grows four column steps followed by four row steps and reverses the axes on close, withholding content until opening completes. That frame-dependent collapse is intentionally no longer used.
- **Shared message presentation:** `seui/message.go` supplies the same opening curve to Talk, timed system notices, battle outcomes, connection messages, settings choices, logout, party/pet confirmations and inventory quantity/use prompts. `Form.AnimateMessage` opts message forms into decorative motion; child layout, hit boxes and text stay usable immediately. Module windows keep their existing behavior. Dismissal/expiry releases controls immediately and closes the complete message over 50 ms, without an empty-frame interlude. Battle outcomes retain their six-second timeout, ignore clicks and keep scrolling for long summaries; notice durations and confirmation callbacks are unchanged.

- **Questions** (kind 6, `FUN_00304fd0` case 6): the frame's text field names one of the map's questions, EVE category 7 (copied to +0x5d9c by the map loader, 0x392fc0): a header whose last byte is the form (0 the talk window; 1 and 3 other pickers, not ported, which the client closes with 40) and 5-byte entries `[index, value u16, kind, type]`, kind 1 a talk text and 2 an item, type 1 the prompt, 2 a Yes/No button, 3 an option, 4 not shown (`client/wlo/world/questions.go`). With options (`FUN_0034d00c`) the window keeps the prompt's size and lists the options below it: the rows start 0x12 − 6 above the text's bottom, 0x14 apart, from 18 px inside the window's left to 23 px inside its right, in a 2-pixel frame of (140, 190, 247); a hovered row gets a bar of that colour behind its yellow text; options wrap within the row width. The window grows by the rows, 8, and 0x20 for the Close button (`Btn_Close_1`), placed 0x2d above the bottom. `TestQuestionSnapshot` renders Robinson's question (map 10003, `In-Game/Question_Robinson_Hover.png`): the prompt, face, rows, Close button and frame align with the capture. Two Yes/No entries and no options show OK and Cancel (`FUN_0034d150`, 0x14 either side of the centre in a 0x28 row). Replies go out as **20/9**: option row r sends 30 + r ("3" + the row, `FUN_00307ca4`; the events branch on 30, 31 …), OK 20 and Cancel 21 (`FUN_00307c4c`, `FUN_00307c78`), Close 40. In mode 0 the step is also done, so 20/6 follows; mode 1 sends only the answer. More than five options scroll in the original; the port lists them all. `TestQuestion` answers map 10005's question 1.
- **Player questions** use the compact upper dialogue form with a small left portrait, even when ordinary player dialogue has large artwork. This applies to option lists and Yes/No questions (`FUN_0034d00c` / `FUN_0034d150`). The portrait anchor uses 45 pixels plus half the body sprite width in portrait action 2 (`FUN_0034d990`), preventing it from covering the prompt. Xaolan's map 12002, question 1 is covered by `TestXaolanPlayerQuestionShowsBothOptions`: both native choices render and send AC20:9 followed by AC20:6. Set `XAOLAN_QUESTION_SNAPSHOT` to render the reference layout.
- **Names**: while the talk window is drawn the map leaves out the characters' names, as both talking captures show.
- **NPC routes** (kind 4, `FUN_00304fd0` case 4; `client/wlo/world/npc_route.go`): each eve NPC record ends with up to ten routes ("ParaNode", read by `FUN_0047e5b4` into +0x2bc by their own slot byte): 0x5c bytes of slot, presence, an unused byte, a point count, then points 1..count at 4 + 8k. The frame's actor is the NPC, its value the slot and its mode: 1 walks from where the NPC stands through every point, 3 does the same and hides the NPC on arrival (`FUN_004187d8` with 0x3e7fc18), 2 stands it on the first point and walks the rest. Route walks use speed byte 10 (`FUN_00482980`: byte ÷ 8 × 0.16 = 0.2 px/ms); on arrival the NPC gets back its record speed (+0x46 → +0xd48, also its starting speed, `FUN_003030ec`). The interpreter queues the walkers (`FUN_0030a404`, at most ten) and the frame loop (`FUN_00307300`) holds the step's 20/6 until each stands on its route's last point, so the server's path groups (several frames, one acknowledgement) wait for the slowest walker. Every one of the 1,504 path operations in `eve.Emg` names a route its NPC has. `TestNPCRoutes`, `TestRouteWalk`, `TestRouteModes` and `TestRouteEvent` cover the record, the walk and the acknowledgement. Not ported: the walk grid's occupied cells (`FUN_004234f4`, `FUN_00418164`).
- **Turning** (kinds 11 and 14, cases 0xb and 0xe): kind 14 turns the player toward the NPC in the actor field (subject 1) or toward direction 1..8 (subject 2: a point 10 pixels up, then clockwise, jump table at 0x306ec5). Kind 11 turns the NPC whose click ID is the subject byte toward the player (mode 1), the NPC named by the value (mode 2) or a direction (mode 3, table at 0x306c43). Both keep the action's family (`FUN_00432638` with `FUN_00427150`), so a standing character stays standing. Not ported: kind 11's +0x1efc check and the release in `FUN_0030a344`.
- **Event music** (kind 15, case 0xf): subject 1 makes `BGM` plus the value padded to four digits the event's track (+0x732c), which the music function (`FUN_00307eb8`) plays instead of the scene's; subject 2 clears it and the scene's track returns; the player's map change (`FUN_002f2fd0`) clears it too.
- **Other kinds**: kinds 7–10, 12 and 13 jump to the interpreter's end (table at 0x30502e) and do not finish the step, but this repository's server waits for 20/6 after kinds 9 (player effect) and 13 (Clive), so the port acknowledges them, and every remaining kind, at once.

`TestNPCTalk` covers the click, the frame, the acknowledgement and the end; `TestCatTalk` the body mode and the sound; `TestParseTalk` the markup. Approximations: the wrap width (the capture allows 39 to 44 characters; 40 is used) and the player's face position (`0x2d` plus half a 128-pixel sprite). Not ported: kinds 0 (counters, NPC states and messages) and 2 (a saved position), both acknowledged at once, the item and trade question forms, the window's OK-only mode (`FUN_0034cf44`), scroll bar and typing, and the talk cursor.

### Doors and areas (ported: `client/wlo/world/areas.go`, `client/wlo/app/areas.go`)

A map's areas are EVE category 1 records (the scene loader `FUN_003090f4` → +0x338): rectangles of 20-pixel cells from (x1, y1), counted from 1, to (x2, y2), so the rectangle starts at ((x1 − 1)·20, (y1 − 1)·20) and spans (|x2 − x1| + 1) cells each way. The record's tail holds x2, y2, the door kind (+0x48: 1 none, 2 small, 3 large) and the light's offset from the corner (+0x4c, +0x50).

- **Entering**: `FUN_00303ff4` looks at the player every 200 ms, or sooner after 21 pixels of movement. With no event in progress, `FUN_0030410c` sends **20/8** with the ID of the first area that has events and holds the player, unless it is the area entered last (player +0x3aa6; cleared when the player is outside every area, and by a map load, `FUN_00304898`). The client then waits as for an event: the server teleports (20/7, AC12 …) or answers 20/8. The player stops. The wire subcommand comes from +0x7102 (the send case at 0x2c7f6b), not from the call's constant 4.
- **Door lights**: `FUN_003030ec` adds a "DoorLight" (13 frames) or "SmallDoorLight" (4 frames) effect from `pic\images.BMg` at each door's point; the effects list (`FUN_00403b8c`, after the map paint) draws the ones whose point is on screen, centred on it with the bottom a quarter frame below, one frame every 300 ms, with rodraw2's `ro_Clipper_LightAlpha_ColorKey_Blt` at level 24: each channel adds the source × level/32, saturating (`surface.DrawLight`).
- The server's area test (`world.Area.Inside`) counts cells from 1 the same way, so the client's 20/8 and the server's walk-in check agree on every region.
- `TestDoorTeleportWalks`: after the door's hold, teleport and closing 20/8, the player walks on the cabin map. `TestDeckDoor`: Ship Deck's cabin door (area 1, cells 93,41–96,45, small light at 1877,857) sends 20/8 once and again only after leaving; `TestDeckDoorLight` renders the light where `In-Game/Ship_Deck_Teleport_Event.png` shows it.

Not ported: the door transition helper `FUN_003ba9d8(…, 10)` and `FUN_00173774` that follow the request, and the NPC walk-in areas (20/2, `FUN_003042b4`; 20/3, `FUN_00304478`).

### Movies (started: `client/wlo/movie`, `client/wlo/app/movie.go`)

The client handles the server's AC186 compatibility exchange before movie
completion: `[186,12,id:u16,opaque:3]` produces `[186,9,id:u16]`. It consumes the
server's `[186,9,id:u16,status:u8,reserved:u32]` reply without echoing it or
finishing the event. Playback ends separately with AC20:6. The identifier is
echoed for any received ID, without map or story exceptions. The layouts follow
the existing Go server and Private Server `AC186.Recv9`; the wider meaning of
native AC186 (`FUN_002a84ec`) remains unresolved. Client support for these two
layouts does not establish complete native AC186 behavior.

Story scenes (the shipwreck storm, the beach rescue) are `.sty` movies that the client plays itself: event kind 5 names one by its value (`<value>.sty`), and its mode sets the game mode (1 or 4, `PTR_DAT_004ca2f8` +0x1d), which hides the HUD and lowers the wide talk form (`FUN_00304fd0` case 5). `TMovie` (loader `FUN_0033e18c`, update `FUN_003406c4`, draw `FUN_00340404`):

- **Data**: `data/media/sty/style_data.json` (768 movies). The export's field split doesn't follow the records, so each section is joined back into bytes: an actor is four ints (template — 0xffff the player — start stage, keyframe count, initial direction) and count + 1 keyframes of 45 bytes (facing, x, y, a flag, speed class, pose, three ints, sound index, loop flag, channel), NPCs one more int; the camera ("Map10003": the map whose scene is drawn) and pictures use 33-byte keyframes (x, y, flag, speed …); lines are records 1…n of `[speaker, talk, stage, delay]` (+4, +8, +0xc, +0x10; the loader reads n + 1 records and record 0 is unused, so the export's rows start two ints into it); the timeline gives each stage a wait (ms) and operands (overlay, two flash tracks, music, four bytes). `TestAllMovies` decodes every movie.
- **Playback**: a stage ends when its wait has passed and every active actor (start ≤ stage ≤ start + count) and the camera have reached their targets; then its line (if any) shows, at least for its delay and until the talk window closes; then every active actor takes its next keyframe (target, speed, pose, facing, sound) and the next stage begins; after the last stage the movie ends, the game mode returns, and the step is acknowledged with 20/6. Actors move at the speed class's rate (0.0008, 0.008, 0.08, 0.12, 0.4, 4, 40 px/ms; `FUN_00339598`) along the line to the target. The camera keeps its own keyframe counter and centres the screen on its keyframes (x − 400, y − 300).
- **Drawing**: a separate view of the movie's map (`world.NewView`: the scene, objects and depth sorting, no names or location) with the movie's actors: the player (dressed as the player) and NPC templates. An actor's action is its pose plus its direction (`FUN_00339930`), the direction being the keyframe's facing or the movement's while moving (`FUN_00342948`), reduced to left/right for two-way pose groups (`FUN_00342af0`). The falling group (26/27) plays once and holds its last frame, lying (`FUN_004122ec`). Lines go to the talk window (the player's in the wide form); a click closes a line. Recorded voices use the original talk ID and speaker suffix (`FUN_0034d3e8`), looking in extracted `audio/odd` then `audio/odd_d01`; plain talk-ID recordings search `odd` then `odd_d01`, including Noa's expansion recordings. Player voices follow `FUN_004855a8`, NPC/companion voices follow Npc.dat offset 104 (`FUN_00485688` / `FUN_004856cc`). OGG recordings replace the preceding voice without stopping sound effects. Optimized bundles include both extracted voice directories in their audio pack while omitting source indexes.
- `TestBeachMovieEvent` plays 12008 through the event path (twelve lines, 20/6 at the end); the beach rescue matches the video `Screencast_20261002_224630.mp4` (Robinson kneeling by the lying player).

- **Frames**: a keyframe's +0x19 fixes the actor's frame (n − 1, clamped to the action's last): Robinson kneels (1) while dragging the lying player (2). Otherwise the actor animates at its speed class's interval (500, 400, 300, 230, 100, 50, 1 ms; `FUN_00343068`, `FUN_0033984c`), wrapping.
- **Game frames**: the original's frame is a 30 ms multimedia timer (`timeSetEvent` at 0x495b13 → message 0x8002 → `FUN_004a1d60`). Effects counted in frames (the fade's 5 a frame, a jolt's step a frame) run on that clock (`movie.GameFrame`), while the client still draws at the display's rate.
- **Pictures** (`client/wlo/movie/pictures.go`): image tracks such as the storm's "?" over the captain (`S10238` in `images3`). The header gives the stage span, colour key (0x07e0 green), light level, frame grid, frame time and repeats; a picture moves through its keyframes like an actor, is drawn from stage start + 1 to start + count, centred on its point with its bottom on it, one frame of its vertical strip at a time (`FUN_00342508`); a keyframe's +0xd fixes the frame.
- **Visibility**: an actor is drawn only from stage start + 1 to start + count (`FUN_00340404`), so the storm's passengers vanish at the cabin door after stage 18.
- **Stage effects** (`client/wlo/movie/effects.go`): entering a stage applies its operands (`FUN_003406c4`). The effect sets the overlay (+0xf9a): 7 a red fill (`ro_ARGB(0x7d, 0xfa, 0x7d, 0x7d)`), 8 grey (`0xaf, 0x7d…`), 9 the stage's own colour, each painted over the scene with `ro_Clipper_Alpha_Fill` (`FUN_003346e8`); 0 clears it; 1 darkens and 2 fades the scene out 5 a frame (DAT_0072a090, `FUN_0033d14c`), holding the movie until done. The two shake operands (`FUN_0033d514`, `FUN_0033d9ac`) jolt the camera each frame (1), settle at shrinking intervals (2), hide the background (3), or zoom the window's view 20×15 pixels a step (4, 5, 6).
- **Music and sounds**: the sound table `sound\soundtabel.txt` (`FUN_00493c7c`: the first line is dropped, entries count from 1). A stage's music entry plays as the music (the storm starts BGM0003 in stage 1); stage 1 of a movie without music restarts the map's track; a movie that changed the music restores the map's when it ends; keyframe sounds play once.
- `TestStormMovieEffects` checks the red stages (1, 3, 5), the music and the passengers' stages.

Effects 3, 4, 5 and 10 set overlays 1 (leaves), 2 (snow), 3 (rain) and 10 (steam), which the movie's view runs through the weather layer in place of the map's weather (below).

Not ported yet: looping keyframe sounds, the actors' trails and draw modes (keyframe +0x1d), pictures' light blit (level below 255) and their place in the depth order, and party-member speakers.

### Weather (ported: `client/wlo/weather`, `client/wlo/app/weather.go`)

The weather object (`PTR_DAT_004ca2b0`) keeps particle pools that maps and movies share. The map paint calls its under pass (`FUN_00334580`) after the ground and before the characters, and its over pass (`FUN_003346e8`) after the effects list. A movie's draw (`FUN_00340404`) calls the same passes with its overlay (+0xf9a) instead of the map's weather. `FUN_00334120` picks the kind from the scene record's weather byte (`SceneData.dat` +0x29, the export's `unknown_u8_offset_35`), keyed by the map's scene as the music is:

| Byte | Kind | Pictures (skin) | Scenes |
| --- | --- | --- | --- |
| 1 | 1, leaves | `icon_leaves`, 5 frames | Cherry Grassplot, Kyoto |
| 2 | 2, snow | `icon_Snow1`/`2`, 4 frames | Frost Peak, Shayii Peak, Sky Land, Christmas Prairie |
| 3 | 10, steam | `icon_steam_1`–`4`, 10 frames, additive at level 10 | the hot springs and bathhouses |
| 4 | 3, rain | `icon_Rains` (missing from the install) | none |
| 5 | 11, leaves | `icon_leaves_2` | a test scene |
| 6 | 12, bubbles | `Icon_Bubble_1`/`2`, 4 frames | the seabed, Dragon Palace and sea channels |
| 7 | 13, ribbons | `Icon_Ribbon1`–`6` | Wedding Hall, only while +0x57f2 is set (`FUN_002053b0`) |

- **Spawning**: one particle per interval into the first free slot of the kind's pool: leaves every 900 ms (8 slots), snow every 500 ms (100), steam every 200 ms (200) and bubbles every 200 ms (10). Leaves and snow start at a random x within the view's 800 pixels at its top and fall toward 600 pixels below. Steam starts 500 to 600 pixels below the top and rises to it. Bubbles start 800 to 900 below, within 1000 pixels, and rise up to 300 above it. The target drifts `Random(5)` × 50 pixels to a random side (bubbles rise straight). In movie modes the view is the movie camera plus its jolt.
- **Movement** (`FUN_003339e0` and its siblings, whose float maths Ghidra drops; read from the disassembly): each frame the particle moves round(elapsed × speed) along its longer axis and proportionally along the other. Elapsed counts from its last animation frame, which changes every 200–400 ms (bubbles 200–220). Speeds by `Random(3)` class: leaves and snow 0.01, 0.015 and 0.02 px/ms; steam 0.005, 0.007 and 0.009. A bubble's speed follows its picture (0.02 large, 0.015 small). Once the step passes both distances the particle lands and is freed in the same frame, so the under pass draws none.
- **Sway** (`FUN_00333c24`, `FUN_00335608`, `FUN_00335d7c`): x is offset by an amount that grows a quarter of the amplitude per tick up to the amplitude (`Random(30)` for leaves, 20 for snow, 10 for steam), holds, returns to 0, then swings to the other side. Leaves tick every 50 ms of their frame and hold 200 ms; snow and steam tick every frame.
- **Drawing**: the frame's strip of the picture with its top-left at the particle (`FUN_0045e5bc` colour key; steam `FUN_0045f0f0` additive at level 10).
- **Rain** (movie overlay 3: 113.sty, 12059.sty): the under pass dims the screen (`ro_ARGB(0xc8, 0x28, 0x28, 0x28)`) and the over pass flashes it (`ro_ARGB(0xc8, 0xc8, 0xc8, 0xc8)`) when `Random(400) < 10`. Its drops use `icon_Rains`, which the install lacks, so the original draws none and the port doesn't spawn them.
- **Clock**: the passes run once per painted frame in the original, so the port steps the layer on the 30 ms game frame. `weather.Rand` is Delphi's `Random` (`FUN_00012d30`).

`TestSceneWeather` checks Hilltop Hot Spring's steam and Frost Peak's snow, `TestSnowMovie` checks 11012.sty's snow overlay, and `client/wlo/weather` tests the spawning, the step and the rising kinds. Not ported: the stars (kind 5, `icon_Star1` missing; scene 11009), the ribbons and their wedding flag, the rain's drops, and the extra conditions that add leaves or snow on castle maps (`FUN_004850b8`, `FUN_004850cc`, `FUN_003eac3c`) and on the `BGM0028` maps (+0x5768, `FUN_004533c0`).

### Minigames (started: `client/wlo/minigame`, `client/wlo/app/minigame.go`)

The sport manager (TSportManage, `PTR_DAT_004c9994`) runs the games the server starts from events. The cabin's two arcade machines (`In-Game/Arcade_Machine_01.png`, `Arcade_Machine_02.png`; events 2 and 3 on every ship cabin map) start type 3, hitting moles, with parameter 11000, and type 4, hunting, with parameter 10000. Playable local games now include types 1, 2, 3, 4, 5, 13, 15 and 17. Egg draws (6/22) and slot machines (8/10/19) have client controls, native AC71 requests and server reply presentation; the server now implements atomic purchases and weighted rewards for these five kinds. The three fruit machines (11/12/14) are ported natively: reels, lamps, price and payment prompts, the AC34:1 balance and AC71 replies, with server purchases. Native-client acceptance remains pending. [MINIGAMES.md](MINIGAMES.md) inventories all 22 kinds, references and remaining work. Unsupported types answer a loss so the server's event goes on.

- **Start** (57/1, `FUN_003bd398`: type, u16 parameter, byte): the HUD's forms are hidden, the game object is created, and the start form (`CH_GameStartForm`, constructor `FUN_001b4d0c`, laid out by `FUN_001b5740`) shows the game's explanation picture with Start, Leave and a close button (tags 1–3, handler at 0x1b4f20). Start runs the game and shows its Exit (`CH_GameExplain`, `FUN_001b20c0`); Leave, close and Exit give up with a loss. The server's 20/9 hold follows.
- **Frame**: the main loop updates the game (`FUN_003be358`) and draws it after the map (`FUN_003bdfc8`); the game's picture covers the screen. Ground clicks go to the game (slot +4, `FUN_003bd32c`) instead of walking.
- **Result** (`FUN_003bdfa4`): 57/1 with 1 for a win or 0, and the event step is marked done (+0x7108), so the frame loop follows with 20/6. The server absorbs that acknowledgment (`resultAck`), so it doesn't answer the outcome branch's first line.
- **End** (57/2, `FUN_003bc120`): the forms come back, the game is freed and the map's music plays again.

**Hitting moles** (`minigame/mole.go`, object `FUN_001b3090`, frame `FUN_001b4820`): the parameter picks mice (11000 and 0x4a3f), turtles (0x4608) or rabbits, with their explanation form (`HitMouse_Exp_Form` and its `HitMouse_40s` patch, which half-covers the English "1 minute" line in the original too). Seven holes on `HitMouse` (map.JMG); after a countdown (beeps `Wav1605`, "Go" `Wav1610`, then `BGM0019`) the player has 40 seconds. Targets rise, stay up and sink faster as the clock runs down (`FUN_001b4908`); with fewer than 2–4 busy holes, 1–3 empty ones get a mole (76 %) or a bomb (`FUN_001b4b78`). A hit (`FUN_001b3ce8`, boxes `FUN_001b2b44`) scores 1 with `Wav1611` and the `S10416` sparkle; a bomb costs 3 with `SEB0008` and `Bomb_2`. The hammer cursor swings through shapes 13–15. Three seconds after time runs out, 30 points or more wins. The layout matches `In-Game/Mole_Minigame.png`.

**Hunting** (`minigame/hunter.go`, TSport_Hunter, `FUN_0017e2c8`): monsters (NPC templates 17114, 17174, 17186, 17036, 17064 and 17199, `DAT_004bce04`) spawn on the `59092` forest with the `59091` and `59093` layers, wander at their own pace (0.1–0.22 px/ms, `FUN_0017df40`) with eight-way walking (`FUN_00411fd8`), and after ten walks one goes to the player at (400, 560), shakes the screen (`FUN_0017fc08`) and attacks, costing a life with the body's cry. A click (`FUN_0017f714`, `SEB0057`) takes 50 from every monster under the pointer (250 for the big one); each kill scores 5. The clock ticks every 50 frames from 30; surviving it wins ("You Win"), losing three lives or downing 50 monsters loses. The original runs the update and the draw once each per 30 ms frame, and part of the logic sits in the draw, so the port runs all of it on the game frame and draws at the display rate. `animAtt` (pic\animAtt) and `SEB0090` are missing from this build, so neither shows or plays.

`TestMoleFlow`, `TestHunterFlow`, `TestAdditionalLocalMinigameFlows`, `TestNativeArcadeDispatchAndCleanup` and `TestUnportedMinigame` drive the packets; `client/wlo/minigame` tests the countdown, hits, results, facing, steps, attacks and shots. `MOLE_SNAPSHOT` and `HUNTER_SNAPSHOT` render the games and their start forms. Not ported: a dead monster's last frame (held by time here), the hunt's repeated results until 57/2 (sent once here), and the remaining kinds listed in [MINIGAMES.md](MINIGAMES.md). `ARCADE_SNAPSHOT_DIR` renders the new explanation forms and games.

### Next steps

Use [CLIENT_ROADMAP.md](CLIENT_ROADMAP.md) for remaining work. The address-based
[catalog](ALOGIN_CATALOG.md) is a reverse-engineering index, not a current feature
completion counter. Keep the optional legacy front end and native workbench
while they remain useful for comparison; neither is the normal game runtime.

### Social window (ported in part: `client/wlo/hud/social*.go`, `client/wlo/app/social.go`, `client/wlo/app/emotes.go`)

Ordinary dialogue and confirmations render above the window forms, including
Social, Inventory and Settings. Covered controls do not intercept dialogue
clicks. The dialogue skin remains translucent; movie dialogue retains its
separate scene/zoom pass. See [layering verification](TESTING.md#gpu-parity-and-snapshots).

- **Opening**: the main menu's Social button (tag 5, `FUN_00295678` → `FUN_0027f8dc`) toggles the window held at `DAT_006c70d4` (constructor `FUN_0027c6a4`, class `TSe_EmotiomForm`, a `TSe_FixedForm`; paint `FUN_0027b41c`). It is 0x164 × 0x1ed and stands at (200, 31): the constructor centres it in the main form object's +0x2d0 → (+0x38, +0x3c) area, which every Social capture shows to be 756 × 555 (the fields are untraced). Tabs `Btn_FriendIcon_1`, `Btn_EmuationalIcon_1` and `Btn_setup_2` sit at y 0x2b from x 0x14, 0x55, 0x97; the selected one is drawn pressed (+0xf8). `Btn_Marry` (0xd9) shows only for a married player (+0x3ca4). Close buttons: `Btn_Close_1` at (0x98, 0x1ca) and `Btn_Close_s_1` at (0x130, 0x18).
- **Friend tab** (`FUN_0027eeb4`, mode 2, picture `Form_Friend_2`; rows `FUN_0027b5e8`): five rows a page from the friend table (`DAT_0072a390`, 0x43-byte records). Labels, ticks and job icons step 0x41 down the rows; mail, element and presence icons step 0x42. Per row: the portrait at (0x38, 0x61 + row × 0x41) in action 1 (a player's face sprite; a body-0 entry draws face 0x1f94 of the `008` archive, the GM's cupid); the name at (0x57, 0x3d); "LV n" at (0xcd, 0x49) with `Num_White3_1` (`FUN_002627fc`); the element orb `Icon_Element_n_1` at 0x10e; presence at (0x104, 0x68): `Icon_online_1`, `Icon_Busy_1`, `Icon_inline_1` or `icon_offline_1`; the job icon at 0xb5 (`Icon_Asn` Killer … `Icon_Prophet` Prophet, with hints); nickname and guild values at x 0x84 (y 0x5f and 0x71); `icon_mail_1` over `icon_mailGary_1` at 0x131; a tick box at (0x131, 0x6b) for every friend but the GM. Page arrows `Btn_ArrowL_8`/`Btn_ArrowR_8` at y 0x188 and "page / pages" in `Num_MainInfo` at (0xa0, 400), the slash being its eleventh row.
- **The table**: 14/5 (dispatcher case 0xe/5) carries records of ID, name, six bytes (level, job, +0x17, element, body, head), two colours, nickname, guild and a VIP byte. The client always keeps record 0 as ID 100, "GM中心" (`FUN_003c4130`); IDs 100 and 101 in the list restart the count. Presence comes from the known-player registry (+0x94) when the friend is a known player, otherwise offline.
- **Buttons**: a tick box toggles `btn_Check_1`/`btn_UnCheck_1` and the record's +0x41. Del Friend (`FUN_0027ada0`) with no tick shows "Select friends to delete" on the notice board; one tick asks "Delete this friend?" with the name, several "Delete these friends?", in the talk window, and OK sends **14/4** with each ID (`FUN_00279230`). Send Mail with no tick shows "Select friends to mail".
- `TestFriendList`, `TestFriendDelete` and the opt-in `TestSocialSnapshot` (against `Social/Social_Friend.png`: frame, tabs, rows, portrait and page numbers line up) cover it.
- **Emote tab** (`LAB_0027e890`, mode 1, picture `Form_Emotion_1`; `client/wlo/hud/social_emote.go`, `client/wlo/app/emotes.go`): five Ctrl phrase editors at x 0x4d (y 74, 94, 114, 134, 154; 29 characters) with their `Btn_ctrlN_1` labels at 0x22; the emoticon grid (`TEmotionImage` `E101`…`E119`, four columns from (0x25, 0xb4), 0x22 apart) and the action grid (`TActionImage` `btn_Action_1`…15 from (0xb4, 0xb4)); eight Alt slots at y 0x194 (x 36, 71 … 281). Cells are `TAnimateImage`s (`FUN_00280b58`, `FUN_0028087c`): at rest they show their third frame, under the pointer a $F5BF89 highlight at alpha 0x50 and their frames every 200 ms. An emoticon plays expression 100 + n locally and sends **AC32:1** (`LAB_0027f1e4`); an action plays the pose the table at 0x4bd7bc names (1-based: 0, 8, 16, 26 … action 13 is 46, the kneeling sit, 46..53) turned to the current facing (`FUN_00430474`: an eight-facing family keeps the facing, a two-facing family the left/right half) and sends **AC32:2** with that action (`LAB_0027e814` → `FUN_00430f90`). Dragging a cell onto an Alt slot puts it there (`FUN_0027f44c`); Alt+N uses slot N (`FUN_0027f248`; Alt+1 defaults to action 13, which replaces the earlier 16/17 guess: those are a standing stance); a Ctrl label or Ctrl+N copies the phrase into the chat bar's message field (0x27f140). The original saves slots, phrases and the attractive switch in `user\<id>\hotkey` (`FUN_0027b254`, read by `FUN_00278b34`); the port keeps them in `emotes-<id>.json` beside the hotbar file.
- **Set Up tab** (`LAB_0027eae4`, mode 3, picture `Form_setup_1`; `client/wlo/hud/social_setup.go`): the nickname (`Panel26` at (0x7d, 0x4b), 14 bytes), Online/Busy ticks (0x90 and 0xc7, 0x6b), the birthday (year at 0x46, month 0x83, day 0xad, y 0x88; digits; an unset year shows 1900), the sun sign `SunSign_N_1` in the panel at (0x24, 0xa8) (`FUN_00279c88`'s month table), blood types A/B/O/AB (y 0x146 from x 0x55, 0x32 apart, one choice), and the "physically attractive" `Btn_on_1`/`Btn_off_1` at (0x68, 0x162). The profile comes from AC3 after the nickname (blood type, year − 1900, month, day; `FUN_0027ab64`). Editing the date clamps it (`FUN_0027977c`: years past now become this year, 1000–1899 become 1900, months stop at 12, days at the month's length, a date this year stops at today) and makes the form dirty, which swaps Close for OK and Cancel. OK confirms "Ensure your data is correct. System will be sending bday gift on specified date. Log in game to receive it." and then (0x27a73c) sends **AC10:1** with a changed nickname, and with a full date and blood type **AC10:2** (social code, blood type, year − 1900, month, day) and **AC10:3**; otherwise "No blood type". The social code is ((year / (month + day + 2)) × sign × blood + ID mod 10 + name length) mod 10 from the profile stored *before* the save (`FUN_0027a304`). Cancel restores the saved values (`LAB_0027aa14`). Online/Busy set the local status (+0x3ca9) and send the bare 5/7 (`FUN_0027b188`).
- `TestEmoteTab`, `TestSetUpSave`, `TestSunSign`, `TestClampDate` and the opt-in `TestSocialEmoteSnapshot`/`TestSocialSetUpSnapshot` (against `Social/Social_Emote.png` and `Social/Social_SetUp.png`, both matching) cover the two tabs.
- **Mail** (`TSe_SendMailForm`, constructor `FUN_003bfdec`, paint `FUN_003c2288`; `client/wlo/hud/mailsend.go`, memo `client/wlo/seui/memo.go`): `Form_MailSend_1` at (10, 0x32) with a `TSe_Memo` (`FUN_0046c178`; bounds (0x1e, 0x5a), 200 × 0xbe, 152 bytes, wrapped at (width − 8) / 8 columns and height / 17 rows, the blinking `inputCursor2` caret), Old Mail (0x2a, 0x11a), clear (0x68, 0x11a), Send Mail (0xa7, 0x11a), Close (0x68, 0x13b) and a corner close (200, 0x17). A row's mail icon opens it for that friend (mode 0: the name at (0x32, 0x43), the face at (0xbc, 0xf4), action 2, or the GM's cupid), Send Mail with ticks for the ticked friends (mode 2, which clears the ticks). Send (0x3c03c4) does nothing without text and plays `wav0155`; it sends **14/1** (kind 1, the friend's ID, the text). The original sends mode 2 as one packet listing every ticked friend; the port sends one 14/1 each, since the server takes one recipient a packet. `TestMailSend` and the opt-in `TestMailSendSnapshot` (against `Social/Social_Mail.png`) cover it. The memo types and deletes only at the end of the text and has no scroll bar.
- **14/9**: the original reads it as a list of (friend ID, status) pairs for the friend-status array (+0x4f4, `FUN_00280034`), not as the mail receipt the server sends under that number.
- **Receiving letters** (14/1, dispatcher case 0xe/1 at 0x2e29cc → `FUN_003c2360` → `FUN_001d4f40` → `FUN_001d495c`; `client/wlo/app/mail.go`): the sender (u32), a TDateTime (f64) and the text, which is **the rest of the packet** (`copy(s, 14, len − 13)`). Senders 100 and 101 are filed as System and named after the GM entry, others as Friends, named from the friend list or a player in view. Letters file newest first; an identical letter is not filed twice; a full box (1000) drops its oldest, and past 900 it warns "Mailbox volume at 90%. When full, old letters get deleted". The arrival plays `wav0155` and sets the chat bar's unread flag (+0x155, `FUN_00267c4c`): its `Anim_GotMailS_1` icon swaps its two 0x15-pixel frames every 800 ms (`FUN_00268084`) until no letter is unread (`FUN_00267c54`). Pressing the 0x16-pixel square at (8, 1) of the bar (`FUN_002695d8`) shows the mailbox. The original saves the box in `user\Server<n>\MailBoxMsgLog<id ^ 0x79>\MailData.dat` (`FUN_001d36bc`); the port keeps `mail-<id>.json` beside the settings, saved on every change and read at world entry.
- **Mailbox** (`Form_MailBox_1`, constructor `FUN_001d0ca8`, paint `FUN_001d1294`, `PTR_DAT_004ca0fc`; `client/wlo/hud/mailbox.go`): a `TSe_Form`, 0x25a × 0x187, centred in the main area, with `Icon_MailBoxTitleMash_1` at (0x11a, 0x46) on the inbox, the tabs `Btn_MailBoxTab_1`/`_2` (inbox and sent, `FUN_001d56ac`), Previous (0xb9, 0x140) and Next (0x16d, 0x140) pages ("At last page"), Send Mail (0xb9, 0x15e), Delete (0x113, 0x15e), Close (0x16d, 0x15e) and a corner close (0x22a, 0x19). Ten rows a page, 0x14 apart: the kind ("Friends", "Guild", "System", "Pet", "Normal ") at x 0x24, the first line of the text at 0x5a (cut below 18 bytes with "..." from 21), the name ("Unknown" when empty) at 0x10e and `yyyy/mm/dd hh:mm:ss` at 400, from y 0x62; unread rows yellow ($FFE0), read rows $AD60. The row under the pointer gets RGB(0x9e, 0xfa, 0xfe) at alpha 0x32, the selected one at 0x50. "page / pages" stands at x 0x12a, y 0x140, blue, red past 900 letters. Pressing and releasing a row (`FUN_001d4130`/`FUN_001d415c`) selects it and opens the letter (`FUN_001d41ac`: marked read; the open letter closes again). Delete (0x1d4760) asks for a selection ("No mail selected"), removes the letter ("Letter removed", "Mailbox emptied") and opens the one in its place. Sent letters (`FUN_001d5128`) are filed read: mode 0 with the recipient, mode 2 once without one.
- **Letter** (`form_MailReceive_1`, constructor `FUN_003c0dc0`, paint `FUN_003c151c` and `FUN_003c4370`, load `FUN_003c58f4`, `PTR_DAT_004c9868`; `client/wlo/hud/mailread.go`): a `TSe_FixedForm` at (10, 0x32), 0xfd × 0x173: the date at (0x1d, 0x32), the name at (0x4c, 0x43), the sender's face at (0xbc, 0xf4) (the GM's cupid, or a friend's face), the text in a read-only green ($07E0) memo at (0x1c, 0x58), 200 × 0xbe; text starting `^inviation^` loses the marker and shows `form_invitation2` at (0x13, 0x38). Previous (0x24, 0x11d) and Next (0x24, 0x136) show only when there is a letter before or after; the grey panels beneath show otherwise. Reply (0x65, 0x11d, `FUN_003c12a4`) opens the send form where the letter stood, addressed to the sender; Delete (0xa4, 0x11d, 0x3c5a80) removes the letter and opens the one now in its place; Close (0x65, 0x14b). `TestLetterArrives`, `TestMailboxOrder`, `TestMailPreview` and the opt-in `TestMailboxSnapshot` cover them; no capture shows either form.
- **Make Friends** (toolbar button 6; handler 0x295820 case 0x295e20; `client/wlo/app/friends.go`): the toolbar handler refuses maps 10085 and 12766 ("Not available here", 2 s) and, for Join Team and Make Friends, map 10701 ("Temporary unavailable", 1.2 s); Make Friends also refuses map 10072 ("Not available here", 1 s) and does nothing with 50 friends (not counting the GM entry). Otherwise it arms target mode 5 (`FUN_0049bbcc`) with cursor 5 (`cursor4.ani`); a click on a player (`FUN_004112f8` case 2 → `FUN_003c1628`) sends **14/2** with their ID, or for a friend shows "Already your friendW". The mode ends on a click on something (`FUN_00496ca4`), on Escape and at world entry, and lights hovered players as Join Team does. Not ported: the busy target ("Unable to apply, player is busy"), fishing ("Fishing, can't act"), the GM check ("Can't add to friends") and the interserver check.
- **Friend questions** (14/2 received, `FUN_003bfb44`): "<name> wants to add you as a friend" in the question box (`FUN_0034d8d4`, ported as the Team confirmation), the name from the players in view. Yes (0x3bfc28) answers **14/3** [ID, 1], or with 50 friends shows "Too many friends in list" and refuses; No (`FUN_003bfcb4`) plays `wav0151` and answers [ID, 2]; after 10 s (`FUN_003c23e4`) the question closes and answers [ID, 3]. A minigame refuses at once (`PTR_DAT_004c9994` +4). **14/3** received [ID, result] notices "Player <name>" with " friend accepted" (and adds the friend from the player in view, `FUN_003c1d44`), " refused to add", " ignored" or "Player busy". `TestMakeFriends` and `TestFriendQuestion` cover them.
- **Server**: 14/3's answer byte decides the request: only 1 forms the friendship (both players then receive 14/3 result 1); 2 and 3 are passed back to the asker, who sees " refused to add" or " ignored". `TestFriendRefusalAndIgnore` covers it.
- **Server**: 14/1 deliveries carry the text as the rest of the packet with no length byte (`wire.TextMailPacket`), as the original reads them; the reference server's `PackString` prefix showed as the letter's first character.
- **Old Mail** (`FUN_003c5b40`) shows the mailbox and closes the send form.
- **Compose form** (`Form_MailBox`, constructor `FUN_001947d8`, send `FUN_00193430`, `PTR_DAT_004ca6c8`; `client/wlo/hud/mailcompose.go`): a `TSe_Form` at (0xe6, 0x1e), 0xfd × 0x173, with a blue name field at (0x33, 0x3d), 0xa0 × 0x19, 10 bytes, a memo at (0x1e, 0x5d), 200 × 200, 152 bytes, Old Mail (0x23, 0x12f), clear (0x61, 0x12f), Send Mail (0xa2, 0x12f), Close (0x61, 0x14d) and a corner close (0xcb, 0x19). The mailbox's Send Mail shows it empty; a Sent row opens its letter in it. Send asks for a name ("No target player") and text ("No mail content"); a friend's name sends through the send form (0x3c03c4 mode 0: 14/1, filed under Sent) and closes the form. Compatibility: the memo's ink is $0841, since the constructor's 0 equals the paper and vanishes here. Not ported: letters to other names (14/6 with a Mailbox item; the port answers "Need Mailbox to send", as the original does without one), the guild branch and "Can't send to GM". `TestMailCompose`.
- Not ported: the guild letter form (`PTR_DAT_004ca8e8`), the blocked-sender check (`FUN_0024fd0c`), replies to non-friends through the compose form (the send form is used), VIP marks, the friend-status array used when `PTR_DAT_004ca5a4` is set, the nickname's illegal-word check (`FUN_004a6d00`), and the attractive switch's heart over a matching nearby player (`FUN_0027a304`).
- **Server mismatches**: 5/7 is handled as a refresh request, so the Online/Busy choice is not stored or shown to others. The server's 14/5 records send the reborn byte where the client draws the job icon, then the job, and an empty guild followed by the online flag, which the client reads as the VIP byte.

### Minimap (ported in part: `client/wlo/hud/minimap.go`, `client/wlo/world/minimap.go`, `client/wlo/app/minimap.go`)

- **When it shows**: entering a map (`FUN_003f830c`) sets up the map frame's minimap (`CH_TMiniMapForm_World`, constructor `FUN_0030e100`, setup `FUN_0030e1b0`) when the map's SceneData record has +0x1d = 1 (the export's `unknown_u8_offset_23`; Newbie Island has it, Ship Deck does not). The form is a `TSe_Form` that cannot be dragged. The picture is `<map ID>_Mini_Form` from `pic\images.BMg`, loaded on first use; maps 10031–10040 use 10003's picture, and map 0xea6c never shows one. Without a picture the form hides. Battles, movies and minigames hide it with the HUD.
- **Layout**: the picture at the right edge (800 − width), 0x26 from the top; Minimize (`Btn_Minimize_1`, "Minimize Map") at its top right and Reload (`Btn_Reload`, "Reload Mini-map") left of it. The scale (+0x140) is the map's width divided by the picture's. Minimize (`FUN_0030e5bc`) swaps in `Form_HotKey_Mini` (0x27 × 0x26), centred on the button, with `Btn_Window_1` ("Expand Map") at (10, 10); expanding puts the picture back so that the button keeps its place (the expand path maps 11031–11040 to 11031, unlike setup).
- **Marks** (`FUN_0030e978` → `FUN_0030f27c`, `FUN_0030eb60`): every shown map NPC, door and mine with a sign gets the skin picture `Icon_Sign_<n>`, centred on its point: an NPC's feet, a door's light point (+0x28, +0x2a), a mine's rectangle centre. Signs start as the eve records' +0x3c (the NPC byte the parser used to skip, the areas' and mines' first tail byte; almost all are 0) and **AC22:12** (`FUN_00459d88`: kind 1 NPC, 2 door, 3 mine, the ID, the sign) replaces them; the server marks NPCs with unfinished quest branches with 7, the "?". Then the player's face: the face sprite (2601/3601/4601/5601 + head) in action 4, frame 0, drawn only for starter body and head pairs (`FUN_00484f20`). A map point (x, y) lands at (x / scale + left − 3, y / scale + top − (k − round(y / height × k))), with k 6, 8, 10, 16, 32 or 36 for y up to 1000, 2000, 3000, 4000, 5000 and beyond (`FUN_0030f008`, constants at 0x30f264).
- `TestMiniMap` covers the layout, the signs and minimizing; `TestMiniMapSnapshot` (opt-in) renders `In-Game/Newbie_Island_Props.png`'s minimap: the picture, buttons, "?" marks and face line up with the capture.
- Not ported: transformed looks for the face (+0x20b8), the tiled-map minimap (`CH_TMiniMapForm`, `FUN_0030add4`, built from Ground.MMG's trailing cell grid, `FUN_0030b36c`), the map frame and the world map.

### Trade (ported in part: `client/wlo/inventory/trade.go`, `client/wlo/hud/business.go`, `client/wlo/app/trade.go`)

- **Business menu** (`Form_Business_1`, constructor `FUN_0025b454`, paint `FUN_0025cfc0`, `PTR_DAT_004c9f14`): the toolbar's button 7 (handler 0x295820, case 0x295a25) shows and hides it after the toolbar's map checks, and refuses map 10072 ("Not available here", 1 s). A `TSe_Form` at the main area's right edge (756 − 0x104, 0xb4), 0x104 × 0xd9 (`FUN_00485774` returns 1, the tall layout), with the gold at (0x46, 0x46), Abandon (0xae, 0x44), Trade Items/Pet/Equipment (y 0x60) and Sell Items/Pet/Equipment (y 0x7c) at x 0x15, 0x5e, 0xa7, Close (0x66, 0xb6) and a corner close (0xd1, 0x18). Only Trade Items is ported; the other buttons and Abandon do nothing.
- **Trade Items** (0x25bdf4) refuses below level 25 counting rebirths ("Requires player LV25!"), without the online-time flag ("Not enough online time to use!"; the player's +0x5791 starts clear and **25/44** 2 sets it), on maps 12064 and 11073 ("Can't trade here"), 10079, 10095, 10096, 10097 and 10099–10103 ("Not available here", `FUN_004851a4`/`FUN_00485160`) and 10701 ("Temporary unavailable"). Otherwise it arms target mode 9 with cursor 2 and hides the menu. A click on a player (`FUN_003c674c`) sends **25/1** with their ID; on maps 10079 and 10095 it shows "Can't trade here", during a minigame it closes the trade form, and a player below level 25 gets "Player is below LV25!". An empty click keeps the mode; Escape ends it.
- **Trade form** (`TRe_TradeItemForm`, constructor `FUN_003658a8`, paint `FUN_003676cc`, `PTR_DAT_004ca040`): `Form_TradeItem_1`, a fixed form centred in the main area (0x191 × 0x1e1). **25/1** received (`FUN_003c6b18` → `FUN_00367d80`) opens it for a player in view, or during a minigame closes it. The bag's 50 cells start at (0xce, 0x44); the partner's 5 × 4 offer at (0x18, 0x45) with its gold at (0x5c, 0xd5); the player's own at (0x18, 0xfa) with the gold field at (0x5c, 0x186) (digits, 9 long; above 0x7fffffff "Can't give that much at once!"); the partner's name at (0x5c, 0x2b). Items cover their cell width × height (item table +0x19a/+0x19b) from the first anchor where they fit (`FUN_00366ee0`).
  - A click on a bag item (press `FUN_003673d8`, release 0x366a94 → `FUN_00366d9c`) offers its whole stack. Locked items get "Item locked, can't use@". These are refused silently: non-tradeable items (record flag), instance +0x32/+0x34 set or +0x35 = 1 (item 34515 excepted), items larger than 5 × 4, and a full grid. Offered bag cells get the pink `<w>x<h>Grid_2` background. A click on an offered item takes it back (0x3671a8).
  - **Confirm** (`FUN_003680f4`, (100, 0x1bd)) checks the gold ("Not enough Gold"), sends **25/2** [gold u32][bag slot per anchor] and locks the offer. **Trade** (0x368000, (0xae, 0x1bd)) shows once the partner's **25/3** has arrived and the offer is locked. It refuses during a minigame ("Feature disabled") or when the partner's items, by size, cannot fit around the whole bag ("Inventory full, can't trade", `FUN_00367aac`); otherwise it sends **25/3** 1. The paint shows it again every frame, as the original does. Cancel (0xfa, 0x1bd) and the corner close (`FUN_00367dfc`) reset the form and send **25/3** 2.
  - **25/3** received (`FUN_003c704c`) is the partner's gold and 30-byte item records: ID, count, damage, then the bag record's metadata with bytes 2–7 ahead of bytes 0–1. The gold is coloured by its digits ($0540, $001F, $F81E, $F800 for 4–7). An item too large for the grid shows "Can't trade that much items" and closes the form. **25/2** received (`FUN_003c6bd0`) shows its result (0 "System busy, trade later" … 4 "Trade complete" … 9 "Other player canceled trade", 11 "Can't have few trades", 14 "Can't trade, pet is crafting", others "Interrupted") and closes the form, which sends 25/3 2 as the original does.
- **Server** (`internal/server/trade.go`): AC25 follows aLogin. 25/1 opens both forms at once (the target's trade setting is the consent; failures answer 25/2 1, 2, 5, 8 or 11). 25/2 locks a side's offer of whole stacks and relays it as 25/3. 25/3 1 from both sides commits the exchange in one SQL transaction, then sends the inventory and gold updates and 25/2 4. 25/3 2 ends the trade with 25/2 9 to the partner; a disconnect sends 25/2 1, and other interruptions send 25/2 3 and 9. World entry sends 25/44 2.
- `TestTradeRequest`, `TestTradeForm`, `TestTradeFits` and the opt-in `TestTradeSnapshot` cover the client; no capture shows either form.
- Not ported: the Business menu's other buttons and Abandon, the busy, fishing, stall, spectating and GM-ID checks of the target, the trade lock (+0x20a), "Forbidden transact" (`FUN_00138ddc`), `FUN_0018b334`'s silent refusal, the VIP marks on the trade form, the instance check `FUN_003cf110`, and `DAT_0072a124`, which the form shows when it opens.

### Notices (ported: `client/wlo/app/notices.go`)

The notice board is `TRe_TalkMsgFormPlus_1` (`PTR_DAT_004c9e40`, constructor `FUN_0034e50c`), a talk window used for timed messages through slot +0x9c (`FUN_0034e70c`):
- at most five entries, each with its own duration; a message containing line breaks (`\r`) replaces them all and keeps its last five lines;
- the window's own timer is the newest message's duration; when it ends every entry is cleared, and with several entries one expired entry is removed per frame (`FUN_0034e614`).

Only the notice use is ported. The layout was measured from `Login_Forced_Error_Message.png` with the window's margins: `panel22` as a nine-slice 27 pixels from the top, as wide as the longest line plus 110, 18 pixels a line plus 62; centred lines that wrap at word boundaries, yellow (0xffe0) embossed text. `TestNoticeSnapshot` (opt-in) reproduces the capture; the panel lines up exactly and about 6% of its pixels differ (the dither and the background).

### Ambient and prop sounds (ported: `client/wlo/world/zones.go`, `client/wlo/app/ambient.go`)

- **Sound zones**: after its walk grid a Ground.MMG record lists 6-byte zones (`FUN_003f830c` → +0xc6ab4/+0xc64b4; the export's `unknown_triples`): a centre cell, a sound number (low byte of the third word) and a radius in cells (high byte). Every 500 ms the frame (`FUN_00498aec`, not during movies) finds the first zone whose square holds the player's cell (`FUN_003f925c`) and plays `sound\wav####.wav` around its centre (`FUN_004054f0`): on one of six looping channels, starting within the radius (×20 px) measured vertically, the volume dropping 3 hundredths of a dB per pixel and the pan following the horizontal offset (×10). A channel stops when the player leaves its range (`FUN_004057c8`); a map load or a movie stops all. Newbie Island has 105 zones: the shore's wav0050/wav0052 and the trees' wav0030/wav0031.
- **Props**: AC22:1 (`FUN_00408054`) sets an NPC's frame (+0x11f); once the map is ready, a prop going from 0 to 1 plays its template's `wav####` (Npc.dat +0x5c: wav9900 for chests, casks and the coconut, wav9902 for treasure chests).
- **Map ready** (+0x133d0): the player's AC12 clears it and 5/4, sent after the 12/1 acknowledgement, sets it. Until then prop sounds and area triggers are off, so the arrival's AC22:1 replay of already opened props (the server's `Sync`) is silent.
- System settings control SFX on/off and volume, including ambient channels. `FUN_004057c8`'s native early-return detail remains unported.

### Background music (ported: `client/wlo/app/music.go`)

- **Login**: FormCreate's timer (`CheckStartMusic`, `FUN_004a3c98`) plays `Sound\BGM0013.wav` through a media player and rewinds it at the end; the port loops it from startup, and again after a disconnect returns to the login screens.
- **Maps**: entering a map (0x3bce95) plays `Sound\<track>.wav`, where the track is the scene record's second name at +0x1f (`SceneData.dat`; Ship Deck plays `BGM0007`). `FUN_004048b8` keeps a track that is already playing, so walking between maps of one scene doesn't restart it. All tracks the scenes name exist in `data/media/sound` (`TestSceneMusic`).
- One process-wide audio context serves sound effects and music across workspaces; initialization is synchronized while decoded caches remain workspace-owned. System settings apply music on/off and volume without restarting the track. Event kind 15 overrides/restores map music, and battle scenes select their own track before restoring the map/event track on exit.
- Not ported: each map's saved position (+0x5706, +0x5710), the `BGM0028` override on some maps (+0x5768, `FUN_004533c0`) and the music player form.

### Cursor (ported: `client/wlo/cursor`)

The original uses animated Windows cursors. At startup (0x3bb990) it loads `cursor\*.ani` into `Screen.Cursors` under fixed indices and selects 12 (`cursor1.ani`, the blue arrow); the table is `cursor.Registered`. In game, `Tjo_Cursor` (`FUN_003ba9f8`; per frame `FUN_003bab68`, switch `FUN_003bac58`) maps hover states to those indices: state 0 is 12, or 9 (`cursor2.ani`) while `DAT_004ca00c` is set. It also keeps picture-strip copies of the cursors (`cursor1`… in the skin), whose frame counter nothing in the traced code draws.

The client shows them as real system cursors too, so the system draws the pointer at the display's rate, outside the game's frames, as Windows did for the original. Ebitengine has no public API for custom cursors, so `client/third_party/ebiten` holds a copy of v2.10.0 (used through a `replace` in `client/go.mod`) with a small patch: `ebiten.NewCursorImage` and `ebiten.SetCursorImage`, over its bundled GLFW's `CreateCursor`, which works on Windows, macOS and Linux (X11, and Wayland through XWayland). `client/third_party/ebiten/PATCHES.md` lists the changes for future updates. Each exported ANI frame (`data/media/cursor/<name>/`) becomes a native cursor once, and the window swaps them on the ANI's jiffy timings (1/60 s; 10 jiffies a frame for all but `cursor18`, 6). An unregistered index shows the system pointer. The XOR (inverted) pixels of the ANI frames are not reproduced. The cursor isn't part of the 16-bit screen, so snapshots don't include it. The shape names describe the artwork, since most situations are still untraced.

**Choosing the cursor** (`client/wlo/app/cursor.go`): the UI's update pass (`FUN_00467148`) asks for the hovered control's own state (+0xc5, `FUN_003ba9e8`), and the game's state applies otherwise (`FUN_003ba9d8`). `TSe_Component` sets 2, the pointing hand (`cursor4.ani`), so buttons, fields and panels (and the talk window) show it; `TSe_Form` and `TSe_FixedForm` set 0. State 0 is `cursor1.ani`, or `cursor2.ani` (the white arrow) once a held button has walked the player for a second (`DAT_00828ae0`); other states are their own index. The Login captures `Hand_Cursor.png` (hand over the password field) and `Regular_Cursor.png` (crystal over the form) and the in-game ones agree; `TestLoginCursor` and `TestNPCHover` check them. Battle skill targeting supplies sword/magic/capture cursors; Join Team, Make Friends, Trade Items and supported minigames also select their cursor states. The field was called `Layer` in the port before.

**Hovering NPCs**: the original tests the pointer against each sprite's opaque pixels while drawing it (`FUN_002fe8e8` → `FUN_004106b0`; IDs 0x5dd–0x640 are map NPCs) and draws the hovered NPC lit; the cursor itself doesn't change. The port tests the frame's rectangle and lights the NPC by raising its colour groups four digit steps (+100 a channel), measured from `Highlight.png` against `Pre-Highlight.png`; the original's lighting code is not traced.

## Server selection and login

Both screens are traced from the `aLogin.exe` build in the WLRI install (SHA-256 `ca19ee087b60…`). The earlier `decompiled/aLogin_decompiled.c` in the sibling server repository comes from a different build, so its addresses do not match any available executable. Function names below refer to a Ghidra decompile of the matching build, kept locally in `var/decompiled/` and not committed.

- **Server list** (`FUN_003fecf4`): region and server lists of 20-pixel rows, selection fill `$F5BF89`, text colour RGB565 `0x0841`, arrows, Next, Leave, a `bar_H4` scroll thumb (`FUN_00474150`/`FUN_0047403c`), and one `icon_ServerSignal` per visible server row (`FUN_003ff704`).
- **SERVER.INI** (`FUN_003fde2c`): `NN[Name]<C>ID` region lines in numeric order, then up to 100 `name*address` lines. Server status IDs are `ID*100 + index + 1`. Region 95 is held separately and not listed.
- **Status**: selecting a region queries one random server of that region on port 6416 (`FUN_003ff82c`). The reply is a three-byte header followed by `(u16 server ID, u8 signal)` records.
- **Login form** (`FUN_003ffbc0`): account and password fields (9-slice `Panel26`, at most 10 characters), Remember-account checkbox, Login, Previous, Sign Up and About buttons. Enter in the account field moves to the password; Enter in the password field logs in. Sign Up and About open external pages in the original and do nothing here.
- **Buttons** (`TSe_FixedButton`): frame 0 idle, 1 under the pointer, 2 held. A click fires when the button is released while still held over it.

To regenerate the decompile, run Ghidra's headless analyzer with the scripts in `tools/ghidra/`. `CreateMissing` creates functions that auto-analysis misses, `DumpAll` writes every function, and `DecompileAt` decompiles specific addresses into an existing project, creating functions first:

```bash
analyzeHeadless /tmp/ghidra aLogin -import aLogin.exe
analyzeHeadless /tmp/ghidra aLogin -process aLogin.exe -scriptPath tools/ghidra \
  -preScript CreateMissing.java -postScript DumpAll.java var/decompiled/aLogin.c
analyzeHeadless /tmp/ghidra aLogin -process aLogin.exe -noanalysis -scriptPath tools/ghidra \
  -postScript DecompileAt.java var/decompiled/extra.c 3fde2c 474318
```

`DecompileAt` raises the decompiler's output and instruction limits and writes the reason when a function still fails. It also handles entries that auto-analysis missed because a stray instruction decoded from the padding before them covers the entry, or because the previous function's body swallowed them (`0x4147f8`, the NPC overlay, was one; it is merged into the full decompile). `DECOMPILE_TIMEOUT` sets the per-function limit in seconds (default 3000). With the default limits the packet dispatcher (`0x2dde1c`) failed after about ten minutes; with the raised limits it decompiles (`var/decompiled/dispatcher.c`, merged into the full decompile). Its action cases can also be read from its jump tables: an action byte table at 0x2dde6c indexing a jump table at 0x2ddf34. The startup loader `FUN_0049bfdc` needed the same limits; both are merged into the full decompile, which now has no failed functions.

The first command imports and analyses the executable. The second adds the missed functions (669 for this build) and writes the decompile. Some functions are still missed. The main packet dispatcher (`FUN_002dde1c`) needs `DecompileAt`'s raised limits.

Rendering reproduces the original's 16-bit pipeline:

- Art is truncated to RGB565 and expanded by bit replication.
- In colour-keyed bitmaps, pure green `(0,255,0)` is transparent. Other pixels with zero red and blue gain one blue step, as the captures show; the loader routine itself has not been traced.
- JPEGs decode with libjpeg 6's integer IDCT, colour tables and fancy upsampling (`internal/clientassets/jpeg.go`). All 75 loose JPEGs match `djpeg -dct int` exactly.
- Text uses the game's own bitmap font (`TATPC1.TWN`, below).

`client/ui/reference_test.go` compares rendered frames with captures of the original in `client/reference/screenshots/Login/`. The server list (empty and with a region selected) and the login form match exactly; only the window frame's rounded corners in the captures are masked.

The earlier `-legacy` front end has the following limitations; these do not
describe the normal `client/wlo` login/creation implementation above:

- the login form's −25 px origin and the two form background positions (both measured from captures);
- the caret blink period and the caret position after text;
- the account validity check (`FUN_004a91d4`) and the client version word sent with login;
- message boxes, the signal tooltip and the remembered-account drop-down;
- sounds (`sound\wav0151.wav` on connect);
- whether the server list's second callback is the double-click event.

Notices are drawn centred below the form until the original message display is traced.

## Verified formats

BMg/JMg directories begin with a UInt16 count followed by 32-byte records: a one-byte name length, 23 filename bytes, UInt32 offset, and UInt32 payload length. The decoder validates the complete directory and bounds each payload. Image payloads in the checked archives are ordinary BMP or JPEG. No archive is blindly extracted to resource-supplied filesystem paths.

`font/TATPC1.TWN` is XORed with `0x58`. It holds 256 single-byte glyphs (8×15, one byte per row) followed by 13,910 Big5 glyphs (16×15, two bytes per row): A440–F9FE in code order, then the A140–A3BF symbols. `TATPC1.JPC` has the same layout for the Japanese build. The larger `TATPC2` fonts are not decoded yet.

Ground.MMG prefix decoding follows sibling `decompiled/aLogin_decompiled.c`, function `FUN_004121a8`: UInt32 scene width/height, a byte layer count with three UInt16 operands per layer, UInt16 grid width/height, and X-major raw cell bytes. The first anonymous record has dimensions 1472×960, grid 74×49, and one layer reference to resource 10000. The verified prefix consumes 3,645 bytes. That prefix-only path remains useful for native inspection. The normal client now resolves map/scene records from compiled runtime data or `ground_data.json`, including background layers, collision cells, sound zones and objects; unresolved trailing fields remain preserved. Runtime land traversal uses the native `0x17` blocked mask. See [client architecture](CLIENT_ARCHITECTURE.md).

### Incoming handler details

These addresses refer to the recorded `ca19ee087b60…` client build in
`var/decompiled/aLogin_ca19ee087b60_full.c`. The dispatcher passes a buffer
whose zero-based offset 0 is the subcommand; the action byte has already been
removed. Delphi string-copy positions below are one-based.

- `53/10` (`FUN_00445784`): the monster-book mask update. It reads
  `(index, mask)` byte pairs at buffer offsets 1 and 2, then advances by two
  while a complete pair remains. For each set mask bit, `FUN_00482abc` selects
  a slot from 1 through 8 and the handler sets the discovery byte at
  `*PTR_DAT_004caa18 + 0x3cac + (index - 1) * 8 + slot` to 1. It leaves other
  discovery bytes unchanged, rebuilds the discovered-template list from the
  marked entries, and sets the page count to integer `discovered / 10 + 1`.
  The server currently sends individual template discoveries as `53/9`, also
  an incoming client message (`FUN_0044566c`).
- `184/2` (`FUN_00159bd0`): when the first buffer byte is `2`, the handler calls
  `FUN_00159bfc` with a starting index of 2. That helper reads `(index, value)`
  byte pairs at zero-based offsets 1 and 2, advancing by two until the buffer
  ends or 1001 pairs have been processed. `FUN_00159c48` accepts indices 1
  through 5 via `FUN_001599c0` and stores each value at object offset `5 + index`.
- `199/14` (`FUN_00318ca4`): a system line. It obtains the buffer length through
  `FUN_0001412c`, then copies `len - 1` bytes with `FUN_00014334` starting at
  Delphi position 2 (zero-based offset 1), skipping the subcommand. It fills
  the chat row through `FUN_001372ac` with `(System):` as the speaker.
- `56`: the recorded native dispatcher's case body has no application action,
  so that client ignores incoming AC56. The Go server uses `CommandStall` for
  its additional stall messages. Native stalls use separate AC23 messages and
  AC20 request adapters.

## Daily Lucky Draw (ported: `client/wlo/lottery`, `client/wlo/app/lucky_draw.go`)

The top toolbar's Lucky Draw chest opens the native `Form_LotteryOL` window
(`FUN_0015aaec`), with its draw and close buttons, reward icon and remaining
allowance. The window initially opens below the top-right toolbar and can be
dragged; closing and reopening retains its position for the session.
Clicking the icon opens the `Article` reward preview
(`FUN_0015a3c0`, `FUN_0015aa00`): up to twenty server-ordered rewards in two
columns of ten, with item icons, names and quantities. Escape closes the preview
first, then the draw window.

AC104:1:1 (`FUN_0015b398`) supplies cumulative usage and the ordered reward
catalog. The client shows three minus usage and never loads rewards or odds
from local seed files. A draw cycles preview icons, slows after 1600 ms and
sends exactly `[104,1]` at 2800 ms (`FUN_0015af1c`, `FUN_0015b2c0`).
AC104:1:2 (`FUN_0015b484`) selects the one-based winning slot and updates usage;
the ordinary AC23:5 inventory receipt delivers the item. The animation itself
does not grant items or consume allowance.

Duplicate clicks remain blocked through the server reply, including after
closing and reopening the window. Closing before the request cancels the
animation. Existing server failure banners release a refused draw without
changing usage; a missing reply times out without an automatic retry. Loading,
trades, battles, scripted interactions, movies, minigames and modal dialogs
block new draws. Character entry and disconnect reset presentation state;
server catalog refreshes supply the current allowance, including at UTC midnight.
This daily feature is separate from the Lucky sport and other lottery games.

## Verification and limitations

Use [native-format verification](TESTING.md#native-format-verification) for
decoder checks and their fixture variables. Historical census/transcript details
remain in Git history; they are not current gameplay acceptance.
Current gameplay gaps are tracked in [the roadmap](CLIENT_ROADMAP.md).

### Editable sprite assets

The character renderer loads standard PNG sheets and JSON animation indexes
from `sprites/` under the asset root (`data/sprites`), or the directory selected by `-sprites`. Explicit editable
archives can override compiled sprite records; runtime does not read native JMA/JXA files. All available sprites have
been converted under `data/sprites`. From `client/`, run `go run . -sprites
../data/sprites`; restart after painting PNGs or changing frame rectangles,
anchors or animation sequences. See [Editing sprites](../data/sprites/EDITING.md).

### Native character creation requests

The native AC9:1 sender at `0x2c396c` emits body/head, two packed colour words,
element and STR/AGI/WIS/INT/CON, then (for first creation) two length-prefixed
strings at `0x2c3bd0`: account password confirmation and deletion password.
The server verifies the first against the authenticated account and persists
only a hash of the second. Legacy requests with zero strings or one deletion
password remain supported. Credential payloads are never logged.

DC 30 (`0/30`) is a creation rejection sent before persistence. Server logs
identify its stage: request decoding, password confirmation, name reservation,
starter assets, character construction, world-entry assets or database commit.
`character created` means persistence succeeded; `character created but world
entry failed` identifies a failure after that commit. Creation regressions use
literal native request bytes, a temporary database and optional installed assets.

### Native map-loading synchronization

After selecting an existing character (`63/2`), native aLogin requests the
player-stall list (`23/77`) before sending map-load acknowledgment (`12/1`).
It also sends a sprite-refresh request (`5/7`) and requests the mall catalog
(`23/54`) during this phase. Mall balance (`23/25`) and modern mall refresh
(`75/2`) requests are also handled while loading.
The server answers the list request with an empty list (`23/4/0`) and its
completion marker (`23/102`), following the legacy `AC23.Recv77` handler.
The separate AC56 player-stall backend is implemented; this native browser remains
an empty-list adapter, and the Go client has no stall/vendor form. Sprite refresh responds with `5/8`, character ID and a zero byte;
`5/4` is a state ping requiring no reply. These read-only requests are accepted
during map loading; inventory changes, mall purchases and teleports still
require an acknowledged map.

Native movement (`6/1`) contains a direction byte and two little-endian uint16
coordinates. The observed native client sends 15 bytes including an eight-byte
tail. Following `AC06.Recv1`, the server reads the seven-byte prefix and ignores
trailing metadata. Truncated prefixes and directions outside 0–7 are rejected.

### Investigating connection loss

Start the server with command metadata tracing and a persistent log:

```sh
./bin/wonderland -config config.local.json -debug -log-file var/server-diagnostics.jsonl
```

`-debug` records packet direction, command codes and payload sizes. It never
dumps credential packet bodies, passwords or deletion codes. Selected gameplay
commands have bounded decoded traces, including AC6 movement, AC32 poses and
Lucky Draw; these are additional to ordinary metadata logs. `-log-file` appends JSON
logs to the given file and also writes them to stderr; new files use owner-only
permissions. Omit `-debug` for normal operation.

Command rejections and unexpected read failures are warnings, visible without
`-debug`. They include session, account, character, map and map-ready state.
`world entry sent` means the initial snapshot was written; `map load acknowledged`
means the client finished loading. `client closed connection` distinguishes a
client-side close from a server command rejection. These events help locate a
failure before character selection, during map loading or after world entry.


### Starter ship cabin routing

Player scene-transition actions use the map's authored warp ID. On deck `10017`,
warp `2` leads to cabin/arcade `10027` at `(674,1067)`; the cabin's warp `1`
returns to deck `10017` at `(1814,904)`. Only deck `10017`'s chapter transition
`1` uses the normalized shipwreck/rescue destination. Ordinary cabin transitions
must not be redirected to the beach just because they originate on a ship map.


### Lost connection (ported: `client/wlo/app/lost.go`)

`ClientSocket1Disconnect` (0x49871c) acts only when a login form is open or the player is in the game. It hides the login forms and shows the message form `PTR_DAT_004ca12c` (constructor `FUN_003a5184`): `panel15`, 200 × 150 at (300, 0x50) with 15-pixel margins, an `editorBG` field (10, 0x19, 0xb4 × 0x16) holding the reason ("Connection lost" unless an action-0 packet left one), and four `btn_module_1` buttons (0x38 × 0x14, top 0x6e): Leave (0x20) and Prev (0x66) are shown; Finish (0x3c) and Update (0x66) stay hidden. Then `FUN_0049a2e8(…, 1)` starts the screen transition: each game frame `FUN_0049a484` fills the screen with black at alpha 12 × step (steps 1 to 15) after the scene and before the forms; at step 15 `FUN_0049ba08` keeps the darkened frame and the scene is no longer drawn (+0x565), so the HUD and the form stay bright over a frozen, dark scene without names or the location line. Leave (0x3a54f0) closes the client; Prev (0x3a547c) hides the form, resets the game state (`FUN_00314bf4`) and returns to the server list. Battles use the same routine's modes 1 (entering) and 2 (leaving); see BATTLE_CLIENT_PARITY.md. `TestLostConnection`; `Chat_02/Connection_Lost.png` is the original (left) beside the port before this (right, which showed the server list over the game).

Button captions: `TSe_Button`'s centre-slice drawer (`FUN_004680d8`) ends by drawing the caption (+0x48), recentred by `FUN_004679bc`, in the button's colours and style; the port's buttons had no captions until this form needed them.

Tooltips (`FUN_00466be4`): `FillRectAlpha($F98B3D, 200)`, then a one-pixel $800000 (navy) frame with a clear brush, then the text, white (0xffff) over a 0x0841 shadow. DelphiX's `FillRectAlpha` blends the 5/6/5-bit channels, (src × alpha + dst × (256 − alpha)) >> 8: over the sea in `Chat/Whisper_Chat_01_(tooltip).png` this gives (41, 134, 231) to the bit (`TestFillAlpha16`). The port had an opaque fill and dark text before. `surface.FillAlpha` now uses this math everywhere.

### Server selection health IDs

The status service on port `6416` must advertise the IDs used by the client's
`SERVER.INI`. Each ID is the trailing region flag multiplied by 100, plus the
server's row within that region, starting at 1. For example, the first server
under `03[Dango Island]3` expects ID `301`.

Set `status_server_ids` in the server configuration to the IDs that represent
this endpoint. Defaults are `[1, 101]` for existing local lists. This installation
uses `[1, 101, 301]` to support the local Dango entry too. Missing IDs appear
offline even when the status socket and login service are working. IDs must be
unique and in the range 1–9999; the setting does not change login ports or routing.

### Inventory (ported core: `client/wlo/inventory`, `client/wlo/app/inventory.go`)

The Inventory toolbar button opens the native `TSe_EquipForm2`. Its constructor
(`FUN_0034fad4`), painter (`FUN_0035172c`), five-column/ten-row grid
(`FUN_003638dc`) and mode switch (`FUN_00356214`) are the layout source of truth.
`Ship_Deck_Inventory.png` provides the visual check. The combined form is
386 × 461 pixels; the arrows switch to status-only or inventory-only forms.
The six worn slots surround a separate character preview in battle-ready
action 17 (`FUN_00353384`).
The adjacent arrows select the player or an occupied pet slot; pet views display
the selected pet's name, vitals, attributes and equipment.
HP/SP/EXP use the native Panel31–Panel36 artwork. Combat values display the
cached AC8 words at +0x1ffc–+0x2004 read by
`FUN_0035172c`. The native battle getter (`FUN_004166e4`) calculates different
values and must not be substituted for the inventory display. Text is drawn
transparently with zero paper and skin ink (0x0841), including quantity-one
bag counters. The element control is at (19,56), as
in `FUN_0034fad4`.

Double-click a bag item to use it; drag equipment onto its worn
slot to equip it. Double-click worn equipment to return it to the first empty
bag slot. Dragging normally moves one item (`FUN_00350914`). Hold **Ctrl** while
dragging to choose a quantity (`FUN_003594b8`; the executable's modifier masks
at 00350e50/00350e54 are Ctrl with left/right mouse). Dragging outside the form
asks for a drop quantity. The native `Form_ThrowThing` keeps its baked “Moving
quantity” title for moves; the initial count is the source count limited by
the destination’s remaining stack capacity. The editor takes focus when clicked.
A protected-item server reply asks separately before
sending a destruction request. Escape closes the form and its child dialogs.

HP/SP recovery and pet amity items use **one item immediately** on the current
inventory character, matching `FUN_003548bc` → `FUN_00360a9c` → `FUN_003ddc50`. The arrows below
the character preview cycle through the player and occupied pet slots. Name,
preview, stats and equipment follow the selected character; this selection does
not change the battle pet. There is no consumable target/quantity dialog.
AC23:15 carries `[slot, 1, target:u16]`, with target 0 for the player and 1–4 for
pets. Pet-only food refuses the player target. Inventory and vitals change only
when the server replies. If neither restored vital needs recovery, the client
shows the timed warning “This item cannot benefit the selected target right now.”
without sending a use request or consuming the item. A combined HP/SP item is
still usable when either restored vital is below its maximum. Pet attribute allocation remains pending and its arrows
are disabled; selecting a pet never spends the player's attribute points.

Potential Pills open the native 299 x 520 `Form_Potential_1`, initially with no
target selected. Its five-row roster contains the player and owned pets, levels
and confirmed potential. Selecting a row shows base attributes, cumulative red
potential bonuses and a seated preview inside the animated twelve-mark ring.
The player's roster portrait uses the dedicated small face (body-family base
plus 641 plus head, action/frame zero) at its native 32-pixel canvas placement,
as in `FUN_001cb9e8`; it does not shrink the large status portrait.
Robinson's NPC ID resolves to his canonical pet appearance. Preview bodies use
only available sitting actions; pets without sitting artwork retain their roster
portrait without a standing-body substitute. Native Confirm/Cancel hover images
and the Cancel `[ESC]` hint match the reference. Confirm is disabled before
selection, without pills and while an attempt is pending.
They send AC23:126 `[slot, target]`. AC23:213 distinguishes a rejected request
from a processed attempt; processed attempts can still fail and reduce potential.
Golden pills require potential 10 or higher; every pill respects the cap of 12.
The form stays open after an attempt, counts usable pills of the selected type
across bag stacks (up to the native display limit of 999), and chooses the first
usable stack for each explicit attempt. Each request consumes exactly one pill
only on the server. Replies update the ring, bonuses and count; pending requests
block duplicates. Closing and reopening after the existing five-second timeout
permits a manual retry; nothing retries automatically. A replacement or removed
pet clears the selection. Inventory's separate player/pet selection is unchanged.

AC15:8 restores the pet roster, vitals, attributes, equipment and potential;
AC8:2 updates pet stats without overwriting player stats, and AC15:2 removes
released targets. Character entry clears the old roster and pending attempts.
AC5:3 restores player potential from its native footer, independently of unrelated
AC8 stat 37. Equipment use sends AC23:11 for the player or AC23:17 for a pet;
pet equip/unequip receipts are AC23:23/22. Other server-dispatched items retain
AC23:96, including reward packs, vouchers, tents and rods.

Double-click invokes the native use dispatcher. Right-click invokes the distinct
`FUN_00355d80` contextual handler; ordinary consumables and equipment have no
right-click use action. Tent retrieval and the remaining special contextual
windows are pending their client modules.

**Remote Control** (34058) opens the main `TTH_AutoPlayForm`, using the native
`form_autoPlay_1` skin and control positions from `FUN_001e5ba4`. Opening Remote
closes Inventory once; Inventory may then be reopened alongside Remote. Closing
either window leaves the other open. Character entry hides both Remote windows.
The **Level-up Setting** button opens the separate `THL_AutoPlayFormPlus` window,
with `Form_AutoPlayerPlus_1/2` skins and Player/Pet/Description tabs.
Confirm copies the local settings, starts automation and hides the form. The
controller shortcut remains visible near the lower-right corner; click it to
reopen Remote and press Stop. **Remote Information** controls the white status
readout independently of the shortcut, matching the two automation screenshots.
Opening these windows never consumes the controller or sends generic item use.

Implemented main options:

- **Auto Walk:** repeatedly walks around the starting position through the normal
  walkability/path planner. It preserves an unfinished path and pauses for map
  loading, server movement locks, events, battles and dead characters. The radius
  is a documented Go fallback of 160 pixels per axis until the native offset table
  is verified. Changing maps stops automation.
- **Auto Atk:** evaluates advanced settings, falling back to basic attacks
  (skill 10001), for the player and their own
  living battle pet in existing monster encounters. Native fighter records,
  turn prompts and round-ready packets gate submissions; duplicate ready packets
  do not repeat attacks. It does not initiate battles or automate PvP.
- **Auto Heal:** outside battle, uses suitable positive HP/SP recovery from the
  configured inclusive inventory slot range when the player or battle pet falls
  below its threshold. Locked items, harmful effects and selected discard items
  are excluded. Each AC23:15 request waits for inventory changes and the use ACK;
  equipment/discard requests also wait for authoritative source changes. Missing
  receipts stop automation after five seconds without an automatic retry.
- **Auto Unequip:** requests removal of unlocked equipment with native wear of
  200 or more, for the player and battle pet, only if its complete footprint fits
  in the bag. No equipment or inventory changes are applied optimistically.
- **Auto Discard:** only deletes unlocked item IDs explicitly dragged into its
  five selection cells and confirmed with the option enabled. Right-click a cell
  to clear it. The controller itself cannot be selected or deleted. Selected IDs
  apply to matching acquired stacks as well; destruction is permanent.
- **Auto Leave:** enabled death limits, elapsed minutes, or missing required
  supplies close the connection and show the leave reason. Unchecked conditions
  cannot log the player out. Missing-supply leave applies when Auto Heal needs
  recovery. Death counts reset on Confirm and count each defeat once.

The runtime is transient and stops at disconnect/character entry or controller
loss. Supply requests use the Go server's transactional AC23:15 path, rather than
native AC23:82. Advanced assignments support separate player/pet defaults,
three opening actions, absolute HP/SP recovery thresholds, enemy-HP conditions
and party-death recovery. Left-click a cell to cycle Attack/Defend/Flee,
right-click to clear it, or drag a learned skill or battle item onto it. Confirm advanced changes,
then Confirm the main Remote window to activate them. Closing the advanced
window discards unconfirmed edits. Main Confirm saves a local
`remote-<character ID>.json` profile beside `settings.json`; login restores
configuration without starting automation. Runtime counters and pending actions
are never saved.
See [advanced Remote](REMOTE_ADVANCED.md) for native evidence and priorities.
Native battle-scene rendering and manual direct targeting
are implemented; see [battle parity](BATTLE_CLIENT_PARITY.md).

AC23 replies alone update the bag and equipment. Addition records are additive,
and move/removal/wear/unequip acknowledgments preserve item metadata. Malformed
known packets leave the state unchanged. Equipment changes refresh both the map
character and the inventory preview. Character entry clears the previous bag.
Icons come from the exported picture atlases; names/descriptions come from the
item catalog and are encoded for the native Big5 bitmap font.

Item hover uses the native name hint and `TSe_ItemInfo` (`FUN_00287e0c`,
`FUN_00287f58`, `FUN_0028cc74`): a 196-pixel `panel4` frame beside the cell,
yellow text, two decoded catalog effect lines before type/rank, type names from `FUN_00485a20`, rank from decoded record byte 45,
and non-tradeable flags from word 123 (`FUN_0028cbec`). Equipment requirements
use byte 113, independently of the legacy server Level projection. Descriptions
wrap by Big5 glyph. Native description sizing reserves extra bottom padding.
The new tooltip/move screenshot pairs check these interactions. The exported
WLRI `menu/Skins/default/panel4.bmp.png` includes green transparency markers
among the blue artwork; the picture cache applies native color-keying.
Equipment bonus/socket/forge and metadata-dependent tradeability lines still
need the remaining native item-info rules.

The attribute arrows preview STR/CON/INT/WIS/AGI allocation using the available
point budget (`FUN_00353ed4`). Confirm sends a counted AC8:1 request with a
stat byte and four-byte amount per entry (`FUN_003540c4`'s allocation workflow).
Four-byte amounts prevent the server's compatibility decoder from consuming the
next stat ID as part of the preceding amount. Cancel or closing the form discards the draft. Player
stats change only when the server replies. Pet-targeted stat replies no longer
overwrite player values. Submission is disabled while awaiting confirmation;
after five seconds without a reply it permits a manual retry with a notice.
Derived combat stats stay authoritative during the preview. Native rebirth/class
allocation caps and pet allocation dialogs remain pending; the server validates
the actual allocation. `TestInventoryPointDraftAndNativeRequest` and
`TestNativePresentationAndAllocation` cover budget, packets, replies and resets.
`TestInventoryPointBatchesReachServerParser` verifies all 31 nonempty attribute
subsets against the actual server decoder and allocation rules.

The native Potential window follows `FUN_001cac48`, `FUN_001cb6f0`,
`FUN_001cc3ac`, `FUN_001cc604` and `FUN_001cd574`; inventory requests retain
`FUN_001cce98`'s target numbering. It uses the exported form and mark artwork,
`PotentialFire1`/`PotentialFire2` strips and independent seated role previews.
Normal pills require confirmation when failure would forget learned skills;
the warning lists the affected skills using the server's qualification rules.
Zero-grade removal records in native skill snapshots clear forgotten skills;
the warning considers only skills with a learned grade.
Potential results update the ring, bonuses and sounds without chat messages.
Rejected attempts use a timed warning instead of chat.
Remaining inventory work includes repair dialogs,
pet attribute allocation and secondary container/crafting forms. These are separate native
forms and are not implemented by the inventory toolbar window yet.

For focused checks and rendered samples, see
[inventory checks](TESTING.md#inventory-checks).

## Settings UI

Open **Options** (the sixth bottom menu button). The System window and its
Channels, Info Visibility, Titles and Blacklist panels use the exported white
skin and the native constructor coordinates, checked against the Settings
screenshots in `client/reference/screenshots/UI/`. Escape closes a child panel
first, then System. Disconnects and character changes close all settings dialogs.

- **PVP and PK, Joining Battle, Trading, Party Invites and Chat Channels** use
  authoritative character settings from AC33:2. Changes send native desired-state
  requests and become effective after the server confirms its snapshot. Joining
  Battle is independent of party invitations.
- **BG Music and SFX** have on/off controls and ten volume steps, initially 5.
  Volume follows the native 300 hundredths of a dB attenuation per step; music
  updates without restarting its track. SFX controls also apply to ambient loops.
- **Chat colors** cycle through the native eleven-color palette and update
  all existing and future messages in that channel immediately. The retained
  history is recolored too, so new messages, resizing and mode changes preserve
  the selected colors. **Blacklist** adds/removes case-insensitive
  names and filters their incoming player messages; system/GM notices stay visible.
- **Info Visibility** saves ten native display preferences. Other players' names
  apply to the current world renderer, as do own/other nickname preferences.
  Following pet labels render, but their visibility toggle is not wired yet;
  guild-name, tent-size and Adventure Level presentation remain pending.
- **Chat Box** toggles the log. **Spawn Point** offers Beach, Record and Carnie
  through AC5:17, with eligibility checked by the server. **Log Out** returns to
  server selection; **Exit** closes the client. Both ask for confirmation.

Local preferences live in `var/client/user/settings.json`, written atomically
outside the generated assets. They apply to this client installation; server
permissions are stored per character in `wonderland.db` and never loaded from
that local file. `app.Options.SettingsPath` supplies a separate profile/test path.

Zoom Mode now toggles the battle's distance-based zoom without scaling the HUD.
Pending: title entitlement synchronization/selection,
Security Lock, Change Login, Change Password, Official Site integration, Balance
Inquiry, redemption, VIP and point-purchase dialogs. Their controls show an
unavailable notice; the title list remains empty until authoritative entitlements
are supported. Readme points to this document and GETTING_STARTED. No external
payment sites or unsupported server requests are opened by these controls.

Implementation: `client/wlo/settings`, `client/wlo/app/settings.go`. Native sources:
System constructor `0x282990`, toggle callback `0x284310`, sender
`0x2d1594..0x2d1757`, snapshot decoder `0x2ea7fe..0x2ea872`, Channels constructor
`0x281fcc`, Info `0x2824bc`, Titles `0x2a4b40`, Blacklist supplement `0x24ecb8`.


## Skills window

Open **Skills** from the main toolbar or press **Ctrl+S**. The window is draggable;
Escape and either Close button dismiss it. It follows `TRe_SkillForm`
(`FUN_002a00f0`, `FUN_002a2234`) and the Skills screenshots in `UI_02`.

- **Physical, Magical, Assistant and Life** show learned skills, icons, SP costs
  and grades. Hover or select a row for its description, attack range,
  proficiency and permitted weapons. Scroll buttons, thumb and wheel browse
  longer lists, retaining each tab's position.
- The arrows select the player or an occupied pet slot. Pet skills use the
  NPC template's skill IDs and authoritative roster/progress packets.
- **Intro** shows the player's elemental tree, with learned nodes colored and
  unavailable nodes gray. Hover a node to read its description and required
  attributes. Requirements use the server's compiled elemental skill tree.

`client/wlo/skills` reads names, descriptions, native category (`effect_layer`),
icons and maximum grades from `skill_data.json`; permitted weapon flags come
from the fixed tail of `animation_data.json`. Stunt alias 15003 displays the
avatar's actual skill. AC5:3 replaces learned skills; AC5:11/12 and AC8:1 update
player progress through compatibility layouts; native alchemy grade/EXP use
AC8:1 stats 110/111. Native AC5:11 is a recent-skill list and AC5:12 changes the
character model, so those compatibility layouts must not be used for alchemy.
AC15:8 plus AC8:2 update pets. Malformed updates are rejected
before mutation. Character changes and disconnects hide the window.

Drag learned skills (including Basic Atk and Defense) onto a hotbar slot.
Double-clicking a skill and activating its hotbar binding use the same battle
action path. Browsing sends no gameplay requests or optimistic unlocks. The
dedicated native battle skill menu remains pending; the standalone Skills
window supplies learned skills to the implemented battle scene and direct targeting.
See [battle parity](BATTLE_CLIENT_PARITY.md).


## Hotbar and logout confirmation

The hotbar supports three pages of eight slots. Drag a skill from the Skills
list onto a slot; drag an existing shortcut to move it, and right-click to
remove it. Page arrows change the F1–F8 bindings. The view button toggles the
native vertical and horizontal layouts without losing bindings.

Assignments send native `[40,1,kind,id:u16,page,slot]`; received AC40:1 binding
lists are validated atomically. Shortcuts save locally per character beside
`settings.json`, in `hotbar-<character-id>.json`. These files store preferences;
server packets remain authoritative for learned skills, pet ownership, resources
and combat. Changing characters loads a separate bar. Pet skill bindings retain
the selected pet's ID and slot and refuse to use a replacement/released pet.

Click a slot or press **F1–F8** to activate it. During a ready battle turn,
Basic Attack and offensive skills arm direct targeting in the battle scene;
Defense submits immediately. Support skills target eligible allies/enemies in
the current formation, with the server validating their effects. The client checks learned skills, ownership and
SP, rejects duplicate actions and revalidates pending choices when the turn or
roster changes. Stop Remote auto-fight before choosing manual actions. Combat
skills outside battle show a warning; overworld healing/casting remains pending.
Battle-scene targeting uses native cursors, allegiance, revival rules and
current-roster checks. Received item shortcuts use the
existing inventory-selected consumption checks and authoritative receipts.

Settings **Log Out** and **Exit** now show the native gold frame, blue dither,
player preview/name and Confirm/Cancel buttons over a darkened scene, based on
`UI_02/Screenshot_20261005_200128.png`. Cancel/Escape leaves the connection intact;
Confirm alone returns to server selection or exits. The blue mask palette is
measured from the reference; original panel22 frame/dither assets are preserved.

Teams, pet controls and the Instance lobby are documented in [TEAMS.md](TEAMS.md).

### Recruitment and interaction feedback

Pet roster additions after map readiness show “PET NAME joins the party” in a
portrait-free dialogue window and play `sound\wav0152.wav` (native
`FUN_00423958`). Initial synchronization, repeated lists and slot moves stay quiet.
Story recruitment messages wait until the active event/movie has ended; dismissing
one does not send an event acknowledgment.

NPC/object clicks outside 169 pixels on either axis show “Too far” for two seconds.
The old WLRI `FUN_00303b54` checks this distance and silently returns; the notice is
an intentional usability addition. Distant clicks do not start an automatic walk.
Movie pictures retain their authored light level: 255 is keyed, other values use
`ro_Clipper_LightAlpha_ColorKey_Blt` (`FUN_00342508`). The rescue dust therefore adds
light to the scene instead of covering it with dark pixels.

The two rescue `.sty` files were hash-checked against the original client under
`Wonderland-Client/sty`. Function addresses above refer to the catalogued WLRI
executable (`var/ghidra/aLogin.exe`, SHA-256 `ca19ee087b601182e872235e58e6bf367f4bb21879226dfe3cc96f1c99624f61`);
the installation's current `aLogin.exe` has a different hash, so these addresses
are not claimed as verified offsets in that executable.

Gonline Server can negotiate local battle backdrop resolution independently of
combat round chains. The client uses its native map regions at the shared battle
entry location. Legacy servers retain control of their own backdrop selectors.
Runtime bundles include `runtime/battle_backdrops.bin`; older bundles decline
this capability until rebuilt. See [the backdrop protocol](BATTLE_BACKDROPS.md).

### Client language

Settings (sidebar or F10) → Language selects English or Traditional Chinese;
Apply (and Keep when previewing display changes) updates all open sessions immediately.
Connections, gameplay state and typed input are retained. `go run . -language en` or
`go run . -language zh-TW` overrides that preference. Compile the language catalogs
with `go run ./cmd/asset-build -data data -text-only` after editing translation
tables, or rebuild the client bundle. See [text compilation and Chinese source
import](IGG_TRANSLATIONS.md). Server messages remain as received.
