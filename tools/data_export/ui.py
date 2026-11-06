#!/usr/bin/env python3
"""Import English and/or Chinese UI artwork into separate locale asset folders."""
import argparse
import json
from pathlib import Path

from export import require, sha256, write_json

UI_LANGUAGES = ('en', 'zh-TW')
UI_IMAGE_SUFFIXES = {'.bmp', '.jpg', '.jpeg', '.png', '.gif', '.ico'}


def skin_source(path):
    return len(path.parts) >= 2 and tuple(p.lower() for p in path.parts[:2]) == ('menu', 'skins')


def skin_sources(client):
    client = client.resolve()
    menu = next((p for p in client.iterdir() if p.is_dir() and p.name.lower() == 'menu'), client / 'menu')
    skins = next((p for p in menu.iterdir() if p.is_dir() and p.name.lower() == 'skins'), menu / 'Skins') if menu.is_dir() else menu / 'Skins'
    sources = sorted(p for p in skins.rglob('*') if p.is_file() and p.suffix.lower() in UI_IMAGE_SUFFIXES)
    require(sources, f'no UI artwork found in {skins}')
    require(all(not p.is_symlink() and p.resolve().is_relative_to(client) for p in sources), 'UI source escapes original installation')
    return sources


def export_ui(client, destination, language):
    # Lazy import also lets the complete media exporter reuse this module.
    from media import export_one
    require(language in UI_LANGUAGES, f'unsupported UI language: {language}')
    client, destination = client.resolve(), destination.resolve()
    require(client.is_dir() and not destination.is_relative_to(client), 'invalid UI source/output directories')
    output = destination / 'locales' / language
    require(not output.is_relative_to(client), 'UI output must be outside original installation')
    require(not output.exists(), f'UI output already exists; choose a new output directory: {output}')
    media_manifest = destination / 'media_manifest.json'
    existing = json.loads(media_manifest.read_text(encoding='utf-8')) if media_manifest.is_file() else None
    prefix = 'locales/' + language + '/'
    if existing:
        require(not any(row['path'].startswith(prefix) for row in existing['outputs']), 'UI metadata already declares this locale; use a fresh output directory')
    sources = skin_sources(client)
    rows = []
    for source in sources:
        row = export_one(source, client, output)
        require(sha256(source) == row['source_sha256'], f'UI source changed during export: {source}')
        rows.append(row)
    document = {'schema_version': 1, 'language': language, 'source': str(client), 'files': rows,
                'outputs': [{'path': p.relative_to(output).as_posix(), 'bytes': p.stat().st_size, 'sha256': sha256(p)}
                            for p in sorted(output.rglob('*')) if p.is_file()]}
    write_json(output / 'skin_manifest.json', document)
    if existing:
        name = prefix + 'skin_manifest.json'
        existing.setdefault('ui_manifests', []).append(name)
        existing['ui_languages'] = sorted(set(existing.get('ui_languages', ['en'])) | {language})
        existing['outputs'].extend({'path': p.relative_to(destination).as_posix(), 'bytes': p.stat().st_size, 'sha256': sha256(p)}
                                  for p in sorted(output.rglob('*')) if p.is_file())
        existing['outputs'].sort(key=lambda row: row['path'])
        write_json(media_manifest, existing)
    return document


def verify_ui(manifest):
    document = json.loads(manifest.read_text(encoding='utf-8'))
    require(document['schema_version'] == 1 and document['language'] in UI_LANGUAGES, 'invalid UI manifest')
    client, output = Path(document['source']), manifest.parent
    expected = {p.relative_to(client).as_posix() for p in skin_sources(client)}
    require(expected == {row['source'] for row in document['files']}, 'UI source census mismatch')
    for row in document['files']:
        source = client / row['source']
        require(source.is_relative_to(client) and source.stat().st_size == row['source_bytes'] and sha256(source) == row['source_sha256'], f'UI source changed: {source}')
    actual = {p.relative_to(output).as_posix() for p in output.rglob('*') if p.is_file() and p.name != 'skin_manifest.json'}
    require(actual == {row['path'] for row in document['outputs']}, 'UI output census mismatch')
    for row in document['outputs']:
        target = output / row['path']
        require(target.is_relative_to(output) and target.stat().st_size == row['bytes'] and sha256(target) == row['sha256'], f'UI output changed: {target}')
    return document


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--english', type=Path, help='Installation containing the English UI')
    parser.add_argument('--chinese', type=Path, help='Installation containing the original Chinese UI')
    parser.add_argument('--output', type=Path, required=True, help='Media directory; selected locale subdirectories must not exist')
    args = parser.parse_args()
    if not args.english and not args.chinese:
        parser.error('provide --english, --chinese, or both')
    sources = [p.resolve() for p in (args.english, args.chinese) if p]
    targets = [args.output.resolve() / 'locales' / code for code, source in [('en', args.english), ('zh-TW', args.chinese)] if source]
    for source in sources:
        require(not args.output.resolve().is_relative_to(source) and all(not target.is_relative_to(source) for target in targets), 'UI output must be outside all original installations')
        skin_sources(source)
    for code, source in [('en', args.english), ('zh-TW', args.chinese)]:
        if source:
            result = export_ui(source, args.output, code)
            print(f"Imported {len(result['files'])} {code} UI images")
