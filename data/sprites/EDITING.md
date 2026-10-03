# Editing sprites

The editable format uses ordinary RGBA PNG sprite sheets and JSON. The Go
client loads these files directly. Sprite edits do not need a native client
executable, asset encryption, JMA/JXA reconstruction or a client rebuild.

All 6,257 available sprites and 816,578 frames are exported with 107
`editable.json` indexes. Sheets use the same atlas packing and duplicate-frame
elimination as the client asset build.
The three missing native archives described in README.md cannot be exported.

## Find and paint a sprite

For example, sprite `12400.jmp` lives in:

```text
data/sprites/002s2/
  editable.json
  atlas/
    page_000.png
```

Open the PNG in an image editor such as GIMP, Krita or Aseprite. Frames share a
sheet. Identical frames may share exactly the same rectangle, even across
sprites in an archive; painting that rectangle changes every frame that uses it.
To edit one frame independently, copy its pixels into a separate PNG and point
only that frame at the new sheet and rectangle.
Paint within the frame's rectangle to change its appearance without changing
JSON. Preserve transparency and sheet dimensions when repainting an existing
frame. Ordinary RGB colors, opaque black and partial alpha are supported;
there is no 256-color limit. The display still uses the client's RGB565 buffer.

Most sprites have one sheet. Larger ones have several numbered pages.
Directories use the sprite ID; malformed native names use `sprite_XXXX`, with
`XXXX` identifying their archive slot. Frame names are descriptive metadata.

## Change placement or animation

Find the sprite by `name` in `editable.json`. Its `index` is the stable archive
slot used for character and equipment lookup. Keep this value when editing;
sprite objects may be reordered in the JSON array.

Each frame specifies:

- `sheet`: a relative PNG path; it may point to your own replacement PNG.
- `rect`: the frame's `x`, `y`, `width` and `height` inside that PNG.
- `canvas_width` and `canvas_height`: the logical canvas used for placement.
- `anchor_x` and `anchor_y`: the cropped frame's offset within that canvas.

To replace a frame with an individual image, give it a PNG path and a rectangle
starting at `(0, 0)` with the image's dimensions. Paths must stay within the
archive directory. Sheet dimensions may be up to 4096 by 4096 pixels; exported
sheets normally stay within 2048 by 2048.

An animation contains an ordered array of frame indexes. For example:

```json
{"name": "action_00", "frames": [0, 1, 2, 1]}
```

Change this array to reorder or repeat frames. Add a frame object and reference
its index to extend an animation. Keep animation objects in their existing
order, because the client selects actions by array index. Names such as
`action_00` are stable placeholders for unresolved native action meanings.
The initial export keeps the current loader's 80 native action slots.
A `-1` reference means that the original index referenced a missing frame;
that position draws nothing. Unreferenced frames are retained for editing.
The client's existing animation clock controls timing.

## Use edited sprites in the Go client

From the repository root:

```sh
cd client
go run . -sprites ../data/sprites
```

For a compiled client, pass `-sprites <directory>` to its executable. Without
this option it looks under `sprites/` in its asset root (`data/sprites`), which
standalone distributions copy with the rest of `data/` (`cmd/asset-build
-copy`). No JMA files or source executable are needed for those sprites.

The client never reads native JMA/JXA archives. Invalid editable JSON or images
produce a diagnostic; the client does not substitute original artwork.

Restart the client after edits, because sprite metadata and images are cached.

Validate your edited files without comparing them to the originals:

```sh
go run ./cmd/sprite-export -verify-editable
```

## Conversion and initial verification

To create editable assets directly from the original sibling client archives
and executable (existing data files are not regeneration inputs):

```sh
go run ./cmd/sprite-export -editable
```

Convert one archive with `-archive 002s2`. Regeneration refuses to overwrite
existing editable indexes. `-overwrite-edits` explicitly replaces existing
artwork and animation definitions; keep your edits before using it.

For initial conversion checks, compare every PNG pixel and placement against
the decoded sources:

```sh
go run ./cmd/sprite-export -verify-editable -compare-original
```

The standard-format runtime does not use the lossless `sprites.json`,
`actions.json` or JXAN metadata. Compressed decoded bytes in `sprites.json` remain
conversion and preservation artifacts. Their source manifest checks do not
restrict deliberate edits to PNGs or `editable.json`.

The original Windows client still requires its proprietary formats. Direct
PNG/JSON loading is a feature of the Go client.

## Build sprite packs

To repack the editable export into consolidated atlas pages for distribution, see [Sprite packs](../../docs/SPRITE_PACKS.md). The client reads the existing decompiled tree directly. Verify exports without repacking:

```sh
go run ./cmd/asset-build
```

To create optional packs separately, select them with the client's `-sprites` override:

```sh
go run ./cmd/sprite-build -editable data/sprites -output var/client-spritepacks
```

## Optimize existing sheets

```sh
go run ./cmd/sprite-build -editable data/sprites -optimize-editable
```

Use `-archives 002c,002h` to optimize selected sets. The shared client builder
deduplicates identical frames, packs padded atlas pages and trims the final
page. Every frame's visible RGBA pixels are compared before publication.
Frame dimensions, names, anchors, animations and painted edits are preserved.
Only previously referenced sheets are removed; unrelated files and lossless
preservation outputs stay intact. Atlas PNGs remain Git-ignored.

Source-only `sprite-export -editable` regeneration performs this optimization
automatically after fresh sibling-source decryption. The optimized data layout
retains `editable.json` for direct client loading; it does not require a separate
client assets directory. Client packaging can still build palette-indexed packs.
