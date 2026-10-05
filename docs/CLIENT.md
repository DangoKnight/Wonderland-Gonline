# Native client replacement

The Go/Ebitengine client recreates the original `aLogin.exe` front end and world
from the decompile (`client/wlo`). Login, character and world code are present;
rendering, interaction and gameplay support continue to be ported. `-legacy`
runs the earlier reference front end (`client/ui`), and `-workbench` opens artwork
inspection. These modes do not represent complete original-game parity.

Start with [GETTING_STARTED.md](GETTING_STARTED.md) for WLRI extraction, server
setup and compiling/running both programs. Use [CONFIGURATION.md](CONFIGURATION.md)
for connection settings and client overrides.

## Direct decoded assets

The login client can now run directly against repository `data/`:

```sh
cd client
go run . -assets ../data
```

Source runs load `data/` or `../data` directly.
The UI uses `media/menu` PNG skins/backgrounds, `media/font/TATPC1_TWN` PNG
font atlases and `media/sound` PCM WAV files. Maps use `pictures/` and decoded
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

Source runs discover repository `data/` directly.
Installed builds use `data/` beside the executable. Use `-assets <directory>`
to select the decompiled data directory. The workbench requires `-client`
pointing to the original sibling installation.

## Compile the client

From the repository root:

```sh
(cd client && go build -o ../bin/wonderland-client .)
./bin/wonderland-client -assets data
```

PowerShell:

```powershell
Push-Location client
go build -o ../bin/wonderland-client.exe .
Pop-Location
.\bin\wonderland-client.exe -assets data
```

The server must be running for login. Use the registration endpoint to create a
Go account; the original private-server database is not an account source.
The root `go build` and `go test` commands do not include this nested module.

## Asset layout and distribution

Repository `data/` is the decompiled asset tree used by the normal client.
It includes JSON data, PNG picture and sprite atlases, exported media and audio.
The client reads these exports exclusively; missing exports do not trigger native
Item.Dat, Formula.Dat or font fallbacks.

Development runs read repository `data/` directly, without links or duplicate
asset directories. From `client/`, run `go run . -assets ../data`. Saved user
state lives in `var/client/user`, outside the asset tree.

From the repository root, verify exports with:

```sh
go run ./cmd/asset-build
```

This command checks the main export manifest and does not create assets.
For standalone distribution, use:

```sh
go run ./cmd/asset-build -copy -output var/client-distribution
```

This copies the complete data tree, including editable sprites, into
`var/client-distribution/data`. Run the client with `-assets` pointing to `var/client-distribution/data`. Copying does not regenerate or repack artwork. No native
asset directory is included. Ship the data payloads with the executable;
large image/audio files remain ignored by Git. After building the client, a
Linux/macOS distribution can be started from the repository root with:

```sh
cp bin/wonderland-client var/client-distribution/wonderland-client
./var/client-distribution/wonderland-client -assets var/client-distribution/data
```

On Windows, copy `bin/wonderland-client.exe` into that directory and launch it
with the same explicit `-assets` argument. Include an authored server list when
targeting a remote host. Copy the verified data tree together with the binary;
the executable does not embed the media payloads.

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

`client/wlo/app/reference_test.go` renders the empty server list, the server list with a region selected, and the account form. All three match the captures in `client/reference/screenshots/Login/` exactly.

Other tests:

- `flow_test.go` drives a connection to a fake server, checking action 0, the 1/9 reply, typing, Enter and the exact 63/4 bytes.
- `server_test.go` runs the same flow against a live server, including a rejected and an accepted login. It is opt-in: set `WLO_SERVER_HOST`, `WLO_ACCOUNT` and `WLO_PASSWORD`. It passed against this repository's server.
- `login_test.go` holds golden bytes for 63/4, plus tests for the validator, status records, reframing, SERVER.INI parsing and disconnect texts.
- `seui/editor_test.go` covers the editor's filters, limits and double-byte editing, and the combo box's ordering.

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
- **Delete** on an occupied slot hides the selection and opens the password dialog in mode 2 (`Form_InputPw_2`, OK at 0x402a0c, Cancel at `FUN_00402a78`). OK checks the fields like creation does, then sends **35/2**: slot, password and secret code, each string with a length byte (send case 0x2d1a8e). The server's 35/2 reply (0x2eab55): 1 "Delete success" (the slot is cleared), 2 and 3 "Wrong Del Pwd", 4 "Delete failed", 5 "Delete failed, error code: " and the next byte. `TestDeleteAgainstServer` (opt-in, `WLO_DELETE_CODE`) deletes slot 1 on a live server.
- **Previous** sends 63/0 and reopens the account form.

**`TSe_Button`** (`FUN_00467834`): a nine-slice panel whose image row is its own state (+0x118). It has a pressed flag and an optional icon of one or three rows. It is distinct from `TSe_FixedButton`.

**Reference**: full-window screenshots of the original in `client/reference/screenshots/`: `Login/` holds the login phase (server list, account form, a forced error message, character selection and the creation steps) and `In-Game/` the first in-game screens (ship deck, cabin, inventory, chat, a teleport event and the mole minigame). The folder is not tracked. They guide layout only; the decompile decides. `selectchar_snapshot_test.go` (opt-in with `SNAPSHOT_DIR`) renders the selection screen for comparison, and the empty screen matches `select_character_empty.png` visually.

#### Server compatibility corrections

The Go server now writes rebirth/job bytes before exactly six equipment IDs,
matching `FUN_00402468`, and sends maximum HP/SP before current HP/SP so the
selection drawer displays current/max. Rebirth and job remain unported and use
zero placeholders. These corrections deliberately differ from the legacy C#
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

### Character sprites (next)

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

**Verification**:
- `role_test.go` covers archive lookups against the installed archives, including the weapon order, and the palette rules.
- The opt-in snapshot test renders two starter characters. With body 4/head 0 and body 1/head 0, the hair, hat, clothes, shoes and portraits all line up. One character has neutral colours (444444444 twice); the other has shifted hair and skin. A second frame dresses them in Knight's Cape and Angel Wings+5 (`select_character_costumes.png`); `role_test.go` checks the patch fallback.
- `colors_test.go` covers the digit layout and the shift and clamp.

**Not ported yet**:
- The visibility check `FUN_00442c58`.
- Body type 0, vehicles and riding pets.

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

**Verification:**
- `create_snapshot_test.go` (opt-in with `SNAPSHOT_DIR`) renders every step with the reference screenshots' character. Step 2 matches `create_character_stats.png` except 16 pixels in the window corners; steps 3 and 4 differ only in the body's idle frame; the password dialog sits one pixel left and one down of the capture.
- `TestCreatePacket` checks 9/1's layout.
- `TestCreateAgainstServer` (opt-in, with `WLO_SERVER_HOST`, `WLO_ACCOUNT`, `WLO_PASSWORD` and `WLO_CREATE_NAME`) creates a character on a live server and waits for the world-entry packets. Against this repository's server it passes, and the stored character has the chosen look, element, points, colours and starter outfit. The account needs no characters, and a password of at most 10 characters (the field's limit).

**Not ported**: stopping voices after 9/1 (`FUN_00405138(0)`).

### World view (started: `client/wlo/world`, `client/wlo/app/world.go`)

**AC3** (receive case 0x2dfdb3) is the player's own character and opens the world. It is read field by field (`FUN_00113b14`) in the layout the server sends: ID, body type, map, X, Y, a byte, head and look, the two colour values, the worn items, a word and two bytes, then the name. The original reads more trailing fields; our server doesn't send them.

**Scene** (`scene.go`, loader `FUN_003f830c`): `Data\Ground.MMG` is a named archive (`clientassets.ReadNamedArchive`, `FUN_0010f154`: payloads, then 29-byte index rows and a count). The record `<map>.map` begins with the scene size, its background layers and a 20-pixel walk grid (`FUN_004121a8`). The client reads it from the export (`ground_data.json`, its `terrain`). Each layer is `<resource>.jpg` from `pic\map_d01.JMg`, `map_c01.JMg` or `map.JMG`, read from their PNG exports (`pictures/map_d01`, `map_c01`, `map`). Map 10017 is the 2432×1792 Ship Deck with one layer, 10001.

