#!/usr/bin/env python3
"""Regenerate ignored client tables, metadata and media from a WLRI installation."""
import argparse
from pathlib import Path
import subprocess
import json
import shutil

from export import REPO, export_all, sha256, write_json, tool_command
from import_untracked import EXTERNAL_ASSET_DIRECTORIES, EXTERNAL_ASSET_FILES, EXTERNAL_IMPORT_FORMAT
from media import export_media
from media_formats import validate_output_formats
from ui import skin_sources
from text_languages import annotate_languages, native_rows, TABLE_FILES


def preserve_server_exports(destination, server_exports):
    """Retain repository-owned server definitions without needing their originals."""
    if not server_exports.is_dir():
        raise ValueError(f'missing repository server definitions: {server_exports}')
    manifest_path = destination / 'asset_manifest.json'
    manifest = json.loads(manifest_path.read_text(encoding='utf-8'))
    known = {entry['asset'] for entry in manifest['assets']}
    for source in sorted(server_exports.rglob('*.json')):
        if source.is_symlink():
            raise ValueError(f'symbolic link in server definitions: {source}')
        document = json.loads(source.read_text(encoding='utf-8'))
        relative = source.relative_to(server_exports)
        asset = Path(document['source'].replace('\\', '/')).name.lower()
        origin = 'server'
        if relative.parts[0] == 'listdata':
            asset, origin = 'listdata/' + asset, 'server-listdata'
        if asset in known:
            continue  # Native client data retains precedence.
        output = destination / 'server' / relative
        output.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(source, output)
        manifest['assets'].append({
            'asset': asset, 'origin': origin, 'source': document['source'],
            'source_bytes': document['source_bytes'], 'source_sha256': document['source_sha256'],
            'output': output.relative_to(destination).as_posix(),
            'output_bytes': output.stat().st_size, 'output_sha256': sha256(output),
        })
        known.add(asset)
    manifest['server_exports_source'] = str(server_exports.resolve())
    write_json(manifest_path, manifest)


def regenerate(destination, client, ui_language=None, chinese_client=None, client_language='en'):
    destination = destination.resolve()
    client = client.resolve()
    if not client.is_dir():
        raise ValueError(f'missing original WLRI client: {client}')
    if client_language not in ('en', 'zh-TW'):
        raise ValueError('invalid client language')
    if chinese_client is not None:
        chinese_client = chinese_client.resolve()
        if client_language != 'en':
            raise ValueError('--chinese-client supplements an English --client; omit it for Chinese-only input')
        if destination.is_relative_to(chinese_client):
            raise ValueError('output must be outside the Chinese installation')
        # Validate all companion text tables before creating any output.
        for table in TABLE_FILES:
            native_rows(chinese_client, table)
    if ui_language is None:
        ui_language = 'both' if chinese_client is not None else client_language
    if client_language == 'zh-TW' and ui_language != 'zh-TW':
        raise ValueError('Chinese-only input provides zh-TW UI; use --ui-language zh-TW')
    if ui_language not in ('en','zh-TW','both'):
        raise ValueError('invalid UI language selection')
    if ui_language == 'both' and chinese_client is None:
        raise ValueError('--chinese-client is required for both UI languages')
    if client_language == 'en' and ui_language == 'zh-TW' and chinese_client is None:
        raise ValueError('Chinese UI requires --chinese-client or --client-language zh-TW')
    if ui_language != 'en':
        chinese_source = (chinese_client or client).resolve()
        if destination.is_relative_to(chinese_source):
            raise ValueError('output must be outside the Chinese UI installation')
        skin_sources(chinese_source)
    if destination.is_relative_to(client):
        raise ValueError('output must be outside the original client installation')
    for directory in (client / 'data', client / 'jma', client / 'pic'):
        if not directory.is_dir():
            raise ValueError(f'missing original asset directory: {directory}')
    if not (client / 'aLogin.exe').is_file():
        raise ValueError(f'missing matching client executable: {client / "aLogin.exe"}')
    if destination.exists():
        raise ValueError('export into a new directory to protect existing asset edits')
    export_all(client / 'data', None, destination)
    annotate_languages(destination, client_language, chinese_client)
    preserve_server_exports(destination, REPO / 'data' / 'server')
    print('Exporting and optimizing native sprite archives...', flush=True)
    subprocess.run(tool_command('sprite-export', '-editable',
                    '-input', str(client / 'jma'), '-client-exe', str(client / 'aLogin.exe'),
                    '-output', str(destination / 'sprites')), cwd=REPO, check=True)
    # pic-export lives in the client module only for source-checkout execution.
    print('Exporting native picture archives...', flush=True)
    subprocess.run(tool_command('pic-export', '-input', str(client / 'pic'),
                    '-output', str(destination / 'pictures')), cwd=REPO / 'client', check=True)
    print('Exporting UI, fonts, cursors, sound and movie resources...', flush=True)
    if ui_language == 'en' and chinese_client is None:
        export_media(client, destination / 'media')
    else:
        export_media(client, destination / 'media', ui_language=ui_language, chinese_client=chinese_client)
    print('Validating extracted media formats...', flush=True)
    formats = validate_output_formats(destination)
    write_json(destination / 'media_formats.json', formats)
    write_json(destination / 'external_asset_import.json', {
        'schema_version': 1, 'format': EXTERNAL_IMPORT_FORMAT,
        'source': str(client), 'directories': list(EXTERNAL_ASSET_DIRECTORIES),
        'files': list(EXTERNAL_ASSET_FILES),
    })
    print(f'Regenerated WLRI external assets into {destination}')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--client', type=Path, required=True,
                        help='WLRI (Wonderland Rhode Island) installation root, containing aLogin.exe')
    parser.add_argument('--output', type=Path, default=REPO / 'var' / 'asset-import',
                        help='new directory for source-derived external assets and their metadata')
    parser.add_argument('--client-language', choices=('en', 'zh-TW'), default='en', help='Language of --client: manually translated English (default) or original Chinese')
    parser.add_argument('--ui-language', choices=('en', 'zh-TW', 'both'), help='UI image sets to include; defaults to the supplied source language(s)')
    parser.add_argument('--chinese-client', type=Path, help='Optional original Chinese installation supplying matching text and UI alongside English --client')
    args = parser.parse_args()
    regenerate(args.output, args.client, args.ui_language, args.chinese_client, args.client_language)
