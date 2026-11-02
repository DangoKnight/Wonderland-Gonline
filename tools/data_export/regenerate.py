#!/usr/bin/env python3
"""Regenerate ignored client tables, metadata and media from a WLRI installation."""
import argparse
from pathlib import Path
import subprocess
import json
import shutil

from export import REPO, export_all, sha256, write_json
from import_untracked import EXTERNAL_ASSET_DIRECTORIES, EXTERNAL_ASSET_FILES, EXTERNAL_IMPORT_FORMAT
from media import export_media


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


def regenerate(destination, client):
    destination = destination.resolve()
    client = client.resolve()
    if not client.is_dir():
        raise ValueError(f'missing original WLRI client: {client}')
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
    preserve_server_exports(destination, REPO / 'data' / 'server')
    subprocess.run(['go', 'run', './cmd/sprite-export', '-editable',
                    '-input', str(client / 'jma'), '-client-exe', str(client / 'aLogin.exe'),
                    '-output', str(destination / 'sprites')], cwd=REPO, check=True)
    subprocess.run(['go', 'run', './cmd/pic-export', '-input', str(client / 'pic'),
                    '-output', str(destination / 'pictures')], cwd=REPO / 'client', check=True)
    export_media(client, destination / 'media')
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
    args = parser.parse_args()
    regenerate(args.output, args.client)
