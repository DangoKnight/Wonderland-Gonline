"""Add evidence-backed Skill.dat labels to an existing export, preserving edits."""
import argparse
import json
from pathlib import Path

from export import sha256, write_json
from skill_fields import label_existing_export
from verify import check_readable


def label(path, manifest_path=None):
    original = json.loads(path.read_text(encoding='utf-8'))
    document = label_existing_export(original)
    check_readable(document)
    manifest = None
    if manifest_path:
        manifest = json.loads(manifest_path.read_text(encoding='utf-8'))
        entries = [entry for entry in manifest['assets']
                   if entry['asset'] == 'skill.dat' and (manifest_path.parent / entry['output']).resolve() == path.resolve()]
        if len(entries) != 1:
            raise ValueError('manifest must reference this exact skill export once')
    write_json(path, document)
    if manifest is not None:
        entries[0].update(output_bytes=path.stat().st_size, output_sha256=sha256(path))
        write_json(manifest_path, manifest)
    print(f'Labelled {len(document["records"])} skill records in {path}')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--input', type=Path, required=True)
    parser.add_argument('--manifest', type=Path, help='Optional asset_manifest.json checksum update')
    args = parser.parse_args()
    label(args.input, args.manifest)
