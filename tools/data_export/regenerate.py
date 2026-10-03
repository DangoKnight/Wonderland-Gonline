#!/usr/bin/env python3
"""Regenerate ignored image/audio/video assets from the WLRI client installation."""
import argparse
from pathlib import Path
import subprocess

from export import REPO, write_json
from extract_audio import extract
from import_untracked import EXTERNAL_ASSET_DIRECTORIES, EXTERNAL_IMPORT_FORMAT
from media import export_media


def regenerate(destination, client=None):
    destination = destination.resolve()
    client = (client or REPO.parent / 'Wonderland-Client').resolve()
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
    extract(client / 'data', destination / 'audio')
    subprocess.run(['go', 'run', './cmd/sprite-export', '-editable',
                    '-input', str(client / 'jma'), '-client-exe', str(client / 'aLogin.exe'),
                    '-output', str(destination / 'sprites')], cwd=REPO, check=True)
    subprocess.run(['go', 'run', './cmd/pic-export', '-input', str(client / 'pic'),
                    '-output', str(destination / 'pictures')], cwd=REPO / 'client', check=True)
    export_media(client, destination / 'media')
    write_json(destination / 'external_asset_import.json', {
        'schema_version': 1, 'format': EXTERNAL_IMPORT_FORMAT,
        'source': str(client), 'directories': list(EXTERNAL_ASSET_DIRECTORIES),
    })
    print(f'Regenerated WLRI external assets into {destination}')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--client', type=Path, default=REPO.parent / 'Wonderland-Client',
                        help='WLRI (Wonderland Rhode Island) installation root, containing aLogin.exe')
    parser.add_argument('--output', type=Path, default=REPO / 'var' / 'asset-import',
                        help='new directory for source-derived external assets and their metadata')
    args = parser.parse_args()
    regenerate(args.output, args.client)
