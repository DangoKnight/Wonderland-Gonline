#!/usr/bin/env python3
"""Verify saved sprite payloads and independently reconstruct metadata files."""
import argparse
import base64
import zlib
import codecs
import hashlib
import json
from pathlib import Path
import struct

# encoding/json replaces each invalid UTF-8 byte separately.
codecs.register_error('go_utf8', lambda error: ('\ufffd' * (error.end - error.start), error.end))

HEADER_BYTES = 14
ENTRY_BYTES = 28
FRAME_BYTES = 40
PALETTE_BYTES = 1024


def digest(data):
    return hashlib.sha256(data).hexdigest()


def verify(root):
    manifest = json.loads((root / 'sprite_manifest.json').read_text())
    sprites = frames = decrypted = missing = 0
    for source in manifest['sources']:
        for output in source['outputs']:
            path = root / output['path']
            assert path.is_relative_to(root), path
            assert path.stat().st_size == output['bytes'], path
            with path.open('rb') as stream:
                assert hashlib.file_digest(stream, 'sha256').hexdigest() == output['sha256'], path
        metadata = next(root / out['path'] for out in source['outputs'] if out['path'].endswith('.json'))
        document = json.loads(metadata.read_text())
        assert document['source_sha256'] == source['sha256'], metadata
        if source['format'] != 'decompiled-sprites':
            original = bytes.fromhex(document['header_hex']) + b''.join(bytes.fromhex(row['record_hex']) for row in document['records'])
            assert len(original) == source['bytes'] and digest(original) == source['sha256'], metadata
            missing += sum(row.get('source_status') == 'missing-companion-archive' for row in document['records'])
            continue
        payload = zlib.decompress(base64.b64decode(document['decoded_archive_zlib_base64']))
        assert len(payload) == document['decoded_archive_bytes'], metadata
        assert len(payload) == source['bytes'] and digest(payload) == document['payload_sha256'], metadata
        assert payload[:HEADER_BYTES] == bytes.fromhex(document['header_hex']), metadata
        assert struct.unpack_from('<I', payload, 10)[0] == len(document['sprites']), metadata
        changed = 0
        for index, sprite in enumerate(document['sprites']):
            entry = payload[HEADER_BYTES + index * ENTRY_BYTES:HEADER_BYTES + (index + 1) * ENTRY_BYTES]
            assert entry[1:1 + entry[0]].decode() == sprite['name'], metadata
            assert struct.unpack_from('<II', entry, 20) == (sprite['bytes'], sprite['file_offset']), metadata
            data = payload[sprite['file_offset']:sprite['file_offset'] + sprite['bytes']]
            assert data[:2] == b'jm' and digest(data) == sprite['decoded_sha256'], metadata
            assert data[:HEADER_BYTES] == bytes.fromhex(sprite['header_hex']), metadata
            count = struct.unpack_from('<I', data, 10)[0]
            assert count == len(sprite['frames']), metadata
            palette_offset = HEADER_BYTES + count * FRAME_BYTES
            assert data[palette_offset:palette_offset + PALETTE_BYTES] == bytes.fromhex(sprite['palette_bgra_hex']), metadata
            for number, frame in enumerate(sprite['frames']):
                record = data[HEADER_BYTES + number * FRAME_BYTES:HEADER_BYTES + (number + 1) * FRAME_BYTES]
                assert record == bytes.fromhex(frame['record_hex']), metadata
                assert record[1:1 + min(record[0], 11)].decode('utf-8', errors='go_utf8') == frame['name'], metadata
                fields = struct.unpack_from('<HHHHIIIII', record, 12)
                names = ('canvas_width', 'canvas_height', 'anchor_x', 'anchor_y', 'pixel_bytes', 'pixel_offset_in_sprite', 'width', 'height', 'unknown_u32_offset_36')
                assert fields == tuple(frame[name] for name in names), metadata
                stride = (frame['width'] + 3) // 4 * 4
                assert stride == frame['stride'], metadata
                assert stride * frame['height'] <= frame['pixel_bytes'], metadata
                assert frame['pixel_offset_in_sprite'] + stride * frame['height'] <= len(data), metadata
            changed += sprite['status'] == 'decrypted-and-authenticated'
            sprites += 1
            frames += count
        assert changed == source.get('encrypted_sprites', 0), metadata
        if not changed:
            assert digest(payload) == source['sha256'], metadata
        decrypted += changed
    assert (sprites, frames, decrypted) == (manifest['sprites'], manifest['frames'], manifest['decrypted_sprites'])
    print(f"Verified {len(manifest['sources'])} sources, {sprites} sprites, {frames} frames, {decrypted} decrypted sprites; {missing} records have missing source archives")


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('directory', nargs='?', type=Path, default=Path('data/sprites'))
    args = parser.parse_args()
    verify(args.directory.resolve())