**Names** (`names.go`): scene names come from the `SceneData.dat` export (`scene_data.json`), and each map's scene from the `eve.Emg` export (`eve_data.json`); map 10017 is scene 10001, "Ship Deck".

**Drawing**: the camera keeps the player at the screen centre (the scene is drawn from X − 400, Y − 300; this matches `In-Game/Ship_Deck.png` exactly) but stays on the map (`FUN_003f94e8`, the map's +0xc/+0x10): within 400 pixels of the left edge (300 of the top) it is 0; when the screen would pass the right edge it stops at the map's width − 800 − 20 (the original's 0x14) and at the bottom edge at its height − 600 (0 when the map is narrower or shorter than the screen). Before this the port drew black past the map's edges (`Chat_02/Go_Beach_Camera_Black_Edge.png`); at Dango Beach X:1082 Y:1195 it now frames the scene as the original's `Chat_02/Beach_Camera_Clamped.png` does. The player's name follows the player instead of a fixed row. The player is drawn by the selection's layered renderer, with the name centred above (yellow) and the location line `<name> <scene> X:<x> Y:<y>` at the bottom right; both positions are measured from the capture. `world_snapshot_test.go` (opt-in) renders the capture's position.

**NPCs** (`npcs.go`, `client/wlo/role/npc.go`): the map loader (`FUN_003030ec`) creates one character object per `eve.Emg` NPC record (parsed with `assets.ParseEVEMap` from `eve_data.json`'s `decoded_hex`; the record conversion is `FUN_00484a50`) and applies its `Npc.dat` template (`FUN_004265a4`):
- the look is the record's word at disk offset 14, modulo 1000; an NPC keeps body type 0, so the draw (`FUN_00433318`) is one sprite of the `001` family, 1000 + look;
- the four colour values at disk offsets 18–30 fill colour groups 1–4 over a neutral block;
- the record's facing r (1–8) becomes (9 − r) mod 8, and `FUN_00430474(…, 8)` stands the NPC: action 8 + facing (facing 6 is action 11, towards the camera);
- flag bit 0 shows the NPC at load; otherwise it starts hidden.

AC22:4 (`FUN_0038cf30`, 14-byte entries: click ID, state, X, Y, kind, duration, action) moves NPCs and shows (kind 1) or hides (kind 2) them. For props (Npc.dat kind 6, 9 or 10, `FUN_00482424`) the state's low byte fixes the drawn frame (+0x11f; 0xff animates), so a chest stays closed (0) or open (1); other NPCs animate their action. NPCs and the player are drawn back to front by Y; an NPC adds its eve record's depth (+0x1180, copied from the record's +0x6b4, the last trailing int), so the coconut up a palm on Newbie Island (depth 150) sorts in front of the palm. The name overlay (`FUN_004147f8`) skips kinds 6, 8 and 9. Templates on the tall name base (Npc.dat +0x57 = 1) draw their sprite 0x44 lower, 0x20 more on the doubled scale (`FUN_002fe8e8`); their name and shadow stay at the feet (Burke the tiger). The talk window's body mode draws through the same placement, so the drop applies there too (`In-Game/Burke_Talk.png`). Verified against `In-Game/Newbie_Island_Props.png` and `Newbie_Island_Coconut_Hover.png`. On Ship Deck the eleven NPCs stand where the capture shows them, and their colours match it exactly.

**AC12** (case 0x2e20cf: ID, map, X, Y, portal) places the player; the map reloads when it changes, then the client acknowledges with 12/1 as `FUN_002f2fd0` does. The server waits for that acknowledgment before publishing the character and accepting actions.

**Names and shadows**: an NPC's name (its template's name, from the object's +9) is drawn by the NPC class's overlay method (`0x4147f8`, called from the depth-sorted pass `FUN_004219d8`): white (0xffff), outlined (style 2), centred on its X, at Y − h − 0x19 (or − 10 when h ≥ 0x32). The name height h (+0x114, `FUN_004265a4`) is 0x6a minus the anchor Y of the sprite's first frame (0xa6 when the template's byte at disk offset 87 is 1), doubled when the byte at offset 56 is 1; ID-specific heights on the 0xa6 base (`FUN_0018b544` and others) are not ported. The ground shadow (`FUN_0030120c`) depends on the template's byte at offset 74: 0 draws the (49,88)–(80,105) cut of the `Shadow` picture at (x − 15, y − 4), 1 the whole `Monster_Shadow` at (x − 47, y − 18), others none; both pictures come from `pictures/images` and are registered in the picture database at startup (`picdb.Add`). On Ship Deck names and shadows land on the capture's pixels. Players and other players get the same `Shadow` cut (`FUN_0030120c` for a role whose +0xb1 is 1 or 2); the shadow switch (+0x1efd) and the map checks (`FUN_00334120`) are not ported.

**Not yet**: wandering NPCs (walk behaviours other than 1 stay at their record position), the scene's objects and animated WEM resources, other players, the HUD (status, toolbar, F1–F8 slots, chat bar), chat lines, walking, the camera at scene edges, and the origin offset for scenes smaller than the screen (`FUN_003f830c` centres them in 20-pixel steps).

### Scene objects (ported: `client/wlo/world/objects.go`)

The background pictures already contain railings, masts and other scenery, but the original repaints those pieces as objects so characters can pass behind them. The map record's object list (resource, X, Y after the walk grid, read by `FUN_003f830c`) creates `TGroundObj`s described by their `Wem.MMG` records (`FUN_003f5f1c`: resource, a dword, depth line in grid cells, frames, interval in ms, width, height, translucency, kind). Each paints its picture (green key transparent; frames stacked vertically) with the top-left corner at its position (`FUN_003f65cc`). The map paint (`FUN_0041e0d8`) orders them by kind (+0xd6):

- kinds 0–2 right after the background;
- kind 3 in the depth-sorted list with the characters (`FUN_0041d13c`), keyed by Y plus the depth line, after characters with the same key;
- kinds 4–5 after every character. The deck's railings are kind 5 (42 of the deck's 72 objects), so they cross the characters' legs as in the captures.

`TestShipDeckObjects` checks the deck's counts. Not ported: the cull test (`FUN_003f5d70`), translucent objects (+0xd4 other than 0xff, or +0xe0 from the picture's first pixel), and the resources 11011–11015 and 11031 that the loader forces to kind 3. The picture loader keeps the last four decoded atlas pages, so a map's objects decode their shared pages once.

### Status panel (ported: `client/wlo/hud`, `client/wlo/world/stats.go`)

The top-left panel is `TSe_MainStatus` (constructor `FUN_0025ff84`, painter `FUN_00260fac`), a fixed form shown when the world opens. From its corner at (2, 10) it draws `Main_Status_1`, the HP and SP gauges, the face, the level, the job icon, HP, SP, EXP and gold; the element icon (`Icon_Element_<e>_1` at +0x71, +7) and `Btn_Allrecovery_1` (0x27, 0x6a; hint "Restore HP/SP using Potions and Bottles", no action yet) are children. The value drawers are the ones the character selection already borrowed (`login.Status`).

- **Gauges** (`FUN_00463d98`, `picdb.DrawGauge`): `Main_Hp` and `Main_Sp` are cut along a line from (0x2e, 0) to (Round(p × 0.82) + 9, 24 − Round(p × 0.24)), so the orb fills along its arc; pictures wider than 100 pixels start at 0x3a and 0x15. The fill is the share of the maximum, at least 1 while anything is left.
- **Face**: the role's face sprite in action 0 (the selection uses the boxed action 2), blinking for 50 ms every 6–8 s.
- **Values** (`world.Stats`): 5/3 (`FUN_004381c4`) fills element, HP, SP, the attributes (read as STR, CON, INT, WIS, AGI; this server writes CON, INT, STR, AGI, WIS, and the 8/1 updates that follow set each by ID), level, EXP, the stored maxima and, after the skills, rebirth and job. 8/1 (`FUN_00416ebc`) sets one stat by ID; the bonus stats 0xcf and 0xd0 (and WIS) recompute the maxima with `Formula.Dat` (`FUN_0036e674`, `FUN_0036e704`): level^k × attribute × k + level × k + attribute × k, rounded, plus a base and the bonuses, with level counting 100 per rebirth. The constants match the server's (HP 1, 2, 0.35, 2, 180; SP 1, 3.2, 0.3, 2, 94). 26/4 sets the gold.

On Ship Deck with the capture's values (level 1, 181/181, 100/100) the panel matches the capture apart from the character's own face and element.

**Not yet**: the panel's slide-in (`FUN_00260b90`), the restore button's action, the discussion button's mode, the pet panel, VIP numbers and the save-lock icon.

### Button bars (ported: `client/wlo/hud/buttons.go`)

- **Toolbar** (`TRE_FuncBtnForm`, constructor `FUN_00295fb8`, paint `FUN_00296744`): `Btn_BackGround_1` at (0x1b0, 0), then World PK (`Btn_PlayerBattle`), PK, Jump In, Spectate, Join Team, Make Friends, Trade (`Btn_1`–`Btn_6`) every 0x25 pixels from 0x1ae; Guild Invite (`Btn_7`) is hidden and Events and Lucky Draw (`Btn_8`, `Btn_9`) take the slots from it, with a 300 ms animation of their rows 2–3 that runs while an event or the lottery is open (not ported); Item Mall (`Btn_711`) at 0x2fb. The paint also rewrites a hint with the gold and starts or stops the lottery animation; neither is ported.
- **Menu** (`TRE_MainBtnForm`, constructor `FUN_002950b8`, paint `FUN_002956ac`): `Btn_BackGround_2` at (0x213, 0x233), then Inventory, Skills, Alchemy, Team, Social, Options (`Btn_11`–`Btn_16`) at their own corners; Pickup Furn (`Btn_BringUpFur_1`, 0x2fb, 0x1fb) stays hidden.

Each button is its picture's width by a third of its height (three state rows). The bars match `Ship_Deck.png`; the buttons have no actions yet.

### Hot-key bar (ported: `client/wlo/hud/hotkey.go`)

`TSe_HotKeyForm` (constructor `FUN_00266098`, layout `FUN_0028f55c`, paint `FUN_00296870`) is the F1–F8 bar: `Form_HotKey_V` (0x38 × 0x112) at the right edge, vertically centred, with eight 0x18 slots at (0x17, 0x1d + 0x1b·i), Toggle View (`Btn_Minimize_1`, 0x25, 0), page arrows (`Btn_ArrowL_3`/`Btn_ArrowR_3` at y 0xf8) that cycle pages 1–3 (`FUN_0028f2d8`, `FUN_0028f2b4`), and the page number `HotKeyPage_<n>` at (0x17, 0xf5). It can be dragged and stays within 800 × 0x226 (`FUN_002968e0`). It matches the capture. Not ported: the slots' icons (`FUN_002a3260`, from the hot-key table), the horizontal and minimised layouts (`FUN_0028f8ec`) and using a slot.

### Chat bar (ported: `client/wlo/hud/inputbar.go`)

`TSe_InputBar` (constructor `FUN_0026348c`, paint `FUN_00268194`) is `main_input_1` (0x209 × 0x19) at (0, 0x23f): the mail icon (`Anim_GotMailS_1`'s first frame) at (4, 2), the channel button (0x1f, 2), the whisper field (0x49, 0x58 wide, 15 characters), the message field (0xa2, 0x148 wide, 60 characters, hint "Press Ctrl+V to paste") and Chat Emotes (`Btn_expression_1`, 0x1ec).

- **Channels** (+0x140): 1 World, 2 Local (the start, and again on each world entry, `FUN_00268980`), 3 Whisper, 4 Team, 5 Guild (6 Ally and 7 GM exist but have no button). The channel button shows the channel's name, `btn_channel_<n>_1` (40 × 60, three states; `FUN_00267db0`), with the hint "Switch Channel".
- **Switching**: the paint (`FUN_00268194`, rectangles by `FUN_00020d08` = Bounds) opens the list while the pointer is over the button (0x1c, 0, 0x28 × 0x15 in the bar) and keeps it open while the pointer is over the list (the 0x7f pixels above): `icon_Channelframe_1` with a button for each other channel (`btn_channel_<n>_1`, 0x14 apart from 0x17 above the bar); a click on one picks it (`FUN_00267c5c`). The button's click (`FUN_00269b30`, World → Local → Whisper → Team) only acts while the list is closed, so with the pointer on the button a click changes nothing; its other handler (`FUN_0026527c`) toggles the list. Choosing Team or Guild without a team or guild adds "(System):You are not in a Team" or "(System):You are not in a Guild" and returns to Local, as `Team_Guild_Chat.png` shows. `TestChannelButtonOpensList`.
- **Whisper field**: read-only outside Whisper, and emptied (with the target, +0x18c) when leaving it. It has no picture and keeps its pixel hit test, so it never takes a press itself: the bar's press (`FUN_002695d8`) inside (0x46, 0x240, 0x56 × 0x16) selects Whisper, which gives the field the keyboard (`TestWhisperAreaPress`); a press on the mail icon would open the mailbox (not ported). Enter in it (`FUN_00268a30`) looks the name up among the known players (letter case ignored): an empty field says "No target"; an unknown name says "<<No such person online>>" and empties the field; a known one becomes the target (`FUN_00269068`), adding "Whisp to <<name>>", joining the recent whisperers and moving the keyboard to the message field. Losing focus also validates a nonempty recipient: an unknown name clears the field and target and shows "<<No such person online>>" once; valid names resolve without taking focus away from the clicked control. Empty blur is silent (`Chat_03/Chat_Whisper.png`, `TestChatWhisperBlurValidation`).
- **Recent whisperers** (+0x174 in the panel +0x178, `FUN_00268cdc`): another player's whisper puts its speaker before the last name; a chosen target moves to the end; at most ten. On the Whisper channel the list opens while the pointer is over the whisper field (0x46, 1, 0x56 × 0x16) or the list, 0x14 a row above the bar; a pick (`FUN_00268c38`) fills the field and runs its Enter. The panel is created without a picture, so only the names show. `TestWhisperers`.
- **Known players** (`PTR_DAT_004c9788`): every AC4 record (`FUN_00429a38`) is kept, whatever its map, until the player leaves (AC12 to map 0), and names whisper targets and chat speakers beyond the map's players. This server sends AC4 only for players on the same map, so a player on another map can be whispered only once the client has seen them.
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
rendering, editing and hints. Character speech bubbles remain pending. Mail animation and the
battle/viewer/auto-play buttons remain pending.

### Chat (ported: `client/wlo/hud/chatlog.go`, `client/wlo/app/chat.go`)

- **Log window**: `TTalkMsgForm` (constructor `FUN_0048bbf4`), 0x208 × 100 at (0, 0x1db), in the transparent mode of the capture: `panel_TalkForm2_1` as the strip at the left (stretched from 86 to 100 pixels through its scroll track, with the `bar_H4` thumb filling the track), Chatbox Lock (7, −8), the scroll arrows and Chat Box Switch. Lines go into the message list (`TSe_CharMsg`, 0x32, 0xe, 405 × 60), rows 0x14 apart, newest at the bottom, wrapped to 50 characters without splitting double-byte characters, at most 100 complete messages.
- **Navigation**: arrows, the draggable thumb and mouse wheel scroll the retained messages. Lock holds the current view as other players speak; an outgoing message releases it and follows the newest rows (`FUN_0048c9d4`). The switch cycles opaque, transparent and ticker-only modes (`FUN_0048d2e4`). Transparent text lets clicks through to the map; opaque message rows and speaker portraits can select whisper targets.
- **Resize and move**: the opaque window's native handle changes width/height with even dimensions and a minimum 224 × 72 (`FUN_0048e004`). The HUD bottom is aligned at y 575, matching `Chat_03/Chat_Background.png`; the binary's y-566 resize clamp displaced the port by nine pixels. All modes retain the same size and position (`Chat_Resizing.png`). Unlocked background mode can be dragged with pointer capture across child controls, bounded to the screen and y 578 (`FUN_0048c32c`, `Chat_Move.png`). Lock prevents dragging. The last 100 complete messages are retained and rewrapped with Big5/emoticon boundaries intact. A locked view follows its message and byte offset when wrapping changes. `TestChatNativeResizeCapture` and `TestChatResizeRewrapAndLockedAnchor` cover input and layout.
- **Background and ticker**: opaque mode tiles the native colour-keyed `32X32Grid` within the stretched frame (`FUN_0048d410`). The ticker follows the window, sits 20 pixels above its bottom and fits its width without splitting Big5 characters. `TestChatReferenceGeometryAndDrag` checks the background, shared geometry and drag/lock capture.
- **Lines** (`FUN_0048ac28(form, speaker, text, colour, channel)`): channel 1 reads "(World)" + name + ":" + text, and likewise 2 "(Local)", 3 "(Whisp)", 4 "(GM)", 5 "(Team)", 6 "(Guild)" and 7 "(Ally)". Channel 0 is the text alone with a speaker and "(SystemPromp):" + text without one, and also joins the ticker. Channel 10 is the client's own messages as given, 11 "(System):" + text and 12 "(Bouquet):" + text.
- **Speaker icons** (`FUN_004905e8`): a row of channels 1–7 whose speaker is the player or a player on the map shows the speaker's face sprite (`<family>f`, 601 + head, as the portraits) in action 4 at its second frame (+0x11e = 1: the head turned three-quarters; the first frame faces front), at the list's left − 10 and the row's top + 10. `Chat_Icons_02.png` (the original) and `Chat_Icons.png` (the port, before the fix) show the difference; `TestChatSpeakerIcon` checks the action and frame.
- **Colours**: channel → entry of the table `FUN_003beb14` fills (0x72a360): system 7 (0xf800 red), World 1, Local 9 (0xff80), Whisper 10 (0xfc00 orange), GM 10, Team 6, Guild 4, Ally 5, channel 10 7 (red); the table is fe31, ffb3, c6f3, 8653, 6e7e, 8e27, ffff, f800, f4a3, ff80, fc00. Every sent and received line first copies the player's `ChannelColor1`…`5` setting (Local, Whisper, Team, Guild, World; read from the character's `user\save` file by `FUN_00284434`) into its channel's slot (+0x4e0 + channel × 2); without a save the defaults are 9, 10, 6, 4 and 1 (0x284d05). The Channels settings window now persists local palette choices; reading legacy save files is not ported. `Chat_02/Connection_Lost.png` shows whispers in orange (the port had them peach before, `Chat_02/Go_Whisper_Colors_Tooltip.png`).
- **Ticker** (`FUN_0048d688`): channel-0 lines also queue for a ticker drawn at (0x32, 0x22b). It starts as 60 spaces; every tick its first character is dropped and, while it is shorter than 0x36, the message's next character is appended, so a message types in from the right. Each message plays three times. The tick is 100 ms, which puts the welcome message where `Ship_Deck.png` caught it 33 ticks in.
- **Welcome**: on the first world entry (`FUN_00492ac4`) the log empties and "Welcome to [<server>] Server" is added on channel 0 with the player as speaker (so no prefix and no icon), with the server's name from 1/9 ("Wonderland Go" from this repository's server).
- **Receiving** (the cases at 0x2df0fb…0x2dfa42, table at 0x2df0b7): 2/n for n up to 7 carries the speaker's ID and the text (cut to 60 characters except 2/0 and 2/4) and is shown on channel n with the speaker's name from the map's players; 2/5 needs a known speaker. 2/16 (case 0x2dfd44) shows its text on the notice board for 2 seconds. (2/3 is Whisper, not a notice, as the port had it before.)
- **Sending** (`FUN_002641c4`, Enter in the message field): by channel,
  - World: needs a radio in the special slot (equipment slot 6: Radio Set 34076, Loudspeaker 34132 or Transceiver 39070, `FUN_00452fc4` → `FUN_003d1018`), else "(System):World Channel requires Radio Set" (`World_Chat_02.png`); with one, at least 15 SP ("(Alert):Not enough SP to use Radio Setq"). Sends 2/1 and adds the line at once.
  - Local: sends 2/2 and adds the line at once (`Local_Chat_02.png`).
  - Whisper: an empty whisper field says "No target" (`Whisper_Chat_02.png`); no target says "No target" too; a target not among the players says "Player is offline". Sends 2/3 with the target's ID before the text; the line comes back from the server. On map 0x29cd it is "Temporary unavailable" on the notice board.
  - Team and Guild: send 2/5 and 2/6 (lines come back from the server); without membership they say "(System):Not in Team Chat anymore" or "… Guild …" and return to Local.
  - "/msg <name> <text>" (`FUN_002687f0`) switches to Whisper and, for a player on the map, takes them as the target, adding "(GM):Whisper <name>" (the original's channel-4 call) and sending the rest. The whisper field stays as it was, so with it empty the send still says "No target", as in the original.
  - A refused message keeps its text; a sent one empties the field and joins the history.
- **Tests**: `TestChatChannels` and `TestChatWhisperAndWorld` replay the Chat captures; `TestChatPrefixes` covers the channels.

Server side (`internal/server/chat.go`, `chat_channels.go`): 2/1 goes to every player in the world except the sender (the sender's client logs it); 2/3 [target][text] goes to the target and back to the sender as 2/3 [sender][text]; 2/5 goes to the party, the sender included; 2/6 is ignored (no guilds). The `/world`, `/team` and `/whisper` commands send the same packets, `/world` echoed to the sender. GM broadcasts are 2/4, which the client shows as "(GM):" lines.

Not ported: the speech bubble over the speaker (`FUN_00428ed0`), GM bans, the radio's 5-second charge, the Loudspeaker's World-line variant, alternate backgrounds, links/VIP marks and the announcement banner (`FUN_0048d9b8`). Team and guild interfaces remain pending.

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

Pending: the gesture selection interface, titles, speech bubbles, riding/pets,
clicking a player and friend-list presence UI. Expressions and nicknames use the
existing player name height; native body-height/ride adjustments remain pending.

### Talking to NPCs (ported: `client/wlo/app/events.go`, `client/wlo/hud/talk.go`)

- **Clicking an NPC** (`FUN_00303b54`): the shown NPC under the pointer (the front one by feet Y; the hit box is its current frame) gets **20/1** with its click ID as a word. Within 0xa9 pixels on each axis it is sent at once; otherwise the player walks to the NPC and sends it on arrival when within reach.
- **Event frames 20/1..6** (receive case 0x2e3dc0 → interpreter `FUN_00304fd0`): after the command byte, `[sub, byte, word, step, kind, subject, actor u16, mode, value u32, text u16, branch]`, matching the server's `eventFrame`. Kind 1 is a line of dialogue: subject 3 is a map NPC (the actor), subject 7 the player. The text is the talk ID's `Talk.dat` record (`data/talk_data.json`).
- **Acknowledging**: a step the client has finished (+0x7108) is answered with **20/6** by the frame loop (`FUN_00307300`). A dialogue line is finished when clicked; 20/7 finishes the step as well; 20/10 (and the unnamed 11 and 13–17, receive cases at 0x2e3dc0) finishes the step too, so server-driven sequences such as the beach rescue advance at once; **20/8** ends the event and releases the server's hold (player +0x2392 = 0, receive case 8), which is how a door event's 6/2 hold ends after its teleport. While an event runs or the server holds the player (**6/2** with 1), clicks, held buttons and arrow keys do not walk.
- **Talk window** (`TSe_TalkMsgFormPlus`, `PTR_DAT_004c9950`, constructor `FUN_0034bfa4`, face mode `FUN_0034d990`): `panel22` at the top centre, y 0x19, fitted to its content by `FUN_0034ba20`: the longest line × 8 + 110 wide and the lines × 18 + 66 tall; a face adds 0x78 and makes it at least 0x9c tall, a body (body mode, below) adds its frame width + 0x1e and makes it at least its height + 0x46 tall; then 5 more (the Visitor's line gives 0x21e × 0xa1, the cat's "Meow~" about 0xdc × 0x6e, both as captured); yellow lines centred and wrapped at word boundaries; an NPC's face (`Npc.dat` +0x58, a `008` sprite in action 0 with the NPC's colours, blinking every 6–8 s) with its feet at (right − 0x49, top + 0x46) and its name in white 0x24 below; the player's portrait (action 2) at the left instead; and `Icon_Nextone_1`'s two frames swapped every 250 ms at the bottom centre. `TestWorldSnapshot` renders `In-Game/Ship_Deck_Talking.png`'s line and the cat's (`Cat_Dialogue.png`): the text, speaker, name, frame and icon align with both captures. - **The player's lines** (`FUN_0046fe50`'s `Form_Talk` branch, +0x2a8): a wide form instead, `Form_Talk_1` (the blue skin, +0x38c = 1; `Form_Talk_2` is the pale one) at (0x23, 0x195), 0x300 × 0xa9, not fitted to the text; lines of 0x2d characters (+0x294, 0x28 in the normal window), left-aligned at 209 and 0x2e from its top; the next icon at its centre. The face sprite's large art (action 3) stands at x = half the **body** sprite's frame width in action 3 (`FUN_00437cd4` measures the body) + 0xa7, its feet 0x1e above the form's bottom (`FUN_0034d990`), without a name. In the game modes 1, 3 and 4 (`PTR_DAT_004ca2f8` +0x1d; the HUD is hidden then) the form sits 0x19 lower and the art 0x19 lower again (`Talk.Lowered`). `TestPlayerTalkSnapshot` renders `In-Game/Player_Dialogue.png` (body 4's art 5607, talk 20306): frame, text and art align with the capture. Not ported: the original's check that the worn look matches its table (`PTR_DAT_004ca774`) before choosing the wide form — the port uses it whenever the art exists — and the modes that hide the HUD.

