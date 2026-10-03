# Client sprites

For editable assets, use the transparent PNG sheets and `editable.json` in
each archive directory. See [Editing sprites](EDITING.md) for painting, animation
edits and direct Go client loading. The files below preserve the original data.

This directory exports all 222 files from
`Wonderland-Client/jma/`: 107 JMA pixel archives,
107 JXA animation indexes and eight JXAN encryption indexes.

The export preserves 6,257 sprites and 816,578 frames. Twenty-one sprites
required authenticated decryption. Three orphan indexes (`002w4`, `003w4`,
`004w4`) contain 18 records whose companion JMA archives are absent from the
source. Those records are saved with `missing-companion-archive` status.
Their pixel data cannot be recovered from an index alone.

## Files

Each archive has a directory named after its original filename stem:

- `sprites.json`: frame names, dimensions, anchors, pixel offsets, full native
  frame records, full BGRA palette bytes and source/decrypted SHA-256 hashes.
  `decoded_archive_zlib_base64` preserves complete decoded bytes, including
  palette indices, gaps and padding; `decoded_archive_bytes` gives their size.
  Tools reconstruct them temporarily for verification and pack generation.
  No decoded JMA files are retained.
- `actions.json`: every 80-byte JXA animation record, including readable frame
  counts by action index and original bytes. Unresolved action meanings retain
  their numeric array indexes.
- `encryption_metadata.json`, where present: complete JXAN records, key IDs,
  nonces, tags, source offsets and processing status. No secret key is exported.

`sprite_manifest.json` lists every source and output, their sizes and SHA-256
hashes, totals and the exact executable used for key extraction. Frame name
bytes that are not UTF-8 remain intact in `record_hex`.

PNG pages are ignored by Git and regenerated locally; JSON metadata and
compressed verification data are available for versioning.
The editable export uses shared PNG atlases with identical frames stored once,
avoiding 816,578 separate files. Atlas locations are recorded in `editable.json`.
Individual legacy frame PNGs can also be extracted as needed.
These are client rendering assets; this export does not add image payloads to
the server asset database or change the server's runtime configuration.

## Reproduce and verify

From the repository root:

```sh
go run ./cmd/sprite-export \
  -input ../Wonderland-Client/jma \
  -client-exe ../Wonderland-Client/aLogin.exe \
  -output data/sprites
python3 tools/data_export/verify_sprites.py
```

The exporter authenticates encrypted sprites before decoding. It reconstructs
and checks the original SHA-256 of each entire JMA archive before publishing
it, including headers, gaps and padding. The Python verifier checks every
output hash, sprite, palette and frame record, and reconstructs the JXA/JXAN
sources byte for byte from their saved records. It needs no original assets.

Key extraction supports the selected translated client executable, whose hash
is recorded in the manifest. The native loader has quirks in SHA padding and
AES key expansion; the compatibility decoder reproduces those routines. It
must not be used as a general cryptography library. Standard AES/HMAC libraries
alone cannot decode these particular files.

Extract a transparent PNG from a decrypted sprite:

```sh
go run ./cmd/sprite-export \
  -preview-archive data/sprites/002s2/sprites.json \
  -sprite 12400.jmp -frame 0 -output /tmp/12400.png
```

PNGs use the native RGB palette, with index zero transparent, and cropped frame
dimensions. Canvas placement and anchors remain in `sprites.json`.

## Source-only regeneration

`go run ./cmd/sprite-export -editable -output data-new/sprites` reads the original
sibling JMA/JXA/JXAN files and `aLogin.exe`, decrypts in temporary storage, and
exports PNG sheets and complete preservation metadata. It does not read saved
`data/sprites/` files as inputs. Decoded native bytes exist only temporarily.
The temporary working directory is removed automatically after the export.

Atlas optimization removes empty frame directories after packing.
