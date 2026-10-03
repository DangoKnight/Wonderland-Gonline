# Sprite packs

Sprite packs are the client's runtime sprite format. Each pack is an ordinary directory of RGBA PNG atlas pages with one JSON index, named after the original archive (`002c`, `001_c01`, …):

```text
var/client-spritepacks/
  index.json          list of packs with sprite, frame and page counts
  002c/
    pack.json
    page_000.png
    index_000.png     palette indices of page_000's frames
```

## Build

The normal client reads the editable atlases directly. `cmd/asset-build` verifies that data tree; `-copy` creates a distribution
copy without generating another set of packs. Optional packs
can be built for palette-index rendering or format inspection; select them with
the client's `-sprites var/client-spritepacks` override. To build from the editable export in `data/sprites` (see `data/sprites/EDITING.md`), from the repository root:

```sh
go run ./cmd/sprite-build -editable data/sprites -output var/client-spritepacks
```

Or build directly from a client's original `jma/` directory:

```sh
go run ./cmd/sprite-build -jma ../Wonderland-Client/jma -output var/client-spritepacks
```

Options:
- `-archives 002c,002h` rebuilds only the listed archives.
- `-workers N` sets how many archives are built in parallel.

Each pack is written to a temporary directory and replaces the previous pack only when complete. A directory that isn't a pack is never replaced. `index.json` lists every pack in the output directory.

The full export (107 archives, 6,257 sprites, 816,578 frames) builds in about 24 seconds into 503 pages and their index pages, about 890 MB. Identical frames within an archive are stored once.

## `pack.json`

```json
{
  "version": 1,
  "archive": "002c",
  "frame_ms": 100,
  "source_bytes": 1234567,
  "pages": ["page_000.png"],
  "index_pages": ["index_000.png"],
  "sprites": [{
    "index": 0, "id": 2200, "name": "2200.jmp",
    "palette": "000000ff00ff…",
    "frames": [{"name": "00-1.bmp", "page": 0,
                "rect": {"x": 0, "y": 0, "w": 22, "h": 19}, "indexed": true,
                "offset_x": -11, "offset_y": -76,
                "canvas_width": 128, "canvas_height": 128, "anchor_x": 53, "anchor_y": 20}],
    "animations": [{"action": 0, "name": "action_00", "frames": [0, 1, 2, 3]}]
  }]
}
```

- **Sprite identity:** `index` is the sprite's archive slot, which the client's ID lookup resolves (`FUN_00302934`). `id` is the numeric sprite name, or −1 if the name isn't numeric.
- **Frame location:** `page` and `rect` locate a frame's pixels. A frame with a zero-size rectangle draws nothing.
- **Frame placement:** `offset_x` and `offset_y` give the frame's top-left corner relative to the drawing point (a character's feet). They resolve the original placement: the canvas is centred on the point and its bottom sits 32 pixels below it. The canvas and anchor are kept for editing.
- **Animations:** listed by action index. `-1` entries draw nothing. `frame_ms` is the animation step (100 ms, as in `FUN_00411f54`).
- **Palette indices:** the client recolours characters by shifting palette ranges, so packs keep each frame's original 8-bit indices.
    - `index_pages` parallel `pages`: 8-bit grey PNGs holding a frame's indices at the same rectangle as its pixels, or `""` for a page without indexed frames.
    - `palette` is the sprite's 256 RGB entries in hex.
    - `indexed` marks a frame that has indices.
    - Builds from the original archives index every frame. Builds from the editable export take the indices from each archive's compressed decoded bytes in `sprites.json`, but only for frames whose PNG pixels still match the original. An edited frame is stored without indices and draws as painted.
- **Packing:** pages are at most 2048 pixels per side (4096 accepted). Frames keep one transparent pixel of padding, so GPU sampling never bleeds between them.

Packs are validated on load: page paths must stay inside the pack directory, and frame, page and animation references must be in range.

## Client

`client/wlo/sprites.Manager` loads archives on demand and searches, in order:
1. the `-sprites` directory (either packs or the editable export);
2. `sprites/` under the asset root (`data/sprites`, the editable export).

The client never reads the original `.jma`, `.Jxa` or `.jxan` files; build packs from them with `cmd/sprite-build -jma`. The editable export is read as a pack, with its sheets as the pages, so edits show without rebuilding. Its sheets carry no palette indices, so the manager derives them (`client/wlo/sprites/native.go`): on an archive's first use it opens the archive's `sprites.json` in the background (about 0.7 s for `001`), and each sprite is then matched once, when first asked for, exactly as a pack build matches it. A frame whose pixels still match the original gets its indices; an edited frame keeps its painted colours. Until `sprites.json` is open, frames draw as painted. The decoded bytes live in an unlinked temporary file (removed by `Manager.Close` on Windows). `TestEditableIndicesMatchPack` checks that derived palettes and indices equal a built pack's.

`source_bytes` records the original `.jma` size. The client derives the original's content level from `001`'s size (`FUN_004a3b50`); it selects the `_L`/`_R` character-selection pictures. Builds from the editable export read the sizes from `sprite_manifest.json`.

- **Frames:** `Frame.Image` gives CPU pixels for the 16-bit renderer, and `Frame.Indices` gives palette indices for `Sprite.Palette`. `Frame.Ebiten` gives an `*ebiten.Image` sub-image of the uploaded page, and `Frame.Draw` draws at the frame's offset from a point.
- **Animation:** `Player` steps an action's animation on the pack's clock.
- **Memory:** decoded pages are cached up to 256 MB, and older pages are dropped first.

The character renderer (`client/wlo/role`) draws through the manager. With fixed animation clocks and colours left out, the character selection renders identically from built packs and from the editable export, and identically to the earlier rendering from the original archives. `TestNativeSourcesAgree` checks that builds from both sources give the same pixels, indices and palettes for `002c`.

Code: `internal/spritepack` (format, builder, sources), `cmd/sprite-build`, `client/wlo/sprites`.