- **Speakers** (`FUN_0034c814`): an NPC whose template has a face sprite, and the player, use face mode. An NPC without one (the Persian Cat) uses body mode (`FUN_0034cb7c`): its whole sprite, standing facing down-left (action 0xb), with its feet at (right − width/2 − 0x1e, top + height + 0x14), its name 0x10 below; `Npc.dat` bytes 86 and 87 lower the sprite and the name.
- **Markup** (`FUN_00478140`, scanner `FUN_00477e2c`, styles `FUN_00470f48`/`FUN_00471028`): an opener `#X` counts only when `/#X` follows; both are removed. `#R` is red (0xf800), `#W` white, `#B` bold, otherwise yellow; `#n` inserts the player's name, `#e` a line break, `#g` the castle owner ("No guild own Castle"); `#F1`–`#F3` (face expressions) and `#P` are removed; `#s` carries a seven-character sound name and plays `sound\<name>.wav` — the Persian Cat's `#swav1541/#sMeow~`. Sounds play when the line shows, since the typing that reaches them is not ported.
- **Voices** (`FUN_0034d3e8`): the original also looks up a recorded line, `<talk ID>[_10|_20|_30<nn>].ogg`, in `Data\odd.dat` and `odd_d01.dat` (exported to `data/audio`). The reference install has no `odd.dat`, so it never plays them, and the port does not either.

