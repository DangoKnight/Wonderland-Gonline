#!/usr/bin/env python3
"""Verify media output integrity and complete style/text source reconstruction."""
import argparse
import json
from pathlib import Path
import struct
from export import REPO, require, sha256
from media import MEDIA_DIRS, ROOT_RESOURCES


def reconstruct_style(doc):
    output = bytearray([doc['unknown_flag']])
    def integers(values):
        output.extend(struct.pack('<' + 'i' * len(values), *values))
    def frames(records):
        for record in records:
            integers(record['unknown_i32_fields'])
            output.append(record[f'unknown_u8_offset_{len(record["unknown_i32_fields"]) * 4}'])
    integers(doc['section_counts'])
    if doc['special_sprite']:
        integers(doc['special_sprite']['header_i32_fields'])
        frames(doc['special_sprite']['frames'])
    for record in doc['sprites']:
        integers(record['header_i32_fields'])
        frames(record['frames'])
    for record in doc['images'] + ([doc['background']] if doc['background'] else []):
        output.extend(bytes.fromhex(record['name_storage_hex']))
        integers(record['header_i32_fields'])
        frames(record['frames'])
        if 'unknown_i32_suffix' in record:
            integers([record['unknown_i32_suffix']])
    if 'timeline' in doc:
        timeline = doc['timeline']
        integers(timeline['stage_i32_values'])
        for operands in timeline['operands']:
            output.extend(struct.pack('<BBBIBBBB', *(operands[k] for k in ('unknown_u8_0','unknown_u8_1','unknown_u8_2','unknown_u32_3','unknown_u8_7','unknown_u8_8','unknown_u8_9','unknown_u8_10'))))
        output.extend(bytes.fromhex(timeline['unknown_suffix_hex']))
    for record in doc['extra_records']:
        integers(record)
    return bytes(output)


def verify(directory):
    manifest = json.loads((directory / 'media_manifest.json').read_text())
    original = Path(manifest['source'])
    expected = {p.relative_to(original).as_posix() for name in MEDIA_DIRS for p in (original / name).rglob('*') if p.is_file()}
    expected.update(name for name in ROOT_RESOURCES if (original / name).is_file())
    require(expected == {r['source'] for r in manifest['files']}, 'media source census mismatch')
    expected_outputs = {r['path'] for r in manifest['outputs']}
    actual_outputs = {p.relative_to(directory).as_posix() for p in directory.rglob('*') if p.is_file() and p.name != 'media_manifest.json'}
    require(expected_outputs == actual_outputs, 'media output census mismatch')
    for row in manifest['outputs']:
        path = directory / row['path']
        require(path.is_relative_to(directory) and path.stat().st_size == row['bytes'] and sha256(path) == row['sha256'], f'media output mismatch: {path}')
    reconstructed = 0
    collections = {}
    for row in manifest['files']:
        source = original / row['source']
        require(source.stat().st_size == row['source_bytes'] and sha256(source) == row['source_sha256'], f'original source changed: {source}')
        if row.get('format') == 'avi':
            video = directory / row['video']
            require(video.read_bytes() == source.read_bytes(), f'preserved video differs from source: {video}')
            reconstructed += 1
        if 'metadata' not in row:
            continue
        metadata = row['metadata']
        if 'record' in row:
            if metadata not in collections:
                collection = json.loads((directory / metadata).read_text())
                require(collection['format'] == 'style-collection', 'invalid style collection format')
                keys = {entry['record'] for entry in manifest['files'] if entry.get('metadata') == metadata}
                require(set(collection['styles']) == keys, 'style collection census mismatch')
                collections[metadata] = collection['styles']
            doc = collections[metadata][row['record']]
            require(doc['source'] == row['source'] and doc['source_sha256'] == row['source_sha256'], 'style collection provenance mismatch')
        else:
            doc = json.loads((directory / metadata).read_text())
        raw = None
        if doc['format'] == 'style-animation':
            raw = reconstruct_style(doc)
        elif doc['format'] in ('text-resource', 'unresolved-resource'):
            raw = bytes.fromhex(doc['source_hex'])
        elif doc['format'] in ('bitmap-font', 'encrypted-officer-text'):
            raw = bytes(b ^ doc['source_xor'] for b in bytes.fromhex(doc['decoded_hex']))
        elif doc['format'] == 'encrypted-agreement-lines':
            raw = b''.join(bytes([len(bytes.fromhex(line['decoded_hex']))]) + bytes(b ^ doc['source_xor'] for b in bytes.fromhex(line['decoded_hex'])) for line in doc['lines'])
        if raw is not None:
            require(raw == source.read_bytes(), f'decoded source reconstruction failed: {source}')
            reconstructed += 1
    print(f'Verified {len(expected)} original media sources, {len(expected_outputs)} outputs, {reconstructed} byte-exact reconstructions; {len(manifest["unresolved"])} unresolved resources')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--data', type=Path, default=REPO / 'data' / 'media')
    verify(parser.parse_args().data.resolve())
