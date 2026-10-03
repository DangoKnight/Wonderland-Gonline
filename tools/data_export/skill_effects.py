"""Build effect annotations from fresh native Skill.dat exports, never saved data."""
import argparse
import json
from pathlib import Path
import subprocess
from export import REPO, FORMATS, export_records, inventory, sha256, write_json

ASSET = 'skill_effects.json'

def annotate(skills, effects):
    original = json.loads(skills.read_text())
    subprocess.run(['go', 'run', './cmd/skill-effects-export', '-skills', str(skills),
                    '-effects', str(effects)], cwd=REPO, check=True)
    annotated = json.loads(skills.read_text())
    for old, new in zip(original['records'], annotated['records'], strict=True):
        old['fields']['effect_refs'] = new['fields']['effect_refs']
    write_json(skills, original)

def entry(skills_entry, effects):
    return {**{k: skills_entry[k] for k in ('origin','source','source_bytes','source_sha256')},
            'asset': ASSET, 'status': 'derived-compatibility-effects', 'output': effects.name,
            'output_bytes': effects.stat().st_size, 'output_sha256': sha256(effects)}

def refresh(destination):
    manifest_path = destination / 'asset_manifest.json'
    manifest = json.loads(manifest_path.read_text())
    selected, _ = inventory(Path(manifest['client_source']), Path(manifest['server_source']))
    origin, source = selected['skill.dat']
    metadata = {'schema_version': 1, 'source': str(source), 'source_bytes': source.stat().st_size,
                'source_sha256': sha256(source)}
    skills, effects = destination / 'skill_data.json', destination / ASSET
    write_json(skills, {**metadata, **export_records(source.read_bytes(), FORMATS['skill.dat'])})
    annotate(skills, effects)
    rows = [r for r in manifest['assets'] if r['asset'] != ASSET]
    skill = next(r for r in rows if r['asset'] == 'skill.dat')
    skill.update(origin=origin, **metadata, output_bytes=skills.stat().st_size, output_sha256=sha256(skills))
    rows.append(entry(skill, effects))
    manifest['assets'] = rows
    write_json(manifest_path, manifest)

if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--overwrite', action='store_true', help='refresh existing source-derived skill outputs')
    args = parser.parse_args()
    if not args.overwrite:
        parser.error('--overwrite is required to refresh existing exports')
    refresh(args.output)
