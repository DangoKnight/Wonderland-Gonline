# Editable picture assets

PNG exports of `Wonderland-Client/pic/`.

- Pictures up to 512×512 share 2048×2048 PNG pages under `atlases/<archive>/`.
  Identical pixels share a rectangle; frames have one pixel of padding.
- Larger images remain standalone, under their archive stem or at this root.
- Patch archives remain separate (`map_c01/`, `map_d01/`, `item1_d01/`, etc.).
  Matching resource names across archives are intentional; preserve their priority
  when selecting artwork. Map loading searches `map_d01`, `map_c01`, then `map`.
- `manifest.json` schema 2 maps every picture to its original resource and source file, with
  dimensions and the SHA-256 of each original file. Paths are relative to this
  directory. For packed images, `logical_path` retains the original image path,
  `png` names the atlas page, and `rect` gives x, y, width and height.
  Native archive entry order is retained.
- `.bls` files are resource lists; their lines, including blank lines, are retained
  in the manifest. `Thumbs.db` is a Windows thumbnail cache and is skipped.

## Pixels and limitations

BMP palette colors remain unchanged. Original green color-key backgrounds remain
visible green; the export does not infer transparency or split multi-frame images.
JPEGs use the project's native-client-compatible decoder before PNG encoding.
Some native BMPs reference missing palette colors. Only those undefined colors
fall back to black, as in Pillow; affected images have a manifest note. The
original source files remain untouched.

The Go client resolves logical names through this manifest when loading from
`data/`, including patch priority and cropped atlas rectangles. Edit PNG pixels
inside the referenced rectangle. Editing a shared rectangle affects all aliases;
to separate an image, point its `png` at a new standalone PNG and remove `rect`,
keeping `logical_path` unchanged. Larger images can be edited directly.

## Regenerate

From the `client/` module directory:

```sh
go run ./cmd/pic-export \
  -input /home/dango/Personal/Wonderland/Wonderland-Client/pic \
  -output ../data/pictures-new
```

The destination must not exist, protecting edited artwork. Compare a new export
before replacing existing PNGs. PNGs stay local and are ignored by Git, like the editable sprite images.
Keep the manifest and this documentation tracked.

## Optimize an existing export

From `client/`, run `go run ./cmd/pic-export -optimize ../data/pictures`.
This converts schema 1 exports once, compares every packed pixel, removes the
replaced small PNGs and empty directories, then publishes the prepared directory.
Failures leave the previous export intact. Fresh source exports atlas automatically.