- **Opening and closing** (`TSe_ShortMsgPanel`'s tick `FUN_00477810`, style 1 set by `FUN_00476bcc`): every line restarts the animation (`FUN_0046fe50` zeroes the steps and stamps the start with the frame clock, `timeGetTime()` taken once a frame). Opening (`FUN_00472664`) grows four column steps, then four row steps, at one step per millisecond (+0x140 = 1.0, default of `FUN_0046fcf4`), sized margins + steps × (text ÷ 3) + 0x14 wide and + 6 tall, centred on the final box; the text, speaker and next icon appear only when both are complete (+0x17a). A click on the window (`FUN_0034e2c4`) clears +0x196 and closing (`FUN_00472868`) runs the rows, then the columns, back down, then hides the window (`FUN_00470ee4`). Because a frame lasts far longer than 4 ms, at 60 frames a second this gives: opening, two frames at the bare margins (about 120 × 68), one at full width, then full with the content; closing, two frames at full width and three quarters of the height, one at the bare margins, then gone (`TestTalkAnimation`). The style-2 animation (`FUN_00476ae8`/`FUN_00476a1c`, width only, one step per interval) is not used by the talk window. The notice board shares this class but its port does not animate yet.

- **Questions** (kind 6, `FUN_00304fd0` case 6): the frame's text field names one of the map's questions, EVE category 7 (copied to +0x5d9c by the map loader, 0x392fc0): a header whose last byte is the form (0 the talk window; 1 and 3 other pickers, not ported, which the client closes with 40) and 5-byte entries `[index, value u16, kind, type]`, kind 1 a talk text and 2 an item, type 1 the prompt, 2 a Yes/No button, 3 an option, 4 not shown (`client/wlo/world/questions.go`). With options (`FUN_0034d00c`) the window keeps the prompt's size and lists the options below it: the rows start 0x12 − 6 above the text's bottom, 0x14 apart, from 18 px inside the window's left to 23 px inside its right, in a 2-pixel frame of (140, 190, 247); a hovered row gets a bar of that colour behind its yellow text; options wrap within the row width. The window grows by the rows, 8, and 0x20 for the Close button (`Btn_Close_1`), placed 0x2d above the bottom. `TestQuestionSnapshot` renders Robinson's question (map 10003, `In-Game/Question_Robinson_Hover.png`): the prompt, face, rows, Close button and frame align with the capture. Two Yes/No entries and no options show OK and Cancel (`FUN_0034d150`, 0x14 either side of the centre in a 0x28 row). Replies go out as **20/9**: option row r sends 30 + r ("3" + the row, `FUN_00307ca4`; the events branch on 30, 31 …), OK 20 and Cancel 21 (`FUN_00307c4c`, `FUN_00307c78`), Close 40. In mode 0 the step is also done, so 20/6 follows; mode 1 sends only the answer. More than five options scroll in the original; the port lists them all. `TestQuestion` answers map 10005's question 1.
- **Names**: while the talk window is drawn the map leaves out the characters' names, as both talking captures show.

`TestNPCTalk` covers the click, the frame, the acknowledgement and the end; `TestCatTalk` the body mode and the sound; `TestParseTalk` the markup. Approximations: the wrap width (the capture allows 39 to 44 characters; 40 is used) and the player's face position (`0x2d` plus half a 128-pixel sprite). Not ported: the other kinds (each is acknowledged at once, as kind 2 is), the item and trade question forms, the window's OK-only mode (`FUN_0034cf44`), scroll bar and typing, the NPC turning to the player, and the talk cursor.

### Doors and areas (ported: `client/wlo/world/areas.go`, `client/wlo/app/areas.go`)

A map's areas are EVE category 1 records (the scene loader `FUN_003090f4` → +0x338): rectangles of 20-pixel cells from (x1, y1), counted from 1, to (x2, y2), so the rectangle starts at ((x1 − 1)·20, (y1 − 1)·20) and spans (|x2 − x1| + 1) cells each way. The record's tail holds x2, y2, the door kind (+0x48: 1 none, 2 small, 3 large) and the light's offset from the corner (+0x4c, +0x50).

- **Entering**: `FUN_00303ff4` looks at the player every 200 ms, or sooner after 21 pixels of movement. With no event in progress, `FUN_0030410c` sends **20/8** with the ID of the first area that has events and holds the player, unless it is the area entered last (player +0x3aa6; cleared when the player is outside every area, and by a map load, `FUN_00304898`). The client then waits as for an event: the server teleports (20/7, AC12 …) or answers 20/8. The player stops. The wire subcommand comes from +0x7102 (the send case at 0x2c7f6b), not from the call's constant 4.
- **Door lights**: `FUN_003030ec` adds a "DoorLight" (13 frames) or "SmallDoorLight" (4 frames) effect from `pic\images.BMg` at each door's point; the effects list (`FUN_00403b8c`, after the map paint) draws the ones whose point is on screen, centred on it with the bottom a quarter frame below, one frame every 300 ms, with rodraw2's `ro_Clipper_LightAlpha_ColorKey_Blt` at level 24: each channel adds the source × level/32, saturating (`surface.DrawLight`).
- The server's area test (`world.Area.Inside`) counts cells from 1 the same way, so the client's 20/8 and the server's walk-in check agree on every region.
- `TestDoorTeleportWalks`: after the door's hold, teleport and closing 20/8, the player walks on the cabin map. `TestDeckDoor`: Ship Deck's cabin door (area 1, cells 93,41–96,45, small light at 1877,857) sends 20/8 once and again only after leaving; `TestDeckDoorLight` renders the light where `In-Game/Ship_Deck_Teleport_Event.png` shows it.

Not ported: the door transition helper `FUN_003ba9d8(…, 10)` and `FUN_00173774` that follow the request, and the NPC walk-in areas (20/2, `FUN_003042b4`; 20/3, `FUN_00304478`).

### Movies (started: `client/wlo/movie`, `client/wlo/app/movie.go`)

Story scenes (the shipwreck storm, the beach rescue) are `.sty` movies that the client plays itself: event kind 5 names one by its value (`<value>.sty`), and its mode sets the game mode (1 or 4, `PTR_DAT_004ca2f8` +0x1d), which hides the HUD and lowers the wide talk form (`FUN_00304fd0` case 5). `TMovie` (loader `FUN_0033e18c`, update `FUN_003406c4`, draw `FUN_00340404`):

- **Data**: `data/media/sty/style_data.json` (768 movies). The export's field split doesn't follow the records, so each section is joined back into bytes: an actor is four ints (template — 0xffff the player — start stage, keyframe count, initial direction) and count + 1 keyframes of 45 bytes (facing, x, y, a flag, speed class, pose, three ints, sound index, loop flag, channel), NPCs one more int; the camera ("Map10003": the map whose scene is drawn) and pictures use 33-byte keyframes (x, y, flag, speed …); lines are records 1…n of `[speaker, talk, stage, delay]` (+4, +8, +0xc, +0x10; the loader reads n + 1 records and record 0 is unused, so the export's rows start two ints into it); the timeline gives each stage a wait (ms) and operands (overlay, two flash tracks, music, four bytes). `TestAllMovies` decodes every movie.
- **Playback**: a stage ends when its wait has passed and every active actor (start ≤ stage ≤ start + count) and the camera have reached their targets; then its line (if any) shows, at least for its delay and until the talk window closes; then every active actor takes its next keyframe (target, speed, pose, facing, sound) and the next stage begins; after the last stage the movie ends, the game mode returns, and the step is acknowledged with 20/6. Actors move at the speed class's rate (0.0008, 0.008, 0.08, 0.12, 0.4, 4, 40 px/ms; `FUN_00339598`) along the line to the target. The camera keeps its own keyframe counter and centres the screen on its keyframes (x − 400, y − 300).
- **Drawing**: a separate view of the movie's map (`world.NewView`: the scene, objects and depth sorting, no names or location) with the movie's actors: the player (dressed as the player) and NPC templates. An actor's action is its pose plus its direction (`FUN_00339930`), the direction being the keyframe's facing or the movement's while moving (`FUN_00342948`), reduced to left/right for two-way pose groups (`FUN_00342af0`). The falling group (26/27) plays once and holds its last frame, lying (`FUN_004122ec`). Lines go to the talk window (the player's in the wide form); a click closes a line.
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

The sport manager (TSportManage, `PTR_DAT_004c9994`) runs the games the server starts from events. The cabin's two arcade machines (`In-Game/Arcade_Machine_01.png`, `Arcade_Machine_02.png`; events 2 and 3 on every ship cabin map) start type 3, hitting moles, with parameter 11000, and type 4, hunting, with parameter 10000. Playable local games now include types 3, 4, 5, 13 and 15. Egg draws (6/22) and slot machines (8/10/19) have client controls, native AC71 requests and server reply presentation; the server now implements atomic purchases and weighted rewards for these five kinds. Native-client acceptance remains pending. [MINIGAMES.md](MINIGAMES.md) inventories all 22 kinds, references and remaining work. Unsupported types answer a loss so the server's event goes on.

- **Start** (57/1, `FUN_003bd398`: type, u16 parameter, byte): the HUD's forms are hidden, the game object is created, and the start form (`CH_GameStartForm`, constructor `FUN_001b4d0c`, laid out by `FUN_001b5740`) shows the game's explanation picture with Start, Leave and a close button (tags 1–3, handler at 0x1b4f20). Start runs the game and shows its Exit (`CH_GameExplain`, `FUN_001b20c0`); Leave, close and Exit give up with a loss. The server's 20/9 hold follows.
- **Frame**: the main loop updates the game (`FUN_003be358`) and draws it after the map (`FUN_003bdfc8`); the game's picture covers the screen. Ground clicks go to the game (slot +4, `FUN_003bd32c`) instead of walking.
- **Result** (`FUN_003bdfa4`): 57/1 with 1 for a win or 0, and the event step is marked done (+0x7108), so the frame loop follows with 20/6. The server absorbs that acknowledgment (`resultAck`), so it doesn't answer the outcome branch's first line.
- **End** (57/2, `FUN_003bc120`): the forms come back, the game is freed and the map's music plays again.

**Hitting moles** (`minigame/mole.go`, object `FUN_001b3090`, frame `FUN_001b4820`): the parameter picks mice (11000 and 0x4a3f), turtles (0x4608) or rabbits, with their explanation form (`HitMouse_Exp_Form` and its `HitMouse_40s` patch, which half-covers the English "1 minute" line in the original too). Seven holes on `HitMouse` (map.JMG); after a countdown (beeps `Wav1605`, "Go" `Wav1610`, then `BGM0019`) the player has 40 seconds. Targets rise, stay up and sink faster as the clock runs down (`FUN_001b4908`); with fewer than 2–4 busy holes, 1–3 empty ones get a mole (76 %) or a bomb (`FUN_001b4b78`). A hit (`FUN_001b3ce8`, boxes `FUN_001b2b44`) scores 1 with `Wav1611` and the `S10416` sparkle; a bomb costs 3 with `SEB0008` and `Bomb_2`. The hammer cursor swings through shapes 13–15. Three seconds after time runs out, 30 points or more wins. The layout matches `In-Game/Mole_Minigame.png`.

**Hunting** (`minigame/hunter.go`, TSport_Hunter, `FUN_0017e2c8`): monsters (NPC templates 17114, 17174, 17186, 17036, 17064 and 17199, `DAT_004bce04`) spawn on the `59092` forest with the `59091` and `59093` layers, wander at their own pace (0.1–0.22 px/ms, `FUN_0017df40`) with eight-way walking (`FUN_00411fd8`), and after ten walks one goes to the player at (400, 560), shakes the screen (`FUN_0017fc08`) and attacks, costing a life with the body's cry. A click (`FUN_0017f714`, `SEB0057`) takes 50 from every monster under the pointer (250 for the big one); each kill scores 5. The clock ticks every 50 frames from 30; surviving it wins ("You Win"), losing three lives or downing 50 monsters loses. The original runs the update and the draw once each per 30 ms frame, and part of the logic sits in the draw, so the port runs all of it on the game frame and draws at the display rate. `animAtt` (pic\animAtt) and `SEB0090` are missing from this build, so neither shows or plays.

`TestMoleFlow`, `TestHunterFlow`, `TestAdditionalLocalMinigameFlows`, `TestNativeArcadeDispatchAndCleanup` and `TestUnportedMinigame` drive the packets; `client/wlo/minigame` tests the countdown, hits, results, facing, steps, attacks and shots. `MOLE_SNAPSHOT` and `HUNTER_SNAPSHOT` render the games and their start forms. Not ported: a dead monster's last frame (held by time here), the hunt's repeated results until 57/2 (sent once here), and the remaining kinds listed in [MINIGAMES.md](MINIGAMES.md). `ARCADE_SNAPSHOT_DIR` renders the new explanation forms and games.

### Next steps

The long-term plan is [CLIENT_ROADMAP.md](CLIENT_ROADMAP.md), and [ALOGIN_CATALOG.md](ALOGIN_CATALOG.md) catalogs every region of the original with its port status. The items below are the short-term list.

1. **The game world**: the remaining event kinds, speech bubbles, the HUD's actions, the remaining chat channels, then wandering NPCs.
   - **NPCs turning to the player** when talked to (the original Burke faces the player in `In-Game/Burke_Talk.png`; ours keeps his facing).
   - **Remaining chat presentation**: alternate backgrounds, VIP marks and character speech bubbles. Transparent mode now lets map clicks through; scroll arrows/thumb/wheel, lock, three modes, line/portrait whisper targets and the native emoticon picker, log rendering and animated editor preview work.
2. **Remaining login-phase pieces**: the talk window's buttons and typing behind the notices.
3. **Earlier startup**: the rest of FormCreate (`Transition`) and `Skins.Flst` parsing instead of the fixed white skin.
4. **Remove the legacy front end**: drop `client/ui` and `client/frontend.go` once the port covers what they show.

### Notices (ported: `client/wlo/app/notices.go`)

The notice board is `TRe_TalkMsgFormPlus_1` (`PTR_DAT_004c9e40`, constructor `FUN_0034e50c`), a talk window used for timed messages through slot +0x9c (`FUN_0034e70c`):
- at most five entries, each with its own duration; a message containing line breaks (`\r`) replaces them all and keeps its last five lines;
- the window's own timer is the newest message's duration; when it ends every entry is cleared, and with several entries one expired entry is removed per frame (`FUN_0034e614`).

Only the notice use is ported. The layout was measured from `Login_Forced_Error_Message.png` with the window's margins: `panel22` as a nine-slice 27 pixels from the top, as wide as the longest line plus 110, 18 pixels a line plus 62; centred lines that wrap at word boundaries, yellow (0xffe0) embossed text. `TestNoticeSnapshot` (opt-in) reproduces the capture; the panel lines up exactly and about 6% of its pixels differ (the dither and the background).

### Ambient and prop sounds (ported: `client/wlo/world/zones.go`, `client/wlo/app/ambient.go`)

- **Sound zones**: after its walk grid a Ground.MMG record lists 6-byte zones (`FUN_003f830c` → +0xc6ab4/+0xc64b4; the export's `unknown_triples`): a centre cell, a sound number (low byte of the third word) and a radius in cells (high byte). Every 500 ms the frame (`FUN_00498aec`, not during movies) finds the first zone whose square holds the player's cell (`FUN_003f925c`) and plays `sound\wav####.wav` around its centre (`FUN_004054f0`): on one of six looping channels, starting within the radius (×20 px) measured vertically, the volume dropping 3 hundredths of a dB per pixel and the pan following the horizontal offset (×10). A channel stops when the player leaves its range (`FUN_004057c8`); a map load or a movie stops all. Newbie Island has 105 zones: the shore's wav0050/wav0052 and the trees' wav0030/wav0031.
- **Props**: AC22:1 (`FUN_00408054`) sets an NPC's frame (+0x11f); once the map is ready, a prop going from 0 to 1 plays its template's `wav####` (Npc.dat +0x5c: wav9900 for chests, casks and the coconut, wav9902 for treasure chests).
- **Map ready** (+0x133d0): the player's AC12 clears it and 5/4, sent after the 12/1 acknowledgement, sets it. Until then prop sounds and area triggers are off, so the arrival's AC22:1 replay of already opened props (the server's `Sync`) is silent.
- Not ported: the sound options (+0x20c and the effects volume) and `FUN_004057c8`'s early return.

### Background music (ported: `client/wlo/app/music.go`)

- **Login**: FormCreate's timer (`CheckStartMusic`, `FUN_004a3c98`) plays `Sound\BGM0013.wav` through a media player and rewinds it at the end; the port loops it from startup, and again after a disconnect returns to the login screens.
- **Maps**: entering a map (0x3bce95) plays `Sound\<track>.wav`, where the track is the scene record's second name at +0x1f (`SceneData.dat`; Ship Deck plays `BGM0007`). `FUN_004048b8` keeps a track that is already playing, so walking between maps of one scene doesn't restart it. All tracks the scenes name exist in `data/media/sound` (`TestSceneMusic`).
- One audio context serves the sound effects and the music. Not ported: each map's saved position (+0x5706, +0x5710), the `musicOn`/`musicVolume` settings, the `BGM0028` override on some maps (+0x5768, `FUN_004533c0`), event music and the music player form.

### Cursor (ported: `client/wlo/cursor`)

The original uses animated Windows cursors. At startup (0x3bb990) it loads `cursor\*.ani` into `Screen.Cursors` under fixed indices and selects 12 (`cursor1.ani`, the blue arrow); the table is `cursor.Registered`. In game, `Tjo_Cursor` (`FUN_003ba9f8`; per frame `FUN_003bab68`, switch `FUN_003bac58`) maps hover states to those indices: state 0 is 12, or 9 (`cursor2.ani`) while `DAT_004ca00c` is set. It also keeps picture-strip copies of the cursors (`cursor1`… in the skin), whose frame counter nothing in the traced code draws.

The client shows them as real system cursors too, so the system draws the pointer at the display's rate, outside the game's frames, as Windows did for the original. Ebitengine has no public API for custom cursors, so `client/third_party/ebiten` holds a copy of v2.10.0 (used through a `replace` in `client/go.mod`) with a small patch: `ebiten.NewCursorImage` and `ebiten.SetCursorImage`, over its bundled GLFW's `CreateCursor`, which works on Windows, macOS and Linux (X11, and Wayland through XWayland). `client/third_party/ebiten/PATCHES.md` lists the changes for future updates. Each exported ANI frame (`data/media/cursor/<name>/`) becomes a native cursor once, and the window swaps them on the ANI's jiffy timings (1/60 s; 10 jiffies a frame for all but `cursor18`, 6). An unregistered index shows the system pointer. The XOR (inverted) pixels of the ANI frames are not reproduced. The cursor isn't part of the 16-bit screen, so snapshots don't include it. The shape names describe the artwork, since most situations are still untraced.

**Choosing the cursor** (`client/wlo/app/cursor.go`): the UI's update pass (`FUN_00467148`) asks for the hovered control's own state (+0xc5, `FUN_003ba9e8`), and the game's state applies otherwise (`FUN_003ba9d8`). `TSe_Component` sets 2, the pointing hand (`cursor4.ani`), so buttons, fields and panels (and the talk window) show it; `TSe_Form` and `TSe_FixedForm` set 0. State 0 is `cursor1.ani`, or `cursor2.ani` (the white arrow) once a held button has walked the player for a second (`DAT_00828ae0`); other states are their own index. The Login captures `Hand_Cursor.png` (hand over the password field) and `Regular_Cursor.png` (crystal over the form) and the in-game ones agree; `TestLoginCursor` and `TestNPCHover` check them. The game's other states (skills, trading …) are not ported. The field was called `Layer` in the port before.

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

Not yet traced or recreated:

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

Ground.MMG prefix decoding follows sibling `decompiled/aLogin_decompiled.c`, function `FUN_004121a8`: UInt32 scene width/height, a byte layer count with three UInt16 operands per layer, UInt16 grid width/height, and X-major raw cell bytes. The first anonymous record has dimensions 1472×960, grid 74×49, and one layer reference to resource 10000. The verified prefix consumes 3,645 bytes. Remaining record fields and index/map associations are not decoded. Cell values are not assumed to be image IDs or a complete collision policy.

## Verification and limitations

- Local migration verification passes for all 4,070 files against the SHA-256 manifest. Staging/ZIP/checksum tests, client tests and vet/build pass; the base image census and a 11016.jpg PNG export pass using only local assets. Upload packages have been prepared locally, not published.
- Directory and payload-signature checks pass for `map.JMG`, `images.BMg`, and `item.BMg` in all three client installations. These three base archives are byte-identical across the installations (SHA-256 comparison).
- Full image decoding passes for 326 map images, 2,190 item icons and 268 of 269 base UI images. `HiTurtle_2.bmp` declares 255 palette entries but uses missing palette index 255. It is retained unchanged, reported explicitly by the test, and displayed as a decode error by the workbench.
- A native 11016.jpg frame was rendered through Ebitengine and visually inspected. No player, NPC, UI composition or gameplay compatibility is implied by rendering a background.
- Image payloads are capped at 64 MiB and decoded dimensions at 128 million pixels. Unsupported/corrupt resources display an error and remain browsable. Additional expansion archives have not yet received the same complete image census.
- JMA/JXA and JXAN asset decoding is implemented in `internal/clientassets` and `cmd/sprite-export`. All selected client resources are exported under `data/sprites`; see [the sprite export documentation](../data/sprites/README.md). The native SHA padding and AES key expansion quirks are covered by independent golden vectors and authenticated native sprite tests. Three orphan JXAN files have no corresponding pixel archives in the source directory.

Run resource checks from the repository root. Linux Bash:

```bash
export WONDERLAND_CLIENT_DATA="$(pwd)/../Wonderland-Client"
export WONDERLAND_TEST_DATA="$(pwd)/../Wonderland-Client/data"
go test ./internal/clientassets -count=1
cd client
go test ./... -count=1
go vet ./...
```

Windows PowerShell:

```powershell
$env:WONDERLAND_CLIENT_DATA = (Resolve-Path ../Wonderland-Client).Path
$env:WONDERLAND_TEST_DATA = (Resolve-Path ../Wonderland-Client/data).Path
go test ./internal/clientassets -count=1
cd client
go test ./... -count=1
go vet ./...
```

Next work: character selection and creation, the untraced login details listed above, the remaining archive variants and sprite animation records, and scene/terrain references. Original-client validation remains separate from testing this replacement.

### Editable sprite assets

The character renderer loads standard PNG sheets and JSON animation indexes
from `sprites/` under the asset root (`data/sprites`), or the directory selected by `-sprites`. Editable
archives take priority over native JMA/JXA files. All available sprites have
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
Player stalls are not implemented. Sprite refresh responds with `5/8`, character ID and a zero byte;
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
records packet bodies, passwords or deletion codes. `-log-file` appends JSON
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

`ClientSocket1Disconnect` (0x49871c) acts only when a login form is open or the player is in the game. It hides the login forms and shows the message form `PTR_DAT_004ca12c` (constructor `FUN_003a5184`): `panel15`, 200 × 150 at (300, 0x50) with 15-pixel margins, an `editorBG` field (10, 0x19, 0xb4 × 0x16) holding the reason ("Connection lost" unless an action-0 packet left one), and four `btn_module_1` buttons (0x38 × 0x14, top 0x6e): Leave (0x20) and Prev (0x66) are shown; Finish (0x3c) and Update (0x66) stay hidden. Then `FUN_0049a2e8(…, 1)` starts the screen transition: each game frame `FUN_0049a484` fills the screen with black at alpha 12 × step (steps 1 to 15) after the scene and before the forms; at step 15 `FUN_0049ba08` keeps the darkened frame and the scene is no longer drawn (+0x565), so the HUD and the form stay bright over a frozen, dark scene without names or the location line. Leave (0x3a54f0) closes the client; Prev (0x3a547c) hides the form, resets the game state (`FUN_00314bf4`) and returns to the server list. `TestLostConnection`; `Chat_02/Connection_Lost.png` is the original (left) beside the port before this (right, which showed the server list over the game).

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
The adjacent arrows select player/pet views in the native client; pet views
remain pending, so these controls are disabled.
HP/SP/EXP use the native Panel31–Panel36 artwork. Combat values display the
cached AC8 words at +0x1ffc–+0x2004 read by
`FUN_0035172c`. The native battle getter (`FUN_004166e4`) calculates different
values and must not be substituted for the inventory display. Text is drawn
transparently with zero paper and skin ink (0x0841), including quantity-one
bag counters. The element control is at (19,56), as
in `FUN_0034fad4`.

Double-click or right-click a bag item to use it; drag equipment onto its worn
slot to equip it. Double-click worn equipment to return it to the first empty
bag slot. Dragging normally moves one item (`FUN_00350914`). Hold **Ctrl** while
dragging to choose a quantity (`FUN_003594b8`; the executable's modifier masks
at 00350e50/00350e54 are Ctrl with left/right mouse). Dragging outside the form
asks for a drop quantity. The native `Form_ThrowThing` keeps its baked “Moving
quantity” title for moves; the initial count is the source count limited by
the destination’s remaining stack capacity. The editor takes focus when clicked.
A protected-item server reply asks separately before
sending a destruction request. Escape closes the form and its quantity dialog.

AC23 replies alone update the bag and equipment. Addition records are additive,
and move/removal/wear/unequip acknowledgments preserve item metadata. Malformed
known packets leave the state unchanged. Equipment changes refresh both the map
character and the inventory preview. Character entry clears the previous bag.
Icons come from the exported picture atlases; names/descriptions come from the
item catalog and are encoded for the native Big5 bitmap font.

Item hover uses the native name hint and `TSe_ItemInfo` (`FUN_00287e0c`,
`FUN_00287f58`, `FUN_0028cc74`): a 196-pixel `panel4` frame beside the cell,
yellow text, type names from `FUN_00485a20`, rank from decoded record byte 45,
and non-tradeable flags from word 123 (`FUN_0028cbec`). Equipment requirements
use byte 113, independently of the legacy server Level projection. Descriptions
wrap by Big5 glyph. Native description sizing reserves extra bottom padding.
The new tooltip/move screenshot pairs check these interactions. The exported
WLRI `menu/Skins/default/panel4.bmp.png` includes green transparency markers
among the blue artwork; the picture cache applies native color-keying.
Equipment bonus/socket/forge and metadata-dependent tradeability lines still
need the remaining native item-info rules.

The attribute arrows preview STR/CON/INT/WIS/AGI allocation using the available
point budget (`FUN_00353ed4`). Confirm sends the native counted AC8:1 stat/word
request (`FUN_003540c4`); Cancel or closing the form discards the draft. Player
stats change only when the server replies. Pet-targeted stat replies no longer
overwrite player values. Submission is disabled while awaiting confirmation;
after five seconds without a reply it permits a manual retry with a notice.
Derived combat stats stay authoritative during the preview. Native rebirth/class
allocation caps and pet allocation dialogs remain pending; the server validates
the actual allocation. `TestInventoryPointDraftAndNativeRequest` and
`TestNativePresentationAndAllocation` cover budget, packets, replies and resets.

Remaining inventory work includes repair and potential dialogs,
pet equipment and secondary container/crafting forms. These are separate native
forms and are not implemented by the inventory toolbar window yet.

Focused validation:

```sh
cd client
go test ./wlo/inventory ./wlo/world ./wlo/login ./wlo/seui ./wlo/app -run 'TestInventory|TestFormula|TestEditor|TestCombo|Test.*BaseStats'
INVENTORY_SNAPSHOT=/tmp/inventory.png go test ./wlo/app -run TestInventoryFlow -count=1
INVENTORY_REFERENCE_SNAPSHOT=/tmp/inventory-reference.png go test ./wlo/app -run TestInventoryOriginalSampleSnapshot -count=1
INVENTORY_INTERACTION_SNAPSHOT=/tmp/inventory-interactions go test ./wlo/app -run TestInventoryInteractionSamples -count=1
```

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
  existing and future chat lines. **Blacklist** adds/removes case-insensitive
  names and filters their incoming player messages; system/GM notices stay visible.
- **Info Visibility** saves ten native display preferences. Other players' names
  apply to the current world renderer. Pet labels, nicknames, guild names, tent
  size and Adventure Level will use these preferences when their renderers are
  ported; they are currently pending.
- **Chat Box** toggles the log. **Spawn Point** offers Beach, Record and Carnie
  through AC5:17, with eligibility checked by the server. **Log Out** returns to
  server selection; **Exit** closes the client. Both ask for confirmation.

Local preferences live in `var/client/user/settings.json`, written atomically
outside the generated assets. They apply to this client installation; server
permissions are stored per character in `wonderland.db` and never loaded from
that local file. `app.Options.SettingsPath` supplies a separate profile/test path.

Pending: Zoom Mode rendering, title entitlement synchronization/selection,
Security Lock, Change Login, Change Password, Official Site integration, Balance
Inquiry, redemption, VIP and point-purchase dialogs. Their controls show an
unavailable notice; the title list remains empty until authoritative entitlements
are supported. Readme points to this document and GETTING_STARTED. No external
payment sites or unsupported server requests are opened by these controls.

Implementation: `client/wlo/settings`, `client/wlo/app/settings.go`. Native sources:
System constructor `0x282990`, toggle callback `0x284310`, sender
`0x2d1594..0x2d1757`, snapshot decoder `0x2ea7fe..0x2ea872`, Channels constructor
`0x281fcc`, Info `0x2824bc`, Titles `0x2a4b40`, Blacklist supplement `0x24ecb8`.
