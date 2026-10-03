#!/usr/bin/env python3
"""Extract individual OGG files directly from the original client audio archives."""
import argparse
import hashlib
import json
from pathlib import Path

SCHEMA_VERSION = 1
OGG_HEADER_BYTES = 27
OGG_BEGIN_STREAM = 2
OGG_END_STREAM = 4
COPY_CHUNK_BYTES = 1024 * 1024


def checksum(path, size=None):
    digest = hashlib.sha256()
    with path.open('rb') as stream:
        remaining = size
        while remaining is None or remaining > 0:
            chunk = stream.read(COPY_CHUNK_BYTES if remaining is None else min(COPY_CHUNK_BYTES, remaining))
            if not chunk:
                if remaining:
                    raise ValueError('truncated audio payload')
                break
            digest.update(chunk)
            if remaining is not None:
                remaining -= len(chunk)
    return digest.hexdigest()


def validate_stream(raw, entry):
    position = pages = 0
    serial = None
    while position < len(raw):
        header = raw[position:position + OGG_HEADER_BYTES]
        if len(header) != OGG_HEADER_BYTES or header[:5] != b'OggS\0':
            raise ValueError('invalid Ogg header')
        current_serial = int.from_bytes(header[14:18], 'little')
        sequence = int.from_bytes(header[18:22], 'little')
        if pages == 0:
            if not header[5] & OGG_BEGIN_STREAM:
                raise ValueError('missing beginning-of-stream flag')
            serial = current_serial
        if current_serial != serial or sequence != pages:
            raise ValueError('invalid Ogg page sequence')
        segments = header[26]
        table = raw[position + OGG_HEADER_BYTES:position + OGG_HEADER_BYTES + segments]
        if len(table) != segments:
            raise ValueError('truncated Ogg lacing table')
        position += OGG_HEADER_BYTES + segments + sum(table)
        if position > len(raw):
            raise ValueError('truncated Ogg page')
        if position < len(raw) and header[5] & OGG_END_STREAM:
            raise ValueError('premature end-of-stream flag')
        pages += 1
    if pages != entry['pages']:
        raise ValueError('page count differs from source index')
    if not header[5] & OGG_END_STREAM and not entry.get('source_warnings'):
        raise ValueError('undocumented missing end-of-stream flag')


def extract_source(source, destination, document, verify_existing=False):
    """Export directly from original bytes; document is freshly decoded metadata."""
    stem = source.stem.lower()
    directory = destination / stem
    seen = set()
    for entry in document['entries']:
        name = entry['name']
        if Path(name).name != name or '/' in name or '\\' in name or not name.lower().endswith('.ogg'):
            raise ValueError(f'unsafe Ogg filename: {name!r}')
        if name.casefold() in seen:
            raise ValueError(f'duplicate Ogg filename: {name!r}')
        seen.add(name.casefold())
    if verify_existing:
        if not directory.is_dir():
            raise ValueError(f'{directory}: existing tracks required')
    else:
        destination.mkdir(parents=True, exist_ok=True)
        directory.mkdir()  # Protect edited artwork/audio from regeneration.
    archive = {'source': str(source.resolve()), 'source_sha256': document['source_sha256'],
               'source_bytes': document['source_bytes'],
               'payload_sha256': document['payload_sha256'], 'tracks': []}
    with source.open('rb') as stream:
        for entry in document['entries']:
            stream.seek(entry['file_offset'])
            raw = stream.read(entry['bytes'])
            validate_stream(raw, entry)
            path = directory / entry['name']
            if not verify_existing:
                with path.open('xb') as output:
                    output.write(raw)
            digest = hashlib.sha256(raw).hexdigest()
            if checksum(path) != digest:
                raise ValueError(f'{path}: output differs from original audio')
            track = {'record_index': entry['record_index'], 'resource': entry['name'],
                     'file': path.relative_to(destination).as_posix(),
                     'bytes': len(raw), 'sha256': digest}
            if entry.get('source_warnings'):
                track['source_warnings'] = entry['source_warnings']
            archive['tracks'].append(track)
    print(f'{source.name}: {len(archive["tracks"])} tracks', flush=True)
    return archive


def extract(root, destination, verify_existing=False):
    from export import export_audio, write_json
    root = root.resolve()
    destination = destination.resolve()
    if destination.is_relative_to(root):
        raise ValueError('output must be outside original source directory')
    sources = {p.name.lower(): p for p in root.iterdir() if p.is_file()}
    selected = [sources[name] for name in ('odd.dat', 'odd_d01.dat') if name in sources]
    if not selected:
        raise ValueError('no original odd.dat / odd_d01.dat audio archives found')
    if not verify_existing and ((destination / 'manifest.json').exists() or
                               any((destination / p.stem.lower()).exists() for p in selected)):
        raise ValueError('audio export exists; use a new output directory to protect edits')
    manifest = {'schema_version': SCHEMA_VERSION, 'archives': []}
    for source in selected:
        metadata = {'schema_version': SCHEMA_VERSION, 'source': str(source),
                    'source_bytes': source.stat().st_size, 'source_sha256': checksum(source)}
        # Decode the original footer and page structures; never read data/ indexes.
        document = export_audio(source, destination / (source.stem.lower() + '.ogg'),
                                metadata, copy_payload_file=False)
        manifest['archives'].append(extract_source(source, destination, document, verify_existing))
        write_json(destination / (source.stem.lower() + '_index.json'), document)
    write_json(destination / 'manifest.json', manifest)
    # Refresh the global export inventory when verifying an existing data tree.
    # Its authoritative source hashes remain independently checked against the originals.
    inventory_path = destination.parent / 'asset_manifest.json'
    if verify_existing and inventory_path.exists():
        inventory = json.loads(inventory_path.read_text())
        for entry in inventory['assets']:
            if entry['asset'] not in ('odd.dat', 'odd_d01.dat'):
                continue
            original = next(p for p in selected if p.name.lower() == entry['asset'])
            if checksum(original) != entry['source_sha256']:
                raise ValueError('global audio provenance differs from original source')
            index = destination / (original.stem.lower() + '_index.json')
            document = json.loads(index.read_text())
            entry.pop('payload', None)
            entry['audio_tracks'] = {'directory': (destination / original.stem.lower()).relative_to(destination.parent).as_posix(),
                                     'bytes': document['payload_bytes'], 'sha256': document['payload_sha256']}
            entry['output_bytes'] = index.stat().st_size
            entry['output_sha256'] = checksum(index)
        write_json(inventory_path, inventory)
    print(f'Verified {sum(len(a["tracks"]) for a in manifest["archives"])} OGG files against original archives')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    repo = Path(__file__).resolve().parents[2]
    parser.add_argument('--input', type=Path, default=repo.parent / 'Wonderland-Client' / 'data',
                        help='original client data directory; exported indexes are never inputs')
    parser.add_argument('--output', type=Path, default=repo / 'data' / 'audio')
    parser.add_argument('--verify-existing', action='store_true',
                        help='verify existing tracks against originals and refresh source-derived manifests')
    args = parser.parse_args()
    extract(args.input, args.output, args.verify_existing)
