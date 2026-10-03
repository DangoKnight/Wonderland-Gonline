#!/usr/bin/env python3
"""Restore missing ignored data payloads from a freshly regenerated asset tree."""
import argparse
import hashlib
import json
from pathlib import Path
import shutil
import subprocess

REPO = Path(__file__).resolve().parents[2]
EXCLUDED_ASSET_OUTPUTS = {'server/database_data.json', 'server/database-data.json',
                          'server/database.override.json'}
EXTERNAL_ASSET_DIRECTORIES = ('audio', 'sprites', 'pictures', 'media')
EXTERNAL_IMPORT_FORMAT = 'wlri-external-assets'


def checksum(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def portable_metadata(value):
    """Ignore relocated source paths and hashes of JSON containing those paths."""
    if isinstance(value, list):
        return [portable_metadata(row) for row in value]
    if not isinstance(value, dict):
        return value
    result = {}
    json_output = str(value.get('output', value.get('path', ''))).endswith('.json')
    for key, item in value.items():
        if key in ('source', 'client_source', 'server_source', 'key_source') and isinstance(item, str) and Path(item).is_absolute():
            continue
        if json_output and key in ('output_bytes', 'output_sha256', 'bytes', 'sha256'):
            continue
        result[key] = portable_metadata(item)
    return result


def restore(source, repo=REPO, dry_run=False):
    source = source.resolve()
    repo = repo.resolve()
    target = repo / 'data'
    if target.is_symlink():
        raise ValueError('data directory must not be a symbolic link')
    if not source.is_dir() or source == target.resolve():
        raise ValueError('source must be a separate, freshly regenerated asset directory')
    external = source / 'external_asset_import.json'
    scoped = external.is_file()
    if scoped:
        scope = json.loads(external.read_text())
        if scope.get('schema_version') != 1 or scope.get('format') != EXTERNAL_IMPORT_FORMAT or scope.get('directories') != list(EXTERNAL_ASSET_DIRECTORIES):
            raise ValueError('invalid external asset import scope')
        for directory in EXTERNAL_ASSET_DIRECTORIES:
            if not (source / directory).is_dir():
                raise ValueError(f'missing external asset directory: {directory}')
    tracked = set(subprocess.check_output(['git', 'ls-files', '-z', '--', 'data/'], cwd=repo).decode().split('\0'))
    # Check all tracked JSON before writing anything. Source provenance and frame
    # layouts must agree; copying new pixels beneath old manifests is unsafe.
    for name in sorted(tracked):
        if not name.endswith('.json'):
            continue
        if scoped and Path(name).parts[1] not in EXTERNAL_ASSET_DIRECTORIES:
            continue
        current = repo / name
        generated = source / Path(name).relative_to('data')
        if not generated.is_file():
            raise ValueError(f'regenerated tree is missing tracked metadata: {name}')
        if checksum(current) == checksum(generated):
            continue
        if portable_metadata(json.loads(current.read_text())) != portable_metadata(json.loads(generated.read_text())):
            raise ValueError(f'metadata differs: {name}; use the complete regenerated tree as an alternate asset root')
    candidates = {}
    for item in sorted(source.rglob('*')):
        if item.is_symlink():
            raise ValueError(f'symbolic link in regenerated tree: {item}')
        if item.is_file():
            relative = item.relative_to(source).as_posix()
            if relative in EXCLUDED_ASSET_OUTPUTS:
                continue
            if scoped and Path(relative).parts[0] not in EXTERNAL_ASSET_DIRECTORIES:
                continue
            name = 'data/' + relative
            if name not in tracked:
                if not (repo / name).resolve().is_relative_to(target.resolve()):
                    raise ValueError(f'destination escapes data directory: {name}')
                candidates[name] = item
    if not candidates:
        return 0
    checked = subprocess.run(['git', 'check-ignore', '-z', '--stdin'], input=('\0'.join(candidates) + '\0').encode(), cwd=repo, stdout=subprocess.PIPE, check=False)
    if checked.returncode not in (0, 1):
        raise ValueError('git check-ignore failed')
    ignored = set(checked.stdout.decode().split('\0')) - {''}
    pending = []
    for name in sorted(ignored):
        destination = repo / name
        if not destination.resolve().is_relative_to(target.resolve()):
            raise ValueError(f'destination escapes data directory: {name}')
        # Preserve edited artwork and existing local outputs, including broken links.
        if destination.exists() or destination.is_symlink():
            continue
        pending.append((candidates[name], destination))
    print(f'{len(pending)} missing ignored asset files; existing files and tracked metadata are preserved')
    if not dry_run:
        for original, destination in pending:
            destination.parent.mkdir(parents=True, exist_ok=True)
            with original.open('rb') as incoming, destination.open('xb') as outgoing:
                shutil.copyfileobj(incoming, outgoing)
    return len(pending)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--from', dest='source', type=Path, required=True, help='fresh complete output of regenerate.py')
    parser.add_argument('--dry-run', action='store_true', help='validate compatibility and report the missing file count without copying')
    args = parser.parse_args()
    try:
        restore(args.source, dry_run=args.dry_run)
    except (ValueError, OSError, subprocess.CalledProcessError) as error:
        parser.exit(1, f'{error}\n')
